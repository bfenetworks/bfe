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

package gemini

import (
	"testing"

	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

func TestExtractUsageFieldsAllFields(t *testing.T) {
	data := []byte(`{"candidates":[{"content":{"parts":[{"text":"hi"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"cachedContentTokenCount":3,"totalTokenCount":15}}`)
	f := New().ExtractUsageFields(data)
	want := utils.UsageFields{
		UsedQuota:        15,
		PromptTokens:     10,
		CompletionTokens: 5,
		CacheReadTokens:  3,
	}
	if f != want {
		t.Errorf("got %+v, want %+v", f, want)
	}
}

func TestExtractUsageFieldsMissingTotalTokenCount(t *testing.T) {
	// totalTokenCount missing: UsedQuota falls back to prompt + candidates.
	data := []byte(`{"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":8,"cachedContentTokenCount":4}}`)
	f := New().ExtractUsageFields(data)
	if f.UsedQuota != 20 {
		t.Errorf("expected UsedQuota 20 (12+8), got %d", f.UsedQuota)
	}
	if f.PromptTokens != 12 || f.CompletionTokens != 8 || f.CacheReadTokens != 4 {
		t.Errorf("unexpected fields: %+v", f)
	}
}

func TestExtractUsageFieldsAllZero(t *testing.T) {
	f := New().ExtractUsageFields([]byte(`{"usageMetadata":{}}`))
	if f != (utils.UsageFields{}) {
		t.Errorf("expected all-zero fields, got %+v", f)
	}
}

func TestExtractUsageFieldsNoForeignFields(t *testing.T) {
	// The gemini chain must not parse OpenAI or Anthropic fields.
	openai := New().ExtractUsageFields([]byte(
		`{"usage":{"total_tokens":12,"prompt_tokens":8,"completion_tokens":4}}`))
	if openai != (utils.UsageFields{}) {
		t.Errorf("expected zero fields for openai body, got %+v", openai)
	}
	anthropic := New().ExtractUsageFields([]byte(
		`{"type":"message","usage":{"input_tokens":320,"output_tokens":150}}`))
	if anthropic != (utils.UsageFields{}) {
		t.Errorf("expected zero fields for anthropic body, got %+v", anthropic)
	}
}
