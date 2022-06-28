// Copyright (c) 2019 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_spdy

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
)

import (
	"github.com/baidu/go-lib/gotrack"
	"github.com/baidu/go-lib/log"
)

import (
	http "github.com/bfenetworks/bfe/bfe_http"
	tls "github.com/bfenetworks/bfe/bfe_tls"
	"github.com/bfenetworks/bfe/bfe_util/pipe"
)

func (sc *serverConn) processFrame(f Frame) error {
	sc.serveG.Check()

	switch f := f.(type) {
	case *SynStreamFrame:
		return sc.processSynStream(f)
	case *DataFrame:
		return sc.processData(f)
	case *WindowUpdateFrame:
		return sc.processWindowUpdate(f)
	case *RstStreamFrame:
		return sc.processResetStream(f)
	case *SettingsFrame:
		return sc.processSettings(f)
	case *PingFrame:
		return sc.processPing(f)
	default:
		state.SpdyUnknownFrame.Inc(1)
		log.Logger.Debug("bfe_spdy: Ignoring frame: %v", f)
		return nil
	}
}

func (sc *serverConn) processPing(f *PingFrame) error {
	sc.serveG.Check()
	if f.Id%2 == 0 {
		return nil
	}
	sc.writeFrame(frameWriteMsg{frame: f})
	return nil
}

func (sc *serverConn) processWindowUpdate(f *WindowUpdateFrame) error {
	sc.serveG.Check()
	switch {
	case f.StreamId != 0: // stream-level flow control
		st := sc.streams[uint32(f.StreamId)]
		if st == nil {
			// "A sender should ignore all the WINDOW_UPDATE frames
			// associated with the stream after it send the last frame
			// for the stream, see Section 2.6.8"
			return nil
		}
		if !st.flow.add(int32(f.DeltaWindowSize)) {
			state.SpdyErrFlowControl.Inc(1)
			return StreamError{uint32(f.StreamId), FlowControlError}
		}
	default: // connection-level flow control
		if !sc.flow.add(int32(f.DeltaWindowSize)) {
			state.SpdyErrFlowControl.Inc(1)
			return goAwayFlowError{}
		}
	}
	sc.scheduleFrameWrite()
	return nil
}

func (sc *serverConn) processResetStream(f *RstStreamFrame) error {
	sc.serveG.Check()

	streamState, st := sc.state(uint32(f.StreamId))
	if streamState == stateIdle {
		// RST_STREAM frames MUST NOT be sent for a
		// stream in the "idle" state. If a RST_STREAM frame
		// identifying an idle stream is received, the
		// recipient MUST treat this as a connection error
		// of type PROTOCOL_ERROR.
		return ConnectionError(ProtocolError)
	}
	if st != nil {
		state.SpdyErrGotReset.Inc(1)
		st.gotReset = true
		sc.closeStream(st, StreamError{uint32(f.StreamId), f.Status})
	}
	return nil
}

func (sc *serverConn) processSettings(f *SettingsFrame) error {
	sc.serveG.Check()
	for _, setting := range f.FlagIdValues {
		if err := sc.processSetting(setting); err != nil {
			return err
		}
	}
	return nil
}

func (sc *serverConn) processSetting(s SettingsFlagIdValue) error {
	sc.serveG.Check()
	log.Logger.Debug("bfe_spdy: processing setting %v", s)
	switch s.Id {
	case SettingsInitialWindowSize:
		return sc.processSettingInitialWindowSize(s.Value)
	default:
		// ignore unknown setting
	}
	return nil
}

func (sc *serverConn) processSettingInitialWindowSize(val uint32) error {
	sc.serveG.Check()
	// Note: val already validated to be within range by
	// processSetting's Valid call.

	// "A SETTINGS frame can alter the initial flow control window
	// size for all current streams. When the value of
	// SETTINGS_INITIAL_WINDOW_SIZE changes, a receiver MUST
	// adjust the size of all stream flow control windows that it
	// maintains by the difference between the new value and the
	// old value. See Section 2.6.8"
	old := sc.initialWindowSize
	sc.initialWindowSize = int32(val)
	growth := sc.initialWindowSize - old // may be negative
	for _, st := range sc.streams {
		if !st.flow.add(growth) {
			// "If a sender receivers a WINDOW_UPDATE that causes a
			// flow control window to exceed this maximum it MUST
			// terminate either the stream or the connection, as
			// appropriate. see Section 2.6.8"
			// Note: We just terminate connection here.
			state.SpdyErrFlowControl.Inc(1)
			return ConnectionError(FlowControlError)
		}
	}
	return nil
}

func (sc *serverConn) processData(f *DataFrame) error {
	sc.serveG.Check()

	// If a DATA frame is received whose stream is not in "open"
	// or "half closed (local)" state, the recipient MUST respond
	// with a stream error of type STREAM_CLOSED.
	id := uint32(f.StreamId)
	st, ok := sc.streams[id]
	if !ok {
		state.SpdyErrInvalidDataStream.Inc(1)
		return StreamError{id, InvalidStream}
	}
	if st.state != stateOpen {
		// This includes sending a RST_STREAM if the stream is
		// in stateHalfClosedLocal (which currently means that
		// the http.Handler returned, so it's done reading &
		// done writing). Try to stop the client from sending
		// more DATA.
		state.SpdyErrStreamAlreadyClosed.Inc(1)
		return StreamError{id, StreamAlreadyClosed}
	}
	if st.body == nil {
		panic("internal error: should have a body in this state")
	}
	data := f.Data

	// Sender sending more than they'd declared?
	if st.declBodyBytes != -1 && st.bodyBytes+int64(len(data)) > st.declBodyBytes {
		// "If a server receives a request where the sum of the data frame
		// payload lengths does not equal the size of the Content-Length
		// header, the server MUST return a 400 (Bad Request) error, see
		// Section 3.2.1"
		// Note: we just treat that as a stream error here
		state.SpdyErrBadRequest.Inc(1)
		st.body.CloseWithError(fmt.Errorf("sender tried to send more than declared Content-Length of %d bytes", st.declBodyBytes))
		return StreamError{id, ProtocolError}
	}
	if len(data) > 0 {
		// Check whether the client has flow control quota.
		if int(st.inflow.available()) < len(data) {
			state.SpdyErrFlowControl.Inc(1)
			return StreamError{id, FlowControlError}
		}
		st.inflow.take(int32(len(data)))
		wrote, err := st.body.Write(data)
		if err != nil {
			state.SpdyErrStreamAlreadyClosed.Inc(1)
			return StreamError{id, StreamAlreadyClosed}
		}
		if wrote != len(data) {
			panic("internal error: bad Writer")
		}
		st.bodyBytes += int64(len(data))
	}
	if f.StreamEnded() {
		if t := st.timeoutTimer; t != nil {
			t.Stop()
		}

		if st.declBodyBytes != -1 && st.declBodyBytes != st.bodyBytes {
			state.SpdyErrBadRequest.Inc(1)
			st.body.CloseWithError(fmt.Errorf("request declared a Content-Length of %d but only wrote %d bytes",
				st.declBodyBytes, st.bodyBytes))
			return StreamError{id, ProtocolError}
		}
		st.body.CloseWithError(io.EOF)
		st.state = stateHalfClosedRemote
	}
	return nil
}

func (sc *serverConn) processSynStream(f *SynStreamFrame) error {
	sc.serveG.Check()
	id := uint32(f.StreamId)
	if sc.inGoAway {
		// Ignore.
		return nil
	}

	// check request rate limit
	if !acceptRequest() {
		state.SpdyReqOverload.Inc(1)
		sc.goAway(GoAwayOK)
	}

	if id%2 != 1 || id < sc.maxStreamID {
		// "If the client is initiating the stream, the Stream-ID must
		// be even. [...] The stream-id MUST increase with each new stream.
		// If an endpoint receives a SYN_STREAM with a stream id which is
		// less than any previously received SYN_STREAM, it MUST issue a
		// session error with the status PROTOCOL_ERROR. See Section 2.3.2"
		state.SpdyErrInvalidSynStream.Inc(1)
		return ConnectionError(ProtocolError)
	}
	if id == sc.maxStreamID {
		// "If a recipient receives a second SYN_STREAM for the same stream,
		// it MUST issue a stream error Section (2.4.2) with the status
		// code PROTOCOL_ERROR. See Section 2.3.2"
		state.SpdyErrInvalidSynStream.Inc(1)
		return StreamError{id, ProtocolError}
	}

	if id > sc.maxStreamID {
		sc.maxStreamID = id
	}
	st := &stream{
		id:     id,
		state:  stateOpen,
		weight: f.Priority,
	}
	if f.StreamEnded() {
		st.state = stateHalfClosedRemote
	}
	st.cw.Init()

	st.flow.conn = &sc.flow // link to conn-level counter
	st.flow.add(sc.initialWindowSize)
	st.inflow.conn = &sc.inflow      // link to conn-level counter
	st.inflow.add(initialWindowSize) // TODO: update this when we send a higher initial window size in the initial settings

	sc.streams[id] = st
	sc.curOpenStreams++
	if sc.curOpenStreams > sc.advMaxStreams {
		state.SpdyErrMaxStreamPerConn.Inc(1)
		return fmt.Errorf("user-agent[%s] curOpenStreams[%d] exceeds maxCurStreams[%d]",
			f.Headers.Get("user-agent"), sc.curOpenStreams, sc.advMaxStreams)
	}
	if sc.curOpenStreams == 1 {
		sc.setConnState(http.StateActive)
	}

	rw, req, err := sc.newWriterAndRequest(st, f)
	if err != nil {
		return err
	}
	st.body = req.Body.(*RequestBody).pipe // may be nil
	st.declBodyBytes = req.ContentLength

	handler := sc.handler.ServeHTTP
	go sc.runHandler(rw, req, handler)
	return nil
}

func (sc *serverConn) newWriterAndRequest(st *stream, f *SynStreamFrame) (
	*responseWriter, *http.Request, error) {
	sc.serveG.Check()

	header := f.Headers
	method := header.Get(headerMethod)
	path := header.Get(headerPath)
	version := header.Get(headerVersion)
	host := header.Get(headerHost)
	scheme := header.Get(headerScheme)

	if method == "" || path == "" || version == "" || host == "" || (scheme != "https" && scheme != "http") {
		// "If a client send a SYN_STREAM without all the method, host, path,
		// scheme, and version headers, the server MUST reply with a HTTP 400
		// Bad Request reply, see Section 3.2.1"
		// Note: we just treat malformed requests as a stream error of
		// PROTOCOL_ERROR here.
		state.SpdyErrBadRequest.Inc(1)
		return nil, nil, StreamError{st.id, ProtocolError}
	}
	bodyOpen := st.state == stateOpen
	if method == "HEAD" && bodyOpen {
		// HEAD requests can't have bodies
		state.SpdyErrBadRequest.Inc(1)
		return nil, nil, StreamError{st.id, ProtocolError}
	}
	var tlsState *tls.ConnectionState // nil if not scheme https
	if scheme == "https" {
		tlsState = sc.tlsState
	}
	needsContinue := header.Get("Expect") == "100-continue"
	if needsContinue {
		header.Del("Expect")
	}
	// Merge Cookie headers into one "; "-delimited value.
	if cookies := header["Cookie"]; len(cookies) > 1 {
		header.Set("Cookie", strings.Join(cookies, "; "))
	}
	body := &RequestBody{
		conn:          sc,
		stream:        st,
		needsContinue: needsContinue,
	}
	url, err := url.ParseRequestURI(path)
	if err != nil {
		state.SpdyErrBadRequest.Inc(1)
		return nil, nil, StreamError{st.id, ProtocolError}
	}

	// remove pesudo headers
	header.Del(headerMethod)
	header.Del(headerPath)
	header.Del(headerVersion)
	header.Del(headerHost)
	header.Del(headerScheme)
	header.Set("Host", host)

	req := &http.Request{
		Method:     method,
		URL:        url,
		RemoteAddr: sc.remoteAddrStr,
		Header:     header,
		RequestURI: path,
		Proto:      version,
		ProtoMajor: 1,
		ProtoMinor: 1,
		TLS:        tlsState,
		Host:       host,
		Body:       body,
		State: &http.RequestState{
			SerialNumber: st.id/2 + 1,
			StartTime:    time.Now(),
		},
	}
	if bodyOpen {
		if vv, ok := header["Content-Length"]; ok {
			// Any Content-Length greater than or equal to zero is a valid
			// value. See HTTP section 14.13
			len, err := strconv.ParseInt(vv[0], 10, 64)
			if len < 0 || err != nil {
				state.SpdyErrBadRequest.Inc(1)
				return nil, nil, StreamError{st.id, ProtocolError}
			}
			req.ContentLength = len
		} else {
			req.ContentLength = -1
		}
		body.pipe = pipe.NewPipeFromBufferPool(&fixBufferPool)
	}

	rws := responseWriterStatePool.Get().(*responseWriterState)
	bwSave := rws.bw
	*rws = responseWriterState{} // zero all the fields
	rws.conn = sc
	rws.bw = bwSave
	rws.bw.Reset(chunkWriter{rws})
	rws.stream = st
	rws.req = req
	rws.body = body

	rw := &responseWriter{rws: rws}
	return rw, req, nil
}

// Run on its own goroutine.
func (sc *serverConn) runHandler(rw *responseWriter, req *http.Request, handler func(http.ResponseWriter, *http.Request)) {
	defer func() {
		if e := recover(); e != nil {
			state.SpdyPanicStream.Inc(1)
			log.Logger.Warn("bfe_spdy: panic serving %v:%v\n%s", sc.conn.RemoteAddr(), e, gotrack.CurrentStackTrace(0))

			sc.writeFrameFromHandler(frameWriteMsg{
				frame:  &PanicFrame{},
				stream: rw.rws.stream,
			})
			return
		}

		rw.handlerDone()
	}()

	handler(rw, req)
}
