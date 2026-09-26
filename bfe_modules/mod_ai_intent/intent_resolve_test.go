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
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bfenetworks/go-lib/web-monitor/metrics"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

const testOpenAIBody = `{"model":"gpt-4","messages":[{"role":"user","content":"帮我实现一个限流器"}]}`

type mockDecisionClient struct {
	answers map[string]*bfe_basic.IntentAnswer
	backend string
	err     error
	calls   int
}

func (m *mockDecisionClient) Classify(ctx context.Context, state string, qs *IntentQuestions) (map[string]*bfe_basic.IntentAnswer, string, error) {
	m.calls++
	return m.answers, m.backend, m.err
}

func mockAnswers() map[string]*bfe_basic.IntentAnswer {
	return map[string]*bfe_basic.IntentAnswer{
		"task_type": {
			QType:            bfe_basic.IntentQTypeChoice,
			Choice:           "coding",
			Probabilities:    map[string]float64{"coding": 0.8, "test_writing": 0.2},
			AnswerConfidence: 0.8,
		},
		"complexity": {
			QType:            bfe_basic.IntentQTypeScore,
			Choice:           "medium",
			Score:            1.0,
			LevelNames:       []string{"simple", "medium", "complex"},
			Probabilities:    map[string]float64{"simple": 0.2, "medium": 0.6, "complex": 0.2},
			AnswerConfidence: 0.6,
		},
	}
}

func newTestState() *ModuleAiIntentState {
	return &ModuleAiIntentState{
		ReqTotal:         new(metrics.Counter),
		ReqResolved:      new(metrics.Counter),
		ReqHeader:        new(metrics.Counter),
		ReqCacheHit:      new(metrics.Counter),
		ReqUnknown:       new(metrics.Counter),
		ReqErr:           new(metrics.Counter),
		BreakerOpen:      new(metrics.Counter),
		QuestionsVersion: new(metrics.State),
	}
}

func newTestResolver(t *testing.T, client DecisionClient, state *ModuleAiIntentState) *intentResolver {
	qc := NewQuestionsConf(writeQuestionsFile(t, testQuestionsValid))
	require.NoError(t, qc.Load())
	conf := &ConfModAiIntent{}
	conf.Basic.ExplicitIntentHeader = DefaultExplicitIntentHeader
	conf.Basic.MaxStateChars = DefaultMaxStateChars
	conf.Breaker.FailureThreshold = 3
	conf.Breaker.ProbeIntervalMs = 5000

	cache := NewIntentCache(DefaultCacheSize, time.Minute)
	return newIntentResolver(conf, qc, cache, client, state, newLatencyHistogram())
}

func newTestRequest(body string, apiKey string, authStyle string, headers map[string]string) *bfe_basic.Request {
	httpReq := &bfe_http.Request{
		URL:    &url.URL{Path: "/v1/chat/completions"},
		Header: bfe_http.Header{},
	}
	if body != "" {
		httpReq.Body = ioutil.NopCloser(bytes.NewReader([]byte(body)))
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	req := &bfe_basic.Request{
		Session:     &bfe_basic.Session{},
		HttpRequest: httpReq,
		Context:     make(map[interface{}]interface{}),
	}
	aiMeta := req.InitAiBasicInfo()
	aiMeta.ClientApiKey = apiKey
	aiMeta.AuthStyle = authStyle
	return req
}

func TestResolveModelSuccess(t *testing.T) {
	client := &mockDecisionClient{answers: mockAnswers(), backend: "laya-multilingual"}
	state := newTestState()
	r := newTestResolver(t, client, state)

	req := newTestRequest(testOpenAIBody, "key-1", bfe_basic.AuthStyleOpenAI, nil)
	intent := r.Resolve(req)

	assert.True(t, intent.Resolved)
	assert.Equal(t, bfe_basic.IntentSourceModel, intent.Source)
	assert.Equal(t, "laya-multilingual", intent.BackendVersion)
	assert.Equal(t, "2026092601", intent.QuestionsVersion)
	assert.Equal(t, int64(1), state.ReqTotal.Get())
	assert.Equal(t, int64(1), state.ReqResolved.Get())
	assert.Equal(t, int64(0), state.ReqErr.Get())

	require.Contains(t, intent.Answers, "task_type")
	assert.Equal(t, "coding", intent.Answers["task_type"].Choice)
	assert.InDelta(t, 0.8, intent.Answers["task_type"].AnswerConfidence, 1e-9)
	require.Contains(t, intent.Answers, "complexity")
	assert.Equal(t, "medium", intent.Answers["complexity"].Choice)
}

func TestResolveStoresRawProbabilities(t *testing.T) {
	// answers below the confidence gate are stored as-is (raw); the unknown
	// state is derived at read time per the current threshold
	answers := mockAnswers()
	answers["task_type"].AnswerConfidence = 0.5 // below global gate 0.6
	client := &mockDecisionClient{answers: answers}
	state := newTestState()
	r := newTestResolver(t, client, state)

	req := newTestRequest(testOpenAIBody, "key-1", bfe_basic.AuthStyleOpenAI, nil)
	intent := r.Resolve(req)

	assert.True(t, intent.Resolved)
	assert.Equal(t, int64(1), state.ReqResolved.Get())
	assert.False(t, intent.Answers["task_type"].Unknown, "unknown must not be persisted")
	assert.InDelta(t, 0.5, intent.Answers["task_type"].AnswerConfidence, 1e-9)
}

func TestResolveHeaderOnly(t *testing.T) {
	client := &mockDecisionClient{answers: mockAnswers()}
	state := newTestState()
	r := newTestResolver(t, client, state)

	headers := map[string]string{
		"X-AI-Intent": "task_type=test_writing; complexity=simple",
	}
	req := newTestRequest(testOpenAIBody, "key-1", bfe_basic.AuthStyleOpenAI, headers)
	intent := r.Resolve(req)

	assert.Equal(t, 0, client.calls, "full header coverage must not call the decision service")
	assert.Equal(t, bfe_basic.IntentSourceHeader, intent.Source)
	assert.True(t, intent.Resolved)
	assert.Equal(t, int64(1), state.ReqHeader.Get())
	assert.Equal(t, int64(1), state.ReqResolved.Get())

	assert.Equal(t, "test_writing", intent.Answers["task_type"].Choice)
	assert.InDelta(t, 1.0, intent.Answers["task_type"].AnswerConfidence, 1e-9)
	assert.Equal(t, "simple", intent.Answers["complexity"].Choice)
}

func TestResolveHeaderPartialAndPriority(t *testing.T) {
	client := &mockDecisionClient{answers: mockAnswers()}
	state := newTestState()
	r := newTestResolver(t, client, state)

	headers := map[string]string{"X-AI-Intent": "task_type=test_writing"}
	req := newTestRequest(testOpenAIBody, "key-1", bfe_basic.AuthStyleOpenAI, headers)
	intent := r.Resolve(req)

	assert.Equal(t, 1, client.calls, "partial header coverage still classifies the rest")
	assert.Equal(t, bfe_basic.IntentSourceModel, intent.Source)
	assert.Equal(t, int64(1), state.ReqHeader.Get())
	// header answer wins over model answer
	assert.Equal(t, "test_writing", intent.Answers["task_type"].Choice)
	assert.InDelta(t, 1.0, intent.Answers["task_type"].AnswerConfidence, 1e-9)
	// the other question comes from the model
	assert.Equal(t, "medium", intent.Answers["complexity"].Choice)
}

func TestResolveHeaderInvalidEntriesIgnored(t *testing.T) {
	client := &mockDecisionClient{answers: mockAnswers()}
	state := newTestState()
	r := newTestResolver(t, client, state)

	headers := map[string]string{
		"X-AI-Intent": "task_type=not_an_option; unknown_q=coding; malformed; task_type=coding",
	}
	req := newTestRequest(testOpenAIBody, "key-1", bfe_basic.AuthStyleOpenAI, headers)
	intent := r.Resolve(req)

	assert.Equal(t, 1, client.calls)
	assert.Equal(t, int64(1), state.ReqHeader.Get(), "the one valid entry still counts")
	assert.Equal(t, "coding", intent.Answers["task_type"].Choice)
	assert.Equal(t, "medium", intent.Answers["complexity"].Choice)
}

func TestResolveCacheHit(t *testing.T) {
	client := &mockDecisionClient{answers: mockAnswers()}
	state := newTestState()
	r := newTestResolver(t, client, state)

	req1 := newTestRequest(testOpenAIBody, "key-1", bfe_basic.AuthStyleOpenAI, nil)
	intent1 := r.Resolve(req1)
	require.Equal(t, bfe_basic.IntentSourceModel, intent1.Source)

	req2 := newTestRequest(testOpenAIBody, "key-1", bfe_basic.AuthStyleOpenAI, nil)
	intent2 := r.Resolve(req2)

	assert.Equal(t, 1, client.calls, "cache hit must not repeat the classification")
	assert.Equal(t, bfe_basic.IntentSourceCache, intent2.Source)
	assert.Equal(t, int64(1), state.ReqCacheHit.Get())
	assert.Equal(t, int64(2), state.ReqResolved.Get())
	assert.Equal(t, "coding", intent2.Answers["task_type"].Choice)
	assert.Equal(t, intent1.QuestionsVersion, intent2.QuestionsVersion)

	// cached intents are cloned per request: read-time gating on one must
	// not leak into the other
	intent2.Answers["task_type"].Unknown = true
	assert.False(t, intent1.Answers["task_type"].Unknown)
}

func TestResolveCacheKeyIncludesApiKeyAndBody(t *testing.T) {
	client := &mockDecisionClient{answers: mockAnswers()}
	state := newTestState()
	r := newTestResolver(t, client, state)

	r.Resolve(newTestRequest(testOpenAIBody, "key-1", bfe_basic.AuthStyleOpenAI, nil))
	r.Resolve(newTestRequest(testOpenAIBody, "key-2", bfe_basic.AuthStyleOpenAI, nil))
	r.Resolve(newTestRequest(`{"messages":[{"role":"user","content":"另一个问题"}]}`, "key-1", bfe_basic.AuthStyleOpenAI, nil))

	assert.Equal(t, 3, client.calls)
}

func TestResolveModelError(t *testing.T) {
	client := &mockDecisionClient{err: errors.New("decision service down")}
	state := newTestState()
	r := newTestResolver(t, client, state)

	req := newTestRequest(testOpenAIBody, "key-1", bfe_basic.AuthStyleOpenAI, nil)
	intent := r.Resolve(req)

	assert.True(t, intent.Resolved)
	assert.Equal(t, int64(1), state.ReqErr.Get())
	assert.Equal(t, int64(0), state.ReqResolved.Get())
	for name, ans := range intent.Answers {
		require.NotNil(t, ans, name)
		assert.True(t, ans.Unknown, name)
		assert.InDelta(t, 0.0, ans.AnswerConfidence, 1e-9, name)
	}
}

func TestResolveBreakerOpen(t *testing.T) {
	client := &mockDecisionClient{err: errors.New("decision service down")}
	state := newTestState()
	r := newTestResolver(t, client, state)
	r.breaker.failureThreshold = 1

	req1 := newTestRequest(testOpenAIBody, "key-1", bfe_basic.AuthStyleOpenAI, nil)
	intent1 := r.Resolve(req1)
	assert.True(t, intent1.Answers["task_type"].Unknown)
	assert.Equal(t, int64(1), state.ReqErr.Get())
	assert.Equal(t, int64(1), state.BreakerOpen.Get())

	// breaker is open: no more calls, all unknown
	req2 := newTestRequest(`{"messages":[{"role":"user","content":"不同的问题"}]}`, "key-1", bfe_basic.AuthStyleOpenAI, nil)
	intent2 := r.Resolve(req2)
	assert.Equal(t, 1, client.calls, "breaker open must reject the call")
	assert.True(t, intent2.Answers["task_type"].Unknown)
	assert.Equal(t, int64(2), state.ReqErr.Get())
}

func TestResolveExtractFailure(t *testing.T) {
	client := &mockDecisionClient{answers: mockAnswers()}
	state := newTestState()
	r := newTestResolver(t, client, state)

	// no body at all
	req := newTestRequest("", "key-1", bfe_basic.AuthStyleOpenAI, nil)
	intent := r.Resolve(req)

	assert.True(t, intent.Resolved)
	assert.Equal(t, 0, client.calls)
	assert.Equal(t, int64(1), state.ReqUnknown.Get())
	assert.True(t, intent.Answers["task_type"].Unknown)
	assert.True(t, intent.Answers["complexity"].Unknown)
}

func TestResolveNonAIRequest(t *testing.T) {
	client := &mockDecisionClient{answers: mockAnswers()}
	state := newTestState()
	r := newTestResolver(t, client, state)

	httpReq := &bfe_http.Request{
		URL:    &url.URL{Path: "/"},
		Header: bfe_http.Header{},
	}
	req := &bfe_basic.Request{
		Session:     &bfe_basic.Session{},
		HttpRequest: httpReq,
		Context:     make(map[interface{}]interface{}),
	}

	intent := r.Resolve(req)
	assert.True(t, intent.Resolved)
	assert.Empty(t, intent.Answers)
	assert.Equal(t, 0, client.calls)
	assert.Equal(t, int64(1), state.ReqUnknown.Get())
}
