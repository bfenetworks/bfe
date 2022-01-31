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

// Copyright 2010 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_http

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

import (
	"github.com/baidu/go-lib/gotrack"
	slog "github.com/baidu/go-lib/log"
)

import (
	bufio "github.com/bfenetworks/bfe/bfe_bufio"
	tls "github.com/bfenetworks/bfe/bfe_tls"
)

// DefaultMaxHeaderBytes is the maximum permitted size of the headers in an HTTP request.
// This can be overridden by setting config.MaxHeaderBytes.
const DefaultMaxHeaderBytes = 1 << 20 // 1 MB

// DefaultMaxHeaderUriBytes is the maximum permitted size of URI in headers in an HTTP request.
// This can be overridden by setting config.MaxHeaderUriBytes.
const DefaultMaxHeaderUriBytes = 8 * 1024

var ErrBodyNotAllowed = errors.New("http: request method or response status code does not allow body")

// A Server defines parameters for running an HTTP server.
// The zero value for Server is a valid configuration.
type Server struct {
	Addr                    string        // TCP address to listen on, ":http" if empty
	Handler                 Handler       // handler to invoke, http.DefaultServeMux if nil
	ReadTimeout             time.Duration // maximum duration before timing out read of the request
	WriteTimeout            time.Duration // maximum duration before timing out write of the response
	TlsHandshakeTimeout     time.Duration // maximum duration before timing out handshake
	GracefulShutdownTimeout time.Duration // maximum duration before timing out graceful shutdown
	MaxHeaderBytes          int           // maximum size of request headers, DefaultMaxHeaderBytes if 0
	MaxHeaderUriBytes       int           // max URI(in header) length in bytes in request
	TLSConfig               *tls.Config   // optional TLS config, used by ListenAndServeTLS

	// TLSNextProto optionally specifies a function to take over
	// ownership of the provided TLS connection when an NPN
	// protocol upgrade has occurred.  The map key is the protocol
	// name negotiated. The Handler argument should be used to
	// handle HTTP requests and will initialize the Request's TLS
	// and RemoteAddr if not already set.  The connection is
	// automatically closed when the function returns.
	TLSNextProto map[string]func(*Server, *tls.Conn, Handler)

	// HTTPNextProto optionally specifies a function to take over
	// ownership of the http connection when an HTTP Upgrade has
	// occurred. The map key is the protocol name negotiated (eg
	// websocket, h2c)
	HTTPNextProto map[string]func(*Server, ResponseWriter, *Request)

	// ConnState specifies an optional callback function that is
	// called when a client connection changes state. See the
	// ConnState type and associated constants for details.
	ConnState func(net.Conn, ConnState)

	// ErrorLog specifies an optional logger for errors accepting
	// connections and unexpected behavior from handlers.
	// If nil, logging goes to os.Stderr via the log package's
	// standard logger.
	ErrorLog *log.Logger

	disableKeepAlives int32 // accessed atomically.

	// CloseNotifyCh allow detecting when the server in graceful shutdown state
	CloseNotifyCh chan bool
}

func (s *Server) DoKeepAlives() bool {
	return atomic.LoadInt32(&s.disableKeepAlives) == 0
}

// SetKeepAlivesEnabled controls whether HTTP keep-alives are enabled.
// By default, keep-alives are always enabled. Only very
// resource-constrained environments or servers in the process of
// shutting down should disable them.
func (s *Server) SetKeepAlivesEnabled(v bool) {
	if v {
		atomic.StoreInt32(&s.disableKeepAlives, 0)
	} else {
		atomic.StoreInt32(&s.disableKeepAlives, 1)
	}
}

// A ConnState represents the state of a client connection to a server.
// It's used by the optional Server.ConnState hook.
type ConnState int

const (
	// StateNew represents a new connection that is expected to
	// send a request immediately. Connections begin at this
	// state and then transition to either StateActive or
	// StateClosed.
	StateNew ConnState = iota

	// StateActive represents a connection that has read 1 or more
	// bytes of a request. The Server.ConnState hook for
	// StateActive fires before the request has entered a handler
	// and doesn't fire again until the request has been
	// handled. After the request is handled, the state
	// transitions to StateClosed, StateHijacked, or StateIdle.
	StateActive

	// StateIdle represents a connection that has finished
	// handling a request and is in the keep-alive state, waiting
	// for a new request. Connections transition from StateIdle
	// to either StateActive or StateClosed.
	StateIdle

	// StateHijacked represents a hijacked connection.
	// This is a terminal state. It does not transition to StateClosed.
	StateHijacked

	// StateClosed represents a closed connection.
	// This is a terminal state. Hijacked connections do not
	// transition to StateClosed.
	StateClosed
)

var stateName = map[ConnState]string{
	StateNew:      "new",
	StateActive:   "active",
	StateIdle:     "idle",
	StateHijacked: "hijacked",
	StateClosed:   "closed",
}

func (c ConnState) String() string {
	return stateName[c]
}

// The Handler Objects implementing the Handler interface can be
// registered to serve a particular path or subtree
// in the HTTP server.
//
// ServeHTTP should write reply headers and data to the ResponseWriter
// and then return.  Returning signals that the request is finished
// and that the HTTP server can move on to the next request on
// the connection.
type Handler interface {
	ServeHTTP(ResponseWriter, *Request)
}

// The Flusher interface is implemented by ResponseWriters that allow
// an HTTP handler to flush buffered data to the client.
//
// Note that even for ResponseWriters that support Flush,
// if the client is connected through an HTTP proxy,
// the buffered data may not reach the client until the response
// completes.
type Flusher interface {
	// Flush sends any buffered data to the client.
	Flush() error
}

type Hijacker interface {
	// Hijack lets the caller take over the connection.
	// After a call to Hijack the HTTP server library
	// will not do anything else with the connection.
	//
	// It becomes the caller's responsibility to manage
	// and close the connection.
	//
	// The returned net.Conn may have read or write deadlines
	// already set, depending on the configuration of the
	// Server. It is the caller's responsibility to set
	// or clear those deadlines as needed.
	//
	// The returned bufio.Reader may contain unprocessed buffered
	// data from the client.
	Hijack() (net.Conn, *bufio.ReadWriter, error)
}

// The CloseNotifier interface is implemented by ResponseWriters which
// allow detecting when the underlying connection has gone away.
//
// This mechanism can be used to cancel long operations on the server
// if the client has disconnected before the response is ready.
type CloseNotifier interface {
	// CloseNotify returns a channel that receives a single value
	// when the client connection has gone away.
	CloseNotify() <-chan bool
}

// The HandlerFunc type is an adapter to allow the use of
// ordinary functions as HTTP handlers.  If f is a function
// with the appropriate signature, HandlerFunc(f) is a
// Handler object that calls f.
type HandlerFunc func(ResponseWriter, *Request)

// ServeHTTP calls f(w, r).
func (f HandlerFunc) ServeHTTP(w ResponseWriter, r *Request) {
	f(w, r)
}

type WriteFlusher interface {
	io.Writer
	Flusher
}

type MaxLatencyWriter struct {
	dst     WriteFlusher
	latency time.Duration
	err     error
	done    chan bool

	// onExitFlushLoop is a callback set by tests to detect the state of the
	// flushLoop() goroutine.
	onExitFlushLoop func()

	lk sync.Mutex // protects Write + Flush
}

func NewMaxLatencyWriter(dst WriteFlusher, latency time.Duration,
	onExitFlushLoop func()) *MaxLatencyWriter {
	m := new(MaxLatencyWriter)
	m.dst = dst
	m.latency = latency
	m.done = make(chan bool)
	m.onExitFlushLoop = onExitFlushLoop
	return m
}

func (m *MaxLatencyWriter) Write(p []byte) (int, error) {
	m.lk.Lock()
	n, err := m.dst.Write(p)
	m.lk.Unlock()

	return n, err
}

func (m *MaxLatencyWriter) Flush() error {
	m.lk.Lock()
	defer m.lk.Unlock()

	if m.err != nil {
		return m.err
	}

	m.err = m.dst.Flush()
	return m.err
}

func (m *MaxLatencyWriter) FlushLoop() {
	t := time.NewTicker(m.latency)

	defer func() {
		if err := recover(); err != nil {
			slog.Logger.Warn("panic:MaxLatencyWriter.FlushLoop():%v\n%s", err, gotrack.CurrentStackTrace(0))
			state.HttpPanicClientFlushLoop.Inc(1)
		}
		t.Stop()
	}()

	for {
		select {
		case <-m.done:
			if m.onExitFlushLoop != nil {
				m.onExitFlushLoop()
			}
			return
		case <-t.C:
			m.Flush()
		}
	}
}

func (m *MaxLatencyWriter) Stop() {
	m.done <- true
}

// Error replies to the request with the specified error message and HTTP code.
// The error message should be plain text.
func Error(w ResponseWriter, error string, code int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintln(w, error)
}

type ConnectError struct {
	Addr string
	Err  error
}

func (e ConnectError) Error() string {
	return fmt.Sprintf("ConnectError: %s, %s", e.Err.Error(), e.Addr)
}

type WriteRequestError struct {
	Err error
}

func (e WriteRequestError) Error() string {
	return fmt.Sprintf("WriteRequestError: %s", e.Err.Error())
}

func (e WriteRequestError) CheckTargetError(addr net.Addr) bool {
	if err, ok := e.Err.(*net.OpError); ok {
		return reflect.DeepEqual(err.Addr, addr)
	}
	return false
}

type RespHeaderTimeoutError struct{}

func (e RespHeaderTimeoutError) Error() string {
	return "RespHeaderTimeoutError: timeout awaiting response headers"
}

type ReadRespHeaderError struct {
	Err error
}

func (e ReadRespHeaderError) Error() string {
	return fmt.Sprintf("ReadRespHeaderError: %s", e.Err.Error())
}

type TransportBrokenError struct{}

func (e TransportBrokenError) Error() string {
	return "TransportBrokenError: transport closed before response was received"
}

type FlowLimiter interface {
	// AcceptConn check whether current connection should be accept or not
	AcceptConn() bool

	// AcceptRequest check whether current request should be accept or not
	AcceptRequest() bool
}

// CloseWatcher can be used to cancel long operations on the server
// if the client has disconnected.
type CloseWatcher struct {
	notifier CloseNotifier // notify if the underlying connection has gone away.
	onClose  func()
	done     chan bool
}

func NewCloseWatcher(notifier CloseNotifier, onClose func()) *CloseWatcher {
	w := new(CloseWatcher)
	w.notifier = notifier
	w.onClose = onClose
	w.done = make(chan bool)

	return w
}

func (w *CloseWatcher) WatchLoop() {
	defer func() {
		if err := recover(); err != nil {
			slog.Logger.Warn("panic:CloseWatcher.WatchLoop():%v\n%s", err, gotrack.CurrentStackTrace(0))
			state.HttpPanicClientWatchLoop.Inc(1)
		}
	}()

	closeCh := w.notifier.CloseNotify()
	for {
		select {
		case <-closeCh:
			slog.Logger.Debug("CloseWatcher found client conn disconnected, fire onClose()")
			if w.onClose != nil {
				state.HttpCancelOnClientClose.Inc(1)
				w.onClose()
			}
		case <-w.done:
			return
		}
	}
}

func (w *CloseWatcher) Stop() {
	w.done <- true
}

// Peeker is common interface for peeking data
type Peeker interface {
	Peek(n int) ([]byte, error)
}

func CopyHeader(dst, src Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
