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

func TestProtectRestoreRoundTrip(t *testing.T) {
	est := NewHeuristicEstimator()
	p := EstimateParams{CharsPerToken: 4, ImageTokenEstimate: 100}

	text := "see `inline code` and ```\ncode block 1.2.3\n``` and https://example.com/a?b=1 " +
		"and /usr/local/bin and v1.2.3 and {\"key\": 42} end"
	masked, spans := protectText(text, est, p)
	require.NotEmpty(t, spans)

	// originals are hidden behind sentinels
	assert.NotContains(t, masked, "inline code")
	assert.NotContains(t, masked, "https://example.com/a?b=1")
	assert.NotContains(t, masked, "v1.2.3")
	assert.NotContains(t, masked, "/usr/local/bin")

	restored, survived := restoreText(masked, spans)
	assert.Equal(t, text, restored, "protected fragments must round-trip exactly")
	assert.Equal(t, totalSpanTokens(spans), survived)
}

func TestRewriteRulesEffect(t *testing.T) {
	est := NewHeuristicEstimator()
	p := EstimateParams{CharsPerToken: 4, ImageTokenEstimate: 100}

	rewritten, survived, total := rewriteText("Please note that the answer is 42.", RewriteStrengthLite, est, p)
	assert.Equal(t, "the answer is 42.", rewritten)
	assert.True(t, total > 0, "the number literal was protected")
	assert.Equal(t, total, survived, "all protected spans survive")

	// Chinese politeness rule
	rewritten, _, _ = rewriteText("请注意，这个函数已经失效。", RewriteStrengthLite, est, p)
	assert.Equal(t, "这个函数已经失效。", rewritten)

	// repeated whitespace collapse
	rewritten, _, _ = rewriteText("a    b\t\tc", RewriteStrengthLite, est, p)
	assert.Equal(t, "a b c", rewritten)
}

func TestRewritePreservesCodeAndUrls(t *testing.T) {
	est := NewHeuristicEstimator()
	p := EstimateParams{CharsPerToken: 4, ImageTokenEstimate: 100}

	code := "curl https://api.example.com/v1/some-endpoint"
	text := "Please note that you can run `" + code + "` directly. Please note that it works."
	rewritten, survived, total := rewriteText(text, RewriteStrengthLite, est, p)

	assert.Contains(t, rewritten, "`"+code+"`", "inline code stays byte-identical")
	assert.NotContains(t, rewritten, "Please note that", "all unprotected filler is rewritten")
	assert.Equal(t, total, survived)
}

func TestRewriteFullStrength(t *testing.T) {
	est := NewHeuristicEstimator()
	p := EstimateParams{CharsPerToken: 4, ImageTokenEstimate: 100}

	lite, _, _ := rewriteText("In order to build it, run make.", RewriteStrengthLite, est, p)
	assert.Contains(t, lite, "In order to")

	full, _, _ := rewriteText("In order to build it, run make.", RewriteStrengthFull, est, p)
	assert.Contains(t, full, "To build it")
}

func TestRewriteConversation(t *testing.T) {
	est := NewHeuristicEstimator()
	p := EstimateParams{CharsPerToken: 4, ImageTokenEstimate: 100}

	msgs := []Message{
		{Role: "system", Content: jsonRaw(`"Please note that system prompt"`)},
		{Role: "assistant", Content: jsonRaw(`"Please note that old answer"`)},
		{Role: "user", Content: jsonRaw(`"Please note that current question"`)}, // idx 2, protected
	}
	outcome := rewriteConversation(msgs, RewriteStrengthLite, 2, est, p)
	assert.True(t, outcome.changed)

	sys, _ := msgs[0].contentString()
	assert.Equal(t, "Please note that system prompt", sys, "system messages are frozen")

	asst, _ := msgs[1].contentString()
	assert.Contains(t, asst, "old answer")
	assert.Contains(t, asst, rewriteMarker, "rewritten messages carry the marker")

	user, _ := msgs[2].contentString()
	assert.Equal(t, "Please note that current question", user, "last user message untouched")

	// idempotency: a second pass skips marked messages
	outcome2 := rewriteConversation(msgs, RewriteStrengthLite, 2, est, p)
	assert.False(t, outcome2.changed)
}

func TestRewriteConversationTypedParts(t *testing.T) {
	est := NewHeuristicEstimator()
	p := EstimateParams{CharsPerToken: 4, ImageTokenEstimate: 100}

	msgs := []Message{{
		Role:    "assistant",
		Content: jsonRaw(`[{"type":"text","text":"Please note that part one"},{"type":"text","text":"请注意，第二部分"}]`),
	}}
	outcome := rewriteConversation(msgs, RewriteStrengthLite, -1, est, p)
	assert.True(t, outcome.changed)

	parts, ok := msgs[0].contentParts()
	require.True(t, ok)
	require.Len(t, parts, 3)
	assert.Equal(t, "part one", parts[0].Text)
	assert.Equal(t, "第二部分", parts[1].Text)
	assert.Equal(t, rewriteMarker, parts[2].Text)
}

func TestFidelityGate(t *testing.T) {
	gate := NewFidelityGate(0.95)

	assert.True(t, gate.Pass(95, 100))
	assert.True(t, gate.Pass(100, 100))
	assert.False(t, gate.Pass(94, 100))
	assert.True(t, gate.Pass(0, 0), "no protected content always passes")
	assert.True(t, gate.Pass(0, 0) && gate.Pass(5, 5))
}
