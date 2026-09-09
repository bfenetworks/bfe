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

package gemini

import (
	"net/http"
	"testing"

	"github.com/bfenetworks/bfe/bfe_http"
)

func TestInjectAuth(t *testing.T) {
	req, err := bfe_http.NewRequest(http.MethodGet, "http://example.com/", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	if err := New().InjectAuth(req, ""); err != nil {
		t.Fatalf("InjectAuth with empty key failed: %v", err)
	}
	if req.Header.Get("x-goog-api-key") != "" {
		t.Error("empty key should not set x-goog-api-key header")
	}

	if err := New().InjectAuth(req, "mykey"); err != nil {
		t.Fatalf("InjectAuth failed: %v", err)
	}
	if got := req.Header.Get("x-goog-api-key"); got != "mykey" {
		t.Errorf("unexpected x-goog-api-key header: %s", got)
	}
}

func TestExtraHeadersNil(t *testing.T) {
	if h := New().ExtraHeaders(); h != nil {
		t.Errorf("expected nil ExtraHeaders for gemini, got %v", h)
	}
}
