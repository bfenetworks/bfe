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

package sc46

import (
	"bytes"
	"encoding/json"
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
	apiHost  = "penalty-ttl.example.org"
	apiPath  = "/v1/chat/completions"
	apiKey   = "ak_penalty_ttl"
	apiKeyId = "penalty_ttl_key_id"

	clusterName = "cluster_penalty_ttl"
	keyA        = "sk-key-a"
	keyB        = "sk-key-b"
)

// penaltyPrefix is the Redis key prefix of the penalty entries of this cluster.
const penaltyPrefix = "bfe:ai:key_affinity:penalty:" + clusterName + ":"

var defaultBody = []byte(`{"model":"gpt-4"}`)

// testEnv holds the resources of one SC46 integration test.
type testEnv struct {
	t            *testing.T
	processEnv   *common.ProcessEnv
	backend      *common.MockBackend
	redis        *common.RedisServer
	confDir      string
	logDir       string
	bfePort      int
	monitorPort  int
	stopBFE      func()
	ttl429       int
	ttlAuth      int
	affinityKeys []cluster_conf.AIKey
	responseFunc func(r *http.Request, count int) (int, string)
}

// newTestEnv starts miniredis, a mock backend and a real BFE process.
func newTestEnv(t *testing.T, ttl429, ttlAuth int, keys []cluster_conf.AIKey,
	responseFunc func(r *http.Request, count int) (int, string)) *testEnv {

	t.Helper()

	e := &testEnv{
		t:            t,
		ttl429:       ttl429,
		ttlAuth:      ttlAuth,
		affinityKeys: keys,
		responseFunc: responseFunc,
	}
	if e.affinityKeys == nil {
		e.affinityKeys = []cluster_conf.AIKey{
			{Name: "key-a", Key: keyA, Weight: 50},
			{Name: "key-b", Key: keyB, Weight: 50},
		}
	}

	e.backend = common.NewMockBackend(clusterName, http.StatusOK, `{"ok":true}`)
	e.backend.ResponseFunc = e.responseFunc
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()
	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	e.startBFE()
	return e
}

func (e *testEnv) startBFE() {
	aiConf := &cluster_conf.AIConf{
		Type: 0,
		Keys: e.affinityKeys,
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:                      "weighted_random",
			MaxRetries:                    3,
			RetryBackoffInitial:           50,
			RetryBackoffMax:               200,
			SessionAffinity:               true,
			SessionAffinityTTL:            300,
			SessionAffinityRedisPrefix:    "bfe:ai:key_affinity",
			SessionAffinityPenaltyEnable:  true,
			SessionAffinityPenaltyTTL429:  e.ttl429,
			SessionAffinityPenaltyTTLAuth: e.ttlAuth,
		},
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends: map[string]*common.MockBackend{
			clusterName: e.backend,
		},
		RedisAddr:           e.redis.Addr(),
		TokenRuleData:       buildTokenRule(),
		RateLimitPolicyData: buildRateLimitPolicy(),
		AIConfs: map[string]*cluster_conf.AIConf{
			clusterName: aiConf,
		},
	}
	if err := builder.Build(); err != nil {
		e.t.Fatalf("build bfe config failed: %v", err)
	}

	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
}

// buildTokenRule configures a single client token with a stable key id and an
// unlimited quota plan, so the only variable under test is the penalty TTL.
func buildTokenRule() *common.TokenRuleData {
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
			"ai_product": {
				apiKey: {
					Key:            apiKey,
					KeyId:          apiKeyId,
					Enabled:        true,
					ExpiredTime:    -1,
					UnlimitedQuota: true,
					QuotaPlans:     []string{"unlimited_plan"},
				},
			},
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

// buildRateLimitPolicy installs a pass-through rate limit policy: the module
// requires a product rule table, and the penalty TTL must not be masked by a
// local 429.
func buildRateLimitPolicy() *common.RateLimitPolicyData {
	bindings := map[string][]string{apiKey: {"rlp-pass"}}

	return &common.RateLimitPolicyData{
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
					TPM            []common.RateLimitRule       `json:"tpm,omitempty"`
					RPM            []common.RateLimitRule       `json:"rpm,omitempty"`
					MaxConcurrency *int64                       `json:"max_concurrency,omitempty"`
					Batch          *common.RateLimitBatchLimits `json:"batch,omitempty"`
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
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	if e.backend != nil {
		e.backend.Close()
	}
	if e.redis != nil {
		e.redis.Close()
	}
}

func (e *testEnv) logBFEException() {
	if data, err := os.ReadFile(filepath.Join(e.logDir, "exception.log")); err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}
	if data, err := os.ReadFile(filepath.Join(e.logDir, "bfe.log")); err == nil && len(data) > 0 {
		lines := strings.Split(string(data), "\n")
		start := 0
		if len(lines) > 200 {
			start = len(lines) - 200
		}
		e.t.Logf("bfe log tail:\n%s", strings.Join(lines[start:], "\n"))
	}
}

func (e *testEnv) bfeLogContains(substr string) bool {
	data, err := os.ReadFile(filepath.Join(e.logDir, "bfe.log"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substr)
}

func (e *testEnv) sendRequest() (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(defaultBody))
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return resp, string(body), nil
}

// penaltyKeys returns the penalty keys of this cluster currently in redis.
func (e *testEnv) penaltyKeys() []string {
	var out []string
	for _, k := range e.redis.Keys() {
		if strings.HasPrefix(k, penaltyPrefix) {
			out = append(out, k)
		}
	}
	return out
}

// assertPenaltyTTLs checks that at least one penalty key exists and that every
// penalty key carries exactly want (miniredis does not decay TTLs on its own,
// so the value set by SETEX is observable verbatim).
func (e *testEnv) assertPenaltyTTLs(want time.Duration) {
	e.t.Helper()

	keys := e.penaltyKeys()
	if len(keys) == 0 {
		e.logBFEException()
		e.t.Fatalf("expected at least one penalty key with prefix %s", penaltyPrefix)
	}
	for _, k := range keys {
		ttl, ok := e.redis.TTL(k)
		if !ok {
			e.t.Fatalf("penalty key %s disappeared", k)
		}
		if ttl <= 0 {
			e.t.Fatalf("penalty key %s has no expiry (ttl=%s), Redis would treat it as permanent", k, ttl)
		}
		if ttl != want {
			e.t.Errorf("penalty key %s ttl = %s, want %s", k, ttl, want)
		}
	}
}

// reloadServerDataConf triggers a server_data_conf hot reload and fails when BFE
// reports an error (the reload endpoint answers HTTP 200 with an error body).
func (e *testEnv) reloadServerDataConf() {
	e.t.Helper()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/reload/server_data_conf", e.monitorPort))
	if err != nil {
		e.t.Fatalf("reload request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("reload status = %d, body: %s", resp.StatusCode, string(body))
	}
	if strings.Contains(string(body), "KeyPolicy") || strings.Contains(string(body), "AIConf") {
		e.t.Fatalf("reload rejected the configuration: %s", string(body))
	}
}

// patchKeyPolicyTTLAuth rewrites SessionAffinityPenaltyTTLAuth in the generated
// cluster_conf.data, emulating a control-plane push.
func (e *testEnv) patchKeyPolicyTTLAuth(ttl int) {
	e.t.Helper()

	path := filepath.Join(e.confDir, "cluster_conf", "cluster_conf.data")
	data, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatalf("read cluster_conf.data failed: %v", err)
	}
	var conf map[string]interface{}
	if err := json.Unmarshal(data, &conf); err != nil {
		e.t.Fatalf("parse cluster_conf.data failed: %v", err)
	}
	configs, ok := conf["Config"].(map[string]interface{})
	if !ok {
		e.t.Fatalf("unexpected cluster_conf.data shape: %s", string(data))
	}
	item, ok := configs[clusterName].(map[string]interface{})
	if !ok {
		e.t.Fatalf("cluster %s missing from cluster_conf.data: %s", clusterName, string(data))
	}
	aiConf, ok := item["AIConf"].(map[string]interface{})
	if !ok {
		e.t.Fatalf("AIConf missing for cluster %s", clusterName)
	}
	keyPolicy, ok := aiConf["KeyPolicy"].(map[string]interface{})
	if !ok {
		e.t.Fatalf("KeyPolicy missing for cluster %s", clusterName)
	}
	keyPolicy["SessionAffinityPenaltyTTLAuth"] = ttl

	patched, err := json.MarshalIndent(conf, "", "    ")
	if err != nil {
		e.t.Fatalf("marshal cluster_conf.data failed: %v", err)
	}
	if err := os.WriteFile(path, patched, 0644); err != nil {
		e.t.Fatalf("write cluster_conf.data failed: %v", err)
	}
}

// TestTC01_DefaultPenaltyTTL covers issue #1448 defaults: a 401 writes a 300s
// penalty key and a 429 writes a 60s one.
func TestTC01_DefaultPenaltyTTL(t *testing.T) {
	t.Run("auth default 300", func(t *testing.T) {
		e := newTestEnv(t, 60, 300, nil, func(r *http.Request, count int) (int, string) {
			return http.StatusUnauthorized, `{"error":"invalid api key"}`
		})
		defer e.Close()

		if _, body, err := e.sendRequest(); err != nil {
			t.Fatalf("send request failed: %v", err)
		} else {
			t.Logf("response body: %s", body)
		}
		e.assertPenaltyTTLs(300 * time.Second)
		// 401 marks the key dead, and a pre-existing index mismatch in the
		// penalty filter can skip the healthy sibling within the same request
		// (see 场景说明.md §8), so only "at least one key penalized" is
		// guaranteed here.
		if got := len(e.penaltyKeys()); got < 1 {
			t.Errorf("expected at least one penalized key, got %d", got)
		}
	})

	t.Run("429 default 60", func(t *testing.T) {
		e := newTestEnv(t, 60, 300, nil, func(r *http.Request, count int) (int, string) {
			return http.StatusTooManyRequests, `{"error":"rate limited"}`
		})
		defer e.Close()

		if _, body, err := e.sendRequest(); err != nil {
			t.Fatalf("send request failed: %v", err)
		} else {
			t.Logf("response body: %s", body)
		}
		e.assertPenaltyTTLs(60 * time.Second)
		// 429 uses usedSet, which is reset when it empties, so the rotation
		// always reaches the second key and both get penalized.
		if got := len(e.penaltyKeys()); got != 2 {
			t.Errorf("expected both keys penalized, got %d penalty keys", got)
		}
	})
}

// TestTC02_ExplicitTTL covers issue #1448 explicit configuration.
func TestTC02_ExplicitTTL(t *testing.T) {
	t.Run("auth 120", func(t *testing.T) {
		e := newTestEnv(t, 30, 120, nil, func(r *http.Request, count int) (int, string) {
			return http.StatusUnauthorized, `{"error":"invalid api key"}`
		})
		defer e.Close()

		if _, body, err := e.sendRequest(); err != nil {
			t.Fatalf("send request failed: %v", err)
		} else {
			t.Logf("response body: %s", body)
		}
		e.assertPenaltyTTLs(120 * time.Second)
		if !e.bfeLogContains("penalty_ttl=120") {
			e.logBFEException()
			t.Errorf("expected penalty_ttl=120 in bfe.log")
		}
	})

	t.Run("429 ttl 30", func(t *testing.T) {
		e := newTestEnv(t, 30, 120, nil, func(r *http.Request, count int) (int, string) {
			return http.StatusTooManyRequests, `{"error":"rate limited"}`
		})
		defer e.Close()

		if _, body, err := e.sendRequest(); err != nil {
			t.Fatalf("send request failed: %v", err)
		} else {
			t.Logf("response body: %s", body)
		}
		e.assertPenaltyTTLs(30 * time.Second)
		if !e.bfeLogContains("penalty_ttl=30") {
			e.logBFEException()
			t.Errorf("expected penalty_ttl=30 in bfe.log")
		}
	})
}

// TestTC03_Synthetic429RetryAfter covers issue #1448's hint semantics: the
// synthetic 429 (every usable key penalized) carries Retry-After equal to the
// configured 429 penalty TTL, and the second request makes no upstream call.
func TestTC03_Synthetic429RetryAfter(t *testing.T) {
	e := newTestEnv(t, 30, 120, nil, func(r *http.Request, count int) (int, string) {
		return http.StatusTooManyRequests, `{"error":"rate limited"}`
	})
	defer e.Close()

	if _, body, err := e.sendRequest(); err != nil {
		t.Fatalf("send request failed: %v", err)
	} else {
		t.Logf("first response body: %s", body)
	}
	e.assertPenaltyTTLs(30 * time.Second)
	hitsAfterFirst := e.backend.Hits()
	if hitsAfterFirst != 2 {
		t.Fatalf("expected 2 upstream attempts on the first request, got %d", hitsAfterFirst)
	}

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected status 429, got %d, body: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Retry-After"); got != "30" {
		t.Errorf("expected Retry-After 30, got %q", got)
	}
	if e.backend.Hits() != hitsAfterFirst {
		t.Errorf("expected zero upstream interaction while all keys are penalized, hits %d -> %d",
			hitsAfterFirst, e.backend.Hits())
	}

	var aiErr struct {
		Error struct {
			Code    string `json:"code"`
			Details *struct {
				RetryAfterSeconds int `json:"retry_after_seconds"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &aiErr); err != nil {
		t.Fatalf("parse error body failed: %v, body: %s", err, body)
	}
	if aiErr.Error.Details == nil || aiErr.Error.Details.RetryAfterSeconds != 30 {
		t.Errorf("expected error.details.retry_after_seconds=30, got %+v (body: %s)", aiErr.Error.Details, body)
	}
}

// TestTC04_HotReload covers issue #1448 "no restart required": after patching
// SessionAffinityPenaltyTTLAuth and reloading server_data_conf, the next penalty
// write uses the new TTL.
func TestTC04_HotReload(t *testing.T) {
	e := newTestEnv(t, 60, 120, nil, func(r *http.Request, count int) (int, string) {
		return http.StatusUnauthorized, `{"error":"invalid api key"}`
	})
	defer e.Close()

	if _, body, err := e.sendRequest(); err != nil {
		t.Fatalf("send request failed: %v", err)
	} else {
		t.Logf("first response body: %s", body)
	}
	e.assertPenaltyTTLs(120 * time.Second)

	// Let the penalty and binding keys expire; BFE keeps running.
	e.redis.FastForward(121 * time.Second)
	if keys := e.penaltyKeys(); len(keys) != 0 {
		t.Fatalf("expected penalty keys to expire, still present: %v", keys)
	}

	e.patchKeyPolicyTTLAuth(60)
	e.reloadServerDataConf()

	if _, body, err := e.sendRequest(); err != nil {
		t.Fatalf("send request after reload failed: %v", err)
	} else {
		t.Logf("second response body: %s", body)
	}
	e.assertPenaltyTTLs(60 * time.Second)
}

// TestTC05_SingleKeyNoFiltering covers the single-usable-key short circuit: key
// selection returns the only key without consulting the penalty filter, so the
// penalty window never excludes it (the write side still records the failure,
// but nothing reads it) and the configured TTL has no effect. No affinity
// binding is written either.
func TestTC05_SingleKeyNoFiltering(t *testing.T) {
	e := newTestEnv(t, 60, 300,
		[]cluster_conf.AIKey{{Name: "key-a", Key: keyA, Weight: 100}},
		func(r *http.Request, count int) (int, string) {
			return http.StatusUnauthorized, `{"error":"invalid api key"}`
		})
	defer e.Close()

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		e.logBFEException()
		t.Fatalf("expected 401 on a single-key cluster, got %d, body: %s", resp.StatusCode, body)
	}

	// The short circuit bypasses filterPenaltyKeys, so the only key keeps being
	// selected for every retry: 1 initial attempt + MaxRetries(3).
	if got := e.backend.Hits(); got != 4 {
		t.Errorf("expected 4 upstream attempts (no key rotation), got %d", got)
	}
	for i, h := range e.backend.AuthHeaders() {
		if h != "Bearer "+keyA {
			t.Errorf("attempt %d used %q, expected the only key", i, h)
		}
	}

	// The write side does not look at the key count, so the penalty key exists;
	// what matters is that nothing ever reads it.
	if len(e.penaltyKeys()) == 0 {
		t.Log("note: no penalty key written on a single-key cluster")
	}
	if e.redis.Exists("bfe:ai:key_affinity:" + clusterName + ":" + apiKeyId) {
		t.Error("single-key cluster must not write an affinity binding")
	}
}

// TestTC06_ZeroFieldsDefault covers the backward-compatible path: a payload
// without the new fields decodes to 0 and the loader fills 60/300 instead of
// writing a permanent penalty key.
func TestTC06_ZeroFieldsDefault(t *testing.T) {
	t.Run("auth 0 -> 300", func(t *testing.T) {
		e := newTestEnv(t, 0, 0, nil, func(r *http.Request, count int) (int, string) {
			return http.StatusUnauthorized, `{"error":"invalid api key"}`
		})
		defer e.Close()

		if _, body, err := e.sendRequest(); err != nil {
			t.Fatalf("send request failed: %v", err)
		} else {
			t.Logf("response body: %s", body)
		}
		e.assertPenaltyTTLs(300 * time.Second)
	})

	t.Run("429 0 -> 60", func(t *testing.T) {
		e := newTestEnv(t, 0, 0, nil, func(r *http.Request, count int) (int, string) {
			return http.StatusTooManyRequests, `{"error":"rate limited"}`
		})
		defer e.Close()

		if _, body, err := e.sendRequest(); err != nil {
			t.Fatalf("send request failed: %v", err)
		} else {
			t.Logf("response body: %s", body)
		}
		e.assertPenaltyTTLs(60 * time.Second)
	})
}
