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

package sc16

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
	apiKey   = "ak_user_a"
	apiKeyId = "user_a_key_id"

	geminiKey     = "goog-gemini-key"
	openaiKey     = "sk-openai-key"
	bothOpenAIKey = "sk-both-openai-key"
	bothGeminiKey = "goog-both-gemini-key"
	multiKeyA     = "goog-multi-a"
	multiKeyB     = "goog-multi-b"

	hostGemini   = "gemini.example.org"
	hostOpenAI   = "openai.example.org"
	hostBoth     = "both.example.org"
	hostMultiKey = "multikey.example.org"

	pathGemini       = "/v1beta/models/gemini-2.5-flash:generateContent"
	pathGeminiStream = "/v1beta/models/gemini-2.5-flash:streamGenerateContent"
	pathOpenAI       = "/v1/chat/completions"

	planToken     = "plan_token"
	redisKeyToken = "quota:plan_token"
	tokenQuota    = int64(1000000)
)

var geminiBody = []byte(`{"model":"gemini-2.5-flash","contents":[{"parts":[{"text":"hi"}]}]}`)

// geminiUsageBody is a complete non-streaming generateContent response:
// camelCase usageMetadata, totalTokenCount = 15.
var geminiUsageBody = `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],` +
	`"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"cachedContentTokenCount":3,"totalTokenCount":15}}`

var openaiUsageBody = `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`

// geminiStreamChunks carries accumulated usageMetadata per chunk
// (totalTokenCount 11 -> 13 -> 15). Gemini has no SSE termination event.
var geminiStreamChunks = []string{
	`{"candidates":[{"content":{"parts":[{"text":"he"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1,"totalTokenCount":11}}`,
	`{"candidates":[{"content":{"parts":[{"text":"llo"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13}}`,
	`{"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"cachedContentTokenCount":4,"totalTokenCount":15}}`,
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
	return map[string]*cluster_conf.AIConf{
		"cluster_gemini_only": {
			Type:           0,
			ModelProtocols: []string{"gemini"},
			Keys: []cluster_conf.AIKey{
				{Name: "gemini-key", Key: geminiKey, Weight: 100},
			},
		},
		"cluster_openai_only": {
			Type:           0,
			ModelProtocols: []string{"openai"},
			Keys: []cluster_conf.AIKey{
				{Name: "openai-key", Key: openaiKey, Weight: 100},
			},
		},
		"cluster_both_protocols": {
			Type:           0,
			ModelProtocols: []string{"openai", "gemini"},
			Keys: []cluster_conf.AIKey{
				{Name: "both-openai-key", Key: bothOpenAIKey, Weight: 50},
				{Name: "both-gemini-key", Key: bothGeminiKey, Weight: 50},
			},
		},
		"cluster_gemini_multikey": {
			Type:           0,
			ModelProtocols: []string{"gemini"},
			Keys: []cluster_conf.AIKey{
				{Name: "multi-a", Key: multiKeyA, Weight: 50},
				{Name: "multi-b", Key: multiKeyB, Weight: 50},
			},
			KeyPolicy: &cluster_conf.AIKeyPolicy{
				Strategy:            "weighted_random",
				MaxRetries:          3,
				RetryBackoffInitial: 50,
				RetryBackoffMax:     200,
			},
		},
	}
}

// newTestEnv starts the billing-enabled environment: mod_ai_route +
// mod_ai_token_auth (token quota plan backed by miniredis) +
// mod_body_process, mirroring scenario-SC03's wiring.
func newTestEnv(t *testing.T) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	e.backends["cluster_gemini_only"] = common.NewMockBackend("cluster_gemini_only", http.StatusOK, geminiUsageBody)
	e.backends["cluster_openai_only"] = common.NewMockBackend("cluster_openai_only", http.StatusOK, openaiUsageBody)
	e.backends["cluster_both_protocols"] = common.NewMockBackend("cluster_both_protocols", http.StatusOK, geminiUsageBody)
	e.backends["cluster_gemini_multikey"] = common.NewMockBackend("cluster_gemini_multikey", http.StatusOK, geminiUsageBody)

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

// newRouteOnlyEnv starts the minimal mod_ai_route-only environment used by
// TC-06: no mod_ai_token_auth, so a request without any credential header
// reaches the reverse proxy and exercises path-based protocol detection.
func newRouteOnlyEnv(t *testing.T) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	e.backends["cluster_gemini_only"] = common.NewMockBackend("cluster_gemini_only", http.StatusOK, geminiUsageBody)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	confDir := filepath.Join(e.processEnv.WorkDir(), "conf")
	logDir := filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata_routeonly",
		TargetConfDir: confDir,
		Backends:      e.backends,
		AIConfs: map[string]*cluster_conf.AIConf{
			"cluster_gemini_only": {
				Type:           0,
				ModelProtocols: []string{"gemini"},
				Keys: []cluster_conf.AIKey{
					{Name: "gemini-key", Key: geminiKey, Weight: 100},
				},
			},
		},
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

// TestTC01 verifies that a non-streaming Gemini request (x-goog-api-key,
// :generateContent path) is forwarded to a gemini-only cluster with the
// x-goog-api-key credential injected, no Authorization header, and the
// usageMetadata tokens are billed exactly once.
func TestTC01_GeminiNonStreamForwardedAndBilled(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, tokenQuota)

	resp, body, err := e.sendRequest(hostGemini, pathGemini, "x-goog-api-key", apiKey, geminiBody)
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

	backend := e.backends["cluster_gemini_only"]
	if backend.Hits() != 1 {
		t.Fatalf("expected 1 hit on gemini-only cluster, got %d", backend.Hits())
	}
	xGoogKeys := backend.XGoogApiKeyHeaders()
	if len(xGoogKeys) != 1 || xGoogKeys[0] != geminiKey {
		t.Fatalf("expected x-goog-api-key '%s', got %v", geminiKey, xGoogKeys)
	}
	authHeaders := backend.AuthHeaders()
	if len(authHeaders) != 1 || authHeaders[0] != "" {
		t.Fatalf("expected no Authorization header, got %v", authHeaders)
	}

	// usageMetadata parsed: totalTokenCount=15 deducted from the token plan.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyToken)
	if remaining != tokenQuota-15 {
		t.Fatalf("remaining quota = %d, want %d (totalTokenCount 15 billed once)", remaining, tokenQuota-15)
	}
}

// TestTC02 verifies that a streaming streamGenerateContent response without
// any SSE termination event is billed by the LAST usage-bearing chunk
// (accumulated usageMetadata 11 -> 13 -> 15), not by an intermediate chunk
// and not by their sum.
func TestTC02_GeminiStreamBilledByLastUsageChunk(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, tokenQuota)

	backend := e.backends["cluster_gemini_only"]
	backend.SSEEvents = geminiStreamChunks

	resp, body, err := e.sendRequest(hostGemini, pathGeminiStream, "x-goog-api-key", apiKey, geminiBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}
	for _, chunk := range geminiStreamChunks {
		if !bytes.Contains([]byte(body), []byte(chunk)) {
			t.Fatalf("expected SSE chunk passed through, missing: %s\nbody: %s", chunk, body)
		}
	}

	xGoogKeys := backend.XGoogApiKeyHeaders()
	if len(xGoogKeys) != 1 || xGoogKeys[0] != geminiKey {
		t.Fatalf("expected x-goog-api-key '%s', got %v", geminiKey, xGoogKeys)
	}

	// Only the final chunk's totalTokenCount (15) may be billed.
	time.Sleep(500 * time.Millisecond)
	remaining := e.redis.GetQuota(redisKeyToken)
	if remaining != tokenQuota-15 {
		t.Fatalf("remaining quota = %d, want %d (billed by last usage chunk, not 11/13/39)", remaining, tokenQuota-15)
	}
}

// TestTC03 verifies that a Gemini-style request routed to an openai-only
// cluster is rejected with 400 PROVIDER_PROTOCOL_MISMATCH.
func TestTC03_GeminiRejectedByOpenAICluster(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, tokenQuota)

	resp, body, err := e.sendRequest(hostOpenAI, pathGemini, "x-goog-api-key", apiKey, geminiBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		e.logBFEException()
		t.Fatalf("expected status 400, got %d, body: %s", resp.StatusCode, body)
	}
	if !bytes.Contains([]byte(body), []byte("PROVIDER_PROTOCOL_MISMATCH")) {
		t.Fatalf("expected PROVIDER_PROTOCOL_MISMATCH in body, got: %s", body)
	}
	if e.backends["cluster_openai_only"].Hits() != 0 {
		t.Fatalf("expected openai-only cluster not hit, got %d", e.backends["cluster_openai_only"].Hits())
	}
	if e.redis.GetQuota(redisKeyToken) != tokenQuota {
		t.Fatalf("rejected request must not deduct quota")
	}
}

// TestTC04 verifies that an OpenAI-style request routed to a gemini-only
// cluster is rejected with 400 PROVIDER_PROTOCOL_MISMATCH.
func TestTC04_OpenAIRejectedByGeminiCluster(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, tokenQuota)

	resp, body, err := e.sendRequest(hostGemini, pathOpenAI, "Authorization", "Bearer "+apiKey, []byte(`{"model":"gpt-4"}`))
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		e.logBFEException()
		t.Fatalf("expected status 400, got %d, body: %s", resp.StatusCode, body)
	}
	if !bytes.Contains([]byte(body), []byte("PROVIDER_PROTOCOL_MISMATCH")) {
		t.Fatalf("expected PROVIDER_PROTOCOL_MISMATCH in body, got: %s", body)
	}
	if e.backends["cluster_gemini_only"].Hits() != 0 {
		t.Fatalf("expected gemini-only cluster not hit, got %d", e.backends["cluster_gemini_only"].Hits())
	}
	if e.redis.GetQuota(redisKeyToken) != tokenQuota {
		t.Fatalf("rejected request must not deduct quota")
	}
}

// TestTC05 verifies that Authorization takes precedence over x-goog-api-key
// for protocol/style detection when both headers are present.
func TestTC05_AuthorizationPrecedence(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, tokenQuota)

	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, pathOpenAI)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(`{"model":"gpt-4"}`)))
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Host = hostBoth
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("x-goog-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, string(respBody))
	}

	backend := e.backends["cluster_both_protocols"]
	authHeaders := backend.AuthHeaders()
	if len(authHeaders) != 1 {
		t.Fatalf("expected one request to backend, got %d", backend.Hits())
	}
	// Authorization wins: the request is treated as openai and BFE injects
	// the cluster key as a Bearer token (either cluster key is acceptable
	// since key selection is weighted random).
	if authHeaders[0] != "Bearer "+bothOpenAIKey && authHeaders[0] != "Bearer "+bothGeminiKey {
		t.Fatalf("expected Authorization to carry a cluster key when both headers present, got auth=%v x-goog-api-key=%v",
			authHeaders, backend.XGoogApiKeyHeaders())
	}
}

// TestTC06 documents the reachability boundary of path-based protocol
// detection: in AI gateway mode a request without any credential header is
// rejected by the routing layer (404 "AI route not found") before it reaches
// the forwarding stage, because mod_ai_route only consults route tables for
// identified API keys. The gemini path rules (:generateContent /
// :streamGenerateContent / /v1beta/models/) live in DetectProtocol, which the
// reverse proxy only calls as a fallback when AuthStyle was not identified
// from request headers — a state that cannot occur end-to-end (any
// credential header identifies a style). Those path rules are therefore
// covered directly by the bfe_model_protocol unit tests (detect_test.go),
// and this TC locks the routing-layer interception behavior.
func TestTC06_PathBasedDetectionRoutingBoundary(t *testing.T) {
	e := newRouteOnlyEnv(t)
	defer e.Close()

	resp, body, err := e.sendRequest("path.example.org", pathGemini, "", "", geminiBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		e.logBFEException()
		t.Fatalf("expected status 404 for keyless request, got %d, body: %s", resp.StatusCode, body)
	}
	if !bytes.Contains([]byte(body), []byte("AI route not found")) {
		t.Fatalf("expected 'AI route not found' in body, got: %s", body)
	}
	if e.backends["cluster_gemini_only"].Hits() != 0 {
		t.Fatalf("expected gemini-only cluster not hit, got %d", e.backends["cluster_gemini_only"].Hits())
	}
}

// TestTC07 verifies that a cluster supporting both protocols handles OpenAI
// and Gemini requests with the correct per-protocol authentication headers.
func TestTC07_BothProtocolsCluster(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, tokenQuota)

	// OpenAI style: Authorization -> Bearer cluster key.
	resp, body, err := e.sendRequest(hostBoth, pathOpenAI, "Authorization", "Bearer "+apiKey, []byte(`{"model":"gpt-4"}`))
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200 for openai style, got %d, body: %s", resp.StatusCode, body)
	}

	// Gemini style: x-goog-api-key -> cluster key, no Authorization.
	resp, body, err = e.sendRequest(hostBoth, pathGemini, "x-goog-api-key", apiKey, geminiBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200 for gemini style, got %d, body: %s", resp.StatusCode, body)
	}

	backend := e.backends["cluster_both_protocols"]
	if backend.Hits() != 2 {
		t.Fatalf("expected 2 hits on both-protocols cluster, got %d", backend.Hits())
	}

	authHeaders := backend.AuthHeaders()
	xGoogKeys := backend.XGoogApiKeyHeaders()
	if authHeaders[0] != "Bearer "+bothOpenAIKey && authHeaders[0] != "Bearer "+bothGeminiKey {
		t.Fatalf("expected OpenAI-style Authorization header, got %v", authHeaders)
	}
	if xGoogKeys[1] != bothOpenAIKey && xGoogKeys[1] != bothGeminiKey {
		t.Fatalf("expected Gemini-style x-goog-api-key header, got %v", xGoogKeys)
	}
	if authHeaders[1] != "" {
		t.Fatalf("expected no Authorization header on the gemini request, got %v", authHeaders)
	}
}

// TestTC08 verifies gemini key-level rotation: an upstream 401 on one key
// marks it dead for the current request and BFE retries with the other key.
func TestTC08_UnauthorizedKeyRotates(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, tokenQuota)

	backend := e.backends["cluster_gemini_multikey"]
	backend.ResponseFunc = func(r *http.Request, count int) (int, string) {
		if r.Header.Get("x-goog-api-key") == multiKeyA {
			return http.StatusUnauthorized, `{"error":"unauthorized"}`
		}
		return http.StatusOK, geminiUsageBody
	}

	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		resp, body, err := e.sendRequest(hostMultiKey, pathGemini, "x-goog-api-key", apiKey, geminiBody)
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
		}
		for _, k := range backend.XGoogApiKeyHeaders() {
			seen[k] = true
		}
		if seen[multiKeyA] && seen[multiKeyB] {
			break
		}
	}
	if !seen[multiKeyA] {
		t.Fatalf("expected key-a to be used at least once")
	}
	if !seen[multiKeyB] {
		t.Fatalf("expected key-b to be used (rotation after 401), x-goog-api-key headers: %v", backend.XGoogApiKeyHeaders())
	}
}
