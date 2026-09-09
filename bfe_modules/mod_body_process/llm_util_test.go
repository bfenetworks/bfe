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
	"bytes"
	"net/http"
	"testing"
	"time"
)

func TestGetSetHTTPClient(t *testing.T) {
	original := GetHTTPClient()
	if original == nil {
		t.Fatal("expected default HTTP client")
	}

	newClient := &http.Client{Timeout: 5 * time.Second}
	SetHTTPClient(newClient)
	if GetHTTPClient() != newClient {
		t.Error("expected SetHTTPClient to update client")
	}

	SetHTTPClient(nil)
	if GetHTTPClient() == nil {
		t.Error("expected fallback client after SetHTTPClient(nil)")
	}

	SetHTTPClient(original)
}

func TestEstimateContentToken(t *testing.T) {
	if got := EstimateContentToken("hello world"); got <= 0 {
		t.Errorf("expected positive token estimate, got %d", got)
	}
}

func TestRemarshal(t *testing.T) {
	src := map[string]int{"a": 1}
	var dst map[string]int
	if err := remarshal(src, &dst); err != nil {
		t.Fatalf("remarshal failed: %s", err)
	}
	if dst["a"] != 1 {
		t.Errorf("expected dst[a]=1, got %d", dst["a"])
	}
}

func TestSSEEventToBytes(t *testing.T) {
	id := "1"
	ev := &SSEEvent{ID: &id, endstyle: "\n"}
	data := ev.ToBytes()
	if string(data) != "id: 1\n\n" {
		t.Errorf("unexpected SSE bytes: %s", string(data))
	}
}

func TestSSEEventToBytesCached(t *testing.T) {
	ev := &SSEEvent{raw: []byte("cached"), dirty: false}
	if string(ev.ToBytes()) != "cached" {
		t.Error("expected cached raw bytes")
	}
}

func TestSSEEventSetAndGetData(t *testing.T) {
	ev := &SSEEvent{}
	ev.SetData([]byte("hello"))
	if string(ev.GetData()) != "hello" {
		t.Errorf("expected data hello, got %s", string(ev.GetData()))
	}

	ev.AppendDataLine([]byte("world"))
	if string(ev.GetData()) != "hello\nworld" {
		t.Errorf("expected joined data, got %s", string(ev.GetData()))
	}
}

func TestSSEEventGetQuotaUsage(t *testing.T) {
	ev := &SSEEvent{DataLines: [][]byte{[]byte(`{"usage":{"total_tokens":10}}`)}}
	q := ev.GetQuotaUsage()
	if q.UsedQuota != 10 {
		t.Errorf("expected UsedQuota 10, got %d", q.UsedQuota)
	}
	if q.IsGuess {
		t.Error("expected IsGuess false")
	}
}

func TestSSEEventGetQuotaUsageWithAudio(t *testing.T) {
	ev := &SSEEvent{DataLines: [][]byte{[]byte(`{"usage":{"total_tokens":4500,"prompt_tokens":4000,"completion_tokens":500,"audio_input_tokens":1000,"audio_output_tokens":200}}`)}}
	q := ev.GetQuotaUsage()
	if q.UsedQuota != 4500 {
		t.Errorf("expected UsedQuota 4500, got %d", q.UsedQuota)
	}
	if q.PromptTokens != 4000 {
		t.Errorf("expected PromptTokens 4000, got %d", q.PromptTokens)
	}
	if q.CompletionTokens != 500 {
		t.Errorf("expected CompletionTokens 500, got %d", q.CompletionTokens)
	}
	if q.AudioInputTokens != 1000 {
		t.Errorf("expected AudioInputTokens 1000, got %d", q.AudioInputTokens)
	}
	if q.AudioOutputTokens != 200 {
		t.Errorf("expected AudioOutputTokens 200, got %d", q.AudioOutputTokens)
	}
	if q.IsGuess {
		t.Error("expected IsGuess false")
	}
}

func TestSSEEventGetQuotaUsage_DeepSeekCache(t *testing.T) {
	// DeepSeek: prompt_cache_hit_tokens
	ev := &SSEEvent{DataLines: [][]byte{[]byte(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"prompt_cache_hit_tokens":5}}`)}}
	usage := ev.GetQuotaUsage()
	if usage.CacheReadTokens != 5 {
		t.Errorf("expected CacheReadTokens 5 for prompt_cache_hit_tokens, got %d", usage.CacheReadTokens)
	}

	// DeepSeek: prompt_tokens_details.cached_tokens
	ev2 := &SSEEvent{DataLines: [][]byte{[]byte(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":6}}}`)}}
	usage2 := ev2.GetQuotaUsage()
	if usage2.CacheReadTokens != 6 {
		t.Errorf("expected CacheReadTokens 6 for prompt_tokens_details.cached_tokens, got %d", usage2.CacheReadTokens)
	}

	// Existing cache_read_tokens takes precedence when non-zero
	ev3 := &SSEEvent{DataLines: [][]byte{[]byte(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"cache_read_tokens":3,"prompt_cache_hit_tokens":5}}`)}}
	usage3 := ev3.GetQuotaUsage()
	if usage3.CacheReadTokens != 3 {
		t.Errorf("expected CacheReadTokens 3 (existing field precedence), got %d", usage3.CacheReadTokens)
	}
}

func TestSSEEventGetQuotaUsage_AnthropicCache(t *testing.T) {
	// Anthropic message_start: input_tokens excludes cache read/write tokens;
	// PromptTokens must be normalized to the total input.
	ev := &SSEEvent{DataLines: [][]byte{[]byte(`{"type":"message_start","usage":{"input_tokens":320,"output_tokens":0,"cache_read_input_tokens":8000,"cache_creation_input_tokens":200}}`)}}
	q := ev.GetQuotaUsage()
	if q.PromptTokens != 8520 {
		t.Errorf("expected PromptTokens 8520 (320+8000+200), got %d", q.PromptTokens)
	}
	if q.CompletionTokens != 0 {
		t.Errorf("expected CompletionTokens 0, got %d", q.CompletionTokens)
	}
	if q.CacheReadTokens != 8000 {
		t.Errorf("expected CacheReadTokens 8000, got %d", q.CacheReadTokens)
	}
	if q.CacheWriteTokens != 200 {
		t.Errorf("expected CacheWriteTokens 200, got %d", q.CacheWriteTokens)
	}
	if q.UsedQuota != 8520 {
		t.Errorf("expected UsedQuota 8520, got %d", q.UsedQuota)
	}
	if q.IsGuess {
		t.Error("expected IsGuess false for full-cache-hit message_start")
	}

	// Full cache hit: input_tokens = 0, usage must still be recognized.
	ev2 := &SSEEvent{DataLines: [][]byte{[]byte(`{"type":"message_start","usage":{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":5000}}`)}}
	q2 := ev2.GetQuotaUsage()
	if q2.PromptTokens != 5000 || q2.CacheReadTokens != 5000 {
		t.Errorf("expected PromptTokens/CacheReadTokens 5000, got %d/%d", q2.PromptTokens, q2.CacheReadTokens)
	}
	if q2.IsGuess {
		t.Error("expected IsGuess false for 100% cache hit")
	}
}

func TestSSEEventSetJsonField(t *testing.T) {
	ev := &SSEEvent{DataLines: [][]byte{[]byte(`{"text":"hello"}`)}}
	if err := ev.SetJsonField("text", "world"); err != nil {
		t.Fatalf("SetJsonField failed: %s", err)
	}
	if string(ev.GetData()) != `{"text":"world"}` {
		t.Errorf("unexpected data: %s", string(ev.GetData()))
	}
}

func TestSSEEventSetIDNoChange(t *testing.T) {
	id := "1"
	ev := &SSEEvent{ID: &id}
	ev.SetID(&id)
	if ev.dirty {
		t.Error("setting same ID pointer should not mark dirty")
	}
}

func TestSSEEventDecoder(t *testing.T) {
	dec, err := NewSSEEventDecoder(bytes.NewBufferString("data: hello\n\n"))
	if err != nil {
		t.Fatalf("NewSSEEventDecoder failed: %s", err)
	}
	events, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %s", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0].(*SSEEvent)
	if string(ev.GetData()) != "hello" {
		t.Errorf("expected data hello, got %s", string(ev.GetData()))
	}
}

func TestSSEEventDecoderTruncated(t *testing.T) {
	dec, err := NewSSEEventDecoder(bytes.NewBufferString("data: hello"))
	if err != nil {
		t.Fatalf("NewSSEEventDecoder failed: %s", err)
	}
	events, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %s", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 truncated event, got %d", len(events))
	}
	ev := events[0].(*SSEEvent)
	if !ev.truncated {
		t.Error("expected truncated event")
	}
}

func TestSSEEventDecoderEOF(t *testing.T) {
	dec, err := NewSSEEventDecoder(bytes.NewBufferString(""))
	if err != nil {
		t.Fatalf("NewSSEEventDecoder failed: %s", err)
	}
	events, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %s", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestRawEventGetQuotaUsage_CompletionFlags(t *testing.T) {
	tests := []struct {
		name            string
		data            string
		wantTermination bool
		wantFinalUsage  bool
	}{
		{
			// Anthropic non-streaming body (chunked): top-level type "message"
			// is both the final usage and the response completion (issue #1364)
			name:            "anthropic non-stream message",
			data:            `{"type":"message","usage":{"input_tokens":574145,"output_tokens":109329,"cache_read_input_tokens":7395200}}`,
			wantTermination: true,
			wantFinalUsage:  true,
		},
		{
			// OpenAI-style non-streaming body without a top-level type
			name:            "openai non-stream body",
			data:            `{"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`,
			wantTermination: true,
			wantFinalUsage:  true,
		},
		{
			// Body without usage stays a guess: neither final nor termination
			name:            "body without usage",
			data:            `{"id":"chatcmpl-1","choices":[{"message":{"content":"hi"}}]}`,
			wantTermination: false,
			wantFinalUsage:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := RawEvent([]byte(tt.data))
			q := ev.GetQuotaUsage()
			if q.IsTermination != tt.wantTermination {
				t.Errorf("IsTermination = %v, want %v", q.IsTermination, tt.wantTermination)
			}
			if q.IsFinalUsage != tt.wantFinalUsage {
				t.Errorf("IsFinalUsage = %v, want %v", q.IsFinalUsage, tt.wantFinalUsage)
			}
		})
	}
}

func TestRawEventGetQuotaUsage_AnthropicNonStreamFields(t *testing.T) {
	ev := RawEvent([]byte(`{"type":"message","usage":{"input_tokens":320,"output_tokens":150,"cache_read_input_tokens":8000,"cache_creation_input_tokens":200}}`))
	q := ev.GetQuotaUsage()
	if q.PromptTokens != 8520 {
		t.Errorf("expected PromptTokens 8520 (320+8000+200), got %d", q.PromptTokens)
	}
	if q.CompletionTokens != 150 {
		t.Errorf("expected CompletionTokens 150, got %d", q.CompletionTokens)
	}
	if q.CacheReadTokens != 8000 || q.CacheWriteTokens != 200 {
		t.Errorf("unexpected cache tokens: %+v", q)
	}
	if q.IsGuess {
		t.Error("expected IsGuess false")
	}
	if !q.IsFinalUsage || !q.IsTermination {
		t.Errorf("expected final usage and termination for non-stream Anthropic body, got final=%v termination=%v", q.IsFinalUsage, q.IsTermination)
	}
}

func TestSSEEventGetQuotaUsage_CompletionFlags(t *testing.T) {
	tests := []struct {
		name            string
		data            string
		wantTermination bool
		wantFinalUsage  bool
	}{
		{
			// Anthropic initial usage: output_tokens = 0, must not be final
			name:            "anthropic message_start",
			data:            `{"type":"message_start","usage":{"input_tokens":320,"output_tokens":0}}`,
			wantTermination: false,
			wantFinalUsage:  false,
		},
		{
			// Anthropic final usage arrives in message_delta
			name:            "anthropic message_delta",
			data:            `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":15}}`,
			wantTermination: false,
			wantFinalUsage:  true,
		},
		{
			name:            "anthropic message_stop",
			data:            `{"type":"message_stop"}`,
			wantTermination: true,
			wantFinalUsage:  false,
		},
		{
			// OpenAI stream termination marker
			name:            "openai done",
			data:            `[DONE]`,
			wantTermination: true,
			wantFinalUsage:  false,
		},
		{
			// OpenAI final chunk with stream_options.include_usage
			name:            "openai final usage chunk",
			data:            `{"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`,
			wantTermination: false,
			wantFinalUsage:  true,
		},
		{
			// Anthropic non-streaming top-level type: the whole-body JSON is
			// the final usage (issue #1364)
			name:            "anthropic non-stream message",
			data:            `{"type":"message","usage":{"input_tokens":320,"output_tokens":150,"cache_read_input_tokens":8000}}`,
			wantTermination: false,
			wantFinalUsage:  true,
		},
		{
			// Intermediate chunk without usage is neither final nor termination
			name:            "openai intermediate chunk",
			data:            `{"id":"chatcmpl-1","choices":[{"delta":{"content":"hi"}}]}`,
			wantTermination: false,
			wantFinalUsage:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := &SSEEvent{DataLines: [][]byte{[]byte(tt.data)}}
			q := ev.GetQuotaUsage()
			if q.IsTermination != tt.wantTermination {
				t.Errorf("IsTermination = %v, want %v", q.IsTermination, tt.wantTermination)
			}
			if q.IsFinalUsage != tt.wantFinalUsage {
				t.Errorf("IsFinalUsage = %v, want %v", q.IsFinalUsage, tt.wantFinalUsage)
			}
		})
	}
}
