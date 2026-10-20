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

package sc35

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost = "audit.example.org"
	apiPath = "/v1/chat/completions"

	clusterAudit = "cluster_audit"

	apiKey   = "ak_audit"
	apiKeyId = "audit_key_id"
)

var defaultBody = []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`)
var defaultResponse = `{"id":"chatcmpl-sc35","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`

type auditEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backend    *common.MockBackend
	audit      *common.MockAudit
	redis      *common.RedisServer

	confDir     string
	logDir      string
	bfePort     int
	monitorPort int
	stopBFE     func()
}

// processorFactory builds a BodyProcessConfig against the live audit URL.
type processorFactory func(auditURL string) map[string]interface{}

// auditOptions controls the body_process rule and startup picture.
type auditOptions struct {
	// cond is the rule condition (default default_t()).
	cond string
	// requestProcess / responseProcess build the BodyProcessConfig. Nil omits
	// the section.
	requestProcess  processorFactory
	responseProcess processorFactory
	// disableAI runs the plain forwarding path instead of the AI gateway.
	disableAI bool
	// responseBody overrides the mock backend body.
	responseBody string
}

func newAuditEnv(t *testing.T, opts auditOptions) *auditEnv {
	e := &auditEnv{t: t}

	backendBody := opts.responseBody
	if backendBody == "" {
		backendBody = defaultResponse
	}
	e.backend = common.NewMockBackend(clusterAudit, http.StatusOK, backendBody)
	e.audit = common.NewMockAudit(t)
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends: map[string]*common.MockBackend{
			clusterAudit: e.backend,
		},
		AIConfs: map[string]*cluster_conf.AIConf{
			clusterAudit: singleKeyAIConf(),
		},
		RedisAddr:     e.redis.Addr(),
		TokenRuleData: unlimitedTokenRule(),
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	e.writeRule(opts)
	if opts.disableAI {
		if err := common.RewriteConfLine(filepath.Join(e.confDir, "bfe.conf"),
			"EnableAiGateway", "false"); err != nil {
			t.Fatalf("disable AI gateway failed: %v", err)
		}
	}

	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
	return e
}

func unlimitedTokenRule() *common.TokenRuleData {
	return &common.TokenRuleData{
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
}

func singleKeyAIConf() *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type: 0,
		Keys: []cluster_conf.AIKey{
			{Name: "key-1", Key: "sk-sc35-1", Weight: 100},
		},
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:            "weighted_random",
			MaxRetries:          0,
			RetryBackoffInitial: 20,
			RetryBackoffMax:     50,
		},
	}
}

// writeRule writes body_process.data with the audit URL substituted.
func (e *auditEnv) writeRule(opts auditOptions) {
	e.t.Helper()
	cond := opts.cond
	if cond == "" {
		cond = "default_t()"
	}
	rule := map[string]interface{}{"Cond": cond}
	if opts.requestProcess != nil {
		rule["RequestProcess"] = opts.requestProcess(e.audit.URL())
	}
	if opts.responseProcess != nil {
		rule["ResponseProcess"] = opts.responseProcess(e.audit.URL())
	}
	data := map[string]interface{}{
		"Version": "1.0",
		"Config": map[string][]map[string]interface{}{
			"ai_product": {rule},
		},
	}
	path := filepath.Join(e.confDir, "mod_body_process", "body_process.data")
	if err := common.WriteJSONFile(path, data); err != nil {
		e.t.Fatalf("write body_process.data failed: %v", err)
	}
}

func (e *auditEnv) reloadModule() {
	e.t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/reload/mod_body_process", e.monitorPort)
	resp, err := http.Get(url)
	if err != nil {
		e.t.Fatalf("reload mod_body_process failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("reload mod_body_process status=%d body=%s", resp.StatusCode, body)
	}
}

func (e *auditEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	if e.backend != nil {
		e.backend.Close()
	}
	if e.audit != nil {
		e.audit.Close()
	}
	if e.redis != nil {
		e.redis.Close()
	}
}

func (e *auditEnv) logBFEException() {
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

func (e *auditEnv) newRequest(path string, body []byte) *http.Request {
	if path == "" {
		path = apiPath
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, path)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		e.t.Fatalf("build request failed: %v", err)
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func (e *auditEnv) newClient() *http.Client {
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
}

func (e *auditEnv) send(path string, body []byte) (*http.Response, string, error) {
	resp, err := e.newClient().Do(e.newRequest(path, body))
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

func (e *auditEnv) mustStatus(resp *http.Response, body string, err error, want int) {
	e.t.Helper()
	if err != nil {
		e.t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != want {
		e.logBFEException()
		e.t.Fatalf("expected %d, got %d, body: %s", want, resp.StatusCode, body)
	}
}

// filterProcess builds a BodyProcessConfig with the audit textfilter processor.
func filterProcess(dec string) processorFactory {
	return func(auditURL string) map[string]interface{} {
		out := map[string]interface{}{
			"Proc": []interface{}{
				map[string]interface{}{
					"Name":   "textfilter",
					"Params": []string{auditURL},
				},
			},
		}
		if dec != "" {
			out["Dec"] = dec
		}
		return out
	}
}

// TestTC01_RequestAuditPass verifies a passing request-body audit forwards
// semantically identical content as a chunked request.
func TestTC01_RequestAuditPass(t *testing.T) {
	e := newAuditEnv(t, auditOptions{requestProcess: filterProcess("json")})
	defer e.Close()

	resp, body, err := e.send("", defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)

	if e.backend.Hits() != 1 {
		t.Fatalf("expected 1 backend hit, got %d", e.backend.Hits())
	}
	var got, want interface{}
	if err := json.Unmarshal(e.backend.RequestBodies()[0], &got); err != nil {
		t.Fatalf("backend body is not JSON: %v", err)
	}
	if err := json.Unmarshal(defaultBody, &want); err != nil {
		t.Fatalf("original body is not JSON: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backend body changed: %s", e.backend.RequestBodies()[0])
	}

	if lens := e.backend.ContentLengths(); len(lens) != 1 || lens[0] != -1 {
		t.Fatalf("expected chunked request (ContentLength=-1), got %v", lens)
	}
	if encs := e.backend.TransferEncodings(); len(encs) != 1 || len(encs[0]) == 0 || encs[0][0] != "chunked" {
		t.Fatalf("expected Transfer-Encoding chunked, got %v", encs)
	}

	calls := e.audit.FilterCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 /text-filter call, got %d", len(calls))
	}
	if calls[0].Text != string(defaultBody) {
		t.Fatalf("audit text mismatch: %q", calls[0].Text)
	}
}

// TestTC02_RequestAuditReject verifies a rejected request body fails the
// request with a gateway 500 without delivering a successful response.
// The upstream connection may already be open when the streamed body is
// aborted, so only a single (incomplete) backend contact is tolerated.
func TestTC02_RequestAuditReject(t *testing.T) {
	e := newAuditEnv(t, auditOptions{requestProcess: filterProcess("json")})
	defer e.Close()

	e.audit.FilterFunc = func(text string) common.TextFilterResponse {
		return common.TextFilterResponse{RiskLevel: "REJECT"}
	}

	resp, body, err := e.send("", defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		e.logBFEException()
		t.Fatalf("expected 500, got %d, body: %s", resp.StatusCode, body)
	}
	if strings.Contains(body, "chatcmpl-sc35") {
		t.Fatalf("rejected request must not return the backend response, body: %s", body)
	}
	if hits := e.backend.Hits(); hits > 1 {
		t.Fatalf("rejected request must not be retried, backend hits=%d", hits)
	}
	if calls := e.audit.FilterCalls(); len(calls) != 1 {
		t.Fatalf("expected 1 audit call, got %d", len(calls))
	}
}

// TestTC03_ResponseAuditRewrite verifies the response body is rewritten with
// the /text-replace result.
func TestTC03_ResponseAuditRewrite(t *testing.T) {
	e := newAuditEnv(t, auditOptions{responseProcess: filterProcess("json")})
	defer e.Close()

	e.audit.ReplaceFunc = func(text string) common.TextFilterResponse {
		return common.TextFilterResponse{ResultText: "REWRITTEN"}
	}

	resp, body, err := e.send("", defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if body != "REWRITTEN" {
		t.Fatalf("expected rewritten body, got %q", body)
	}
	calls := e.audit.ReplaceCalls()
	if len(calls) != 1 || calls[0].Text != defaultResponse {
		t.Fatalf("unexpected replace calls: %+v", calls)
	}
}

// TestTC04_ResponseAuditStreaming verifies SSE events are audited per event
// and forwarded without buffering the whole stream.
func TestTC04_ResponseAuditStreaming(t *testing.T) {
	e := newAuditEnv(t, auditOptions{responseProcess: filterProcess("sse")})
	defer e.Close()

	hold := make(chan struct{})
	e.backend.SSEEvents = []string{
		`{"choices":[{"delta":{"content":"a"}}]}`,
		`{"choices":[{"delta":{"content":"b"}}]}`,
		`{"choices":[{"delta":{"content":"c"}}]}`,
	}
	e.backend.SSEHold = hold
	e.audit.ReplaceFunc = func(text string) common.TextFilterResponse {
		return common.TextFilterResponse{ResultText: "AUDITED " + text}
	}

	resp, err := e.newClient().Do(e.newRequest("", defaultBody))
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	first, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read first frame failed: %v", err)
	}
	if !strings.Contains(first, "AUDITED ") {
		t.Fatalf("first frame was not audited before the hold released: %q", first)
	}

	close(hold)
	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read remaining frames failed: %v", err)
	}
	all := first + string(rest)
	// The SSE event text handed to the auditor already includes its "data: "
	// prefix, so the rewritten payload repeats it; count the audit markers
	// instead of the "data:" tokens.
	if got := strings.Count(all, "AUDITED "); got != 3 {
		t.Fatalf("expected 3 audited events, got %d: %q", got, all)
	}
	if calls := e.audit.ReplaceCalls(); len(calls) != 3 {
		t.Fatalf("expected 3 replace calls, got %d", len(calls))
	}
}

// TestTC05_LineDecoder verifies the line decoder emits one event per line and
// preserves the bytes end-to-end.
func TestTC05_LineDecoder(t *testing.T) {
	e := newAuditEnv(t, auditOptions{requestProcess: filterProcess("line")})
	defer e.Close()

	lineBody := []byte("{\"a\":1}\n{\"b\":2}\n")
	resp, body, err := e.send("", lineBody)
	e.mustStatus(resp, body, err, http.StatusOK)

	got := e.backend.RequestBodies()
	if len(got) != 1 || !bytes.Equal(got[0], lineBody) {
		t.Fatalf("expected unchanged line body, got %q", got)
	}
	if calls := e.audit.FilterCalls(); len(calls) != 2 {
		t.Fatalf("expected 2 audit calls (one per line), got %d", len(calls))
	}
}

// TestTC06_AuditServiceDownPassesThrough verifies the current fail-open
// behavior: an unreachable audit service does not block forwarding.
func TestTC06_AuditServiceDownPassesThrough(t *testing.T) {
	e := newAuditEnv(t, auditOptions{requestProcess: filterProcess("json")})
	defer e.Close()

	e.audit.Close()

	resp, body, err := e.send("", defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if e.backend.Hits() != 1 {
		t.Fatalf("expected the request to be forwarded, hits=%d", e.backend.Hits())
	}
}

// TestTC07_NonAiRequestSkipped verifies the body processor does not run when
// the AI gateway is disabled, even with a matching rule.
func TestTC07_NonAiRequestSkipped(t *testing.T) {
	e := newAuditEnv(t, auditOptions{
		requestProcess:  filterProcess("json"),
		responseProcess: filterProcess("json"),
		disableAI:       true,
	})
	defer e.Close()

	resp, body, err := e.send("", defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)

	if calls := e.audit.CallCount(); calls != 0 {
		t.Fatalf("expected no audit calls for a non-AI request, got %d", calls)
	}
	got := e.backend.RequestBodies()
	if len(got) != 1 || !bytes.Equal(got[0], defaultBody) {
		t.Fatalf("expected unchanged body, got %q", got)
	}
}

// TestTC08_ModuleMetrics verifies the module counters move with the request
// lifecycle.
func TestTC08_ModuleMetrics(t *testing.T) {
	e := newAuditEnv(t, auditOptions{requestProcess: filterProcess("json")})
	defer e.Close()

	resp, body, err := e.send("", defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)

	if got := common.WaitForModuleMetricValue(e.monitorPort, "mod_body_process", "ReqTotal", 1, 5*time.Second); got < 1 {
		t.Fatalf("expected REQ_TOTAL >= 1, got %f", got)
	}
	for _, field := range []string{"ReqProcess", "ResProcess"} {
		if got := common.WaitForModuleMetricValue(e.monitorPort, "mod_body_process", field, 1, 5*time.Second); got < 1 {
			t.Fatalf("expected %s >= 1, got %f", field, got)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		inFlight, metricErr := common.ModuleMetric(e.monitorPort, "mod_body_process", "InFlight")
		if metricErr == nil && inFlight == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected IN_FLIGHT to return to 0, got %f", inFlight)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestTC09_RuleConditionAndReload verifies a rule only applies to matching
// paths and that /reload/mod_body_process takes effect without a restart.
func TestTC09_RuleConditionAndReload(t *testing.T) {
	e := newAuditEnv(t, auditOptions{
		cond:           `req_path_prefix_in("/api", true)`,
		requestProcess: filterProcess("json"),
	})
	defer e.Close()

	resp, body, err := e.send("", defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if calls := e.audit.CallCount(); calls != 0 {
		t.Fatalf("rule must not match /v1/..., audit calls=%d", calls)
	}

	resp, body, err = e.send("/api/chat", defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if calls := e.audit.CallCount(); calls != 1 {
		t.Fatalf("expected 1 audit call for /api/chat, got %d", calls)
	}

	// Broaden the rule and reload.
	e.writeRule(auditOptions{requestProcess: filterProcess("json")})
	e.reloadModule()

	resp, body, err = e.send("", defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if calls := e.audit.CallCount(); calls != 2 {
		t.Fatalf("expected the reloaded rule to apply to /v1/..., calls=%d", calls)
	}
}
