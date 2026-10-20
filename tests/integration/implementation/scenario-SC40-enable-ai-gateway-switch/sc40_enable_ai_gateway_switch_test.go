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

package sc40

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

	apiKey   = "ak_auth"
	apiKeyId = "auth_key_id"
)

var defaultBody = []byte(`{"model":"gpt-4"}`)
var usageResponse = `{"id":"chatcmpl-sc40","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`

type aiErrorBody struct {
	Error struct {
		Code string `json:"code"`
		Type string `json:"type"`
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

// newTestEnv builds a topology with the AI modules loaded, then sets
// EnableAiGateway to enableAI before starting BFE.
func newTestEnv(t *testing.T, enableAI bool) *testEnv {
	e := &testEnv{t: t}

	e.backend = common.NewMockBackend(clusterAuth, http.StatusOK, usageResponse)
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	tokenRule := &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": {},
		},
		Tokens: map[string]map[string]common.TokenFile{
			"ai_product": {
				apiKey: {
					Key:            apiKey,
					KeyId:          apiKeyId,
					Enabled:        true,
					ExpiredTime:    -1,
					UnlimitedQuota: true,
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
		t.Fatalf("build bfe config failed: %v", err)
	}

	if err := common.RewriteConfLine(filepath.Join(e.confDir, "bfe.conf"), "EnableAiGateway",
		fmt.Sprintf("%t", enableAI)); err != nil {
		t.Fatalf("set EnableAiGateway failed: %v", err)
	}

	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
	return e
}

func singleKeyAIConf() *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type: 0,
		Keys: []cluster_conf.AIKey{
			{Name: "key-1", Key: "sk-sc40-1", Weight: 100},
		},
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:            "weighted_random",
			MaxRetries:          0,
			RetryBackoffInitial: 20,
			RetryBackoffMax:     50,
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

func (e *testEnv) stopAndParseLogs() []*bfe_access_pb.RequestLog {
	e.t.Helper()
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

func (e *testEnv) sendRequest(key string, body []byte, headers map[string]string) (*http.Response, string, error) {
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
	for k, v := range headers {
		req.Header.Set(k, v)
	}

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

func lastRequestLog(t *testing.T, logs []*bfe_access_pb.RequestLog) *bfe_access_pb.RequestLog {
	t.Helper()
	if len(logs) == 0 {
		t.Fatalf("expected at least 1 request log, got 0")
	}
	return logs[len(logs)-1]
}

// TestTC01_GatewayOffPlainForward verifies that with EnableAiGateway=false an
// AI-shaped request is forwarded by the plain reverse proxy: no auth, backend
// receives the original Host and body.
func TestTC01_GatewayOffPlainForward(t *testing.T) {
	e := newTestEnv(t, false)
	defer e.Close()

	resp, body, err := e.sendRequest("", defaultBody, nil)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
	}
	if strings.Contains(body, "NO_API_KEY") {
		t.Fatalf("AI auth must be disabled, got body: %s", body)
	}
	if hits := e.backend.Hits(); hits != 1 {
		t.Fatalf("expected 1 backend hit, got %d", hits)
	}
	hosts := e.backend.Hosts()
	if len(hosts) != 1 || hosts[0] != apiHost {
		t.Fatalf("expected backend Host=%s, got %v", apiHost, hosts)
	}
	bodies := e.backend.RequestBodies()
	if len(bodies) != 1 || !bytes.Equal(bodies[0], defaultBody) {
		t.Fatalf("expected unchanged request body, got %s", bodies)
	}
}

// TestTC02_GatewayOffNoPanic verifies that AI modules loaded while the gateway
// is disabled skip safely (the nil-AiBasicInfo panic fix): repeated requests
// with and without keys all succeed and BFE stays alive.
func TestTC02_GatewayOffNoPanic(t *testing.T) {
	e := newTestEnv(t, false)
	defer e.Close()

	for i := 0; i < 10; i++ {
		key := ""
		if i%2 == 0 {
			key = apiKey
		}
		resp, body, err := e.sendRequest(key, defaultBody, nil)
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("request %d: expected 200, got %d, body: %s", i+1, resp.StatusCode, body)
		}
	}

	// BFE must still answer on the monitor port (process alive, no panic).
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/monitor/module_status", e.monitorPort))
	if err != nil {
		t.Fatalf("monitor request after 10 requests failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("monitor status = %d", resp.StatusCode)
	}
	if hits := e.backend.Hits(); hits != 10 {
		t.Fatalf("expected 10 backend hits, got %d", hits)
	}
}

// TestTC03_GatewayOffNoAiLogFields verifies the access log carries no AI fields
// when the gateway is disabled.
func TestTC03_GatewayOffNoAiLogFields(t *testing.T) {
	e := newTestEnv(t, false)

	resp, body, err := e.sendRequest("", defaultBody, nil)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
	}

	reqLog := lastRequestLog(t, e.stopAndParseLogs())
	if reqLog.AiMode != nil {
		t.Fatalf("expected no ai_mode, got %q", reqLog.GetAiMode())
	}
	if reqLog.AiTargetModel != nil {
		t.Fatalf("expected no ai_target_model, got %q", reqLog.GetAiTargetModel())
	}
	if reqLog.AiTotalTokens != nil {
		t.Fatalf("expected no ai_total_tokens, got %d", reqLog.GetAiTotalTokens())
	}
}

// TestTC04_GatewayOnBaseline verifies the enabled baseline for the same
// topology: auth is enforced and AI log fields are populated.
func TestTC04_GatewayOnBaseline(t *testing.T) {
	e := newTestEnv(t, true)

	// Without a key the AI auth rejects.
	resp, body, err := e.sendRequest("", defaultBody, nil)
	if err != nil {
		t.Fatalf("send unauthenticated request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		e.logBFEException()
		t.Fatalf("expected 401, got %d, body: %s", resp.StatusCode, body)
	}
	if eb := parseAIError(t, body); eb.Error.Code != "NO_API_KEY" {
		t.Fatalf("expected code NO_API_KEY, got %q, body: %s", eb.Error.Code, body)
	}

	// With a key the request succeeds and logs AI fields.
	resp, body, err = e.sendRequest(apiKey, defaultBody, nil)
	if err != nil {
		t.Fatalf("send authenticated request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
	}

	reqLog := lastRequestLog(t, e.stopAndParseLogs())
	if got := reqLog.GetAiMode(); got != "chat" {
		t.Fatalf("expected ai_mode chat, got %q", got)
	}
	if got := reqLog.GetAiTargetModel(); got != "gpt-4" {
		t.Fatalf("expected ai_target_model gpt-4, got %q", got)
	}
}

// TestTC05_GatewayOffSseStillWorks verifies SSE detection lives outside the AI
// gateway switch: a streamed response is delivered and counted in proxy_state.
func TestTC05_GatewayOffSseStillWorks(t *testing.T) {
	e := newTestEnv(t, false)
	defer e.Close()

	e.backend.SSEEvents = []string{
		`{"choices":[{"delta":{"content":"hello"}}]}`,
		`{"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
	}

	resp, body, err := e.sendRequest("", defaultBody, map[string]string{"Accept": "text/event-stream"})
	if err != nil {
		t.Fatalf("send SSE request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "hello") {
		t.Fatalf("expected streamed events in body, got %q", body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream content type, got %q", ct)
	}

	served := common.WaitForModuleMetricValue(e.monitorPort, "proxy_state", "SseReqServed", 1, 5*time.Second)
	if served < 1 {
		t.Fatalf("expected proxy_state SSE_REQ_SERVED >= 1, got %f", served)
	}
}
