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

package condition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func TestBuildReqAiIntentIn(t *testing.T) {
	// two args: third (confidence threshold) omitted
	c, err := Build(`req_ai_intent_in("task_type", "coding|test_writing")`)
	require.NoError(t, err)
	require.NotNil(t, c)

	// three args: explicit threshold
	c, err = Build(`req_ai_intent_in("task_type", "coding", 0.9)`)
	require.NoError(t, err)
	require.NotNil(t, c)

	// explicit 0 is allowed (float literal)
	c, err = Build(`req_ai_intent_in("task_type", "coding", 0.0)`)
	require.NoError(t, err)
	require.NotNil(t, c)

	// too many args: still rejected
	_, err = Build(`req_ai_intent_in("task_type", "coding", 0.9, 1)`)
	assert.Error(t, err)

	// wrong arg type
	_, err = Build(`req_ai_intent_in("task_type", "coding", "0.9")`)
	assert.Error(t, err)

	// integer literal is not a float
	_, err = Build(`req_ai_intent_in("task_type", "coding", 0)`)
	assert.Error(t, err)

	_, err = Build(`req_ai_intent_in("task_type")`)
	assert.Error(t, err)
}

func testIntentReq() *bfe_basic.Request {
	return &bfe_basic.Request{
		Session:     &bfe_basic.Session{},
		HttpRequest: &bfe_http.Request{},
		Context:     make(map[interface{}]interface{}),
	}
}

func TestPrimitiveAiIntentIn(t *testing.T) {
	defer bfe_basic.SetAiIntentResolver(nil)
	defer bfe_basic.SetAiIntentThreshold(nil)

	bfe_basic.SetAiIntentThreshold(func(question string) float64 {
		if question == "strict_q" {
			return 0.9
		}
		return 0.6
	})

	var calls int
	bfe_basic.SetAiIntentResolver(func(req *bfe_basic.Request) *bfe_basic.AiIntent {
		calls++
		return &bfe_basic.AiIntent{
			QuestionsVersion: "v1",
			Source:           bfe_basic.IntentSourceModel,
			Answers: map[string]*bfe_basic.IntentAnswer{
				"task_type": {QType: bfe_basic.IntentQTypeChoice, Choice: "coding", AnswerConfidence: 0.8},
				"low_conf":  {QType: bfe_basic.IntentQTypeChoice, Choice: "coding", AnswerConfidence: 0.5},
				"strict_q":  {QType: bfe_basic.IntentQTypeChoice, Choice: "high", AnswerConfidence: 0.85},
			},
			Resolved: true,
		}
	})

	req := testIntentReq()

	// choice hit through the compiled condition; the resolver runs once and
	// the context result is reused by later evaluations
	cond, err := Build(`req_ai_intent_in("task_type", "test_writing|coding")`)
	require.NoError(t, err)
	assert.True(t, cond.Match(req))
	assert.Equal(t, 1, calls)
	assert.True(t, cond.Match(req))
	assert.Equal(t, 1, calls, "resolved intent must be reused from the request context")

	// option list semantics
	assert.True(t, PrimitiveAiIntentIn(req, "task_type", "coding", -1))
	assert.False(t, PrimitiveAiIntentIn(req, "task_type", "doc_writing|test_writing", -1))
	assert.False(t, PrimitiveAiIntentIn(req, "task_type", "", -1))

	// unconfigured / absent questions never match
	assert.False(t, PrimitiveAiIntentIn(req, "not_configured", "coding", -1))

	// omitted threshold: per-question gate applies (0.6 for these questions)
	assert.False(t, PrimitiveAiIntentIn(req, "low_conf", "coding", -1), "below the gate -> unknown")

	// explicit threshold is an additional, stricter gate
	assert.False(t, PrimitiveAiIntentIn(req, "task_type", "coding", 0.9), "0.8 < 0.9")
	assert.True(t, PrimitiveAiIntentIn(req, "task_type", "coding", 0.5), "0.8 >= 0.5 and gate passed")
	assert.True(t, PrimitiveAiIntentIn(req, "task_type", "coding", 0))

	// per-question gate can be stricter than the rule threshold
	assert.False(t, PrimitiveAiIntentIn(req, "strict_q", "high", 0.5), "0.85 < 0.9 per-question gate")

	// nil request / nil resolver
	assert.False(t, PrimitiveAiIntentIn(nil, "task_type", "coding", -1))
	bfe_basic.SetAiIntentResolver(nil)
	assert.False(t, PrimitiveAiIntentIn(testIntentReq(), "task_type", "coding", -1))
}
