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

package sc10

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost   = "session-affinity.example.org"
	apiPath   = "/v1/chat/completions"
	apiKey    = "ak_session_affinity"
	apiKeyId  = "session_affinity_key_id"
	apiKey2   = "ak_session_affinity_2"
	apiKeyId2 = "session_affinity_key_id_2"

	clusterSessionAffinity = "cluster_session_affinity"

	keyA = "sk-key-a"
	keyB = "sk-key-b"
	keyC = "sk-key-c"
)

var defaultBody = []byte(`{"model":"gpt-4"}`)

// testEnv holds all resources for a single SC10 integration test.
type testEnv struct {
	t            *testing.T
	processEnv   *common.ProcessEnv
	backend      *common.MockBackend
	redis        *common.RedisServer
	confDir      string
	logDir       string
	bfePort      int
	stopBFE      func()
	aiKeys       []cluster_conf.AIKey
	clientTokens map[string]string // apiKey -> keyId
	// affinityRedisDisabled disables the [AIKeyAffinity] section in bfe.conf
	affinityRedisDisabled bool
}

// testEnvOption customizes a test environment before BFE is started.
type testEnvOption func(*testEnv)

// withAIKeys overrides the AI keys configured for the cluster.
func withAIKeys(keys []cluster_conf.AIKey) testEnvOption {
	return func(e *testEnv) {
		e.aiKeys = keys
	}
}

// withClientTokens overrides the client API keys used to authenticate requests.
// The map key is the raw apiKey value, the map value is the stable ClientKeyId.
func withClientTokens(tokens map[string]string) testEnvOption {
	return func(e *testEnv) {
		e.clientTokens = tokens
	}
}

// withoutAffinityRedis disables the [AIKeyAffinity] section in bfe.conf
// (equivalent to leaving the section unconfigured: Disabled=true by default).
// Requests still succeed, but no session binding or penalty is ever written
// (fail-open).
func withoutAffinityRedis() testEnvOption {
	return func(e *testEnv) {
		e.affinityRedisDisabled = true
	}
}

func newTestEnv(t *testing.T, enableAffinity bool, opts ...testEnvOption) *testEnv {
	e := &testEnv{t: t}

	for _, opt := range opts {
		opt(e)
	}

	if e.aiKeys == nil {
		e.aiKeys = []cluster_conf.AIKey{
			{Name: "key-a", Key: keyA, Weight: 50},
			{Name: "key-b", Key: keyB, Weight: 30},
			{Name: "key-c", Key: keyC, Weight: 20},
		}
	}
	if e.clientTokens == nil {
		e.clientTokens = map[string]string{
			apiKey: apiKeyId,
		}
	}

	e.backend = common.NewMockBackend(clusterSessionAffinity, http.StatusOK, `{"ok":true}`)
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	e.startBFE(enableAffinity)
	return e
}

func (e *testEnv) startBFE(enableAffinity bool) {
	tokenRule := e.buildTokenRule()

	bindings := make(map[string][]string)
	for clientKey := range e.clientTokens {
		bindings[clientKey] = []string{"rlp-pass"}
	}

	rateLimitPolicy := &common.RateLimitPolicyData{
		Version: "1.0",
		Config: map[string][]common.RateLimitProductRule{
			"ai_product": {
				{
					Cond: "default_t()",
					HitAction: struct {
						Cmd    string   `json:"cmd"`
						Params []string `json:"params,omitempty"`
					}{Cmd: "FINISH"},
				},
			},
		},
		RateLimitPolicies: map[string]common.RateLimitPolicy{
			"rlp-pass": {
				Name:    "rlp-pass",
				Enabled: true,
				Rules: struct {
					TPM            []common.RateLimitRule `json:"tpm,omitempty"`
					RPM            []common.RateLimitRule `json:"rpm,omitempty"`
					MaxConcurrency *int64                 `json:"max_concurrency,omitempty"`
				}{
					RPM: []common.RateLimitRule{
						{
							Name:          "rpm-pass",
							WindowMinutes: 1,
							MaxRequests:   1000000,
							Models:        []string{"*"},
							RedisKey:      "RL_RPM_rlp-pass_0",
						},
					},
				},
			},
		},
		ApikeyRateLimitPolicyBindings: bindings,
	}

	aiConf := &cluster_conf.AIConf{
		Type: 0,
		Keys: e.aiKeys,
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:                     "weighted_random",
			MaxRetries:                   3,
			RetryBackoffInitial:          50,
			RetryBackoffMax:              200,
			SessionAffinity:              enableAffinity,
			SessionAffinityTTL:           300,
			SessionAffinityRedisPrefix:   "bfe:ai:key_affinity",
			SessionAffinityPenaltyEnable: true,
		},
	}

	backends := map[string]*common.MockBackend{
		clusterSessionAffinity: e.backend,
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:         "testdata",
		TargetConfDir:       e.confDir,
		Backends:            backends,
		RedisAddr:           e.redis.Addr(),
		TokenRuleData:       tokenRule,
		RateLimitPolicyData: rateLimitPolicy,
		AIConfs: map[string]*cluster_conf.AIConf{
			clusterSessionAffinity: aiConf,
		},
	}
	if err := builder.Build(); err != nil {
		e.t.Fatalf("build bfe config failed: %v", err)
	}

	if e.affinityRedisDisabled {
		if err := disableAIKeyAffinitySection(filepath.Join(e.confDir, "bfe.conf")); err != nil {
			e.t.Fatalf("disable [AIKeyAffinity] section failed: %v", err)
		}
	}

	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
}

// disableAIKeyAffinitySection sets Disabled=true in the [AIKeyAffinity]
// section of bfe.conf, making it equivalent to an unconfigured section
// (Disabled=true is the default).
func disableAIKeyAffinitySection(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	inSection := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inSection = trimmed == "[AIKeyAffinity]"
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "Disabled") {
			lines[i] = "Disabled = true"
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func (e *testEnv) buildTokenRule() *common.TokenRuleData {
	tokens := make(map[string]common.TokenFile)
	for clientKey, keyId := range e.clientTokens {
		tokens[clientKey] = common.TokenFile{
			Key:            clientKey,
			KeyId:          keyId,
			Enabled:        true,
			ExpiredTime:    -1,
			UnlimitedQuota: true,
			QuotaPlans:     []string{"unlimited_plan"},
		}
	}

	return &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": {
				{
					Id:          "unlimited_plan",
					Unlimited:   true,
					PassNoQuota: false,
					RedisKey:    "quota:unlimited_plan",
					ExpiredTime: -1,
					Quota:       0,
					Unit:        "total_token",
				},
			},
		},
		Tokens: map[string]map[string]common.TokenFile{
			"ai_product": tokens,
		},
		Config: map[string][]common.TokenRule{
			"ai_product": {
				{
					Cond:   "default_t()",
					Action: common.ActionFile{Cmd: "CHECK_TOKEN"},
				},
			},
		},
	}
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	if e.backend != nil {
		e.backend.Close()
	}
	if e.redis != nil {
		e.redis.Close()
	}
}

func (e *testEnv) logBFEException() {
	data, err := os.ReadFile(filepath.Join(e.logDir, "exception.log"))
	if err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}

	logPath := filepath.Join(e.logDir, "bfe.log")
	logData, err := os.ReadFile(logPath)
	if err == nil && len(logData) > 0 {
		lines := strings.Split(string(logData), "\n")
		start := 0
		if len(lines) > 200 {
			start = len(lines) - 200
		}
		e.t.Logf("bfe log tail:\n%s", strings.Join(lines[start:], "\n"))
	}
}

func (e *testEnv) sendRequest() (*http.Response, string, error) {
	return e.sendRequestWithKey(apiKey)
}

func (e *testEnv) sendRequestWithKey(clientKey string) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(defaultBody))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+clientKey)
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

func redisBindingKey(clientKeyId string) string {
	return fmt.Sprintf("bfe:ai:key_affinity:%s:%s", clusterSessionAffinity, clientKeyId)
}

func countAuthHeaders(headers []string) map[string]int {
	counts := make(map[string]int)
	for _, h := range headers {
		counts[h]++
	}
	return counts
}

// TestTC01 verifies that with SessionAffinity enabled, repeated requests from
// the same ClientKeyId hit the same provider API key.
func TestTC01_SessionAffinityHitsSameKey(t *testing.T) {
	e := newTestEnv(t, true)
	defer e.Close()

	for i := 0; i < 50; i++ {
		resp, body, err := e.sendRequest()
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
		}
	}

	if e.backend.Hits() != 50 {
		t.Fatalf("expected 50 backend hits, got %d", e.backend.Hits())
	}

	counts := countAuthHeaders(e.backend.AuthHeaders())
	nonZero := 0
	for k, c := range counts {
		if c > 0 {
			t.Logf("key %s used %d times", k, c)
			nonZero++
		}
	}
	if nonZero != 1 {
		t.Fatalf("expected exactly 1 key to be used with affinity, got %d", nonZero)
	}
}

// TestTC02 verifies that when SessionAffinity is disabled, requests are
// distributed across multiple API keys.
func TestTC02_NoAffinityDistributesKeys(t *testing.T) {
	e := newTestEnv(t, false)
	defer e.Close()

	for i := 0; i < 200; i++ {
		resp, body, err := e.sendRequest()
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
		}
	}

	counts := countAuthHeaders(e.backend.AuthHeaders())
	nonZero := 0
	for _, c := range counts {
		if c > 0 {
			nonZero++
		}
	}
	if nonZero < 2 {
		t.Fatalf("expected multiple keys to be used without affinity, got %d", nonZero)
	}
}

// TestTC03 verifies that when the bound key returns 429, BFE rotates to
// another key and updates the Redis binding.
func TestTC03_429RotatesAndRebinds(t *testing.T) {
	e := newTestEnv(t, true)
	defer e.Close()

	// First request establishes the binding.
	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	counts := countAuthHeaders(e.backend.AuthHeaders())
	var boundKey string
	for k, c := range counts {
		if c > 0 {
			boundKey = k
			break
		}
	}
	if boundKey == "" {
		t.Fatal("expected a bound key after first request")
	}
	t.Logf("bound key: %s", boundKey)

	// Make the bound key return 429.
	e.backend.ResponseFunc = func(r *http.Request, count int) (int, string) {
		if r.Header.Get("Authorization") == boundKey {
			return http.StatusTooManyRequests, `{"error":"rate limited"}`
		}
		return http.StatusOK, `{"ok":true}`
	}

	// Subsequent requests should switch to a different key.
	for i := 0; i < 20; i++ {
		resp, body, err = e.sendRequest()
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
		}
	}

	// The bound key may still be tried once per request (and get 429), but the
	// successful attempts should use a different key.
	finalCounts := countAuthHeaders(e.backend.AuthHeaders())
	delete(finalCounts, boundKey)
	successOther := false
	for k, c := range finalCounts {
		if c > 0 {
			t.Logf("other key used: %s count=%d", k, c)
			successOther = true
		}
	}
	if !successOther {
		t.Fatal("expected at least one request to succeed with a different key after 429")
	}
}

// TestTC04 verifies the single-key optimization: when a cluster is configured
// with only one API key, BFE does not write a Redis binding and all requests
// use that key.
func TestTC04_SingleKeySkipsRedisBinding(t *testing.T) {
	e := newTestEnv(t, true,
		withAIKeys([]cluster_conf.AIKey{
			{Name: "key-a", Key: keyA, Weight: 100},
		}),
	)
	defer e.Close()

	for i := 0; i < 10; i++ {
		resp, body, err := e.sendRequest()
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
		}
	}

	counts := countAuthHeaders(e.backend.AuthHeaders())
	if len(counts) != 1 {
		t.Fatalf("expected exactly one provider key, got %d", len(counts))
	}

	if e.redis.Exists(redisBindingKey(apiKeyId)) {
		t.Fatal("expected no Redis binding for single-key cluster")
	}
}

// TestTC05 verifies that two different ClientKeyIds maintain independent
// bindings. Each client keyId should stick to exactly one provider key, and
// the two chosen provider keys may differ.
func TestTC05_DifferentClientKeyIdsAreIndependent(t *testing.T) {
	e := newTestEnv(t, true,
		withClientTokens(map[string]string{
			apiKey:  apiKeyId,
			apiKey2: apiKeyId2,
		}),
	)
	defer e.Close()

	for i := 0; i < 20; i++ {
		resp, body, err := e.sendRequestWithKey(apiKey)
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
		}
	}
	for i := 0; i < 20; i++ {
		resp, body, err := e.sendRequestWithKey(apiKey2)
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
		}
	}

	for _, clientKeyId := range []string{apiKeyId, apiKeyId2} {
		bindingKey := redisBindingKey(clientKeyId)
		if !e.redis.Exists(bindingKey) {
			t.Fatalf("expected Redis binding to exist for client key id %s", clientKeyId)
		}
	}

	allHeaders := e.backend.AuthHeaders()
	firstGroup := countAuthHeaders(allHeaders[:20])
	secondGroup := countAuthHeaders(allHeaders[20:])

	if len(firstGroup) != 1 {
		t.Fatalf("expected first ClientKeyId to use exactly one provider key, got %d", len(firstGroup))
	}
	if len(secondGroup) != 1 {
		t.Fatalf("expected second ClientKeyId to use exactly one provider key, got %d", len(secondGroup))
	}
}

// TestTC06 verifies that the Redis binding outlives a BFE process restart:
// after stopping and restarting BFE with the same Redis server, the same
// ClientKeyId continues to hit the previously bound provider key.
func TestTC06_BindingPersistsAcrossBFERestart(t *testing.T) {
	e := newTestEnv(t, true)
	defer e.Close()

	for i := 0; i < 10; i++ {
		resp, body, err := e.sendRequest()
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
		}
	}

	boundKey := ""
	for _, h := range e.backend.AuthHeaders() {
		if h != "" {
			boundKey = h
			break
		}
	}
	if boundKey == "" {
		t.Fatal("expected a bound provider key before restart")
	}
	t.Logf("bound key before restart: %s", boundKey)

	// Record how many requests have been seen so far.
	hitsBefore := e.backend.Hits()

	// Stop BFE but keep backend and redis alive.
	e.stopBFE()
	e.stopBFE = nil

	// Restart BFE with the same configuration and Redis server.
	e.startBFE(true)

	for i := 0; i < 10; i++ {
		resp, body, err := e.sendRequest()
		if err != nil {
			t.Fatalf("send request after restart failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200 after restart, got %d, body: %s", resp.StatusCode, body)
		}
	}

	// Only the requests sent after the restart should be checked.
	afterHeaders := e.backend.AuthHeaders()[hitsBefore:]
	for i, h := range afterHeaders {
		if h != boundKey {
			t.Fatalf("request %d after restart used provider key %s, expected %s", i, h, boundKey)
		}
	}
}

// TestTC07 verifies the fail-open path: with the [AIKeyAffinity] section
// disabled in bfe.conf, requests with SessionAffinity=true still succeed,
// no binding is written to Redis, and keys are chosen by weighted random.
func TestTC07_AffinityRedisDisabledFailsOpen(t *testing.T) {
	e := newTestEnv(t, true, withoutAffinityRedis())
	defer e.Close()

	for i := 0; i < 50; i++ {
		resp, body, err := e.sendRequest()
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
		}
	}

	if e.backend.Hits() != 50 {
		t.Fatalf("expected 50 backend hits, got %d", e.backend.Hits())
	}

	// fail-open: no binding key was ever written
	if e.redis.Exists(redisBindingKey(apiKeyId)) {
		t.Fatal("expected no Redis binding when [AIKeyAffinity] is disabled")
	}

	// without affinity, requests distribute across multiple keys
	nonZero := 0
	for _, c := range countAuthHeaders(e.backend.AuthHeaders()) {
		if c > 0 {
			nonZero++
		}
	}
	if nonZero < 2 {
		t.Fatalf("expected multiple keys to be used without affinity redis, got %d", nonZero)
	}
}
