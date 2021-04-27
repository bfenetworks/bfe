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

// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_spdy

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"sync"
)

import (
	http "github.com/bfenetworks/bfe/bfe_http"
)

var VerboseLogs = false

const (
	// NextProtoTLS is the NPN/ALPN protocol negotiated during SPDY's TLS setup.
	NextProtoTLS = "spdy/3.1"

	initialWindowSize = 65536

	defaultMaxReadFrameSize = 1 << 20

	defaultMaxWriteFrameSize = 1 << 14
)

// stream state for server side
//
//      recv S       recv ES                   send ES
// Idle ------> Open -------> HalfClosedRemote -------> Closed
//   |                               ^         send R
//   |        send US                |         recv R
//   +-------------------------------+
//
// send: endpoint sends this frame
// recv: endpoint receives this frame
//
// S : SynStream frame
// US: SynStream frame with UNIDIRECTIONAL flag
// ES: frame with END_STREAM flag
// R : Reset frame
type streamState int

const (
	stateIdle streamState = iota
	stateOpen
	stateHalfClosedLocal
	stateHalfClosedRemote
	stateResvLocal
	stateResvRemote
	stateClosed
)

var stateName = [...]string{
	stateIdle:             "Idle",
	stateOpen:             "Open",
	stateHalfClosedLocal:  "HalfClosedLocal",
	stateHalfClosedRemote: "HalfClosedRemote",
	stateResvLocal:        "ResvLocal",
	stateResvRemote:       "ResvRemote",
	stateClosed:           "Closed",
}

type timeoutTag int

const (
	ConnTag timeoutTag = iota
	ReadStreamTag
	WriteStreamTag
)

var tagString = map[timeoutTag]string{
	ConnTag:        "connection",
	ReadStreamTag:  "read stream",
	WriteStreamTag: "write stream",
}

func (t timeoutTag) String() string {
	return tagString[t]
}

func (st streamState) String() string {
	return stateName[st]
}

// ConnectionError is an error that results in the termination of the
// entire connection.
type ConnectionError RstStreamStatus

func (e ConnectionError) Error() string {
	return fmt.Sprintf("connection error: %v", RstStreamStatus(e))
}

// StreamError is an error that only affects one stream within an spdy connection.
type StreamError struct {
	StreamID uint32
	Code     RstStreamStatus
}

func (e StreamError) Error() string {
	return fmt.Sprintf("stream error: stream ID %d; %v", e.StreamID, e.Code)
}

type goAwayFlowError struct{}

func (goAwayFlowError) Error() string {
	return "connection exceeded flow control window size"
}

const (
	headerMethod  = ":method"
	headerPath    = ":path"
	headerVersion = ":version"
	headerHost    = ":host"
	headerScheme  = ":scheme"
)

func validHeader(v string) bool {
	if len(v) == 0 {
		return false
	}
	for _, r := range v {
		// "Just as in HTTP/1.x, header field names are
		// strings of ASCII characters that are compared in a
		// case-insensitive fashion. However, header field
		// names MUST be converted to lowercase prior to their
		// encoding in SPDY. "
		if r >= 127 || ('A' <= r && r <= 'Z') {
			return false
		}
	}
	return true
}

var httpCodeStringCommon = map[int]string{} // n -> strconv.Itoa(n)

func init() {
	for i := 100; i <= 999; i++ {
		if v := http.StatusTextGet(i); v != "" {
			httpCodeStringCommon[i] = strconv.Itoa(i)
		}
	}
}

func httpCodeString(code int) string {
	if s, ok := httpCodeStringCommon[code]; ok {
		return s
	}
	return strconv.Itoa(code)
}

// from pkg io
type stringWriter interface {
	WriteString(s string) (n int, err error)
}

// A closeWaiter is like a sync.WaitGroup but only goes 1 to 0 (open to closed).
type closeWaiter chan struct{}

// Init makes a closeWaiter usable.
// It exists because so a closeWaiter value can be placed inside a
// larger struct and have the Mutex and Cond's memory in the same
// allocation.
func (cw *closeWaiter) Init() {
	*cw = make(chan struct{})
}

// Close marks the closeWaiter as closed and unblocks any waiters.
func (cw closeWaiter) Close() {
	close(cw)
}

// Wait waits for the closeWaiter to become closed.
func (cw closeWaiter) Wait() {
	<-cw
}

// bufferedWriter is a buffered writer that writes to w.
// Its buffered writer is lazily allocated as needed, to minimize
// idle memory usage with many connections.
type bufferedWriter struct {
	w  io.Writer     // immutable
	bw *bufio.Writer // non-nil when data is buffered
}

func newBufferedWriter(w io.Writer) *bufferedWriter {
	return &bufferedWriter{w: w}
}

var bufWriterPool = sync.Pool{
	New: func() interface{} {
		// TODO: pick something better? this is a bit under
		// (3 x typical 1500 byte MTU) at least.
		return bufio.NewWriterSize(nil, 4<<10)
	},
}

func (w *bufferedWriter) Write(p []byte) (n int, err error) {
	if w.bw == nil {
		bw := bufWriterPool.Get().(*bufio.Writer)
		bw.Reset(w.w)
		w.bw = bw
	}
	return w.bw.Write(p)
}

func (w *bufferedWriter) Flush() error {
	bw := w.bw
	if bw == nil {
		return nil
	}
	err := bw.Flush()
	bw.Reset(nil)
	bufWriterPool.Put(bw)
	w.bw = nil
	return err
}

func mustUint31(v int32) uint32 {
	if v < 0 || v > 2147483647 {
		panic("out of range")
	}
	return uint32(v)
}

// CloseConn close underlying connection for request
func CloseConn(body io.ReadCloser) {
	if b, ok := body.(*RequestBody); ok {
		if b.conn != nil {
			b.conn.Close()
		}
	}
}

var spdyLimiter http.FlowLimiter

// SetFlowLimiter init flow limiter for spdy
func SetFlowLimiter(limiter http.FlowLimiter) {
	spdyLimiter = limiter
}

func acceptConn() bool {
	if spdyLimiter == nil {
		return true
	}
	return spdyLimiter.AcceptConn()
}

func acceptRequest() bool {
	if spdyLimiter == nil {
		return true
	}
	return spdyLimiter.AcceptRequest()
}
