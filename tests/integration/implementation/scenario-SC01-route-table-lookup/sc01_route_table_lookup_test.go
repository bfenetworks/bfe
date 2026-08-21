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

package sc01

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost         = "api.example.org"
	otherHost       = "other.example.org"
	entityHost      = "entity.example.org"
	unknownHost     = "unknown.example.org"
	largeHost       = "large.example.org"
	holderHost      = "holder.example.org"
	apiPath         = "/v1/chat/completions"
	apiKeyUserA     = "ak_user_a"
	apiKeyUserB     = "ak_user_b"
	apiKeyNoBinding = "ak_no_binding"

	// accessibleBodySize mirrors the value in bfe.conf.
	accessibleBodySize = 4 * 1024 * 1024
)

var clusterNames = []string{
	"cluster_primary_a",
	"cluster_primary_b",
	"cluster_primary_c",
	"cluster_fallback_1",
	"cluster_fallback_2",
	"cluster_entity_default",
	"cluster_global_default",
	"cluster_holder",
}

// testEnvOption configures a testEnv before BFE is started.
type testEnvOption func(*testEnv)

// withTotalBodyBufferSize sets the BFE totalBodyBufferSize config.
func withTotalBodyBufferSize(size int64) testEnvOption {
	return func(e *testEnv) {
		e.totalBodyBufferSize = size
	}
}

var emptyJSONBody = []byte("{}")

// testEnv holds all resources for a single SC01 integration test.
type testEnv struct {
	t                   *testing.T
	processEnv          *common.ProcessEnv
	backends            map[string]*common.MockBackend
	bfePort             int
	bfeMonitorPort      int
	stopBFE             func()
	totalBodyBufferSize int64
}

func newTestEnv(t *testing.T, responseMap map[string]int, opts ...testEnvOption) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	// Start mock backends.
	for _, name := range clusterNames {
		resp := http.StatusOK
		if responseMap != nil {
			if r, ok := responseMap[name]; ok {
				resp = r
			}
		}
		e.backends[name] = common.NewMockBackend(name, resp, fmt.Sprintf("response from %s", name))
	}

	// Apply options before building the BFE config.
	for _, opt := range opts {
		opt(e)
	}

	// Build BFE binary and start real BFE process.
	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	confDir := filepath.Join(e.processEnv.WorkDir(), "conf")
	logDir := filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:         "testdata",
		TargetConfDir:       confDir,
		Backends:            e.backends,
		TotalBodyBufferSize: e.totalBodyBufferSize,
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	e.bfePort, e.bfeMonitorPort, e.stopBFE = e.processEnv.StartBFE(confDir, logDir)
	return e
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	for _, b := range e.backends {
		b.Close()
	}
}

func (e *testEnv) logBFEException() {
	data, err := os.ReadFile(filepath.Join(e.processEnv.WorkDir(), "log", "exception.log"))
	if err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}
}

// waitForTotalBytesBodyBuffer polls the BFE monitor endpoint until the total
// bytes_body buffer size reaches at least limit or the timeout expires.
func (e *testEnv) waitForTotalBytesBodyBuffer(limit int64) {
	deadline := time.Now().Add(60 * time.Second)
	var lastTotal int64
	var lastErr error
	for time.Now().Before(deadline) {
		total, err := common.GetBFETotalBytesBodyBuffer(e.bfeMonitorPort)
		lastTotal = total
		lastErr = err
		if err == nil && total >= limit {
			e.t.Logf("total_bytes_body_buffer reached %d (limit %d)", total, limit)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	e.t.Fatalf("total_bytes_body_buffer did not reach %d in time (last=%d, err=%v)", limit, lastTotal, lastErr)
}

func (e *testEnv) sendRequest(host, apiKey string, body []byte) (*http.Response, string, error) {
	contentType := ""
	if body != nil {
		contentType = "application/json"
	}
	return e.sendRequestWithContentType(host, apiKey, body, contentType)
}

func (e *testEnv) sendRequestWithContentType(host, apiKey string, body []byte, contentType string, timeout ...time.Duration) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(http.MethodPost, url, bodyReader)
	if err != nil {
		return nil, "", err
	}
	req.Host = host
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	clientTimeout := 30 * time.Second
	if len(timeout) > 0 {
		clientTimeout = timeout[0]
	}
	client := &http.Client{Timeout: clientTimeout}
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

func generateBody(size int) []byte {
	body := make([]byte, size)
	for i := range body {
		body[i] = byte('a' + i%26)
	}
	return body
}

func TestTC01_APIKeyRouteHit(t *testing.T) {
	e := newTestEnv(t, nil)
	defer e.Close()

	resp, body, err := e.sendRequest(apiHost, apiKeyUserA, emptyJSONBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	hits := e.backends["cluster_primary_a"].Hits() +
		e.backends["cluster_primary_b"].Hits() +
		e.backends["cluster_primary_c"].Hits()
	if hits != 1 {
		t.Fatalf("expected exactly one primary cluster hit, got %d", hits)
	}
	if e.backends["cluster_entity_default"].Hits() != 0 {
		t.Fatalf("expected entity cluster not hit, got %d", e.backends["cluster_entity_default"].Hits())
	}
	if e.backends["cluster_global_default"].Hits() != 0 {
		t.Fatalf("expected global cluster not hit, got %d", e.backends["cluster_global_default"].Hits())
	}
}

func TestTC02_EntityFallback(t *testing.T) {
	e := newTestEnv(t, nil)
	defer e.Close()

	resp, body, err := e.sendRequest(otherHost, apiKeyUserA, emptyJSONBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends["cluster_entity_default"].Hits() != 1 {
		t.Fatalf("expected cluster_entity_default hit once, got %d", e.backends["cluster_entity_default"].Hits())
	}

	hits := e.backends["cluster_primary_a"].Hits() +
		e.backends["cluster_primary_b"].Hits() +
		e.backends["cluster_primary_c"].Hits()
	if hits != 0 {
		t.Fatalf("expected no primary cluster hit, got %d", hits)
	}
	if e.backends["cluster_global_default"].Hits() != 0 {
		t.Fatalf("expected global cluster not hit, got %d", e.backends["cluster_global_default"].Hits())
	}
}

func TestTC03_NoBinding404(t *testing.T) {
	e := newTestEnv(t, nil)
	defer e.Close()

	resp, body, err := e.sendRequest(apiHost, apiKeyNoBinding, emptyJSONBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		e.logBFEException()
		t.Fatalf("expected status 404, got %d, body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "AI route not found") {
		t.Fatalf("expected 'AI route not found' in body, got %q", body)
	}
	for _, name := range clusterNames {
		if e.backends[name].Hits() != 0 {
			t.Fatalf("expected %s not hit, got %d", name, e.backends[name].Hits())
		}
	}
}

func TestTC04_MultiTargetsWeightedSelection(t *testing.T) {
	e := newTestEnv(t, nil)
	defer e.Close()

	const total = 1000
	for i := 0; i < total; i++ {
		resp, _, err := e.sendRequest(apiHost, apiKeyUserA, emptyJSONBody)
		if err != nil {
			t.Fatalf("request %d: send failed: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: expected status 200, got %d", i, resp.StatusCode)
		}
	}

	hitsA := e.backends["cluster_primary_a"].Hits()
	hitsB := e.backends["cluster_primary_b"].Hits()
	hitsC := e.backends["cluster_primary_c"].Hits()

	t.Logf("hits distribution: a=%d b=%d c=%d", hitsA, hitsB, hitsC)

	if hitsA+hitsB+hitsC != total {
		t.Fatalf("expected total hits %d, got %d", total, hitsA+hitsB+hitsC)
	}

	// weights: 60 / 30 / 10, allow +/-50 tolerance.
	if hitsA < 550 || hitsA > 650 {
		t.Fatalf("expected hitsA around 600, got %d", hitsA)
	}
	if hitsB < 250 || hitsB > 350 {
		t.Fatalf("expected hitsB around 300, got %d", hitsB)
	}
	if hitsC < 50 || hitsC > 150 {
		t.Fatalf("expected hitsC around 100, got %d", hitsC)
	}
}

func TestTC05_MultiFallbacksSuccess(t *testing.T) {
	e := newTestEnv(t, map[string]int{
		"cluster_primary_a":  http.StatusInternalServerError,
		"cluster_primary_b":  http.StatusInternalServerError,
		"cluster_primary_c":  http.StatusInternalServerError,
		"cluster_fallback_1": http.StatusBadGateway,
		"cluster_fallback_2": http.StatusOK,
	})
	defer e.Close()

	resp, body, err := e.sendRequest(apiHost, apiKeyUserA, emptyJSONBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	primaryHits := e.backends["cluster_primary_a"].Hits() +
		e.backends["cluster_primary_b"].Hits() +
		e.backends["cluster_primary_c"].Hits()
	if primaryHits < 1 {
		t.Fatalf("expected at least one primary hit, got %d", primaryHits)
	}
	if e.backends["cluster_fallback_1"].Hits() != 1 {
		t.Fatalf("expected cluster_fallback_1 hit once, got %d", e.backends["cluster_fallback_1"].Hits())
	}
	if e.backends["cluster_fallback_2"].Hits() != 1 {
		t.Fatalf("expected cluster_fallback_2 hit once, got %d", e.backends["cluster_fallback_2"].Hits())
	}
}

func TestTC06_MultiFallbacksAllFail(t *testing.T) {
	e := newTestEnv(t, map[string]int{
		"cluster_primary_a":  http.StatusInternalServerError,
		"cluster_primary_b":  http.StatusInternalServerError,
		"cluster_primary_c":  http.StatusInternalServerError,
		"cluster_fallback_1": http.StatusInternalServerError,
		"cluster_fallback_2": http.StatusInternalServerError,
	})
	defer e.Close()

	resp, body, err := e.sendRequest(apiHost, apiKeyUserA, emptyJSONBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		e.logBFEException()
		t.Fatalf("expected status 500, got %d, body: %s", resp.StatusCode, body)
	}

	if e.backends["cluster_fallback_2"].Hits() != 1 {
		t.Fatalf("expected cluster_fallback_2 hit once, got %d", e.backends["cluster_fallback_2"].Hits())
	}
}

func TestTC07_ModelOverrideAndFallback(t *testing.T) {
	e := newTestEnv(t, map[string]int{
		"cluster_primary_a":  http.StatusInternalServerError,
		"cluster_primary_b":  http.StatusInternalServerError,
		"cluster_primary_c":  http.StatusInternalServerError,
		"cluster_fallback_1": http.StatusOK,
	})
	defer e.Close()

	body := []byte(`{"model":"origin-model","messages":[{"role":"user","content":"hello"}]}`)
	resp, respBody, err := e.sendRequest(apiHost, apiKeyUserA, body)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, respBody)
	}

	var primaryGotTargetModel bool
	for _, name := range []string{"cluster_primary_a", "cluster_primary_b", "cluster_primary_c"} {
		models := e.backends[name].Models()
		for _, m := range models {
			if strings.HasPrefix(m, "target-model-") {
				primaryGotTargetModel = true
			}
		}
	}
	if !primaryGotTargetModel {
		t.Fatalf("expected at least one primary backend to receive target model")
	}

	fallbackModels := e.backends["cluster_fallback_1"].Models()
	if len(fallbackModels) != 1 || fallbackModels[0] != "fallback-model-1" {
		t.Fatalf("expected fallback model 'fallback-model-1', got %v", fallbackModels)
	}

	fallbackBodies := e.backends["cluster_fallback_1"].RequestBodies()
	if len(fallbackBodies) != 1 {
		t.Fatalf("expected exactly one fallback request body, got %d", len(fallbackBodies))
	}
	var fallbackBody map[string]interface{}
	if err := json.Unmarshal(fallbackBodies[0], &fallbackBody); err != nil {
		t.Fatalf("fallback body is not valid json: %v, body: %q", err, fallbackBodies[0])
	}
	if fallbackBody["model"] != "fallback-model-1" {
		t.Fatalf("expected fallback model 'fallback-model-1', got %v", fallbackBody["model"])
	}
	messages, ok := fallbackBody["messages"].([]interface{})
	if !ok || len(messages) != 1 {
		t.Fatalf("expected fallback body to preserve messages, got %v", fallbackBody["messages"])
	}
}

func TestTC08_FallbackWithPartialBody(t *testing.T) {
	// Primary reads 1 KB of the body and then closes the connection.
	// Because the whole body (1 MB) is smaller than accessibleBodySize,
	// BFE can buffer and rewind it, so the fallback backend receives the
	// complete body.
	e := newTestEnv(t, map[string]int{
		"cluster_fallback_1": http.StatusOK,
	})
	defer e.Close()

	e.backends["cluster_primary_a"].ReadBeforeClose = 1024

	body := generateBody(1024 * 1024)
	resp, respBody, err := e.sendRequestWithContentType(largeHost, apiKeyUserA, body, "application/octet-stream")
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, respBody)
	}

	if e.backends["cluster_primary_a"].Hits() != 1 {
		t.Fatalf("expected cluster_primary_a hit once, got %d", e.backends["cluster_primary_a"].Hits())
	}
	if e.backends["cluster_fallback_1"].Hits() != 1 {
		t.Fatalf("expected cluster_fallback_1 hit once, got %d", e.backends["cluster_fallback_1"].Hits())
	}

	fallbackBodies := e.backends["cluster_fallback_1"].RequestBodies()
	if len(fallbackBodies) != 1 {
		t.Fatalf("expected exactly one fallback request body, got %d", len(fallbackBodies))
	}
	if len(fallbackBodies[0]) != len(body) {
		t.Fatalf("expected fallback body length %d, got %d", len(body), len(fallbackBodies[0]))
	}
	for i := range body {
		if fallbackBodies[0][i] != body[i] {
			t.Fatalf("fallback body differs at byte %d", i)
		}
	}
}

func TestTC09_BodyExceedsAccessibleBodySize(t *testing.T) {
	// Body is larger than accessibleBodySize (4 MB). BFE cannot buffer the
	// entire body, so after the primary cluster fails, fallback is disabled.
	e := newTestEnv(t, map[string]int{
		"cluster_primary_a":  http.StatusInternalServerError,
		"cluster_fallback_1": http.StatusOK,
	})
	defer e.Close()

	body := generateBody(5 * 1024 * 1024)
	resp, _, err := e.sendRequestWithContentType(largeHost, apiKeyUserA, body, "application/octet-stream")
	if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
		t.Fatalf("expected request not to succeed via fallback, got status 200")
	}

	if e.backends["cluster_primary_a"].Hits() != 1 {
		t.Fatalf("expected cluster_primary_a hit once, got %d", e.backends["cluster_primary_a"].Hits())
	}
	if e.backends["cluster_fallback_1"].Hits() != 0 {
		t.Fatalf("expected cluster_fallback_1 not hit, got %d", e.backends["cluster_fallback_1"].Hits())
	}
}

func TestTC10_TotalBodyBufferSizeExceedsLimit(t *testing.T) {
	// Set totalBodyBufferSize to 2 MB and keep a holder request alive with a
	// 2 MB body. When the test request tries fallback, BFE sees that the
	// total bytes_body buffer already reaches the limit and disables fallback.
	const limit = 2 * 1024 * 1024
	e := newTestEnv(t, map[string]int{
		"cluster_primary_a":  http.StatusInternalServerError,
		"cluster_fallback_1": http.StatusOK,
		"cluster_holder":     http.StatusOK,
	}, withTotalBodyBufferSize(limit))
	defer e.Close()

	// Use a context to unblock the holder backend on test exit. This guarantees
	// cleanup does not hang if an assertion fails before we explicitly cancel.
	holderCtx, cancelHolder := context.WithCancel(context.Background())
	defer cancelHolder()
	e.backends["cluster_holder"].HoldBeforeRead = holderCtx.Done()

	holderBody := generateBody(limit)
	holderDone := make(chan struct{})
	go func() {
		defer close(holderDone)
		// The holder request intentionally blocks until we release the backend.
		// Use a long timeout so the client does not give up before the body is
		// wrapped and counted towards the global buffer total.
		resp, body, err := e.sendRequestWithContentType(holderHost, apiKeyUserA, holderBody, "application/octet-stream", 2*time.Minute)
		if err != nil {
			e.t.Logf("holder request finished with error: %v", err)
		} else {
			e.t.Logf("holder request finished: status=%d body=%q", resp.StatusCode, body)
		}
	}()

	// Poll the BFE monitor endpoint until the holder's body has actually been
	// wrapped and counted towards the global total.  The holder backend blocks
	// before reading the body, so BFE cannot finish writing the request and
	// close the bytes_body buffer while we run the test request.
	e.waitForTotalBytesBodyBuffer(limit)

	testBody := generateBody(512 * 1024)
	resp, respBody, err := e.sendRequestWithContentType(largeHost, apiKeyUserA, testBody, "application/octet-stream")
	if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
		t.Fatalf("expected request not to succeed via fallback, got status %d, body: %s", resp.StatusCode, respBody)
	}

	if e.backends["cluster_primary_a"].Hits() != 1 {
		t.Fatalf("expected cluster_primary_a hit once, got %d", e.backends["cluster_primary_a"].Hits())
	}
	if e.backends["cluster_fallback_1"].Hits() != 0 {
		t.Fatalf("expected cluster_fallback_1 not hit, got %d", e.backends["cluster_fallback_1"].Hits())
	}

	cancelHolder()
	<-holderDone
}

func TestMain(m *testing.M) {
	rand.Seed(time.Now().UnixNano())
	os.Exit(m.Run())
}
