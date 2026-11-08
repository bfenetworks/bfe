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
	"bytes"
	"io"
	"testing"

	"github.com/bfenetworks/bfe/bfe_http"
)

// Issue #1494 (SC3101-TC006): prepareRequestBodyForRetry retains the wrapped
// body buffer for replay, so the gauge must keep the bytes accounted after
// the body is Closed (written to the main target), until the terminal
// ReleaseBodyBuffer settles it. Relative to the pre-test baseline because
// the gauge is process-global.
func TestPrepareRequestBodyForRetryRetainsBufferAccounting(t *testing.T) {
	bfe_http.SetTotalBodyBufferSizeLimit(0)
	defer bfe_http.SetTotalBodyBufferSizeLimit(0)

	data := []byte("a body retained for fallback replay")
	base := bfe_http.TotalBytesBodyBuffer()
	req := &bfe_http.Request{Body: io.NopCloser(bytes.NewReader(data))}

	if !prepareRequestBodyForRetry(req) {
		t.Fatal("prepareRequestBodyForRetry failed")
	}
	if got := bfe_http.TotalBytesBodyBuffer() - base; got != int64(len(data)) {
		t.Fatalf("gauge = base+%d, want base+%d after wrap", got, len(data))
	}

	// Writing the body to the main target closes it; the retained buffer
	// must stay accounted for the whole fallback window.
	if err := req.Body.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if got := bfe_http.TotalBytesBodyBuffer() - base; got != int64(len(data)) {
		t.Fatalf("retained buffer must stay accounted after Close: base+%d, want base+%d", got, len(data))
	}

	// Terminal settlement (the deferred release in ServeHTTPForAI).
	bfe_http.ReleaseBodyBuffer(req.Body)
	if got := bfe_http.TotalBytesBodyBuffer() - base; got != 0 {
		t.Fatalf("gauge must return to baseline after Release, still base+%d", got)
	}
}

// Issue #1494: while a retained buffer still occupies the gauge, the
// TotalBodyBufferSize limit must block wrapping another request — the
// capacity-protection semantics of bfe.conf.md Server.TotalBodyBufferSize.
func TestTotalBodyBufferLimitBlocksWrapWhileRetained(t *testing.T) {
	data := []byte("0123456789abcdef") // 16 bytes

	base := bfe_http.TotalBytesBodyBuffer()
	bfe_http.SetTotalBodyBufferSizeLimit(base + int64(len(data)))
	defer bfe_http.SetTotalBodyBufferSizeLimit(0)

	// First request wraps and retains its buffer: gauge reaches the limit.
	reqA := &bfe_http.Request{Body: io.NopCloser(bytes.NewReader(data))}
	if !prepareRequestBodyForRetry(reqA) {
		t.Fatal("first prepareRequestBodyForRetry failed")
	}
	if got := bfe_http.TotalBytesBodyBuffer() - base; got != int64(len(data)) {
		t.Fatalf("gauge = base+%d, want base+%d", got, len(data))
	}

	// Second request: the retained bytes are visible, so the wrap must be
	// rejected (fallback disabled) instead of bypassing the limit.
	reqB := &bfe_http.Request{Body: io.NopCloser(bytes.NewReader(data))}
	if prepareRequestBodyForRetry(reqB) {
		t.Error("second prepareRequestBodyForRetry must be blocked by the retained buffer")
	}
	if _, ok := reqB.Body.(bfe_http.Rewindable); ok {
		t.Error("blocked request body must not have been wrapped")
	}

	// After the terminal settlement the capacity is usable again.
	bfe_http.ReleaseBodyBuffer(reqA.Body)
	reqC := &bfe_http.Request{Body: io.NopCloser(bytes.NewReader(data))}
	if !prepareRequestBodyForRetry(reqC) {
		t.Error("prepareRequestBodyForRetry must succeed again after Release")
	}
	bfe_http.ReleaseBodyBuffer(reqC.Body)
	if got := bfe_http.TotalBytesBodyBuffer() - base; got != 0 {
		t.Fatalf("gauge must return to baseline, still base+%d", got)
	}
}
