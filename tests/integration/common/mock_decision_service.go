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

package common

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
)

// systemOneEndpoint is the decision service API path (System One protocol).
const systemOneEndpoint = "/v1/systemone"

// DecisionAnswer describes the scripted System One answer of a single
// intent question. For choice questions set Choice and Probabilities
// (the probability of Choice is taken as the answer confidence by BFE).
// For score questions set Score (level index) and Probabilities keyed by
// level index ("0", "1", ...), optionally with Legend (level names).
type DecisionAnswer struct {
	Type             string             // "choice" or "score"
	Choice           string             // chosen option / level name
	Score            float64            // expected level index (score questions)
	Probabilities    map[string]float64 // option name / level index -> probability
	AnswerConfidence float64            // reported answer_confidence (informative)
	Legend           []string           // level names for score answers (optional)
}

// DecisionResponse describes the scripted System One response for a prompt
// keyword. Status is the HTTP status code (0 means 200).
type DecisionResponse struct {
	Status  int                       // HTTP status code, 0 means 200
	Answers map[string]DecisionAnswer // question name -> answer
}

// DecisionScript maps prompt keywords to scripted responses. The matching
// rule is "longest keyword first": the longest keyword contained in the
// prompt wins, so a prompt containing "写设计文档" matches that entry
// before the shorter "写文档".
type DecisionScript map[string]DecisionResponse

// MockDecisionService is an in-process httptest server implementing the
// System One decision protocol (POST /v1/systemone). It classifies the
// request state (the last user message) against a keyword script and
// returns the scripted answers, plus a concurrent-safe call counter.
type MockDecisionService struct {
	t      *testing.T
	server *httptest.Server

	mu     sync.Mutex
	script DecisionScript
	status int // forced status code for all responses, 0 means off
	hits   int
	bodies [][]byte
}

// NewMockDecisionService starts a local decision service mock serving the
// given script.
func NewMockDecisionService(t *testing.T, script DecisionScript) *MockDecisionService {
	m := &MockDecisionService{t: t, script: script}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

func (m *MockDecisionService) handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.hits++
	m.bodies = append(m.bodies, append([]byte(nil), body...))
	script := m.script
	status := m.status
	m.mu.Unlock()

	resp := DecisionResponse{Status: http.StatusOK, Answers: map[string]DecisionAnswer{}}
	prompt := extractDecisionPrompt(body)
	if prompt != "" {
		if keyword, ok := matchScriptKeyword(script, prompt); ok {
			resp = script[keyword]
		}
	}

	code := resp.Status
	if code == 0 {
		code = http.StatusOK
	}
	if status != 0 {
		code = status
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if code != http.StatusOK {
		_, _ = w.Write([]byte(`{"error":"mock decision service failure"}`))
		return
	}
	_, _ = w.Write(marshalSystemOneResponse(resp))
}

// marshalSystemOneResponse serializes the scripted response into the
// System One wire format: top-level model, answers (choice -> {type,
// choice, probabilities, answer_confidence}; score -> {type, score,
// probabilities, legend, answer_confidence}) and usage.
func marshalSystemOneResponse(resp DecisionResponse) []byte {
	answers := make(map[string]interface{}, len(resp.Answers))
	for name, ans := range resp.Answers {
		switch ans.Type {
		case "score":
			a := map[string]interface{}{
				"type":              "score",
				"score":             ans.Score,
				"probabilities":     ans.Probabilities,
				"answer_confidence": ans.AnswerConfidence,
			}
			if len(ans.Legend) > 0 {
				a["legend"] = ans.Legend
			}
			answers[name] = a
		default: // choice
			answers[name] = map[string]interface{}{
				"type":              "choice",
				"choice":            ans.Choice,
				"probabilities":     ans.Probabilities,
				"answer_confidence": ans.AnswerConfidence,
			}
		}
	}
	out := map[string]interface{}{
		"model":   "mock",
		"answers": answers,
		"usage":   map[string]interface{}{"prompt_tokens": 12, "completion_tokens": 3},
	}
	data, err := json.Marshal(out)
	if err != nil {
		panic(err)
	}
	return data
}

// extractDecisionPrompt returns the classified state of a decision service
// request. The BFE decision client sends the extracted text in the "state"
// field; for flexibility a "messages" openai-style body is also accepted,
// taking the content of the last message (string or text parts).
func extractDecisionPrompt(body []byte) string {
	var req struct {
		State    string        `json:"state"`
		Messages []decisionMsg `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	if req.State != "" {
		return req.State
	}
	if len(req.Messages) == 0 {
		return ""
	}
	return req.Messages[len(req.Messages)-1].text()
}

type decisionMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func (m decisionMsg) text() string {
	if len(m.Content) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(m.Content, &parts); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == "text" {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

// matchScriptKeyword finds the longest script keyword contained in the
// prompt. Ties are broken lexicographically for determinism.
func matchScriptKeyword(script DecisionScript, prompt string) (string, bool) {
	keywords := make([]string, 0, len(script))
	for k := range script {
		keywords = append(keywords, k)
	}
	sort.Slice(keywords, func(i, j int) bool {
		if len(keywords[i]) != len(keywords[j]) {
			return len(keywords[i]) > len(keywords[j])
		}
		return keywords[i] < keywords[j]
	})
	for _, k := range keywords {
		if strings.Contains(prompt, k) {
			return k, true
		}
	}
	return "", false
}

// URL returns the http address of the mock decision service.
func (m *MockDecisionService) URL() string {
	return m.server.URL
}

// Hits returns the number of decision service calls received.
func (m *MockDecisionService) Hits() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hits
}

// Calls returns the number of decision service calls received. It is an
// alias of Hits.
func (m *MockDecisionService) Calls() int {
	return m.Hits()
}

// SetStatus forces the given HTTP status code for all subsequent responses
// (0 disables the override).
func (m *MockDecisionService) SetStatus(status int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = status
}

// SetFailAll switches the mock between global failure (every response is
// HTTP 500) and scripted behavior.
func (m *MockDecisionService) SetFailAll(fail bool) {
	if fail {
		m.SetStatus(http.StatusInternalServerError)
		return
	}
	m.SetStatus(0)
}

// SetScript replaces the response script.
func (m *MockDecisionService) SetScript(script DecisionScript) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.script = script
}

// RequestBodies returns a deep copy of all received request bodies.
func (m *MockDecisionService) RequestBodies() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([][]byte, len(m.bodies))
	for i, b := range m.bodies {
		result[i] = append([]byte(nil), b...)
	}
	return result
}

// Close shuts down the mock decision service.
func (m *MockDecisionService) Close() {
	m.server.Close()
}
