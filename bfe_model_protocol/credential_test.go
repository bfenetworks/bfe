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
	"reflect"
	"sort"
	"testing"

	"github.com/bfenetworks/bfe/bfe_http"
)

// forEachAdapter runs fn for every registered adapter in a deterministic
// order, so the matrix tests below cover a newly registered protocol without
// being edited.
func forEachAdapter(t *testing.T, fn func(a ProtocolAdapter)) {
	t.Helper()
	keys := make([]string, 0, len(adapters))
	for key := range adapters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fn(adapters[key])
	}
}

func newTestRequest(t *testing.T) *bfe_http.Request {
	t.Helper()
	req, err := bfe_http.NewRequest(http.MethodPost, "http://upstream.example.com/v1/chat/completions", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	return req
}

// setAllCredentialHeaders makes the request look like the reporting client of
// issue #1447: it carries every credential header detect.go recognizes.
func setAllCredentialHeaders(req *bfe_http.Request, value string) {
	req.Header.Set("Authorization", "Bearer "+value)
	req.Header.Set("x-api-key", value)
	req.Header.Set("x-goog-api-key", value)
}

// TestCredentialHeaderNames verifies the outbound sanitize set: it is the
// union of the registered adapters' credential headers (canonical, sorted,
// deduplicated) and matches the headers detect.go recognizes on the inbound
// side.
func TestCredentialHeaderNames(t *testing.T) {
	want := []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key"}
	got := CredentialHeaderNames()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	// issue #1447 relies on deriving the set from the registry: every
	// registered adapter must be covered.
	covered := map[string]bool{}
	for _, name := range got {
		covered[name] = true
	}
	for key, a := range adapters {
		name := bfe_http.CanonicalHeaderKey(a.CredentialHeader())
		if name == "" || !covered[name] {
			t.Errorf("adapter %q: credential header %q is not covered by CredentialHeaderNames()", key, name)
		}
	}
}

func TestForeignCredentialHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		keep    string
		want    []string
	}{
		{
			name:    "all present, openai keep",
			headers: map[string]string{"Authorization": "Bearer client-key", "x-api-key": "client-key", "x-goog-api-key": "client-key"},
			keep:    "Authorization",
			want:    []string{"X-Api-Key", "X-Goog-Api-Key"},
		},
		{
			name:    "all present, anthropic keep",
			headers: map[string]string{"Authorization": "Bearer client-key", "x-api-key": "client-key", "x-goog-api-key": "client-key"},
			keep:    "x-api-key",
			want:    []string{"Authorization", "X-Goog-Api-Key"},
		},
		{
			name:    "all present, gemini keep",
			headers: map[string]string{"Authorization": "Bearer client-key", "x-api-key": "client-key", "x-goog-api-key": "client-key"},
			keep:    "X-Goog-Api-Key",
			want:    []string{"Authorization", "X-Api-Key"},
		},
		{
			name:    "empty keep selects every credential header",
			headers: map[string]string{"Authorization": "Bearer client-key", "x-api-key": "client-key"},
			keep:    "",
			want:    []string{"Authorization", "X-Api-Key"},
		},
		{
			name:    "no foreign header",
			headers: map[string]string{"Authorization": "Bearer client-key"},
			keep:    "Authorization",
			want:    nil,
		},
		{
			name:    "present but empty value counts as present",
			headers: map[string]string{"Authorization": "Bearer client-key", "x-api-key": ""},
			keep:    "Authorization",
			want:    []string{"X-Api-Key"},
		},
		{
			name:    "unrelated headers are ignored",
			headers: map[string]string{"Content-Type": "application/json", "X-Request-Id": "1"},
			keep:    "Authorization",
			want:    nil,
		},
		{
			name:    "no headers",
			headers: map[string]string{},
			keep:    "Authorization",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := bfe_http.Header{}
			for k, v := range tt.headers {
				h.Set(k, v)
			}
			if got := ForeignCredentialHeaders(h, tt.keep); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}

	// a nil header map must be safe
	if got := ForeignCredentialHeaders(nil, "Authorization"); got != nil {
		t.Errorf("expected nil for a nil header map, got %v", got)
	}
}

func TestRemoveForeignCredentialHeaders(t *testing.T) {
	h := bfe_http.Header{}
	h.Set("Authorization", "Bearer client-key")
	h.Set("x-api-key", "client-key")
	h.Set("x-goog-api-key", "client-key")
	h.Set("Content-Type", "application/json")

	removed := RemoveForeignCredentialHeaders(h, "Authorization")
	if want := []string{"X-Api-Key", "X-Goog-Api-Key"}; !reflect.DeepEqual(removed, want) {
		t.Errorf("expected %v removed, got %v", want, removed)
	}
	if got := h.Get("x-api-key"); got != "" {
		t.Errorf("expected x-api-key removed, got %q", got)
	}
	if got := h.Get("x-goog-api-key"); got != "" {
		t.Errorf("expected x-goog-api-key removed, got %q", got)
	}
	if got := h.Get("Authorization"); got != "Bearer client-key" {
		t.Errorf("expected the keep header to survive, got %q", got)
	}
	if got := h.Get("Content-Type"); got != "application/json" {
		t.Errorf("expected unrelated headers to survive, got %q", got)
	}

	// idempotent: nothing left to remove
	if removed := RemoveForeignCredentialHeaders(h, "Authorization"); removed != nil {
		t.Errorf("expected no second-round removals, got %v", removed)
	}
}

// TestInjectAuthMutualExclusion reproduces the issue #1447 forwarding
// sequence per protocol (sanitize, then inject) and asserts the resulting
// invariant: the outbound request carries exactly the injected credential,
// whatever the client sent.
func TestInjectAuthMutualExclusion(t *testing.T) {
	const key = "upstream-key"
	forEachAdapter(t, func(a ProtocolAdapter) {
		keep := bfe_http.CanonicalHeaderKey(a.CredentialHeader())
		req := newTestRequest(t)
		setAllCredentialHeaders(req, "client-key")

		RemoveForeignCredentialHeaders(req.Header, keep)
		if err := a.InjectAuth(req, key); err != nil {
			t.Fatalf("%s: InjectAuth failed: %v", a.Key(), err)
		}

		var present []string
		for _, name := range CredentialHeaderNames() {
			if len(req.Header.Values(name)) > 0 {
				present = append(present, name)
			}
		}
		if !reflect.DeepEqual(present, []string{keep}) {
			t.Fatalf("%s: expected exactly one credential header %s, got %v", a.Key(), keep, present)
		}
		if got := req.Header.Get(keep); got != key && got != "Bearer "+key {
			t.Errorf("%s: expected the injected key, got %q", a.Key(), got)
		}
	})
}

// TestInjectAuthEmptyKeyKeepsClientCredentials is the passthrough guard: a
// cluster without configured keys does not inject anything, so the client
// credential headers must be forwarded untouched (issue #1447 must not break
// the BYOK passthrough mode documented in multi_api_key.md).
func TestInjectAuthEmptyKeyKeepsClientCredentials(t *testing.T) {
	forEachAdapter(t, func(a ProtocolAdapter) {
		req := newTestRequest(t)
		setAllCredentialHeaders(req, "client-key")

		if err := a.InjectAuth(req, ""); err != nil {
			t.Fatalf("%s: InjectAuth with empty key failed: %v", a.Key(), err)
		}

		if got := req.Header.Get("Authorization"); got != "Bearer client-key" {
			t.Errorf("%s: expected Authorization untouched, got %q", a.Key(), got)
		}
		if got := req.Header.Get("x-api-key"); got != "client-key" {
			t.Errorf("%s: expected x-api-key untouched, got %q", a.Key(), got)
		}
		if got := req.Header.Get("x-goog-api-key"); got != "client-key" {
			t.Errorf("%s: expected x-goog-api-key untouched, got %q", a.Key(), got)
		}
	})
}
