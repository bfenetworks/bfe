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

// SC49 访问日志 Header 白名单采集与动态配置热加载（bfenetworks/bfe#1506）。
//
// 测试设计文档见 ../../测试设计文档/scenario-SC49-访问日志Header白名单采集/。
package sc49

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	bfe_access_pb "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost  = "hdr.example.org"
	apiPath  = "/v1/chat/completions"
	apiKey   = "ak_hdr_a"
	apiKeyId = "kid-hdr-a"

	clusterHdr = "cluster_hdr"

	// dataConfRelPath mirrors DataConfPath in testdata mod_access_pb3.conf.
	dataConfRelPath = "mod_access_pb3/mod_access_pb.data"
)

var okBody = `{"id":"chatcmpl-sc49","choices":[]}`

// testEnv holds all resources for a single SC49 integration test.
type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backend    *common.MockBackend
	redis      *common.RedisServer

	confDir     string
	logDir      string
	dataConf    string // absolute path of mod_access_pb.data
	bfePort     int
	monitorPort int
	stopBFE     func()
}

type envOptions struct {
	// dataConfContent, when non-empty, is written to mod_access_pb.data
	// before BFE starts; when empty, the file is simply not created
	// (missing-file tolerance path).
	dataConfContent string
	// noDataConfPath drops the DataConfPath line from mod_access_pb3.conf
	// (dynamic config not enabled at all).
	noDataConfPath bool
	// responseHeaders are injected by the mock backend into every response.
	responseHeaders map[string]string
}

func newTestEnv(t *testing.T, opts envOptions) *testEnv {
	e := &testEnv{t: t}

	e.backend = common.NewMockBackend(clusterHdr, http.StatusOK, okBody)
	e.backend.ResponseHeaders = opts.responseHeaders

	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")
	e.dataConf = filepath.Join(e.confDir, dataConfRelPath)

	tokenRule := &common.TokenRuleData{
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
					UnlimitedQuota: true,
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

	aiConf := &cluster_conf.AIConf{
		Type: 0,
		Keys: []cluster_conf.AIKey{
			{Name: "key-hdr", Key: "sk-key-hdr", Weight: 100},
		},
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:            "weighted_random",
			MaxRetries:          1,
			RetryBackoffInitial: 50,
			RetryBackoffMax:     200,
		},
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends: map[string]*common.MockBackend{
			clusterHdr: e.backend,
		},
		RedisAddr:     e.redis.Addr(),
		TokenRuleData: tokenRule,
		AIConfs: map[string]*cluster_conf.AIConf{
			clusterHdr: aiConf,
		},
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	if opts.noDataConfPath {
		if err := common.RewriteConfLine(filepath.Join(e.confDir, "mod_access_pb3", "mod_access_pb3.conf"),
			"DataConfPath", ""); err != nil {
			t.Fatalf("drop DataConfPath failed: %v", err)
		}
	}

	if opts.dataConfContent != "" {
		if err := os.MkdirAll(filepath.Dir(e.dataConf), 0755); err != nil {
			t.Fatalf("mkdir data conf dir failed: %v", err)
		}
		if err := ioutil.WriteFile(e.dataConf, []byte(opts.dataConfContent), 0644); err != nil {
			t.Fatalf("write data conf failed: %v", err)
		}
	}

	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
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
	if e.redis != nil {
		e.redis.Close()
	}
}

// writeDataConf rewrites mod_access_pb.data at runtime (hot reload tests).
func (e *testEnv) writeDataConf(content string) {
	e.t.Helper()
	if err := ioutil.WriteFile(e.dataConf, []byte(content), 0644); err != nil {
		e.t.Fatalf("rewrite data conf failed: %v", err)
	}
}

// reloadDataConf calls /reload/mod_access_pb3 and reports whether BFE
// accepted the new data file.
func (e *testEnv) reloadDataConf() error {
	url := fmt.Sprintf("http://127.0.0.1:%d/reload/mod_access_pb3", e.monitorPort)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("reload request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reload status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	// the BFE monitor endpoint answers errors with HTTP 200 and an error
	// body ({"error":...}); success is exactly {"error":null}
	if !strings.Contains(string(body), `"error":null`) {
		return fmt.Errorf("reload error body=%s", strings.TrimSpace(string(body)))
	}
	return nil
}

// sendRequest posts one AI request. extraHeaders are written verbatim
// (keys are NOT canonicalized by the Go client), so tests can send
// non-canonical header names on the wire.
func (e *testEnv) sendRequest(extraHeaders map[string][]string) (*http.Response, string) {
	e.t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(`{"model":"deepseek-chat"}`)))
	if err != nil {
		e.t.Fatalf("new request failed: %v", err)
	}
	req.Host = apiHost
	req.Header["Authorization"] = []string{"Bearer " + apiKey}
	req.Header["Content-Type"] = []string{"application/json"}
	for k, values := range extraHeaders {
		req.Header[k] = values
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		e.t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("read response failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return resp, string(respBody)
}

// accessLogs waits briefly for the async log4go writer to flush, stops BFE,
// and returns all decoded request logs.
func (e *testEnv) accessLogs() []*bfe_access_pb.RequestLog {
	e.t.Helper()
	// log4go flushes asynchronously; give it a moment before killing the
	// process (same 500ms convention as SC05/SC41).
	time.Sleep(500 * time.Millisecond)
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	reqLogs, err := common.ParseAccessLogAfterStop(e.logDir)
	if err != nil {
		e.t.Fatalf("parse access log failed: %v", err)
	}
	return reqLogs
}

// findLogs returns the request logs of this scenario's calls in order.
func findLogs(t *testing.T, reqLogs []*bfe_access_pb.RequestLog, want int) []*bfe_access_pb.RequestLog {
	t.Helper()
	var matched []*bfe_access_pb.RequestLog
	for _, l := range reqLogs {
		if l.GetAiApikeyId() == apiKeyId {
			matched = append(matched, l)
		}
	}
	if len(matched) != want {
		t.Fatalf("expected %d request logs for key_id %s, got %d (total %d)",
			want, apiKeyId, len(matched), len(reqLogs))
	}
	return matched
}

// headersToMap flattens the collected HttpHeader list into key -> value.
func headersToMap(entries []*bfe_access_pb.HttpHeader) map[string]string {
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.GetKey()] = e.GetValue()
	}
	return m
}

const traceHeader = "X-Trace-Id"

func traceBackend() map[string]string {
	return map[string]string{traceHeader: "trace-abc"}
}

// TC-01 白名单采集与规范化键
func TestWhitelistedHeadersCollected(t *testing.T) {
	e := newTestEnv(t, envOptions{
		dataConfContent: `{"Version":"v1","access_header":{
			"req_headers":["business_type","x-multi","x-absent-req"],
			"res_headers":["x-trace-id","x-absent-resp"]}}`,
		responseHeaders: traceBackend(),
	})
	defer e.Close()

	e.sendRequest(map[string][]string{
		"Business_Type":     {"rag"},
		"X-Multi":           {"v1", "v2"},
		"X-Not-Whitelisted": {"nope"},
	})

	logs := findLogs(t, e.accessLogs(), 1)
	reqHeaders := headersToMap(logs[0].ReqHeaders)

	if got := reqHeaders["Business_type"]; got != "rag" {
		t.Errorf("Business_type should be collected, got: %q (all: %v)", got, reqHeaders)
	}
	if got := reqHeaders["X-Multi"]; got != "v1,v2" {
		t.Errorf("multi-value should be joined with ',', got: %q", got)
	}
	if _, ok := reqHeaders["X-Not-Whitelisted"]; ok {
		t.Errorf("non-whitelisted header must not be collected: %v", reqHeaders)
	}
	if _, ok := reqHeaders["X-Absent-Req"]; ok {
		t.Errorf("absent whitelisted header must not be collected: %v", reqHeaders)
	}
	if len(logs[0].ReqHeaders) != 2 {
		t.Errorf("ReqHeaders should have exactly 2 entries, got: %v", logs[0].ReqHeaders)
	}

	resHeaders := headersToMap(logs[0].ResHeaders)
	if got := resHeaders[traceHeader]; got != "trace-abc" {
		t.Errorf("X-Trace-Id should be collected from backend response, got: %q (all: %v)", got, resHeaders)
	}
	if _, ok := resHeaders["X-Absent-Resp"]; ok {
		t.Errorf("absent response header must not be collected: %v", resHeaders)
	}
}

// TC-02 大小写不敏感配置：白名单与传输键任意大小写均可命中
func TestCaseInsensitiveMatching(t *testing.T) {
	e := newTestEnv(t, envOptions{
		dataConfContent: `{"Version":"v1","access_header":{
			"req_headers":["business_type","X-request-ID"],
			"res_headers":["X-TRACE-id"]}}`,
		responseHeaders: map[string]string{"x-trace-ID": "trace-xyz"},
	})
	defer e.Close()

	// non-canonical keys on the wire
	e.sendRequest(map[string][]string{
		"BUSINESS_TYPE": {"rag"},
		"x-ReQuEsT-iD":  {"rid-1"},
	})

	logs := findLogs(t, e.accessLogs(), 1)
	reqHeaders := headersToMap(logs[0].ReqHeaders)
	if got := reqHeaders["Business_type"]; got != "rag" {
		t.Errorf("got: %q, want Business_type=rag (%v)", got, reqHeaders)
	}
	if got := reqHeaders["X-Request-Id"]; got != "rid-1" {
		t.Errorf("got: %q, want X-Request-Id=rid-1 (%v)", got, reqHeaders)
	}
	resHeaders := headersToMap(logs[0].ResHeaders)
	if got := resHeaders[traceHeader]; got != "trace-xyz" {
		t.Errorf("got: %q, want X-Trace-Id=trace-xyz (%v)", got, resHeaders)
	}
}

// TC-03 默认零采集回归：三组对照
func TestDefaultNoCollection(t *testing.T) {
	cases := []struct {
		name string
		opts envOptions
	}{
		{"A_no_DataConfPath", envOptions{noDataConfPath: true, responseHeaders: traceBackend()}},
		{"B_missing_data_file", envOptions{responseHeaders: traceBackend()}},
		{"C_empty_whitelists", envOptions{
			dataConfContent: `{"Version":"v1","access_header":{"req_headers":[],"res_headers":[]}}`,
			responseHeaders: traceBackend(),
		}},
		{"C_missing_section", envOptions{
			dataConfContent: `{"Version":"v1"}`,
			responseHeaders: traceBackend(),
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEnv(t, c.opts)
			defer e.Close()
			e.sendRequest(map[string][]string{"Business_Type": {"rag"}})
			logs := findLogs(t, e.accessLogs(), 1)
			if len(logs[0].ReqHeaders) != 0 || len(logs[0].ResHeaders) != 0 {
				t.Errorf("no headers should be collected by default, got req=%v res=%v",
					logs[0].ReqHeaders, logs[0].ResHeaders)
			}
			if logs[0].Authorization != nil {
				t.Error("standalone authorization field must stay absent (#1357)")
			}
		})
	}
}

// TC-04 动态配置热加载
func TestHotReloadDataConf(t *testing.T) {
	e := newTestEnv(t, envOptions{
		dataConfContent: `{"Version":"v1","access_header":{
			"req_headers":["business_type"],"res_headers":[]}}`,
		responseHeaders: traceBackend(),
	})
	defer e.Close()

	e.sendRequest(map[string][]string{"Business_Type": {"rag"}, "X-Request-Id": {"rid-1"}})

	e.writeDataConf(`{"Version":"v2","access_header":{
		"req_headers":["x-request-id"],"res_headers":["x-trace-id"]}}`)
	if err := e.reloadDataConf(); err != nil {
		t.Fatalf("reload should succeed: %v", err)
	}

	e.sendRequest(map[string][]string{"Business_Type": {"rag"}, "X-Request-Id": {"rid-2"}})

	logs := findLogs(t, e.accessLogs(), 2)

	before := headersToMap(logs[0].ReqHeaders)
	if got := before["Business_type"]; got != "rag" {
		t.Errorf("before reload: Business_type should be collected, got: %v", before)
	}
	if _, ok := before["X-Request-Id"]; ok {
		t.Errorf("before reload: X-Request-Id should not be collected, got: %v", before)
	}
	if len(logs[0].ResHeaders) != 0 {
		t.Errorf("before reload: no res headers, got: %v", logs[0].ResHeaders)
	}

	after := headersToMap(logs[1].ReqHeaders)
	if got := after["X-Request-Id"]; got != "rid-2" {
		t.Errorf("after reload: X-Request-Id should be collected, got: %v", after)
	}
	if _, ok := after["Business_type"]; ok {
		t.Errorf("after reload: removed header must stop being collected, got: %v", after)
	}
	afterRes := headersToMap(logs[1].ResHeaders)
	if got := afterRes[traceHeader]; got != "trace-abc" {
		t.Errorf("after reload: X-Trace-Id should be collected, got: %v", afterRes)
	}
}

// TC-05 reload 失败保留旧配置（5a 非法 JSON、5b 非法头名）
func TestFailedReloadKeepsOldConf(t *testing.T) {
	cases := []struct {
		name    string
		badConf string
	}{
		{"invalid_json", `{broken`},
		{"invalid_header_name", `{"Version":"v2","access_header":{"req_headers":["bad name"]}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEnv(t, envOptions{
				dataConfContent: `{"Version":"v1","access_header":{
					"req_headers":["business_type"],"res_headers":[]}}`,
			})
			defer e.Close()

			e.sendRequest(map[string][]string{"Business_Type": {"rag"}, "X-Request-Id": {"rid-1"}})

			e.writeDataConf(c.badConf)
			if err := e.reloadDataConf(); err == nil {
				t.Fatal("reload with invalid data conf should return error")
			}

			e.sendRequest(map[string][]string{"Business_Type": {"rag2"}, "X-Request-Id": {"rid-2"}})

			logs := findLogs(t, e.accessLogs(), 2)
			for i, l := range logs {
				reqHeaders := headersToMap(l.ReqHeaders)
				if got := reqHeaders["Business_type"]; got == "" {
					t.Errorf("log %d: old whitelist must keep serving after failed reload, got: %v", i, reqHeaders)
				}
				if _, ok := reqHeaders["X-Request-Id"]; ok {
					t.Errorf("log %d: new whitelist must not take effect, got: %v", i, reqHeaders)
				}
			}
		})
	}
}

// TC-06 未知段与缺段前向兼容
func TestUnknownAndMissingSections(t *testing.T) {
	e := newTestEnv(t, envOptions{
		dataConfContent: `{"Version":"v1",
			"access_header":{"req_headers":["business_type"],"res_headers":[]},
			"sampling":{"rate":10}}`,
	})
	defer e.Close()

	e.sendRequest(map[string][]string{"Business_Type": {"rag"}})

	// reload with a file whose access_header section is missing
	e.writeDataConf(`{"Version":"v2"}`)
	if err := e.reloadDataConf(); err != nil {
		t.Fatalf("reload with missing section should succeed: %v", err)
	}
	e.sendRequest(map[string][]string{"Business_Type": {"rag"}})

	logs := findLogs(t, e.accessLogs(), 2)
	if got := headersToMap(logs[0].ReqHeaders)["Business_type"]; got != "rag" {
		t.Errorf("unknown section must not break known ones, got: %v", logs[0].ReqHeaders)
	}
	if len(logs[1].ReqHeaders) != 0 {
		t.Errorf("missing access_header section should run with empty whitelist, got: %v", logs[1].ReqHeaders)
	}
}

// TC-07 value 2048 截断
func TestValueTruncatedToColumnWidth(t *testing.T) {
	e := newTestEnv(t, envOptions{
		dataConfContent: `{"Version":"v1","access_header":{
			"req_headers":["x-big"],"res_headers":["x-trace-id"]}}`,
		responseHeaders: traceBackend(),
	})
	defer e.Close()

	big := strings.Repeat("a", 3000)
	e.sendRequest(map[string][]string{"X-Big": {big}})

	logs := findLogs(t, e.accessLogs(), 1)
	got := headersToMap(logs[0].ReqHeaders)["X-Big"]
	if len(got) != 2048 {
		t.Errorf("value should be truncated to 2048 bytes, got: %d", len(got))
	}
	// the rest of the row stays intact
	if got := headersToMap(logs[0].ResHeaders)[traceHeader]; got != "trace-abc" {
		t.Errorf("res header should stay intact, got: %v", logs[0].ResHeaders)
	}
}

// TC-08 原始凭证掩码兜底（红线 bfenetworks/bfe#1357）
func TestCredentialMaskingCoversWhitelistedHeaders(t *testing.T) {
	e := newTestEnv(t, envOptions{
		dataConfContent: `{"Version":"v1","access_header":{
			"req_headers":["x-echo","authorization"],"res_headers":[]}}`,
	})
	defer e.Close()

	// the raw API key is sent both as the credential and embedded in a
	// whitelisted header value; neither must land in the access log
	e.sendRequest(map[string][]string{
		"X-Echo": {"prefix-" + apiKey + "-suffix"},
	})

	logs := findLogs(t, e.accessLogs(), 1)
	reqLog := logs[0]

	if reqLog.Authorization != nil {
		t.Errorf("standalone authorization field must stay absent (#1357), got: %q", reqLog.GetAuthorization())
	}
	reqHeaders := headersToMap(reqLog.ReqHeaders)
	// AI mode injects the upstream provider key into the request's
	// Authorization header in place before forwarding, so by request-finish
	// time the header already carries the UPSTREAM credential. The data
	// plane does not restrict whitelist content (管控归上层配置管理); this
	// assertion documents the actual behavior rather than a masking of the
	// client credential.
	if got := reqHeaders["Authorization"]; got != "Bearer sk-key-hdr" {
		t.Errorf("whitelisted Authorization carries the upstream key after in-place injection, got: %q", got)
	}
	if got := reqHeaders["X-Echo"]; got != "prefix-"+apiKeyId+"-suffix" {
		t.Errorf("raw client key inside whitelisted value should be masked to key_id, got: %q", got)
	}

	// byte-level scan of the whole serialized log: the raw key must not
	// appear in ANY field
	raw, err := proto.Marshal(reqLog)
	if err != nil {
		t.Fatalf("marshal reqLog failed: %v", err)
	}
	if bytes.Contains(raw, []byte(apiKey)) {
		t.Error("raw API key must not appear anywhere in the serialized request log")
	}
}
