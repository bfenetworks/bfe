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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_basic"
)

// intentResolver implements the lazy resolve entry injected into bfe_basic:
// it classifies on first condition evaluation and caches the result in the
// request context. Any failure degrades to unknown answers and never blocks
// the main request.
type intentResolver struct {
	conf      *ConfModAiIntent
	questions *QuestionsConf
	cache     *IntentCache
	client    DecisionClient
	breaker   *intentBreaker
	state     *ModuleAiIntentState
	latency   *latencyHistogram
}

func newIntentResolver(conf *ConfModAiIntent, questions *QuestionsConf, cache *IntentCache,
	client DecisionClient, state *ModuleAiIntentState, latency *latencyHistogram) *intentResolver {
	r := &intentResolver{
		conf:      conf,
		questions: questions,
		cache:     cache,
		client:    client,
		state:     state,
		latency:   latency,
	}
	r.breaker = newIntentBreaker(conf.Breaker.FailureThreshold,
		time.Duration(conf.Breaker.ProbeIntervalMs)*time.Millisecond,
		func() { state.BreakerOpen.Inc(1) })
	return r
}

func (r *intentResolver) Resolve(req *bfe_basic.Request) *bfe_basic.AiIntent {
	r.state.ReqTotal.Inc(1)

	intent := &bfe_basic.AiIntent{Answers: map[string]*bfe_basic.IntentAnswer{}}

	// 1. not an AI request: no classification, every intent condition misses
	aiMeta := req.GetAiBasicInfo()
	if aiMeta == nil {
		r.state.ReqUnknown.Inc(1)
		intent.Resolved = true
		return intent
	}

	qs := r.questions.Current()
	if qs == nil || len(qs.order) == 0 {
		r.state.ReqUnknown.Inc(1)
		intent.Resolved = true
		return intent
	}
	intent.QuestionsVersion = qs.Version()

	// 2. explicit intent header has the highest priority; invalid entries
	// are ignored and fall through to model classification
	headerAnswers := r.resolveExplicitHeader(req, qs)
	if len(headerAnswers) > 0 {
		r.state.ReqHeader.Inc(1)
		mergeAnswers(intent.Answers, headerAnswers)
	}

	// 3. all questions covered by the header: no decision service call
	if len(intent.Answers) == len(qs.order) {
		intent.Source = bfe_basic.IntentSourceHeader
		intent.Resolved = true
		r.state.ReqResolved.Inc(1)
		return intent
	}

	// 4. extract the last user message as classification state
	text, err := r.extractState(req, aiMeta)
	if err != nil {
		if openDebug {
			log.Logger.Debug("mod_ai_intent: extract state err: %s", err)
		}
		r.fillUnknown(intent, qs)
		r.state.ReqUnknown.Inc(1)
		intent.Resolved = true
		return intent
	}

	// 5. process-wide cache; entries hold raw model answers only
	key := cacheKey(aiMeta.ClientApiKey, qs.Version(), text)
	if cached := r.cache.Get(key); cached != nil {
		intent = cloneIntent(cached)
		mergeAnswers(intent.Answers, headerAnswers)
		intent.Source = bfe_basic.IntentSourceCache
		intent.Resolved = true
		r.state.ReqCacheHit.Inc(1)
		r.state.ReqResolved.Inc(1)
		return intent
	}

	// 6. breaker rejects while the decision service is failing
	if !r.breaker.allow() {
		r.state.ReqErr.Inc(1)
		r.fillUnknown(intent, qs)
		intent.Resolved = true
		return intent
	}

	// 7. classify via the decision service
	start := time.Now()
	answers, backendVersion, err := r.client.Classify(context.Background(), text, qs)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		r.breaker.record(false)
		if openDebug {
			log.Logger.Debug("mod_ai_intent: classify err: %s", err)
		}
		r.state.ReqErr.Inc(1)
		r.fillUnknown(intent, qs)
		intent.Resolved = true
		return intent
	}
	r.breaker.record(true)
	r.latency.observe(latencyMs)

	modelIntent := &bfe_basic.AiIntent{
		QuestionsVersion: qs.Version(),
		Answers:          answers,
		Source:           bfe_basic.IntentSourceModel,
		BackendVersion:   backendVersion,
		LatencyMs:        latencyMs,
	}
	if modelIntent.Answers == nil {
		modelIntent.Answers = map[string]*bfe_basic.IntentAnswer{}
	}
	// questions absent from the response are unknown
	for _, q := range qs.OrderedQuestions() {
		if _, ok := modelIntent.Answers[q.Name]; !ok {
			modelIntent.Answers[q.Name] = unknownAnswer(q)
		}
	}

	// 8. cache the model answers (header answers are request-specific)
	r.cache.Set(key, cloneIntent(modelIntent))

	mergeAnswers(modelIntent.Answers, headerAnswers)
	modelIntent.Resolved = true
	r.state.ReqResolved.Inc(1)
	return modelIntent
}

// resolveExplicitHeader parses the explicit intent header. Format:
// "<question>=<option>" entries separated by ";". An entry is valid only
// when the question is configured and the option exists for it.
func (r *intentResolver) resolveExplicitHeader(req *bfe_basic.Request, qs *IntentQuestions) map[string]*bfe_basic.IntentAnswer {
	if req == nil || req.HttpRequest == nil {
		return nil
	}
	raw := req.HttpRequest.Header.Get(r.conf.Basic.ExplicitIntentHeader)
	if raw == "" {
		return nil
	}

	ret := map[string]*bfe_basic.IntentAnswer{}
	for _, entry := range strings.Split(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, option, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		option = strings.TrimSpace(option)
		q, ok := qs.Question(name)
		if !ok || !q.HasOption(option) {
			if openDebug {
				log.Logger.Debug("mod_ai_intent: ignore invalid explicit intent entry %q", entry)
			}
			continue
		}
		ans := &bfe_basic.IntentAnswer{
			QType:            q.Type,
			Choice:           option,
			AnswerConfidence: 1.0,
		}
		if q.Type == QuestionTypeScore {
			ans.LevelNames = q.LevelNames
		}
		ret[name] = ans
	}
	return ret
}

// extractState reads the request body (rewindable) and extracts the last
// user message, truncated to MaxStateChars runes.
func (r *intentResolver) extractState(req *bfe_basic.Request, aiMeta *bfe_basic.AiBasicInfo) (string, error) {
	httpReq := req.HttpRequest
	if httpReq == nil {
		return "", fmt.Errorf("nil http request")
	}
	accessor, err := httpReq.GetBodyAccessor()
	if err != nil || accessor == nil {
		return "", fmt.Errorf("body accessor unavailable")
	}
	body, _ := accessor.GetBytes()
	if len(body) == 0 {
		return "", fmt.Errorf("empty body")
	}

	protocol := aiMeta.AuthStyle
	if protocol == "" || protocol == bfe_basic.AuthStyleUnknown {
		protocol = bfe_basic.DetectAuthStyle(req)
	}
	text, err := ExtractLastUserMessage(body, protocol)
	if err != nil {
		return "", err
	}
	text = truncateRunes(text, r.conf.Basic.MaxStateChars)
	if text == "" {
		return "", fmt.Errorf("empty user message")
	}
	return text, nil
}

// fillUnknown marks every configured question unknown.
func (r *intentResolver) fillUnknown(intent *bfe_basic.AiIntent, qs *IntentQuestions) {
	for _, q := range qs.OrderedQuestions() {
		if _, ok := intent.Answers[q.Name]; !ok {
			intent.Answers[q.Name] = unknownAnswer(q)
		}
	}
}

func unknownAnswer(q *IntentQuestion) *bfe_basic.IntentAnswer {
	ans := &bfe_basic.IntentAnswer{
		QType:            q.Type,
		AnswerConfidence: 0,
		Unknown:          true,
	}
	if q.Type == QuestionTypeScore {
		ans.LevelNames = q.LevelNames
	}
	return ans
}

// mergeAnswers overlays src answers onto dst (src wins, e.g. header answers
// take precedence over model/cache answers).
func mergeAnswers(dst map[string]*bfe_basic.IntentAnswer, src map[string]*bfe_basic.IntentAnswer) {
	for name, ans := range src {
		dst[name] = ans
	}
}

// cloneIntent copies an intent so that read-time gating (Unknown refresh in
// Match) on a cached shared intent never races between requests. Slices and
// maps inside answers are shared: they are never mutated after resolve.
func cloneIntent(src *bfe_basic.AiIntent) *bfe_basic.AiIntent {
	dst := &bfe_basic.AiIntent{
		QuestionsVersion: src.QuestionsVersion,
		Source:           src.Source,
		BackendVersion:   src.BackendVersion,
		LatencyMs:        src.LatencyMs,
		Resolved:         src.Resolved,
		Answers:          make(map[string]*bfe_basic.IntentAnswer, len(src.Answers)),
	}
	for name, ans := range src.Answers {
		if ans == nil {
			continue
		}
		c := *ans
		dst.Answers[name] = &c
	}
	return dst
}
