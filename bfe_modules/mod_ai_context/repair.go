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

// repair.go: structural fidelity after trimming (design 7.3, 不可妥协项).
// Trimming can break tool_call/tool pairs; repair fixes the structure with
// multi-pass cleanup and validates the result. Still illegal after repair =>
// the handler rolls back to the original body (repair_rollback, fail-open).

// maxRepairPasses bounds the multi-pass cleanup; each pass can only shrink
// the message list, so a small bound is enough.
const maxRepairPasses = 5

// valid roles of an OpenAI chat completions message
var validMessageRoles = map[string]bool{
	"system":    true,
	"developer": true,
	"user":      true,
	"assistant": true,
	"tool":      true,
}

// repairMessages fixes structural breakage: orphaned tool messages removed,
// dangling tool_calls stripped and emptied messages dropped. Multi-pass
// until no change, because stripping a tool_call orphans its tool message
// and removing a tool message can leave another tool_call dangling.
func repairMessages(msgs []Message) []Message {
	for pass := 0; pass < maxRepairPasses; pass++ {
		var changed bool
		var c bool
		msgs, c = stripOrphanToolMessages(msgs)
		changed = changed || c
		msgs, c = stripDanglingToolCalls(msgs)
		changed = changed || c
		msgs, c = dropEmptyMessages(msgs)
		changed = changed || c
		if !changed {
			break
		}
	}
	return msgs
}

// stripOrphanToolMessages removes tool messages whose tool_call_id has no
// matching assistant tool_call earlier in the conversation. The last user
// message is never removed (it cannot be a tool message anyway, the guard is
// defensive for future role extensions).
func stripOrphanToolMessages(msgs []Message) ([]Message, bool) {
	protected := lastIndexOfRole(msgs, "user")
	changed := false
	out := msgs[:0]
	for i, m := range msgs {
		if m.Role == "tool" && i != protected && !hasAnsweringAssistant(msgs, i, m.ToolCallID) {
			changed = true
			continue
		}
		out = append(out, m)
	}
	return out, changed
}

// hasAnsweringAssistant reports whether an assistant message before idx
// declares a tool_call with the given id.
func hasAnsweringAssistant(msgs []Message, idx int, toolCallID string) bool {
	if toolCallID == "" {
		return false
	}
	for j := 0; j < idx; j++ {
		if msgs[j].Role != "assistant" {
			continue
		}
		for _, tc := range msgs[j].ToolCalls {
			if tc != nil && tc.ID == toolCallID {
				return true
			}
		}
	}
	return false
}

// stripDanglingToolCalls removes tool_calls entries (of assistant messages)
// that have no matching tool response anywhere after them. Stripping a
// tool_call orphans the matching tool message; the multi-pass loop removes
// it in the next pass.
func stripDanglingToolCalls(msgs []Message) ([]Message, bool) {
	changed := false
	for i := range msgs {
		if msgs[i].Role != "assistant" || len(msgs[i].ToolCalls) == 0 {
			continue
		}
		kept := msgs[i].ToolCalls[:0]
		for _, tc := range msgs[i].ToolCalls {
			if tc != nil && hasToolResponse(msgs, i, tc.ID) {
				kept = append(kept, tc)
			}
		}
		if len(kept) != len(msgs[i].ToolCalls) {
			changed = true
		}
		if len(kept) == 0 {
			msgs[i].ToolCalls = nil
		} else {
			msgs[i].ToolCalls = kept
		}
	}
	return msgs, changed
}

// hasToolResponse reports whether a tool message after idx answers the given
// tool_call id.
func hasToolResponse(msgs []Message, idx int, toolCallID string) bool {
	if toolCallID == "" {
		return false
	}
	for j := idx + 1; j < len(msgs); j++ {
		if msgs[j].Role == "tool" && msgs[j].ToolCallID == toolCallID {
			return true
		}
	}
	return false
}

// dropEmptyMessages removes messages that carry no content at all and no
// tool structure (e.g. an assistant message whose only thinking part was
// removed by L2). System messages and the last user message are never
// dropped (design 7.3: system frozen, last user message untouched).
func dropEmptyMessages(msgs []Message) ([]Message, bool) {
	protected := lastIndexOfRole(msgs, "user")
	changed := false
	out := msgs[:0]
	for i, m := range msgs {
		if i == protected || m.Role == "system" {
			out = append(out, m)
			continue
		}
		if m.emptyContent() && len(m.ToolCalls) == 0 && m.ToolCallID == "" {
			changed = true
			continue
		}
		out = append(out, m)
	}
	return out, changed
}

// emptyContent reports whether the message content carries no text/parts.
func (m *Message) emptyContent() bool {
	trimmed := trimSpaceBytes(m.Content)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return true
	}
	if s, ok := m.contentString(); ok {
		return s == ""
	}
	if parts, ok := m.contentParts(); ok {
		return len(parts) == 0
	}
	return false
}

func trimSpaceBytes(b []byte) []byte {
	start := 0
	for start < len(b) && isSpaceByte(b[start]) {
		start++
	}
	end := len(b)
	for end > start && isSpaceByte(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// validateMessages performs the final legality check after repair (design
// 7.1-10): messages non-empty, roles known, tool messages paired with an
// earlier assistant tool_call, every tool_call answered by a later tool
// message, and structured content parts non-empty.
func validateMessages(msgs []Message) bool {
	if len(msgs) == 0 {
		return false
	}
	for i := range msgs {
		m := &msgs[i]
		if !validMessageRoles[m.Role] {
			return false
		}
		if m.Role == "tool" {
			if m.ToolCallID == "" {
				return false
			}
			if !hasAnsweringAssistant(msgs, i, m.ToolCallID) {
				return false
			}
		}
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				if tc == nil || tc.ID == "" {
					return false
				}
				if !hasToolResponse(msgs, i, tc.ID) {
					return false
				}
			}
		}
		if parts, ok := m.contentParts(); ok && len(parts) == 0 {
			return false
		}
	}
	return true
}
