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

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// HTTP server.  See RFC 2616.

package bfe_server

import (
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_bufio"
	"github.com/bfenetworks/bfe/bfe_http"
)

const (
	sniffLen = 512 // previous defined in net/http/sniff.go
)

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

// Errors introduced by the HTTP server.
var (
	ErrBodyNotAllowed = errors.New("http: request method or response status code does not allow body")
	ErrContentLength  = errors.New("Conn.Write wrote more than the declared Content-Length")
	ErrHijacked       = errors.New("http: connection has been hijacked")
)

// A switchWriter can have its Writer changed at runtime.
// It's not safe for concurrent Writes and switches.
type switchWriter struct {
	io.Writer
}

type atomicBool int32

func (b *atomicBool) isSet() bool { return atomic.LoadInt32((*int32)(b)) != 0 }
func (b *atomicBool) setTrue()    { atomic.StoreInt32((*int32)(b), 1) }
func (b *atomicBool) setFalse()   { atomic.StoreInt32((*int32)(b), 0) }

// A response represents the server side of an HTTP response.
type response struct {
	conn          *conn
	req           *bfe_http.Request // request for this response
	wroteHeader   bool              // reply header has been (logically) written
	wroteContinue bool              // 100 Continue response was written

	// canWriteContinue is a boolean value accessed as an atomic int32
	// that says whether or not a 100 Continue header can be written
	// to the connection.
	// writeContinueMu must be held while writing the header.
	// These two fields together synchronize the body reader
	// (the expectContinueReader, which wants to write 100 Continue)
	// against the main writer.
	canWriteContinue atomicBool
	writeContinueMu  sync.Mutex

	w  *bfe_bufio.Writer // buffers output in chunks to chunkWriter
	cw chunkWriter
	sw *switchWriter // of the bufio.Writer, for return to putBufioWriter

	// handlerHeader is the Header that Handlers get access to,
	// which may be retained and mutated even after WriteHeader.
	// handlerHeader is copied into cw.header at WriteHeader
	// time, and privately mutated thereafter.
	handlerHeader bfe_http.Header
	calledHeader  bool // handler accessed handlerHeader via Header

	headerWritten int64 // number of bytes written in header
	written       int64 // number of bytes written in body
	contentLength int64 // explicitly-declared Content-Length; or -1
	status        int   // status code passed to WriteHeader

	// close connection after this reply.  set on request and
	// updated after response from handler if there's a
	// "Connection: keep-alive" response header and a
	// Content-Length.
	closeAfterReply bool

	// requestBodyLimitHit is set by requestTooLarge when
	// maxBytesReader hits its max size. It is checked in
	// WriteHeader, to make sure we don't consume the
	// remaining request body to try to advance to the next HTTP
	// request. Instead, when this is set, we stop reading
	// subsequent requests on this connection and stop reading
	// input from it.
	requestBodyLimitHit bool

	handlerDone bool // set true when the handler exits
	hijackedv   bool

	// Buffers for Date and Content-Length
	dateBuf [len(bfe_http.TimeFormat)]byte
	clenBuf [10]byte
}

func newResponse(c *conn, req *bfe_http.Request) *response {
	w := &response{
		conn:          c,
		req:           req,
		handlerHeader: make(bfe_http.Header),
		contentLength: -1,
	}
	w.cw.res = w
	w.w = c.server.BufioCache.newBufioWriterSize(&w.cw, bufferBeforeChunkingSize)
	return w
}

// requestTooLarge is called by maxBytesReader when too much input has
// been read from the client.
func (w *response) requestTooLarge() {
	w.closeAfterReply = true
	w.requestBodyLimitHit = true
	if !w.wroteHeader {
		w.Header().Set("Connection", "close")
	}
}

// needsSniff reports whether a Content-Type still needs to be sniffed.
func (w *response) needsSniff() bool {
	_, haveType := w.handlerHeader["Content-Type"]
	return !w.cw.wroteHeader && !haveType && w.written < sniffLen
}

// writerOnly hides an io.Writer value's optional ReadFrom method
// from io.Copy.
type writerOnly struct {
	io.Writer
}

func srcIsRegularFile(src io.Reader) (isRegular bool, err error) {
	switch v := src.(type) {
	case *os.File:
		fi, err := v.Stat()
		if err != nil {
			return false, err
		}
		return fi.Mode().IsRegular(), nil
	case *io.LimitedReader:
		return srcIsRegularFile(v.R)
	default:
		return
	}
}

// SetSigner set signature calculator for response
func (w *response) SetSigner(signer bfe_http.SignCalculator) {
	w.cw.Signer = signer
}

// ReadFrom is here to optimize copying from an *os.File regular file
// to a *net.TCPConn with sendfile.
func (w *response) ReadFrom(src io.Reader) (n int64, err error) {
	// Our underlying w.conn.rwc is usually a *TCPConn (with its
	// own ReadFrom method). If not, or if our src isn't a regular
	// file, just fall back to the normal copy method.
	rf, ok := w.conn.rwc.(io.ReaderFrom)
	regFile, err := srcIsRegularFile(src)
	if err != nil {
		return 0, err
	}
	if !ok || !regFile {
		return io.Copy(writerOnly{w}, src)
	}

	// sendfile path:

	if !w.wroteHeader {
		w.WriteHeader(bfe_http.StatusOK)
	}

	if w.needsSniff() {
		n0, err := io.Copy(writerOnly{w}, io.LimitReader(src, sniffLen))
		n += n0
		if err != nil {
			return n, err
		}
	}

	w.w.Flush()  // get rid of any previous writes
	w.cw.flush() // make sure Header is written; flush data to rwc

	// Now that cw has been flushed, its chunking field is guaranteed initialized.
	if !w.cw.chunking && w.bodyAllowed() {
		n0, err := rf.ReadFrom(src)
		n += n0
		w.written += n0
		return n, err
	}

	n0, err := io.Copy(writerOnly{w}, src)
	n += n0
	return n, err
}

func (w *response) Header() bfe_http.Header {
	if w.cw.header == nil && w.wroteHeader && !w.cw.wroteHeader {
		// Accessing the header between logically writing it
		// and physically writing it means we need to allocate
		// a clone to snapshot the logically written state.
		w.cw.header = w.handlerHeader.Clone()
	}
	w.calledHeader = true
	return w.handlerHeader
}

func (w *response) WriteHeader(code int) {
	if w.wroteHeader {
		log.Logger.Warn("http: multiple response.WriteHeader calls")
		return
	}
	w.wroteHeader = true
	w.status = code

	// if server in graceful shutdown state, signal client that
	// the connection will be closed after completion of the response
	if w.conn.server.CheckGracefulShutdown() {
		if w.req.Proto == "HTTP/1.1" {
			w.handlerHeader.Set("Connection", "close")
		}
		// Note: http <1.1 application do not support persistent
		// connections and will close connection directly after
		// completion of the response
	}

	if w.calledHeader && w.cw.header == nil {
		w.cw.header = w.handlerHeader.Clone()
	}

	if cl := w.handlerHeader.GetDirect("Content-Length"); cl != "" {
		v, err := strconv.ParseInt(cl, 10, 64)
		if err == nil && v >= 0 {
			w.contentLength = v
		} else {
			log.Logger.Warn("http: invalid Content-Length of %q", cl)
			w.handlerHeader.Del("Content-Length")
		}
	}
}

// bodyAllowed returns true if a Write is allowed for this response type.
// It's illegal to call this before the header has been flushed.
func (w *response) bodyAllowed() bool {
	if !w.wroteHeader {
		panic("")
	}
	return w.status != bfe_http.StatusNotModified
}

// The Life Of A Write is like this:
//
// Handler starts. No header has been sent. The handler can either
// write a header, or just start writing.  Writing before sending a header
// sends an implicitly empty 200 OK header.
//
// If the handler didn't declare a Content-Length up front, we either
// go into chunking mode or, if the handler finishes running before
// the chunking buffer size, we compute a Content-Length and send that
// in the header instead.
//
// Likewise, if the handler didn't set a Content-Type, we sniff that
// from the initial chunk of output.
//
// The Writers are wired together like:
//
// 1. *response (the ResponseWriter) ->
// 2. (*response).w, a *bufio.Writer of bufferBeforeChunkingSize bytes
// 3. chunkWriter.Writer (whose writeHeader finalizes Content-Length/Type)
//    and which writes the chunk headers, if needed.
// 4. conn.buf, a bufio.Writer of default (4kB) bytes
// 5. the rwc, the net.Conn.
//
// TODO(bradfitz): short-circuit some of the buffering when the
// initial header contains both a Content-Type and Content-Length.
// Also short-circuit in (1) when the header's been sent and not in
// chunking mode, writing directly to (4) instead, if (2) has no
// buffered data.  More generally, we could short-circuit from (1) to
// (3) even in chunking mode if the write size from (1) is over some
// threshold and nothing is in (2).  The answer might be mostly making
// bufferBeforeChunkingSize smaller and having bufio's fast-paths deal
// with this instead.
func (w *response) Write(data []byte) (n int, err error) {
	return w.write(len(data), data, "")
}

func (w *response) WriteString(data string) (n int, err error) {
	return w.write(len(data), nil, data)
}

// either dataB or dataS is non-zero.
func (w *response) write(lenData int, dataB []byte, dataS string) (n int, err error) {
	if w.canWriteContinue.isSet() {
		// Body reader wants to write 100 Continue but hasn't yet.
		// Tell it not to. The store must be done while holding the lock
		// because the lock makes sure that there is not an active write
		// this very moment.
		w.writeContinueMu.Lock()
		w.canWriteContinue.setFalse()
		w.writeContinueMu.Unlock()
	}

	if !w.wroteHeader {
		w.WriteHeader(bfe_http.StatusOK)
	}
	if lenData == 0 {
		return 0, nil
	}
	if !w.bodyAllowed() {
		return 0, ErrBodyNotAllowed
	}

	w.written += int64(lenData) // ignoring errors, for errorKludge
	if w.contentLength != -1 && w.written > w.contentLength {
		return 0, ErrContentLength
	}
	if dataB != nil {
		return w.w.Write(dataB)
	}
	return w.w.WriteString(dataS)
}

func (w *response) finishRequest() {
	w.handlerDone = true

	if !w.wroteHeader {
		w.WriteHeader(bfe_http.StatusOK)
	}

	w.w.Flush()
	w.conn.server.BufioCache.putBufioWriter(w.w)
	w.cw.close()
	w.conn.buf.Flush()

	// Close the body, unless we're about to close the whole TCP connection
	// anyway.
	if !w.closeAfterReply {
		w.req.Body.Close()
	}
	if w.req.MultipartForm != nil {
		w.req.MultipartForm.RemoveAll()
	}

	if w.req.Method != "HEAD" && w.contentLength != -1 && w.bodyAllowed() && w.contentLength != w.written {
		// Did not write enough. Avoid getting out of sync.
		w.closeAfterReply = true
	}
}

func (w *response) Flush() error {
	if !w.wroteHeader {
		w.WriteHeader(bfe_http.StatusOK)
	}
	w.w.Flush()
	w.cw.flush()
	return nil
}

func (w *response) sendExpectationFailed() {
	// TODO(bradfitz): let ServeHTTP handlers handle
	// requests with non-standard expectation[s]? Seems
	// theoretical at best, and doesn't fit into the
	// current ServeHTTP model anyway.  We'd need to
	// make the ResponseWriter an optional
	// "ExpectReplier" interface or something.
	//
	// For now we'll just obey RFC 2616 14.20 which says
	// "If a server receives a request containing an
	// Expect field that includes an expectation-
	// extension that it does not support, it MUST
	// respond with a 417 (Expectation Failed) status."
	w.Header().Set("Connection", "close")
	w.WriteHeader(bfe_http.StatusExpectationFailed)
	w.finishRequest()
}

func (w *response) CloseNotify() <-chan bool {
	return w.conn.closeNotify()
}

func (w *response) prepareForCloseConn() {
	if w.req.MultipartForm != nil {
		w.req.MultipartForm.RemoveAll()
	}
}

// Hijack implements the Hijacker.Hijack method. Our response is both a ResponseWriter
// and a Hijacker.
func (w *response) Hijack() (rwc net.Conn, buf *bfe_bufio.ReadWriter, err error) {
	if w.hijackedv {
		return nil, nil, ErrHijacked
	}
	w.hijackedv = true

	if w.wroteHeader {
		w.cw.flush()
	}

	c := w.conn
	c.rwc.SetDeadline(time.Time{})
	return c.rwc, c.buf, nil
}
