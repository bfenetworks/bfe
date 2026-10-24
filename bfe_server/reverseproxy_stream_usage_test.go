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
	"io/ioutil"
	"net/url"
	"strings"
	"testing"

	"github.com/tidwall/gjson"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func newStreamUsageTestRequest(body string) (*bfe_basic.Request, *bfe_basic.AiBasicInfo) {
	httpReq := &bfe_http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/v1/chat/completions"},
		Header: bfe_http.Header{},
		Body:   ioutil.NopCloser(strings.NewReader(body)),
	}
	outreq := new(bfe_http.Request)
	*outreq = *httpReq
	basicReq := &bfe_basic.Request{HttpRequest: httpReq, OutRequest: outreq}
	aiMeta := &bfe_basic.AiBasicInfo{
		AuthStyle: bfe_basic.AuthStyleOpenAI,
		Mode:      bfe_basic.ModeChat,
	}
	return basicReq, aiMeta
}

func bodyAfterInjection(t *testing.T, basicReq *bfe_basic.Request) string {
	t.Helper()
	accessor, err := basicReq.OutRequest.GetBodyAccessor()
	if err != nil || accessor == nil {
		t.Fatalf("GetBodyAccessor failed: %v", err)
	}
	body, _ := accessor.GetBytes()
	return string(body)
}

func TestMaybeInjectStreamUsage(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		authStyle   string
		mode        string
		body        string
		wantChanged bool
		wantInclude bool // expected include_usage value after the call
	}{
		{
			name:        "injects for openai streaming chat without stream_options",
			enabled:     true,
			authStyle:   bfe_basic.AuthStyleOpenAI,
			mode:        bfe_basic.ModeChat,
			body:        `{"model":"m1","stream":true,"messages":[]}`,
			wantChanged: true,
			wantInclude: true,
		},
		{
			name:        "injects for openai completions streaming",
			enabled:     true,
			authStyle:   bfe_basic.AuthStyleOpenAI,
			mode:        bfe_basic.ModeCompletion,
			body:        `{"model":"m1","stream":true,"prompt":"hi"}`,
			wantChanged: true,
			wantInclude: true,
		},
		{
			name:        "does not override explicit false",
			enabled:     true,
			authStyle:   bfe_basic.AuthStyleOpenAI,
			mode:        bfe_basic.ModeChat,
			body:        `{"model":"m1","stream":true,"stream_options":{"include_usage":false}}`,
			wantChanged: false,
			wantInclude: false,
		},
		{
			name:        "does not touch explicit true",
			enabled:     true,
			authStyle:   bfe_basic.AuthStyleOpenAI,
			mode:        bfe_basic.ModeChat,
			body:        `{"model":"m1","stream":true,"stream_options":{"include_usage":true}}`,
			wantChanged: false,
			wantInclude: true,
		},
		{
			name:        "skips non-streaming request",
			enabled:     true,
			authStyle:   bfe_basic.AuthStyleOpenAI,
			mode:        bfe_basic.ModeChat,
			body:        `{"model":"m1","stream":false}`,
			wantChanged: false,
		},
		{
			name:        "skips anthropic protocol",
			enabled:     true,
			authStyle:   bfe_basic.AuthStyleAnthropic,
			mode:        bfe_basic.ModeChat,
			body:        `{"model":"m1","stream":true}`,
			wantChanged: false,
		},
		{
			name:        "skips responses api",
			enabled:     true,
			authStyle:   bfe_basic.AuthStyleOpenAI,
			mode:        bfe_basic.ModeResponses,
			body:        `{"model":"m1","stream":true}`,
			wantChanged: false,
		},
		{
			name:        "skips when disabled",
			enabled:     false,
			authStyle:   bfe_basic.AuthStyleOpenAI,
			mode:        bfe_basic.ModeChat,
			body:        `{"model":"m1","stream":true}`,
			wantChanged: false,
		},
		{
			name:        "non-json body passes through",
			enabled:     true,
			authStyle:   bfe_basic.AuthStyleOpenAI,
			mode:        bfe_basic.ModeChat,
			body:        `not-a-json-body`,
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			basicReq, aiMeta := newStreamUsageTestRequest(tt.body)
			aiMeta.AuthStyle = tt.authStyle
			aiMeta.Mode = tt.mode

			if changed := maybeInjectStreamUsage(basicReq, aiMeta, tt.enabled); changed != tt.wantChanged {
				t.Errorf("maybeInjectStreamUsage changed = %v, want %v", changed, tt.wantChanged)
			}

			got := gjson.GetBytes([]byte(bodyAfterInjection(t, basicReq)), "stream_options.include_usage")
			if tt.wantInclude {
				if !got.Exists() || !got.Bool() {
					t.Errorf("expected stream_options.include_usage=true, body: %s", bodyAfterInjection(t, basicReq))
				}
			} else if got.Exists() && got.Bool() {
				t.Errorf("include_usage must not be injected, body: %s", bodyAfterInjection(t, basicReq))
			}
		})
	}
}

func TestMaybeInjectStreamUsageIdempotent(t *testing.T) {
	basicReq, aiMeta := newStreamUsageTestRequest(`{"model":"m1","stream":true}`)
	if !maybeInjectStreamUsage(basicReq, aiMeta, true) {
		t.Fatal("first injection must report changed")
	}
	if maybeInjectStreamUsage(basicReq, aiMeta, true) {
		t.Error("second injection must be a no-op (include_usage already present)")
	}
}
