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

package sc39

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

	bfe_access_pb "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost = "auth.example.org"
	apiPath = "/v1/chat/completions"

	clusterAuth = "cluster_auth"

	apiKey      = "ak_auth"
	apiKeyId    = "auth_key_id"
	apiKeyTwo   = "ak_auth_second"
	apiKeyIdTwo = "auth_second_key_id"

	planUnlimited = "plan-unlimited"
	planToken     = "plan-token"
	planPass      = "plan-pass"

	redisKeyToken = "quota:token"
	redisKeyPass  = "quota:pass"
)

var defaultBody = []byte(`{"model":"gpt-4"}`)
var usageResponse = `{"id":"chatcmpl-sc39","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`

type aiErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Type    string `json:"type"`
		Message string `json:"message"`
		Details struct {
			ApiKey      string `json:"api_key"`
			KeyId       string `json:"key_id"`
			QuotaPlanId string `json:"quota_plan_id"`
			LimitType   string `json:"limit_type"`
		} `json:"details"`
	} `json:"error"`
}

func parseAIError(t *testing.T, body string) aiErrorBody {
	t.Helper()
	var eb aiErrorBody
	if err := json.Unmarshal([]byte(body), &eb); err != nil {
		t.Fatalf("parse AiError body failed: %v, body: %s", err, body)
	}
	return eb
}

// tokenSpec describes one api key written into token_rule.data.
type tokenSpec struct {
	key            string
	keyId          string
	enabled        bool
	expiredTime    int64
	unlimitedQuota bool
	subnet         string
	plans          []string
}

func apiToken(key, keyId string, plans ...string) tokenSpec {
	return tokenSpec{
		key:         key,
		keyId:       keyId,
		enabled:     true,
		expiredTime: -1,
		plans:       plans,
	}
}

func unlimitedToken(key, keyId string) tokenSpec {
	return tokenSpec{
		key:            key,
		keyId:          keyId,
		enabled:        true,
		expiredTime:    -1,
		unlimitedQuota: true,
	}
}

func tokenPlan(id, redisKey string, quota int64) common.QuotaPlan {
	return common.QuotaPlan{
		Id:          id,
		Unlimited:   false,
		PassNoQuota: false,
		RedisKey:    redisKey,
		ExpiredTime: -1,
		Quota:       quota,
		Unit:        "total_token",
	}
}

// testOptions controls the token rule/data and startup picture of one test.
type testOptions struct {
	plans        []common.QuotaPlan
	tokens       []tokenSpec
	cond         string // empty => default_t()
	initialQuota map[string]int64
	// clearDefaultPath removes ProductRulePath from mod_ai_token_auth.conf so
	// the module falls back to its built-in default path.
	clearDefaultPath bool
}

type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backend    *common.MockBackend
	redis      *common.RedisServer

	confDir     string
	logDir      string
	bfePort     int
	monitorPort int
	stopBFE     func()
}

func newTestEnv(t *testing.T, opts testOptions) *testEnv {
	e := &testEnv{t: t}

	e.backend = common.NewMockBackend(clusterAuth, http.StatusOK, usageResponse)
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	e.startBFE(opts)

	for key, quota := range opts.initialQuota {
		e.redis.SetQuota(key, quota)
	}
	return e
}

func (e *testEnv) startBFE(opts testOptions) {
	tokenRule := buildTokenRule(opts)

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends: map[string]*common.MockBackend{
			clusterAuth: e.backend,
		},
		AIConfs: map[string]*cluster_conf.AIConf{
			clusterAuth: singleKeyAIConf(),
		},
		RedisAddr:     e.redis.Addr(),
		TokenRuleData: tokenRule,
	}
	if err := builder.Build(); err != nil {
		e.t.Fatalf("build bfe config failed: %v", err)
	}

	if opts.clearDefaultPath {
		confPath := filepath.Join(e.confDir, "mod_ai_token_auth", "mod_ai_token_auth.conf")
		if err := common.RewriteConfLine(confPath, "ProductRulePath", ""); err != nil {
			e.t.Fatalf("clear ProductRulePath failed: %v", err)
		}
	}

	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
}

func singleKeyAIConf() *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type: 0,
		Keys: []cluster_conf.AIKey{
			{Name: "key-1", Key: "sk-sc39-1", Weight: 100},
		},
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:            "weighted_random",
			MaxRetries:          0,
			RetryBackoffInitial: 20,
			RetryBackoffMax:     50,
		},
	}
}

func buildTokenRule(opts testOptions) *common.TokenRuleData {
	cond := opts.cond
	if cond == "" {
		cond = "default_t()"
	}

	plans := opts.plans
	if plans == nil {
		// Emit an empty JSON array instead of null: the module rejects a null
		// quota plan list for a configured product.
		plans = []common.QuotaPlan{}
	}

	tokens := make(map[string]common.TokenFile, len(opts.tokens))
	for _, spec := range opts.tokens {
		token := common.TokenFile{
			Key:            spec.key,
			KeyId:          spec.keyId,
			Enabled:        spec.enabled,
			ExpiredTime:    spec.expiredTime,
			UnlimitedQuota: spec.unlimitedQuota,
			QuotaPlans:     spec.plans,
		}
		if spec.subnet != "" {
			subnet := spec.subnet
			token.Subnet = &subnet
		}
		tokens[spec.key] = token
	}

	return &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": plans,
		},
		Tokens: map[string]map[string]common.TokenFile{
			"ai_product": tokens,
		},
		Config: map[string][]common.TokenRule{
			"ai_product": {
				{
					Cond:   cond,
					Action: common.ActionFile{Cmd: "CHECK_TOKEN"},
				},
			},
		},
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
	data, err := os.ReadFile(filepath.Join(e.logDir, "exception.log"))
	if err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}

	logPath := filepath.Join(e.logDir, "bfe.log")
	logData, err := os.ReadFile(logPath)
	if err == nil && len(logData) > 0 {
		lines := strings.Split(string(logData), "\n")
		start := 0
		if len(lines) > 120 {
			start = len(lines) - 120
		}
		e.t.Logf("bfe log tail:\n%s", strings.Join(lines[start:], "\n"))
	}
}

// stopAndParseLogs stops BFE so the access log is flushed, then returns the
// decoded request logs.
func (e *testEnv) stopAndParseLogs() []*bfe_access_pb.RequestLog {
	e.t.Helper()
	// mod_access_pb3 writes through log4go, which flushes on a short interval;
	// give it a beat before the process is killed so the row is not lost.
	time.Sleep(500 * time.Millisecond)
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	logs, err := common.ParseAccessLogAfterStop(e.logDir)
	if err != nil {
		e.t.Fatalf("parse access log failed: %v", err)
	}
	return logs
}

func (e *testEnv) sendRequest(key string, body []byte) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second,
		// Local rejections may close the connection; avoid reusing a stale
		// pooled connection for the next POST (not retried automatically).
		Transport: &http.Transport{DisableKeepAlives: true},
	}
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

func (e *testEnv) sendAuthenticated() (*http.Response, string, error) {
	return e.sendRequest(apiKey, defaultBody)
}

// waitForQuota polls a quota key until it reaches want.
func (e *testEnv) waitForQuota(key string, want int64) int64 {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var got int64
	for time.Now().Before(deadline) {
		got = e.redis.GetQuota(key)
		if got == want {
			return got
		}
		time.Sleep(50 * time.Millisecond)
	}
	return got
}

func lastRequestLog(t *testing.T, logs []*bfe_access_pb.RequestLog) *bfe_access_pb.RequestLog {
	t.Helper()
	if len(logs) == 0 {
		t.Fatalf("expected at least 1 request log, got 0")
	}
	return logs[len(logs)-1]
}

// TestTC01_NoApiKey verifies a request without an api key is rejected with
// NO_API_KEY and the rejection reason lands in the access log.
func TestTC01_NoApiKey(t *testing.T) {
	e := newTestEnv(t, testOptions{
		plans:  []common.QuotaPlan{tokenPlan(planToken, redisKeyToken, 100000)},
		tokens: []tokenSpec{apiToken(apiKey, apiKeyId, planToken)},
	})

	resp, body, err := e.sendRequest("", defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		e.logBFEException()
		t.Fatalf("expected 401, got %d, body: %s", resp.StatusCode, body)
	}
	if eb := parseAIError(t, body); eb.Error.Code != "NO_API_KEY" {
		t.Fatalf("expected code NO_API_KEY, got %q, body: %s", eb.Error.Code, body)
	}
	if hits := e.backend.Hits(); hits != 0 {
		t.Fatalf("rejected request must not reach the backend, hits=%d", hits)
	}

	logs := e.stopAndParseLogs()
	reqLog := lastRequestLog(t, logs)
	if got := reqLog.GetAiAuthRejectReason(); got != "NO_API_KEY" {
		t.Fatalf("expected ai_auth_reject_reason NO_API_KEY, got %q", got)
	}
}

// TestTC02_InvalidApiKey verifies an unknown api key is rejected with
// INVALID_API_KEY and the key is echoed in details.
func TestTC02_InvalidApiKey(t *testing.T) {
	e := newTestEnv(t, testOptions{
		plans:  []common.QuotaPlan{tokenPlan(planToken, redisKeyToken, 100000)},
		tokens: []tokenSpec{apiToken(apiKey, apiKeyId, planToken)},
	})
	defer e.Close()

	resp, body, err := e.sendRequest("ak_wrong", defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		e.logBFEException()
		t.Fatalf("expected 401, got %d, body: %s", resp.StatusCode, body)
	}
	eb := parseAIError(t, body)
	if eb.Error.Code != "INVALID_API_KEY" {
		t.Fatalf("expected code INVALID_API_KEY, got %q, body: %s", eb.Error.Code, body)
	}
	if eb.Error.Details.ApiKey != "ak_wrong" {
		t.Fatalf("expected details.api_key=ak_wrong, got %q", eb.Error.Details.ApiKey)
	}
}

// TestTC03_KeyDisabled verifies a disabled key is rejected with KEY_DISABLED.
func TestTC03_KeyDisabled(t *testing.T) {
	disabled := apiToken(apiKey, apiKeyId, planToken)
	disabled.enabled = false

	e := newTestEnv(t, testOptions{
		plans:  []common.QuotaPlan{tokenPlan(planToken, redisKeyToken, 100000)},
		tokens: []tokenSpec{disabled},
	})

	resp, body, err := e.sendAuthenticated()
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		e.logBFEException()
		t.Fatalf("expected 403, got %d, body: %s", resp.StatusCode, body)
	}
	eb := parseAIError(t, body)
	if eb.Error.Code != "KEY_DISABLED" {
		t.Fatalf("expected code KEY_DISABLED, got %q, body: %s", eb.Error.Code, body)
	}
	if eb.Error.Details.KeyId != apiKeyId {
		t.Fatalf("expected details.key_id=%s, got %q", apiKeyId, eb.Error.Details.KeyId)
	}

	logs := e.stopAndParseLogs()
	if got := lastRequestLog(t, logs).GetAiAuthRejectReason(); got != "KEY_DISABLED" {
		t.Fatalf("expected ai_auth_reject_reason KEY_DISABLED, got %q", got)
	}
}

// TestTC04_KeyExpired verifies an expired key is rejected with KEY_EXPIRED.
func TestTC04_KeyExpired(t *testing.T) {
	expired := apiToken(apiKey, apiKeyId, planToken)
	expired.expiredTime = time.Now().Add(-time.Hour).Unix()

	e := newTestEnv(t, testOptions{
		plans:  []common.QuotaPlan{tokenPlan(planToken, redisKeyToken, 100000)},
		tokens: []tokenSpec{expired},
	})
	defer e.Close()

	resp, body, err := e.sendAuthenticated()
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		e.logBFEException()
		t.Fatalf("expected 403, got %d, body: %s", resp.StatusCode, body)
	}
	if eb := parseAIError(t, body); eb.Error.Code != "KEY_EXPIRED" {
		t.Fatalf("expected code KEY_EXPIRED, got %q, body: %s", eb.Error.Code, body)
	}
}

// TestTC05_SubnetNotAllowed verifies the source IP subnet check.
func TestTC05_SubnetNotAllowed(t *testing.T) {
	restricted := apiToken(apiKey, apiKeyId, planToken)
	restricted.subnet = "10.0.0.0/8"

	e := newTestEnv(t, testOptions{
		plans:        []common.QuotaPlan{tokenPlan(planToken, redisKeyToken, 100000)},
		tokens:       []tokenSpec{restricted},
		initialQuota: map[string]int64{redisKeyToken: 100000},
	})
	defer e.Close()

	resp, body, err := e.sendAuthenticated()
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		e.logBFEException()
		t.Fatalf("expected 403, got %d, body: %s", resp.StatusCode, body)
	}
	if eb := parseAIError(t, body); eb.Error.Code != "SUBNET_NOT_ALLOWED" {
		t.Fatalf("expected code SUBNET_NOT_ALLOWED, got %q, body: %s", eb.Error.Code, body)
	}
}

// TestTC06_TokenQuotaDeduction verifies a total_token plan is deducted by the
// observed usage and the quota plan is recorded as hit.
func TestTC06_TokenQuotaDeduction(t *testing.T) {
	e := newTestEnv(t, testOptions{
		plans:        []common.QuotaPlan{tokenPlan(planToken, redisKeyToken, 100000)},
		tokens:       []tokenSpec{apiToken(apiKey, apiKeyId, planToken)},
		initialQuota: map[string]int64{redisKeyToken: 1000},
	})

	resp, body, err := e.sendAuthenticated()
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
	}
	if got := e.waitForQuota(redisKeyToken, 850); got != 850 {
		t.Fatalf("expected token quota 850 after usage 150, got %d", got)
	}

	logs := e.stopAndParseLogs()
	reqLog := lastRequestLog(t, logs)
	found := false
	for _, plan := range reqLog.AiAuthHitQuotaPlans {
		if plan == planToken {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ai_auth_hit_quota_plans to contain %s, got %v", planToken, reqLog.AiAuthHitQuotaPlans)
	}
}

// TestTC07_TokenQuotaExhausted verifies a zero balance rejects the next request
// with QUOTA_EXHAUSTED and details.limit_type=api_key_quota.
func TestTC07_TokenQuotaExhausted(t *testing.T) {
	e := newTestEnv(t, testOptions{
		plans:        []common.QuotaPlan{tokenPlan(planToken, redisKeyToken, 50)},
		tokens:       []tokenSpec{apiToken(apiKey, apiKeyId, planToken)},
		initialQuota: map[string]int64{redisKeyToken: 50},
	})

	resp, body, err := e.sendAuthenticated()
	if err != nil {
		t.Fatalf("send first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected first request 200, got %d, body: %s", resp.StatusCode, body)
	}
	if got := e.waitForQuota(redisKeyToken, 0); got != 0 {
		t.Fatalf("expected token quota to be drained to 0, got %d", got)
	}

	resp, body, err = e.sendAuthenticated()
	if err != nil {
		t.Fatalf("send second request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected second request 429, got %d, body: %s", resp.StatusCode, body)
	}
	eb := parseAIError(t, body)
	if eb.Error.Code != "QUOTA_EXHAUSTED" {
		t.Fatalf("expected code QUOTA_EXHAUSTED, got %q, body: %s", eb.Error.Code, body)
	}
	if eb.Error.Details.LimitType != "api_key_quota" {
		t.Fatalf("expected limit_type api_key_quota, got %q", eb.Error.Details.LimitType)
	}
	if eb.Error.Details.QuotaPlanId != planToken {
		t.Fatalf("expected quota_plan_id %s, got %q", planToken, eb.Error.Details.QuotaPlanId)
	}

	logs := e.stopAndParseLogs()
	reqLog := lastRequestLog(t, logs)
	if got := reqLog.GetAiAuthRejectReason(); got != "QUOTA_EXHAUSTED" {
		t.Fatalf("expected ai_auth_reject_reason QUOTA_EXHAUSTED, got %q", got)
	}
	if len(reqLog.AiAuthRejectQuotaPlans) != 1 || reqLog.AiAuthRejectQuotaPlans[0] != planToken {
		t.Fatalf("expected ai_auth_reject_quota_plans=[%s], got %v", planToken, reqLog.AiAuthRejectQuotaPlans)
	}
}

// TestTC08_QuotaPlanExpired verifies an expired quota plan rejects the request
// with QUOTA_EXPIRED (distinct from an exhausted balance).
func TestTC08_QuotaPlanExpired(t *testing.T) {
	plan := tokenPlan(planToken, redisKeyToken, 100000)
	plan.ExpiredTime = time.Now().Add(-time.Hour).Unix()

	e := newTestEnv(t, testOptions{
		plans:        []common.QuotaPlan{plan},
		tokens:       []tokenSpec{apiToken(apiKey, apiKeyId, planToken)},
		initialQuota: map[string]int64{redisKeyToken: 100000},
	})

	resp, body, err := e.sendAuthenticated()
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected 429, got %d, body: %s", resp.StatusCode, body)
	}
	if eb := parseAIError(t, body); eb.Error.Code != "QUOTA_EXPIRED" {
		t.Fatalf("expected code QUOTA_EXPIRED, got %q, body: %s", eb.Error.Code, body)
	}

	logs := e.stopAndParseLogs()
	reqLog := lastRequestLog(t, logs)
	if got := reqLog.GetAiAuthRejectReason(); got != "QUOTA_EXPIRED" {
		t.Fatalf("expected ai_auth_reject_reason QUOTA_EXPIRED, got %q", got)
	}
}

// TestTC09_QuotaRedisError verifies a redis outage during the quota balance
// check surfaces as INTERNAL_QUOTA_ERROR (not as an exhausted balance).
func TestTC09_QuotaRedisError(t *testing.T) {
	e := newTestEnv(t, testOptions{
		plans:        []common.QuotaPlan{tokenPlan(planToken, redisKeyToken, 100000)},
		tokens:       []tokenSpec{apiToken(apiKey, apiKeyId, planToken)},
		initialQuota: map[string]int64{redisKeyToken: 100000},
	})
	defer e.Close()

	e.redis.Fault()

	resp, body, err := e.sendAuthenticated()
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		e.logBFEException()
		t.Fatalf("expected 500, got %d, body: %s", resp.StatusCode, body)
	}
	if eb := parseAIError(t, body); eb.Error.Code != "INTERNAL_QUOTA_ERROR" {
		t.Fatalf("expected code INTERNAL_QUOTA_ERROR, got %q, body: %s", eb.Error.Code, body)
	}
}

// TestTC10_UnlimitedAndPassNoQuota verifies both bypass shapes: an unlimited
// token and a PassNoQuota plan do not check or touch the quota balance.
func TestTC10_UnlimitedAndPassNoQuota(t *testing.T) {
	t.Run("unlimited token", func(t *testing.T) {
		e := newTestEnv(t, testOptions{
			tokens: []tokenSpec{unlimitedToken(apiKey, apiKeyId)},
		})
		defer e.Close()

		resp, body, err := e.sendAuthenticated()
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
		}
		if keys := e.redis.Keys(); len(keys) != 0 {
			t.Fatalf("unlimited token must not touch redis, keys=%v", keys)
		}
	})

	t.Run("pass no quota plan", func(t *testing.T) {
		plan := tokenPlan(planPass, redisKeyPass, 0)
		plan.PassNoQuota = true
		e := newTestEnv(t, testOptions{
			plans:        []common.QuotaPlan{plan},
			tokens:       []tokenSpec{apiToken(apiKey, apiKeyId, planPass)},
			initialQuota: map[string]int64{redisKeyPass: 0},
		})
		defer e.Close()

		resp, body, err := e.sendAuthenticated()
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
		}
	})
}

// TestTC11_MultipleQuotaPlans verifies every non-unlimited plan is checked and
// deducted, and any insufficient plan rejects the request.
func TestTC11_MultipleQuotaPlans(t *testing.T) {
	t.Run("both plans deducted", func(t *testing.T) {
		e := newTestEnv(t, testOptions{
			plans: []common.QuotaPlan{
				tokenPlan(planToken, redisKeyToken, 1000),
				tokenPlan(planPass, redisKeyPass, 1000),
			},
			tokens: []tokenSpec{apiToken(apiKey, apiKeyId, planToken, planPass)},
			initialQuota: map[string]int64{
				redisKeyToken: 1000,
				redisKeyPass:  1000,
			},
		})
		defer e.Close()

		resp, body, err := e.sendAuthenticated()
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
		}
		if got := e.waitForQuota(redisKeyToken, 850); got != 850 {
			t.Fatalf("expected first plan quota 850, got %d", got)
		}
		if got := e.waitForQuota(redisKeyPass, 850); got != 850 {
			t.Fatalf("expected second plan quota 850, got %d", got)
		}
	})

	t.Run("one plan insufficient", func(t *testing.T) {
		e := newTestEnv(t, testOptions{
			plans: []common.QuotaPlan{
				tokenPlan(planToken, redisKeyToken, 1000),
				tokenPlan(planPass, redisKeyPass, 0),
			},
			tokens: []tokenSpec{apiToken(apiKeyTwo, apiKeyIdTwo, planToken, planPass)},
			initialQuota: map[string]int64{
				redisKeyToken: 1000,
				redisKeyPass:  0,
			},
		})
		defer e.Close()

		resp, body, err := e.sendRequest(apiKeyTwo, defaultBody)
		if err != nil {
			t.Fatalf("send multi-plan request failed: %v", err)
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			e.logBFEException()
			t.Fatalf("expected 429, got %d, body: %s", resp.StatusCode, body)
		}
		eb := parseAIError(t, body)
		if eb.Error.Code != "QUOTA_EXHAUSTED" {
			t.Fatalf("expected code QUOTA_EXHAUSTED, got %q, body: %s", eb.Error.Code, body)
		}
		if eb.Error.Details.QuotaPlanId != planPass {
			t.Fatalf("expected quota_plan_id %s, got %q", planPass, eb.Error.Details.QuotaPlanId)
		}
	})
}

// TestTC12_CondMismatchSkipsAuth verifies the auth rule is only applied when
// its Cond matches. The arm uses a valid, route-bound key whose only quota
// plan has zero balance: a Cond match would reject with QUOTA_EXHAUSTED, so a
// 200 proves the auth rule was skipped when the path does not match.
func TestTC12_CondMismatchSkipsAuth(t *testing.T) {
	e := newTestEnv(t, testOptions{
		plans:        []common.QuotaPlan{tokenPlan(planToken, redisKeyToken, 0)},
		tokens:       []tokenSpec{apiToken(apiKey, apiKeyId, planToken)},
		cond:         `req_path_prefix_in("/admin", true)`,
		initialQuota: map[string]int64{redisKeyToken: 0},
	})
	defer e.Close()

	resp, body, err := e.sendAuthenticated()
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected 200 (auth rule skipped by Cond), got %d, body: %s", resp.StatusCode, body)
	}
	if hits := e.backend.Hits(); hits != 1 {
		t.Fatalf("expected the request to reach the backend, hits=%d", hits)
	}
	if got := e.redis.GetQuota(redisKeyToken); got != 0 {
		t.Fatalf("auth rule skipped must not deduct quota, got %d", got)
	}
}

// TestTC13_DefaultRulePath verifies mod_ai_token_auth falls back to its default
// ProductRulePath when the conf omits it.
func TestTC13_DefaultRulePath(t *testing.T) {
	e := newTestEnv(t, testOptions{
		plans:            []common.QuotaPlan{tokenPlan(planToken, redisKeyToken, 100000)},
		tokens:           []tokenSpec{apiToken(apiKey, apiKeyId, planToken)},
		initialQuota:     map[string]int64{redisKeyToken: 100000},
		clearDefaultPath: true,
	})
	defer e.Close()

	resp, body, err := e.sendAuthenticated()
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
	}

	resp, body, err = e.sendRequest("", defaultBody)
	if err != nil {
		t.Fatalf("send unauthenticated request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		e.logBFEException()
		t.Fatalf("expected 401 without api key, got %d, body: %s", resp.StatusCode, body)
	}
}
