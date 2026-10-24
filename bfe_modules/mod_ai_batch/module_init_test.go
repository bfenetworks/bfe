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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_module"
)

// writeModuleConfTree 在 confRoot 下按 BFE 约定落一套模块配置。
func writeModuleConfTree(t *testing.T, confRoot, redisBns string) {
	t.Helper()

	modDir := filepath.Join(confRoot, ModAiBatch)
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir module dir failed: %v", err)
	}

	// ProductRulePath 相对 confRoot（与 conf/ 里的样例一致：../conf/mod_ai_batch/...）
	conf := "[basic]\nProductRulePath = mod_ai_batch/mod_ai_batch.data\n\n[redis]\nBns = " + redisBns +
		"\nConnectTimeout = 500\nReadTimeout = 500\nWriteTimeout = 500\nMaxIdle = 4\nMaxActive = 8\n\n[log]\nOpenDebug = false\n"
	if err := os.WriteFile(filepath.Join(modDir, ModAiBatch+".conf"), []byte(conf), 0644); err != nil {
		t.Fatalf("write module conf failed: %v", err)
	}

	data := `{"Version": "2.0", "MaxFileBytes": 1024, "MaxFileLines": 10, "MaxLineParseBytes": 4096}`
	if err := os.WriteFile(filepath.Join(modDir, ModAiBatch+".data"), []byte(data), 0644); err != nil {
		t.Fatalf("write module data failed: %v", err)
	}
}

// Init 装配配置、Redis 与回调；缺配置时报错。
func TestModuleInit(t *testing.T) {
	t.Run("WithoutRedis", func(t *testing.T) {
		confRoot := t.TempDir()
		writeModuleConfTree(t, confRoot, "")

		m := NewModuleAiBatch()
		if err := m.Init(bfe_module.NewBfeCallbacks(), web_monitor.NewWebHandlers(), confRoot); err != nil {
			t.Fatalf("Init failed: %v", err)
		}
		if m.redisClient != nil {
			t.Error("redisClient should stay nil when Redis.Bns is empty")
		}
		if got := m.Name(); got != ModAiBatch {
			t.Errorf("Name = %q, want %q", got, ModAiBatch)
		}
		// data 文件已加载
		if got := m.getData(); got.MaxFileBytes != 1024 || got.Version != "2.0" {
			t.Errorf("data not loaded: %+v", got)
		}

		// 监控输出可序列化
		raw, err := m.getState(nil)
		if err != nil {
			t.Fatalf("getState failed: %v", err)
		}
		if len(raw) == 0 {
			t.Error("getState returned empty output")
		}
		if m.getStateDiff() == nil {
			t.Error("getStateDiff should not be nil")
		}
	})

	t.Run("WithRedis", func(t *testing.T) {
		confRoot := t.TempDir()
		writeModuleConfTree(t, confRoot, testRedisBns)

		m := NewModuleAiBatch()
		if err := m.Init(bfe_module.NewBfeCallbacks(), web_monitor.NewWebHandlers(), confRoot); err != nil {
			t.Fatalf("Init failed: %v", err)
		}
		if m.redisClient == nil {
			t.Fatal("redisClient should be created when Redis.Bns is set")
		}
	})

	t.Run("MissingConf", func(t *testing.T) {
		m := NewModuleAiBatch()
		err := m.Init(bfe_module.NewBfeCallbacks(), web_monitor.NewWebHandlers(), t.TempDir())
		if err == nil {
			t.Fatal("Init should fail without a module conf file")
		}
		if !strings.Contains(err.Error(), ModAiBatch) {
			t.Errorf("error = %q, want it to mention the module name", err.Error())
		}
	})
}

// 结算抢占：同一批次只有第一次能拿到结算权。
func TestTryMarkSettled(t *testing.T) {
	m := newRedisModule(t)
	const batchID = "batch_settle_1"

	if !m.tryMarkSettled(batchID, defaultBatchStateTTL) {
		t.Fatal("first claim should win")
	}
	if m.tryMarkSettled(batchID, defaultBatchStateTTL) {
		t.Error("second claim should lose (already settled)")
	}

	// 空 id 直接失败
	if m.tryMarkSettled("", defaultBatchStateTTL) {
		t.Error("empty batchId should not be claimed")
	}

	// Redis 未配置时 fail-closed：不抢占，并计数
	noRedis := newNoRedisModule()
	if noRedis.tryMarkSettled("batch_x", 60) {
		t.Error("claim should fail when redis is not configured")
	}
	if got := noRedis.state.GetCounter("REDIS_ERR"); got < 1 {
		t.Errorf("REDIS_ERR = %d, want >= 1", got)
	}

	// 键名带前缀，便于运维检索
	keys := testRedis.Keys()
	found := false
	for _, k := range keys {
		if k == batchSettledKey(batchID) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("settled key %q not found in %v", batchSettledKey(batchID), keys)
	}
}

// 监控输出可被 JSON 解析（供 /monitor 接口直接消费）。
func TestModuleStateOutputIsJSON(t *testing.T) {
	m := newNoRedisModule()
	m.state.Inc("FILE_BIND_WRITE", 3)

	raw, err := m.getState(nil)
	if err != nil {
		t.Fatalf("getState failed: %v", err)
	}

	var decoded struct {
		SCounters map[string]int64
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("getState output is not valid JSON: %v (raw=%s)", err, raw)
	}
	if got, ok := decoded.SCounters["FILE_BIND_WRITE"]; !ok || got != 3 {
		t.Errorf("SCounters[FILE_BIND_WRITE] = %d (present=%v), want 3; raw=%s", got, ok, raw)
	}
}