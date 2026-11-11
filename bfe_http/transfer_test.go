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

// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_http

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bfenetworks/bfe/bfe_bufio"
)

func TestBodyReadBadTrailer(t *testing.T) {
	b := &body{
		src: strings.NewReader("foobar"),
		hdr: true, // force reading the trailer
		r:   bfe_bufio.NewReader(strings.NewReader("")),
	}
	buf := make([]byte, 7)
	n, err := b.Read(buf[:3])
	got := string(buf[:n])
	if got != "foo" || err != nil {
		t.Fatalf(`first Read = %d (%q), %v; want 3 ("foo")`, n, got, err)
	}

	n, err = b.Read(buf[:])
	got = string(buf[:n])
	if got != "bar" || err != nil {
		t.Fatalf(`second Read = %d (%q), %v; want 3 ("bar")`, n, got, err)
	}

	n, err = b.Read(buf[:])
	got = string(buf[:n])
	if err == nil {
		t.Errorf("final Read was successful (%q), expected error from trailer read", got)
	}
}

func resetTotalBytesBodyBuffer() {
	atomic.StoreInt64(&totalBytesBodyBuffer, 0)
	SetTotalBodyBufferSizeLimit(0)
}

func TestBytesBodyBufferAccounting(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	data := []byte("hello world")
	body, err := NewBytesBody(io.NopCloser(bytes.NewReader(data)), int64(len(data)+1))
	if err != nil {
		t.Fatalf("NewBytesBody failed: %v", err)
	}

	if got := TotalBytesBodyBuffer(); got != int64(len(data)) {
		t.Fatalf("expected total %d, got %d", len(data), got)
	}

	if err := body.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0 after close, got %d", got)
	}
}

func TestBytesBodySetBytesAdjustsTotal(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	data := []byte("hello")
	body, err := NewBytesBody(io.NopCloser(bytes.NewReader(data)), int64(len(data)+1))
	if err != nil {
		t.Fatalf("NewBytesBody failed: %v", err)
	}

	if got := TotalBytesBodyBuffer(); got != int64(len(data)) {
		t.Fatalf("expected total %d, got %d", len(data), got)
	}

	newData := []byte("hello world")
	body.(BodyAccessor).SetBytes(newData, true)

	if got := TotalBytesBodyBuffer(); got != int64(len(newData)) {
		t.Fatalf("expected total %d after SetBytes, got %d", len(newData), got)
	}

	if err := body.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0 after close, got %d", got)
	}
}

func TestAddTotalBytesBodyBufferCountsBeyondLimit(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	SetTotalBodyBufferSizeLimit(10)

	// The limit is an admission gate: once a buffer exists, it must be
	// accounted for even if the total exceeds the limit, otherwise the gauge
	// under-reports and the later release pushes it below zero.
	addTotalBytesBodyBuffer(5)
	addTotalBytesBodyBuffer(10)
	if got := TotalBytesBodyBuffer(); got != 15 {
		t.Fatalf("expected total 15, got %d", got)
	}

	subTotalBytesBodyBuffer(15)
	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0, got %d", got)
	}
}

// newTestRequest returns a request whose body can be buffered by
// GetBodyAccessor, mirroring a server-side request.
func newTestRequest(data []byte) *Request {
	return &Request{
		Body:        io.NopCloser(bytes.NewReader(data)),
		bodyBuffers: &bodyBufferRegistry{},
	}
}

func TestRequestBodyCloseDoesNotReleaseAccounting(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	data := []byte("hello world")
	req := newTestRequest(data)

	if _, err := req.GetBodyAccessor(); err != nil {
		t.Fatalf("GetBodyAccessor failed: %v", err)
	}
	if got := TotalBytesBodyBuffer(); got != int64(len(data)) {
		t.Fatalf("expected total %d, got %d", len(data), got)
	}

	// The transport closes the body after writing it, but the buffer is still
	// alive and may be rewound for a retry: the accounting must stay.
	if err := req.Body.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if got := TotalBytesBodyBuffer(); got != int64(len(data)) {
		t.Fatalf("expected total %d after Close, got %d", len(data), got)
	}

	if !req.Body.(Rewindable).Rewind() {
		t.Fatal("expected Rewind to succeed while the request is alive")
	}
	if got := TotalBytesBodyBuffer(); got != int64(len(data)) {
		t.Fatalf("expected total %d after Rewind, got %d", len(data), got)
	}

	req.ReleaseBodyBuffers()
	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0 after ReleaseBodyBuffers, got %d", got)
	}

	// Release is idempotent.
	req.ReleaseBodyBuffers()
	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0 after second release, got %d", got)
	}
}

func TestRequestRewindAfterReleaseIsRejected(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	data := []byte("hello")
	req := newTestRequest(data)
	acc, err := req.GetBodyAccessor()
	if err != nil {
		t.Fatalf("GetBodyAccessor failed: %v", err)
	}

	req.ReleaseBodyBuffers()
	if req.Body.(Rewindable).Rewind() {
		t.Fatal("expected Rewind to fail after the buffer was released")
	}

	// A released buffer must not be re-accounted, otherwise it would leak.
	acc.SetBytes(data, true)
	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0, got %d", got)
	}
}

func TestRequestBodyConcurrentCloseAndRelease(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	req := newTestRequest([]byte("hello world"))
	if _, err := req.GetBodyAccessor(); err != nil {
		t.Fatalf("GetBodyAccessor failed: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = req.Body.Close()
	}()
	go func() {
		defer wg.Done()
		req.ReleaseBodyBuffers()
	}()
	wg.Wait()

	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0, got %d", got)
	}
}

func TestRequestBodyBufferSharedWithShallowCopy(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	// HTTP/2 and SPDY requests are not built by ReadRequest, so bfe_basic.NewRequest
	// initializes the registry before OutRequest is made as a shallow copy. The
	// copy and the original must share the same accounting lifecycle.
	req := new(Request)
	req.InitBodyBufferTracking()
	req.Body = io.NopCloser(bytes.NewReader([]byte("hello")))

	outreq := new(Request)
	*outreq = *req

	if _, err := outreq.GetBodyAccessor(); err != nil {
		t.Fatalf("GetBodyAccessor failed: %v", err)
	}
	if got := TotalBytesBodyBuffer(); got != 5 {
		t.Fatalf("expected total 5, got %d", got)
	}

	// Releasing through the original settles the buffer created on the copy.
	req.ReleaseBodyBuffers()
	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0, got %d", got)
	}
}

func TestRequestBodyBufferedAfterReleaseIsSettled(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	req := newTestRequest([]byte("hello"))
	if _, err := req.GetBodyAccessor(); err != nil {
		t.Fatalf("GetBodyAccessor failed: %v", err)
	}
	req.ReleaseBodyBuffers()

	// A late body buffered after the request finished can never be rewound by
	// a retry, so its accounting must be settled immediately.
	req.Body = io.NopCloser(bytes.NewReader([]byte("hello world")))
	if _, err := req.GetBodyAccessor(); err != nil {
		t.Fatalf("GetBodyAccessor failed: %v", err)
	}
	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0, got %d", got)
	}
}
