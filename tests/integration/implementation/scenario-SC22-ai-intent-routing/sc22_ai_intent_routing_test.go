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

package sc22

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost      = "intent.example.org"
	apiPath      = "/v1/chat/completions"
	apiKeyIntent = "ak_intent"
	apiKeyPlain  = "ak_plain"

	clusterFlash = "cluster_flash"
	clusterKimi  = "cluster_kimi"

	questionsVersion1 = "2026092601"
	questionsVersion2 = "2026092602"

	intentHeader = "X-AI-Intent"
)

var (
	unitTestBody      = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"帮我给这个函数写单元测试"}]}`)
	complexTestBody   = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"这个测试但比较复杂，帮我写"}]}`)
	vagueBody         = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"很模糊，帮我看看"}]}`)
	refactorBody      = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"帮我重构代码，提取公共函数"}]}`)
	writeDocBody      = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"帮我写文档，说明这个模块的用法"}]}`)
	writeDesignBody   = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"帮我写设计文档，梳理整体架构"}]}`)
	failureInjectBody = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"故障注入：帮我写单元测试"}]}`)

	flashAnswer = `{"id":"chatcmpl-flash","choices":[{"index":0,"message":{"role":"assistant","content":"from cluster_flash"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`
	kimiAnswer  = `{"id":"chatcmpl-kimi","choices":[{"index":0,"message":{"role":"assistant","content":"from cluster_kimi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`

	complexityLegend = []string{"simple", "medium", "complex"}
)

// testEnv holds all resources for a single SC22 integration test.
type testEnv struct {
	t           *testing.T
	processEnv  *common.ProcessEnv
	flash       *common.MockBackend
	kimi        *common.MockBackend
	decision    *common.MockDecisionService
	redis       *common.RedisServer
	confDir     string
	logDir      string
	bfePort     int
	monitorPort int
	stopBFE     func()
}

func newTestEnv(t *testing.T) *testEnv {
	e := &testEnv{t: t}

	e.flash = common.NewMockBackend(clusterFlash, http.StatusOK, flashAnswer)
	e.kimi = common.NewMockBackend(clusterKimi, http.StatusOK, kimiAnswer)
	e.decision = common.NewMockDecisionService(t, baseDecisionScript())
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	return e
}

// baseDecisionScript scripts the decision service by last user message
// keyword (longest keyword wins, see common.DecisionScript).
func baseDecisionScript() common.DecisionScript {
	return common.DecisionScript{
		"单元测试":  testWritingAnswer(0.93),
		"测试但复杂": testWritingAnswer(0.72),
		"很模糊":   testWritingAnswer(0.45),
		"重构代码":  codingAnswer(),
		"写设计文档": docWritingAnswer(0.90, 2, map[string]float64{"0": 0.05, "1": 0.07, "2": 0.88}),
		"写文档":   docWritingAnswer(0.95, 1, map[string]float64{"0": 0.05, "1": 0.90, "2": 0.05}),
	}
}

func testWritingAnswer(confidence float64) common.DecisionResponse {
	return common.DecisionResponse{
		Status: http.StatusOK,
		Answers: map[string]common.DecisionAnswer{
			"task_type": {
				Type:             "choice",
				Choice:           "test_writing",
				Probabilities:    map[string]float64{"coding": 0.02, "test_writing": confidence, "doc_writing": 0.05},
				AnswerConfidence: confidence,
			},
		},
	}
}

func codingAnswer() common.DecisionResponse {
	return common.DecisionResponse{
		Status: http.StatusOK,
		Answers: map[string]common.DecisionAnswer{
			"task_type": {
				Type:             "choice",
				Choice:           "coding",
				Probabilities:    map[string]float64{"coding": 0.90, "test_writing": 0.05, "doc_writing": 0.05},
				AnswerConfidence: 0.90,
			},
		},
	}
}

// docWritingAnswer answers task_type=doc_writing and complexity with the
// given level score ("1" is medium, "2" is complex).
func docWritingAnswer(confidence float64, level int, probabilities map[string]float64) common.DecisionResponse {
	return common.DecisionResponse{
		Status: http.StatusOK,
		Answers: map[string]common.DecisionAnswer{
			"task_type": {
				Type:             "choice",
				Choice:           "doc_writing",
				Probabilities:    map[string]float64{"coding": 0.03, "test_writing": 0.02, "doc_writing": confidence},
				AnswerConfidence: confidence,
			},
			"complexity": {
				Type:             "score",
				Score:            float64(level),
				Probabilities:    probabilities,
				AnswerConfidence: probabilities[fmt.Sprintf("%d", level)],
				Legend:           complexityLegend,
			},
		},
	}
}

// unitTestAnswerV2 is the TC-12 answer script: with the unit_test option
// present in the reloaded questions, the same prompt is classified as
// unit_test@0.95.
func unitTestAnswerV2() common.DecisionResponse {
	return common.DecisionResponse{
		Status: http.StatusOK,
		Answers: map[string]common.DecisionAnswer{
			"task_type": {
				Type:             "choice",
				Choice:           "unit_test",
				Probabilities:    map[string]float64{"coding": 0.02, "test_writing": 0.02, "doc_writing": 0.01, "unit_test": 0.95},
				AnswerConfidence: 0.95,
			},
		},
	}
}

// unlimitedTokenRule returns a token rule with an unlimited quota plan and
// the two API keys of this scenario (ak_intent / ak_plain).
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
				apiKeyIntent: {
					Key:            apiKeyIntent,
					KeyId:          "intent_key_id",
					Enabled:        true,
					ExpiredTime:    -1,
					UnlimitedQuota: false,
					QuotaPlans:     []string{"unlimited_plan"},
				},
				apiKeyPlain: {
					Key:            apiKeyPlain,
					KeyId:          "plain_key_id",
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

func (e *testEnv) startBFE() {
	backends := map[string]*common.MockBackend{
		clusterFlash: e.flash,
		clusterKimi:  e.kimi,
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:         "testdata",
		TargetConfDir:       e.confDir,
		Backends:            backends,
		RedisAddr:           e.redis.Addr(),
		TokenRuleData:       unlimitedTokenRule(),
		DecisionServiceAddr: e.decision.URL(),
	}
	if err := builder.Build(); err != nil {
		e.t.Fatalf("build bfe config failed: %v", err)
	}

	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	if e.flash != nil {
		e.flash.Close()
	}
	if e.kimi != nil {
		e.kimi.Close()
	}
	if e.decision != nil {
		e.decision.Close()
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

// sendRequest posts an AI chat request through BFE. Extra headers are
// applied after the default Authorization/Content-Type headers.
func (e *testEnv) sendRequest(apikey string, body []byte, headers map[string]string) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apikey)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
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

// ---------------------------------------------------------------------------
// module state page (/monitor/mod_ai_intent)
// ---------------------------------------------------------------------------

// intentModuleState is the JSON document served by /monitor/mod_ai_intent.
// Counter keys are rendered in SCREAMING_SNAKE_CASE by go-lib metrics.
type intentModuleState struct {
	CounterData map[string]int64  `json:"CounterData"`
	StateData   map[string]string `json:"StateData"`
}

const monitorPrefixModAiIntent = "mod_ai_intent"

func (e *testEnv) fetchIntentState() intentModuleState {
	e.t.Helper()
	u := fmt.Sprintf("http://127.0.0.1:%d/monitor/%s", e.monitorPort, monitorPrefixModAiIntent)
	resp, err := http.Get(u)
	if err != nil {
		e.t.Fatalf("get mod_ai_intent state failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("get mod_ai_intent state: expected 200, got %d", resp.StatusCode)
	}
	var st intentModuleState
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		e.t.Fatalf("decode mod_ai_intent state failed: %v", err)
	}
	return st
}

// waitIntentState polls the module state page until cond is satisfied or
// the deadline expires; it returns the last state fetched.
func (e *testEnv) waitIntentState(timeout time.Duration, cond func(intentModuleState) bool) intentModuleState {
	e.t.Helper()
	deadline := time.Now().Add(timeout)
	st := e.fetchIntentState()
	for time.Now().Before(deadline) {
		if cond(st) {
			return st
		}
		time.Sleep(50 * time.Millisecond)
		st = e.fetchIntentState()
	}
	return st
}

func counterIs(name string, want int64) func(intentModuleState) bool {
	return func(st intentModuleState) bool { return st.CounterData[name] == want }
}

func (e *testEnv) expectCounter(name string, want int64, step ...string) intentModuleState {
	label := name
	if len(step) > 0 {
		label = step[0]
	}
	st := e.waitIntentState(5*time.Second, counterIs(name, want))
	if got := st.CounterData[name]; got != want {
		e.logBFEException()
		e.t.Fatalf("%s: expected counter %s=%d, got %d (state: %+v)", label, name, want, got, st.CounterData)
	}
	return st
}

// reloadModule calls /reload/<module>?path=<path> on the monitor port and
// fails the test unless the reload succeeds (empty "error" field).
func (e *testEnv) reloadModule(module, path string) {
	e.t.Helper()
	u := fmt.Sprintf("http://127.0.0.1:%d/reload/%s", e.monitorPort, module)
	if path != "" {
		u += "?path=" + url.QueryEscape(path)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		e.t.Fatalf("call reload %s failed: %v", module, err)
	}
	defer resp.Body.Close()
	rsp := &struct {
		Error string `json:"error"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(rsp); err != nil {
		e.t.Fatalf("decode reload %s rsp failed: %v", module, err)
	}
	if rsp.Error != "" {
		e.t.Fatalf("reload %s failed: %s", module, rsp.Error)
	}
}

// ---------------------------------------------------------------------------
// TC-01 高置信测试意图分流
// ---------------------------------------------------------------------------

func TestTC01_HighConfidenceTestWriting(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE()

	resp, body, err := e.sendRequest(apiKeyIntent, unitTestBody, nil)
	e.expectOK(resp, body, err)
	if !strings.Contains(body, "from cluster_flash") {
		t.Fatalf("response should come from cluster_flash, got: %s", body)
	}
	if e.flash.Hits() != 1 || e.kimi.Hits() != 0 {
		t.Fatalf("expected flash=1 kimi=0, got flash=%d kimi=%d", e.flash.Hits(), e.kimi.Hits())
	}
	if e.decision.Hits() != 1 {
		t.Fatalf("decision service should be called exactly once, got %d", e.decision.Hits())
	}
	e.expectCounter("REQ_TOTAL", 1, "req_total")
	e.expectCounter("REQ_RESOLVED", 1, "req_resolved")
}

// ---------------------------------------------------------------------------
// TC-02 中置信测试意图分层
// ---------------------------------------------------------------------------

func TestTC02_MidConfidenceLayeredRouting(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE()

	resp, body, err := e.sendRequest(apiKeyIntent, complexTestBody, nil)
	e.expectOK(resp, body, err)
	if !strings.Contains(body, "from cluster_kimi") {
		t.Fatalf("response should come from cluster_kimi, got: %s", body)
	}
	if e.flash.Hits() != 0 || e.kimi.Hits() != 1 {
		t.Fatalf("expected flash=0 kimi=1, got flash=%d kimi=%d", e.flash.Hits(), e.kimi.Hits())
	}
	if e.decision.Hits() != 1 {
		t.Fatalf("decision service should be called exactly once, got %d", e.decision.Hits())
	}
}

// ---------------------------------------------------------------------------
// TC-03 低置信 unknown 兜底
// ---------------------------------------------------------------------------

func TestTC03_LowConfidenceUnknownFallback(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE()

	resp, body, err := e.sendRequest(apiKeyIntent, vagueBody, nil)
	e.expectOK(resp, body, err)
	if !strings.Contains(body, "from cluster_kimi") {
		t.Fatalf("response should come from cluster_kimi (default rule), got: %s", body)
	}
	if e.flash.Hits() != 0 || e.kimi.Hits() != 1 {
		t.Fatalf("expected flash=0 kimi=1, got flash=%d kimi=%d", e.flash.Hits(), e.kimi.Hits())
	}
	if e.decision.Hits() != 1 {
		t.Fatalf("classification should still happen once (read-time gating), got %d", e.decision.Hits())
	}
	e.expectCounter("REQ_RESOLVED", 1, "req_resolved")
}

// ---------------------------------------------------------------------------
// TC-04 coding 意图走默认
// ---------------------------------------------------------------------------

func TestTC04_CodingIntentDefaultRoute(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE()

	resp, body, err := e.sendRequest(apiKeyIntent, refactorBody, nil)
	e.expectOK(resp, body, err)
	if !strings.Contains(body, "from cluster_kimi") {
		t.Fatalf("response should come from cluster_kimi (default rule), got: %s", body)
	}
	if e.flash.Hits() != 0 || e.kimi.Hits() != 1 {
		t.Fatalf("expected flash=0 kimi=1, got flash=%d kimi=%d", e.flash.Hits(), e.kimi.Hits())
	}
	if e.decision.Hits() != 1 {
		t.Fatalf("decision service should be called exactly once, got %d", e.decision.Hits())
	}
}

// ---------------------------------------------------------------------------
// TC-05 多问题组合条件
// ---------------------------------------------------------------------------

func TestTC05_MultiQuestionConjunction(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE()

	// step 1: doc + medium -> rule intent-doc -> cluster_flash
	resp, body, err := e.sendRequest(apiKeyIntent, writeDocBody, nil)
	e.expectOK(resp, body, err, "step 1")
	if !strings.Contains(body, "from cluster_flash") {
		t.Fatalf("step 1: response should come from cluster_flash, got: %s", body)
	}

	// step 2: doc + complex -> rule intent-doc misses -> cluster_kimi
	resp, body, err = e.sendRequest(apiKeyIntent, writeDesignBody, nil)
	e.expectOK(resp, body, err, "step 2")
	if !strings.Contains(body, "from cluster_kimi") {
		t.Fatalf("step 2: response should come from cluster_kimi, got: %s", body)
	}

	if e.flash.Hits() != 1 || e.kimi.Hits() != 1 {
		t.Fatalf("expected flash=1 kimi=1, got flash=%d kimi=%d", e.flash.Hits(), e.kimi.Hits())
	}
	if e.decision.Hits() != 2 {
		t.Fatalf("each request should classify once, got %d", e.decision.Hits())
	}
	e.expectCounter("REQ_TOTAL", 2, "req_total")
	e.expectCounter("REQ_RESOLVED", 2, "req_resolved")
}

// ---------------------------------------------------------------------------
// TC-06 显式意图头优先
// ---------------------------------------------------------------------------

func TestTC06_ExplicitIntentHeaderPriority(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE()

	// The body would be classified as coding; the explicit header covering
	// all configured questions is trusted with confidence 1.0 and must win.
	resp, body, err := e.sendRequest(apiKeyIntent, refactorBody, map[string]string{
		intentHeader: "task_type=test_writing;complexity=simple",
	})
	e.expectOK(resp, body, err)
	if !strings.Contains(body, "from cluster_flash") {
		t.Fatalf("response should come from cluster_flash, got: %s", body)
	}
	if e.flash.Hits() != 1 || e.kimi.Hits() != 0 {
		t.Fatalf("expected flash=1 kimi=0, got flash=%d kimi=%d", e.flash.Hits(), e.kimi.Hits())
	}
	if e.decision.Hits() != 0 {
		t.Fatalf("valid explicit intent header must not call the decision service, got %d", e.decision.Hits())
	}
	e.expectCounter("REQ_HEADER", 1, "req_header")
	e.expectCounter("REQ_TOTAL", 1, "req_total")
	e.expectCounter("REQ_RESOLVED", 1, "req_resolved")
}

// ---------------------------------------------------------------------------
// TC-07 非法显式头忽略
// ---------------------------------------------------------------------------

func TestTC07_InvalidExplicitHeaderIgnored(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE()

	resp, body, err := e.sendRequest(apiKeyIntent, writeDocBody, map[string]string{
		intentHeader: "task_type=bogus",
	})
	e.expectOK(resp, body, err)
	if !strings.Contains(body, "from cluster_flash") {
		t.Fatalf("response should come from cluster_flash (model classification), got: %s", body)
	}
	if e.flash.Hits() != 1 || e.kimi.Hits() != 0 {
		t.Fatalf("expected flash=1 kimi=0, got flash=%d kimi=%d", e.flash.Hits(), e.kimi.Hits())
	}
	if e.decision.Hits() != 1 {
		t.Fatalf("invalid header entry should fall back to model classification, got %d calls", e.decision.Hits())
	}
	e.expectCounter("REQ_HEADER", 0, "req_header")
}

// ---------------------------------------------------------------------------
// TC-08 决策服务故障降级
// ---------------------------------------------------------------------------

func TestTC08_DecisionServiceFailureDegradation(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE()

	e.decision.SetStatus(http.StatusInternalServerError)

	resp, body, err := e.sendRequest(apiKeyIntent, failureInjectBody, nil)
	e.expectOK(resp, body, err)
	if !strings.Contains(body, "from cluster_kimi") {
		t.Fatalf("response should come from cluster_kimi (default rule), got: %s", body)
	}
	if e.flash.Hits() != 0 || e.kimi.Hits() != 1 {
		t.Fatalf("expected flash=0 kimi=1, got flash=%d kimi=%d", e.flash.Hits(), e.kimi.Hits())
	}
	if e.decision.Hits() != 1 {
		t.Fatalf("the failed decision call should still be counted, got %d", e.decision.Hits())
	}
	e.expectCounter("REQ_ERR", 1, "req_err")
	// a single failure (FailureThreshold=2) must not open the breaker
	e.expectCounter("BREAKER_OPEN", 0, "breaker_open")
}

// ---------------------------------------------------------------------------
// TC-09 熔断打开
// ---------------------------------------------------------------------------

func TestTC09_CircuitBreakerOpen(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE()

	e.decision.SetFailAll(true)

	// two consecutive failures reach FailureThreshold=2: breaker opens
	for i := 0; i < 2; i++ {
		resp, body, err := e.sendRequest(apiKeyIntent, unitTestBody, nil)
		e.expectOK(resp, body, err, fmt.Sprintf("failure request %d", i+1))
		if !strings.Contains(body, "from cluster_kimi") {
			t.Fatalf("failure request %d: response should come from cluster_kimi, got: %s", i+1, body)
		}
	}
	if e.decision.Hits() != 2 {
		t.Fatalf("expected 2 decision calls before the breaker opens, got %d", e.decision.Hits())
	}

	// mock recovers, but the open breaker must not call it anymore
	e.decision.SetFailAll(false)

	resp, body, err := e.sendRequest(apiKeyIntent, unitTestBody, nil)
	e.expectOK(resp, body, err, "request after breaker open")
	if !strings.Contains(body, "from cluster_kimi") {
		t.Fatalf("request after breaker open: response should come from cluster_kimi, got: %s", body)
	}
	if e.decision.Hits() != 2 {
		t.Fatalf("decision calls must stop growing while the breaker is open, got %d", e.decision.Hits())
	}
	if e.flash.Hits() != 0 || e.kimi.Hits() != 3 {
		t.Fatalf("expected flash=0 kimi=3, got flash=%d kimi=%d", e.flash.Hits(), e.kimi.Hits())
	}

	e.expectCounter("BREAKER_OPEN", 1, "breaker_open")
	st := e.waitIntentState(5*time.Second, func(st intentModuleState) bool {
		return st.CounterData["REQ_ERR"] >= 2
	})
	if got := st.CounterData["REQ_ERR"]; got < 2 {
		t.Fatalf("expected REQ_ERR>=2, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// TC-10 进程内缓存生效
// ---------------------------------------------------------------------------

func TestTC10_InProcessCacheHit(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE()

	for i := 0; i < 2; i++ {
		resp, body, err := e.sendRequest(apiKeyIntent, unitTestBody, nil)
		e.expectOK(resp, body, err, fmt.Sprintf("request %d", i+1))
		if !strings.Contains(body, "from cluster_flash") {
			t.Fatalf("request %d: response should come from cluster_flash, got: %s", i+1, body)
		}
	}
	if e.flash.Hits() != 2 {
		t.Fatalf("expected 2 flash hits, got %d", e.flash.Hits())
	}
	if e.decision.Hits() != 1 {
		t.Fatalf("the second request should hit the in-process cache (1 decision call), got %d", e.decision.Hits())
	}
	e.expectCounter("REQ_CACHE_HIT", 1, "req_cache_hit")
	e.expectCounter("REQ_RESOLVED", 2, "req_resolved")
}

// ---------------------------------------------------------------------------
// TC-11 不用意图的规则零开销
// ---------------------------------------------------------------------------

func TestTC11_NoIntentRuleZeroOverhead(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE()

	resp, body, err := e.sendRequest(apiKeyPlain, unitTestBody, nil)
	e.expectOK(resp, body, err)
	if !strings.Contains(body, "from cluster_kimi") {
		t.Fatalf("response should come from cluster_kimi (plain-default rule), got: %s", body)
	}
	if e.flash.Hits() != 0 || e.kimi.Hits() != 1 {
		t.Fatalf("expected flash=0 kimi=1, got flash=%d kimi=%d", e.flash.Hits(), e.kimi.Hits())
	}
	if e.decision.Hits() != 0 {
		t.Fatalf("rules without intent primitives must not call the decision service, got %d", e.decision.Hits())
	}
	e.expectCounter("REQ_TOTAL", 0, "req_total")
}

// ---------------------------------------------------------------------------
// TC-12 questions 热加载
// ---------------------------------------------------------------------------

// questionsDataV2 adds the unit_test option to task_type and bumps the
// Version, invalidating cached intents.
const questionsDataV2 = `{
    "Version": "2026092602",
    "MinConfidence": 0.6,
    "Questions": [
        {
            "Name": "task_type",
            "Type": "choice",
            "Instructions": "这条请求属于哪类研发任务？",
            "Criteria": {
                "coding": "编写或修改代码、调试、重构、代码审查",
                "test_writing": "编写测试用例、单元测试、集成测试、补充断言",
                "doc_writing": "编写文档、README、注释、接口说明、使用示例",
                "unit_test": "编写单元测试、用例设计、断言补充"
            }
        },
        {
            "Name": "complexity",
            "Type": "score",
            "Instructions": "这个任务的复杂度如何？",
            "MinConfidence": 0.7,
            "Levels": [
                { "Name": "simple",  "Description": "单步即可完成" },
                { "Name": "medium",  "Description": "多步但模式常见" },
                { "Name": "complex", "Description": "需要深入推理或跨模块设计" }
            ]
        }
    ]
}`

// aiRouteDataV2 inserts the intent-unit rule (unit_test -> cluster_kimi)
// in front of the original rules.
const aiRouteDataV2 = `{
    "Version": "2026092602",
    "route_rules": {
        "apikey_ak_intent": {
            "type": "apikey",
            "owner": "ak_intent",
            "rules": [
                {
                    "name": "intent-unit",
                    "Cond": "req_ai_intent_in(\"task_type\", \"unit_test\", 0.9)",
                    "targets": [
                        {
                            "ClusterName": "cluster_kimi",
                            "Model": "",
                            "Weight": 100
                        }
                    ],
                    "fallbacks": []
                },
                {
                    "name": "intent-flash",
                    "Cond": "req_ai_intent_in(\"task_type\", \"test_writing\", 0.9)",
                    "targets": [
                        {
                            "ClusterName": "cluster_flash",
                            "Model": "",
                            "Weight": 100
                        }
                    ],
                    "fallbacks": []
                },
                {
                    "name": "intent-kimi",
                    "Cond": "req_ai_intent_in(\"task_type\", \"test_writing\")",
                    "targets": [
                        {
                            "ClusterName": "cluster_kimi",
                            "Model": "",
                            "Weight": 100
                        }
                    ],
                    "fallbacks": []
                },
                {
                    "name": "intent-doc",
                    "Cond": "req_ai_intent_in(\"task_type\", \"doc_writing\") && req_ai_intent_in(\"complexity\", \"simple|medium\")",
                    "targets": [
                        {
                            "ClusterName": "cluster_flash",
                            "Model": "",
                            "Weight": 100
                        }
                    ],
                    "fallbacks": []
                },
                {
                    "name": "intent-default",
                    "Cond": "default_t()",
                    "targets": [
                        {
                            "ClusterName": "cluster_kimi",
                            "Model": "",
                            "Weight": 100
                        }
                    ],
                    "fallbacks": []
                }
            ]
        },
        "apikey_ak_plain": {
            "type": "apikey",
            "owner": "ak_plain",
            "rules": [
                {
                    "name": "plain-default",
                    "Cond": "default_t()",
                    "targets": [
                        {
                            "ClusterName": "cluster_kimi",
                            "Model": "",
                            "Weight": 100
                        }
                    ],
                    "fallbacks": []
                }
            ]
        }
    },
    "ApikeyRouteTableBindings": {
        "ak_intent": [
            "apikey_ak_intent"
        ],
        "ak_plain": [
            "apikey_ak_plain"
        ]
    }
}`

func TestTC12_QuestionsHotReload(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE()

	// step 1: v1 questions -> test_writing@0.93 -> intent-flash -> cluster_flash
	resp, body, err := e.sendRequest(apiKeyIntent, unitTestBody, nil)
	e.expectOK(resp, body, err, "step 1")
	if !strings.Contains(body, "from cluster_flash") {
		t.Fatalf("step 1: response should come from cluster_flash, got: %s", body)
	}
	if e.decision.Hits() != 1 {
		t.Fatalf("step 1: expected 1 decision call, got %d", e.decision.Hits())
	}

	// step 2: overwrite questions data (bump Version, add unit_test option)
	// and the route table (new intent-unit rule), then hot reload both.
	questionsPath := filepath.Join(e.confDir, "mod_ai_intent", "intent_questions.data")
	if err := os.WriteFile(questionsPath, []byte(questionsDataV2), 0644); err != nil {
		t.Fatalf("write v2 questions data failed: %v", err)
	}
	routePath := filepath.Join(e.confDir, "mod_ai_route", "ai_route.data")
	if err := os.WriteFile(routePath, []byte(aiRouteDataV2), 0644); err != nil {
		t.Fatalf("write v2 ai_route.data failed: %v", err)
	}
	e.reloadModule("mod_ai_intent", questionsPath)
	e.reloadModule("mod_ai_route", routePath)

	st := e.waitIntentState(5*time.Second, func(st intentModuleState) bool {
		return st.StateData["QUESTIONS_VERSION"] == questionsVersion2
	})
	if got := st.StateData["QUESTIONS_VERSION"]; got != questionsVersion2 {
		t.Fatalf("expected QUESTIONS_VERSION=%s, got %q", questionsVersion2, got)
	}

	// the reloaded questions carry the unit_test option: the same prompt is
	// now classified as unit_test@0.95 and hits the new intent-unit rule.
	e.decision.SetScript(common.DecisionScript{
		"单元测试": unitTestAnswerV2(),
	})

	// step 3: same prompt again -> cache invalidated by the Version bump,
	// re-classified and routed to cluster_kimi by the new rule.
	resp, body, err = e.sendRequest(apiKeyIntent, unitTestBody, nil)
	e.expectOK(resp, body, err, "step 3")
	if !strings.Contains(body, "from cluster_kimi") {
		t.Fatalf("step 3: response should come from cluster_kimi, got: %s", body)
	}
	if e.decision.Hits() != 2 {
		t.Fatalf("step 3: the Version bump must invalidate the cached intent (2 decision calls), got %d", e.decision.Hits())
	}
	if e.flash.Hits() != 1 || e.kimi.Hits() != 1 {
		t.Fatalf("expected flash=1 kimi=1, got flash=%d kimi=%d", e.flash.Hits(), e.kimi.Hits())
	}
}
