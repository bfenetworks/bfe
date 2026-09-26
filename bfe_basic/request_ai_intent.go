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

// ai intent context for request

package bfe_basic

const CtxAiIntent = "__REQ_AI_INTENT"

// intent question types
const (
	IntentQTypeChoice = "choice"
	IntentQTypeScore  = "score"
)

// intent answer sources
const (
	IntentSourceHeader = "header" // explicitly declared by client request header
	IntentSourceModel  = "model"  // classified by decision service
	IntentSourceCache  = "cache"  // served from in-process cache
)

// IntentAnswer is the answer of a single intent question. It stores raw
// probabilities only: the Unknown flag is never persisted but derived at
// read time against the current MinConfidence threshold, so threshold
// hot-update takes effect immediately without re-classification.
type IntentAnswer struct {
	QType            string             // question type: choice / score
	Choice           string             // selected option; for score, level name derived from Score
	Score            float64            // expected level index, for score questions
	LevelNames       []string           // ordered level names (low to high), for score questions
	Probabilities    map[string]float64 // option name / level name -> probability
	AnswerConfidence float64            // confidence of the reported answer
	Unknown          bool               // derived at read time, see Match()
}

// AiIntent holds intent classification results for a request.
type AiIntent struct {
	QuestionsVersion string                   // version of the questions conf in use
	Answers          map[string]*IntentAnswer // answers by question name
	Source           string                   // answer source: header / model / cache
	BackendVersion   string                   // decision service backend version
	LatencyMs        int64                    // classification latency in milliseconds
	Resolved         bool                     // true once resolve finished (even if all unknown)
}

// Match reports whether the answer of the given question is available (not
// unknown under the current threshold) and its choice is one of options.
func (ai *AiIntent) Match(question string, options ...string) bool {
	if ai == nil {
		return false
	}
	answer, ok := ai.Answers[question]
	if !ok || answer == nil {
		return false
	}

	// read-time gating: refresh Unknown against the current threshold
	answer.Unknown = answer.AnswerConfidence < AiIntentThreshold(question)
	if answer.Unknown {
		return false
	}

	for _, option := range options {
		if option != "" && answer.Choice == option {
			return true
		}
	}
	return false
}

func (r *Request) SetAiIntent(v *AiIntent) {
	r.SetContext(CtxAiIntent, v)
}

// aiIntentResolver is the lazy resolver injected by mod_ai_intent at Init.
// bfe_basic only holds the function pointer, so it does not depend on the
// module package and module registration order is not sensitive.
var aiIntentResolver func(req *Request) *AiIntent

// aiIntentThreshold returns the current effective confidence gate of a
// question (per-question override, then global, then 0 for unconfigured
// questions). Injected by mod_ai_intent at Init.
var aiIntentThreshold func(question string) float64

func SetAiIntentResolver(fn func(*Request) *AiIntent) {
	aiIntentResolver = fn
}

func SetAiIntentThreshold(fn func(question string) float64) {
	aiIntentThreshold = fn
}

// AiIntentThreshold returns the current effective confidence gate of a
// question. Without an injected threshold (module not loaded) it is 0,
// i.e. only the presence of an answer is required.
func AiIntentThreshold(question string) float64 {
	if aiIntentThreshold == nil {
		return 0
	}
	return aiIntentThreshold(question)
}

// GetAiIntent returns the intent classification result of the request,
// triggering lazy resolve on first call via the injected resolver.
func GetAiIntent(req *Request) *AiIntent {
	if req == nil {
		return nil
	}

	if val := req.GetContext(CtxAiIntent); val != nil {
		if intent, ok := val.(*AiIntent); ok && intent != nil && intent.Resolved {
			return intent
		}
	}

	if aiIntentResolver == nil {
		return nil
	}

	intent := aiIntentResolver(req)
	if intent != nil {
		req.SetContext(CtxAiIntent, intent)
	}
	return intent
}
