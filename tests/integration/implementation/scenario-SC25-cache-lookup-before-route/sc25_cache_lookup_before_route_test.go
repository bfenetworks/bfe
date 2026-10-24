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

package sc25

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bfe_access_pb "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost  = "front.example.org"
	apiPath  = "/v1/chat/completions"
	apiKey   = "ak_front"
	apiKeyId = "front_key_id"

	clusterFlash = "cluster_flash"
	clusterKimi  = "cluster_kimi"

	cacheKeyPrefix = "ai_cache:front_key_id:"
)

var (
	unitTestBody = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"帮我给这个函数写单元测试"}]}`)
	docBody      = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"写文档：补充这个模块的 README"}]}`)

	flashAnswer = `{"id":"chatcmpl-flash","choices":[{"index":0,"message":{"role":"assistant","content":"from cluster_flash"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`
	kimiAnswer  = `{"id":"chatcmpl-kimi","choices":[{"index":0,"message":{"role":"assistant","content":"from cluster_kimi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`
)

// testEnv holds all resources for a single SC25 integration test.
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
// keyword (longest keyword wins, see common.DecisionScript). Prompts without
// a keyword get an empty-answers response (all questions unknown).
func baseDecisionScript() common.DecisionScript {
	return common.DecisionScript{
		"单元测试": testWritingAnswer(0.93),
		"写文档":  docWritingAnswer(0.90),
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

func docWritingAnswer(confidence float64) common.DecisionResponse {
	return common.DecisionResponse{
		Status: http.StatusOK,
		Answers: map[string]common.DecisionAnswer{
			"task_type": {
				Type:             "choice",
				Choice:           "doc_writing",
				Probabilities:    map[string]float64{"coding": 0.03, "test_writing": 0.02, "doc_writing": confidence},
				AnswerConfidence: confidence,
			},
		},
	}
}

// unlimitedTokenRule returns a token rule with an unlimited quota plan and
// the single API key of this scenario.
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

// cacheRule builds an AiCacheRuleData with a single rule for ai_product.
func cacheRule(cond, strategy string, ttl int, maxBodyBytes, maxValueBytes int64) *common.AiCacheRuleData {
	return &common.AiCacheRuleData{
		Version: "1.0",
		Config: map[string][]common.AiCacheRule{
			"ai_product": {
				{
					Cond:             cond,
					CacheKeyStrategy: strategy,
					CacheTTL:         ttl,
					MaxBodyBytes:     maxBodyBytes,
					MaxValueBytes:    maxValueBytes,
				},
			},
		},
	}
}

func defaultCacheRule() *common.AiCacheRuleData {
	return cacheRule("default_t()", "lastQuestion", 3600, 1048576, 1048576)
}

func (e *testEnv) startBFE(cacheRuleData *common.AiCacheRuleData) {
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
		AiCacheRuleData:     cacheRuleData,
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

func (e *testEnv) sendRequest(apikey string, body []byte) (*http.Response, string, error) {
	return e.sendRequestHeaders(apikey, body, nil)
}

func (e *testEnv) sendRequestHeaders(apikey string, body []byte, headers map[string]string) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	if apikey != "" {
		req.Header.Set("Authorization", "Bearer "+apikey)
	}
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

// waitForCacheKey polls redis until a key with the given prefix appears or
// the deadline expires. The cache write-back happens when BFE closes the
// response body, which is not strictly ordered with the client receiving
// the full response, so a short poll avoids a startup race.
func (e *testEnv) waitForCacheKey(prefix string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, k := range e.redis.Keys() {
			if strings.HasPrefix(k, prefix) {
				return true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// accessLogs parses the b2log records written by mod_access_pb3; BFE must be
// stopped first so all buffered logs are flushed.
func (e *testEnv) accessLogs() []*bfe_access_pb.RequestLog {
	e.t.Helper()
	reqLogs, err := common.ParseAccessLogAfterStop(e.logDir)
	if err != nil {
		e.t.Fatalf("parse access log failed: %v", err)
	}
	return reqLogs
}

// TestTC01 verifies the core lazy-resolve-sinking benefit: the first request
// misses, classifies once and reaches cluster_flash; the identical second
// request hits the cache and must NOT call the decision service again — the
// hit short-circuits before mod_ai_route evaluates the req_ai_intent_in
// rule (which is what triggers the lazy intent resolve).
func TestTC01_HitSkipsIntentResolve(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(defaultCacheRule())

	// first request: miss, one decision call, routed by intent to flash
	resp, body, err := e.sendRequest(apiKey, unitTestBody)
	e.expectOK(resp, body, err, "first request")
	if !strings.Contains(body, "from cluster_flash") {
		t.Fatalf("first response should come from cluster_flash, got: %s", body)
	}
	if e.decision.Calls() != 1 {
		t.Fatalf("first request should call decision service once, got %d", e.decision.Calls())
	}
	if e.flash.Hits() != 1 {
		t.Fatalf("expected 1 upstream call, got %d", e.flash.Hits())
	}
	if !e.waitForCacheKey(cacheKeyPrefix, 5*time.Second) {
		e.logBFEException()
		t.Fatalf("cache key should exist in redis, keys: %v", e.redis.Keys())
	}

	// second request: hit — decision service and upstream must stay untouched
	resp, body, err = e.sendRequest(apiKey, unitTestBody)
	e.expectOK(resp, body, err, "second request")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache != "hit" {
		t.Fatalf("expected X-Bfe-Ai-Cache hit, got %q", cache)
	}
	if !strings.Contains(body, "from cluster_flash") {
		t.Fatalf("second response should contain cached answer, got: %s", body)
	}
	if e.decision.Calls() != 1 {
		t.Fatalf("cache hit must not call decision service again, got %d calls", e.decision.Calls())
	}
	if e.flash.Hits() != 1 {
		t.Fatalf("cache hit should not call upstream, got %d hits", e.flash.Hits())
	}
	if e.kimi.Hits() != 0 {
		t.Fatalf("cluster_kimi should not be hit at all, got %d", e.kimi.Hits())
	}
}

// TestTC02 is the control case: two different questions both miss, so each
// must classify and route normally — the lookup move must not change the
// miss-path routing semantics.
func TestTC02_MissPathRoutingUnchanged(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(defaultCacheRule())

	resp, body, err := e.sendRequest(apiKey, unitTestBody)
	e.expectOK(resp, body, err, "unit test request")
	if !strings.Contains(body, "from cluster_flash") {
		t.Fatalf("test_writing should route to cluster_flash, got: %s", body)
	}

	resp, body, err = e.sendRequest(apiKey, docBody)
	e.expectOK(resp, body, err, "doc request")
	if !strings.Contains(body, "from cluster_kimi") {
		t.Fatalf("doc_writing should fall through to cluster_kimi, got: %s", body)
	}

	if e.decision.Calls() != 2 {
		t.Fatalf("each miss should classify once, got %d calls", e.decision.Calls())
	}
	if e.flash.Hits() != 1 || e.kimi.Hits() != 1 {
		t.Fatalf("expected 1 hit per backend, got flash=%d kimi=%d", e.flash.Hits(), e.kimi.Hits())
	}
}

// TestTC03 locks the access-log shape of the hit path: a hit request has
// ai_cache_status=hit and no route result (ai_route_rule_hits /
// ai_target_model) nor intent fields (ai_intent_*) at all, because the
// request finished before mod_ai_route evaluated any rule.
func TestTC03_HitRequestAccessLogFields(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(defaultCacheRule())

	e.expectOK(e.sendRequest(apiKey, unitTestBody))
	if !e.waitForCacheKey(cacheKeyPrefix, 5*time.Second) {
		t.Fatalf("cache key should exist, keys: %v", e.redis.Keys())
	}
	resp, body, err := e.sendRequest(apiKey, unitTestBody)
	e.expectOK(resp, body, err, "hit request")
	if resp.Header.Get("X-Bfe-Ai-Cache") != "hit" {
		t.Fatalf("second request should hit cache")
	}

	// wait for the access log to be flushed before stopping BFE
	time.Sleep(500 * time.Millisecond)
	e.stopBFE()
	e.stopBFE = nil

	reqLogs := e.accessLogs()
	if len(reqLogs) != 2 {
		e.logBFEException()
		t.Fatalf("expected 2 access logs, got %d", len(reqLogs))
	}

	// miss log: route ran, intent resolved
	miss := reqLogs[0]
	if miss.AiCacheStatus == nil || *miss.AiCacheStatus != "miss" {
		t.Errorf("miss log: ai_cache_status should be miss, got: %v", miss.AiCacheStatus)
	}
	if len(miss.AiRouteRuleHits) != 1 || *miss.AiRouteRuleHits[0].RuleName != "intent-flash" {
		t.Errorf("miss log: expected 1 route rule hit (intent-flash), got: %v", miss.AiRouteRuleHits)
	}
	if miss.AiTargetModel == nil || *miss.AiTargetModel == "" {
		t.Errorf("miss log: ai_target_model should be set, got: %v", miss.AiTargetModel)
	}
	if miss.AiIntentQuestion == nil || *miss.AiIntentQuestion != "task_type" {
		t.Errorf("miss log: ai_intent_question should be task_type, got: %v", miss.AiIntentQuestion)
	}
	if miss.AiIntentAnswer == nil || *miss.AiIntentAnswer != "test_writing" {
		t.Errorf("miss log: ai_intent_answer should be test_writing, got: %v", miss.AiIntentAnswer)
	}

	// hit log: finished before routing — no route result, no intent fields.
	// ai_target_model still carries the client model: InitAiBasicInfo sets
	// TargetModel = client model before any routing, and a cache hit never
	// reaches the routing overwrite.
	hit := reqLogs[1]
	if hit.AiCacheStatus == nil || *hit.AiCacheStatus != "hit" {
		t.Errorf("hit log: ai_cache_status should be hit, got: %v", hit.AiCacheStatus)
	}
	if len(hit.AiRouteRuleHits) != 0 {
		t.Errorf("hit log: ai_route_rule_hits should be empty, got: %v", hit.AiRouteRuleHits)
	}
	if hit.AiTargetModel == nil || *hit.AiTargetModel != "deepseek-chat" {
		t.Errorf("hit log: ai_target_model should carry the client model, got: %v", hit.AiTargetModel)
	}
	if hit.AiIntentQuestion != nil || hit.AiIntentAnswer != nil ||
		hit.AiIntentConfidence != nil || hit.AiIntentSource != nil {
		t.Errorf("hit log: ai_intent_* fields should all be absent, got q=%v a=%v c=%v s=%v",
			hit.AiIntentQuestion, hit.AiIntentAnswer, hit.AiIntentConfidence, hit.AiIntentSource)
	}
}

// TestTC04 verifies the skip header semantics under the new callback
// position: a skipped request neither reads nor writes the cache and goes
// through the full chain (auth -> cache skip -> route -> intent -> upstream).
func TestTC04_SkipHeaderFullPath(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(defaultCacheRule())

	// establish the cache entry
	e.expectOK(e.sendRequest(apiKey, unitTestBody))
	if !e.waitForCacheKey(cacheKeyPrefix, 5*time.Second) {
		t.Fatalf("cache key should exist, keys: %v", e.redis.Keys())
	}

	// skip request with the same question: bypasses the cache read
	resp, body, err := e.sendRequestHeaders(apiKey, unitTestBody, map[string]string{
		"x-bfe-skip-ai-cache": "on",
	})
	e.expectOK(resp, body, err, "skip request")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache == "hit" {
		t.Fatal("skip request must not be served from cache")
	}
	if !strings.Contains(body, "from cluster_flash") {
		t.Fatalf("skip request should route by intent to cluster_flash, got: %s", body)
	}
	// the decision call count stays 1: the in-process intent LRU absorbs the
	// re-classification of the same prompt; the full-chain evidence is the
	// upstream call plus intent routing above.
	if e.decision.Calls() != 1 {
		t.Fatalf("intent LRU should absorb the skip request re-classification, got %d calls", e.decision.Calls())
	}
	if e.flash.Hits() != 2 {
		t.Fatalf("skip request should reach upstream, got %d hits", e.flash.Hits())
	}

	// the skip request must not overwrite the cache entry
	resp, _, err = e.sendRequest(apiKey, unitTestBody)
	e.expectOK(resp, "", err, "request after skip")
	if resp.Header.Get("X-Bfe-Ai-Cache") != "hit" {
		t.Fatal("cache entry should survive the skip request")
	}
	if e.flash.Hits() != 2 {
		t.Fatalf("cached entry should be served after skip, got %d hits", e.flash.Hits())
	}
}

// TestTC05 verifies tolerant loading of a cache rule whose cond references
// req_ai_intent_in: BFE starts fine, the rule works (miss then hit), and the
// intent resolve is triggered by the cache-cond evaluation (the WARN about
// negating the lookup-early benefit is locked by the module unit test).
func TestTC05_IntentCondCacheRuleTolerantLoading(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	intentCondRule := cacheRule(`req_ai_intent_in("task_type", "doc_writing") && default_t()`,
		"lastQuestion", 3600, 1048576, 1048576)
	e.startBFE(intentCondRule)

	// first request: cache cond evaluates the intent primitive (1 decision
	// call), cond matches (doc_writing), cache miss -> upstream via default rule
	resp, body, err := e.sendRequest(apiKey, docBody)
	e.expectOK(resp, body, err, "first request")
	if !strings.Contains(body, "from cluster_kimi") {
		t.Fatalf("doc_writing should route to cluster_kimi, got: %s", body)
	}
	if e.decision.Calls() != 1 {
		t.Fatalf("cache cond should trigger one intent resolve, got %d calls", e.decision.Calls())
	}
	if e.kimi.Hits() != 1 {
		t.Fatalf("expected 1 upstream call, got %d", e.kimi.Hits())
	}
	if !e.waitForCacheKey(cacheKeyPrefix, 5*time.Second) {
		t.Fatalf("cache key should exist, keys: %v", e.redis.Keys())
	}

	// second request: hit (cond matches again via the in-process intent LRU)
	resp, body, err = e.sendRequest(apiKey, docBody)
	e.expectOK(resp, body, err, "second request")
	if resp.Header.Get("X-Bfe-Ai-Cache") != "hit" {
		t.Fatalf("second request should hit cache, header: %v", resp.Header)
	}
	if e.decision.Calls() != 1 {
		t.Fatalf("intent LRU should absorb the second evaluation, got %d calls", e.decision.Calls())
	}
	if e.kimi.Hits() != 1 {
		t.Fatalf("cache hit should not call upstream, got %d hits", e.kimi.Hits())
	}
}

// TestTC06 is the safety regression: cache lookup is registered AFTER
// mod_ai_token_auth, so an unauthenticated request must never be served
// from the cache.
func TestTC06_UnauthenticatedNoCache(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(defaultCacheRule())

	// establish the cache entry with a valid key
	e.expectOK(e.sendRequest(apiKey, unitTestBody))
	if !e.waitForCacheKey(cacheKeyPrefix, 5*time.Second) {
		t.Fatalf("cache key should exist, keys: %v", e.redis.Keys())
	}

	// same question without any API key: rejected by auth before cache lookup
	resp, body, err := e.sendRequest("", unitTestBody)
	if err != nil {
		e.logBFEException()
		t.Fatalf("unauthenticated request failed: %v", err)
	}
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("unauthenticated request must not succeed, body: %s", body)
	}
	if strings.Contains(body, "from cluster_flash") {
		t.Fatalf("unauthenticated request must not receive the cached answer, body: %s", body)
	}
	if e.decision.Calls() != 1 {
		t.Fatalf("auth rejection must happen before any intent resolve, got %d calls", e.decision.Calls())
	}
	if e.flash.Hits() != 1 || e.kimi.Hits() != 0 {
		t.Fatalf("auth rejection must happen before upstream, flash=%d kimi=%d", e.flash.Hits(), e.kimi.Hits())
	}
}
