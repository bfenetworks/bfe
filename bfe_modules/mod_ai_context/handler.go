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
	"time"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

// annotationHeader is set on responses of actually compressed requests
// (design 8)
const annotationHeader = "x-ai-context-compression"

// defaultContextWindow is the fallback when neither a model table entry nor
// a model-name heuristic can determine the context window (128k).
const defaultContextWindow = 128000

// contextCompressHandler is the HandleAfterAITargetModel filter: it runs
// after the target model is resolved and before the request is forwarded,
// and compresses the OpenAI chat completions messages of oversized requests.
// The callback fires per cluster attempt; AiBasicInfo.ContextCompressStatus
// is the per-request idempotency guard. The whole handler is fail-open: any
// problem only records a status and lets the original request through.
func (m *ModuleAiContext) contextCompressHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	m.state.Inc("CTX_TOTAL", 1)

	basic := req.GetAiBasicInfo()
	if basic == nil {
		// not an AI flow; the callback never fires without it, guard anyway
		return bfe_module.BfeHandlerGoOn, nil
	}

	// 1. idempotency guard: the callback fires on every cluster attempt
	//    (key rotation / fallback); compression must run at most once.
	if basic.ContextCompressStatus != "" {
		return bfe_module.BfeHandlerGoOn, nil
	}

	// 2. rule match: no product rule or mode=off
	rule, params, ok := m.ruleTable.Match(req)
	if !ok || rule.Mode == ModeOff {
		basic.ContextCompressStatus = StatusSkipNoRule
		m.state.Inc("CTX_SKIP_NO_RULE", 1)
		return bfe_module.BfeHandlerGoOn, nil
	}

	// 3. protocol: phase 1 only handles OpenAI chat completions; Anthropic /
	//    Gemini style bodies are identified and skipped (their parse would
	//    fail anyway, but skipping early keeps the skip reason accurate).
	if !isOpenAIChat(basic) {
		basic.ContextCompressStatus = StatusSkipProtocol
		m.state.Inc("CTX_SKIP_PROTOCOL", 1)
		return bfe_module.BfeHandlerGoOn, nil
	}

	// 4. body read: rewrite OutRequest (the forwarded request), never the
	//    original HttpRequest, so access logging and billing still see the
	//    original request. Not fully buffered => skip (fail-open).
	outReq := req.OutRequest
	if outReq == nil {
		basic.ContextCompressStatus = StatusSkipBodyIncomplete
		m.state.Inc("CTX_SKIP_BODY_INCOMPLETE", 1)
		return bfe_module.BfeHandlerGoOn, nil
	}
	bodyAccessor, err := outReq.GetBodyAccessor()
	if err != nil || bodyAccessor == nil {
		basic.ContextCompressStatus = StatusSkipBodyIncomplete
		m.state.Inc("CTX_SKIP_BODY_INCOMPLETE", 1)
		return bfe_module.BfeHandlerGoOn, nil
	}
	body, all := bodyAccessor.GetBytes()
	if !all {
		if openDebug {
			log.Logger.Debug("%s: request body not fully buffered, skip", m.name)
		}
		basic.ContextCompressStatus = StatusSkipBodyIncomplete
		m.state.Inc("CTX_SKIP_BODY_INCOMPLETE", 1)
		return bfe_module.BfeHandlerGoOn, nil
	}

	// 5. parse messages (string and typed-parts content are both supported)
	msgs, err := parseMessages(body)
	if err != nil {
		if openDebug {
			log.Logger.Debug("%s: parse messages err[%v]", m.name, err)
		}
		basic.ContextCompressStatus = StatusSkipParseErr
		m.state.Inc("CTX_SKIP_PARSE_ERR", 1)
		return bfe_module.BfeHandlerGoOn, nil
	}

	// 6. budget: window (model table later; name heuristics now) reduced by
	//    the output reserve. The original body stays in the accessor until
	//    the very last step, which is the rollback guarantee.
	window, defaulted := modelContextWindow(basic.TargetModel)
	if defaulted {
		m.state.Inc("CTX_WINDOW_DEFAULTED", 1)
	}
	if params.MaxContextTokens > 0 && params.MaxContextTokens < window {
		window = params.MaxContextTokens
	}
	reserve := params.ReserveTokens
	if reserve <= 0 {
		reserve = clamp(window*15/100, 256, 16000)
	}
	budget := window - reserve
	if budget <= 0 {
		// misconfiguration (maxContextTokens smaller than the reserve);
		// nothing sane to compress towards, fail-open
		if openDebug {
			log.Logger.Debug("%s: budget[%d] <= 0, window[%d] reserve[%d], skip",
				m.name, budget, window, reserve)
		}
		basic.ContextCompressStatus = StatusSkipNoRule
		m.state.Inc("CTX_SKIP_NO_RULE", 1)
		return bfe_module.BfeHandlerGoOn, nil
	}

	// 7. estimate and proactive trigger threshold
	estimated := m.estimator.EstimateMessages(msgs, params.estimateParams())
	if estimated <= int64(float64(budget)*params.TriggerRatio) {
		basic.ContextCompressStatus = StatusSkipUnderThreshold
		m.state.Inc("CTX_SKIP_UNDER_THRESHOLD", 1)
		return bfe_module.BfeHandlerGoOn, nil
	}

	// 8-10. degradation pipeline (L1 -> L1.5 -> L2 -> P2, re-estimating)
	m.state.Inc("CTX_TRIGGERED", 1)
	switch rule.Mode {
	case ModeConservative:
		m.state.Inc("CTX_TRIGGERED_CONSERVATIVE", 1)
	case ModeBalanced:
		m.state.Inc("CTX_TRIGGERED_BALANCED", 1)
	case ModeAggressive:
		m.state.Inc("CTX_TRIGGERED_AGGRESSIVE", 1)
	}
	start := time.Now()
	result := m.pipeline.Run(msgs, rule.Mode, params, budget)
	m.state.Inc("CTX_LATENCY_MS", int(time.Since(start).Milliseconds()))
	if result.GateFallback {
		m.state.Inc("CTX_GATE_FALLBACK", 1)
	}

	if !result.Legal {
		// repair could not make the messages legal: roll back as a whole;
		// the original body was never touched (fail-open)
		basic.ContextCompressStatus = StatusRepairRollback
		m.state.Inc("CTX_REPAIR_ROLLBACK", 1)
		return bfe_module.BfeHandlerGoOn, nil
	}

	// 11. re-serialize the whole body (messages changed length, sjson path
	//     rewrites do not apply) and fix the forwarding headers: without
	//     ContentLength=-1 and the Content-Length header removal the body
	//     would be truncated on the wire (precedent: reverseproxy model
	//     rewrite).
	//
	//     Note: OutRequest is a shallow copy of HttpRequest, so its body
	//     buffer is SHARED with the original request and the header maps are
	//     the same object. SetBytes above therefore already replaced the
	//     buffered bytes that a fallback retry replays. Reset the original
	//     request's ContentLength as well (same compensation the model
	//     rewrite applies) so the retried attempt re-sends the compressed
	//     body consistently instead of failing on a stale length; the
	//     idempotency guard prevents a second compression pass.
	newBody, err := replaceMessages(body, result.Messages)
	if err != nil {
		basic.ContextCompressStatus = StatusRepairRollback
		m.state.Inc("CTX_REPAIR_ROLLBACK", 1)
		return bfe_module.BfeHandlerGoOn, nil
	}
	bodyAccessor.SetBytes(newBody, false)
	outReq.ContentLength = -1
	outReq.Header.Del("Content-Length")
	if req.HttpRequest != nil && req.HttpRequest.ContentLength >= 0 {
		req.HttpRequest.ContentLength = -1
		req.HttpRequest.Header.Del("Content-Length")
	}

	// 12. record the outcome in AiBasicInfo (access log + response header)
	basic.ContextCompressStatus = result.Stage
	basic.ContextTokensBefore = estimated
	basic.ContextTokensAfter = result.Estimated
	basic.ContextCompressMode = rule.Mode
	if result.Stage == StatusRewrite {
		m.state.Inc("CTX_DONE_REWRITE", 1)
	} else {
		m.state.Inc("CTX_DONE_TRIM", 1)
	}

	if openDebug {
		log.Logger.Debug("%s: compressed mode[%s] stage[%s] tokens[%d->%d] budget[%d]",
			m.name, rule.Mode, result.Stage, estimated, result.Estimated, budget)
	}

	return bfe_module.BfeHandlerGoOn, nil
}

// responseAnnotationHandler is the HandleReadResponse filter: responses of
// actually compressed requests carry an x-ai-context-compression header so
// clients and operators can verify the compression took effect (design 8).
// Streaming and non-streaming semantics are untouched.
func (m *ModuleAiContext) responseAnnotationHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	if res == nil || res.Header == nil {
		return bfe_module.BfeHandlerGoOn
	}
	basic := req.GetAiBasicInfo()
	if basic == nil || !isCompressDone(basic.ContextCompressStatus) {
		return bfe_module.BfeHandlerGoOn
	}
	res.Header.Set(annotationHeader, fmt.Sprintf("tokens=%d->%d; mode=%s",
		basic.ContextTokensBefore, basic.ContextTokensAfter, basic.ContextCompressMode))
	return bfe_module.BfeHandlerGoOn
}

// isOpenAIChat reports whether the request is an OpenAI chat completions
// request. AuthStyle is set by the auth detection (GetApiKey) earlier in the
// chain; unknown/empty styles are tolerated as OpenAI because the message
// parse in the next step is the real gate (fail-open).
func isOpenAIChat(basic *bfe_basic.AiBasicInfo) bool {
	if basic.Mode != bfe_basic.ModeChat {
		return false
	}
	switch basic.AuthStyle {
	case "", bfe_basic.AuthStyleOpenAI, bfe_basic.AuthStyleUnknown:
		return true
	default:
		return false
	}
}

// modelContextWindow resolves the context window of the target model.
// Priority (design 7.1-6): model table context_window (control plane, WBS
// #7, hooked in later) -> model-name heuristic -> 128k default. defaulted is
// true when the 128k fallback was used (CTX_WINDOW_DEFAULTED).
func modelContextWindow(model string) (window int64, defaulted bool) {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "gemini"):
		return 1000000, false
	case strings.Contains(m, "claude"):
		return 200000, false
	case strings.Contains(m, "codex"):
		return 400000, false
	default:
		return defaultContextWindow, true
	}
}

func clamp(v, lo, hi int64) int64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
