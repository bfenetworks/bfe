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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bfenetworks/bfe/bfe_basic"
)

func testQuestions(t *testing.T) *IntentQuestions {
	qc := NewQuestionsConf(writeQuestionsFile(t, testQuestionsValid))
	require.NoError(t, qc.Load())
	return qc.Current()
}

func TestSystemOneClientClassify(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/systemone", r.URL.Path)
		var reqBody map[string]interface{}
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody)) {
			return
		}
		assert.Equal(t, "router", reqBody["model"])
		assert.Equal(t, "帮我写代码", reqBody["state"])

		resp := `{
		  "model": "laya-multilingual",
		  "answers": {
		    "task_type": {"type": "choice", "choice": "coding",
		                  "probabilities": {"coding": 0.9, "test_writing": 0.1},
		                  "confidence": 0.5, "answer_confidence": 0.7},
		    "complexity": {"type": "score", "score": 1.6,
		                   "probabilities": {"0": 0.1, "1": 0.5, "2": 0.4},
		                   "confidence": 0.6, "answer_confidence": 0.6}
		  },
		  "usage": {"input_tokens": 12, "output_tokens": 0}
		}`
		w.Write([]byte(resp))
	}))
	defer server.Close()

	client := NewDecisionClient(server.URL, 2000)
	answers, backend, err := client.Classify(context.Background(), "帮我写代码", testQuestions(t))
	require.NoError(t, err)
	assert.Equal(t, "laya-multilingual", backend)

	// choice: confidence gate uses probabilities[choice]
	choice, ok := answers["task_type"]
	require.True(t, ok)
	assert.Equal(t, bfe_basic.IntentQTypeChoice, choice.QType)
	assert.Equal(t, "coding", choice.Choice)
	assert.InDelta(t, 0.9, choice.AnswerConfidence, 1e-9)
	assert.InDelta(t, 0.1, choice.Probabilities["test_writing"], 1e-9)

	// score: choice is the rounded level name, probabilities remapped to names
	score, ok := answers["complexity"]
	require.True(t, ok)
	assert.Equal(t, bfe_basic.IntentQTypeScore, score.QType)
	assert.InDelta(t, 1.6, score.Score, 1e-9)
	assert.Equal(t, "complex", score.Choice, "round(1.6) maps to the 3rd level")
	assert.Equal(t, []string{"simple", "medium", "complex"}, score.LevelNames)
	assert.InDelta(t, 0.5, score.Probabilities["medium"], 1e-9)
	assert.InDelta(t, 0.4, score.AnswerConfidence, 1e-9)
}

func TestSystemOneClientScoreOutOfRangeClamp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := `{
		  "model": "m",
		  "answers": {
		    "complexity": {"type": "score", "score": 9.9,
		                   "probabilities": {"0": 0.0, "1": 1.0, "2": 0.0}}
		  }
		}`
		w.Write([]byte(resp))
	}))
	defer server.Close()

	client := NewDecisionClient(server.URL, 2000)
	answers, _, err := client.Classify(context.Background(), "text", testQuestions(t))
	require.NoError(t, err)
	assert.Equal(t, "complex", answers["complexity"].Choice, "out-of-range score clamps to last level")

	// choice question absent from the response: no answer is synthesized here
	_, ok := answers["task_type"]
	assert.False(t, ok)
}

func TestSystemOneClientErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewDecisionClient(server.URL, 2000)
	_, _, err := client.Classify(context.Background(), "text", testQuestions(t))
	assert.Error(t, err)
}

func TestSystemOneClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewDecisionClient(server.URL, 50)
	_, _, err := client.Classify(context.Background(), "text", testQuestions(t))
	assert.Error(t, err)
}

func TestIntentBreaker(t *testing.T) {
	opened := 0
	b := newIntentBreaker(2, time.Minute, func() { opened++ })

	assert.True(t, b.allow())
	b.record(false)
	assert.True(t, b.allow(), "below threshold, still closed")
	b.record(false)
	assert.Equal(t, 1, opened)
	assert.False(t, b.allow(), "breaker open rejects calls")

	// probe after interval: success closes the breaker
	time.Sleep(10 * time.Millisecond)
	b.probeInterval = 5 * time.Millisecond
	assert.True(t, b.allow(), "one probe allowed")
	assert.False(t, b.allow(), "only one probe at a time")
	b.record(true)
	assert.True(t, b.allow(), "closed again after probe success")
	assert.Equal(t, 1, opened)

	// failure after recovery re-opens
	b.record(false)
	b.record(false)
	assert.False(t, b.allow())
	assert.Equal(t, 2, opened)
}
