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

package bfe_basic

import (
	"strconv"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_http"
)

func newCorrelationTestRequest(logId string, inboundHeaders map[string]string) *Request {
	httpReq := &bfe_http.Request{Header: bfe_http.Header{}}
	for k, v := range inboundHeaders {
		httpReq.Header.Set(k, v)
	}
	return &Request{HttpRequest: httpReq, LogId: logId}
}

func TestCreateErrorResponseCorrelationHeaders(t *testing.T) {
	testErr := NewAiError(CodeNoAvailableKey, TypeRateLimitError, "no available upstream api key")

	t.Run("req.LogId wins and is written back", func(t *testing.T) {
		req := newCorrelationTestRequest("12345", map[string]string{HeaderBfeLogId: "99999"})
		res := testErr.CreateErrorResponse(req)
		if got := res.Header.Get(HeaderBfeLogId); got != "12345" {
			t.Errorf("X-Bfe-Log-Id = %q, want req.LogId 12345", got)
		}
	})

	t.Run("inbound X-Bfe-Log-Id echoed and back-filled to req.LogId", func(t *testing.T) {
		req := newCorrelationTestRequest("", map[string]string{HeaderBfeLogId: "77777"})
		res := testErr.CreateErrorResponse(req)
		if got := res.Header.Get(HeaderBfeLogId); got != "77777" {
			t.Errorf("X-Bfe-Log-Id = %q, want inbound 77777", got)
		}
		if req.LogId != "77777" {
			t.Errorf("req.LogId = %q, want back-fill 77777", req.LogId)
		}
		if got := req.HttpRequest.Header.Get(HeaderBfeLogId); got != "77777" {
			t.Errorf("request header X-Bfe-Log-Id = %q, want back-fill 77777", got)
		}
	})

	t.Run("generated when nothing present", func(t *testing.T) {
		req := newCorrelationTestRequest("", nil)
		res := testErr.CreateErrorResponse(req)
		got := res.Header.Get(HeaderBfeLogId)
		v, err := strconv.ParseUint(got, 10, 64)
		if err != nil || v == 0 || v > uint64(1)<<63-1 {
			t.Errorf("X-Bfe-Log-Id = %q, want decimal id in (0, 2^63-1]", got)
		}
		if req.LogId != got {
			t.Errorf("req.LogId = %q, want back-fill %q", req.LogId, got)
		}
		if req.HttpRequest.Header.Get(HeaderBfeLogId) != got {
			t.Errorf("request header X-Bfe-Log-Id = %q, want back-fill %q", req.HttpRequest.Header.Get(HeaderBfeLogId), got)
		}
	})

	t.Run("client X-Request-ID echoed byte-identical", func(t *testing.T) {
		req := newCorrelationTestRequest("12345", map[string]string{HeaderXRequestId: "req-id-ABC-123"})
		res := testErr.CreateErrorResponse(req)
		if got := res.Header.Get(HeaderXRequestId); got != "req-id-ABC-123" {
			t.Errorf("X-Request-ID = %q, want byte-identical echo", got)
		}
		if got := res.Header.Get(HeaderBfeLogId); got != "12345" {
			t.Errorf("X-Bfe-Log-Id = %q, want 12345", got)
		}
	})

	t.Run("X-Request-ID absent when client did not send one", func(t *testing.T) {
		req := newCorrelationTestRequest("12345", nil)
		res := testErr.CreateErrorResponse(req)
		if got := res.Header.Get(HeaderXRequestId); got != "" {
			t.Errorf("X-Request-ID = %q, want absent (gateway must not invent one)", got)
		}
	})

	t.Run("nil HttpRequest does not panic", func(t *testing.T) {
		req := &Request{LogId: "12345"}
		res := testErr.CreateErrorResponse(req)
		if got := res.Header.Get(HeaderBfeLogId); got != "" {
			t.Errorf("X-Bfe-Log-Id = %q, want absent when HttpRequest is nil", got)
		}
	})

	t.Run("X-Bfe-Gw-Error and body unchanged", func(t *testing.T) {
		req := newCorrelationTestRequest("12345", nil)
		res := testErr.CreateErrorResponse(req)
		if res.Header.Get(HeaderBfeGwError) != "1" {
			t.Errorf("%s = %q, want 1", HeaderBfeGwError, res.Header.Get(HeaderBfeGwError))
		}
		if !strings.Contains(res.Header.Get("Content-Type"), "application/json") {
			t.Errorf("Content-Type = %q, want application/json", res.Header.Get("Content-Type"))
		}
	})
}
