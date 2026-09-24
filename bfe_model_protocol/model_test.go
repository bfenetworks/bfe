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

package bfe_model_protocol

import (
	"net/http"
	"testing"

	"github.com/bfenetworks/bfe/bfe_http"
)

func TestExtractModelFromPath(t *testing.T) {
	newReq := func(t *testing.T, path string) *bfe_http.Request {
		t.Helper()
		req, err := bfe_http.NewRequest(http.MethodPost, "http://example.com"+path, nil)
		if err != nil {
			t.Fatalf("NewRequest failed: %v", err)
		}
		return req
	}

	if got := ExtractModelFromPath(nil); got != "" {
		t.Errorf("nil request: got %q, want empty", got)
	}

	emptyURLReq := new(bfe_http.Request)
	if got := ExtractModelFromPath(emptyURLReq); got != "" {
		t.Errorf("nil URL: got %q, want empty", got)
	}

	if got := ExtractModelFromPath(newReq(t, "/v1beta/models/gemini-2.5-flash:generateContent")); got != "gemini-2.5-flash" {
		t.Errorf("gemini generateContent: got %q, want gemini-2.5-flash", got)
	}

	if got := ExtractModelFromPath(newReq(t, "/v1beta/models/gemini-2.5-flash:streamGenerateContent")); got != "gemini-2.5-flash" {
		t.Errorf("gemini streamGenerateContent: got %q, want gemini-2.5-flash", got)
	}

	if got := ExtractModelFromPath(newReq(t, "/v1/chat/completions")); got != "" {
		t.Errorf("openai path: got %q, want empty", got)
	}

	if got := ExtractModelFromPath(newReq(t, "/v1beta/models/")); got != "" {
		t.Errorf("empty model segment: got %q, want empty", got)
	}
}
