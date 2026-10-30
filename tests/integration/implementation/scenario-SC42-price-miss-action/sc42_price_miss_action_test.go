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

// Package sc42 verifies the missing-price actions and shape signals introduced
// by bfe issue #1479: AIConf.PriceMissAction (fallback_chat / bill_zero /
// reject) decide what happens when the requested (model, mode) has no price
// entry, and the four signal layers (structured PRICE_MISS log, counters,
// ai_price_miss / ai_price_warn access-log fields) keep the gap observable.
package sc42

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bfe_access_pb "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiKey   = "ak_user_a"
	apiKeyId = "user_a_key_id"

	hostFallback = "price-fallback.example.org"
	hostBillZero = "price-billzero.example.org"
	hostReject   = "price-reject.example.org"
	hostSubitem  = "price-subitem.example.org"

	clusterFallback = "cluster_price_fallback"
	clusterBillZero = "cluster_price_billzero"
	clusterReject   = "cluster_price_reject"
	clusterSubitem  = "cluster_price_subitem"

	planRMB     = "plan_rmb"
	redisKeyRMB = "quota:plan_rmb"

	initialQuota = int64(10000000000)

	// chat-mode price of the test model: 300 / 900 fixed units per token.
	inputUnit  = int64(300)
	outputUnit = int64(900)
)

var (
	// Responses-API request whose mode ("responses") has no price entry in the
	// test clusters: only a chat entry exists for the model.
	responsesBody = []byte(`{"model":"gpt-5-codex","input":"hi","max_output_tokens":16}`)

	// Upstream usage returned by the mock backends. Responses-style keys are
	// parsed cross-protocol (input_tokens/output_tokens); the sub-item arm
	// additionally reports cache tokens.
	plainUsage  = `{"usage":{"input_tokens":100,"output_tokens":50}}`
	cacheUsage  = `{"usage":{"input_tokens":100,"output_tokens":50,"cache_read_tokens":40,"cache_write_tokens":10}}`
	expectPlain = int64(100)*inputUnit + int64(50)*outputUnit
)

type testEnv struct {
	t              *testing.T
	processEnv     *common.ProcessEnv
	backends       map[string]*common.MockBackend
	redis          *common.RedisServer
	bfePort        int
	bfeMonitorPort int
	logDir         string
	stopBFE        func()
}

func newTestEnv(t *testing.T) *testEnv {
	e := &testEnv{t: t, backends: make(map[string]*common.MockBackend)}

	e.redis = common.NewRedisServer(t)
	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()
	confDir := filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	e.backends[clusterFallback] = common.NewMockBackend(clusterFallback, http.StatusOK, plainUsage)
	e.backends[clusterBillZero] = common.NewMockBackend(clusterBillZero, http.StatusOK, plainUsage)
	e.backends[clusterReject] = common.NewMockBackend(clusterReject, http.StatusOK, plainUsage)
	e.backends[clusterSubitem] = common.NewMockBackend(clusterSubitem, http.StatusOK, cacheUsage)

	tokenRule := &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": {{
				Id:          planRMB,
				Unlimited:   false,
				PassNoQuota: false,
				RedisKey:    redisKeyRMB,
				ExpiredTime: -1,
				Quota:       initialQuota,
				Unit:        "RMB",
			}},
		},
		Tokens: map[string]map[string]common.TokenFile{
			"ai_product": {apiKey: {
				Key:            apiKey,
				KeyId:          apiKeyId,
				Enabled:        true,
				ExpiredTime:    -1,
				UnlimitedQuota: false,
				QuotaPlans:     []string{planRMB},
			}},
		},
		Config: map[string][]common.TokenRule{
			"ai_product": {{Cond: "default_t()", Action: common.ActionFile{Cmd: "CHECK_TOKEN"}}},
		},
	}

	aiConfs := map[string]*cluster_conf.AIConf{
		clusterFallback: priceMissAIConf(cluster_conf.PriceMissActionFallbackChat),
		clusterBillZero: priceMissAIConf(cluster_conf.PriceMissActionBillZero),
		clusterReject:   priceMissAIConf(cluster_conf.PriceMissActionReject),
		clusterSubitem:  priceMissAIConf(cluster_conf.PriceMissActionFallbackChat),
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

	e.bfePort, e.bfeMonitorPort, e.stopBFE = e.processEnv.StartBFE(confDir, e.logDir)
	return e
}

// priceMissAIConf builds a cluster AIConf with only a chat-mode price entry for
// the test model, so a responses-mode request hits the missing-price path. The
// chat entry deliberately omits cache sub-item prices.
func priceMissAIConf(action string) *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type:            0,
		Provider:        "mock-provider",
		Keys:            []cluster_conf.AIKey{{Name: "key-primary", Key: "sk-primary", Weight: 100}},
		PriceMissAction: action,
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:            "weighted_random",
			MaxRetries:          0,
			RetryBackoffInitial: 50,
			RetryBackoffMax:     200,
		},
		ModelTable: &cluster_conf.ModelTable{
			Currency: "RMB",
			Models: []cluster_conf.ModelPrice{{
				Provider:     "mock-provider",
				Model:        "gpt-5-codex",
				BaseModel:    "gpt-5-codex",
				Mode:         "chat",
				Capabilities: []string{"chat"},
				Prices: cluster_conf.PriceMap{
					"input_cost_per_token":  0.000003,
					"output_cost_per_token": 0.000009,
				},
			}},
		},
	}
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

// post sends one request and returns status and body.
func (e *testEnv) post(host string, body []byte) (int, string) {
	e.t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/responses", e.bfePort)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		e.t.Fatalf("new request failed: %v", err)
	}
	req.Host = host
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		e.t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func (e *testEnv) quotaRemaining() int64 {
	e.t.Helper()
	// The deduction happens synchronously in the finish handler.
	time.Sleep(500 * time.Millisecond)
	return e.redis.GetQuota(redisKeyRMB)
}

func (e *testEnv) tokenAuthMetric(field string) float64 {
	e.t.Helper()
	v, err := common.ModuleMetric(e.bfeMonitorPort, "mod_ai_token_auth", field)
	if err != nil {
		e.t.Fatalf("read mod_ai_token_auth.%s failed: %v", field, err)
	}
	return v
}

func (e *testEnv) waitTokenAuthMetric(field string, want float64) {
	e.t.Helper()
	_, err := common.WaitForModuleMetric(e.bfeMonitorPort, "mod_ai_token_auth", field,
		10*time.Second, func(v float64) bool { return v >= want })
	if err != nil {
		e.t.Fatalf("wait mod_ai_token_auth.%s >= %v failed: %v", field, want, err)
	}
}

// stopAndLogs stops BFE (flushing the pb3 access log) and returns the parsed
// rows.
func (e *testEnv) stopAndLogs() []*bfe_access_pb.RequestLog {
	e.t.Helper()
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	logs, err := common.ParseAccessLogAfterStop(e.logDir)
	if err != nil {
		e.t.Fatalf("parse access log failed: %v", err)
	}
	if len(logs) == 0 {
		e.t.Fatal("expected at least 1 access log row, got 0")
	}
	return logs
}

func logPriceWarn(log *bfe_access_pb.RequestLog) string {
	if log.AiPriceWarn == nil {
		return ""
	}
	return *log.AiPriceWarn
}

func logInt64(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}

// TestTC01 verifies the fallback_chat action: a responses request whose mode
// has no price entry is billed at the model's chat price, and the access log
// reports ai_price_miss + ai_price_warn=fallback.
func TestTC01_FallbackChatBillsAndMarks(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	status, body := e.post(hostFallback, responsesBody)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	if hits := e.backends[clusterFallback].Hits(); hits != 1 {
		t.Fatalf("backend hits = %d, want 1", hits)
	}
	if remaining := e.quotaRemaining(); remaining != initialQuota-expectPlain {
		t.Fatalf("remaining = %d, want %d (billed at the chat price)", remaining, initialQuota-expectPlain)
	}
	if got := e.tokenAuthMetric("PriceFallbackUsed"); got != 1 {
		t.Errorf("PriceFallbackUsed = %v, want 1", got)
	}

	logs := e.stopAndLogs()
	log := logs[len(logs)-1]
	if log.AiPriceMiss == nil || !*log.AiPriceMiss {
		t.Error("ai_price_miss should be true")
	}
	if got := logPriceWarn(log); got != "fallback" {
		t.Errorf("ai_price_warn = %q, want %q", got, "fallback")
	}
	if got := logInt64(log.AiCostValue); got != expectPlain {
		t.Errorf("ai_cost_value = %d, want %d", got, expectPlain)
	}
}

// TestTC02 verifies the bill_zero action: the request succeeds but is billed 0,
// the miss is still marked (ai_price_warn=no_entry + ai_zero_cost_reason).
func TestTC02_BillZeroActionMarksAndChargesNothing(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	status, body := e.post(hostBillZero, responsesBody)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	if hits := e.backends[clusterBillZero].Hits(); hits != 1 {
		t.Fatalf("backend hits = %d, want 1 (bill_zero still calls the upstream)", hits)
	}
	if remaining := e.quotaRemaining(); remaining != initialQuota {
		t.Fatalf("remaining = %d, want %d (bill_zero charges nothing)", remaining, initialQuota)
	}

	logs := e.stopAndLogs()
	log := logs[len(logs)-1]
	if log.AiPriceMiss == nil || !*log.AiPriceMiss {
		t.Error("ai_price_miss should be true")
	}
	if got := logPriceWarn(log); got != "no_entry" {
		t.Errorf("ai_price_warn = %q, want %q", got, "no_entry")
	}
	if got := logInt64(log.AiCostValue); got != 0 {
		t.Errorf("ai_cost_value = %d, want 0", got)
	}
	if log.AiZeroCostReason == nil || *log.AiZeroCostReason != "price_miss" {
		t.Errorf("ai_zero_cost_reason = %v, want price_miss", log.AiZeroCostReason)
	}
}

// TestTC03 verifies the reject action: the request is rejected before
// forwarding with 503 PRICE_NOT_CONFIGURED, the upstream is never called, the
// balance is untouched, and the (non-2xx) row still carries ai_price_miss +
// ai_price_warn=reject (issue #1479 plan A).
func TestTC03_RejectActionRejectsBeforeForwarding(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	status, body := e.post(hostReject, responsesBody)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", status, body)
	}
	if !strings.Contains(body, "PRICE_NOT_CONFIGURED") {
		t.Fatalf("error code PRICE_NOT_CONFIGURED missing from body: %s", body)
	}
	if hits := e.backends[clusterReject].Hits(); hits != 0 {
		t.Fatalf("backend hits = %d, want 0 (rejected before forwarding)", hits)
	}
	if remaining := e.quotaRemaining(); remaining != initialQuota {
		t.Fatalf("remaining = %d, want %d (rejected requests are not billed)", remaining, initialQuota)
	}
	e.waitTokenAuthMetric("PriceRejected", 1)

	logs := e.stopAndLogs()
	log := logs[len(logs)-1]
	if got := log.GetResStatusCode(); got != http.StatusServiceUnavailable {
		t.Errorf("res_status_code = %d, want 503", got)
	}
	if log.AiPriceMiss == nil || !*log.AiPriceMiss {
		t.Error("ai_price_miss must be written for a rejected request (issue #1479 plan A)")
	}
	if got := logPriceWarn(log); got != "reject" {
		t.Errorf("ai_price_warn = %q, want %q", got, "reject")
	}
}

// TestTC04 verifies the accepted residual: a fallback-billed request that used
// cache tokens while the borrowed chat entry lacks cache prices is still billed
// (cache dimensions at 0), reported as ai_price_warn=subitem_missing and
// counted by PriceFallbackSubItemMiss — never rejected.
func TestTC04_FallbackSubitemMissingIsCountedNotRejected(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	status, body := e.post(hostSubitem, responsesBody)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (the residual is accepted, not rejected); body: %s", status, body)
	}
	if hits := e.backends[clusterSubitem].Hits(); hits != 1 {
		t.Fatalf("backend hits = %d, want 1", hits)
	}
	if remaining := e.quotaRemaining(); remaining >= initialQuota {
		t.Fatalf("remaining = %d, want a non-zero deduction (fallback price applies)", remaining)
	}
	e.waitTokenAuthMetric("PriceFallbackSubItemMiss", 1)

	logs := e.stopAndLogs()
	log := logs[len(logs)-1]
	if got := logPriceWarn(log); got != "subitem_missing" {
		t.Errorf("ai_price_warn = %q, want %q (sub-item missing is the most specific shape)", got, "subitem_missing")
	}
	if log.AiPriceMiss == nil || !*log.AiPriceMiss {
		t.Error("ai_price_miss should be true")
	}
}
