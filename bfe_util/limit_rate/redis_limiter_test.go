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

package limit_rate

import (
	"testing"
	"time"
)

// QPMLimiter 本地快速路径：无需访问 Redis 就能给出结论。
func TestQPMLimiterFastPaths(t *testing.T) {
	resetRedis(t)

	t.Run("NoConsume", func(t *testing.T) {
		limiter := NewQPMLimiter("qpm_limiter_zero_cost", 0, 60, 1)
		allowed, _, _, err := limiter.Check(0, testAgent)
		if err != nil || !allowed {
			t.Fatalf("allowed=%v err=%v, want true/nil", allowed, err)
		}
		if testRedis.Exists("qpm_limiter_zero_cost") {
			t.Error("zero cost check must not touch redis")
		}
	})

	t.Run("Unlimited", func(t *testing.T) {
		limiter := NewQPMLimiter("qpm_limiter_unlimited", 0, 60, -1)
		allowed, _, _, err := limiter.Check(1, testAgent)
		if err != nil || !allowed {
			t.Fatalf("allowed=%v err=%v, want true/nil", allowed, err)
		}
		if testRedis.Exists("qpm_limiter_unlimited") {
			t.Error("unlimited limiter must not touch redis")
		}
	})

	t.Run("Disabled", func(t *testing.T) {
		limiter := NewQPMLimiter("qpm_limiter_disabled", 0, 60, 0)
		allowed, _, _, err := limiter.Check(1, testAgent)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if allowed {
			t.Error("limit=0 should reject")
		}
		if testRedis.Exists("qpm_limiter_disabled") {
			t.Error("disabled limiter must not touch redis")
		}
	})
}

// QPMLimiter 正常路径：委托给 agent，并把 tat 透传出去。
func TestQPMLimiterCheck(t *testing.T) {
	resetRedis(t)
	limiter := NewQPMLimiter("qpm_limiter_normal", 0, 10, 1)

	tick(0)
	allowed, _, tat, err := limiter.Check(1, testAgent)
	if err != nil || !allowed {
		t.Fatalf("first check: allowed=%v err=%v", allowed, err)
	}
	if tat != 1 {
		t.Errorf("tat = %v, want 1", tat)
	}

	tick(2 * time.Second)
	allowed, _, _, err = limiter.Check(1, testAgent)
	if err != nil {
		t.Fatalf("second check failed: %v", err)
	}
	if allowed {
		t.Error("second check should be rejected at limit 1")
	}
}

// QPMLimiter 规则热更新后新参数立即生效。
func TestQPMLimiterUpdateArgs(t *testing.T) {
	resetRedis(t)
	limiter := NewQPMLimiter("qpm_limiter_update", 0, 10, 1)

	tick(0)
	if allowed, _, _, err := limiter.Check(1, testAgent); err != nil || !allowed {
		t.Fatalf("first check should be allowed, err=%v", err)
	}

	limiter.UpdateArgs(0, 10, -1)
	if allowed, _, _, err := limiter.Check(1, testAgent); err != nil || !allowed {
		t.Fatalf("after update to unlimited, check should be allowed, err=%v", err)
	}

	limiter.UpdateArgs(0, 10, 0)
	if allowed, _, _, err := limiter.Check(1, testAgent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if allowed {
		t.Error("after update to limit=0, check should be rejected")
	}
}

// QPMLimiter 在 Redis 故障时把错误透传，由上层决定如何降级。
func TestQPMLimiterRedisError(t *testing.T) {
	resetRedis(t)
	limiter := NewQPMLimiter("qpm_limiter_dead", 0, 10, 10)

	allowed, _, _, err := limiter.Check(1, deadAgent)
	if err == nil {
		t.Fatal("expected redis error")
	}
	if allowed {
		t.Error("check should not be allowed on redis error")
	}
}

// TPMLimiter 本地快速路径。
func TestTPMLimiterFastPaths(t *testing.T) {
	resetRedis(t)

	t.Run("Unlimited", func(t *testing.T) {
		limiter := NewTPMLimiter("tpm_limiter_unlimited", -1, 60, 50, 10)
		allowed, _, preConsume, err := limiter.TryCheck(10, testAgent)
		if err != nil || !allowed {
			t.Fatalf("allowed=%v err=%v, want true/nil", allowed, err)
		}
		if preConsume != 0 {
			t.Errorf("preConsume = %d, want 0 (unlimited)", preConsume)
		}
		if testRedis.Exists("tpm_limiter_unlimited") {
			t.Error("unlimited limiter must not touch redis")
		}
	})

	t.Run("Disabled", func(t *testing.T) {
		limiter := NewTPMLimiter("tpm_limiter_disabled", 0, 60, 50, 10)
		allowed, _, _, err := limiter.TryCheck(10, testAgent)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if allowed {
			t.Error("maxTokensPerWindow=0 should reject")
		}
	})

	t.Run("SingleRequestTooLarge", func(t *testing.T) {
		limiter := NewTPMLimiter("tpm_limiter_too_large", 100, 60, 50, 10)
		allowed, _, _, err := limiter.TryCheck(50, testAgent)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if allowed {
			t.Error("tokens >= bucket peak should be rejected locally")
		}
		if testRedis.Exists("tpm_limiter_too_large") {
			t.Error("local rejection must not touch redis")
		}
	})
}

// TPMLimiter 正常路径：允许时返回预扣量，拒绝时为 0。
func TestTPMLimiterTryCheck(t *testing.T) {
	resetRedis(t)
	limiter := NewTPMLimiter("tpm_limiter_normal", 100, 60, 50, 10)

	tick(0)
	allowed, _, preConsume, err := limiter.TryCheck(20, testAgent)
	if err != nil || !allowed {
		t.Fatalf("first try: allowed=%v err=%v", allowed, err)
	}
	if preConsume != 20 {
		t.Errorf("preConsume = %d, want 20", preConsume)
	}

	// 打满当前桶：50-20=30 剩余，再要 40 会被桶阈值拒绝
	allowed, _, preConsume, err = limiter.TryCheck(40, testAgent)
	if err != nil {
		t.Fatalf("second try failed: %v", err)
	}
	if allowed {
		t.Error("second try should be rejected by bucket peak")
	}
	if preConsume != 0 {
		t.Errorf("preConsume = %d, want 0 when rejected", preConsume)
	}
}

// TPMLimiter 事后记账：delta 为 0 时跳过，非 0 时写入。
func TestTPMLimiterUpdateTokenUsage(t *testing.T) {
	resetRedis(t)
	limiter := NewTPMLimiter("tpm_limiter_usage", 1000, 60, 1000, 10)

	tick(0)
	_, currentTime, _, err := limiter.TryCheck(10, testAgent)
	if err != nil {
		t.Fatalf("TryCheck failed: %v", err)
	}

	// delta=0：不写 Redis
	if err := limiter.UpdateTokenUsage(currentTime, 0, testAgent); err != nil {
		t.Fatalf("UpdateTokenUsage with delta=0 failed: %v", err)
	}
	if got := testRedis.HGet("tpm_limiter_usage", bucketField(0)); got != "10" {
		t.Errorf("bucket counter = %q, want 10 (delta=0 must not change it)", got)
	}

	// delta>0：累加真实用量
	if err := limiter.UpdateTokenUsage(currentTime, 90, testAgent); err != nil {
		t.Fatalf("UpdateTokenUsage failed: %v", err)
	}
	if got := testRedis.HGet("tpm_limiter_usage", bucketField(0)); got != "100" {
		t.Errorf("bucket counter = %q, want 100", got)
	}
}

// TPMLimiter 与 QPMLimiter 在 Redis 故障时返回错误。
func TestTPMLimiterRedisError(t *testing.T) {
	resetRedis(t)
	limiter := NewTPMLimiter("tpm_limiter_dead", 100, 60, 50, 10)

	allowed, _, _, err := limiter.TryCheck(10, deadAgent)
	if err == nil {
		t.Fatal("expected redis error on TryCheck")
	}
	if allowed {
		t.Error("TryCheck should not be allowed on redis error")
	}

	if err := limiter.UpdateTokenUsage(testBaseTime.Unix(), 10, deadAgent); err == nil {
		t.Fatal("expected redis error on UpdateTokenUsage")
	}
}

// TPMLimiter 规则热更新后新参数立即生效。
func TestTPMLimiterUpdateArgs(t *testing.T) {
	resetRedis(t)
	limiter := NewTPMLimiter("tpm_limiter_update", 100, 60, 50, 10)

	limiter.UpdateArgs(-1, 60, 50, 10)
	if allowed, _, _, err := limiter.TryCheck(10, testAgent); err != nil || !allowed {
		t.Fatalf("after update to unlimited, try should be allowed, err=%v", err)
	}

	limiter.UpdateArgs(100, 60, 5, 10)
	if allowed, _, _, err := limiter.TryCheck(10, testAgent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if allowed {
		t.Error("after update, tokens >= bucket peak should be rejected")
	}
}
