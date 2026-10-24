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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeuristicEstimatorText(t *testing.T) {
	est := NewHeuristicEstimator()
	p := EstimateParams{CharsPerToken: 4, ImageTokenEstimate: 100}

	assert.Equal(t, int64(0), est.EstimateText("", p))
	assert.Equal(t, int64(1), est.EstimateText("abcd", p))
	assert.Equal(t, int64(2), est.EstimateText("abcde", p)) // ceil division

	// CJK text: 3 bytes per char, denser coefficient applies per call
	cjk := "你好世界"
	assert.Equal(t, int64(3), est.EstimateText(cjk, EstimateParams{CharsPerToken: 4, ImageTokenEstimate: 100}))
	assert.Equal(t, int64(4), est.EstimateText(cjk, EstimateParams{CharsPerToken: 3, ImageTokenEstimate: 100}))

	// params flow through on every call (Init does not bake them in)
	assert.Equal(t, int64(2), est.EstimateText("abcdefgh", p))
	assert.Equal(t, int64(8), est.EstimateText("abcdefgh", EstimateParams{CharsPerToken: 1, ImageTokenEstimate: 100}))
}

func TestHeuristicEstimatorMessages(t *testing.T) {
	est := NewHeuristicEstimator()
	p := EstimateParams{CharsPerToken: 4, ImageTokenEstimate: 100}

	msgs := []Message{
		{Role: "system", Content: jsonRaw(`"sys"`)},
		{Role: "user", Content: jsonRaw(`"` + strings.Repeat("a", 100) + `"`)},
	}
	total := est.EstimateMessages(msgs, p)
	// 100 chars / 4 = 25 tokens for the user text plus small role overhead
	assert.True(t, total >= 25, "expected >= 25 tokens, got %d", total)

	// the same text with a denser coefficient yields more tokens
	dense := est.EstimateMessages(msgs, EstimateParams{CharsPerToken: 2, ImageTokenEstimate: 100})
	assert.True(t, dense > total)
}

func TestHeuristicEstimatorInlineImages(t *testing.T) {
	est := NewHeuristicEstimator()
	p := EstimateParams{CharsPerToken: 4, ImageTokenEstimate: 100}

	inline := jsonRaw(`[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"data:image/png;base64,` +
		strings.Repeat("QUJD", 1000) + `"}}]`)
	msgs := []Message{
		{Role: "user", Content: inline},
	}
	total := est.EstimateMessages(msgs, p)
	// inline base64 (12k+ bytes) must NOT be counted by raw bytes; it counts
	// ImageTokenEstimate plus the small text part
	assert.True(t, total < 100+30, "inline image should count as ~ImageTokenEstimate, got %d", total)
	assert.True(t, total >= 100, "expected the image placeholder estimate, got %d", total)

	// a remote image url is ordinary text
	remote := jsonRaw(`[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]`)
	msgs[0].Content = remote
	remoteTotal := est.EstimateMessages(msgs, p)
	assert.True(t, remoteTotal < 50, "remote image url is small text, got %d", remoteTotal)
	assert.True(t, remoteTotal > 0)
}

func TestHeuristicEstimatorToolCalls(t *testing.T) {
	est := NewHeuristicEstimator()
	p := EstimateParams{CharsPerToken: 4, ImageTokenEstimate: 100}

	base := []Message{{Role: "user", Content: jsonRaw(`"q"`)}}
	baseTokens := est.EstimateMessages(base, p)

	withTools := append(cloneMessages(base), Message{
		Role: "assistant",
		ToolCalls: []*ToolCall{{
			ID:   "call_1",
			Type: "function",
			Function: ToolCallFunction{
				Name:      "get_weather",
				Arguments: fmt.Sprintf(`{"city":"%s"}`, strings.Repeat("x", 100)),
			},
		}},
	})
	toolTokens := est.EstimateMessages(withTools, p)
	assert.True(t, toolTokens > baseTokens, "tool_calls participate in the estimate")
}

func TestModelContextWindow(t *testing.T) {
	window, defaulted := modelContextWindow("claude-3-5-sonnet")
	assert.Equal(t, int64(200000), window)
	assert.False(t, defaulted)

	window, defaulted = modelContextWindow("gemini-1.5-pro")
	assert.Equal(t, int64(1000000), window)
	assert.False(t, defaulted)

	window, defaulted = modelContextWindow("gpt-5-codex")
	assert.Equal(t, int64(400000), window)
	assert.False(t, defaulted)

	window, defaulted = modelContextWindow("some-random-model")
	assert.Equal(t, int64(128000), window)
	assert.True(t, defaulted)

	window, defaulted = modelContextWindow("")
	assert.Equal(t, int64(128000), window)
	assert.True(t, defaulted)
}

func TestClamp(t *testing.T) {
	assert.Equal(t, int64(256), clamp(1, 256, 16000))
	assert.Equal(t, int64(16000), clamp(99999, 256, 16000))
	assert.Equal(t, int64(1000), clamp(1000, 256, 16000))
}
