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

package embedding

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestOpenAIProviderEmbedSuccess(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())

	p, err := NewOpenAIProvider(Config{
		ServiceHost: u.Hostname(),
		ServicePort: port,
		Model:       "test-model",
		ApiKey:      "secret-key",
		TimeoutMs:   500,
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider error: %v", err)
	}

	emb, err := p.Embed("hello")
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if len(emb) != 3 || emb[0] != 0.1 || emb[2] != 0.3 {
		t.Errorf("unexpected embedding: %v", emb)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("unexpected authorization header: %q", gotAuth)
	}
	var reqPayload map[string]interface{}
	if err := json.Unmarshal([]byte(gotBody), &reqPayload); err != nil {
		t.Fatalf("request body is not json: %v", err)
	}
	if reqPayload["model"] != "test-model" || reqPayload["input"] != "hello" {
		t.Errorf("unexpected request payload: %s", gotBody)
	}
}

func TestOpenAIProviderEmbedFailures(t *testing.T) {
	// non-200 status
	srv500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv500.Close()

	// malformed response
	srvBad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srvBad.Close()

	// empty data
	srvEmpty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srvEmpty.Close()

	// slow server, triggers the client timeout
	srvSlow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Write([]byte(`{"data":[{"embedding":[0.1]}]}`))
	}))
	defer srvSlow.Close()

	cases := []struct {
		name string
		srv  *httptest.Server
	}{
		{"non 200", srv500},
		{"malformed response", srvBad},
		{"empty data", srvEmpty},
		{"timeout", srvSlow},
	}

	for _, c := range cases {
		u, _ := url.Parse(c.srv.URL)
		port, _ := strconv.Atoi(u.Port())
		p, err := NewOpenAIProvider(Config{
			ServiceHost: u.Hostname(),
			ServicePort: port,
			Model:       "test-model",
			TimeoutMs:   50,
		})
		if err != nil {
			t.Fatalf("%s: NewOpenAIProvider error: %v", c.name, err)
		}
		if _, err := p.Embed("hello"); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

func TestOpenAIProviderConfigCheck(t *testing.T) {
	base := Config{ServiceHost: "127.0.0.1", ServicePort: 11434, Model: "m", TimeoutMs: 500}

	cfg := base
	cfg.ServiceHost = ""
	if _, err := NewOpenAIProvider(cfg); err == nil {
		t.Error("empty host should be rejected")
	}

	cfg = base
	cfg.ServicePort = 0
	if _, err := NewOpenAIProvider(cfg); err == nil {
		t.Error("invalid port should be rejected")
	}

	cfg = base
	cfg.Model = ""
	if _, err := NewOpenAIProvider(cfg); err == nil {
		t.Error("empty model should be rejected")
	}

	// default timeout should be applied
	p, err := NewOpenAIProvider(Config{ServiceHost: "127.0.0.1", ServicePort: 1, Model: "m"})
	if err != nil {
		t.Fatalf("NewOpenAIProvider error: %v", err)
	}
	if p.timeout != 500*time.Millisecond {
		t.Errorf("default timeout should be 500ms, got %v", p.timeout)
	}

	// https scheme
	p, err = NewOpenAIProvider(Config{ServiceHost: "api.example.com", ServicePort: 443,
		UseHttps: true, Model: "m"})
	if err != nil {
		t.Fatalf("NewOpenAIProvider error: %v", err)
	}
	if p.endpoint != fmt.Sprintf("https://api.example.com:443%s", openAIEmbeddingPath) {
		t.Errorf("unexpected endpoint: %s", p.endpoint)
	}
}
