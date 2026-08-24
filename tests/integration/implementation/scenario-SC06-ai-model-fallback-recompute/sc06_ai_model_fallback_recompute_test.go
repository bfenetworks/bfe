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

package sc06

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost = "api.example.org"
	apiPath = "/v1/chat/completions"
	apiKey  = "ak_user_a"

	clusterPrimary  = "cluster_openrouter"
	clusterFallback = "cluster_fallback"
	clusterDefault  = "cluster_default"
)

type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backends   map[string]*common.MockBackend
	bfePort    int
	stopBFE    func()
}

func newTestEnv(t *testing.T, primaryStatus, fallbackStatus, defaultStatus int,
	primaryAIConf, fallbackAIConf *cluster_conf.AIConf) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	e.backends[clusterPrimary] = common.NewMockBackend(clusterPrimary, primaryStatus, `{"ok":true}`)
	e.backends[clusterFallback] = common.NewMockBackend(clusterFallback, fallbackStatus, `{"ok":true}`)
	e.backends[clusterDefault] = common.NewMockBackend(clusterDefault, defaultStatus, `{"ok":true}`)

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

func (e *testEnv) sendRequest(body []byte) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apiKey)
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

func primaryAIConf() *cluster_conf.AIConf {
	mappings := map[string]string{
		"modelA": "mapped-primary-model",
	}
	return &cluster_conf.AIConf{
		Type:         0,
		MatchPrefix:  "openrouter/",
		StripPrefix:  true,
		ModelMapping: &mappings,
	}
}

func fallbackAIConfStripClientPrefix() *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type:        0,
		MatchPrefix: "clientprefix/",
		StripPrefix: true,
	}
}

// TestTC01 verifies that when the primary cluster fails, the fallback cluster
// recomputes the target model from ClientModel instead of inheriting the
// primary cluster's mapped model.
func TestTC01_FallbackRecomputeFromClientModel(t *testing.T) {
	// primary returns 503 to trigger fallback; fallback returns 200
	e := newTestEnv(t, http.StatusServiceUnavailable, http.StatusOK, http.StatusOK,
		primaryAIConf(), nil)
	defer e.Close()

	clientModel := "clientprefix/mymodel"
	body := []byte(`{"model":"` + clientModel + `","messages":[{"role":"user","content":"hello"}]}`)
	resp, respBody, err := e.sendRequest(body)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, respBody)
	}

	if e.backends[clusterPrimary].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterPrimary, e.backends[clusterPrimary].Hits())
	}
	if e.backends[clusterFallback].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterFallback, e.backends[clusterFallback].Hits())
	}
	if e.backends[clusterDefault].Hits() != 0 {
		t.Fatalf("expected 0 hit on %s, got %d", clusterDefault, e.backends[clusterDefault].Hits())
	}

	primaryModels := e.backends[clusterPrimary].Models()
	if len(primaryModels) != 1 || primaryModels[0] != "mapped-primary-model" {
		t.Fatalf("expected primary model 'mapped-primary-model', got %v", primaryModels)
	}

	fallbackModels := e.backends[clusterFallback].Models()
	if len(fallbackModels) != 1 || fallbackModels[0] != clientModel {
		// If the fallback inherited the primary's TargetModel, it would send
		// "mapped-primary-model" instead of the original client model.
		t.Fatalf("expected fallback model '%s' (recomputed from ClientModel), got %v", clientModel, fallbackModels)
	}
}

// TestTC02 verifies that when the fallback cluster has its own strip
// configuration, it recomputes from ClientModel and applies the fallback
// cluster's own transformation, not the primary's mapped model.
func TestTC02_FallbackOwnStripConfig(t *testing.T) {
	// primary returns 503 to trigger fallback; fallback returns 200
	e := newTestEnv(t, http.StatusServiceUnavailable, http.StatusOK, http.StatusOK,
		primaryAIConf(), fallbackAIConfStripClientPrefix())
	defer e.Close()

	clientModel := "clientprefix/mymodel"
	body := []byte(`{"model":"` + clientModel + `","messages":[{"role":"user","content":"hello"}]}`)
	resp, respBody, err := e.sendRequest(body)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, respBody)
	}

	if e.backends[clusterPrimary].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterPrimary, e.backends[clusterPrimary].Hits())
	}
	if e.backends[clusterFallback].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterFallback, e.backends[clusterFallback].Hits())
	}

	primaryModels := e.backends[clusterPrimary].Models()
	if len(primaryModels) != 1 || primaryModels[0] != "mapped-primary-model" {
		t.Fatalf("expected primary model 'mapped-primary-model', got %v", primaryModels)
	}

	fallbackModels := e.backends[clusterFallback].Models()
	// The fallback cluster strips the "clientprefix/" prefix from the
	// recomputed ClientModel.
	if len(fallbackModels) != 1 || fallbackModels[0] != "mymodel" {
		t.Fatalf("expected fallback model 'mymodel', got %v", fallbackModels)
	}
}
