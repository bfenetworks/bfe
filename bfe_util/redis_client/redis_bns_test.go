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
	"fmt"
	"strings"
	"testing"
)

// 单个 bns：权重默认 1。
func TestParseRedisBnsConfSingle(t *testing.T) {
	confs, err := ParseRedisBnsConf("redis_bns")
	if err != nil {
		t.Fatalf("ParseRedisBnsConf failed: %v", err)
	}
	if len(confs) != 1 {
		t.Fatalf("len(confs) = %d, want 1", len(confs))
	}
	if confs[0].bns != "redis_bns" {
		t.Errorf("bns = %q, want redis_bns", confs[0].bns)
	}
	if confs[0].weight != 1 {
		t.Errorf("weight = %d, want 1", confs[0].weight)
	}
}

// 多个 bns 带权重，格式为 "name,weight:N"，用 '|' 分隔。
func TestParseRedisBnsConfMulti(t *testing.T) {
	confs, err := ParseRedisBnsConf("redis_a,weight:10|redis_b,weight:20")
	if err != nil {
		t.Fatalf("ParseRedisBnsConf failed: %v", err)
	}
	if len(confs) != 2 {
		t.Fatalf("len(confs) = %d, want 2", len(confs))
	}
	if confs[0].bns != "redis_a" || confs[0].weight != 10 {
		t.Errorf("confs[0] = %+v, want {redis_a 10}", confs[0])
	}
	if confs[1].bns != "redis_b" || confs[1].weight != 20 {
		t.Errorf("confs[1] = %+v, want {redis_b 20}", confs[1])
	}
}

// 空格会被忽略。
func TestParseRedisBnsConfToleratesSpaces(t *testing.T) {
	confs, err := ParseRedisBnsConf(" redis_a , weight:10 | redis_b , weight:20 ")
	if err != nil {
		t.Fatalf("ParseRedisBnsConf failed: %v", err)
	}
	if len(confs) != 2 || confs[0].bns != "redis_a" || confs[1].bns != "redis_b" {
		t.Fatalf("unexpected parse result: %+v", confs)
	}
}

// 非法输入一律返回错误，不 panic。
func TestParseRedisBnsConfInvalid(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "Empty", input: "", wantErr: "empty"},
		{name: "MissingWeight", input: "redis_a,10", wantErr: "length err"},
		{name: "BadWeightFormat", input: "redis_a,weight:10:20", wantErr: "length err"},
		{name: "NoWeightKey", input: "redis_a,w:10", wantErr: "find no 'weight'"},
		{name: "NonNumericWeight", input: "redis_a,weight:abc", wantErr: "weight(abc) err"},
		{name: "WeightTooSmall", input: "redis_a,weight:0", wantErr: "weight should be"},
		{name: "WeightTooLarge", input: "redis_a,weight:101", wantErr: "weight should be"},
		{name: "BnsConflict", input: "redis_a,weight:10|redis_a,weight:10", wantErr: "conflict"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confs, err := ParseRedisBnsConf(tt.input)
			if err == nil {
				t.Fatalf("expected error for %q, got %+v", tt.input, confs)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// 权重和超限被拒绝。
func TestParseRedisBnsConfWeightSumOverLimit(t *testing.T) {
	count := MaxWeightSum/MaxWeightValue + 1
	parts := make([]string, 0, count)
	for i := 0; i < count; i++ {
		parts = append(parts, fmt.Sprintf("redis_%d,weight:%d", i, MaxWeightValue))
	}

	if _, err := ParseRedisBnsConf(strings.Join(parts, "|")); err == nil {
		t.Fatal("weight sum over limit should be rejected")
	} else if !strings.Contains(err.Error(), "weight sum overlimit") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Options.Format 解析 ServiceConf 并填充 clusterList。
func TestOptionsFormat(t *testing.T) {
	opts := &Options{ServiceConf: "redis_a,weight:10|redis_b,weight:20"}
	if err := opts.Format(); err != nil {
		t.Fatalf("Format failed: %v", err)
	}
	if len(opts.clusterList) != 2 {
		t.Errorf("clusterList = %+v, want 2 entries", opts.clusterList)
	}

	bad := &Options{ServiceConf: ""}
	if err := bad.Format(); err == nil {
		t.Error("Format should fail on empty ServiceConf")
	}
}

// CheckRedisConf 对合法/非法配置的判定。
func TestCheckRedisConf(t *testing.T) {
	if err := CheckRedisConf("redis_bns"); err != nil {
		t.Errorf("simple bns should be valid, got %v", err)
	}
	if err := CheckRedisConf("redis_a,weight:10|redis_b,weight:20"); err != nil {
		t.Errorf("multi bns should be valid, got %v", err)
	}
	if err := CheckRedisConf("redis_a,10"); err == nil {
		t.Error("malformed conf should be rejected")
	}
}

// getHash 用于把 key 映射到 redis cluster：同一 key 必须稳定落在同一槽位。
func TestGetHash(t *testing.T) {
	const base = 1024
	first := getHash([]byte("some-key"), base)
	for i := 0; i < 10; i++ {
		if got := getHash([]byte("some-key"), base); got != first {
			t.Fatalf("getHash is not stable: %d != %d", got, first)
		}
	}
	if first < 0 || first >= base {
		t.Errorf("getHash = %d, want in [0, %d)", first, base)
	}

	// nil value 走随机分支，但必须落在合法范围内（base=1 时恒为 0）
	if got := getHash(nil, 1); got != 0 {
		t.Errorf("getHash(nil, 1) = %d, want 0", got)
	}
}

// IsKeyNotFound 只对 redis.ErrNil 判真。
func TestIsKeyNotFound(t *testing.T) {
	resetRedis(t)
	c := newLiveClient(t)

	if _, err := c.Get("not-exist-key"); err == nil {
		t.Fatal("Get on missing key should return error")
	} else if !IsKeyNotFound(err) {
		t.Errorf("IsKeyNotFound(%v) should be true", err)
	}

	if err := c.Setex("exists-key", []byte("v"), 60); err != nil {
		t.Fatalf("Setex failed: %v", err)
	}
	if _, err := c.Get("exists-key"); err != nil {
		t.Errorf("Get on existing key failed: %v", err)
	}
	if IsKeyNotFound(nil) {
		t.Error("IsKeyNotFound(nil) should be false")
	}
}
