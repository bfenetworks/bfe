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

package mod_ai_context

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Message is the typed model of an OpenAI chat completions message. Content
// stays a json.RawMessage because it is legal both as a plain string and as
// an array of typed parts. Fields the pipeline does not know about round-trip
// through Extra so reserialization preserves the original message shape
// (precedent: mod_ai_intent/msg_extract.go tolerates string and typed-parts
// content; the typed model here is what the trim/rewrite stages operate on).
type Message struct {
	Role             string          `json:"role"`
	Content          json.RawMessage `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	Name             string          `json:"name,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
	ToolCalls        []*ToolCall     `json:"tool_calls,omitempty"`

	// Extra carries message fields unknown to the pipeline (e.g. refusal,
	// provider-specific extensions) and round-trips them on marshal.
	Extra map[string]json.RawMessage `json:"-"`
}

func (m *Message) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = Message{}
	for k, v := range raw {
		switch k {
		case "role":
			if err := json.Unmarshal(v, &m.Role); err != nil {
				return err
			}
		case "content":
			m.Content = cloneRaw(v)
		case "reasoning_content":
			if err := json.Unmarshal(v, &m.ReasoningContent); err != nil {
				return err
			}
		case "name":
			if err := json.Unmarshal(v, &m.Name); err != nil {
				return err
			}
		case "tool_call_id":
			if err := json.Unmarshal(v, &m.ToolCallID); err != nil {
				return err
			}
		case "tool_calls":
			if err := json.Unmarshal(v, &m.ToolCalls); err != nil {
				return err
			}
		default:
			if m.Extra == nil {
				m.Extra = make(map[string]json.RawMessage)
			}
			m.Extra[k] = cloneRaw(v)
		}
	}
	return nil
}

func (m Message) MarshalJSON() ([]byte, error) {
	raw := make(map[string]json.RawMessage, len(m.Extra)+6)
	for k, v := range m.Extra {
		raw[k] = v
	}
	put := func(k string, v interface{}) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		raw[k] = b
		return nil
	}
	if err := put("role", m.Role); err != nil {
		return nil, err
	}
	if m.Content != nil {
		raw["content"] = m.Content
	}
	if m.ReasoningContent != "" {
		if err := put("reasoning_content", m.ReasoningContent); err != nil {
			return nil, err
		}
	}
	if m.Name != "" {
		if err := put("name", m.Name); err != nil {
			return nil, err
		}
	}
	if m.ToolCallID != "" {
		if err := put("tool_call_id", m.ToolCallID); err != nil {
			return nil, err
		}
	}
	if len(m.ToolCalls) > 0 {
		if err := put("tool_calls", m.ToolCalls); err != nil {
			return nil, err
		}
	}
	return json.Marshal(raw)
}

// ToolCall is the OpenAI tool_call object of an assistant message.
type ToolCall struct {
	Index    int              `json:"index,omitempty"`
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ContentPart is one element of a typed-parts message content.
type ContentPart struct {
	Type     string    `json:"type"` // text / image_url / thinking / ...
	Text     string    `json:"text,omitempty"`
	Thinking string    `json:"thinking,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`

	// Extra carries part fields unknown to the pipeline (detail, file data,
	// provider extensions) and round-trips them on marshal.
	Extra map[string]json.RawMessage `json:"-"`
}

type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

func (p *ContentPart) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*p = ContentPart{}
	for k, v := range raw {
		switch k {
		case "type":
			if err := json.Unmarshal(v, &p.Type); err != nil {
				return err
			}
		case "text":
			if err := json.Unmarshal(v, &p.Text); err != nil {
				return err
			}
		case "thinking":
			if err := json.Unmarshal(v, &p.Thinking); err != nil {
				return err
			}
		case "image_url":
			var iu ImageURL
			if err := json.Unmarshal(v, &iu); err != nil {
				return err
			}
			p.ImageURL = &iu
		default:
			if p.Extra == nil {
				p.Extra = make(map[string]json.RawMessage)
			}
			p.Extra[k] = cloneRaw(v)
		}
	}
	return nil
}

func (p ContentPart) MarshalJSON() ([]byte, error) {
	raw := make(map[string]json.RawMessage, len(p.Extra)+4)
	for k, v := range p.Extra {
		raw[k] = v
	}
	put := func(k string, v interface{}) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		raw[k] = b
		return nil
	}
	if err := put("type", p.Type); err != nil {
		return nil, err
	}
	if p.Text != "" {
		if err := put("text", p.Text); err != nil {
			return nil, err
		}
	}
	if p.Thinking != "" {
		if err := put("thinking", p.Thinking); err != nil {
			return nil, err
		}
	}
	if p.ImageURL != nil {
		if err := put("image_url", p.ImageURL); err != nil {
			return nil, err
		}
	}
	return json.Marshal(raw)
}

func cloneRaw(v json.RawMessage) json.RawMessage {
	if v == nil {
		return nil
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out
}

// contentString returns the content as a plain string; ok is false when the
// content is absent or a typed-parts array.
func (m *Message) contentString() (string, bool) {
	if m.Content == nil {
		return "", false
	}
	trimmed := bytes.TrimSpace(m.Content)
	if len(trimmed) == 0 {
		return "", false
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return s, true
		}
		return "", false
	}
	if bytes.Equal(trimmed, []byte("null")) {
		// content: null (common on tool-calling assistant messages)
		return "", true
	}
	return "", false
}

// contentParts returns the content as a typed-parts array; ok is false when
// the content is absent, null or a plain string.
func (m *Message) contentParts() ([]ContentPart, bool) {
	if m.Content == nil {
		return nil, false
	}
	trimmed := bytes.TrimSpace(m.Content)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, false
	}
	var parts []ContentPart
	if err := json.Unmarshal(trimmed, &parts); err != nil {
		return nil, false
	}
	return parts, true
}

func (m *Message) setContentString(s string) {
	if s == "" {
		m.Content = nil
		return
	}
	b, err := json.Marshal(s)
	if err != nil {
		return
	}
	m.Content = b
}

func (m *Message) setContentParts(parts []ContentPart) {
	if len(parts) == 0 {
		m.Content = json.RawMessage("null")
		return
	}
	b, err := json.Marshal(parts)
	if err != nil {
		return
	}
	m.Content = b
}

// isThinkingPart reports whether the part is a thinking/reasoning block.
func isThinkingPart(p *ContentPart) bool {
	return p.Type == "thinking" || p.Type == "reasoning"
}

// lastIndexOfRole returns the index of the last message with the given role,
// or -1 when there is none.
func lastIndexOfRole(msgs []Message, role string) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == role {
			return i
		}
	}
	return -1
}

// cloneMessages deep-copies a messages array so the pipeline can work on its
// own copy and the caller's version stays untouched (rollback safety).
func cloneMessages(msgs []Message) []Message {
	out := make([]Message, len(msgs))
	for i := range msgs {
		out[i] = msgs[i]
		out[i].Content = cloneRaw(msgs[i].Content)
		if msgs[i].Extra != nil {
			out[i].Extra = make(map[string]json.RawMessage, len(msgs[i].Extra))
			for k, v := range msgs[i].Extra {
				out[i].Extra[k] = cloneRaw(v)
			}
		}
		if msgs[i].ToolCalls != nil {
			out[i].ToolCalls = make([]*ToolCall, len(msgs[i].ToolCalls))
			for j, tc := range msgs[i].ToolCalls {
				if tc != nil {
					c := *tc
					out[i].ToolCalls[j] = &c
				}
			}
		}
	}
	return out
}

// parseMessages parses the messages array of an OpenAI chat completions
// request body. Only the messages entry is interpreted; every other
// top-level field (model, stream, tools, temperature, ...) is preserved by
// replaceMessages.
func parseMessages(body []byte) ([]Message, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("empty body")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, fmt.Errorf("parse request body err: %s", err)
	}
	raw, ok := top["messages"]
	if !ok {
		return nil, fmt.Errorf("no messages in request body")
	}
	var msgs []Message
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, fmt.Errorf("parse messages err: %s", err)
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}
	return msgs, nil
}

// replaceMessages rebuilds the request body with the processed messages;
// all other top-level fields of the original body are preserved. The body is
// re-serialized as a whole (not sjson path-wise) because trimming changes
// the messages array length (design-changes.md 6.2).
func replaceMessages(body []byte, msgs []Message) ([]byte, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, fmt.Errorf("parse request body err: %s", err)
	}
	newMsgs, err := json.Marshal(msgs)
	if err != nil {
		return nil, fmt.Errorf("marshal messages err: %s", err)
	}
	top["messages"] = newMsgs
	return json.Marshal(top)
}
