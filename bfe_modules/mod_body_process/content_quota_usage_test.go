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
