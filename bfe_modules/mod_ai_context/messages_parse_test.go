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
	"encoding/json"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jsonRaw(s string) json.RawMessage {
	return json.RawMessage(s)
}

func TestParseMessagesStringContent(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"system","content":"sys"},{"role":"user","content":"hello"}]}`)
	msgs, err := parseMessages(body)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "system", msgs[0].Role)
	s, ok := msgs[0].contentString()
	require.True(t, ok)
	assert.Equal(t, "sys", s)
	_, ok = msgs[0].contentParts()
	assert.False(t, ok)
}

func TestParseMessagesTypedParts(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[
		{"type":"text","text":"look at this"},
		{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJD","detail":"low"}}
	]}]}`)
	msgs, err := parseMessages(body)
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	parts, ok := msgs[0].contentParts()
	require.True(t, ok)
	require.Len(t, parts, 2)
	assert.Equal(t, "text", parts[0].Type)
	assert.Equal(t, "look at this", parts[0].Text)
	assert.Equal(t, "image_url", parts[1].Type)
	require.NotNil(t, parts[1].ImageURL)
	assert.True(t, parts[1].isInlineImage())
	assert.Equal(t, "low", parts[1].ImageURL.Detail)

	// unknown part fields round-trip
	body2 := []byte(`{"messages":[{"role":"user","content":[{"type":"file","file":{"id":"f1"}}]}]}`)
	msgs2, err := parseMessages(body2)
	require.NoError(t, err)
	out, err := json.Marshal(msgs2)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"file":{"id":"f1"}`)
}

func TestParseMessagesToolCalls(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"call_1","name":"get","content":"result"}
	]}`)
	msgs, err := parseMessages(body)
	require.NoError(t, err)
	require.Len(t, msgs, 2)

	require.Len(t, msgs[0].ToolCalls, 1)
	assert.Equal(t, "call_1", msgs[0].ToolCalls[0].ID)
	assert.Equal(t, "get", msgs[0].ToolCalls[0].Function.Name)
	s, ok := msgs[0].contentString()
	assert.True(t, ok) // null content reads as empty string
	assert.Equal(t, "", s)

	assert.Equal(t, "tool", msgs[1].Role)
	assert.Equal(t, "call_1", msgs[1].ToolCallID)
	assert.Equal(t, "get", msgs[1].Name)
	s, ok = msgs[1].contentString()
	require.True(t, ok)
	assert.Equal(t, "result", s)
}

func TestParseMessagesThinking(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","reasoning_content":"let me think","content":"answer"}]}`)
	msgs, err := parseMessages(body)
	require.NoError(t, err)
	assert.Equal(t, "let me think", msgs[0].ReasoningContent)

	// anthropic-style thinking part
	body = []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"answer"}]}]}`)
	msgs, err = parseMessages(body)
	require.NoError(t, err)
	parts, ok := msgs[0].contentParts()
	require.True(t, ok)
	require.Len(t, parts, 2)
	assert.True(t, isThinkingPart(&parts[0]))
	assert.False(t, isThinkingPart(&parts[1]))
}

func TestParseMessagesExtraFieldRoundTrip(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":"a","refusal":"no","custom":{"k":1}}]}`)
	msgs, err := parseMessages(body)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.JSONEq(t, `"no"`, string(msgs[0].Extra["refusal"]))

	out, err := json.Marshal(msgs)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"refusal":"no"`)
	assert.Contains(t, string(out), `"custom":{"k":1}`)
	assert.Contains(t, string(out), `"content":"a"`)
}

func TestParseMessagesErrors(t *testing.T) {
	_, err := parseMessages([]byte(""))
	assert.Error(t, err)

	_, err = parseMessages([]byte("not-json"))
	assert.Error(t, err)

	_, err = parseMessages([]byte(`{"model":"m"}`))
	assert.Error(t, err)

	_, err = parseMessages([]byte(`{"messages":[]}`))
	assert.Error(t, err)

	_, err = parseMessages([]byte(`{"messages":"not-array"}`))
	assert.Error(t, err)
}

func TestReplaceMessagesPreservesTopLevelFields(t *testing.T) {
	body := []byte(`{"model":"m","stream":true,"temperature":0.7,"tools":[{"type":"function"}],"messages":[{"role":"user","content":"q"}]}`)
	msgs, err := parseMessages(body)
	require.NoError(t, err)
	msgs[0].setContentString("compressed-question")

	newBody, err := replaceMessages(body, msgs)
	require.NoError(t, err)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(newBody, &top))
	assert.Contains(t, top, "model")
	assert.Contains(t, top, "stream")
	assert.Contains(t, top, "temperature")
	assert.Contains(t, top, "tools")

	var outMsgs []Message
	require.NoError(t, json.Unmarshal(top["messages"], &outMsgs))
	require.Len(t, outMsgs, 1)
	s, _ := outMsgs[0].contentString()
	assert.Equal(t, "compressed-question", s)

	var model string
	require.NoError(t, json.Unmarshal(top["model"], &model))
	assert.Equal(t, "m", model)
}

func TestCloneMessagesIndependent(t *testing.T) {
	orig := []Message{{
		Role:    "user",
		Content: jsonRaw(`"abc"`),
		Extra:   map[string]json.RawMessage{"k": jsonRaw(`1`)},
		ToolCalls: []*ToolCall{{
			ID: "c1", Type: "function",
			Function: ToolCallFunction{Name: "f", Arguments: "{}"},
		}},
	}}
	cloned := cloneMessages(orig)

	cloned[0].setContentString("changed")
	cloned[0].Extra["k"] = jsonRaw(`2`)
	cloned[0].ToolCalls[0].ID = "changed"

	s, _ := orig[0].contentString()
	assert.Equal(t, "abc", s)
	assert.Equal(t, `1`, string(orig[0].Extra["k"]))
	assert.Equal(t, "c1", orig[0].ToolCalls[0].ID)
}

func TestSetContentPartsEmptyBecomesNull(t *testing.T) {
	m := Message{Role: "assistant"}
	m.setContentParts(nil)
	s, ok := m.contentString()
	assert.True(t, ok)
	assert.Equal(t, "", s)
}

func TestLastIndexOfRole(t *testing.T) {
	msgs := []Message{
		{Role: "user"}, {Role: "assistant"}, {Role: "tool"}, {Role: "user"},
	}
	assert.Equal(t, 3, lastIndexOfRole(msgs, "user"))
	assert.Equal(t, 1, lastIndexOfRole(msgs, "assistant"))
	assert.Equal(t, -1, lastIndexOfRole(msgs, "system"))
}

func TestMessageText(t *testing.T) {
	m := Message{Content: jsonRaw(`"plain"`)}
	assert.Equal(t, "plain", messageText(&m))

	m = Message{Content: jsonRaw(`[{"type":"text","text":"a"},{"type":"thinking","thinking":"b"}]`)}
	assert.Equal(t, "ab", messageText(&m))
}
