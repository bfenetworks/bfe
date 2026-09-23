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

package sc18

import (
	"bytes"
	"encoding/json"
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
	apiKey   = "ak_user_a"
	apiKeyId = "user_a_key_id"

	redirectKey = "sk-redirect-key"
	plainKey    = "sk-plain-key"
	geminiKey   = "goog-gemini-key"

	hostRedirect = "redirect.example.org"
	hostPlain    = "plain.example.org"
	hostFallback = "fallback.example.org"
	hostGemini   = "gemini.example.org"

	pathChat = "/v1/chat/completions"
	// pathGeminiGenerate 的模型（gemini-2.5-flash）在请求路径上，请求体无 model 字段。
	pathGeminiGenerate = "/v1beta/models/gemini-2.5-flash:generateContent"

	// clientModel 是请求体原始模型；经 cluster_redirect 的 ModelMapping
	// 重定向后得到 targetModel。
	clientModel = "glm-5.2-abc"
	targetModel = "glm-5.2"

	planToken     = "plan_token"
	redisKeyToken = "quota:plan_token"
	tokenQuota    = int64(1000000)
)

// openaiUsageBody 是完整的非流式 chat.completions 响应：usage.total_tokens=15。
var openaiUsageBody = `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`

// geminiUsageBody 是完整的非流式 generateContent 响应：totalTokenCount=15。
var geminiUsageBody = `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],` +
	`"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`

// geminiNativeBody 是 Gemini 客户端发送的原生请求体：只有 contents，无 model 字段。
var geminiNativeBody = []byte(`{"contents":[{"parts":[{"text":"hi"}]}]}`)

// aiErrorBody 是 BFE AI 错误响应体（bfe_basic.AiErrorBody）的反序列化视图，
// 用于断言错误码与 details.model（issue #1387 要求按目标模型拒绝）。
type aiErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details struct {
			Model string `json:"model"`
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

func defaultAIConfs() map[string]*cluster_conf.AIConf {
	redirectMapping := map[string]string{
		clientModel: targetModel,
	}
	return map[string]*cluster_conf.AIConf{
		"cluster_redirect": {
			Type:           0,
			ModelProtocols: []string{"openai"},
			ModelMapping:   &redirectMapping,
			Keys: []cluster_conf.AIKey{
				{Name: "redirect-key", Key: redirectKey, Weight: 100},
			},
		},
		"cluster_plain": {
			Type:           0,
			ModelProtocols: []string{"openai"},
			Keys: []cluster_conf.AIKey{
				{Name: "plain-key", Key: plainKey, Weight: 100},
			},
		},
		"cluster_gemini": {
			Type:           0,
			ModelProtocols: []string{"gemini"},
			Keys: []cluster_conf.AIKey{
				{Name: "gemini-key", Key: geminiKey, Weight: 100},
			},
		},
	}
}

// newTestEnv 启动计费环境：mod_ai_route + mod_ai_token_auth（token 配额方案，
// miniredis 支撑）+ mod_body_process，与 SC16 的接线方式一致。token ak_user_a
// 同时支持 allow_models 与 block_models 配置（nil 表示不配置该列表）。
func newTestEnv(t *testing.T, allowModels, blockModels *string) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	e.backends["cluster_redirect"] = common.NewMockBackend("cluster_redirect", http.StatusOK, openaiUsageBody)
	e.backends["cluster_plain"] = common.NewMockBackend("cluster_plain", http.StatusOK, openaiUsageBody)
	e.backends["cluster_gemini"] = common.NewMockBackend("cluster_gemini", http.StatusOK, geminiUsageBody)

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
					Id:          planToken,
					Unlimited:   false,
					PassNoQuota: false,
					RedisKey:    redisKeyToken,
					ExpiredTime: -1,
					Quota:       tokenQuota,
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
					Models:         allowModels,
					BlockModels:    blockModels,
					QuotaPlans:     []string{planToken},
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
		AIConfs:       defaultAIConfs(),
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

func (e *testEnv) sendRequest(host, path, authHeader, authValue string, body []byte) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, path)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = host
	if authHeader != "" {
		req.Header.Set(authHeader, authValue)
	}
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

func chatBody(model string) []byte {
	return []byte(`{"model":"` + model + `","messages":[{"role":"user","content":"hello"}]}`)
}

// parseAIError 解析 BFE AI 错误响应体，用于断言错误码与 details.model。
func parseAIError(t *testing.T, body string) aiErrorBody {
	t.Helper()
	var eb aiErrorBody
	if err := json.Unmarshal([]byte(body), &eb); err != nil {
		t.Fatalf("parse ai error body failed: %v, body: %s", err, body)
	}
	return eb
}

// TestTC01 验证 issue #1387 修复主链路：token allow_models 按重定向后的目标模型
// 校验。allow_models="glm-5.2"（目标模型），请求 model=glm-5.2-abc 经
// cluster_redirect 的 ModelMapping 重定向为 glm-5.2 后命中白名单：响应 200，
// 后端收到的 body model 为 glm-5.2，响应体透传并计费。修复前鉴权期按请求体
// 原始模型 glm-5.2-abc 校验会被误拒。
func TestTC01_AllowlistMatchedByTargetModel(t *testing.T) {
	allow := targetModel
	e := newTestEnv(t, &allow, nil)
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, tokenQuota)

	resp, body, err := e.sendRequest(hostRedirect, pathChat, "Authorization", "Bearer "+apiKey, chatBody(clientModel))
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if !bytes.Contains([]byte(body), []byte(`"total_tokens":15`)) {
		t.Fatalf("expected backend response passed through, got: %s", body)
	}

	backend := e.backends["cluster_redirect"]
	if backend.Hits() != 1 {
		t.Fatalf("expected 1 hit on cluster_redirect, got %d", backend.Hits())
	}
	models := backend.Models()
	if len(models) != 1 || models[len(models)-1] != targetModel {
		t.Fatalf("expected backend received model %q (rewritten after redirect), got %v", targetModel, models)
	}

	// usage 解析：total_tokens=15 扣减一次。
	time.Sleep(500 * time.Millisecond)
	if remaining := e.redis.GetQuota(redisKeyToken); remaining != tokenQuota-15 {
		t.Fatalf("remaining quota = %d, want %d (total_tokens 15 billed once)", remaining, tokenQuota-15)
	}
}

// TestTC02 验证拒绝路径：同路由（重定向后目标模型 glm-5.2），但 token
// allow_models="glm-4" 不含目标模型 → 400 MODEL_NOT_ALLOWED，且 details.model
// 为目标模型 glm-5.2（而非请求体原始模型 glm-5.2-abc）；请求未到达后端，
// 配额余额不变。
func TestTC02_RejectByTargetModelNotInAllowlist(t *testing.T) {
	allow := "glm-4"
	e := newTestEnv(t, &allow, nil)
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, tokenQuota)

	resp, body, err := e.sendRequest(hostRedirect, pathChat, "Authorization", "Bearer "+apiKey, chatBody(clientModel))
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		e.logBFEException()
		t.Fatalf("expected status 400, got %d, body: %s", resp.StatusCode, body)
	}
	eb := parseAIError(t, body)
	if eb.Error.Code != "MODEL_NOT_ALLOWED" {
		t.Fatalf("expected code MODEL_NOT_ALLOWED, got %q, body: %s", eb.Error.Code, body)
	}
	if eb.Error.Details.Model != targetModel {
		t.Fatalf("expected details.model = %q (target model), got %q, body: %s", targetModel, eb.Error.Details.Model, body)
	}
	if eb.Error.Details.Model == clientModel {
		t.Fatalf("details.model must be the target model, not the raw client model %q, body: %s", clientModel, body)
	}

	backend := e.backends["cluster_redirect"]
	if backend.Hits() != 0 {
		t.Fatalf("rejected request must not reach backend, hits = %d", backend.Hits())
	}
	if e.redis.GetQuota(redisKeyToken) != tokenQuota {
		t.Fatalf("rejected request must not deduct quota")
	}
}

// TestTC03 验证 block_models 与 allow_models 对称地按目标模型校验：
// (a) block_models="glm-5.2"（目标模型被 block）→ 400 MODEL_NOT_ALLOWED，
// details.model=glm-5.2，请求未到达后端；
// (b) block_models="glm-5.2-abc"（block 的是请求体原始模型名，而非重定向后的
// 目标模型）→ 放行，响应 200，后端收到的 model 为 glm-5.2。
func TestTC03_BlockModelsMatchedByTargetModel(t *testing.T) {
	t.Run("block_target_model_rejected", func(t *testing.T) {
		block := targetModel
		e := newTestEnv(t, nil, &block)
		defer e.Close()

		e.redis.SetQuota(redisKeyToken, tokenQuota)

		resp, body, err := e.sendRequest(hostRedirect, pathChat, "Authorization", "Bearer "+apiKey, chatBody(clientModel))
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			e.logBFEException()
			t.Fatalf("expected status 400, got %d, body: %s", resp.StatusCode, body)
		}
		eb := parseAIError(t, body)
		if eb.Error.Code != "MODEL_NOT_ALLOWED" {
			t.Fatalf("expected code MODEL_NOT_ALLOWED, got %q, body: %s", eb.Error.Code, body)
		}
		if eb.Error.Details.Model != targetModel {
			t.Fatalf("expected details.model = %q (target model), got %q, body: %s", targetModel, eb.Error.Details.Model, body)
		}
		if e.backends["cluster_redirect"].Hits() != 0 {
			t.Fatalf("rejected request must not reach backend, hits = %d", e.backends["cluster_redirect"].Hits())
		}
		if e.redis.GetQuota(redisKeyToken) != tokenQuota {
			t.Fatalf("rejected request must not deduct quota")
		}
	})

	t.Run("block_raw_client_model_passed", func(t *testing.T) {
		block := clientModel
		e := newTestEnv(t, nil, &block)
		defer e.Close()

		e.redis.SetQuota(redisKeyToken, tokenQuota)

		resp, body, err := e.sendRequest(hostRedirect, pathChat, "Authorization", "Bearer "+apiKey, chatBody(clientModel))
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
		}

		backend := e.backends["cluster_redirect"]
		if backend.Hits() != 1 {
			t.Fatalf("expected 1 hit on cluster_redirect, got %d", backend.Hits())
		}
		models := backend.Models()
		if len(models) != 1 || models[0] != targetModel {
			t.Fatalf("expected backend received model %q (redirect applied), got %v", targetModel, models)
		}
	})
}

// TestTC04 验证无重定向场景行为不退化：token allow_models="glm-5.2-abc"，
// 路由到无 ModelMapping 的 cluster_plain，目标模型等于请求体原始模型
// glm-5.2-abc → 命中白名单，响应 200，后端收到的 model 保持 glm-5.2-abc。
func TestTC04_NoRedirectBehaviorUnchanged(t *testing.T) {
	allow := clientModel
	e := newTestEnv(t, &allow, nil)
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, tokenQuota)

	resp, body, err := e.sendRequest(hostPlain, pathChat, "Authorization", "Bearer "+apiKey, chatBody(clientModel))
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if !bytes.Contains([]byte(body), []byte(`"total_tokens":15`)) {
		t.Fatalf("expected backend response passed through, got: %s", body)
	}

	backend := e.backends["cluster_plain"]
	if backend.Hits() != 1 {
		t.Fatalf("expected 1 hit on cluster_plain, got %d", backend.Hits())
	}
	models := backend.Models()
	if len(models) != 1 || models[0] != clientModel {
		t.Fatalf("expected backend received model %q (target == client model), got %v", clientModel, models)
	}
}

// TestTC05 验证集群级 fallback 时对每个 attempt 的目标模型复校验：
// 主目标 cluster_plain（无重定向，目标模型 glm-5.2-abc）+ fallback
// cluster_redirect（重定向后目标模型 glm-5.2）；token allow_models="glm-5.2"。
// 主目标 attempt 校验失败产生本地 400（在 clusterInvoke 之前，后端未收到请求），
// 400 在默认 aiFallbackStatusCodes 内触发集群级 fallback；fallback attempt 重算
// 目标模型为 glm-5.2 后复校验通过 → 最终响应 200。
func TestTC05_FallbackRevalidateTargetModel(t *testing.T) {
	allow := targetModel
	e := newTestEnv(t, &allow, nil)
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, tokenQuota)

	resp, body, err := e.sendRequest(hostFallback, pathChat, "Authorization", "Bearer "+apiKey, chatBody(clientModel))
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if !bytes.Contains([]byte(body), []byte(`"total_tokens":15`)) {
		t.Fatalf("expected fallback backend response passed through, got: %s", body)
	}

	// 校验点在 doSingleAIForward 内、clusterInvoke 之前：主目标 cluster_plain
	// 的后端未收到请求；fallback cluster_redirect 收到 1 次请求。
	if hits := e.backends["cluster_plain"].Hits(); hits != 0 {
		t.Fatalf("expected 0 hit on cluster_plain (rejected before clusterInvoke), got %d", hits)
	}
	redirectBackend := e.backends["cluster_redirect"]
	if redirectBackend.Hits() != 1 {
		t.Fatalf("expected 1 hit on cluster_redirect, got %d", redirectBackend.Hits())
	}
	models := redirectBackend.Models()
	if len(models) != 1 || models[0] != targetModel {
		t.Fatalf("expected fallback backend received model %q (redirect applied), got %v", targetModel, models)
	}

	// 最终成功仅按 fallback 响应计费一次。
	time.Sleep(500 * time.Millisecond)
	if remaining := e.redis.GetQuota(redisKeyToken); remaining != tokenQuota-15 {
		t.Fatalf("remaining quota = %d, want %d (total_tokens 15 billed once)", remaining, tokenQuota-15)
	}
}

// TestTC06 验证 gemini 路径模型在 issue #1387 修复后语义不回退（#1384 语义继承）：
// gemini 原生请求体无 model 字段，模型经 extractClientModel 从路径提取进入
// ClientModel，无路由覆盖/无 ModelMapping 时目标模型继承 ClientModel；
// token allow_models="gemini-2.5-flash" → 放行转发并计费。
func TestTC06_GeminiPathModelRegression(t *testing.T) {
	allow := "gemini-2.5-flash"
	e := newTestEnv(t, &allow, nil)
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, tokenQuota)

	resp, body, err := e.sendRequest(hostGemini, pathGeminiGenerate, "x-goog-api-key", apiKey, geminiNativeBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	if !bytes.Contains([]byte(body), []byte(`"totalTokenCount":15`)) {
		t.Fatalf("expected gemini response passed through, got: %s", body)
	}

	backend := e.backends["cluster_gemini"]
	if backend.Hits() != 1 {
		t.Fatalf("expected 1 hit on cluster_gemini, got %d", backend.Hits())
	}
	xGoogKeys := backend.XGoogApiKeyHeaders()
	if len(xGoogKeys) != 1 || xGoogKeys[0] != geminiKey {
		t.Fatalf("expected x-goog-api-key %q, got %v", geminiKey, xGoogKeys)
	}

	time.Sleep(500 * time.Millisecond)
	if remaining := e.redis.GetQuota(redisKeyToken); remaining != tokenQuota-15 {
		t.Fatalf("remaining quota = %d, want %d (totalTokenCount 15 billed once)", remaining, tokenQuota-15)
	}
}
