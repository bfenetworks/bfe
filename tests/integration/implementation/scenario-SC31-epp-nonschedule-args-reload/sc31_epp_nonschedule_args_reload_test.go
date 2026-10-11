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

// Package sc31 exercises the hot reload of BFE's EPP non-scheduling
// parameters (EPPTLS / EPPTimeout.Connect / EPPCheck.Disabled / EPPBreaker)
// against a real BFE process. Every test drives a config change through
// /reload/server_data_conf (which re-reads cluster_conf.data and calls
// BalTable.SetGslbBasic) and asserts the observable effect on the data
// connection, the health probe, failover state or the circuit breaker.
//
// The mock EPP runs in dual mode (TLS + plaintext on one port) so a test can
// switch EPPTLS.Plaintext on reload without changing EPPAddr and prove which
// transport BFE actually uses: after the reload the abandoned transport is
// broken (existing connections closed, new ones rejected), so only a client
// that re-established the connection with the new transport keeps working.
package sc31

import (
	"encoding/json"
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

const clusterName = "cluster_epp_reload"

const testHost = "epp.example.org"

// testAPIKey is bound to the apikey/global route tables in testdata
// mod_ai_route/ai_route.data.
const testAPIKey = "ak_epp"

// backendBody is the fixed body served by the mock backend.
const backendBody = "epp-reload-backend"

// testEnv holds all resources for a single SC31 integration test.
type testEnv struct {
	t              *testing.T
	processEnv     *common.ProcessEnv
	backend        *common.MockBackend
	backendAddr    string
	epp            []*common.MockEPP
	builder        *common.BFEConfigBuilder
	eppConf        *common.EPPClusterConf
	confDir        string
	httpPort       int
	bfeMonitorPort int
	stopBFE        func()
}

// newTestEnv starts one mock backend, eppCount dual-mode mock EPP servers,
// builds the BFE config with the given EPP cluster conf and starts a real BFE
// process. Every mock EPP decides for the mock backend address.
func newTestEnv(t *testing.T, eppCount int, eppConf *common.EPPClusterConf) *testEnv {
	e := &testEnv{t: t}
	defer func() { t.Cleanup(e.Close) }()

	e.backend = common.NewMockBackend(clusterName, http.StatusOK, backendBody)
	e.backendAddr = e.backend.Addr()

	for i := 0; i < eppCount; i++ {
		e.epp = append(e.epp, common.NewMockEPP(t, common.WithDualMode()))
	}
	for _, m := range e.epp {
		m.SetDecisionEndpoint(e.backendAddr)
	}

	addrs := make([]string, len(e.epp))
	for i, m := range e.epp {
		addrs[i] = m.Addr()
	}
	eppConf.Addrs = addrs
	e.eppConf = eppConf

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	logDir := filepath.Join(e.processEnv.WorkDir(), "log")
	e.builder = &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends:      map[string]*common.MockBackend{clusterName: e.backend},
		EPPClusters:   map[string]*common.EPPClusterConf{clusterName: eppConf},
	}
	if err := e.builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	e.httpPort, e.bfeMonitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, logDir)
	return e
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	if e.backend != nil {
		e.backend.Close()
	}
}

// post sends one HTTP POST through BFE with the scenario host and api key,
// and returns status and body.
func (e *testEnv) post(marker string) (int, string) {
	e.t.Helper()
	body := fmt.Sprintf(`{"model":"epp-test-model","messages":[{"role":"user","content":"%s"}]}`, marker)
	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", e.httpPort), strings.NewReader(body))
	if err != nil {
		e.t.Fatalf("build request failed: %v", err)
	}
	req.Host = testHost
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		e.t.Fatalf("post failed: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

// reloadWith mutates the EPP cluster conf, regenerates cluster_conf.data and
// triggers /reload/server_data_conf. It fails the test if the reload reports
// an error.
func (e *testEnv) reloadWith(mutate func(*common.EPPClusterConf)) {
	e.t.Helper()
	mutate(e.eppConf)
	if err := e.builder.WriteClusterConfData(); err != nil {
		e.t.Fatalf("regenerate cluster_conf.data failed: %v", err)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/reload/server_data_conf", e.bfeMonitorPort)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		e.t.Fatalf("call reload server_data_conf failed: %v", err)
	}
	defer resp.Body.Close()

	rsp := &struct {
		Error string `json:"error"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(rsp); err != nil {
		e.t.Fatalf("decode reload rsp failed: %v", err)
	}
	if rsp.Error != "" {
		e.t.Fatalf("reload server_data_conf failed: %s", rsp.Error)
	}
}

// eppMetrics fetches /monitor/epp_metrics and returns metric lines keyed by
// their full text (name + labels).
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

// metricVal returns the value of the metric whose full key starts with name.
func metricVal(m map[string]float64, name string) float64 {
	for k, v := range m {
		if strings.HasPrefix(k, name) {
			return v
		}
	}
	return 0
}

// waitFor polls cond until it holds or the timeout elapses.
func waitFor(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cond()
}

// activeIndex returns the current active EPP address index of the cluster.
func (e *testEnv) activeIndex() int {
	key := fmt.Sprintf(`epp_active_addr_index{cluster="%s"}`, clusterName)
	return int(metricVal(e.eppMetrics(), key))
}

func (e *testEnv) fallbackLocal() float64 {
	key := fmt.Sprintf(`epp_fallback_local_total{cluster="%s"}`, clusterName)
	return metricVal(e.eppMetrics(), key)
}

func (e *testEnv) breakerTransitions(state string) float64 {
	key := fmt.Sprintf(`epp_breaker_transitions_total{cluster="%s",state="%s"}`, clusterName, state)
	return metricVal(e.eppMetrics(), key)
}

func (e *testEnv) eppCallResults(result string) float64 {
	key := fmt.Sprintf(`epp_calls_total{cluster="%s",result="%s"}`, clusterName, result)
	return metricVal(e.eppMetrics(), key)
}

// expectBackend asserts a 200 response whose body comes from the mock backend.
func (e *testEnv) expectBackend(status int, body, what string) {
	e.t.Helper()
	if status != http.StatusOK {
		e.t.Fatalf("%s: status = %d, want 200; body: %s", what, status, body)
	}
	if !strings.Contains(body, backendBody) {
		e.t.Fatalf("%s: body = %q, want it to contain %q", what, body, backendBody)
	}
}

// TestTC01_TLSToPlaintextHotUpdate covers D1: switching EPPTLS.Plaintext from
// false to true (unchanged EPPAddr) must rebuild the EPP runtime so the data
// connection and the health probe both use plaintext. The mock closes all
// existing TLS connections and rejects new TLS ones after the reload, so only
// a client that really re-dialed with plaintext keeps serving requests through
// EPP instead of falling back to local balance.
func TestTC01_TLSToPlaintextHotUpdate(t *testing.T) {
	e := newTestEnv(t, 1, &common.EPPClusterConf{
		EPPTLS: map[string]interface{}{"Insecure": true},
		EPPCheck: map[string]interface{}{
			"CheckInterval":    "200ms",
			"FailThreshold":    1,
			"Cooldown":         "30s",
			"SuccessThreshold": 1,
		},
		EPPTimeout: map[string]interface{}{"Connect": "200ms", "Call": "2s"},
	})
	m := e.epp[0]

	// baseline: data connection over TLS
	status, body := e.post("tc01-baseline")
	e.expectBackend(status, body, "baseline")
	if !waitFor(5*time.Second, func() bool { return m.TLSConnCount() >= 1 }) {
		t.Fatalf("baseline should establish a TLS connection, tls=%d", m.TLSConnCount())
	}

	// reload: plaintext=true, EPPAddr unchanged
	e.reloadWith(func(c *common.EPPClusterConf) {
		c.EPPTLS = map[string]interface{}{"Plaintext": true}
	})

	// break TLS: close existing TLS conns and reject new ones
	m.RestrictProtocols(false, true)

	status, body = e.post("tc01-after")
	e.expectBackend(status, body, "post-reload")

	// the request must have been decided by EPP over plaintext, not by local
	// fallback (a stale TLS data connection would have been broken by the
	// mock)
	if got := e.fallbackLocal(); got != 0 {
		t.Fatalf("post-reload fallback_local_total = %v, want 0 (data conn must be plaintext)", got)
	}
	if !waitFor(5*time.Second, func() bool { return m.StreamCount() >= 2 }) {
		t.Fatalf("post-reload request must reach EPP over plaintext, stream count = %d", m.StreamCount())
	}
	if !waitFor(5*time.Second, func() bool { return m.PlaintextConnCount() >= 1 }) {
		t.Fatalf("post-reload should establish a plaintext connection, plain=%d", m.PlaintextConnCount())
	}
}

// TestTC02_PlaintextToTLSHotUpdate covers D2: switching EPPTLS.Plaintext from
// true to false (unchanged EPPAddr) must rebuild the runtime and build a
// non-nil tlsConf so both the data connection and the probe use TLS. The mock
// closes existing plaintext connections and rejects new plaintext ones after
// the reload.
func TestTC02_PlaintextToTLSHotUpdate(t *testing.T) {
	e := newTestEnv(t, 1, &common.EPPClusterConf{
		EPPTLS: map[string]interface{}{"Plaintext": true},
		EPPCheck: map[string]interface{}{
			"CheckInterval":    "200ms",
			"FailThreshold":    1,
			"Cooldown":         "30s",
			"SuccessThreshold": 1,
		},
		EPPTimeout: map[string]interface{}{"Connect": "200ms", "Call": "2s"},
	})
	m := e.epp[0]

	// baseline: data connection over plaintext
	status, body := e.post("tc02-baseline")
	e.expectBackend(status, body, "baseline")
	if !waitFor(5*time.Second, func() bool { return m.PlaintextConnCount() >= 1 }) {
		t.Fatalf("baseline should establish a plaintext connection, plain=%d", m.PlaintextConnCount())
	}

	// reload: plaintext dropped -> TLS with InsecureSkipVerify, addr unchanged
	e.reloadWith(func(c *common.EPPClusterConf) {
		c.EPPTLS = map[string]interface{}{"Insecure": true}
	})

	// break plaintext: close existing plaintext conns and reject new ones
	m.RestrictProtocols(true, false)

	status, body = e.post("tc02-after")
	e.expectBackend(status, body, "post-reload")

	if got := e.fallbackLocal(); got != 0 {
		t.Fatalf("post-reload fallback_local_total = %v, want 0 (data conn must be TLS)", got)
	}
	if !waitFor(5*time.Second, func() bool { return m.StreamCount() >= 2 }) {
		t.Fatalf("post-reload request must reach EPP over TLS, stream count = %d", m.StreamCount())
	}
	if !waitFor(5*time.Second, func() bool { return m.TLSConnCount() >= 2 }) {
		t.Fatalf("post-reload should establish new TLS connections, tls=%d", m.TLSConnCount())
	}
}

// TestTC03_CheckDisabledHotUpdate covers D3: toggling EPPCheck.Disabled must
// start/stop the health check loop. While disabled, a failing primary does
// not trigger failover; re-enabling it does.
func TestTC03_CheckDisabledHotUpdate(t *testing.T) {
	e := newTestEnv(t, 2, &common.EPPClusterConf{
		EPPCheck: map[string]interface{}{
			"CheckInterval":    "200ms",
			"FailThreshold":    1,
			"Cooldown":         "30s",
			"SuccessThreshold": 1,
		},
	})

	// baseline: primary serves the request
	status, body := e.post("tc03-baseline")
	e.expectBackend(status, body, "baseline")
	if c := e.epp[0].StreamCount(); c != 1 {
		t.Fatalf("primary stream count = %d, want 1", c)
	}

	// disable health check (keep the same hysteresis so the old behavior,
	// where the loop keeps running, would fail over quickly)
	e.reloadWith(func(c *common.EPPClusterConf) {
		c.EPPCheck = map[string]interface{}{
			"Disabled":         true,
			"CheckInterval":    "200ms",
			"FailThreshold":    1,
			"Cooldown":         "30s",
			"SuccessThreshold": 1,
		}
	})

	// primary stops reporting SERVING
	e.epp[0].SetServing(false)

	// with the health loop stopped there must be no failover within several
	// check intervals
	if waitFor(2*time.Second, func() bool { return e.activeIndex() != 0 }) {
		t.Fatalf("failover happened while EPPCheck.Disabled=true (active index = %d)", e.activeIndex())
	}

	// re-enable health check -> the stopped loop must restart and fail over
	e.reloadWith(func(c *common.EPPClusterConf) {
		c.EPPCheck = map[string]interface{}{
			"CheckInterval":    "200ms",
			"FailThreshold":    1,
			"Cooldown":         "30s",
			"SuccessThreshold": 1,
		}
	})

	if !waitFor(15*time.Second, func() bool { return e.activeIndex() == 1 }) {
		t.Fatalf("failover did not happen after re-enabling EPPCheck (active index = %d)", e.activeIndex())
	}

	// subsequent request is served by the backup
	before := e.epp[1].StreamCount()
	status, body = e.post("tc03-after")
	e.expectBackend(status, body, "post-reenable")
	if c := e.epp[1].StreamCount(); c <= before {
		t.Fatalf("backup stream count = %d, want > %d (request must go to backup)", c, before)
	}
}

// TestTC04_CallTimeoutHotUpdate covers a read-type parameter
// (EPPTimeout.Call): a change applies to the next request without disrupting
// the runtime. The mock delays its decision beyond the initial call timeout
// (so BFE times out and falls back to local balance) and within the reloaded
// one (so EPP decides again).
func TestTC04_CallTimeoutHotUpdate(t *testing.T) {
	e := newTestEnv(t, 1, &common.EPPClusterConf{
		EPPCheck:   map[string]interface{}{"Disabled": true},
		EPPTimeout: map[string]interface{}{"Connect": "200ms", "Call": "150ms"},
	})
	m := e.epp[0]
	m.SetDecisionDelay(800 * time.Millisecond)

	// initial call timeout is too small: EPP times out, request falls back
	// to local balance
	status, body := e.post("tc04-timeout")
	e.expectBackend(status, body, "timeout")
	if !waitFor(5*time.Second, func() bool { return e.fallbackLocal() >= 1 }) {
		t.Fatalf("expected a local fallback on EPP call timeout, fallback = %v", e.fallbackLocal())
	}
	fallbackBefore := e.fallbackLocal()
	okBefore := e.eppCallResults("ok")

	// reload a larger call timeout (read-type, applied in place)
	e.reloadWith(func(c *common.EPPClusterConf) {
		c.EPPTimeout = map[string]interface{}{"Connect": "200ms", "Call": "3s"}
	})

	status, body = e.post("tc04-ok")
	e.expectBackend(status, body, "post-reload")

	// the new timeout takes effect: EPP now decides the request, and no extra
	// local fallback is recorded
	if !waitFor(5*time.Second, func() bool { return e.eppCallResults("ok") > okBefore }) {
		t.Fatalf("post-reload EPP ok calls did not increase (ok=%v, want > %v)",
			e.eppCallResults("ok"), okBefore)
	}
	if got := e.fallbackLocal(); got != fallbackBefore {
		t.Fatalf("post-reload fallback_local_total = %v, want unchanged %v", got, fallbackBefore)
	}
}

// TestTC05_BreakerWindowSurvivesUnchangedReload covers D5: a reload that does
// not change EPPBreaker must not reset the sliding window. Four failures
// before an unchanged reload plus one after must complete the window (size 5)
// and open the breaker; with the old behavior the reload cleared the window
// and the breaker would stay closed.
func TestTC05_BreakerWindowSurvivesUnchangedReload(t *testing.T) {
	breakerConf := map[string]interface{}{
		"WindowSize":       5,
		"MinVolume":        5,
		"ErrorRatePercent": 50,
		"OpenTimeout":      "30s",
	}
	e := newTestEnv(t, 1, &common.EPPClusterConf{
		EPPCheck:   map[string]interface{}{"Disabled": true},
		EPPTimeout: map[string]interface{}{"Connect": "200ms", "Call": "1s"},
		EPPBreaker: breakerConf,
	})

	// make every EPP call fail: close the mock EPP process
	e.epp[0].Close()

	for i := 0; i < 4; i++ {
		status, body := e.post(fmt.Sprintf("tc05-fail-%d", i))
		e.expectBackend(status, body, "pre-reload failure")
	}

	// reload with an unchanged EPPBreaker config: the window must survive
	e.reloadWith(func(c *common.EPPClusterConf) {
		c.EPPBreaker = map[string]interface{}{
			"WindowSize":       5,
			"MinVolume":        5,
			"ErrorRatePercent": 50,
			"OpenTimeout":      "30s",
		}
	})

	// a fifth failure completes the window and opens the breaker
	for i := 0; i < 3; i++ {
		status, body := e.post(fmt.Sprintf("tc05-after-%d", i))
		e.expectBackend(status, body, "post-reload request")
		if waitFor(2*time.Second, func() bool { return e.breakerTransitions("open") >= 1 }) {
			break
		}
	}
	if got := e.breakerTransitions("open"); got < 1 {
		t.Fatalf("breaker should open (window survived unchanged reload), transitions = %v", got)
	}
}

// TestTC06_AddrChangeRebuild covers the topology case: changing EPPAddr
// rebuilds the runtime and subsequent requests are decided by the new EPP.
func TestTC06_AddrChangeRebuild(t *testing.T) {
	e := newTestEnv(t, 2, &common.EPPClusterConf{
		EPPCheck:   map[string]interface{}{"Disabled": true},
		EPPTimeout: map[string]interface{}{"Connect": "200ms", "Call": "2s"},
	})

	status, body := e.post("tc06-baseline")
	e.expectBackend(status, body, "baseline")
	if c := e.epp[0].StreamCount(); c != 1 {
		t.Fatalf("primary stream count = %d, want 1", c)
	}

	// point the cluster at the second EPP only
	e.reloadWith(func(c *common.EPPClusterConf) {
		c.Addrs = []string{e.epp[1].Addr()}
	})

	status, body = e.post("tc06-after")
	e.expectBackend(status, body, "post-reload")
	if c := e.epp[1].StreamCount(); c != 1 {
		t.Fatalf("new EPP stream count = %d, want 1", c)
	}
	if c := e.epp[0].StreamCount(); c != 1 {
		t.Fatalf("old EPP stream count = %d, want 1 (unchanged)", c)
	}
}

// TestTC07_ConnectTimeoutRebuildKeepsState covers D4 combined with the state
// inheritance contract: changing EPPTimeout.Connect is a connection-shaped
// change that rebuilds the runtime, and the failover state (active index)
// must be preserved because the address table did not change.
func TestTC07_ConnectTimeoutRebuildKeepsState(t *testing.T) {
	e := newTestEnv(t, 2, &common.EPPClusterConf{
		EPPCheck: map[string]interface{}{
			"CheckInterval":    "200ms",
			"FailThreshold":    1,
			"Cooldown":         "30s",
			"SuccessThreshold": 1,
		},
		EPPTimeout: map[string]interface{}{"Connect": "200ms", "Call": "1s"},
	})

	// force failover to the backup
	e.epp[0].SetServing(false)
	if !waitFor(15*time.Second, func() bool { return e.activeIndex() == 1 }) {
		t.Fatalf("failover did not happen (active index = %d)", e.activeIndex())
	}

	// reload a new connect timeout (signature change -> rebuild, same addrs)
	e.reloadWith(func(c *common.EPPClusterConf) {
		c.EPPTimeout = map[string]interface{}{"Connect": "1s", "Call": "1s"}
	})

	// failover state must be inherited across the rebuild
	if got := e.activeIndex(); got != 1 {
		t.Fatalf("active index after rebuild = %d, want 1 (state must be inherited)", got)
	}

	// service keeps working through the backup
	before := e.epp[1].StreamCount()
	status, body := e.post("tc07-after")
	e.expectBackend(status, body, "post-rebuild")
	if c := e.epp[1].StreamCount(); c <= before {
		t.Fatalf("backup stream count = %d, want > %d", c, before)
	}
}

// TestTC08_PlaintextWithCheckDisabled covers the combined production shape
// EPPCheck.Disabled=true + EPPTLS.Plaintext=true applied in a single reload:
// the runtime must be rebuilt for the plaintext signature change (data
// connection dials plaintext) while the health check loop stays stopped. The
// data connection must keep serving over plaintext once TLS is broken, and no
// health probes may be made afterwards.
func TestTC08_PlaintextWithCheckDisabled(t *testing.T) {
	e := newTestEnv(t, 1, &common.EPPClusterConf{
		EPPCheck: map[string]interface{}{
			"CheckInterval":    "200ms",
			"FailThreshold":    1,
			"Cooldown":         "30s",
			"SuccessThreshold": 1,
		},
		EPPTLS:     map[string]interface{}{"Insecure": true},
		EPPTimeout: map[string]interface{}{"Connect": "200ms", "Call": "2s"},
	})
	m := e.epp[0]

	// baseline: TLS data connection, health check enabled
	status, body := e.post("tc08-baseline")
	e.expectBackend(status, body, "baseline")
	if !waitFor(5*time.Second, func() bool { return m.TLSConnCount() >= 1 }) {
		t.Fatalf("baseline should establish a TLS connection, tls=%d", m.TLSConnCount())
	}

	// one reload flips both: stop the health check and switch to plaintext
	e.reloadWith(func(c *common.EPPClusterConf) {
		c.EPPCheck = map[string]interface{}{
			"Disabled":         true,
			"CheckInterval":    "200ms",
			"FailThreshold":    1,
			"Cooldown":         "30s",
			"SuccessThreshold": 1,
		}
		c.EPPTLS = map[string]interface{}{"Plaintext": true}
	})

	// break TLS so only a plaintext data connection can serve the request
	m.RestrictProtocols(false, true)

	status, body = e.post("tc08-after")
	e.expectBackend(status, body, "post-reload")
	if got := e.fallbackLocal(); got != 0 {
		t.Fatalf("post-reload fallback_local_total = %v, want 0 (data conn must be plaintext)", got)
	}
	if !waitFor(5*time.Second, func() bool { return m.StreamCount() >= 2 }) {
		t.Fatalf("post-reload request must reach EPP over plaintext, stream count = %d", m.StreamCount())
	}
	if !waitFor(5*time.Second, func() bool { return m.PlaintextConnCount() >= 1 }) {
		t.Fatalf("post-reload should establish a plaintext data connection, plain=%d", m.PlaintextConnCount())
	}

	// with EPPCheck.Disabled=true there must be no health probes: the
	// plaintext connection count stays flat across several probe intervals
	pc := m.PlaintextConnCount()
	time.Sleep(1500 * time.Millisecond)
	if got := m.PlaintextConnCount(); got != pc {
		t.Fatalf("health probes still running with EPPCheck.Disabled=true: plaintext conns %d -> %d", pc, got)
	}
}

// TestTC09_PlaintextWithBreakerDisabled covers the combined production shape
// EPPBreaker.Disabled=true + EPPTLS.Plaintext=true: the control plane only
// flips Plaintext on reload (breaker stays disabled), so the runtime must be
// rebuilt for the plaintext signature change and the data connection must
// dial plaintext. With the breaker disabled, sustained EPP failures must
// never open the breaker and every request must still attempt EPP (no
// short-circuit to local balance).
func TestTC09_PlaintextWithBreakerDisabled(t *testing.T) {
	e := newTestEnv(t, 1, &common.EPPClusterConf{
		EPPCheck:   map[string]interface{}{"Disabled": true},
		EPPTLS:     map[string]interface{}{"Insecure": true},
		EPPBreaker: map[string]interface{}{"Disabled": true},
		EPPTimeout: map[string]interface{}{"Connect": "200ms", "Call": "2s"},
	})
	m := e.epp[0]

	// baseline: TLS data connection
	st, bd := e.post("tc09-baseline")
	e.expectBackend(st, bd, "baseline")
	if !waitFor(5*time.Second, func() bool { return m.TLSConnCount() >= 1 }) {
		t.Fatalf("baseline should establish a TLS connection, tls=%d", m.TLSConnCount())
	}

	// control plane only flips Plaintext; EPPBreaker stays disabled
	e.reloadWith(func(c *common.EPPClusterConf) {
		c.EPPTLS = map[string]interface{}{"Plaintext": true}
	})

	// break TLS so only a plaintext data connection can serve the request
	m.RestrictProtocols(false, true)

	st, bd = e.post("tc09-after")
	e.expectBackend(st, bd, "post-reload")
	if got := e.fallbackLocal(); got != 0 {
		t.Fatalf("post-reload fallback_local_total = %v, want 0 (data conn must be plaintext)", got)
	}
	if !waitFor(5*time.Second, func() bool { return m.StreamCount() >= 2 }) {
		t.Fatalf("post-reload request must reach EPP over plaintext, stream count = %d", m.StreamCount())
	}
	if !waitFor(5*time.Second, func() bool { return m.PlaintextConnCount() >= 1 }) {
		t.Fatalf("post-reload should establish a plaintext data connection, plain=%d", m.PlaintextConnCount())
	}

	// with EPPBreaker.Disabled=true sustained EPP failures must never open
	// the breaker, and every request must still attempt EPP
	m.SetRequestError(status.Error(codes.Unavailable, "connection refused"))
	transportBefore := e.eppCallResults("transport")
	fallbackBefore := e.fallbackLocal()
	for i := 0; i < 5; i++ {
		st, bd := e.post(fmt.Sprintf("tc09-fail-%d", i))
		e.expectBackend(st, bd, "breaker-disabled failure")
	}
	if !waitFor(5*time.Second, func() bool { return e.eppCallResults("transport") >= transportBefore+5 }) {
		t.Fatalf("EPP must be called for every request while breaker disabled, transport=%v want>=%v",
			e.eppCallResults("transport"), transportBefore+5)
	}
	if got := e.fallbackLocal(); got < fallbackBefore+5 {
		t.Fatalf("every failed EPP call must fall back to local, fallback=%v want>=%v", got, fallbackBefore+5)
	}
	if got := e.breakerTransitions("open"); got != 0 {
		t.Fatalf("breaker must never open while disabled, open transitions=%v", got)
	}

	// recovery: clearing the error serves again through the EPP plaintext conn
	m.SetRequestError(nil)
	st, bd = e.post("tc09-recover")
	e.expectBackend(st, bd, "recovery")
	if got := e.fallbackLocal(); got != fallbackBefore+5 {
		t.Fatalf("recovery request must be decided by EPP again, fallback=%v want=%v", got, fallbackBefore+5)
	}
}
