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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
)

// maxDecisionRespBody bounds the decision service response body (8MB).
const maxDecisionRespBody = 8 << 20

// systemOneEndpoint is the decision service API path (TypeSafe Jev wire protocol).
const systemOneEndpoint = "/v1/systemone"

// DecisionClient classifies a state text against the configured questions
// via the System One protocol.
type DecisionClient interface {
	// Classify returns the answers by question name and the backend
	// version string. Any error (timeout, 5xx, malformed response) is
	// reported to the caller, which degrades to unknown answers.
	Classify(ctx context.Context, state string, qs *IntentQuestions) (map[string]*bfe_basic.IntentAnswer, string, error)
}

type systemOneAnswer struct {
	Type             string             `json:"type"`
	Choice           string             `json:"choice"`
	Score            float64            `json:"score"`
	Probabilities    map[string]float64 `json:"probabilities"`
	Confidence       float64            `json:"confidence"`
	AnswerConfidence float64            `json:"answer_confidence"`
}

type systemOneResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]systemOneAnswer `json:"answers"`
}

type systemOneClient struct {
	addr    string
	timeout time.Duration
	client  *http.Client
}

func NewDecisionClient(addr string, timeoutMs int) DecisionClient {
	if timeoutMs <= 0 {
		timeoutMs = DefaultTimeoutMs
	}
	return &systemOneClient{
		addr:    strings.TrimRight(addr, "/"),
		timeout: time.Duration(timeoutMs) * time.Millisecond,
		client:  &http.Client{},
	}
}

func (c *systemOneClient) Classify(ctx context.Context, state string, qs *IntentQuestions) (map[string]*bfe_basic.IntentAnswer, string, error) {
	if qs == nil {
		return nil, "", fmt.Errorf("nil questions")
	}

	reqBody := map[string]interface{}{
		"model":     "router",
		"state":     state,
		"questions": qs.SystemOneQuestions(),
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("marshal request err: %s", err)
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.addr+systemOneEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, "", fmt.Errorf("new request err: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("do request err: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}

	respBody, err := ioutil.ReadAll(http.MaxBytesReader(nil, resp.Body, maxDecisionRespBody))
	if err != nil {
		return nil, "", fmt.Errorf("read response err: %s", err)
	}

	var sr systemOneResponse
	if err := json.Unmarshal(respBody, &sr); err != nil {
		return nil, "", fmt.Errorf("parse response err: %s", err)
	}
	if len(sr.Answers) == 0 {
		return nil, "", fmt.Errorf("empty answers")
	}

	return parseDecisionAnswers(qs, &sr), sr.Model, nil
}

// parseDecisionAnswers converts the wire answers of configured questions
// into IntentAnswer. The gate (Unknown) is intentionally not decided here:
// only raw probabilities are stored and the gate is derived at read time.
func parseDecisionAnswers(qs *IntentQuestions, sr *systemOneResponse) map[string]*bfe_basic.IntentAnswer {
	ret := make(map[string]*bfe_basic.IntentAnswer)
	for _, q := range qs.OrderedQuestions() {
		ra, ok := sr.Answers[q.Name]
		if !ok {
			continue // absent answer: treated as unknown by the resolver
		}

		ans := &bfe_basic.IntentAnswer{QType: q.Type}
		switch q.Type {
		case QuestionTypeChoice:
			ans.Choice = ra.Choice
			ans.Probabilities = ra.Probabilities
			ans.AnswerConfidence = ra.Probabilities[ra.Choice]
		case QuestionTypeScore:
			ans.Score = ra.Score
			ans.LevelNames = q.LevelNames
			ans.Probabilities = remapScoreProbabilities(ra.Probabilities, q.LevelNames)
			if len(q.LevelNames) > 0 {
				idx := int(math.Round(ra.Score))
				if idx < 0 {
					idx = 0
				}
				if idx > len(q.LevelNames)-1 {
					idx = len(q.LevelNames) - 1
				}
				ans.Choice = q.LevelNames[idx]
			}
			ans.AnswerConfidence = ans.Probabilities[ans.Choice]
		}
		ret[q.Name] = ans
	}
	return ret
}

// remapScoreProbabilities maps numeric score probability keys ("0","1",...)
// to level names; keys that are already names are kept as is.
func remapScoreProbabilities(probs map[string]float64, levelNames []string) map[string]float64 {
	ret := make(map[string]float64, len(probs))
	for k, v := range probs {
		if idx, err := strconv.Atoi(k); err == nil && idx >= 0 && idx < len(levelNames) {
			ret[levelNames[idx]] = v
			continue
		}
		ret[k] = v
	}
	return ret
}

// intentBreaker is a simple consecutive-failure circuit breaker. After
// failureThreshold consecutive failures it opens and rejects calls; every
// probeInterval one probe call is allowed, and its success closes the breaker.
// Atomic counters only, no third-party library.
type intentBreaker struct {
	failureThreshold int32
	probeInterval    time.Duration
	onOpen           func()

	consecutiveFails int32
	openedAt         int64 // unix nanoseconds, 0 means closed
}

func newIntentBreaker(failureThreshold int, probeInterval time.Duration, onOpen func()) *intentBreaker {
	if failureThreshold <= 0 {
		failureThreshold = DefaultFailureThreshold
	}
	if probeInterval <= 0 {
		probeInterval = time.Duration(DefaultProbeIntervalMs) * time.Millisecond
	}
	return &intentBreaker{
		failureThreshold: int32(failureThreshold),
		probeInterval:    probeInterval,
		onOpen:           onOpen,
	}
}

// allow reports whether a call may go to the decision service.
func (b *intentBreaker) allow() bool {
	opened := atomic.LoadInt64(&b.openedAt)
	if opened == 0 {
		return true
	}
	if time.Since(time.Unix(0, opened)) >= b.probeInterval {
		// serialize probes: only one goroutine gets to probe per interval
		return atomic.CompareAndSwapInt64(&b.openedAt, opened, time.Now().UnixNano())
	}
	return false
}

// record feeds the result of an allowed call back into the breaker.
func (b *intentBreaker) record(success bool) {
	if success {
		atomic.StoreInt64(&b.openedAt, 0)
		atomic.StoreInt32(&b.consecutiveFails, 0)
		return
	}

	n := atomic.AddInt32(&b.consecutiveFails, 1)
	if n >= b.failureThreshold {
		now := time.Now().UnixNano()
		if atomic.SwapInt64(&b.openedAt, now) == 0 && b.onOpen != nil {
			b.onOpen()
		}
	}
}
