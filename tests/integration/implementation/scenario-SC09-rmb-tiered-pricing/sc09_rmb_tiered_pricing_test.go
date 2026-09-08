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

package sc09

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
	apiHost       = "tier.example.org"
	apiHostNoTier = "notier.example.org"
	apiPath       = "/v1/chat/completions"
	apiKey        = "ak_user_a"
	apiKeyId      = "user_a_key_id"

	// Reuse the well-known cluster/sub names from SC03 so that no common
	// helper changes are required.
	clusterTierPeak = "cluster_rmb"
	clusterNoTier   = "cluster_no_table"

	planRMB     = "plan_rmb"
	redisKeyRMB = "quota:plan_rmb"
)

var defaultBody = []byte(`{"model":"deepseek-chat"}`)
var highPrecisionBody = []byte(`{"model":"glm-4.6"}`)
var streamBody = []byte(`{"model":"deepseek-chat","stream":true}`)
var cacheBody = []byte(`{"model":"deepseek-chat"}`)
var cacheStreamBody = []byte(`{"model":"deepseek-chat","stream":true}`)

var usageResponse = `{"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`

var streamUsageResponse = "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
	"data: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":50,\"total_tokens\":150}}\n\n"

var cacheUsageResponse = `{"usage":{"prompt_tokens":8000,"completion_tokens":1500,"total_tokens":9500,"cache_read_tokens":5000,"cache_write_tokens":1000}}`

var cacheStreamUsageResponse = "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
	"data: {\"usage\":{\"prompt_tokens\":8000,\"completion_tokens\":1500,\"total_tokens\":9500,\"cache_read_tokens\":5000,\"cache_write_tokens\":1000}}\n\n"

var deepseekCacheUsageResponse = `{"usage":{"prompt_tokens":8000,"completion_tokens":1500,"total_tokens":9500,"prompt_cache_hit_tokens":5000}}`

var deepseekCacheDetailsUsageResponse = `{"usage":{"prompt_tokens":8000,"completion_tokens":1500,"total_tokens":9500,"prompt_tokens_details":{"cached_tokens":5000}}}`

var deepseekCacheStreamUsageResponse = "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
	"data: {\"usage\":{\"prompt_tokens\":8000,\"completion_tokens\":1500,\"total_tokens\":9500,\"prompt_cache_hit_tokens\":5000}}\n\n"

// testEnv holds all resources for a single SC09 integration test.
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

	e.backends[clusterTierPeak] = common.NewMockBackend(clusterTierPeak, http.StatusOK, usageResponse)
	e.backends[clusterNoTier] = common.NewMockBackend(clusterNoTier, http.StatusOK, usageResponse)

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

func (e *testEnv) sendRequest(host string, body []byte) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
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

// tierPeakAIConf configures a ModelTable with an all-day "peak" tier and tier prices.
func tierPeakAIConf() *cluster_conf.AIConf {
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
			TimeZone: "Asia/Shanghai",
			Tiers: []cluster_conf.PriceTier{
				{
					Name: "peak",
					TimeRanges: []cluster_conf.TimeRange{
						{
							// Cover all weekdays and all minutes of the day so the test
							// is deterministic regardless of when it runs.
							Weekdays: []int{0, 1, 2, 3, 4, 5, 6},
							Start:    "00:00",
							End:      "23:59",
						},
					},
				},
			},
			Models: []cluster_conf.ModelPrice{
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
						"input_cost_per_token":        0.000001,
						"output_cost_per_token":       0.000002,
						"cache_read_input_token_cost": 0.0000005,
					},
					TierPrices: cluster_conf.TierPriceMap{
						"peak": {
							"input_cost_per_token":        0.000002,
							"output_cost_per_token":       0.000004,
							"cache_read_input_token_cost": 0.000001,
						},
					},
				},
			},
		},
	}
}

// noTierAIConf configures the same default prices but without any Tiers / TierPrices.
func noTierAIConf() *cluster_conf.AIConf {
	conf := tierPeakAIConf()
	conf.ModelTable.Tiers = nil
	for i := range conf.ModelTable.Models {
		conf.ModelTable.Models[i].TierPrices = nil
	}
	return conf
}

// tierPeakHighPrecisionAIConf adds a model whose prices need more than 8
// decimal places. The peak tier only overrides the output price, so the
// input price exercises tier fallback to the default price. When marshaled
// to cluster_conf.data the values are emitted in scientific notation
// (e.g. 3.0084492e-06), which also exercises config parsing.
func tierPeakHighPrecisionAIConf() *cluster_conf.AIConf {
	conf := tierPeakAIConf()
	conf.ModelTable.Models = append(conf.ModelTable.Models, cluster_conf.ModelPrice{
		Provider:            "mock-provider",
		Model:               "glm-4.6",
		BaseModel:           "glm-4.6",
		Mode:                "chat",
		Capabilities:        []string{"chat"},
		SupportedParameters: []string{"temperature", "max_tokens"},
		Limits: map[string]interface{}{
			"context_window": 128000,
		},
		Prices: cluster_conf.PriceMap{
			"input_cost_per_token":  3.0084492e-06,
			"output_cost_per_token": 1.4049457764e-05,
		},
		TierPrices: cluster_conf.TierPriceMap{
			"peak": {
				"output_cost_per_token": 2.8098915528e-05,
			},
		},
	})
	return conf
}

// TestTC01 verifies RMB quota deduction using peak tier prices for a non-streaming request.
func TestTC01_RMBQuotaDeduction_Peak_NonStreaming(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterTierPeak: tierPeakAIConf(),
	}
	e := newTestEnv(t, aiConfs)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	resp, body, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterTierPeak].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterTierPeak, e.backends[clusterTierPeak].Hits())
	}

	// wait for async redis deduction
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	// peak: input=0.000002 -> 200, output=0.000004 -> 400
	want := int64(10000000000 - (100*200 + 50*400))
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC02 verifies that the absence of Tiers falls back to default prices.
func TestTC02_RMBQuotaDeduction_NoTier_Fallback(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterNoTier: noTierAIConf(),
	}
	e := newTestEnv(t, aiConfs)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	resp, body, err := e.sendRequest(apiHostNoTier, defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterNoTier].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterNoTier, e.backends[clusterNoTier].Hits())
	}

	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	// default: input=0.000001 -> 100, output=0.000002 -> 200
	want := int64(10000000000 - (100*100 + 50*200))
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC03 verifies peak tier cache-aware pricing for a non-streaming request.
func TestTC03_RMBQuotaDeduction_Peak_Cache_NonStreaming(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterTierPeak: tierPeakAIConf(),
	}
	e := newTestEnv(t, aiConfs)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)
	e.backends[clusterTierPeak].Body = cacheUsageResponse

	resp, body, err := e.sendRequest(apiHost, cacheBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterTierPeak].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterTierPeak, e.backends[clusterTierPeak].Hits())
	}

	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	// normal_input = 8000 - 5000 - 1000 = 2000
	// peak: input=200, cache_read=100, cache_creation not configured (0), output=400
	// cost = 2000*200 + 5000*100 + 1000*0 + 1500*400 = 1,500,000
	want := int64(10000000000 - 1500000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC04 verifies peak tier cache-aware pricing for an SSE streaming request.
func TestTC04_RMBQuotaDeduction_Peak_Cache_Streaming(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterTierPeak: tierPeakAIConf(),
	}
	e := newTestEnv(t, aiConfs)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)
	e.backends[clusterTierPeak].ResponseHeaders = map[string]string{"Content-Type": "text/event-stream"}
	e.backends[clusterTierPeak].Body = cacheStreamUsageResponse

	resp, body, err := e.sendRequest(apiHost, cacheStreamBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterTierPeak].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterTierPeak, e.backends[clusterTierPeak].Hits())
	}

	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	// normal_input = 8000 - 5000 - 1000 = 2000
	// peak: input=200, cache_read=100, cache_creation not configured (0), output=400
	// cost = 2000*200 + 5000*100 + 1000*0 + 1500*400 = 1,500,000
	want := int64(10000000000 - 1500000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC05 verifies that the DeepSeek cache field prompt_cache_hit_tokens is correctly
// recognized and billed using peak tier cache-aware prices for a non-streaming request.
func TestTC05_RMBQuotaDeduction_DeepSeekCacheField_NonStreaming(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterTierPeak: tierPeakAIConf(),
	}
	e := newTestEnv(t, aiConfs)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)
	e.backends[clusterTierPeak].Body = deepseekCacheUsageResponse

	resp, body, err := e.sendRequest(apiHost, cacheBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterTierPeak].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterTierPeak, e.backends[clusterTierPeak].Hits())
	}

	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	// normal_input = 8000 - 5000 - 0 = 3000 (response has no cache_write_tokens)
	// peak: input=200, cache_read=100, output=400
	// cost = 3000*200 + 5000*100 + 1500*400 = 1,700,000
	want := int64(10000000000 - 1700000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC06 verifies that the DeepSeek cache field prompt_tokens_details.cached_tokens is
// correctly recognized and billed using peak tier cache-aware prices for an SSE streaming request.
func TestTC06_RMBQuotaDeduction_DeepSeekCacheDetailsField_Streaming(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterTierPeak: tierPeakAIConf(),
	}
	e := newTestEnv(t, aiConfs)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)
	e.backends[clusterTierPeak].ResponseHeaders = map[string]string{"Content-Type": "text/event-stream"}
	e.backends[clusterTierPeak].Body = "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
		"data: {\"usage\":{\"prompt_tokens\":8000,\"completion_tokens\":1500,\"total_tokens\":9500,\"prompt_tokens_details\":{\"cached_tokens\":5000}}}\n\n"

	resp, body, err := e.sendRequest(apiHost, cacheStreamBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterTierPeak].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterTierPeak, e.backends[clusterTierPeak].Hits())
	}

	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	// normal_input = 8000 - 5000 - 0 = 3000 (response has no cache_write_tokens)
	// peak: input=200, cache_read=100, output=400
	// cost = 3000*200 + 5000*100 + 1500*400 = 1,700,000
	want := int64(10000000000 - 1700000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}


// TestTC07 verifies peak tier billing with prices that need more than 8
// decimal places (scientific notation in cluster_conf.data). The peak tier
// only overrides the output price; the input price falls back to the
// default price. With prompt=100 and completion=50 (usageResponse):
//
//	input:  round(100 * 3.0084492e-06 * 1e8) = round(30084.492)    = 30084
//	output: round(50 * 2.8098915528e-05 * 1e8) = round(140494.57764) = 140495
//	total = 170579 fixed-point units
func TestTC07_RMBQuotaDeduction_Peak_HighPrecision(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterTierPeak: tierPeakHighPrecisionAIConf(),
	}
	e := newTestEnv(t, aiConfs)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	resp, body, err := e.sendRequest(apiHost, highPrecisionBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterTierPeak].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterTierPeak, e.backends[clusterTierPeak].Hits())
	}

	// wait for async redis deduction
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 170579)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}
