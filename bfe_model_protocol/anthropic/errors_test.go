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

package anthropic

import (
	"testing"

	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

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
		{"below 400", 200, `{"type":"error","error":{"type":"api_error","message":"x"}}`, "", "", true},
		{"missing type error", 500, `{"error":{"type":"api_error","message":"x"}}`, "", "", true},
		{"unrecognized body", 500, `{"message":"oops"}`, "", "", true},
		{
			"invalid request", 400,
			`{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`,
			utils.CodeUpstreamInvalidRequest, "invalid_request_error", false,
		},
		{
			"request too large", 400,
			`{"type":"error","error":{"type":"request_too_large","message":"big"}}`,
			utils.CodeContextLengthExceeded, "request_too_large", false,
		},
		{
			"authentication", 401,
			`{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`,
			utils.CodeUpstreamAuthError, "authentication_error", false,
		},
		{
			"permission", 403,
			`{"type":"error","error":{"type":"permission_error","message":"denied"}}`,
			utils.CodeUpstreamAuthError, "permission_error", false,
		},
		{
			"not found", 404,
			`{"type":"error","error":{"type":"not_found_error","message":"no model"}}`,
			utils.CodeUpstreamModelNotFound, "not_found_error", false,
		},
		{
			"rate limit", 429,
			`{"type":"error","error":{"type":"rate_limit_error","message":"slow"}}`,
			utils.CodeUpstreamRateLimited, "rate_limit_error", false,
		},
		{
			"overloaded", 529,
			`{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`,
			utils.CodeUpstreamOverloaded, "overloaded_error", false,
		},
		{
			"api error", 500,
			`{"type":"error","error":{"type":"api_error","message":"boom"}}`,
			utils.CodeModelInternalError, "api_error", false,
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
		{"normal message_start", utils.StreamEvent{Type: "message_start", Data: `{"type":"message_start"}`}, "", true},
		{"content block", utils.StreamEvent{Type: "content_block_delta", Data: `{"type":"content_block_delta"}`}, "", true},
		{
			"error event",
			utils.StreamEvent{Type: "error", Data: `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`},
			utils.CodeUpstreamOverloaded, false,
		},
		{
			"rate limit event",
			utils.StreamEvent{Type: "error", Data: `{"type":"error","error":{"type":"rate_limit_error","message":"slow"}}`},
			utils.CodeUpstreamRateLimited, false,
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
