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

package sc17

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost = "api.example.org"
	apiKey  = "ak_user_a"

	clusterPrimary  = "cluster_primary"
	clusterFallback = "cluster_fallback"
	clusterDefault  = "cluster_default"

	pathChatCompletions = "/v1/chat/completions"
	pathMessages        = "/v1/messages"
)

type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backends   map[string]*common.MockBackend
	bfePort    int
	stopBFE    func()
}

func newTestEnv(t *testing.T, primaryStatus, fallbackStatus int,
	primaryAIConf, fallbackAIConf *cluster_conf.AIConf) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	e.backends[clusterPrimary] = common.NewMockBackend(clusterPrimary, primaryStatus, `{"ok":true}`)
	e.backends[clusterFallback] = common.NewMockBackend(clusterFallback, fallbackStatus, `{"ok":true}`)
	// cluster_default is referenced by server_data_conf/route_rule.data (basic
	// route); it is not used by any test case assertion.
	e.backends[clusterDefault] = common.NewMockBackend(clusterDefault, http.StatusOK, `{"ok":true}`)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	confDir := filepath.Join(e.processEnv.WorkDir(), "conf")
	logDir := filepath.Join(e.processEnv.WorkDir(), "log")

	aiConfs := map[string]*cluster_conf.AIConf{}
	if primaryAIConf != nil {
		aiConfs[clusterPrimary] = primaryAIConf
	}
	if fallbackAIConf != nil {
		aiConfs[clusterFallback] = fallbackAIConf
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: confDir,
		Backends:      e.backends,
		AIConfs:       aiConfs,
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(confDir, logDir)
	return e
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	for _, b := range e.backends {
		b.Close()
	}
}

func (e *testEnv) logBFEException() {
	data, err := os.ReadFile(filepath.Join(e.processEnv.WorkDir(), "log", "exception.log"))
	if err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}
}

// sendRequest sends a request to BFE. authHeader is the header carrying the
// apikey for mod_ai_route lookup ("Authorization" for openai style,
// "x-api-key" for anthropic style).
func (e *testEnv) sendRequest(path, authHeader string, body []byte) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, path)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	req.Header.Set(authHeader, apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return resp, string(respBody), nil
}

func (e *testEnv) sendOpenAI(body string) (*http.Response, string, error) {
	return e.sendRequest(pathChatCompletions, "Authorization", []byte(body))
}

func (e *testEnv) sendAnthropic(body string) (*http.Response, string, error) {
	return e.sendRequest(pathMessages, "x-api-key", []byte(body))
}

func openAIBody() string {
	return `{"model":"test-model","messages":[{"role":"user","content":"hello"}]}`
}

func anthropicBody() string {
	return `{"model":"test-model","messages":[{"role":"user","content":"hello"}],"max_tokens":1}`
}

func assertStatus(t *testing.T, e *testEnv, resp *http.Response, respBody string, want int) {
	t.Helper()
	if resp.StatusCode != want {
		e.logBFEException()
		t.Fatalf("expected status %d, got %d, body: %s", want, resp.StatusCode, respBody)
	}
}

func assertPaths(t *testing.T, b *common.MockBackend, want []string) {
	t.Helper()
	got := b.URLPaths()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backend %s paths = %v, want %v", b.ClusterName, got, want)
	}
}

// TC01: openai protocol rewrite (Bailian shape).
func TestTC01_OpenAIPathRewrite(t *testing.T) {
	e := newTestEnv(t, http.StatusOK, http.StatusOK,
		&cluster_conf.AIConf{
			Type:           0,
			ModelProtocols: []string{"openai", "anthropic"},
			ProtocolPaths:  map[string]string{"openai": "/compatible-mode/v1"},
		}, nil)
	defer e.Close()

	resp, respBody, err := e.sendOpenAI(openAIBody())
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	assertStatus(t, e, resp, respBody, http.StatusOK)

	assertPaths(t, e.backends[clusterPrimary], []string{"/compatible-mode/v1/chat/completions"})
	if e.backends[clusterFallback].Hits() != 0 {
		t.Fatalf("expected 0 hit on %s, got %d", clusterFallback, e.backends[clusterFallback].Hits())
	}
}

// TC02: anthropic protocol rewrite (Bailian shape).
func TestTC02_AnthropicPathRewrite(t *testing.T) {
	e := newTestEnv(t, http.StatusOK, http.StatusOK,
		&cluster_conf.AIConf{
			Type:           0,
			ModelProtocols: []string{"openai", "anthropic"},
			ProtocolPaths:  map[string]string{"anthropic": "/apps/anthropic"},
		}, nil)
	defer e.Close()

	resp, respBody, err := e.sendAnthropic(anthropicBody())
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	assertStatus(t, e, resp, respBody, http.StatusOK)

	assertPaths(t, e.backends[clusterPrimary], []string{"/apps/anthropic/v1/messages"})
	if e.backends[clusterFallback].Hits() != 0 {
		t.Fatalf("expected 0 hit on %s, got %d", clusterFallback, e.backends[clusterFallback].Hits())
	}
}

// TC03: dual-protocol cluster rewrites both protocols independently
// (Kimi Code shape: openai under /coding/v1, anthropic under /coding).
func TestTC03_DualProtocolRewrite(t *testing.T) {
	e := newTestEnv(t, http.StatusOK, http.StatusOK,
		&cluster_conf.AIConf{
			Type:           0,
			ModelProtocols: []string{"openai", "anthropic"},
			ProtocolPaths:  map[string]string{"openai": "/coding/v1", "anthropic": "/coding"},
		}, nil)
	defer e.Close()

	resp, respBody, err := e.sendOpenAI(openAIBody())
	if err != nil {
		t.Fatalf("send openai request failed: %v", err)
	}
	assertStatus(t, e, resp, respBody, http.StatusOK)

	resp, respBody, err = e.sendAnthropic(anthropicBody())
	if err != nil {
		t.Fatalf("send anthropic request failed: %v", err)
	}
	assertStatus(t, e, resp, respBody, http.StatusOK)

	assertPaths(t, e.backends[clusterPrimary],
		[]string{"/coding/v1/chat/completions", "/coding/v1/messages"})
}

// TC04 (patent case): route-level fallback recomputes the upstream path from
// the original client path. The fallback cluster must receive
// /coding/v1/messages; if the primary's rewritten path leaked into the
// inbound request, the fallback would inherit /apps/anthropic/v1/messages.
func TestTC04_FallbackRecomputePath(t *testing.T) {
	e := newTestEnv(t, http.StatusServiceUnavailable, http.StatusOK,
		&cluster_conf.AIConf{
			Type:           0,
			ModelProtocols: []string{"openai", "anthropic"},
			ProtocolPaths:  map[string]string{"anthropic": "/apps/anthropic"},
		},
		&cluster_conf.AIConf{
			Type:           0,
			ModelProtocols: []string{"openai", "anthropic"},
			ProtocolPaths:  map[string]string{"anthropic": "/coding"},
		})
	defer e.Close()

	resp, respBody, err := e.sendAnthropic(anthropicBody())
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	assertStatus(t, e, resp, respBody, http.StatusOK)

	if e.backends[clusterPrimary].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterPrimary, e.backends[clusterPrimary].Hits())
	}
	if e.backends[clusterFallback].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterFallback, e.backends[clusterFallback].Hits())
	}
	assertPaths(t, e.backends[clusterPrimary], []string{"/apps/anthropic/v1/messages"})
	assertPaths(t, e.backends[clusterFallback], []string{"/coding/v1/messages"})
}

// TC05: without ProtocolPaths the request path is forwarded unchanged.
func TestTC05_NoRewritePassthrough(t *testing.T) {
	e := newTestEnv(t, http.StatusOK, http.StatusOK, nil, nil)
	defer e.Close()

	resp, respBody, err := e.sendOpenAI(openAIBody())
	if err != nil {
		t.Fatalf("send openai request failed: %v", err)
	}
	assertStatus(t, e, resp, respBody, http.StatusOK)

	resp, respBody, err = e.sendAnthropic(anthropicBody())
	if err != nil {
		t.Fatalf("send anthropic request failed: %v", err)
	}
	assertStatus(t, e, resp, respBody, http.StatusOK)

	assertPaths(t, e.backends[clusterPrimary],
		[]string{"/v1/chat/completions", "/v1/messages"})
}

// TC06: non-standard entry paths are never rewritten, so clients can always
// call the gateway with provider-native paths (transparent passthrough mode).
func TestTC06_NonStandardEntryPassthrough(t *testing.T) {
	e := newTestEnv(t, http.StatusOK, http.StatusOK,
		&cluster_conf.AIConf{
			Type:           0,
			ModelProtocols: []string{"openai", "anthropic"},
			ProtocolPaths:  map[string]string{"openai": "/api/v3"},
		}, nil)
	defer e.Close()

	resp, respBody, err := e.sendRequest("/compatible-mode/v1/chat/completions", "Authorization", []byte(openAIBody()))
	if err != nil {
		t.Fatalf("send native path request failed: %v", err)
	}
	assertStatus(t, e, resp, respBody, http.StatusOK)

	resp, respBody, err = e.sendRequest("/v10/xxx", "Authorization", []byte(openAIBody()))
	if err != nil {
		t.Fatalf("send /v10 request failed: %v", err)
	}
	assertStatus(t, e, resp, respBody, http.StatusOK)

	assertPaths(t, e.backends[clusterPrimary],
		[]string{"/compatible-mode/v1/chat/completions", "/v10/xxx"})
}
