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
	"time"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_modules/mod_ai_cache/provider/embedding"
	"github.com/bfenetworks/bfe/bfe_modules/mod_ai_cache/provider/vector"
)

// semanticCache glues the embedding service and the vector store into the
// semantic lookup / upload operations of mod_ai_cache. All failures are
// fail-open: the caller degrades to a plain exact-match cache miss.
type semanticCache struct {
	embedding        embedding.EmbeddingProvider
	vector           vector.VectorProvider
	maxQuestionBytes int64           // question length limit for semantic lookup
	table            *cacheRuleTable // counters of the module
}

func newSemanticCache(emb embedding.EmbeddingProvider, vec vector.VectorProvider,
	maxQuestionBytes int64, table *cacheRuleTable) *semanticCache {
	if maxQuestionBytes <= 0 {
		maxQuestionBytes = DefaultMaxQuestionBytes
	}
	return &semanticCache{
		embedding:        emb,
		vector:           vec,
		maxQuestionBytes: maxQuestionBytes,
		table:            table,
	}
}

// Lookup runs embedding + vector query + threshold judgment. Any failure
// returns ok=false (the caller degrades to a miss). On success it returns
// the cached answer, the normalized similarity and the computed embedding;
// the embedding is returned in all non-embedding-failure cases so that the
// response phase can reuse it for the write-back without recomputation.
func (s *semanticCache) Lookup(question, tenant string, ttl time.Duration,
	cfg *SemanticConf) (answer string, similarity float64, emb []float32, ok bool) {
	start := time.Now()
	emb, err := s.embedding.Embed(question)
	s.table.addEmbeddingLatencyMs(time.Since(start).Milliseconds())
	if err != nil {
		s.table.incEmbeddingErr()
		log.Logger.Warn("%s: embedding failed, degrade to miss, err[%v]", ModAiCache, err)
		return "", 0, nil, false
	}
	if len(emb) == 0 {
		s.table.incEmbeddingErr()
		log.Logger.Warn("%s: embedding returned empty vector, degrade to miss", ModAiCache)
		return "", 0, nil, false
	}

	start = time.Now()
	results, err := s.vector.Query(emb, tenant, cfg.TopK, ttl)
	s.table.addVectorLatencyMs(time.Since(start).Milliseconds())
	if err != nil {
		s.table.incVectorErr()
		log.Logger.Warn("%s: vector query failed, degrade to miss, err[%v]", ModAiCache, err)
		return "", 0, emb, false
	}
	if len(results) == 0 {
		return "", 0, emb, false
	}

	// only the best neighbor is judged against the threshold
	best := results[0]
	if !compareScore(best.Score, cfg.Threshold, cfg.ThresholdRelation) {
		if openDebug {
			log.Logger.Debug("%s: semantic score %.4f not %s threshold %.4f, miss",
				ModAiCache, best.Score, cfg.ThresholdRelation, cfg.Threshold)
		}
		return "", 0, emb, false
	}
	if best.Answer == "" {
		// defensive: a record passing the threshold without an answer must
		// not produce a hit (dirty data)
		log.Logger.Warn("%s: semantic hit without answer, id[%s], treat as miss", ModAiCache, best.ID)
		return "", 0, emb, false
	}

	return best.Answer, best.Similarity, emb, true
}

// Upload writes one record to the vector store with the deterministic id
// sha256(tenant + "\n" + question), so repeated uploads of the same question
// are idempotent. It only returns the error (fail-open) and is safe to call
// in a goroutine.
func (s *semanticCache) Upload(question, tenant string, emb []float32, answer string) error {
	return s.vector.Upload(vector.Item{
		ID:        vector.ItemID(tenant, question),
		Tenant:    tenant,
		Question:  question,
		Answer:    answer,
		Embedding: emb,
		CreatedAt: time.Now(),
	})
}

// compareScore judges the raw score of the vector store against the
// threshold with the given relation. The relation shields the direction
// difference of store metrics (distance: lt/lte, the smaller the more
// similar; similarity: gt/gte, the bigger the more similar).
func compareScore(score, threshold float64, relation string) bool {
	switch relation {
	case SemanticRelationLt:
		return score < threshold
	case SemanticRelationLte:
		return score <= threshold
	case SemanticRelationGt:
		return score > threshold
	case SemanticRelationGte:
		return score >= threshold
	}
	return false
}

// semanticEnabled reports whether the module-level semantic cache is ready.
// It is used by the request / response handlers to keep the exact-match
// paths untouched when the semantic cache is not configured.
func (m *ModuleAiCache) semanticEnabled() bool {
	return m.semantic != nil
}
