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
