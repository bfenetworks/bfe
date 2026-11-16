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

	"github.com/bfenetworks/go-lib/web-monitor/metrics"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_http"
)

func newReasoningTestRequest(body string, headers map[string]string) (*bfe_basic.Request, *bfe_http.Request, *bfe_basic.AiBasicInfo) {
	httpReq := &bfe_http.Request{
		Method:        "POST",
		URL:           &url.URL{Path: "/v1/chat/completions"},
		Header:        bfe_http.Header{},
		Body:          ioutil.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	outreq := new(bfe_http.Request)
	*outreq = *httpReq

	basicReq := &bfe_basic.Request{HttpRequest: httpReq, OutRequest: outreq}
	aiMeta := &bfe_basic.AiBasicInfo{
		AuthStyle: bfe_basic.AuthStyleOpenAI,
		Mode:      bfe_basic.ModeChat,
	}
	return basicReq, outreq, aiMeta
}

func newReasoningTestState() *ProxyState {
	return &ProxyState{
		ReasoningPatched:  new(metrics.Counter),
		ReasoningSkip:     new(metrics.Counter),
		ReasoningFail:     new(metrics.Counter),
		ReasoningStill400: new(metrics.Counter),
	}
}

func outRequestBody(t *testing.T, outreq *bfe_http.Request) string {
	t.Helper()
	accessor, err := outreq.GetBodyAccessor()
	if err != nil || accessor == nil {
		t.Fatalf("GetBodyAccessor failed: %v", err)
	}
	body, _ := accessor.GetBytes()
	return string(body)
}

// TestPatchReasoningContent verifies the pure body transformation: only the
// assistant messages after the last user message are touched, existing values
// are kept, and everything else stays byte-for-byte identical.
func TestPatchReasoningContent(t *testing.T) {
	const injected = `,"reasoning_content":""`

	tests := []struct {
		name      string
		body      string
		protocol  string
		wantPatch int
		wantField []string // paths expected to be "" after patching
	}{
		{
			name: "history untouched, window patched",
			body: `{"model":"m","messages":[` +
				`{"role":"user","content":"q1"},` +
				`{"role":"assistant","content":"a1"},` +
				`{"role":"user","content":"q2"},` +
				`{"role":"assistant","content":"a2"},` +
				`{"role":"tool","content":"r","tool_call_id":"t1"}]}`,
			protocol:  bfe_basic.AuthStyleOpenAI,
			wantPatch: 1,
			wantField: []string{"messages.3.reasoning_content"},
		},
		{
			name: "existing non-empty value kept",
			body: `{"messages":[` +
				`{"role":"user","content":"q"},` +
				`{"role":"assistant","reasoning_content":"think","content":"a"}]}`,
			protocol:  bfe_basic.AuthStyleOpenAI,
			wantPatch: 0,
		},
		{
			name: "tool_calls assistant with null content",
			body: `{"messages":[` +
				`{"role":"user","content":"q"},` +
				`{"role":"assistant","content":null,"tool_calls":[{"id":"t1","type":"function","function":{"name":"f","arguments":"{}"}}]}]}`,
			protocol:  bfe_basic.AuthStyleOpenAI,
			wantPatch: 1,
			wantField: []string{"messages.1.reasoning_content"},
		},
		{
			name: "two assistant turns in window both patched",
			body: `{"messages":[` +
				`{"role":"user","content":"q"},` +
				`{"role":"assistant","content":"a1"},` +
				`{"role":"assistant","content":"a2"}]}`,
			protocol:  bfe_basic.AuthStyleOpenAI,
			wantPatch: 2,
			wantField: []string{"messages.1.reasoning_content", "messages.2.reasoning_content"},
		},
		{
			name:      "no messages",
			body:      `{"model":"m"}`,
			protocol:  bfe_basic.AuthStyleOpenAI,
			wantPatch: 0,
		},
		{
			name:      "no user message",
			body:      `{"messages":[{"role":"assistant","content":"a"}]}`,
			protocol:  bfe_basic.AuthStyleOpenAI,
			wantPatch: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, patched, err := patchReasoningContent([]byte(tt.body), tt.protocol)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if patched != tt.wantPatch {
				t.Fatalf("patched = %d, want %d, body: %s", patched, tt.wantPatch, string(got))
			}
			if patched == 0 {
				if string(got) != tt.body {
					t.Fatalf("body changed without patching:\n got: %s\nwant: %s", string(got), tt.body)
				}
				return
			}
			for _, path := range tt.wantField {
				if v := gjson.GetBytes(got, path); v.String() != "" || !v.Exists() {
					t.Fatalf("%s = %q (exists=%v), want empty string", path, v.String(), v.Exists())
				}
			}
			// byte-level invariant: removing the injected fields must restore
			// the original body exactly
			if stripped := strings.ReplaceAll(string(got), injected, ""); stripped != tt.body {
				t.Fatalf("bytes other than the injected field changed:\n got: %s\nwant: %s", stripped, tt.body)
			}
		})
	}
}

// TestPatchReasoningContentNullValue verifies that an explicit null is
// normalized to an empty string in place (the only case where an existing
// value changes, so the "restore by removing the field" invariant does not
// apply).
func TestPatchReasoningContentNullValue(t *testing.T) {
	const body = `{"messages":[{"role":"user","content":"q"},` +
		`{"role":"assistant","reasoning_content":null,"content":"a"}]}`

	got, patched, err := patchReasoningContent([]byte(body), bfe_basic.AuthStyleOpenAI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patched != 1 {
		t.Fatalf("patched = %d, want 1", patched)
	}
	if v := gjson.GetBytes(got, "messages.1.reasoning_content"); v.String() != "" || v.Type == gjson.Null {
		t.Fatalf("reasoning_content = %q (type %v), want empty string", v.String(), v.Type)
	}
	// the null value is the only difference
	want := strings.Replace(body, `"reasoning_content":null`, `"reasoning_content":""`, 1)
	if string(got) != want {
		t.Fatalf("unexpected body:\n got: %s\nwant: %s", string(got), want)
	}
}

// TestPatchReasoningContentAnthropic verifies the anthropic branch: an empty
// thinking block is prepended once, existing blocks are kept and a non-array
// content is left alone.
func TestPatchReasoningContentAnthropic(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantPatch int
	}{
		{
			name: "prepend thinking block",
			body: `{"messages":[{"role":"user","content":"q"},` +
				`{"role":"assistant","content":[{"type":"text","text":"a"}]}]}`,
			wantPatch: 1,
		},
		{
			name: "existing thinking block kept",
			body: `{"messages":[{"role":"user","content":"q"},` +
				`{"role":"assistant","content":[{"type":"thinking","thinking":"real","signature":"sig"},{"type":"text","text":"a"}]}]}`,
			wantPatch: 0,
		},
		{
			name: "content is a string",
			body: `{"messages":[{"role":"user","content":"q"},` +
				`{"role":"assistant","content":"plain text"}]}`,
			wantPatch: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, patched, err := patchReasoningContent([]byte(tt.body), bfe_basic.AuthStyleAnthropic)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if patched != tt.wantPatch {
				t.Fatalf("patched = %d, want %d, body: %s", patched, tt.wantPatch, string(got))
			}
			if patched == 0 {
				if string(got) != tt.body {
					t.Fatalf("body changed without patching:\n got: %s\nwant: %s", string(got), tt.body)
				}
				return
			}
			if !strings.Contains(string(got), anthropicThinkingBlock) {
				t.Fatalf("thinking block not injected: %s", string(got))
			}
			// the original parts must survive byte-for-byte
			if !strings.Contains(string(got), `{"type":"text","text":"a"}`) {
				t.Fatalf("existing content part changed: %s", string(got))
			}
		})
	}
}

// TestEnsureReasoningContentGates verifies the gates around the pure
// transformation: switch, protocol, mode, content-type, encoding, size.
func TestEnsureReasoningContentGates(t *testing.T) {
	const body = `{"messages":[{"role":"user","content":"q"},{"role":"assistant","content":"a"}]}`

	openaiCfg := func() *cluster_conf.AIConf {
		return &cluster_conf.AIConf{EnsureReasoningContent: []string{"openai"}}
	}

	tests := []struct {
		name        string
		cfg         *cluster_conf.AIConf
		headers     map[string]string
		mode        string
		authStyle   string
		wantPatched bool
		wantSkip    int64
	}{
		{
			name:        "switch on",
			cfg:         openaiCfg(),
			wantPatched: true,
		},
		{
			name: "switch off",
			cfg:  &cluster_conf.AIConf{},
		},
		{
			name:      "protocol not listed",
			cfg:       openaiCfg(),
			authStyle: bfe_basic.AuthStyleAnthropic,
		},
		{
			name: "responses mode is out of scope",
			cfg:  openaiCfg(),
			mode: bfe_basic.ModeResponses,
		},
		{
			name:     "batch mode skipped",
			cfg:      openaiCfg(),
			mode:     bfe_basic.ModeBatch,
			wantSkip: 1,
		},
		{
			name:     "non-json content type skipped",
			cfg:      openaiCfg(),
			headers:  map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
			wantSkip: 1,
		},
		{
			name:     "compressed body skipped",
			cfg:      openaiCfg(),
			headers:  map[string]string{"Content-Encoding": "gzip"},
			wantSkip: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			basicReq, outreq, aiMeta := newReasoningTestRequest(body, tt.headers)
			if tt.mode != "" {
				aiMeta.Mode = tt.mode
			}
			if tt.authStyle != "" {
				aiMeta.AuthStyle = tt.authStyle
			}
			state := newReasoningTestState()
			p := &ReverseProxy{proxyState: state}

			got := p.ensureReasoningContent(basicReq, outreq, tt.cfg, aiMeta)
			if got != tt.wantPatched {
				t.Fatalf("patched = %v, want %v", got, tt.wantPatched)
			}
			if n := state.ReasoningSkip.Get(); n != tt.wantSkip {
				t.Fatalf("ReasoningSkip = %d, want %d", n, tt.wantSkip)
			}
			if tt.wantPatched {
				if n := state.ReasoningPatched.Get(); n != 1 {
					t.Fatalf("ReasoningPatched = %d, want 1", n)
				}
				if aiMeta.ReasoningPatched != true {
					t.Fatal("AiBasicInfo.ReasoningPatched not set")
				}
				if outreq.ContentLength != -1 {
					t.Fatalf("ContentLength = %d, want -1", outreq.ContentLength)
				}
				if outreq.Header.Get("Content-Length") != "" {
					t.Fatalf("Content-Length header not removed")
				}
				if b := outRequestBody(t, outreq); !strings.Contains(b, `"reasoning_content":""`) {
					t.Fatalf("field not injected: %s", b)
				}
				return
			}
			if b := outRequestBody(t, outreq); b != body {
				t.Fatalf("body changed on a skipped path:\n got: %s\nwant: %s", b, body)
			}
		})
	}
}

// TestEnsureReasoningContentOversizedBody verifies the AccessibleBodySize
// gate: a body that cannot be buffered in full must be left alone and counted
// as a skip.
func TestEnsureReasoningContentOversizedBody(t *testing.T) {
	orig := bfe_http.GetAccessibleBodySize()
	bfe_http.SetAccessibleBodySize(64)
	defer bfe_http.SetAccessibleBodySize(orig)

	body := `{"messages":[{"role":"user","content":"` + strings.Repeat("x", 128) + `"},` +
		`{"role":"assistant","content":"a"}]}`
	basicReq, outreq, aiMeta := newReasoningTestRequest(body, nil)
	state := newReasoningTestState()
	p := &ReverseProxy{proxyState: state}

	cfg := &cluster_conf.AIConf{EnsureReasoningContent: []string{"openai"}}
	if p.ensureReasoningContent(basicReq, outreq, cfg, aiMeta) {
		t.Fatal("oversized body must not be patched")
	}
	if n := state.ReasoningSkip.Get(); n != 1 {
		t.Fatalf("ReasoningSkip = %d, want 1", n)
	}
}

// TestEnsureReasoningContentIdempotent verifies that a second pass over an
// already patched body is a no-op (fallback re-entry).
func TestEnsureReasoningContentIdempotent(t *testing.T) {
	const body = `{"messages":[{"role":"user","content":"q"},{"role":"assistant","content":"a"}]}`

	basicReq, outreq, aiMeta := newReasoningTestRequest(body, nil)
	state := newReasoningTestState()
	p := &ReverseProxy{proxyState: state}
	cfg := &cluster_conf.AIConf{EnsureReasoningContent: []string{"openai"}}

	if !p.ensureReasoningContent(basicReq, outreq, cfg, aiMeta) {
		t.Fatal("first pass must patch")
	}
	first := outRequestBody(t, outreq)

	if p.ensureReasoningContent(basicReq, outreq, cfg, aiMeta) {
		t.Fatal("second pass must be a no-op")
	}
	if second := outRequestBody(t, outreq); second != first {
		t.Fatalf("body changed on the second pass:\n first: %s\nsecond: %s", first, second)
	}
	if n := state.ReasoningPatched.Get(); n != 1 {
		t.Fatalf("ReasoningPatched = %d, want 1", n)
	}
}

// BenchmarkPatchReasoningContentLargeBody measures the cost on a ~1MB chat
// body (the largest shape seen in production is ~360k tokens).
func BenchmarkPatchReasoningContentLargeBody(b *testing.B) {
	const chunk = 2500
	var sb strings.Builder
	sb.WriteString(`{"model":"m","messages":[`)
	for i := 0; i < 100; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"role":"user","content":"` + strings.Repeat("q", chunk) + `"},`)
		sb.WriteString(`{"role":"assistant","content":"` + strings.Repeat("a", chunk) + `"}`)
	}
	sb.WriteString(`]}`)
	body := []byte(sb.String())

	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := patchReasoningContent(body, bfe_basic.AuthStyleOpenAI); err != nil {
			b.Fatal(err)
		}
	}
}

// TestPatchReasoningContentMalformedBody verifies fail-open on unparsable
// messages: a truncated body is reported as a parse failure instead of being
// extended by sjson, and ensureReasoningContent passes it through unchanged
// while counting ReasoningFail.
func TestPatchReasoningContentMalformedBody(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"q"},{"role":"assistant"}`

	got, patched, err := patchReasoningContent([]byte(body), bfe_basic.AuthStyleOpenAI)
	if err == nil {
		t.Fatal("truncated body must be rejected")
	}
	if patched != 0 || string(got) != body {
		t.Fatalf("malformed body must be passed through unchanged, got %q patched=%d", string(got), patched)
	}

	basicReq, outreq, aiMeta := newReasoningTestRequest(body, nil)
	state := newReasoningTestState()
	p := &ReverseProxy{proxyState: state}
	cfg := &cluster_conf.AIConf{EnsureReasoningContent: []string{"openai"}}

	if p.ensureReasoningContent(basicReq, outreq, cfg, aiMeta) {
		t.Fatal("malformed body must not be patched")
	}
	if n := state.ReasoningFail.Get(); n != 1 {
		t.Fatalf("ReasoningFail = %d, want 1", n)
	}
	if b := outRequestBody(t, outreq); b != body {
		t.Fatalf("body changed on the fail-open path:\n got: %s\nwant: %s", b, body)
	}
}
