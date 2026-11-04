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

// SC45 reasoning content normalization (AIConf.EnsureReasoningContent).
// Design docs:
//
//	tests/integration/测试设计文档/scenario-SC45-推理内容归一化/
//	docs/zh_cn/modifications/2026-11-03-ai-reasoning-content-normalization/design-changes.md
//	docs/zh_cn/sys_design/ai_reasoning_content_normalization.md
package sc45

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost = "front.example.org"

	chatPath       = "/v1/chat/completions"
	anthropicPath  = "/v1/messages"
	responsesPath  = "/v1/responses"
	filesPath      = "/v1/files"
	batchesPath    = "/v1/batches"
	monitorModule  = "proxy_state"
	contextHeader  = "x-ai-context-compression"
	injectedField  = `,"reasoning_content":""`
	thinkingBlock  = `{"type":"thinking","thinking":"","signature":""}`
	quotedThinking = `"type":"thinking"`

	apiKey = "ak_front"
	fbKey  = "ak_fb"

	clusterDS = "cluster_ds"
	clusterFB = "cluster_fb"

	upstreamKey   = "sk-upstream-secret"
	upstreamFbKey = "sk-fb-secret"
)

// windowBody has one assistant message outside the validation window
// (messages[1]) and one inside it (messages[3]); only the latter must be
// patched.
const windowBody = `{"model":"deepseek-chat","messages":[` +
	`{"role":"user","content":"q1"},` +
	`{"role":"assistant","content":"a1"},` +
	`{"role":"user","content":"q2"},` +
	`{"role":"assistant","content":"a2"},` +
	`{"role":"tool","content":"result","tool_call_id":"t1"}]}`

const chatResponse = `{"id":"chatcmpl-1","object":"chat.completion","model":"deepseek-chat",` +
	`"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],` +
	`"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`

const anthropicResponse = `{"id":"msg_1","type":"message","role":"assistant","model":"deepseek-chat",` +
	`"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn",` +
	`"usage":{"input_tokens":10,"output_tokens":2}}`

const upstream400 = `{"error":{"message":"The ` + "`reasoning_content`" + ` in the thinking mode must be passed back to the API.",` +
	`"type":"invalid_request_error","code":"invalid_request_error"}}`

// envOptions selects the per-test configuration variations.
type envOptions struct {
	// ensure is cluster_ds's EnsureReasoningContent (nil = not configured).
	ensure []string
	// ensureFB is cluster_fb's EnsureReasoningContent (nil = not configured).
	ensureFB []string
	// modelProtocols defaults to ["openai", "anthropic"].
	modelProtocols []string
	// accessibleBodySize rewrites Server.AccessibleBodySize when > 0.
	accessibleBodySize int
	// dsStatus/dsBody configure the primary mock upstream response.
	dsStatus int
	dsBody   string
	// fbStatus/fbBody configure the fallback mock upstream response.
	fbStatus int
	fbBody   string
	// contextCompress loads mod_ai_context with an always-triggering rule.
	contextCompress bool
}

type testEnv struct {
	t           *testing.T
	processEnv  *common.ProcessEnv
	dsBk        *common.MockBackend
	fbBk        *common.MockBackend
	confDir     string
	logDir      string
	bfePort     int
	monitorPort int
	stopBFE     func()
}

func (o *envOptions) withDefaults() {
	if o.modelProtocols == nil {
		o.modelProtocols = []string{"openai", "anthropic"}
	}
	if o.dsStatus == 0 {
		o.dsStatus = http.StatusOK
	}
	if o.dsBody == "" {
		o.dsBody = chatResponse
	}
	if o.fbStatus == 0 {
		o.fbStatus = http.StatusOK
	}
	if o.fbBody == "" {
		o.fbBody = chatResponse
	}
}

func aiConf(ensure []string, protocols []string, key string) *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type:                   0,
		Provider:               "mock-deepseek",
		ModelProtocols:         protocols,
		ProtocolPaths:          map[string]string{"openai": "/v1", "anthropic": "/anthropic"},
		EnsureReasoningContent: ensure,
		Keys:                   []cluster_conf.AIKey{{Name: "key-primary", Key: key, Weight: 100}},
		KeyPolicy:              &cluster_conf.AIKeyPolicy{Strategy: "weighted_random", MaxRetries: 0},
	}
}

func newTestEnv(t *testing.T, opt envOptions) *testEnv {
	t.Helper()
	opt.withDefaults()

	e := &testEnv{t: t}
	e.dsBk = common.NewMockBackend(clusterDS, opt.dsStatus, opt.dsBody)
	e.fbBk = common.NewMockBackend(clusterFB, opt.fbStatus, opt.fbBody)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends: map[string]*common.MockBackend{
			clusterDS: e.dsBk,
			clusterFB: e.fbBk,
		},
		AIConfs: map[string]*cluster_conf.AIConf{
			clusterDS: aiConf(opt.ensure, opt.modelProtocols, upstreamKey),
			clusterFB: aiConf(opt.ensureFB, opt.modelProtocols, upstreamFbKey),
		},
		TokenRuleData: tokenRule(),
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	if opt.accessibleBodySize > 0 {
		if err := rewriteAccessibleBodySize(filepath.Join(e.confDir, "bfe.conf"), opt.accessibleBodySize); err != nil {
			t.Fatalf("rewrite AccessibleBodySize failed: %v", err)
		}
	}
	if opt.contextCompress {
		if err := enableContextCompress(filepath.Join(e.confDir, "bfe.conf")); err != nil {
			t.Fatalf("enable mod_ai_context failed: %v", err)
		}
	}

	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
	t.Cleanup(e.close)
	return e
}

// rewriteAccessibleBodySize replaces the accessibleBodySize line of bfe.conf.
func rewriteAccessibleBodySize(path string, size int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "accessibleBodySize") {
			lines[i] = fmt.Sprintf("accessibleBodySize = %d", size)
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, fmt.Sprintf("accessibleBodySize = %d", size))
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// enableContextCompress adds mod_ai_context to the module list.
func enableContextCompress(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	marker := "Modules = mod_ai_route\n"
	if !strings.Contains(content, marker) {
		return fmt.Errorf("module list marker not found in bfe.conf")
	}
	content = strings.Replace(content, marker, marker+"Modules = mod_ai_context\n", 1)
	return os.WriteFile(path, []byte(content), 0644)
}

// tokenRule uses an unlimited quota plan so the scenario never fails on quota.
func tokenRule() *common.TokenRuleData {
	return &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": {
				{
					Id: "unlimited_plan", Unlimited: true, RedisKey: "quota:unlimited_plan",
					ExpiredTime: -1, Unit: "total_token",
				},
			},
		},
		Tokens: map[string]map[string]common.TokenFile{
			"ai_product": {
				apiKey: {Key: apiKey, KeyId: "front_key_id", Enabled: true, ExpiredTime: -1,
					QuotaPlans: []string{"unlimited_plan"}},
				fbKey: {Key: fbKey, KeyId: "fb_key_id", Enabled: true, ExpiredTime: -1,
					QuotaPlans: []string{"unlimited_plan"}},
			},
		},
		Config: map[string][]common.TokenRule{
			"ai_product": {
				{Cond: "default_t()", Action: common.ActionFile{Cmd: "CHECK_TOKEN"}},
			},
		},
	}
}

func (e *testEnv) close() {
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	if e.dsBk != nil {
		e.dsBk.Close()
	}
	if e.fbBk != nil {
		e.fbBk.Close()
	}
}

func (e *testEnv) url(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, path)
}

// post sends a request through BFE with an OpenAI-style credential header and
// returns the status code and body.
func (e *testEnv) post(key, path string, headers map[string]string, body []byte) (int, string) {
	return e.do(path, mergeHeader(map[string]string{"Authorization": "Bearer " + key}, headers), body)
}

// postXAPIKey sends a request through BFE with an Anthropic-style credential
// header only. Real anthropic clients send x-api-key without Authorization; a
// request carrying both is detected as openai (Authorization wins), which
// would exercise the wrong protocol branch.
func (e *testEnv) postXAPIKey(key, path string, headers map[string]string, body []byte) (int, string) {
	return e.do(path, mergeHeader(map[string]string{"x-api-key": key}, headers), body)
}

func mergeHeader(base, extra map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	return merged
}

func (e *testEnv) do(path string, headers map[string]string, body []byte) (int, string) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.url(path), bytes.NewReader(body))
	if err != nil {
		e.t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// route to the product declared in host_rule.data while connecting to the
	// loopback listener
	req.Host = apiHost
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		e.t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		e.t.Logf("request %s -> %d body=%s", path, resp.StatusCode, string(respBody))
		if logData, err := os.ReadFile(filepath.Join(e.logDir, "bfe.log")); err == nil {
			lines := strings.Split(string(logData), "\n")
			start := 0
			if len(lines) > 20 {
				start = len(lines) - 20
			}
			e.t.Logf("bfe log tail:\n%s", strings.Join(lines[start:], "\n"))
		}
	}
	return resp.StatusCode, string(respBody)
}

func (e *testEnv) postChat(body []byte) (int, string) {
	return e.post(apiKey, chatPath, nil, body)
}

// lastUpstreamBody returns the request body recorded by the given mock.
func lastUpstreamBody(t *testing.T, bk *common.MockBackend) []byte {
	t.Helper()
	bodies := bk.RequestBodies()
	if len(bodies) == 0 {
		t.Fatalf("mock upstream %s received no request", bk.ClusterName)
	}
	return bodies[len(bodies)-1]
}

// onlyInserted checks the byte-level invariant: removing the injected
// fragments from got must reproduce want exactly, and they must appear
// wantCount times.
func onlyInserted(t *testing.T, got []byte, want, injected string, wantCount int) {
	t.Helper()
	if n := strings.Count(string(got), injected); n != wantCount {
		t.Fatalf("%q occurs %d times, want %d; body: %s", injected, n, wantCount, string(got))
	}
	if stripped := strings.ReplaceAll(string(got), injected, ""); stripped != want {
		t.Fatalf("bytes other than the injected field changed:\n got: %s\nwant: %s", stripped, want)
	}
}

// metric reads a proxy_state counter.
func (e *testEnv) metric(field string) float64 {
	e.t.Helper()
	v, err := common.ModuleMetric(e.monitorPort, monitorModule, field)
	if err != nil {
		e.t.Fatalf("read metric %s failed: %v", field, err)
	}
	return v
}

// waitMetric waits until the counter reaches want.
func (e *testEnv) waitMetric(field string, want float64) {
	e.t.Helper()
	got := common.WaitForModuleMetricValue(e.monitorPort, monitorModule, field, want, 3*time.Second)
	if got != want {
		e.t.Fatalf("%s = %v, want %v", field, got, want)
	}
}

// TestTC01_DefaultOffBaseline verifies that a cluster without the field
// passes the body through byte for byte.
func TestTC01_DefaultOffBaseline(t *testing.T) {
	e := newTestEnv(t, envOptions{})

	status, _ := e.postChat([]byte(windowBody))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	if got := string(lastUpstreamBody(t, e.dsBk)); got != windowBody {
		t.Fatalf("body changed with the switch off:\n got: %s\nwant: %s", got, windowBody)
	}
	for _, m := range []string{"ReasoningPatched", "ReasoningSkip", "ReasoningFail", "ReasoningStill400"} {
		if v := e.metric(m); v != 0 {
			t.Fatalf("%s = %v, want 0", m, v)
		}
	}
}

// TestTC02_WindowPatch verifies that only the assistant message after the
// last user message is patched, with no other byte touched.
func TestTC02_WindowPatch(t *testing.T) {
	e := newTestEnv(t, envOptions{ensure: []string{"openai"}})

	status, _ := e.postChat([]byte(windowBody))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	got := lastUpstreamBody(t, e.dsBk)
	if v := gjson.GetBytes(got, "messages.1.reasoning_content"); v.Exists() {
		t.Fatalf("historical assistant must not be patched, got %s", v.Raw)
	}
	if v := gjson.GetBytes(got, "messages.3.reasoning_content"); !v.Exists() || v.String() != "" {
		t.Fatalf("window assistant must carry an empty reasoning_content, got %s", v.Raw)
	}
	onlyInserted(t, got, windowBody, injectedField, 1)
	e.waitMetric("ReasoningPatched", 1)
	if v := e.metric("ReasoningFail"); v != 0 {
		t.Fatalf("ReasoningFail = %v, want 0", v)
	}
}

// TestTC03_ExistingValueAndNull verifies that a non-empty value is kept and
// a null is normalized to an empty string.
func TestTC03_ExistingValueAndNull(t *testing.T) {
	e := newTestEnv(t, envOptions{ensure: []string{"openai"}})

	withValue := `{"model":"deepseek-chat","messages":[{"role":"user","content":"q"},` +
		`{"role":"assistant","reasoning_content":"real thinking","content":"a"}]}`
	if status, _ := e.postChat([]byte(withValue)); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got := string(lastUpstreamBody(t, e.dsBk)); got != withValue {
		t.Fatalf("existing value must be kept:\n got: %s\nwant: %s", got, withValue)
	}

	withNull := `{"model":"deepseek-chat","messages":[{"role":"user","content":"q"},` +
		`{"role":"assistant","reasoning_content":null,"content":"a"}]}`
	if status, _ := e.postChat([]byte(withNull)); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	got := string(lastUpstreamBody(t, e.dsBk))
	want := strings.Replace(withNull, `"reasoning_content":null`, `"reasoning_content":""`, 1)
	if got != want {
		t.Fatalf("null must be normalized in place:\n got: %s\nwant: %s", got, want)
	}

	e.waitMetric("ReasoningPatched", 1)
}

// TestTC04_ProtocolGate verifies that an anthropic request is not patched
// when only openai is enabled.
func TestTC04_ProtocolGate(t *testing.T) {
	const anthropicBody = `{"model":"deepseek-chat","max_tokens":64,"messages":[` +
		`{"role":"user","content":"q"},{"role":"assistant","content":[{"type":"text","text":"a"}]}]}`

	t.Run("anthropic not listed", func(t *testing.T) {
		e := newTestEnv(t, envOptions{ensure: []string{"openai"}})
		status, _ := e.postXAPIKey(apiKey, anthropicPath,
			map[string]string{"x-api-key": apiKey, "anthropic-version": "2023-06-01"}, []byte(anthropicBody))
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if got := string(lastUpstreamBody(t, e.dsBk)); got != anthropicBody {
			t.Fatalf("anthropic request must not be patched:\n got: %s\nwant: %s", got, anthropicBody)
		}
		if v := e.metric("ReasoningPatched"); v != 0 {
			t.Fatalf("ReasoningPatched = %v, want 0", v)
		}
	})

	t.Run("anthropic listed", func(t *testing.T) {
		e := newTestEnv(t, envOptions{ensure: []string{"openai", "anthropic"}, dsBody: anthropicResponse})
		status, _ := e.postXAPIKey(apiKey, anthropicPath,
			map[string]string{"x-api-key": apiKey, "anthropic-version": "2023-06-01"}, []byte(anthropicBody))
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		got := lastUpstreamBody(t, e.dsBk)
		if !strings.Contains(string(got), thinkingBlock) {
			t.Fatalf("thinking block not injected: %s", string(got))
		}
		e.waitMetric("ReasoningPatched", 1)
	})
}

// TestTC05_AnthropicBranch verifies the injection position and the
// "existing thinking block is kept" rule.
func TestTC05_AnthropicBranch(t *testing.T) {
	e := newTestEnv(t, envOptions{
		ensure: []string{"openai", "anthropic"},
		dsBody: anthropicResponse,
	})

	noThinking := `{"model":"deepseek-chat","max_tokens":64,"messages":[{"role":"user","content":"q"},` +
		`{"role":"assistant","content":[{"type":"text","text":"a"}]}]}`
	status, _ := e.postXAPIKey(apiKey, anthropicPath,
		map[string]string{"x-api-key": apiKey, "anthropic-version": "2023-06-01"}, []byte(noThinking))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	got := lastUpstreamBody(t, e.dsBk)
	if !gjson.GetBytes(got, "messages.1.content.0.type").Exists() ||
		gjson.GetBytes(got, "messages.1.content.0.type").String() != "thinking" {
		t.Fatalf("thinking block must be prepended at content[0]: %s", string(got))
	}
	if v := gjson.GetBytes(got, "messages.1.content.1.type"); v.String() != "text" {
		t.Fatalf("original content part must shift to content[1], got %s", v.Raw)
	}
	if !strings.Contains(string(got), `{"type":"text","text":"a"}`) {
		t.Fatalf("original content part bytes changed: %s", string(got))
	}

	withThinking := `{"model":"deepseek-chat","max_tokens":64,"messages":[{"role":"user","content":"q"},` +
		`{"role":"assistant","content":[{"type":"thinking","thinking":"real","signature":"sig"},{"type":"text","text":"a"}]}]}`
	if status, _ := e.postXAPIKey(apiKey, anthropicPath,
		map[string]string{"x-api-key": apiKey, "anthropic-version": "2023-06-01"}, []byte(withThinking)); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got := string(lastUpstreamBody(t, e.dsBk)); got != withThinking {
		t.Fatalf("existing thinking block must be kept:\n got: %s\nwant: %s", got, withThinking)
	}
}

// TestTC06_EncodingAndContentTypeGates verifies that a compressed body and a
// non-JSON content type are passed through untouched.
func TestTC06_EncodingAndContentTypeGates(t *testing.T) {
	e := newTestEnv(t, envOptions{ensure: []string{"openai"}})

	var compressed bytes.Buffer
	gw := gzip.NewWriter(&compressed)
	if _, err := gw.Write([]byte(windowBody)); err != nil {
		t.Fatalf("gzip failed: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	status, _ := e.post(apiKey, chatPath, map[string]string{"Content-Encoding": "gzip"}, compressed.Bytes())
	if status != http.StatusOK {
		t.Fatalf("gzip arm status = %d, want 200", status)
	}
	if got := lastUpstreamBody(t, e.dsBk); !bytes.Equal(got, compressed.Bytes()) {
		t.Fatalf("compressed body must be forwarded byte for byte, got %d bytes, want %d",
			len(got), compressed.Len())
	}

	status, _ = e.post(apiKey, chatPath,
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, []byte(windowBody))
	if status != http.StatusOK {
		t.Fatalf("non-json arm status = %d, want 200", status)
	}
	if got := string(lastUpstreamBody(t, e.dsBk)); got != windowBody {
		t.Fatalf("non-json body must not be patched:\n got: %s\nwant: %s", got, windowBody)
	}

	e.waitMetric("ReasoningSkip", 2)
	if v := e.metric("ReasoningPatched"); v != 0 {
		t.Fatalf("ReasoningPatched = %v, want 0", v)
	}
}

// TestTC07_FallbackIdempotent verifies that the patched body is not rewritten
// again when a fallback attempt reuses it.
func TestTC07_FallbackIdempotent(t *testing.T) {
	e := newTestEnv(t, envOptions{
		ensure:   []string{"openai"},
		ensureFB: []string{"openai"},
		dsStatus: http.StatusInternalServerError,
		dsBody:   `{"error":{"message":"boom"}}`,
	})

	status, _ := e.post(fbKey, chatPath, nil, []byte(windowBody))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (fallback must succeed)", status)
	}
	if e.dsBk.Hits() != 1 || e.fbBk.Hits() != 1 {
		t.Fatalf("expected one hit per cluster, ds=%d fb=%d", e.dsBk.Hits(), e.fbBk.Hits())
	}

	dsBody := lastUpstreamBody(t, e.dsBk)
	fbBody := lastUpstreamBody(t, e.fbBk)
	if n := strings.Count(string(dsBody), `"reasoning_content"`); n != 1 {
		t.Fatalf("primary attempt body has %d reasoning_content occurrences, want 1: %s", n, string(dsBody))
	}
	if string(dsBody) != string(fbBody) {
		t.Fatalf("fallback attempt must reuse the patched body unchanged:\n ds: %s\n fb: %s",
			string(dsBody), string(fbBody))
	}
	if v := e.metric("ReasoningPatched"); v != 1 {
		t.Fatalf("ReasoningPatched = %v, want 1 (patched once, not per attempt)", v)
	}
}

// TestTC08_ContractReversalSignal verifies ReasoningStill400 fires when a
// patched request still gets 400, even though the fallback saves the client.
func TestTC08_ContractReversalSignal(t *testing.T) {
	t.Run("signal on patched 400", func(t *testing.T) {
		e := newTestEnv(t, envOptions{
			ensure:   []string{"openai"},
			ensureFB: []string{"openai"},
			dsStatus: http.StatusBadRequest,
			dsBody:   upstream400,
		})
		status, _ := e.post(fbKey, chatPath, nil, []byte(windowBody))
		if status != http.StatusOK {
			t.Fatalf("client status = %d, want 200 (fallback saved it)", status)
		}
		e.waitMetric("ReasoningStill400", 1)
		if v := e.metric("ReasoningPatched"); v != 1 {
			t.Fatalf("ReasoningPatched = %v, want 1", v)
		}
	})

	t.Run("no signal when the switch is off", func(t *testing.T) {
		e := newTestEnv(t, envOptions{
			dsStatus: http.StatusBadRequest,
			dsBody:   upstream400,
		})
		if status, _ := e.postChat([]byte(windowBody)); status != http.StatusBadRequest {
			t.Fatalf("client status = %d, want 400", status)
		}
		if v := e.metric("ReasoningStill400"); v != 0 {
			t.Fatalf("ReasoningStill400 = %v, want 0 (nothing was patched)", v)
		}
	})
}

// TestTC09_AccessibleBodySize verifies the oversized-body gate.
func TestTC09_AccessibleBodySize(t *testing.T) {
	e := newTestEnv(t, envOptions{ensure: []string{"openai"}, accessibleBodySize: 4096})

	// ~8KB body: larger than the configured limit
	big := `{"model":"deepseek-chat","messages":[{"role":"user","content":"` +
		strings.Repeat("x", 8000) + `"},{"role":"assistant","content":"a"}]}`
	if status, _ := e.postChat([]byte(big)); status != http.StatusOK {
		t.Fatalf("oversized arm status = %d, want 200", status)
	}
	if got := string(lastUpstreamBody(t, e.dsBk)); got != big {
		t.Fatalf("oversized body must be forwarded unchanged (len got %d, want %d)", len(got), len(big))
	}
	e.waitMetric("ReasoningSkip", 1)

	// control arm: same instance, small body still patched
	if status, _ := e.postChat([]byte(windowBody)); status != http.StatusOK {
		t.Fatalf("small arm status = %d, want 200", status)
	}
	if got := lastUpstreamBody(t, e.dsBk); !strings.Contains(string(got), `"reasoning_content":""`) {
		t.Fatalf("small body must still be patched: %s", string(got))
	}
	e.waitMetric("ReasoningPatched", 1)
}

// TestTC10_ContextCompressCoexistence verifies the ordering constraint: the
// patch survives mod_ai_context's whole-body re-marshal.
func TestTC10_ContextCompressCoexistence(t *testing.T) {
	e := newTestEnv(t, envOptions{ensure: []string{"openai"}, contextCompress: true})

	body := `{"model":"deepseek-chat","messages":[` +
		`{"role":"user","content":"q1"},` +
		`{"role":"assistant","content":"` + strings.Repeat("a", 2000) + `"},` +
		`{"role":"user","content":"q2"},` +
		`{"role":"assistant","content":"a2"}]}`

	req, err := http.NewRequest(http.MethodPost, e.url(chatPath), strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Host = apiHost
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.Header.Get(contextHeader) == "" {
		t.Fatalf("compression did not run (missing %s header)", contextHeader)
	}

	got := lastUpstreamBody(t, e.dsBk)
	if v := gjson.GetBytes(got, "messages.3.reasoning_content"); !v.Exists() || v.String() != "" {
		t.Fatalf("patch must survive the compression re-marshal, body: %s", string(got))
	}
	e.waitMetric("ReasoningPatched", 1)
}

// TestTC11_ResponsesPassthrough verifies that /responses is never patched.
func TestTC11_ResponsesPassthrough(t *testing.T) {
	e := newTestEnv(t, envOptions{ensure: []string{"openai"}})

	const body = `{"model":"deepseek-chat","input":[{"role":"user","content":"q"},` +
		`{"type":"reasoning","summary":[{"type":"summary_text","text":"prev"}]},` +
		`{"role":"assistant","content":[{"type":"output_text","text":"a"}]}]}`
	if status, _ := e.post(apiKey, responsesPath, nil, []byte(body)); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got := string(lastUpstreamBody(t, e.dsBk)); got != body {
		t.Fatalf("/responses must be forwarded unchanged:\n got: %s\nwant: %s", got, body)
	}
	for _, m := range []string{"ReasoningPatched", "ReasoningSkip"} {
		if v := e.metric(m); v != 0 {
			t.Fatalf("%s = %v, want 0", m, v)
		}
	}
}

// TestTC12_BatchFilePassthrough verifies that file uploads and batch creation
// are neither buffered nor rewritten.
func TestTC12_BatchFilePassthrough(t *testing.T) {
	e := newTestEnv(t, envOptions{ensure: []string{"openai"}})

	boundary := "SC45BOUNDARY"
	multipart := "--" + boundary + "\r\n" +
		`Content-Disposition: form-data; name="purpose"` + "\r\n\r\nbatch\r\n" +
		"--" + boundary + "\r\n" +
		`Content-Disposition: form-data; name="file"; filename="b.jsonl"` + "\r\n" +
		"Content-Type: application/jsonl\r\n\r\n" +
		`{"custom_id":"1","method":"POST","url":"/v1/chat/completions","body":{"messages":[{"role":"user","content":"q"},{"role":"assistant","content":"a"}]}}` + "\n" +
		"--" + boundary + "--\r\n"

	if status, _ := e.post(apiKey, filesPath,
		map[string]string{"Content-Type": "multipart/form-data; boundary=" + boundary}, []byte(multipart)); status != http.StatusOK {
		t.Fatalf("file arm status = %d, want 200", status)
	}
	if got := string(lastUpstreamBody(t, e.dsBk)); got != multipart {
		t.Fatalf("multipart body must be forwarded unchanged")
	}

	const batchBody = `{"input_file_id":"file-1","endpoint":"/v1/chat/completions","completion_window":"24h"}`
	if status, _ := e.post(apiKey, batchesPath, nil, []byte(batchBody)); status != http.StatusOK {
		t.Fatalf("batch arm status = %d, want 200", status)
	}
	if got := string(lastUpstreamBody(t, e.dsBk)); got != batchBody {
		t.Fatalf("batch body must be forwarded unchanged:\n got: %s\nwant: %s", got, batchBody)
	}

	e.waitMetric("ReasoningSkip", 2)
	if v := e.metric("ReasoningPatched"); v != 0 {
		t.Fatalf("ReasoningPatched = %v, want 0", v)
	}
}

// TestTC13_Metrics verifies the counters on their own branches.
func TestTC13_Metrics(t *testing.T) {
	e := newTestEnv(t, envOptions{ensure: []string{"openai"}})

	// A1/A2: patched twice
	for i := 0; i < 2; i++ {
		if status, _ := e.postChat([]byte(windowBody)); status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
	}
	e.waitMetric("ReasoningPatched", 2)

	// B1: existing value -> no counter moves
	withValue := `{"model":"deepseek-chat","messages":[{"role":"user","content":"q"},` +
		`{"role":"assistant","reasoning_content":"real","content":"a"}]}`
	if status, _ := e.postChat([]byte(withValue)); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if v := e.metric("ReasoningPatched"); v != 2 {
		t.Fatalf("ReasoningPatched = %v, want 2 (unchanged)", v)
	}

	// C1: skip
	var compressed bytes.Buffer
	gw := gzip.NewWriter(&compressed)
	_, _ = gw.Write([]byte(windowBody))
	_ = gw.Close()
	if status, _ := e.post(apiKey, chatPath, map[string]string{"Content-Encoding": "gzip"}, compressed.Bytes()); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	e.waitMetric("ReasoningSkip", 1)

	// C2: fail-open on a malformed body
	truncated := `{"messages":[{"role":"user","content":"q"},{"role":"assistant"}`
	if status, _ := e.postChat([]byte(truncated)); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	e.waitMetric("ReasoningFail", 1)
	if v := e.metric("ReasoningStill400"); v != 0 {
		t.Fatalf("ReasoningStill400 = %v, want 0", v)
	}
}

// TestTC14_ConfigValidation verifies that an unsupported value is rejected at
// load time and that a failed reload keeps the previous configuration.
func TestTC14_ConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		ensure    []string
		protocols []string
		wantErr   string
	}{
		{
			name:      "gemini is not applicable",
			ensure:    []string{"gemini"},
			protocols: []string{"openai", "gemini"},
			wantErr:   "not supported",
		},
		{
			name:      "protocol not declared by the cluster",
			ensure:    []string{"anthropic"},
			protocols: []string{"openai"},
			wantErr:   "not in ModelProtocols",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processEnv := common.NewProcessEnv(t)
			processEnv.Build()
			confDir := filepath.Join(processEnv.WorkDir(), "conf")
			logDir := filepath.Join(processEnv.WorkDir(), "log")

			dsBk := common.NewMockBackend(clusterDS, http.StatusOK, chatResponse)
			defer dsBk.Close()

			builder := &common.BFEConfigBuilder{
				TemplateDir:   "testdata",
				TargetConfDir: confDir,
				Backends:      map[string]*common.MockBackend{clusterDS: dsBk},
				AIConfs: map[string]*cluster_conf.AIConf{
					clusterDS: aiConf(tt.ensure, tt.protocols, upstreamKey),
				},
				TokenRuleData: tokenRule(),
			}
			if err := builder.Build(); err != nil {
				t.Fatalf("build bfe config failed: %v", err)
			}

			_, _, stop, err := processEnv.StartBFERaw(confDir, logDir)
			if err == nil {
				if stop != nil {
					stop()
				}
				t.Fatalf("bfe started with an invalid config, want a load error containing %q", tt.wantErr)
			}
		})
	}

	t.Run("failed reload keeps the previous configuration", func(t *testing.T) {
		e := newTestEnv(t, envOptions{ensure: []string{"openai"}})

		// baseline: patch works
		if status, _ := e.postChat([]byte(windowBody)); status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if got := lastUpstreamBody(t, e.dsBk); !strings.Contains(string(got), `"reasoning_content":""`) {
			t.Fatalf("baseline patch missing: %s", string(got))
		}

		// break the config and reload
		confPath := filepath.Join(e.confDir, "cluster_conf", "cluster_conf.data")
		data, err := os.ReadFile(confPath)
		if err != nil {
			t.Fatalf("read cluster_conf.data failed: %v", err)
		}
		var conf map[string]interface{}
		if err := json.Unmarshal(data, &conf); err != nil {
			t.Fatalf("parse cluster_conf.data failed: %v", err)
		}
		configs, ok := conf["Config"].(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected cluster_conf.data shape: %s", string(data))
		}
		dsConf, ok := configs[clusterDS].(map[string]interface{})
		if !ok {
			t.Fatalf("cluster %s missing from cluster_conf.data: %s", clusterDS, string(data))
		}
		aiConf, ok := dsConf["AIConf"].(map[string]interface{})
		if !ok {
			t.Fatalf("AIConf missing from cluster %s: %s", clusterDS, string(data))
		}
		aiConf["EnsureReasoningContent"] = []string{"gemini"}
		broken, err := json.MarshalIndent(conf, "", "    ")
		if err != nil {
			t.Fatalf("marshal cluster_conf.data failed: %v", err)
		}
		if err := os.WriteFile(confPath, broken, 0644); err != nil {
			t.Fatalf("write cluster_conf.data failed: %v", err)
		}

		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/reload/server_data_conf", e.monitorPort))
		if err != nil {
			t.Fatalf("reload request failed: %v", err)
		}
		reloadBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		// the reload endpoint reports a failed load as HTTP 200 with an error
		// body, so the payload is what distinguishes success from failure
		if !strings.Contains(string(reloadBody), "EnsureReasoningContent") {
			t.Fatalf("reload must report the invalid config, body: %s", string(reloadBody))
		}

		// old configuration is still effective
		if status, _ := e.postChat([]byte(windowBody)); status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if got := lastUpstreamBody(t, e.dsBk); !strings.Contains(string(got), `"reasoning_content":""`) {
			t.Fatalf("previous configuration must stay effective after a failed reload: %s", string(got))
		}
	})
}
