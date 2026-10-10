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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_modules/mod_ai_cache/provider/vector"
)

// fakeEmbeddingProvider is an embedding.EmbeddingProvider for unit tests.
type fakeEmbeddingProvider struct {
	calls int
	err   error
	vec   []float32
}

func (f *fakeEmbeddingProvider) Embed(text string) ([]float32, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.vec, nil
}

// fakeVectorProvider is a vector.VectorProvider for unit tests. Upload is
// called from an asynchronous write-back goroutine (see ModuleAiCache.writeBack),
// so all recorded state is mutex-guarded and must be read via the accessors.
type fakeVectorProvider struct {
	mu sync.Mutex

	queryCalls  int
	uploadCalls int
	err         error
	results     []vector.Result

	lastQueryTenant string
	lastQueryTopK   int
	lastQueryTTL    time.Duration
	lastUpload      vector.Item
}

func (f *fakeVectorProvider) Query(embedding []float32, tenant string, topK int,
	ttl time.Duration) ([]vector.Result, error) {
	f.mu.Lock()
	f.queryCalls++
	f.lastQueryTenant = tenant
	f.lastQueryTopK = topK
	f.lastQueryTTL = ttl
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func (f *fakeVectorProvider) Upload(item vector.Item) error {
	f.mu.Lock()
	f.uploadCalls++
	f.lastUpload = item
	f.mu.Unlock()
	return f.err
}

func (f *fakeVectorProvider) queries() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queryCalls
}

func (f *fakeVectorProvider) uploads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.uploadCalls
}

func (f *fakeVectorProvider) lastQuery() (string, int, time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastQueryTenant, f.lastQueryTopK, f.lastQueryTTL
}

func (f *fakeVectorProvider) lastUploaded() vector.Item {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastUpload
}

func TestCompareScore(t *testing.T) {
	cases := []struct {
		relation  string
		score     float64
		threshold float64
		want      bool
	}{
		// lt
		{SemanticRelationLt, 0.14, 0.15, true},
		{SemanticRelationLt, 0.15, 0.15, false},
		{SemanticRelationLt, 0.16, 0.15, false},
		// lte
		{SemanticRelationLte, 0.15, 0.15, true},
		{SemanticRelationLte, 0.16, 0.15, false},
		// gt
		{SemanticRelationGt, 0.86, 0.85, true},
		{SemanticRelationGt, 0.85, 0.85, false},
		{SemanticRelationGt, 0.84, 0.85, false},
		// gte
		{SemanticRelationGte, 0.85, 0.85, true},
		{SemanticRelationGte, 0.84, 0.85, false},
	}
	for _, c := range cases {
		if got := compareScore(c.score, c.threshold, c.relation); got != c.want {
			t.Errorf("compareScore(%f, %f, %s) = %v, want %v",
				c.score, c.threshold, c.relation, got, c.want)
		}
	}

	// unknown relation never passes (defensive, defaults are applied at load)
	if compareScore(0.1, 0.15, "bogus") {
		t.Error("unknown relation should not pass")
	}
}

func newTestSemanticCache(emb *fakeEmbeddingProvider, vec *fakeVectorProvider) *semanticCache {
	return newSemanticCache(emb, vec, DefaultMaxQuestionBytes, newCacheRuleTable())
}

func TestSemanticLookupHit(t *testing.T) {
	emb := &fakeEmbeddingProvider{vec: []float32{0.1, 0.2}}
	vec := &fakeVectorProvider{results: []vector.Result{
		{ID: "id1", Question: "q", Answer: "cached answer", Score: 0.1, Similarity: 0.9},
	}}
	s := newTestSemanticCache(emb, vec)

	cfg := &SemanticConf{TopK: 3, Threshold: 0.15, ThresholdRelation: SemanticRelationLt}
	answer, similarity, emb0, ok := s.Lookup("question", "key_001", time.Minute, cfg)
	if !ok {
		t.Fatal("expected semantic hit")
	}
	if answer != "cached answer" {
		t.Errorf("unexpected answer: %q", answer)
	}
	if similarity != 0.9 {
		t.Errorf("unexpected similarity: %f", similarity)
	}
	if len(emb0) != 2 {
		t.Errorf("embedding should be returned for the write-back reuse, got %v", emb0)
	}
	tenant, topK, ttl := vec.lastQuery()
	if tenant != "key_001" || topK != 3 || ttl != time.Minute {
		t.Errorf("unexpected query args: tenant=%s topK=%d ttl=%v", tenant, topK, ttl)
	}
}

func TestSemanticLookupEmbeddingFailure(t *testing.T) {
	emb := &fakeEmbeddingProvider{err: errors.New("embedding down")}
	vec := &fakeVectorProvider{}
	s := newTestSemanticCache(emb, vec)

	cfg := &SemanticConf{TopK: 1, Threshold: 0.15, ThresholdRelation: SemanticRelationLt}
	_, _, emb0, ok := s.Lookup("question", "key_001", time.Minute, cfg)
	if ok {
		t.Error("embedding failure must degrade to miss")
	}
	if emb0 != nil {
		t.Errorf("embedding must be nil on failure, got %v", emb0)
	}
	if vec.queries() != 0 {
		t.Error("vector query must not be called when embedding fails")
	}
	if s.table.snapshotCounters().embeddingErr != 1 {
		t.Error("EMBEDDING_ERR counter should be incremented")
	}
}

func TestSemanticLookupVectorFailureKeepsEmbedding(t *testing.T) {
	emb := &fakeEmbeddingProvider{vec: []float32{0.1}}
	vec := &fakeVectorProvider{err: errors.New("vector down")}
	s := newTestSemanticCache(emb, vec)

	cfg := &SemanticConf{TopK: 1, Threshold: 0.15, ThresholdRelation: SemanticRelationLt}
	_, _, emb0, ok := s.Lookup("question", "key_001", time.Minute, cfg)
	if ok {
		t.Error("vector failure must degrade to miss")
	}
	if len(emb0) != 1 {
		t.Error("the computed embedding must be kept for the write-back reuse")
	}
	if s.table.snapshotCounters().vectorErr != 1 {
		t.Error("VECTOR_ERR counter should be incremented")
	}
}

func TestSemanticLookupThresholdNotPassed(t *testing.T) {
	emb := &fakeEmbeddingProvider{vec: []float32{0.1}}
	vec := &fakeVectorProvider{results: []vector.Result{
		{ID: "id1", Answer: "cached answer", Score: 0.2, Similarity: 0.8},
	}}
	s := newTestSemanticCache(emb, vec)

	cfg := &SemanticConf{TopK: 1, Threshold: 0.15, ThresholdRelation: SemanticRelationLt}
	_, _, emb0, ok := s.Lookup("question", "key_001", time.Minute, cfg)
	if ok {
		t.Error("score beyond the threshold must be a miss")
	}
	if len(emb0) != 1 {
		t.Error("the computed embedding must be kept even when the threshold is not passed")
	}
}

func TestSemanticLookupEmptyResults(t *testing.T) {
	emb := &fakeEmbeddingProvider{vec: []float32{0.1}}
	vec := &fakeVectorProvider{}
	s := newTestSemanticCache(emb, vec)

	cfg := &SemanticConf{TopK: 1, Threshold: 0.15, ThresholdRelation: SemanticRelationLt}
	_, _, emb0, ok := s.Lookup("question", "key_001", time.Minute, cfg)
	if ok {
		t.Error("empty results must be a miss")
	}
	if len(emb0) != 1 {
		t.Error("the computed embedding must be kept even with empty results")
	}
}

func TestSemanticLookupEmptyAnswerGuard(t *testing.T) {
	emb := &fakeEmbeddingProvider{vec: []float32{0.1}}
	vec := &fakeVectorProvider{results: []vector.Result{
		{ID: "id1", Answer: "", Score: 0.1, Similarity: 0.9},
	}}
	s := newTestSemanticCache(emb, vec)

	cfg := &SemanticConf{TopK: 1, Threshold: 0.15, ThresholdRelation: SemanticRelationLt}
	if _, _, _, ok := s.Lookup("question", "key_001", time.Minute, cfg); ok {
		t.Error("a threshold-passing record without an answer must be treated as a miss")
	}
}

func TestSemanticUpload(t *testing.T) {
	emb := &fakeEmbeddingProvider{vec: []float32{0.1}}
	vec := &fakeVectorProvider{}
	s := newTestSemanticCache(emb, vec)

	if err := s.Upload("what is bfe", "key_001", []float32{0.1, 0.2}, "the answer"); err != nil {
		t.Fatalf("Upload error: %v", err)
	}
	if vec.uploads() != 1 {
		t.Fatalf("expected 1 upload, got %d", vec.uploads())
	}
	item := vec.lastUploaded()
	if item.ID != vector.ItemID("key_001", "what is bfe") {
		t.Errorf("upload id must be deterministic, got %s", item.ID)
	}
	if item.Tenant != "key_001" || item.Question != "what is bfe" || item.Answer != "the answer" {
		t.Errorf("unexpected upload item: %+v", item)
	}
	if item.CreatedAt.IsZero() {
		t.Error("CreatedAt must be set")
	}

	// upload error is returned to the caller (fail-open is the caller's job)
	vec.err = errors.New("vector down")
	if err := s.Upload("q", "key_001", []float32{0.1}, "a"); err == nil {
		t.Error("upload failure should be reported")
	}
}

func TestNewSemanticCacheDefaults(t *testing.T) {
	emb := &fakeEmbeddingProvider{}
	vec := &fakeVectorProvider{}
	s := newSemanticCache(emb, vec, 0, newCacheRuleTable())
	if s.maxQuestionBytes != DefaultMaxQuestionBytes {
		t.Errorf("maxQuestionBytes default should be %d, got %d",
			DefaultMaxQuestionBytes, s.maxQuestionBytes)
	}
}
