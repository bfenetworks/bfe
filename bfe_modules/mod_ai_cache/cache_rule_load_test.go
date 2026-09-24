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
