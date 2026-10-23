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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost           = "multikey.example.org"
	apiHostNoFallback = "multikey-nofallback.example.org"
	apiHostDeadBackend = "deadbackend.example.org"
	apiPath           = "/v1/chat/completions"
	apiKey            = "ak_user_a"
	keyA              = "sk-key-a"
	keyB              = "sk-key-b"
	keyC              = "sk-key-c"

	clusterMultiKey    = "cluster_multi_key"
	clusterFallbackOK  = "cluster_fallback_ok"
	clusterDeadBackend = "cluster_dead_backend"
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
		// cluster_dead_backend (TC-10) is declared in the shared testdata
		// gslb.data; every env must give it a cluster_table entry or BFE
		// startup fails with "no backend conf". Only TC-10 routes to it.
		StaticBackends: map[string]string{clusterDeadBackend: "127.0.0.1:1"},
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
	return e.sendRequestToHost(apiHost, body)
}

func (e *testEnv) sendRequestToHost(host string, body []byte) (*http.Response, string, error) {
	return e.sendRequestToHostWithHeaders(host, body, nil)
}

// sendRequestToHostWithHeaders is sendRequestToHost plus extra request
// headers (used by the issue #1441 round-2 correlation-header assertions).
func (e *testEnv) sendRequestToHostWithHeaders(host string, body []byte, headers map[string]string) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = host
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

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

// isDecimalLogIdInRange mirrors the access-log logid contract
// (bfenetworks/bfe#1417): pure decimal, non-zero, fits the signed BIGINT of
// the report databases.
func isDecimalLogIdInRange(s string) bool {
	v, err := strconv.ParseUint(s, 10, 64)
	return err == nil && v > 0 && v <= uint64(1)<<63-1
}

// bfeLogContains reports whether the BFE runtime log contains the substring.
func (e *testEnv) bfeLogContains(substr string) bool {
	data, err := os.ReadFile(filepath.Join(e.processEnv.WorkDir(), "log", "bfe.log"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substr)
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

// TestTC05 verifies that when all keys are exhausted by 429/401/403 errors,
// the final 4xx response triggers cluster-level fallback.
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 from fallback, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterFallbackOK].Hits() != 1 {
		t.Fatalf("expected fallback hit once, got %d", e.backends[clusterFallbackOK].Hits())
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
					Prices: cluster_conf.PriceMap{
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

// TestTC09 verifies issue #1440: when the very first key selection fails
// (all keys Weight=0, zero upstream interaction), aiClusterInvoke must
// return a decidable synthetic 429 instead of (nil, nil).
//   - Arm A: with a fallback configured, the outer loop degrades to the
//     fallback cluster and the client gets 200.
//   - Arm B: without a fallback, the client gets a retryable 429 with a
//     JSON error body instead of a 500.
func TestTC09_FirstSelectionFailureSynthetic429(t *testing.T) {
	aiConf := defaultMultiKeyAIConf()
	aiConf.Keys = []cluster_conf.AIKey{
		{Name: "key-a", Key: keyA, Weight: 0},
		{Name: "key-b", Key: keyB, Weight: 0},
	}
	aiConf.KeyPolicy.MaxRetries = 1

	t.Run("armA_with_fallback", func(t *testing.T) {
		e := newTestEnv(t, aiConf)
		defer e.Close()

		resp, body, err := e.sendRequest(defaultBody)
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200 from fallback cluster, got %d, body: %s", resp.StatusCode, body)
		}
		// Boundary (issue #1441 round 2): arm A's final response comes from
		// the fallback upstream, not the gateway, so it must NOT carry the
		// §3.3 correlation headers written by AiError.CreateErrorResponse.
		if h := resp.Header.Get("X-Bfe-Log-Id"); h != "" {
			t.Errorf("upstream 200 response must not carry X-Bfe-Log-Id, got %q", h)
		}

		// The exhausted cluster must see zero upstream interaction.
		if hits := e.backends[clusterMultiKey].Hits(); hits != 0 {
			t.Fatalf("expected 0 upstream hits on exhausted cluster, got %d", hits)
		}
		if hits := e.backends[clusterFallbackOK].Hits(); hits != 1 {
			t.Fatalf("expected fallback cluster hit once, got %d", hits)
		}
		if !e.bfeLogContains("all ai keys exhausted for cluster[cluster_multi_key], return synthetic 429") {
			t.Error("expected 'all ai keys exhausted ... return synthetic 429' in bfe.log")
		}
		if !e.bfeLogContains("fallback triggered, cluster[cluster_multi_key]") {
			t.Error("expected 'fallback triggered' in bfe.log")
		}
	})

	t.Run("armB_without_fallback", func(t *testing.T) {
		e := newTestEnv(t, aiConf)
		defer e.Close()

		resp, body, err := e.sendRequestToHostWithHeaders(apiHostNoFallback, defaultBody,
			map[string]string{"X-Request-ID": "tc09-req-id"})
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			e.logBFEException()
			t.Fatalf("expected status 429, got %d, body: %s", resp.StatusCode, body)
		}
		if !strings.Contains(body, `"type":"rate_limit_error"`) {
			t.Errorf("expected JSON error body with rate_limit_error, got: %s", body)
		}
		// issue #1441: unified error body must carry error.code (§3.2 必填).
		if !strings.Contains(body, `"code":"NO_AVAILABLE_KEY"`) {
			t.Errorf("expected JSON error body with code NO_AVAILABLE_KEY, got: %s", body)
		}
		// issue #1441: backoff signal - Retry-After header and
		// details.retry_after_seconds (429 penalty TTL, 60s).
		if ra := resp.Header.Get("Retry-After"); ra != "60" {
			t.Errorf("expected Retry-After 60, got %q", ra)
		}
		if !strings.Contains(body, `"retry_after_seconds":60`) {
			t.Errorf("expected details.retry_after_seconds=60 in body, got: %s", body)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("expected application/json content type, got %q", ct)
		}
		// issue #1441 round 2: correlation headers (ai_error_codes.md §3.3).
		// This scenario does not load mod_logid, so the gateway generates the
		// id (fallback chain) and back-fills req.LogId.
		if logId := resp.Header.Get("X-Bfe-Log-Id"); !isDecimalLogIdInRange(logId) {
			t.Errorf("expected non-empty decimal X-Bfe-Log-Id, got %q", logId)
		}
		if rid := resp.Header.Get("X-Request-ID"); rid != "tc09-req-id" {
			t.Errorf("expected echoed X-Request-ID %q, got %q", "tc09-req-id", rid)
		}
		if hits := e.backends[clusterMultiKey].Hits(); hits != 0 {
			t.Fatalf("expected 0 upstream hits on exhausted cluster, got %d", hits)
		}
	})
}

// TestTC10 verifies issue #1441 step 3 + round 2 on the gateway-side
// synthesized 502: when the target cluster's backend is unreachable (dial
// fails), ServeHTTPForAI returns the unified BACKEND_UNAVAILABLE body (not a
// bare 500) and, like the synthetic 429, carries the §3.3 correlation
// headers - the "upstream unavailable" representative proving the headers
// are not a 429-only patch.
func TestTC10_GatewaySideFailureSynthesized502(t *testing.T) {
	e := newDeadBackendEnv(t)
	defer e.Close()

	resp, body, err := e.sendRequestToHostWithHeaders(apiHostDeadBackend, defaultBody,
		map[string]string{"X-Request-ID": "tc10-req-id"})
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadGateway {
		e.logBFEException()
		t.Fatalf("expected status 502, got %d, body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"code":"BACKEND_UNAVAILABLE"`) {
		t.Errorf("expected JSON error body with code BACKEND_UNAVAILABLE, got: %s", body)
	}
	if !strings.Contains(body, `"type":"internal_error"`) {
		t.Errorf("expected JSON error body with type internal_error, got: %s", body)
	}
	if resp.Header.Get("X-Bfe-Gw-Error") != "1" {
		t.Errorf("expected X-Bfe-Gw-Error: 1 on gateway-generated 502")
	}
	// regression guard: only the key-exhaustion 429 sets Retry-After.
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		t.Errorf("BACKEND_UNAVAILABLE must not carry Retry-After, got %q", ra)
	}
	// issue #1441 round 2: §3.3 correlation headers (mod_logid not loaded in
	// this scenario, so the id is gateway-generated and back-filled).
	if logId := resp.Header.Get("X-Bfe-Log-Id"); !isDecimalLogIdInRange(logId) {
		t.Errorf("expected non-empty decimal X-Bfe-Log-Id, got %q", logId)
	}
	if rid := resp.Header.Get("X-Request-ID"); rid != "tc10-req-id" {
		t.Errorf("expected echoed X-Request-ID %q, got %q", "tc10-req-id", rid)
	}
	// zero upstream bytes: the dead backend has no listener at all, and the
	// live mocks must not be hit either.
	if hits := e.backends[clusterMultiKey].Hits(); hits != 0 {
		t.Errorf("expected 0 hits on cluster_multi_key, got %d", hits)
	}
	if hits := e.backends[clusterFallbackOK].Hits(); hits != 0 {
		t.Errorf("expected 0 hits on cluster_fallback_ok, got %d", hits)
	}
	if !e.bfeLogContains("refused") {
		t.Error("expected backend dial failure (refused) recorded in bfe.log")
	}
}

// newDeadBackendEnv builds an env whose route host deadbackend.example.org
// targets cluster_dead_backend, a cluster whose single backend address has
// no listener (connection refused on dial).
func newDeadBackendEnv(t *testing.T) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	e.backends[clusterMultiKey] = common.NewMockBackend(clusterMultiKey, http.StatusOK, `{"ok":true}`)
	e.backends[clusterFallbackOK] = common.NewMockBackend(clusterFallbackOK, http.StatusOK, `{"ok":true}`)

	// grab a free port and release it: nothing will listen there, so the
	// BFE dial gets ECONNREFUSED immediately (fast, no retry backoff).
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve closed port failed: %v", err)
	}
	deadAddr := l.Addr().String()
	l.Close()

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	confDir := filepath.Join(e.processEnv.WorkDir(), "conf")
	logDir := filepath.Join(e.processEnv.WorkDir(), "log")

	deadAIConf := &cluster_conf.AIConf{
		Type: 0,
		Keys: []cluster_conf.AIKey{
			{Name: "key-dead", Key: "sk-key-dead", Weight: 100},
		},
		// fail fast on dial errors (no backoff sleep in the test)
		KeyPolicy: &cluster_conf.AIKeyPolicy{MaxRetries: 0},
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:    "testdata",
		TargetConfDir:  confDir,
		Backends:       e.backends,
		AIConfs:        map[string]*cluster_conf.AIConf{clusterDeadBackend: deadAIConf},
		StaticBackends: map[string]string{clusterDeadBackend: deadAddr},
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(confDir, logDir)
	return e
}
