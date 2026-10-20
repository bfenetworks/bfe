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
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// AuditCall records one content-audit request.
type AuditCall struct {
	Path string
	Text string
}

// TextFilterResponse is the JSON body understood by mod_body_process. The
// gateway struct has no json tags, so the field names are matched
// case-insensitively (CamelCase is the canonical form).
type TextFilterResponse struct {
	RiskLevel      string
	SentimentScore float32
	ResultText     string
}

// MockAudit is a ToolGood-style text filter/replace service used by
// mod_body_process content audit tests.
type MockAudit struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  []AuditCall

	// FilterFunc handles /text-filter (request body audit). The default
	// returns {"RiskLevel":"PASS"}.
	FilterFunc func(text string) TextFilterResponse
	// ReplaceFunc handles /text-replace (response body rewrite). The default
	// echoes the input text back via ResultText.
	ReplaceFunc func(text string) TextFilterResponse
}

// NewMockAudit starts the audit service.
func NewMockAudit(t *testing.T) *MockAudit {
	t.Helper()
	m := &MockAudit{}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		text := r.PostFormValue("txt")

		m.mu.Lock()
		m.calls = append(m.calls, AuditCall{Path: r.URL.Path, Text: text})
		filterFn, replaceFn := m.FilterFunc, m.ReplaceFunc
		m.mu.Unlock()

		var resp TextFilterResponse
		switch r.URL.Path {
		case "/text-filter":
			if filterFn != nil {
				resp = filterFn(text)
			} else {
				resp = TextFilterResponse{RiskLevel: "PASS"}
			}
		case "/text-replace":
			if replaceFn != nil {
				resp = replaceFn(text)
			} else {
				resp = TextFilterResponse{ResultText: text}
			}
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	return m
}

// URL returns the base URL to put into body_process rule params.
func (m *MockAudit) URL() string {
	return m.server.URL
}

// Calls returns a snapshot of the recorded audit calls.
func (m *MockAudit) Calls() []AuditCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]AuditCall(nil), m.calls...)
}

// CallCount returns the number of recorded audit calls.
func (m *MockAudit) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// FilterCalls returns the recorded /text-filter calls.
func (m *MockAudit) FilterCalls() []AuditCall {
	var out []AuditCall
	for _, c := range m.Calls() {
		if c.Path == "/text-filter" {
			out = append(out, c)
		}
	}
	return out
}

// ReplaceCalls returns the recorded /text-replace calls.
func (m *MockAudit) ReplaceCalls() []AuditCall {
	var out []AuditCall
	for _, c := range m.Calls() {
		if c.Path == "/text-replace" {
			out = append(out, c)
		}
	}
	return out
}

// Close stops the audit service.
func (m *MockAudit) Close() {
	if m.server != nil {
		m.server.Close()
	}
}
