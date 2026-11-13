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

package sc29

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"

	bfe_access_pb "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost  = "usage-billing.example.org"
	apiKey   = "ak_user_a"
	apiKeyId = "user_a_key_id"

	clusterUsageSnapshot = "cluster_usage_snapshot"

	planRMB     = "plan_rmb"
	redisKeyRMB = "quota:plan_rmb"

	initialQuota = int64(10000000000)
)

var (
	openAIStreamBody = []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	// Explicit opt-out: BFE must not override it and the backend (honoring
	// the flag) sends no usage chunk, leaving the estimate fallback.
	openAIStreamBodyOptOut = []byte(`{"model":"gpt-4","stream":true,"stream_options":{"include_usage":false},"messages":[{"role":"user","content":"hi"}]}`)

	openAIContentChunk = `{"id":"chatcmpl-1","choices":[{"delta":{"content":"hello"}}]}`
	openAIUsageChunk   = `{"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`
	// Issue #1497: an early usage riding a non-terminal event type. The type
	// is outside the final-usage whitelist, so the values must feed the
	// protocol context only and never become the billing basis.
	openAIEarlyUsageChunk = `{"id":"chatcmpl-1","type":"response.output_text.delta","choices":[{"delta":{"content":" world"}}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`

	// expectedActualCost: prompt 10 * 300 + completion 20 * 900 = 21000
	// fixed-point units (input 0.000003 yuan/token, output 0.000009).
	expectedActualCost = int64(10*300 + 20*900)

	// Large trailing frame used to force a client write error (ECONNRESET)
	// after the client has aborted the connection (same trick as SC12).
	openAILargeChunk = fmt.Sprintf(`{"id":"chatcmpl-1","choices":[{"delta":{"content":%q}}]}`,
		strings.Repeat("x", 1<<20))

	// Issue #1415: an Anthropic stream whose upstream ends at a clean HTTP
	// EOF without message_stop and without any usage event — the production
	// bucket-A shape. Data payloads are kept in separate variables so the
	// per-event estimate math in the tests stays exact.
	anthropicMessagesBody = []byte(`{"model":"claude-3","stream":true,"messages":[{"role":"user","content":"hi"}],"max_tokens":100}`)

	anthropicMessageStartData = `{"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-3-sonnet"}}`
	anthropicDeltaData        = `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`
	anthropicBlockStopData    = `{"type":"content_block_stop","index":0}`
)

type testEnv struct {
	t              *testing.T
	processEnv     *common.ProcessEnv
	backend        *common.MockBackend
	redis          *common.RedisServer
	bfePort        int
	bfeMonitorPort int
	logDir         string
	stopBFE        func()
}

func newTestEnv(t *testing.T) *testEnv {
	return newTestEnvWithEstimateToken(t, true)
}

// newTestEnvWithEstimateToken builds the SC29 environment; estimateToken
// rewrites the generated bfe.conf so TC-06 can run with estimation disabled
// (the template ships EstimateToken = true).
func newTestEnvWithEstimateToken(t *testing.T, estimateToken bool) *testEnv {
	e := &testEnv{t: t}

	e.backend = common.NewMockBackend(clusterUsageSnapshot, http.StatusOK, "")
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	confDir := filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	tokenRule := &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": {
				{
					Id:          planRMB,
					Unlimited:   false,
					PassNoQuota: false,
					RedisKey:    redisKeyRMB,
					ExpiredTime: -1,
					Quota:       initialQuota,
					Unit:        "RMB",
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
					QuotaPlans:     []string{planRMB},
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

	aiConfs := map[string]*cluster_conf.AIConf{
		clusterUsageSnapshot: usageSnapshotAIConf(),
	}
	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: confDir,
		Backends:      map[string]*common.MockBackend{clusterUsageSnapshot: e.backend},
		AIConfs:       aiConfs,
		RedisAddr:     e.redis.Addr(),
		TokenRuleData: tokenRule,
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	if !estimateToken {
		rewriteEstimateToken(t, filepath.Join(confDir, "bfe.conf"))
	}

	e.bfePort, e.bfeMonitorPort, e.stopBFE = e.processEnv.StartBFE(confDir, e.logDir)
	return e
}

// proxyStateCounter reads /monitor/proxy_state and returns one counter value
// (e.g. PanicClientConnServe for the issue #1501 no-panic assertion). The
// body is a metrics.MetricsData serialization: counters live in CounterData
// under UPPER_SNAKE keys (CamelCase is converted), NOT in module_state2's
// SCounters — parsing the wrong field silently yields a nil map and a
// vacuous assertion (PR #1504 second-round review).
func (e *testEnv) proxyStateCounter(t *testing.T, key string) int64 {
	t.Helper()

	url := fmt.Sprintf("http://127.0.0.1:%d/monitor/proxy_state", e.bfeMonitorPort)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get proxy_state failed: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read proxy_state failed: %v", err)
	}
	var data struct {
		CounterData map[string]int64 `json:"CounterData"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse proxy_state failed: %v, body: %s", err, raw)
	}
	v, ok := data.CounterData[key]
	if !ok {
		t.Fatalf("counter %q not found in proxy_state: %s", key, raw)
	}
	return v
}

// rewriteEstimateToken flips the generated bfe.conf to EstimateToken = false.
func rewriteEstimateToken(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bfe.conf failed: %v", err)
	}
	replaced := strings.Replace(string(data), "EstimateToken = true", "EstimateToken = false", 1)
	if replaced == string(data) {
		t.Fatalf("EstimateToken = true not found in %s", path)
	}
	if err := os.WriteFile(path, []byte(replaced), 0644); err != nil {
		t.Fatalf("write bfe.conf failed: %v", err)
	}
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	if e.backend != nil {
		e.backend.Close()
	}
	if e.redis != nil {
		e.redis.Close()
	}
}

// stopAndParseLogs stops BFE (flushing the pb3 access log) and parses it.
func (e *testEnv) stopAndParseLogs() []*bfe_access_pb.RequestLog {
	e.t.Helper()
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	reqLogs, err := common.ParseAccessLogAfterStop(e.logDir)
	if err != nil {
		e.t.Fatalf("parse access log failed: %v", err)
	}
	if len(reqLogs) == 0 {
		e.t.Fatal("expected at least 1 access log, got 0")
	}
	return reqLogs
}

func (e *testEnv) logBFEException() {
	data, err := os.ReadFile(filepath.Join(e.processEnv.WorkDir(), "log", "exception.log"))
	if err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}
}

func usageSnapshotAIConf() *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type:           0,
		ModelProtocols: []string{"openai", "anthropic"}, // TC-05 drives an anthropic-compat upstream
		ModelMapping: &map[string]string{
			"gpt-4":    "deepseek-chat",
			"claude-3": "deepseek-chat",
		},
		Provider: "mock-provider",
		Keys: []cluster_conf.AIKey{
			{Name: "key-primary", Key: "sk-primary", Weight: 100},
		},
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:            "weighted_random",
			MaxRetries:          0,
			RetryBackoffInitial: 50,
			RetryBackoffMax:     200,
		},
		ModelTable: &cluster_conf.ModelTable{
			Currency: "RMB",
			Models: []cluster_conf.ModelPrice{
				{
					Provider:            "mock-provider",
					Model:               "deepseek-chat",
					BaseModel:           "deepseek-chat",
					Mode:                "chat",
					Capabilities:        []string{"chat"},
					SupportedParameters: []string{"temperature", "max_tokens"},
					Prices: cluster_conf.PriceMap{
						"input_cost_per_token":  0.000003,
						"output_cost_per_token": 0.000009,
					},
				},
			},
		},
	}
}

// sendOpenAIStream sends a chat completion request and drains the SSE stream.
func (e *testEnv) sendOpenAIStream(t *testing.T, body []byte) string {
	t.Helper()

	url := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", e.bfePort)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		e.logBFEException()
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response failed: %v", err)
	}
	return string(respBody)
}

// sendAnthropicStream sends a messages request (x-api-key auth) and drains
// the SSE stream.
func (e *testEnv) sendAnthropicStream(t *testing.T, body []byte) string {
	t.Helper()

	url := fmt.Sprintf("http://127.0.0.1:%d/v1/messages", e.bfePort)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Host = apiHost
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		e.logBFEException()
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response failed: %v", err)
	}
	return string(respBody)
}

// startRawStream opens a raw TCP connection for the abort scenario.
func (e *testEnv) startRawStream(t *testing.T, body []byte) (*http.Response, *bufio.Reader, net.Conn) {
	t.Helper()

	addr := fmt.Sprintf("127.0.0.1:%d", e.bfePort)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial bfe failed: %v", err)
	}

	httpReq, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/v1/chat/completions", nil)
	fmt.Fprintf(conn, "POST /v1/chat/completions HTTP/1.1\r\n")
	fmt.Fprintf(conn, "Host: %s\r\n", apiHost)
	fmt.Fprintf(conn, "Authorization: Bearer %s\r\n", apiKey)
	fmt.Fprintf(conn, "Content-Type: application/json\r\n")
	fmt.Fprintf(conn, "Content-Length: %d\r\n", len(body))
	fmt.Fprintf(conn, "Connection: close\r\n\r\n")
	if _, err := conn.Write(body); err != nil {
		conn.Close()
		t.Fatalf("write request failed: %v", err)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, httpReq)
	if err != nil {
		conn.Close()
		t.Fatalf("read response failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		e.logBFEException()
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	return resp, bufio.NewReader(resp.Body), conn
}

func readUntilMarker(t *testing.T, conn net.Conn, reader *bufio.Reader, marker string) {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		line, err := reader.ReadString('\n')
		if strings.Contains(line, marker) {
			return
		}
		if err != nil {
			t.Fatalf("SSE stream ended before %q: %v (last line: %q)", marker, err, line)
		}
	}
}

func abortRST(t *testing.T, conn net.Conn) {
	t.Helper()

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		conn.Close()
		return
	}
	_ = tcpConn.SetLinger(0)
	_ = tcpConn.Close()
}

func (e *testEnv) quotaRemaining(t *testing.T) int64 {
	t.Helper()
	// Deduction happens synchronously in the request finish handler; give
	// the aborted request time to settle before checking redis.
	time.Sleep(1500 * time.Millisecond)
	return e.redis.GetQuota(redisKeyRMB)
}

// lastBackendBody returns the most recent request body captured by the mock
// backend.
func (e *testEnv) lastBackendBody(t *testing.T) string {
	t.Helper()
	bodies := e.backend.RequestBodies()
	if len(bodies) == 0 {
		t.Fatal("backend received no request bodies")
	}
	return string(bodies[len(bodies)-1])
}

// logInt64 dereferences an optional int64 log field (nil -> -1).
func logInt64(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}

// TestTC01 verifies issue #1398: an OpenAI streaming request without
// stream_options gets stream_options.include_usage=true injected, the
// upstream returns the real final usage chunk, and both billing and the
// access log land the actual usage.
func TestTC01_InjectIncludeUsageBillsActualUsage(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	e.backend.SSEEvents = []string{openAIContentChunk, openAIUsageChunk, "[DONE]"}

	respBody := e.sendOpenAIStream(t, openAIStreamBody)
	if !strings.Contains(respBody, `"total_tokens":30`) {
		t.Errorf("client must receive the usage chunk passed through, got: %s", respBody)
	}

	// Injection reached the backend.
	gotBody := e.lastBackendBody(t)
	includeUsage := gjson.Get(gotBody, "stream_options.include_usage")
	if !includeUsage.Exists() || !includeUsage.Bool() {
		t.Errorf("backend body must carry stream_options.include_usage=true, got: %s", gotBody)
	}

	// Billing by the actual usage.
	remaining := e.quotaRemaining(t)
	if remaining != initialQuota-expectedActualCost {
		t.Fatalf("billing by actual usage: remaining = %d, want %d", remaining, initialQuota-expectedActualCost)
	}

	// Access log carries the actual usage fields.
	reqLogs := e.stopAndParseLogs()
	reqLog := reqLogs[len(reqLogs)-1]
	if reqLog.AiInputTokens == nil || *reqLog.AiInputTokens != 10 {
		t.Errorf("ai_input_tokens = %v, want 10", reqLog.AiInputTokens)
	}
	if reqLog.AiOutputTokens == nil || *reqLog.AiOutputTokens != 20 {
		t.Errorf("ai_output_tokens = %v, want 20", reqLog.AiOutputTokens)
	}
	if reqLog.AiTotalTokens == nil || *reqLog.AiTotalTokens != 30 {
		t.Errorf("ai_total_tokens = %v, want 30", reqLog.AiTotalTokens)
	}
}

// TestTC02 verifies issue #1398: an explicit include_usage=false is never
// overridden; with no usage chunk and a completed stream, the approved
// ContentLength/4 estimate fallback bills the request and the access log
// records the estimate (never silently zeroed).
func TestTC02_ExplicitIncludeUsageFalseFallsBackToEstimate(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	e.backend.SSEEvents = []string{openAIContentChunk, "[DONE]"}

	e.sendOpenAIStream(t, openAIStreamBodyOptOut)

	// Explicit opt-out preserved.
	gotBody := e.lastBackendBody(t)
	includeUsage := gjson.Get(gotBody, "stream_options.include_usage")
	if !includeUsage.Exists() || includeUsage.Bool() {
		t.Errorf("backend body must keep stream_options.include_usage=false, got: %s", gotBody)
	}

	// Estimate fallback (same formula as the implementation):
	// prompt = request body bytes / 4 (seeded at auth time);
	// completion = sum of per-event data bytes / 4 (QuotaUsageProcessor).
	promptEst := int64(len(openAIStreamBodyOptOut)) / 4
	completionEst := int64(len(openAIContentChunk))/4 + int64(len("[DONE]"))/4
	expectedCost := promptEst*300 + completionEst*900

	remaining := e.quotaRemaining(t)
	if remaining != initialQuota-expectedCost {
		t.Fatalf("billing by estimate: remaining = %d, want %d (promptEst=%d completionEst=%d)",
			remaining, initialQuota-expectedCost, promptEst, completionEst)
	}

	// Access log records the estimate, not zeros.
	reqLogs := e.stopAndParseLogs()
	reqLog := reqLogs[len(reqLogs)-1]
	if got := logInt64(reqLog.AiInputTokens); got != promptEst {
		t.Errorf("ai_input_tokens = %d, want %d", got, promptEst)
	}
	if got := logInt64(reqLog.AiOutputTokens); got != completionEst {
		t.Errorf("ai_output_tokens = %d, want %d", got, completionEst)
	}
	if got := logInt64(reqLog.AiTotalTokens); got != promptEst+completionEst {
		t.Errorf("ai_total_tokens = %d, want %d", got, promptEst+completionEst)
	}
}

// TestTC03 verifies issue #1478 (billed abort arm, pb3 log view): with
// EstimateToken=true a client abort without a final usage is billed by the
// forwarded-content estimate, and the access log reports the estimate
// (in + out) with ai_usage_source=aborted instead of the issue #1398-era
// "keep the seeding residue, total=0" shape. The estimation-disabled
// zero-deduction arm is covered by SC12 TC-04.
func TestTC03_ClientAbortKeepsLogViewZeroDeduction(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	hold := make(chan struct{})
	defer close(hold)
	e.backend.SSEEvents = []string{openAIContentChunk}
	e.backend.SSEHold = hold
	e.backend.SSETrailing = []string{openAILargeChunk}

	resp, reader, conn := e.startRawStream(t, openAIStreamBody)
	readUntilMarker(t, conn, reader, "chatcmpl-1")
	abortRST(t, conn)

	// prompt = auth-time seeding (request body / 4); completion = every frame
	// the estimator saw / 4 — the forwarded content chunk plus the error frame
	// issue #1474's injector emits when the abort closes the upstream body
	// (same accounting as TC-07/08).
	promptEst := int64(len(openAIStreamBody)) / 4
	completionEst := int64(len(openAIContentChunk))/4 + int64(len(openAIInjectedErrorData))/4
	wantRemaining := initialQuota - (promptEst*300 + completionEst*900)

	remaining := e.quotaRemaining(t)
	if remaining != wantRemaining {
		t.Fatalf("client abort must be billed by the forwarded-content estimate: remaining = %d, want %d (promptEst=%d completionEst=%d)",
			remaining, wantRemaining, promptEst, completionEst)
	}
	_ = resp.Body.Close()

	reqLogs := e.stopAndParseLogs()
	reqLog := reqLogs[len(reqLogs)-1]
	if reqLog.AiInputTokens == nil || *reqLog.AiInputTokens != promptEst {
		t.Errorf("ai_input_tokens = %v, want %d (auth estimate)", reqLog.AiInputTokens, promptEst)
	}
	if reqLog.AiOutputTokens == nil || *reqLog.AiOutputTokens != completionEst {
		t.Errorf("ai_output_tokens = %v, want %d (forwarded-content estimate)", reqLog.AiOutputTokens, completionEst)
	}
	if reqLog.AiTotalTokens == nil || *reqLog.AiTotalTokens != promptEst+completionEst {
		t.Errorf("ai_total_tokens = %v, want %d (mirrored estimate, issue #1398 semantics)",
			reqLog.AiTotalTokens, promptEst+completionEst)
	}
	if got := reqLog.GetAiUsageSource(); got != bfe_basic.UsageSourceAborted {
		t.Errorf("ai_usage_source = %q, want %q", got, bfe_basic.UsageSourceAborted)
	}
}

// TestTC04 verifies issue #1415 (OpenAI arm): an upstream that ends its SSE
// stream at a clean HTTP EOF without [DONE] and without any usage chunk
// still completes the response — the approved EstimateToken fallback bills
// the request and the access log records the estimate (never silently zero).
func TestTC04_CleanEofWithoutDoneBillsEstimate(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	// No usage chunk, no [DONE]; the handler returns right after the frame,
	// so the chunked stream terminates cleanly at HTTP EOF.
	e.backend.SSERaw = []string{"data: " + openAIContentChunk + "\n\n"}

	respBody := e.sendOpenAIStream(t, openAIStreamBody)
	if !strings.Contains(respBody, "chatcmpl-1") {
		t.Errorf("client must receive the content frame, got: %s", respBody)
	}

	// Estimate fallback (same formula as TC-02): prompt = request body
	// bytes / 4 (seeded at auth time); completion = per-event data bytes / 4
	// (QuotaUsageProcessor). The [DONE] frame is absent, so it contributes
	// nothing.
	promptEst := int64(len(openAIStreamBody)) / 4
	completionEst := int64(len(openAIContentChunk)) / 4
	expectedCost := promptEst*300 + completionEst*900

	remaining := e.quotaRemaining(t)
	if remaining != initialQuota-expectedCost {
		t.Fatalf("billing by estimate on clean EOF: remaining = %d, want %d (promptEst=%d completionEst=%d)",
			remaining, initialQuota-expectedCost, promptEst, completionEst)
	}

	reqLogs := e.stopAndParseLogs()
	reqLog := reqLogs[len(reqLogs)-1]
	if got := logInt64(reqLog.AiInputTokens); got != promptEst {
		t.Errorf("ai_input_tokens = %d, want %d", got, promptEst)
	}
	if got := logInt64(reqLog.AiOutputTokens); got != completionEst {
		t.Errorf("ai_output_tokens = %d, want %d", got, completionEst)
	}
	if got := logInt64(reqLog.AiTotalTokens); got != promptEst+completionEst {
		t.Errorf("ai_total_tokens = %d, want %d", got, promptEst+completionEst)
	}
	if got := reqLog.GetAiUsageSource(); got != bfe_basic.UsageSourceEstimated {
		t.Errorf("ai_usage_source = %q, want %q", got, bfe_basic.UsageSourceEstimated)
	}
	// issue #1473: the clean-EOF shape is also a truncation, and with the
	// default cluster configuration (no NormalizeUpstreamError) the injector is
	// the only component watching the stream.
	if reqLog.AiStreamTruncated == nil || !reqLog.GetAiStreamTruncated() {
		t.Errorf("ai_stream_truncated must be set on a clean EOF without [DONE], got %v",
			reqLog.AiStreamTruncated)
	}
	if reqLog.AiClientClosedAfterComplete != nil {
		t.Error("ai_client_closed_after_complete must stay unset: the client read to EOF")
	}
}

// TestTC05 verifies issue #1415 (Anthropic arm, the production bucket-A
// shape): an anthropic-compat upstream that omits usage entirely and closes
// at a clean HTTP EOF without message_stop is completed by the EOF fallback
// and billed by the approved estimate.
func TestTC05_AnthropicCleanEofWithoutMessageStopBillsEstimate(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	// message_start carries NO usage field; content frames; no message_delta,
	// no message_stop. Handler return = clean chunked EOF.
	e.backend.SSERaw = []string{
		"event: message_start\ndata: " + anthropicMessageStartData + "\n\n",
		"event: content_block_delta\ndata: " + anthropicDeltaData + "\n\n",
		"event: content_block_stop\ndata: " + anthropicBlockStopData + "\n\n",
	}

	respBody := e.sendAnthropicStream(t, anthropicMessagesBody)
	if !strings.Contains(respBody, "msg_01") {
		t.Errorf("client must receive the message_start frame, got: %s", respBody)
	}
	if strings.Contains(respBody, "message_stop") {
		t.Errorf("upstream sent no message_stop; client must not receive one: %s", respBody)
	}

	promptEst := int64(len(anthropicMessagesBody)) / 4
	completionEst := int64(len(anthropicMessageStartData))/4 +
		int64(len(anthropicDeltaData))/4 +
		int64(len(anthropicBlockStopData))/4
	expectedCost := promptEst*300 + completionEst*900

	remaining := e.quotaRemaining(t)
	if remaining != initialQuota-expectedCost {
		t.Fatalf("billing by estimate on clean EOF: remaining = %d, want %d (promptEst=%d completionEst=%d)",
			remaining, initialQuota-expectedCost, promptEst, completionEst)
	}

	reqLogs := e.stopAndParseLogs()
	reqLog := reqLogs[len(reqLogs)-1]
	if got := logInt64(reqLog.AiInputTokens); got != promptEst {
		t.Errorf("ai_input_tokens = %d, want %d", got, promptEst)
	}
	if got := logInt64(reqLog.AiOutputTokens); got != completionEst {
		t.Errorf("ai_output_tokens = %d, want %d", got, completionEst)
	}
	if got := logInt64(reqLog.AiTotalTokens); got != promptEst+completionEst {
		t.Errorf("ai_total_tokens = %d, want %d", got, promptEst+completionEst)
	}
	if got := reqLog.GetAiUsageSource(); got != bfe_basic.UsageSourceEstimated {
		t.Errorf("ai_usage_source = %q, want %q", got, bfe_basic.UsageSourceEstimated)
	}
}

// TestTC06 verifies issue #1415 stays observable when estimation is
// disabled: the clean-EOF stream is still marked completed, but with
// EstimateToken=false the request is unbillable — zero deduction with the
// ai_usage_source=unrecognized signal (issue #1410/#1414), never a silent
// zero.
func TestTC06_CleanEofEstimateTokenOffStaysZeroWithSignal(t *testing.T) {
	e := newTestEnvWithEstimateToken(t, false)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	e.backend.SSERaw = []string{"data: " + openAIContentChunk + "\n\n"}

	e.sendOpenAIStream(t, openAIStreamBody)

	remaining := e.quotaRemaining(t)
	if remaining != initialQuota {
		t.Fatalf("EstimateToken=false must not bill: remaining = %d, want %d", remaining, initialQuota)
	}

	reqLogs := e.stopAndParseLogs()
	reqLog := reqLogs[len(reqLogs)-1]
	// Issue #1480: the unbillable residue is still reported, so the token
	// triple stays complete even though nothing was charged.
	if reqLog.AiTotalTokens == nil {
		t.Fatalf("ai_total_tokens is nil, want input+output (issue #1480)")
	}
	var in, out int64
	if reqLog.AiInputTokens != nil {
		in = *reqLog.AiInputTokens
	}
	if reqLog.AiOutputTokens != nil {
		out = *reqLog.AiOutputTokens
	}
	// EstimateToken=false seeds nothing, so the triple is 0/0/0 — but it must
	// still be internally consistent (issue #1480 keeps total = in + out for
	// every shape it reports).
	if got := *reqLog.AiTotalTokens; got != in+out {
		t.Errorf("ai_total_tokens = %d, want %d (ai_input_tokens + ai_output_tokens)", got, in+out)
	}
	if got := reqLog.GetAiUsageSource(); got != bfe_basic.UsageSourceUnrecognized {
		t.Errorf("ai_usage_source = %q, want %q (unbillable shape must keep its signal)",
			got, bfe_basic.UsageSourceUnrecognized)
	}
}

// TestTC10 verifies issue #1497 (product ruling on the issue, 2026-11-11):
// an upstream that emits an early usage on a non-terminal event and then a
// usage-less termination event must not bill the early usage values. The
// early usage feeds the protocol context only (prompt fields); with no final
// usage the request is billed by the forwarded-content estimate (issue
// #1415 yardstick), reported as ai_usage_source=early_suppressed. Before the
// fix the early aggregate (10*300 + 20*900 = 21000 units) was billed
// verbatim, which is exactly what this test's exact-remaining assertion
// rules out.
func TestTC10_EarlyUsageNoFinalBillsForwardedContentEstimate(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	// Frame 1: plain content chunk (guess event, accumulates into the
	// forwarded-content estimate). Frame 2: the early usage rides a
	// non-terminal event type, so it is not a final usage (issue #1497);
	// its prompt values become protocol context only. Frame 3: usage-less
	// termination ([DONE]) — completes the response, contributes its own
	// data length to the estimate.
	e.backend.SSERaw = []string{
		"data: " + openAIContentChunk + "\n\n",
		"data: " + openAIEarlyUsageChunk + "\n\n",
		"data: [DONE]\n\n",
	}

	respBody := e.sendOpenAIStream(t, openAIStreamBody)
	if !strings.Contains(respBody, `"total_tokens":30`) {
		t.Errorf("client must receive the early usage frame passed through, got: %s", respBody)
	}

	// Estimate fallback: prompt = the early usage's prompt context (10,
	// mirrored by the context arm); completion = guess frames only (the
	// content chunk and [DONE]; the early usage event contributes 0 as a
	// non-guess event). Never the early aggregate 10+20.
	prompt := int64(10)
	completionEst := int64(len(openAIContentChunk))/4 + int64(len("[DONE]"))/4
	expectedCost := prompt*300 + completionEst*900

	remaining := e.quotaRemaining(t)
	if remaining != initialQuota-expectedCost {
		t.Fatalf("billing by forwarded-content estimate: remaining = %d, want %d (prompt=%d completionEst=%d; the early usage aggregate 10+20 must not be billed)",
			remaining, initialQuota-expectedCost, prompt, completionEst)
	}

	reqLogs := e.stopAndParseLogs()
	reqLog := reqLogs[len(reqLogs)-1]
	if got := logInt64(reqLog.AiInputTokens); got != prompt {
		t.Errorf("ai_input_tokens = %d, want %d (early usage prompt context)", got, prompt)
	}
	if got := logInt64(reqLog.AiOutputTokens); got != completionEst {
		t.Errorf("ai_output_tokens = %d, want %d (forwarded-content estimate)", got, completionEst)
	}
	if got := logInt64(reqLog.AiTotalTokens); got != prompt+completionEst {
		t.Errorf("ai_total_tokens = %d, want %d (mirrored estimate, issue #1398 semantics)", got, prompt+completionEst)
	}
	if got := reqLog.GetAiUsageSource(); got != bfe_basic.UsageSourceEarlySuppressed {
		t.Errorf("ai_usage_source = %q, want %q", got, bfe_basic.UsageSourceEarlySuppressed)
	}
}

// TestTC15 verifies issue #1501: a non-streaming 200 response whose body is
// truncated mid-transfer must not panic BFE (pre-fix: GetBodyAccessor nil-ed
// res.Body and mod_body_process's JsonDecoder dereferenced a nil reader), and
// the request must keep its full observability: a RequestLog row with
// err_code=BK_READ_RESP_BODY, ai_usage_source=upstream_dropped, and billing
// by the forwarded-content yardstick (issue #1474 non-stream boundary).
// PR #1504 review revisions: EstimateToken=true/false two arms, the no-panic
// assertion runs only after BFE stops (flushed logs, no vacuous pass), and
// PanicClientConnServe is asserted directly.
// Issue #1501 follow-up (client abort signal): the client assertion is
// tightened to require a truncated transport error — a clean EOF is a
// regression (issue #1501 #1504-deploy复验).
func TestTC15_NonStreamTruncatedBodyNoPanic(t *testing.T) {
	runArm := func(t *testing.T, estimateToken bool) {
		e := newTestEnvWithEstimateToken(t, estimateToken)
		defer e.Close()

		e.redis.SetQuota(redisKeyRMB, initialQuota)

		// A full Anthropic message body; the mock declares the full length but
		// writes only the first 128 bytes before killing the connection.
		e.backend.Body = `{"id":"msg_01","type":"message","role":"assistant","model":"claude-3-sonnet","content":[{"type":"text","text":"BFE is a layer-7 load balancer originated at Baidu and a CNCF sandbox project."}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`
		e.backend.BodyTruncateAt = 128

		panicBefore := e.proxyStateCounter(t, "PANIC_CLIENT_CONN_SERVE")

		nonStreamBody := []byte(`{"model":"claude-3","stream":false,"messages":[{"role":"user","content":"hi"}],"max_tokens":100}`)
		respBody, readErr := e.sendStreamTolerant(t, "/v1/messages",
			map[string]string{
				"x-api-key":         apiKey,
				"anthropic-version": "2023-06-01",
				"Content-Type":      "application/json",
			},
			nonStreamBody)
		// The upstream declared the full length but died mid-body, so the
		// client MUST see a truncated transport error (issue #1474 non-stream
		// boundary clause 3; issue #1501 follow-up: a clean EOF after a 200
		// with an empty body is a regression — BFE must abort the connection,
		// not finish it gracefully). A clean read of the full body would mean
		// BFE fabricated the missing bytes.
		if readErr == nil {
			t.Fatalf("client must see a truncated transport error for a truncated upstream response (body=%d bytes)", len(respBody))
		}
		if len(respBody) >= len(e.backend.Body) {
			t.Fatalf("client read the full body of a truncated upstream response (err=%v, len=%d)", readErr, len(respBody))
		}

		// No connection-level panic at all (pre-fix the request died before
		// mod_access_pb3 wrote its RequestLog).
		if panicAfter := e.proxyStateCounter(t, "PANIC_CLIENT_CONN_SERVE"); panicAfter != panicBefore {
			t.Fatalf("PanicClientConnServe grew %d -> %d (issue #1501)", panicBefore, panicAfter)
		}

		if estimateToken {
			promptEst := int64(len(nonStreamBody)) / 4
			wantRemaining := initialQuota - promptEst*300 // completion estimate: nothing forwarded
			if remaining := e.quotaRemaining(t); remaining != wantRemaining {
				e.logBFEException()
				t.Fatalf("billed by the forwarded-content yardstick: remaining = %d, want %d (promptEst=%d)",
					remaining, wantRemaining, promptEst)
			}
		} else {
			if remaining := e.quotaRemaining(t); remaining != initialQuota {
				e.logBFEException()
				t.Fatalf("EstimateToken=false must not bill: remaining = %d, want %d", remaining, initialQuota)
			}
		}

		// Access log assertions; stopAndParseLogs stops BFE first, so the
		// exception-log read below sees flushed logs (review fix: the old
		// order could pass vacuously on buffered output).
		reqLogs := e.stopAndParseLogs()
		if len(reqLogs) == 0 {
			t.Fatal("expected at least 1 access log row, got 0")
		}
		reqLog := reqLogs[len(reqLogs)-1]
		if got := reqLog.GetErrCode(); got != bfe_basic.ErrBkReadRespBody.Error() {
			t.Errorf("err_code = %q, want %q", got, bfe_basic.ErrBkReadRespBody)
		}
		if reqLog.GetAiStream() {
			t.Error("ai_stream must be false for a non-streaming request")
		}
		if estimateToken {
			if got := reqLog.GetAiUsageSource(); got != bfe_basic.UsageSourceUpstreamDropped {
				t.Errorf("ai_usage_source = %q, want %q", got, bfe_basic.UsageSourceUpstreamDropped)
			}
		} else {
			if got := reqLog.GetAiUsageSource(); got != bfe_basic.UsageSourceUnrecognized {
				t.Errorf("ai_usage_source = %q, want %q", got, bfe_basic.UsageSourceUnrecognized)
			}
			if got := reqLog.GetAiZeroCostReason(); got != bfe_basic.ZeroCostReasonUpstreamDropped {
				t.Errorf("ai_zero_cost_reason = %q, want %q", got, bfe_basic.ZeroCostReasonUpstreamDropped)
			}
		}

		// exception.log is created lazily on the first exception: absence
		// means no panic at all. Guard against a wrong log path by requiring
		// the log directory itself to exist (the access log was just parsed
		// from it, so this cannot pass vacuously).
		logDir := filepath.Join(e.processEnv.WorkDir(), "log")
		if _, err := os.Stat(logDir); err != nil {
			t.Fatalf("log directory must exist after BFE stops: %v", err)
		}
		if data, err := os.ReadFile(filepath.Join(logDir, "exception.log")); err == nil && len(data) > 0 {
			e.logBFEException()
			t.Fatalf("bfe exception log must stay empty (issue #1501):\n%s", data)
		} else if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read exception log failed: %v", err)
		}
	}

	t.Run("EstimateToken=true", func(t *testing.T) { runArm(t, true) })
	t.Run("EstimateToken=false", func(t *testing.T) { runArm(t, false) })
}

// TestTC16_NonStreamTruncatedClientReset verifies the issue #1501 follow-up
// (QA #1504 deploy re-check): an incomplete non-streaming response must abort
// the client connection (closeDirectly + SO_LINGER(0) RST), not finish it
// with a clean FIN. A raw TCP read must return a connection error instead of
// a clean io.EOF. Framing note: BFE always frames an HTTP/1.1 response
// (Content-Length or chunked), so a close-delimited BFE-to-client shape is
// not constructible in this harness; the RST pins the abortive close
// directly and covers the close-delimited shape by construction.
func TestTC16_NonStreamTruncatedClientReset(t *testing.T) {
	e := newTestEnvWithEstimateToken(t, true)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	// Same shape as TC15: the mock declares the full length but writes only
	// the first 128 bytes before killing the connection.
	e.backend.Body = `{"id":"msg_01","type":"message","role":"assistant","model":"claude-3-sonnet","content":[{"type":"text","text":"BFE is a layer-7 load balancer originated at Baidu and a CNCF sandbox project."}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`
	e.backend.BodyTruncateAt = 128

	panicBefore := e.proxyStateCounter(t, "PANIC_CLIENT_CONN_SERVE")

	nonStreamBody := `{"model":"claude-3","stream":false,"messages":[{"role":"user","content":"hi"}],"max_tokens":100}`
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", e.bfePort))
	if err != nil {
		t.Fatalf("dial BFE failed: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	fmt.Fprintf(conn, "POST /v1/messages HTTP/1.1\r\nHost: %s\r\nx-api-key: %s\r\n"+
		"anthropic-version: 2023-06-01\r\nContent-Type: application/json\r\n"+
		"Content-Length: %d\r\nConnection: keep-alive\r\n\r\n%s",
		apiHost, apiKey, len(nonStreamBody), nonStreamBody)

	raw, readErr := io.ReadAll(conn)
	if !strings.HasPrefix(string(raw), "HTTP/1.1 200") {
		t.Fatalf("expected a 200 response head, got: %q", string(raw))
	}
	if readErr == nil {
		t.Fatal("client connection must be aborted with a reset (issue #1501 follow-up): got a clean EOF")
	}

	if panicAfter := e.proxyStateCounter(t, "PANIC_CLIENT_CONN_SERVE"); panicAfter != panicBefore {
		t.Fatalf("PanicClientConnServe grew %d -> %d (issue #1501)", panicBefore, panicAfter)
	}

	reqLogs := e.stopAndParseLogs()
	if len(reqLogs) == 0 {
		t.Fatal("expected at least 1 access log row, got 0")
	}
	reqLog := reqLogs[len(reqLogs)-1]
	if got := reqLog.GetErrCode(); got != bfe_basic.ErrBkReadRespBody.Error() {
		t.Errorf("err_code = %q, want %q", got, bfe_basic.ErrBkReadRespBody)
	}
}
