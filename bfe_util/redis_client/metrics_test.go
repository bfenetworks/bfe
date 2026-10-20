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

package redis_client

import (
	"testing"

	"github.com/bfenetworks/go-lib/web-monitor/delay_counter"
	"github.com/bfenetworks/go-lib/web-monitor/module_state2"
)

// 挂上 module_state2 后，各操作应对应地累加监控计数。
func TestModuleStateCounters(t *testing.T) {
	resetRedis(t)
	c := NewRedisClient2(liveBns, 4, 16, true, 500, 500, 500)

	state := &module_state2.State{}
	state.Init()
	c.SetModuleState2(state)

	if err := c.Setex("m1", []byte("v"), 60); err != nil {
		t.Fatalf("Setex failed: %v", err)
	}
	if _, err := c.Get("m1"); err != nil {
		t.Fatalf("Get hit failed: %v", err)
	}
	if _, err := c.Get("missing-metric-key"); err == nil {
		t.Fatal("Get on missing key should fail")
	}
	if _, err := c.Incr("m2"); err != nil {
		t.Fatalf("Incr failed: %v", err)
	}

	want := map[string]int64{
		RedisSetex:   1,
		RedisGet:     2,
		RedisGetHit:  1,
		RedisGetMiss: 1,
		RedisIncr:    1,
	}
	for key, expected := range want {
		if got := state.GetCounter(key); got != expected {
			t.Errorf("counter %s = %d, want %d", key, got, expected)
		}
	}
}

// 挂上延时统计后，操作应记录耗时且不 panic。
func TestDelayCounters(t *testing.T) {
	resetRedis(t)
	c := NewRedisClient2(liveBns, 4, 16, true, 500, 500, 500)

	delay := &delay_counter.DelayRecent{}
	delay.Init(1, 1, 5)
	connDelay := &delay_counter.DelayRecent{}
	connDelay.Init(1, 1, 5)

	c.SetDelay(delay)
	c.SetConnDelay(connDelay)

	if err := c.Setex("d1", []byte("v"), 60); err != nil {
		t.Fatalf("Setex failed: %v", err)
	}
	if _, err := c.Get("d1"); err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// 只要求不 panic；具体分桶结果依赖时钟，不做数值断言
	_ = delay.Get()
	_ = connDelay.Get()
}

// ActiveConnNum 反映连接池活跃连接数，空闲时为 0。
func TestActiveConnNum(t *testing.T) {
	resetRedis(t)
	c := NewRedisClient2(liveBns, 4, 16, true, 500, 500, 500)

	if err := c.Setex("a1", []byte("v"), 60); err != nil {
		t.Fatalf("Setex failed: %v", err)
	}

	if got := c.ActiveConnNum(); got < 0 {
		t.Errorf("ActiveConnNum = %d, want >= 0", got)
	}
}

// 旧式构造函数返回可用客户端。
func TestLegacyConstructors(t *testing.T) {
	resetRedis(t)

	c1 := NewRedisClient1(liveBns, 4, 500, 500, 500)
	if c1 == nil {
		t.Fatal("NewRedisClient1 returned nil")
	}
	if err := c1.Setex("legacy1", []byte("v"), 60); err != nil {
		t.Errorf("NewRedisClient1 client unusable: %v", err)
	}

	c2 := NewRedisClient2(liveBns, 4, 16, true, 500, 500, 500)
	if c2 == nil {
		t.Fatal("NewRedisClient2 returned nil")
	}
	if err := c2.Setex("legacy2", []byte("v"), 60); err != nil {
		t.Errorf("NewRedisClient2 client unusable: %v", err)
	}
}

// 非法 ServiceConf 使构造失败：返回的接口非 nil，但内部指针为 nil。
//
// 这是一个"typed nil"陷阱：NewRedisBnsClient 失败时返回 (*RedisClient)(nil)，
// NewRedisClient 把它装进 Client 接口后，调用方的 `if client == nil` 判断会失效，
// 继续调用方法就会空指针 panic。此处固化现状，见方案文档 §8.1 D6。
func TestNewRedisClientInvalidConf(t *testing.T) {
	tests := []struct {
		name string
		conf string
	}{
		{name: "EmptyConf", conf: ""},
		{name: "BadWeight", conf: "a,weight:abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewRedisClient(&Options{ServiceConf: tt.conf})
			if c == nil {
				t.Fatal("interface should be non-nil (see typed-nil note above)")
			}
			concrete, ok := c.(*RedisClient)
			if !ok {
				t.Fatalf("client type = %T, want *RedisClient", c)
			}
			if concrete != nil {
				t.Errorf("underlying *RedisClient = %v, want nil", concrete)
			}
		})
	}
}
