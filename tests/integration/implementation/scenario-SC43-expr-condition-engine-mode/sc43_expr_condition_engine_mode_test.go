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

// Package sc42 implements scenario SC42: expr condition engine three modes
// (LegacyFirst / ExprCompatible / ExprOnly) selected by bfe.conf
// Server.ConditionEngineMode, consumed via route_rule.data ProductRule Cond.
package sc42

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	clusterHit     = "cluster_expr_hit"
	clusterDefault = "cluster_expr_default"

	hostExpr     = "expr.example.org"
	hostLegacy   = "legacy.example.org"
	hostFallback = "fallback.example.org"
)

// routeRule is one ProductRule entry: Cond selects the target cluster.
type routeRule struct {
	ClusterName string `json:"ClusterName"`
	Cond        string `json:"Cond"`
}

type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backendHit *common.MockBackend
	backendDef *common.MockBackend

	confDir string
	logDir  string
	bfePort int
	stopBFE func()
}

// newTestEnv builds a config dir from testdata and writes the given engine
// mode and route rules. mode == "" rewrites ConditionEngineMode to the empty
// value, which the config loader treats as the default (LegacyFirst), so
// TC-03 can verify default-mode behavior with the same template.
func newTestEnv(t *testing.T, mode string, productRules map[string][]routeRule) *testEnv {
	e := &testEnv{t: t}

	e.backendHit = common.NewMockBackend(clusterHit, http.StatusOK, clusterHit)
	e.backendDef = common.NewMockBackend(clusterDefault, http.StatusOK, clusterDefault)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends: map[string]*common.MockBackend{
			clusterHit:     e.backendHit,
			clusterDefault: e.backendDef,
		},
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	confPath := filepath.Join(e.confDir, "bfe.conf")
	if err := common.RewriteConfLine(confPath, "EnableAiGateway", "false"); err != nil {
		t.Fatalf("set EnableAiGateway=false failed: %v", err)
	}
	if err := common.RewriteConfLine(confPath, "ConditionEngineMode", mode); err != nil {
		t.Fatalf("set ConditionEngineMode=%q failed: %v", mode, err)
	}

	routeRuleData := map[string]interface{}{
		"Version":     "20261030000000",
		"ProductRule": productRules,
	}
	if err := common.WriteJSONFile(filepath.Join(e.confDir, "server_data_conf", "route_rule.data"), routeRuleData); err != nil {
		t.Fatalf("write route_rule.data failed: %v", err)
	}

	return e
}

func (e *testEnv) start() {
	e.t.Helper()
	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
}

// startFailed asserts that BFE refuses to start (fail-fast), e.g. an illegal
// ConditionEngineMode or a condition that fails to compile in a no-fallback
// mode. StartBFERaw already reaps the process on timeout.
func (e *testEnv) startFailed() {
	e.t.Helper()
	if _, _, stop, err := e.processEnv.StartBFERaw(e.confDir, e.logDir); err == nil {
		stop()
		e.t.Fatalf("expected BFE to fail fast, but it started")
	}
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	if e.backendHit != nil {
		e.backendHit.Close()
	}
	if e.backendDef != nil {
		e.backendDef.Close()
	}
}

// logBFEException dumps exception.log and the tail of bfe.log for debugging.
func (e *testEnv) logBFEException() {
	data, err := os.ReadFile(filepath.Join(e.logDir, "exception.log"))
	if err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}
	logData, err := os.ReadFile(filepath.Join(e.logDir, "bfe.log"))
	if err == nil && len(logData) > 0 {
		lines := strings.Split(string(logData), "\n")
		start := 0
		if len(lines) > 120 {
			start = len(lines) - 120
		}
		e.t.Logf("bfe log tail:\n%s", strings.Join(lines[start:], "\n"))
	}
}

// send issues one request to BFE with the given Host and returns status and body.
func (e *testEnv) send(method, host, path string, headers map[string]string) (int, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, path)
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return 0, "", err
	}
	req.Host = host
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}
	return resp.StatusCode, string(body), nil
}

// expectCluster sends one request and asserts the response came from the
// cluster identified by body content (mock backends respond with the cluster
// name). HEAD responses carry no body; use expectClusterByBackend for those.
func (e *testEnv) expectCluster(method, host, path string, headers map[string]string, wantCluster string) {
	e.t.Helper()
	status, body, err := e.send(method, host, path, headers)
	if err != nil {
		e.logBFEException()
		e.t.Fatalf("send %s %s (Host=%s) failed: %v", method, path, host, err)
	}
	if status != http.StatusOK {
		e.logBFEException()
		e.t.Fatalf("send %s %s (Host=%s): expected 200, got %d, body: %s", method, path, host, status, body)
	}
	if !strings.Contains(body, wantCluster) {
		e.t.Fatalf("send %s %s (Host=%s): expected body to contain %q, got %q", method, path, host, wantCluster, body)
	}
}

// expectClusterByBackend sends one request and asserts which mock backend
// served it by inspecting recorded request paths (for HEAD, where the
// response body is always empty).
func (e *testEnv) expectClusterByBackend(method, host, path string, headers map[string]string, wantBackend *common.MockBackend) {
	e.t.Helper()
	before := map[*common.MockBackend]int{
		e.backendHit: e.backendHit.Hits(),
		e.backendDef: e.backendDef.Hits(),
	}
	status, _, err := e.send(method, host, path, headers)
	if err != nil {
		e.logBFEException()
		e.t.Fatalf("send %s %s (Host=%s) failed: %v", method, path, host, err)
	}
	if status != http.StatusOK {
		e.logBFEException()
		e.t.Fatalf("send %s %s (Host=%s): expected 200, got %d", method, path, host, status)
	}
	for _, b := range []*common.MockBackend{e.backendHit, e.backendDef} {
		if b != wantBackend && b.Hits() > before[b] {
			e.t.Fatalf("send %s %s (Host=%s): request wrongly routed to %s", method, path, host, b.ClusterName)
		}
	}
	if wantBackend.Hits() <= before[wantBackend] {
		e.t.Fatalf("send %s %s (Host=%s): request did not reach %s", method, path, host, wantBackend.ClusterName)
	}
}

// exprProductRules builds the expr_product rule pair: the probe condition
// plus an always-true fallback. The fallback uses the expr-native constant
// `true` rather than the legacy `default_t()` because ExprOnly intentionally
// rejects legacy primitive names (verified negative in TC-04(b)); `true`
// compiles in all three engine modes (LegacyFirst falls back to the expr
// engine for it).
func exprProductRules(cond string) map[string][]routeRule {
	return map[string][]routeRule{
		"expr_product": {
			{ClusterName: clusterHit, Cond: cond},
			{ClusterName: clusterDefault, Cond: "true"},
		},
	}
}

// TestTC01_ExprOnly verifies ExprOnly mode: new-style expressions compile
// and route by expression semantics; misses fall through to default_t().
func TestTC01_ExprOnly(t *testing.T) {
	e := newTestEnv(t, "ExprOnly",
		exprProductRules(`ReqMethod() in ["GET", "HEAD"] && ReqPath() startsWith "/api"`))
	defer e.Close()
	e.start()

	e.expectCluster(http.MethodGet, hostExpr, "/api/users", nil, clusterHit)
	e.expectClusterByBackend(http.MethodHead, hostExpr, "/api/status", nil, e.backendHit)
	e.expectCluster(http.MethodPost, hostExpr, "/api/users", nil, clusterDefault) // method not in set
	e.expectCluster(http.MethodGet, hostExpr, "/web/index", nil, clusterDefault)  // path prefix miss
	e.expectCluster(http.MethodGet, hostExpr, "/api", nil, clusterHit)            // exact prefix boundary
	e.expectCluster(http.MethodGet, hostExpr, "/apiary/list", nil, clusterHit)    // non-segment prefix hit
}

// TestTC02_ExprCompatible verifies ExprCompatible mode: legacy primitives
// work and can be mixed with new-style Env method calls in one expression.
func TestTC02_ExprCompatible(t *testing.T) {
	e := newTestEnv(t, "ExprCompatible",
		exprProductRules(`req_path_prefix_in("/api", false) && ReqHeader("X-Expr") == "on"`))
	defer e.Close()
	e.start()

	e.expectCluster(http.MethodGet, hostExpr, "/api/users", map[string]string{"X-Expr": "on"}, clusterHit)
	e.expectCluster(http.MethodGet, hostExpr, "/api/users", nil, clusterDefault)                                // header absent
	e.expectCluster(http.MethodGet, hostExpr, "/api/users", map[string]string{"X-Expr": "off"}, clusterDefault) // header mismatch
	e.expectCluster(http.MethodGet, hostExpr, "/web", map[string]string{"X-Expr": "on"}, clusterDefault)        // path prefix miss
}

// TestTC03_LegacyFirstDefault verifies the default mode (no explicit mode):
// legacy primitives keep exact legacy behavior (baseline), and new-style
// expressions fall back to the expr engine after legacy compilation fails.
func TestTC03_LegacyFirstDefault(t *testing.T) {
	rules := map[string][]routeRule{
		"legacy_product": {
			{ClusterName: clusterHit, Cond: `req_path_prefix_in("/api", false)`},
			{ClusterName: clusterDefault, Cond: "default_t()"},
		},
		"fallback_product": {
			{ClusterName: clusterHit, Cond: `ReqMethod() in ["GET"] && ReqPath() startsWith "/api"`},
			{ClusterName: clusterDefault, Cond: "default_t()"},
		},
	}
	e := newTestEnv(t, "", rules) // empty value = default LegacyFirst
	defer e.Close()
	e.start()

	// Baseline: legacy primitives unchanged under default mode.
	e.expectCluster(http.MethodGet, hostLegacy, "/api/users", nil, clusterHit)
	e.expectCluster(http.MethodGet, hostLegacy, "/web", nil, clusterDefault)
	// Fallback: new-style expression compiles via expr engine in LegacyFirst.
	e.expectCluster(http.MethodGet, hostFallback, "/api/users", nil, clusterHit)
	e.expectCluster(http.MethodPost, hostFallback, "/api/users", nil, clusterDefault)
	e.expectCluster(http.MethodGet, hostFallback, "/web", nil, clusterDefault)
}

// TestTC04_IllegalConfigFailFast verifies three kinds of illegal config all
// fail fast at startup, and that no request reaches the mock backends.
func TestTC04_IllegalConfigFailFast(t *testing.T) {
	cases := []struct {
		name string
		mode string
		cond string
	}{
		{"IllegalModeValue", "Bogus", "default_t()"},
		{"ExprOnlyLegacyPrimitive", "ExprOnly", `req_path_prefix_in("/api", false)`},
		{"ExprOnlyTypeError", "ExprOnly", `ReqMethod() && true`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEnv(t, tc.mode, exprProductRules(tc.cond))
			defer e.Close()
			e.startFailed()

			if n := e.backendHit.Hits(); n != 0 {
				t.Fatalf("backend %s received %d requests, want 0", clusterHit, n)
			}
			if n := e.backendDef.Hits(); n != 0 {
				t.Fatalf("backend %s received %d requests, want 0", clusterDefault, n)
			}
		})
	}
}

// TestTC05_NumericAndCaseInsensitive verifies ConditionEngineMode parsing of
// numeric forms (0/1/2) and case-insensitive strings behaves like the
// standard string forms.
func TestTC05_NumericAndCaseInsensitive(t *testing.T) {
	cases := []struct {
		name string
		mode string
	}{
		{"NumericExprOnly", "2"},
		{"NumericExprCompatible", "1"},
		{"NumericLegacyFirst", "0"},
		{"LowerCaseExprCompatible", "exprcompatible"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEnv(t, tc.mode,
				exprProductRules(`ReqMethod() in ["GET"] && ReqPath() startsWith "/api"`))
			defer e.Close()
			e.start()

			e.expectCluster(http.MethodGet, hostExpr, "/api/users", nil, clusterHit)
		})
	}
}
