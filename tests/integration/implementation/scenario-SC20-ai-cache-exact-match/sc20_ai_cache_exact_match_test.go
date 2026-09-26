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

package sc20

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

	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost   = "cache.example.org"
	apiPath   = "/v1/chat/completions"
	apiKey    = "ak_cache"
	apiKey2   = "ak_cache2"
	apiKeyId  = "cache_key_id"
	apiKeyId2 = "cache_key_id_2"

	clusterCache = "cluster_cache"

	quotaKeyTotal = "quota:plan_total"
)

var (
	questionBody = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"what is bfe?"}]}`)
	otherBody    = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"something else?"}]}`)

	nonStreamAnswer = `{"choices":[{"index":0,"message":{"role":"assistant","content":"BFE is a layer-7 load balancer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":8,"total_tokens":18}}`

	// sseChunks simulates a real provider answering in multiple network
	// frames: role-only first chunk, then two content chunks.
	sseChunks = []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"BFE is"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":" a gateway"}}]}`,
	}
)

// testEnv holds all resources for a single SC20 integration test.
type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backend    *common.MockBackend
	redis      *common.RedisServer
	confDir    string
	logDir     string
	bfePort    int
	stopBFE    func()
}

func newTestEnv(t *testing.T) *testEnv {
	e := &testEnv{t: t}

	e.backend = common.NewMockBackend(clusterCache, http.StatusOK, nonStreamAnswer)
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	return e
}

// unlimitedTokenRule returns a token rule with an unlimited quota plan and
// two API keys (ak_cache / ak_cache2) for tenant isolation tests.
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
				apiKey2: {
					Key:            apiKey2,
					KeyId:          apiKeyId2,
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

// quotaTokenRule returns a token rule with a limited total_token quota plan
// so that billing behavior can be observed through the quota balance.
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
	for key, token := range rule.Tokens["ai_product"] {
		token.QuotaPlans = []string{"plan_total"}
		rule.Tokens["ai_product"][key] = token
	}
	return rule
}

// cacheRule builds an AiCacheRuleData with a single default rule.
func cacheRule(strategy string, ttl int, maxBodyBytes, maxValueBytes int64) *common.AiCacheRuleData {
	return &common.AiCacheRuleData{
		Version: "1.0",
		Config: map[string][]common.AiCacheRule{
			"ai_product": {
				{
					Cond:             "default_t()",
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
	return cacheRule("lastQuestion", 3600, 1048576, 1048576)
}

func (e *testEnv) startBFE(cacheRuleData *common.AiCacheRuleData, tokenRule *common.TokenRuleData) {
	backends := map[string]*common.MockBackend{
		clusterCache: e.backend,
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:     "testdata",
		TargetConfDir:   e.confDir,
		Backends:        backends,
		RedisAddr:       e.redis.Addr(),
		TokenRuleData:   tokenRule,
		AiCacheRuleData: cacheRuleData,
	}
	if err := builder.Build(); err != nil {
		e.t.Fatalf("build bfe config failed: %v", err)
	}

	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	if e.backend != nil {
		e.backend.Close()
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
	return e.sendRequestSkip(apikey, body, false)
}

func (e *testEnv) sendRequestSkip(apikey string, body []byte, skipCache bool) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apikey)
	req.Header.Set("Content-Type", "application/json")
	if skipCache {
		req.Header.Set("x-bfe-skip-ai-cache", "on")
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

// waitQuota polls the quota balance until it equals the expected value or
// the deadline expires (deduction is asynchronous).
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

// TestTC01 verifies the basic exact-match flow: the first request goes
// upstream and is written back to redis; the identical second request is
// served from the cache without calling the upstream again.
func TestTC01_ExactMatchHitAfterMiss(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(defaultCacheRule(), unlimitedTokenRule())

	// first request: miss, upstream called
	resp, body, err := e.sendRequest(apiKey, questionBody)
	e.expectOK(resp, body, err, "first request")
	if !strings.Contains(body, "BFE is a layer-7 load balancer") {
		t.Fatalf("first response should contain upstream answer, got: %s", body)
	}
	if e.backend.Hits() != 1 {
		t.Fatalf("expected 1 upstream call, got %d", e.backend.Hits())
	}
	if !e.waitForCacheKey("ai_cache:cache_key_id:", 5*time.Second) {
		e.logBFEException()
		t.Fatalf("cache key should exist in redis, keys: %v", e.redis.Keys())
	}

	// second request with the same question: hit, no upstream call
	resp, body, err = e.sendRequest(apiKey, questionBody)
	e.expectOK(resp, body, err, "second request")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache != "hit" {
		t.Fatalf("expected X-Bfe-Ai-Cache hit, got %q", cache)
	}
	if !strings.Contains(body, "BFE is a layer-7 load balancer") {
		t.Fatalf("second response should contain cached answer, got: %s", body)
	}
	if e.backend.Hits() != 1 {
		t.Fatalf("cache hit should not call upstream, got %d hits", e.backend.Hits())
	}

	// third request: still a hit (idempotent)
	resp, _, err = e.sendRequest(apiKey, questionBody)
	e.expectOK(resp, body, err, "third request")
	if e.backend.Hits() != 1 {
		t.Fatalf("repeated hits should not call upstream, got %d hits", e.backend.Hits())
	}
}

// TestTC02 verifies that a different question is a cache miss.
func TestTC02_DifferentQuestionMisses(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(defaultCacheRule(), unlimitedTokenRule())

	e.expectOK(e.sendRequest(apiKey, questionBody))
	e.expectOK(e.sendRequest(apiKey, otherBody))

	if e.backend.Hits() != 2 {
		t.Fatalf("different questions should both reach upstream, got %d hits", e.backend.Hits())
	}
}

// TestTC03 verifies that the skip header disables both the cache read and
// the cache write, even when a cache entry already exists.
func TestTC03_SkipHeaderBypassesCache(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(defaultCacheRule(), unlimitedTokenRule())

	// establish the cache entry
	e.expectOK(e.sendRequest(apiKey, questionBody))
	if !e.waitForCacheKey("ai_cache:cache_key_id:", 5*time.Second) {
		t.Fatalf("cache key should exist, keys: %v", e.redis.Keys())
	}

	// skip request with the same question: bypasses the cache read
	resp, body, err := e.sendRequestSkip(apiKey, questionBody, true)
	e.expectOK(resp, body, err, "skip request")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache == "hit" {
		t.Fatal("skip request must not be served from cache")
	}
	if e.backend.Hits() != 2 {
		t.Fatalf("skip request should reach upstream, got %d hits", e.backend.Hits())
	}

	// the skip request must not overwrite the cache entry
	e.expectOK(e.sendRequest(apiKey, questionBody))
	if e.backend.Hits() != 2 {
		t.Fatalf("cache entry should survive the skip request, got %d hits", e.backend.Hits())
	}
}

// TestTC04 verifies streaming with real per-frame flushes: the SSE answer
// is accumulated across chunks, cached, and the identical streaming request
// is served a complete SSE sequence.
func TestTC04_StreamCacheHit(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.backend.SSEEvents = sseChunks
	e.startBFE(defaultCacheRule(), unlimitedTokenRule())

	streamBody := []byte(`{"model":"deepseek-chat","stream":true,"messages":[{"role":"user","content":"what is bfe?"}]}`)

	// first request: miss, upstream SSE answer cached
	resp, body, err := e.sendRequest(apiKey, streamBody)
	e.expectOK(resp, body, err, "first stream request")
	if !strings.Contains(body, "BFE is") || !strings.Contains(body, " a gateway") {
		t.Fatalf("first response should pass through the SSE chunks, got: %s", body)
	}
	if e.backend.Hits() != 1 {
		t.Fatalf("expected 1 upstream call, got %d", e.backend.Hits())
	}
	if !e.waitForCacheKey("ai_cache:cache_key_id:", 5*time.Second) {
		e.logBFEException()
		t.Fatalf("stream answer should be cached, redis keys: %v", e.redis.Keys())
	}

	// second request: hit with a single complete SSE sequence
	resp, body, err = e.sendRequest(apiKey, streamBody)
	e.expectOK(resp, body, err, "second stream request")
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", ct)
	}
	if !strings.Contains(body, "BFE is a gateway") {
		t.Fatalf("cached SSE should contain the joined answer, got: %s", body)
	}
	if !strings.Contains(body, "data:[DONE]") {
		t.Fatalf("cached SSE should end with [DONE], got: %s", body)
	}
	if e.backend.Hits() != 1 {
		t.Fatalf("stream cache hit should not call upstream, got %d hits", e.backend.Hits())
	}
}

// TestTC05 verifies tenant isolation: two API keys asking the same question
// never read each other's cache entries.
func TestTC05_TenantIsolation(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(defaultCacheRule(), unlimitedTokenRule())

	// key 1 asks the question: miss, cached under key 1's id
	e.expectOK(e.sendRequest(apiKey, questionBody))
	if !e.waitForCacheKey("ai_cache:cache_key_id:", 5*time.Second) {
		t.Fatalf("key 1 cache should exist, keys: %v", e.redis.Keys())
	}

	// key 2 asks the same question: must NOT hit key 1's cache
	resp, body, err := e.sendRequest(apiKey2, questionBody)
	e.expectOK(resp, body, err, "key 2 request")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache == "hit" {
		t.Fatal("key 2 must not read key 1's cache entry")
	}
	if e.backend.Hits() != 2 {
		t.Fatalf("key 2 should reach upstream, got %d hits", e.backend.Hits())
	}

	// both keys now hit their own entries independently
	e.expectOK(e.sendRequest(apiKey, questionBody))
	e.expectOK(e.sendRequest(apiKey2, questionBody))
	if e.backend.Hits() != 2 {
		t.Fatalf("each tenant should hit its own cache, got %d hits", e.backend.Hits())
	}
	if !e.waitForCacheKey("ai_cache:cache_key_id_2:", 5*time.Second) {
		t.Fatalf("key 2 cache should exist, keys: %v", e.redis.Keys())
	}
}

// TestTC06 verifies TTL expiry: after the TTL elapses the same question
// misses again and is re-fetched from upstream.
func TestTC06_TTLExpiry(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(cacheRule("lastQuestion", 1, 1048576, 1048576), unlimitedTokenRule())

	e.expectOK(e.sendRequest(apiKey, questionBody))
	if !e.waitForCacheKey("ai_cache:cache_key_id:", 5*time.Second) {
		t.Fatalf("cache key should exist before expiry, keys: %v", e.redis.Keys())
	}

	// advance the redis clock past the 1s TTL
	e.redis.FastForward(2 * time.Second)

	resp, _, err := e.sendRequest(apiKey, questionBody)
	e.expectOK(resp, "", err, "request after expiry")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache == "hit" {
		t.Fatal("expired entry must not be served")
	}
	if e.backend.Hits() != 2 {
		t.Fatalf("expired entry should be re-fetched from upstream, got %d hits", e.backend.Hits())
	}
}

// TestTC07 verifies maxValueBytes: an answer larger than the limit is never
// written back, so identical questions keep missing.
func TestTC07_AnswerOverLimitNotCached(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	// the answer is ~170 bytes; the limit is 16
	e.startBFE(cacheRule("lastQuestion", 3600, 1048576, 16), unlimitedTokenRule())

	e.expectOK(e.sendRequest(apiKey, questionBody))
	if e.waitForCacheKey("ai_cache:", 500*time.Millisecond) {
		t.Fatalf("over-limit answer must not be cached, keys: %v", e.redis.Keys())
	}

	e.expectOK(e.sendRequest(apiKey, questionBody))
	if e.backend.Hits() != 2 {
		t.Fatalf("over-limit answer should be re-fetched every time, got %d hits", e.backend.Hits())
	}
}

// TestTC08 verifies that a failed upstream response is never cached.
func TestTC08_FailedUpstreamNotCached(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(defaultCacheRule(), unlimitedTokenRule())

	// the backend reads part of the request and closes the connection
	// without sending any response
	e.backend.ReadBeforeClose = 10
	resp, _, err := e.sendRequest(apiKey, questionBody)
	if err == nil && resp.StatusCode == http.StatusOK {
		t.Fatal("first request should fail when the backend dies")
	}

	e.backend.ReadBeforeClose = 0
	e.expectOK(e.sendRequest(apiKey, questionBody))
	if e.backend.Hits() != 2 {
		t.Fatalf("failed response must not be cached, got %d hits", e.backend.Hits())
	}
}

// TestTC09 verifies billing coordination: a cache hit skips the token quota
// deduction while a miss deducts the actual usage.
func TestTC09_CacheHitSkipsQuotaDeduction(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(defaultCacheRule(), quotaTokenRule())
	e.redis.SetQuota(quotaKeyTotal, 1000)

	// miss: deducts prompt(10) + completion(8) = 18
	e.expectOK(e.sendRequest(apiKey, questionBody))
	if !e.waitQuota(quotaKeyTotal, 982, 5*time.Second) {
		t.Fatalf("miss should deduct 18 tokens, quota: %d", e.redis.GetQuota(quotaKeyTotal))
	}

	// hit: no deduction
	e.expectOK(e.sendRequest(apiKey, questionBody))
	time.Sleep(500 * time.Millisecond)
	if got := e.redis.GetQuota(quotaKeyTotal); got != 982 {
		t.Fatalf("cache hit must not deduct tokens, quota: %d", got)
	}

	// another miss (different question): deducts again
	e.expectOK(e.sendRequest(apiKey, otherBody))
	if !e.waitQuota(quotaKeyTotal, 964, 5*time.Second) {
		t.Fatalf("second miss should deduct 18 tokens, quota: %d", e.redis.GetQuota(quotaKeyTotal))
	}
}

// TestTC10 verifies the allQuestions strategy: the whole user-message
// history participates in the key, so the same last question with a
// different earlier turn must miss.
func TestTC10_AllQuestionsStrategy(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(cacheRule("allQuestions", 3600, 1048576, 1048576), unlimitedTokenRule())

	conversationA := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"my name is tom"},{"role":"assistant","content":"nice to meet you"},{"role":"user","content":"what is my name?"}]}`)
	// same last question, different history
	conversationB := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"my name is jerry"},{"role":"assistant","content":"nice to meet you"},{"role":"user","content":"what is my name?"}]}`)

	// first time of conversation A: miss
	e.expectOK(e.sendRequest(apiKey, conversationA))
	if e.backend.Hits() != 1 {
		t.Fatalf("conversation A should reach upstream, got %d hits", e.backend.Hits())
	}

	// identical conversation A: hit
	resp, _, err := e.sendRequest(apiKey, conversationA)
	e.expectOK(resp, "", err, "repeat conversation A")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache != "hit" {
		t.Fatalf("identical conversation should hit, got %q", cache)
	}

	// same last question, different history: must miss under allQuestions
	e.expectOK(e.sendRequest(apiKey, conversationB))
	if e.backend.Hits() != 2 {
		t.Fatalf("different history should miss under allQuestions, got %d hits", e.backend.Hits())
	}
}

// TestTC11 verifies maxBodyBytes: an oversized request body is never read
// for caching and nothing is cached.
func TestTC11_OversizedRequestBodyNotCached(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	// the request body is ~120 bytes; the limit is 64
	e.startBFE(cacheRule("lastQuestion", 3600, 64, 1048576), unlimitedTokenRule())

	e.expectOK(e.sendRequest(apiKey, questionBody))
	if e.waitForCacheKey("ai_cache:", 500*time.Millisecond) {
		t.Fatalf("oversized request must not be cached, keys: %v", e.redis.Keys())
	}

	e.expectOK(e.sendRequest(apiKey, questionBody))
	if e.backend.Hits() != 2 {
		t.Fatalf("oversized request should reach upstream every time, got %d hits", e.backend.Hits())
	}
}
