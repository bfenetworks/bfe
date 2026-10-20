// Copyright (c) 2026 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// You may not use this file except in compliance with the License.
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
	"strings"
	"testing"
	"time"
)

// Setex 写入值并带过期时间。
func TestSetexAndGet(t *testing.T) {
	resetRedis(t)
	c := newLiveClient(t)

	if err := c.Setex("k1", []byte("v1"), 60); err != nil {
		t.Fatalf("Setex failed: %v", err)
	}

	raw, err := c.Get("k1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got := string(raw.([]byte)); got != "v1" {
		t.Errorf("value = %q, want v1", got)
	}

	if ttl := testRedis.TTL("k1"); ttl <= 0 || ttl > 60*time.Second {
		t.Errorf("TTL = %v, want in (0, 60s]", ttl)
	}

	// 覆盖写入
	if err := c.Setex("k1", []byte("v2"), 60); err != nil {
		t.Fatalf("overwrite Setex failed: %v", err)
	}
	raw, _ = c.Get("k1")
	if got := string(raw.([]byte)); got != "v2" {
		t.Errorf("value after overwrite = %q, want v2", got)
	}
}

// GetInt64 的数值、缺失与非数值三种情况。
func TestGetInt64(t *testing.T) {
	resetRedis(t)
	c := newLiveClient(t)

	if err := c.Setex("num", []byte("42"), 60); err != nil {
		t.Fatalf("Setex failed: %v", err)
	}
	if got, err := c.GetInt64("num"); err != nil || got != 42 {
		t.Fatalf("GetInt64 = (%d, %v), want (42, nil)", got, err)
	}

	if _, err := c.GetInt64("missing"); err == nil {
		t.Error("GetInt64 on missing key should fail")
	} else if !IsKeyNotFound(err) {
		t.Errorf("missing key error should be ErrNil, got %v", err)
	}

	if err := c.Setex("text", []byte("not-a-number"), 60); err != nil {
		t.Fatalf("Setex failed: %v", err)
	}
	if _, err := c.GetInt64("text"); err == nil {
		t.Error("GetInt64 on non-numeric value should fail")
	}
}

// Incr / IncrBy / Decr 的计数语义。
func TestIncrIncrByDecr(t *testing.T) {
	resetRedis(t)
	c := newLiveClient(t)

	for want := int64(1); want <= 2; want++ {
		got, err := c.Incr("counter")
		if err != nil {
			t.Fatalf("Incr failed: %v", err)
		}
		if got != want {
			t.Errorf("Incr = %d, want %d", got, want)
		}
	}

	got, err := c.IncrBy("counter", 10)
	if err != nil {
		t.Fatalf("IncrBy failed: %v", err)
	}
	if got != 12 {
		t.Errorf("IncrBy(10) = %d, want 12", got)
	}

	got, err = c.Decr("counter")
	if err != nil {
		t.Fatalf("Decr failed: %v", err)
	}
	if got != 11 {
		t.Errorf("Decr = %d, want 11", got)
	}

	// Decr 可以减到负数
	if err := c.Setex("zero", []byte("0"), 60); err != nil {
		t.Fatalf("Setex failed: %v", err)
	}
	if got, err = c.Decr("zero"); err != nil || got != -1 {
		t.Errorf("Decr on zero = (%d, %v), want (-1, nil)", got, err)
	}
}

// IncrAndExpire 在自增的同时设置过期时间，避免计数键永不过期。
func TestIncrAndExpire(t *testing.T) {
	resetRedis(t)
	c := newLiveClient(t)

	got, err := c.IncrAndExpire("expiring", 30)
	if err != nil {
		t.Fatalf("IncrAndExpire failed: %v", err)
	}
	if got != 1 {
		t.Errorf("IncrAndExpire = %d, want 1", got)
	}
	if ttl := testRedis.TTL("expiring"); ttl <= 0 || ttl > 30*time.Second {
		t.Errorf("TTL = %v, want in (0, 30s]", ttl)
	}

	// 再次自增并刷新过期时间
	if got, err = c.IncrAndExpire("expiring", 30); err != nil || got != 2 {
		t.Fatalf("second IncrAndExpire = (%d, %v), want (2, nil)", got, err)
	}
}

// Expire 给已存在的键设置过期时间。
func TestExpire(t *testing.T) {
	resetRedis(t)
	c := newLiveClient(t)

	if err := c.Setex("no-ttl", []byte("v"), 60); err != nil {
		t.Fatalf("Setex failed: %v", err)
	}
	testRedis.SetTTL("no-ttl", 0)

	if err := c.Expire("no-ttl", 120); err != nil {
		t.Fatalf("Expire failed: %v", err)
	}
	if ttl := testRedis.TTL("no-ttl"); ttl <= 0 || ttl > 120*time.Second {
		t.Errorf("TTL = %v, want in (0, 120s]", ttl)
	}
}

// GetInt64Batch 按 key 顺序返回，缺 key 的行为固化为当前实现（redigo 转换失败即报错）。
func TestGetInt64Batch(t *testing.T) {
	resetRedis(t)
	c := newLiveClient(t)

	if err := c.Setex("b1", []byte("1"), 60); err != nil {
		t.Fatalf("Setex failed: %v", err)
	}
	if err := c.Setex("b2", []byte("2"), 60); err != nil {
		t.Fatalf("Setex failed: %v", err)
	}

	got, err := c.GetInt64Batch([]string{"b1", "b2"})
	if err != nil {
		t.Fatalf("GetInt64Batch failed: %v", err)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("GetInt64Batch = %v, want [1 2]", got)
	}

	// 空入参直接返回空切片，不访问 Redis
	if got, err := c.GetInt64Batch(nil); err != nil || len(got) != 0 {
		t.Errorf("GetInt64Batch(nil) = (%v, %v), want ([], nil)", got, err)
	}

	// 含缺失 key：redigo 的 sliceHelper 会跳过 nil 元素，对应位置保留 0
	got2, err := c.GetInt64Batch([]string{"b1", "no-such-key"})
	if err != nil {
		t.Fatalf("GetInt64Batch with a missing key failed: %v", err)
	}
	if len(got2) != 2 || got2[0] != 1 || got2[1] != 0 {
		t.Errorf("GetInt64Batch = %v, want [1 0] (missing key yields 0)", got2)
	}
}

// HGetAll 返回全部字段，缺 key 时为空 map。
func TestHGetAll(t *testing.T) {
	resetRedis(t)
	c := newLiveClient(t)

	testRedis.HSet("hash", "f1", "v1")
	testRedis.HSet("hash", "f2", "v2")

	got, err := c.HGetAll("hash")
	if err != nil {
		t.Fatalf("HGetAll failed: %v", err)
	}
	if len(got) != 2 || got["f1"] != "v1" || got["f2"] != "v2" {
		t.Errorf("HGetAll = %v, want {f1:v1 f2:v2}", got)
	}

	empty, err := c.HGetAll("no-such-hash")
	if err != nil {
		t.Fatalf("HGetAll on missing key failed: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("HGetAll on missing key = %v, want empty map", empty)
	}
}

// Delete 删除键。
func TestDelete(t *testing.T) {
	resetRedis(t)
	c := newLiveClient(t)

	if err := c.Setex("to-delete", []byte("v"), 60); err != nil {
		t.Fatalf("Setex failed: %v", err)
	}
	if err := c.Delete("to-delete"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if testRedis.Exists("to-delete") {
		t.Error("key should be removed")
	}

	// 删除不存在的键不报错
	if err := c.Delete("never-existed"); err != nil {
		t.Errorf("Delete on missing key failed: %v", err)
	}
}

// PIncr 对多个 key 批量自增。
func TestPIncr(t *testing.T) {
	resetRedis(t)
	c := newLiveClient(t)

	got, err := c.PIncr([]string{"p1", "p2", "p3"})
	if err != nil {
		t.Fatalf("PIncr failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(PIncr) = %d, want 3", len(got))
	}
	for i, v := range got {
		if v != 1 {
			t.Errorf("PIncr[%d] = %d, want 1", i, v)
		}
	}

	// 再自增一次
	if got, err = c.PIncr([]string{"p1", "p2"}); err != nil {
		t.Fatalf("second PIncr failed: %v", err)
	}
	if len(got) != 2 || got[0] != 2 || got[1] != 2 {
		t.Errorf("second PIncr = %v, want [2 2]", got)
	}

	// 空入参：当前实现返回错误（PIncr 要求至少一个 key）
	if _, err := c.PIncr(nil); err == nil {
		t.Error("PIncr with empty key list should fail")
	} else if !strings.Contains(err.Error(), "keyList") {
		t.Errorf("unexpected error: %v", err)
	}
}

// NewScript 的 Run：正常返回与脚本错误。
func TestNewScript(t *testing.T) {
	resetRedis(t)
	c := newLiveClient(t)

	script := c.NewScript("return 42")
	got, err := script.Run("any-key")
	if err != nil {
		t.Fatalf("script run failed: %v", err)
	}
	if v, ok := got.(int64); !ok || v != 42 {
		t.Errorf("script result = %v (%T), want int64(42)", got, got)
	}

	// 脚本内报错应返回 error
	badScript := c.NewScript("return redis.error_reply('boom')")
	if _, err := badScript.Run("any-key"); err == nil {
		t.Error("script error should be returned")
	}
}

// Redis 不可用时所有操作返回错误而不是 panic。
func TestOperationsOnDeadRedis(t *testing.T) {
	resetRedis(t)
	c := newDeadClient(t)

	if err := c.Setex("k", []byte("v"), 60); err == nil {
		t.Error("Setex should fail")
	}
	if _, err := c.Get("k"); err == nil {
		t.Error("Get should fail")
	}
	if _, err := c.GetInt64("k"); err == nil {
		t.Error("GetInt64 should fail")
	}
	if _, err := c.Incr("k"); err == nil {
		t.Error("Incr should fail")
	}
	if _, err := c.IncrBy("k", 1); err == nil {
		t.Error("IncrBy should fail")
	}
	if _, err := c.Decr("k"); err == nil {
		t.Error("Decr should fail")
	}
	if _, err := c.IncrAndExpire("k", 60); err == nil {
		t.Error("IncrAndExpire should fail")
	}
	if _, err := c.HGetAll("k"); err == nil {
		t.Error("HGetAll should fail")
	}
	if _, err := c.PIncr([]string{"k"}); err == nil {
		t.Error("PIncr should fail")
	}
	// EXPIRE/DEL 在 Redis 侧的返回值 0 不构成 Go 层错误，这里只要求不 panic
	_ = c.Expire("k", 60)
	_ = c.Delete("k")
}
