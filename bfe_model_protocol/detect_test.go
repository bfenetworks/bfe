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

package bfe_model_protocol

import (
	"net/http"
	"testing"

	"github.com/bfenetworks/bfe/bfe_http"
)

func newDetectTestRequest(t *testing.T, path, auth, xApiKey string) *bfe_http.Request {
	t.Helper()
	req, err := bfe_http.NewRequest(http.MethodPost, "http://example.com"+path, nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if xApiKey != "" {
		req.Header.Set("x-api-key", xApiKey)
	}
	return req
}

func TestDetectProtocolAndKey(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		auth      string
		xApiKey   string
		wantProto string
		wantKey   string
	}{
		{"authorization bearer with sk prefix", "/v1/chat/completions", "Bearer sk-abc123", "", ProtocolOpenAI, "abc123"},
		{"authorization bare sk key", "/v1/chat/completions", "sk-xyz789", "", ProtocolOpenAI, "xyz789"},
		{"authorization bearer plain key", "/v1/chat/completions", "Bearer plainkey", "", ProtocolOpenAI, "plainkey"},
		{"x-api-key fallback", "/v1/messages", "", "ak-ant", ProtocolAnthropic, "ak-ant"},
		{"both headers prefer authorization", "/v1/messages", "Bearer sk-abc123", "ak-ant", ProtocolOpenAI, "abc123"},
		{"no credential headers", "/v1/chat/completions", "", "", "", ""},
	}

	for _, tc := range cases {
		req := newDetectTestRequest(t, tc.path, tc.auth, tc.xApiKey)
		proto, key := DetectProtocolAndKey(req)
		if proto != tc.wantProto || key != tc.wantKey {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", tc.name, proto, key, tc.wantProto, tc.wantKey)
		}
	}
}

func TestDetectProtocolAndKeySkPrefixOnlyAfterBearer(t *testing.T) {
	// "sk-" prefix is stripped once, even without "Bearer "
	req := newDetectTestRequest(t, "/v1/chat/completions", "sk-sk-double", "")
	_, key := DetectProtocolAndKey(req)
	if key != "sk-double" {
		t.Errorf("expected single sk- strip, got %q", key)
	}
}

func TestDetectProtocol(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		auth    string
		xApiKey string
		want    string
	}{
		{"nil request", "", "", "", ProtocolUnknown},
		{"messages path", "/v1/messages", "", "", ProtocolAnthropic},
		{"messages path with subpath", "/v1/messages/count_tokens", "", "", ProtocolAnthropic},
		{"x-api-key without authorization", "/v1/chat/completions", "", "ak-ant", ProtocolAnthropic},
		{"both headers", "/v1/chat/completions", "Bearer sk-abc", "ak-ant", ProtocolOpenAI},
		{"default openai", "/v1/chat/completions", "", "", ProtocolOpenAI},
	}

	for _, tc := range cases {
		var req *bfe_http.Request
		if tc.name != "nil request" {
			req = newDetectTestRequest(t, tc.path, tc.auth, tc.xApiKey)
		}
		if got := DetectProtocol(req); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
