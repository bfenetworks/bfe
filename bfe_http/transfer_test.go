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

// Issue #1494 (SC3101-TC006): a buffer retained for fallback/key retry replay
// must stay accounted for after Close() (the body was written to the main
// target, but the bytes are still held for Rewind), and only Release() at
// the request's terminal state settles the gauge. This is the gauge
// visibility the system test samples during the in-flight window.
func TestBytesBodyRetainForRetryCloseDoesNotRelease(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	data := make([]byte, 1024*1024) // 1 MiB, like the SC3101 hold request
	body, err := NewBytesBody(io.NopCloser(bytes.NewReader(data)), int64(len(data)+1))
	if err != nil {
		t.Fatalf("NewBytesBody failed: %v", err)
	}

	rb, ok := body.(RetryBuffer)
	if !ok {
		t.Fatal("bytes_body must implement RetryBuffer")
	}
	rb.RetainForRetry()

	if got := TotalBytesBodyBuffer(); got != int64(len(data)) {
		t.Fatalf("expected total %d after wrap, got %d", len(data), got)
	}

	if err := body.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if got := TotalBytesBodyBuffer(); got != int64(len(data)) {
		t.Fatalf("retained buffer must stay accounted after Close: total = %d, want %d", got, len(data))
	}

	ReleaseBodyBuffer(body)
	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0 after Release, got %d", got)
	}
}

// Issue #1494: rewind between attempts must keep the retained buffer
// accounted (previously SetBytes skipped accounting once released, so the
// replayed buffer never re-appeared on the gauge).
func TestBytesBodyRetainRewindKeepsAccounting(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	data := []byte("replay me")
	body, err := NewBytesBody(io.NopCloser(bytes.NewReader(data)), int64(len(data)+1))
	if err != nil {
		t.Fatalf("NewBytesBody failed: %v", err)
	}
	body.(RetryBuffer).RetainForRetry()

	// simulate: write to main target -> Close; fallback -> Rewind
	if err := body.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !body.(Rewindable).Rewind() {
		t.Fatal("Rewind failed")
	}
	if got := TotalBytesBodyBuffer(); got != int64(len(data)) {
		t.Fatalf("rewound retained buffer must stay accounted: total = %d, want %d", got, len(data))
	}

	// second attempt: write -> Close again, then terminal Release
	if err := body.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
	if got := TotalBytesBodyBuffer(); got != int64(len(data)) {
		t.Fatalf("retained buffer must stay accounted after second Close: total = %d, want %d", got, len(data))
	}

	ReleaseBodyBuffer(body)
	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0 after Release, got %d", got)
	}
}

// Issue #1494: Release is idempotent and a Close after Release must not
// double-subtract (the gauge must never go negative).
func TestBytesBodyReleaseIdempotentAndCloseAfterRelease(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	data := []byte("hello world")
	body, err := NewBytesBody(io.NopCloser(bytes.NewReader(data)), int64(len(data)+1))
	if err != nil {
		t.Fatalf("NewBytesBody failed: %v", err)
	}
	body.(RetryBuffer).RetainForRetry()

	ReleaseBodyBuffer(body)
	ReleaseBodyBuffer(body)
	if err := body.Close(); err != nil {
		t.Fatalf("Close after Release failed: %v", err)
	}

	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0 (no double subtract), got %d", got)
	}
}

// Issue #1494: SetBytes on an already released buffer re-activates the
// accounting (defensive branch); a later Release settles it again.
func TestBytesBodySetBytesReactivatesAfterRelease(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	data := []byte("hello")
	body, err := NewBytesBody(io.NopCloser(bytes.NewReader(data)), int64(len(data)+1))
	if err != nil {
		t.Fatalf("NewBytesBody failed: %v", err)
	}
	body.(RetryBuffer).RetainForRetry()
	ReleaseBodyBuffer(body)
	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0 after Release, got %d", got)
	}

	newData := []byte("hello world")
	body.(BodyAccessor).SetBytes(newData, true)
	if got := TotalBytesBodyBuffer(); got != int64(len(newData)) {
		t.Fatalf("SetBytes after Release must re-activate accounting: total = %d, want %d", got, len(newData))
	}

	ReleaseBodyBuffer(body)
	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("expected total 0 after second Release, got %d", got)
	}
}

// Issue #1494: the helper no-ops on non-replay buffers and on nil, so the
// reverseproxy defer is safe on every request.
func TestReleaseBodyBufferNoopOnForeignBodies(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	ReleaseBodyBuffer(nil)
	foreign := io.NopCloser(bytes.NewReader([]byte("x")))
	ReleaseBodyBuffer(foreign)
	RetainBodyBufferForRetry(foreign)
	if got := TotalBytesBodyBuffer(); got != 0 {
		t.Fatalf("foreign bodies must not touch the gauge, got %d", got)
	}
}

// The gauge always reflects the bytes actually held, even beyond
// TotalBodyBufferSize: the limit is enforced at the documented gate
// (prepareRequestBodyForRetry), not by dropping bytes from the accounting
// (issue #1494).
func TestAddTotalBytesBodyBufferCountsBeyondLimit(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	SetTotalBodyBufferSizeLimit(10)

	addTotalBytesBodyBuffer(5)
	if TotalBytesBodyBuffer() != 5 {
		t.Fatalf("expected total 5, got %d", TotalBytesBodyBuffer())
	}

	addTotalBytesBodyBuffer(5)
	if TotalBytesBodyBuffer() != 10 {
		t.Fatalf("expected total 10, got %d", TotalBytesBodyBuffer())
	}

	// Beyond the limit the gauge still counts the real bytes; enforcement
	// is the wrap gate, not the accounting.
	addTotalBytesBodyBuffer(1)
	if TotalBytesBodyBuffer() != 11 {
		t.Fatalf("expected total 11 (counted beyond limit), got %d", TotalBytesBodyBuffer())
	}
}

// Issue #1494: creating a buffer beyond TotalBodyBufferSize still succeeds
// and stays accounted — the limit gates retry wrapping in
// prepareRequestBodyForRetry, it must not produce unaccounted buffers (an
// uncounted buffer would under-report memory and drive the gauge negative
// when released).
func TestNewBytesBodyCountsBeyondLimit(t *testing.T) {
	resetTotalBytesBodyBuffer()
	defer resetTotalBytesBodyBuffer()

	SetTotalBodyBufferSizeLimit(10)

	if _, err := NewBytesBody(io.NopCloser(bytes.NewReader([]byte("12345"))), 6); err != nil {
		t.Fatalf("first NewBytesBody failed: %v", err)
	}
	if got := TotalBytesBodyBuffer(); got != 5 {
		t.Fatalf("expected total 5, got %d", got)
	}

	if _, err := NewBytesBody(io.NopCloser(bytes.NewReader([]byte("123456"))), 7); err != nil {
		t.Fatalf("second NewBytesBody failed: %v", err)
	}
	if got := TotalBytesBodyBuffer(); got != 11 {
		t.Fatalf("expected total 11 (both buffers accounted), got %d", got)
	}
}
