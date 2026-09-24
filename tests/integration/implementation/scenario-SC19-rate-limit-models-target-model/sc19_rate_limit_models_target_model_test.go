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

package sc19

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
	apiKeyRL = "ak_rl"
	apiKeyUP = "ak_up"

	hostRedirect = "redirect.example.org"
	hostMulti    = "multi.example.org"
	hostFallback = "fallback.example.org"
	hostPlain    = "plain.example.org"
	hostUpstream = "upstream.example.org"
	hostNoRoute  = "noroute.example.org"
	pathChat     = "/v1/chat/completions"

	clusterRedirect = "cluster_redirect"
	clusterMulti    = "cluster_multi"
	clusterMain     = "cluster_main"
	clusterBackup   = "cluster_backup"
	clusterPlain    = "cluster_plain"
	clusterUpstream = "cluster_upstream"

	redirectKey  = "sk-redirect-key"
	multiKey1    = "sk-multi-1"
	multiKey2    = "sk-multi-2"
	mainKey      = "sk-main-key"
	backupKey    = "sk-backup-key"
	plainKey     = "sk-plain-key"
	upstreamKey1 = "sk-up-1"
	upstreamKey2 = "sk-up-2"

	// clientModel 是请求体原始模型；经 ModelMapping 重定向后得到 targetModel。
	clientModel = "glm-5.2-abc"
	targetModel = "glm-5.2"

	policyID    = "rlp-sc19"
	policyIDCon = "rlp-sc19-con"
	rpmRedisKey = "RL_RPM_rlp-sc19_0"
)

// openaiUsageBody 是完整的非流式 chat.completions 响应：usage.total_tokens=15。
var openaiUsageBody = `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`

// aiErrorBody 是 BFE AI 错误响应体（bfe_basic.AiErrorBody）的反序列化视图，
// 用于断言限流拒绝时的错误码与 details.limit_type。
type aiErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details struct {
			ApiKey    string `json:"api_key"`
			LimitType string `json:"limit_type"`
			Model     string `json:"model"`
		} `json:"details"`
	} `json:"error"`
}

type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backends   map[string]*common.MockBackend
	redis      *common.RedisServer
	bfePort    int
	stopBFE    func()
}

// newTestEnv 启动限流环境：mod_ai_route + mod_ai_token_auth（token 无
// allow/block 列表，unlimited 配额方案）+ mod_ai_rate_limit，与 SC07 的
// 接线方式一致。aiConfs 与 rateLimitData 由每个 TC 按需提供。
func newTestEnv(t *testing.T, aiConfs map[string]*cluster_conf.AIConf, rateLimitData *common.RateLimitPolicyData) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	for _, name := range []string{clusterRedirect, clusterMulti, clusterMain, clusterBackup, clusterPlain, clusterUpstream} {
		e.backends[name] = common.NewMockBackend(name, http.StatusOK, openaiUsageBody)
	}

	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	confDir := filepath.Join(e.processEnv.WorkDir(), "conf")
	logDir := filepath.Join(e.processEnv.WorkDir(), "log")

	unlimitedPlan := func(id string) common.QuotaPlan {
		return common.QuotaPlan{
			Id:          id,
			Unlimited:   true,
			PassNoQuota: false,
			RedisKey:    "quota:" + id,
			ExpiredTime: -1,
			Quota:       0,
			Unit:        "total_token",
		}
	}
	tokenRule := &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": {
				unlimitedPlan("unlimited_rl"),
				unlimitedPlan("unlimited_up"),
			},
		},
		Tokens: map[string]map[string]common.TokenFile{
			"ai_product": {
				apiKeyRL: {
					Key:            apiKeyRL,
					KeyId:          "rl_key_id",
					Enabled:        true,
					ExpiredTime:    -1,
					UnlimitedQuota: false,
					QuotaPlans:     []string{"unlimited_rl"},
				},
				apiKeyUP: {
					Key:            apiKeyUP,
					KeyId:          "up_key_id",
					Enabled:        true,
					ExpiredTime:    -1,
					UnlimitedQuota: false,
					QuotaPlans:     []string{"unlimited_up"},
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
		TemplateDir:         "testdata",
		TargetConfDir:       confDir,
		Backends:            e.backends,
		AIConfs:             aiConfs,
		RedisAddr:           e.redis.Addr(),
		TokenRuleData:       tokenRule,
		RateLimitPolicyData: rateLimitData,
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

func (e *testEnv) sendRequest(host, apiKey, model string) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, pathChat)
	body := []byte(`{"model":"` + model + `","messages":[{"role":"user","content":"hello"}]}`)
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

// parseAIError 解析 BFE AI 错误响应体，用于断言错误码与 details.limit_type。
func parseAIError(t *testing.T, body string) aiErrorBody {
	t.Helper()
	var eb aiErrorBody
	if err := json.Unmarshal([]byte(body), &eb); err != nil {
		t.Fatalf("parse ai error body failed: %v, body: %s", err, body)
	}
	return eb
}

func countDistinct(values []string) int {
	set := make(map[string]struct{})
	for _, v := range values {
		set[v] = struct{}{}
	}
	return len(set)
}

// redirectAIConf 单 provider key + ModelMapping（glm-5.2-abc → glm-5.2）。
func redirectAIConf() map[string]*cluster_conf.AIConf {
	mapping := map[string]string{clientModel: targetModel}
	return map[string]*cluster_conf.AIConf{
		clusterRedirect: {
			Type:           0,
			ModelProtocols: []string{"openai"},
			ModelMapping:   &mapping,
			Keys: []cluster_conf.AIKey{
				{Name: "redirect-key", Key: redirectKey, Weight: 100},
			},
		},
	}
}

// multiKeyAIConf 多 provider key + ModelMapping + 开启会话亲和与 429 惩罚，
// MaxRetries=1 使 key 轮换路径可用。
func multiKeyAIConf() map[string]*cluster_conf.AIConf {
	mapping := map[string]string{clientModel: targetModel}
	return map[string]*cluster_conf.AIConf{
		clusterMulti: {
			Type:           0,
			ModelProtocols: []string{"openai"},
			ModelMapping:   &mapping,
			Keys: []cluster_conf.AIKey{
				{Name: "multi-key-1", Key: multiKey1, Weight: 100},
				{Name: "multi-key-2", Key: multiKey2, Weight: 100},
			},
			KeyPolicy: &cluster_conf.AIKeyPolicy{
				Strategy:                     "weighted_random",
				MaxRetries:                   1,
				RetryBackoffInitial:          10,
				RetryBackoffMax:              50,
				SessionAffinity:              true,
				SessionAffinityPenaltyEnable: true,
			},
		},
	}
}

// fallbackAIConf 主目标 cluster_main + fallback cluster_backup，均无 ModelMapping。
func fallbackAIConf() map[string]*cluster_conf.AIConf {
	return map[string]*cluster_conf.AIConf{
		clusterMain: {
			Type:           0,
			ModelProtocols: []string{"openai"},
			Keys: []cluster_conf.AIKey{
				{Name: "main-key", Key: mainKey, Weight: 100},
			},
		},
		clusterBackup: {
			Type:           0,
			ModelProtocols: []string{"openai"},
			Keys: []cluster_conf.AIKey{
				{Name: "backup-key", Key: backupKey, Weight: 100},
			},
		},
	}
}

func plainAIConf() map[string]*cluster_conf.AIConf {
	return map[string]*cluster_conf.AIConf{
		clusterPlain: {
			Type:           0,
			ModelProtocols: []string{"openai"},
			Keys: []cluster_conf.AIKey{
				{Name: "plain-key", Key: plainKey, Weight: 100},
			},
		},
	}
}

// upstreamAIConf 多 provider key、MaxRetries=1（轮换可用），无会话亲和。
func upstreamAIConf() map[string]*cluster_conf.AIConf {
	return map[string]*cluster_conf.AIConf{
		clusterUpstream: {
			Type:           0,
			ModelProtocols: []string{"openai"},
			Keys: []cluster_conf.AIKey{
				{Name: "upstream-key-1", Key: upstreamKey1, Weight: 100},
				{Name: "upstream-key-2", Key: upstreamKey2, Weight: 100},
			},
			KeyPolicy: &cluster_conf.AIKeyPolicy{
				Strategy:            "weighted_random",
				MaxRetries:          1,
				RetryBackoffInitial: 10,
				RetryBackoffMax:     50,
			},
		},
	}
}

// rpmPolicy 返回「适用模型=models，RPM=1」策略并绑定到 apiKeyRL。
func rpmPolicy(models []string) *common.RateLimitPolicyData {
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
				Name:    "sc19-rpm",
				Enabled: true,
				Rules: struct {
					TPM            []common.RateLimitRule `json:"tpm,omitempty"`
					RPM            []common.RateLimitRule `json:"rpm,omitempty"`
					MaxConcurrency *int64                 `json:"max_concurrency,omitempty"`
				}{
					RPM: []common.RateLimitRule{
						{
							Name:          "sc19-rpm-rule",
							WindowMinutes: 1,
							MaxRequests:   1,
							Burst:         1,
							Models:        models,
							RedisKey:      rpmRedisKey,
						},
					},
				},
			},
		},
		ApikeyRateLimitPolicyBindings: map[string][]string{
			apiKeyRL: {policyID},
		},
	}
}

// concurrencyPolicy 返回「适用模型=models，最大并发=1」策略并绑定到 apiKeyRL。
func concurrencyPolicy(models []string) *common.RateLimitPolicyData {
	maxCon := int64(1)
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
			policyIDCon: {
				Name:    "sc19-concurrency",
				Enabled: true,
				Rules: struct {
					TPM            []common.RateLimitRule `json:"tpm,omitempty"`
					RPM            []common.RateLimitRule `json:"rpm,omitempty"`
					MaxConcurrency *int64                 `json:"max_concurrency,omitempty"`
				}{
					MaxConcurrency: &maxCon,
				},
			},
		},
		ApikeyRateLimitPolicyBindings: map[string][]string{
			apiKeyRL: {policyIDCon},
		},
	}
}

// emptyPolicy 返回空的限流策略数据（模块已加载但无策略/绑定），
// 用于 TC-06 等不需要本地限流的用例。
func emptyPolicy() *common.RateLimitPolicyData {
	return &common.RateLimitPolicyData{
		Version:                       "1.0",
		Config:                        map[string][]common.RateLimitProductRule{},
		RateLimitPolicies:             map[string]common.RateLimitPolicy{},
		ApikeyRateLimitPolicyBindings: map[string][]string{},
	}
}

// TestTC01 验证姊妹卡修复主链路：限流策略「适用模型」按重定向后的目标模型
// 圈定。provider 模型 glm-5.2 + 集群 ModelMapping（glm-5.2-abc→glm-5.2）+
// 策略「适用模型=[glm-5.2]，RPM=1」：请求 model=glm-5.2-abc 第 1 次 200（
// 后端收到 model=glm-5.2），第 2 次本地 429 且 limit_type=RPM。修复前按请求体
// 原始模型 glm-5.2-abc 匹配，策略不生效，两次均 200。
func TestTC01_RateLimitMatchedByTargetModel(t *testing.T) {
	e := newTestEnv(t, redirectAIConf(), rpmPolicy([]string{targetModel}))
	defer e.Close()

	resp, body, err := e.sendRequest(hostRedirect, apiKeyRL, clientModel)
	if err != nil {
		t.Fatalf("send first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected first request 200, got %d, body: %s", resp.StatusCode, body)
	}

	backend := e.backends[clusterRedirect]
	if backend.Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterRedirect, backend.Hits())
	}
	models := backend.Models()
	if len(models) != 1 || models[0] != targetModel {
		t.Fatalf("expected backend received model %q (rewritten after redirect), got %v", targetModel, models)
	}

	resp, body, err = e.sendRequest(hostRedirect, apiKeyRL, clientModel)
	if err != nil {
		t.Fatalf("send second request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected second request 429, got %d, body: %s", resp.StatusCode, body)
	}
	eb := parseAIError(t, body)
	if eb.Error.Code != "RPM_LIMIT_EXCEEDED" {
		t.Fatalf("expected code RPM_LIMIT_EXCEEDED, got %q, body: %s", eb.Error.Code, body)
	}
	if eb.Error.Details.LimitType != "rpm" {
		t.Fatalf("expected limit_type rpm, got %q, body: %s", eb.Error.Details.LimitType, body)
	}
	if eb.Error.Details.ApiKey != apiKeyRL {
		t.Fatalf("expected details.api_key %q, got %q, body: %s", apiKeyRL, eb.Error.Details.ApiKey, body)
	}

	// 本地 429 未到达后端。
	if backend.Hits() != 1 {
		t.Fatalf("expected backend hits still 1 after local 429, got %d", backend.Hits())
	}
}

// TestTC02 验证本地限流 429 不轮换 provider key：多 provider key（含会话亲和
// 与 429 惩罚）集群下，第 2 次请求被本地策略 429 后，不得把 429 当作 provider
// key 问题而轮换——后端 Hits 保持 1、只出现过 1 个 provider key、Redis 中无
// affinity penalty key。
func TestTC02_Local429DoesNotRotateProviderKey(t *testing.T) {
	e := newTestEnv(t, multiKeyAIConf(), rpmPolicy([]string{targetModel}))
	defer e.Close()

	resp, body, err := e.sendRequest(hostMulti, apiKeyRL, clientModel)
	if err != nil {
		t.Fatalf("send first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected first request 200, got %d, body: %s", resp.StatusCode, body)
	}

	resp, body, err = e.sendRequest(hostMulti, apiKeyRL, clientModel)
	if err != nil {
		t.Fatalf("send second request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected second request 429, got %d, body: %s", resp.StatusCode, body)
	}
	eb := parseAIError(t, body)
	if eb.Error.Code != "RPM_LIMIT_EXCEEDED" {
		t.Fatalf("expected code RPM_LIMIT_EXCEEDED, got %q, body: %s", eb.Error.Code, body)
	}

	backend := e.backends[clusterMulti]
	if backend.Hits() != 1 {
		t.Fatalf("local 429 must not rotate provider key, expected 1 backend hit, got %d", backend.Hits())
	}
	if n := countDistinct(backend.AuthHeaders()); n != 1 {
		t.Fatalf("local 429 must not rotate provider key, expected 1 distinct auth header, got %d: %v",
			n, backend.AuthHeaders())
	}

	// 无 affinity penalty key 生成（penalty key 形如
	// bfe:ai:key_affinity:penalty:<cluster>:<keyName>）。
	for _, k := range e.redis.Keys() {
		if strings.Contains(k, ":penalty:") {
			t.Fatalf("local 429 must not set affinity penalty key, found %q", k)
		}
	}
}

// TestTC03 验证本地限流 429 不触发集群 fallback：主目标集群策略 429 时，
// 429 在默认 aiFallbackStatusCodes 内，但不允许因此转发到 fallback 集群——
// 客户端仍收到 429，fallback 集群后端 Hits 为 0。
func TestTC03_Local429DoesNotTriggerFallback(t *testing.T) {
	e := newTestEnv(t, fallbackAIConf(), rpmPolicy([]string{targetModel}))
	defer e.Close()

	resp, body, err := e.sendRequest(hostFallback, apiKeyRL, targetModel)
	if err != nil {
		t.Fatalf("send first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected first request 200, got %d, body: %s", resp.StatusCode, body)
	}
	if e.backends[clusterMain].Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterMain, e.backends[clusterMain].Hits())
	}

	resp, body, err = e.sendRequest(hostFallback, apiKeyRL, targetModel)
	if err != nil {
		t.Fatalf("send second request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected second request 429, got %d, body: %s", resp.StatusCode, body)
	}
	eb := parseAIError(t, body)
	if eb.Error.Code != "RPM_LIMIT_EXCEEDED" {
		t.Fatalf("expected code RPM_LIMIT_EXCEEDED, got %q, body: %s", eb.Error.Code, body)
	}

	if hits := e.backends[clusterMain].Hits(); hits != 1 {
		t.Fatalf("expected %s hits still 1, got %d", clusterMain, hits)
	}
	if hits := e.backends[clusterBackup].Hits(); hits != 0 {
		t.Fatalf("local 429 must not trigger cluster fallback, expected 0 hit on %s, got %d", clusterBackup, hits)
	}
}

// TestTC04 验证无重定向场景行为不退化：无 ModelMapping + 策略「适用模型=
// [glm-5.2]」+ 请求 model=glm-5.2 → 第 1 次 200、第 2 次 429（与改动前一致）；
// 另外验证无路由的 404 请求不计入限流：先发 404 请求，再发同样请求仍 200。
func TestTC04_NoRedirectNoRegression404NotCounted(t *testing.T) {
	e := newTestEnv(t, plainAIConf(), rpmPolicy([]string{targetModel}))
	defer e.Close()

	// 无路由 404：host 能解析到 ai_product，但 ak_rl 的路由表无匹配规则。
	resp, body, err := e.sendRequest(hostNoRoute, apiKeyRL, targetModel)
	if err != nil {
		t.Fatalf("send no-route request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		e.logBFEException()
		t.Fatalf("expected no-route request 404, got %d, body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "AI route not found") {
		t.Fatalf("expected 'AI route not found' in body, got %q", body)
	}

	// 404 不计数：同样的 glm-5.2 请求仍放行。
	resp, body, err = e.sendRequest(hostPlain, apiKeyRL, targetModel)
	if err != nil {
		t.Fatalf("send first plain request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected first plain request 200 (404 must not count), got %d, body: %s", resp.StatusCode, body)
	}

	backend := e.backends[clusterPlain]
	if backend.Hits() != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterPlain, backend.Hits())
	}
	models := backend.Models()
	if len(models) != 1 || models[0] != targetModel {
		t.Fatalf("expected backend received model %q (target == client model), got %v", targetModel, models)
	}

	resp, body, err = e.sendRequest(hostPlain, apiKeyRL, targetModel)
	if err != nil {
		t.Fatalf("send second plain request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected second plain request 429, got %d, body: %s", resp.StatusCode, body)
	}
	eb := parseAIError(t, body)
	if eb.Error.Code != "RPM_LIMIT_EXCEEDED" {
		t.Fatalf("expected code RPM_LIMIT_EXCEEDED, got %q, body: %s", eb.Error.Code, body)
	}
	if eb.Error.Details.LimitType != "rpm" {
		t.Fatalf("expected limit_type rpm, got %q, body: %s", eb.Error.Details.LimitType, body)
	}
}

// TestTC05 验证并发上限单请求单槽位：MaxConcurrency=1 + 主目标 5xx 触发
// fallback 时，每请求仅检查/预占一次并发槽位（fallback attempt 由幂等守卫
// 跳过）——fallback attempt 不被自身并发槽位误拒，请求结束后槽位正确释放。
func TestTC05_MaxConcurrencySingleSlotPerRequest(t *testing.T) {
	e := newTestEnv(t, fallbackAIConf(), concurrencyPolicy([]string{targetModel}))
	defer e.Close()

	// 主目标始终返回 500，fallback 集群正常返回 200。
	e.backends[clusterMain].ResponseFunc = func(r *http.Request, count int) (int, string) {
		return http.StatusInternalServerError, `{"error":"backend down"}`
	}

	// 请求 1：主目标 5xx → fallback → 200。若 fallback attempt 重复预占并发
	// 槽位（1+1>1），请求会被本地 429 误拒。
	resp, body, err := e.sendRequest(hostFallback, apiKeyRL, targetModel)
	if err != nil {
		t.Fatalf("send first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected first request 200, got %d, body: %s", resp.StatusCode, body)
	}
	if hits := e.backends[clusterMain].Hits(); hits != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterMain, hits)
	}
	if hits := e.backends[clusterBackup].Hits(); hits != 1 {
		t.Fatalf("expected 1 hit on %s, got %d", clusterBackup, hits)
	}

	// 请求 2：请求 1 结束后槽位必须已释放（不泄漏、不重复占用），
	// 完整走一遍 5xx → fallback → 200。
	resp, body, err = e.sendRequest(hostFallback, apiKeyRL, targetModel)
	if err != nil {
		t.Fatalf("send second request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected second request 200 (concurrency slot must be released), got %d, body: %s",
			resp.StatusCode, body)
	}
	if hits := e.backends[clusterMain].Hits(); hits != 2 {
		t.Fatalf("expected 2 hits on %s, got %d", clusterMain, hits)
	}
	if hits := e.backends[clusterBackup].Hits(); hits != 2 {
		t.Fatalf("expected 2 hits on %s, got %d", clusterBackup, hits)
	}
}

// TestTC06 验证上游 429 语义不变：模拟后端对首个 key 返回 429 时，BFE 仍把
// 上游 429 归类为 provider key 限流并轮换到下一个 key（区别于本地策略 429
// 不轮换）：最终响应 200，后端收到 2 次请求、2 个不同 provider key。
func TestTC06_Upstream429StillRotatesKey(t *testing.T) {
	e := newTestEnv(t, upstreamAIConf(), emptyPolicy())
	defer e.Close()

	// 后端首次命中（任意 key）返回 429，后续命中返回 200：
	// 首个 key 收到 429 后 BFE 轮换到第二个 key 并成功。
	e.backends[clusterUpstream].ResponseFunc = func(r *http.Request, count int) (int, string) {
		if count == 1 {
			return http.StatusTooManyRequests, `{"error":"upstream rate limited"}`
		}
		return http.StatusOK, openaiUsageBody
	}

	resp, body, err := e.sendRequest(hostUpstream, apiKeyUP, targetModel)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200 after key rotation, got %d, body: %s", resp.StatusCode, body)
	}

	backend := e.backends[clusterUpstream]
	if backend.Hits() != 2 {
		t.Fatalf("expected 2 backend hits (429 then rotated retry), got %d", backend.Hits())
	}
	if n := countDistinct(backend.AuthHeaders()); n != 2 {
		t.Fatalf("expected 2 distinct provider keys used, got %d: %v", n, backend.AuthHeaders())
	}
}
