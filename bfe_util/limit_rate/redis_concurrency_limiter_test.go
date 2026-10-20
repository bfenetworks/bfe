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
)

// 负阈值表示不限流：直接放行，且不访问 Redis。
func TestLimiterUnlimited(t *testing.T) {
	resetRedis(t)
	limiter := NewConcurrencyLimiter("limiter_unlimited", -1, 60)

	allowed, currentTime, curCount, err := limiter.ConnAcquire(testAgent)
	if err != nil {
		t.Fatalf("ConnAcquire failed: %v", err)
	}
	if !allowed {
		t.Error("negative threshold should always allow")
	}
	if curCount != -1 {
		t.Errorf("curCount = %d, want -1 (unlimited marker)", curCount)
	}
	if currentTime <= 0 {
		t.Errorf("currentTime = %d, want > 0", currentTime)
	}
	if testRedis.Exists("limiter_unlimited") {
		t.Error("unlimited limiter must not touch redis")
	}
}

// 零阈值表示禁用：直接拒绝，且不访问 Redis。
func TestLimiterZeroThreshold(t *testing.T) {
	resetRedis(t)
	limiter := NewConcurrencyLimiter("limiter_zero", 0, 60)

	allowed, _, curCount, err := limiter.ConnAcquire(testAgent)
	if err != nil {
		t.Fatalf("ConnAcquire failed: %v", err)
	}
	if allowed {
		t.Error("zero threshold should always reject")
	}
	if curCount != 0 {
		t.Errorf("curCount = %d, want 0", curCount)
	}
	if testRedis.Exists("limiter_zero") {
		t.Error("disabled limiter must not touch redis")
	}
}

// 正常路径：limiter 复用 agent 的 acquire/release，键与阈值取自自身配置。
func TestLimiterAcquireRelease(t *testing.T) {
	resetRedis(t)
	limiter := NewConcurrencyLimiter("limiter_normal", 1, 60)

	allowed, _, curCount, err := limiter.ConnAcquire(testAgent)
	if err != nil || !allowed || curCount != 1 {
		t.Fatalf("first acquire: allowed=%v curCount=%d err=%v", allowed, curCount, err)
	}

	allowed, _, curCount, err = limiter.ConnAcquire(testAgent)
	if err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}
	if allowed {
		t.Error("second acquire should be rejected at threshold 1")
	}
	if curCount != 1 {
		t.Errorf("curCount = %d, want 1", curCount)
	}

	curCount, err = limiter.ConnRelease(testAgent)
	if err != nil {
		t.Fatalf("ConnRelease failed: %v", err)
	}
	if curCount != 0 {
		t.Errorf("curCount after release = %d, want 0", curCount)
	}
}

// 规则热更新后新阈值立即生效。
func TestLimiterUpdateArgs(t *testing.T) {
	resetRedis(t)
	limiter := NewConcurrencyLimiter("limiter_update", 10, 60)

	if allowed, _, _, err := limiter.ConnAcquire(testAgent); err != nil || !allowed {
		t.Fatalf("acquire with threshold 10 should be allowed, err=%v", err)
	}

	limiter.UpdateArgs(0)
	allowed, _, _, err := limiter.ConnAcquire(testAgent)
	if err != nil {
		t.Fatalf("acquire after update failed: %v", err)
	}
	if allowed {
		t.Error("acquire should be rejected after threshold updated to 0")
	}

	limiter.UpdateArgs(-1)
	if allowed, _, _, err := limiter.ConnAcquire(testAgent); err != nil || !allowed {
		t.Fatalf("acquire after update to -1 should be allowed, err=%v", err)
	}
}

// Redis 故障时错误向上传播，由上层决定 fail-open / fail-closed。
func TestLimiterRedisError(t *testing.T) {
	resetRedis(t)
	limiter := NewConcurrencyLimiter("limiter_dead", 1, 60)

	allowed, _, _, err := limiter.ConnAcquire(deadAgent)
	if err == nil {
		t.Fatal("expected redis error")
	}
	if allowed {
		t.Error("acquire should not be allowed on redis error")
	}

	if _, err := limiter.ConnRelease(deadAgent); err == nil {
		t.Fatal("expected redis error on release")
	}
}
