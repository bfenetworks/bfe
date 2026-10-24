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

package sc32

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	weekdayHost    = "weekday.example.org"
	weekdayUTCHost = "weekday-utc.example.org"
	apiPath        = "/v1/chat/completions"
	apiKeyUserA    = "ak_user_a"

	clusterWork       = "cluster_weekday_work"
	clusterSat        = "cluster_weekday_sat"
	clusterSun        = "cluster_weekday_sun"
	clusterUTCWork    = "cluster_weekday_utc_work"
	clusterUTCDefault = "cluster_weekday_utc_default"
	clusterDefault    = "cluster_weekday_default"

	// debugTimeHeader mocks the current time for condition primitives; the
	// value format is yyyymmddhhmmssZ where Z is a military time zone letter
	// ("H" = UTC+8, "Z" = UTC).
	debugTimeHeader = "X-Bfe-Debug-Time"

	// beijingOffset is the UTC+8 offset of the "H" military time zone.
	beijingOffset = 8 * 3600
)

var (
	// clusterNames lists every cluster of this scenario. All of them get a
	// mock backend, which is what makes BFEConfigBuilder generate matching
	// cluster_table.data entries.
	clusterNames = []string{
		clusterWork,
		clusterSat,
		clusterSun,
		clusterUTCWork,
		clusterUTCDefault,
		clusterDefault,
	}

	// beijing is the UTC+8 location used to derive the expected weekday of the
	// real-system-clock arm.
	beijing = time.FixedZone("UTC+8", beijingOffset)

	requestBody = []byte(`{"model":"m","messages":[]}`)
)

// testEnv holds all resources for a single SC32 integration test.
type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backends   map[string]*common.MockBackend
	bfePort    int
	stopBFE    func()
}

func newTestEnv(t *testing.T) *testEnv {
	e := &testEnv{
		t:        t,
		backends: make(map[string]*common.MockBackend),
	}

	for _, name := range clusterNames {
		e.backends[name] = common.NewMockBackend(name, http.StatusOK, "response from "+name)
	}

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	confDir := filepath.Join(e.processEnv.WorkDir(), "conf")
	logDir := filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: confDir,
		Backends:      e.backends,
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

// sendRequest posts one chat-completions request with the given Host and API
// key. debugTime is sent as the X-Bfe-Debug-Time header when non-empty and
// omitted otherwise (so the BFE process system clock is used).
func (e *testEnv) sendRequest(host, apiKey, debugTime string) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, "", err
	}
	req.Host = host
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	if debugTime != "" {
		req.Header.Set(debugTimeHeader, debugTime)
	}

	client := &http.Client{Timeout: 30 * time.Second}
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

// hitSnapshot records the current per-cluster hit counters.
func (e *testEnv) hitSnapshot() map[string]int {
	snapshot := make(map[string]int, len(clusterNames))
	for _, name := range clusterNames {
		snapshot[name] = e.backends[name].Hits()
	}
	return snapshot
}

// assertRoutedTo verifies that exactly wantCluster received one request since
// before and that no other cluster of the scenario was touched.
func (e *testEnv) assertRoutedTo(before map[string]int, wantCluster, label string) {
	e.t.Helper()
	for _, name := range clusterNames {
		delta := e.backends[name].Hits() - before[name]
		if name == wantCluster {
			if delta != 1 {
				e.t.Fatalf("%s: expected %s hit once, got %d", label, name, delta)
			}
			continue
		}
		if delta != 0 {
			e.t.Fatalf("%s: expected %s not hit, got %d", label, name, delta)
		}
	}
}

// routeArm sends one request with a mocked clock and asserts it is served by
// exactly wantCluster (200, response body of that cluster's mock backend).
func (e *testEnv) routeArm(host, debugTime, wantCluster, label string) {
	e.t.Helper()
	before := e.hitSnapshot()

	resp, body, err := e.sendRequest(host, apiKeyUserA, debugTime)
	if err != nil {
		e.t.Fatalf("%s: send request failed: %v", label, err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		e.t.Fatalf("%s: expected status 200, got %d, body: %s", label, resp.StatusCode, body)
	}
	if strings.Contains(body, "AI route not found") {
		e.t.Fatalf("%s: request did not match any route rule, body: %s", label, body)
	}
	if !strings.Contains(body, wantCluster) {
		e.t.Fatalf("%s: expected response from %s, got body: %s", label, wantCluster, body)
	}

	e.assertRoutedTo(before, wantCluster, label)
	e.t.Logf("%s: host=%s %s=%s -> %s", label, host, debugTimeHeader, debugTime, wantCluster)
}

// clusterForWeekday maps a Go weekday to the cluster expected from the
// weekday.example.org rules. time.Weekday uses the same numbering as the
// time_weekday_in primitive (0=Sunday .. 6=Saturday) and the rules judge in
// UTC+8 ("H").
func clusterForWeekday(wd time.Weekday) string {
	switch wd {
	case time.Saturday:
		return clusterSat
	case time.Sunday:
		return clusterSun
	default:
		return clusterWork
	}
}

// assertRealClockArm drives the "no X-Bfe-Debug-Time" path: BFE must judge
// with its process system clock. The expected cluster is derived from
// time.Now() in UTC+8. The weekday is sampled before and after the request to
// survive the midnight race of a real clock: if it changed, the request is
// retried (and the expectation recomputed) instead of failing the test.
func (e *testEnv) assertRealClockArm() {
	const maxAttempts = 3
	const label = "step 8 (real system clock, no debug header)"

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		before := e.hitSnapshot()
		weekdayBefore := time.Now().In(beijing).Weekday()

		resp, body, err := e.sendRequest(weekdayHost, apiKeyUserA, "")
		if err != nil {
			e.t.Fatalf("%s: send request failed: %v", label, err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			e.t.Fatalf("%s: expected status 200, got %d, body: %s", label, resp.StatusCode, body)
		}

		weekdayAfter := time.Now().In(beijing).Weekday()
		if weekdayBefore != weekdayAfter {
			e.t.Logf("%s: UTC+8 weekday changed during the request (%v -> %v), retrying (attempt %d)",
				label, weekdayBefore, weekdayAfter, attempt)
			continue
		}

		want := clusterForWeekday(weekdayAfter)
		if !strings.Contains(body, want) {
			e.t.Fatalf("%s: expected response from %s, got body: %s", label, want, body)
		}
		e.assertRoutedTo(before, want, label)
		e.t.Logf("%s: UTC+8 weekday %v -> %s", label, weekdayAfter, want)
		return
	}

	e.t.Fatalf("%s: UTC+8 weekday kept changing across %d attempts", label, maxAttempts)
}

// TestTC01_WeekdayNumberingAndSetHit locks the weekday numbering (0=Sunday,
// 6=Saturday) and the set semantics of time_weekday_in by walking a full week
// of mocked times over the three mutually exclusive and exhaustive rules of
// host weekday.example.org, plus one arm with the real system clock.
func TestTC01_WeekdayNumberingAndSetHit(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	// 2019-02-03 is a Sunday, so 2019-02-04 is a Monday and the following days
	// shift by one. All times are Beijing time ("H").
	arms := []struct {
		label     string
		debugTime string
		want      string
	}{
		{"step 1 (Monday 20:30)", "20190204203000H", clusterWork},
		{"step 2 (Tuesday 20:30)", "20190205203000H", clusterWork},
		{"step 3 (Wednesday 20:30)", "20190206203000H", clusterWork},
		{"step 4 (Thursday 20:30)", "20190207203000H", clusterWork},
		{"step 5 (Friday 20:30)", "20190208203000H", clusterWork},
		{"step 6 (Saturday 20:30)", "20190209203000H", clusterSat},
		{"step 7 (Sunday 20:30)", "20190203203000H", clusterSun},
	}
	for _, arm := range arms {
		e.routeArm(weekdayHost, arm.debugTime, arm.want, arm.label)
	}

	e.assertRealClockArm()
}

// TestTC02_TimeZoneSemanticsAndDefaultUTC locks the time zone semantics of
// time_weekday_in: "H" (UTC+8) is judged in UTC+8, an omitted zone is judged
// in UTC, and a non-matching rule falls through to the next rule of the table.
func TestTC02_TimeZoneSemanticsAndDefaultUTC(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	// Beijing Monday 07:59:59 is still UTC Sunday 23:59:59; Beijing Monday
	// 08:00:00 is exactly UTC Monday 00:00:00, so the two tz arms diverge on
	// the first time and converge on the second.
	e.routeArm(weekdayHost, "20190204075959H", clusterWork,
		"step 1 (divergence: \"H\" arm is Monday)")
	e.routeArm(weekdayUTCHost, "20190204075959H", clusterUTCDefault,
		"step 2 (divergence: omitted zone is UTC Sunday, falls through)")
	e.routeArm(weekdayUTCHost, "20190204080000H", clusterUTCWork,
		"step 3 (convergence: UTC arm crossed midnight)")
	e.routeArm(weekdayHost, "20190204080000H", clusterWork,
		"step 4 (convergence: \"H\" arm is still Monday)")
}

// TestTC03_InvalidArgsFailConfigLoad locks the fail-fast behavior of
// time_weekday_in: illegal arguments must make the configuration load fail
// with a diagnosable message (primitive name, illegal value, rule text) so
// that a broken rule can never be loaded and silently never match.
func TestTC03_InvalidArgsFailConfigLoad(t *testing.T) {
	processEnv := common.NewProcessEnv(t)
	processEnv.Build()

	// The illegal expressions are injected into the first rule of the single
	// route table; every other part of the config stays identical to the
	// baseline arm that must pass validation.
	arms := []struct {
		name     string
		cond     string
		wantErrs []string
	}{
		{
			name: "baseline",
			cond: `req_host_in("weekday.example.org") && time_weekday_in("1|2|3|4|5", "H")`,
		},
		{
			name: "invalid-weekday",
			cond: `req_host_in("weekday.example.org") && time_weekday_in("7", "H")`,
			wantErrs: []string{
				`time_weekday_in "7": invalid weekday:7, should be an integer in [0, 6]`,
			},
		},
		{
			name: "invalid-zone",
			cond: `req_host_in("weekday.example.org") && time_weekday_in("0", "QQ")`,
			wantErrs: []string{
				`time_weekday_in "0": invalid zone:QQ`,
			},
		},
		{
			name: "arg-type",
			cond: `req_host_in("weekday.example.org") && time_weekday_in(0)`,
			wantErrs: []string{
				`primitive time_weekday_in arg 0 expect STRING, got INT`,
			},
		},
		{
			name: "arg-count",
			cond: `time_weekday_in("0", "H", "Z")`,
			wantErrs: []string{
				`primitive args len error, expect 2, got 3`,
			},
		},
	}

	for i, arm := range arms {
		confDir := filepath.Join(processEnv.WorkDir(), fmt.Sprintf("conf-%d-%s", i, arm.name))
		logDir := filepath.Join(processEnv.WorkDir(), fmt.Sprintf("log-%d-%s", i, arm.name))

		builder := &common.BFEConfigBuilder{
			TemplateDir:    "testdata",
			TargetConfDir:  confDir,
			StaticBackends: staticBackends(),
		}
		if err := builder.Build(); err != nil {
			t.Fatalf("%s: build bfe config failed: %v", arm.name, err)
		}
		rewriteConfPorts(t, filepath.Join(confDir, "bfe.conf"))
		setRuleCond(t, filepath.Join(confDir, "mod_ai_route", "ai_route.data"), arm.cond)

		if err := os.MkdirAll(logDir, 0755); err != nil {
			t.Fatalf("%s: create log dir failed: %v", arm.name, err)
		}

		exitCode, output := runBFETestConf(processEnv.BinaryPath(), confDir, logDir)

		if len(arm.wantErrs) == 0 {
			if exitCode != 0 {
				t.Fatalf("%s: exit code = %d, want 0, output:\n%s", arm.name, exitCode, output)
			}
			if !strings.Contains(output, "test is successful") {
				t.Fatalf("%s: output does not report a successful configuration test:\n%s", arm.name, output)
			}
			t.Logf("%s: exit code 0, config test successful", arm.name)
			continue
		}

		if exitCode == 0 {
			t.Fatalf("%s: expected the configuration test to fail (cond %s), got exit code 0, output:\n%s",
				arm.name, arm.cond, output)
		}
		if !strings.Contains(output, arm.cond) {
			t.Fatalf("%s: output does not contain the rule text %q:\n%s", arm.name, arm.cond, output)
		}
		for _, want := range arm.wantErrs {
			if !strings.Contains(output, want) {
				t.Fatalf("%s: output does not contain %q:\n%s", arm.name, want, output)
			}
		}
		t.Logf("%s: exit code %d, error message present", arm.name, exitCode)
	}
}

// staticBackends returns dummy backend addresses for every cluster of the
// scenario. TC-03 only validates the configuration, so no real backend is
// needed, but cluster_table.data still has to cover every cluster.
func staticBackends() map[string]string {
	backends := make(map[string]string, len(clusterNames))
	for _, name := range clusterNames {
		backends[name] = "127.0.0.1:1"
	}
	return backends
}

// rewriteConfPorts gives bfe.conf a port set that passes BFE's own config
// validation. BFEConfigBuilder rewrites monitorPort to 0 ("unset"), which
// BfeConfigLoad rejects with "MonitorPort[0] should be in [1, 65535]"; "-t"
// never binds the listeners, but the values still have to be in range.
func rewriteConfPorts(t *testing.T, confPath string) {
	t.Helper()

	ports := make([]int, 3)
	for i := range ports {
		port, err := common.FindFreePort()
		if err != nil {
			t.Fatalf("find free port failed: %v", err)
		}
		ports[i] = port
	}
	if err := common.RewriteBFEPorts(confPath, ports[0], ports[1], ports[2]); err != nil {
		t.Fatalf("rewrite bfe ports in %s failed: %v", confPath, err)
	}
}

// setRuleCond rewrites the Cond of the first rule ("weekday-work") in
// ai_route.data, so the surrounding configuration of an arm stays exactly the
// baseline one.
func setRuleCond(t *testing.T, path, cond string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}

	var file map[string]interface{}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("parse %s failed: %v", path, err)
	}

	routeRules, ok := file["route_rules"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: route_rules missing", path)
	}
	table, ok := routeRules["apikey_ak_user_a"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: route table apikey_ak_user_a missing", path)
	}
	rules, ok := table["rules"].([]interface{})
	if !ok || len(rules) == 0 {
		t.Fatalf("%s: route table apikey_ak_user_a has no rules", path)
	}
	rule, ok := rules[0].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: first rule is not an object", path)
	}
	rule["Cond"] = cond

	out, err := json.MarshalIndent(file, "", "    ")
	if err != nil {
		t.Fatalf("marshal %s failed: %v", path, err)
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		t.Fatalf("write %s failed: %v", path, err)
	}
}

// runBFETestConf runs the built bfe binary with "-t" (test configuration and
// exit) and returns the exit code with the combined output. With -t BFE logs
// to stdout as well, so the failure details are part of the output.
func runBFETestConf(binaryPath, confDir, logDir string) (int, string) {
	cmd := exec.Command(binaryPath, "-c", confDir, "-l", logDir, "-t")
	cmd.Dir = confDir

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err == nil {
		return 0, out.String()
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), out.String()
	}
	return -1, out.String() + fmt.Sprintf("\n[run bfe failed: %v]", err)
}
