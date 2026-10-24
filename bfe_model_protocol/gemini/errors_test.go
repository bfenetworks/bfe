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

package gemini

import (
	"testing"

	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

func TestStreamEndsAtEOF(t *testing.T) {
	if !New().StreamEndsAtEOF() {
		t.Error("Gemini streams end at HTTP EOF; StreamEndsAtEOF must be true")
	}
}

func TestParseErrorMapping(t *testing.T) {
	a := New()
	cases := []struct {
		name       string
		status     int
		body       string
		wantCode   string
		wantUpCode string
		wantNil    bool
	}{
		{"below 400", 200, `{"error":{"status":"INTERNAL","message":"x"}}`, "", "", true},
		{"unrecognized body", 500, `{"message":"oops"}`, "", "", true},
		{
			"invalid argument", 400,
			`{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"bad"}}`,
			utils.CodeUpstreamInvalidRequest, "INVALID_ARGUMENT", false,
		},
		{
			"unauthenticated", 401,
			`{"error":{"code":401,"status":"UNAUTHENTICATED","message":"bad key"}}`,
			utils.CodeUpstreamAuthError, "UNAUTHENTICATED", false,
		},
		{
			"not found", 404,
			`{"error":{"code":404,"status":"NOT_FOUND","message":"no model"}}`,
			utils.CodeUpstreamModelNotFound, "NOT_FOUND", false,
		},
		{
			"resource exhausted rate", 429,
			`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"rate limit exceeded"}}`,
			utils.CodeUpstreamRateLimited, "RESOURCE_EXHAUSTED", false,
		},
		{
			"resource exhausted quota", 429,
			`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"quota balance is zero"}}`,
			utils.CodeUpstreamQuotaExhausted, "RESOURCE_EXHAUSTED", false,
		},
		{
			"deadline exceeded", 504,
			`{"error":{"code":504,"status":"DEADLINE_EXCEEDED","message":"timeout"}}`,
			utils.CodeBackendTimeout, "DEADLINE_EXCEEDED", false,
		},
		{
			"internal", 500,
			`{"error":{"code":500,"status":"INTERNAL","message":"boom"}}`,
			utils.CodeModelInternalError, "INTERNAL", false,
		},
		{
			"no status enum fallback", 403,
			`{"error":{"code":403,"message":"denied"}}`,
			utils.CodeUpstreamAuthError, "403", false,
		},
	}
	for _, c := range cases {
		perr := a.ParseError(c.status, []byte(c.body), nil)
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
		{
			"normal chunk",
			utils.StreamEvent{Data: `{"candidates":[{"content":{"parts":[{"text":"hi"}]}}],"usageMetadata":{"totalTokenCount":5}}`},
			"", true,
		},
		{
			"error chunk",
			utils.StreamEvent{Data: `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"quota exhausted"}}`},
			utils.CodeUpstreamQuotaExhausted, false,
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
	}
}
