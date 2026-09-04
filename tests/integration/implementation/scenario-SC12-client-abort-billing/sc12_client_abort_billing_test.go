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

	// Large trailing frame used to force a client write error (ECONNRESET)
	// after the client has aborted the connection.
	anthropicLargeBlock = fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%q}}`,
		strings.Repeat("x", 1<<20))

	// Expected deduction for the full stream:
	// prompt 320 * 452 + completion 150 * 2262 = 483940 units.
	expectedStreamCost = int64(320*452 + 150*2262)
)

type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backend    *common.MockBackend
	redis      *common.RedisServer
	bfePort    int
	stopBFE    func()
}

func newTestEnv(t *testing.T) *testEnv {
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

	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(confDir, logDir)
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

// TestTC01 verifies issue #1352: EstimateToken=true, the client aborts
// (TCP RST) right after message_start, before any final usage. BFE must not
// bill the request by full-request estimation.
func TestTC01_ClientAbortAfterMessageStartNoDeduction(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, initialQuota)

	hold := make(chan struct{})
	defer close(hold)
	e.backend.SSEEvents = []string{anthropicMessageStart}
	e.backend.SSEHold = hold
	// After the abort, push a large frame so BFE's next write to the dead
	// connection fails with ECONNRESET (ErrClientWrite).
	e.backend.SSETrailing = []string{anthropicLargeBlock}

	_, reader, conn := e.startSSERequest(t, "/anthropic/v1/messages", claudeStreamBody)
	readUntilSSEMarker(t, conn, reader, "message_start")
	abortRST(t, conn)

	remaining := e.quotaRemaining(t)
	if remaining != initialQuota {
		t.Fatalf("client abort after message_start must not be deducted: remaining = %d, want %d",
			remaining, initialQuota)
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
