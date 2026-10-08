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

package mod_ai_cache

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_modules/mod_ai_cache/provider/vector"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

func newTestRequest(product, body string) *bfe_basic.Request {
	var httpReq *bfe_http.Request
	if body == "" {
		httpReq, _ = bfe_http.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions", nil)
	} else {
		httpReq, _ = bfe_http.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions",
			ioutil.NopCloser(bytes.NewBufferString(body)))
	}
	httpReq.Header.Set("Content-Type", "application/json")
	req := bfe_basic.NewRequest(httpReq, nil, nil, nil, nil)
	req.Route = bfe_basic.RequestRoute{Product: product}
	return req
}

func prepareTestModule(t *testing.T) *ModuleAiCache {
	m := NewModuleAiCache()
	m.productConfPath = "testdata/mod_ai_cache/mod_ai_cache_rule.data"
	if _, err := m.loadProductRuleTable(nil); err != nil {
		t.Fatalf("loadProductRuleTable failed: %s", err)
	}
	return m
}

func chatBody(question string, stream bool) string {
	streamStr := "false"
	if stream {
		streamStr = "true"
	}
	return `{"model":"deepseek-chat","stream":` + streamStr + `,"messages":[{"role":"system","content":"sys"},{"role":"user","content":"` + question + `"}]}`
}

func TestBuildCacheKeyLastQuestion(t *testing.T) {
	m := NewModuleAiCache()
	m.cacheKeyPrefix = "ai_cache"

	req := newTestRequest("default", "")
	ai := req.InitAiBasicInfo()
	ai.ClientKeyId = "key_001"

	rule := &ProductRuleConf{
		CacheKeyStrategy: CacheKeyStrategyLastQuestion,
		CacheKeyFrom:     "",
	}

	body := []byte(chatBody("hello", false))
	question, err := extractQuestion(body, rule)
	if err != nil || question != "hello" {
		t.Fatalf("extractQuestion failed: question=%q err=%v", question, err)
	}
	key1, stream := m.buildCacheKey(req, question, body)
	if key1 == "" {
		t.Fatal("buildCacheKey failed")
	}
	if stream {
		t.Error("stream should be false")
	}

	// same question produces the same key (system prompt differences are ignored)
	body2 := []byte(`{"model":"other-model","messages":[{"role":"system","content":"different"},{"role":"user","content":"hello"}]}`)
	question2, err := extractQuestion(body2, rule)
	if err != nil {
		t.Fatalf("extractQuestion failed: %v", err)
	}
	key2, _ := m.buildCacheKey(req, question2, body2)
	if key1 != key2 {
		t.Errorf("same question should produce the same key: %s vs %s", key1, key2)
	}

	// different tenant produces a different key
	ai.ClientKeyId = "key_002"
	key3, _ := m.buildCacheKey(req, question, body)
	if key1 == key3 {
		t.Errorf("different tenant should produce different keys")
	}

	// stream flag detected
	streamBody := []byte(chatBody("hello", true))
	question3, err := extractQuestion(streamBody, rule)
	if err != nil {
		t.Fatalf("extractQuestion failed: %v", err)
	}
	_, stream = m.buildCacheKey(req, question3, streamBody)
	if !stream {
		t.Error("stream should be true")
	}
}

func TestBuildCacheKeyAllQuestions(t *testing.T) {
	m := NewModuleAiCache()
	m.cacheKeyPrefix = "ai_cache"

	req := newTestRequest("default", "")
	ai := req.InitAiBasicInfo()
	ai.ClientKeyId = "key_001"

	rule := &ProductRuleConf{CacheKeyStrategy: CacheKeyStrategyAllQuestions}

	multiTurn := `{"messages":[{"role":"user","content":"q1"},{"role":"assistant","content":"a1"},{"role":"user","content":"q2"}]}`
	question, err := extractQuestion([]byte(multiTurn), rule)
	if err != nil || question == "" {
		t.Fatalf("extractQuestion failed: question=%q err=%v", question, err)
	}
	key, _ := m.buildCacheKey(req, question, []byte(multiTurn))
	if key == "" {
		t.Fatal("buildCacheKey failed")
	}

	// order change produces a different key
	reordered := `{"messages":[{"role":"user","content":"q2"},{"role":"assistant","content":"a1"},{"role":"user","content":"q1"}]}`
	question2, _ := extractQuestion([]byte(reordered), rule)
	key2, _ := m.buildCacheKey(req, question2, []byte(reordered))
	if key == key2 {
		t.Error("reordered questions should produce different keys")
	}
}

func TestExtractQuestionInvalidJson(t *testing.T) {
	rule := &ProductRuleConf{CacheKeyStrategy: CacheKeyStrategyLastQuestion}
	if _, err := extractQuestion([]byte("not json"), rule); err == nil {
		t.Error("invalid json should return an error")
	}
}

func TestBuildCacheKeyEmptyQuestion(t *testing.T) {
	m := NewModuleAiCache()
	req := newTestRequest("default", "")
	req.InitAiBasicInfo()

	rule := &ProductRuleConf{CacheKeyStrategy: CacheKeyStrategyLastQuestion}
	question, err := extractQuestion([]byte(`{"messages":[]}`), rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if question != "" {
		t.Errorf("empty question expected, got %q", question)
	}
	key, _ := m.buildCacheKey(req, question, []byte(`{"messages":[]}`))
	if key != "" {
		t.Errorf("empty question should produce empty key, got %s", key)
	}
}

func TestJsonQuote(t *testing.T) {
	got := jsonQuote("a\"b\\c\nd")
	if got != `a\"b\\c\nd` {
		t.Errorf("unexpected quoting: %q", got)
	}
}

func TestBuildHitResponseNonStream(t *testing.T) {
	m := NewModuleAiCache()
	req := newTestRequest("default", "")
	req.InitAiBasicInfo()

	rule := &ProductRuleConf{
		ResponseTemplate:       DefaultResponseTemplate,
		StreamResponseTemplate: DefaultStreamResponseTmpl,
	}

	value := "hi \"there\"\nbye"
	res := m.buildHitResponse(req, rule, value, false)
	if res.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	body, _ := ioutil.ReadAll(res.Body)
	parsed := gjson.GetBytes(body, "choices.0.message.content")
	if parsed.String() != value {
		t.Errorf("expected content %q, got %q", value, parsed.String())
	}
}

func TestBuildHitResponseStream(t *testing.T) {
	m := NewModuleAiCache()
	req := newTestRequest("default", "")
	req.InitAiBasicInfo()

	rule := &ProductRuleConf{
		ResponseTemplate:       DefaultResponseTemplate,
		StreamResponseTemplate: DefaultStreamResponseTmpl,
	}

	res := m.buildHitResponse(req, rule, "cached answer", true)
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}

	body, _ := ioutil.ReadAll(res.Body)
	s := string(body)
	if !strings.HasPrefix(s, "data:") {
		t.Errorf("expected SSE data frame, got %q", s)
	}
	if !strings.Contains(s, `"content":"cached answer"`) {
		t.Errorf("expected cached content in frame, got %q", s)
	}
	if !strings.Contains(s, "data:[DONE]") {
		t.Errorf("expected [DONE] frame, got %q", s)
	}
	if !strings.HasSuffix(s, "\n\n") {
		t.Errorf("expected trailing blank line, got %q", s)
	}
}

func TestCacheRequestHandlerMissAndHit(t *testing.T) {
	fake := newFakeRedisClient("")
	m := prepareTestModule(t)
	m.redisCache = newRedisCache(m.name, fake, m.ruleTable)

	// miss: context saved, status miss, request continues
	req := newTestRequest("default", chatBody("what is bfe", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"

	ret, res := m.cacheRequestHandler(req)
	if ret != bfe_module.BfeHandlerGoOn || res != nil {
		t.Fatalf("expected pass on miss, got ret=%d", ret)
	}
	if getAiCacheContext(req) == nil {
		t.Error("cache context should be saved on miss")
	}
	if ai := req.GetAiBasicInfo(); ai.AiCacheStatus != CacheStatusMiss {
		t.Errorf("expected status miss, got %q", ai.AiCacheStatus)
	}

	// hit: second request with the same question is served from cache
	fake.value = "BFE is a layer-7 load balancer"
	key := getAiCacheContext(req).Key

	req2 := newTestRequest("default", chatBody("what is bfe", false))
	req2.InitAiBasicInfo().ClientKeyId = "key_001"

	ret2, res2 := m.cacheRequestHandler(req2)
	if ret2 != bfe_module.BfeHandlerFinish || res2 == nil {
		t.Fatalf("expected finish on hit, got ret=%d", ret2)
	}
	if fake.lastGetKey != key {
		t.Errorf("expected redis GET with key %s, got %s", key, fake.lastGetKey)
	}
	if ai := req2.GetAiBasicInfo(); !ai.AiCacheHit || ai.AiCacheStatus != CacheStatusHit {
		t.Errorf("expected cache hit recorded, got hit=%v status=%q", ai.AiCacheHit, ai.AiCacheStatus)
	}
	body, _ := ioutil.ReadAll(res2.Body)
	if !strings.Contains(string(body), "BFE is a layer-7 load balancer") {
		t.Errorf("hit response should contain cached value, got %q", body)
	}
}

func TestCacheRequestHandlerSkip(t *testing.T) {
	fake := newFakeRedisClient("cached")
	m := prepareTestModule(t)
	m.redisCache = newRedisCache(m.name, fake, m.ruleTable)

	req := newTestRequest("default", chatBody("q", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"
	req.HttpRequest.Header.Set(HeaderSkipAiCache, SkipCacheOn)

	ret, res := m.cacheRequestHandler(req)
	if ret != bfe_module.BfeHandlerGoOn || res != nil {
		t.Fatalf("skip header should pass the request, got ret=%d", ret)
	}
	if fake.getCalls != 0 {
		t.Error("skip header should not trigger redis GET")
	}
	if getAiCacheContext(req) != nil {
		t.Error("skip header should not save cache context")
	}
	if ai := req.GetAiBasicInfo(); ai.AiCacheStatus != CacheStatusSkip {
		t.Errorf("expected status skip, got %q", ai.AiCacheStatus)
	}
}

func TestCacheRequestHandlerProductNotFound(t *testing.T) {
	m := prepareTestModule(t)

	req := newTestRequest("no_such_product", chatBody("q", false))
	req.InitAiBasicInfo()

	ret, _ := m.cacheRequestHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("unknown product should pass, got ret=%d", ret)
	}
	if ai := req.GetAiBasicInfo(); ai.AiCacheStatus != "" {
		t.Errorf("status should stay empty for unknown product, got %q", ai.AiCacheStatus)
	}
}

func TestCacheRequestHandlerDisabledStrategy(t *testing.T) {
	m := prepareTestModule(t)

	req := newTestRequest("disabled_product", chatBody("q", false))
	req.InitAiBasicInfo()

	ret, _ := m.cacheRequestHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("disabled strategy should pass, got ret=%d", ret)
	}
	if getAiCacheContext(req) != nil {
		t.Error("disabled strategy should not save cache context")
	}
}

// fakeRedisClient implements redis_client.Client for unit tests.
type fakeRedisClient struct {
	value      string
	getCalls   int
	lastGetKey string
	setexCalls int
	lastSetKey string
	lastSetVal []byte
	lastSetTTL int
}

func newFakeRedisClient(value string) *fakeRedisClient {
	return &fakeRedisClient{value: value}
}

func (f *fakeRedisClient) Setex(key string, value []byte, expire int) error {
	f.setexCalls++
	f.lastSetKey = key
	f.lastSetVal = value
	f.lastSetTTL = expire
	return nil
}

func (f *fakeRedisClient) Get(key string) (interface{}, error) {
	f.getCalls++
	f.lastGetKey = key
	if f.value == "" {
		return nil, nil
	}
	return f.value, nil
}

func (f *fakeRedisClient) Expire(key string, expire int) error { return nil }
func (f *fakeRedisClient) Incr(key string) (int64, error)      { return 0, nil }
func (f *fakeRedisClient) IncrAndExpire(key string, expire int) (int64, error) {
	return 0, nil
}
func (f *fakeRedisClient) Decr(key string) (int64, error) { return 0, nil }
func (f *fakeRedisClient) PIncr(keys []string) ([]int64, error) {
	return make([]int64, len(keys)), nil
}
func (f *fakeRedisClient) GetInt64(key string) (int64, error) { return 0, nil }
func (f *fakeRedisClient) GetInt64Batch(keys []string) ([]int64, error) {
	return make([]int64, len(keys)), nil
}
func (f *fakeRedisClient) IncrBy(key string, delta int64) (int64, error) { return 0, nil }
func (f *fakeRedisClient) Delete(key string) error                       { return nil }
func (f *fakeRedisClient) HGetAll(key string) (map[string]string, error) {
	return nil, nil
}

func (f *fakeRedisClient) NewScript(src string) redis_client.RedisScript {
	return nil
}

// prepareSemanticTestModule loads the rule file with the Semantic block and
// wires fake embedding / vector providers into the module.
func prepareSemanticTestModule(t *testing.T, emb *fakeEmbeddingProvider, vec *fakeVectorProvider) *ModuleAiCache {
	m := prepareTestModule(t)
	m.productConfPath = "testdata/mod_ai_cache/mod_ai_cache_rule_semantic.data"
	if _, err := m.loadProductRuleTable(nil); err != nil {
		t.Fatalf("loadProductRuleTable failed: %s", err)
	}
	m.semantic = newSemanticCache(emb, vec, DefaultMaxQuestionBytes, m.ruleTable)
	return m
}

func TestCacheRequestHandlerExactHitSkipsEmbedding(t *testing.T) {
	// exact hit has priority: redis hit must not trigger any embedding call
	fake := newFakeRedisClient("exact cached answer")
	emb := &fakeEmbeddingProvider{vec: []float32{0.1}}
	vec := &fakeVectorProvider{}
	m := prepareSemanticTestModule(t, emb, vec)
	m.redisCache = newRedisCache(m.name, fake, m.ruleTable)

	req := newTestRequest("default", chatBody("what is bfe", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"

	ret, res := m.cacheRequestHandler(req)
	if ret != bfe_module.BfeHandlerFinish || res == nil {
		t.Fatalf("expected finish on exact hit, got ret=%d", ret)
	}
	if emb.calls != 0 {
		t.Errorf("exact hit must not call embedding, got %d calls", emb.calls)
	}
	if vec.queryCalls != 0 {
		t.Errorf("exact hit must not call vector query, got %d calls", vec.queryCalls)
	}
	ai := req.GetAiBasicInfo()
	if !ai.AiCacheHit || ai.AiCacheStatus != CacheStatusHit || ai.AiCacheSemantic {
		t.Errorf("unexpected ai cache info: %+v", ai)
	}
	if res.Header.Get("X-Bfe-Ai-Cache") != CacheStatusHit {
		t.Errorf("unexpected X-Bfe-Ai-Cache header: %s", res.Header.Get("X-Bfe-Ai-Cache"))
	}
}

func TestCacheRequestHandlerSemanticHit(t *testing.T) {
	fake := newFakeRedisClient("")
	emb := &fakeEmbeddingProvider{vec: []float32{0.1, 0.2}}
	vec := &fakeVectorProvider{results: []vector.Result{
		{ID: "id1", Question: "what is bfe", Answer: "semantic cached answer",
			Score: 0.1, Similarity: 0.9},
	}}
	m := prepareSemanticTestModule(t, emb, vec)
	m.redisCache = newRedisCache(m.name, fake, m.ruleTable)

	req := newTestRequest("default", chatBody("what is bfe", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"

	ret, res := m.cacheRequestHandler(req)
	if ret != bfe_module.BfeHandlerFinish || res == nil {
		t.Fatalf("expected finish on semantic hit, got ret=%d", ret)
	}

	ai := req.GetAiBasicInfo()
	if !ai.AiCacheHit || ai.AiCacheStatus != CacheStatusHitSemantic || !ai.AiCacheSemantic {
		t.Errorf("semantic hit should set hit/hit_semantic/semantic, got: %+v", ai)
	}
	if ai.AiCacheSimilarity != 0.9 {
		t.Errorf("unexpected similarity: %f", ai.AiCacheSimilarity)
	}
	if res.Header.Get("X-Bfe-Ai-Cache") != CacheStatusHitSemantic {
		t.Errorf("unexpected X-Bfe-Ai-Cache header: %s", res.Header.Get("X-Bfe-Ai-Cache"))
	}
	body, _ := ioutil.ReadAll(res.Body)
	if !strings.Contains(string(body), "semantic cached answer") {
		t.Errorf("hit response should contain the semantic answer, got %q", body)
	}

	// the embedding is kept for the write-back reuse, not recomputed
	if emb.calls != 1 {
		t.Errorf("embedding should be computed exactly once, got %d", emb.calls)
	}
	if vec.lastQueryTenant != "key_001" {
		t.Errorf("vector query must be tenant-scoped, got %s", vec.lastQueryTenant)
	}
	if vec.lastQueryTTL != 60*time.Second {
		t.Errorf("vector query ttl should be the rule cacheTTL, got %v", vec.lastQueryTTL)
	}
}

func TestCacheRequestHandlerSemanticMiss(t *testing.T) {
	fake := newFakeRedisClient("")
	emb := &fakeEmbeddingProvider{vec: []float32{0.1, 0.2}}
	vec := &fakeVectorProvider{} // no neighbors
	m := prepareSemanticTestModule(t, emb, vec)
	m.redisCache = newRedisCache(m.name, fake, m.ruleTable)

	req := newTestRequest("default", chatBody("what is bfe", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"

	ret, res := m.cacheRequestHandler(req)
	if ret != bfe_module.BfeHandlerGoOn || res != nil {
		t.Fatalf("expected pass on miss, got ret=%d", ret)
	}
	ai := req.GetAiBasicInfo()
	if ai.AiCacheStatus != CacheStatusMiss || ai.AiCacheHit || ai.AiCacheSemantic {
		t.Errorf("unexpected ai cache info: %+v", ai)
	}
	ctx := getAiCacheContext(req)
	if ctx == nil {
		t.Fatal("cache context should be saved on miss")
	}
	if ctx.Question != "what is bfe" {
		t.Errorf("context should carry the question, got %q", ctx.Question)
	}
	if len(ctx.Embedding) != 2 {
		t.Errorf("context should carry the computed embedding for the write-back, got %v", ctx.Embedding)
	}
}

func TestCacheRequestHandlerSemanticEmbeddingFailure(t *testing.T) {
	fake := newFakeRedisClient("")
	emb := &fakeEmbeddingProvider{err: errors.New("embedding down")}
	vec := &fakeVectorProvider{}
	m := prepareSemanticTestModule(t, emb, vec)
	m.redisCache = newRedisCache(m.name, fake, m.ruleTable)

	req := newTestRequest("default", chatBody("what is bfe", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"

	ret, _ := m.cacheRequestHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected pass on embedding failure, got ret=%d", ret)
	}
	if getAiCacheContext(req).Embedding != nil {
		t.Error("embedding must stay nil when the embedding call failed")
	}
	if m.ruleTable.snapshotCounters().embeddingErr != 1 {
		t.Error("EMBEDDING_ERR counter should be incremented")
	}
}

func TestCacheRequestHandlerSemanticQuestionTooLong(t *testing.T) {
	fake := newFakeRedisClient("")
	emb := &fakeEmbeddingProvider{vec: []float32{0.1}}
	vec := &fakeVectorProvider{}
	m := prepareSemanticTestModule(t, emb, vec)
	m.redisCache = newRedisCache(m.name, fake, m.ruleTable)
	m.semantic.maxQuestionBytes = 3

	req := newTestRequest("default", chatBody("what is bfe", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"

	ret, _ := m.cacheRequestHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected pass when the question is too long, got ret=%d", ret)
	}
	if emb.calls != 0 || vec.queryCalls != 0 {
		t.Error("overlong question must skip the semantic lookup entirely")
	}
	if m.ruleTable.snapshotCounters().semanticSkipped != 1 {
		t.Error("SEMANTIC_SKIPPED counter should be incremented")
	}
	// exact-match cache still works: the request passed through as a miss
	if getAiCacheContext(req) == nil {
		t.Error("context should still be saved for the exact write-back")
	}
}

func TestCacheRequestHandlerSemanticNotEnabled(t *testing.T) {
	// rule file without the Semantic block: enableSemanticCache on rules can
	// never take effect (backward compatibility)
	fake := newFakeRedisClient("")
	emb := &fakeEmbeddingProvider{vec: []float32{0.1}}
	vec := &fakeVectorProvider{}
	m := prepareTestModule(t) // no semantic wiring at all
	m.redisCache = newRedisCache(m.name, fake, m.ruleTable)

	req := newTestRequest("default", chatBody("what is bfe", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"

	ret, _ := m.cacheRequestHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected pass, got ret=%d", ret)
	}
	if emb.calls != 0 || vec.queryCalls != 0 {
		t.Error("semantic lookup must stay off when the module has no semantic cache")
	}
}

func TestSetCacheStatusSemantic(t *testing.T) {
	m := NewModuleAiCache()
	req := newTestRequest("default", "")
	ai := req.InitAiBasicInfo()

	m.setCacheStatus(req, CacheStatusHitSemantic)
	if !ai.AiCacheHit || !ai.AiCacheSemantic || ai.AiCacheStatus != CacheStatusHitSemantic {
		t.Errorf("hit_semantic should set hit + semantic + status, got %+v", ai)
	}

	m.setCacheSimilarity(req, 0.87)
	if ai.AiCacheSimilarity != 0.87 {
		t.Errorf("similarity not recorded: %f", ai.AiCacheSimilarity)
	}
}
