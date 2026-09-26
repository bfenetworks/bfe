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

package bfe_basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testIntent() *AiIntent {
	return &AiIntent{
		QuestionsVersion: "v1",
		Source:           IntentSourceModel,
		Answers: map[string]*IntentAnswer{
			"task_type": {QType: IntentQTypeChoice, Choice: "coding", AnswerConfidence: 0.8},
			"low":       {QType: IntentQTypeChoice, Choice: "coding", AnswerConfidence: 0.5},
			"strict":    {QType: IntentQTypeChoice, Choice: "high", AnswerConfidence: 0.85},
			"nilanswer": nil,
		},
		Resolved: true,
	}
}

func TestAiIntentMatch(t *testing.T) {
	defer SetAiIntentThreshold(nil)

	SetAiIntentThreshold(func(question string) float64 {
		if question == "strict" {
			return 0.9
		}
		return 0.6
	})

	intent := testIntent()
	assert.True(t, intent.Match("task_type", "doc_writing", "coding"))
	assert.False(t, intent.Match("task_type", "doc_writing"))
	assert.False(t, intent.Match("task_type"))
	assert.False(t, intent.Match("task_type", ""))

	// read-time gating: below the current threshold the answer is unknown
	assert.False(t, intent.Match("low", "coding"))
	assert.True(t, intent.Answers["low"].Unknown, "unknown derived at read time")
	assert.False(t, intent.Match("strict", "high"), "0.85 below the strict gate 0.9")
	assert.True(t, intent.Answers["strict"].Unknown)

	// missing / nil answers never match
	assert.False(t, intent.Match("missing", "coding"))
	assert.False(t, intent.Match("nilanswer", "coding"))

	// threshold hot-update takes effect on the same raw answers, without
	// any re-classification
	SetAiIntentThreshold(func(question string) float64 { return 0.4 })
	assert.True(t, intent.Match("low", "coding"))
	assert.False(t, intent.Answers["low"].Unknown)
	assert.True(t, intent.Match("strict", "high"))

	// no injected threshold: gate is 0, any answer with confidence matches
	SetAiIntentThreshold(nil)
	assert.True(t, intent.Match("low", "coding"))

	var nilIntent *AiIntent
	assert.False(t, nilIntent.Match("task_type", "coding"))
}

func TestGetAiIntentLazyResolve(t *testing.T) {
	defer SetAiIntentResolver(nil)
	SetAiIntentResolver(nil)

	// no resolver injected (module not loaded): nil
	req := &Request{Context: make(map[interface{}]interface{})}
	assert.Nil(t, GetAiIntent(req))
	assert.Nil(t, GetAiIntent(nil))

	// resolver injected: called once, result written back to the context
	var calls int
	SetAiIntentResolver(func(req *Request) *AiIntent {
		calls++
		return testIntent()
	})
	intent := GetAiIntent(req)
	require.NotNil(t, intent)
	assert.Equal(t, 1, calls)
	again := GetAiIntent(req)
	assert.Same(t, intent, again)
	assert.Equal(t, 1, calls, "resolved intent must be reused from the request context")

	// an unresolved context value does not short-circuit resolving
	req2 := &Request{Context: make(map[interface{}]interface{})}
	req2.SetContext(CtxAiIntent, &AiIntent{Resolved: false})
	GetAiIntent(req2)
	assert.Equal(t, 2, calls)

	// resolver returning nil: stays nil, nothing cached
	SetAiIntentResolver(func(req *Request) *AiIntent { return nil })
	req3 := &Request{Context: make(map[interface{}]interface{})}
	assert.Nil(t, GetAiIntent(req3))
	assert.Nil(t, req3.GetContext(CtxAiIntent))
}

func TestSetGetAiIntent(t *testing.T) {
	defer SetAiIntentResolver(nil)
	SetAiIntentResolver(nil)

	req := &Request{Context: make(map[interface{}]interface{})}
	intent := testIntent()
	req.SetAiIntent(intent)
	assert.Same(t, intent, GetAiIntent(req))
}
