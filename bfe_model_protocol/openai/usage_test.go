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

package openai

import (
	"testing"

	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

// Expected values below are derived field-by-field from the legacy
// extraction chain (see utils.ParseOpenAIUsageFields), which was moved
// verbatim from bfe_modules/mod_body_process/llm_util.go and
// bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go.

func TestExtractUsageFieldsChatUsage(t *testing.T) {
	data := []byte(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"cache_read_tokens":3,"cache_write_tokens":2,"audio_input_tokens":1,"audio_output_tokens":1,"input_token_details":{"image_tokens":7},"image_count":2,"video_count":1}}`)
	f := New().ExtractUsageFields(data)

	want := utils.UsageFields{
		UsedQuota:         12,
		PromptTokens:      8,
		CompletionTokens:  4,
		CacheReadTokens:   3,
		CacheWriteTokens:  2,
		AudioInputTokens:  1,
		AudioOutputTokens: 1,
		ImageInputTokens:  7,
		ImageCount:        2,
		VideoCount:        1,
	}
	if f != want {
		t.Errorf("got %+v, want %+v", f, want)
	}
}

func TestExtractUsageFieldsImageInputFallback(t *testing.T) {
	// image_input_tokens is the fallback when input_token_details.image_tokens is absent
	data := []byte(`{"usage":{"total_tokens":10,"prompt_tokens":8,"completion_tokens":2,"image_input_tokens":9}}`)
	f := New().ExtractUsageFields(data)
	if f.ImageInputTokens != 9 {
		t.Errorf("expected ImageInputTokens 9, got %d", f.ImageInputTokens)
	}
}

func TestExtractUsageFieldsDataArrayFallback(t *testing.T) {
	// data.# feeds both image_count and video_count when usage fields are absent
	data := []byte(`{"usage":{},"data":[{},{},{}]}`)
	f := New().ExtractUsageFields(data)
	if f.ImageCount != 3 || f.VideoCount != 3 {
		t.Errorf("expected ImageCount/VideoCount 3/3, got %d/%d", f.ImageCount, f.VideoCount)
	}
}

func TestExtractUsageFieldsDeepSeekCache(t *testing.T) {
	// DeepSeek: prompt_cache_hit_tokens
	data := []byte(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"prompt_cache_hit_tokens":5}}`)
	f := New().ExtractUsageFields(data)
	if f.CacheReadTokens != 5 {
		t.Errorf("expected CacheReadTokens 5 for prompt_cache_hit_tokens, got %d", f.CacheReadTokens)
	}

	// DeepSeek: prompt_tokens_details.cached_tokens
	data2 := []byte(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":6}}}`)
	f2 := New().ExtractUsageFields(data2)
	if f2.CacheReadTokens != 6 {
		t.Errorf("expected CacheReadTokens 6 for prompt_tokens_details.cached_tokens, got %d", f2.CacheReadTokens)
	}

	// Existing cache_read_tokens takes precedence when non-zero
	data3 := []byte(`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4,"cache_read_tokens":3,"prompt_cache_hit_tokens":5}}`)
	f3 := New().ExtractUsageFields(data3)
	if f3.CacheReadTokens != 3 {
		t.Errorf("expected CacheReadTokens 3 (existing field precedence), got %d", f3.CacheReadTokens)
	}
}

func TestExtractUsageFieldsResponsesCachedTokens(t *testing.T) {
	// Responses API: input_token_details.cached_tokens
	data := []byte(`{"usage":{"total_tokens":150,"prompt_tokens":100,"completion_tokens":50,"input_token_details":{"cached_tokens":20}}}`)
	f := New().ExtractUsageFields(data)
	if f.CacheReadTokens != 20 {
		t.Errorf("expected CacheReadTokens 20 for input_token_details.cached_tokens, got %d", f.CacheReadTokens)
	}
	if f.UsedQuota != 150 || f.PromptTokens != 100 || f.CompletionTokens != 50 {
		t.Errorf("unexpected usage fields: %+v", f)
	}
}

func TestExtractUsageFieldsAllZero(t *testing.T) {
	data := []byte(`{"usage":{}}`)
	f := New().ExtractUsageFields(data)
	if f != (utils.UsageFields{}) {
		t.Errorf("expected all-zero UsageFields, got %+v", f)
	}
}

func TestExtractUsageFieldsNoClaudeChain(t *testing.T) {
	// The openai adapter must NOT apply the Claude fallback chain: an
	// Anthropic-style body keeps prompt/completion at zero here.
	data := []byte(`{"usage":{"input_tokens":320,"output_tokens":150,"cache_read_input_tokens":8000,"cache_creation_input_tokens":200}}`)
	f := New().ExtractUsageFields(data)
	if f.PromptTokens != 0 || f.CompletionTokens != 0 || f.CacheReadTokens != 0 || f.CacheWriteTokens != 0 {
		t.Errorf("expected no Claude parsing from openai adapter, got %+v", f)
	}
}
