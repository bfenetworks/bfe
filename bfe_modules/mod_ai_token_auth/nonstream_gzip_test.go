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

package mod_ai_token_auth

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func gzipFixture(t *testing.T, body string) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(body)); err != nil {
		t.Fatalf("gzip write: %s", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %s", err)
	}
	return buf.Bytes()
}

// TestTokenReadResponseHandler_ChunkedGzipAnthropic reproduces issue #1406: a
// non-streaming Anthropic response framed as gzip + chunked (ContentLength ==
// -1, Content-Encoding: gzip). Before the fix the handler skipped the whole
// body, so neither the final usage nor the response completion was marked.
func TestTokenReadResponseHandler_ChunkedGzipAnthropic(t *testing.T) {
	m := NewModuleAITokenAuth()
	req := newTestRequest("ak-123", "AI_product")
	ai := req.InitAiBasicInfo()
	ai.AuthStyle = bfe_basic.AuthStyleAnthropic
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id"}, 2, nil)

	const raw = `{"id":"msg_01","type":"message","usage":{"input_tokens":425,"output_tokens":52,"cache_read_input_tokens":36096}}`
	compressed := gzipFixture(t, raw)
	res := &bfe_http.Response{
		StatusCode:    200,
		ContentLength: -1, // chunked upstream
		IsSse:         false,
		Header: bfe_http.Header{
			"Content-Type":     {"application/json; charset=utf-8"},
			"Content-Encoding": {"gzip"},
		},
		Body: ioutil.NopCloser(bytes.NewReader(compressed)),
	}

	if ret := m.tokenReadResponseHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	usage := ai.GetTokenUsage()
	if usage.CompletionTokens != 52 {
		t.Errorf("CompletionTokens = %d, want 52", usage.CompletionTokens)
	}
	if usage.CacheReadTokens != 36096 {
		t.Errorf("CacheReadTokens = %d, want 36096", usage.CacheReadTokens)
	}
	if usage.PromptTokens != 36521 {
		t.Errorf("PromptTokens = %d, want 36521 (425+36096)", usage.PromptTokens)
	}
	if usage.UsedQuota != 36573 {
		t.Errorf("UsedQuota = %d, want 36573 (36521+52)", usage.UsedQuota)
	}
	if !ai.IsFinalUsageSeen() {
		t.Error("expected final usage to be marked")
	}
	if !ai.IsResponseCompleted() {
		t.Error("expected response completion to be marked")
	}

	// The decode must be parse-only: the forwarded header and body stay exactly
	// as the upstream sent them.
	if got := res.Header.Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
	forwarded, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read forwarded body: %s", err)
	}
	if !bytes.Equal(forwarded, compressed) {
		t.Errorf("forwarded body changed: got %d bytes, want %d", len(forwarded), len(compressed))
	}
}

// TestTokenReadResponseHandler_ChunkedPlainAnthropic covers the chunked but
// uncompressed variant: broadening the gate alone must be enough.
func TestTokenReadResponseHandler_ChunkedPlainAnthropic(t *testing.T) {
	m := NewModuleAITokenAuth()
	req := newTestRequest("ak-123", "AI_product")
	ai := req.InitAiBasicInfo()
	ai.AuthStyle = bfe_basic.AuthStyleAnthropic
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id"}, 2, nil)

	const raw = `{"type":"message","usage":{"input_tokens":425,"output_tokens":52,"cache_read_input_tokens":36096}}`
	res := &bfe_http.Response{
		StatusCode:    200,
		ContentLength: -1,
		Header:        bfe_http.Header{"Content-Type": {"application/json"}},
		Body:          ioutil.NopCloser(strings.NewReader(raw)),
	}

	m.tokenReadResponseHandler(req, res)

	usage := ai.GetTokenUsage()
	if usage.CompletionTokens != 52 || usage.UsedQuota != 36573 {
		t.Errorf("usage = %+v, want CompletionTokens 52 / UsedQuota 36573", usage)
	}
	if !ai.IsFinalUsageSeen() || !ai.IsResponseCompleted() {
		t.Error("expected final usage and response completion to be marked")
	}
}

// TestTokenReadResponseHandler_SSEChunkedSkipped guards the streaming path: an
// SSE response must never be read whole or parsed here.
func TestTokenReadResponseHandler_SSEChunkedSkipped(t *testing.T) {
	m := NewModuleAITokenAuth()
	req := newTestRequest("ak-123", "AI_product")
	ai := req.InitAiBasicInfo()
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id"}, 2, nil)

	res := &bfe_http.Response{
		StatusCode:    200,
		ContentLength: -1,
		IsSse:         true,
		Header:        bfe_http.Header{"Content-Type": {"text/event-stream"}},
		Body:          ioutil.NopCloser(strings.NewReader("data: {\"type\":\"message_delta\"}\n\n")),
	}

	m.tokenReadResponseHandler(req, res)

	if ai.IsResponseCompleted() {
		t.Error("an SSE stream must not be marked completed by the read-response handler")
	}
	if got := ai.GetTokenUsage().UsedQuota; got != 0 {
		t.Errorf("an SSE stream must not be parsed here, got UsedQuota %d", got)
	}
}

// TestTokenReadResponseHandler_ChunkedEstimateUsesBodyLen verifies the estimate
// branch no longer divides the missing Content-Length (-1) by 4.
func TestTokenReadResponseHandler_ChunkedEstimateUsesBodyLen(t *testing.T) {
	m := NewModuleAITokenAuth()
	req := newTestRequest("ak-123", "AI_product")
	ai := req.InitAiBasicInfo()
	ai.SetAllowEstimateToken(true)
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id"}, 2, nil)

	body := `{"id":"x","type":"message"}`
	res := &bfe_http.Response{
		StatusCode:    200,
		ContentLength: -1,
		Header:        bfe_http.Header{"Content-Type": {"application/json"}},
		Body:          ioutil.NopCloser(strings.NewReader(body)),
	}

	m.tokenReadResponseHandler(req, res)

	want := int64(len(body)) / 4
	if got := ai.GetTokenUsage().CompletionTokens; got != want {
		t.Errorf("CompletionTokens = %d, want %d (estimate from body length)", got, want)
	}
}
