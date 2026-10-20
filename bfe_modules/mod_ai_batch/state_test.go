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

package mod_ai_batch

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_util/bns"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

const testRedisBns = "mod_ai_batch_test"

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

	dir, err := os.MkdirTemp("", "bfe-mod-ai-batch")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp dir failed: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)

	host, portStr, err := net.SplitHostPort(mr.Addr())
	if err != nil {
		fmt.Fprintf(os.Stderr, "split miniredis addr failed: %v\n", err)
		return 1
	}
	nameConf := filepath.Join(dir, "name_conf.data")
	content := fmt.Sprintf(`{
	"Version": "1",
	"Config": {"%s": [{"Host": "%s", "Port": %s, "Weight": 10}]}
}`, testRedisBns, host, portStr)
	if err := os.WriteFile(nameConf, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write name conf failed: %v\n", err)
		return 1
	}
	if err := bns.LoadLocalNameConf(nameConf); err != nil {
		fmt.Fprintf(os.Stderr, "load name conf failed: %v\n", err)
		return 1
	}

	return m.Run()
}

// newRedisModule 返回一个接了 miniredis 的模块实例。
func newRedisModule(t *testing.T) *ModuleAiBatch {
	t.Helper()

	testRedis.FlushAll()
	testRedis.SetTime(time.Unix(1700000000, 0))

	m := NewModuleAiBatch()
	client := redis_client.NewRedisClient(&redis_client.Options{
		ServiceConf:    testRedisBns,
		MaxIdle:        4,
		MaxActive:      16,
		Wait:           true,
		ConnTimeoutMs:  500,
		ReadTimeoutMs:  500,
		WriteTimeoutMs: 500,
	})
	if client == nil {
		t.Fatal("create redis client failed")
	}
	m.redisClient = client
	return m
}

// newNoRedisModule 返回一个未配置 Redis 的模块实例（fail-open 路径）。
func newNoRedisModule() *ModuleAiBatch {
	m := NewModuleAiBatch()
	m.redisClient = nil
	return m
}

// conf_load：合法配置可加载；ProductRulePath 为空、Redis.Bns 非法则报错。
func TestConfLoad(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s failed: %v", name, err)
		}
		return path
	}

	t.Run("Valid", func(t *testing.T) {
		confFile := write("ok.conf", "[basic]\nProductRulePath = mod_ai_batch.data\n\n[redis]\nBns =\n\n[log]\nOpenDebug = false\n")
		conf, err := ConfLoad(confFile, dir)
		if err != nil {
			t.Fatalf("ConfLoad failed: %v", err)
		}
		// ConfPathProc 用 path.Join（正斜杠）拼接，与 filepath.Join 在 Windows 上不同
		if want := path.Join(dir, "mod_ai_batch.data"); conf.Basic.ProductRulePath != want {
			t.Errorf("ProductRulePath = %q, want %q", conf.Basic.ProductRulePath, want)
		}
	})

	t.Run("EmptyProductRulePath", func(t *testing.T) {
		confFile := write("empty.conf", "[basic]\nProductRulePath =\n")
		if _, err := ConfLoad(confFile, dir); err == nil {
			t.Fatal("empty ProductRulePath should be rejected")
		}
	})

	t.Run("BadRedisBns", func(t *testing.T) {
		confFile := write("badredis.conf", "[basic]\nProductRulePath = mod_ai_batch.data\n\n[redis]\nBns = a,10\n")
		if _, err := ConfLoad(confFile, dir); err == nil {
			t.Fatal("malformed redis bns should be rejected")
		}
	})

	t.Run("MissingFile", func(t *testing.T) {
		if _, err := ConfLoad(filepath.Join(dir, "nope.conf"), dir); err == nil {
			t.Fatal("missing conf file should be rejected")
		}
	})
}

// BATCH_FILE 绑定：写入后可读回，并带 TTL。
func TestFileBindingRoundTrip(t *testing.T) {
	m := newRedisModule(t)
	const cluster, fileID = "cluster_a", "file_001"

	meta := map[string]string{
		"api_key_id": "ak_1",
		"key_name":   "key_a",
		"cluster":    cluster,
		"lines":      "12",
		"bytes":      "345",
		"purpose":    "batch",
		"filename":   "input.jsonl",
		"dir":        "uploads",
		"batch_id":   "batch_1",
		"lines_src":  "exact",
		"bytes_src":  "exact",
	}
	m.writeFileBinding(cluster, fileID, meta, defaultBatchStateTTL)

	if got := m.state.GetCounter("FILE_BIND_WRITE"); got != 1 {
		t.Errorf("FILE_BIND_WRITE = %d, want 1", got)
	}
	if ttl := testRedis.TTL(batchFileKey(cluster, fileID)); ttl <= 0 {
		t.Errorf("binding key should carry a TTL, got %v", ttl)
	}

	binding, err := m.readFileBinding(cluster, fileID)
	if err != nil {
		t.Fatalf("readFileBinding failed: %v", err)
	}
	if binding == nil {
		t.Fatal("binding should not be nil after write")
	}
	if binding.ApiKeyId != "ak_1" || binding.KeyName != "key_a" || binding.Cluster != cluster {
		t.Errorf("unexpected binding: %+v", binding)
	}
	if binding.Lines != 12 || binding.Bytes != 345 {
		t.Errorf("lines/bytes = %d/%d, want 12/345", binding.Lines, binding.Bytes)
	}
	if binding.Filename != "input.jsonl" || binding.BatchId != "batch_1" {
		t.Errorf("unexpected binding: %+v", binding)
	}
}

// 空 fileId 不写、不计数；未配置 Redis 时读取返回 errRedisDisabled，写入计数为失败。
func TestFileBindingEdgeCases(t *testing.T) {
	m := newRedisModule(t)
	m.writeFileBinding("cluster_a", "", map[string]string{"k": "v"}, 60)
	if got := m.state.GetCounter("FILE_BIND_WRITE"); got != 0 {
		t.Errorf("FILE_BIND_WRITE = %d, want 0 for empty fileId", got)
	}

	// 未命中
	if binding, err := m.readFileBinding("cluster_a", "not-exist"); err != nil || binding != nil {
		t.Errorf("readFileBinding on miss = (%v, %v), want (nil, nil)", binding, err)
	}

	// Redis 未配置
	noRedis := newNoRedisModule()
	if _, err := noRedis.readFileBinding("cluster_a", "x"); !errors.Is(err, errRedisDisabled) {
		t.Errorf("err = %v, want errRedisDisabled", err)
	}
	noRedis.writeFileBinding("cluster_a", "x", map[string]string{"k": "v"}, 60)
	if got := noRedis.state.GetCounter("FILE_BIND_WRITE_FAIL"); got != 1 {
		t.Errorf("FILE_BIND_WRITE_FAIL = %d, want 1 (fail-open with counter)", got)
	}
}

// BATCH_TASK 记录：写入后可读回，Reserved 以 "1" 表示。
func TestTaskRecordRoundTrip(t *testing.T) {
	m := newRedisModule(t)
	const batchID = "batch_002"

	m.writeTaskRecord(batchID, map[string]string{
		"api_key_id":     "ak_2",
		"cluster":        "cluster_b",
		"key_name":       "key_b",
		"input_file_id":  "file_in",
		"output_file_id": "file_out",
		"endpoint":       "/v1/chat/completions",
		"request_counts": "10,9,1",
		"est_lines":      "10",
		"reserved_units": "12345",
		"status":         "completed",
		"created_at":     "1700000000",
		"reserved":       "1",
	}, defaultBatchStateTTL)

	if got := m.state.GetCounter("TASK_WRITE"); got != 1 {
		t.Errorf("TASK_WRITE = %d, want 1", got)
	}

	task, err := m.readTaskRecord(batchID)
	if err != nil {
		t.Fatalf("readTaskRecord failed: %v", err)
	}
	if task == nil {
		t.Fatal("task should not be nil after write")
	}
	if task.ApiKeyId != "ak_2" || task.Status != "completed" {
		t.Errorf("unexpected task: %+v", task)
	}
	if task.EstLines != 10 || task.ReservedUnits != 12345 || task.CreatedAt != 1700000000 {
		t.Errorf("unexpected numeric fields: %+v", task)
	}
	if !task.Reserved {
		t.Error("Reserved should be true for value \"1\"")
	}

	// 空 batchId 不写
	m.writeTaskRecord("", map[string]string{"k": "v"}, 60)
	if got := m.state.GetCounter("TASK_WRITE"); got != 1 {
		t.Errorf("TASK_WRITE = %d, want 1 for empty batchId", got)
	}

	// 未命中
	if task, err := m.readTaskRecord("not-exist"); err != nil || task != nil {
		t.Errorf("readTaskRecord on miss = (%v, %v), want (nil, nil)", task, err)
	}
}

// batchContext 在请求上下文中的存取。
func TestBatchContextGetSet(t *testing.T) {
	req := &bfe_basic.Request{
		HttpRequest: &bfe_http.Request{Header: bfe_http.Header{}},
		Context:     make(map[interface{}]interface{}),
	}

	if ctx := getBatchContext(req); ctx != nil {
		t.Errorf("getBatchContext on a fresh request = %v, want nil", ctx)
	}

	ctx := &batchContext{op: bfe_basic.BatchOpUpload, fileId: "file_1"}
	setBatchContext(req, ctx)

	got := getBatchContext(req)
	if got == nil {
		t.Fatal("getBatchContext should return the stored context")
	}
	if got != ctx {
		t.Error("getBatchContext should return the same pointer")
	}
	if got.op != bfe_basic.BatchOpUpload || got.fileId != "file_1" {
		t.Errorf("unexpected context: %+v", got)
	}
}

// 全局硬上限与活跃批次数：无 Redis 时计数为 0（fail-open）。
func TestCeilingsAndActiveCount(t *testing.T) {
	m := newNoRedisModule()

	maxBytes, maxLines := BatchGlobalFileCeilings()
	if maxBytes != m.data.MaxFileBytes || maxLines != m.data.MaxFileLines {
		t.Errorf("ceilings = (%d, %d), want (%d, %d)",
			maxBytes, maxLines, m.data.MaxFileBytes, m.data.MaxFileLines)
	}

	if got := BatchActiveCount(""); got != 0 {
		t.Errorf("BatchActiveCount(\"\") = %d, want 0", got)
	}
	if got := BatchActiveCount("ak_without_redis"); got != 0 {
		t.Errorf("BatchActiveCount without redis = %d, want 0 (fail-open)", got)
	}
	if got := m.state.GetCounter("REDIS_ERR"); got < 1 {
		t.Errorf("REDIS_ERR = %d, want >= 1", got)
	}

	// 接上 Redis 后按窗口统计
	withRedis := newRedisModule(t)
	withRedis.activeAdd("ak_1", "batch_1", 60)
	withRedis.activeAdd("ak_1", "batch_2", 60)
	if got := withRedis.activeCount("ak_1", batchActiveWindowSec); got != 2 {
		t.Errorf("activeCount = %d, want 2", got)
	}
	withRedis.activeRemove("ak_1", "batch_1")
	if got := withRedis.activeCount("ak_1", batchActiveWindowSec); got != 1 {
		t.Errorf("activeCount after remove = %d, want 1", got)
	}
}

// mod_ai_batch.data 的默认值、校验与热加载。
func TestBatchDataConf(t *testing.T) {
	def := defaultBatchDataConf()
	if def.MaxFileBytes <= 0 || def.MaxFileLines <= 0 {
		t.Errorf("default ceilings should be positive: %+v", def)
	}
	if def.OwnerCheckMissPolicy != OwnerCheckMissAllowLog {
		t.Errorf("OwnerCheckMissPolicy = %q, want %q", def.OwnerCheckMissPolicy, OwnerCheckMissAllowLog)
	}

	// 校验：解析缓冲必须为正；策略只能取约定值（空值回填默认）
	if err := (&BatchDataConf{MaxLineParseBytes: 0}).check(); err == nil {
		t.Error("MaxLineParseBytes = 0 should be rejected")
	}
	if err := (&BatchDataConf{MaxLineParseBytes: 1024, OwnerCheckMissPolicy: "bogus"}).check(); err == nil {
		t.Error("unknown OwnerCheckMissPolicy should be rejected")
	}
	empty := &BatchDataConf{MaxLineParseBytes: 1024}
	if err := empty.check(); err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if empty.OwnerCheckMissPolicy != OwnerCheckMissAllowLog {
		t.Errorf("empty policy should default to %q, got %q", OwnerCheckMissAllowLog, empty.OwnerCheckMissPolicy)
	}
}

// loadBatchData：读取 data 文件并替换内存配置；异常输入返回错误且不改变实例。
func TestLoadBatchData(t *testing.T) {
	dir := t.TempDir()
	write := func(content string) string {
		t.Helper()
		path := filepath.Join(dir, fmt.Sprintf("data_%d.json", time.Now().UnixNano()))
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write data file failed: %v", err)
		}
		return path
	}

	m := newNoRedisModule()
	m.conf = &ConfModAiBatch{}

	path := write(`{"Version": "9.9", "MaxFileBytes": 123, "MaxFileLines": 456, "MaxLineParseBytes": 2048}`)
	m.conf.Basic.ProductRulePath = path

	msg, err := m.loadBatchData(url.Values{})
	if err != nil {
		t.Fatalf("loadBatchData failed: %v", err)
	}
	if msg != path+"=9.9" {
		t.Errorf("message = %q, want %q", msg, path+"=9.9")
	}
	if got := m.getData(); got.MaxFileBytes != 123 || got.MaxFileLines != 456 {
		t.Errorf("data not replaced: %+v", got)
	}

	// 非法 JSON
	m.conf.Basic.ProductRulePath = write(`{not-json}`)
	if _, err := m.loadBatchData(url.Values{}); err == nil {
		t.Error("bad json should be rejected")
	}

	// 校验失败
	m.conf.Basic.ProductRulePath = write(`{"MaxLineParseBytes": 0}`)
	if _, err := m.loadBatchData(url.Values{}); err == nil {
		t.Error("invalid data conf should be rejected")
	}

	// 文件不存在
	m.conf.Basic.ProductRulePath = filepath.Join(dir, "nope.json")
	if _, err := m.loadBatchData(url.Values{}); err == nil {
		t.Error("missing data file should be rejected")
	}
}
