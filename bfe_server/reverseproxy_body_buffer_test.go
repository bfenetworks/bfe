// Copyright (c) 2026 The BFE Authors.
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

package bfe_server

import (
	"io"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func newBodyBufferTestRequest(body string) *bfe_basic.Request {
	httpReq := &bfe_http.Request{
		Method: "POST",
		Header: bfe_http.Header{},
		Body:   io.NopCloser(strings.NewReader(body)),
	}
	outreq := new(bfe_http.Request)
	// Mirrors bfe_basic.NewRequest: the registry is initialized before the
	// shallow copy so HttpRequest and OutRequest share the same lifecycle.
	httpReq.InitBodyBufferTracking()
	*outreq = *httpReq
	return &bfe_basic.Request{HttpRequest: httpReq, OutRequest: outreq}
}

// TestPrepareRequestBodyForRetryKeepsAccounting covers issue #1494: the
// buffer must stay counted while it is retained for a possible replay, even
// after the transport closed the body, and must be released once the request
// can no longer be retried.
func TestPrepareRequestBodyForRetryKeepsAccounting(t *testing.T) {
	body := "hello world"
	req := newBodyBufferTestRequest(body)
	before := bfe_http.TotalBytesBodyBuffer()

	if !prepareRequestBodyForRetry(req.HttpRequest) {
		t.Fatal("expected request body to be prepared for retry")
	}
	if got := bfe_http.TotalBytesBodyBuffer() - before; got != int64(len(body)) {
		t.Fatalf("expected accounting delta %d, got %d", len(body), got)
	}

	// First attempt: the transport writes the body and closes it.
	first, err := io.ReadAll(req.HttpRequest.Body)
	if err != nil {
		t.Fatalf("read first attempt failed: %v", err)
	}
	if err := req.HttpRequest.Body.Close(); err != nil {
		t.Fatalf("close body failed: %v", err)
	}
	if got := bfe_http.TotalBytesBodyBuffer() - before; got != int64(len(body)) {
		t.Fatalf("expected accounting delta %d after Close, got %d", len(body), got)
	}

	// Fallback replays the retained buffer.
	if !rewindRequestBody(req.HttpRequest) {
		t.Fatal("expected rewind to succeed for a retained buffer")
	}
	second, err := io.ReadAll(req.HttpRequest.Body)
	if err != nil {
		t.Fatalf("read second attempt failed: %v", err)
	}
	if string(first) != body || string(second) != body {
		t.Fatalf("unexpected replay: first=%q second=%q want %q", first, second, body)
	}

	req.HttpRequest.ReleaseBodyBuffers()
	if got := bfe_http.TotalBytesBodyBuffer(); got != before {
		t.Fatalf("expected accounting back to %d, got %d", before, got)
	}
}

func TestPrepareRequestBodyForRetryRefusesAtLimit(t *testing.T) {
	oldLimit := bfe_http.TotalBodyBufferSizeLimit()
	defer bfe_http.SetTotalBodyBufferSizeLimit(oldLimit)

	// Occupy the budget with one retained request body.
	holder := newBodyBufferTestRequest("x")
	if !prepareRequestBodyForRetry(holder.HttpRequest) {
		t.Fatal("expected holder body to be prepared")
	}
	defer holder.HttpRequest.ReleaseBodyBuffers()
	bfe_http.SetTotalBodyBufferSizeLimit(bfe_http.TotalBytesBodyBuffer())
	before := bfe_http.TotalBytesBodyBuffer()

	req := newBodyBufferTestRequest("hello world")
	if prepareRequestBodyForRetry(req.HttpRequest) {
		t.Fatal("expected prepare to fail once the buffer budget is exhausted")
	}
	if _, ok := req.HttpRequest.Body.(bfe_http.Rewindable); ok {
		t.Fatal("body must not be buffered when the limit is reached")
	}
	if got := bfe_http.TotalBytesBodyBuffer(); got != before {
		t.Fatalf("expected accounting unchanged %d, got %d", before, got)
	}
}
