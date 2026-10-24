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

// SC24: AI context compression and trimming (mod_ai_context).
//
// Verifies the request-side OpenAI messages compression pipeline end to end
// against a real bfe process: trim layers (tool results / thinking blocks /
// inline images), the P2 rule rewrite with the fidelity gate, the proactive
// trigger threshold, per-mode semantics (conservative/balanced/aggressive),
// tool_call pairing repair, streaming requests, hot reload of the rule data,
// per-request idempotency across cluster fallback attempts, coexistence with
// mod_ai_cache (hit path never compresses) and fail-open pass-through of
// malformed bodies. See
// docs/zh_cn/modifications/2026-10-01-ai-context-compress/design-changes.md
// 13.2.
package sc24

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bfe_access_pb "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
	"github.com/bfenetworks/bfe/bfe_modules/mod_ai_context"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost     = "ctx.example.org"
	apiPath     = "/v1/chat/completions"
	apiKey      = "ak_ctx"
	apiKeyCache = "ak_cache"

	// TC06: one product per mode, distinguished by host (host_rule.data)
	hostCons = "ctx-cons.example.org"
	hostBal  = "ctx-bal.example.org"
	hostAgg  = "ctx-agg.example.org"

	clusterPrimary  = "cluster_ctx_primary"
	clusterFallback = "cluster_ctx_fallback"
	clusterCache    = "cluster_cache"

	annotationHeader = "x-ai-context-compression"
	truncatedMarker  = "...[truncated]"
	rewriteMarker    = "[COMPRESSED:rewrite]"
	imagePlaceholder = "[Earlier image removed to fit context window]"

	// context rule tuning (testdata/mod_ai_context/context_rule.data):
	// maxContextTokens=200, reserveTokens=50 => budget 150;
	// triggerRatio=0.8 => proactive trigger at 120 estimated tokens.
	ruleCharsPerToken      = 4
	ruleImageTokenEstimate = 100
	triggerTokens          = 120
	budgetTokens           = 150
)

// backendAnswer is the mock upstream answer for a non-streaming request.
const backendAnswer = `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`

// inlineImageURL is a tiny inline base64 image (data URI).
const inlineImageURL = "data:image/png;base64,QUJD"

type testEnv struct {
	t           *testing.T
	processEnv  *common.ProcessEnv
	backends    map[string]*common.MockBackend
	redis       *common.RedisServer
	confDir     string
	logDir      string
	bfePort     int
	monitorPort int
	stopBFE     func()
}

// newTestEnvBuilder prepares backends, config builder output and dirs, but
// does not start bfe yet (mutateConf can customize the conf directory first).
func newTestEnvBuilder(t *testing.T, withCache bool) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	e.backends[clusterPrimary] = common.NewMockBackend(clusterPrimary, http.StatusOK, backendAnswer)
	e.backends[clusterFallback] = common.NewMockBackend(clusterFallback, http.StatusOK, backendAnswer)
	e.backends[clusterCache] = common.NewMockBackend(clusterCache, http.StatusOK, backendAnswer)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends:      e.backends,
	}
	if withCache {
		e.redis = common.NewRedisServer(t)
		builder.RedisAddr = e.redis.Addr()
		builder.AiCacheRuleData = &common.AiCacheRuleData{
			Version: "1.0",
			Config: map[string][]common.AiCacheRule{
				"ai_product": {
					{
						Cond:             "default_t()",
						CacheKeyStrategy: "lastQuestion",
						CacheTTL:         3600,
						MaxBodyBytes:     1048576,
						MaxValueBytes:    1048576,
					},
				},
			},
		}
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	if withCache {
		if err := e.enableModule("mod_ai_cache"); err != nil {
			t.Fatalf("enable mod_ai_cache failed: %v", err)
		}
	}
	return e
}

func (e *testEnv) start() {
	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
}

// newTestEnv starts a bfe with the three SC24 clusters. withCache enables
// mod_ai_cache (TC04) which requires the embedded redis.
func newTestEnv(t *testing.T, withCache bool) *testEnv {
	e := newTestEnvBuilder(t, withCache)
	e.start()
	return e
}

// newTestEnvWithConf starts a bfe after applying mutate to the generated conf
// directory (e.g. overwriting context_rule.data before the module loads it).
func newTestEnvWithConf(t *testing.T, withCache bool, mutate func(confDir string)) *testEnv {
	e := newTestEnvBuilder(t, withCache)
	if mutate != nil {
		mutate(e.confDir)
	}
	e.start()
	return e
}

// enableModule appends a Modules line to bfe.conf after the config builder
// ran. Module Init order is governed by the fixed moduleList registration
// order (mod_ai_cache before mod_ai_context), so appending is safe.
func (e *testEnv) enableModule(modName string) error {
	path := filepath.Join(e.confDir, "bfe.conf")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if !strings.Contains(content, "Modules = "+modName) {
		content += "\nModules = " + modName + "\n"
	}
	return os.WriteFile(path, []byte(content), 0644)
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
}

// stopAndParseLogs stops bfe (flushes the pb access log) and returns all log
// records.
func (e *testEnv) stopAndParseLogs() []*bfe_access_pb.RequestLog {
	e.t.Helper()
	time.Sleep(500 * time.Millisecond)
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	reqLogs, err := common.ParseAccessLogAfterStop(e.logDir)
	if err != nil {
		e.t.Fatalf("parse access log failed: %v", err)
	}
	return reqLogs
}

func (e *testEnv) sendRequest(apikey string, body []byte) (*http.Response, string, error) {
	return e.sendRequestToHost(apiHost, apikey, body)
}

func (e *testEnv) sendRequestToHost(host, apikey string, body []byte) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = host
	req.Header.Set("Authorization", "Bearer "+apikey)
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

func (e *testEnv) expectOK(resp *http.Response, body string, err error, step ...string) {
	label := "request"
	if len(step) > 0 {
		label = step[0]
	}
	if err != nil {
		e.logBFEException()
		e.t.Fatalf("%s: request failed: %v", label, err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		e.t.Fatalf("%s: expected 200, got %d, body: %s", label, resp.StatusCode, body)
	}
}

// contextRulePath returns the mod_ai_context rule file inside the generated
// conf directory.
func (e *testEnv) contextRulePath() string {
	return filepath.Join(e.confDir, "mod_ai_context", "context_rule.data")
}

// writeContextRule overwrites the mod_ai_context rule file (hot reload via
// the monitor endpoint makes bfe pick it up without a restart).
func (e *testEnv) writeContextRule(rule interface{}) error {
	return common.WriteJSONFile(e.contextRulePath(), rule)
}

// reloadContextRule calls the web monitor reload endpoint of mod_ai_context.
func (e *testEnv) reloadContextRule(t *testing.T) {
	t.Helper()
	reloadURL := fmt.Sprintf("http://127.0.0.1:%d/reload/mod_ai_context?path=%s",
		e.monitorPort, url.QueryEscape(e.contextRulePath()))
	resp, err := http.Get(reloadURL)
	if err != nil {
		t.Fatalf("call reload mod_ai_context failed: %v", err)
	}
	defer resp.Body.Close()
	reloadBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reload should return 200, got %d, body: %s", resp.StatusCode, reloadBody)
	}
	if !strings.Contains(string(reloadBody), "context_rule.data=1.0") {
		t.Fatalf("reload response should contain context_rule.data=1.0, got: %s", reloadBody)
	}
}

// ---------- message construction and estimation ----------

type chatMessage struct {
	Role             string      `json:"role"`
	Content          interface{} `json:"content,omitempty"`
	ReasoningContent string      `json:"reasoning_content,omitempty"`
	ToolCallID       string      `json:"tool_call_id,omitempty"`
	ToolCalls        []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls,omitempty"`
}

// buildBody marshals a chat completions request body (non-streaming).
func buildBody(model string, messages []chatMessage) []byte {
	return marshalBody(model, messages, false)
}

// buildStreamBody marshals a streaming chat completions request body.
func buildStreamBody(model string, messages []chatMessage) []byte {
	return marshalBody(model, messages, true)
}

func marshalBody(model string, messages []chatMessage, stream bool) []byte {
	body, err := json.Marshal(map[string]interface{}{
		"model":    model,
		"stream":   stream,
		"messages": messages,
	})
	if err != nil {
		panic(err)
	}
	return body
}

func strContent(s string) interface{} { return s }

// imageContentPart builds a typed-parts inline image element.
func imageContentPart() interface{} {
	return map[string]interface{}{
		"type":      "image_url",
		"image_url": map[string]interface{}{"url": inlineImageURL},
	}
}

// textContentPart builds a typed-parts text element.
func textContentPart(s string) interface{} {
	return map[string]interface{}{"type": "text", "text": s}
}

func toolCallAssistant(id, name string) chatMessage {
	var m chatMessage
	m.Role = "assistant"
	m.ToolCalls = []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	}{{ID: id, Type: "function"}}
	m.ToolCalls[0].Function.Name = name
	m.ToolCalls[0].Function.Arguments = "{}"
	return m
}

func toolResult(id, content string) chatMessage {
	return chatMessage{Role: "tool", ToolCallID: id, Content: strContent(content)}
}

// estimateBody computes the token estimate with the module's own estimator
// and the rule coefficients, i.e. exactly what bfe computes at runtime.
func estimateBody(t *testing.T, body []byte) int64 {
	t.Helper()
	var req struct {
		Messages []mod_ai_context.Message `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("estimateBody: parse body failed: %v", err)
	}
	est := mod_ai_context.NewHeuristicEstimator()
	return est.EstimateMessages(req.Messages, mod_ai_context.EstimateParams{
		CharsPerToken:      ruleCharsPerToken,
		ImageTokenEstimate: ruleImageTokenEstimate,
	})
}

// padToEstimate grows the filler of the given message until the body estimate
// equals target exactly (adding 4 chars adds exactly 1 token with
// charsPerToken=4).
func padToEstimate(t *testing.T, target int64, base []chatMessage, padRoleIdx int) []byte {
	t.Helper()
	filler := ""
	for i := 0; i < 100000; i++ {
		msgs := make([]chatMessage, len(base))
		copy(msgs, base)
		pad := strings.Repeat("x", len(filler))
		if s, ok := msgs[padRoleIdx].Content.(string); ok {
			msgs[padRoleIdx].Content = s + pad
		} else {
			msgs[padRoleIdx].Content = pad
		}
		body := buildBody("gpt-test", msgs)
		if got := estimateBody(t, body); got == target {
			return body
		}
		filler += "xxxx"
	}
	t.Fatalf("padToEstimate: could not reach estimate %d", target)
	return nil
}

// simulateToolTrim returns a copy of the body where every tool message
// content is replaced by the L1 truncation product (toolResultMaxChars=32
// plus the marker), so the post-L1 estimate can be computed in-test.
func simulateToolTrim(t *testing.T, body []byte) []byte {
	t.Helper()
	var req struct {
		Model    string                   `json:"model"`
		Stream   bool                     `json:"stream"`
		Messages []map[string]interface{} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("simulateToolTrim: parse failed: %v", err)
	}
	for _, m := range req.Messages {
		if m["role"] != "tool" {
			continue
		}
		if s, ok := m["content"].(string); ok && len([]rune(s)) > 32 {
			runes := []rune(s)
			m["content"] = string(runes[:32]) + truncatedMarker
		}
	}
	out, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("simulateToolTrim: marshal failed: %v", err)
	}
	return out
}

// findLogByStatus returns the first access log record with the given
// ai_context_compress_status.
func findLogByStatus(logs []*bfe_access_pb.RequestLog, status string) *bfe_access_pb.RequestLog {
	for _, l := range logs {
		if l.GetAiContextCompressStatus() == status {
			return l
		}
	}
	return nil
}

// parseBackendMessages extracts the messages array of a backend-received body.
func parseBackendMessages(t *testing.T, body []byte) []map[string]interface{} {
	t.Helper()
	var req struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("backend body is not valid JSON: %v\nbody: %s", err, truncateForLog(body))
	}
	return req.Messages
}

func truncateForLog(b []byte) string {
	s := string(b)
	if len(s) > 400 {
		return s[:400] + "...(truncated)"
	}
	return s
}

// countInlineImages counts inline data-URI image parts of a typed-parts
// message.
func countInlineImages(m map[string]interface{}) int {
	parts, _ := m["content"].([]interface{})
	n := 0
	for _, p := range parts {
		part, _ := p.(map[string]interface{})
		if part["type"] != "image_url" {
			continue
		}
		iu, _ := part["image_url"].(map[string]interface{})
		if strings.HasPrefix(fmt.Sprintf("%v", iu["url"]), "data:image") {
			n++
		}
	}
	return n
}

// countImagePlaceholders counts the L1.5 placeholder text parts.
func countImagePlaceholders(m map[string]interface{}) int {
	parts, _ := m["content"].([]interface{})
	n := 0
	for _, p := range parts {
		part, _ := p.(map[string]interface{})
		if part["type"] == "text" && part["text"] == imagePlaceholder {
			n++
		}
	}
	return n
}

// partsAllTyped asserts every content part of the message carries a type.
func partsAllTyped(t *testing.T, m map[string]interface{}) {
	t.Helper()
	parts, ok := m["content"].([]interface{})
	if !ok {
		t.Fatalf("message content is not a parts array: %v", m)
	}
	if len(parts) == 0 {
		t.Fatalf("message content parts must not be empty: %v", m)
	}
	for _, p := range parts {
		part, ok := p.(map[string]interface{})
		if !ok || part["type"] == nil || part["type"] == "" {
			t.Fatalf("content part without type field: %v", p)
		}
	}
}

// assertToolPairsLegal checks the OpenAI tool pairing rules the module's
// repair enforces: every tool message answers an earlier assistant
// tool_call, and every assistant tool_call is answered by a later tool
// message.
func assertToolPairsLegal(t *testing.T, msgs []map[string]interface{}) {
	t.Helper()
	declared := make(map[string]int) // tool_call id -> declaring message index
	for i, m := range msgs {
		if m["role"] != "assistant" {
			continue
		}
		tcs, _ := m["tool_calls"].([]interface{})
		for _, tc := range tcs {
			c, _ := tc.(map[string]interface{})
			id, _ := c["id"].(string)
			if id != "" {
				declared[id] = i
			}
		}
	}
	answered := make(map[string]bool)
	for j, m := range msgs {
		if m["role"] != "tool" {
			continue
		}
		id, _ := m["tool_call_id"].(string)
		declIdx, ok := declared[id]
		if !ok || declIdx >= j {
			t.Fatalf("orphan tool message at %d (tool_call_id=%q)", j, id)
		}
		answered[id] = true
	}
	for id, idx := range declared {
		found := false
		for j := idx + 1; j < len(msgs); j++ {
			if msgs[j]["role"] == "tool" && msgs[j]["tool_call_id"] == id {
				found = true
			}
		}
		if !found || !answered[id] {
			t.Fatalf("dangling tool_call %q declared at %d", id, idx)
		}
	}
}

// compressionAnnotation is the parsed x-ai-context-compression header value.
type compressionAnnotation struct {
	before int64
	after  int64
	mode   string
}

// parseAnnotation parses "tokens=<before>-><after>; mode=<mode>".
func parseAnnotation(h string) (compressionAnnotation, bool) {
	var a compressionAnnotation
	if h == "" {
		return a, false
	}
	n, err := fmt.Sscanf(h, "tokens=%d->%d; mode=%s", &a.before, &a.after, &a.mode)
	if err != nil || n != 3 {
		return compressionAnnotation{}, false
	}
	return a, true
}

// mustAnnotation asserts the exact annotation header value against the
// estimates computed by the module's own estimator.
func mustAnnotation(t *testing.T, resp *http.Response, reqBody, backendBody []byte, mode string) compressionAnnotation {
	t.Helper()
	h := resp.Header.Get(annotationHeader)
	a, ok := parseAnnotation(h)
	if !ok {
		t.Fatalf("invalid %s header: %q", annotationHeader, h)
	}
	if want := estimateBody(t, reqBody); a.before != want {
		t.Fatalf("annotation before mismatch: got %d, want %d (header %q)", a.before, want, h)
	}
	if want := estimateBody(t, backendBody); a.after != want {
		t.Fatalf("annotation after mismatch: got %d, want %d (header %q)", a.after, want, h)
	}
	if a.mode != mode {
		t.Fatalf("annotation mode mismatch: got %q, want %q", a.mode, mode)
	}
	return a
}

// mustSameMessage asserts that message idx survived compression byte-for-byte
// (semantic JSON equality of the re-serialized message object).
func mustSameMessage(t *testing.T, wantBody []byte, gotMsg map[string]interface{}, idx int) {
	t.Helper()
	wantMsgs := parseBackendMessages(t, wantBody)
	if idx >= len(wantMsgs) {
		t.Fatalf("message index %d out of range", idx)
	}
	wantJSON, err := json.Marshal(wantMsgs[idx])
	if err != nil {
		t.Fatalf("marshal want message failed: %v", err)
	}
	gotJSON, err := json.Marshal(gotMsg)
	if err != nil {
		t.Fatalf("marshal got message failed: %v", err)
	}
	if !bytes.Equal(wantJSON, gotJSON) {
		t.Fatalf("message %d changed:\nwant: %s\ngot:  %s", idx, wantJSON, gotJSON)
	}
}

// smallBudgetRule builds a context rule with one product entry.
func smallBudgetRule(product, mode string) map[string]interface{} {
	return map[string]interface{}{
		"Version": "1.0",
		"Defaults": map[string]interface{}{
			"triggerRatio":       0.8,
			"keepLatestImages":   1,
			"toolResultMaxChars": 32,
			"thinkingPolicy":     "trim-all-but-last",
			"charsPerToken":      ruleCharsPerToken,
			"imageTokenEstimate": ruleImageTokenEstimate,
			"rewrite": map[string]interface{}{
				"strength":              "lite",
				"protectedSurvivalRate": 0.95,
			},
		},
		"Config": map[string]interface{}{
			product: []map[string]interface{}{
				{
					"cond":             "default_t()",
					"mode":             mode,
					"maxContextTokens": 200,
					"reserveTokens":    50,
				},
			},
		},
	}
}

// ---------- shared oversized bodies ----------

// trimBody is an oversized conversation with a long tool result and an old
// thinking block: L1 truncates the tool result, L2 removes the old thinking,
// and the last user question stays untouched.
func trimBody() []byte {
	return buildBody("gpt-test", []chatMessage{
		{Role: "system", Content: strContent("sys prompt")},
		{Role: "user", Content: strContent("earlier question")},
		{Role: "assistant", ReasoningContent: strings.Repeat("thought ", 60), Content: strContent("earlier answer")},
		toolCallAssistant("call_1", "get_data"),
		toolResult("call_1", strings.Repeat("x", 2000)),
		{Role: "user", Content: strContent("current question")},
	})
}

// rewriteBody is a conversation the trim layers cannot help (no tool
// results, no images, no thinking): only the P2 rule rewrite removes the
// redundant phrases. One unit contains a URL that must survive untouched.
func rewriteBody() []byte {
	unit := "Please note that the system is stable. "
	urlUnit := "Please note that docs live at https://bfe.example.com/v1.2/spec today. "
	filler := strings.Repeat(unit, 14) + urlUnit
	return buildBody("gpt-test", []chatMessage{
		{Role: "system", Content: strContent("sys prompt")},
		{Role: "user", Content: strContent("hi")},
		{Role: "assistant", Content: strContent(filler)},
		{Role: "user", Content: strContent("current question two")},
	})
}

// modeBody mixes a long tool result with rewritable filler so that L1 alone
// does not reach the budget and the mode decides whether P2 runs.
func modeBody() []byte {
	unit := "Please note that the system is stable. "
	return buildBody("gpt-test", []chatMessage{
		{Role: "system", Content: strContent("sys prompt")},
		toolCallAssistant("call_1", "get_data"),
		toolResult("call_1", strings.Repeat("x", 2000)),
		{Role: "assistant", Content: strContent(strings.Repeat(unit, 14))},
		{Role: "user", Content: strContent("current question")},
	})
}

// ---------- TC01: trim path + rewrite path ----------

func TestTC01_TrimAndRewrite(t *testing.T) {
	e := newTestEnv(t, false)
	defer e.Close()

	// phase 1: trim path
	trimReq := trimBody()
	resp, body, err := e.sendRequest(apiKey, trimReq)
	e.expectOK(resp, body, err, "trim request")

	backendBodies := e.backends[clusterPrimary].RequestBodies()
	if len(backendBodies) != 1 {
		t.Fatalf("expected 1 backend call, got %d", len(backendBodies))
	}
	mustAnnotation(t, resp, trimReq, backendBodies[0], "balanced")

	msgs := parseBackendMessages(t, backendBodies[0])
	if len(msgs) != 6 {
		t.Fatalf("expected 6 messages at backend, got %d", len(msgs))
	}
	toolContent, _ := msgs[4]["content"].(string)
	if !strings.HasSuffix(toolContent, truncatedMarker) || strings.Contains(toolContent, strings.Repeat("x", 100)) {
		t.Fatalf("tool result should be truncated with marker, got %q", toolContent)
	}
	if _, hasThinking := msgs[2]["reasoning_content"]; hasThinking {
		t.Fatalf("old assistant thinking should be removed, got %v", msgs[2])
	}
	if msgs[2]["content"] != "earlier answer" {
		t.Fatalf("assistant content must stay intact, got %v", msgs[2]["content"])
	}
	toolCalls, _ := msgs[3]["tool_calls"].([]interface{})
	if len(toolCalls) != 1 {
		t.Fatalf("tool_calls must survive trimming, got %v", msgs[3])
	}
	if msgs[4]["tool_call_id"] != "call_1" {
		t.Fatalf("tool message must stay paired with the assistant tool_call, got %v", msgs[4])
	}
	// system and last user message byte-identical
	mustSameMessage(t, trimReq, msgs[0], 0)
	mustSameMessage(t, trimReq, msgs[5], 5)
	// forwarding integrity: declared Content-Length (when present) must match
	// the received body length, and the body must be valid JSON (no truncation)
	if cl := e.backends[clusterPrimary].HeaderCopies()[0].Get("Content-Length"); cl != "" {
		var n int
		if _, err := fmt.Sscanf(cl, "%d", &n); err != nil || n != len(backendBodies[0]) {
			t.Fatalf("Content-Length %q does not match received body length %d", cl, len(backendBodies[0]))
		}
	}

	// phase 2: rewrite path (P2)
	rewriteReq := rewriteBody()
	if est := estimateBody(t, rewriteReq); est <= budgetTokens {
		t.Fatalf("precondition: rewrite body estimate must exceed budget 150, got %d", est)
	}
	resp, body, err = e.sendRequest(apiKey, rewriteReq)
	e.expectOK(resp, body, err, "rewrite request")

	backendBodies = e.backends[clusterPrimary].RequestBodies()
	if len(backendBodies) != 2 {
		t.Fatalf("expected 2 backend calls, got %d", len(backendBodies))
	}
	mustAnnotation(t, resp, rewriteReq, backendBodies[1], "balanced")

	rewriteMsgs := parseBackendMessages(t, backendBodies[1])
	asstContent, _ := rewriteMsgs[2]["content"].(string)
	if strings.Contains(asstContent, "Please note that") {
		t.Fatalf("redundant phrases should be rewritten, got: %.120q", asstContent)
	}
	if !strings.Contains(asstContent, rewriteMarker) {
		t.Fatalf("rewritten message must carry the %s marker, got: %.120q", rewriteMarker, asstContent)
	}
	if strings.Count(asstContent, rewriteMarker) != 1 {
		t.Fatalf("marker must appear exactly once, got %d", strings.Count(asstContent, rewriteMarker))
	}
	if !strings.Contains(asstContent, "https://bfe.example.com/v1.2/spec") {
		t.Fatalf("protected URL must survive the rewrite, got: %.160q", asstContent)
	}
	// system and last user message byte-identical
	mustSameMessage(t, rewriteReq, rewriteMsgs[0], 0)
	mustSameMessage(t, rewriteReq, rewriteMsgs[3], 3)

	// access log: 793-796 fields for both requests
	logs := e.stopAndParseLogs()
	trimLog := findLogByStatus(logs, "trim")
	if trimLog == nil {
		t.Fatalf("no trim record in access log: %v", logs)
	}
	if trimLog.GetAiContextCompressMode() != "balanced" {
		t.Fatalf("expected mode balanced, got %q", trimLog.GetAiContextCompressMode())
	}
	if trimLog.GetAiContextTokensBefore() <= trimLog.GetAiContextTokensAfter() ||
		trimLog.GetAiContextTokensAfter() <= 0 {
		t.Fatalf("unexpected token counts: before=%d after=%d",
			trimLog.GetAiContextTokensBefore(), trimLog.GetAiContextTokensAfter())
	}
	rewriteLog := findLogByStatus(logs, "rewrite")
	if rewriteLog == nil {
		t.Fatalf("no rewrite record in access log: %v", logs)
	}
	if rewriteLog.GetAiContextTokensBefore() <= rewriteLog.GetAiContextTokensAfter() {
		t.Fatalf("rewrite must reduce the estimate: before=%d after=%d",
			rewriteLog.GetAiContextTokensBefore(), rewriteLog.GetAiContextTokensAfter())
	}
}

// ---------- TC02: trigger threshold boundary ----------

func TestTC02_TriggerThresholdBoundary(t *testing.T) {
	e := newTestEnv(t, false)
	defer e.Close()

	// below the trigger line (120): estimate exactly 119 => no rewrite at all
	skipBase := []chatMessage{
		{Role: "system", Content: strContent("sys")},
		{Role: "user", Content: strContent("q")},
	}
	skipBody := padToEstimate(t, triggerTokens-1, skipBase, 1)
	if est := estimateBody(t, skipBody); est != triggerTokens-1 {
		t.Fatalf("precondition: skip body estimate must be %d, got %d", triggerTokens-1, est)
	}

	resp, body, err := e.sendRequest(apiKey, skipBody)
	e.expectOK(resp, body, err, "below-threshold request")
	if h := resp.Header.Get(annotationHeader); h != "" {
		t.Fatalf("below-threshold request must not carry %s, got %q", annotationHeader, h)
	}
	backendBodies := e.backends[clusterPrimary].RequestBodies()
	if len(backendBodies) != 1 || !bytes.Equal(backendBodies[0], skipBody) {
		t.Fatalf("below-threshold request must be forwarded byte-identical, got %s", truncateForLog(backendBodies[0]))
	}

	// above the trigger line: estimate exactly 121 => pipeline runs (L1 marks
	// the tool result)
	triggerBase := []chatMessage{
		{Role: "user", Content: strContent("q")},
		toolCallAssistant("call_9", "get_data"),
		toolResult("call_9", ""),
	}
	triggerBody := padToEstimate(t, triggerTokens+1, triggerBase, 2)
	if est := estimateBody(t, triggerBody); est != triggerTokens+1 {
		t.Fatalf("precondition: trigger body estimate must be %d, got %d", triggerTokens+1, est)
	}

	resp, body, err = e.sendRequest(apiKey, triggerBody)
	e.expectOK(resp, body, err, "above-threshold request")
	if h := resp.Header.Get(annotationHeader); !strings.Contains(h, "tokens=") {
		t.Fatalf("above-threshold request must carry %s, got %q", annotationHeader, h)
	}
	backendBodies = e.backends[clusterPrimary].RequestBodies()
	if len(backendBodies) != 2 {
		t.Fatalf("expected 2 backend calls, got %d", len(backendBodies))
	}
	if !bytes.Contains(backendBodies[1], []byte(truncatedMarker)) {
		t.Fatalf("above-threshold request must be compressed, backend got: %s", truncateForLog(backendBodies[1]))
	}

	// access log: skip_under_threshold vs trim
	logs := e.stopAndParseLogs()
	skipLog := findLogByStatus(logs, "skip_under_threshold")
	if skipLog == nil {
		t.Fatalf("no skip_under_threshold record in access log")
	}
	if skipLog.AiContextCompressMode != nil || skipLog.AiContextTokensBefore != nil {
		t.Fatalf("skip record must not carry mode/tokens: %v", skipLog)
	}
	if findLogByStatus(logs, "trim") == nil {
		t.Fatalf("no trim record in access log")
	}
}

// ---------- TC03: fallback idempotency ----------

func TestTC03_FallbackIdempotent(t *testing.T) {
	e := newTestEnv(t, false)
	defer e.Close()

	// primary fails so the request falls back to the second cluster attempt
	e.backends[clusterPrimary].Response = http.StatusServiceUnavailable
	e.backends[clusterPrimary].Body = `{"error":"primary down"}`

	body := trimBody()
	resp, respBody, err := e.sendRequest(apiKey, body)
	e.expectOK(resp, respBody, err, "fallback request")
	if h := resp.Header.Get(annotationHeader); !strings.Contains(h, "mode=balanced") {
		t.Fatalf("expected %s annotation, got %q", annotationHeader, h)
	}

	if e.backends[clusterPrimary].Hits() != 1 {
		t.Fatalf("expected 1 hit on primary, got %d", e.backends[clusterPrimary].Hits())
	}
	if e.backends[clusterFallback].Hits() != 1 {
		t.Fatalf("expected 1 hit on fallback, got %d", e.backends[clusterFallback].Hits())
	}

	// first attempt: compressed body forwarded to the primary
	primaryBodies := e.backends[clusterPrimary].RequestBodies()
	if len(primaryBodies) != 1 || !bytes.Contains(primaryBodies[0], []byte(truncatedMarker)) {
		t.Fatalf("primary should receive the compressed body, got: %s", truncateForLog(primaryBodies[0]))
	}

	// fallback attempt: the retried OutRequest replays the shared body
	// buffer (compressed once); the idempotency guard must skip a second
	// compression pass, so the fallback receives byte-identical content with
	// exactly one truncation marker
	fallbackBodies := e.backends[clusterFallback].RequestBodies()
	if len(fallbackBodies) != 1 {
		t.Fatalf("expected 1 fallback backend call, got %d", len(fallbackBodies))
	}
	if !bytes.Equal(fallbackBodies[0], primaryBodies[0]) {
		t.Fatalf("fallback must replay the once-compressed body\nfallback: %s\nprimary: %s",
			truncateForLog(fallbackBodies[0]), truncateForLog(primaryBodies[0]))
	}
	if bytes.Count(fallbackBodies[0], []byte(truncatedMarker)) != 1 {
		t.Fatalf("compression must run exactly once, markers: %d", bytes.Count(fallbackBodies[0], []byte(truncatedMarker)))
	}

	// one request, one log record: compressed exactly once
	logs := e.stopAndParseLogs()
	if len(logs) != 1 {
		t.Fatalf("expected exactly 1 access log record, got %d", len(logs))
	}
	if logs[0].GetAiContextCompressStatus() != "trim" {
		t.Fatalf("expected trim status, got %q", logs[0].GetAiContextCompressStatus())
	}
	if logs[0].GetAiContextTokensBefore() <= logs[0].GetAiContextTokensAfter() {
		t.Fatalf("compression must reduce the estimate: before=%d after=%d",
			logs[0].GetAiContextTokensBefore(), logs[0].GetAiContextTokensAfter())
	}
}

// ---------- TC04: cache hit path never compresses ----------

func TestTC04_CacheCoexistence(t *testing.T) {
	e := newTestEnv(t, true)
	defer e.Close()

	// a compressible body whose last user question is the cache key material;
	// compression must not touch the last user message, so the exact-match
	// key stays identical between the original and the compressed request
	body := buildBody("gpt-test", []chatMessage{
		toolCallAssistant("call_1", "get_data"),
		toolResult("call_1", strings.Repeat("x", 2000)),
		{Role: "user", Content: strContent("what is bfe?")},
	})

	// request A: cache miss, compressed upstream, written back to redis
	resp, rb, err := e.sendRequest(apiKeyCache, body)
	e.expectOK(resp, rb, err, "request A")
	if h := resp.Header.Get(annotationHeader); !strings.Contains(h, "mode=balanced") {
		t.Fatalf("request A must be compressed, annotation: %q", h)
	}
	if e.backends[clusterCache].Hits() != 1 {
		t.Fatalf("expected 1 upstream call, got %d", e.backends[clusterCache].Hits())
	}
	if !bytes.Contains(e.backends[clusterCache].RequestBodies()[0], []byte(truncatedMarker)) {
		t.Fatalf("request A body should be compressed at the backend")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		found := false
		for _, k := range e.redis.Keys() {
			if strings.HasPrefix(k, "ai_cache:") {
				found = true
			}
		}
		if found || !time.Now().Before(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(e.redis.Keys()) == 0 {
		t.Fatalf("cache entry should exist in redis")
	}

	// request B: identical request hits the cache; the hit path short-circuits
	// before HandleAfterAITargetModel, so compression never runs
	resp, rb, err = e.sendRequest(apiKeyCache, body)
	e.expectOK(resp, rb, err, "request B")
	if ct := resp.Header.Get("X-Bfe-Ai-Cache"); ct != "hit" {
		t.Fatalf("request B should be a cache hit, got %q", ct)
	}
	if h := resp.Header.Get(annotationHeader); h != "" {
		t.Fatalf("cache hit must not carry %s, got %q", annotationHeader, h)
	}
	if e.backends[clusterCache].Hits() != 1 {
		t.Fatalf("cache hit must not call upstream, got %d hits", e.backends[clusterCache].Hits())
	}

	logs := e.stopAndParseLogs()
	if len(logs) != 2 {
		t.Fatalf("expected 2 access log records, got %d", len(logs))
	}
	aLog, bLog := logs[0], logs[1]
	if aLog.GetAiContextCompressStatus() != "trim" {
		t.Fatalf("request A should be compressed (trim), got %q", aLog.GetAiContextCompressStatus())
	}
	if aLog.GetAiCacheStatus() != "miss" {
		t.Fatalf("request A should be a cache miss, got %q", aLog.GetAiCacheStatus())
	}
	if bLog.GetAiCacheStatus() != "hit" {
		t.Fatalf("request B should be a cache hit, got %q", bLog.GetAiCacheStatus())
	}
	if bLog.GetAiContextCompressStatus() != "" || bLog.AiContextTokensBefore != nil ||
		bLog.AiContextTokensAfter != nil || bLog.AiContextCompressMode != nil {
		t.Fatalf("cache hit path must show zero compression fields: status=%q before=%v",
			bLog.GetAiContextCompressStatus(), bLog.AiContextTokensBefore)
	}
}

// ---------- TC05: fail-open pass-through ----------

func TestTC05_FailOpenPassthrough(t *testing.T) {
	e := newTestEnv(t, false)
	defer e.Close()

	// malformed JSON: parse fails, original body forwarded untouched
	badJSON := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"},`)
	resp, body, err := e.sendRequest(apiKey, badJSON)
	e.expectOK(resp, body, err, "malformed json request")
	if h := resp.Header.Get(annotationHeader); h != "" {
		t.Fatalf("malformed request must not carry %s, got %q", annotationHeader, h)
	}

	// structurally unfixable messages (unknown role): repair rollback
	wizardBody := buildBody("gpt-test", []chatMessage{
		{Role: "wizard", Content: strContent(strings.Repeat("z", 600))},
		{Role: "user", Content: strContent("q")},
	})
	if est := estimateBody(t, wizardBody); est <= triggerTokens {
		t.Fatalf("precondition: wizard body estimate must exceed the trigger, got %d", est)
	}
	resp, body, err = e.sendRequest(apiKey, wizardBody)
	e.expectOK(resp, body, err, "unfixable messages request")
	if h := resp.Header.Get(annotationHeader); h != "" {
		t.Fatalf("rollback request must not carry %s, got %q", annotationHeader, h)
	}

	backendBodies := e.backends[clusterPrimary].RequestBodies()
	if len(backendBodies) != 2 {
		t.Fatalf("expected 2 backend calls, got %d", len(backendBodies))
	}
	if !bytes.Equal(backendBodies[0], badJSON) {
		t.Fatalf("malformed body must pass through byte-identical, got: %s", truncateForLog(backendBodies[0]))
	}
	if !bytes.Equal(backendBodies[1], wizardBody) {
		t.Fatalf("unfixable body must roll back to the byte-identical original, got: %s", truncateForLog(backendBodies[1]))
	}

	logs := e.stopAndParseLogs()
	if findLogByStatus(logs, "skip_parse_err") == nil {
		t.Fatalf("no skip_parse_err record in access log")
	}
	rollbackLog := findLogByStatus(logs, "repair_rollback")
	if rollbackLog == nil {
		t.Fatalf("no repair_rollback record in access log")
	}
	if rollbackLog.AiContextTokensBefore != nil || rollbackLog.AiContextTokensAfter != nil {
		t.Fatalf("rollback record must not carry after-tokens: %v", rollbackLog)
	}
}

// ---------- TC06: mode semantics ----------

// TestTC06 verifies the three mode tiers with one product per mode,
// distinguished by host (host_rule.data): conservative trims only, balanced
// adds the lite rewrite, aggressive runs the full-strength rewrite.
func TestTC06_ModeSemantics(t *testing.T) {
	rule := map[string]interface{}{
		"Version": "1.0",
		"Defaults": map[string]interface{}{
			"triggerRatio":       0.8,
			"keepLatestImages":   1,
			"toolResultMaxChars": 32,
			"thinkingPolicy":     "trim-all-but-last",
			"charsPerToken":      ruleCharsPerToken,
			"imageTokenEstimate": ruleImageTokenEstimate,
			"rewrite": map[string]interface{}{
				"strength":              "lite",
				"protectedSurvivalRate": 0.95,
			},
		},
		"Config": map[string]interface{}{
			"ai_product_cons": []map[string]interface{}{
				{"cond": "default_t()", "mode": "conservative", "maxContextTokens": 200, "reserveTokens": 50},
			},
			"ai_product_bal": []map[string]interface{}{
				{"cond": "default_t()", "mode": "balanced", "maxContextTokens": 200, "reserveTokens": 50},
			},
			"ai_product_agg": []map[string]interface{}{
				{"cond": "default_t()", "mode": "aggressive", "maxContextTokens": 200, "reserveTokens": 50},
			},
		},
	}
	e := newTestEnvWithConf(t, false, func(confDir string) {
		if err := common.WriteJSONFile(filepath.Join(confDir, "mod_ai_context", "context_rule.data"), rule); err != nil {
			t.Fatalf("overwrite context_rule.data failed: %v", err)
		}
	})
	defer e.Close()

	body := modeBody()
	// precondition: after L1 the estimate must still exceed the budget, so
	// the mode (not the trim layers) decides whether P2 runs
	if est := estimateBody(t, simulateToolTrim(t, body)); est <= budgetTokens {
		t.Fatalf("precondition: post-L1 estimate must exceed budget 150, got %d", est)
	}

	cases := []struct {
		host        string
		mode        string
		wantRewrite bool
	}{
		{hostCons, "conservative", false},
		{hostBal, "balanced", true},
		{hostAgg, "aggressive", true},
	}
	for _, c := range cases {
		resp, rb, err := e.sendRequestToHost(c.host, apiKey, body)
		e.expectOK(resp, rb, err, c.mode+" request")

		backendBodies := e.backends[clusterPrimary].RequestBodies()
		got := backendBodies[len(backendBodies)-1]
		mustAnnotation(t, resp, body, got, c.mode)

		if !bytes.Contains(got, []byte(truncatedMarker)) {
			t.Fatalf("%s: trim mark expected, backend got: %s", c.mode, truncateForLog(got))
		}
		if hasMarker := bytes.Contains(got, []byte(rewriteMarker)); hasMarker != c.wantRewrite {
			t.Fatalf("%s: rewrite mark presence = %v, want %v", c.mode, hasMarker, c.wantRewrite)
		}
		msgs := parseBackendMessages(t, got)
		assertToolPairsLegal(t, msgs)
	}

	logs := e.stopAndParseLogs()
	if len(logs) != 3 {
		t.Fatalf("expected 3 access log records, got %d", len(logs))
	}
	for _, c := range cases {
		var rec *bfe_access_pb.RequestLog
		for _, l := range logs {
			if l.GetAiContextCompressMode() == c.mode {
				rec = l
			}
		}
		if rec == nil {
			t.Fatalf("no access log record for mode %s", c.mode)
		}
		wantStatus := "trim"
		if c.wantRewrite {
			wantStatus = "rewrite"
		}
		if rec.GetAiContextCompressStatus() != wantStatus {
			t.Fatalf("mode %s: expected status %s, got %q", c.mode, wantStatus, rec.GetAiContextCompressStatus())
		}
		if rec.GetAiContextTokensBefore() <= rec.GetAiContextTokensAfter() {
			t.Fatalf("mode %s: before=%d after=%d", c.mode, rec.GetAiContextTokensBefore(), rec.GetAiContextTokensAfter())
		}
	}
}

// ---------- TC07: image trimming and multimodal fidelity ----------

// imageBody carries a historical user message with 4 inline images and a
// protected last user message with 1 inline image.
func imageBody() []byte {
	return buildBody("gpt-test", []chatMessage{
		{Role: "user", Content: []interface{}{imageContentPart(), imageContentPart(), imageContentPart(), imageContentPart(), textContentPart("look at these figures")}},
		{Role: "assistant", ReasoningContent: strings.Repeat("thought ", 75), Content: strContent("thinking answer")},
		{Role: "user", Content: []interface{}{imageContentPart(), textContentPart("current question")}},
	})
}

func imageRule(keepLatest int) map[string]interface{} {
	rule := smallBudgetRule("ai_product", "conservative")
	defaults := rule["Defaults"].(map[string]interface{})
	defaults["keepLatestImages"] = keepLatest
	return rule
}

func TestTC07_ImageTrimMultimodal(t *testing.T) {
	// case 1: keepLatestImages=2, conservative => pure L1/L1.5/L2 product
	e := newTestEnvWithConf(t, false, func(confDir string) {
		if err := common.WriteJSONFile(filepath.Join(confDir, "mod_ai_context", "context_rule.data"), imageRule(2)); err != nil {
			t.Fatalf("overwrite context_rule.data failed: %v", err)
		}
	})

	body := imageBody()
	if est := estimateBody(t, body); est <= triggerTokens {
		t.Fatalf("precondition: image body estimate must exceed the trigger, got %d", est)
	}
	resp, rb, err := e.sendRequest(apiKey, body)
	e.expectOK(resp, rb, err, "image trim request")
	mustAnnotation(t, resp, body, e.backends[clusterPrimary].RequestBodies()[0], "conservative")

	msgs := parseBackendMessages(t, e.backends[clusterPrimary].RequestBodies()[0])
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if n := countInlineImages(msgs[0]); n != 2 {
		t.Fatalf("historical message should keep 2 inline images, got %d", n)
	}
	if n := countImagePlaceholders(msgs[0]); n != 2 {
		t.Fatalf("historical message should have 2 placeholders, got %d", n)
	}
	partsAllTyped(t, msgs[0])
	if _, kept := msgs[0]["content"].([]interface{}); !kept || len(msgs[0]["content"].([]interface{})) != 5 {
		t.Fatalf("historical message parts unexpected: %v", msgs[0]["content"])
	}
	// msg1 is the only (thus last) assistant: L2 keeps its thinking block;
	// image trimming is what this case verifies
	if msgs[1]["content"] != "thinking answer" {
		t.Fatalf("assistant content must stay intact, got %v", msgs[1]["content"])
	}
	// the protected last user message: image untouched, no placeholder
	if n := countInlineImages(msgs[2]); n != 1 {
		t.Fatalf("protected last user message must keep its image, got %d", n)
	}
	if n := countImagePlaceholders(msgs[2]); n != 0 {
		t.Fatalf("protected last user message must not be trimmed, got %d placeholders", n)
	}
	partsAllTyped(t, msgs[2])
	mustSameMessage(t, body, msgs[2], 2)

	logs := e.stopAndParseLogs()
	if rec := findLogByStatus(logs, "trim"); rec == nil || rec.GetAiContextCompressMode() != "conservative" {
		t.Fatalf("expected trim record with mode conservative, got %v", rec)
	}
	e.Close()

	// case 2: keepLatestImages=0 disables image trimming (design 4.2)
	e0 := newTestEnvWithConf(t, false, func(confDir string) {
		if err := common.WriteJSONFile(filepath.Join(confDir, "mod_ai_context", "context_rule.data"), imageRule(0)); err != nil {
			t.Fatalf("overwrite context_rule.data failed: %v", err)
		}
	})
	defer e0.Close()

	resp, rb, err = e0.sendRequest(apiKey, body)
	e0.expectOK(resp, rb, err, "keep=0 request")
	msgs = parseBackendMessages(t, e0.backends[clusterPrimary].RequestBodies()[0])
	if n := countInlineImages(msgs[0]); n != 4 {
		t.Fatalf("keepLatestImages=0 must not trim images, got %d", n)
	}
	if n := countImagePlaceholders(msgs[0]); n != 0 {
		t.Fatalf("keepLatestImages=0 must not create placeholders, got %d", n)
	}
	// image trimming disabled: every inline image survives byte-identically
	mustSameMessage(t, body, msgs[0], 0)
	if _, ok := parseAnnotation(resp.Header.Get(annotationHeader)); !ok {
		t.Fatalf("keep=0 request should still be annotated (thinking trimmed), got %q",
			resp.Header.Get(annotationHeader))
	}
}

// ---------- TC08: tool_call pairing repair (fixable path) ----------

func TestTC08_ToolPairRepair(t *testing.T) {
	e := newTestEnv(t, false)
	defer e.Close()

	// case 1: orphan tool message (no assistant declares its tool_call_id);
	// repair drops it and the request is still forwarded
	orphanBody := buildBody("gpt-test", []chatMessage{
		{Role: "user", Content: strContent("q")},
		toolCallAssistant("call_1", "get_data"),
		toolResult("call_1", "r1"),
		toolResult("orphan", strings.Repeat("z", 600)),
		{Role: "user", Content: strContent("current question")},
	})
	resp, rb, err := e.sendRequest(apiKey, orphanBody)
	e.expectOK(resp, rb, err, "orphan tool request")
	backendBodies := e.backends[clusterPrimary].RequestBodies()
	msgs := parseBackendMessages(t, backendBodies[0])
	if len(msgs) != 4 {
		t.Fatalf("orphan tool message should be repaired away, got %d messages", len(msgs))
	}
	assertToolPairsLegal(t, msgs)
	if bytes.Contains(backendBodies[0], []byte(strings.Repeat("z", 100))) {
		t.Fatalf("orphan tool content must be removed, got: %s", truncateForLog(backendBodies[0]))
	}

	// case 2: dangling tool_call (assistant declares call_1 and call_2 but
	// only call_1 is answered); repair strips the dangling call
	danglingBody := buildBody("gpt-test", []chatMessage{
		{Role: "user", Content: strContent("q")},
		func() chatMessage {
			m := toolCallAssistant("call_1", "get_data")
			m.Content = strContent(strings.Repeat("y", 600))
			m.ToolCalls = append(m.ToolCalls, m.ToolCalls[0])
			m.ToolCalls[1].ID = "call_2"
			return m
		}(),
		toolResult("call_1", "r1"),
		{Role: "user", Content: strContent("current question")},
	})
	resp, rb, err = e.sendRequest(apiKey, danglingBody)
	e.expectOK(resp, rb, err, "dangling tool_call request")
	backendBodies = e.backends[clusterPrimary].RequestBodies()
	msgs = parseBackendMessages(t, backendBodies[1])
	assertToolPairsLegal(t, msgs)
	tcs, _ := msgs[1]["tool_calls"].([]interface{})
	if len(tcs) != 1 {
		t.Fatalf("dangling tool_call should be stripped, got %d calls", len(tcs))
	}
	if c, _ := tcs[0].(map[string]interface{}); c["id"] != "call_1" {
		t.Fatalf("answered call_1 must survive, got %v", c)
	}

	// both requests forwarded (NOT rolled back): the exact exit stage depends
	// on the post-trim estimate (trim or rewrite), never repair_rollback
	logs := e.stopAndParseLogs()
	if len(logs) != 2 {
		t.Fatalf("expected 2 access log records, got %d", len(logs))
	}
	for i, l := range logs {
		status := l.GetAiContextCompressStatus()
		if status != "trim" && status != "rewrite" {
			t.Fatalf("record %d: repair must succeed (trim/rewrite), got %q", i, status)
		}
	}
	if findLogByStatus(logs, "repair_rollback") != nil {
		t.Fatalf("no repair_rollback expected on the fixable path")
	}
}

// ---------- TC09: streaming request compression ----------

func TestTC09_StreamRequest(t *testing.T) {
	e := newTestEnv(t, false)
	defer e.Close()

	e.backends[clusterPrimary].SSEEvents = []string{
		`{"choices":[{"index":0,"delta":{"content":"BFE is"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":" a gateway"}}]}`,
		"[DONE]",
	}

	streamReq := buildStreamBody("gpt-test", []chatMessage{
		toolCallAssistant("call_1", "get_data"),
		toolResult("call_1", strings.Repeat("x", 2000)),
		{Role: "user", Content: strContent("current question")},
	})
	resp, body, err := e.sendRequest(apiKey, streamReq)
	e.expectOK(resp, body, err, "stream request")
	mustAnnotation(t, resp, streamReq, e.backends[clusterPrimary].RequestBodies()[0], "balanced")

	if !bytes.Contains(e.backends[clusterPrimary].RequestBodies()[0], []byte(truncatedMarker)) {
		t.Fatalf("backend should receive the compressed stream request body")
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", ct)
	}
	if !strings.Contains(body, "BFE is") || !strings.Contains(body, " a gateway") || !strings.Contains(body, "[DONE]") {
		t.Fatalf("SSE response should pass through complete, got: %s", truncateForLog([]byte(body)))
	}

	logs := e.stopAndParseLogs()
	if rec := findLogByStatus(logs, "trim"); rec == nil {
		t.Fatalf("expected trim record for the stream request")
	}
}

// ---------- TC10: rule hot reload ----------

func TestTC10_RuleHotReload(t *testing.T) {
	// start with compression switched off for the product
	offRule := map[string]interface{}{
		"Version": "1.0",
		"Defaults": map[string]interface{}{
			"triggerRatio":       0.8,
			"keepLatestImages":   1,
			"toolResultMaxChars": 32,
			"thinkingPolicy":     "trim-all-but-last",
			"charsPerToken":      ruleCharsPerToken,
			"imageTokenEstimate": ruleImageTokenEstimate,
			"rewrite": map[string]interface{}{
				"strength":              "lite",
				"protectedSurvivalRate": 0.95,
			},
		},
		"Config": map[string]interface{}{
			"ai_product": []map[string]interface{}{
				{"cond": "default_t()", "mode": "off"},
			},
		},
	}
	e := newTestEnvWithConf(t, false, func(confDir string) {
		if err := common.WriteJSONFile(filepath.Join(confDir, "mod_ai_context", "context_rule.data"), offRule); err != nil {
			t.Fatalf("overwrite context_rule.data failed: %v", err)
		}
	})
	defer e.Close()

	// rule off: byte-identical pass-through
	body := trimBody()
	resp, rb, err := e.sendRequest(apiKey, body)
	e.expectOK(resp, rb, err, "off-mode request")
	if h := resp.Header.Get(annotationHeader); h != "" {
		t.Fatalf("off mode must not annotate, got %q", h)
	}
	if got := e.backends[clusterPrimary].RequestBodies()[0]; !bytes.Equal(got, body) {
		t.Fatalf("off mode must forward byte-identical, got: %s", truncateForLog(got))
	}

	// hot reload to balanced with a small budget; no restart
	if err := e.writeContextRule(smallBudgetRule("ai_product", "balanced")); err != nil {
		t.Fatalf("rewrite context_rule.data failed: %v", err)
	}
	e.reloadContextRule(t)

	// the same request shape is now compressed
	resp, rb, err = e.sendRequest(apiKey, body)
	e.expectOK(resp, rb, err, "post-reload request")
	mustAnnotation(t, resp, body, e.backends[clusterPrimary].RequestBodies()[1], "balanced")
	if got := e.backends[clusterPrimary].RequestBodies()[1]; !bytes.Contains(got, []byte(truncatedMarker)) {
		t.Fatalf("post-reload request must be compressed, got: %s", truncateForLog(got))
	}

	logs := e.stopAndParseLogs()
	if findLogByStatus(logs, "skip_no_rule") == nil {
		t.Fatalf("expected skip_no_rule record for the off-mode request")
	}
	if findLogByStatus(logs, "trim") == nil {
		t.Fatalf("expected trim record after reload")
	}
}
