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

package sc05

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

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	bfe_access_pb "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
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

var usageResponse = `{"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`

// SSE format: final chunk contains usage. The trailing blank line is required.
var streamUsageResponse = "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
	"data: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":50,\"total_tokens\":150}}\n\n"

// testEnv holds all resources for a single SC05 integration test.
type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backends   map[string]*common.MockBackend
	redis      *common.RedisServer
	bfePort    int
	stopBFE    func()
	logDir     string
}

func newTestEnv(t *testing.T, aiConfs map[string]*cluster_conf.AIConf, quotaPlans []common.QuotaPlan,
	enableRateLimit bool) *testEnv {
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
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

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
					Tags: []bfe_basic.ApikeyTag{
						{TagName: "department", TagValue: "ai-team"},
					},
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

	if enableRateLimit {
		if err := e.setupRateLimitConf(confDir); err != nil {
			t.Fatalf("setup rate limit conf failed: %v", err)
		}
		if err := e.enableModuleInBFEConf(confDir, "mod_ai_rate_limit"); err != nil {
			t.Fatalf("enable mod_ai_rate_limit in bfe.conf failed: %v", err)
		}
	}

	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(confDir, e.logDir)
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

func (e *testEnv) accessLogs() []*bfe_access_pb.RequestLog {
	e.t.Helper()
	reqLogs, err := common.ParseAccessLogAfterStop(e.logDir)
	if err != nil {
		e.t.Fatalf("parse access log failed: %v", err)
	}
	return reqLogs
}

func (e *testEnv) mustFindSingleLog(reqLogs []*bfe_access_pb.RequestLog) *bfe_access_pb.RequestLog {
	e.t.Helper()
	if len(reqLogs) == 0 {
		e.t.Fatalf("expected at least 1 access log, got 0")
	}
	return reqLogs[len(reqLogs)-1]
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
					Prices: map[string]float64{
						"input_cost_per_token":  0.000001,
						"output_cost_per_token": 0.000002,
					},
				},
			},
		},
	}
}

func multiKeyRMBAIConf() *cluster_conf.AIConf {
	conf := defaultRMBAIConf()
	conf.Keys = []cluster_conf.AIKey{
		{Name: "key-primary", Key: "sk-primary", Weight: 100},
		{Name: "key-secondary", Key: "sk-secondary", Weight: 100},
	}
	conf.KeyPolicy.MaxRetries = 2
	return conf
}

func fallbackRMBAIConf() *cluster_conf.AIConf {
	conf := defaultRMBAIConf()
	conf.Provider = "mock-provider-fallback"
	conf.ModelTable.Models[0].Prices = map[string]float64{
		"input_cost_per_token":  0.000003,
		"output_cost_per_token": 0.000004,
	}
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


// TestTC01 verifies all major AI access log fields for a successful RMB request.
func TestTC01_SuccessfulRequestAIFields(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)}, false)
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

	// Wait for access log to be flushed before stopping BFE.
	time.Sleep(500 * time.Millisecond)

	e.stopBFE()
	e.stopBFE = nil

	reqLog := e.mustFindSingleLog(e.accessLogs())
	assertStringField(t, reqLog.AiApikeyId, "ai_apikey_id", apiKeyId)
	assertApikeyTags(t, reqLog.AiApikeytags, []bfe_basic.ApikeyTag{{TagName: "department", TagValue: "ai-team"}})
	assertStringField(t, reqLog.AiRequestedModel, "ai_requested_model", "deepseek-chat")
	assertStringField(t, reqLog.AiTargetModel, "ai_target_model", "deepseek-chat")
	assertStringField(t, reqLog.AiProvider, "ai_provider", "mock-provider")
	assertInt64Field(t, reqLog.AiInputTokens, "ai_input_tokens", 100)
	assertInt64Field(t, reqLog.AiOutputTokens, "ai_output_tokens", 50)
	assertInt64Field(t, reqLog.AiTotalTokens, "ai_total_tokens", 150)
	assertInt64Field(t, reqLog.AiCostValue, "ai_cost_value", 100*100+50*200)
	assertStringField(t, reqLog.AiCostCurrency, "ai_cost_currency", "RMB")
	if reqLog.AiRetryCount != nil && *reqLog.AiRetryCount != 0 {
		t.Errorf("ai_retry_count should be 0 or nil, got %d", *reqLog.AiRetryCount)
	}
	assertRouteRuleHits(t, reqLog.AiRouteRuleHits, []expectedRouteRuleHit{{Owner: "ak_user_a", OwnerType: "apikey", RuleName: "user_a-rmb"}})
	assertClusterKeyNames(t, reqLog.AiClusterKeyNames, []expectedClusterKeyName{{ClusterName: clusterRMB, KeyName: "key-primary"}})
	assertStringSliceField(t, reqLog.AiAuthHitQuotaPlans, "ai_auth_hit_quota_plans", []string{planRMB})
}

// TestTC02 verifies ai_requested_model and ai_target_model after ModelMapping.
func TestTC02_ModelMappingTargetModel(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)}, false)
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

	// Wait for access log to be flushed before stopping BFE.
	time.Sleep(500 * time.Millisecond)

	e.stopBFE()
	e.stopBFE = nil

	reqLog := e.mustFindSingleLog(e.accessLogs())
	assertStringField(t, reqLog.AiRequestedModel, "ai_requested_model", "gpt-4")
	assertStringField(t, reqLog.AiTargetModel, "ai_target_model", "deepseek-chat")
	assertInt64Field(t, reqLog.AiCostValue, "ai_cost_value", 100*100+50*200)
}

// TestTC03 verifies ai_retry_count and ai_cluster_key_names during key-level retry.
func TestTC03_KeyRetryCountAndClusterKeyNames(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: multiKeyRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)}, false)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	// First request returns 500, subsequent requests return 200.
	e.backends[clusterRMB].ResponseFunc = func(r *http.Request, count int) (int, string) {
		if count == 1 {
			return http.StatusInternalServerError, ""
		}
		return http.StatusOK, usageResponse
	}

	resp, body, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends[clusterRMB].Hits() < 2 {
		t.Fatalf("expected at least 2 hits on %s, got %d", clusterRMB, e.backends[clusterRMB].Hits())
	}

	// Wait for access log to be flushed before stopping BFE.
	time.Sleep(500 * time.Millisecond)

	e.stopBFE()
	e.stopBFE = nil

	reqLog := e.mustFindSingleLog(e.accessLogs())
	if reqLog.AiRetryCount == nil || *reqLog.AiRetryCount == 0 {
		t.Errorf("ai_retry_count should be > 0, got %v", reqLog.AiRetryCount)
	}
	if len(reqLog.AiClusterKeyNames) < 2 {
		t.Errorf("expected at least 2 cluster_key_names, got %d: %s", len(reqLog.AiClusterKeyNames), common.FormatAccessLogError(reqLog))
	}
	for _, ckn := range reqLog.AiClusterKeyNames {
		if ckn.GetClusterName() != clusterRMB {
			t.Errorf("expected cluster_name %s, got %s", clusterRMB, ckn.GetClusterName())
		}
	}
}

// TestTC04 verifies ai_auth_reject_reason and ai_auth_reject_quota_plans when quota exhausted.
func TestTC04_QuotaExhaustedRejectFields(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(0)}, false)
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
	if e.backends[clusterRMB].Hits() != 0 {
		t.Fatalf("expected no backend hit, got %d", e.backends[clusterRMB].Hits())
	}

	// Wait for access log to be flushed before stopping BFE.
	time.Sleep(500 * time.Millisecond)

	e.stopBFE()
	e.stopBFE = nil

	reqLog := e.mustFindSingleLog(e.accessLogs())
	assertStringField(t, reqLog.AiApikeyId, "ai_apikey_id", apiKeyId)
	if reqLog.AiAuthRejectReason == nil || *reqLog.AiAuthRejectReason == "" {
		t.Errorf("ai_auth_reject_reason should not be empty")
	}
	assertStringSliceField(t, reqLog.AiAuthRejectQuotaPlans, "ai_auth_reject_quota_plans", []string{planRMB})
	if len(reqLog.AiRouteRuleHits) != 0 {
		t.Errorf("expected no route rule hits on rejected request, got %d", len(reqLog.AiRouteRuleHits))
	}
	if len(reqLog.AiClusterKeyNames) != 0 {
		t.Errorf("expected no cluster_key_names on rejected request, got %d", len(reqLog.AiClusterKeyNames))
	}
}

// TestTC05 verifies ai_provider and ai_cluster_key_names after cluster fallback.
func TestTC05_FallbackProviderAndClusterKeyNames(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB:         defaultRMBAIConf(),
		clusterFallbackRMB: fallbackRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)}, false)
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

	// Wait for access log to be flushed before stopping BFE.
	time.Sleep(500 * time.Millisecond)

	e.stopBFE()
	e.stopBFE = nil

	reqLog := e.mustFindSingleLog(e.accessLogs())
	assertStringField(t, reqLog.AiProvider, "ai_provider", "mock-provider-fallback")
	assertInt64Field(t, reqLog.AiCostValue, "ai_cost_value", 100*300+50*400)
	assertRouteRuleHits(t, reqLog.AiRouteRuleHits, []expectedRouteRuleHit{{Owner: "ak_user_a", OwnerType: "apikey", RuleName: "user_a-rmb"}})
	if len(reqLog.AiClusterKeyNames) < 2 {
		t.Errorf("expected at least 2 cluster_key_names, got %d: %s", len(reqLog.AiClusterKeyNames), common.FormatAccessLogError(reqLog))
	}
}

// TestTC06 verifies ai_stream, ai_ttft_us and ai_tpot_us for SSE responses.
func TestTC06_StreamingFields(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)}, false)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)
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

	// Wait for async redis deduction and log flush after response finishes.
	time.Sleep(500 * time.Millisecond)

	e.stopBFE()
	e.stopBFE = nil

	reqLog := e.mustFindSingleLog(e.accessLogs())
	if reqLog.AiStream == nil || !*reqLog.AiStream {
		t.Errorf("ai_stream should be true")
	}
	if reqLog.AiTtftUs == nil || *reqLog.AiTtftUs <= 0 {
		t.Errorf("ai_ttft_us should be > 0, got %v", reqLog.AiTtftUs)
	}
	if reqLog.AiTpotUs == nil || *reqLog.AiTpotUs <= 0 {
		t.Errorf("ai_tpot_us should be > 0, got %v", reqLog.AiTpotUs)
	}
	assertInt64Field(t, reqLog.AiInputTokens, "ai_input_tokens", 100)
	assertInt64Field(t, reqLog.AiOutputTokens, "ai_output_tokens", 50)
}


type expectedRouteRuleHit struct {
	Owner     string
	OwnerType string
	RuleName  string
}

type expectedClusterKeyName struct {
	ClusterName string
	KeyName     string
}

func assertStringField(t *testing.T, ptr *string, name, want string) {
	t.Helper()
	if ptr == nil {
		t.Errorf("%s is nil, want %s", name, want)
		return
	}
	if *ptr != want {
		t.Errorf("%s = %s, want %s", name, *ptr, want)
	}
}

func assertInt64Field(t *testing.T, ptr *int64, name string, want int64) {
	t.Helper()
	if ptr == nil {
		t.Errorf("%s is nil, want %d", name, want)
		return
	}
	if *ptr != want {
		t.Errorf("%s = %d, want %d", name, *ptr, want)
	}
}

func assertStringSliceField(t *testing.T, got []string, name string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s = %v, want %v", name, got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %s, want %s", name, i, got[i], want[i])
		}
	}
}

func assertApikeyTags(t *testing.T, got []*bfe_access_pb.ApikeyTag, want []bfe_basic.ApikeyTag) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("ai_apikeytags length = %d, want %d", len(got), len(want))
		return
	}
	for i, w := range want {
		if got[i].GetTagname() != w.TagName || got[i].GetTagvalue() != w.TagValue {
			t.Errorf("ai_apikeytags[%d] = (%s,%s), want (%s,%s)", i,
				got[i].GetTagname(), got[i].GetTagvalue(), w.TagName, w.TagValue)
		}
	}
}

func assertRouteRuleHits(t *testing.T, got []*bfe_access_pb.AIRouteRuleHit, want []expectedRouteRuleHit) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("ai_route_rule_hits length = %d, want %d", len(got), len(want))
		return
	}
	for i, w := range want {
		if got[i].GetRuleOwner() != w.Owner || got[i].GetRuleOwnerType() != w.OwnerType || got[i].GetRuleName() != w.RuleName {
			t.Errorf("ai_route_rule_hits[%d] = (%s,%s,%s), want (%s,%s,%s)", i,
				got[i].GetRuleOwner(), got[i].GetRuleOwnerType(), got[i].GetRuleName(),
				w.Owner, w.OwnerType, w.RuleName)
		}
	}
}

func assertClusterKeyNames(t *testing.T, got []*bfe_access_pb.ClusterKeyName, want []expectedClusterKeyName) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("ai_cluster_key_names length = %d, want %d", len(got), len(want))
		return
	}
	for i, w := range want {
		if got[i].GetClusterName() != w.ClusterName || got[i].GetKeyName() != w.KeyName {
			t.Errorf("ai_cluster_key_names[%d] = (%s,%s), want (%s,%s)", i,
				got[i].GetClusterName(), got[i].GetKeyName(), w.ClusterName, w.KeyName)
		}
	}
}

// enableModuleInBFEConf appends a module to the Modules list in bfe.conf.
func (e *testEnv) enableModuleInBFEConf(confDir, modName string) error {
	path := filepath.Join(confDir, "bfe.conf")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if !strings.Contains(content, "Modules = "+modName) {
		content += "\nModules = " + modName + "\n"
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// setupRateLimitConf writes mod_ai_rate_limit configuration files.
func (e *testEnv) setupRateLimitConf(confDir string) error {
	modDir := filepath.Join(confDir, "mod_ai_rate_limit")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		return err
	}

	confContent := `[basic]
ProductRulePath = mod_ai_rate_limit/ai_rate_limit.data
IsRejectOnRedisError = true

[redis]
bns = redis_bns
connectTimeout = 20
readTimeout = 20
writeTimeout = 20
maxIdle = 20

[log]
OpenDebug = false
`
	if err := os.WriteFile(filepath.Join(modDir, "mod_ai_rate_limit.conf"), []byte(confContent), 0644); err != nil {
		return err
	}

	rateLimitData := map[string]interface{}{
		"Version": "1.0",
		"Config": map[string]interface{}{
			"ai_product": []map[string]interface{}{
				{
					"cond": "default_t()",
					"hit_action": map[string]interface{}{
						"cmd":    "FINISH",
						"params": []string{},
					},
				},
			},
		},
		"RateLimitPolicies": map[string]interface{}{
			"rlp-rpm-1": map[string]interface{}{
				"name":    "ratelimitRPM",
				"enabled": true,
				"rules": map[string]interface{}{
					"rpm": []map[string]interface{}{
						{
							"name":           "rpm1",
							"window_minutes": 1,
							"max_requests":   1,
							"burst":          1,
						},
					},
				},
			},
		},
		"ApikeyRateLimitPolicyBindings": map[string]interface{}{
			apiKey: []string{"rlp-rpm-1"},
		},
	}
	data, err := json.MarshalIndent(rateLimitData, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(modDir, "ai_rate_limit.data"), data, 0644)
}




// TestTC07 verifies ai_rate_limit_hits when RPM limit is triggered.
func TestTC07_RateLimitHits(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{rmbQuotaPlan(10000000000)}, true)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000000000)

	// First request should succeed.
	resp1, body1, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send first request failed: %v", err)
	}
	if resp1.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected first status 200, got %d, body: %s", resp1.StatusCode, body1)
	}

	// Second request should be rate limited (rpm max_requests=1).
	resp2, body2, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send second request failed: %v", err)
	}
	if resp2.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected second status 429, got %d, body: %s", resp2.StatusCode, body2)
	}

	// Wait for access log to be flushed before stopping BFE.
	time.Sleep(500 * time.Millisecond)

	e.stopBFE()
	e.stopBFE = nil

	reqLogs := e.accessLogs()
	if len(reqLogs) != 2 {
		t.Fatalf("expected 2 access logs, got %d", len(reqLogs))
	}

	// First log should have no rate limit hits.
	if len(reqLogs[0].AiRateLimitHits) != 0 {
		t.Errorf("first request should have no rate limit hits, got %d", len(reqLogs[0].AiRateLimitHits))
	}

	// Second log should record the RPM hit.
	if len(reqLogs[1].AiRateLimitHits) == 0 {
		t.Fatalf("second request should have rate limit hits, got 0: %s", common.FormatAccessLogError(reqLogs[1]))
	}
	hit := reqLogs[1].AiRateLimitHits[0]
	if hit.GetRateLimitPolicyId() != "rlp-rpm-1" {
		t.Errorf("rate_limit_policy_id = %s, want rlp-rpm-1", hit.GetRateLimitPolicyId())
	}
	if hit.GetRateLimitType() != "rpm" {
		t.Errorf("rate_limit_type = %s, want rpm", hit.GetRateLimitType())
	}
	if len(hit.GetRuleNames()) == 0 {
		t.Errorf("rate_limit rule_names should not be empty")
	}
}
