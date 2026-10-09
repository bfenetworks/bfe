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

	// expectedActualCost: prompt 10 * 300 + completion 20 * 900 = 21000
	// fixed-point units (input 0.000003 yuan/token, output 0.000009).
	expectedActualCost = int64(10*300 + 20*900)

	// Large trailing frame used to force a client write error (ECONNRESET)
	// after the client has aborted the connection (same trick as SC12).
	openAILargeChunk = fmt.Sprintf(`{"id":"chatcmpl-1","choices":[{"delta":{"content":%q}}]}`,
		strings.Repeat("x", 1<<20))
)

type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backend    *common.MockBackend
	redis      *common.RedisServer
	bfePort    int
	logDir     string
	stopBFE    func()
}

func newTestEnv(t *testing.T) *testEnv {
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

	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(confDir, e.logDir)
	return e
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
		Type: 0,
		ModelMapping: &map[string]string{
			"gpt-4": "deepseek-chat",
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

// TestTC03 verifies issue #1398: a client abort without final usage is not
// billed (issue #1352 semantics) while the access log keeps the auth-seeded
// prompt estimate — the reset guard must never wipe the log view.
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

	remaining := e.quotaRemaining(t)
	if remaining != initialQuota {
		t.Fatalf("client abort without final usage must not be deducted: remaining = %d, want %d",
			remaining, initialQuota)
	}
	_ = resp.Body.Close()

	// Log view: the auth-seeded prompt estimate survives; total stays 0
	// (nothing billed, no final usage).
	promptEst := int64(len(openAIStreamBody)) / 4
	reqLogs := e.stopAndParseLogs()
	reqLog := reqLogs[len(reqLogs)-1]
	if reqLog.AiInputTokens == nil || *reqLog.AiInputTokens != promptEst {
		t.Errorf("ai_input_tokens = %v, want %d (log view must keep the auth estimate)", reqLog.AiInputTokens, promptEst)
	}
	if reqLog.AiTotalTokens == nil || *reqLog.AiTotalTokens != 0 {
		t.Errorf("ai_total_tokens = %v, want 0 (aborted, nothing billed)", reqLog.AiTotalTokens)
	}
}
