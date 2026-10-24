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
	"fmt"

	"github.com/bfenetworks/bfe/bfe_util"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
	gcfg "gopkg.in/gcfg.v1"
)

const (
	DefaultCacheKeyPrefix = "ai_cache"
	DefaultCacheTTL       = 3600 // seconds

	DefaultMaxBodyBytes  = 1048576 // 1MB
	DefaultMaxValueBytes = 1048576 // 1MB
)

// semantic cache defaults
const (
	DefaultEmbeddingTimeoutMs = 500
	DefaultVectorTimeoutMs    = 300
	DefaultVectorCollection   = "ai_cache_semantic"
	DefaultMaxQuestionBytes   = 4096

	// vector store types supported by the semantic cache
	VectorTypeChroma = "chroma"
)

// EmbeddingConnectConf is the normalized [embedding] section of
// mod_ai_cache.conf.
type EmbeddingConnectConf struct {
	ServiceHost string // embedding service host
	ServicePort int    // embedding service port
	UseHttps    bool   // use https scheme
	ApiKey      string // bearer credential, never logged
	Model       string // embedding model name
	TimeoutMs   int    // per-call timeout (ms)
}

// VectorConnectConf is the normalized [vector] section of mod_ai_cache.conf.
type VectorConnectConf struct {
	Type             string // vector store type, only "chroma" for now
	ServiceHost      string // vector store host
	ServicePort      int    // vector store port
	ApiKey           string // bearer credential, never logged
	Collection       string // collection name
	TimeoutMs        int    // per-call timeout (ms)
	MaxQuestionBytes int64  // question length limit for semantic lookup
}

// SemanticConnectConf carries the connection info of the semantic cache
// (embedding service + vector store). It is nil when the semantic cache is
// not configured.
type SemanticConnectConf struct {
	Embedding *EmbeddingConnectConf
	Vector    *VectorConnectConf
}

type ConfModAiCache struct {
	Basic struct {
		ProductRulePath string // path for product rule
		CacheKeyPrefix  string // prefix of cache key
		DefaultCacheTTL int    // default cache TTL (seconds)
	}

	// redis conf
	Redis struct {
		Bns            string // bns name for redis proxy
		ConnectTimeout int    // connect timeout (ms)
		ReadTimeout    int    // read timeout (ms)
		WriteTimeout   int    // write timeout(ms)

		// max idle connections in pool
		MaxIdle int

		// redis password, ignore if not set
		Password string

		// max active connections in pool,
		// when set 0, there is no connection num limit
		MaxActive int
	}

	Log struct {
		OpenDebug bool // whether open debug
	}

	// embedding service conf (optional): semantic cache is enabled only when
	// both [embedding] and [vector] are configured
	Embedding struct {
		ServiceHost string // embedding service host
		ServicePort int    // embedding service port
		UseHttps    bool   // use https scheme
		ApiKey      string // bearer credential, never logged
		Model       string // embedding model name
		TimeoutMs   int    // per-call timeout (ms)
	}

	// vector store conf (optional), see [embedding]
	Vector struct {
		Type             string // vector store type, only "chroma" for now
		ServiceHost      string // vector store host
		ServicePort      int    // vector store port
		ApiKey           string // bearer credential, never logged
		Collection       string // collection name
		TimeoutMs        int    // per-call timeout (ms)
		MaxQuestionBytes int64  // question length limit for semantic lookup
	}
}

/* load config from config file */
func ConfLoad(filePath string, confRoot string) (*ConfModAiCache, error) {
	var cfg ConfModAiCache
	var err error

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return &cfg, err
	}

	// check conf
	err = cfg.Check(confRoot)
	if err != nil {
		return &cfg, err
	}

	return &cfg, nil
}

func (cfg *ConfModAiCache) Check(confRoot string) error {
	return ConfModAiCacheCheck(cfg, confRoot)
}

func ConfModAiCacheCheck(cfg *ConfModAiCache, confRoot string) error {
	// check ProductRulePath
	if cfg.Basic.ProductRulePath == "" {
		return fmt.Errorf("ConfModAiCacheCheck ProductRulePath is nil")
	}

	// build conf path
	cfg.Basic.ProductRulePath = bfe_util.ConfPathProc(cfg.Basic.ProductRulePath, confRoot)

	// set default cache key prefix
	if cfg.Basic.CacheKeyPrefix == "" {
		cfg.Basic.CacheKeyPrefix = DefaultCacheKeyPrefix
	}

	// set default cache TTL
	if cfg.Basic.DefaultCacheTTL == 0 {
		cfg.Basic.DefaultCacheTTL = DefaultCacheTTL
	}
	if cfg.Basic.DefaultCacheTTL < 0 {
		return fmt.Errorf("Basic.DefaultCacheTTL must >= 0")
	}

	// check redis server conf
	if err := redis_client.CheckRedisConf(cfg.Redis.Bns); err != nil {
		return err
	}

	// check connectTimeOut
	if cfg.Redis.ConnectTimeout <= 0 {
		return fmt.Errorf("Redis.ConnectTimeout must > 0")
	}

	// check Read/Write Timeout
	if cfg.Redis.ReadTimeout <= 0 || cfg.Redis.WriteTimeout <= 0 {
		return fmt.Errorf("Redis.ReadTimeout/WriteTimeout must > 0")
	}

	return nil
}

// SemanticConnect validates the optional [embedding]/[vector] sections and
// returns the normalized connection conf. It returns (nil, nil) when neither
// section is configured (pure exact-match cache mode). Any validation error
// is returned to the caller, which is expected to disable the semantic cache
// (fail-open) and keep the exact-match cache running.
func (cfg *ConfModAiCache) SemanticConnect() (*SemanticConnectConf, error) {
	emb := cfg.Embedding
	vec := cfg.Vector

	embConfigured := emb.ServiceHost != "" || emb.ServicePort != 0 || emb.UseHttps ||
		emb.ApiKey != "" || emb.Model != "" || emb.TimeoutMs != 0
	vecConfigured := vec.Type != "" || vec.ServiceHost != "" || vec.ServicePort != 0 ||
		vec.ApiKey != "" || vec.Collection != "" || vec.TimeoutMs != 0 || vec.MaxQuestionBytes != 0

	if !embConfigured && !vecConfigured {
		return nil, nil
	}
	if embConfigured != vecConfigured {
		return nil, fmt.Errorf("[embedding] and [vector] must be configured together")
	}

	// check embedding conf
	if emb.ServiceHost == "" {
		return nil, fmt.Errorf("Embedding.ServiceHost must not be empty")
	}
	if emb.ServicePort < 1 || emb.ServicePort > 65535 {
		return nil, fmt.Errorf("Embedding.ServicePort must be in 1-65535")
	}
	if emb.Model == "" {
		return nil, fmt.Errorf("Embedding.Model must not be empty")
	}
	if emb.TimeoutMs == 0 {
		emb.TimeoutMs = DefaultEmbeddingTimeoutMs
	}
	if emb.TimeoutMs < 1 || emb.TimeoutMs > 5000 {
		return nil, fmt.Errorf("Embedding.TimeoutMs must be in 1-5000")
	}

	// check vector conf
	if vec.Type == "" {
		vec.Type = VectorTypeChroma
	}
	if vec.Type != VectorTypeChroma {
		return nil, fmt.Errorf("Vector.Type only supports %s", VectorTypeChroma)
	}
	if vec.ServiceHost == "" {
		return nil, fmt.Errorf("Vector.ServiceHost must not be empty")
	}
	if vec.ServicePort < 1 || vec.ServicePort > 65535 {
		return nil, fmt.Errorf("Vector.ServicePort must be in 1-65535")
	}
	if vec.Collection == "" {
		vec.Collection = DefaultVectorCollection
	}
	if vec.TimeoutMs == 0 {
		vec.TimeoutMs = DefaultVectorTimeoutMs
	}
	if vec.TimeoutMs < 1 || vec.TimeoutMs > 5000 {
		return nil, fmt.Errorf("Vector.TimeoutMs must be in 1-5000")
	}
	if vec.MaxQuestionBytes == 0 {
		vec.MaxQuestionBytes = DefaultMaxQuestionBytes
	}
	if vec.MaxQuestionBytes < 0 {
		return nil, fmt.Errorf("Vector.MaxQuestionBytes must >= 0")
	}

	return &SemanticConnectConf{
		Embedding: &EmbeddingConnectConf{
			ServiceHost: emb.ServiceHost,
			ServicePort: emb.ServicePort,
			UseHttps:    emb.UseHttps,
			ApiKey:      emb.ApiKey,
			Model:       emb.Model,
			TimeoutMs:   emb.TimeoutMs,
		},
		Vector: &VectorConnectConf{
			Type:             vec.Type,
			ServiceHost:      vec.ServiceHost,
			ServicePort:      vec.ServicePort,
			ApiKey:           vec.ApiKey,
			Collection:       vec.Collection,
			TimeoutMs:        vec.TimeoutMs,
			MaxQuestionBytes: vec.MaxQuestionBytes,
		},
	}, nil
}
