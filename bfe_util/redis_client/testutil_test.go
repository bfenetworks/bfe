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
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/bfenetworks/bfe/bfe_util/bns"
)

const (
	// liveBns 指向进程内 miniredis
	liveBns = "redis_client_test"
	// deadBns 指向已释放端口，用于验证连接失败路径
	deadBns = "redis_client_test_dead"
)

var testRedis *miniredis.Miniredis

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

	dir, err := os.MkdirTemp("", "bfe-redis-client")
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
	if err := bns.LoadLocalNameConf(nameConf); err != nil {
		fmt.Fprintf(os.Stderr, "load name conf failed: %v\n", err)
		return 1
	}

	return m.Run()
}

// writeNameConf 生成两条 bns 记录：一条指向 miniredis，一条指向已关闭的端口。
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
`, liveBns, host, port, deadBns, deadPort)

	path := filepath.Join(dir, "name_conf.data")
	return path, os.WriteFile(path, []byte(content), 0644)
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// newLiveClient 返回一个连到 miniredis 的客户端。
func newLiveClient(t *testing.T) Client {
	t.Helper()
	return newTestClient(t, liveBns, 500)
}

// newDeadClient 返回一个连不上任何服务的客户端。
func newDeadClient(t *testing.T) Client {
	t.Helper()
	return newTestClient(t, deadBns, 50)
}

func newTestClient(t *testing.T, serviceConf string, timeoutMs int) Client {
	t.Helper()

	c := NewRedisClient(&Options{
		ServiceConf:    serviceConf,
		MaxIdle:        4,
		MaxActive:      16,
		Wait:           true,
		ConnTimeoutMs:  timeoutMs,
		ReadTimeoutMs:  timeoutMs,
		WriteTimeoutMs: timeoutMs,
	})
	if c == nil {
		t.Fatalf("create redis client for %q failed", serviceConf)
	}
	return c
}

// resetRedis 清空 miniredis 数据，保证用例互不影响（本包测试串行）。
func resetRedis(t *testing.T) {
	t.Helper()
	testRedis.FlushAll()
}
