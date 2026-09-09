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

func TestIsStreamTerminalAlwaysFalse(t *testing.T) {
	// Gemini streams carry no SSE termination event; HTTP stream EOF ends
	// the response. No event shape may be treated as a terminator.
	a := New()
	cases := []utils.StreamEvent{
		{Type: "", Data: `[DONE]`},
		{Type: "message_stop", Data: `{"type":"message_stop"}`},
		{Data: `{"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`},
		{Data: `{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]`},
	}
	for _, ev := range cases {
		if a.IsStreamTerminal(ev) {
			t.Errorf("IsStreamTerminal(%+v) = true, want false", ev)
		}
	}
}

func TestIsFinalUsageEvent(t *testing.T) {
	a := New()
	cases := []struct {
		name string
		ev   utils.StreamEvent
		want bool
	}{
		{
			name: "chunk with usageMetadata",
			ev:   utils.StreamEvent{Data: `{"candidates":[{"content":{"parts":[{"text":"hi"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`},
			want: true,
		},
		{
			name: "final accumulated chunk",
			ev:   utils.StreamEvent{Data: `{"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":20,"totalTokenCount":30}}`},
			want: true,
		},
		{
			name: "content-only chunk",
			ev:   utils.StreamEvent{Data: `{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]`},
			want: false,
		},
		{
			name: "openai final chunk is not a gemini usage event",
			ev:   utils.StreamEvent{Data: `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`},
			want: false,
		},
		{
			name: "anthropic message_delta is not a gemini usage event",
			ev:   utils.StreamEvent{Type: "message_delta", Data: `{"type":"message_delta","usage":{"output_tokens":15}}`},
			want: false,
		},
	}
	for _, tc := range cases {
		if got := a.IsFinalUsageEvent(tc.ev); got != tc.want {
			t.Errorf("%s: IsFinalUsageEvent = %v, want %v", tc.name, got, tc.want)
		}
	}
}
