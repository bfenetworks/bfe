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

package sc12

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

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost  = "abort-billing.example.org"
	apiKey   = "ak_user_a"
	apiKeyId = "user_a_key_id"

	clusterClientAbort = "cluster_client_abort"

	planRMB     = "plan_rmb"
	redisKeyRMB = "quota:plan_rmb"

	initialQuota = int64(10000000000)
)

var (
	claudeStreamBody = []byte(`{"model":"claude-opus-4-6","stream":true,"max_tokens":1024}`)

	// Anthropic SSE frames. The initial usage is nested under
	// message.usage (output_tokens = 0); the final usage arrives in the
	// top-level usage of the message_delta event.
	anthropicMessageStart = `{"type":"message_start","message":{"id":"msg_01","type":"message","usage":{"input_tokens":320,"output_tokens":0}}}`
	anthropicContentDelta = `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`
	anthropicMessageDelta = `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":150}}`
	anthropicMessageStop  = `{"type":"message_stop"}`

	// Usage-free first frame. The mock backend only flushes the SSE response
	// header when it writes an event, so arm A1 (abort before any usage
	// event) needs one frame that carries no usage for the client to observe
	// the 200 and synchronize with the stream.
	anthropicPing = `{"type":"ping"}`

	// Large trailing frame used to force a client write error (ECONNRESET)
	// after the client has aborted the connection.
	anthropicLargeBlock = fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%q}}`,
		strings.Repeat("x", 1<<20))

	// injectedErrorFrame is the OpenAI-native error event issue #1474's
	// streamTruncationInjector emits when it observes the upstream body being
	// closed mid-stream (the abort path closes it via CloseWatcher). It is an
	// observation of the read path, not model output, but the
	// forwarded-content estimator counts every frame it sees — TC-01 budgets
	// for it explicitly so the expected amount stays exact.
	injectedErrorFrame = `{"error":{"message":"upstream stream terminated abnormally.","type":"server_error","param":null,"code":"upstream_stream_truncated"}}`

	// Expected deduction for the full stream:
	// prompt 320 * 452 + completion 150 * 2262 = 483940 units.
	expectedStreamCost = int64(320*452 + 150*2262)
)

type testEnv struct {
	t              *testing.T
	processEnv     *common.ProcessEnv
	backend        *common.MockBackend
	redis          *common.RedisServer
	bfePort        int
	bfeMonitorPort int
	stopBFE        func()
}

func newTestEnv(t *testing.T) *testEnv {
	return newTestEnvWithEstimateToken(t, true)
}

// newTestEnvWithEstimateToken builds the SC12 environment; estimateToken
// rewrites the generated bfe.conf after Build (the template ships
// EstimateToken = true), so the billed arm (issue #1478, TC-01~03) and the
// zero-billed arm (EstimateToken=false, TC-04) share one static template.
func newTestEnvWithEstimateToken(t *testing.T, estimateToken bool) *testEnv {
	e := &testEnv{t: t}

	e.backend = common.NewMockBackend(clusterClientAbort, http.StatusOK, "")
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
		clusterClientAbort: clientAbortAIConf(),
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: confDir,
		Backends:      map[string]*common.MockBackend{clusterClientAbort: e.backend},
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

	e.bfePort, e.bfeMonitorPort, e.stopBFE = e.processEnv.StartBFE(confDir, logDir)
	return e
}

// rewriteEstimateToken flips the generated bfe.conf to EstimateToken = false
// for the issue #1478 zero-billed abort arm (TC-04).
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

func (e *testEnv) logBFEException() {
	data, err := os.ReadFile(filepath.Join(e.processEnv.WorkDir(), "log", "exception.log"))
	if err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}
}

func clientAbortAIConf() *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type: 0,
		ModelMapping: &map[string]string{
			"gpt-4": "claude-opus-4-6",
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
					Model:               "claude-opus-4-6",
					BaseModel:           "claude-opus-4-6",
					Mode:                "chat",
					Capabilities:        []string{"chat"},
					SupportedParameters: []string{"temperature", "max_tokens"},
					Limits: map[string]interface{}{
						"context_window": 128000,
					},
					Prices: cluster_conf.PriceMap{
						"input_cost_per_token":  0.00000452,
						"output_cost_per_token": 0.00002262,
					},
				},
			},
		},
	}
}

// startSSERequest opens a raw TCP connection to BFE and sends the request.
// The returned reader yields the SSE stream (chunked-decoded); the caller
// decides when to abort the connection.
func (e *testEnv) startSSERequest(t *testing.T, path string, body []byte) (*http.Response, *bufio.Reader, net.Conn) {
	t.Helper()

	addr := fmt.Sprintf("127.0.0.1:%d", e.bfePort)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial bfe failed: %v", err)
	}

	httpReq, _ := http.NewRequest(http.MethodPost, "http://"+addr+path, nil)
	fmt.Fprintf(conn, "POST %s HTTP/1.1\r\n", path)
	fmt.Fprintf(conn, "Host: %s\r\n", apiHost)
	fmt.Fprintf(conn, "Authorization: Bearer %s\r\n", apiKey)
	fmt.Fprintf(conn, "anthropic-version: 2023-06-01\r\n")
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

// readUntilSSEMarker reads SSE lines until a line containing the marker is
// seen, so the caller can synchronize with the stream before aborting.
func readUntilSSEMarker(t *testing.T, conn net.Conn, reader *bufio.Reader, marker string) {
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

// abortRST closes the connection with SO_LINGER=0 so the kernel sends a TCP
// RST instead of a graceful FIN, mimicking the client abort in issue #1352.
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

// tokenAuthMetric reads a mod_ai_token_auth counter by its Go field name
// (issue #1414: BillingZeroCost / AbortBillingSkipped / UsageUnrecognized).
func (e *testEnv) tokenAuthMetric(t *testing.T, field string) float64 {
	t.Helper()
	v, err := common.ModuleMetric(e.bfeMonitorPort, "mod_ai_token_auth", field)
	if err != nil {
		t.Fatalf("read mod_ai_token_auth.%s failed: %v", field, err)
	}
	return v
}

// waitTokenAuthMetric polls a counter until it reaches want (the abort branch
// is only reached after BFE fails to write to the closed client connection,
// i.e. after the trailing frame is released).
func (e *testEnv) waitTokenAuthMetric(t *testing.T, field string, want float64) float64 {
	t.Helper()
	got, err := common.WaitForModuleMetric(e.bfeMonitorPort, "mod_ai_token_auth", field,
		10*time.Second, func(v float64) bool { return v >= want })
	if err != nil {
		t.Fatalf("read mod_ai_token_auth.%s failed: %v", field, err)
	}
	return got
}

// assertAbortSignals asserts the issue #1414 zero-deduction signals of one
// aborted request: the abort branch is counted on its own counter, the
// unrecognized-usage counter stays untouched (the semantic split), and the
// request is counted by the zero-cost checkpoint while it holds an RMB plan.
func (e *testEnv) assertAbortSignals(t *testing.T, arm string, wantAbort, wantZeroCost float64) {
	t.Helper()

	abort := e.waitTokenAuthMetric(t, "AbortBillingSkipped", wantAbort)
	// The abort branch increments both counters in the same handler call; wait
	// for each of them so an HTTP round trip cannot observe the gap.
	zeroCost := e.waitTokenAuthMetric(t, "BillingZeroCost", wantZeroCost)
	unrecognized := e.tokenAuthMetric(t, "UsageUnrecognized")
	t.Logf("%s: AbortBillingSkipped = %v (want %v), BillingZeroCost = %v (want %v), UsageUnrecognized = %v (want 0)",
		arm, abort, wantAbort, zeroCost, wantZeroCost, unrecognized)

	if abort != wantAbort {
		t.Errorf("%s: AbortBillingSkipped = %v, want %v", arm, abort, wantAbort)
	}
	if zeroCost != wantZeroCost {
		t.Errorf("%s: BillingZeroCost = %v, want %v (RMB plan bound, charged 0)", arm, zeroCost, wantZeroCost)
	}
	if unrecognized != 0 {
		t.Errorf("%s: UsageUnrecognized = %v, want 0 (a client abort is not an upstream usage problem)", arm, unrecognized)
	}
}

// TestTC01 verifies issue #1478: EstimateToken=true, the client aborts (TCP
// RST) mid-stream without any final usage. BFE bills the request by the
// forwarded-content estimate — the auth-time prompt seeding plus the
// per-chunk accumulation of the frames actually forwarded to the client —
// instead of dropping it to zero as the issue #1352-era behavior did.
func TestTC01_ClientAbortBilledByForwardedContent(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	hold := make(chan struct{})
	defer close(hold)
	// Usage-free frames only: billing runs entirely through the
	// forwarded-content estimate (issue #1478).
	e.backend.SSEEvents = []string{anthropicPing, anthropicContentDelta}
	e.backend.SSEHold = hold

	_, reader, conn := e.startSSERequest(t, "/anthropic/v1/messages", claudeStreamBody)
	readUntilSSEMarker(t, conn, reader, "hello") // content frame forwarded
	abortRST(t, conn)

	// prompt = request body bytes / 4 (auth-time seeding); completion = the
	// data length of every frame the estimator sees / 4 (QuotaUsageProcessor,
	// per-event integer division). Besides the two forwarded frames, the abort
	// closes the upstream body (CloseWatcher), which issue #1474's truncation
	// injector reports as a mid-stream drop and answers with one protocol
	// error event; that frame enters the same read path and is counted — the
	// same accounting as bfe SC29 TC-07/08 and documented there.
	promptEst := int64(len(claudeStreamBody)) / 4
	completionEst := int64(len(anthropicPing))/4 +
		int64(len(anthropicContentDelta))/4 +
		int64(len(injectedErrorFrame))/4
	want := initialQuota - (promptEst*452 + completionEst*2262)

	remaining := e.quotaRemaining(t)
	if remaining != want {
		t.Fatalf("client abort must be billed by the forwarded-content estimate: remaining = %d, want %d (promptEst=%d completionEst=%d)",
			remaining, want, promptEst, completionEst)
	}

	// issue #1478 signal split: the billed arm counts AbortBillingEstimated
	// and leaves the zero-billed shape untouched.
	billed := e.waitTokenAuthMetric(t, "AbortBillingEstimated", 1)
	if billed != 1 {
		t.Errorf("AbortBillingEstimated = %v, want 1", billed)
	}
	if got := e.tokenAuthMetric(t, "AbortBillingSkipped"); got != 0 {
		t.Errorf("AbortBillingSkipped = %v, want 0 (the abort was billed)", got)
	}
	if got := e.tokenAuthMetric(t, "BillingZeroCost"); got != 0 {
		t.Errorf("BillingZeroCost = %v, want 0 (the abort was billed)", got)
	}
	if got := e.tokenAuthMetric(t, "UsageUnrecognized"); got != 0 {
		t.Errorf("UsageUnrecognized = %v, want 0", got)
	}
}

// TestTC02 verifies issue #1352: the final usage (message_delta) was already
// received when the client aborts. The request is still billed by the actual
// usage, not by estimation and not for free.
func TestTC02_ClientAbortAfterFinalUsageBillsActualUsage(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	hold := make(chan struct{})
	defer close(hold)
	e.backend.SSEEvents = []string{anthropicMessageStart, anthropicMessageDelta}
	e.backend.SSEHold = hold
	e.backend.SSETrailing = []string{anthropicMessageStop}

	_, reader, conn := e.startSSERequest(t, "/anthropic/v1/messages", claudeStreamBody)
	readUntilSSEMarker(t, conn, reader, "message_delta")
	abortRST(t, conn)

	remaining := e.quotaRemaining(t)
	want := initialQuota - expectedStreamCost
	if remaining != want {
		t.Fatalf("client abort after final usage must bill actual usage: remaining = %d, want %d",
			remaining, want)
	}
}

// TestTC03 verifies the normal path is unchanged: a complete SSE stream is
// billed by the actual usage (prompt 320 * 452 + completion 150 * 2262).
func TestTC03_CompleteStreamBillsActualUsage(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	e.backend.SSEEvents = []string{
		anthropicMessageStart,
		anthropicContentDelta,
		anthropicMessageDelta,
		anthropicMessageStop,
	}

	resp, reader, conn := e.startSSERequest(t, "/anthropic/v1/messages", claudeStreamBody)
	readUntilSSEMarker(t, conn, reader, "message_stop")
	_, _ = io.Copy(io.Discard, reader)
	_ = resp.Body.Close()
	_ = conn.Close()

	remaining := e.quotaRemaining(t)
	want := initialQuota - expectedStreamCost
	if remaining != want {
		t.Fatalf("complete stream must bill actual usage: remaining = %d, want %d", remaining, want)
	}
}

// TestTC04_AbortBillingSkippedSignals verifies issue #1414 + #1478: with
// EstimateToken=false the client-abort zero-deduction is observable and
// separable from "upstream usage unrecognized". Two arms cover the two shapes
// the abort branch absorbs:
//
//	A1: the client aborts before any usage event was seen;
//	A2: message_start (initial usage, UsedQuota > 0, not a final event) was
//	    already parsed when the client aborts.
//
// Both must stay at zero deduction (the #1478 billed arm is TC-01), be counted
// by AbortBillingSkipped, counted by BillingZeroCost (an RMB plan is bound),
// must NOT touch UsageUnrecognized, and must NOT advance AbortBillingEstimated.
func TestTC04_AbortBillingSkippedSignals(t *testing.T) {
	e := newTestEnvWithEstimateToken(t, false)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	// Arm A1: abort with no usage event seen. The usage-free ping frame only
	// flushes the response header so the client can observe the 200 and
	// synchronize before the RST.
	holdA1 := make(chan struct{})
	e.backend.SSEEvents = []string{anthropicPing}
	e.backend.SSEHold = holdA1
	e.backend.SSETrailing = []string{anthropicLargeBlock}

	_, reader, conn := e.startSSERequest(t, "/anthropic/v1/messages", claudeStreamBody)
	readUntilSSEMarker(t, conn, reader, "ping")
	abortRST(t, conn)
	close(holdA1)

	if remaining := e.quotaRemaining(t); remaining != initialQuota {
		t.Fatalf("arm A1: abort before any usage must not be deducted: remaining = %d, want %d",
			remaining, initialQuota)
	}
	e.assertAbortSignals(t, "arm A1 (no usage before abort)", 1, 1)

	// Arm A2: message_start was parsed (initial usage, UsedQuota > 0) before
	// the client aborts; the abort branch must still absorb this shape.
	holdA2 := make(chan struct{})
	e.backend.SSEEvents = []string{anthropicMessageStart}
	e.backend.SSEHold = holdA2
	e.backend.SSETrailing = []string{anthropicLargeBlock}

	_, reader, conn = e.startSSERequest(t, "/anthropic/v1/messages", claudeStreamBody)
	readUntilSSEMarker(t, conn, reader, "message_start")
	abortRST(t, conn)
	close(holdA2)

	if remaining := e.quotaRemaining(t); remaining != initialQuota {
		t.Fatalf("arm A2: abort after the initial usage must not be deducted either: remaining = %d, want %d",
			remaining, initialQuota)
	}
	e.assertAbortSignals(t, "arm A2 (initial usage before abort)", 2, 2)

	// issue #1478: the billed-abort counter must stay untouched on the
	// estimation-disabled arm.
	if got := e.tokenAuthMetric(t, "AbortBillingEstimated"); got != 0 {
		t.Errorf("AbortBillingEstimated = %v, want 0 (no abort was billed)", got)
	}
}
