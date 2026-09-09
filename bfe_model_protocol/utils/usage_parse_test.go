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

package utils

import "testing"

func TestParseUsageFieldsCrossProtocol_AnthropicBody(t *testing.T) {
	// OpenAI chain parses nothing from an Anthropic body; the anthropic
	// chain must recover the full usage (issue #1364).
	fields := ParseUsageFieldsCrossProtocol([]byte(
		`{"id":"msg_01","type":"message","role":"assistant","usage":{"input_tokens":320,"output_tokens":150,"cache_read_input_tokens":8000,"cache_creation_input_tokens":200}}`))
	if fields.PromptTokens != 8520 {
		t.Errorf("expected PromptTokens 8520 (320+8000+200), got %d", fields.PromptTokens)
	}
	if fields.CompletionTokens != 150 {
		t.Errorf("expected CompletionTokens 150, got %d", fields.CompletionTokens)
	}
	if fields.CacheReadTokens != 8000 || fields.CacheWriteTokens != 200 {
		t.Errorf("unexpected cache tokens: %+v", fields)
	}
	if fields.UsedQuota != 8670 {
		t.Errorf("expected UsedQuota 8670 (8520+150), got %d", fields.UsedQuota)
	}
}

func TestParseUsageFieldsCrossProtocol_OpenAIBodyUnchanged(t *testing.T) {
	// OpenAI-style bodies keep the openai chain result; the anthropic
	// fallback must not kick in.
	fields := ParseUsageFieldsCrossProtocol([]byte(
		`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"cache_read_tokens":5}}`))
	if fields.PromptTokens != 8 || fields.CompletionTokens != 4 || fields.UsedQuota != 12 {
		t.Errorf("unexpected openai fields: %+v", fields)
	}
	if fields.CacheReadTokens != 5 {
		t.Errorf("expected CacheReadTokens 5, got %d", fields.CacheReadTokens)
	}
}

func TestParseUsageFieldsCrossProtocol_FullCacheHit(t *testing.T) {
	fields := ParseUsageFieldsCrossProtocol([]byte(
		`{"type":"message","usage":{"input_tokens":0,"output_tokens":42,"cache_read_input_tokens":5000}}`))
	if fields.PromptTokens != 5000 || fields.CacheReadTokens != 5000 {
		t.Errorf("unexpected full-cache-hit fields: %+v", fields)
	}
	if fields.UsedQuota != 5042 {
		t.Errorf("expected UsedQuota 5042 (5000+42), got %d", fields.UsedQuota)
	}
}

func TestParseUsageFieldsCrossProtocol_NoUsage(t *testing.T) {
	fields := ParseUsageFieldsCrossProtocol([]byte(`{"id":"chatcmpl-1","choices":[{"delta":{"content":"hi"}}]}`))
	if fields.PromptTokens != 0 || fields.CompletionTokens != 0 || fields.UsedQuota != 0 {
		t.Errorf("expected all-zero fields for usage-less body, got %+v", fields)
	}
}

func TestParseAnthropicUsageFields_CacheWrite1h(t *testing.T) {
	// Anthropic extended-TTL: 1h cache write tokens reported separately,
	// still included in the total cache write tokens.
	fields := ParseAnthropicUsageFields([]byte(
		`{"usage":{"input_tokens":320,"output_tokens":150,"cache_creation_input_tokens":1200,"cache_creation":{"ephemeral_1h_input_tokens":1000}}}`))
	if fields.CacheWriteTokens != 1200 || fields.CacheWriteTokens1h != 1000 {
		t.Errorf("unexpected cache write fields: %+v", fields)
	}

	// streaming message_start nests usage under message.usage
	fields2 := ParseAnthropicUsageFields([]byte(
		`{"type":"message_start","message":{"usage":{"input_tokens":320,"output_tokens":0,"cache_creation_input_tokens":1200,"cache_creation":{"ephemeral_1h_input_tokens":1000}}}}`))
	if fields2.CacheWriteTokens != 1200 || fields2.CacheWriteTokens1h != 1000 {
		t.Errorf("unexpected nested cache write fields: %+v", fields2)
	}

	// relay fallback field
	fields3 := ParseAnthropicUsageFields([]byte(
		`{"usage":{"input_tokens":320,"output_tokens":150,"cache_creation_input_tokens":1200,"cache_creation_input_tokens_1h":800}}`))
	if fields3.CacheWriteTokens1h != 800 {
		t.Errorf("expected CacheWriteTokens1h 800 (fallback field), got %d", fields3.CacheWriteTokens1h)
	}
}

func TestParseGeminiUsageFields(t *testing.T) {
	// All fields present: totalTokenCount used as-is.
	fields := ParseGeminiUsageFields([]byte(
		`{"candidates":[{"content":{"parts":[{"text":"hi"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"cachedContentTokenCount":3,"totalTokenCount":15}}`))
	want := UsageFields{
		UsedQuota:        15,
		PromptTokens:     10,
		CompletionTokens: 5,
		CacheReadTokens:  3,
	}
	if fields != want {
		t.Errorf("got %+v, want %+v", fields, want)
	}
}

func TestParseGeminiUsageFieldsMissingTotal(t *testing.T) {
	// totalTokenCount absent: UsedQuota falls back to prompt + candidates.
	fields := ParseGeminiUsageFields([]byte(
		`{"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":8,"cachedContentTokenCount":4}}`))
	if fields.UsedQuota != 20 {
		t.Errorf("expected UsedQuota 20 (12+8), got %d", fields.UsedQuota)
	}
	if fields.PromptTokens != 12 || fields.CompletionTokens != 8 || fields.CacheReadTokens != 4 {
		t.Errorf("unexpected fields: %+v", fields)
	}
}

func TestParseGeminiUsageFieldsAllZero(t *testing.T) {
	// Empty usageMetadata stays all-zero (a guess for the callers).
	fields := ParseGeminiUsageFields([]byte(`{"usageMetadata":{}}`))
	if fields != (UsageFields{}) {
		t.Errorf("expected all-zero fields, got %+v", fields)
	}
}

func TestParseGeminiUsageFieldsNoForeignFields(t *testing.T) {
	// The gemini chain does not parse OpenAI / Anthropic fields.
	openai := ParseGeminiUsageFields([]byte(
		`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4}}`))
	if openai != (UsageFields{}) {
		t.Errorf("expected zero fields for openai body, got %+v", openai)
	}
	anthropic := ParseGeminiUsageFields([]byte(
		`{"type":"message","usage":{"input_tokens":320,"output_tokens":150}}`))
	if anthropic != (UsageFields{}) {
		t.Errorf("expected zero fields for anthropic body, got %+v", anthropic)
	}
}

func TestParseUsageFieldsCrossProtocol_GeminiBody(t *testing.T) {
	// A Bearer key detected as openai while the backend returns a Gemini
	// body: the gemini third stage recovers the usage (issue #1364
	// mismatch fallback).
	fields := ParseUsageFieldsCrossProtocol([]byte(
		`{"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"cachedContentTokenCount":3,"totalTokenCount":15}}`))
	if fields.PromptTokens != 10 || fields.CompletionTokens != 5 {
		t.Errorf("unexpected prompt/completion: %+v", fields)
	}
	if fields.CacheReadTokens != 3 {
		t.Errorf("expected CacheReadTokens 3, got %d", fields.CacheReadTokens)
	}
	if fields.UsedQuota != 15 {
		t.Errorf("expected UsedQuota 15, got %d", fields.UsedQuota)
	}
}
