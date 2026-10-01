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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testParams() ContextParams {
	f := &DefaultsConfFile{}
	f.setDefaults()
	return f.Convert().toParams()
}

func TestPipelineTrimLayerFits(t *testing.T) {
	est := NewHeuristicEstimator()
	p := NewPipeline(est)
	params := testParams()
	params.ToolResultMaxChars = 10

	msgs := []Message{
		{Role: "system", Content: jsonRaw(`"sys"`)},
		{Role: "assistant", ReasoningContent: "keep me", Content: jsonRaw(`"a1"`)},
		assistantWithCall("c1", "call"),
		toolResponse("c1", strings.Repeat("x", 4000)),
		{Role: "user", Content: jsonRaw(`"current question"`)},
	}

	// budget 900: L1 alone brings the estimate below the budget, so the
	// pipeline exits before L1.5/L2 (thinking stays)
	result := p.Run(msgs, ModeConservative, params, 900)
	require.True(t, result.Legal)
	assert.Equal(t, StatusTrim, result.Stage)
	assert.False(t, result.GateFallback)
	assert.True(t, result.Estimated <= 900)

	s, _ := result.Messages[3].contentString()
	assert.Equal(t, strings.Repeat("x", 10)+toolTruncatedMarker, s)
	assert.Equal(t, "keep me", result.Messages[1].ReasoningContent, "L2 never ran")

	// input slice untouched (clone safety)
	tool, _ := msgs[3].contentString()
	assert.Equal(t, strings.Repeat("x", 4000), tool)
}

func TestPipelineThinkingTrimmedWhenNeeded(t *testing.T) {
	est := NewHeuristicEstimator()
	p := NewPipeline(est)
	params := testParams()
	params.ToolResultMaxChars = 0
	params.ThinkingPolicy = ThinkingPolicyTrimAllButLast

	bigThinking := strings.Repeat("thought ", 2000) // ~16000 bytes
	msgs := []Message{
		{Role: "assistant", ReasoningContent: bigThinking, Content: jsonRaw(`"old"`)},
		{Role: "assistant", ReasoningContent: "last thinking", Content: jsonRaw(`"last"`)},
		{Role: "user", Content: jsonRaw(`"q"`)},
	}

	result := p.Run(msgs, ModeConservative, params, 900)
	require.True(t, result.Legal)
	assert.Equal(t, StatusTrim, result.Stage)
	assert.Equal(t, "", result.Messages[0].ReasoningContent, "old thinking removed")
	assert.Equal(t, "last thinking", result.Messages[1].ReasoningContent, "last assistant keeps thinking")
	assert.True(t, result.Estimated <= 900)
}

func TestPipelineConservativeStopsBeforeRewrite(t *testing.T) {
	est := NewHeuristicEstimator()
	p := NewPipeline(est)
	params := testParams()
	params.ToolResultMaxChars = 0
	params.ThinkingPolicy = ThinkingPolicyKeep

	filler := strings.Repeat("Please note that the production server runs fine today. ", 200)
	msgs := []Message{
		{Role: "assistant", Content: jsonRaw(`"` + filler + `"`)},
		{Role: "user", Content: jsonRaw(`"q"`)},
	}
	budget := int64(900)
	preEstimate := est.EstimateMessages(msgs, params.estimateParams())
	require.True(t, preEstimate > budget, "precondition: conversation exceeds the budget")

	// conservative stops at the lossless layers even though the estimate
	// still exceeds the budget
	result := p.Run(msgs, ModeConservative, params, budget)
	require.True(t, result.Legal)
	assert.Equal(t, StatusTrim, result.Stage)
	assert.True(t, result.Estimated > budget, "best effort: still above budget")

	// nothing lossless to trim => content unchanged
	s, _ := result.Messages[0].contentString()
	assert.Contains(t, s, "Please note that")
	assert.NotContains(t, s, rewriteMarker)
}

func TestPipelineRewritePath(t *testing.T) {
	est := NewHeuristicEstimator()
	p := NewPipeline(est)
	params := testParams()
	params.ToolResultMaxChars = 0
	params.ThinkingPolicy = ThinkingPolicyKeep

	// filler sized so the trim layers cannot help but the rewrite can:
	// removable "Please note that " is 17 of 57 chars per repetition
	unit := "Please note that the production server runs fine today. "
	ep := params.estimateParams()
	n := 0
	for {
		n++
		msgs := []Message{
			{Role: "assistant", Content: jsonRaw(`"` + strings.Repeat(unit, n) + `"`)},
			{Role: "user", Content: jsonRaw(`"q"`)},
		}
		work := cloneMessages(msgs)
		trimToolResults(work, params.ToolResultMaxChars)
		trimOldImages(work, params.KeepLatestImages, 1)
		trimThinkingBlocks(work, params.ThinkingPolicy)
		if est.EstimateMessages(work, ep) > 900 {
			break
		}
	}

	msgs := []Message{
		{Role: "assistant", Content: jsonRaw(`"` + strings.Repeat(unit, n) + `"`)},
		{Role: "user", Content: jsonRaw(`"q"`)},
	}
	pre := est.EstimateMessages(msgs, ep)
	require.True(t, pre > 900 && pre < 1285, "precondition: trim layers insufficient, rewrite sufficient (pre=%d)", pre)

	result := p.Run(msgs, ModeBalanced, params, 900)
	require.True(t, result.Legal)
	assert.Equal(t, StatusRewrite, result.Stage)
	assert.False(t, result.GateFallback)
	assert.True(t, result.Estimated <= 900, "rewrite brought the estimate into budget: %d", result.Estimated)

	s, _ := result.Messages[0].contentString()
	assert.Contains(t, s, rewriteMarker)
	assert.Equal(t, "q", mustContentString(result.Messages[1]), "last user message untouched")
}

func TestPipelineGateFallback(t *testing.T) {
	est := NewHeuristicEstimator()
	p := NewPipeline(est)
	// simulate a rewrite that loses protected content: the fidelity gate
	// fails and the pipeline falls back to the trim-layer product
	p.rewrite = func(msgs []Message, strength string, protectedUserIdx int,
		e TokenEstimator, ep EstimateParams) rewriteOutcome {
		msgs[0].setContentString("damaged rewrite")
		return rewriteOutcome{changed: true, survivedTokens: 1, totalTokens: 100}
	}
	params := testParams()
	params.ToolResultMaxChars = 0

	msgs := []Message{
		{Role: "assistant", ReasoningContent: strings.Repeat("t", 4000), Content: jsonRaw(`"a1"`)},
		{Role: "user", Content: jsonRaw(`"q"`)},
	}
	result := p.Run(msgs, ModeBalanced, params, 900)
	require.True(t, result.Legal)
	assert.True(t, result.GateFallback)
	assert.Equal(t, StatusTrim, result.Stage, "fallback ships the trim-layer product")

	// the damaged rewrite must not leak into the fallback product
	s, _ := result.Messages[0].contentString()
	assert.Equal(t, "a1", s)
	assert.Equal(t, strings.Repeat("t", 4000), result.Messages[0].ReasoningContent)
}

func TestPipelineRepairRollback(t *testing.T) {
	est := NewHeuristicEstimator()
	p := NewPipeline(est)
	params := testParams()

	// unknown role: repair cannot make it legal => caller rolls back
	msgs := []Message{
		{Role: "wizard", Content: jsonRaw(`"` + strings.Repeat("x", 4000) + `"`)},
		{Role: "user", Content: jsonRaw(`"q"`)},
	}
	result := p.Run(msgs, ModeConservative, params, 900)
	assert.False(t, result.Legal)
}

func TestPipelineBudgetBoundary(t *testing.T) {
	est := NewHeuristicEstimator()
	p := NewPipeline(est)
	params := testParams()
	params.ToolResultMaxChars = 0
	params.ThinkingPolicy = ThinkingPolicyKeep

	msgs := []Message{
		{Role: "assistant", Content: jsonRaw(`"` + strings.Repeat("z", 40) + `"`)}, // 10 tokens
		{Role: "user", Content: jsonRaw(`"q"`)},
	}
	ep := params.estimateParams()
	exact := est.EstimateMessages(msgs, ep)

	// estimate == budget fits: no trim happened
	fits := p.Run(msgs, ModeConservative, params, exact)
	require.True(t, fits.Legal)
	assert.Equal(t, exact, fits.Estimated)
	assert.Equal(t, strings.Repeat("z", 40), mustContentString(fits.Messages[0]))

	// estimate > budget triggers the pipeline (nothing to trim, conservative
	// best effort)
	overflow := p.Run(msgs, ModeConservative, params, exact-1)
	require.True(t, overflow.Legal)
	assert.Equal(t, StatusTrim, overflow.Stage)
}

func mustContentString(m Message) string {
	s, _ := m.contentString()
	return s
}
