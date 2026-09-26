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

package sc21

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost  = "mirror.example.org"
	apiPath  = "/v1/chat/completions"
	apiKey   = "ak_mirror"
	apiKeyId = "mirror_key_id"

	clusterPrimary    = "cluster_primary"
	clusterFallback   = "cluster_fallback"
	clusterMirror     = "cluster_mirror"
	clusterMirrorDead = "cluster_mirror_dead"

	quotaKeyTotal = "quota:plan_total"
)

var (
	requestBody = []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello mirror"}]}`)

	primaryAnswer = `{"choices":[{"index":0,"message":{"role":"assistant","content":"primary answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":8,"total_tokens":18}}`

	// mirrorAnswer carries a different usage on purpose: it proves the mirror
	// inference really happened, and (in the billing TC) that this usage is
	// never deducted from the customer quota.
	mirrorAnswer = `{"choices":[{"index":0,"message":{"role":"assistant","content":"mirror answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":99,"completion_tokens":77,"total_tokens":176}}`
)

// mirrorBodyRewriteConf / mirrorRuleConf / mirrorRuleData mirror the JSON
// schema of mod_traffic_mirror/mirror_rule.data.
type mirrorBodyRewriteConf struct {
	Path  string `json:"path"`
	Value string `json:"value"`
}

type mirrorRuleConf struct {
	Cond          string                   `json:"cond"`
	MirrorCluster string                   `json:"mirrorCluster"`
	Percentage    int                      `json:"percentage"`
	RemoveHeaders []string                 `json:"removeHeaders,omitempty"`
	SetHeaders    map[string]string        `json:"setHeaders,omitempty"`
	BodyRewrites  []*mirrorBodyRewriteConf `json:"bodyRewrites,omitempty"`
	PathRewrite   string                   `json:"pathRewrite,omitempty"`
}

type mirrorRuleData struct {
	Version string                       `json:"Version"`
	Config  map[string][]*mirrorRuleConf `json:"Config"`
}

// defaultMirrorRule returns the standard rule used by most TCs: match
// everything, mirror everything, strip the default sensitive headers.
func defaultMirrorRule() *mirrorRuleData {
	return mirrorRule("1.0", "default_t()", clusterMirror, 100)
}

func mirrorRule(version, cond, cluster string, percentage int) *mirrorRuleData {
	return &mirrorRuleData{
		Version: version,
		Config: map[string][]*mirrorRuleConf{
			"ai_product": {
				{
					Cond:          cond,
					MirrorCluster: cluster,
					Percentage:    percentage,
					RemoveHeaders: []string{"Authorization", "Cookie", "X-Api-Key"},
				},
			},
		},
	}
}

// testEnv holds all resources for a single SC21 integration test.
type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	primary    *common.MockBackend
	fallback   *common.MockBackend
	mirror     *common.MockBackend
	redis      *common.RedisServer
	tokenRule  *common.TokenRuleData

	confDir     string
	logDir      string
	bfePort     int
	monitorPort int
	stopBFE     func()
}

func newTestEnv(t *testing.T) *testEnv {
	e := &testEnv{t: t}

	e.primary = common.NewMockBackend(clusterPrimary, http.StatusOK, primaryAnswer)
	e.fallback = common.NewMockBackend(clusterFallback, http.StatusOK, `{"ok":true}`)
	e.mirror = common.NewMockBackend(clusterMirror, http.StatusOK, mirrorAnswer)
	e.redis = common.NewRedisServer(t)
	e.tokenRule = unlimitedTokenRule()

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	return e
}

// unlimitedTokenRule binds ak_mirror to an unlimited quota plan.
func unlimitedTokenRule() *common.TokenRuleData {
	return &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": {
				{
					Id:          "unlimited_plan",
					Unlimited:   true,
					PassNoQuota: false,
					RedisKey:    "quota:unlimited_plan",
					ExpiredTime: -1,
					Quota:       0,
					Unit:        "total_token",
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
					QuotaPlans:     []string{"unlimited_plan"},
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
}

// quotaTokenRule binds ak_mirror to a limited total_token plan so billing
// behavior can be observed through the quota balance.
func quotaTokenRule() *common.TokenRuleData {
	rule := unlimitedTokenRule()
	rule.QuotaPlans["ai_product"] = []common.QuotaPlan{
		{
			Id:          "plan_total",
			Unlimited:   false,
			PassNoQuota: false,
			RedisKey:    quotaKeyTotal,
			ExpiredTime: -1,
			Quota:       1000,
			Unit:        "total_token",
		},
	}
	tokens := rule.Tokens["ai_product"]
	tokens[apiKey] = common.TokenFile{
		Key:            apiKey,
		KeyId:          apiKeyId,
		Enabled:        true,
		ExpiredTime:    -1,
		UnlimitedQuota: false,
		QuotaPlans:     []string{"plan_total"},
	}
	rule.Tokens["ai_product"] = tokens
	return rule
}

// startBFE generates the config (with the given mirror rule data), applies
// the optional conf mutation (e.g. breaker thresholds) and starts BFE.
func (e *testEnv) startBFE(ruleData *mirrorRuleData, staticBackends map[string]string, confMutate func(confDir string)) {
	backends := map[string]*common.MockBackend{
		clusterPrimary:  e.primary,
		clusterFallback: e.fallback,
		clusterMirror:   e.mirror,
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:    "testdata",
		TargetConfDir:  e.confDir,
		Backends:       backends,
		StaticBackends: staticBackends,
		RedisAddr:      e.redis.Addr(),
		TokenRuleData:  e.tokenRule,
	}
	if err := builder.Build(); err != nil {
		e.t.Fatalf("build bfe config failed: %v", err)
	}

	if err := writeMirrorRuleData(e.confDir, ruleData); err != nil {
		e.t.Fatalf("write mirror_rule.data failed: %v", err)
	}

	// gslb.data must list exactly the clusters present in cluster_table /
	// cluster_conf (otherwise balTable init fails), so regenerate it from the
	// actual backend set of this test
	if err := writeGslbData(e.confDir, backends, staticBackends); err != nil {
		e.t.Fatalf("write gslb.data failed: %v", err)
	}

	if confMutate != nil {
		confMutate(e.confDir)
	}

	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
}

func writeMirrorRuleData(confDir string, ruleData *mirrorRuleData) error {
	path := filepath.Join(confDir, "mod_traffic_mirror", "mirror_rule.data")
	return common.WriteJSONFile(path, ruleData)
}

// writeGslbData regenerates cluster_conf/gslb.data listing exactly the given
// clusters, using the same sub-cluster naming as the config builder.
func writeGslbData(confDir string, backends map[string]*common.MockBackend, static map[string]string) error {
	clusters := map[string]interface{}{}
	for name := range backends {
		clusters[name] = map[string]interface{}{"GSLB_BLACKHOLE": 0, "sub_" + common.ClusterSubName(name): 100}
	}
	for name := range static {
		clusters[name] = map[string]interface{}{"GSLB_BLACKHOLE": 0, "sub_" + common.ClusterSubName(name): 100}
	}
	gslb := map[string]interface{}{
		"clusters": clusters,
		"hostname": "gslb-test",
		"ts":       "20260720150000",
	}
	return common.WriteJSONFile(filepath.Join(confDir, "cluster_conf", "gslb.data"), gslb)
}

// rewriteModTrafficMirrorConf rewrites one key in the generated
// mod_traffic_mirror.conf (e.g. CircuitBreakerFailThreshold).
func rewriteModTrafficMirrorConf(t *testing.T, confDir, key, value string) {
	t.Helper()
	path := filepath.Join(confDir, "mod_traffic_mirror", "mod_traffic_mirror.conf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read mod_traffic_mirror.conf failed: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), key) {
			lines[i] = key + " = " + value
			found = true
		}
	}
	if !found {
		t.Fatalf("key %s not found in mod_traffic_mirror.conf", key)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatalf("write mod_traffic_mirror.conf failed: %v", err)
	}
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	if e.primary != nil {
		e.primary.Close()
	}
	if e.fallback != nil {
		e.fallback.Close()
	}
	if e.mirror != nil {
		e.mirror.Close()
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
	if logData, err := os.ReadFile(logPath); err == nil && len(logData) > 0 {
		lines := strings.Split(string(logData), "\n")
		start := 0
		if len(lines) > 100 {
			start = len(lines) - 100
		}
		e.t.Logf("bfe log tail:\n%s", strings.Join(lines[start:], "\n"))
	}
}

// sendRequest posts one AI request through BFE. extraHeaders are added on
// top of the default Authorization/Content-Type headers.
func (e *testEnv) sendRequest(path string, body []byte, extraHeaders map[string]string) (*http.Response, string, error) {
	if path == "" {
		path = apiPath
	}
	u := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, path)
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
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

// expectOK fails the test unless the response is 200, dumping BFE logs.
func (e *testEnv) expectOK(resp *http.Response, body string, err error, step ...string) {
	label := "request"
	if len(step) > 0 {
		label = step[0]
	}
	if err != nil {
		e.logBFEException()
		e.t.Fatalf("%s: request failed: %v", label, err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		e.t.Fatalf("%s: expected 200, got %d, body: %s", label, resp.StatusCode, body)
	}
}

// waitQuota polls the quota balance until it equals the expected value.
func (e *testEnv) waitQuota(key string, expected int64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if e.redis.GetQuota(key) == expected {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// deadAddr returns a loopback address that is guaranteed to refuse
// connections (listen and immediately close).
func deadAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find dead addr failed: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// ---------------------------------------------------------------------------
// prometheus metric helpers
// ---------------------------------------------------------------------------

type promMetric struct {
	name   string
	labels map[string]string
	value  float64
}

func parsePromMetrics(data string) []promMetric {
	var out []promMetric
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, ' ')
		if i < 0 {
			continue
		}
		head, valStr := line[:i], strings.TrimSpace(line[i+1:])
		m := promMetric{labels: map[string]string{}}
		if j := strings.IndexByte(head, '{'); j >= 0 && strings.HasSuffix(head, "}") {
			m.name = head[:j]
			for _, kv := range strings.Split(head[j+1:len(head)-1], ",") {
				if kv == "" {
					continue
				}
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) != 2 {
					continue
				}
				m.labels[strings.TrimSpace(parts[0])] = strings.Trim(strings.TrimSpace(parts[1]), `"`)
			}
		} else {
			m.name = head
		}
		v, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		m.value = v
		out = append(out, m)
	}
	return out
}

// metricSum sums the values of all metrics with the given name whose labels
// contain every entry of want.
func metricSum(ms []promMetric, name string, want map[string]string) float64 {
	total := 0.0
	for _, m := range ms {
		if m.name != name {
			continue
		}
		match := true
		for k, v := range want {
			if m.labels[k] != v {
				match = false
				break
			}
		}
		if match {
			total += m.value
		}
	}
	return total
}

// mirrorMetrics fetches /monitor/mod_traffic_mirror.prometheus.
func (e *testEnv) mirrorMetrics() []promMetric {
	e.t.Helper()
	u := fmt.Sprintf("http://127.0.0.1:%d/monitor/mod_traffic_mirror.prometheus", e.monitorPort)
	resp, err := http.Get(u)
	if err != nil {
		e.t.Fatalf("get mod_traffic_mirror.prometheus failed: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return parsePromMetrics(string(data))
}

// waitMirrorMetric polls the prometheus endpoint until the summed value of
// the matching metrics satisfies cond.
func (e *testEnv) waitMirrorMetric(timeout time.Duration, name string, labels map[string]string, cond func(float64) bool) bool {
	e.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond(metricSum(e.mirrorMetrics(), name, labels)) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func eq(v float64) func(float64) bool {
	return func(got float64) bool { return got == v }
}

func ge(v float64) func(float64) bool {
	return func(got float64) bool { return got >= v }
}

// ---------------------------------------------------------------------------
// TC-01 基本命中与复制一致性
// ---------------------------------------------------------------------------

func TestTC01_BasicHitAndCopyConsistency(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(defaultMirrorRule(), nil, nil)

	resp, body, err := e.sendRequest("", requestBody, map[string]string{
		"Cookie":    "session=abc123",
		"X-Api-Key": "secret-x",
	})
	e.expectOK(resp, body, err)

	if !strings.Contains(body, "primary answer") {
		t.Fatalf("main response should come from the primary upstream, got: %s", body)
	}

	// main upstream received the untouched request
	if e.primary.Hits() != 1 {
		t.Fatalf("expected 1 primary hit, got %d", e.primary.Hits())
	}
	if models := e.primary.Models(); len(models) != 1 || models[0] != "gpt-4o" {
		t.Fatalf("primary should see model gpt-4o, got %v", models)
	}
	if auth := e.primary.AuthHeaders(); len(auth) != 1 || auth[0] != "Bearer "+apiKey {
		t.Fatalf("primary should keep the Authorization header, got %v", auth)
	}
	primaryHdr := e.primary.HeaderCopies()
	if len(primaryHdr) != 1 || primaryHdr[0].Get("X-Bfe-Mirror") != "" {
		t.Fatalf("primary request must not carry X-Bfe-Mirror, got %v", primaryHdr)
	}

	// mirror received an exact copy with sanitized headers
	if e.mirror.Hits() != 1 {
		t.Fatalf("expected 1 mirror hit, got %d", e.mirror.Hits())
	}
	mirrorBodies := e.mirror.RequestBodies()
	if len(mirrorBodies) != 1 || !bytes.Equal(mirrorBodies[0], requestBody) {
		t.Fatalf("mirror body should be an exact copy of the request, got %q", mirrorBodies)
	}
	if paths := e.mirror.URLPaths(); len(paths) != 1 || paths[0] != apiPath {
		t.Fatalf("mirror path should be %s, got %v", apiPath, paths)
	}
	if models := e.mirror.Models(); len(models) != 1 || models[0] != "gpt-4o" {
		t.Fatalf("mirror should see model gpt-4o, got %v", models)
	}
	mh := e.mirror.HeaderCopies()
	if len(mh) != 1 {
		t.Fatalf("expected 1 mirror header record, got %d", len(mh))
	}
	h := mh[0]
	if h.Get("Authorization") != "" || h.Get("Cookie") != "" || h.Get("X-Api-Key") != "" {
		t.Fatalf("sensitive headers must be stripped on the mirror copy, got auth=%q cookie=%q x-api-key=%q",
			h.Get("Authorization"), h.Get("Cookie"), h.Get("X-Api-Key"))
	}
	if h.Get("X-Bfe-Mirror") != "true" {
		t.Fatalf("mirror copy should carry X-Bfe-Mirror: true, got %q", h.Get("X-Bfe-Mirror"))
	}
	if h.Get("X-Bfe-Logid") == "" {
		t.Fatal("mirror copy should carry X-Bfe-Logid for cross-cluster reconciliation")
	}
	if ct := h.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("mirror copy should keep Content-Type, got %q", ct)
	}

	// prometheus: one labeled request and one 200 response status
	if !e.waitMirrorMetric(5*time.Second, "req_labeled_total",
		map[string]string{"product": "ai_product", "cluster": clusterMirror, "model": "gpt-4o"}, eq(1)) {
		t.Fatalf("req_labeled_total{cluster=%s,model=gpt-4o} should be 1", clusterMirror)
	}
	if !e.waitMirrorMetric(5*time.Second, "resp_status_total",
		map[string]string{"cluster": clusterMirror, "status": "200"}, eq(1)) {
		t.Fatal("resp_status_total{cluster=cluster_mirror,status=200} should be 1")
	}
}

// ---------------------------------------------------------------------------
// TC-02 body model 改写与路径改写
// ---------------------------------------------------------------------------

func TestTC02_ModelAndPathRewrite(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	rule := defaultMirrorRule()
	rule.Config["ai_product"][0].BodyRewrites = []*mirrorBodyRewriteConf{{Path: "model", Value: "shadow-v3"}}
	rule.Config["ai_product"][0].PathRewrite = "/v1/internal/chat/completions"
	rule.Config["ai_product"][0].SetHeaders = map[string]string{"X-Test-Flag": "yes"}

	e.startBFE(rule, nil, nil)

	resp, body, err := e.sendRequest(apiPath+"?trace=1", requestBody, nil)
	e.expectOK(resp, body, err)

	// primary keeps the original model, path and query
	if models := e.primary.Models(); len(models) != 1 || models[0] != "gpt-4o" {
		t.Fatalf("primary should see the original model gpt-4o, got %v", models)
	}
	if paths := e.primary.URLPaths(); len(paths) != 1 || paths[0] != apiPath {
		t.Fatalf("primary path should stay %s, got %v", apiPath, paths)
	}
	if qs := e.primary.RawQueries(); len(qs) != 1 || qs[0] != "trace=1" {
		t.Fatalf("primary query should stay trace=1, got %v", qs)
	}

	// mirror sees the rewritten model, path (query preserved) and set header
	if e.mirror.Hits() != 1 {
		t.Fatalf("expected 1 mirror hit, got %d", e.mirror.Hits())
	}
	if models := e.mirror.Models(); len(models) != 1 || models[0] != "shadow-v3" {
		t.Fatalf("mirror should see the rewritten model shadow-v3, got %v", models)
	}
	if paths := e.mirror.URLPaths(); len(paths) != 1 || paths[0] != "/v1/internal/chat/completions" {
		t.Fatalf("mirror path should be rewritten, got %v", paths)
	}
	if qs := e.mirror.RawQueries(); len(qs) != 1 || qs[0] != "trace=1" {
		t.Fatalf("mirror query should preserve trace=1, got %v", qs)
	}
	mirrorBodies := e.mirror.RequestBodies()
	if len(mirrorBodies) != 1 || !strings.Contains(string(mirrorBodies[0]), `"shadow-v3"`) ||
		!strings.Contains(string(mirrorBodies[0]), "hello mirror") {
		t.Fatalf("mirror body should rewrite only the model field, got %q", mirrorBodies)
	}
	if mh := e.mirror.HeaderCopies(); len(mh) != 1 || mh[0].Get("X-Test-Flag") != "yes" {
		t.Fatalf("mirror copy should carry the configured X-Test-Flag header, got %v", mh)
	}

	// the model label keeps the client-requested model, not the rewritten one
	if !e.waitMirrorMetric(5*time.Second, "req_labeled_total",
		map[string]string{"cluster": clusterMirror, "model": "gpt-4o"}, eq(1)) {
		t.Fatal("req_labeled_total{cluster=cluster_mirror,model=gpt-4o} should be 1")
	}
}

// ---------------------------------------------------------------------------
// TC-03 采样比例 0 不镜像
// ---------------------------------------------------------------------------

func TestTC03_SamplingZeroMirrorsNothing(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(mirrorRule("1.0", "default_t()", clusterMirror, 0), nil, nil)

	for i := 0; i < 5; i++ {
		e.expectOK(e.sendRequest("", requestBody, nil))
	}

	if e.mirror.Hits() != 0 {
		t.Fatalf("percentage=0 must not mirror anything, got %d mirror hits", e.mirror.Hits())
	}
	if !e.waitMirrorMetric(5*time.Second, "skip_total", map[string]string{"reason": "sample"}, eq(5)) {
		t.Fatal("skip_total{reason=sample} should be 5")
	}
	if !e.waitMirrorMetric(5*time.Second, "skip_sample_total", nil, eq(5)) {
		t.Fatal("skip_sample_total should be 5")
	}
	if got := metricSum(e.mirrorMetrics(), "req_labeled_total", nil); got != 0 {
		t.Fatalf("req_labeled_total should stay 0, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// TC-04 采样比例 100 全量镜像
// ---------------------------------------------------------------------------

func TestTC04_SamplingHundredMirrorsAll(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(mirrorRule("1.0", "default_t()", clusterMirror, 100), nil, nil)

	for i := 0; i < 5; i++ {
		e.expectOK(e.sendRequest("", requestBody, nil))
	}

	if e.mirror.Hits() != 5 {
		t.Fatalf("percentage=100 should mirror every request, got %d mirror hits", e.mirror.Hits())
	}
	if !e.waitMirrorMetric(5*time.Second, "req_labeled_total",
		map[string]string{"cluster": clusterMirror}, eq(5)) {
		t.Fatal("req_labeled_total{cluster=cluster_mirror} should be 5")
	}
	if !e.waitMirrorMetric(5*time.Second, "resp_status_total",
		map[string]string{"cluster": clusterMirror, "status": "200"}, eq(5)) {
		t.Fatal("resp_status_total{cluster=cluster_mirror,status=200} should be 5")
	}
	if got := metricSum(e.mirrorMetrics(), "skip_total", map[string]string{"reason": "sample"}); got != 0 {
		t.Fatalf("skip_total{reason=sample} should stay 0, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// TC-05 采样比例 50 统计分布
// ---------------------------------------------------------------------------

func TestTC05_SamplingFiftyStatistical(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(mirrorRule("1.0", "default_t()", clusterMirror, 50), nil, nil)

	const total = 200
	for i := 0; i < total; i++ {
		e.expectOK(e.sendRequest("", requestBody, nil))
	}

	hits := e.mirror.Hits()
	// expectation 100, stddev ~7.1; [60,140] is about +-5.6 sigma so the
	// probability of an accidental failure is negligible
	if hits < 60 || hits > 140 {
		t.Fatalf("percentage=50 over %d requests should mirror within [60,140], got %d", total, hits)
	}
	if !e.waitMirrorMetric(5*time.Second, "req_labeled_total",
		map[string]string{"cluster": clusterMirror}, func(v float64) bool { return v == float64(hits) }) {
		t.Fatal("req_labeled_total should equal the observed mirror hit count")
	}
	skipped := metricSum(e.mirrorMetrics(), "skip_total", map[string]string{"reason": "sample"})
	if int(skipped)+hits != total {
		t.Fatalf("skip_total(sample) + mirror hits should equal %d, got %v + %d", total, skipped, hits)
	}
}

// ---------------------------------------------------------------------------
// TC-06 条件不匹配不镜像
// ---------------------------------------------------------------------------

func TestTC06_CondMismatchMirrorsNothing(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(mirrorRule("1.0", `req_path_prefix_in("/v1/other", true)`, clusterMirror, 100), nil, nil)

	e.expectOK(e.sendRequest("", requestBody, nil))

	if e.primary.Hits() != 1 {
		t.Fatalf("primary should be hit once, got %d", e.primary.Hits())
	}
	if e.mirror.Hits() != 0 {
		t.Fatalf("a non-matching cond must not produce mirror traffic, got %d hits", e.mirror.Hits())
	}
	// no mirror metrics at all (neither requests nor skips)
	time.Sleep(300 * time.Millisecond)
	if got := metricSum(e.mirrorMetrics(), "req_labeled_total", nil); got != 0 {
		t.Fatalf("req_labeled_total should stay 0, got %v", got)
	}
	if got := metricSum(e.mirrorMetrics(), "skip_total", nil); got != 0 {
		t.Fatalf("skip_total should stay 0 on cond mismatch, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// TC-07 fallback 重试去重
// ---------------------------------------------------------------------------

func TestTC07_FallbackRetryMirroredOnce(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	// primary fails on the first hit to force the AI fallback
	e.primary.ResponseFunc = func(r *http.Request, count int) (int, string) {
		if count == 1 {
			return http.StatusServiceUnavailable, `{"error":"primary down"}`
		}
		return http.StatusOK, primaryAnswer
	}

	e.startBFE(defaultMirrorRule(), nil, nil)

	resp, body, err := e.sendRequest("", requestBody, nil)
	e.expectOK(resp, body, err, "fallback request")

	if e.primary.Hits() != 1 {
		t.Fatalf("expected 1 primary attempt, got %d", e.primary.Hits())
	}
	if e.fallback.Hits() != 1 {
		t.Fatalf("expected 1 fallback attempt, got %d", e.fallback.Hits())
	}
	if e.mirror.Hits() != 1 {
		t.Fatalf("the same client request must be mirrored exactly once across fallback retries, got %d", e.mirror.Hits())
	}
	if !e.waitMirrorMetric(5*time.Second, "req_labeled_total",
		map[string]string{"cluster": clusterMirror}, eq(1)) {
		t.Fatal("req_labeled_total{cluster=cluster_mirror} should be 1")
	}
	mirrorBodies := e.mirror.RequestBodies()
	if len(mirrorBodies) != 1 || !bytes.Equal(mirrorBodies[0], requestBody) {
		t.Fatalf("mirror body should be an exact copy of the request, got %q", mirrorBodies)
	}
}

// ---------------------------------------------------------------------------
// TC-08 SSE 读空与 usage/finish_reason 解析
// ---------------------------------------------------------------------------

func TestTC08_SSEDrainAndUsageParse(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.mirror.SSEEvents = []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"hi"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"!"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":5,"total_tokens":12}}`,
		`[DONE]`,
	}

	e.startBFE(defaultMirrorRule(), nil, nil)

	streamBody := []byte(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hello mirror"}]}`)
	e.expectOK(e.sendRequest("", streamBody, nil))

	if e.mirror.Hits() != 1 {
		t.Fatalf("expected 1 mirror hit, got %d", e.mirror.Hits())
	}
	if !e.waitMirrorMetric(5*time.Second, "resp_status_total",
		map[string]string{"cluster": clusterMirror, "status": "200"}, eq(1)) {
		t.Fatal("resp_status_total{cluster=cluster_mirror,status=200} should be 1")
	}
	if !e.waitMirrorMetric(5*time.Second, "finish_reason_total",
		map[string]string{"cluster": clusterMirror, "reason": "stop"}, eq(1)) {
		t.Fatal("finish_reason_total{cluster=cluster_mirror,reason=stop} should be 1")
	}
	if !e.waitMirrorMetric(5*time.Second, "tokens_total",
		map[string]string{"cluster": clusterMirror, "kind": "prompt"}, eq(7)) {
		t.Fatal("tokens_total{cluster=cluster_mirror,kind=prompt} should be 7")
	}
	if !e.waitMirrorMetric(5*time.Second, "tokens_total",
		map[string]string{"cluster": clusterMirror, "kind": "completion"}, eq(5)) {
		t.Fatal("tokens_total{cluster=cluster_mirror,kind=completion} should be 5")
	}
	if got := metricSum(e.mirrorMetrics(), "fail_total",
		map[string]string{"cluster": clusterMirror}); got != 0 {
		t.Fatalf("a fully drained SSE response must not count as failure, got fail_total=%v", got)
	}
}

// ---------------------------------------------------------------------------
// TC-09 镜像错误分类
// ---------------------------------------------------------------------------

func TestTC09_MirrorErrorClassification(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.mirror.ResponseFunc = func(r *http.Request, count int) (int, string) {
		return http.StatusTooManyRequests,
			`{"error":{"message":"too many requests","type":"rate_limit_exceeded","code":"rpm_limit"}}`
	}

	e.startBFE(defaultMirrorRule(), nil, nil)

	resp, body, err := e.sendRequest("", requestBody, nil)
	e.expectOK(resp, body, err, "main request")

	if e.mirror.Hits() != 1 {
		t.Fatalf("expected 1 mirror hit, got %d", e.mirror.Hits())
	}
	if !e.waitMirrorMetric(5*time.Second, "resp_status_total",
		map[string]string{"cluster": clusterMirror, "status": "429"}, eq(1)) {
		t.Fatal("resp_status_total{cluster=cluster_mirror,status=429} should be 1")
	}
	if !e.waitMirrorMetric(5*time.Second, "error_type_total",
		map[string]string{"cluster": clusterMirror, "type": "rate_limit_exceeded", "code": "rpm_limit"}, eq(1)) {
		t.Fatal("error_type_total{cluster=cluster_mirror,type=rate_limit_exceeded,code=rpm_limit} should be 1")
	}
	if got := metricSum(e.mirrorMetrics(), "fail_total",
		map[string]string{"cluster": clusterMirror}); got != 0 {
		t.Fatalf("a received (and drained) 429 is not a send failure, got fail_total=%v", got)
	}
}

// ---------------------------------------------------------------------------
// TC-10 熔断与冷却
// ---------------------------------------------------------------------------

func TestTC10_CircuitBreakerAndCooldown(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	static := map[string]string{clusterMirrorDead: deadAddr(t)}
	confMutate := func(confDir string) {
		rewriteModTrafficMirrorConf(t, confDir, "CircuitBreakerFailThreshold", "3")
		rewriteModTrafficMirrorConf(t, confDir, "CircuitBreakerCooldownSec", "2")
	}

	e.startBFE(mirrorRule("1.0", "default_t()", clusterMirrorDead, 100), static, confMutate)

	failLabels := map[string]string{"cluster": clusterMirrorDead, "reason": "send"}

	// phase 1: three consecutive failures open the circuit
	for i := 0; i < 3; i++ {
		resp, body, err := e.sendRequest("", requestBody, nil)
		e.expectOK(resp, body, err, fmt.Sprintf("phase1 request %d", i+1))
	}
	if !e.waitMirrorMetric(5*time.Second, "fail_total", failLabels, eq(3)) {
		t.Fatalf("fail_total{cluster=%s,reason=send} should be 3, got %v",
			clusterMirrorDead, metricSum(e.mirrorMetrics(), "fail_total", failLabels))
	}

	// phase 2: inside the cooldown window new tasks are dropped, not sent
	for i := 0; i < 2; i++ {
		resp, body, err := e.sendRequest("", requestBody, nil)
		e.expectOK(resp, body, err, fmt.Sprintf("phase2 request %d", i+1))
	}
	if !e.waitMirrorMetric(5*time.Second, "circuit_open_total",
		map[string]string{"cluster": clusterMirrorDead}, eq(2)) {
		t.Fatal("circuit_open_total{cluster=cluster_mirror_dead} should be 2")
	}
	if got := metricSum(e.mirrorMetrics(), "fail_total", failLabels); got != 3 {
		t.Fatalf("fail_total should stay 3 while the circuit drops tasks, got %v", got)
	}

	// phase 3: after the cooldown a probe request goes through and fails again
	time.Sleep(2500 * time.Millisecond)
	resp, body, err := e.sendRequest("", requestBody, nil)
	e.expectOK(resp, body, err, "phase3 probe request")
	if !e.waitMirrorMetric(5*time.Second, "fail_total", failLabels, eq(4)) {
		t.Fatalf("fail_total should be 4 after the cooldown probe, got %v",
			metricSum(e.mirrorMetrics(), "fail_total", failLabels))
	}

	// the main path never noticed the dead mirror target
	if e.primary.Hits() != 6 {
		t.Fatalf("all 6 requests should reach the primary, got %d", e.primary.Hits())
	}
	if got := metricSum(e.mirrorMetrics(), "req_labeled_total", nil); got != 0 {
		t.Fatalf("no mirror request should ever complete, got req_labeled_total=%v", got)
	}
}

// ---------------------------------------------------------------------------
// TC-11 响应截断
// ---------------------------------------------------------------------------

func TestTC11_ResponseTruncation(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.mirror.ResponseFunc = func(r *http.Request, count int) (int, string) {
		return http.StatusOK, strings.Repeat("x", 256*1024)
	}
	confMutate := func(confDir string) {
		rewriteModTrafficMirrorConf(t, confDir, "MaxResponseBodyBytes", "65536")
	}

	e.startBFE(defaultMirrorRule(), nil, confMutate)

	e.expectOK(e.sendRequest("", requestBody, nil))

	if !e.waitMirrorMetric(5*time.Second, "resp_truncated_total",
		map[string]string{"cluster": clusterMirror}, eq(1)) {
		t.Fatal("resp_truncated_total{cluster=cluster_mirror} should be 1")
	}
	if !e.waitMirrorMetric(5*time.Second, "resp_status_total",
		map[string]string{"cluster": clusterMirror, "status": "200"}, eq(1)) {
		t.Fatal("resp_status_total{cluster=cluster_mirror,status=200} should be 1")
	}
	if got := metricSum(e.mirrorMetrics(), "fail_total",
		map[string]string{"cluster": clusterMirror}); got != 0 {
		t.Fatalf("truncation must not count as failure, got fail_total=%v", got)
	}
}

// ---------------------------------------------------------------------------
// TC-12 客户端断连镜像继续读完
// ---------------------------------------------------------------------------

func TestTC12_ClientAbortMirrorCompletes(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	// primary: write the first SSE frame, then block (long inference)
	primaryHold := make(chan struct{})
	e.primary.SSEEvents = []string{
		`{"choices":[{"index":0,"delta":{"content":"partial"}}]}`,
	}
	e.primary.SSEHold = primaryHold

	// mirror: hold the response until the client has aborted, so a completed
	// mirror strictly proves the mirror outlives the client abort
	mirrorHold := make(chan struct{})
	e.mirror.HoldResponse = mirrorHold

	e.startBFE(defaultMirrorRule(), nil, nil)

	streamBody := []byte(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hello mirror"}]}`)
	u := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(streamBody))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		e.logBFEException()
		t.Fatalf("stream request failed: %v", err)
	}

	// read the first SSE frame, then abort the connection
	reader := bufio.NewReader(resp.Body)
	firstLine, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(firstLine, "data:") {
		e.logBFEException()
		t.Fatalf("expected the first SSE frame, got %q, err=%v", firstLine, err)
	}
	_ = resp.Body.Close()

	// client is gone: release the mirror target; the mirror must still complete
	close(mirrorHold)

	if !e.waitMirrorMetric(5*time.Second, "req_labeled_total",
		map[string]string{"cluster": clusterMirror}, eq(1)) {
		t.Fatal("mirror must complete after the client abort (req_labeled_total should be 1)")
	}
	if !e.waitMirrorMetric(5*time.Second, "resp_status_total",
		map[string]string{"cluster": clusterMirror, "status": "200"}, eq(1)) {
		t.Fatal("resp_status_total{cluster=cluster_mirror,status=200} should be 1")
	}
	if e.mirror.Hits() != 1 {
		t.Fatalf("mirror target should receive the request once, got %d", e.mirror.Hits())
	}
	if bodies := e.mirror.RequestBodies(); len(bodies) != 1 || !bytes.Equal(bodies[0], streamBody) {
		t.Fatalf("mirror should receive the full request body, got %q", bodies)
	}

	// let the primary finish its held stream for a clean shutdown
	close(primaryHold)
}

// ---------------------------------------------------------------------------
// TC-13 访问日志镜像字段
// ---------------------------------------------------------------------------

func TestTC13_AccessLogMirrorFields(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(defaultMirrorRule(), nil, nil)

	e.expectOK(e.sendRequest("", requestBody, nil))

	// wait until the mirror has fully completed before reading the log
	if !e.waitMirrorMetric(5*time.Second, "req_labeled_total",
		map[string]string{"cluster": clusterMirror}, eq(1)) {
		t.Fatal("mirror should complete before checking the access log")
	}

	records, err := common.ParseAccessLog(e.logDir, 5*time.Second)
	if err != nil {
		e.logBFEException()
		t.Fatalf("parse access log failed: %v", err)
	}
	var mirrorRecord interface {
		GetMirrorHit() bool
		GetMirrorCluster() string
		GetMirrorStatus() int32
	}
	found := false
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].GetMirrorHit() {
			mirrorRecord = records[i]
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("access log should contain a record with mirror_hit=true, got %d records", len(records))
	}
	if mirrorRecord.GetMirrorCluster() != clusterMirror {
		t.Fatalf("mirror_cluster should be %s, got %q", clusterMirror, mirrorRecord.GetMirrorCluster())
	}
	if mirrorRecord.GetMirrorStatus() != 0 {
		t.Fatalf("async mirror_status must not be written back to the access log, got %d",
			mirrorRecord.GetMirrorStatus())
	}
}

// ---------------------------------------------------------------------------
// TC-14 规则热加载
// ---------------------------------------------------------------------------

func TestTC14_RuleHotReload(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(mirrorRule("1.0", "default_t()", clusterMirror, 0), nil, nil)

	// p=0: nothing mirrored
	e.expectOK(e.sendRequest("", requestBody, nil))
	if e.mirror.Hits() != 0 {
		t.Fatalf("percentage=0 must not mirror, got %d hits", e.mirror.Hits())
	}
	if !e.waitMirrorMetric(5*time.Second, "skip_total", map[string]string{"reason": "sample"}, ge(1)) {
		t.Fatal("skip_total{reason=sample} should be >= 1")
	}

	// hot reload with p=100
	reloaded := mirrorRule("2.0", "default_t()", clusterMirror, 100)
	if err := writeMirrorRuleData(e.confDir, reloaded); err != nil {
		t.Fatalf("rewrite mirror_rule.data failed: %v", err)
	}
	rulePath := filepath.Join(e.confDir, "mod_traffic_mirror", "mirror_rule.data")
	reloadURL := fmt.Sprintf("http://127.0.0.1:%d/reload/mod_traffic_mirror?path=%s",
		e.monitorPort, url.QueryEscape(rulePath))
	reloadResp, err := http.Get(reloadURL)
	if err != nil {
		t.Fatalf("call reload mod_traffic_mirror failed: %v", err)
	}
	defer reloadResp.Body.Close()
	reloadBody, _ := io.ReadAll(reloadResp.Body)
	if reloadResp.StatusCode != http.StatusOK {
		t.Fatalf("reload should return 200, got %d, body: %s", reloadResp.StatusCode, reloadBody)
	}
	if !strings.Contains(string(reloadBody), "mirror_rule.data=2.0") {
		t.Fatalf("reload response should contain mirror_rule.data=2.0, got: %s", reloadBody)
	}

	// the same request shape is now mirrored
	e.expectOK(e.sendRequest("", requestBody, nil))
	if e.mirror.Hits() != 1 {
		t.Fatalf("percentage=100 after reload should mirror, got %d hits", e.mirror.Hits())
	}
	if !e.waitMirrorMetric(5*time.Second, "req_labeled_total",
		map[string]string{"cluster": clusterMirror}, eq(1)) {
		t.Fatal("req_labeled_total{cluster=cluster_mirror} should be 1 after reload")
	}
}

// ---------------------------------------------------------------------------
// TC-15 计费隔离
// ---------------------------------------------------------------------------

func TestTC15_BillingIsolation(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.tokenRule = quotaTokenRule()
	e.startBFE(defaultMirrorRule(), nil, nil)
	e.redis.SetQuota(quotaKeyTotal, 1000)

	e.expectOK(e.sendRequest("", requestBody, nil))

	// the main request deducts prompt(10) + completion(8) = 18
	if !e.waitQuota(quotaKeyTotal, 982, 5*time.Second) {
		t.Fatalf("main request should deduct 18 tokens, quota: %d", e.redis.GetQuota(quotaKeyTotal))
	}

	// the mirror really happened and recorded its own usage (99 + 77)
	if !e.waitMirrorMetric(5*time.Second, "req_labeled_total",
		map[string]string{"cluster": clusterMirror}, eq(1)) {
		t.Fatal("mirror should complete")
	}
	if !e.waitMirrorMetric(5*time.Second, "tokens_total",
		map[string]string{"cluster": clusterMirror, "kind": "completion"}, eq(77)) {
		t.Fatal("tokens_total{cluster=cluster_mirror,kind=completions} should be 77")
	}

	// but the mirror usage must not be deducted from the customer quota
	time.Sleep(500 * time.Millisecond)
	if got := e.redis.GetQuota(quotaKeyTotal); got != 982 {
		t.Fatalf("mirror traffic must not deduct quota, expected 982, got %d", got)
	}
	if e.primary.Hits() != 1 || e.mirror.Hits() != 1 {
		t.Fatalf("primary and mirror should each be hit once, got %d/%d",
			e.primary.Hits(), e.mirror.Hits())
	}
}
