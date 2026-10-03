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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrimToolResults(t *testing.T) {
	msgs := []Message{
		{Role: "tool", ToolCallID: "c1", Content: jsonRaw(`"` + strings.Repeat("x", 50) + `"`)},
		{Role: "tool", ToolCallID: "c2", Content: jsonRaw(`"short"`)},
		{Role: "user", Content: jsonRaw(`"` + strings.Repeat("u", 50) + `"`)},
	}

	changed := trimToolResults(msgs, 10)
	assert.True(t, changed)

	s, ok := msgs[0].contentString()
	require.True(t, ok)
	assert.Equal(t, strings.Repeat("x", 10)+toolTruncatedMarker, s)

	// short tool result untouched
	s, ok = msgs[1].contentString()
	require.True(t, ok)
	assert.Equal(t, "short", s)

	// user message untouched (not a tool result)
	s, ok = msgs[2].contentString()
	require.True(t, ok)
	assert.Equal(t, strings.Repeat("u", 50), s)

	// disabled with 0
	assert.False(t, trimToolResults(msgs, 0))
}

func TestTrimToolResultsTypedParts(t *testing.T) {
	msgs := []Message{{
		Role: "tool",
		Content: jsonRaw(`[
			{"type":"text","text":"` + strings.Repeat("y", 30) + `"},
			{"type":"text","text":"keep"}
		]`),
	}}
	changed := trimToolResults(msgs, 5)
	assert.True(t, changed)
	parts, ok := msgs[0].contentParts()
	require.True(t, ok)
	require.Len(t, parts, 2)
	assert.Equal(t, strings.Repeat("y", 5)+toolTruncatedMarker, parts[0].Text)
	assert.Equal(t, "keep", parts[1].Text)
}

func TestTrimToolResultsRuneBased(t *testing.T) {
	// CJK truncation is rune-based, not byte-based
	msgs := []Message{
		{Role: "tool", Content: jsonRaw(`"` + strings.Repeat("文", 20) + `"`)},
	}
	changed := trimToolResults(msgs, 5)
	assert.True(t, changed)
	s, _ := msgs[0].contentString()
	assert.Equal(t, strings.Repeat("文", 5)+toolTruncatedMarker, s)
}

func inlineImagePart() ContentPart {
	return ContentPart{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,QUJD"}}
}

func TestTrimOldImages(t *testing.T) {
	img := inlineImagePart
	placeholder := ContentPart{Type: "text", Text: imageRemovedPlaceholder}

	msgs := []Message{
		{Role: "user", Content: mustPartsJSON([]ContentPart{img(), img(), {Type: "text", Text: "first"}})}, // idx 0: old + 2nd newest
		{Role: "assistant", Content: jsonRaw(`"a1"`)},
		{Role: "user", Content: mustPartsJSON([]ContentPart{img(), {Type: "text", Text: "second"}})}, // idx 2 (last user, protected)
	}

	// keepLatestImages = 1: only the newest compressible image survives
	// (idx 0 second part); the older one is replaced by the placeholder and
	// the protected message keeps its image
	changed := trimOldImages(msgs, 1, 2)
	assert.True(t, changed)

	p0, _ := msgs[0].contentParts()
	assert.Equal(t, placeholder, p0[0])
	assert.Equal(t, img(), p0[1], "newest compressible image is kept")

	p2, _ := msgs[2].contentParts()
	assert.Equal(t, img(), p2[0], "protected last user message keeps its image")
}

func TestTrimOldImagesKeepLatestZeroDisabled(t *testing.T) {
	img := inlineImagePart
	msgs := []Message{
		{Role: "user", Content: mustPartsJSON([]ContentPart{img(), {Type: "text", Text: "a"}})},
		{Role: "user", Content: jsonRaw(`"tail"`)},
	}
	assert.False(t, trimOldImages(msgs, 0, 1))
	p, _ := msgs[0].contentParts()
	assert.Equal(t, img(), p[0])
}

func TestTrimOldImagesKeepLatestN(t *testing.T) {
	img := inlineImagePart
	// three compressible images across two messages + a protected one
	msgs := []Message{
		{Role: "user", Content: mustPartsJSON([]ContentPart{img(), img()})},                     // idx 0: 2 images
		{Role: "assistant", Content: mustPartsJSON([]ContentPart{img()})},                       // idx 1: 1 image
		{Role: "user", Content: mustPartsJSON([]ContentPart{img(), {Type: "text", Text: "q"}})}, // idx 2 protected
	}
	changed := trimOldImages(msgs, 2, 2)
	assert.True(t, changed)

	// newest compressible image is kept
	p1, _ := msgs[1].contentParts()
	assert.Equal(t, img(), p1[0])

	// second newest (idx 0, second part) is kept, the oldest is replaced
	p0, _ := msgs[0].contentParts()
	assert.Equal(t, ContentPart{Type: "text", Text: imageRemovedPlaceholder}, p0[0])
	assert.Equal(t, img(), p0[1])
}

func TestTrimOldImagesKeepLatestCoversAll(t *testing.T) {
	img := inlineImagePart
	msgs := []Message{
		{Role: "user", Content: mustPartsJSON([]ContentPart{img()})},
		{Role: "user", Content: jsonRaw(`"tail"`)},
	}
	assert.False(t, trimOldImages(msgs, 5, 1), "enough budget keeps every image")
	p, _ := msgs[0].contentParts()
	assert.Equal(t, img(), p[0])
}

func TestTrimThinkingBlocks(t *testing.T) {
	msgs := []Message{
		{Role: "assistant", ReasoningContent: "old thinking", Content: jsonRaw(`"a1"`)},
		{Role: "user", Content: jsonRaw(`"q"`)},
		{Role: "assistant", ReasoningContent: "new thinking", Content: jsonRaw(`"a2"`)},
	}

	changed := trimThinkingBlocks(msgs, ThinkingPolicyTrimAllButLast)
	assert.True(t, changed)
	assert.Equal(t, "", msgs[0].ReasoningContent, "old assistant thinking removed")
	assert.Equal(t, "new thinking", msgs[2].ReasoningContent, "last assistant thinking kept")

	// policy=keep disables the layer
	msgs[0].ReasoningContent = "old thinking"
	assert.False(t, trimThinkingBlocks(msgs, ThinkingPolicyKeep))
	assert.Equal(t, "old thinking", msgs[0].ReasoningContent)
}

func TestTrimThinkingBlocksParts(t *testing.T) {
	msgs := []Message{
		{Role: "assistant", Content: jsonRaw(`[
			{"type":"thinking","thinking":"hmm"},
			{"type":"text","text":"answer"}
		]`)},
		{Role: "user", Content: jsonRaw(`"q"`)},
		{Role: "assistant", Content: jsonRaw(`"last"`)},
	}
	changed := trimThinkingBlocks(msgs, ThinkingPolicyTrimAllButLast)
	assert.True(t, changed)
	parts, ok := msgs[0].contentParts()
	require.True(t, ok)
	require.Len(t, parts, 1)
	assert.Equal(t, "text", parts[0].Type)
	assert.Equal(t, "answer", parts[0].Text)
}

func TestTrimThinkingBlocksNoAssistant(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: jsonRaw(`"q"`)},
	}
	assert.False(t, trimThinkingBlocks(msgs, ThinkingPolicyTrimAllButLast))
}

// mustPartsJSON marshals parts for a Content field (test helper).
func mustPartsJSON(parts []ContentPart) json.RawMessage {
	b, err := json.Marshal(parts)
	if err != nil {
		panic(err)
	}
	return b
}
