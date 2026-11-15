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

package sc47

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

// SC47 verifies that unselected route targets join the fallback chain:
// attempts = [selected target] + [remaining targets by weight desc]
//          + [fallbacks], deduplicated by (cluster_name, model).
// Design docs: tests/integration/测试设计文档/scenario-SC47-未选中targets并入降级链/
// Change record: docs/zh_cn/modifications/2026-11-12-ai-route-targets-as-fallbacks/

const (
	apiPath = "/v1/chat/completions"
	apiKey  = "ak_target_fb"

	clusterTa = "cluster_ta"
	clusterTb = "cluster_tb"
	clusterFc = "cluster_fc"
)

var defaultBody = []byte(`{"model":"gpt-4"}`)

type backendResponse struct {
	status int
	body   string
}

type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backends   map[string]*common.MockBackend
	bfePort    int
	stopBFE    func()
}

// newTestEnv starts three mock backends (ta/tb/fc) and a real BFE process.
// responses maps cluster name to its fixed (status, body) for every request.
func newTestEnv(t *testing.T, responses map[string]backendResponse) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	for _, name := range []string{clusterTa, clusterTb, clusterFc} {
		resp := responses[name]
		e.backends[name] = common.NewMockBackend(name, resp.status, resp.body)
	}

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	confDir := filepath.Join(e.processEnv.WorkDir(), "conf")
	logDir := filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: confDir,
		Backends:      e.backends,
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

func (e *testEnv) sendRequest(host string) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(defaultBody))
	if err != nil {
		return nil, "", err
	}
	req.Host = host
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

func (e *testEnv) hits(name string) int {
	return e.backends[name].Hits()
}

// TestTC01_SiblingTargetFallback covers the core behavior: when the selected
// target fails, the unselected sibling target is tried BEFORE any configured
// fallback. ta answers 500, tb answers 200, fc (fallback) answers 200 but
// must receive zero hits: falling back to fc while a sibling target is
// healthy was the old behavior and is the decisive new-vs-old signal.
func TestTC01_SiblingTargetFallback(t *testing.T) {
	e := newTestEnv(t, map[string]backendResponse{
		clusterTa: {http.StatusInternalServerError, `{"error":"ta unavailable"}`},
		clusterTb: {http.StatusOK, `{"cluster":"tb"}`},
		clusterFc: {http.StatusOK, `{"cluster":"fc"}`},
	})
	defer e.Close()

	const requests = 20
	for i := 0; i < requests; i++ {
		resp, body, err := e.sendRequest("tc1.target-fb.example.org")
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK || body != `{"cluster":"tb"}` {
			e.logBFEException()
			t.Fatalf("request %d: expected 200 from tb, got %d, body: %s", i, resp.StatusCode, body)
		}
	}

	if got := e.hits(clusterFc); got != 0 {
		t.Errorf("fallback cluster_fc must stay untouched while a sibling target serves, got %d hits", got)
	}
	// Every request is served by tb exactly once: directly when tb was the
	// weighted pick, or as the sibling fallback when ta was picked first.
	if got := e.hits(clusterTb); got != requests {
		t.Errorf("cluster_tb must serve every request exactly once, got %d hits over %d requests", got, requests)
	}
	// Requests that picked ta first cost one extra upstream attempt: ta+tb ==
	// requests + ta, which is already implied by the two assertions above;
	// what remains is sanity that both selection outcomes occurred.
	if ta := e.hits(clusterTa); ta == 0 || ta >= requests {
		t.Errorf("expected both selection outcomes over %d requests (ta picked first by some but not all), ta hits = %d", requests, ta)
	}
}

// TestTC02_CrossGroupDedup covers the legal config where a fallback repeats
// the selected target's (cluster_name, model): the chain is deduplicated so
// cluster_ta is tried exactly once per request, then cluster_fc serves.
// Under the old behavior cluster_ta would be tried twice (select + fallback).
func TestTC02_CrossGroupDedup(t *testing.T) {
	e := newTestEnv(t, map[string]backendResponse{
		clusterTa: {http.StatusInternalServerError, `{"error":"ta unavailable"}`},
		clusterTb: {http.StatusOK, `{"unused":true}`},
		clusterFc: {http.StatusOK, `{"cluster":"fc"}`},
	})
	defer e.Close()

	const requests = 10
	for i := 0; i < requests; i++ {
		resp, body, err := e.sendRequest("tc2.target-fb.example.org")
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK || body != `{"cluster":"fc"}` {
			e.logBFEException()
			t.Fatalf("request %d: expected 200 from fc, got %d, body: %s", i, resp.StatusCode, body)
		}
	}

	if got := e.hits(clusterTa); got != requests {
		t.Errorf("cluster_ta must be tried exactly once per request (dedup of the cross-group duplicate), got %d hits over %d requests", got, requests)
	}
	if got := e.hits(clusterFc); got != requests {
		t.Errorf("cluster_fc must serve every request, got %d hits over %d", got, requests)
	}
}

// TestTC03_ChainTailFallback covers the tail of the chain: only after BOTH
// targets fail does the configured fallback get tried, and when everything
// fails the LAST attempt's error body is returned ("return primary error" is
// a separate, not-yet-implemented optimization).
func TestTC03_ChainTailFallback(t *testing.T) {
	e := newTestEnv(t, map[string]backendResponse{
		clusterTa: {http.StatusInternalServerError, `{"error":"500 from ta"}`},
		clusterTb: {http.StatusInternalServerError, `{"error":"500 from tb"}`},
		clusterFc: {http.StatusInternalServerError, `{"error":"500 from fc"}`},
	})
	defer e.Close()

	const requests = 20
	for i := 0; i < requests; i++ {
		resp, body, err := e.sendRequest("tc3.target-fb.example.org")
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		if resp.StatusCode != http.StatusInternalServerError || body != `{"error":"500 from fc"}` {
			e.logBFEException()
			t.Fatalf("request %d: expected 500 with last-attempt (fc) error body, got %d, body: %s", i, resp.StatusCode, body)
		}
	}

	total := e.hits(clusterTa) + e.hits(clusterTb) + e.hits(clusterFc)
	if total != 3*requests {
		t.Errorf("expected the full chain (target -> sibling target -> fallback) once per request, got %d hits over %d requests (ta=%d tb=%d fc=%d)",
			total, requests, e.hits(clusterTa), e.hits(clusterTb), e.hits(clusterFc))
	}
	for _, name := range []string{clusterTa, clusterTb, clusterFc} {
		if e.hits(name) == 0 {
			t.Errorf("cluster %s never tried: every chain position must be exercised", name)
		}
	}
}

// TestTC04_SingleTargetRegression is the baseline: a single target without
// fallbacks behaves exactly as before — one attempt, upstream error passed
// through verbatim, no extra attempts introduced by the new chain building.
func TestTC04_SingleTargetRegression(t *testing.T) {
	e := newTestEnv(t, map[string]backendResponse{
		clusterTa: {http.StatusInternalServerError, `{"error":"ta unavailable"}`},
		clusterTb: {http.StatusOK, `{"unused":true}`},
		clusterFc: {http.StatusOK, `{"unused":true}`},
	})
	defer e.Close()

	const requests = 5
	for i := 0; i < requests; i++ {
		resp, body, err := e.sendRequest("tc4.target-fb.example.org")
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		if resp.StatusCode != http.StatusInternalServerError || body != `{"error":"ta unavailable"}` {
			e.logBFEException()
			t.Fatalf("request %d: expected upstream 500 passed through, got %d, body: %s", i, resp.StatusCode, body)
		}
	}

	if got := e.hits(clusterTa); got != requests {
		t.Errorf("single target must be tried exactly once per request, got %d hits over %d", got, requests)
	}
}
