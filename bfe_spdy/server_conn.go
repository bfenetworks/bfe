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

// spdy connection for server side

package bfe_spdy

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
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

const (
	handlerChunkWriteSize = 4 << 10
	// https://www.chromium.org/spdy/spdy-protocol/spdy-protocol-draft3-1
	// 2.6.4 SETTINGS, For implementors it is recommended that this value be no smaller than 100
	defaultMaxStreams             = 200
	defaultReadClientAgainTimeout = 60 * time.Second
)

var (
	errClientDisconnected = errors.New("client disconnected")
	errClosedBody         = errors.New("body closed by handler")
	errHandlerComplete    = errors.New("spdy: request body closed due to handler exiting")
	errHandlerPanic       = errors.New("spdy: request handler goroutine panic")
	errStreamClosed       = errors.New("spdy: stream closed")
)

// fix buffer for recv window
var fixBufferPool = sync.Pool{
	New: func() interface{} {
		buffer := make([]byte, initialWindowSize)
		return pipe.NewFixedBuffer(buffer)
	},
}

var (
	testHookOnPanicMu *sync.Mutex // nil except in tests
	testHookOnPanic   func(sc *serverConn, panicVal interface{}) (rePanic bool)
)

type Server struct {
	// MaxConcurrentStreams optionally specifies the number of
	// concurrent streams that each client may have open at a
	// time. This is unrelated to the number of http.Handler goroutines
	// which may be active globally, which is MaxHandlers.
	// If zero, MaxConcurrentStreams defaults to at least 100, per
	// the SPDY spec's recommendations.
	MaxConcurrentStreams uint32

	// MaxReadFrameSize optionally specifies the largest frame
	// this server is willing to read. A valid value is between
	// 16k and 16M, inclusive. If zero or otherwise invalid, a
	// default value is used.
	MaxReadFrameSize uint32
}

func (s *Server) maxReadFrameSize() uint32 {
	if v := s.MaxReadFrameSize; v >= MinMaxFrameSize && v <= MaxFrameSize {
		return v
	}
	return defaultMaxReadFrameSize
}

func (s *Server) maxConcurrentStreams() uint32 {
	if v := s.MaxConcurrentStreams; v > 0 {
		return v
	}
	return defaultMaxStreams
}

// NewProtoHandler creates TLS application level protocol handler for spdy
func NewProtoHandler(conf *Server) func(*http.Server, *tls.Conn, http.Handler) {
	if conf == nil {
		conf = new(Server)
	}

	protoHandler := func(hs *http.Server, c *tls.Conn, h http.Handler) {
		// create and initial server conn
		if sc := conf.handleConn(hs, c, h); sc != nil {
			// check conn rate limit
			if !acceptConn() {
				state.SpdyConnOverload.Inc(1)
				sc.rejectConn("spdy overload")
				return
			}

			// process server conn
			sc.serve()
		}
	}
	return protoHandler
}

func SetConnTimeout(body *RequestBody, d time.Duration) {
	sc := body.conn
	sc.serveG.CheckNotOn() // NOT on serve goroutine
	select {
	// just send timeout value to chan
	case sc.timeoutValueCh <- timeoutValueElem{
		tag:      ConnTag,
		rb:       body,
		duration: d,
	}:
	case <-sc.doneServing:
		return
	}
}

func SetReadStreamTimeout(body *RequestBody, d time.Duration) {
	setStreamTimeout(ReadStreamTag, body, d)
}

func SetWriteStreamTimeout(body *RequestBody, d time.Duration) {
	setStreamTimeout(WriteStreamTag, body, d)
}

func setStreamTimeout(tag timeoutTag, body *RequestBody, d time.Duration) {
	sc := body.conn
	sc.serveG.CheckNotOn() // NOT on serve goroutine
	select {
	// just send timeout value to chan
	case sc.timeoutValueCh <- timeoutValueElem{
		tag:      tag,
		rb:       body,
		duration: d,
	}:
	case <-sc.doneServing:
		return
	}
}

func (sc *serverConn) maxHeaderUriSize() uint32 {
	n := sc.hs.MaxHeaderUriBytes
	if n <= 0 {
		n = http.DefaultMaxHeaderUriBytes
	}

	return uint32(n)
}

func (srv *Server) handleConn(hs *http.Server, c net.Conn, h http.Handler) *serverConn {
	sc := &serverConn{
		srv:               srv,
		hs:                hs,
		conn:              c,
		remoteAddrStr:     c.RemoteAddr().String(),
		bw:                newBufferedWriter(c),
		handler:           h,
		streams:           make(map[uint32]*stream),
		recvChan:          make(chan readFrameResult),
		sendChan:          make(chan frameWriteMsg, 1),    // buffered; one recv in writeFrames
		wroteChan:         make(chan frameWriteResult, 1), // buffered; one send in writeFrames
		bodyReadCh:        make(chan bodyReadMsg),         // buffering doesn't matter either way
		writeMsgChan:      make(chan frameWriteMsg, 8),
		doneServing:       make(chan struct{}),
		closeNotifyCh:     hs.CloseNotifyCh,
		advMaxStreams:     srv.maxConcurrentStreams(),
		writeSched:        writeScheduler{maxFrameSize: defaultMaxWriteFrameSize},
		initialWindowSize: initialWindowSize,
		serveG:            gotrack.NewGoroutineLock(),

		readClientAgainTimeout: defaultReadClientAgainTimeout,
		timeoutEventCh:         make(chan timeoutEventElem, srv.maxConcurrentStreams()),
		timeoutValueCh:         make(chan timeoutValueElem, srv.maxConcurrentStreams()),
	}
	sc.flow.add(initialWindowSize)
	sc.inflow.add(initialWindowSize)

	fr, err := NewFramer(sc.bw, c)
	if err != nil {
		state.SpdyErrNewFramer.Inc(1)
		log.Logger.Debug("bfe_spdy: conn %s: NewFramer() err %s", sc.remoteAddrStr, err)
		return nil
	}
	fr.MaxHeaderUriSize = sc.maxHeaderUriSize()

	sc.framer = fr

	if tc, ok := c.(*tls.Conn); ok {
		sc.tlsState = new(tls.ConnectionState)
		*sc.tlsState = tc.ConnectionState()
	}
	return sc
}

type serverConn struct {
	// Immutable:
	srv           *Server
	hs            *http.Server
	conn          net.Conn
	bw            *bufferedWriter // writing to conn
	handler       http.Handler
	framer        *Framer
	doneServing   chan struct{}         // closed when serverConn.serve ends
	recvChan      chan readFrameResult  // written by serverConn.readFrames
	sendChan      chan frameWriteMsg    // from serve -> writeFrames
	wroteChan     chan frameWriteResult // from writeFrames -> serve, tickles more frame writes
	bodyReadCh    chan bodyReadMsg      // from handlers -> serve
	writeMsgChan  chan frameWriteMsg    // from handlers -> serve
	closeNotifyCh chan bool             // from outside -> serve
	flow          flow                  // conn-wide (not stream-specific) outbound flow control
	inflow        flow                  // conn-wide inbound flow control
	tlsState      *tls.ConnectionState  // shared by all handlers, like net/http
	remoteAddrStr string

	// Everything following is owned by the serve loop; use serveG.Check():
	serveG            gotrack.GoroutineLock // used to verify funcs are on serve()
	advMaxStreams     uint32                // our SETTINGS_MAX_CONCURRENT_STREAMS advertised the client
	curOpenStreams    uint32                // client's number of open streams
	maxStreamID       uint32                // max ever seen
	streams           map[uint32]*stream    // stream table
	initialWindowSize int32
	writingFrame      bool // started write frame but haven't heard back on wroteChan
	needsFrameFlush   bool // last frame write wasn't a flush
	writeSched        writeScheduler
	inGoAway          bool // we've started to or sent GOAWAY
	needToSendGoAway  bool // we need to schedule a GOAWAY frame write
	goAwayCode        GoAwayStatus
	shutdownTimerCh   <-chan time.Time // nil until used
	shutdownTimer     *time.Timer      // nil until used

	// how long to wait when there is no request processing on the connection
	// it is updated once ServeHTTP() routine found a request is finished
	// but only used when connection become idle(no left request if processing)
	readClientAgainTimeout time.Duration

	// when timeout, timer hook write to chan
	// main routine read from chan
	timeoutEventCh chan timeoutEventElem

	// when save timeout, ServeHTTP() routine write to chan
	// main routine read from chan
	timeoutValueCh chan timeoutValueElem
}

// timeout event
type timeoutEventElem struct {
	tag      timeoutTag
	streamID uint32
}

// timeout value
type timeoutValueElem struct {
	tag      timeoutTag
	rb       *RequestBody
	duration time.Duration
}

// stream represents a stream. This is the minimal metadata needed by
// the serve goroutine. Most of the actual stream state is owned by
// the http.Handler's goroutine in the responseWriter. Because the
// responseWriter's responseWriterState is recycled at the end of a
// handler, this struct intentionally has no pointer to the
// *responseWriter{,State} itself, as the Handler ending nils out the
// responseWriter's state field.
type stream struct {
	// immutable:
	id   uint32      // stream id
	body *pipe.Pipe  // non-nil if expecting DATA frames
	cw   closeWaiter // closed wait stream transitions to closed state

	// owned by serverConn's serve loop:
	bodyBytes     int64 // body bytes seen so far
	declBodyBytes int64 // or -1 if undeclared
	flow          flow  // limits writing from Handler to client
	inflow        flow  // what the client is allowed to POST/etc to us
	weight        uint8
	state         streamState
	sentReset     bool // only true once detached from streams map
	gotReset      bool // only true once detacted from streams map

	// timeout timer, used both for TimeoutReadClient and TimeoutWriteClient
	// need to stop if it is not timeout
	timeoutTimer *time.Timer
}

// readFrameResult is the message passed from readFrames goroutine to the serve goroutine.
type readFrameResult struct {
	f   Frame
	err error
}

// readFrames is the loop that reads incoming frames.
// It's run on its own goroutine.
func (sc *serverConn) readFrames() {
	for {
		f, err := sc.framer.ReadFrame()
		if err == nil && f != nil {
			if _, ok := f.(*SynStreamFrame); ok {
				// no timeout till now, cancel read timeout
				var zero time.Time
				sc.conn.SetReadDeadline(zero)
			}
		}

		select {
		case sc.recvChan <- readFrameResult{f, err}:
		case <-sc.doneServing:
			return
		}
	}
}

// frameWriteResult is the message passed from writeFrames to the serve goroutine.
type frameWriteResult struct {
	wm  frameWriteMsg // what was written (or attempted)
	err error         // result of the writeFrame call
}

// writeFrames runs in its own goroutine and writes frame
// and then reports when it's done.
// At most one frame can be added to sendChan per serverConn.
func (sc *serverConn) writeFrames() {
	var wm frameWriteMsg
	var err error
	defer sc.framer.ReleaseWriter()

	for {
		// get frame from sendChan
		select {
		case wm = <-sc.sendChan:
		case <-sc.doneServing:
			return
		}

		// write frame
		switch wm.frame.(type) {
		case *FlushFrame:
			err = sc.Flush()

		default:
			err = sc.framer.WriteFrame(wm.frame)
		}

		// report write result
		select {
		case sc.wroteChan <- frameWriteResult{wm, err}:
		case <-sc.doneServing:
			return
		}
	}
}

// Note: should not be called after serve() conn
func (sc *serverConn) rejectConn(debug string) {
	log.Logger.Info("bfe_spdy: server rejecting conn: %s", debug)
	// ignoring errors. hanging up anyway.
	// If no streams were replied to, last-good-streams-id Must be 0.
	// See Spdy Protocol(draft-mbelshe-httpbis-spdy-00) Section 2.6.6
	sc.framer.WriteFrame(&GoAwayFrame{Status: GoAwayOK})
	sc.bw.Flush()
	sc.conn.Close()
	sc.framer.ReleaseWriter()
}

func (sc *serverConn) serve() {
	sc.serveG.Check()
	defer sc.notePanic()
	defer sc.conn.Close()
	defer sc.closeAllStreamsOnConnClose()
	defer sc.stopShutdownTimer()
	defer close(sc.doneServing) // unblocks handlers trying to send

	log.Logger.Debug("bfe_spdy: SPDY connection from %v on %p", sc.conn.RemoteAddr(), sc.hs)

	// set read client timeout for the first request on connection
	sc.conn.SetReadDeadline(time.Now().Add(sc.hs.ReadTimeout))

	settings := new(SettingsFrame)
	settings.FlagIdValues = []SettingsFlagIdValue{
		{0, SettingsInitialWindowSize, uint32(sc.initialWindowSize)},
		// we don't set MaxConcurrentStreams in setting frame here, because some browser
		// such as chrome version 48.0.2564.116 (64-bit) won't recognize it, and the
		// connection will be broken.
		//{0, SettingsMaxConcurrentStreams, uint32(sc.advMaxStreams)},
	}
	sc.writeFrame(frameWriteMsg{frame: settings})

	// Get us out of the"StateNew" state.  We can't go directly to idle, though.
	// Active means we read some data and anticipate a request. We'll
	// do another Active when we get a SYN_STREAM frame.
	sc.setConnState(http.StateActive)
	sc.setConnState(http.StateIdle)

	go sc.readFrames()  // closed by defer sc.conn.Close above
	go sc.writeFrames() // closed by defer sc.conn.Close above

	for {
		select {
		case res := <-sc.recvChan:
			if !sc.processFrameFromReader(res) {
				return
			}
		case m := <-sc.bodyReadCh:
			sc.noteBodyRead(m.st, m.n)
		case wm := <-sc.writeMsgChan:
			if !sc.writeFrame(wm) {
				return
			}
		case res := <-sc.wroteChan:
			sc.wroteFrame(res)
		case <-sc.shutdownTimerCh:
			log.Logger.Debug("bfe_spdy: GOAWAY close timer fired; closing conn from %v",
				sc.conn.RemoteAddr())
			return
		case ch := <-sc.timeoutEventCh: // timeout event happens
			sc.handleTimeout(ch)
		case v := <-sc.timeoutValueCh: // get timeout value update notification
			// set timeout value
			sc.setTimeout(v)
		case <-sc.closeNotifyCh: // graceful shutdown
			log.Logger.Debug("bfe_spdy: graceful closing spdy conn from %v", sc.conn.RemoteAddr())
			sc.closeNotifyCh = nil
			sc.goAway(GoAwayOK)
		}
	}
}

// hand timeout event for stream timeout, stream timeout, rst stream
func (sc *serverConn) handleTimeout(ch timeoutEventElem) {
	tag := ch.tag
	log.Logger.Debug("bfe_spdy: %s timeout, resetting frame id[%d] from %v",
		tag.String(), ch.streamID, sc.conn.RemoteAddr())
	// stream timeout, rst the stream
	errRst := StreamError{ch.streamID, ProtocolError}
	sc.resetStream(errRst)
}

func (sc *serverConn) setTimeout(elem timeoutValueElem) {
	tag := elem.tag
	rb := elem.rb
	duration := elem.duration
	stream := rb.stream

	if sc != rb.conn {
		// there Must be some error, panic
		panic("internal error: bad request body")
	}

	if tag == ConnTag {
		// just update timeout value here
		sc.readClientAgainTimeout = duration
	}
	if (tag == ReadStreamTag && stream.body != nil) || tag == WriteStreamTag {
		// just launch timeout timer for TimeoutReadClient/TimeoutWriteClient
		stream.timeoutTimer = time.AfterFunc(duration, func() {
			select {
			// timer hook: send timeout event to chan
			case sc.timeoutEventCh <- timeoutEventElem{
				streamID: stream.id,
				tag:      tag,
			}:
			case <-sc.doneServing:
				return
			}
			if tag == ReadStreamTag {
				state.SpdyTimeoutReadStream.Inc(1)
			}
			if tag == WriteStreamTag {
				state.SpdyTimeoutWriteStream.Inc(1)
			}
		})
	}
}

// processFrameFromReader processes the serve loop's read from recvChan from the
// frame-reading goroutine.
// processFrameFromReader returns whether the connection should be kept open.
func (sc *serverConn) processFrameFromReader(res readFrameResult) bool {
	sc.serveG.Check()
	err := res.err
	if err != nil {
		//TODO: check errFrameTooLarge
		clientGone := err == io.EOF || strings.Contains(err.Error(), "use of closed network connection")
		if clientGone {
			// TODO: could we also get into this state if
			// the peer does a half close
			// (e.g. CloseWrite) because they're done
			// sending frames but they're still wanting
			// our open replies?  Investigate.
			return false
		}
	} else {
		f := res.f
		log.Logger.Debug("bfe_spdy: got frame: %#v", f)
		err = sc.processFrame(f)
		if err == nil {
			return true
		}
	}

	switch ev := err.(type) {
	case net.Error:
		if ev.Timeout() {
			state.SpdyTimeoutConn.Inc(1)
			log.Logger.Debug("bfe_spdy: conn timeout from %v, closing the conn.",
				sc.conn.RemoteAddr())
		}
		return false
	case StreamError:
		sc.resetStream(ev)
		return true
	case goAwayFlowError:
		sc.goAway(GoAwayStatus(FlowControlError))
		return true
	case ConnectionError:
		log.Logger.Debug("bfe_spdy: %v: %v", sc.conn.RemoteAddr(), ev)
		sc.goAway(GoAwayStatus(ev))
		return true // goAway will handle shutdown
	default:
		if res.err != nil {
			log.Logger.Debug("bfe_spdy: disconnecting; error reading frame from client %s: %v",
				sc.conn.RemoteAddr(), err)
		} else {
			log.Logger.Debug("bfe_spdy: disconnection due to other error: %v", err)
		}
		return false
	}
}

// writeFrame schedules a frame to write and sends it if there's nothing
// already being written.
//
// There is no pushback here (the serve goroutine never blocks). It's
// the http.Handlers that block, waiting for their previous frames to
// make it onto the wire
//
// If you're not on the serve goroutine, use writeFrameFromHandler instead.
//
// writeFrame returns whether the connection should be kept open.
func (sc *serverConn) writeFrame(wm frameWriteMsg) bool {
	sc.serveG.Check()

	// process special frame
	switch wm.frame.(type) {
	case *PanicFrame:
		sc.closeStream(wm.stream, errHandlerPanic)
		return true
	case *FinFrame:
		return false
	}

	sc.writeSched.add(wm)
	sc.scheduleFrameWrite()
	return true
}

// scheduleFrameWrite tickles the frame writing scheduler.
func (sc *serverConn) scheduleFrameWrite() {
	sc.serveG.Check()

	// If a frame is already being written, nothing happens. This will be called again
	// when the frame is done being written.
	if sc.writingFrame {
		return
	}

	// If a frame isn't being written we need to send one, the best frame
	// to send is selected, preferring first things that aren't
	// stream-specific (e.g. GoAway frame), and then finding the
	// highest priority stream.
	if sc.needToSendGoAway {
		sc.needToSendGoAway = false
		sc.startFrameWrite(frameWriteMsg{
			frame: &GoAwayFrame{
				LastGoodStreamId: StreamId(sc.maxStreamID),
				Status:           sc.goAwayCode,
			},
		})
		return
	}
	if !sc.inGoAway || sc.goAwayCode == GoAwayOK {
		if wm, ok := sc.writeSched.take(); ok {
			sc.startFrameWrite(wm)
			return
		}
	}

	// If a frame isn't being written and there's nothing else to send, we
	// flush the write buffer.
	if sc.needsFrameFlush {
		sc.startFrameWrite(frameWriteMsg{frame: &FlushFrame{}})
		sc.needsFrameFlush = false // after startFrameWrite, since it sets this true
		return
	}
}

// startFrameWrite starts a goroutine to write wm (in a separate
// goroutine since that might block on the network), and updates the
// serve goroutine's state about the world, updated from info in wm.
func (sc *serverConn) startFrameWrite(wm frameWriteMsg) {
	sc.serveG.Check()
	if sc.writingFrame {
		panic("internal error: can only be writing one frame at a time")
	}

	st := wm.stream
	if st != nil {
		switch st.state {
		case stateHalfClosedLocal:
			panic("internal error: attempt to send frame on half-closed-local stream")
		case stateClosed:
			if st.sentReset || st.gotReset {
				// Skip this frame.
				sc.scheduleFrameWrite()
				return
			}
			panic(fmt.Sprintf("internal error: attempt to send a write %v on a closed stream", wm))
		}
	}

	sc.writingFrame = true
	sc.needsFrameFlush = true
	sc.sendChan <- wm
}

// wroteFrame is called on the serve goroutine with the result of
// whatever happened on writeFrames.
func (sc *serverConn) wroteFrame(res frameWriteResult) {
	sc.serveG.Check()
	if !sc.writingFrame {
		panic("internal error: expected to be already writing a frame")
	}
	sc.writingFrame = false

	wm := res.wm
	st := wm.stream

	closeStream := endsStream(wm.frame)

	// Reply (if requested) to the blocked ServeHTTP goroutine.
	if ch := wm.done; ch != nil {
		select {
		case ch <- res.err:
		default:
			panic(fmt.Sprintf("unbuffered done channel passed in for type %T", wm.frame))
		}
	}

	wm.frame = nil // prevent use (assume it's tainted after wm.done send)

	if closeStream {
		if st == nil {
			panic("internal error: expecting non-nil stream")
		}
		switch st.state {
		case stateOpen:
			// Here we would go to stateHalfClosedLocal in
			// theory, but since our handler is done and
			// the net/http package provides no mechanism
			// for finishing writing to a ResponseWriter
			// while still reading data, we go into closed
			// state here anyway, after telling the peer
			// we're hanging up on them.
			st.state = stateHalfClosedLocal // won't last long, but necessary for closeStream via resetStream
			errCancel := StreamError{st.id, Cancel}
			sc.resetStream(errCancel)
			state.SpdyErrStreamCancel.Inc(1)
		case stateHalfClosedRemote:
			sc.closeStream(st, errHandlerComplete)
		}
	}

	sc.scheduleFrameWrite()
}

// endsStream reports whether the given frame writer w will locally
// close the stream.
func endsStream(w Frame) bool {
	switch v := w.(type) {
	case *DataFrame:
		return (v.Flags & DataFlagFin) != 0
	case *SynReplyFrame:
		return (v.CFHeader.Flags & ControlFlagFin) != 0
	case nil:
		// This can only happen if the caller reuses w after it's
		// been intentionally nil'ed out to prevent use. Keep this
		// here to catch future refactoring breaking it.
		panic("endsStream called on nil writeFramer")
	}
	return false
}

func (sc *serverConn) goAway(code GoAwayStatus) {
	sc.serveG.Check()
	if sc.inGoAway {
		return
	}
	if code != GoAwayOK {
		sc.shutDownIn(250 * time.Millisecond)
	} else {
		sc.shutDownIn(sc.hs.GracefulShutdownTimeout)
	}
	sc.inGoAway = true
	sc.needToSendGoAway = true
	sc.goAwayCode = code
	sc.scheduleFrameWrite()
}

func (sc *serverConn) shutDownIn(d time.Duration) {
	sc.serveG.Check()
	sc.shutdownTimer = time.NewTimer(d)
	sc.shutdownTimerCh = sc.shutdownTimer.C
}

func (sc *serverConn) resetStream(se StreamError) {
	sc.serveG.Check()
	sc.writeFrame(frameWriteMsg{
		frame: &RstStreamFrame{
			StreamId: StreamId(se.StreamID),
			Status:   se.Code,
		},
	})
	if st, ok := sc.streams[se.StreamID]; ok {
		st.sentReset = true
		sc.closeStream(st, se)
	}
}

func (sc *serverConn) CloseConn() error {
	return sc.conn.Close()
}

func (sc *serverConn) Flush() error {
	return sc.bw.Flush()
}

func (sc *serverConn) closeStream(st *stream, err error) {
	sc.serveG.Check()
	if st.state == stateIdle || st.state == stateClosed {
		panic(fmt.Sprintf("invariant; can't close stream in state %v", st.state))
	}
	if t := st.timeoutTimer; t != nil {
		t.Stop()
	}

	st.state = stateClosed
	sc.curOpenStreams--
	if sc.curOpenStreams == 0 {
		// no request processing on the conn, set read client again timeout
		sc.setReadClientAgainTimeout()
		sc.setConnState(http.StateIdle)
	}
	delete(sc.streams, st.id)
	if p := st.body; p != nil {
		p.CloseWithError(err)
		p.Release(&fixBufferPool)
	}
	st.cw.Close() // signals Handler's CloseNotifier, unblocks writes, etc
	sc.writeSched.forgetStream(st.id)
}

func (sc *serverConn) setReadClientAgainTimeout() {
	t := time.Now().Add(sc.readClientAgainTimeout)
	sc.conn.SetReadDeadline(t)
}

func (sc *serverConn) closeAllStreamsOnConnClose() {
	sc.serveG.Check()
	for _, st := range sc.streams {
		sc.closeStream(st, errClientDisconnected)
	}
}

func (sc *serverConn) stopShutdownTimer() {
	sc.serveG.Check()
	if t := sc.shutdownTimer; t != nil {
		t.Stop()
	}
}

func (sc *serverConn) notePanic() {
	if e := recover(); e != nil {
		state.SpdyPanicConn.Inc(1)
		if testHookOnPanicMu != nil {
			testHookOnPanicMu.Lock()
			defer testHookOnPanicMu.Unlock()
		}
		if testHookOnPanic != nil {
			if testHookOnPanic(sc, e) {
				panic(e)
			}
		}
	}
}

func (sc *serverConn) state(streamID uint32) (streamState, *stream) {
	sc.serveG.Check()
	if st, ok := sc.streams[streamID]; ok {
		return st.state, st
	}
	// The first use of a new stream identifier implicitly closes all
	// streams in the "idle" state that might have been initiated by
	// that peer with a lower-valued stream identifier. For example, if
	// a client sends a SynStream frame on stream 7 without ever sending a
	// frame on stream 5, then stream 5 transitions to the "closed"
	// state when the first frame for stream 7 is sent or received.
	if streamID <= sc.maxStreamID {
		return stateClosed, nil
	}
	return stateIdle, nil
}

// setConnState calls the net/http ConnState hook for this connection, if configured.
// Note that the net/http package does StateNew and StateClosed for us.
// There is currently no plan for StateHijacked or hijacking spdy connections.
func (sc *serverConn) setConnState(state http.ConnState) {
	if sc.hs.ConnState != nil {
		sc.hs.ConnState(sc.conn, state)
	}
}

// writeHeaders writes response header to specified stream.
// Note: called from handler goroutines
func (sc *serverConn) writeHeaders(st *stream, status int, header http.Header, endStream bool) error {
	sc.serveG.CheckNotOn() // NOT on serve goroutine

	// prepare SynReplyFrame
	frame := &SynReplyFrame{StreamId: StreamId(st.id), Headers: header}
	if frame.Headers == nil {
		frame.Headers = make(http.Header)
	}
	frame.Headers.Set(":status", fmt.Sprintf("%d", status))
	frame.Headers.Set(":version", "HTTP/1.1")
	for field := range invalidRespHeaders {
		frame.Headers.Del(field)
	}
	if endStream {
		frame.CFHeader.Flags = ControlFlagFin
	}

	errc := make(chan error, 1)

	// request for writing SynReplyFrame
	if err := sc.writeFrameFromHandler(frameWriteMsg{
		frame:  frame,
		stream: st,
		done:   errc,
	}); err != nil {
		return err
	}

	// wait for write result
	select {
	case err := <-errc:
		return err
	case <-sc.doneServing:
		return errClientDisconnected
	case <-st.cw:
		return errStreamClosed
	}
}

// write100ContinueHeaders writes 100 continue response to specified stream.
// Note: called from handler goroutines
func (sc *serverConn) write100ContinueHeaders(st *stream) {
	sc.serveG.CheckNotOn() // NOT on serve goroutine

	header := make(http.Header)
	header.Set(":status", "100")
	header.Set(":version", "HTTP/1.1")
	sc.writeFrameFromHandler(frameWriteMsg{
		frame:  &SynReplyFrame{StreamId: StreamId(st.id), Headers: header},
		stream: st,
	})
}

// writeDataFromHandler writes DATA response frames from a handler on the given stream.
// Note: called from handler goroutines
func (sc *serverConn) writeDataFromHandler(stream *stream, data []byte, endStream bool) error {
	sc.serveG.CheckNotOn() // NOT on serve goroutine

	// prepare DataFrame
	frame := &DataFrame{
		StreamId: StreamId(stream.id),
		Data:     data,
	}
	if endStream {
		frame.Flags = DataFlagFin
	}
	errc := make(chan error, 1)

	// request for writing DataFrame
	err := sc.writeFrameFromHandler(frameWriteMsg{
		frame:  frame,
		stream: stream,
		done:   errc,
	})
	if err != nil {
		return err
	}

	// wait for write result
	select {
	case err = <-errc:
		return err
	case <-sc.doneServing:
		return errClientDisconnected
	case <-stream.cw:
		// If both ch and stream.cw were ready (as might
		// happen on the final Write after an http.Handler
		// ends), prefer the write result. Otherwise this
		// might just be us successfully closing the stream.
		// The writeFrames and serve goroutines guarantee
		// that the ch send will happen before the stream.cw
		// close.
		select {
		case err = <-errc:
			return err
		default:
			return errStreamClosed
		}
	}
}

// writeFrameFromHandler sends wm to sc.writeMsgChan, but aborts
// if the connection has gone away.
//
// This must not be run from the serve goroutine itself, else it might
// deadlock writing to sc.writeMsgChan (which is only mildly
// buffered and is read by serve itself). If you're on the serve
// goroutine, call writeFrame instead.
func (sc *serverConn) writeFrameFromHandler(wm frameWriteMsg) error {
	sc.serveG.CheckNotOn() // NOT on serve goroutine
	select {
	case sc.writeMsgChan <- wm:
		return nil
	case <-sc.doneServing:
		// Serve loop is gone.
		// Client has closed their connection to the server.
		return errClientDisconnected
	}
}

// Close requests serverConn to finish
func (sc *serverConn) Close() {
	sc.serveG.CheckNotOn() // NOT on serve goroutine
	sc.writeFrameFromHandler(frameWriteMsg{frame: &FinFrame{}})
}
