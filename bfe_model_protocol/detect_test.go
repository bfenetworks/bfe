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

func newGeminiDetectTestRequest(t *testing.T, path, auth, xGoogApiKey string) *bfe_http.Request {
	t.Helper()
	req, err := bfe_http.NewRequest(http.MethodPost, "http://example.com"+path, nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if xGoogApiKey != "" {
		req.Header.Set("x-goog-api-key", xGoogApiKey)
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

func TestDetectProtocolAndKeyGemini(t *testing.T) {
	// x-goog-api-key alone: gemini style, appended at the end of the
	// detection chain (Authorization and x-api-key keep their priority).
	req := newGeminiDetectTestRequest(t, "/v1beta/models/gemini-2.5-flash:generateContent", "", "goog-key")
	proto, key := DetectProtocolAndKey(req)
	if proto != ProtocolGemini || key != "goog-key" {
		t.Errorf("got (%q, %q), want (%q, %q)", proto, key, ProtocolGemini, "goog-key")
	}

	// Authorization wins over x-goog-api-key (existing priority unchanged).
	req = newGeminiDetectTestRequest(t, "/v1beta/models/gemini-2.5-flash:generateContent", "Bearer sk-abc123", "goog-key")
	proto, key = DetectProtocolAndKey(req)
	if proto != ProtocolOpenAI || key != "abc123" {
		t.Errorf("got (%q, %q), want (%q, %q)", proto, key, ProtocolOpenAI, "abc123")
	}

	// x-api-key wins over x-goog-api-key (existing fallback priority).
	req = newGeminiDetectTestRequest(t, "/v1/messages", "", "goog-key")
	req.Header.Set("x-api-key", "ak-ant")
	proto, key = DetectProtocolAndKey(req)
	if proto != ProtocolAnthropic || key != "ak-ant" {
		t.Errorf("got (%q, %q), want (%q, %q)", proto, key, ProtocolAnthropic, "ak-ant")
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

func TestDetectProtocolGemini(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		auth    string
		xGoog   string
		xApiKey string
		want    string
	}{
		{"generateContent path", "/v1beta/models/gemini-2.5-flash:generateContent", "", "", "", ProtocolGemini},
		{"streamGenerateContent path", "/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse", "", "", "", ProtocolGemini},
		{"models prefix path", "/v1beta/models/gemini-2.5-flash", "", "", "", ProtocolGemini},
		{"x-goog-api-key without authorization", "/v1beta/models/gemini-2.5-flash:generateContent", "", "goog-key", "", ProtocolGemini},
		{"x-goog-api-key on openai path", "/v1/chat/completions", "", "goog-key", "", ProtocolGemini},
		// Authorization keeps priority over x-goog-api-key.
		{"authorization wins over x-goog-api-key", "/v1/chat/completions", "Bearer sk-abc", "goog-key", "", ProtocolOpenAI},
		// x-api-key keeps its fallback priority over x-goog-api-key.
		{"x-api-key wins over x-goog-api-key", "/v1/chat/completions", "", "goog-key", "ak-ant", ProtocolAnthropic},
		// Gemini path wins even with x-api-key present (path shape is decisive).
		{"gemini path with x-api-key", "/v1beta/models/gemini-2.5-flash:generateContent", "", "", "ak-ant", ProtocolGemini},
		// Existing anthropic / openai recognition is unaffected.
		{"messages path still anthropic", "/v1/messages", "", "", "", ProtocolAnthropic},
		{"x-api-key still anthropic", "/v1/chat/completions", "", "", "ak-ant", ProtocolAnthropic},
		{"default stays openai", "/v1/chat/completions", "", "", "", ProtocolOpenAI},
	}

	for _, tc := range cases {
		req := newGeminiDetectTestRequest(t, tc.path, tc.auth, tc.xGoog)
		if tc.xApiKey != "" {
			req.Header.Set("x-api-key", tc.xApiKey)
		}
		if got := DetectProtocol(req); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
