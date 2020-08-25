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
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

import (
	http "github.com/bfenetworks/bfe/bfe_http"
)

type serverTester struct {
	t testing.TB

	// conn for client side
	cc        net.Conn
	fr        *Framer
	frc       chan Frame
	frErrc    chan error
	readTimer *time.Timer

	// conn for server side
	sc *serverConn
}

func newLocalListener() (net.Listener, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return l, nil
}

func newServerTester(t testing.TB, handler http.HandlerFunc) *serverTester {
	// create serverTester
	st := &serverTester{
		t:      t,
		frc:    make(chan Frame, 1),
		frErrc: make(chan error, 1),
	}

	var l net.Listener
	var err error
	var ccon net.Conn
	var scon net.Conn
	ch := make(chan net.Conn)

	if l, err = newLocalListener(); err != nil {
		t.Fatalf("Error newLocalListener(): %v", err)
	}

	go func() {
		var c net.Conn
		var err error
		if c, err = l.Accept(); err != nil {
			t.Fatalf("Error Accept(): %v", err)
		}

		ch <- c
	}()

	time.Sleep(1 * time.Millisecond)
	if ccon, err = net.Dial("tcp", l.Addr().String()); err != nil {
		t.Fatalf("Error Dial(): %v", err)
	}

	scon = <-ch

	// init client
	st.cc = ccon
	st.fr, err = NewFramer(st.cc, st.cc)
	if err != nil {
		t.Fatalf("Error NewFramer(): %v", err)
	}

	// init server
	conf := new(Server)
	conf.MaxConcurrentStreams = 2
	hs := &http.Server{ReadTimeout: 1 * time.Second}
	st.sc = conf.handleConn(hs, scon, handler)
	go st.sc.serve()

	return st
}

// greet initiates the client's SPDY connection into a state where
// frames may be sent.
func (st *serverTester) greet() error {
	var err error
	if err = st.writeInitialSettings(); err != nil {
		return err
	}
	st.wantSettings()
	return nil
}

func (st *serverTester) writeInitialSettings() error {
	return st.writeSettings(uint32(initialWindowSize))
}

func (st *serverTester) writeSettings(initWindowSize uint32) error {
	settings := new(SettingsFrame)
	settings.FlagIdValues = []SettingsFlagIdValue{
		{0, SettingsInitialWindowSize, initWindowSize},
	}
	return st.writeFrame(settings)
}

func (st *serverTester) writeSynStream(streamId uint32, header http.Header, endStream bool) error {
	f := new(SynStreamFrame)
	f.StreamId = StreamId(streamId) // clients send odd numbers
	f.Headers = make(http.Header)
	f.Headers.Set(headerMethod, "GET")
	f.Headers.Set(headerPath, "/")
	f.Headers.Set(headerVersion, "HTTP/1.1")
	f.Headers.Set(headerHost, "spdy.bfe.com")
	f.Headers.Set(headerScheme, "https")
	for k := range header {
		v := header.Get(k)
		f.Headers.Set(k, v)
	}
	if endStream {
		f.CFHeader.Flags = ControlFlagFin // no DATA frames
	}
	return st.writeFrame(f)
}

func (st *serverTester) writeData(streamId uint32, data []byte, endStream bool) error {
	d := new(DataFrame)
	d.StreamId = StreamId(streamId)
	d.Data = data
	if endStream {
		d.Flags = DataFlagFin
	}
	return st.writeFrame(d)
}

func (st *serverTester) writeWindowUpdate(streamId uint32, delta uint32) {
	f := new(WindowUpdateFrame)
	f.StreamId = StreamId(streamId)
	f.DeltaWindowSize = delta
	st.writeFrame(f)
}

func (st *serverTester) writeRstStream(streamId uint32, status RstStreamStatus) {
	f := new(RstStreamFrame)
	f.StreamId = 1
	f.Status = status
	st.writeFrame(f)
}

func (st *serverTester) writeGoAway(streamId uint32, status GoAwayStatus) error {
	f := new(GoAwayFrame)
	f.LastGoodStreamId = StreamId(streamId)
	f.Status = status
	return st.writeFrame(f)
}

func (st *serverTester) writeFrame(f Frame) error {
	return st.fr.WriteFrame(f)
	//st.t.Fatalf("Error writing Frame: %v", err)
}

// read frame until timeout
func (st *serverTester) readFrame() (Frame, error) {
	// start read frame
	go func() {
		fr, err := st.fr.ReadFrame()
		if err != nil {
			st.frErrc <- err
		} else {
			st.frc <- fr
		}
	}()

	// wait for result
	t := st.readTimer
	if t == nil {
		t = time.NewTimer(2 * time.Second)
		st.readTimer = t
	}
	t.Reset(2 * time.Second)
	defer t.Stop()
	select {
	case f := <-st.frc:
		return f, nil
	case err := <-st.frErrc:
		return nil, err
	case <-t.C:
		return nil, errors.New("timeout waiting for frame")
	}
}

func (st *serverTester) wantSynReply() (*SynReplyFrame, error) {
	f, err := st.readFrame()
	if err != nil {
		return nil, fmt.Errorf("Error while expecting a SynReply frame %s", err)
	}
	hf, ok := f.(*SynReplyFrame)
	if !ok {
		return nil, fmt.Errorf("got a %T(%#v); want *SynReplyFrame", f, f)
	}
	return hf, nil
}

func (st *serverTester) wantData() *DataFrame {
	f, err := st.readFrame()
	if err != nil {
		st.t.Fatalf("Error while expecting a DATA frame: %v", err)
	}
	df, ok := f.(*DataFrame)
	if !ok {
		st.t.Fatalf("got a %T(%#v); want *DataFrame", f, f)
	}
	return df
}

func (st *serverTester) wantSettings() *SettingsFrame {
	f, err := st.readFrame()
	if err != nil {
		st.t.Fatalf("Error while expecting a SETTINGS frame: %v", err)
	}
	sf, ok := f.(*SettingsFrame)
	if !ok {
		st.t.Fatalf("got a %T(%#v); want *SettingsFrame", f, f)
	}
	return sf
}

func (st *serverTester) wantPing() *PingFrame {
	f, err := st.readFrame()
	if err != nil {
		st.t.Fatalf("Error while expecting a PING frame: %v", err)
	}
	pf, ok := f.(*PingFrame)
	if !ok {
		st.t.Fatalf("got a %T(%#v); want *PingFrame", f, f)
	}
	return pf
}

func (st *serverTester) wantGoAway() *GoAwayFrame {
	f, err := st.readFrame()
	if err != nil {
		st.t.Fatalf("Error while expecting a GOAWAY frame: %v", err)
	}
	gf, ok := f.(*GoAwayFrame)
	if !ok {
		st.t.Fatalf("got a %T(%#v); want *GoAwayFrame", f, f)
	}
	return gf
}

func (st *serverTester) wantRstStream(streamID uint32, status RstStreamStatus) error {
	f, err := st.readFrame()
	if err != nil {
		return fmt.Errorf("Error while expecting an RstStream frame: %v", err)
	}
	rs, ok := f.(*RstStreamFrame)
	if !ok {
		return fmt.Errorf("got a %T(%#v); want *RstStreamFrame", f, f)
	}
	if uint32(rs.StreamId) != streamID {
		return fmt.Errorf("RstStream StreamId = %d; want %d", rs.StreamId, streamID)
	}
	if rs.Status != status {
		return fmt.Errorf("RstStream ErrCode = %d; want %d", rs.Status, status)
	}

	return nil
}

func (st *serverTester) wantWindowUpdate(streamID, incr uint32) {
	f, err := st.readFrame()
	if err != nil {
		st.t.Fatalf("Error while expecting a WINDOW_UPDATE frame: %v", err)
	}
	wu, ok := f.(*WindowUpdateFrame)
	if !ok {
		st.t.Fatalf("got a %T; want *WindowUpdateFrame", f)
	}
	if uint32(wu.StreamId) != streamID {
		st.t.Fatalf("WindowUpdate StreamId = %d; want %d", wu.StreamId, streamID)
	}
	if wu.DeltaWindowSize != incr {
		st.t.Fatalf("WindowUpdate increment = %d; want %d", wu.DeltaWindowSize, incr)
	}
}

func (st *serverTester) Close() {
	st.cc.Close()
	st.sc.conn.Close()
}

// testServerRequest sets up an idle SPDY connection and lets you
// write a single request with writeReq, and then verify that the
// *http.Request is built correctly in checkReq.
func testServerRequest(t *testing.T, writeReq func(*serverTester), checkReq func(*http.Request)) {
	gotReq := make(chan bool, 1)
	st := newServerTester(t,
		// check req in Server Handler
		func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil {
				t.Fatal("nil Body")
			}
			checkReq(r)
			gotReq <- true
		})
	var err error
	if err = st.greet(); err != nil {
		st.t.Fatalf("Error writing Frame: %v", err)
	}
	defer st.Close()

	// client send request
	writeReq(st)

	// wait check result
	select {
	case <-gotReq:
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for request")
	}
}

func testRejectRequest(t *testing.T, send func(*serverTester)) {
	st := newServerTester(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server request made it to handler; should've been rejected")
	})
	defer st.Close()

	var err error
	if err = st.greet(); err != nil {
		st.t.Fatalf("Error writing Frame: %v", err)
	}

	send(st)
	if err = st.wantRstStream(1, ProtocolError); err != nil {
		st.t.Fatalf("Error wantRstStream: %v", err)
	}
}

func testBodyContents(t *testing.T, wantContentLength int64, wantBody string, write func(st *serverTester)) {
	testServerRequest(t, write, func(r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %q; want POST", r.Method)
		}
		if r.ContentLength != wantContentLength {
			t.Errorf("ContentLength = %v; want %d", r.ContentLength, wantContentLength)
		}
		all, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(all) != wantBody {
			t.Errorf("Read = %q; want %q", all, wantBody)
		}
		if err := r.Body.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})
}

func testBodyContentsFail(t *testing.T, wantContentLength int64, wantReadError string, write func(st *serverTester)) {
	testServerRequest(t, write, func(r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %q; want POST", r.Method)
		}
		if r.ContentLength != wantContentLength {
			t.Errorf("ContentLength = %v; want %d", r.ContentLength, wantContentLength)
		}
		all, err := ioutil.ReadAll(r.Body)
		if err == nil {
			t.Fatalf("expected an error (%q) reading from the body. Successfully read %q instead.",
				wantReadError, all)
		}
		if !strings.Contains(err.Error(), wantReadError) {
			t.Fatalf("Body.Read = %v; want substring %q", err, wantReadError)
		}
		if err := r.Body.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})
}

// testServerPostUnblock sends a hanging POST with unsent data to handler,
// then runs fn once in the handler, and verifies that the error returned from
// handler is acceptable. It fails if takes over 5 seconds for handler to exit.
func testServerPostUnblock(t *testing.T,
	handler func(http.ResponseWriter, *http.Request) error,
	fn func(*serverTester),
	checkErr func(error),
	otherHeaders ...string) {

	inHandler := make(chan bool)
	errc := make(chan error, 1)
	st := newServerTester(t, func(w http.ResponseWriter, r *http.Request) {
		inHandler <- true
		errc <- handler(w, r)
	})

	// send post request
	var err error
	if err = st.greet(); err != nil {
		st.t.Fatalf("Error writing Frame: %v", err)
	}

	headers := make(http.Header)
	headers.Set(":method", "POST")
	for i := 0; i < len(otherHeaders); i += 2 {
		headers.Set(otherHeaders[i], otherHeaders[i+1])
	}
	st.writeSynStream(1, headers, false)

	<-inHandler
	fn(st)

	// wait for server handler error
	select {
	case err := <-errc:
		if checkErr != nil {
			checkErr(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Handler to return")
	}
	st.Close()
}

// testServerResponse sets up an idle SPDY connection and lets you
// write a single request with writeReq, and then reply to it in some way with the provided handler,
// and then verify the output with the serverTester again (assuming the handler returns nil)
func testServerResponse(t testing.TB,
	handler func(http.ResponseWriter, *http.Request) error,
	client func(*serverTester),
) {
	errc := make(chan error, defaultMaxStreams)
	st := newServerTester(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			t.Fatal("nil Body")
		}
		errc <- handler(w, r)
	})
	defer st.Close()

	// client send request
	donec := make(chan bool)
	go func() {
		defer close(donec)
		var err error
		if err = st.greet(); err != nil {
			st.t.Fatalf("Error writing Frame: %v", err)
		}
		client(st)
	}()

	select {
	case <-donec:
		return
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}

	// verify output of server handler
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("Error in handler: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for handler to finish")
	}
}

// testServerRejects tests that the server hangs up with a GOAWAY
// frame and a server close after the client does something
// deserving a CONNECTION_ERROR.
func testServerRejects(t *testing.T, writeReq func(*serverTester)) {
	st := newServerTester(t, func(w http.ResponseWriter, r *http.Request) {})
	defer st.Close()

	// client send request
	var err error
	if err = st.greet(); err != nil {
		st.t.Fatalf("Error writing Frame: %v", err)
	}

	writeReq(st)

	// wait for GoAway
	st.wantGoAway()

	// wait for connection closed
	errc := make(chan error, 1)
	go func() {
		fr, err := st.fr.ReadFrame()
		if err == nil {
			err = fmt.Errorf("got frame of type %T", fr)
		}
		errc <- err
	}()
	select {
	case err := <-errc:
		if err != io.EOF {
			t.Errorf("ReadFrame = %v; want io.EOF", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for disconnect")
	}
}

func TestServer_Request(t *testing.T) {
	gotReq := make(chan bool, 1)
	st := newServerTester(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Foo", "Bar")
		gotReq <- true
	})
	defer st.Close()

	var err error
	if err = st.writeInitialSettings(); err != nil {
		st.t.Fatalf("Error writing Frame: %v", err)
	}
	st.wantSettings()
	st.writeSynStream(1, nil, true)

	select {
	case <-gotReq:
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for request")
	}
}

func TestServer_Request_Get(t *testing.T) {
	testServerRequest(t, func(st *serverTester) {
		headers := make(http.Header)
		headers.Set(":host", "example.com")
		headers.Set("foo-bar", "some-value")
		st.writeSynStream(1, headers, true)
	}, func(r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Method = %q; want GET", r.Method)
		}
		if r.URL.Path != "/" {
			t.Errorf("URL.Path = %q; want /", r.URL.Path)
		}
		if r.ContentLength != 0 {
			t.Errorf("ContentLength = %v; want 0", r.ContentLength)
		}
		if r.Close {
			t.Error("Close = true; want false")
		}
		if r.Proto != "HTTP/1.1" || r.ProtoMajor != 1 || r.ProtoMinor != 1 {
			t.Errorf("Proto = %q Major=%v,Minor=%v; want HTTP/1.1", r.Proto, r.ProtoMajor, r.ProtoMinor)
		}
		wantHeader := http.Header{
			"Foo-Bar": []string{"some-value"},
			"Host":    []string{"example.com"},
		}
		if !reflect.DeepEqual(r.Header, wantHeader) {
			t.Errorf("Header = %#v; want %#v", r.Header, wantHeader)
		}
		if n, err := r.Body.Read([]byte(" ")); err != io.EOF || n != 0 {
			t.Errorf("Read = %d, %v; want 0, EOF", n, err)
		}
	})
}

func TestServer_Request_Post_NoContentLength_EndStream(t *testing.T) {
	testServerRequest(t, func(st *serverTester) {
		headers := make(http.Header)
		headers.Set(":method", "POST")
		st.writeSynStream(1, headers, true)
	}, func(r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %q; want POST", r.Method)
		}
		if r.ContentLength != 0 {
			t.Errorf("ContentLength = %v; want 0", r.ContentLength)
		}
		if n, err := r.Body.Read([]byte(" ")); err != io.EOF || n != 0 {
			t.Errorf("Read = %d, %v; want 0, EOF", n, err)
		}
	})
}

func TestServer_Request_Post_Body_ImmediateEOF(t *testing.T) {
	testBodyContents(t, -1, "", func(st *serverTester) {
		headers := make(http.Header)
		headers.Set(":method", "POST")
		st.writeSynStream(1, headers, false)

		// just kidding. empty body.
		data := make([]byte, 0)
		st.writeData(1, data, true)
	})
}

func TestServer_Request_Post_Body_OneData(t *testing.T) {
	const content = "Some content"
	testBodyContents(t, -1, content, func(st *serverTester) {
		headers := make(http.Header)
		headers.Set(":method", "POST")
		st.writeSynStream(1, headers, false)

		data := []byte(content)
		st.writeData(1, data, true)
	})
}

func TestServer_Request_Post_Body_TwoData(t *testing.T) {
	const content = "Some content"
	testBodyContents(t, -1, content, func(st *serverTester) {
		headers := make(http.Header)
		headers.Set(":method", "POST")
		st.writeSynStream(1, headers, false)

		st.writeData(1, []byte(content[:5]), false)
		st.writeData(1, []byte(content[5:]), true)
	})
}

func TestServer_Request_Post_Body_ContentLength_Correct(t *testing.T) {
	const content = "Some content"
	testBodyContents(t, int64(len(content)), content, func(st *serverTester) {
		headers := make(http.Header)
		headers.Set(":method", "POST")
		headers.Set("content-length", strconv.Itoa(len(content)))
		st.writeSynStream(1, headers, false)

		st.writeData(1, []byte(content), true)
	})
}

func TestServer_Request_Post_Body_ContentLength_TooLarge(t *testing.T) {
	testBodyContentsFail(t, 3, "request declared a Content-Length of 3 but only wrote 2 bytes",
		func(st *serverTester) {
			headers := make(http.Header)
			headers.Set(":method", "POST")
			headers.Set("content-length", "3")
			st.writeSynStream(1, headers, false)

			st.writeData(1, []byte("12"), true)
		})
}

func TestServer_Request_Post_Body_ContentLength_TooSmall(t *testing.T) {
	testBodyContentsFail(t, 4, "sender tried to send more than declared Content-Length of 4 bytes",
		func(st *serverTester) {
			headers := make(http.Header)
			headers.Set(":method", "POST")
			headers.Set("content-length", "4")
			st.writeSynStream(1, headers, false)

			st.writeData(1, []byte("12345"), true)
		})
}

func newReqHeader(headers ...string) http.Header {
	h := make(http.Header)
	h.Set(":method", "GET")
	h.Set(":host", "spdy.bfe.com")
	h.Set(":path", "/")
	h.Set(":version", "HTTP/1.1")
	h.Set(":schema", "https")
	for i := 0; i < len(headers); i += 2 {
		h.Set(headers[i], headers[i+1])
	}
	return h
}

func TestServer_Request_Get_Host(t *testing.T) {
	const host = "example.com"
	testServerRequest(t, func(st *serverTester) {
		headers := newReqHeader(":host", host)
		st.writeSynStream(1, headers, true)
	}, func(r *http.Request) {
		if r.Host != host {
			t.Errorf("Host = %q; want %q", r.Host, host)
		}
	})
}

func TestServer_Ping(t *testing.T) {
	st := newServerTester(t, nil)
	defer st.Close()
	var err error
	if err = st.greet(); err != nil {
		st.t.Fatalf("Error writing Frame: %v", err)
	}

	// Server should ignore this one, since it is ping ack.
	ackPing := &PingFrame{Id: 128}
	if err := st.fr.WriteFrame(ackPing); err != nil {
		t.Fatal(err)
	}

	// But the server should reply to this one.
	ping := &PingFrame{Id: 1}
	if err := st.fr.WriteFrame(ping); err != nil {
		t.Fatal(err)
	}

	pf := st.wantPing()
	if pf.Id != 1 {
		t.Errorf("response ping id %d; want 1", pf.Id)
	}
}

func TestServer_Handler_Sends_WindowUpdate(t *testing.T) {
	puppet := newHandlerPuppet()
	st := newServerTester(t, func(w http.ResponseWriter, r *http.Request) {
		puppet.act(w, r)
	})
	defer st.Close()
	defer puppet.done()

	var err error
	if err = st.greet(); err != nil {
		st.t.Fatalf("Error writing Frame: %v", err)
	}

	headers := make(http.Header)
	headers.Set("foo-bar", "some-value")
	st.writeSynStream(1, headers, false)

	st.writeData(1, []byte("abcdef"), false)
	puppet.do(readBodyHandler(t, "abc"))
	st.wantWindowUpdate(0, 3)
	st.wantWindowUpdate(1, 3)

	puppet.do(readBodyHandler(t, "def"))
	st.wantWindowUpdate(0, 3)
	st.wantWindowUpdate(1, 3)

	st.writeData(1, []byte("ghijkl"), true)
	puppet.do(readBodyHandler(t, "ghi"))
	puppet.do(readBodyHandler(t, "jkl"))
	st.wantWindowUpdate(0, 3)
	st.wantWindowUpdate(0, 3) // no more stream-level, since END_STREAM
}

func TestServer_Send_GoAway_After_Bogus_WindowUpdate(t *testing.T) {
	st := newServerTester(t, nil)
	defer st.Close()

	var err error
	if err = st.greet(); err != nil {
		st.t.Fatalf("Error writing Frame: %v", err)
	}

	st.writeWindowUpdate(0, 1<<31-1)

	gf := st.wantGoAway()
	if uint32(gf.Status) != uint32(FlowControlError) {
		t.Errorf("GOAWAY err = %v; want %v", gf.Status, FlowControlError)
	}
	if gf.LastGoodStreamId != 0 {
		t.Errorf("GOAWAY last stream ID = %v; want %v", gf.LastGoodStreamId, 0)
	}
}

func TestServer_RstStream_Unblocks_Read(t *testing.T) {
	testServerPostUnblock(t,
		func(w http.ResponseWriter, r *http.Request) (err error) {
			_, err = r.Body.Read(make([]byte, 1))
			return
		},
		func(st *serverTester) {
			st.writeRstStream(1, Cancel)
		},
		func(err error) {
			if err == nil {
				t.Error("unexpected nil error from Request.Body.Read")
			}
		},
	)
}

func TestServer_DeadConn_Unblocks_Read(t *testing.T) {
	testServerPostUnblock(t,
		func(w http.ResponseWriter, r *http.Request) (err error) {
			_, err = r.Body.Read(make([]byte, 1))
			return
		},
		func(st *serverTester) { st.cc.Close() },
		func(err error) {
			if err == nil {
				t.Error("unexpected nil error from Request.Body.Read")
			}
		},
	)
}

var blockUntilClosed = func(w http.ResponseWriter, r *http.Request) error {
	<-w.(http.CloseNotifier).CloseNotify()
	return nil
}

func TestServer_CloseNotify_After_RstStream(t *testing.T) {
	testServerPostUnblock(t, blockUntilClosed, func(st *serverTester) {
		st.writeRstStream(1, Cancel)
	}, nil)
}

func TestServer_CloseNotify_After_ConnClose(t *testing.T) {
	testServerPostUnblock(t, blockUntilClosed, func(st *serverTester) { st.cc.Close() }, nil)
}

// that CloseNotify unblocks after a stream error due to the client's
// problem that's unrelated to them explicitly canceling it (which is
// TestServer_CloseNotify_After_RstStream above)
func TestServer_CloseNotify_After_StreamError(t *testing.T) {
	testServerPostUnblock(t, blockUntilClosed, func(st *serverTester) {
		// data longer than declared Content-Length => stream error
		st.writeData(1, []byte("12345"), true)
	}, nil, "content-length", "3")
}

//writes a SynStream frames with StreamId 1 and EndStream set.
func getSlash(st *serverTester) error {
	return st.writeSynStream(1, nil, true)
}

func newResHeader(headers ...string) http.Header {
	h := make(http.Header)
	h.Set(":version", "HTTP/1.1")
	h.Set(":status", "200")
	for i := 0; i < len(headers); i += 2 {
		h.Set(headers[i], headers[i+1])
	}
	return h
}

func TestServer_Response_NoData(t *testing.T) {
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		// Nothing.
		return nil
	}, func(st *serverTester) {
		var err error
		if err = getSlash(st); err != nil {
			st.t.Fatalf("Error writing Frame: %v", err)
		}

		var hf *SynReplyFrame
		if hf, err = st.wantSynReply(); err != nil {
			st.t.Fatalf("%v", err)
		}
		if !hf.StreamEnded() {
			t.Fatal("want END_STREAM flag")
		}
	})
}

func TestServer_Response_NoData_Header_FooBar(t *testing.T) {
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Foo-Bar", "some-value")
		return nil
	}, func(st *serverTester) {
		var err error
		if err = getSlash(st); err != nil {
			st.t.Fatalf("Error writing Frame: %v", err)
		}

		var hf *SynReplyFrame
		if hf, err = st.wantSynReply(); err != nil {
			st.t.Fatalf("%v", err)
		}
		if !hf.StreamEnded() {
			t.Fatal("want END_STREAM flag")
		}

		goth := hf.Headers
		wanth := newResHeader("foo-bar", "some-value")
		if !reflect.DeepEqual(goth, wanth) {
			t.Errorf("Got headers %v; want %v", goth, wanth)
		}
	})
}

func TestServer_Response_TransferEncoding_chunked(t *testing.T) {
	const msg = "hi"
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Transfer-Encoding", "chunked") // should be stripped
		io.WriteString(w, msg)
		return nil
	}, func(st *serverTester) {
		var err error
		if err := getSlash(st); err != nil {
			st.t.Fatalf("Error writing Frame: %v", err)
		}

		var hf *SynReplyFrame
		if hf, err = st.wantSynReply(); err != nil {
			st.t.Fatalf("%v", err)
		}
		goth := hf.Headers
		wanth := newResHeader()
		if !reflect.DeepEqual(goth, wanth) {
			t.Errorf("Got headers %v; want %v", goth, wanth)
		}
	})
}

// Header accessed only after the initial write.
func TestServer_Response_Data_IgnoreHeaderAfterWrite_After(t *testing.T) {
	const msg = "<html>this is HTML."
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		io.WriteString(w, msg)
		w.Header().Set("foo", "should be ignored")
		return nil
	}, func(st *serverTester) {
		var err error
		if err := getSlash(st); err != nil {
			st.t.Fatalf("Error writing Frame: %v", err)
		}

		var hf *SynReplyFrame
		if hf, err = st.wantSynReply(); err != nil {
			st.t.Fatalf("%v", err)
		}
		if hf.StreamEnded() {
			t.Fatal("unexpected END_STREAM")
		}
		goth := hf.Headers
		wanth := newResHeader()
		if !reflect.DeepEqual(goth, wanth) {
			t.Errorf("Got headers %v; want %v", goth, wanth)
		}
	})
}

// Header accessed before the initial write and later mutated.
func TestServer_Response_Data_IgnoreHeaderAfterWrite_Overwrite(t *testing.T) {
	const msg = "<html>this is HTML."
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("foo", "proper value")
		io.WriteString(w, msg)
		w.Header().Set("foo", "should be ignored")
		return nil
	}, func(st *serverTester) {
		var err error
		if err := getSlash(st); err != nil {
			st.t.Fatalf("Error writing Frame: %v", err)
		}

		var hf *SynReplyFrame
		if hf, err = st.wantSynReply(); err != nil {
			st.t.Fatalf("%v", err)
		}
		if hf.StreamEnded() {
			t.Fatal("unexpected END_STREAM")
		}

		goth := hf.Headers
		wanth := newResHeader("foo", "proper value")
		if !reflect.DeepEqual(goth, wanth) {
			t.Errorf("Got headers %v; want %v", goth, wanth)
		}
	})
}

func TestServer_Response_Header_Flush_MidWrite(t *testing.T) {
	const msg = "<html>this is HTML"
	const msg2 = ", and this is the next chunk"
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		io.WriteString(w, msg)
		w.(http.Flusher).Flush()
		io.WriteString(w, msg2)
		return nil
	}, func(st *serverTester) {
		var err error
		if err := getSlash(st); err != nil {
			st.t.Fatalf("Error writing Frame: %v", err)
		}

		var hf *SynReplyFrame
		if hf, err = st.wantSynReply(); err != nil {
			st.t.Fatalf("%v", err)
		}
		if hf.StreamEnded() {
			t.Fatal("unexpected END_STREAM flag")
		}
		goth := hf.Headers
		wanth := newResHeader()
		if !reflect.DeepEqual(goth, wanth) {
			t.Errorf("Got headers %v; want %v", goth, wanth)
		}
		{
			df := st.wantData()
			if df.StreamEnded() {
				t.Error("unexpected END_STREAM flag")
			}
			if got := string(df.Data); got != msg {
				t.Errorf("got DATA %q; want %q", got, msg)
			}
		}
		{
			df := st.wantData()
			if !df.StreamEnded() {
				t.Error("wanted END_STREAM flag on last data chunk")
			}
			if got := string(df.Data); got != msg2 {
				t.Errorf("got DATA %q; want %q", got, msg2)
			}
		}
	})
}

// Test that the handler can't write more than the client allows
func TestServer_Response_LargeWrite_FlowControlled(t *testing.T) {
	const size = 1 << 20
	const maxFrameSize = 16 << 10
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		w.(http.Flusher).Flush()
		n, err := w.Write(bytes.Repeat([]byte("a"), size))
		if err != nil {
			return fmt.Errorf("Write error: %v", err)
		}
		if n != size {
			return fmt.Errorf("wrong size %d from Write", n)
		}
		return nil
	}, func(st *serverTester) {
		var err error
		// Set the window size to something explicit for this test.
		// It's also how much initial data we expect.
		const initWindowSize = 123
		if err = st.writeSettings(uint32(initWindowSize)); err != nil {
			st.t.Fatalf("Error writing Frame: %v", err)
		}
		// make the single request
		if err := getSlash(st); err != nil {
			st.t.Fatalf("Error writing Frame: %v", err)
		}

		defer func() {
			st.writeRstStream(1, Cancel)
		}()

		var hf *SynReplyFrame
		if hf, err = st.wantSynReply(); err != nil {
			st.t.Fatalf("%v", err)
		}
		if hf.StreamEnded() {
			t.Fatal("unexpected END_STREAM flag")
		}

		df := st.wantData()
		if got := len(df.Data); got != initWindowSize {
			t.Fatalf("Initial window size = %d but got DATA with %d bytes", initWindowSize, got)
		}

		for _, quota := range []int{1, 13, 127} {
			st.writeWindowUpdate(1, uint32(quota))
			df := st.wantData()
			if int(quota) != len(df.Data) {
				t.Fatalf("read %d bytes after giving %d quota", len(df.Data), quota)
			}
		}

		st.writeRstStream(1, Cancel)
	})
}

// Test that the handler blocked in a Write is unblocked if the server sends a RST_STREAM.
func TestServer_Response_RST_Unblocks_LargeWrite(t *testing.T) {
	const size = 1 << 20
	const maxFrameSize = 16 << 10
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		w.(http.Flusher).Flush()
		errc := make(chan error, 1)
		go func() {
			_, err := w.Write(bytes.Repeat([]byte("a"), size))
			errc <- err
		}()
		select {
		case err := <-errc:
			if err == nil {
				return errors.New("unexpected nil error from Write in handler")
			}
			return nil
		case <-time.After(2 * time.Second):
			return errors.New("timeout waiting for Write in handler")
		}
	}, func(st *serverTester) {
		var err error
		settings := new(SettingsFrame)
		settings.FlagIdValues = []SettingsFlagIdValue{
			{0, SettingsInitialWindowSize, 0},
		}

		if err := st.fr.WriteFrame(settings); err != nil {
			t.Fatal(err)
		}

		// make the single request
		if err := getSlash(st); err != nil {
			st.t.Fatalf("Error writing Frame: %v", err)
		}

		defer func() {
			st.writeRstStream(1, Cancel)
		}()

		var hf *SynReplyFrame
		if hf, err = st.wantSynReply(); err != nil {
			st.t.Fatalf("%v", err)
		}
		if hf.StreamEnded() {
			t.Fatal("unexpected END_STREAM flag")
		}

		st.writeRstStream(1, Cancel)
	})
}

func TestServer_Response_Empty_Data_Not_FlowControlled(t *testing.T) {
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		w.(http.Flusher).Flush()
		// Nothing; send empty DATA
		return nil
	}, func(st *serverTester) {
		var err error
		settings := new(SettingsFrame)
		settings.FlagIdValues = []SettingsFlagIdValue{
			{0, SettingsInitialWindowSize, 0},
		}

		// Handler gets no data quota:
		if err := st.fr.WriteFrame(settings); err != nil {
			t.Fatal(err)
		}

		// make the single request
		if err := getSlash(st); err != nil {
			st.t.Fatalf("Error writing Frame: %v", err)
		}

		var hf *SynReplyFrame
		if hf, err = st.wantSynReply(); err != nil {
			st.t.Fatalf("%v", err)
		}
		if hf.StreamEnded() {
			t.Fatal("unexpected END_STREAM flag")
		}

		df := st.wantData()
		if got := len(df.Data); got != 0 {
			t.Fatalf("unexpected %d DATA bytes; want 0", got)
		}
		if !df.StreamEnded() {
			t.Fatal("DATA didn't have END_STREAM")
		}
	})
}

func TestServer_Response_Automatic100Continue(t *testing.T) {
	const msg = "foo"
	const reply = "bar"
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		if v := r.Header.Get("Expect"); v != "" {
			t.Errorf("Expect header = %q; want empty", v)
		}
		buf := make([]byte, len(msg))
		// This read should trigger the 100-continue being sent.
		if n, err := io.ReadFull(r.Body, buf); err != nil || n != len(msg) || string(buf) != msg {
			return fmt.Errorf("ReadFull = %q, %v; want %q, nil", buf[:n], err, msg)
		}
		_, err := io.WriteString(w, reply)
		return err
	}, func(st *serverTester) {
		var err error
		headers := make(http.Header)
		headers.Set(":method", "POST")
		headers.Set("expect", "100-continue")
		st.writeSynStream(1, headers, false)

		var hf *SynReplyFrame
		if hf, err = st.wantSynReply(); err != nil {
			st.t.Fatalf("%v", err)
		}
		if hf.StreamEnded() {
			t.Fatal("unexpected END_STREAM flag")
		}
		goth := hf.Headers
		wanth := newResHeader(":status", "100")
		if !reflect.DeepEqual(goth, wanth) {
			t.Fatalf("Got headers %v; want %v", goth, wanth)
		}

		// Okay, they sent status 100, so we can send our
		// gigantic and/or sensitive "foo" payload now.
		st.writeData(1, []byte(msg), true)

		st.wantWindowUpdate(0, uint32(len(msg)))

		if hf, err = st.wantSynReply(); err != nil {
			st.t.Fatalf("%v", err)
		}
		if hf.StreamEnded() {
			t.Fatal("expected data to follow")
		}
		goth = hf.Headers
		wanth = newResHeader()
		if !reflect.DeepEqual(goth, wanth) {
			t.Errorf("Got headers %v; want %v", goth, wanth)
		}

		df := st.wantData()
		if string(df.Data) != reply {
			t.Errorf("Client read %q; want %q", df.Data, reply)
		}
		if !df.StreamEnded() {
			t.Errorf("expect data stream end")
		}
	})
}

func TestServer_HandlerWriteErrorOnDisconnect(t *testing.T) {
	errc := make(chan error, 1)
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		p := []byte("some data.\n")
		for {
			_, err := w.Write(p)
			if err != nil {
				errc <- err
				return nil
			}
		}
	}, func(st *serverTester) {
		var err error
		st.writeSynStream(1, nil, true)

		var hf *SynReplyFrame
		if hf, err = st.wantSynReply(); err != nil {
			st.t.Fatalf("%v", err)
		}
		if hf.StreamEnded() {
			t.Fatal("unexpected END_STREAM flag")
		}
		// Close the connection and wait for the handler to (hopefully) notice.
		st.cc.Close()
		select {
		case <-errc:
		case <-time.After(5 * time.Second):
			t.Error("timeout")
		}
	})
}

func TestServer_Connection_Close(t *testing.T) {
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		CloseConn(r.Body) // just block connection
		return nil
	}, func(st *serverTester) {
		if err := getSlash(st); err != nil {
			st.t.Fatal("client writing Frame should not fail")
		}
		if _, err := st.wantSynReply(); err == nil {
			st.t.Fatalf("client reading Frame should fail")
		}
	})
}

func TestServer_Exceed_Max_Concurrent_Streams(t *testing.T) {
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		time.Sleep(1000 * time.Millisecond)
		return nil
	}, func(st *serverTester) {
		headers := make(http.Header)
		headers.Set(":user-agent", "test-ua")
		if err := getSlash(st); err != nil {
			st.t.Fatal("client writing Frame should not fail")
		}
		if err := st.writeSynStream(3, headers, true); err != nil {
			st.t.Fatalf("client writing Frame should not fail, %v", err)
		}
		if err := st.writeSynStream(5, headers, true); err != nil {
			st.t.Fatal("client writing Frame should not fail")
		}
		st.writeSynStream(7, headers, true)
		if _, err := st.readFrame(); err == nil {
			st.t.Fatal("client writing Frame should fail")
		}
	})
}

func TestServer_Read_First_Request_Header_Timeout(t *testing.T) {
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Foo-Bar", "some-value")
		return nil
	}, func(st *serverTester) {
		time.Sleep(1200 * time.Millisecond)
		getSlash(st)
		if _, err := st.readFrame(); err == nil {
			st.t.Fatal("client writing Frame should fail")
		} else {
			fmt.Printf("expecting error: %v\n", err)
		}
	})
}

func TestServer_Read_Request_Body_Timeout(t *testing.T) {
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		SetReadStreamTimeout(r.Body.(*RequestBody), 100*time.Millisecond)
		w.Header().Set("Foo-Bar", "some-value")
		time.Sleep(200 * time.Millisecond)
		return nil
	}, func(st *serverTester) {
		headers := make(http.Header)
		headers.Set(":method", "POST")
		headers.Set(":content-ength", "4")
		st.writeSynStream(1, headers, false)
		time.Sleep(200 * time.Millisecond)
		if err := st.wantRstStream(1, ProtocolError); err != nil {
			st.t.Fatalf("wantRstStream for id 1, %v", err)
		}
	})
}

func TestServer_Write_Client_Timeout(t *testing.T) {
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		SetWriteStreamTimeout(r.Body.(*RequestBody), 100*time.Millisecond)
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte("some response data"))
		return nil
	}, func(st *serverTester) {
		st.writeSynStream(1, nil, true)
		time.Sleep(300 * time.Millisecond)
		if err := st.wantRstStream(1, ProtocolError); err != nil {
			st.t.Fatalf("want ResStream for id 1, %v", err)
		}
	})
}

func TestServer_Read_Client_Again_Timeout(t *testing.T) {
	testServerResponse(t, func(w http.ResponseWriter, r *http.Request) error {
		defer SetConnTimeout(r.Body.(*RequestBody), 1000*time.Millisecond)
		return nil
	}, func(st *serverTester) {
		getSlash(st)
		if _, err := st.wantSynReply(); err != nil {
			st.t.Fatalf("%v: wantSynReply for id1: %v", time.Now(), err)
		}
		time.Sleep(500 * time.Millisecond)
		if err := st.writeSynStream(3, nil, true); err != nil {
			st.t.Fatalf("client writing Frame should not fail, %v", err)
		}
		fmt.Printf("%v: writing id 3 frame to server\n", time.Now())
		if _, err := st.wantSynReply(); err != nil {
			st.t.Fatalf("%v: wantSynReply for id3: %v", time.Now(), err)
		}
		time.Sleep(800 * time.Millisecond)
		if err := st.writeSynStream(5, nil, true); err != nil {
			st.t.Fatalf("client writing Frame should not fail, %v", err)
		}
		if _, err := st.wantSynReply(); err != nil {
			st.t.Fatalf("%v: wantSynReply for id5: %v", time.Now(), err)
		}
		time.Sleep(1200 * time.Millisecond)
		st.writeSynStream(4, nil, true)
		if _, err := st.readFrame(); err == nil {
			st.t.Fatalf("%v: client writing Frame should fail", time.Now())
		} else {
			fmt.Printf("expecting err: %v\n", err)
		}
	})
}

// readBodyHandler returns an http Handler func that reads len(want)
// bytes from r.Body and fails t if the contents read were not
// the value of want.
func readBodyHandler(t *testing.T, want string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, len(want))
		_, err := io.ReadFull(r.Body, buf)
		if err != nil {
			t.Error(err)
			return
		}
		if string(buf) != want {
			t.Errorf("read %q; want %q", buf, want)
		}
	}
}

func BenchmarkServerGets(b *testing.B) {
	b.ReportAllocs()

	const msg = "Hello, world"
	st := newServerTester(b, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, msg)
	})
	defer st.Close()

	var err error
	if err = st.greet(); err != nil {
		st.t.Fatalf("Error writing Frame: %v", err)
	}

	// Give the server quota to reply. (plus it has the the 64KB)
	st.writeWindowUpdate(0, uint32(b.N*len(msg)))

	for i := 0; i < b.N; i++ {
		id := 1 + uint32(i)*2
		st.writeSynStream(id, nil, true)

		st.wantSynReply()
		df := st.wantData()
		if !df.StreamEnded() {
			b.Fatalf("DATA didn't have END_STREAM; got %v", df)
		}
	}
}

func BenchmarkServerPosts(b *testing.B) {
	b.ReportAllocs()

	const msg = "Hello, world"
	st := newServerTester(b, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, msg)
	})
	defer st.Close()

	var err error
	if err = st.greet(); err != nil {
		st.t.Fatalf("Error writing Frame: %v", err)
	}

	// Give the server quota to reply. (plus it has the the 64KB)
	st.writeWindowUpdate(0, uint32(b.N*len(msg)))

	for i := 0; i < b.N; i++ {
		id := 1 + uint32(i)*2

		headers := newReqHeader(":method", "POST")
		st.writeSynStream(id, headers, false)
		st.writeData(id, nil, true)

		st.wantSynReply()
		df := st.wantData()
		if !df.StreamEnded() {
			b.Fatalf("DATA didn't have END_STREAM; got %v", df)
		}
	}
}

type puppetCommand struct {
	fn   func(w http.ResponseWriter, r *http.Request)
	done chan<- bool
}

type handlerPuppet struct {
	ch chan puppetCommand
}

func newHandlerPuppet() *handlerPuppet {
	return &handlerPuppet{
		ch: make(chan puppetCommand),
	}
}

func (p *handlerPuppet) act(w http.ResponseWriter, r *http.Request) {
	for cmd := range p.ch {
		cmd.fn(w, r)
		cmd.done <- true
	}
}

func (p *handlerPuppet) done() { close(p.ch) }
func (p *handlerPuppet) do(fn func(http.ResponseWriter, *http.Request)) {
	done := make(chan bool)
	p.ch <- puppetCommand{fn, done}
	<-done
}
