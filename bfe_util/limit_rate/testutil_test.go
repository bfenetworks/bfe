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
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/bfenetworks/bfe/bfe_util/bns"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

const (
	// testRedisBns 指向进程内 miniredis
	testRedisBns = "limit_rate_test"
	// deadRedisBns 指向一个已释放的端口，用于验证故障传播
	deadRedisBns = "limit_rate_test_dead"
)

// testBaseTime 是固定的时间基准。所有用例都基于它拨动时钟，
// 不依赖真实时间，避免 flaky。
var testBaseTime = time.Unix(1700000000, 0)

var (
	testRedis *miniredis.Miniredis
	testAgent *RedisLRAgent
	deadAgent *RedisLRAgent
)

func TestMain(m *testing.M) {
	os.Exit(runMain(m))
}

func runMain(m *testing.M) int {
	mr, err := miniredis.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start miniredis failed: %v\n", err)
		return 1
	}
	defer mr.Close()
	testRedis = mr
	testRedis.SetTime(testBaseTime)

	dir, err := os.MkdirTemp("", "bfe-limit-rate")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp dir failed: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)

	nameConf, err := writeNameConf(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write name conf failed: %v\n", err)
		return 1
	}
	// bns 的本地名字表是包级全局，只在这里加载一次
	if err := bns.LoadLocalNameConf(nameConf); err != nil {
		fmt.Fprintf(os.Stderr, "load name conf failed: %v\n", err)
		return 1
	}

	testAgent = NewRedisLRAgent(newTestClient(testRedisBns, 500))
	deadAgent = NewRedisLRAgent(newTestClient(deadRedisBns, 50))
	if testAgent == nil || deadAgent == nil {
		fmt.Fprintln(os.Stderr, "create redis agent failed")
		return 1
	}

	return m.Run()
}

// writeNameConf 生成指向 miniredis 与一个已关闭端口的 name_conf。
func writeNameConf(dir string) (string, error) {
	host, portStr, err := net.SplitHostPort(testRedis.Addr())
	if err != nil {
		return "", err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", err
	}

	deadPort, err := freePort()
	if err != nil {
		return "", err
	}

	content := fmt.Sprintf(`{
	"Version": "1",
	"Config": {
		"%s": [{"Host": "%s", "Port": %d, "Weight": 10}],
		"%s": [{"Host": "127.0.0.1", "Port": %d, "Weight": 10}]
	}
}
`, testRedisBns, host, port, deadRedisBns, deadPort)

	path := filepath.Join(dir, "name_conf.data")
	return path, os.WriteFile(path, []byte(content), 0644)
}

// freePort 返回一个当前空闲的端口；关闭监听后该端口不再可连。
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func newTestClient(bnsName string, timeoutMs int) redis_client.Client {
	return redis_client.NewRedisClient(&redis_client.Options{
		ServiceConf:    bnsName,
		MaxIdle:        4,
		MaxActive:      16,
		Wait:           true,
		ConnTimeoutMs:  timeoutMs,
		ReadTimeoutMs:  timeoutMs,
		WriteTimeoutMs: timeoutMs,
	})
}

// resetRedis 清空数据并把时钟拨回基准，保证用例之间互不影响。
// 注意：miniredis 是包级共享的，本包测试必须串行，不能使用 t.Parallel。
func resetRedis(t *testing.T) {
	t.Helper()
	testRedis.FlushAll()
	testRedis.SetTime(testBaseTime)
}

// tick 把逻辑时钟推进到「基准时间 + d」。脚本里的 TIME 会读到该时钟。
func tick(d time.Duration) {
	testRedis.SetTime(testBaseTime.Add(d))
}

// redisInt 读取计数器类键的值，键不存在时返回 0。
func redisInt(t *testing.T, key string) int64 {
	t.Helper()
	s, err := testRedis.Get(key)
	if err != nil {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatalf("value of %s = %q is not an integer", key, s)
	}
	return v
}
