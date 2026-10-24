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

package mod_ai_cache

import (
	"testing"
)

func TestExtractStreamAnswerBasic(t *testing.T) {
	payload := "data:{\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
		"data:{\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"}}]}\n\n" +
		"data:{\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"}}]}\n\n" +
		"data:[DONE]\n\n"

	got := extractStreamAnswer([]byte(payload), DefaultCacheStreamPath)
	if got != "Hello world" {
		t.Errorf("expected %q, got %q", "Hello world", got)
	}
}

func TestExtractStreamAnswerDoneSamePackage(t *testing.T) {
	// [DONE] and the last content chunk in the same package (no trailing
	// blank line after [DONE])
	payload := "data:{\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"}}]}\n\n" +
		"data:[DONE]"

	got := extractStreamAnswer([]byte(payload), DefaultCacheStreamPath)
	if got != "answer" {
		t.Errorf("expected %q, got %q", "answer", got)
	}
}

func TestExtractStreamAnswerRoleOnlyFirstChunk(t *testing.T) {
	// first chunk carries only role, no delta.content
	payload := "data:{\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"}}]}\n\n" +
		"data:{\"choices\":[{\"index\":0,\"delta\":{\"content\":\"x\"}}]}\n\n" +
		"data:[DONE]\n\n"

	got := extractStreamAnswer([]byte(payload), DefaultCacheStreamPath)
	if got != "x" {
		t.Errorf("expected %q, got %q", "x", got)
	}
}

func TestExtractStreamAnswerCrlf(t *testing.T) {
	payload := "data:{\"choices\":[{\"index\":0,\"delta\":{\"content\":\"a\"}}]}\r\n\r\n" +
		"data:[DONE]\r\n\r\n"

	got := extractStreamAnswer([]byte(payload), DefaultCacheStreamPath)
	if got != "a" {
		t.Errorf("expected %q, got %q", "a", got)
	}
}

func TestExtractStreamAnswerMultiLineData(t *testing.T) {
	payload := "data:{\ndata:\"choices\":null}\n\n"

	// invalid JSON payload contributes nothing, no panic
	got := extractStreamAnswer([]byte(payload), DefaultCacheStreamPath)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestSplitSSEEventsTrailingPartial(t *testing.T) {
	events := splitSSEEvents("data:a\n\ndata:b")
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if sseEventData(events[0]) != "a" || sseEventData(events[1]) != "b" {
		t.Errorf("unexpected events: %v", events)
	}
}

func TestBuildSSEEvent(t *testing.T) {
	if got := buildSSEEvent("x"); got != "data:x\n\n" {
		t.Errorf("unexpected event framing: %q", got)
	}
}
