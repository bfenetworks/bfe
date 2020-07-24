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

package bfe_websocket

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

import (
	"golang.org/x/net/websocket"
)

import (
	"github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_bufio"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_util"
)

type ServerTester struct {
	t testing.TB

	// for websocket client
	cc net.Conn
	wc *websocket.Conn

	// for websocket proxy
	mp *bfe_util.MockServer
	hl func(*bfe_http.Server, bfe_http.ResponseWriter, *bfe_http.Request)

	// for websocket server
	mb *httptest.Server
}

func NewServerTester(t testing.TB, h HandlerMap, c *Server) *ServerTester {
	st := &ServerTester{t: t}

	// init websocket server
	st.startWebsocketServer(h)

	// init websocket proxy
	if c == nil {
		c = new(Server)
	}
	if c.BalanceHandler == nil {
		c.BalanceHandler = func(req interface{}) (*backend.BfeBackend, error) {
			b := backend.NewBfeBackend()
			b.AddrInfo = st.mb.Listener.Addr().String()
			return b, nil
		}
	}
	st.hl = NewProtoHandler(c)
	st.mp = bfe_util.NewUnstartedServer(bfe_util.MockHandler(st.handleWebsocketConn))
	st.mp.StartTCP()

	// init websocket client
	cc, err := net.Dial("tcp", st.mp.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	st.cc = cc

	return st
}

func (st *ServerTester) startWebsocketServer(handlers HandlerMap) {
	// register websocet handlers
	for uri, handler := range handlers {
		http.Handle(uri, handler)
	}
	// start websocket server
	st.mb = httptest.NewServer(nil)
}

func (st *ServerTester) handleWebsocketConn(conn net.Conn) {
	defer conn.Close()

	// read first request from conn
	br := bfe_bufio.NewReader(conn)
	bw := bfe_bufio.NewWriter(conn)
	wr := bfe_bufio.NewReadWriter(br, bw)
	req, err := bfe_http.ReadRequest(wr.Reader, 1024)
	if err != nil {
		return
	}

	// create ResponseWriter
	rw := NewMockResponseWriter(conn, wr)

	// check and process websocket upgrade
	if CheckUpgradeWebSocket(req) {
		st.hl(st.mp.Config, rw, req)
		return
	}
}

// WebSocketHandshake starts websocket handshake (client perspective)
func (st *ServerTester) WebSocketHandshake(c *websocket.Config) error {
	var err error
	st.wc, err = websocket.NewClient(c, st.cc)
	return err
}

// WebSocketWrite sends webscoket message (client perspective)
func (st *ServerTester) WebSocketWrite(data []byte) (int, error) {
	return st.wc.Write(data)
}

// WebSocketRead recv websocket message (client perspective)
func (st *ServerTester) WebSocketRead(msg []byte) (int, error) {
	return st.wc.Read(msg)
}

// Read reads until timeout (client perspective)
func (st *ServerTester) Read(buf []byte) error {
	st.cc.SetReadDeadline(time.Now().Add(4 * time.Second))
	_, err := io.ReadFull(st.cc, buf)
	return err
}

// Write writes raw data (client perspective)
func (st *ServerTester) Write(data []byte) error {
	_, err := st.cc.Write(data)
	return err
}

// WantError read and check error (client perspective)
func (st *ServerTester) WantError(e string) {
	err := st.Read(make([]byte, 256))
	if err == nil {
		st.t.Fatalf("Expecting error")
	}
	if !strings.Contains(err.Error(), e) {
		st.t.Fatalf("Expecting error got %v ; want %v", err.Error(), e)
	}
}

func (st *ServerTester) Close() {
	st.cc.Close()
	st.mp.Close()
	st.mb.Close()
}

type HandlerMap map[string]http.Handler

func websocketHandlers(uri string, h websocket.Handler) HandlerMap {
	m := make(HandlerMap)
	m[uri] = h
	return m
}

type MockResponseWriter struct {
	conn        net.Conn
	brw         *bfe_bufio.ReadWriter
	header      bfe_http.Header
	wroteHeader bool
}

func NewMockResponseWriter(conn net.Conn, brw *bfe_bufio.ReadWriter) *MockResponseWriter {
	rw := new(MockResponseWriter)
	rw.conn = conn
	rw.brw = brw
	rw.header = make(bfe_http.Header)
	return rw
}

// Header returns the header map that will be sent by WriteHeader.
// Changing the header after a call to WriteHeader (or Write) has no effect.
func (rw *MockResponseWriter) Header() bfe_http.Header {
	return rw.header
}

// Write writes the data to the connection as part of an HTTP reply.
func (rw *MockResponseWriter) Write(data []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(bfe_http.StatusOK)
	}

	// not support chunked-encoding
	n, err := rw.brw.Write(data)
	if err != nil {
		return n, err
	}
	err = rw.brw.Flush()
	return n, err
}

// WriteHeader sends an HTTP response header with status code.
func (rw *MockResponseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.wroteHeader = true

	statusLine := fmt.Sprintf("HTTP/1.1 %d %s\r\n", code, bfe_http.StatusText[code])
	rw.brw.Write([]byte(statusLine))
	rw.header.Write(rw.brw.Writer)
	rw.brw.Write([]byte("\r\n"))
	rw.brw.Flush()
}

func (rw *MockResponseWriter) Flush() error {
	return nil
}

func (rw *MockResponseWriter) Hijack() (rwc net.Conn, buf *bfe_bufio.ReadWriter, err error) {
	return rw.conn, rw.brw, nil
}

func testWebSocketProxy(t *testing.T, f func(st *ServerTester), h HandlerMap, c *Server) {
	// create server tester
	st := NewServerTester(t, h, c)
	defer st.Close()

	// perform test actions
	f(st)
}
