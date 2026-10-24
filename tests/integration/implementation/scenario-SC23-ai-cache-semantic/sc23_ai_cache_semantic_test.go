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

package sc23

import (
	"bytes"
	"fmt"
	"io"
	"math"
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
	apiHost   = "cache.example.org"
	apiPath   = "/v1/chat/completions"
	apiKey    = "ak_cache"
	apiKey2   = "ak_cache2"
	apiKeyId  = "cache_key_id"
	apiKeyId2 = "cache_key_id_2"

	clusterCache = "cluster_cache"

	quotaKeyTotal = "quota:plan_total"

	// semanticThreshold is the global score threshold (cosine distance,
	// thresholdRelation=lt) of the Semantic block of the rule file.
	semanticThreshold = 0.15

	// vectorCollection matches the [vector] collection of the testdata conf.
	vectorCollection = "ai_cache_semantic"
)

var (
	questionA   = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"what is bfe?"}]}`)
	paraphraseA = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"could you introduce bfe to me please?"}]}`)
	questionB   = []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"how to cook rice at home?"}]}`)

	nonStreamAnswer = `{"choices":[{"index":0,"message":{"role":"assistant","content":"BFE is a layer-7 load balancer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":8,"total_tokens":18}}`

	// sseChunks simulates a real provider answering in multiple network
	// frames: role-only first chunk, then two content chunks.
	sseChunks = []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"BFE is"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":" a gateway"}}]}`,
	}

	// streamPrefix / streamSuffix wrap the question text of a streaming body.
	streamPrefix = []byte(`{"model":"deepseek-chat","stream":true,"messages":[{"role":"user","content":"`)
	streamSuffix = []byte(`"}]}`)

	// expectedSimilarity is the normalized similarity (1 - cosine distance)
	// between the scripted vectors of questionA ([1,0,0]) and paraphraseA
	// ([0.99,0.01,0]); the access log ai_cache_similarity (field 792) must
	// match what the mock vector store actually computed.
	expectedSimilarity = cosineSimilarity([]float64{1, 0, 0}, []float64{0.99, 0.01, 0})
)

// testEnv holds all resources for a single SC23 integration test.
type testEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backend    *common.MockBackend
	redis      *common.RedisServer
	embedding  *common.MockEmbeddingService
	chroma     *common.MockChromaService
	confDir    string
	logDir     string
	bfePort    int
	stopBFE    func()
}

func newTestEnv(t *testing.T) *testEnv {
	e := &testEnv{t: t}

	e.backend = common.NewMockBackend(clusterCache, http.StatusOK, nonStreamAnswer)
	e.redis = common.NewRedisServer(t)
	e.embedding = common.NewMockEmbeddingService(t, embeddingScript())
	e.chroma = common.NewMockChromaService(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	return e
}

// embeddingScript scripts the embedding vectors by question keyword (longest
// keyword wins, see common.EmbeddingScript): questionA maps to [1,0,0], its
// paraphrase to a near-parallel vector and the unrelated questionB to an
// orthogonal one. cosine distances against the stored [1,0,0] record are
// ~0.00005 (hit), 1.0 (miss) respectively.
func embeddingScript() common.EmbeddingScript {
	return common.EmbeddingScript{
		"introduce bfe": {0.99, 0.01, 0},
		"what is bfe":   {1, 0, 0},
		"cook rice":     {0, 1, 0},
	}
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

// semanticCacheRule builds an AiCacheRuleData with the top-level Semantic
// block (topK=1, cosine-distance threshold 0.15, relation lt) and a single
// default rule with the semantic cache enabled.
func semanticCacheRule() *common.AiCacheRuleData {
	return &common.AiCacheRuleData{
		Version: "1.0",
		Semantic: &common.AiCacheSemanticRule{
			TopK:              intPtr(1),
			Threshold:         float64Ptr(semanticThreshold),
			ThresholdRelation: strPtr("lt"),
		},
		Config: map[string][]common.AiCacheRule{
			"ai_product": {
				{
					Cond:                "default_t()",
					CacheKeyStrategy:    "lastQuestion",
					CacheTTL:            3600,
					MaxBodyBytes:        1048576,
					MaxValueBytes:       1048576,
					EnableSemanticCache: boolPtr(true),
				},
			},
		},
	}
}

func (e *testEnv) startBFE(cacheRuleData *common.AiCacheRuleData, tokenRule *common.TokenRuleData) {
	backends := map[string]*common.MockBackend{
		clusterCache: e.backend,
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:          "testdata",
		TargetConfDir:        e.confDir,
		Backends:             backends,
		RedisAddr:            e.redis.Addr(),
		TokenRuleData:        tokenRule,
		AiCacheRuleData:      cacheRuleData,
		EmbeddingServiceAddr: e.embedding.Addr(),
		VectorServiceAddr:    e.chroma.Addr(),
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
	if e.embedding != nil {
		e.embedding.Close()
	}
	if e.chroma != nil {
		e.chroma.Close()
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
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	// every request uses its own connection: a pooled idle connection may
	// have been closed by BFE in the meantime and POSTs are not retried
	req.Close = true
	req.Header.Set("Authorization", "Bearer "+apikey)
	req.Header.Set("Content-Type", "application/json")

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
// the deadline expires.
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

// waitChromaUpserts polls the vector store until at least n upsert calls were
// recorded. The semantic write-back is asynchronous (a goroutine at response
// close), so a short poll avoids the startup race.
func (e *testEnv) waitChromaUpserts(n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if e.chroma.UpsertHits() >= n {
			return true
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

func boolPtr(v bool) *bool          { return &v }
func intPtr(v int) *int             { return &v }
func float64Ptr(v float64) *float64 { return &v }
func strPtr(v string) *string       { return &v }

// cosineSimilarity computes the cosine similarity of two vectors with the
// same formula as the mock vector store.
func cosineSimilarity(a, b []float64) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// tenantOfWhere extracts the tenant_id $eq value of a Chroma where
// expression, in either the plain {tenant_id:{$eq:...}} form or the $and
// conjunction emitted by the mod_ai_cache provider.
func tenantOfWhere(where map[string]interface{}) (string, bool) {
	if where == nil {
		return "", false
	}
	if and, ok := where["$and"].([]interface{}); ok {
		for _, item := range and {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if t, ok := tenantOfWhere(m); ok {
				return t, true
			}
		}
		return "", false
	}
	cond, ok := where["tenant_id"].(map[string]interface{})
	if !ok {
		return "", false
	}
	t, ok := cond["$eq"].(string)
	return t, ok
}

// TestTC01 verifies that an exact redis hit never pays for an embedding
// call: only the initial miss runs the semantic lookup, the identical
// follow-up requests are served from the exact-match cache with zero
// additional embedding/vector traffic.
func TestTC01_ExactHitSkipsEmbedding(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(semanticCacheRule(), unlimitedTokenRule())

	// first request: exact miss, semantic lookup miss (embedding #1), cached
	resp, body, err := e.sendRequest(apiKey, questionA)
	e.expectOK(resp, body, err, "first request")
	if !strings.Contains(body, "BFE is a layer-7 load balancer") {
		t.Fatalf("first response should contain upstream answer, got: %s", body)
	}
	if e.backend.Hits() != 1 {
		t.Fatalf("expected 1 upstream call, got %d", e.backend.Hits())
	}
	if e.embedding.Hits() != 1 {
		t.Fatalf("miss should call embedding once, got %d", e.embedding.Hits())
	}
	if !e.waitForCacheKey("ai_cache:cache_key_id:", 5*time.Second) {
		e.logBFEException()
		t.Fatalf("cache key should exist in redis, keys: %v", e.redis.Keys())
	}
	if !e.waitChromaUpserts(1, 5*time.Second) {
		e.logBFEException()
		t.Fatalf("vector record should be uploaded, upserts: %d", e.chroma.UpsertHits())
	}

	// second and third identical requests: exact hit, no embedding
	for i := 2; i <= 3; i++ {
		resp, body, err = e.sendRequest(apiKey, questionA)
		e.expectOK(resp, body, err, fmt.Sprintf("request %d", i))
		if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache != "hit" {
			t.Fatalf("request %d: expected X-Bfe-Ai-Cache hit, got %q", i, cache)
		}
		if e.backend.Hits() != 1 {
			t.Fatalf("request %d: cache hit should not call upstream, got %d hits", i, e.backend.Hits())
		}
		if e.embedding.Hits() != 1 {
			t.Fatalf("request %d: exact hit must not call embedding, got %d calls", i, e.embedding.Hits())
		}
	}
	if e.chroma.QueryHits() != 1 {
		t.Fatalf("exact hits must not query the vector store, got %d queries", e.chroma.QueryHits())
	}
}

// TestTC02 verifies the semantic hit: a paraphrased question whose embedding
// is near-parallel to the stored one is served from the vector cache with
// X-Bfe-Ai-Cache: hit_semantic, and the access log carries
// ai_cache_status=hit_semantic plus the ai_cache_semantic/ai_cache_similarity
// fields (791/792) matching the similarity the mock vector store computed.
func TestTC02_SemanticHitParaphrase(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(semanticCacheRule(), unlimitedTokenRule())

	// step 1: original question misses and seeds the vector store
	resp, body, err := e.sendRequest(apiKey, questionA)
	e.expectOK(resp, body, err, "step 1")
	if e.backend.Hits() != 1 || e.embedding.Hits() != 1 {
		t.Fatalf("step 1: expected backend=1 embedding=1, got backend=%d embedding=%d",
			e.backend.Hits(), e.embedding.Hits())
	}
	if !e.waitChromaUpserts(1, 5*time.Second) {
		e.logBFEException()
		t.Fatalf("step 1: vector record should be uploaded, upserts: %d", e.chroma.UpsertHits())
	}

	// step 2: paraphrase hits semantically without calling the upstream
	resp, body, err = e.sendRequest(apiKey, paraphraseA)
	e.expectOK(resp, body, err, "step 2")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache != "hit_semantic" {
		t.Fatalf("step 2: expected X-Bfe-Ai-Cache hit_semantic, got %q", cache)
	}
	if !strings.Contains(body, "BFE is a layer-7 load balancer") {
		t.Fatalf("step 2: response should contain the cached answer, got: %s", body)
	}
	if e.backend.Hits() != 1 {
		t.Fatalf("step 2: semantic hit should not call upstream, got %d hits", e.backend.Hits())
	}
	if e.embedding.Hits() != 2 || e.chroma.QueryHits() != 2 {
		t.Fatalf("step 2: expected embedding=2 query=2, got embedding=%d query=%d",
			e.embedding.Hits(), e.chroma.QueryHits())
	}

	// access log assertions: stop BFE so the b2log buffer is flushed
	time.Sleep(500 * time.Millisecond)
	e.stopBFE()
	e.stopBFE = nil

	reqLogs := e.accessLogs()
	if len(reqLogs) != 2 {
		e.logBFEException()
		t.Fatalf("expected 2 access logs, got %d", len(reqLogs))
	}

	l := reqLogs[0]
	if l.AiCacheStatus == nil || *l.AiCacheStatus != "miss" {
		t.Errorf("step 1: ai_cache_status should be miss, got: %v", l.AiCacheStatus)
	}
	if l.AiCacheSemantic != nil || l.AiCacheSimilarity != nil {
		t.Errorf("step 1: ai_cache_semantic/ai_cache_similarity should be unset, got: %v %v",
			l.AiCacheSemantic, l.AiCacheSimilarity)
	}

	l = reqLogs[1]
	if l.AiCacheStatus == nil || *l.AiCacheStatus != "hit_semantic" {
		t.Errorf("step 2: ai_cache_status should be hit_semantic, got: %v", l.AiCacheStatus)
	}
	if l.AiCacheSemantic == nil || !*l.AiCacheSemantic {
		t.Errorf("step 2: ai_cache_semantic should be true, got: %v", l.AiCacheSemantic)
	}
	if l.AiCacheSimilarity == nil {
		t.Fatalf("step 2: ai_cache_similarity should be set")
	}
	if math.Abs(*l.AiCacheSimilarity-expectedSimilarity) > 1e-9 {
		t.Errorf("step 2: ai_cache_similarity should match the mock-computed %.8f, got: %.8f",
			expectedSimilarity, *l.AiCacheSimilarity)
	}
}

// TestTC03 verifies the threshold: a question whose embedding is orthogonal
// to the stored record (cosine distance 1.0, way above the 0.15 threshold)
// is a miss and reaches the upstream.
func TestTC03_BelowThresholdMisses(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(semanticCacheRule(), unlimitedTokenRule())

	e.expectOK(e.sendRequest(apiKey, questionA))
	if !e.waitChromaUpserts(1, 5*time.Second) {
		t.Fatalf("vector record should be uploaded, upserts: %d", e.chroma.UpsertHits())
	}

	resp, body, err := e.sendRequest(apiKey, questionB)
	e.expectOK(resp, body, err, "unrelated question")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache != "" {
		t.Fatalf("below-threshold question must not be served from cache, got %q", cache)
	}
	if e.backend.Hits() != 2 {
		t.Fatalf("below-threshold question should reach upstream, got %d hits", e.backend.Hits())
	}
	if e.embedding.Hits() != 2 || e.chroma.QueryHits() != 2 {
		t.Fatalf("expected embedding=2 query=2, got embedding=%d query=%d",
			e.embedding.Hits(), e.chroma.QueryHits())
	}
}

// TestTC04 verifies tenant isolation of the semantic cache: the second API
// key asking the same question never reads the first tenant's vector record;
// the recorded Chroma where expressions prove the query was scoped to the
// requester's own tenant.
func TestTC04_TenantIsolationSemantic(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(semanticCacheRule(), unlimitedTokenRule())

	// key 1 asks the question: miss, record uploaded under key 1's tenant
	e.expectOK(e.sendRequest(apiKey, questionA))
	if !e.waitChromaUpserts(1, 5*time.Second) {
		t.Fatalf("key 1 vector record should be uploaded, upserts: %d", e.chroma.UpsertHits())
	}

	// key 2 asks the same question text: exact redis miss and semantic miss
	resp, body, err := e.sendRequest(apiKey2, questionA)
	e.expectOK(resp, body, err, "key 2 request")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache != "" {
		t.Fatalf("key 2 must not read key 1's semantic record, got %q", cache)
	}
	if e.backend.Hits() != 2 {
		t.Fatalf("key 2 should reach upstream, got %d hits", e.backend.Hits())
	}
	if e.embedding.Hits() != 2 || e.chroma.QueryHits() != 2 {
		t.Fatalf("expected embedding=2 query=2, got embedding=%d query=%d",
			e.embedding.Hits(), e.chroma.QueryHits())
	}

	// the semantic lookup of key 2 must be scoped to key 2's tenant
	filters := e.chroma.QueryFilters()
	if len(filters) != 2 {
		t.Fatalf("expected 2 recorded query filters, got %d", len(filters))
	}
	tenant, ok := tenantOfWhere(filters[1])
	if !ok {
		t.Fatalf("key 2 query filter has no tenant_id $eq condition: %v", filters[1])
	}
	if tenant != apiKeyId2 {
		t.Fatalf("key 2 query filter should scope tenant_id to %s, got %s", apiKeyId2, tenant)
	}

	// key 2 asking again now hits its own freshly uploaded record
	resp, _, err = e.sendRequest(apiKey2, questionA)
	e.expectOK(resp, body, err, "key 2 repeat")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache != "hit" {
		t.Fatalf("key 2 repeat should be an exact hit, got %q", cache)
	}
	if e.backend.Hits() != 2 {
		t.Fatalf("key 2 repeat should not call upstream, got %d hits", e.backend.Hits())
	}
}

// TestTC05 verifies fail-open degradation: with the embedding service
// failing (HTTP 500), the request degrades to the pure exact-match flow —
// the main request succeeds, the exact cache still works and nothing is
// written to the vector store.
func TestTC05_EmbeddingFailureDegrades(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.embedding.SetFailAll(true)
	e.startBFE(semanticCacheRule(), unlimitedTokenRule())

	// miss with embedding down: degrade to plain miss, request succeeds
	resp, body, err := e.sendRequest(apiKey, questionA)
	e.expectOK(resp, body, err, "request with embedding down")
	if !strings.Contains(body, "BFE is a layer-7 load balancer") {
		t.Fatalf("response should contain upstream answer, got: %s", body)
	}
	if e.backend.Hits() != 1 || e.embedding.Hits() != 1 {
		t.Fatalf("expected backend=1 embedding=1, got backend=%d embedding=%d",
			e.backend.Hits(), e.embedding.Hits())
	}
	if e.chroma.QueryHits() != 0 || e.chroma.UpsertHits() != 0 {
		t.Fatalf("vector store must not be touched when embedding fails, got query=%d upsert=%d",
			e.chroma.QueryHits(), e.chroma.UpsertHits())
	}
	if !e.waitForCacheKey("ai_cache:cache_key_id:", 5*time.Second) {
		t.Fatalf("exact cache should still be written, keys: %v", e.redis.Keys())
	}

	// exact hit still works while the embedding service is down
	resp, _, err = e.sendRequest(apiKey, questionA)
	e.expectOK(resp, body, err, "exact hit with embedding down")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache != "hit" {
		t.Fatalf("exact hit should work with embedding down, got %q", cache)
	}
	if e.backend.Hits() != 1 || e.embedding.Hits() != 1 {
		t.Fatalf("exact hit must not call upstream/embedding, got backend=%d embedding=%d",
			e.backend.Hits(), e.embedding.Hits())
	}
}

// TestTC06 verifies streaming with the semantic cache: the upstream SSE
// answer is accumulated and uploaded, and the paraphrased streaming request
// is served a complete SSE sequence (including data:[DONE]) from the vector
// cache.
func TestTC06_StreamSemanticHit(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.backend.SSEEvents = sseChunks
	e.startBFE(semanticCacheRule(), unlimitedTokenRule())

	streamA := append(append(append([]byte(nil), streamPrefix...), "what is bfe?"...), streamSuffix...)
	streamParaphrase := append(append(append([]byte(nil), streamPrefix...),
		"could you introduce bfe to me please?"...), streamSuffix...)

	// first request: miss, upstream SSE answer cached and uploaded
	resp, body, err := e.sendRequest(apiKey, streamA)
	e.expectOK(resp, body, err, "first stream request")
	if !strings.Contains(body, "BFE is") || !strings.Contains(body, " a gateway") {
		t.Fatalf("first response should pass through the SSE chunks, got: %s", body)
	}
	if e.backend.Hits() != 1 {
		t.Fatalf("expected 1 upstream call, got %d", e.backend.Hits())
	}
	if !e.waitForCacheKey("ai_cache:cache_key_id:", 5*time.Second) {
		t.Fatalf("stream answer should be cached, redis keys: %v", e.redis.Keys())
	}
	if !e.waitChromaUpserts(1, 5*time.Second) {
		t.Fatalf("stream answer should be uploaded to the vector store, upserts: %d", e.chroma.UpsertHits())
	}

	// paraphrased stream request: semantic hit with a single complete SSE seq
	resp, body, err = e.sendRequest(apiKey, streamParaphrase)
	e.expectOK(resp, body, err, "paraphrase stream request")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache != "hit_semantic" {
		t.Fatalf("expected X-Bfe-Ai-Cache hit_semantic, got %q", cache)
	}
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
		t.Fatalf("stream semantic hit should not call upstream, got %d hits", e.backend.Hits())
	}
	if e.embedding.Hits() != 2 || e.chroma.QueryHits() != 2 {
		t.Fatalf("expected embedding=2 query=2, got embedding=%d query=%d",
			e.embedding.Hits(), e.chroma.QueryHits())
	}
}

// TestTC07 verifies the semantic TTL: a pre-existing vector record whose
// created_at is older than the rule TTL is filtered out by the where
// conjunction of the query, so the same question misses and goes upstream.
func TestTC07_SemanticTTLExpiry(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	// seed an expired record: same tenant/question/vector as questionA but
	// created 2h ago, beyond the 3600s rule TTL
	e.chroma.UpsertRecord(vectorCollection, common.ChromaRecord{
		ID:        "expired-seed",
		Tenant:    apiKeyId,
		Question:  "what is bfe?",
		Answer:    "STALE ANSWER MUST NOT BE SERVED",
		Embedding: []float64{1, 0, 0},
		CreatedAt: time.Now().Add(-2 * time.Hour),
	})

	e.startBFE(semanticCacheRule(), unlimitedTokenRule())

	resp, body, err := e.sendRequest(apiKey, questionA)
	e.expectOK(resp, body, err, "request with expired vector record")
	if strings.Contains(body, "STALE ANSWER MUST NOT BE SERVED") {
		t.Fatalf("expired vector record must not be served, got: %s", body)
	}
	if !strings.Contains(body, "BFE is a layer-7 load balancer") {
		t.Fatalf("response should contain the fresh upstream answer, got: %s", body)
	}
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache != "" {
		t.Fatalf("expired record must not produce a cache hit, got %q", cache)
	}
	if e.backend.Hits() != 1 {
		t.Fatalf("expired record should be re-fetched from upstream, got %d hits", e.backend.Hits())
	}
	if e.embedding.Hits() != 1 || e.chroma.QueryHits() != 1 {
		t.Fatalf("expected embedding=1 query=1, got embedding=%d query=%d",
			e.embedding.Hits(), e.chroma.QueryHits())
	}
}

// TestTC08 verifies billing coordination: a semantic hit skips the token
// quota deduction exactly like an exact hit, while misses deduct the actual
// usage.
func TestTC08_SemanticHitSkipsQuotaDeduction(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.startBFE(semanticCacheRule(), quotaTokenRule())
	e.redis.SetQuota(quotaKeyTotal, 1000)

	// miss: deducts prompt(10) + completion(8) = 18
	e.expectOK(e.sendRequest(apiKey, questionA))
	if !e.waitQuota(quotaKeyTotal, 982, 5*time.Second) {
		t.Fatalf("miss should deduct 18 tokens, quota: %d", e.redis.GetQuota(quotaKeyTotal))
	}
	if !e.waitChromaUpserts(1, 5*time.Second) {
		t.Fatalf("vector record should be uploaded, upserts: %d", e.chroma.UpsertHits())
	}

	// semantic hit: no deduction
	resp, _, err := e.sendRequest(apiKey, paraphraseA)
	e.expectOK(resp, "", err, "semantic hit request")
	if cache := resp.Header.Get("X-Bfe-Ai-Cache"); cache != "hit_semantic" {
		t.Fatalf("expected semantic hit, got %q", cache)
	}
	time.Sleep(500 * time.Millisecond)
	if got := e.redis.GetQuota(quotaKeyTotal); got != 982 {
		t.Fatalf("semantic hit must not deduct tokens, quota: %d", got)
	}

	// another miss (different question): deducts again
	e.expectOK(e.sendRequest(apiKey, questionB))
	if !e.waitQuota(quotaKeyTotal, 964, 5*time.Second) {
		t.Fatalf("second miss should deduct 18 tokens, quota: %d", e.redis.GetQuota(quotaKeyTotal))
	}
}
