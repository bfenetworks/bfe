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

// 并发槽位内放行：计数随 acquire 递增。
func TestConAcquireWithinLimit(t *testing.T) {
	resetRedis(t)
	const key, threshold, ttl = "con_within", 2, 60

	for i := int64(1); i <= threshold; i++ {
		currentTime, allowed, curCount, err := testAgent.ConAcquire(key, threshold, ttl)
		if err != nil {
			t.Fatalf("ConAcquire #%d failed: %v", i, err)
		}
		if !allowed {
			t.Fatalf("ConAcquire #%d should be allowed", i)
		}
		if curCount != i {
			t.Errorf("curCount #%d = %d, want %d", i, curCount, i)
		}
		if currentTime <= 0 {
			t.Errorf("currentTime = %d, want > 0", currentTime)
		}
	}

	if got := redisInt(t, key); got != threshold {
		t.Errorf("stored counter = %d, want %d", got, threshold)
	}
	if ttlLeft := testRedis.TTL(key); ttlLeft <= 0 {
		t.Errorf("key should carry a TTL, got %v", ttlLeft)
	}
}

// 超过上限拒绝，且不递增计数。
func TestConAcquireExceedLimit(t *testing.T) {
	resetRedis(t)
	const key, threshold, ttl = "con_exceed", 1, 60

	if _, allowed, _, err := testAgent.ConAcquire(key, threshold, ttl); err != nil || !allowed {
		t.Fatalf("first acquire should be allowed, err=%v", err)
	}

	_, allowed, curCount, err := testAgent.ConAcquire(key, threshold, ttl)
	if err != nil {
		t.Fatalf("ConAcquire failed: %v", err)
	}
	if allowed {
		t.Fatal("acquire over limit should be rejected")
	}
	if curCount != threshold {
		t.Errorf("curCount = %d, want %d (rejected acquire must not increment)", curCount, threshold)
	}
	if got := redisInt(t, key); got != threshold {
		t.Errorf("stored counter = %d, want %d", got, threshold)
	}
}

// 释放递减计数，且不会减成负数。
func TestConRelease(t *testing.T) {
	resetRedis(t)
	const key, threshold, ttl = "con_release", 2, 60

	for i := 0; i < threshold; i++ {
		if _, allowed, _, err := testAgent.ConAcquire(key, threshold, ttl); err != nil || !allowed {
			t.Fatalf("acquire #%d should be allowed, err=%v", i, err)
		}
	}

	for want := int64(threshold - 1); want >= 0; want-- {
		curCount, err := testAgent.ConRelease(key, ttl)
		if err != nil {
			t.Fatalf("ConRelease failed: %v", err)
		}
		if curCount != want {
			t.Errorf("curCount = %d, want %d", curCount, want)
		}
	}

	// 已归零后再释放：返回值 0，且不写负值
	curCount, err := testAgent.ConRelease(key, ttl)
	if err != nil {
		t.Fatalf("ConRelease on empty key failed: %v", err)
	}
	if curCount != 0 {
		t.Errorf("curCount = %d, want 0", curCount)
	}
	if testRedis.Exists(key) && redisInt(t, key) < 0 {
		t.Error("counter must not go negative")
	}
}

// 未 acquire 就 release：返回 0，不创建键。
func TestConReleaseWithoutAcquire(t *testing.T) {
	resetRedis(t)
	curCount, err := testAgent.ConRelease("con_release_missing", 60)
	if err != nil {
		t.Fatalf("ConRelease failed: %v", err)
	}
	if curCount != 0 {
		t.Errorf("curCount = %d, want 0", curCount)
	}
	if testRedis.Exists("con_release_missing") {
		t.Error("release must not create the counter key")
	}
}

// TTL 到期后槽位自动回收，避免实例崩溃导致并发额度永久占用。
func TestConAcquireAfterTTLExpire(t *testing.T) {
	resetRedis(t)
	const key, threshold, ttl = "con_ttl", 1, 2

	if _, allowed, _, err := testAgent.ConAcquire(key, threshold, ttl); err != nil || !allowed {
		t.Fatalf("first acquire should be allowed, err=%v", err)
	}
	if _, allowed, _, err := testAgent.ConAcquire(key, threshold, ttl); err != nil {
		t.Fatalf("ConAcquire failed: %v", err)
	} else if allowed {
		t.Fatal("second acquire should be rejected before TTL expires")
	}

	testRedis.FastForward(3 * time.Second)
	if testRedis.Exists(key) {
		t.Fatal("counter key should expire after TTL")
	}

	_, allowed, curCount, err := testAgent.ConAcquire(key, threshold, ttl)
	if err != nil {
		t.Fatalf("ConAcquire after expiry failed: %v", err)
	}
	if !allowed {
		t.Fatal("acquire after TTL expiry should be allowed")
	}
	if curCount != 1 {
		t.Errorf("curCount = %d, want 1 (counter restarted)", curCount)
	}
}

// 脚本未初始化时报错。
func TestConScriptNotInitialized(t *testing.T) {
	resetRedis(t)
	agent := &RedisLRAgent{}
	if _, _, _, err := agent.ConAcquire("con_no_script", 1, 60); err == nil {
		t.Fatal("expected error when acquire script is nil")
	}
	if _, err := agent.ConRelease("con_no_script", 60); err == nil {
		t.Fatal("expected error when release script is nil")
	}
}

// Redis 不可用时错误向上传播。
func TestConRedisError(t *testing.T) {
	resetRedis(t)
	if _, _, _, err := deadAgent.ConAcquire("con_dead", 1, 60); err == nil {
		t.Fatal("expected redis error on acquire")
	}
	if _, err := deadAgent.ConRelease("con_dead", 60); err == nil {
		t.Fatal("expected redis error on release")
	}
}
