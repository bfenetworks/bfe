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

// pipeline.go: degradation pipeline orchestration (design 7.4): the
// lossless trim layers L1 (tool results) -> L1.5 (old images) -> L2
// (thinking blocks) run first; the lossy rule rewrite P2 runs only for
// balanced/aggressive modes. Every layer is followed by a re-estimation and
// the pipeline exits as soon as the estimate fits the budget.

// PipelineResult is the outcome of the pipeline for one request.
type PipelineResult struct {
	Messages     []Message // processed messages (repair already applied)
	Stage        string    // StatusTrim or StatusRewrite (pipeline exit stage)
	Estimated    int64     // token estimate of the processed messages
	Legal        bool      // false: repair/validation failed, caller must roll back
	GateFallback bool      // true: fidelity gate failed, P2 product discarded
}

type Pipeline struct {
	estimator TokenEstimator
	// rewrite is the P2 step; a function field so tests can simulate a
	// fidelity-gate failure without damaging real protected spans
	rewrite func(msgs []Message, strength string, protectedUserIdx int,
		est TokenEstimator, p EstimateParams) rewriteOutcome
}

func NewPipeline(est TokenEstimator) *Pipeline {
	return &Pipeline{estimator: est, rewrite: rewriteConversation}
}

// Run degrades the messages until the estimate fits the budget or the mode's
// capability is exhausted. The input slice is not modified: the pipeline
// works on a clone, so a rollback is simply not using the result.
func (p *Pipeline) Run(msgs []Message, mode string, params ContextParams, budget int64) PipelineResult {
	work := cloneMessages(msgs)
	ep := params.estimateParams()
	protectedUser := lastIndexOfRole(work, "user")

	// L1: truncate tool results
	trimToolResults(work, params.ToolResultMaxChars)
	if p.estimator.EstimateMessages(work, ep) <= budget {
		return p.finish(work, StatusTrim, ep)
	}

	// L1.5: drop old inline images
	trimOldImages(work, params.KeepLatestImages, protectedUser)
	if p.estimator.EstimateMessages(work, ep) <= budget {
		return p.finish(work, StatusTrim, ep)
	}

	// L2: drop thinking blocks except in the last assistant message
	trimThinkingBlocks(work, params.ThinkingPolicy)
	if p.estimator.EstimateMessages(work, ep) <= budget {
		return p.finish(work, StatusTrim, ep)
	}

	// conservative stops at the lossless layers
	if mode == ModeConservative {
		return p.finish(work, StatusTrim, ep)
	}

	// P2: rule rewrite with tombstone protection + fidelity gate
	snapshot := cloneMessages(work)
	outcome := p.rewrite(work, params.RewriteStrength, protectedUser, p.estimator, ep)
	gate := NewFidelityGate(params.ProtectedSurvivalRate)
	if !gate.Pass(outcome.survivedTokens, outcome.totalTokens) {
		// gate failed: fall back to the trim-layer product (GATE_FALLBACK)
		result := p.finish(snapshot, StatusTrim, ep)
		result.GateFallback = true
		return result
	}
	return p.finish(work, StatusRewrite, ep)
}

// finish applies the structural repair, validates the result and packages
// the pipeline result.
func (p *Pipeline) finish(msgs []Message, stage string, ep EstimateParams) PipelineResult {
	msgs = repairMessages(msgs)
	return PipelineResult{
		Messages:  msgs,
		Stage:     stage,
		Estimated: p.estimator.EstimateMessages(msgs, ep),
		Legal:     validateMessages(msgs),
	}
}
