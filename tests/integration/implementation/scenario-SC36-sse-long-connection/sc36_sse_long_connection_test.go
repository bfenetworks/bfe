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

package sc36

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
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
	apiHost = "sse.example.org"
	apiPath = "/v1/chat/completions"

	clusterSSE = "cluster_sse"

	apiKey   = "ak_auth"
	apiKeyId = "sse_key_id"
)

var defaultBody = []byte(`{"model":"gpt-4"}`)
var usageResponse = `{"id":"chatcmpl-sc36","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`

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

// newTestEnv starts the SSE topology. By default the AI gateway is enabled;
// callers may pass false to exercise the plain forwarding path (where the
// request-side Accept check lives).
func newTestEnv(t *testing.T, enableAI ...bool) *testEnv {
	aiEnabled := true
	if len(enableAI) > 0 {
		aiEnabled = enableAI[0]
	}
	e := &testEnv{t: t}

	e.backend = common.NewMockBackend(clusterSSE, http.StatusOK, usageResponse)
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
			clusterSSE: e.backend,
		},
		AIConfs: map[string]*cluster_conf.AIConf{
			clusterSSE: singleKeyAIConf(),
		},
		RedisAddr:     e.redis.Addr(),
		TokenRuleData: tokenRule,
		// Small client write timeout makes the SSE timeout exemption (and the
		// non-SSE control arm) observable within a second.
		ClusterOverrides: map[string]*common.ClusterOverride{
			clusterSSE: {
				ClusterBasic: map[string]interface{}{
					"TimeoutWriteClient": 300,
				},
			},
		},
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	if !aiEnabled {
		if err := common.RewriteConfLine(filepath.Join(e.confDir, "bfe.conf"),
			"EnableAiGateway", "false"); err != nil {
			t.Fatalf("disable AI gateway failed: %v", err)
		}
	}

	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
	return e
}

func singleKeyAIConf() *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type: 0,
		Keys: []cluster_conf.AIKey{
			{Name: "key-1", Key: "sk-sc36-1", Weight: 100},
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

// newRequest builds an AI request against the SSE cluster. Accept is only set
// when non-empty so the request-side SSE detection can be controlled.
func (e *testEnv) newRequest(accept string) *http.Request {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(defaultBody))
	if err != nil {
		e.t.Fatalf("build request failed: %v", err)
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	return req
}

func (e *testEnv) newClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		// No connection reuse: local rejections and aborts close the
		// connection, and SSE assertions are per-connection.
		Transport: &http.Transport{DisableKeepAlives: true},
	}
}

// do sends one request and reads the full body.
func (e *testEnv) do(accept string) (*http.Response, string, error) {
	resp, err := e.newClient().Do(e.newRequest(accept))
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	return resp, string(body), readErr
}

// releaseAfter closes ch after d in the background.
func releaseAfter(ch chan struct{}, d time.Duration) {
	go func() {
		time.Sleep(d)
		close(ch)
	}()
}

// TestTC01_SseRequestExemptFromWriteTimeout verifies that a request declaring
// Accept: text/event-stream is not cut by the cluster write timeout: the
// trailing frame sent after the timeout window is still delivered.
func TestTC01_SseRequestExemptFromWriteTimeout(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	hold := make(chan struct{})
	e.backend.SSEEvents = []string{`{"delta":"first"}`}
	e.backend.SSEHold = hold
	e.backend.SSETrailing = []string{`{"delta":"last"}`}
	releaseAfter(hold, 800*time.Millisecond)

	resp, body, err := e.do("text/event-stream")
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "first") || !strings.Contains(body, "last") {
		t.Fatalf("expected both frames despite the 300ms write timeout, got %q", body)
	}
}

// TestTC02_NonSseRequestCutByWriteTimeout is the control arm: the same
// backend timing without any SSE signal must be cut by the write timeout.
func TestTC02_NonSseRequestCutByWriteTimeout(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	hold := make(chan struct{})
	e.backend.SSEEvents = nil
	e.backend.Response = http.StatusOK
	e.backend.Body = usageResponse
	e.backend.HoldAfterHeader = hold
	releaseAfter(hold, 800*time.Millisecond)

	_, body, err := e.do("")
	if err == nil && strings.Contains(body, "chatcmpl-sc36") {
		t.Fatalf("expected the non-SSE response to be cut by the 300ms write timeout, got full body %q", body)
	}
}

// TestTC03_ResponseSideSseDetection verifies that the response Content-Type
// alone enables the SSE timeout exemption for vendor media types.
func TestTC03_ResponseSideSseDetection(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	for _, contentType := range []string{
		"application/sse",
		"application/x-sse",
		"text/event-stream; charset=utf-8",
	} {
		t.Run(contentType, func(t *testing.T) {
			hold := make(chan struct{})
			e.backend.SSEContentType = contentType
			e.backend.SSEEvents = []string{`{"delta":"first"}`}
			e.backend.SSEHold = hold
			e.backend.SSETrailing = []string{`{"delta":"last"}`}
			releaseAfter(hold, 800*time.Millisecond)

			resp, body, err := e.do("")
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				e.logBFEException()
				t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
			}
			if !strings.Contains(body, "last") {
				t.Fatalf("expected the trailing frame for content type %q, got %q", contentType, body)
			}
		})
	}
}

// TestTC04_ClientDisconnectCancelsUpstream verifies cancelling the upstream
// read on client disconnect: the active SSE gauge drops while the backend is
// still holding the stream open.
func TestTC04_ClientDisconnectCancelsUpstream(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	hold := make(chan struct{})
	e.backend.SSEEvents = []string{`{"delta":"first"}`}
	e.backend.SSEHold = hold
	e.backend.SSETrailing = []string{`{"delta":"never delivered"}`}
	defer close(hold)

	client := e.newClient()
	resp, err := client.Do(e.newRequest("text/event-stream"))
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(line, "first") {
		t.Fatalf("expected the first frame before abort, line=%q err=%v", line, err)
	}

	// Abort the client connection while the backend still holds the stream.
	_ = resp.Body.Close()
	client.CloseIdleConnections()

	// The active gauge must return to zero without releasing the backend hold.
	deadline := time.Now().Add(5 * time.Second)
	for {
		active, metricErr := common.ModuleMetric(e.monitorPort, "proxy_state", "SseReqActive")
		if metricErr == nil && active == 0 {
			break
		}
		if time.Now().After(deadline) {
			e.logBFEException()
			t.Fatalf("expected SSE_REQ_ACTIVE to drop to 0 after client abort, got %f", active)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestTC05_SseMetrics verifies proxy_state counts request-side and
// response-side SSE requests once each and leaves the active gauge at zero.
func TestTC05_SseMetrics(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	// Arm 1: request declares SSE.
	e.backend.SSEEvents = []string{`{"delta":"a"}`}
	if _, body, err := e.do("text/event-stream"); err != nil || !strings.Contains(body, "a") {
		t.Fatalf("request-side SSE failed: body=%q err=%v", body, err)
	}

	// Arm 2: a normal JSON request must not increment the SSE counter.
	e.backend.SSEEvents = nil
	e.backend.Response = http.StatusOK
	e.backend.Body = usageResponse
	if _, body, err := e.do("application/json"); err != nil || !strings.Contains(body, "chatcmpl-sc36") {
		t.Fatalf("normal request failed: body=%q err=%v", body, err)
	}
	if served := common.WaitForModuleMetricValue(e.monitorPort, "proxy_state", "SseReqServed", 1, 5*time.Second); served != 1 {
		t.Fatalf("expected SSE_REQ_SERVED=1 before the response-side arm, got %f", served)
	}

	// Arm 3: no Accept header, SSE response content type.
	e.backend.SSEContentType = "text/event-stream"
	e.backend.SSEEvents = []string{`{"delta":"b"}`}
	if _, body, err := e.do(""); err != nil || !strings.Contains(body, "b") {
		t.Fatalf("response-side SSE failed: body=%q err=%v", body, err)
	}

	if served := common.WaitForModuleMetricValue(e.monitorPort, "proxy_state", "SseReqServed", 2, 5*time.Second); served != 2 {
		t.Fatalf("expected SSE_REQ_SERVED=2, got %f", served)
	}
	if active, err := common.ModuleMetric(e.monitorPort, "proxy_state", "SseReqActive"); err != nil || active != 0 {
		t.Fatalf("expected SSE_REQ_ACTIVE=0, got %f (err=%v)", active, err)
	}
}

// TestTC06_AcceptHeaderRecognition pins the request-side Accept parser: a
// mixed-case value among other media types is recognized, while a value with
// a parameter suffix is not (documented current behavior). It runs with the AI
// gateway disabled because the request-side check lives in the plain
// forwarding path (ServeHTTP); the AI path relies on the response Content-Type.
func TestTC06_AcceptHeaderRecognition(t *testing.T) {
	e := newTestEnv(t, false)
	defer e.Close()

	// Slow non-SSE response: only the request-side flag can save it from the
	// 300ms write timeout, so the two arms cannot be confused by response-side
	// SSE detection.
	e.backend.SSEEvents = nil
	e.backend.Response = http.StatusOK
	e.backend.Body = usageResponse

	hold := make(chan struct{})
	e.backend.HoldAfterHeader = hold
	releaseAfter(hold, 800*time.Millisecond)

	t.Run("mixed case among media types", func(t *testing.T) {
		resp, body, err := e.do("application/json, TEXT/EVENT-STREAM")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, "chatcmpl-sc36") {
			e.logBFEException()
			t.Fatalf("expected full response for a recognized Accept value, got %d, body: %q", resp.StatusCode, body)
		}
	})

	t.Run("parameter suffix not recognized", func(t *testing.T) {
		// The previous arm consumed the hold; arm the next one.
		next := make(chan struct{})
		e.backend.HoldAfterHeader = next
		releaseAfter(next, 800*time.Millisecond)

		_, body, err := e.do("text/event-stream; q=0.9")
		if err == nil && strings.Contains(body, "chatcmpl-sc36") {
			t.Fatalf("expected the parameterized Accept value to be treated as non-SSE, got full body %q", body)
		}
	})
}
