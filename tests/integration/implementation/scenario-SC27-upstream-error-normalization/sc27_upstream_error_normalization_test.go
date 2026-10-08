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

// SC27 upstream error normalization (AIConf.NormalizeUpstreamError).
// Design docs:
//
//	tests/integration/测试设计文档/scenario-SC27-上游错误体归一/
//	docs/zh_cn/modifications/2026-10-06-upstream-error-normalization/design-changes.md
package sc27

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
	apiHost = "front.example.org"

	apiPath       = "/v1/chat/completions"
	anthropicPath = "/v1/messages"
	geminiPath    = "/v1beta/models/gemini-pro:streamGenerateContent"

	apiKey  = "ak_front"
	fbKey   = "ak_fb"
	denyKey = "ak_deny"

	clusterErr = "cluster_err"
	clusterFb  = "cluster_fb"

	upstreamKey = "sk-upstream-secret"
	redactMask  = "••••••••"

	gwErrorHeader = "X-Bfe-Gw-Error"
)

var chatBody = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}]}`)

// unifiedErrBody is the client-facing OpenAI-compatible error envelope.
type unifiedErrBody struct {
	Error struct {
		Code    string `json:"code"`
		Type    string `json:"type"`
		Message string `json:"message"`
		Details struct {
			UpstreamStatus    *int   `json:"upstream_status"`
			UpstreamCode      string `json:"upstream_code"`
			RetryAfterSeconds int    `json:"retry_after_seconds"`
			Model             string `json:"model"`
		} `json:"details"`
	} `json:"error"`
}

func parseUnifiedErr(t *testing.T, body string) unifiedErrBody {
	t.Helper()
	var ue unifiedErrBody
	if err := json.Unmarshal([]byte(body), &ue); err != nil {
		t.Fatalf("response is not a unified error body: %v, body: %s", err, body)
	}
	return ue
}

// testEnv holds all resources for a single SC27 integration test.
type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	errBk      *common.MockBackend
	fbBk       *common.MockBackend
	confDir    string
	logDir     string
	bfePort    int
	stopBFE    func()
}

// newTestEnv starts the mock backends and BFE with the given per-cluster
// AIConfs. aiConfs[nil] is allowed: the cluster gets keys but no
// NormalizeUpstreamError.
func newTestEnv(t *testing.T, errConf, fbConf *cluster_conf.UpstreamErrorNormalizeConf) *testEnv {
	e := &testEnv{t: t}

	e.errBk = common.NewMockBackend(clusterErr, http.StatusOK, "")
	e.fbBk = common.NewMockBackend(clusterFb, http.StatusOK, "")

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends: map[string]*common.MockBackend{
			clusterErr: e.errBk,
			clusterFb:  e.fbBk,
		},
		AIConfs: map[string]*cluster_conf.AIConf{
			clusterErr: errAIConf(errConf),
			clusterFb:  fbAIConf(fbConf),
		},
		TokenRuleData: tokenRule(),
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
	return e
}

func errAIConf(nuc *cluster_conf.UpstreamErrorNormalizeConf) *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type:                   0,
		ModelProtocols:         []string{"openai", "anthropic", "gemini"},
		Keys:                   []cluster_conf.AIKey{{Name: "key-primary", Key: upstreamKey, Weight: 100}},
		NormalizeUpstreamError: nuc,
	}
}

func fbAIConf(nuc *cluster_conf.UpstreamErrorNormalizeConf) *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type:                   0,
		ModelProtocols:         []string{"openai", "anthropic", "gemini"},
		Keys:                   []cluster_conf.AIKey{{Name: "key-fb", Key: "sk-fb-secret", Weight: 100}},
		NormalizeUpstreamError: nuc,
	}
}

// tokenRule returns a token rule with an unlimited quota plan and the three
// API keys of this scenario. ak_deny restricts allow_models to gpt-4.
func tokenRule() *common.TokenRuleData {
	allow := "gpt-4"
	return &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": {
				{
					Id:          "unlimited_plan",
					Unlimited:   true,
					PassNoQuota: false,
					RedisKey:    "quota:unlimited_plan",
					ExpiredTime: -1,
					Quota:       0,
					Unit:        "total_token",
				},
			},
		},
		Tokens: map[string]map[string]common.TokenFile{
			"ai_product": {
				apiKey: {
					Key: apiKey, KeyId: "front_key_id", Enabled: true, ExpiredTime: -1,
					QuotaPlans: []string{"unlimited_plan"},
				},
				fbKey: {
					Key: fbKey, KeyId: "fb_key_id", Enabled: true, ExpiredTime: -1,
					QuotaPlans: []string{"unlimited_plan"},
				},
				denyKey: {
					Key: denyKey, KeyId: "deny_key_id", Enabled: true, ExpiredTime: -1,
					Models: &allow, QuotaPlans: []string{"unlimited_plan"},
				},
			},
		},
		Config: map[string][]common.TokenRule{
			"ai_product": {
				{Cond: "default_t()", Action: common.ActionFile{Cmd: "CHECK_TOKEN"}},
			},
		},
	}
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	if e.errBk != nil {
		e.errBk.Close()
	}
	if e.fbBk != nil {
		e.fbBk.Close()
	}
}

func (e *testEnv) logBFEException() {
	data, err := os.ReadFile(filepath.Join(e.logDir, "exception.log"))
	if err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}
	if logData, err := os.ReadFile(filepath.Join(e.logDir, "bfe.log")); err == nil && len(logData) > 0 {
		lines := strings.Split(string(logData), "\n")
		start := 0
		if len(lines) > 100 {
			start = len(lines) - 100
		}
		e.t.Logf("bfe log tail:\n%s", strings.Join(lines[start:], "\n"))
	}
}

// sendRaw posts to the given path with the given credential header style.
func (e *testEnv) sendRaw(path, apikey, credStyle string, body []byte) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, path)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	switch credStyle {
	case "x-api-key":
		req.Header.Set("x-api-key", apikey)
	case "x-goog-api-key":
		req.Header.Set("x-goog-api-key", apikey)
	default:
		req.Header.Set("Authorization", "Bearer "+apikey)
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

func (e *testEnv) sendChat(apikey string, body []byte) (*http.Response, string, error) {
	return e.sendRaw(apiPath, apikey, "bearer", body)
}

// stopAndLogs stops BFE (flushing access logs) and parses the b2log records.
func (e *testEnv) stopAndLogs() []*bfe_access_pb.RequestLog {
	e.t.Helper()
	if e.stopBFE != nil {
		time.Sleep(500 * time.Millisecond)
		e.stopBFE()
		e.stopBFE = nil
	}
	reqLogs, err := common.ParseAccessLogAfterStop(e.logDir)
	if err != nil {
		e.t.Fatalf("parse access log failed: %v", err)
	}
	return reqLogs
}

func enabledConf() *cluster_conf.UpstreamErrorNormalizeConf {
	return &cluster_conf.UpstreamErrorNormalizeConf{Enabled: true}
}

func streamConf() *cluster_conf.UpstreamErrorNormalizeConf {
	return &cluster_conf.UpstreamErrorNormalizeConf{StreamEnabled: true}
}

// TestTC01 verifies the main non-streaming path: an upstream OpenAI 401
// envelope (echoing the injected cluster key) is rewritten into the unified
// error body with status remapped to 502 UPSTREAM_AUTH_ERROR, details carry
// the original status/code, the echoed key is masked, and the rewritten
// response carries the X-Bfe-Gw-Error marker.
func TestTC01_OpenAIErrorRewriteAndRedact(t *testing.T) {
	e := newTestEnv(t, enabledConf(), nil)
	defer e.Close()

	e.errBk.ResponseFunc = func(r *http.Request, count int) (int, string) {
		return http.StatusUnauthorized,
			`{"error":{"message":"Incorrect API key provided: ` + upstreamKey + `","type":"authentication_error","code":"invalid_api_key"}}`
	}

	resp, body, err := e.sendChat(apiKey, chatBody)
	if err != nil {
		e.logBFEException()
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d, body: %s", resp.StatusCode, body)
	}
	if resp.Header.Get(gwErrorHeader) != "1" {
		t.Fatalf("rewritten response must carry %s marker, headers: %v", gwErrorHeader, resp.Header)
	}

	ue := parseUnifiedErr(t, body)
	if ue.Error.Code != "UPSTREAM_AUTH_ERROR" {
		t.Errorf("code = %q, want UPSTREAM_AUTH_ERROR", ue.Error.Code)
	}
	if ue.Error.Type != "internal_error" {
		t.Errorf("type = %q, want internal_error", ue.Error.Type)
	}
	if ue.Error.Details.UpstreamStatus == nil || *ue.Error.Details.UpstreamStatus != 401 {
		t.Errorf("details.upstream_status = %v, want 401", ue.Error.Details.UpstreamStatus)
	}
	if ue.Error.Details.UpstreamCode != "invalid_api_key" {
		t.Errorf("details.upstream_code = %q, want invalid_api_key", ue.Error.Details.UpstreamCode)
	}
	if strings.Contains(body, upstreamKey) || !strings.Contains(ue.Error.Message, redactMask) {
		t.Errorf("credential not redacted in message: %q", ue.Error.Message)
	}

	reqLogs := e.stopAndLogs()
	if len(reqLogs) != 1 {
		t.Fatalf("expected 1 access log, got %d", len(reqLogs))
	}
	log := reqLogs[0]
	if log.AiErrNormalized == nil || !*log.AiErrNormalized {
		t.Errorf("ai_err_normalized should be true, got %v", log.AiErrNormalized)
	}
	if log.AiUpstreamStatus == nil || *log.AiUpstreamStatus != 401 {
		t.Errorf("ai_upstream_status should be 401, got %v", log.AiUpstreamStatus)
	}
	if log.AiUpstreamErrCode == nil || *log.AiUpstreamErrCode != "invalid_api_key" {
		t.Errorf("ai_upstream_err_code should be invalid_api_key, got %v", log.AiUpstreamErrCode)
	}
	if log.AiErrNormalizeMiss != nil {
		t.Errorf("ai_err_normalize_miss should be unset, got %v", log.AiErrNormalizeMiss)
	}
}

// TestTC02 verifies the 429 mapping: rate_limit_exceeded keeps 429 with
// Retry-After parsed into details; insufficient_quota maps to
// UPSTREAM_QUOTA_EXHAUSTED.
func TestTC02_Upstream429AndRetryAfter(t *testing.T) {
	e := newTestEnv(t, enabledConf(), nil)
	defer e.Close()

	e.errBk.ResponseHeaders = map[string]string{"Retry-After": "30"}
	e.errBk.ResponseFunc = func(r *http.Request, count int) (int, string) {
		// the handler has already consumed r.Body before ResponseFunc runs,
		// so dispatch on the per-backend request counter (requests are
		// sequential within this test)
		if count == 2 {
			return http.StatusTooManyRequests,
				`{"error":{"message":"Balance is zero","type":"rate_limit_error","code":"insufficient_quota"}}`
		}
		return http.StatusTooManyRequests,
			`{"error":{"message":"Rate limit reached","type":"rate_limit_error","code":"rate_limit_exceeded"}}`
	}

	resp, body, err := e.sendChat(apiKey, chatBody)
	if err != nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("first request: status=%v err=%v body=%s", resp.StatusCode, err, body)
	}
	ue := parseUnifiedErr(t, body)
	if ue.Error.Code != "UPSTREAM_RATE_LIMITED" || ue.Error.Type != "rate_limit_error" {
		t.Errorf("first request: code/type = %q/%q", ue.Error.Code, ue.Error.Type)
	}
	if ue.Error.Details.RetryAfterSeconds != 30 {
		t.Errorf("retry_after_seconds = %d, want 30", ue.Error.Details.RetryAfterSeconds)
	}

	quotaBody := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"quota"}]}`)
	resp, body, err = e.sendChat(apiKey, quotaBody)
	if err != nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second request: status=%v err=%v body=%s", resp.StatusCode, err, body)
	}
	ue = parseUnifiedErr(t, body)
	if ue.Error.Code != "UPSTREAM_QUOTA_EXHAUSTED" || ue.Error.Type != "quota_error" {
		t.Errorf("second request: code/type = %q/%q", ue.Error.Code, ue.Error.Type)
	}

	reqLogs := e.stopAndLogs()
	if len(reqLogs) != 2 {
		t.Fatalf("expected 2 access logs, got %d", len(reqLogs))
	}
	for i, lg := range reqLogs {
		if lg.AiErrNormalized == nil || !*lg.AiErrNormalized {
			t.Errorf("log %d: ai_err_normalized should be true", i)
		}
		if lg.AiUpstreamStatus == nil || *lg.AiUpstreamStatus != 429 {
			t.Errorf("log %d: ai_upstream_status should be 429, got %v", i, lg.AiUpstreamStatus)
		}
	}
}

// TestTC03 verifies the default passthrough for unrecognized error bodies:
// the upstream status and semantics stay unchanged, while RedactSecrets
// (on by default) still masks the echoed key in the passed-through body.
func TestTC03_UnrecognizedPassthroughRedacts(t *testing.T) {
	e := newTestEnv(t, enabledConf(), nil)
	defer e.Close()

	upstreamHTML := `<html><body>proxy error: backend rejected key ` + upstreamKey + `</body></html>`
	e.errBk.ResponseHeaders = map[string]string{"Content-Type": "text/html"}
	e.errBk.ResponseFunc = func(r *http.Request, count int) (int, string) {
		return http.StatusBadGateway, upstreamHTML
	}

	resp, body, err := e.sendChat(apiKey, chatBody)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("passthrough must keep upstream status 502, got %d", resp.StatusCode)
	}
	if strings.Contains(body, upstreamKey) {
		t.Errorf("passthrough body not redacted: %s", body)
	}
	if !strings.Contains(body, redactMask) {
		t.Errorf("passthrough body should carry the mask: %s", body)
	}
	if resp.Header.Get(gwErrorHeader) != "" {
		t.Errorf("passthrough must not carry the %s marker", gwErrorHeader)
	}

	reqLogs := e.stopAndLogs()
	if len(reqLogs) != 1 {
		t.Fatalf("expected 1 access log, got %d", len(reqLogs))
	}
	log := reqLogs[0]
	if log.AiErrNormalizeMiss == nil || !*log.AiErrNormalizeMiss {
		t.Errorf("ai_err_normalize_miss should be true, got %v", log.AiErrNormalizeMiss)
	}
	if log.AiErrNormalized != nil {
		t.Errorf("ai_err_normalized should be unset, got %v", log.AiErrNormalized)
	}
	if log.AiUpstreamStatus == nil || *log.AiUpstreamStatus != 502 {
		t.Errorf("ai_upstream_status should be 502, got %v", log.AiUpstreamStatus)
	}
}

// TestTC04 verifies rewrite_generic: an unrecognized body is replaced by the
// generic UPSTREAM_UNKNOWN unified error.
func TestTC04_UnrecognizedRewriteGeneric(t *testing.T) {
	e := newTestEnv(t, &cluster_conf.UpstreamErrorNormalizeConf{
		Enabled:            true,
		UnrecognizedAction: cluster_conf.NormalizeActionRewriteGeneric,
	}, nil)
	defer e.Close()

	e.errBk.ResponseHeaders = map[string]string{"Content-Type": "text/html"}
	e.errBk.ResponseFunc = func(r *http.Request, count int) (int, string) {
		return http.StatusBadGateway, `<html>oops</html>`
	}

	resp, body, err := e.sendChat(apiKey, chatBody)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", resp.StatusCode)
	}
	ue := parseUnifiedErr(t, body)
	if ue.Error.Code != "UPSTREAM_UNKNOWN" || ue.Error.Type != "internal_error" {
		t.Errorf("code/type = %q/%q, want UPSTREAM_UNKNOWN/internal_error", ue.Error.Code, ue.Error.Type)
	}
	if strings.Contains(body, "oops") {
		t.Errorf("generic rewrite must not leak the upstream body: %s", body)
	}
	if resp.Header.Get(gwErrorHeader) != "1" {
		t.Errorf("rewritten response must carry %s marker", gwErrorHeader)
	}

	reqLogs := e.stopAndLogs()
	if len(reqLogs) != 1 {
		t.Fatalf("expected 1 access log, got %d", len(reqLogs))
	}
	log := reqLogs[0]
	if log.AiErrNormalized == nil || !*log.AiErrNormalized {
		t.Errorf("ai_err_normalized should be true, got %v", log.AiErrNormalized)
	}
	if log.AiErrNormalizeMiss == nil || !*log.AiErrNormalizeMiss {
		t.Errorf("ai_err_normalize_miss should be true, got %v", log.AiErrNormalizeMiss)
	}
}

// TestTC05 is the disabled baseline: without NormalizeUpstreamError the
// upstream error passes through byte-identical (historical behavior).
func TestTC05_DisabledBaseline(t *testing.T) {
	e := newTestEnv(t, nil, nil)
	defer e.Close()

	upstreamBody := `{"error":{"message":"Incorrect API key provided: ` + upstreamKey + `","type":"authentication_error","code":"invalid_api_key"}}`
	e.errBk.ResponseFunc = func(r *http.Request, count int) (int, string) {
		return http.StatusUnauthorized, upstreamBody
	}

	resp, body, err := e.sendChat(apiKey, chatBody)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected upstream status 401, got %d", resp.StatusCode)
	}
	if body != upstreamBody {
		t.Errorf("disabled cluster must pass the body through byte-identical\n got: %s\nwant: %s", body, upstreamBody)
	}
	if resp.Header.Get(gwErrorHeader) != "" {
		t.Errorf("no marker header expected, got %q", resp.Header.Get(gwErrorHeader))
	}

	reqLogs := e.stopAndLogs()
	if len(reqLogs) != 1 {
		t.Fatalf("expected 1 access log, got %d", len(reqLogs))
	}
	log := reqLogs[0]
	if log.AiUpstreamStatus != nil || log.AiUpstreamErrCode != nil ||
		log.AiErrNormalized != nil || log.AiErrNormalizeMiss != nil ||
		log.AiStreamErrorRewritten != nil || log.AiStreamTruncated != nil {
		t.Errorf("all 810-815 fields must be unset when disabled: %+v", log)
	}
}

// TestTC06 verifies the timing/config-ownership boundary: normalization runs
// after the fallback loop and takes the config of the cluster that produced
// the final response. Step 1: fallback cluster without normalization -> the
// final 429 passes through unchanged; step 2 (mirror check): fallback
// cluster with normalization -> the final 429 is rewritten.
func TestTC06_FallbackNormalization(t *testing.T) {
	// step 1: only cluster_err enables normalization; the final response
	// comes from cluster_fb (no normalization) and must pass through
	func() {
		e := newTestEnv(t, enabledConf(), nil)
		defer e.Close()

		e.errBk.ResponseFunc = func(r *http.Request, count int) (int, string) {
			return http.StatusInternalServerError, `{"error":{"message":"boom","type":"server_error"}}`
		}
		e.fbBk.ResponseFunc = func(r *http.Request, count int) (int, string) {
			return http.StatusTooManyRequests,
				`{"error":{"message":"slow","type":"rate_limit_error","code":"rate_limit_exceeded"}}`
		}

		resp, body, err := e.sendChat(fbKey, chatBody)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if e.errBk.Hits() != 1 || e.fbBk.Hits() != 1 {
			t.Fatalf("fallback must try both clusters, err=%d fb=%d", e.errBk.Hits(), e.fbBk.Hits())
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d, body: %s", resp.StatusCode, body)
		}
		want := `{"error":{"message":"slow","type":"rate_limit_error","code":"rate_limit_exceeded"}}`
		if body != want {
			t.Errorf("final cluster has no normalization; body must pass through\n got: %s\nwant: %s", body, want)
		}
	}()

	// step 2: both clusters enable normalization; the final 429 from
	// cluster_fb must be rewritten
	func() {
		e := newTestEnv(t, enabledConf(), enabledConf())
		defer e.Close()

		e.errBk.ResponseFunc = func(r *http.Request, count int) (int, string) {
			return http.StatusInternalServerError, `{"error":{"message":"boom","type":"server_error"}}`
		}
		e.fbBk.ResponseFunc = func(r *http.Request, count int) (int, string) {
			return http.StatusTooManyRequests,
				`{"error":{"message":"slow","type":"rate_limit_error","code":"rate_limit_exceeded"}}`
		}

		resp, body, err := e.sendChat(fbKey, chatBody)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if e.errBk.Hits() != 1 || e.fbBk.Hits() != 1 {
			t.Fatalf("fallback must try both clusters, err=%d fb=%d", e.errBk.Hits(), e.fbBk.Hits())
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("429 keeps its status, got %d", resp.StatusCode)
		}
		ue := parseUnifiedErr(t, body)
		if ue.Error.Code != "UPSTREAM_RATE_LIMITED" {
			t.Errorf("code = %q, want UPSTREAM_RATE_LIMITED", ue.Error.Code)
		}
	}()
}

// TestTC07 verifies in-stream error-event rewriting for both event shapes:
// OpenAI (data: {"error":...}) and Anthropic (event: error). The response
// status stays 200, non-error events pass through unchanged, and the error
// event data payload carries the unified error JSON.
func TestTC07_SSEErrorEventRewrite(t *testing.T) {
	t.Run("openai", func(t *testing.T) {
		e := newTestEnv(t, streamConf(), nil)
		defer e.Close()

		e.errBk.SSEEvents = []string{
			`{"choices":[{"delta":{"content":"部分结果"}}]}`,
			`{"error":{"message":"stream failed with key ` + upstreamKey + `","type":"authentication_error"}}`,
			`[DONE]`,
		}

		streamBody := []byte(`{"model":"deepseek-chat","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
		resp, body, err := e.sendChat(apiKey, streamBody)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("stream keeps status 200, got %d, body: %s", resp.StatusCode, body)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
			t.Fatalf("expected SSE content type, got %q", ct)
		}
		if !strings.Contains(body, `"content":"部分结果"`) {
			t.Errorf("content event not passed through: %s", body)
		}
		if !strings.Contains(body, `"code":"UPSTREAM_AUTH_ERROR"`) {
			t.Errorf("error event not rewritten: %s", body)
		}
		if strings.Contains(body, upstreamKey) {
			t.Errorf("stream error event not redacted: %s", body)
		}
		if !strings.Contains(body, "data: [DONE]") {
			t.Errorf("[DONE] marker missing: %s", body)
		}

		reqLogs := e.stopAndLogs()
		if len(reqLogs) != 1 {
			t.Fatalf("expected 1 access log, got %d", len(reqLogs))
		}
		log := reqLogs[0]
		if log.AiStreamErrorRewritten == nil || !*log.AiStreamErrorRewritten {
			t.Errorf("ai_stream_error_rewritten should be true, got %v", log.AiStreamErrorRewritten)
		}
		if log.AiErrNormalized == nil || !*log.AiErrNormalized {
			t.Errorf("ai_err_normalized should be true, got %v", log.AiErrNormalized)
		}
		if log.AiUpstreamStatus == nil || *log.AiUpstreamStatus != 200 {
			t.Errorf("ai_upstream_status should be 200, got %v", log.AiUpstreamStatus)
		}
		if log.AiStreamTruncated != nil && *log.AiStreamTruncated {
			t.Errorf("[DONE] seen; must not be truncated")
		}
	})

	t.Run("anthropic", func(t *testing.T) {
		e := newTestEnv(t, streamConf(), nil)
		defer e.Close()

		e.errBk.SSERaw = []string{
			"event: message_start\ndata: {\"type\":\"message_start\"}\n\n",
			"event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"rate_limit_error\",\"message\":\"stream rate limited\"}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}

		anthropicBody := []byte(`{"model":"claude-sonnet-4","max_tokens":32,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
		resp, body, err := e.sendRaw(anthropicPath, apiKey, "x-api-key", anthropicBody)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("stream keeps status 200, got %d, body: %s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "event: error") {
			t.Errorf("SSE event name must be preserved: %s", body)
		}
		if !strings.Contains(body, `"code":"UPSTREAM_RATE_LIMITED"`) {
			t.Errorf("anthropic error event not rewritten: %s", body)
		}
		if !strings.Contains(body, "event: message_start") || !strings.Contains(body, "event: message_stop") {
			t.Errorf("non-error events must pass through: %s", body)
		}

		reqLogs := e.stopAndLogs()
		if len(reqLogs) != 1 {
			t.Fatalf("expected 1 access log, got %d", len(reqLogs))
		}
		log := reqLogs[0]
		if log.AiStreamErrorRewritten == nil || !*log.AiStreamErrorRewritten {
			t.Errorf("ai_stream_error_rewritten should be true, got %v", log.AiStreamErrorRewritten)
		}
		if log.AiStreamTruncated != nil && *log.AiStreamTruncated {
			t.Errorf("message_stop seen; must not be truncated")
		}
	})
}

// TestTC08 verifies stream-truncation marking: a stream that ends at EOF
// without the protocol's terminal event is marked truncated; a [DONE]-ended
// stream and a Gemini stream (StreamEndsAtEOF) are not.
func TestTC08_StreamTruncation(t *testing.T) {
	t.Run("truncated", func(t *testing.T) {
		e := newTestEnv(t, streamConf(), nil)
		defer e.Close()

		e.errBk.SSEEvents = []string{
			`{"choices":[{"delta":{"content":"一半"}}]}`,
		}

		streamBody := []byte(`{"model":"deepseek-chat","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
		resp, body, err := e.sendChat(apiKey, streamBody)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, "一半") {
			t.Fatalf("truncated stream must pass content through, status=%d body=%s", resp.StatusCode, body)
		}

		reqLogs := e.stopAndLogs()
		if len(reqLogs) != 1 {
			t.Fatalf("expected 1 access log, got %d", len(reqLogs))
		}
		if reqLogs[0].AiStreamTruncated == nil || !*reqLogs[0].AiStreamTruncated {
			t.Errorf("ai_stream_truncated should be true, got %v", reqLogs[0].AiStreamTruncated)
		}
	})

	t.Run("done", func(t *testing.T) {
		e := newTestEnv(t, streamConf(), nil)
		defer e.Close()

		e.errBk.SSEEvents = []string{
			`{"choices":[{"delta":{"content":"完整"}}]}`,
			`[DONE]`,
		}

		streamBody := []byte(`{"model":"deepseek-chat","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
		resp, _, err := e.sendChat(apiKey, streamBody)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("request failed: %v %v", resp, err)
		}

		reqLogs := e.stopAndLogs()
		if len(reqLogs) != 1 {
			t.Fatalf("expected 1 access log, got %d", len(reqLogs))
		}
		if reqLogs[0].AiStreamTruncated != nil {
			t.Errorf("ai_stream_truncated should be unset for [DONE]-ended stream, got %v", reqLogs[0].AiStreamTruncated)
		}
	})

	t.Run("gemini_eof", func(t *testing.T) {
		e := newTestEnv(t, streamConf(), nil)
		defer e.Close()

		e.errBk.SSEEvents = []string{
			`{"candidates":[{"content":{"parts":[{"text":"hi"}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`,
		}

		geminiBody := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
		resp, body, err := e.sendRaw(geminiPath, apiKey, "x-goog-api-key", geminiBody)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("gemini stream should succeed, got %d, body: %s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "usageMetadata") {
			t.Errorf("gemini chunk must pass through: %s", body)
		}

		reqLogs := e.stopAndLogs()
		if len(reqLogs) != 1 {
			t.Fatalf("expected 1 access log, got %d", len(reqLogs))
		}
		if reqLogs[0].AiStreamTruncated != nil {
			t.Errorf("Gemini stream ends at EOF; must never be truncated, got %v", reqLogs[0].AiStreamTruncated)
		}
	})
}

// TestTC09 verifies that gateway-generated errors (X-Bfe-Gw-Error marker)
// are never normalized: a local MODEL_NOT_ALLOWED rejection keeps its
// original code and shape even with normalization enabled on the cluster.
func TestTC09_GatewayGeneratedSkipped(t *testing.T) {
	e := newTestEnv(t, enabledConf(), nil)
	defer e.Close()

	resp, body, err := e.sendChat(denyKey, chatBody)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", resp.StatusCode, body)
	}
	ue := parseUnifiedErr(t, body)
	if ue.Error.Code != "MODEL_NOT_ALLOWED" {
		t.Errorf("local error must keep its code, got %q", ue.Error.Code)
	}
	if strings.HasPrefix(ue.Error.Code, "UPSTREAM_") {
		t.Errorf("gateway-generated error must not be normalized: %q", ue.Error.Code)
	}
	if resp.Header.Get(gwErrorHeader) != "1" {
		t.Errorf("local AiError response must carry the %s marker", gwErrorHeader)
	}
	if e.errBk.Hits() != 0 {
		t.Errorf("rejected request must not reach the backend, hits=%d", e.errBk.Hits())
	}

	reqLogs := e.stopAndLogs()
	if len(reqLogs) != 1 {
		t.Fatalf("expected 1 access log, got %d", len(reqLogs))
	}
	log := reqLogs[0]
	if log.AiErrNormalized != nil || log.AiErrNormalizeMiss != nil || log.AiUpstreamStatus != nil {
		t.Errorf("normalization fields must stay unset for local errors: %+v", log)
	}
}

// TestTC10 validates the access-log value matrix of the 810-815 fields
// across recognized / miss / stream-rewrite / truncation requests.
func TestTC10_AccessLogFieldMatrix(t *testing.T) {
	// non-stream rows: recognized 401, recognized 429, unrecognized HTML
	func() {
		e := newTestEnv(t, enabledConf(), nil)
		defer e.Close()

		e.errBk.ResponseHeaders = map[string]string{"Content-Type": "application/json"}
		e.errBk.ResponseFunc = func(r *http.Request, count int) (int, string) {
			// dispatch on the counter: the handler has consumed r.Body
			// before ResponseFunc runs and the requests are sequential
			switch count {
			case 1:
				return http.StatusUnauthorized,
					`{"error":{"message":"bad key","type":"authentication_error","code":"invalid_api_key"}}`
			case 2:
				return http.StatusTooManyRequests,
					`{"error":{"message":"slow","type":"rate_limit_error","code":"rate_limit_exceeded"}}`
			default:
				return http.StatusBadGateway, `<html>oops</html>`
			}
		}

		e.sendChat(apiKey, []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"auth"}]}`))
		e.sendChat(apiKey, []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"limit"}]}`))
		e.sendChat(apiKey, chatBody)

		reqLogs := e.stopAndLogs()
		if len(reqLogs) != 3 {
			t.Fatalf("expected 3 access logs, got %d", len(reqLogs))
		}
		// row 1: recognized 401
		if reqLogs[0].AiUpstreamStatus == nil || *reqLogs[0].AiUpstreamStatus != 401 ||
			reqLogs[0].AiErrNormalized == nil || !*reqLogs[0].AiErrNormalized ||
			reqLogs[0].AiErrNormalizeMiss != nil {
			t.Errorf("row1(401) fields wrong: status=%v normalized=%v miss=%v",
				reqLogs[0].AiUpstreamStatus, reqLogs[0].AiErrNormalized, reqLogs[0].AiErrNormalizeMiss)
		}
		// row 2: recognized 429
		if reqLogs[1].AiUpstreamStatus == nil || *reqLogs[1].AiUpstreamStatus != 429 ||
			reqLogs[1].AiErrNormalized == nil || !*reqLogs[1].AiErrNormalized {
			t.Errorf("row2(429) fields wrong: status=%v normalized=%v",
				reqLogs[1].AiUpstreamStatus, reqLogs[1].AiErrNormalized)
		}
		// row 3: unrecognized passthrough
		if reqLogs[2].AiUpstreamStatus == nil || *reqLogs[2].AiUpstreamStatus != 502 ||
			reqLogs[2].AiErrNormalizeMiss == nil || !*reqLogs[2].AiErrNormalizeMiss ||
			reqLogs[2].AiErrNormalized != nil {
			t.Errorf("row3(html) fields wrong: status=%v miss=%v normalized=%v",
				reqLogs[2].AiUpstreamStatus, reqLogs[2].AiErrNormalizeMiss, reqLogs[2].AiErrNormalized)
		}
	}()

	// stream row: error-rewrite stream
	func() {
		e := newTestEnv(t, streamConf(), nil)
		defer e.Close()

		e.errBk.SSEEvents = []string{
			`{"choices":[{"delta":{"content":"x"}}]}`,
			`{"error":{"message":"bad","type":"authentication_error"}}`,
			`[DONE]`,
		}

		streamBody := []byte(`{"model":"deepseek-chat","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
		e.sendChat(apiKey, streamBody)

		reqLogs := e.stopAndLogs()
		if len(reqLogs) != 1 {
			t.Fatalf("expected 1 access log, got %d", len(reqLogs))
		}
		log := reqLogs[0]
		if log.AiStreamErrorRewritten == nil || !*log.AiStreamErrorRewritten ||
			log.AiErrNormalized == nil || !*log.AiErrNormalized ||
			log.AiUpstreamStatus == nil || *log.AiUpstreamStatus != 200 ||
			log.AiUpstreamErrCode == nil || *log.AiUpstreamErrCode != "authentication_error" {
			t.Errorf("stream-rewrite row fields wrong: %+v", log)
		}
		if log.AiStreamTruncated != nil {
			t.Errorf("stream-rewrite row must not be truncated: %v", log.AiStreamTruncated)
		}
	}()

	// stream row: truncated stream
	func() {
		e := newTestEnv(t, streamConf(), nil)
		defer e.Close()

		e.errBk.SSEEvents = []string{
			`{"choices":[{"delta":{"content":"y"}}]}`,
		}

		streamBody := []byte(`{"model":"deepseek-chat","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
		e.sendChat(apiKey, streamBody)

		reqLogs := e.stopAndLogs()
		if len(reqLogs) != 1 {
			t.Fatalf("expected 1 access log, got %d", len(reqLogs))
		}
		log := reqLogs[0]
		if log.AiStreamTruncated == nil || !*log.AiStreamTruncated {
			t.Errorf("truncation row: ai_stream_truncated should be true, got %v", log.AiStreamTruncated)
		}
		if log.AiStreamErrorRewritten != nil || log.AiErrNormalized != nil {
			t.Errorf("truncation row: rewrite fields must stay unset: %+v", log)
		}
	}()
}
