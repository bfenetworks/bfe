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

package openai

import (
	"net/http"
	"testing"

	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

func TestParseErrorMapping(t *testing.T) {
	a := New()
	cases := []struct {
		name       string
		status     int
		body       string
		header     http.Header
		wantCode   string
		wantUpCode string
		wantNil    bool
	}{
		{"nil below 400", 200, `{"error":{"message":"x"}}`, nil, "", "", true},
		{"unrecognized body", 500, `{"message":"oops"}`, nil, "", "", true},
		{"plain text body", 502, `Bad Gateway`, nil, "", "", true},
		{
			"400 invalid request", 400,
			`{"error":{"message":"bad","type":"invalid_request_error"}}`, nil,
			utils.CodeUpstreamInvalidRequest, "invalid_request_error", false,
		},
		{
			"context length", 400,
			`{"error":{"message":"too long","type":"invalid_request_error","code":"context_length_exceeded"}}`, nil,
			utils.CodeContextLengthExceeded, "context_length_exceeded", false,
		},
		{
			"content policy", 400,
			`{"error":{"message":"filtered","type":"invalid_request_error","code":"content_policy_violation"}}`, nil,
			utils.CodeContentFiltered, "content_policy_violation", false,
		},
		{
			"upstream 401 auth", 401,
			`{"error":{"message":"Incorrect API key","type":"authentication_error","code":"invalid_api_key"}}`, nil,
			utils.CodeUpstreamAuthError, "invalid_api_key", false,
		},
		{
			"model not found 404", 404,
			`{"error":{"message":"no such model","type":"invalid_request_error","code":"model_not_found"}}`, nil,
			utils.CodeUpstreamModelNotFound, "model_not_found", false,
		},
		{
			"rate limited 429", 429,
			`{"error":{"message":"slow down","type":"rate_limit_error","code":"rate_limit_exceeded"}}`,
			http.Header{"Retry-After": []string{"30"}},
			utils.CodeUpstreamRateLimited, "rate_limit_exceeded", false,
		},
		{
			"insufficient quota 429", 429,
			`{"error":{"message":"no quota","type":"rate_limit_error","code":"insufficient_quota"}}`, nil,
			utils.CodeUpstreamQuotaExhausted, "insufficient_quota", false,
		},
		{
			"timeout 408", 408,
			`{"error":{"message":"timeout","type":"server_error"}}`, nil,
			utils.CodeBackendTimeout, "server_error", false,
		},
		{
			"internal 500", 500,
			`{"error":{"message":"boom","type":"server_error"}}`, nil,
			utils.CodeModelInternalError, "server_error", false,
		},
		{
			"nested code object", 400,
			`{"error":{"message":"weird","type":"invalid_request_error","code":{"some":"object"}}}`, nil,
			utils.CodeUpstreamInvalidRequest, "invalid_request_error", false,
		},
	}
	for _, c := range cases {
		perr := a.ParseError(c.status, []byte(c.body), c.header)
		if c.wantNil {
			if perr != nil {
				t.Errorf("%s: expected nil, got %+v", c.name, perr)
			}
			continue
		}
		if perr == nil {
			t.Errorf("%s: expected non-nil", c.name)
			continue
		}
		if perr.Code != c.wantCode {
			t.Errorf("%s: code = %q, want %q", c.name, perr.Code, c.wantCode)
		}
		if perr.UpstreamCode != c.wantUpCode {
			t.Errorf("%s: upstream code = %q, want %q", c.name, perr.UpstreamCode, c.wantUpCode)
		}
		if perr.StatusCode != c.status {
			t.Errorf("%s: status = %d, want %d", c.name, perr.StatusCode, c.status)
		}
	}
}

func TestParseErrorRetryAfter(t *testing.T) {
	a := New()
	header := http.Header{"Retry-After": []string{"42"}}
	perr := a.ParseError(429, []byte(`{"error":{"message":"slow","type":"rate_limit_error"}}`), header)
	if perr == nil || perr.RetryAfterSeconds != 42 {
		t.Errorf("RetryAfterSeconds = %v, want 42 (perr=%+v)", perr, perr)
	}
	// invalid Retry-After leaves the field at 0
	perr = a.ParseError(429, []byte(`{"error":{"message":"slow","type":"rate_limit_error"}}`), http.Header{"Retry-After": []string{"abc"}})
	if perr == nil || perr.RetryAfterSeconds != 0 {
		t.Errorf("RetryAfterSeconds = %v, want 0 (perr=%+v)", perr, perr)
	}
}

func TestParseStreamError(t *testing.T) {
	a := New()
	cases := []struct {
		name     string
		ev       utils.StreamEvent
		wantCode string
		wantNil  bool
	}{
		{"done marker", utils.StreamEvent{Data: "[DONE]"}, "", true},
		{"normal chunk", utils.StreamEvent{Data: `{"choices":[{"delta":{"content":"hi"}}]}`}, "", true},
		{
			"error event",
			utils.StreamEvent{Data: `{"error":{"message":"rate limited","type":"server_error","code":"rate_limit_exceeded"}}`},
			utils.CodeUpstreamRateLimited, false,
		},
		{
			"error event no code",
			utils.StreamEvent{Data: `{"error":{"message":"bad request","type":"invalid_request_error"}}`},
			utils.CodeUpstreamInvalidRequest, false,
		},
	}
	for _, c := range cases {
		perr := a.ParseStreamError(c.ev)
		if c.wantNil {
			if perr != nil {
				t.Errorf("%s: expected nil, got %+v", c.name, perr)
			}
			continue
		}
		if perr == nil {
			t.Errorf("%s: expected non-nil", c.name)
			continue
		}
		if perr.Code != c.wantCode {
			t.Errorf("%s: code = %q, want %q", c.name, perr.Code, c.wantCode)
		}
		if perr.StatusCode != 200 {
			t.Errorf("%s: status = %d, want 200", c.name, perr.StatusCode)
		}
	}
}
