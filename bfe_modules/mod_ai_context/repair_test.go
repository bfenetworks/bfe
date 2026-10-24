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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assistantWithCall(id string, content string) Message {
	return Message{
		Role:    "assistant",
		Content: jsonRaw(`"` + content + `"`),
		ToolCalls: []*ToolCall{{
			ID:       id,
			Type:     "function",
			Function: ToolCallFunction{Name: "f", Arguments: "{}"},
		}},
	}
}

func toolResponse(id string, content string) Message {
	return Message{
		Role:       "tool",
		ToolCallID: id,
		Content:    jsonRaw(`"` + content + `"`),
	}
}

func TestRepairOrphanToolMessage(t *testing.T) {
	msgs := []Message{
		assistantWithCall("c1", "a1"),
		toolResponse("c1", "r1"),
		toolResponse("orphan", "rogue"), // no assistant declares it
		{Role: "user", Content: jsonRaw(`"q"`)},
	}
	repaired := repairMessages(cloneMessages(msgs))
	require.Len(t, repaired, 3)
	assert.Equal(t, "c1", repaired[1].ToolCallID)
	assert.True(t, validateMessages(repaired))
}

func TestRepairDanglingToolCalls(t *testing.T) {
	// last assistant declares a tool_call that never got a response
	msgs := []Message{
		{Role: "user", Content: jsonRaw(`"q"`)},
		assistantWithCall("c1", "a1"),
	}
	repaired := repairMessages(cloneMessages(msgs))
	require.Len(t, repaired, 2)
	assert.Nil(t, repaired[1].ToolCalls, "dangling tool_calls stripped")
	assert.True(t, validateMessages(repaired))
}

func TestRepairPartiallyAnsweredToolCalls(t *testing.T) {
	// one assistant declares c1 and c2, only c1 is answered: c2 is stripped
	// as dangling, c1 stays paired with its tool response
	msgs := []Message{
		{Role: "assistant", Content: jsonRaw(`"a1"`),
			ToolCalls: []*ToolCall{
				{ID: "c1", Type: "function", Function: ToolCallFunction{Name: "f", Arguments: "{}"}},
				{ID: "c2", Type: "function", Function: ToolCallFunction{Name: "f", Arguments: "{}"}},
			}},
		toolResponse("c1", "r1"),
		{Role: "user", Content: jsonRaw(`"q"`)},
	}
	repaired := repairMessages(cloneMessages(msgs))
	assert.True(t, validateMessages(repaired))
	require.Len(t, repaired[0].ToolCalls, 1)
	assert.Equal(t, "c1", repaired[0].ToolCalls[0].ID)
	assert.Equal(t, "c1", repaired[1].ToolCallID, "answered tool response survives")
}

func TestRepairMultiPass(t *testing.T) {
	// assistant declares c1 and c2; only c2 is answered. Dangling c1 is
	// stripped in pass 1, which orphans tool(c2)... no: c2 stays declared
	// and answered. Use the reverse: c1 answered, c2 dangling is stripped;
	// tool(c1) stays paired.
	msgs := []Message{
		{Role: "assistant", Content: jsonRaw(`"a0"`),
			ToolCalls: []*ToolCall{
				{ID: "c1", Type: "function", Function: ToolCallFunction{Name: "f", Arguments: "{}"}},
				{ID: "c2", Type: "function", Function: ToolCallFunction{Name: "f", Arguments: "{}"}},
			}},
		toolResponse("c1", "r1"),
		toolResponse("c2", "r2"),
		{Role: "user", Content: jsonRaw(`"q"`)},
	}
	// c1 and c2 both declared and answered: nothing to do
	repaired := repairMessages(cloneMessages(msgs))
	assert.Len(t, repaired, 4)
	assert.True(t, validateMessages(repaired))

	// now drop the c2 answer: c2 becomes dangling on the assistant and
	// tool(c2) becomes an orphan; multi-pass must converge to a legal state
	msgs2 := []Message{
		{Role: "assistant", Content: jsonRaw(`"a0"`),
			ToolCalls: []*ToolCall{
				{ID: "c1", Type: "function", Function: ToolCallFunction{Name: "f", Arguments: "{}"}},
				{ID: "c2", Type: "function", Function: ToolCallFunction{Name: "f", Arguments: "{}"}},
			}},
		toolResponse("c1", "r1"),
		{Role: "user", Content: jsonRaw(`"q"`)},
	}
	repaired2 := repairMessages(cloneMessages(msgs2))
	assert.True(t, validateMessages(repaired2))
	require.Len(t, repaired2, 3)
	assert.Equal(t, "c1", repaired2[1].ToolCallID)
}

func TestRepairDropsEmptyMessages(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: jsonRaw(`"sys"`)},
		{Role: "assistant", Content: jsonRaw(`""`)}, // emptied by L2 in practice
		{Role: "user", Content: jsonRaw(`"q"`)},
	}
	repaired := repairMessages(cloneMessages(msgs))
	require.Len(t, repaired, 2)
	assert.Equal(t, "system", repaired[0].Role)
	assert.Equal(t, "user", repaired[1].Role)
	assert.True(t, validateMessages(repaired))
}

func TestRepairKeepsSystemAndLastUser(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: jsonRaw(`""`)}, // even an empty system stays
		{Role: "user", Content: jsonRaw(`"q"`)},
	}
	repaired := repairMessages(cloneMessages(msgs))
	assert.Len(t, repaired, 2)
	assert.True(t, validateMessages(repaired))
}

func TestValidateMessages(t *testing.T) {
	// empty
	assert.False(t, validateMessages(nil))

	// unknown role: repair cannot fix it => caller rolls back
	bad := []Message{{Role: "wizard", Content: jsonRaw(`"x"`)}}
	assert.False(t, validateMessages(bad))

	// orphan tool message
	orphan := []Message{toolResponse("c1", "r")}
	assert.False(t, validateMessages(orphan))

	// dangling tool_call
	dangling := []Message{
		{Role: "user", Content: jsonRaw(`"q"`)},
		assistantWithCall("c1", "a"),
	}
	assert.False(t, validateMessages(dangling))

	// empty parts content
	emptyParts := []Message{
		{Role: "user", Content: jsonRaw(`[]`)},
	}
	assert.False(t, validateMessages(emptyParts))

	// developer role is accepted (OpenAI standard)
	dev := []Message{
		{Role: "developer", Content: jsonRaw(`"d"`)},
		{Role: "user", Content: jsonRaw(`"q"`)},
	}
	assert.True(t, validateMessages(dev))
}
