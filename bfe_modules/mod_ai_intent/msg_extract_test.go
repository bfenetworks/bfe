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

package mod_ai_intent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bfenetworks/bfe/bfe_basic"
)

func TestExtractOpenAIStringContent(t *testing.T) {
	body := `{"model":"gpt-4","messages":[{"role":"system","content":"be nice"},{"role":"user","content":"hello world"}]}`
	text, err := ExtractLastUserMessage([]byte(body), bfe_basic.AuthStyleOpenAI)
	require.NoError(t, err)
	assert.Equal(t, "hello world", text)
}

func TestExtractOpenAIContentParts(t *testing.T) {
	// anthropic-style content array with text segments
	body := `{"messages":[{"role":"assistant","content":"prev"},{"role":"user","content":[{"type":"text","text":"part1 "},{"type":"image_url","image_url":{"url":"x"}},{"type":"text","text":"part2"}]}]}`
	text, err := ExtractLastUserMessage([]byte(body), bfe_basic.AuthStyleAnthropic)
	require.NoError(t, err)
	assert.Equal(t, "part1 part2", text)
}

func TestExtractGemini(t *testing.T) {
	body := `{"contents":[{"role":"user","parts":[{"text":"first "},{"text":"second"}]},{"role":"model","parts":[{"text":"reply"}]},{"role":"user","parts":[{"text":"latest question"}]}]}`
	text, err := ExtractLastUserMessage([]byte(body), bfe_basic.AuthStyleGemini)
	require.NoError(t, err)
	assert.Equal(t, "latest question", text)
}

func TestExtractLastNotUser(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"answer"}]}`
	_, err := ExtractLastUserMessage([]byte(body), bfe_basic.AuthStyleOpenAI)
	assert.Error(t, err)
}

func TestExtractInvalidJSON(t *testing.T) {
	_, err := ExtractLastUserMessage([]byte("{not json"), bfe_basic.AuthStyleOpenAI)
	assert.Error(t, err)

	_, err = ExtractLastUserMessage(nil, bfe_basic.AuthStyleOpenAI)
	assert.Error(t, err)
}

func TestExtractUnknownProtocolTolerant(t *testing.T) {
	// unrecognized protocol values fall back to openai format parsing
	body := `{"messages":[{"role":"user","content":"hi"}]}`
	text, err := ExtractLastUserMessage([]byte(body), bfe_basic.AuthStyleUnknown)
	require.NoError(t, err)
	assert.Equal(t, "hi", text)

	text, err = ExtractLastUserMessage([]byte(body), "whatever")
	require.NoError(t, err)
	assert.Equal(t, "hi", text)
}

func TestExtractEmptyContent(t *testing.T) {
	body := `{"messages":[{"role":"user","content":""}]}`
	_, err := ExtractLastUserMessage([]byte(body), bfe_basic.AuthStyleOpenAI)
	assert.Error(t, err)

	body = `{"messages":[{"role":"user","content":[{"type":"image_url"}]}]}`
	_, err = ExtractLastUserMessage([]byte(body), bfe_basic.AuthStyleOpenAI)
	assert.Error(t, err)
}

func TestTruncateRunes(t *testing.T) {
	s := strings.Repeat("汉", 10)
	assert.Equal(t, strings.Repeat("汉", 5), truncateRunes(s, 5))
	assert.Equal(t, s, truncateRunes(s, 10))
	assert.Equal(t, s, truncateRunes(s, 20))
	assert.Equal(t, "", truncateRunes(s, 0))
	assert.Equal(t, "abc", truncateRunes("abc", 5))
}
