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

package sc37

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost = "switch.example.org"
	apiPath = "/v1/chat/completions"

	clusterSwitch = "cluster_switch"
)

var defaultBody = []byte(`{"model":"gpt-4"}`)
var okBody = `{"id":"chatcmpl-sc37","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`

type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backend    *common.MockBackend

	confDir string
	logDir  string
	bfePort int
	stopBFE func()
}

// newTestEnv starts BFE in plain-forwarding mode with the given cluster
// override (nil keeps the harness defaults).
func newTestEnv(t *testing.T, ov *common.ClusterOverride) *testEnv {
	e := &testEnv{t: t}

	e.backend = common.NewMockBackend(clusterSwitch, http.StatusOK, okBody)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends: map[string]*common.MockBackend{
			clusterSwitch: e.backend,
		},
		ClusterOverrides: map[string]*common.ClusterOverride{
			clusterSwitch: ov,
		},
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	// The cluster host/health switches are general forwarding features; run
	// them on the plain path so AI modules cannot interfere.
	if err := common.RewriteConfLine(filepath.Join(e.confDir, "bfe.conf"),
		"EnableAiGateway", "false"); err != nil {
		t.Fatalf("disable AI gateway failed: %v", err)
	}

	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
	return e
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	if e.backend != nil {
		e.backend.Close()
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

func (e *testEnv) sendRequest() (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(defaultBody))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return resp, string(body), nil
}

// businessHits counts business-path requests (health checks excluded).
func (e *testEnv) businessHits() int {
	n := 0
	for _, p := range e.backend.URLPaths() {
		if p != "/healthcheck" {
			n++
		}
	}
	return n
}

// healthHits counts backend health check requests.
func (e *testEnv) healthHits() int {
	n := 0
	for _, p := range e.backend.URLPaths() {
		if p == "/healthcheck" {
			n++
		}
	}
	return n
}

func (e *testEnv) waitForHealthHit(timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		if n := e.healthHits(); n > 0 {
			return n
		}
		if time.Now().After(deadline) {
			return 0
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// healthCheckOverride makes the passive failure threshold reachable quickly.
func healthCheckOverride() map[string]interface{} {
	return map[string]interface{}{
		"Schem":         "http",
		"Uri":           "/healthcheck",
		"Host":          "example.org",
		"StatusCode":    200,
		"FailNum":       1,
		"SuccNum":       1,
		"CheckInterval": 100,
	}
}

// TestTC01_DefaultHostPassthrough verifies the backend receives the original
// client Host when DisableHostHeader is not set.
func TestTC01_DefaultHostPassthrough(t *testing.T) {
	e := newTestEnv(t, nil)
	defer e.Close()

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
	}
	hosts := e.backend.Hosts()
	if len(hosts) != 1 || hosts[0] != apiHost {
		t.Fatalf("expected backend Host=%s, got %v", apiHost, hosts)
	}
}

// TestTC02_DisableHostHeader verifies the backend Host falls back to the
// backend address when DisableHostHeader=true.
func TestTC02_DisableHostHeader(t *testing.T) {
	e := newTestEnv(t, &common.ClusterOverride{
		ClusterBasic: map[string]interface{}{"DisableHostHeader": true},
	})
	defer e.Close()

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
	}
	host, port := e.backend.HostPort()
	want := fmt.Sprintf("%s:%d", host, port)
	hosts := e.backend.Hosts()
	if len(hosts) != 1 || hosts[0] != want {
		t.Fatalf("expected backend Host=%s (backend address), got %v", want, hosts)
	}
}

// TestTC03_DisableHealthCheckKeepsBackend verifies failures do not mark the
// backend down and never start the health checker.
func TestTC03_DisableHealthCheckKeepsBackend(t *testing.T) {
	e := newTestEnv(t, &common.ClusterOverride{
		BackendConf:  map[string]interface{}{"OutlierDetectionHttpCode": "5xx"},
		CheckConf:    healthCheckOverride(),
		ClusterBasic: map[string]interface{}{"DisableHealthCheck": true},
	})
	defer e.Close()

	e.backend.ResponseFunc = func(r *http.Request, count int) (int, string) {
		if r.URL.Path == "/healthcheck" {
			return http.StatusOK, "ok"
		}
		return http.StatusInternalServerError, `{"error":"backend down"}`
	}

	for i := 0; i < 3; i++ {
		resp, body, err := e.sendRequest()
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		if resp.StatusCode != http.StatusInternalServerError {
			e.logBFEException()
			t.Fatalf("request %d: expected upstream 500 passthrough, got %d, body: %s", i+1, resp.StatusCode, body)
		}
	}
	if hits := e.businessHits(); hits != 3 {
		t.Fatalf("expected all 3 requests to reach the backend, got %d", hits)
	}
	if hits := e.healthHits(); hits != 0 {
		t.Fatalf("health checker must not start with DisableHealthCheck=true, got %d checks", hits)
	}
}

// TestTC04_HealthCheckMarksBackendDown verifies the passive failure threshold
// removes the backend and starts the health checker.
func TestTC04_HealthCheckMarksBackendDown(t *testing.T) {
	e := newTestEnv(t, &common.ClusterOverride{
		BackendConf:  map[string]interface{}{"OutlierDetectionHttpCode": "5xx"},
		CheckConf:    healthCheckOverride(),
		ClusterBasic: map[string]interface{}{"DisableHealthCheck": false},
	})
	defer e.Close()

	e.backend.ResponseFunc = func(r *http.Request, count int) (int, string) {
		return http.StatusInternalServerError, `{"error":"backend down"}`
	}

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		e.logBFEException()
		t.Fatalf("first request: expected upstream 500, got %d, body: %s", resp.StatusCode, body)
	}
	if hits := e.businessHits(); hits != 1 {
		t.Fatalf("expected 1 business hit after the first request, got %d", hits)
	}

	// The backend is now below the fail threshold: the next request must not
	// reach it, and the active health checker must start.
	resp, body, err = e.sendRequest()
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	if resp.StatusCode == http.StatusOK {
		e.logBFEException()
		t.Fatalf("second request: expected gateway error after backend removal, got 200, body: %s", body)
	}
	if hits := e.businessHits(); hits != 1 {
		t.Fatalf("removed backend must not receive business traffic, hits=%d", hits)
	}
	if hits := e.waitForHealthHit(5 * time.Second); hits == 0 {
		t.Fatalf("expected the health checker to probe the removed backend")
	}
}

// TestTC05_HealthCheckRecovery verifies the backend returns to service after
// enough successful active checks.
func TestTC05_HealthCheckRecovery(t *testing.T) {
	e := newTestEnv(t, &common.ClusterOverride{
		BackendConf:  map[string]interface{}{"OutlierDetectionHttpCode": "5xx"},
		CheckConf:    healthCheckOverride(),
		ClusterBasic: map[string]interface{}{"DisableHealthCheck": false},
	})
	defer e.Close()

	var healthOK atomic.Bool
	var bizCount atomic.Int32
	e.backend.ResponseFunc = func(r *http.Request, count int) (int, string) {
		if r.URL.Path == "/healthcheck" {
			if healthOK.Load() {
				return http.StatusOK, "ok"
			}
			return http.StatusInternalServerError, "down"
		}
		if bizCount.Add(1) == 1 {
			return http.StatusInternalServerError, `{"error":"backend down"}`
		}
		return http.StatusOK, okBody
	}

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		e.logBFEException()
		t.Fatalf("first request: expected upstream 500, got %d, body: %s", resp.StatusCode, body)
	}
	if hits := e.waitForHealthHit(5 * time.Second); hits == 0 {
		t.Fatalf("expected the health checker to probe the backend")
	}

	// Let the active check succeed; the backend should come back after
	// SuccNum=1 successful probe.
	healthOK.Store(true)

	deadline := time.Now().Add(8 * time.Second)
	for {
		resp, body, err = e.sendRequest()
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			e.logBFEException()
			t.Fatalf("backend did not recover: last status=%v err=%v body=%q", resp, err, body)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if hits := e.businessHits(); hits < 2 {
		t.Fatalf("expected a post-recovery business hit, got %d", hits)
	}
}

// TestTC06_BfeDefaultsForBothSwitches verifies BFE's own defaults when the two
// knobs are absent: Host passthrough and passive failure removal still work.
func TestTC06_BfeDefaultsForBothSwitches(t *testing.T) {
	e := newTestEnv(t, &common.ClusterOverride{
		BackendConf:        map[string]interface{}{"OutlierDetectionHttpCode": "5xx"},
		CheckConf:          healthCheckOverride(),
		RemoveClusterBasic: []string{"DisableHealthCheck", "DisableHostHeader"},
	})
	defer e.Close()

	e.backend.ResponseFunc = func(r *http.Request, count int) (int, string) {
		if r.URL.Path == "/healthcheck" {
			return http.StatusOK, "ok"
		}
		return http.StatusInternalServerError, `{"error":"backend down"}`
	}

	resp, body, err := e.sendRequest()
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		e.logBFEException()
		t.Fatalf("first request: expected upstream 500, got %d, body: %s", resp.StatusCode, body)
	}
	hosts := e.backend.Hosts()
	if len(hosts) != 1 || hosts[0] != apiHost {
		t.Fatalf("expected default Host passthrough (%s), got %v", apiHost, hosts)
	}

	// DisableHealthCheck defaults to false: the failure above removes the
	// backend, so the next request must not reach it.
	resp, body, err = e.sendRequest()
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	if resp.StatusCode == http.StatusOK {
		e.logBFEException()
		t.Fatalf("second request: expected gateway error, got 200, body: %s", body)
	}
	if hits := e.businessHits(); hits != 1 {
		t.Fatalf("default health check should remove the backend, business hits=%d", hits)
	}
}
