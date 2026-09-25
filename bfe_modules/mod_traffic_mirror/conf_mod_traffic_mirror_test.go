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
	"testing"
)

func TestConfLoad(t *testing.T) {
	cfg, err := ConfLoad("testdata/mod_traffic_mirror/mod_traffic_mirror.conf", "./testdata")
	if err != nil {
		t.Fatalf("ConfLoad failed: %v", err)
	}

	if cfg.Basic.ProductRulePath == "" {
		t.Error("ProductRulePath should be set")
	}
	if cfg.Basic.ConnectTimeoutMs != 2000 {
		t.Errorf("ConnectTimeoutMs = %d, want 2000", cfg.Basic.ConnectTimeoutMs)
	}
	if cfg.Basic.TTFBTimeoutMs != 30000 {
		t.Errorf("TTFBTimeoutMs = %d, want 30000", cfg.Basic.TTFBTimeoutMs)
	}
	if cfg.Basic.TotalTimeoutMs != 600000 {
		t.Errorf("TotalTimeoutMs = %d, want 600000", cfg.Basic.TotalTimeoutMs)
	}
	if cfg.Basic.MaxMirrorBodyBytes != 2097152 {
		t.Errorf("MaxMirrorBodyBytes = %d, want 2097152", cfg.Basic.MaxMirrorBodyBytes)
	}
	if cfg.Basic.MaxResponseBodyBytes != 16777216 {
		t.Errorf("MaxResponseBodyBytes = %d, want 16777216", cfg.Basic.MaxResponseBodyBytes)
	}
	if cfg.Basic.MaxConcurrent != 8 {
		t.Errorf("MaxConcurrent = %d, want 8", cfg.Basic.MaxConcurrent)
	}
	if cfg.Basic.QueueCapacity != 16 {
		t.Errorf("QueueCapacity = %d, want 16", cfg.Basic.QueueCapacity)
	}
	if cfg.Basic.CircuitBreakerFailThreshold != 3 {
		t.Errorf("CircuitBreakerFailThreshold = %d, want 3", cfg.Basic.CircuitBreakerFailThreshold)
	}
	if cfg.Basic.CircuitBreakerCooldownSec != 1 {
		t.Errorf("CircuitBreakerCooldownSec = %d, want 1", cfg.Basic.CircuitBreakerCooldownSec)
	}
}

func TestConfLoadDefaults(t *testing.T) {
	cfg, err := ConfLoad("testdata/mod_traffic_mirror/mod_traffic_mirror_defaults.conf", "./testdata")
	if err != nil {
		t.Fatalf("ConfLoad failed: %v", err)
	}

	if cfg.Basic.ConnectTimeoutMs != DefaultConnectTimeoutMs {
		t.Errorf("ConnectTimeoutMs = %d, want default %d", cfg.Basic.ConnectTimeoutMs, DefaultConnectTimeoutMs)
	}
	if cfg.Basic.TTFBTimeoutMs != DefaultTTFBTimeoutMs {
		t.Errorf("TTFBTimeoutMs = %d, want default %d", cfg.Basic.TTFBTimeoutMs, DefaultTTFBTimeoutMs)
	}
	if cfg.Basic.TotalTimeoutMs != DefaultTotalTimeoutMs {
		t.Errorf("TotalTimeoutMs = %d, want default %d", cfg.Basic.TotalTimeoutMs, DefaultTotalTimeoutMs)
	}
	if cfg.Basic.MaxMirrorBodyBytes != DefaultMaxMirrorBodyBytes {
		t.Errorf("MaxMirrorBodyBytes = %d, want default %d", cfg.Basic.MaxMirrorBodyBytes, DefaultMaxMirrorBodyBytes)
	}
	if cfg.Basic.MaxResponseBodyBytes != DefaultMaxResponseBodyBytes {
		t.Errorf("MaxResponseBodyBytes = %d, want default %d", cfg.Basic.MaxResponseBodyBytes, DefaultMaxResponseBodyBytes)
	}
	if cfg.Basic.MaxConcurrent != DefaultMaxConcurrent {
		t.Errorf("MaxConcurrent = %d, want default %d", cfg.Basic.MaxConcurrent, DefaultMaxConcurrent)
	}
	if cfg.Basic.QueueCapacity != DefaultQueueCapacity {
		t.Errorf("QueueCapacity = %d, want default %d", cfg.Basic.QueueCapacity, DefaultQueueCapacity)
	}
	if cfg.Basic.CircuitBreakerFailThreshold != DefaultCBFailThreshold {
		t.Errorf("CircuitBreakerFailThreshold = %d, want default %d",
			cfg.Basic.CircuitBreakerFailThreshold, DefaultCBFailThreshold)
	}
	if cfg.Basic.CircuitBreakerCooldownSec != DefaultCBCooldownSec {
		t.Errorf("CircuitBreakerCooldownSec = %d, want default %d",
			cfg.Basic.CircuitBreakerCooldownSec, DefaultCBCooldownSec)
	}
}

func TestConfCheckTotalTimeoutLessThanTTFB(t *testing.T) {
	cfg, err := ConfLoad("testdata/mod_traffic_mirror/mod_traffic_mirror.conf", "./testdata")
	if err != nil {
		t.Fatalf("ConfLoad failed: %v", err)
	}

	cfg.Basic.TotalTimeoutMs = cfg.Basic.TTFBTimeoutMs - 1
	if err := cfg.Check("./testdata"); err == nil {
		t.Error("Check should fail when TotalTimeoutMs < TTFBTimeoutMs")
	}
}

func TestConfCheckInvalidBodyLimit(t *testing.T) {
	cfg, err := ConfLoad("testdata/mod_traffic_mirror/mod_traffic_mirror.conf", "./testdata")
	if err != nil {
		t.Fatalf("ConfLoad failed: %v", err)
	}

	cfg.Basic.MaxMirrorBodyBytes = -1
	if err := cfg.Check("./testdata"); err == nil {
		t.Error("Check should fail for negative MaxMirrorBodyBytes")
	}
}
