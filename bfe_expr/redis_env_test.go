// Copyright (c) 2025 The BFE Authors.
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

/* redis_env_test.go - tests for redis access in expressions */

package bfe_expr

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/bfenetworks/bfe/bfe_util/bns"
)

// testRedisBns 指向进程内 miniredis 的 bns 名
const testRedisBns = "bfe_expr_test_redis"

var testRedis *miniredis.Miniredis

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	mr, err := miniredis.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start miniredis failed: %v\n", err)
		return 1
	}
	defer mr.Close()
	testRedis = mr

	dir, err := os.MkdirTemp("", "bfe-expr-redis")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp dir failed: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)

	nameConf, err := writeRedisNameConf(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write name conf failed: %v\n", err)
		return 1
	}
	if err := bns.LoadLocalNameConf(nameConf); err != nil {
		fmt.Fprintf(os.Stderr, "load name conf failed: %v\n", err)
		return 1
	}

	return m.Run()
}

// writeRedisNameConf 生成指向 miniredis 的本地 bns 记录
func writeRedisNameConf(dir string) (string, error) {
	host, portStr, err := net.SplitHostPort(testRedis.Addr())
	if err != nil {
		return "", err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", err
	}

	content := fmt.Sprintf(`{
	"Version": "1",
	"Config": {
		"%s": [{"Host": "%s", "Port": %d, "Weight": 10}]
	}
}
`, testRedisBns, host, port)

	path := filepath.Join(dir, "name_conf.data")
	return path, os.WriteFile(path, []byte(content), 0644)
}

func newTestRedisClient(t *testing.T) *RedisClient {
	t.Helper()
	return (&RedisEnv{}).GetRedisClientWithOpts(testRedisBns, "", 500, 500, 500)
}

// 读操作：命中/未命中/整数/Hash
func TestRedisClientRead(t *testing.T) {
	testRedis.FlushAll()
	if err := testRedis.Set("s", "hello"); err != nil {
		t.Fatalf("miniredis Set: %v", err)
	}
	if err := testRedis.Set("n", "42"); err != nil {
		t.Fatalf("miniredis Set: %v", err)
	}
	testRedis.HSet("h", "f1", "v1")

	c := newTestRedisClient(t)

	if got := c.Get("s"); got != "hello" {
		t.Errorf("Get(s) = %q, want hello", got)
	}
	if got := c.Get("missing"); got != "" {
		t.Errorf("Get(missing) = %q, want empty", got)
	}
	if got := c.GetInt("n"); got != 42 {
		t.Errorf("GetInt(n) = %d, want 42", got)
	}
	if got := c.GetInt("missing"); got != 0 {
		t.Errorf("GetInt(missing) = %d, want 0", got)
	}

	m := c.HGetAll("h")
	if len(m) != 1 || m["f1"] != "v1" {
		t.Errorf("HGetAll(h) = %v, want map[f1:v1]", m)
	}
	if m = c.HGetAll("missing"); len(m) != 0 {
		t.Errorf("HGetAll(missing) = %v, want empty", m)
	}
}

// nil/空句柄安全：返回零值
func TestRedisClientNilSafe(t *testing.T) {
	var c *RedisClient
	if c.Get("k") != "" {
		t.Error("nil client Get should return empty")
	}
	if c.GetInt("k") != 0 {
		t.Error("nil client GetInt should return 0")
	}
	if c.HGetAll("k") != nil {
		t.Error("nil client HGetAll should return nil")
	}

	e := &RedisClient{}
	if e.Get("k") != "" || e.GetInt("k") != 0 || e.HGetAll("k") != nil {
		t.Error("empty client should return zero values")
	}
}

// 连接不可用（非法 serviceConf）：返回不可用句柄，调用返回零值
func TestRedisClientUnavailable(t *testing.T) {
	c := (&RedisEnv{}).GetRedisClientWithOpts("", "", 100, 100, 100)
	if c == nil {
		t.Fatal("client handle should not be nil")
	}
	if c.Get("k") != "" || c.GetInt("k") != 0 || c.HGetAll("k") != nil {
		t.Error("unavailable client should return zero values")
	}
}

// 单例：相同 地址+参数 返回同一实例，参数不同则不同实例
func TestGetRedisClientSingleton(t *testing.T) {
	e := &RedisEnv{}

	c1 := e.GetRedisClient(testRedisBns)
	c2 := e.GetRedisClient(testRedisBns)
	if c1 != c2 {
		t.Error("same addr should reuse client singleton")
	}

	c3 := e.GetRedisClientWithOpts(testRedisBns, "", 501, 501, 501)
	if c3 == c1 {
		t.Error("different opts should create new client")
	}
}

// 表达式中链式调用：GetRedisClient(addr).Get(key)
func TestRedisEnvInExpression(t *testing.T) {
	testRedis.FlushAll()
	if err := testRedis.Set("expr-key", "expr-value"); err != nil {
		t.Fatalf("miniredis Set: %v", err)
	}

	p, err := CompileParam(fmt.Sprintf(`=GetRedisClient(%q).Get("expr-key")`, testRedisBns))
	if err != nil {
		t.Fatalf("CompileParam: %v", err)
	}

	v, err := p.Eval(newTestRequest())
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if v != "expr-value" {
		t.Errorf("Eval = %q, want expr-value", v)
	}
}
