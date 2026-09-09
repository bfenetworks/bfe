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
