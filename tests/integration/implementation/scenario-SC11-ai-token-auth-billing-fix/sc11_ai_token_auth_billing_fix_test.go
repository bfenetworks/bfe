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

package sc11

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
	apiHost  = "billing-fix.example.org"
	apiKey   = "ak_user_a"
	apiKeyId = "user_a_key_id"

	clusterBillingFix = "cluster_billing_fix"

	planRMB     = "plan_rmb"
	redisKeyRMB = "quota:plan_rmb"
)

var (
	claudeBody      = []byte(`{"model":"claude-opus-4-6"}`)
	deepseekBody    = []byte(`{"model":"deepseek-chat"}`)
	countTokensBody = []byte(`{"model":"claude-opus-4-6","messages":[{"role":"user","content":"hello"}]}`)

	// Anthropic usage: input_tokens only counts cache miss; cache_read_input_tokens is the real cache hit.
	anthropicHighCacheUsageResponse = `{"usage":{"input_tokens":320,"output_tokens":150,"cache_read_input_tokens":8000,"cache_creation_input_tokens":200}}`

	// Normal usage for deepseek-chat.
	deepseekUsageResponse = `{"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`

	// Count tokens endpoint returns a token count, not a billable completion.
	countTokensResponse = `{"input_tokens":100}`
)

type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backend    *common.MockBackend
	redis      *common.RedisServer
	bfePort    int
	stopBFE    func()
}

func newTestEnv(t *testing.T) *testEnv {
	e := &testEnv{t: t}

	e.backend = common.NewMockBackend(clusterBillingFix, http.StatusOK, deepseekUsageResponse)
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	confDir := filepath.Join(e.processEnv.WorkDir(), "conf")
	logDir := filepath.Join(e.processEnv.WorkDir(), "log")

	tokenRule := &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": {
				{
					Id:          planRMB,
					Unlimited:   false,
					PassNoQuota: false,
					RedisKey:    redisKeyRMB,
					ExpiredTime: -1,
					Quota:       10000000000,
					Unit:        "RMB",
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
					QuotaPlans:     []string{planRMB},
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

	aiConfs := map[string]*cluster_conf.AIConf{
		clusterBillingFix: billingFixAIConf(),
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: confDir,
		Backends:      map[string]*common.MockBackend{clusterBillingFix: e.backend},
		AIConfs:       aiConfs,
		RedisAddr:     e.redis.Addr(),
		TokenRuleData: tokenRule,
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
	if e.backend != nil {
		e.backend.Close()
	}
	if e.redis != nil {
		e.redis.Close()
	}
}

func (e *testEnv) logBFEException() {
	data, err := os.ReadFile(filepath.Join(e.processEnv.WorkDir(), "log", "exception.log"))
	if err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}
}

func (e *testEnv) sendRequest(host, path string, body []byte) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, path)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
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

func billingFixAIConf() *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type: 0,
		ModelMapping: &map[string]string{
			"gpt-4": "deepseek-chat",
		},
		Provider: "mock-provider",
		Keys: []cluster_conf.AIKey{
			{Name: "key-primary", Key: "sk-primary", Weight: 100},
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
					Model:               "claude-opus-4-6",
					BaseModel:           "claude-opus-4-6",
					Mode:                "chat",
					Capabilities:        []string{"chat"},
					SupportedParameters: []string{"temperature", "max_tokens"},
					Limits: map[string]interface{}{
						"context_window": 128000,
					},
					Prices: cluster_conf.PriceMap{
						"input_cost_per_token":            0.00000452,
						"output_cost_per_token":           0.00002262,
						"cache_read_input_token_cost":     0.00000045,
						"cache_creation_input_token_cost": 0.00000565,
					},
				},
				{
					Provider:            "mock-provider",
					Model:               "deepseek-chat",
					BaseModel:           "deepseek-chat",
					Mode:                "chat",
					Capabilities:        []string{"chat"},
					SupportedParameters: []string{"temperature", "max_tokens"},
					Limits: map[string]interface{}{
						"context_window": 128000,
					},
					Prices: cluster_conf.PriceMap{
						"input_cost_per_token":  0.000001,
						"output_cost_per_token": 0.000002,
					},
				},
			},
		},
	}
}

// TestTC01 verifies issue #1343: cache read tokens are not truncated to prompt tokens.
func TestTC01_AnthropicHighCacheHit(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)
	e.backend.Body = anthropicHighCacheUsageResponse

	resp, body, err := e.sendRequest(apiHost, "/v1/chat/completions", claudeBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backend.Hits() != 1 {
		t.Fatalf("expected 1 backend hit, got %d", e.backend.Hits())
	}

	// Wait for async redis deduction.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)

	// Anthropic usage is normalized at parse time:
	// PromptTokens = input_tokens + cache_read + cache_write = 320 + 8000 + 200 = 8520
	// normal_input = 8520 - 8000 - 200 = 320
	// cost = 320*452 + 8000*45 + 200*565 + 150*2262 = 956940
	want := int64(10000000000 - 956940)
	if remaining != want {
		t.Fatalf("remaining quota = %d, want %d", remaining, want)
	}
}

// TestTC02 verifies issue #1344: count_tokens endpoint is not billed.
func TestTC02_CountTokensNotBilled(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)
	e.backend.Body = countTokensResponse

	resp, body, err := e.sendRequest(apiHost, "/anthropic/v1/messages/count_tokens", countTokensBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backend.Hits() != 1 {
		t.Fatalf("expected 1 backend hit, got %d", e.backend.Hits())
	}

	// Wait for any potential async redis deduction.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	if remaining != int64(10000000000) {
		t.Fatalf("count_tokens should not deduct quota, remaining = %d, want 10000000000", remaining)
	}
}

// TestTC03 verifies issue #1345: a single successful request only deducts once.
// If HandleRequestFinish were triggered multiple times without the deducted guard,
// the remaining quota would be lower than the expected single-deduction amount.
func TestTC03_NoDuplicateDeduction(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)
	e.backend.Body = deepseekUsageResponse

	resp, body, err := e.sendRequest(apiHost, "/v1/chat/completions", deepseekBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backend.Hits() != 1 {
		t.Fatalf("expected 1 backend hit, got %d", e.backend.Hits())
	}

	// Wait for async redis deduction.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)

	// cost = 100*100 + 50*200 = 20000
	want := int64(10000000000 - 20000)
	if remaining != want {
		// If duplicate deduction happened, remaining would be less than want.
		t.Fatalf("remaining quota = %d, want %d (possible duplicate deduction)", remaining, want)
	}
}
