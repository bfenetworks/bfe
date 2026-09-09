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

package mod_body_process

import (
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func TestNewQuotaUsageProcessorNonOK(t *testing.T) {
	req := newTestRequest("AI_product")
	res := &bfe_http.Response{StatusCode: bfe_http.StatusBadRequest}
	if p := NewQuotaUsageProcessor(req, res); p != nil {
		t.Error("expected nil processor for non-OK response")
	}
}

func TestNewQuotaUsageProcessorOK(t *testing.T) {
	req := newTestRequest("AI_product")
	req.InitAiBasicInfo()
	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	p := NewQuotaUsageProcessor(req, res)
	if p == nil {
		t.Fatal("expected processor for OK response")
	}
}

func TestQuotaUsageProcessorProcessWithUsage(t *testing.T) {
	req := newTestRequest("AI_product")
	ai := req.InitAiBasicInfo()
	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	p := NewQuotaUsageProcessor(req, res)

	events := []Event{newRawEvent(`{"usage":{"total_tokens":10,"prompt_tokens":3,"completion_tokens":7}}`)}
	out, err := p.Process(events)
	if err != nil {
		t.Fatalf("Process failed: %s", err)
	}
	if len(out) != 1 {
		t.Errorf("expected 1 event, got %d", len(out))
	}
	usage := ai.GetTokenUsage()
	if usage.UsedQuota != 10 {
		t.Errorf("expected UsedQuota 10, got %d", usage.UsedQuota)
	}
}

// Issue #1364: a non-streaming Anthropic body (top-level type "message")
// must mark both the final usage and the response completion, otherwise
// the request-finish guard treats a fully parsed usage as unconfirmed.
func TestQuotaUsageProcessorProcessAnthropicNonStreamMarks(t *testing.T) {
	req := newTestRequest("AI_product")
	ai := req.InitAiBasicInfo()
	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	p := NewQuotaUsageProcessor(req, res)

	body := `{"id":"msg_01","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":574145,"output_tokens":109329,"cache_read_input_tokens":7395200}}`
	events := []Event{newRawEvent(body)}
	if _, err := p.Process(events); err != nil {
		t.Fatalf("Process failed: %s", err)
	}

	if !ai.IsFinalUsageSeen() {
		t.Error("expected final usage seen for non-stream Anthropic body")
	}
	if !ai.IsResponseCompleted() {
		t.Error("expected response completed for non-stream Anthropic body")
	}
	usage := ai.GetTokenUsage()
	if usage.PromptTokens != 7969345 {
		t.Errorf("expected PromptTokens 7969345 (574145+7395200), got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 109329 {
		t.Errorf("expected CompletionTokens 109329, got %d", usage.CompletionTokens)
	}
	if usage.CacheReadTokens != 7395200 {
		t.Errorf("expected CacheReadTokens 7395200, got %d", usage.CacheReadTokens)
	}
	if usage.UsedQuota != 8078674 {
		t.Errorf("expected UsedQuota 8078674 (7969345+109329), got %d", usage.UsedQuota)
	}
}

func TestQuotaUsageProcessorProcessEstimate(t *testing.T) {
	req := newTestRequest("AI_product")
	ai := req.InitAiBasicInfo()
	ai.SetAllowEstimateToken(true)
	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	p := NewQuotaUsageProcessor(req, res)

	events := []Event{newRawEvent(`{"text":"hello world"}`)}
	_, err := p.Process(events)
	if err != nil {
		t.Fatalf("Process failed: %s", err)
	}
	usage := ai.GetTokenUsage()
	if usage.CompletionTokens <= 0 {
		t.Errorf("expected positive completion tokens, got %d", usage.CompletionTokens)
	}
}

func TestQuotaUsageProcessorProcessWithCache(t *testing.T) {
	req := newTestRequest("AI_product")
	ai := req.InitAiBasicInfo()
	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	p := NewQuotaUsageProcessor(req, res)

	events := []Event{newRawEvent(`{"usage":{"total_tokens":9500,"prompt_tokens":8000,"completion_tokens":1500,"cache_read_tokens":5000,"cache_write_tokens":1000}}`)}
	out, err := p.Process(events)
	if err != nil {
		t.Fatalf("Process failed: %s", err)
	}
	if len(out) != 1 {
		t.Errorf("expected 1 event, got %d", len(out))
	}
	usage := ai.GetTokenUsage()
	if usage.UsedQuota != 9500 {
		t.Errorf("expected UsedQuota 9500, got %d", usage.UsedQuota)
	}
	if usage.PromptTokens != 8000 || usage.CompletionTokens != 1500 {
		t.Errorf("unexpected prompt/completion: %+v", usage)
	}
	if usage.CacheReadTokens != 5000 {
		t.Errorf("expected CacheReadTokens 5000, got %d", usage.CacheReadTokens)
	}
	if usage.CacheWriteTokens != 1000 {
		t.Errorf("expected CacheWriteTokens 1000, got %d", usage.CacheWriteTokens)
	}
}

func TestQuotaUsageProcessorProcessWithDeepSeekCache(t *testing.T) {
	req := newTestRequest("AI_product")
	ai := req.InitAiBasicInfo()
	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	p := NewQuotaUsageProcessor(req, res)

	events := []Event{newRawEvent(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"prompt_cache_hit_tokens":5}}`)}
	out, err := p.Process(events)
	if err != nil {
		t.Fatalf("Process failed: %s", err)
	}
	if len(out) != 1 {
		t.Errorf("expected 1 event, got %d", len(out))
	}
	usage := ai.GetTokenUsage()
	if usage.CacheReadTokens != 5 {
		t.Errorf("expected CacheReadTokens 5, got %d", usage.CacheReadTokens)
	}
}

func TestQuotaUsageProcessorProcessWithDeepSeekCacheDetails(t *testing.T) {
	req := newTestRequest("AI_product")
	ai := req.InitAiBasicInfo()
	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	p := NewQuotaUsageProcessor(req, res)

	events := []Event{newRawEvent(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":6}}}`)}
	out, err := p.Process(events)
	if err != nil {
		t.Fatalf("Process failed: %s", err)
	}
	if len(out) != 1 {
		t.Errorf("expected 1 event, got %d", len(out))
	}
	usage := ai.GetTokenUsage()
	if usage.CacheReadTokens != 6 {
		t.Errorf("expected CacheReadTokens 6, got %d", usage.CacheReadTokens)
	}
}

func TestQuotaUsageProcessorProcessWithAnthropicCache(t *testing.T) {
	req := newTestRequest("AI_product")
	ai := req.InitAiBasicInfo()
	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	p := NewQuotaUsageProcessor(req, res)

	// Anthropic: input_tokens excludes cache read/write; PromptTokens is
	// normalized to the total input (320 + 8000 + 200 = 8520).
	events := []Event{newRawEvent(`{"usage":{"input_tokens":320,"output_tokens":150,"cache_read_input_tokens":8000,"cache_creation_input_tokens":200}}`)}
	_, err := p.Process(events)
	if err != nil {
		t.Fatalf("Process failed: %s", err)
	}
	usage := ai.GetTokenUsage()
	if usage.PromptTokens != 8520 {
		t.Errorf("expected PromptTokens 8520, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 150 {
		t.Errorf("expected CompletionTokens 150, got %d", usage.CompletionTokens)
	}
	if usage.CacheReadTokens != 8000 || usage.CacheWriteTokens != 200 {
		t.Errorf("unexpected cache tokens: %+v", usage)
	}
	if usage.UsedQuota != 8670 {
		t.Errorf("expected UsedQuota 8670 (8520+150), got %d", usage.UsedQuota)
	}
}

func TestQuotaUsageProcessorProcessWithAudio(t *testing.T) {
	req := newTestRequest("AI_product")
	ai := req.InitAiBasicInfo()
	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	p := NewQuotaUsageProcessor(req, res)

	events := []Event{newRawEvent(`{"usage":{"total_tokens":4500,"prompt_tokens":4000,"completion_tokens":500,"audio_input_tokens":1000,"audio_output_tokens":200}}`)}
	out, err := p.Process(events)
	if err != nil {
		t.Fatalf("Process failed: %s", err)
	}
	if len(out) != 1 {
		t.Errorf("expected 1 event, got %d", len(out))
	}
	usage := ai.GetTokenUsage()
	if usage.UsedQuota != 4500 {
		t.Errorf("expected UsedQuota 4500, got %d", usage.UsedQuota)
	}
	if usage.PromptTokens != 4000 || usage.CompletionTokens != 500 {
		t.Errorf("unexpected prompt/completion: %+v", usage)
	}
	if usage.AudioInputTokens != 1000 {
		t.Errorf("expected AudioInputTokens 1000, got %d", usage.AudioInputTokens)
	}
	if usage.AudioOutputTokens != 200 {
		t.Errorf("expected AudioOutputTokens 200, got %d", usage.AudioOutputTokens)
	}
}

// Gemini stream: streamGenerateContent carries no SSE termination event
// and every chunk carries the accumulated usageMetadata. Billing must
// take the values of the LAST usage-bearing chunk (taking an intermediate
// chunk would under-count), and no event may mark the response completed
// (HTTP stream EOF is the completion fallback).
func TestQuotaUsageProcessorProcessGeminiStream(t *testing.T) {
	req := newTestRequest("AI_product")
	ai := req.InitAiBasicInfo()
	ai.AuthStyle = bfe_basic.AuthStyleGemini
	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	p := NewQuotaUsageProcessor(req, res)

	sseEvent := func(data string) Event {
		return &SSEEvent{DataLines: [][]byte{[]byte(data)}}
	}
	events := []Event{
		sseEvent(`{"candidates":[{"content":{"parts":[{"text":"he"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1,"totalTokenCount":11}}`),
		sseEvent(`{"candidates":[{"content":{"parts":[{"text":"llo"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13}}`),
		sseEvent(`{"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"cachedContentTokenCount":4,"totalTokenCount":15}}`),
	}
	if _, err := p.Process(events); err != nil {
		t.Fatalf("Process failed: %s", err)
	}

	usage := ai.GetTokenUsage()
	if usage.PromptTokens != 10 || usage.CompletionTokens != 5 {
		t.Errorf("expected final chunk usage prompt=10 completion=5, got %d/%d",
			usage.PromptTokens, usage.CompletionTokens)
	}
	if usage.CacheReadTokens != 4 {
		t.Errorf("expected CacheReadTokens 4 from the final chunk, got %d", usage.CacheReadTokens)
	}
	if usage.UsedQuota != 15 {
		t.Errorf("expected UsedQuota 15 (last chunk accumulated total), got %d", usage.UsedQuota)
	}
	if !ai.IsFinalUsageSeen() {
		t.Error("expected final usage seen for gemini stream")
	}
	if ai.IsResponseCompleted() {
		t.Error("gemini stream has no termination event; completion must rely on EOF")
	}
}

// Gemini non-streaming generateContent body: the whole-body JSON carries
// usageMetadata and is both the final usage and the response completion.
func TestQuotaUsageProcessorProcessGeminiNonStream(t *testing.T) {
	req := newTestRequest("AI_product")
	ai := req.InitAiBasicInfo()
	ai.AuthStyle = bfe_basic.AuthStyleGemini
	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	p := NewQuotaUsageProcessor(req, res)

	events := []Event{
		newRawEvent(`{"candidates":[{"content":{"parts":[{"text":"hi"}]}}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":8,"cachedContentTokenCount":4,"totalTokenCount":20}}`),
	}
	if _, err := p.Process(events); err != nil {
		t.Fatalf("Process failed: %s", err)
	}

	usage := ai.GetTokenUsage()
	if usage.UsedQuota != 20 || usage.PromptTokens != 12 || usage.CompletionTokens != 8 {
		t.Errorf("unexpected usage: %+v", usage)
	}
	if usage.CacheReadTokens != 4 {
		t.Errorf("expected CacheReadTokens 4, got %d", usage.CacheReadTokens)
	}
	if !ai.IsFinalUsageSeen() || !ai.IsResponseCompleted() {
		t.Errorf("expected final usage and completion for non-stream gemini body, got final=%v completed=%v",
			ai.IsFinalUsageSeen(), ai.IsResponseCompleted())
	}
}
