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

package sc02

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
	apiHost = "multikey.example.org"
	apiPath = "/v1/chat/completions"
	apiKey  = "ak_user_a"
	keyA    = "sk-key-a"
	keyB    = "sk-key-b"
	keyC    = "sk-key-c"

	clusterMultiKey   = "cluster_multi_key"
	clusterFallbackOK = "cluster_fallback_ok"
)

var defaultBody = []byte(`{"model":"gpt-4"}`)

// testEnv holds all resources for a single SC02 integration test.
type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backends   map[string]*common.MockBackend
	bfePort    int
	stopBFE    func()
}

func newTestEnv(t *testing.T, aiConf *cluster_conf.AIConf) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	e.backends[clusterMultiKey] = common.NewMockBackend(clusterMultiKey, http.StatusOK, `{"ok":true}`)
	e.backends[clusterFallbackOK] = common.NewMockBackend(clusterFallbackOK, http.StatusOK, `{"ok":true}`)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	confDir := filepath.Join(e.processEnv.WorkDir(), "conf")
	logDir := filepath.Join(e.processEnv.WorkDir(), "log")

	aiConfs := map[string]*cluster_conf.AIConf{}
	if aiConf != nil {
		aiConfs[clusterMultiKey] = aiConf
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

func defaultMultiKeyAIConf() *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type: 0,
		Keys: []cluster_conf.AIKey{
			{Name: "key-a", Key: keyA, Weight: 50},
			{Name: "key-b", Key: keyB, Weight: 30},
			{Name: "key-c", Key: keyC, Weight: 20},
		},
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:            "weighted_random",
			MaxRetries:          3,
			RetryBackoffInitial: 50,
			RetryBackoffMax:     200,
		},
	}
}

func countAuthHeaders(headers []string) map[string]int {
	counts := make(map[string]int)
	for _, h := range headers {
		counts[h]++
	}
	return counts
}

func generateBody(size int) []byte {
	body := make([]byte, size)
	for i := range body {
		body[i] = byte('a' + i%26)
	}
	return body
}

// wrapBody wraps a fixed prefix and suffix around generated filler content.
func wrapBody(prefix, suffix string, fillerSize int) []byte {
	filler := generateBody(fillerSize)
	return []byte(prefix + string(filler) + suffix)
}

// TestTC01 verifies weighted random selection across multiple API keys.
func TestTC01_WeightedKeySelection(t *testing.T) {
	aiConf := defaultMultiKeyAIConf()
	aiConf.KeyPolicy.MaxRetries = 0
	e := newTestEnv(t, aiConf)
	defer e.Close()

	for i := 0; i < 500; i++ {
		resp, body, err := e.sendRequest(defaultBody)
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
		}
	}

	if e.backends[clusterMultiKey].Hits() != 500 {
		t.Fatalf("expected %d hits on %s, got %d", 500, clusterMultiKey, e.backends[clusterMultiKey].Hits())
	}
	if e.backends[clusterFallbackOK].Hits() != 0 {
		t.Fatalf("expected fallback not hit, got %d", e.backends[clusterFallbackOK].Hits())
	}

	counts := countAuthHeaders(e.backends[clusterMultiKey].AuthHeaders())
	if delta := abs(counts["Bearer "+keyA] - 250); delta > 50 {
		t.Fatalf("key-a count %d deviates too far from 250", counts["Bearer "+keyA])
	}
	if delta := abs(counts["Bearer "+keyB] - 150); delta > 40 {
		t.Fatalf("key-b count %d deviates too far from 150", counts["Bearer "+keyB])
	}
	if delta := abs(counts["Bearer "+keyC] - 100); delta > 35 {
		t.Fatalf("key-c count %d deviates too far from 100", counts["Bearer "+keyC])
	}
}

// TestTC02 verifies that a 429 response causes BFE to rotate to another key.
func TestTC02_429RotatesKey(t *testing.T) {
	e := newTestEnv(t, defaultMultiKeyAIConf())
	defer e.Close()

	multiKey := e.backends[clusterMultiKey]
	multiKey.ResponseFunc = func(r *http.Request, count int) (int, string) {
		if r.Header.Get("Authorization") == "Bearer "+keyA {
			return http.StatusTooManyRequests, `{"error":"rate limited"}`
		}
		return http.StatusOK, `{"ok":true}`
	}

	// Send enough requests so that key-a is hit at least once and a different
	// key eventually succeeds.
	for i := 0; i < 100; i++ {
		resp, body, err := e.sendRequest(defaultBody)
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
		}
	}

	if e.backends[clusterFallbackOK].Hits() != 0 {
		t.Fatalf("expected fallback not hit, got %d", e.backends[clusterFallbackOK].Hits())
	}
	if multiKey.Hits() <= 100 {
		t.Fatalf("expected more than %d hits due to retries, got %d", 100, multiKey.Hits())
	}

	counts := countAuthHeaders(multiKey.AuthHeaders())
	if counts["Bearer "+keyA] == 0 {
		t.Fatalf("expected key-a to be used at least once")
	}
	if counts["Bearer "+keyB]+counts["Bearer "+keyC] == 0 {
		t.Fatalf("expected at least one other key to be used")
	}
}

// TestTC03 verifies that 401/403 responses mark the key dead for the current
// aiClusterInvoke call, so subsequent attempts within the same request skip it.
func TestTC03_401And403MarkKeyDead(t *testing.T) {
	aiConf := &cluster_conf.AIConf{
		Type: 0,
		Keys: []cluster_conf.AIKey{
			{Name: "key-a", Key: keyA, Weight: 40},
			{Name: "key-b", Key: keyB, Weight: 40},
			{Name: "key-c", Key: keyC, Weight: 20},
		},
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:            "weighted_random",
			MaxRetries:          3,
			RetryBackoffInitial: 50,
			RetryBackoffMax:     200,
		},
	}
	e := newTestEnv(t, aiConf)
	defer e.Close()

	multiKey := e.backends[clusterMultiKey]
	multiKey.ResponseFunc = func(r *http.Request, count int) (int, string) {
		auth := r.Header.Get("Authorization")
		if auth == "Bearer "+keyA {
			return http.StatusUnauthorized, `{"error":"unauthorized"}`
		}
		if auth == "Bearer "+keyB {
			return http.StatusForbidden, `{"error":"forbidden"}`
		}
		return http.StatusOK, `{"ok":true}`
	}

	// Send enough requests so that key-a/key-b are hit at least once and key-c
	// eventually succeeds within each request.
	for i := 0; i < 100; i++ {
		resp, body, err := e.sendRequest(defaultBody)
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
		}
	}

	if e.backends[clusterFallbackOK].Hits() != 0 {
		t.Fatalf("expected fallback not hit, got %d", e.backends[clusterFallbackOK].Hits())
	}

	counts := countAuthHeaders(multiKey.AuthHeaders())
	if counts["Bearer "+keyA] == 0 {
		t.Fatalf("expected key-a to be used at least once")
	}
	if counts["Bearer "+keyB] == 0 {
		t.Fatalf("expected key-b to be used at least once")
	}
	if counts["Bearer "+keyC] == 0 {
		t.Fatalf("expected key-c to be used at least once")
	}
}

// TestTC04 verifies that 5xx responses trigger retry on the same key with backoff.
func TestTC04_5xxRetriesSameKey(t *testing.T) {
	e := newTestEnv(t, defaultMultiKeyAIConf())
	defer e.Close()

	multiKey := e.backends[clusterMultiKey]
	failCount := 0
	multiKey.ResponseFunc = func(r *http.Request, count int) (int, string) {
		if failCount < 2 {
			failCount++
			return http.StatusServiceUnavailable, `{"error":"unavailable"}`
		}
		return http.StatusOK, `{"ok":true}`
	}

	start := time.Now()
	resp, body, err := e.sendRequest(defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	elapsed := time.Since(start)
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterFallbackOK].Hits() != 0 {
		t.Fatalf("expected fallback not hit, got %d", e.backends[clusterFallbackOK].Hits())
	}
	if multiKey.Hits() != 3 {
		t.Fatalf("expected 3 attempts, got %d", multiKey.Hits())
	}

	authHeaders := multiKey.AuthHeaders()
	firstAuth := authHeaders[0]
	for _, h := range authHeaders {
		if h != firstAuth {
			t.Fatalf("expected all attempts to use the same key, got %s and %s", firstAuth, h)
		}
	}

	// Two retries with initial backoff 50ms should take at least 50ms even with
	// jitter; allow a small margin.
	if elapsed < 40*time.Millisecond {
		t.Fatalf("expected retry backoff, elapsed %v", elapsed)
	}
}

// TestTC05 verifies that when all keys are exhausted by 4xx errors, the last
// response is returned and cluster fallback is not triggered.
func TestTC05_AllKeysExhausted(t *testing.T) {
	e := newTestEnv(t, defaultMultiKeyAIConf())
	defer e.Close()

	multiKey := e.backends[clusterMultiKey]
	multiKey.ResponseFunc = func(r *http.Request, count int) (int, string) {
		switch r.Header.Get("Authorization") {
		case "Bearer " + keyA:
			return http.StatusTooManyRequests, `{"error":"rate limited"}`
		case "Bearer " + keyB:
			return http.StatusUnauthorized, `{"error":"unauthorized"}`
		default:
			return http.StatusForbidden, `{"error":"forbidden"}`
		}
	}

	resp, body, err := e.sendRequest(defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	// The final 4xx status depends on which key was selected last.
	if resp.StatusCode != http.StatusTooManyRequests &&
		resp.StatusCode != http.StatusUnauthorized &&
		resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 4xx status, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterFallbackOK].Hits() != 0 {
		t.Fatalf("expected fallback not hit, got %d", e.backends[clusterFallbackOK].Hits())
	}
	// With MaxRetries=3, BFE may try up to 4 times (including a reset of the
	// used_set when only 429 keys remain).
	if multiKey.Hits() < 3 || multiKey.Hits() > 4 {
		t.Fatalf("expected 3 or 4 attempts, got %d", multiKey.Hits())
	}
}

// TestTC06 verifies that 5xx key-level retry exhaustion triggers cluster fallback.
func TestTC06_KeyExhaustionTriggersFallback(t *testing.T) {
	aiConf := defaultMultiKeyAIConf()
	aiConf.KeyPolicy.MaxRetries = 2
	e := newTestEnv(t, aiConf)
	defer e.Close()

	e.backends[clusterMultiKey].ResponseFunc = func(r *http.Request, count int) (int, string) {
		return http.StatusServiceUnavailable, `{"error":"unavailable"}`
	}

	resp, body, err := e.sendRequest(defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200 from fallback, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterMultiKey].Hits() > 3 {
		t.Fatalf("expected at most 3 attempts on multi-key cluster, got %d", e.backends[clusterMultiKey].Hits())
	}
	if e.backends[clusterFallbackOK].Hits() != 1 {
		t.Fatalf("expected fallback hit once, got %d", e.backends[clusterFallbackOK].Hits())
	}
}

// TestTC07 verifies that the request body is fully rewound across key rotations.
func TestTC07_BodyRewoundOnKeyRotation(t *testing.T) {
	e := newTestEnv(t, defaultMultiKeyAIConf())
	defer e.Close()

	multiKey := e.backends[clusterMultiKey]
	multiKey.ResponseFunc = func(r *http.Request, count int) (int, string) {
		if r.Header.Get("Authorization") == "Bearer "+keyA {
			return http.StatusTooManyRequests, `{"error":"rate limited"}`
		}
		return http.StatusOK, `{"ok":true}`
	}

	// Build a ~100 KB body so any truncation would be detectable while keeping
	// the test fast and within the default bytes_body limits.
	body := wrapBody(`{"model":"gpt-4","content":"`, `"}`, 100*1024)

	// Send enough requests to trigger at least one rotation.
	var rotated bool
	var successes int
	for i := 0; i < 100; i++ {
		resp, _, err := e.sendRequest(body)
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode == http.StatusOK {
			successes++
		}
		if multiKey.Hits() > i+1 {
			rotated = true
		}
	}
	if successes == 0 {
		t.Fatalf("expected at least one successful request")
	}
	if !rotated {
		t.Fatalf("expected at least one request to rotate keys")
	}

	bodies := multiKey.RequestBodies()
	for i, got := range bodies {
		if !bytes.Equal(got, body) {
			t.Fatalf("request body %d differs from original (len %d vs %d)", i, len(got), len(body))
		}
	}
}

// TestTC08 verifies that AIConf extended fields (Provider, ModelTable) are loaded
// and do not break forwarding or model mapping.
func TestTC08_AIConfExtendedFields(t *testing.T) {
	aiConf := &cluster_conf.AIConf{
		Type: 0,
		ModelMapping: &map[string]string{
			"gpt-4": "mapped-model",
		},
		Provider: "mock-provider",
		Keys: []cluster_conf.AIKey{
			{Name: "key-c", Key: keyC, Weight: 100},
		},
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:            "weighted_random",
			MaxRetries:          0,
			RetryBackoffInitial: 50,
			RetryBackoffMax:     200,
		},
		ModelTable: &cluster_conf.ModelTable{
			Currency: "RMB",
			Models: []cluster_conf.ModelPrice{
				{
					Provider:            "mock-provider",
					Model:               "mapped-model",
					BaseModel:           "mapped-model",
					Mode:                "chat",
					Capabilities:        []string{"chat"},
					SupportedParameters: []string{"temperature"},
					Limits: map[string]interface{}{
						"context_window":    128000,
						"max_input_tokens":  128000,
						"max_output_tokens": 8192,
					},
					Prices: map[string]float64{
						"input_cost_per_token":  0.000002,
						"output_cost_per_token": 0.000008,
					},
				},
			},
		},
	}

	e := newTestEnv(t, aiConf)
	defer e.Close()

	resp, _, err := e.sendRequest(defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if e.backends[clusterMultiKey].Hits() != 1 {
		t.Fatalf("expected one hit on multi-key cluster, got %d", e.backends[clusterMultiKey].Hits())
	}
	if e.backends[clusterFallbackOK].Hits() != 0 {
		t.Fatalf("expected fallback not hit, got %d", e.backends[clusterFallbackOK].Hits())
	}

	authHeaders := e.backends[clusterMultiKey].AuthHeaders()
	if len(authHeaders) != 1 || authHeaders[0] != "Bearer "+keyC {
		t.Fatalf("expected key-c, got %v", authHeaders)
	}

	models := e.backends[clusterMultiKey].Models()
	if len(models) != 1 || models[0] != "mapped-model" {
		t.Fatalf("expected model mapping to mapped-model, got %v", models)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
