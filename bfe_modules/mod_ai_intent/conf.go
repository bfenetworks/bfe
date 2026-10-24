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

package mod_ai_intent

import (
	"fmt"

	"github.com/bfenetworks/bfe/bfe_util"

	gcfg "gopkg.in/gcfg.v1"
)

// default values for optional configuration items
const (
	DefaultTimeoutMs            = 300
	DefaultMaxStateChars        = 2000
	DefaultCacheSize            = 10000
	DefaultCacheTTLSeconds      = 1800
	DefaultExplicitIntentHeader = "X-AI-Intent"
	DefaultFailureThreshold     = 5
	DefaultProbeIntervalMs      = 5000
)

type ConfModAiIntent struct {
	Basic struct {
		DecisionServiceAddr  string // decision service address (System One protocol)
		QuestionsPath        string // path for intent questions data file
		TimeoutMs            int    // per classification timeout (ms)
		MaxStateChars        int    // rune truncation length of the classified text
		CacheSize            int    // max entries of in-process LRU cache
		CacheTTLSeconds      int    // TTL of cache entries (s)
		ExplicitIntentHeader string // request header for explicit intent declaration
	}
	Breaker struct {
		FailureThreshold int // consecutive failures before breaker opens
		ProbeIntervalMs  int // probe interval while breaker is open (ms)
	}
	Log struct {
		OpenDebug bool
	}
}

func ConfLoad(filePath string, confRoot string) (*ConfModAiIntent, error) {
	var cfg ConfModAiIntent
	if err := gcfg.ReadFileInto(&cfg, filePath); err != nil {
		return &cfg, err
	}
	if err := cfg.Check(confRoot); err != nil {
		return &cfg, err
	}
	return &cfg, nil
}

func (cfg *ConfModAiIntent) Check(confRoot string) error {
	return ConfModAiIntentCheck(cfg, confRoot)
}

func ConfModAiIntentCheck(cfg *ConfModAiIntent, confRoot string) error {
	if cfg.Basic.DecisionServiceAddr == "" {
		return fmt.Errorf("ConfModAiIntentCheck: DecisionServiceAddr is empty")
	}
	if cfg.Basic.QuestionsPath == "" {
		return fmt.Errorf("ConfModAiIntentCheck: QuestionsPath is empty")
	}
	cfg.Basic.QuestionsPath = bfe_util.ConfPathProc(cfg.Basic.QuestionsPath, confRoot)

	if cfg.Basic.TimeoutMs <= 0 {
		cfg.Basic.TimeoutMs = DefaultTimeoutMs
	}
	if cfg.Basic.MaxStateChars <= 0 {
		cfg.Basic.MaxStateChars = DefaultMaxStateChars
	}
	if cfg.Basic.CacheSize < 0 {
		return fmt.Errorf("ConfModAiIntentCheck: CacheSize should >= 0")
	}
	if cfg.Basic.CacheSize == 0 {
		cfg.Basic.CacheSize = DefaultCacheSize
	}
	if cfg.Basic.CacheTTLSeconds <= 0 {
		cfg.Basic.CacheTTLSeconds = DefaultCacheTTLSeconds
	}
	if cfg.Basic.ExplicitIntentHeader == "" {
		cfg.Basic.ExplicitIntentHeader = DefaultExplicitIntentHeader
	}

	if cfg.Breaker.FailureThreshold <= 0 {
		cfg.Breaker.FailureThreshold = DefaultFailureThreshold
	}
	if cfg.Breaker.ProbeIntervalMs <= 0 {
		cfg.Breaker.ProbeIntervalMs = DefaultProbeIntervalMs
	}
	return nil
}
