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
	"strings"
	"testing"
	"time"
)

// 窗口内放行：limit 次请求依次通过，返回值逐次累加。
func TestQpmWithinLimit(t *testing.T) {
	resetRedis(t)
	const key, period, limit = "qpm_within", 60, 3

	for i := int64(1); i <= limit; i++ {
		tick(time.Duration(i) * time.Second)
		currentTime, allowed, tat, err := testAgent.QpmCheck(key, period, limit, 1)
		if err != nil {
			t.Fatalf("QpmCheck #%d failed: %v", i, err)
		}
		if !allowed {
			t.Fatalf("QpmCheck #%d should be allowed", i)
		}
		if tat != float64(i) {
			t.Errorf("tat #%d = %v, want %d", i, tat, i)
		}
		if currentTime <= 0 {
			t.Errorf("currentTime = %d, want > 0", currentTime)
		}
	}

	if ttl := testRedis.TTL(key); ttl <= 0 {
		t.Errorf("key should carry a TTL, got %v", ttl)
	}
}

// 超限拒绝：达到 limit 后再请求被拒，且不写入新成员。
func TestQpmExceedLimit(t *testing.T) {
	resetRedis(t)
	const key, period, limit = "qpm_exceed", 60, 2

	for i := 0; i < limit; i++ {
		tick(time.Duration(i) * time.Second)
		if _, allowed, _, err := testAgent.QpmCheck(key, period, limit, 1); err != nil || !allowed {
			t.Fatalf("request #%d should be allowed, err=%v", i, err)
		}
	}

	tick(3 * time.Second)
	_, allowed, tat, err := testAgent.QpmCheck(key, period, limit, 1)
	if err != nil {
		t.Fatalf("QpmCheck failed: %v", err)
	}
	if allowed {
		t.Fatal("request over limit should be rejected")
	}
	// 拒绝时返回的是当前窗口计数，不含本次
	if tat != float64(limit) {
		t.Errorf("tat = %v, want %d", tat, limit)
	}

	members, err := testRedis.ZMembers(key)
	if err != nil {
		t.Fatalf("ZMembers failed: %v", err)
	}
	if len(members) != limit {
		t.Errorf("zset size = %d, want %d (rejected request must not be recorded)", len(members), limit)
	}
}

// 窗口过期后恢复：整窗滑出后计数归零。
func TestQpmWindowExpire(t *testing.T) {
	resetRedis(t)
	const key, period, limit = "qpm_expire", 10, 3

	for i := 0; i < limit; i++ {
		tick(time.Duration(i) * time.Second)
		if _, allowed, _, err := testAgent.QpmCheck(key, period, limit, 1); err != nil || !allowed {
			t.Fatalf("request #%d should be allowed, err=%v", i, err)
		}
	}

	// 推进到所有成员都早于 now-period
	tick(time.Duration(period+10) * time.Second)

	_, allowed, tat, err := testAgent.QpmCheck(key, period, limit, 1)
	if err != nil {
		t.Fatalf("QpmCheck failed: %v", err)
	}
	if !allowed {
		t.Fatal("request should be allowed after window expired")
	}
	if tat != 1 {
		t.Errorf("tat = %v, want 1 (window should be empty)", tat)
	}
}

// 滑动而非固定窗口：只滑出已过期的成员，保留仍在窗口内的成员。
func TestQpmSlidingWindow(t *testing.T) {
	resetRedis(t)
	const key, period, limit = "qpm_sliding", 10, 2

	tick(0)
	if _, allowed, _, err := testAgent.QpmCheck(key, period, limit, 1); err != nil || !allowed {
		t.Fatalf("t=0 should be allowed, err=%v", err)
	}

	tick(5 * time.Second)
	if _, allowed, _, err := testAgent.QpmCheck(key, period, limit, 1); err != nil || !allowed {
		t.Fatalf("t=5 should be allowed, err=%v", err)
	}

	tick(9 * time.Second)
	if _, allowed, _, err := testAgent.QpmCheck(key, period, limit, 1); err != nil {
		t.Fatalf("QpmCheck failed: %v", err)
	} else if allowed {
		t.Fatal("t=9 should be rejected: both members are still in window")
	}

	// 10.5s 时只剩 t=5 的成员在窗口内（now-period = 0.5），允许 1 次
	tick(10500 * time.Millisecond)
	_, allowed, tat, err := testAgent.QpmCheck(key, period, limit, 1)
	if err != nil {
		t.Fatalf("QpmCheck failed: %v", err)
	}
	if !allowed {
		t.Fatal("t=10.5 should be allowed: the t=0 member has slid out")
	}
	if tat != 2 {
		t.Errorf("tat = %v, want 2 (one old member remains)", tat)
	}
}

// 同一微秒内的多次请求会折叠成同一个 ZSET 成员，因此只计 1 次。
// 这是成员命名（now:microsecond）带来的既有语义，此处固化以防回归。
// 同一微秒内的多次请求会命中同一个 ZSET 成员（成员名为 now:microsecond），
// 因此成员数只增加 1 个，而返回值 tat 仍按 count+cost 递增。
// 即：极短时间内的并发请求在存储侧被折叠、实际计数偏小。这是既有语义，
// 此处固化以防回归；如需严格计数需改造成员命名（如加入随机后缀）。
func TestQpmSameInstantCollapsesMember(t *testing.T) {
	resetRedis(t)
	const key, period, limit = "qpm_same_instant", 60, 10

	tick(0)
	if _, allowed, tat, err := testAgent.QpmCheck(key, period, limit, 1); err != nil || !allowed {
		t.Fatalf("first call should be allowed, err=%v", err)
	} else if tat != 1 {
		t.Errorf("first tat = %v, want 1", tat)
	}

	// 时钟不动，再请求一次
	_, allowed, tat, err := testAgent.QpmCheck(key, period, limit, 1)
	if err != nil {
		t.Fatalf("QpmCheck failed: %v", err)
	}
	if !allowed {
		t.Fatal("second call should be allowed")
	}
	if tat != 2 {
		t.Errorf("tat = %v, want 2 (returned count still increments)", tat)
	}

	members, err := testRedis.ZMembers(key)
	if err != nil {
		t.Fatalf("ZMembers failed: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("zset size = %d, want 1: same-instant calls collapse into one member", len(members))
	}
}

// expectUse 只影响返回值，不改变实际记账的成员数：限流按"请求次数"生效，
// 不按"消耗单位数"生效。调用方不能依赖 tat 做累计计费。
func TestQpmExpectUseAffectsOnlyReturnedCount(t *testing.T) {
	resetRedis(t)
	const key, period, limit = "qpm_cost", 60, 10

	tick(0)
	_, allowed, tat, err := testAgent.QpmCheck(key, period, limit, 5)
	if err != nil || !allowed {
		t.Fatalf("first call should be allowed, err=%v", err)
	}
	if tat != 5 {
		t.Errorf("tat = %v, want 5", tat)
	}

	tick(time.Second)
	_, allowed, tat, err = testAgent.QpmCheck(key, period, limit, 5)
	if err != nil || !allowed {
		t.Fatalf("second call should be allowed, err=%v", err)
	}
	if tat != 6 {
		t.Errorf("tat = %v, want 6", tat)
	}

	members, _ := testRedis.ZMembers(key)
	if len(members) != 2 {
		t.Errorf("zset size = %d, want 2: each call records one member regardless of cost", len(members))
	}
}

// 不同 key 互不影响。
func TestQpmKeyIsolation(t *testing.T) {
	resetRedis(t)
	const period, limit = 60, 1

	tick(0)
	if _, allowed, _, err := testAgent.QpmCheck("qpm_iso_a", period, limit, 1); err != nil || !allowed {
		t.Fatalf("key a should be allowed, err=%v", err)
	}
	tick(time.Second)
	if _, allowed, _, err := testAgent.QpmCheck("qpm_iso_a", period, limit, 1); err != nil {
		t.Fatalf("QpmCheck failed: %v", err)
	} else if allowed {
		t.Fatal("key a should be exhausted")
	}
	if _, allowed, _, err := testAgent.QpmCheck("qpm_iso_b", period, limit, 1); err != nil || !allowed {
		t.Fatalf("key b should be independent and allowed, err=%v", err)
	}
}

// 非法 period 直接报错，不触达 Redis。
func TestQpmInvalidPeriod(t *testing.T) {
	resetRedis(t)
	for _, period := range []int64{0, -1} {
		if _, _, _, err := testAgent.QpmCheck("qpm_bad_period", period, 10, 1); err == nil {
			t.Fatalf("period=%d should fail", period)
		} else if !strings.Contains(err.Error(), "period") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

// 脚本未初始化时报错，避免空指针。
func TestQpmScriptNotInitialized(t *testing.T) {
	resetRedis(t)
	agent := &RedisLRAgent{}
	if _, _, _, err := agent.QpmCheck("qpm_no_script", 60, 10, 1); err == nil {
		t.Fatal("expected error when script is nil")
	}
}

// Redis 不可用时错误向上传播。
func TestQpmRedisError(t *testing.T) {
	resetRedis(t)
	if _, _, _, err := deadAgent.QpmCheck("qpm_dead", 60, 10, 1); err == nil {
		t.Fatal("expected redis error")
	}
}
