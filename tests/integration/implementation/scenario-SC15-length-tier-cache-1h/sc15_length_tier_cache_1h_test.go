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

package sc15

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bfenetworks/go-lib/quota"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost  = "sc15.example.org"
	apiPath  = "/v1/chat/completions"
	apiKey   = "ak_user_a"
	apiKeyId = "user_a_key_id"

	clusterSC15 = "cluster_sc15"

	planRMB     = "plan_rmb"
	redisKeyRMB = "quota:plan_rmb"

	initialQuota = int64(10000000000)
)

var tierAboveBody = []byte(`{"model":"gpt-5.5"}`)
var tierBelowBody = []byte(`{"model":"gpt-5.5"}`)
var cache1hBody = []byte(`{"model":"claude-opus-4-8"}`)
var cache1hFallbackBody = []byte(`{"model":"claude-opus-4-8"}`)
var cache1hNoPriceBody = []byte(`{"model":"claude-sonnet-5"}`)
var cache1hStreamBody = []byte(`{"model":"claude-opus-4-8","stream":true}`)

var tierAboveUsageResponse = `{"usage":{"prompt_tokens":300000,"completion_tokens":50000,"total_tokens":350000}}`

var tierBelowUsageResponse = `{"usage":{"prompt_tokens":100000,"completion_tokens":5000,"total_tokens":105000}}`

// OpenAI-style body carrying the Anthropic extended-TTL field
// usage.cache_creation.ephemeral_1h_input_tokens (1h portion included in
// cache_write_tokens).
var cache1hUsageResponse = `{"usage":{"prompt_tokens":20000,"completion_tokens":1000,"total_tokens":21000,"cache_write_tokens":15000,"cache_creation":{"ephemeral_1h_input_tokens":10000}}}`

// Relay flattened fallback field usage.cache_creation_input_tokens_1h.
var cache1hFallbackUsageResponse = `{"usage":{"prompt_tokens":20000,"completion_tokens":1000,"total_tokens":21000,"cache_write_tokens":15000,"cache_creation_input_tokens_1h":8000}}`

var cache1hStreamUsageResponse = "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
	"data: {\"usage\":{\"prompt_tokens\":20000,\"completion_tokens\":1000,\"total_tokens\":21000,\"cache_write_tokens\":15000,\"cache_creation\":{\"ephemeral_1h_input_tokens\":10000}}}\n\n"

// testEnv holds all resources for a single SC15 integration test.
type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backends   map[string]*common.MockBackend
	redis      *common.RedisServer
	bfePort    int
	stopBFE    func()
}

func newTestEnv(t *testing.T, aiConfs map[string]*cluster_conf.AIConf) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	e.backends[clusterSC15] = common.NewMockBackend(clusterSC15, http.StatusOK, tierAboveUsageResponse)

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
					Quota:       initialQuota,
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

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: confDir,
		Backends:      e.backends,
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
	for _, b := range e.backends {
		b.Close()
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

func (e *testEnv) logBFEAccess() {
	data, err := os.ReadFile(filepath.Join(e.processEnv.WorkDir(), "log", "access.log"))
	if err == nil && len(data) > 0 {
		e.t.Logf("bfe access log:\n%s", string(data))
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

func (e *testEnv) assertDeducted(want int64, body string) {
	e.t.Helper()
	// wait for async redis deduction
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	if remaining != initialQuota-want {
		e.logBFEException()
		e.logBFEAccess()
		e.t.Fatalf("remaining quota = %d, want %d (deducted %d), response body: %s",
			remaining, initialQuota-want, want, body)
	}
}

// sc15AIConf configures a ModelTable with:
//   - gpt-5.5: base prices plus 272k length-tier prices on both sides;
//   - claude-opus-4-8: base prices plus 5m cache write price and 1h price;
//   - claude-sonnet-5: base prices plus 5m cache write price only (no 1h),
//     for the no-1h-price regression arm.
func sc15AIConf() *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type:     0,
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
					Model:               "gpt-5.5",
					BaseModel:           "gpt-5.5",
					Mode:                "chat",
					Capabilities:        []string{"chat"},
					SupportedParameters: []string{"temperature", "max_tokens"},
					Limits: map[string]interface{}{
						"context_window": 400000,
					},
					Prices: cluster_conf.PriceMap{
						cluster_conf.PriceInputCostPerToken:                 2.431e-05,
						cluster_conf.PriceOutputCostPerToken:                0.00014586,
						cluster_conf.PriceCacheReadInputTokenCost:           2.431e-06,
						cluster_conf.PriceInputCostPerTokenAbove272kTokens:  4.862e-05,
						cluster_conf.PriceOutputCostPerTokenAbove272kTokens: 0.00021879,
					},
				},
				{
					Provider:            "mock-provider",
					Model:               "claude-opus-4-8",
					BaseModel:           "claude-opus-4-8",
					Mode:                "chat",
					Capabilities:        []string{"chat"},
					SupportedParameters: []string{"temperature", "max_tokens"},
					Limits: map[string]interface{}{
						"context_window": 200000,
					},
					Prices: cluster_conf.PriceMap{
						cluster_conf.PriceInputCostPerToken:             3.077e-05,
						cluster_conf.PriceOutputCostPerToken:            0.00015385,
						cluster_conf.PriceCacheCreationInputTokenCost:   3.84625e-05,
						cluster_conf.PriceCacheCreationInputTokenCost1h: 6.154e-05,
					},
				},
				{
					Provider:            "mock-provider",
					Model:               "claude-sonnet-5",
					BaseModel:           "claude-sonnet-5",
					Mode:                "chat",
					Capabilities:        []string{"chat"},
					SupportedParameters: []string{"temperature", "max_tokens"},
					Limits: map[string]interface{}{
						"context_window": 200000,
					},
					Prices: cluster_conf.PriceMap{
						cluster_conf.PriceInputCostPerToken:           3.077e-05,
						cluster_conf.PriceOutputCostPerToken:          0.00015385,
						cluster_conf.PriceCacheCreationInputTokenCost: 3.84625e-05,
					},
				},
			},
		},
	}
}

// TestTC01 verifies length-tier billing: 300k input tokens exceed the 272k
// tier, so the whole request is billed at the tier prices.
func TestTC01_LengthTier272k_AboveThreshold(t *testing.T) {
	e := newTestEnv(t, map[string]*cluster_conf.AIConf{clusterSC15: sc15AIConf()})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)
	e.backends[clusterSC15].Body = tierAboveUsageResponse

	resp, body, err := e.sendRequest(tierAboveBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if e.backends[clusterSC15].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterSC15, e.backends[clusterSC15].Hits())
	}

	want := quota.CalcCostUnits(300000, 4.862e-05) + quota.CalcCostUnits(50000, 0.00021879)
	// sanity: tier price must differ from the base price, otherwise the
	// assertion cannot prove tier selection took effect.
	base := quota.CalcCostUnits(300000, 2.431e-05) + quota.CalcCostUnits(50000, 0.00014586)
	if want == base {
		t.Fatalf("test setup error: tier price equals base price")
	}
	e.assertDeducted(want, body)
}

// TestTC02 is the below-threshold control arm: 100k input tokens do not
// exceed the 272k tier, so base prices apply.
func TestTC02_LengthTier272k_BelowThreshold(t *testing.T) {
	e := newTestEnv(t, map[string]*cluster_conf.AIConf{clusterSC15: sc15AIConf()})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)
	e.backends[clusterSC15].Body = tierBelowUsageResponse

	resp, body, err := e.sendRequest(tierBelowBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if e.backends[clusterSC15].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterSC15, e.backends[clusterSC15].Hits())
	}

	want := quota.CalcCostUnits(100000, 2.431e-05) + quota.CalcCostUnits(5000, 0.00014586)
	e.assertDeducted(want, body)
}

// TestTC03 verifies the 1h-TTL cache write split: with the 1h price
// configured, the 1h portion bills at the 1h price and the remainder at the
// 5m base price.
func TestTC03_CacheWrite1hSplit(t *testing.T) {
	e := newTestEnv(t, map[string]*cluster_conf.AIConf{clusterSC15: sc15AIConf()})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)
	e.backends[clusterSC15].Body = cache1hUsageResponse

	resp, body, err := e.sendRequest(cache1hBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if e.backends[clusterSC15].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterSC15, e.backends[clusterSC15].Hits())
	}

	// normal_input = 20000 - 15000 = 5000
	want := quota.CalcCostUnits(5000, 3.077e-05) +
		quota.CalcCostUnits(5000, 3.84625e-05) + // 5m cache write
		quota.CalcCostUnits(10000, 6.154e-05) + // 1h cache write
		quota.CalcCostUnits(1000, 0.00015385)
	e.assertDeducted(want, body)
}

// TestTC04 verifies the relay fallback field
// usage.cache_creation_input_tokens_1h feeds the same 1h split.
func TestTC04_CacheWrite1hFallbackField(t *testing.T) {
	e := newTestEnv(t, map[string]*cluster_conf.AIConf{clusterSC15: sc15AIConf()})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)
	e.backends[clusterSC15].Body = cache1hFallbackUsageResponse

	resp, body, err := e.sendRequest(cache1hFallbackBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if e.backends[clusterSC15].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterSC15, e.backends[clusterSC15].Hits())
	}

	want := quota.CalcCostUnits(5000, 3.077e-05) +
		quota.CalcCostUnits(7000, 3.84625e-05) + // 5m cache write
		quota.CalcCostUnits(8000, 6.154e-05) + // 1h cache write
		quota.CalcCostUnits(1000, 0.00015385)
	e.assertDeducted(want, body)
}

// TestTC05 is the no-1h-price regression arm: the upstream still reports a
// 1h portion, but without the 1h price the whole cache write bills at the
// base 5m price (identical to pre-change behavior).
func TestTC05_CacheWrite1hPriceNotConfigured_Regression(t *testing.T) {
	e := newTestEnv(t, map[string]*cluster_conf.AIConf{clusterSC15: sc15AIConf()})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)
	e.backends[clusterSC15].Body = cache1hUsageResponse

	resp, body, err := e.sendRequest(cache1hNoPriceBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if e.backends[clusterSC15].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterSC15, e.backends[clusterSC15].Hits())
	}

	want := quota.CalcCostUnits(5000, 3.077e-05) +
		quota.CalcCostUnits(15000, 3.84625e-05) + // all cache write at 5m base price
		quota.CalcCostUnits(1000, 0.00015385)
	e.assertDeducted(want, body)
}

// TestTC06 verifies the 1h split on the SSE streaming path.
func TestTC06_CacheWrite1hSplit_Streaming(t *testing.T) {
	e := newTestEnv(t, map[string]*cluster_conf.AIConf{clusterSC15: sc15AIConf()})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)
	e.backends[clusterSC15].ResponseHeaders = map[string]string{"Content-Type": "text/event-stream"}
	e.backends[clusterSC15].Body = cache1hStreamUsageResponse

	resp, body, err := e.sendRequest(cache1hStreamBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if e.backends[clusterSC15].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterSC15, e.backends[clusterSC15].Hits())
	}

	want := quota.CalcCostUnits(5000, 3.077e-05) +
		quota.CalcCostUnits(5000, 3.84625e-05) +
		quota.CalcCostUnits(10000, 6.154e-05) +
		quota.CalcCostUnits(1000, 0.00015385)
	e.assertDeducted(want, body)
}
