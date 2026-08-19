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

var usageResponse = `{"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`

// testEnv holds all resources for a single SC03 integration test.
type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backends   map[string]*common.MockBackend
	redis      *common.RedisServer
	bfePort    int
	stopBFE    func()
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
					Enabled:        1,
					Status:         1,
					UpdateTime:     0,
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

	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(confDir, logDir)
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

func fallbackRMBAIConf() *cluster_conf.AIConf {
	conf := defaultRMBAIConf()
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
		CreateTime:  0,
		ExpiredTime: -1,
		Quota:       quota,
		ResetMode:   0,
		Unit:        "RMB",
	}
}

func tokenQuotaPlan(quota int64) common.QuotaPlan {
	return common.QuotaPlan{
		Id:          planToken,
		Unlimited:   false,
		PassNoQuota: false,
		RedisKey:    redisKeyToken,
		CreateTime:  0,
		ExpiredTime: -1,
		Quota:       quota,
		ResetMode:   0,
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
