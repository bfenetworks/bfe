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
	"os"
	"testing"
)

func TestProductRuleConfLoad(t *testing.T) {
	conf, err := ProductRuleConfLoad("testdata/mod_ai_cache/mod_ai_cache_rule.data", DefaultCacheTTL)
	if err != nil {
		t.Fatalf("ProductRuleConfLoad() error: %v", err)
	}

	if conf.Version == nil || *conf.Version != "1.0" {
		t.Errorf("expected version 1.0, got %v", conf.Version)
	}

	if conf.Config == nil {
		t.Fatal("Config should not be nil")
	}

	rules, ok := (*conf.Config)["default"]
	if !ok || rules == nil || len(*rules) != 1 {
		t.Fatalf("default product should have 1 rule")
	}

	rule := (*rules)[0]
	if rule.CacheKeyStrategy != CacheKeyStrategyLastQuestion {
		t.Errorf("expected strategy lastQuestion, got %s", rule.CacheKeyStrategy)
	}
	if rule.CacheTTL != 60 {
		t.Errorf("expected cacheTTL 60, got %d", rule.CacheTTL)
	}
	if rule.CacheValueFrom != DefaultCacheValuePath {
		t.Errorf("expected default cacheValueFrom %s, got %s", DefaultCacheValuePath, rule.CacheValueFrom)
	}
	if rule.CacheStreamFrom != DefaultCacheStreamPath {
		t.Errorf("expected default cacheStreamValueFrom %s, got %s", DefaultCacheStreamPath, rule.CacheStreamFrom)
	}
	if rule.MaxBodyBytes != 1024 || rule.MaxValueBytes != 1024 {
		t.Errorf("expected size limits 1024/1024, got %d/%d", rule.MaxBodyBytes, rule.MaxValueBytes)
	}
}

func TestProductRuleConfLoadDefaults(t *testing.T) {
	conf, err := ProductRuleConfLoad("testdata/mod_ai_cache/mod_ai_cache_rule.data", DefaultCacheTTL)
	if err != nil {
		t.Fatalf("ProductRuleConfLoad() error: %v", err)
	}

	rules := (*conf.Config)["disabled_product"]
	rule := (*rules)[0]
	if !rule.Disabled {
		t.Error("disabled strategy rule should be marked Disabled")
	}
}

func TestConfLoad(t *testing.T) {
	cfg, err := ConfLoad("testdata/mod_ai_cache/mod_ai_cache.conf", "./testdata")
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}

	if cfg.Basic.CacheKeyPrefix != "ai_cache" {
		t.Errorf("expected cache key prefix ai_cache, got %s", cfg.Basic.CacheKeyPrefix)
	}
	if cfg.Basic.DefaultCacheTTL != 3600 {
		t.Errorf("expected default cache TTL 3600, got %d", cfg.Basic.DefaultCacheTTL)
	}
}

func TestConfLoadDefaultFilling(t *testing.T) {
	cfg, err := ConfLoad("testdata/mod_ai_cache/mod_ai_cache_defaults.conf", "./testdata")
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}

	if cfg.Basic.CacheKeyPrefix != DefaultCacheKeyPrefix {
		t.Errorf("expected default prefix %s, got %s", DefaultCacheKeyPrefix, cfg.Basic.CacheKeyPrefix)
	}
	if cfg.Basic.DefaultCacheTTL != DefaultCacheTTL {
		t.Errorf("expected default TTL %d, got %d", DefaultCacheTTL, cfg.Basic.DefaultCacheTTL)
	}
}

func TestConfLoadSemantic(t *testing.T) {
	cfg, err := ConfLoad("testdata/mod_ai_cache/mod_ai_cache_semantic.conf", "./testdata")
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}

	conn, err := cfg.SemanticConnect()
	if err != nil {
		t.Fatalf("SemanticConnect() error: %v", err)
	}
	if conn == nil {
		t.Fatal("semantic conf should be parsed")
	}
	if conn.Embedding.ServiceHost != "127.0.0.1" || conn.Embedding.ServicePort != 11434 {
		t.Errorf("unexpected embedding conf: %+v", conn.Embedding)
	}
	if conn.Embedding.Model != "nomic-embed-text" || conn.Embedding.TimeoutMs != 500 {
		t.Errorf("unexpected embedding model/timeout: %+v", conn.Embedding)
	}
	if conn.Vector.Type != "chroma" || conn.Vector.Collection != "ai_cache_semantic" {
		t.Errorf("unexpected vector conf: %+v", conn.Vector)
	}
	if conn.Vector.MaxQuestionBytes != 4096 {
		t.Errorf("unexpected maxQuestionBytes: %d", conn.Vector.MaxQuestionBytes)
	}
}

func TestSemanticConnectNotConfigured(t *testing.T) {
	// old-style conf without [embedding]/[vector]: pure exact-match mode
	cfg, err := ConfLoad("testdata/mod_ai_cache/mod_ai_cache.conf", "./testdata")
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}
	conn, err := cfg.SemanticConnect()
	if err != nil {
		t.Fatalf("SemanticConnect() error: %v", err)
	}
	if conn != nil {
		t.Errorf("semantic conf should be nil when not configured, got %+v", conn)
	}
}

func TestSemanticConnectValidation(t *testing.T) {
	base, err := ConfLoad("testdata/mod_ai_cache/mod_ai_cache_semantic.conf", "./testdata")
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}

	// only [embedding] configured
	cfg := *base
	cfg.Vector.Type = ""
	cfg.Vector.ServiceHost = ""
	cfg.Vector.ServicePort = 0
	cfg.Vector.Collection = ""
	cfg.Vector.TimeoutMs = 0
	cfg.Vector.MaxQuestionBytes = 0
	if _, err := cfg.SemanticConnect(); err == nil {
		t.Error("half-configured semantic conf (embedding only) should be rejected")
	}

	// only [vector] configured
	cfg = *base
	cfg.Embedding.ServiceHost = ""
	cfg.Embedding.ServicePort = 0
	cfg.Embedding.Model = ""
	cfg.Embedding.TimeoutMs = 0
	if _, err := cfg.SemanticConnect(); err == nil {
		t.Error("half-configured semantic conf (vector only) should be rejected")
	}

	// invalid embedding port
	cfg = *base
	cfg.Embedding.ServicePort = 70000
	if _, err := cfg.SemanticConnect(); err == nil {
		t.Error("invalid embedding port should be rejected")
	}

	// missing embedding model
	cfg = *base
	cfg.Embedding.Model = ""
	if _, err := cfg.SemanticConnect(); err == nil {
		t.Error("missing embedding model should be rejected")
	}

	// unsupported vector type
	cfg = *base
	cfg.Vector.Type = "milvus"
	if _, err := cfg.SemanticConnect(); err == nil {
		t.Error("unsupported vector type should be rejected")
	}

	// empty vector type defaults to chroma
	cfg = *base
	cfg.Vector.Type = ""
	conn, err := cfg.SemanticConnect()
	if err != nil {
		t.Fatalf("empty vector type should default to chroma: %v", err)
	}
	if conn.Vector.Type != VectorTypeChroma {
		t.Errorf("expected default vector type chroma, got %s", conn.Vector.Type)
	}

	// defaults filling
	cfg = *base
	cfg.Embedding.TimeoutMs = 0
	cfg.Vector.Collection = ""
	cfg.Vector.TimeoutMs = 0
	cfg.Vector.MaxQuestionBytes = 0
	conn, err = cfg.SemanticConnect()
	if err != nil {
		t.Fatalf("SemanticConnect() error: %v", err)
	}
	if conn.Embedding.TimeoutMs != DefaultEmbeddingTimeoutMs ||
		conn.Vector.TimeoutMs != DefaultVectorTimeoutMs ||
		conn.Vector.Collection != DefaultVectorCollection ||
		conn.Vector.MaxQuestionBytes != DefaultMaxQuestionBytes {
		t.Errorf("defaults not filled: %+v", conn)
	}
}

func TestProductRuleConfLoadSemantic(t *testing.T) {
	conf, err := ProductRuleConfLoad("testdata/mod_ai_cache/mod_ai_cache_rule_semantic.data", DefaultCacheTTL)
	if err != nil {
		t.Fatalf("ProductRuleConfLoad() error: %v", err)
	}

	if conf.Semantic == nil {
		t.Fatal("Semantic block should be parsed")
	}
	if conf.Semantic.TopK != 3 || conf.Semantic.Threshold != 0.2 || conf.Semantic.ThresholdRelation != "lt" {
		t.Errorf("unexpected semantic conf: %+v", conf.Semantic)
	}

	rules := (*conf.Config)["default"]
	if (*rules)[0].EnableSemanticCache != true {
		t.Error("enableSemanticCache should be true on the default rule")
	}

	// disabled strategy ignores enableSemanticCache
	disabled := (*conf.Config)["disabled_product"]
	if (*disabled)[0].EnableSemanticCache {
		t.Error("enableSemanticCache must be ignored on disabled rules")
	}

	// rule without the field defaults to false
	off := (*conf.Config)["semantic_off_product"]
	if (*off)[0].EnableSemanticCache {
		t.Error("enableSemanticCache should default to false")
	}
}

func TestProductRuleConfLoadNoSemanticBlock(t *testing.T) {
	// old-style rule file (phase 1): loads fine, semantic stays disabled
	conf, err := ProductRuleConfLoad("testdata/mod_ai_cache/mod_ai_cache_rule.data", DefaultCacheTTL)
	if err != nil {
		t.Fatalf("ProductRuleConfLoad() error: %v", err)
	}
	if conf.Semantic != nil {
		t.Errorf("old rule file should have no semantic conf, got %+v", conf.Semantic)
	}
}

func TestProductRuleConfLoadSemanticDefaultsAndCheck(t *testing.T) {
	writeRuleData := func(t *testing.T, semantic string) string {
		dir := t.TempDir()
		path := dir + "/rule.data"
		content := `{"Version":"1.0","Semantic":` + semantic + `,"Config":{"default":[{"cond":"default_t()","cacheKeyStrategy":"disabled"}]}}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write rule data: %v", err)
		}
		return path
	}

	// empty Semantic block gets all defaults
	path := writeRuleData(t, `{}`)
	conf, err := ProductRuleConfLoad(path, DefaultCacheTTL)
	if err != nil {
		t.Fatalf("ProductRuleConfLoad() error: %v", err)
	}
	if conf.Semantic.TopK != DefaultSemanticTopK ||
		conf.Semantic.Threshold != DefaultSemanticThreshold ||
		conf.Semantic.ThresholdRelation != DefaultSemanticThresholdRelation {
		t.Errorf("unexpected semantic defaults: %+v", conf.Semantic)
	}

	// invalid values are rejected
	cases := []struct {
		name     string
		semantic string
	}{
		{"topK too small", `{"topK":0}`},
		{"topK too large", `{"topK":11}`},
		{"threshold negative", `{"threshold":-0.1}`},
		{"threshold too large", `{"threshold":2.1}`},
		{"bad relation", `{"thresholdRelation":"eq"}`},
	}
	for _, c := range cases {
		path := writeRuleData(t, c.semantic)
		if _, err := ProductRuleConfLoad(path, DefaultCacheTTL); err == nil {
			t.Errorf("%s: expected load error, got nil", c.name)
		}
	}
}
