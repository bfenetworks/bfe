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

package sc07

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

	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost  = "ratelimit.example.org"
	apiPath  = "/v1/chat/completions"
	apiKey   = "ak_ratelimit"
	apiKeyId = "ratelimit_key_id"

	clusterRateLimit = "cluster_ratelimit"

	policyID = "rlp-0001"
	rpmRedisKey = "RL_RPM_rlp-0001_0"
)

var defaultBody = []byte(`{"model":"gpt-4"}`)
var usageResponse = `{"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`

// testEnv holds all resources for a single SC07 integration test.
type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backend    *common.MockBackend
	redis      *common.RedisServer
	confDir    string
	logDir     string
	bfePort    int
	stopBFE    func()
}

func newTestEnv(t *testing.T) *testEnv {
	e := &testEnv{t: t}

	e.backend = common.NewMockBackend(clusterRateLimit, http.StatusOK, usageResponse)
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	return e
}

func (e *testEnv) startBFE(rateLimitData *common.RateLimitPolicyData) {
	tokenRule := &common.TokenRuleData{
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
					UnlimitedQuota: false,
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

	backends := map[string]*common.MockBackend{
		clusterRateLimit: e.backend,
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:         "testdata",
		TargetConfDir:       e.confDir,
		Backends:            backends,
		RedisAddr:           e.redis.Addr(),
		TokenRuleData:       tokenRule,
		RateLimitPolicyData: rateLimitData,
	}
	if err := builder.Build(); err != nil {
		e.t.Fatalf("build bfe config failed: %v", err)
	}

	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
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
	return e.sendRequestWithBody(defaultBody)
}

func (e *testEnv) sendRequestWithBody(body []byte) (*http.Response, string, error) {
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

func rateLimitPolicy(ruleName, model, redisKey string) *common.RateLimitPolicyData {
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
			policyID: {
				Name:    "ratelimitX",
				Enabled: true,
				Rules: struct {
					TPM            []common.RateLimitRule `json:"tpm,omitempty"`
					RPM            []common.RateLimitRule `json:"rpm,omitempty"`
					MaxConcurrency *int64                 `json:"max_concurrency,omitempty"`
					Batch          *common.RateLimitBatchLimits `json:"batch,omitempty"`
				}{
					RPM: []common.RateLimitRule{
						{
							Name:          ruleName,
							WindowMinutes: 1,
							MaxRequests:   1,
							Burst:         1,
							Models:        []string{model},
							RedisKey:      redisKey,
						},
					},
				},
			},
		},
		ApikeyRateLimitPolicyBindings: map[string][]string{
			apiKey: {policyID},
		},
	}
}

// TestTC01 verifies that the RPM counter persists when the rule name changes
// but the redis_key stays the same.
func TestTC01_RPMCounterPersistsAcrossNameChange(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	// Start BFE with rule name "old-rule" and a stable redis_key.
	e.startBFE(rateLimitPolicy("old-rule", "gpt-4", rpmRedisKey))

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected first request 200, got %d, body: %s", resp.StatusCode, body)
	}

	resp, body, err = e.sendRequest()
	if err != nil {
		t.Fatalf("send second request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected second request 429, got %d, body: %s", resp.StatusCode, body)
	}

	// Restart BFE with the same redis_key but a different rule name.
	e.stopBFE()
	e.stopBFE = nil
	e.startBFE(rateLimitPolicy("new-rule", "gpt-4", rpmRedisKey))

	resp, body, err = e.sendRequest()
	if err != nil {
		t.Fatalf("send third request after rename failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected third request still 429 after rename, got %d, body: %s", resp.StatusCode, body)
	}
}

// TestTC02 verifies that the RPM counter persists when the rule model changes
// but the redis_key stays the same. The request body model is updated to match
// the new rule model so the rule is still evaluated.
func TestTC02_RPMCounterPersistsAcrossModelChange(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	// Start BFE with model "gpt-4" and a stable redis_key.
	e.startBFE(rateLimitPolicy("rule-1", "gpt-4", rpmRedisKey))

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected first request 200, got %d, body: %s", resp.StatusCode, body)
	}

	resp, body, err = e.sendRequest()
	if err != nil {
		t.Fatalf("send second request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected second request 429, got %d, body: %s", resp.StatusCode, body)
	}

	// Restart BFE with the same redis_key but a different model.
	e.stopBFE()
	e.stopBFE = nil
	e.startBFE(rateLimitPolicy("rule-1", "gpt-3.5", rpmRedisKey))

	// Send a request matching the new model; the counter should still persist.
	resp, body, err = e.sendRequestWithBody([]byte(`{"model":"gpt-3.5"}`))
	if err != nil {
		t.Fatalf("send third request after model change failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected third request still 429 after model change, got %d, body: %s", resp.StatusCode, body)
	}
}

// TestTC03 verifies backward compatibility: when redis_key is absent,
// BFE falls back to the original name-based Redis key.
func TestTC03_RPMFallbackToNameWhenRedisKeyMissing(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	// Start BFE without redis_key; BFE should build key from rule name.
	e.startBFE(rateLimitPolicy("name-based-rule", "gpt-4", ""))

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected first request 200, got %d, body: %s", resp.StatusCode, body)
	}

	resp, body, err = e.sendRequest()
	if err != nil {
		t.Fatalf("send second request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected second request 429, got %d, body: %s", resp.StatusCode, body)
	}
}
