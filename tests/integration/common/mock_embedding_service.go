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
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
)

// openAIEmbeddingEndpoint is the OpenAI compatible embedding endpoint
// implemented by this mock (the same path the mod_ai_cache embedding client
// calls).
const openAIEmbeddingEndpoint = "/v1/embeddings"

// EmbeddingScript maps question keywords to scripted embedding vectors. The
// matching rule is "longest keyword first" (mirroring DecisionScript): the
// longest keyword contained in the input text wins, so an input containing
// "introduce bfe" matches that entry before the shorter "bfe". Inputs
// matching no keyword get a deterministic default vector far from any
// scripted one.
type EmbeddingScript map[string][]float64

// MockEmbeddingService is an in-process httptest server implementing the
// OpenAI compatible embedding API (POST /v1/embeddings) consumed by
// mod_ai_cache. It returns scripted vectors per request text, plus a
// concurrent-safe call counter, a global failure switch (HTTP 500) and a
// request body recorder for assertions.
type MockEmbeddingService struct {
	t      *testing.T
	server *httptest.Server

	mu         sync.Mutex
	script     EmbeddingScript
	defaultVec []float64
	status     int // forced status code for all responses, 0 means off
	hits       int
	bodies     [][]byte
}

// NewMockEmbeddingService starts a local embedding service mock serving the
// given script.
func NewMockEmbeddingService(t *testing.T, script EmbeddingScript) *MockEmbeddingService {
	m := &MockEmbeddingService{
		t:          t,
		script:     script,
		defaultVec: []float64{0, 0, 1},
	}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

func (m *MockEmbeddingService) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != openAIEmbeddingEndpoint || r.Method != http.MethodPost {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

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

	w.Header().Set("Content-Type", "application/json")
	if status != 0 {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"mock embedding service failure"}`))
		return
	}

	text := extractEmbeddingText(body)
	vec := m.vectorFor(script, text)
	resp := map[string]interface{}{
		"object": "list",
		"data": []map[string]interface{}{
			{"object": "embedding", "index": 0, "embedding": vec},
		},
		"model": "mock-embedding",
		"usage": map[string]interface{}{"prompt_tokens": 1, "total_tokens": 1},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// vectorFor resolves the scripted vector of the input text. The script lookup
// runs without the lock: scripts are replaced atomically (SetScript) and
// never mutated in place.
func (m *MockEmbeddingService) vectorFor(script EmbeddingScript, text string) []float64 {
	if keyword, ok := matchEmbeddingKeyword(script, text); ok {
		return append([]float64(nil), script[keyword]...)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]float64(nil), m.defaultVec...)
}

// extractEmbeddingText returns the input text of an OpenAI embedding request.
// The BFE client sends "input" as a plain string; an array of strings (batch
// form) is also accepted, taking the first entry.
func extractEmbeddingText(body []byte) string {
	var req struct {
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	if len(req.Input) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(req.Input, &s); err == nil {
		return s
	}
	var arr []string
	if err := json.Unmarshal(req.Input, &arr); err == nil {
		if len(arr) > 0 {
			return arr[0]
		}
		return ""
	}
	return ""
}

// matchEmbeddingKeyword finds the longest script keyword contained in the
// input text. Ties are broken lexicographically for determinism.
func matchEmbeddingKeyword(script EmbeddingScript, text string) (string, bool) {
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
		if strings.Contains(text, k) {
			return k, true
		}
	}
	return "", false
}

// URL returns the http address of the mock embedding service.
func (m *MockEmbeddingService) URL() string {
	return m.server.URL
}

// Addr returns the host:port of the mock embedding service.
func (m *MockEmbeddingService) Addr() string {
	u, _ := url.Parse(m.server.URL)
	return u.Host
}

// Hits returns the number of embedding calls received.
func (m *MockEmbeddingService) Hits() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hits
}

// Calls returns the number of embedding calls received. It is an alias of
// Hits.
func (m *MockEmbeddingService) Calls() int {
	return m.Hits()
}

// SetStatus forces the given HTTP status code for all subsequent responses
// (0 disables the override).
func (m *MockEmbeddingService) SetStatus(status int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = status
}

// SetFailAll switches the mock between global failure (every response is
// HTTP 500) and scripted behavior.
func (m *MockEmbeddingService) SetFailAll(fail bool) {
	if fail {
		m.SetStatus(http.StatusInternalServerError)
		return
	}
	m.SetStatus(0)
}

// SetScript replaces the keyword script.
func (m *MockEmbeddingService) SetScript(script EmbeddingScript) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.script = script
}

// RequestBodies returns a deep copy of all received request bodies.
func (m *MockEmbeddingService) RequestBodies() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([][]byte, len(m.bodies))
	for i, b := range m.bodies {
		result[i] = append([]byte(nil), b...)
	}
	return result
}

// Close shuts down the mock embedding service.
func (m *MockEmbeddingService) Close() {
	m.server.Close()
}
