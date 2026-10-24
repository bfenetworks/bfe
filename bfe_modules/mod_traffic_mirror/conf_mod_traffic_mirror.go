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

package mod_traffic_mirror

import (
	"fmt"

	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_util"
	gcfg "gopkg.in/gcfg.v1"
)

const (
	DefaultConnectTimeoutMs     = 2000
	DefaultTTFBTimeoutMs        = 30000
	DefaultTotalTimeoutMs       = 600000
	DefaultMaxMirrorBodyBytes   = 2 * 1024 * 1024
	DefaultMaxResponseBodyBytes = 16 * 1024 * 1024
	DefaultMaxConcurrent        = 1024
	DefaultQueueCapacity        = 4096
	DefaultCBFailThreshold      = 50
	DefaultCBCooldownSec        = 30
)

type ConfModTrafficMirror struct {
	Basic struct {
		ProductRulePath string // path for product rule

		ConnectTimeoutMs int // connect timeout (ms)
		TTFBTimeoutMs    int // time to first byte timeout (ms)
		TotalTimeoutMs   int // total mirror request timeout (ms)

		MaxMirrorBodyBytes   int64 // max request body bytes to mirror
		MaxResponseBodyBytes int64 // max mirror response body bytes to drain

		MaxConcurrent int // module level concurrency limit (semaphore)
		QueueCapacity int // submit queue capacity, 0 means no queueing

		CircuitBreakerFailThreshold int // consecutive fails to open circuit
		CircuitBreakerCooldownSec   int // circuit cooldown (seconds)
	}

	Log struct {
		OpenDebug bool // whether open debug
	}
}

/* load config from config file */
func ConfLoad(filePath string, confRoot string) (*ConfModTrafficMirror, error) {
	var cfg ConfModTrafficMirror
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

func (cfg *ConfModTrafficMirror) Check(confRoot string) error {
	return ConfModTrafficMirrorCheck(cfg, confRoot)
}

func ConfModTrafficMirrorCheck(cfg *ConfModTrafficMirror, confRoot string) error {
	// check ProductRulePath
	if cfg.Basic.ProductRulePath == "" {
		return fmt.Errorf("ConfModTrafficMirrorCheck ProductRulePath is nil")
	}

	// build conf path
	cfg.Basic.ProductRulePath = bfe_util.ConfPathProc(cfg.Basic.ProductRulePath, confRoot)

	// set defaults and check layered timeouts
	if cfg.Basic.ConnectTimeoutMs == 0 {
		cfg.Basic.ConnectTimeoutMs = DefaultConnectTimeoutMs
	}
	if cfg.Basic.ConnectTimeoutMs <= 0 {
		return fmt.Errorf("Basic.ConnectTimeoutMs must > 0")
	}

	if cfg.Basic.TTFBTimeoutMs == 0 {
		cfg.Basic.TTFBTimeoutMs = DefaultTTFBTimeoutMs
	}
	if cfg.Basic.TTFBTimeoutMs <= 0 {
		return fmt.Errorf("Basic.TTFBTimeoutMs must > 0")
	}

	if cfg.Basic.TotalTimeoutMs == 0 {
		cfg.Basic.TotalTimeoutMs = DefaultTotalTimeoutMs
	}
	if cfg.Basic.TotalTimeoutMs < cfg.Basic.TTFBTimeoutMs {
		return fmt.Errorf("Basic.TotalTimeoutMs must >= Basic.TTFBTimeoutMs")
	}

	// check body limits
	if cfg.Basic.MaxMirrorBodyBytes == 0 {
		cfg.Basic.MaxMirrorBodyBytes = DefaultMaxMirrorBodyBytes
	}
	if cfg.Basic.MaxMirrorBodyBytes <= 0 ||
		cfg.Basic.MaxMirrorBodyBytes > bfe_http.MaxAccessibleBodySize {
		return fmt.Errorf("Basic.MaxMirrorBodyBytes must be in (0, %d]",
			bfe_http.MaxAccessibleBodySize)
	}

	if cfg.Basic.MaxResponseBodyBytes == 0 {
		cfg.Basic.MaxResponseBodyBytes = DefaultMaxResponseBodyBytes
	}
	if cfg.Basic.MaxResponseBodyBytes <= 0 {
		return fmt.Errorf("Basic.MaxResponseBodyBytes must > 0")
	}

	// check concurrency control
	if cfg.Basic.MaxConcurrent == 0 {
		cfg.Basic.MaxConcurrent = DefaultMaxConcurrent
	}
	if cfg.Basic.MaxConcurrent <= 0 {
		return fmt.Errorf("Basic.MaxConcurrent must > 0")
	}

	if cfg.Basic.QueueCapacity == 0 {
		cfg.Basic.QueueCapacity = DefaultQueueCapacity
	}
	if cfg.Basic.QueueCapacity < 0 {
		return fmt.Errorf("Basic.QueueCapacity must >= 0")
	}

	// check circuit breaker
	if cfg.Basic.CircuitBreakerFailThreshold == 0 {
		cfg.Basic.CircuitBreakerFailThreshold = DefaultCBFailThreshold
	}
	if cfg.Basic.CircuitBreakerFailThreshold <= 0 {
		return fmt.Errorf("Basic.CircuitBreakerFailThreshold must > 0")
	}

	if cfg.Basic.CircuitBreakerCooldownSec == 0 {
		cfg.Basic.CircuitBreakerCooldownSec = DefaultCBCooldownSec
	}
	if cfg.Basic.CircuitBreakerCooldownSec <= 0 {
		return fmt.Errorf("Basic.CircuitBreakerCooldownSec must > 0")
	}

	return nil
}
