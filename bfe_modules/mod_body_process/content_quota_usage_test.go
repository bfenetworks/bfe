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
