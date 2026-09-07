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

package sc14

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

// clusterName is the only cluster of this scenario. Its backends are real
// llm-d-inference-sim processes; scheduling decisions come from mock EPP
// servers.
const clusterName = "cluster_epp_sim"

const testHost = "epp.example.org"

// testAPIKey is bound to the apikey/global route tables in testdata
// mod_ai_route/ai_route.data.
const testAPIKey = "ak_epp"

// testEnv holds all resources for a single SC14 integration test.
type testEnv struct {
	t              *testing.T
	processEnv     *common.ProcessEnv
	simAddr        string
	stopSim        func()
	epp            []*common.MockEPP
	confDir        string
	httpPort       int
	bfeMonitorPort int
	stopBFE        func()
}

// newTestEnv starts one inference-sim backend, eppCount mock EPP servers,
// builds the BFE config and starts a real BFE process. Every mock EPP
// decides for the inference-sim address unless the test overrides it.
// Resources are stopped via t.Cleanup (registered after all TempDirs so
// cleanup runs before the temp dirs are removed).
func newTestEnv(t *testing.T, eppCount int, eppConf *common.EPPClusterConf) *testEnv {
	e := &testEnv{t: t}
	defer func() { t.Cleanup(e.Close) }()

	logDir := filepath.Join(t.TempDir(), "log")
	e.simAddr, e.stopSim = common.StartInferenceSim(t, logDir, "inference-sim", "epp-test-model")

	for i := 0; i < eppCount; i++ {
		e.epp = append(e.epp, common.NewMockEPP(t))
	}
	for _, m := range e.epp {
		m.SetDecisionEndpoint(e.simAddr)
	}

	addrs := make([]string, len(e.epp))
	for i, m := range e.epp {
		addrs[i] = m.Addr()
	}
	eppConf.Addrs = addrs

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	builder := &common.BFEConfigBuilder{
		TemplateDir:    "testdata",
		TargetConfDir:  e.confDir,
		StaticBackends: map[string]string{clusterName: e.simAddr},
		EPPClusters:    map[string]*common.EPPClusterConf{clusterName: eppConf},
	}
	if err := builder.Build(); err != nil {
		e.stopSim()
		t.Fatalf("build bfe config failed: %v", err)
	}

	e.httpPort, e.bfeMonitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, logDir)
	return e
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	if e.stopSim != nil {
		e.stopSim()
	}
}

// post sends one HTTP POST through BFE with the scenario host and api key,
// and returns status and body.
func (e *testEnv) post(path, body string) (int, string) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d%s", e.httpPort, path), strings.NewReader(body))
	if err != nil {
		e.t.Fatalf("build request failed: %v", err)
	}
	req.Host = testHost
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		e.t.Fatalf("post %s failed: %v", path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

// eppMetrics fetches /monitor/epp_metrics and returns metric lines keyed by
// their full text (name + labels), e.g.
// `epp_calls_total{cluster="cluster_epp_sim",result="ok"}`.
func (e *testEnv) eppMetrics() map[string]float64 {
	e.t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/monitor/epp_metrics", e.bfeMonitorPort)
	resp, err := http.Get(url)
	if err != nil {
		e.t.Fatalf("get epp_metrics failed: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	metrics := make(map[string]float64)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.LastIndex(line, " ")
		if idx < 0 {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(line[idx+1:]), 64)
		if err != nil {
			continue
		}
		metrics[strings.TrimSpace(line[:idx])] = v
	}
	return metrics
}

// waitForMetrics polls epp_metrics until cond is satisfied or the timeout
// elapses.
func (e *testEnv) waitForMetrics(timeout time.Duration, cond func(map[string]float64) bool) bool {
	e.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond(e.eppMetrics()) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func metricVal(m map[string]float64, name string) float64 {
	for k, v := range m {
		if strings.HasPrefix(k, name) {
			return v
		}
	}
	return 0
}

// TestTC01_DecisionForwarding verifies the basic EPP path: BFE injects the
// cluster name as pool metadata, the mock EPP decision (inference-sim
// address) is honored, the request body reaches the EPP and the response
// comes from the sim backend.
func TestTC01_DecisionForwarding(t *testing.T) {
	epp := &common.EPPClusterConf{}
	e := newTestEnv(t, 1, epp)

	const reqBody = `{"model":"epp-test-model","messages":[{"role":"user","content":"hello-epp-tc01"}]}`
	statusCode, respBody := e.post("/v1/chat/completions", reqBody)
	if statusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", statusCode, respBody)
	}
	if !strings.Contains(respBody, "hello-epp-tc01") {
		t.Fatalf("response should be echoed by inference-sim, got: %s", respBody)
	}

	// pool metadata: cluster name must be carried in llm-d.ai/inference-pool
	pools := e.epp[0].Pools()
	if len(pools) != 1 || pools[0] != clusterName {
		t.Fatalf("pools = %v, want [%s]", pools, clusterName)
	}

	// request body must be forwarded to EPP completely
	bodies := e.epp[0].RequestBodies()
	if len(bodies) != 1 || string(bodies[0]) != reqBody {
		t.Fatalf("epp request bodies = %q, want %q", bodies, reqBody)
	}

	// request headers must reach EPP
	hs := e.epp[0].RequestHeadersList()
	if len(hs) != 1 || !strings.Contains(hs[0].Get("Content-Type"), "application/json") {
		t.Fatalf("epp request headers = %v, want Content-Type application/json", hs)
	}

	// metrics: one successful EPP call
	key := fmt.Sprintf(`epp_calls_total{cluster="%s",result="ok"}`, clusterName)
	if !e.waitForMetrics(5*time.Second, func(m map[string]float64) bool { return m[key] >= 1 }) {
		t.Fatalf("metric %s should be >= 1", key)
	}
}

// TestTC02_SSEResponseBodyRelay verifies the response path: for a streaming
// (SSE) response, the mock EPP receives response headers and the complete
// response body (chunks reassembled, terminated by EndOfStream), while the
// client receives the full SSE stream.
func TestTC02_SSEResponseBodyRelay(t *testing.T) {
	epp := &common.EPPClusterConf{}
	e := newTestEnv(t, 1, epp)

	statusCode, respBody := e.post("/v1/chat/completions",
		`{"model":"epp-test-model","messages":[{"role":"user","content":"hello-epp-tc02"}],"stream":true}`)
	if statusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", statusCode, respBody)
	}
	if !strings.Contains(respBody, "data:") {
		t.Fatalf("response should be an SSE stream, got: %s", respBody)
	}

	// response headers must be relayed to EPP
	respHeaders := e.epp[0].ResponseHeadersList()
	if len(respHeaders) != 1 {
		t.Fatalf("epp response headers count = %d, want 1", len(respHeaders))
	}
	if ct := respHeaders[0].Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type relayed to EPP = %q, want text/event-stream", ct)
	}

	// the assembled body received by EPP must contain the SSE payload and,
	// being terminated by EndOfStream, must be complete
	respBodies := e.epp[0].ResponseBodies()
	if len(respBodies) != 1 {
		t.Fatalf("epp response bodies count = %d, want 1", len(respBodies))
	}
	if !strings.Contains(string(respBodies[0]), "data:") {
		t.Fatalf("body relayed to EPP should contain SSE data, got: %q", respBodies[0])
	}
	if len(respBodies[0]) == 0 || len(respBodies[0]) < len(respBody)/2 {
		t.Fatalf("body relayed to EPP (%d bytes) looks truncated, client got %d bytes",
			len(respBodies[0]), len(respBody))
	}
}

// TestTC03_HealthCheckFailover verifies ordered primary/backup consumption:
// when the primary EPP becomes unreachable, BFE's health checker fails over
// to the backup after FailThreshold, and subsequent requests are scheduled by
// the backup.
func TestTC03_HealthCheckFailover(t *testing.T) {
	epp := &common.EPPClusterConf{
		EPPCheck: map[string]interface{}{
			"CheckInterval":    "200ms",
			"FailThreshold":    1,
			"Cooldown":         "30s",
			"SuccessThreshold": 1,
		},
	}
	e := newTestEnv(t, 2, epp)

	// baseline: request served via primary
	statusCode, _ := e.post("/v1/chat/completions",
		`{"model":"epp-test-model","messages":[{"role":"user","content":"hello-epp-tc03"}]}`)
	if statusCode != http.StatusOK {
		t.Fatalf("baseline status = %d, want 200", statusCode)
	}
	if c := e.epp[0].StreamCount(); c != 1 {
		t.Fatalf("primary stream count = %d, want 1", c)
	}

	// primary goes down
	e.epp[0].Close()

	// wait for failover: active address index switches to 1
	gaugeKey := fmt.Sprintf(`epp_active_addr_index{cluster="%s"}`, clusterName)
	if !e.waitForMetrics(15*time.Second, func(m map[string]float64) bool { return m[gaugeKey] == 1 }) {
		t.Fatalf("failover did not happen: %s != 1", gaugeKey)
	}

	// subsequent request must be scheduled by the backup
	statusCode, _ = e.post("/v1/chat/completions",
		`{"model":"epp-test-model","messages":[{"role":"user","content":"hello-epp-tc03"}]}`)
	if statusCode != http.StatusOK {
		t.Fatalf("status after failover = %d, want 200", statusCode)
	}
	if c := e.epp[1].StreamCount(); c != 1 {
		t.Fatalf("backup stream count = %d, want 1", c)
	}

	failoverKey := fmt.Sprintf(`epp_failover_total{cluster="%s"}`, clusterName)
	if !e.waitForMetrics(5*time.Second, func(m map[string]float64) bool { return m[failoverKey] >= 1 }) {
		t.Fatalf("metric %s should be >= 1", failoverKey)
	}
}

// TestTC04_ErrorDrivenRetry verifies that a retryable EPP error (Unavailable
// / cell is not serving) makes BFE retry the same request on the next EPP
// address immediately, without waiting for the health check state machine.
func TestTC04_ErrorDrivenRetry(t *testing.T) {
	epp := &common.EPPClusterConf{
		EPPCheck: map[string]interface{}{
			"CheckInterval":    "200ms",
			"FailThreshold":    3,
			"Cooldown":         "30s",
			"SuccessThreshold": 2,
		},
	}
	e := newTestEnv(t, 2, epp)

	// primary rejects every stream as a draining cell
	e.epp[0].SetRequestError(status.Error(codes.Unavailable, "cell is not serving"))

	statusCode, respBody := e.post("/v1/chat/completions",
		`{"model":"epp-test-model","messages":[{"role":"user","content":"hello-epp-tc04"}]}`)
	if statusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", statusCode, respBody)
	}
	if !strings.Contains(respBody, "hello-epp-tc04") {
		t.Fatalf("response should come from the sim via backup decision, got: %s", respBody)
	}

	if c := e.epp[0].StreamCount(); c != 1 {
		t.Fatalf("primary stream count = %d, want 1", c)
	}
	if c := e.epp[1].StreamCount(); c != 1 {
		t.Fatalf("backup stream count = %d, want 1 (request must be retried on backup)", c)
	}

	// note: epp_calls_total records the final outcome of the request (ok),
	// the draining attempt on the primary is visible only in BFE logs.
}

// TestTC05_UnknownPoolFallback verifies that when every configured EPP
// address reports "unknown inference pool", BFE falls back to local WRR
// balancing and the request still succeeds.
func TestTC05_UnknownPoolFallback(t *testing.T) {
	epp := &common.EPPClusterConf{
		EPPCheck: map[string]interface{}{
			"CheckInterval":    "200ms",
			"FailThreshold":    3,
			"Cooldown":         "30s",
			"SuccessThreshold": 2,
		},
	}
	e := newTestEnv(t, 2, epp)

	errUnknownPool := status.Error(codes.Internal, "unknown inference pool")
	e.epp[0].SetRequestError(errUnknownPool)
	e.epp[1].SetRequestError(errUnknownPool)

	statusCode, respBody := e.post("/v1/chat/completions",
		`{"model":"epp-test-model","messages":[{"role":"user","content":"hello-epp-tc05"}]}`)
	if statusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (local fallback); body: %s", statusCode, respBody)
	}
	if !strings.Contains(respBody, "hello-epp-tc05") {
		t.Fatalf("response should come from the local backend, got: %s", respBody)
	}

	unknownKey := fmt.Sprintf(`epp_calls_total{cluster="%s",result="unknown_pool"}`, clusterName)
	if !e.waitForMetrics(5*time.Second, func(m map[string]float64) bool { return m[unknownKey] >= 1 }) {
		t.Fatalf("metric %s should be >= 1", unknownKey)
	}
	fallbackKey := fmt.Sprintf(`epp_fallback_local_total{cluster="%s"}`, clusterName)
	if !e.waitForMetrics(5*time.Second, func(m map[string]float64) bool { return m[fallbackKey] >= 1 }) {
		t.Fatalf("metric %s should be >= 1", fallbackKey)
	}
}

// TestTC06_EppDownFallback verifies the degradation path with a single EPP
// address: when the EPP process is gone, requests fall back to local
// balancing and still get served.
func TestTC06_EppDownFallback(t *testing.T) {
	epp := &common.EPPClusterConf{
		EPPTimeout: map[string]interface{}{
			"Connect": "200ms",
			"Call":    "1s",
		},
	}
	e := newTestEnv(t, 1, epp)

	// simulate EPP process crash
	e.epp[0].Close()

	statusCode, respBody := e.post("/v1/chat/completions",
		`{"model":"epp-test-model","messages":[{"role":"user","content":"hello-epp-tc06"}]}`)
	if statusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (local fallback); body: %s", statusCode, respBody)
	}
	if !strings.Contains(respBody, "hello-epp-tc06") {
		t.Fatalf("response should come from the local backend, got: %s", respBody)
	}

	fallbackKey := fmt.Sprintf(`epp_fallback_local_total{cluster="%s"}`, clusterName)
	if !e.waitForMetrics(5*time.Second, func(m map[string]float64) bool { return m[fallbackKey] >= 1 }) {
		t.Fatalf("metric %s should be >= 1", fallbackKey)
	}
}

// TestTC07_CircuitBreaker verifies the cluster-level circuit breaker: after
// enough consecutive EPP failures the breaker opens (recorded as a state
// transition), requests are short-circuited to local balancing and still
// succeed.
func TestTC07_CircuitBreaker(t *testing.T) {
	epp := &common.EPPClusterConf{
		EPPCheck: map[string]interface{}{
			"Disabled": true,
		},
		EPPBreaker: map[string]interface{}{
			"WindowSize":       2,
			"MinVolume":        1,
			"ErrorRatePercent": 50,
			"OpenTimeout":      "2s",
		},
		EPPTimeout: map[string]interface{}{
			"Connect": "200ms",
			"Call":    "1s",
		},
	}
	e := newTestEnv(t, 1, epp)

	// simulate EPP process crash: every EPP call fails with a transport error
	e.epp[0].Close()

	for i := 0; i < 3; i++ {
		statusCode, _ := e.post("/v1/chat/completions",
			`{"model":"epp-test-model","messages":[{"role":"user","content":"hello-epp-tc07"}]}`)
		if statusCode != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (local fallback)", i, statusCode)
		}
	}

	openKey := fmt.Sprintf(`epp_breaker_transitions_total{cluster="%s",state="open"}`, clusterName)
	if !e.waitForMetrics(10*time.Second, func(m map[string]float64) bool { return m[openKey] >= 1 }) {
		t.Fatalf("metric %s should be >= 1", openKey)
	}
}
