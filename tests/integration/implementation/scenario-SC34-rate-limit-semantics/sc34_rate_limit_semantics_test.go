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

package sc34

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
	apiHost = "rl.example.org"
	apiPath = "/v1/chat/completions"

	apiKey          = "ak_sc34"
	apiKeyId        = "sc34_key_id"
	apiKeySecond    = "ak_sc34_second"
	apiKeyIdSec     = "sc34_second_key_id"
	apiKeyUnbound   = "ak_sc34_unbound"
	apiKeyIdUnbound = "sc34_unbound_key_id"

	clusterMain     = "cluster_rl_main"
	clusterFallback = "cluster_rl_fallback"

	policyID = "rlp-sc34"

	rpmRedisKey = "default_bfe_sc34_rpm"
	tpmRedisKey = "default_bfe_sc34_tpm"
)

var defaultBody = []byte(`{"model":"gpt-4"}`)
var usageResponse = `{"id":"chatcmpl-sc34","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`

// aiErrorBody mirrors the gateway's unified AiError response body.
type aiErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Type    string `json:"type"`
		Message string `json:"message"`
		Details struct {
			ApiKey      string `json:"api_key"`
			LimitType   string `json:"limit_type"`
			QuotaPlanId string `json:"quota_plan_id"`
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

// testEnv holds the resources of one SC34 integration test.
type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backends   map[string]*common.MockBackend
	redis      *common.RedisServer

	confDir     string
	logDir      string
	bfePort     int
	monitorPort int
	stopBFE     func()
}

// testOptions controls the rate limit picture written for one test.
type testOptions struct {
	// rateLimitData is written to mod_ai_rate_limit/ai_rate_limit.data.
	rateLimitData *common.RateLimitPolicyData
	// rejectOnRedisError sets IsRejectOnRedisError in mod_ai_rate_limit.conf.
	// Defaults to true (template value) when nil.
	rejectOnRedisError *bool
	// aiConf optionally overrides the AIConf for both clusters.
	aiConf *cluster_conf.AIConf
	// responseFunc optionally overrides the main backend behavior.
	responseFunc func(r *http.Request, count int) (int, string)
}

func defaultAIConf() *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type: 0,
		Keys: []cluster_conf.AIKey{
			{Name: "key-1", Key: "sk-sc34-1", Weight: 100},
		},
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:            "weighted_random",
			MaxRetries:          0,
			RetryBackoffInitial: 20,
			RetryBackoffMax:     50,
		},
	}
}

func multiKeyAIConf() *cluster_conf.AIConf {
	conf := defaultAIConf()
	conf.Keys = []cluster_conf.AIKey{
		{Name: "key-1", Key: "sk-sc34-1", Weight: 100},
		{Name: "key-2", Key: "sk-sc34-2", Weight: 100},
	}
	conf.KeyPolicy.MaxRetries = 2
	return conf
}

func newTestEnv(t *testing.T, opts testOptions) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	e.backends[clusterMain] = common.NewMockBackend(clusterMain, http.StatusOK, usageResponse)
	e.backends[clusterFallback] = common.NewMockBackend(clusterFallback, http.StatusOK, usageResponse)
	if opts.responseFunc != nil {
		e.backends[clusterMain].ResponseFunc = opts.responseFunc
	}

	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	aiConf := opts.aiConf
	if aiConf == nil {
		aiConf = defaultAIConf()
	}

	tokenRule := &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": {
				{
					Id:          "unlimited",
					Unlimited:   true,
					PassNoQuota: false,
					RedisKey:    "quota:unlimited",
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
					QuotaPlans:     []string{"unlimited"},
				},
				apiKeySecond: {
					Key:            apiKeySecond,
					KeyId:          apiKeyIdSec,
					Enabled:        true,
					ExpiredTime:    -1,
					UnlimitedQuota: false,
					QuotaPlans:     []string{"unlimited"},
				},
				apiKeyUnbound: {
					Key:            apiKeyUnbound,
					KeyId:          apiKeyIdUnbound,
					Enabled:        true,
					ExpiredTime:    -1,
					UnlimitedQuota: false,
					QuotaPlans:     []string{"unlimited"},
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
		TargetConfDir: e.confDir,
		Backends:      e.backends,
		AIConfs: map[string]*cluster_conf.AIConf{
			clusterMain:     aiConf,
			clusterFallback: aiConf,
		},
		RedisAddr:           e.redis.Addr(),
		TokenRuleData:       tokenRule,
		RateLimitPolicyData: opts.rateLimitData,
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	if opts.rejectOnRedisError != nil && !*opts.rejectOnRedisError {
		confPath := filepath.Join(e.confDir, "mod_ai_rate_limit", "mod_ai_rate_limit.conf")
		if err := common.RewriteConfLine(confPath, "IsRejectOnRedisError", "false"); err != nil {
			t.Fatalf("disable IsRejectOnRedisError failed: %v", err)
		}
	}

	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
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

func (e *testEnv) sendRequest() (*http.Response, string, error) {
	return e.sendRequestWithKey(apiKey, defaultBody)
}

func (e *testEnv) sendRequestWithKey(key string, body []byte) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second,
		// Local rejections (429/401/...) may close the client connection; a
		// pooled connection from http.DefaultTransport could then leak into
		// the next request as EOF since POST is not retried automatically.
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

type requestResult struct {
	resp *http.Response
	body string
	err  error
}

func (e *testEnv) sendRequestAsync(key string, body []byte) <-chan requestResult {
	ch := make(chan requestResult, 1)
	go func() {
		resp, respBody, err := e.sendRequestWithKey(key, body)
		ch <- requestResult{resp: resp, body: respBody, err: err}
	}()
	return ch
}

func (e *testEnv) reloadRateLimit() {
	e.t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/reload/mod_ai_rate_limit", e.monitorPort)
	resp, err := http.Get(url)
	if err != nil {
		e.t.Fatalf("reload mod_ai_rate_limit failed: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("reload mod_ai_rate_limit status = %d, body: %s", resp.StatusCode, data)
	}
}

func (e *testEnv) writeRateLimitData(data *common.RateLimitPolicyData) {
	e.t.Helper()
	path := filepath.Join(e.confDir, "mod_ai_rate_limit", "ai_rate_limit.data")
	if err := common.WriteJSONFile(path, data); err != nil {
		e.t.Fatalf("write ai_rate_limit.data failed: %v", err)
	}
}

// waitForTPMConsumed polls the TPM redis hash until the accumulated token
// count reaches want, returning the observed sum.
func (e *testEnv) waitForTPMConsumed(want int64, timeout time.Duration) int64 {
	e.t.Helper()
	deadline := time.Now().Add(timeout)
	var got int64
	for time.Now().Before(deadline) {
		got = 0
		for _, v := range e.redis.HGetAll(tpmRedisKey) {
			var n int64
			fmt.Sscanf(v, "%d", &n)
			got += n
		}
		if got >= want {
			return got
		}
		time.Sleep(50 * time.Millisecond)
	}
	return got
}

func rpmPolicy(maxRequests int64) *common.RateLimitPolicyData {
	return policyWithRules(common.RateLimitPolicy{
		Name:    "sc34-rpm",
		Enabled: true,
		Rules:   rateLimitRules(nil, rpmRule(maxRequests), nil),
	})
}

func tpmPolicy(maxTokens int64) *common.RateLimitPolicyData {
	return policyWithRules(common.RateLimitPolicy{
		Name:    "sc34-tpm",
		Enabled: true,
		Rules:   rateLimitRules(tpmRule(maxTokens), nil, nil),
	})
}

func concurrencyPolicy(maxConcurrency int64) *common.RateLimitPolicyData {
	return policyWithRules(common.RateLimitPolicy{
		Name:    "sc34-concurrency",
		Enabled: true,
		Rules:   rateLimitRules(nil, nil, &maxConcurrency),
	})
}

func rateLimitRules(tpm, rpm []common.RateLimitRule, maxCon *int64) struct {
	TPM            []common.RateLimitRule       `json:"tpm,omitempty"`
	RPM            []common.RateLimitRule       `json:"rpm,omitempty"`
	MaxConcurrency *int64                       `json:"max_concurrency,omitempty"`
	Batch          *common.RateLimitBatchLimits `json:"batch,omitempty"`
} {
	return struct {
		TPM            []common.RateLimitRule       `json:"tpm,omitempty"`
		RPM            []common.RateLimitRule       `json:"rpm,omitempty"`
		MaxConcurrency *int64                       `json:"max_concurrency,omitempty"`
		Batch          *common.RateLimitBatchLimits `json:"batch,omitempty"`
	}{TPM: tpm, RPM: rpm, MaxConcurrency: maxCon}
}

func tpmRule(maxTokens int64) []common.RateLimitRule {
	return []common.RateLimitRule{
		{
			Name:          "sc34-tpm-rule",
			WindowMinutes: 1,
			MaxTokens:     maxTokens,
			StepMinutes:   1,
			RedisKey:      tpmRedisKey,
		},
	}
}

func rpmRule(maxRequests int64) []common.RateLimitRule {
	return []common.RateLimitRule{
		{
			Name:          "sc34-rpm-rule",
			WindowMinutes: 1,
			MaxRequests:   maxRequests,
			Burst:         1,
			RedisKey:      rpmRedisKey,
		},
	}
}

func policyWithRules(policies ...common.RateLimitPolicy) *common.RateLimitPolicyData {
	policyMap := make(map[string]common.RateLimitPolicy, len(policies))
	for i, p := range policies {
		id := policyID
		if i > 0 {
			id = fmt.Sprintf("%s-%d", policyID, i)
		}
		policyMap[id] = p
	}
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
		RateLimitPolicies: policyMap,
		ApikeyRateLimitPolicyBindings: map[string][]string{
			apiKey: {policyID},
		},
	}
}

// TestTC01 verifies the RPM sliding window boundary and recovery: the third
// request in a one-minute window is rejected, and after the window slides the
// request is allowed again.
func TestTC01_RPMSlidingWindowBoundaryAndRecovery(t *testing.T) {
	e := newTestEnv(t, testOptions{rateLimitData: rpmPolicy(2)})
	defer e.Close()

	for i := 0; i < 2; i++ {
		resp, body, err := e.sendRequest()
		if err != nil {
			t.Fatalf("send request %d failed: %v", i+1, err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("request %d: expected 200, got %d, body: %s", i+1, resp.StatusCode, body)
		}
	}

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send third request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("third request: expected 429, got %d, body: %s", resp.StatusCode, body)
	}
	if eb := parseAIError(t, body); eb.Error.Code != "RPM_LIMIT_EXCEEDED" {
		t.Fatalf("third request: expected code RPM_LIMIT_EXCEEDED, got %q, body: %s", eb.Error.Code, body)
	}
	if hits := e.backends[clusterMain].Hits(); hits != 2 {
		t.Fatalf("expected 2 backend hits before rejection, got %d", hits)
	}

	// Slide the window past the one-minute period: the limiter builds its
	// window from the Redis TIME command, so the virtual clock must advance
	// (AdvanceTime) rather than only key TTLs (FastForward). 90s leaves a
	// margin over the 60s window so the assertion is not sensitive to the
	// time elapsed between the two original requests and the jump.
	if members := e.redis.ZMembers(rpmRedisKey); len(members) != 2 {
		t.Fatalf("expected 2 window members before slide, got %v", members)
	}
	e.redis.AdvanceTime(90 * time.Second)

	resp, body, err = e.sendRequest()
	if err != nil {
		e.logBFEException()
		t.Fatalf("send request after window slide failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("after window slide: expected 200, got %d, body: %s", resp.StatusCode, body)
	}
	// The old window members are pruned and the admitted request appends a
	// single new member.
	if members := e.redis.ZMembers(rpmRedisKey); len(members) != 1 {
		t.Fatalf("expected the old window to be pruned, got members=%v", members)
	}
}

// TestTC02 verifies that exceeding the TPM threshold rejects the next request.
// The first request consumes real usage (the TPM pre-consume is zero), so the
// denial happens on the following request.
func TestTC02_TPMExceededRejectsNextRequest(t *testing.T) {
	e := newTestEnv(t, testOptions{rateLimitData: tpmPolicy(100)})
	defer e.Close()

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected first request 200, got %d, body: %s", resp.StatusCode, body)
	}

	if got := e.waitForTPMConsumed(150, 5*time.Second); got != 150 {
		t.Fatalf("expected TPM counter 150 after first request, got %d", got)
	}

	resp, body, err = e.sendRequest()
	if err != nil {
		t.Fatalf("send second request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected second request 429, got %d, body: %s", resp.StatusCode, body)
	}
	eb := parseAIError(t, body)
	if eb.Error.Code != "TPM_LIMIT_EXCEEDED" {
		t.Fatalf("expected code TPM_LIMIT_EXCEEDED, got %q, body: %s", eb.Error.Code, body)
	}
	if eb.Error.Details.LimitType != "tpm" {
		t.Fatalf("expected limit_type tpm, got %q, body: %s", eb.Error.Details.LimitType, body)
	}
}

// TestTC03 verifies TPM accounting under a generous threshold: both requests
// pass and redis accumulates the real usage of each response.
func TestTC03_TPMAccountedAndAllowed(t *testing.T) {
	e := newTestEnv(t, testOptions{rateLimitData: tpmPolicy(100000)})
	defer e.Close()

	for i := 0; i < 2; i++ {
		resp, body, err := e.sendRequest()
		if err != nil {
			t.Fatalf("send request %d failed: %v", i+1, err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("request %d: expected 200, got %d, body: %s", i+1, resp.StatusCode, body)
		}
	}

	if got := e.waitForTPMConsumed(300, 5*time.Second); got != 300 {
		t.Fatalf("expected TPM counter 300 after two requests, got %d", got)
	}
	if hits := e.backends[clusterMain].Hits(); hits != 2 {
		t.Fatalf("expected 2 backend hits, got %d", hits)
	}
}

// TestTC04 verifies the concurrency limiter: a second request is rejected
// while the first holds the only slot, and the slot is released afterwards.
func TestTC04_ConcurrencySlotRejectedAndReleased(t *testing.T) {
	release := make(chan struct{})
	e := newTestEnv(t, testOptions{
		rateLimitData: concurrencyPolicy(1),
		responseFunc: func(r *http.Request, count int) (int, string) {
			<-release
			return http.StatusOK, usageResponse
		},
	})
	defer e.Close()

	first := e.sendRequestAsync(apiKey, defaultBody)

	// Wait until the first request occupies the slot (the limiter acquires it
	// before the backend call) and the backend is actually invoked.
	deadline := time.Now().Add(5 * time.Second)
	for e.backends[clusterMain].Hits() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if hits := e.backends[clusterMain].Hits(); hits != 1 {
		t.Fatalf("expected the first request to reach the backend, hits=%d", hits)
	}

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send concurrent request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("concurrent request: expected 429, got %d, body: %s", resp.StatusCode, body)
	}
	eb := parseAIError(t, body)
	if eb.Error.Code != "CONCURRENCY_LIMIT_EXCEEDED" {
		t.Fatalf("expected code CONCURRENCY_LIMIT_EXCEEDED, got %q, body: %s", eb.Error.Code, body)
	}

	close(release)
	select {
	case res := <-first:
		if res.err != nil {
			t.Fatalf("first request failed: %v", res.err)
		}
		if res.resp.StatusCode != http.StatusOK {
			t.Fatalf("first request: expected 200, got %d, body: %s", res.resp.StatusCode, res.body)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("first request did not finish after release")
	}

	// The slot is released in the request-finish handler; poll until the next
	// request is admitted.
	deadline = time.Now().Add(5 * time.Second)
	for {
		resp, body, err = e.sendRequest()
		if err != nil {
			t.Fatalf("send request after release failed: %v", err)
		}
		if resp.StatusCode == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			e.logBFEException()
			t.Fatalf("request after release: expected 200, got %d, body: %s", resp.StatusCode, body)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestTC05 verifies fail-closed behavior: with IsRejectOnRedisError=true a
// redis outage rejects the request and the access log records the redis error.
func TestTC05_RedisErrorFailClosed(t *testing.T) {
	e := newTestEnv(t, testOptions{rateLimitData: rpmPolicy(100)})

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected first request 200, got %d, body: %s", resp.StatusCode, body)
	}

	// Cut redis: the RPM check cannot read its counter.
	e.redis.Fault()

	resp, body, err = e.sendRequest()
	if err != nil {
		t.Fatalf("send request during redis outage failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		e.logBFEException()
		t.Fatalf("expected redis outage to reject with 500, got %d, body: %s", resp.StatusCode, body)
	}
	if eb := parseAIError(t, body); eb.Error.Code != "RATE_LIMIT_REDIS_ERROR" {
		t.Fatalf("expected code RATE_LIMIT_REDIS_ERROR, got %q, body: %s", eb.Error.Code, body)
	}
	if hits := e.backends[clusterMain].Hits(); hits != 1 {
		t.Fatalf("rejected request must not reach the backend, hits=%d", hits)
	}

	// Flush the access log before parsing: log4go flushes on a short interval,
	// so give it a beat before killing the process.
	time.Sleep(500 * time.Millisecond)
	e.stopBFE()
	e.stopBFE = nil

	reqLogs, err := common.ParseAccessLogAfterStop(e.logDir)
	if err != nil {
		t.Fatalf("parse access log failed: %v", err)
	}
	if len(reqLogs) < 2 {
		t.Fatalf("expected at least 2 request logs, got %d", len(reqLogs))
	}
	rejected := reqLogs[len(reqLogs)-1]
	var found bool
	for _, hit := range rejected.AiRateLimitHits {
		if hit.GetRateLimitPolicyId() == policyID && hit.GetRateLimitType() == "redis_error" {
			found = true
			if len(hit.GetRuleNames()) != 1 || hit.GetRuleNames()[0] != "acc_redis_error" {
				t.Fatalf("expected rule_names=[acc_redis_error], got %v", hit.GetRuleNames())
			}
		}
	}
	if !found {
		t.Fatalf("expected a redis_error rate limit hit in the access log, got %v", rejected.AiRateLimitHits)
	}
}

// TestTC06 verifies fail-open behavior: with IsRejectOnRedisError=false a redis
// outage does not block traffic.
func TestTC06_RedisErrorFailOpen(t *testing.T) {
	reject := false
	e := newTestEnv(t, testOptions{
		rateLimitData:      rpmPolicy(100),
		rejectOnRedisError: &reject,
	})
	defer e.Close()

	e.redis.Fault()

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send request during redis outage failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected fail-open 200, got %d, body: %s", resp.StatusCode, body)
	}
	if hits := e.backends[clusterMain].Hits(); hits != 1 {
		t.Fatalf("expected fail-open request to reach the backend, hits=%d", hits)
	}
}

// TestTC07 verifies multi-policy binding: any bound enabled policy can reject,
// a disabled policy is skipped, and an api key without any policy binding is
// not limited.
func TestTC07_MultiPolicyAndUnboundKey(t *testing.T) {
	data := policyWithRules(
		common.RateLimitPolicy{
			Name:    "sc34-rpm-tight",
			Enabled: true,
			Rules:   rateLimitRules(nil, rpmRule(1), nil),
		},
		common.RateLimitPolicy{
			Name:    "sc34-rpm-disabled",
			Enabled: false,
			Rules:   rateLimitRules(nil, rpmRule(1), nil),
		},
	)
	// Bind the disabled policy only to the second key; the third key has no
	// binding at all.
	data.ApikeyRateLimitPolicyBindings = map[string][]string{
		apiKey:       {policyID, policyID + "-1"},
		apiKeySecond: {policyID + "-1"},
	}

	e := newTestEnv(t, testOptions{rateLimitData: data})
	defer e.Close()

	// First bound key: policy 0 (RPM=1) rejects the second request.
	if resp, body, err := e.sendRequest(); err != nil || resp.StatusCode != http.StatusOK {
		if err != nil {
			t.Fatalf("send first request failed: %v", err)
		}
		e.logBFEException()
		t.Fatalf("first request: expected 200, got %d, body: %s", resp.StatusCode, body)
	}
	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send second request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("second request: expected 429, got %d, body: %s", resp.StatusCode, body)
	}

	// Second bound key only has the disabled policy: both requests pass.
	for i := 0; i < 2; i++ {
		resp, body, err = e.sendRequestWithKey(apiKeySecond, defaultBody)
		if err != nil {
			t.Fatalf("send disabled-policy request %d failed: %v", i+1, err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("disabled-policy request %d: expected 200, got %d, body: %s", i+1, resp.StatusCode, body)
		}
	}

	// Third key has no rate limit binding: both requests pass.
	for i := 0; i < 2; i++ {
		resp, body, err = e.sendRequestWithKey(apiKeyUnbound, defaultBody)
		if err != nil {
			t.Fatalf("send unbound-key request %d failed: %v", i+1, err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("unbound-key request %d: expected 200, got %d, body: %s", i+1, resp.StatusCode, body)
		}
	}
}

// TestTC08 verifies rule hot reload: raising max_requests admits requests that
// were rejected before the reload.
func TestTC08_RateLimitRuleHotReload(t *testing.T) {
	data := rpmPolicy(1)
	e := newTestEnv(t, testOptions{rateLimitData: data})
	defer e.Close()

	if resp, body, err := e.sendRequest(); err != nil || resp.StatusCode != http.StatusOK {
		if err != nil {
			t.Fatalf("send first request failed: %v", err)
		}
		e.logBFEException()
		t.Fatalf("first request: expected 200, got %d, body: %s", resp.StatusCode, body)
	}

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send second request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("second request: expected 429, got %d, body: %s", resp.StatusCode, body)
	}

	// Raise the limit and reload the rule table.
	e.writeRateLimitData(rpmPolicy(100))
	e.reloadRateLimit()

	resp, body, err = e.sendRequest()
	if err != nil {
		t.Fatalf("send request after reload failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("after reload: expected 200, got %d, body: %s", resp.StatusCode, body)
	}
}

// TestTC09 verifies a rate limit rejection does not rotate provider keys and
// does not fall back to the fallback cluster.
func TestTC09_RejectionDoesNotRotateKeyOrFallback(t *testing.T) {
	e := newTestEnv(t, testOptions{
		rateLimitData: rpmPolicy(1),
		aiConf:        multiKeyAIConf(),
	})
	defer e.Close()

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("first request: expected 200, got %d, body: %s", resp.StatusCode, body)
	}

	mainHits := e.backends[clusterMain].Hits()
	authHeaders := len(e.backends[clusterMain].AuthHeaders())

	resp, body, err = e.sendRequest()
	if err != nil {
		t.Fatalf("send second request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("second request: expected 429, got %d, body: %s", resp.StatusCode, body)
	}
	if eb := parseAIError(t, body); eb.Error.Code != "RPM_LIMIT_EXCEEDED" {
		t.Fatalf("expected code RPM_LIMIT_EXCEEDED, got %q, body: %s", eb.Error.Code, body)
	}

	if hits := e.backends[clusterMain].Hits(); hits != mainHits {
		t.Fatalf("rejected request must not reach the main backend, hits %d -> %d", mainHits, hits)
	}
	if hits := e.backends[clusterFallback].Hits(); hits != 0 {
		t.Fatalf("rejected request must not reach the fallback cluster, hits=%d", hits)
	}
	if got := len(e.backends[clusterMain].AuthHeaders()); got != authHeaders {
		t.Fatalf("rejected request must not rotate provider keys, auth headers %d -> %d", authHeaders, got)
	}
}
