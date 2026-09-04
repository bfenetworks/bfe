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

// Expected values below are derived field-by-field from the legacy Claude
// fallback chain, which was moved verbatim from
// bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:151-169 and
// bfe_modules/mod_body_process/llm_util.go:160-175.

func TestExtractUsageFieldsClaudeCache(t *testing.T) {
	data := []byte(`{"type":"message_start","usage":{"input_tokens":320,"output_tokens":150,"cache_read_input_tokens":8000,"cache_creation_input_tokens":200}}`)
	f := New().ExtractUsageFields(data)

	// PromptTokens normalized to 320+8000+200 (total input), UsedQuota
	// falls back to prompt+completion (8520+150).
	want := utils.UsageFields{
		UsedQuota:        8670,
		PromptTokens:     8520,
		CompletionTokens: 150,
		CacheReadTokens:  8000,
		CacheWriteTokens: 200,
	}
	if f != want {
		t.Errorf("got %+v, want %+v", f, want)
	}
}

func TestExtractUsageFieldsClaudeStreamingMessageUsage(t *testing.T) {
	// Real Anthropic streaming nests the initial usage under message.usage
	// (message_start); the final usage arrives in the top-level usage of
	// message_delta. Both shapes must be parsed (issue #1352).
	start := []byte(`{"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":320,"output_tokens":0,"cache_read_input_tokens":8000,"cache_creation_input_tokens":200}}}`)
	f := New().ExtractUsageFields(start)
	want := utils.UsageFields{
		UsedQuota:        8520,
		PromptTokens:     8520,
		CompletionTokens: 0,
		CacheReadTokens:  8000,
		CacheWriteTokens: 200,
	}
	if f != want {
		t.Errorf("message_start: got %+v, want %+v", f, want)
	}

	delta := []byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":150}}`)
	f = New().ExtractUsageFields(delta)
	if f.CompletionTokens != 150 || f.UsedQuota != 150 {
		t.Errorf("message_delta: got %+v, want CompletionTokens 150 UsedQuota 150", f)
	}
}

func TestExtractUsageFieldsFullCacheHit(t *testing.T) {
	// 100% cache hit: input_tokens = 0. Usage must still be recognized.
	data := []byte(`{"type":"message_start","usage":{"input_tokens":0,"output_tokens":42,"cache_read_input_tokens":5000}}`)
	f := New().ExtractUsageFields(data)

	want := utils.UsageFields{
		UsedQuota:        5042,
		PromptTokens:     5000,
		CompletionTokens: 42,
		CacheReadTokens:  5000,
	}
	if f != want {
		t.Errorf("got %+v, want %+v", f, want)
	}
}

func TestExtractUsageFieldsTotalTokensKept(t *testing.T) {
	// total_tokens is used as-is when present (not recomputed).
	data := []byte(`{"usage":{"total_tokens":9000,"input_tokens":320,"output_tokens":150,"cache_read_input_tokens":8000}}`)
	f := New().ExtractUsageFields(data)
	if f.UsedQuota != 9000 {
		t.Errorf("expected UsedQuota 9000, got %d", f.UsedQuota)
	}
	if f.PromptTokens != 8320 || f.CompletionTokens != 150 || f.CacheReadTokens != 8000 {
		t.Errorf("unexpected fields: %+v", f)
	}
}

func TestExtractUsageFieldsNoOpenAIFields(t *testing.T) {
	// The anthropic adapter does not parse OpenAI-style fields.
	data := []byte(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4}}`)
	f := New().ExtractUsageFields(data)
	if f.PromptTokens != 0 || f.CompletionTokens != 0 || f.UsedQuota != 12 {
		t.Errorf("unexpected fields: %+v", f)
	}
}
