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

package sc03

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
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
	apiHost  = "rmb.example.org"
	apiPath  = "/v1/chat/completions"
	apiKey   = "ak_user_a"
	apiKeyId = "user_a_key_id"

	clusterRMB         = "cluster_rmb"
	clusterNoTable     = "cluster_no_table"
	clusterFallbackRMB = "cluster_fallback_rmb"

	planRMB   = "plan_rmb"
	planToken = "plan_token"

	redisKeyRMB   = "quota:plan_rmb"
	redisKeyToken = "quota:plan_token"
)

var defaultBody = []byte(`{"model":"deepseek-chat"}`)
var modelMappingBody = []byte(`{"model":"gpt-4"}`)
var streamBody = []byte(`{"model":"deepseek-chat","stream":true}`)
var cacheBody = []byte(`{"model":"claude-opus-4-6"}`)
var cacheStreamBody = []byte(`{"model":"claude-opus-4-6","stream":true}`)
var audioBody = []byte(`{"model":"gpt-audio-1.5"}`)
var audioStreamBody = []byte(`{"model":"gpt-audio-1.5","stream":true}`)
var imageGenerationBody = []byte(`{"model":"flux-2-pro","n":2}`)
var highPrecisionBody = []byte(`{"model":"qwen2.5-omni-7b"}`)
var anthropicBody = []byte(`{"model":"claude-sonnet-4-5"}`)

var usageResponse = `{"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`
var imageGenerationUsageResponse = `{"usage":{"image_count":2}}`

// imageTokenBillingBody and tokenPricedImageUsageResponse reproduce issue
// #1420: a token-priced image_generation rule (gpt-image-2) with the
// Responses-API-style usage field names (input/output_tokens) and image token
// details reported by the real upstream.
var imageTokenBillingBody = []byte(`{"model":"gpt-image-2"}`)
var tokenPricedImageUsageResponse = `{"data":[{"b64_json":"..."}],` +
	`"usage":{"input_tokens":19,"input_tokens_details":{"cached_tokens":0,"image_tokens":0,"text_tokens":19},` +
	`"output_tokens":196,"output_tokens_details":{"image_tokens":196,"text_tokens":0},"total_tokens":215}}`

// SSE format: final chunk contains usage. The trailing blank line is required.
// anthropicUsageResponse is a complete Anthropic "message" response (note the
// type:"message" field and input_tokens/output_tokens usage field names).
// Together with anthropicBody and a Bearer (openai-style) API key it
// reproduces the auth-style/response-format mismatch of issue #1364: the
// token-auth module detects AuthStyle=openai from the request while the
// backend actually speaks the Anthropic protocol.
var anthropicUsageResponse = `{"id":"msg_01","type":"message","role":"assistant",` +
	`"content":[{"type":"text","text":"hi"}],` +
	`"usage":{"input_tokens":2000,"output_tokens":1500,"cache_read_input_tokens":8000}}`

var streamUsageResponse = "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
	"data: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":50,\"total_tokens\":150}}\n\n"

var cacheUsageResponse = `{"usage":{"prompt_tokens":8000,"completion_tokens":1500,"total_tokens":9500,"cache_read_tokens":5000,"cache_write_tokens":1000}}`

// SSE format with cache usage in the final chunk.
var cacheStreamUsageResponse = "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
	"data: {\"usage\":{\"prompt_tokens\":8000,\"completion_tokens\":1500,\"total_tokens\":9500,\"cache_read_tokens\":5000,\"cache_write_tokens\":1000}}\n\n"

var audioUsageResponse = `{"usage":{"prompt_tokens":4000,"completion_tokens":500,"total_tokens":4500,"audio_input_tokens":1000,"audio_output_tokens":200}}`

// SSE format with audio usage in the final chunk.
var audioStreamUsageResponse = "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
	"data: {\"usage\":{\"prompt_tokens\":4000,\"completion_tokens\":500,\"total_tokens\":4500,\"audio_input_tokens\":1000,\"audio_output_tokens\":200}}\n\n"

// testEnv holds all resources for a single SC03 integration test.
type testEnv struct {
	t              *testing.T
	processEnv     *common.ProcessEnv
	backends       map[string]*common.MockBackend
	redis          *common.RedisServer
	bfePort        int
	bfeMonitorPort int
	stopBFE        func()
}

func newTestEnv(t *testing.T, aiConfs map[string]*cluster_conf.AIConf, quotaPlans []common.QuotaPlan) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	e.backends[clusterRMB] = common.NewMockBackend(clusterRMB, http.StatusOK, usageResponse)
	e.backends[clusterNoTable] = common.NewMockBackend(clusterNoTable, http.StatusOK, usageResponse)
	e.backends[clusterFallbackRMB] = common.NewMockBackend(clusterFallbackRMB, http.StatusOK, usageResponse)

	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	confDir := filepath.Join(e.processEnv.WorkDir(), "conf")
	logDir := filepath.Join(e.processEnv.WorkDir(), "log")

	tokenRule := &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": quotaPlans,
		},
		Tokens: map[string]map[string]common.TokenFile{
			"ai_product": {
				apiKey: {
					Key:            apiKey,
					KeyId:          apiKeyId,
					Enabled:        true,
					ExpiredTime:    -1,
					UnlimitedQuota: false,
					QuotaPlans:     planIDs(quotaPlans),
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

	e.bfePort, e.bfeMonitorPort, e.stopBFE = e.processEnv.StartBFE(confDir, logDir)
	return e
}

func planIDs(plans []common.QuotaPlan) []string {
	ids := make([]string, len(plans))
	for i, p := range plans {
		ids[i] = p.Id
	}
	return ids
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

// tokenAuthMetricsRaw fetches /monitor/mod_ai_token_auth in prometheus
// format and returns the raw body.
func (e *testEnv) tokenAuthMetricsRaw() string {
	e.t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/monitor/mod_ai_token_auth?format=prometheus", e.bfeMonitorPort)
	resp, err := http.Get(url)
	if err != nil {
		e.t.Fatalf("get mod_ai_token_auth monitor failed: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("mod_ai_token_auth monitor status = %d, body: %s", resp.StatusCode, data)
	}
	return string(data)
}

// tokenAuthMetric returns the value of the named counter (0 when absent).
// The web-monitor prometheus export renders keys in SNAKE_CASE, so callers
// pass the Go field name (e.g. "UsageUnrecognized") and the lookup key
// becomes "mod_ai_token_auth_USAGE_UNRECOGNIZED".
func (e *testEnv) tokenAuthMetric(name string) float64 {
	e.t.Helper()
	data := e.tokenAuthMetricsRaw()

	fullName := "mod_ai_token_auth_" + camelToSnake(name)
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.LastIndex(line, " ")
		if idx < 0 || strings.TrimSpace(line[:idx]) != fullName {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(line[idx+1:]), 64)
		if err != nil {
			continue
		}
		return v
	}
	return 0
}

// camelToSnake converts a CamelCase counter field name to the SNAKE_CASE
// key used by the web-monitor prometheus export (e.g. "UsageUnrecognized"
// -> "USAGE_UNRECOGNIZED").
func camelToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return strings.ToUpper(b.String())
}

// waitForTokenAuthMetric polls a mod_ai_token_auth counter until cond is
// satisfied or the timeout elapses (the monitor handler reads counters on
// demand, so no refresh lag is expected; the poll only guards the startup
// race).
func (e *testEnv) waitForTokenAuthMetric(name string, timeout time.Duration, cond func(float64) bool) bool {
	e.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond(e.tokenAuthMetric(name)) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func (e *testEnv) sendRequest(host string, body []byte) (*http.Response, string, error) {
	return e.sendRequestToPath(host, apiPath, body)
}

func (e *testEnv) sendRequestToPath(host string, path string, body []byte) (*http.Response, string, error) {
	return e.sendRequestWithHeaders(host, path, body, nil)
}

// sendRequestWithHeaders is sendRequestToPath with extra request headers. It is
// used to set Accept-Encoding explicitly so net/http does not transparently
// gunzip the response: the test then observes exactly the bytes BFE forwards
// (issue #1406).
func (e *testEnv) sendRequestWithHeaders(host string, path string, body []byte, headers map[string]string) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, path)
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

func defaultRMBAIConf() *cluster_conf.AIConf {
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
					Model:               "gpt-audio-1.5",
					BaseModel:           "gpt-audio-1.5",
					Mode:                "chat",
					Capabilities:        []string{"chat", "audio_input"},
					SupportedParameters: []string{"temperature", "max_tokens"},
					Limits: map[string]interface{}{
						"context_window": 128000,
					},
					Prices: cluster_conf.PriceMap{
						"input_cost_per_token":        0.00000178,
						"output_cost_per_token":       0.00000715,
						"input_cost_per_audio_token":  0.00002288,
						"output_cost_per_audio_token": 0.00004576,
					},
				},
			},
		},
	}
}

func fallbackRMBAIConf() *cluster_conf.AIConf {
	conf := defaultRMBAIConf()
	conf.ModelTable.Models[0].Prices = cluster_conf.PriceMap{
		"input_cost_per_token":  0.000003,
		"output_cost_per_token": 0.000004,
	}
	return conf
}

func imageGenerationAIConf() *cluster_conf.AIConf {
	conf := defaultRMBAIConf()
	conf.ModelTable.Models = append(conf.ModelTable.Models, cluster_conf.ModelPrice{
		Provider:            "mock-provider",
		Model:               "flux-2-pro",
		BaseModel:           "flux-2-pro",
		Mode:                "image_generation",
		Capabilities:        []string{"image_generation"},
		SupportedParameters: []string{"prompt", "n", "size"},
		Limits: map[string]interface{}{
			"context_window": 128000,
		},
		Prices: cluster_conf.PriceMap{
			"output_cost_per_image": 0.03,
		},
	})
	return conf
}

// tokenPricedImageGenerationAIConf adds the issue #1420 reproduction entry: a
// token-priced image_generation rule whose Prices carry only token unit price
// keys (exactly the reported gpt-image-2 pricing rule) and no
// output_cost_per_image. Before the fix the per-image formula alone was
// applied and the RMB plan was never charged.
func tokenPricedImageGenerationAIConf() *cluster_conf.AIConf {
	conf := defaultRMBAIConf()
	conf.ModelTable.Models = append(conf.ModelTable.Models, cluster_conf.ModelPrice{
		Provider:            "mock-provider",
		Model:               "gpt-image-2",
		BaseModel:           "gpt-image-2",
		Mode:                "image_generation",
		Capabilities:        []string{"image_generation"},
		SupportedParameters: []string{"prompt", "n", "size"},
		Limits: map[string]interface{}{
			"context_window": 128000,
		},
		Prices: cluster_conf.PriceMap{
			"input_cost_per_token":        2.402e-5,
			"output_cost_per_token":       0.00014414,
			"cache_read_input_token_cost": 6.01e-6,
			"input_cost_per_image_token":  3.844e-5,
		},
	})
	return conf
}

// billingMethodAIConf adds the three entries of TC-28: a mixed entry
// (per-image key plus token prices), an entry with an explicit
// output_cost_per_image = 0 (key present), and a token-priced
// video_generation entry (no output_cost_per_video key).
func billingMethodAIConf() *cluster_conf.AIConf {
	conf := defaultRMBAIConf()
	conf.ModelTable.Models = append(conf.ModelTable.Models,
		cluster_conf.ModelPrice{
			Provider:            "mock-provider",
			Model:               "gpt-image-mixed",
			BaseModel:           "gpt-image-mixed",
			Mode:                "image_generation",
			Capabilities:        []string{"image_generation"},
			SupportedParameters: []string{"prompt", "n", "size"},
			Limits:              map[string]interface{}{"context_window": 128000},
			Prices: cluster_conf.PriceMap{
				"output_cost_per_image": 0.03,
				"input_cost_per_token":  0.000001,
				"output_cost_per_token": 0.000002,
			},
		},
		cluster_conf.ModelPrice{
			Provider:            "mock-provider",
			Model:               "gpt-image-free",
			BaseModel:           "gpt-image-free",
			Mode:                "image_generation",
			Capabilities:        []string{"image_generation"},
			SupportedParameters: []string{"prompt", "n", "size"},
			Limits:              map[string]interface{}{"context_window": 128000},
			Prices: cluster_conf.PriceMap{
				"output_cost_per_image": 0,
				"input_cost_per_token":  0.000001,
				"output_cost_per_token": 0.000002,
			},
		},
		cluster_conf.ModelPrice{
			Provider:            "mock-provider",
			Model:               "kling-token",
			BaseModel:           "kling-token",
			Mode:                "video_generation",
			Capabilities:        []string{"video_generation"},
			SupportedParameters: []string{"prompt", "n"},
			Limits:              map[string]interface{}{"context_window": 128000},
			Prices: cluster_conf.PriceMap{
				"input_cost_per_token":  0.000001,
				"output_cost_per_token": 0.000002,
			},
		},
	)
	return conf
}

// highPrecisionAIConf adds a model whose prices need more than 8 decimal
// places. When marshaled to cluster_conf.data, encoding/json emits them in
// scientific notation (e.g. 6.0168984e-09), which also exercises BFE's
// config parsing of scientific-notation prices.
func highPrecisionAIConf() *cluster_conf.AIConf {
	conf := defaultRMBAIConf()
	conf.ModelTable.Models = append(conf.ModelTable.Models, cluster_conf.ModelPrice{
		Provider:            "mock-provider",
		Model:               "qwen2.5-omni-7b",
		BaseModel:           "qwen2.5-omni-7b",
		Mode:                "chat",
		Capabilities:        []string{"chat"},
		SupportedParameters: []string{"temperature", "max_tokens"},
		Limits: map[string]interface{}{
			"context_window": 128000,
		},
		Prices: cluster_conf.PriceMap{
			"input_cost_per_token":  6.0168984e-09,
			"output_cost_per_token": 7.6234102728e-08,
		},
	})
	return conf
}

// anthropicAIConf adds an Anthropic-protocol model with cache pricing. The
// round per-token prices (1e-6 / 2e-6 / 5e-7 / 1.5e-6 yuan) make the expected
// fixed-point deduction easy to verify by hand:
//
//	cost = input*100 + cache_read*50 + output*200  (units of 1e-8 yuan)
func anthropicAIConf() *cluster_conf.AIConf {
	conf := defaultRMBAIConf()
	conf.ModelTable.Models = append(conf.ModelTable.Models, cluster_conf.ModelPrice{
		Provider:            "mock-provider",
		Model:               "claude-sonnet-4-5",
		BaseModel:           "claude-sonnet-4-5",
		Mode:                "chat",
		Capabilities:        []string{"chat"},
		SupportedParameters: []string{"temperature", "max_tokens"},
		Limits: map[string]interface{}{
			"context_window": 128000,
		},
		Prices: cluster_conf.PriceMap{
			"input_cost_per_token":            0.000001,
			"output_cost_per_token":           0.000002,
			"cache_read_input_token_cost":     0.0000005,
			"cache_creation_input_token_cost": 0.0000015,
		},
	})
	return conf
}

func noTableAIConf() *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type: 0,
		Keys: []cluster_conf.AIKey{
			{Name: "key-primary", Key: "sk-primary", Weight: 100},
		},
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:            "weighted_random",
			MaxRetries:          0,
			RetryBackoffInitial: 50,
			RetryBackoffMax:     200,
		},
	}
}

func rmbQuotaPlan(quota int64) common.QuotaPlan {
	return common.QuotaPlan{
		Id:          planRMB,
		Unlimited:   false,
		PassNoQuota: false,
		RedisKey:    redisKeyRMB,
		ExpiredTime: -1,
		Quota:       quota,
		Unit:        "RMB",
	}
}

func tokenQuotaPlan(quota int64) common.QuotaPlan {
	return common.QuotaPlan{
		Id:          planToken,
		Unlimited:   false,
		PassNoQuota: false,
		RedisKey:    redisKeyToken,
		ExpiredTime: -1,
		Quota:       quota,
		Unit:        "total_token",
	}
}

// TestTC01 verifies RMB quota deduction after a successful request.
func TestTC01_RMBQuotaDeduction(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
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

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// wait for async redis deduction
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - (100*100 + 50*200))
	if remaining != want {
		t.Fatalf("remaining quota = %d, want %d", remaining, want)
	}
}

// TestTC02 verifies that a request is rejected when RMB quota is exhausted.
func TestTC02_RMBQuotaExhausted(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(0)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 0)

	resp, body, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected status 429, got %d, body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "quota") {
		t.Fatalf("expected quota error in body, got: %s", body)
	}
	if e.backends[clusterRMB].Hits() != 0 {
		t.Fatalf("expected no backend hit, got %d", e.backends[clusterRMB].Hits())
	}
}

// TestTC03 verifies billing by the mapped model when ModelMapping is used.
func TestTC03_ModelMappingBilling(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	resp, body, err := e.sendRequest(apiHost, modelMappingBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	models := e.backends[clusterRMB].Models()
	if len(models) != 1 || models[0] != "deepseek-chat" {
		t.Fatalf("expected backend model deepseek-chat, got %v", models)
	}

	time.Sleep(200 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - (100*100 + 50*200))
	if remaining != want {
		t.Fatalf("remaining quota = %d, want %d", remaining, want)
	}
}

// TestTC04 verifies that token and RMB quota plans are deducted together.
func TestTC04_TokenAndRMBQuotaCoexist(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000), tokenQuotaPlan(1000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)
	e.redis.SetQuota(redisKeyToken, 1000)

	resp, body, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	time.Sleep(200 * time.Millisecond)
	rmbRemaining := e.redis.GetQuota(redisKeyRMB)
	rmbWant := int64(10000000000 - (100*100 + 50*200))
	if rmbRemaining != rmbWant {
		t.Fatalf("RMB remaining = %d, want %d", rmbRemaining, rmbWant)
	}
	tokenRemaining := e.redis.GetQuota(redisKeyToken)
	tokenWant := int64(1000 - 150)
	if tokenRemaining != tokenWant {
		t.Fatalf("token remaining = %d, want %d", tokenRemaining, tokenWant)
	}
}

// TestTC05 verifies zero-cost handling when ModelTable is missing.
func TestTC05_NoModelTableZeroCost(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterNoTable: noTableAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	resp, body, err := e.sendRequest("notable.example.org", defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterNoTable].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterNoTable, e.backends[clusterNoTable].Hits())
	}

	time.Sleep(200 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	if remaining != int64(10000000000) {
		t.Fatalf("remaining quota = %d, want unchanged 10000000000", remaining)
	}
}

// TestTC06 verifies billing by the final cluster after fallback.
func TestTC06_FallbackBilling(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB:         defaultRMBAIConf(),
		clusterFallbackRMB: fallbackRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)
	e.backends[clusterRMB].ResponseFunc = func(r *http.Request, count int) (int, string) {
		return http.StatusBadGateway, ""
	}

	resp, body, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}
	if e.backends[clusterFallbackRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterFallbackRMB, e.backends[clusterFallbackRMB].Hits())
	}

	time.Sleep(200 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - (100*300 + 50*400))
	if remaining != want {
		t.Fatalf("remaining quota = %d, want %d", remaining, want)
	}
}

// TestTC07 verifies RMB quota deduction for streaming (SSE) responses.
// This is the regression test for https://github.com/bfenetworks/bfe/issues/1316.
func TestTC07_RMBQuotaDeduction_Streaming(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	// Configure backend to return SSE stream with usage in the final chunk.
	e.backends[clusterRMB].ResponseHeaders = map[string]string{"Content-Type": "text/event-stream"}
	e.backends[clusterRMB].Body = streamUsageResponse

	resp, body, err := e.sendRequest(apiHost, streamBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for async redis deduction after response finishes.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - (100*100 + 50*200))
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC08 verifies RMB quota deduction with cache pricing for non-streaming responses.
func TestTC08_RMBQuotaDeduction_Cache_NonStreaming(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	// Configure backend to return response with cache usage.
	e.backends[clusterRMB].Body = cacheUsageResponse

	resp, body, err := e.sendRequest(apiHost, cacheBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for async redis deduction.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	// normal_input = 8000 - 5000 - 1000 = 2000
	// cost = 2000*452 + 5000*45 + 1000*565 + 1500*2262 = 5087000
	want := int64(10000000000 - 5087000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC09 verifies RMB quota deduction with cache pricing for streaming (SSE) responses.
func TestTC09_RMBQuotaDeduction_Cache_Streaming(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	// Configure backend to return SSE stream with cache usage in the final chunk.
	e.backends[clusterRMB].ResponseHeaders = map[string]string{"Content-Type": "text/event-stream"}
	e.backends[clusterRMB].Body = cacheStreamUsageResponse

	resp, body, err := e.sendRequest(apiHost, cacheStreamBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for async redis deduction after response finishes.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	// normal_input = 8000 - 5000 - 1000 = 2000
	// cost = 2000*452 + 5000*45 + 1000*565 + 1500*2262 = 5087000
	want := int64(10000000000 - 5087000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC10 verifies RMB quota deduction with audio pricing for non-streaming responses.
func TestTC10_RMBQuotaDeduction_Audio_NonStreaming(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	// Configure backend to return response with audio usage.
	e.backends[clusterRMB].Body = audioUsageResponse

	resp, body, err := e.sendRequest(apiHost, audioBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for async redis deduction.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	// normal_input = 4000 - 1000 = 3000
	// normal_output = 500 - 200 = 300
	// cost = 3000*178 + 1000*2288 + 300*715 + 200*4576 = 3951700
	want := int64(10000000000 - 3951700)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC11 verifies RMB quota deduction with audio pricing for streaming (SSE) responses.
func TestTC11_RMBQuotaDeduction_Audio_Streaming(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	// Configure backend to return SSE stream with audio usage in the final chunk.
	e.backends[clusterRMB].ResponseHeaders = map[string]string{"Content-Type": "text/event-stream"}
	e.backends[clusterRMB].Body = audioStreamUsageResponse

	resp, body, err := e.sendRequest(apiHost, audioStreamBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for async redis deduction after response finishes.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 3951700)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC12 verifies RMB quota deduction for image generation by image count.
func TestTC12_RMBQuotaDeduction_ImageGeneration(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: imageGenerationAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)
	e.backends[clusterRMB].Body = imageGenerationUsageResponse

	resp, body, err := e.sendRequestToPath(apiHost, "/v1/images/generations", imageGenerationBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for async redis deduction after response finishes.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	// cost = 2 * 0.03 RMB = 2 * 3000000 fixed-point units
	want := int64(10000000000 - 2*3000000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC27 verifies issue #1420: a token-priced image_generation entry (token
// unit prices but no output_cost_per_image) bills by the token formula. With
// the issue's exact prices and usage (prompt=19, completion=196):
//
//	cost = round(19 * 2.402e-5 * 1e8) + round(196 * 0.00014414 * 1e8)
//	     = 45638 + 2825144 = 2870782 fixed-point units
//
// Before the fix the per-image formula alone was applied (per-image price
// unconfigured, image input tokens 0) and the RMB plan was never charged.
func TestTC27_RMBQuotaDeduction_ImageGenerationTokenPriced(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: tokenPricedImageGenerationAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)
	e.backends[clusterRMB].Body = tokenPricedImageUsageResponse

	resp, body, err := e.sendRequestToPath(apiHost, "/v1/images/generations", imageTokenBillingBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for async redis deduction after response finishes.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 2870782)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC28 locks the entry-driven billing-method discrimination of issue
// #1420 with three arms:
//
//  1. mixed entry (per-image key plus token prices) bills per image:
//     2 * 0.03 = 6000000 (the token keys must not win);
//  2. an explicit output_cost_per_image = 0 (key present) stays per-image
//     billing at zero cost and does not fall back to the token formula;
//  3. a token-priced video_generation entry (no output_cost_per_video key)
//     falls back to the token formula: 100*100 + 50*200 = 20000.
func TestTC28_RMBQuotaDeduction_BillingMethodDiscriminator(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: billingMethodAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	// Arm 1: per-image key wins over the token keys on a mixed entry.
	e.backends[clusterRMB].Body = imageGenerationUsageResponse
	resp, body, err := e.sendRequestToPath(apiHost, "/v1/images/generations",
		[]byte(`{"model":"gpt-image-mixed","n":2}`))
	if err != nil {
		t.Fatalf("arm 1 send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("arm 1 expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	// Arm 2: explicit output_cost_per_image = 0 (key present) bills per image
	// at zero cost; the token formula must not take over.
	e.backends[clusterRMB].Body = usageResponse
	resp, body, err = e.sendRequestToPath(apiHost, "/v1/images/generations",
		[]byte(`{"model":"gpt-image-free"}`))
	if err != nil {
		t.Fatalf("arm 2 send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("arm 2 expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	// Arm 3: token-priced video_generation entry falls back to the token
	// formula.
	resp, body, err = e.sendRequestToPath(apiHost, "/v1/video/generations",
		[]byte(`{"model":"kling-token"}`))
	if err != nil {
		t.Fatalf("arm 3 send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("arm 3 expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 3 {
		t.Fatalf("expected 3 hits on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for async redis deduction after response finishes.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	// arm1: 2 * 3000000 = 6000000; arm2: 0; arm3: 100*100 + 50*200 = 20000
	want := int64(10000000000 - 6020000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d", remaining, want)
	}
}

// TestTC13 verifies that a total_token quota plan with quota=0 can be loaded
// and rejects requests with QuotaExhausted.
func TestTC13_TokenQuotaZeroLoaded(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{tokenQuotaPlan(0)})
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, 0)

	resp, body, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected status 429, got %d, body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "quota") {
		t.Fatalf("expected quota error in body, got: %s", body)
	}
	if e.backends[clusterRMB].Hits() != 0 {
		t.Fatalf("expected no backend hit, got %d", e.backends[clusterRMB].Hits())
	}
}

// TestTC14 verifies that a total_token quota plan with quota=0 rejects requests
// with QuotaExhausted even when its redis key was never initialized
// (regression test for https://github.com/rainway-ai-gateway/ai-gateway-api/issues/136).
func TestTC14_TokenQuotaZeroMissingRedisKey(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{tokenQuotaPlan(0)})
	defer e.Close()

	// note: redis key redisKeyToken is intentionally not initialized

	resp, body, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected status 429, got %d, body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "quota") {
		t.Fatalf("expected quota error in body, got: %s", body)
	}
	if e.backends[clusterRMB].Hits() != 0 {
		t.Fatalf("expected no backend hit, got %d", e.backends[clusterRMB].Hits())
	}
}

// TestTC15 verifies end-to-end billing with prices that need more than 8
// decimal places (scientific notation in cluster_conf.data). The prices
// below come from the generated model catalog. With prompt=100 and
// completion=50 (usageResponse):
//
//	cost = round(100 * 6.0168984e-09 * 1e8) + round(50 * 7.6234102728e-08 * 1e8)
//	     = round(60.168984) + round(381.17051364)
//	     = 60 + 381 = 441 fixed-point units
//
// The pre-v0.6 load-time fixed-point conversion truncated the prices to
// 0 and 7 (1e-8 yuan), which would have deducted only 350.
func TestTC15_RMBQuotaDeduction_HighPrecision(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: highPrecisionAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
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

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// wait for async redis deduction
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 441)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// anthropicMismatchRequest verifies the fixed-point deduction for an
// Anthropic-protocol response received on an openai-style (Bearer key)
// request, i.e. the issue #1364 shape. Prices are 1e-6 input / 2e-6 output /
// 5e-7 cache_read yuan per token, i.e. 100 / 200 / 50 units per token.
// Anthropic input_tokens excludes cached tokens, so billable normal input is
// 2000 + 8000 - 8000 = 2000 and the total deduction is
//
//	2000*100 + 8000*50 + 1500*200 = 900000
//
// Before the fix, the unrecognized final usage was reduced to cache-read only
// (400000 for the chunked framing, 0 for the Content-Length framing).
func anthropicMismatchRequest(t *testing.T, noContentLength bool) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: anthropicAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	e.backends[clusterRMB].ResponseHeaders = map[string]string{"Content-Type": "application/json"}
	e.backends[clusterRMB].NoContentLength = noContentLength
	e.backends[clusterRMB].Body = anthropicUsageResponse

	resp, body, err := e.sendRequest(apiHost, anthropicBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for async redis deduction.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 900000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d (NoContentLength=%v), response body: %s",
			remaining, want, noContentLength, body)
	}
}

// TestTC16 verifies RMB quota deduction for a non-streaming Anthropic
// response framed with chunked transfer encoding (no Content-Length). This
// exercises the mod_body_process completion path: the final usage must be
// recognized (type:"message") and marked, otherwise the request-finish guard
// zeroes the billing fields and only cache-read would be deducted (issue
// #1364).
func TestTC16_RMBQuotaDeduction_Anthropic_NonStream_Chunked(t *testing.T) {
	anthropicMismatchRequest(t, true)
}

// TestTC17 verifies RMB quota deduction for a non-streaming Anthropic
// response with a Content-Length header. This exercises the
// tokenReadResponseHandler path (res.ContentLength >= 0): the single
// openai-style adapter parses nothing from the Anthropic body, so the
// cross-protocol fallback must recover the usage (issue #1364). Before the
// fix this deducted 0.
func TestTC17_RMBQuotaDeduction_Anthropic_NonStream_ContentLength(t *testing.T) {
	anthropicMismatchRequest(t, false)
}

// gzipResponseBody returns the gzip-compressed form of body, to be served as a
// MockBackend body together with Content-Encoding: gzip.
func gzipResponseBody(t *testing.T, body string) string {
	t.Helper()

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(body)); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	return buf.String()
}

// TestTC22 verifies RMB quota deduction for a non-streaming Anthropic response
// that is gzip-compressed, framed as chunked (no Content-Length) and labelled
// with a media type parameter ("application/json; charset=utf-8"). Before the
// fix (issue #1406) none of these shapes reached the usage parser: the final
// usage was never marked and the deduction was 0. The test also asserts the
// forwarded bytes and Content-Encoding header are unchanged, i.e. the
// decompression only feeds the parser.
func TestTC22_RMBQuotaDeduction_Anthropic_NonStream_ChunkedGzip(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: anthropicAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	e.backends[clusterRMB].ResponseHeaders = map[string]string{
		"Content-Type":     "application/json; charset=utf-8",
		"Content-Encoding": "gzip",
	}
	e.backends[clusterRMB].NoContentLength = true
	e.backends[clusterRMB].Body = gzipResponseBody(t, anthropicUsageResponse)

	// Accept-Encoding is set explicitly so net/http does not transparently
	// gunzip the response and the test observes the forwarded gzip bytes.
	resp, body, err := e.sendRequestWithHeaders(apiHost, apiPath, anthropicBody, map[string]string{
		"Accept-Encoding": "gzip",
	})
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Forwarding stays byte-identical: gzip payload + Content-Encoding header.
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
	zr, err := gzip.NewReader(strings.NewReader(body))
	if err != nil {
		t.Fatalf("forwarded body is not gzip: %v", err)
	}
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip forwarded body: %v", err)
	}
	if string(decoded) != anthropicUsageResponse {
		t.Errorf("forwarded body mismatch:\n got: %s\nwant: %s", decoded, anthropicUsageResponse)
	}

	// Wait for async redis deduction.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 900000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC23 verifies the issue #1408 mode-price fallback: when the cluster
// price table has no (model, mode) entry for the request's mode but the model
// has a chat baseline entry, the cost falls back to the chat price (product
// decision: an unpriced mode is billed the same as chat), the request is
// marked ai_price_miss and PriceFallbackUsed counts it. The chat-mode arm on
// the same model bills unchanged, and a model absent from the price table
// entirely still bills 0 (the chat baseline never falls back).
func TestTC23_RMBQuotaDeduction_ModePriceFallback(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(), // chat-only entries, no "responses" mode
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	e.backends[clusterRMB].ResponseHeaders = map[string]string{"Content-Type": "application/json"}
	e.backends[clusterRMB].Body = usageResponse // prompt 100, completion 50

	// chat arm: hits the chat entry directly (100*100 + 50*200 = 20000).
	resp, body, err := e.sendRequest(apiHost, []byte(`{"model":"deepseek-chat"}`))
	if err != nil {
		t.Fatalf("send chat request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	// responses arm: (deepseek-chat, responses) has no entry; falls back to
	// the chat price and bills the same 20000.
	resp2, body2, err := e.sendRequestToPath(apiHost, "/v1/responses", []byte(`{"model":"deepseek-chat"}`))
	if err != nil {
		t.Fatalf("send responses request failed: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp2.StatusCode, body2)
	}

	if e.backends[clusterRMB].Hits() != 2 {
		t.Fatalf("expected 2 hits on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// no-baseline arm: the model is absent from the price table entirely; the
	// chat mode never falls back, so this bills 0 (PriceMissZeroBilled).
	resp3, body3, err := e.sendRequest(apiHost, []byte(`{"model":"no-such-model"}`))
	if err != nil {
		t.Fatalf("send no-baseline request failed: %v", err)
	}
	if resp3.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp3.StatusCode, body3)
	}

	// Wait for async redis deduction: only the two priced arms deduct.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 20000 - 20000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d", remaining, want)
	}
}

var responsesBody = []byte(`{"model":"gpt-5-codex","stream":true}`)
var responsesNonStreamBody = []byte(`{"model":"gpt-5-codex"}`)

// Responses API (issue #1381, Codex): the SSE stream is terminated by
// response.completed, which carries the final usage under response.usage
// with the input/output_tokens naming. OpenAI subset semantics (issue
// #1389): input_tokens already includes cached_tokens; the fixture keeps
// cached_tokens (8000) < input_tokens (10000) as a real upstream would.
var responsesStreamUsageResponse = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_01\",\"status\":\"in_progress\"}}\n\n" +
	"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
	"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_01\",\"status\":\"completed\",\"usage\":{\"input_tokens\":10000,\"output_tokens\":1500,\"total_tokens\":11500,\"input_tokens_details\":{\"cached_tokens\":8000}}}}\n\n"

// Non-streaming create-response object: usage stays at the top level but
// uses the Responses API leaf names.
var responsesUsageResponse = `{"id":"resp_01","status":"completed","usage":{"input_tokens":10000,"output_tokens":1500,"total_tokens":11500,"input_tokens_details":{"cached_tokens":8000}}}`

// responsesAIConf adds a Responses API (mode "responses") model with cache
// pricing. The round per-token prices (1e-6 / 2e-6 / 5e-7 yuan) mirror
// anthropicAIConf, so the expected fixed-point deduction is easy to verify
// by hand:
//
//	cost = input*100 + cache_read*50 + output*200  (units of 1e-8 yuan)
//
// OpenAI subset semantics (issue #1389): input_tokens=10000 already
// includes cached_tokens=8000, so billable normal input is 10000-8000=2000
// and the total deduction is
//
//	2000*100 + 8000*50 + 1500*200 = 900000
func responsesAIConf() *cluster_conf.AIConf {
	conf := defaultRMBAIConf()
	conf.ModelTable.Models = append(conf.ModelTable.Models, cluster_conf.ModelPrice{
		Provider:            "mock-provider",
		Model:               "gpt-5-codex",
		BaseModel:           "gpt-5-codex",
		Mode:                "responses",
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
	})
	return conf
}

// TestTC18 verifies RMB quota deduction for a Responses API streaming (SSE)
// response terminated by response.completed (issue #1381: Codex via
// /compatible-mode/v1/responses was never billed; the final usage arrives
// only in this event, nested under response.usage). Before the fix the
// deduction was skipped entirely: usage parsed as 0, the final-usage mark
// was never set, and the request-finish guards zeroed everything.
func TestTC18_RMBQuotaDeduction_ResponsesAPI_Streaming(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: responsesAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	e.backends[clusterRMB].ResponseHeaders = map[string]string{"Content-Type": "text/event-stream"}
	e.backends[clusterRMB].Body = responsesStreamUsageResponse

	resp, body, err := e.sendRequestToPath(apiHost, "/v1/responses", responsesBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for async redis deduction after response finishes.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 900000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC19 verifies RMB quota deduction for a non-streaming Responses API
// create-response object: usage sits at the top level with the
// input/output_tokens leaf names, gated on input_tokens_details (issue
// #1381 non-streaming arm).
func TestTC19_RMBQuotaDeduction_ResponsesAPI_NonStreaming(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: responsesAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	e.backends[clusterRMB].Body = responsesUsageResponse

	resp, body, err := e.sendRequestToPath(apiHost, "/v1/responses", responsesNonStreamBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for async redis deduction.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 900000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// TestTC20 verifies RMB quota deduction for the provider-native client
// entry /compatible-mode/v1/responses (issue #1382, Codex real path):
// the entry reduces to the /responses endpoint for billing-mode detection,
// so the Responses-mode price is hit. Before the fix the mode fell back to
// chat, the price lookup missed and the request was billed 0. Together with
// TC-18 (issue #1381) this covers the full Codex billing chain.
func TestTC20_RMBQuotaDeduction_ResponsesAPI_ProviderNativePath(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: responsesAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	e.backends[clusterRMB].ResponseHeaders = map[string]string{"Content-Type": "text/event-stream"}
	e.backends[clusterRMB].Body = responsesStreamUsageResponse

	resp, body, err := e.sendRequestToPath(apiHost, "/compatible-mode/v1/responses", responsesBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for async redis deduction after response finishes.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 900000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// responsesNoCacheUsageResponse is the issue #1389 controlled fixture:
// input_tokens=100 already includes cached_tokens=20 (total = 100 + 50).
var responsesNoCacheUsageResponse = `{"id":"resp_01","status":"completed","usage":{"input_tokens":100,"output_tokens":50,"total_tokens":150,"input_tokens_details":{"cached_tokens":20}}}`

// responsesNoCacheAIConf adds a Responses API (mode "responses") model
// priced for input/output tokens only, without a cache_read price: cached
// tokens are already included in input_tokens (OpenAI subset semantics,
// issue #1389) and must be billed exactly once, at the input price.
// Per-token prices 1e-5 / 2e-5 yuan are 1000 / 2000 units of 1e-8 yuan.
func responsesNoCacheAIConf() *cluster_conf.AIConf {
	conf := defaultRMBAIConf()
	conf.ModelTable.Models = append(conf.ModelTable.Models, cluster_conf.ModelPrice{
		Provider:            "mock-provider",
		Model:               "gpt-5-codex",
		BaseModel:           "gpt-5-codex",
		Mode:                "responses",
		Capabilities:        []string{"chat"},
		SupportedParameters: []string{"temperature", "max_tokens"},
		Limits: map[string]interface{}{
			"context_window": 128000,
		},
		Prices: cluster_conf.PriceMap{
			"input_cost_per_token":  0.00001,
			"output_cost_per_token": 0.00002,
		},
	})
	return conf
}

// TestTC21 verifies RMB quota deduction for a non-streaming Responses API
// request when no cache price is configured (issue #1389, from
// ai-gateway-api#206): input_tokens=100 already includes cached_tokens=20,
// so the deduction must be 100*1000 + 50*2000 = 200000 units. Before the
// fix the additive normalization inflated PromptTokens to 120 and the
// deduction was 220000 (cached tokens billed twice at the input price).
func TestTC21_RMBQuotaDeduction_ResponsesAPI_NoCachePrice(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: responsesNoCacheAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	e.backends[clusterRMB].Body = responsesNoCacheUsageResponse

	resp, body, err := e.sendRequestToPath(apiHost, "/v1/responses", responsesNonStreamBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for async redis deduction.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 200000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d, response body: %s", remaining, want, body)
	}
}

// Issue #1410: a 2xx response whose usage is never recognized (no usage
// field at all) and not estimatable (EstimateToken off in this scenario's
// bfe.conf) keeps the yardstick behavior (zero deduction) but must now be
// observable: the UsageUnrecognized counter increments and a warn is
// logged. The zero-output and normal control arms prove recognized traffic
// never pollutes the bypass counter.
func TestTC24_RMBQuotaDeduction_UsageUnrecognizedSignal(t *testing.T) {
	e := newTestEnv(t, map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	backend := e.backends[clusterRMB]

	// Arm 1 (unrecognized): 200 response without any usage field. The
	// yardstick needs "not estimatable": Server.EstimateToken defaults to
	// true in bfe.conf, so a completable JSON body would be estimate-billed
	// (source=estimated, not this branch). A chunked non-SSE non-JSON body
	// is never marked responseCompleted (tokenReadResponseHandler only
	// parses complete non-stream bodies; mod_body_process finds no
	// termination event in plain text), so the request lands in the
	// unrecognized branch: zero deduction, counter +1, one warn.
	backend.ResponseHeaders = map[string]string{"Content-Type": "text/plain"}
	backend.NoContentLength = true
	backend.Body = `hello, no usage here`
	resp, body, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send unrecognized-usage request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if !e.waitForTokenAuthMetric("UsageUnrecognized", 5*time.Second, func(v float64) bool { return v == 1 }) {
		e.t.Logf("mod_ai_token_auth monitor body:\n%s", e.tokenAuthMetricsRaw())
		e.logBFEException()
		t.Fatalf("UsageUnrecognized = %v, want 1 after the unrecognized arm", e.tokenAuthMetric("UsageUnrecognized"))
	}

	// Arm 2 (zero-output parsed control): completion_tokens=0 is a real
	// final usage (issue #1398 yardstick); it bills prompt-only and must
	// NOT count as unrecognized.
	backend.NoContentLength = false
	backend.ResponseHeaders = map[string]string{"Content-Type": "application/json"}
	backend.Body = `{"usage":{"prompt_tokens":100,"completion_tokens":0,"total_tokens":100}}`
	resp2, body2, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send zero-output request failed: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp2.StatusCode, body2)
	}
	if v := e.tokenAuthMetric("UsageUnrecognized"); v != 1 {
		e.logBFEException()
		t.Fatalf("UsageUnrecognized = %v, want 1 (zero-output control arm must not be counted)", v)
	}

	// Arm 3 (normal billing control): the standard usage fixture.
	backend.Body = usageResponse
	resp3, body3, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send normal request failed: %v", err)
	}
	if resp3.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp3.StatusCode, body3)
	}
	if v := e.tokenAuthMetric("UsageUnrecognized"); v != 1 {
		e.logBFEException()
		t.Fatalf("UsageUnrecognized = %v, want 1 (normal arm must not be counted)", v)
	}

	if e.backends[clusterRMB].Hits() != 3 {
		t.Fatalf("expected 3 hits on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Async deduction: arm 1 bills 0 (yardstick), arm 2 bills 100*100,
	// arm 3 bills 100*100 + 50*200.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 10000 - 20000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d", remaining, want)
	}

	// The warn itself goes to the BFE process console (StartBFE wires stdout
	// directly to the test process, and this scenario's bfe.conf has no
	// [log] section, so no exception.log file exists); it is visible in the
	// test output and shares the guard body with the asserted counter, so
	// the integration layer asserts the counter as the signal of record.
}

// Issue #1414 (assertion A-01): usage recognition is presence-based, so an
// upstream that explicitly reports an all-zero usage object is a real
// observation (parsed, billed at 0) instead of being estimated or misfiled as
// unrecognized. The balance assertion is the proof that presence wins over the
// EstimateToken yardstick: before #1414 this arm was billed by the estimate.
func TestTC25_RMBQuotaDeduction_UsagePresenceDistinguishable(t *testing.T) {
	e := newTestEnv(t, map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	backend := e.backends[clusterRMB]

	// Arm 1: explicit all-zero usage. EstimateToken is true in this scenario's
	// bfe.conf, so only presence-based recognition keeps this arm at 0.
	backend.NoContentLength = false
	backend.ResponseHeaders = map[string]string{"Content-Type": "application/json"}
	backend.Body = `{"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	resp, body, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send all-zero-usage request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if v := e.tokenAuthMetric("UsageUnrecognized"); v != 0 {
		e.t.Logf("mod_ai_token_auth monitor body:\n%s", e.tokenAuthMetricsRaw())
		t.Fatalf("UsageUnrecognized = %v, want 0: an explicit all-zero usage is not unrecognized usage", v)
	}
	if !e.waitForTokenAuthMetric("BillingZeroCost", 5*time.Second, func(v float64) bool { return v == 1 }) {
		e.t.Logf("mod_ai_token_auth monitor body:\n%s", e.tokenAuthMetricsRaw())
		t.Fatalf("BillingZeroCost = %v, want 1 after the all-zero-usage arm", e.tokenAuthMetric("BillingZeroCost"))
	}

	// Arm 2: no usage at all and not estimatable (unchanged yardstick: zero
	// deduction, but this one IS unrecognized usage).
	backend.ResponseHeaders = map[string]string{"Content-Type": "text/plain"}
	backend.NoContentLength = true
	backend.Body = `hello, no usage here`
	resp2, body2, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send missing-usage request failed: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp2.StatusCode, body2)
	}
	if !e.waitForTokenAuthMetric("UsageUnrecognized", 5*time.Second, func(v float64) bool { return v == 1 }) {
		e.t.Logf("mod_ai_token_auth monitor body:\n%s", e.tokenAuthMetricsRaw())
		t.Fatalf("UsageUnrecognized = %v, want 1 after the missing-usage arm", e.tokenAuthMetric("UsageUnrecognized"))
	}
	if !e.waitForTokenAuthMetric("BillingZeroCost", 5*time.Second, func(v float64) bool { return v == 2 }) {
		t.Fatalf("BillingZeroCost = %v, want 2 after the missing-usage arm", e.tokenAuthMetric("BillingZeroCost"))
	}

	// Arm 3: normal billing control.
	backend.NoContentLength = false
	backend.ResponseHeaders = map[string]string{"Content-Type": "application/json"}
	backend.Body = usageResponse
	resp3, body3, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send normal request failed: %v", err)
	}
	if resp3.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp3.StatusCode, body3)
	}
	if v := e.tokenAuthMetric("UsageUnrecognized"); v != 1 {
		t.Fatalf("UsageUnrecognized = %v, want 1 (the billed control arm must not be counted)", v)
	}
	if v := e.tokenAuthMetric("BillingZeroCost"); v != 2 {
		t.Fatalf("BillingZeroCost = %v, want 2 (the billed control arm must not be counted)", v)
	}

	if e.backends[clusterRMB].Hits() != 3 {
		t.Fatalf("expected 3 hits on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Async deduction: arms 1 and 2 bill 0, arm 3 bills 100*100 + 50*200.
	// Arm 1's unchanged share is what proves it was not estimated.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 20000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d (arm 1 must not be estimate-billed)", remaining, want)
	}
}

// Issue #1414: every 2xx request that holds an RMB plan and was charged 0 is
// counted by BillingZeroCost at a single checkpoint, so pricing/configuration
// gaps stop being silent while staying distinguishable from unrecognized
// usage (UsageUnrecognized must not move: the usage was recognized).
func TestTC26_RMBQuotaDeduction_BillingZeroCostSignal(t *testing.T) {
	e := newTestEnv(t, map[string]*cluster_conf.AIConf{
		clusterRMB:     defaultRMBAIConf(),
		clusterNoTable: noTableAIConf(),
	}, []common.QuotaPlan{rmbQuotaPlan(10000000000)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	// Arm 1 model_table_missing: the cluster has no AIConf.ModelTable, so the
	// cost computation bails out with 0 (previously only a warn).
	resp, body, err := e.sendRequest("notable.example.org", defaultBody)
	if err != nil {
		t.Fatalf("send no-model-table request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if e.backends[clusterNoTable].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterNoTable, e.backends[clusterNoTable].Hits())
	}
	if !e.waitForTokenAuthMetric("BillingZeroCost", 5*time.Second, func(v float64) bool { return v == 1 }) {
		e.t.Logf("mod_ai_token_auth monitor body:\n%s", e.tokenAuthMetricsRaw())
		t.Fatalf("BillingZeroCost = %v, want 1 after the no-model-table arm", e.tokenAuthMetric("BillingZeroCost"))
	}

	// Arm 2 price_miss: the model is absent from the price table entirely and
	// the chat mode never falls back (TC-23's no-baseline shape).
	resp2, body2, err := e.sendRequest(apiHost, []byte(`{"model":"no-such-model"}`))
	if err != nil {
		t.Fatalf("send no-baseline request failed: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp2.StatusCode, body2)
	}
	if !e.waitForTokenAuthMetric("BillingZeroCost", 5*time.Second, func(v float64) bool { return v == 2 }) {
		t.Fatalf("BillingZeroCost = %v, want 2 after the price-miss arm", e.tokenAuthMetric("BillingZeroCost"))
	}
	if !e.waitForTokenAuthMetric("PriceMissZeroBilled", 5*time.Second, func(v float64) bool { return v == 1 }) {
		t.Fatalf("PriceMissZeroBilled = %v, want 1 (issue #1408 signal must not regress)", e.tokenAuthMetric("PriceMissZeroBilled"))
	}

	// Both zero-cost arms have recognized usage: zero cost is not
	// unrecognized usage, and the two must stay separable.
	if v := e.tokenAuthMetric("UsageUnrecognized"); v != 0 {
		t.Fatalf("UsageUnrecognized = %v, want 0: pricing zero-cost is not unrecognized usage", v)
	}

	// Arm 3 billed control.
	e.backends[clusterRMB].Body = usageResponse
	resp3, body3, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send normal request failed: %v", err)
	}
	if resp3.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp3.StatusCode, body3)
	}
	if v := e.tokenAuthMetric("BillingZeroCost"); v != 2 {
		t.Fatalf("BillingZeroCost = %v, want 2 (the billed control arm must not be counted)", v)
	}

	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyRMB)
	want := int64(10000000000 - 20000)
	if remaining != want {
		e.logBFEException()
		e.logBFEAccess()
		t.Fatalf("remaining quota = %d, want %d", remaining, want)
	}
}
