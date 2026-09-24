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

package bfe_server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

// issue #1384: the Gemini native protocol carries the model in the request
// path, so ClientModel population must fall back to path-based extraction
// when the body has no "model" field; body model keeps precedence.
func TestExtractClientModel(t *testing.T) {
	newReq := func(t *testing.T, path, body string) *bfe_basic.Request {
		t.Helper()
		httpReq, err := bfe_http.NewRequest(http.MethodPost, "http://example.com"+path, strings.NewReader(body))
		if err != nil {
			t.Fatalf("NewRequest failed: %v", err)
		}
		return bfe_basic.NewRequest(httpReq, nil, nil, nil, nil)
	}

	// openai style: body model
	if got := extractClientModel(newReq(t, "/v1/chat/completions", `{"model":"gpt-4"}`)); got != "gpt-4" {
		t.Errorf("openai body model: got %q, want gpt-4", got)
	}

	// gemini style: model in path, body has no model field
	geminiBody := `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`
	if got := extractClientModel(newReq(t, "/v1beta/models/gemini-2.5-flash:generateContent", geminiBody)); got != "gemini-2.5-flash" {
		t.Errorf("gemini path model: got %q, want gemini-2.5-flash", got)
	}

	// body model takes precedence over the path model
	if got := extractClientModel(newReq(t, "/v1beta/models/gemini-2.5-flash:generateContent", `{"model":"gpt-4"}`)); got != "gpt-4" {
		t.Errorf("body precedence: got %q, want gpt-4", got)
	}

	// neither body nor path carries a model
	if got := extractClientModel(newReq(t, "/v1/chat/completions", `{"messages":[]}`)); got != "" {
		t.Errorf("no model anywhere: got %q, want empty", got)
	}
}
