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

func TestIsFinalUsageEvent(t *testing.T) {
	a := &Adapter{}
	cases := []struct {
		name string
		ev   utils.StreamEvent
		want bool
	}{
		// Cross-protocol stream: a Bearer request detected as openai may be
		// answered with an Anthropic body whose final usage arrives in
		// message_delta (SC12 client-abort billing).
		{"anthropic message_delta", utils.StreamEvent{Type: "message_delta"}, true},
		// Non-streaming Anthropic body (issue #1364).
		{"anthropic message", utils.StreamEvent{Type: "message"}, true},
		// Responses API terminal event carries the final usage (issue #1381).
		{"responses completed", utils.StreamEvent{Type: "response.completed"}, true},
		// Regular OpenAI chunks are not final usage events.
		{"openai chunk", utils.StreamEvent{Type: "chat.completion.chunk"}, false},
		// Responses API non-terminal events must not be mistaken for the
		// final usage (issue #1381: no prefix/wildcard matching).
		{"responses created", utils.StreamEvent{Type: "response.created"}, false},
		{"responses output_item.done", utils.StreamEvent{Type: "response.output_item.done"}, false},
		{"responses incomplete", utils.StreamEvent{Type: "response.incomplete"}, false},
		{"responses output_text.delta", utils.StreamEvent{Type: "response.output_text.delta"}, false},
		// OpenAI stream_options.include_usage final chunk has no type.
		{"openai usage chunk", utils.StreamEvent{Type: ""}, true},
		// Anthropic message_start carries initial usage only.
		{"anthropic message_start", utils.StreamEvent{Type: "message_start"}, false},
		{"anthropic content_block_delta", utils.StreamEvent{Type: "content_block_delta"}, false},
		{"anthropic message_stop", utils.StreamEvent{Type: "message_stop"}, false},
	}
	for _, c := range cases {
		if got := a.IsFinalUsageEvent(c.ev); got != c.want {
			t.Errorf("%s: IsFinalUsageEvent = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsStreamTerminal(t *testing.T) {
	a := &Adapter{}
	cases := []struct {
		name string
		ev   utils.StreamEvent
		want bool
	}{
		{"anthropic message_stop", utils.StreamEvent{Type: "message_stop"}, true},
		{"openai done", utils.StreamEvent{Data: "[DONE]"}, true},
		// Responses API stream terminates at response.completed (issue #1381).
		{"responses completed", utils.StreamEvent{Type: "response.completed"}, true},
		{"openai chunk", utils.StreamEvent{Type: "chat.completion.chunk"}, false},
		{"responses created", utils.StreamEvent{Type: "response.created"}, false},
		{"responses incomplete", utils.StreamEvent{Type: "response.incomplete"}, false},
		{"anthropic message_delta", utils.StreamEvent{Type: "message_delta"}, false},
	}
	for _, c := range cases {
		if got := a.IsStreamTerminal(c.ev); got != c.want {
			t.Errorf("%s: IsStreamTerminal = %v, want %v", c.name, got, c.want)
		}
	}
}
