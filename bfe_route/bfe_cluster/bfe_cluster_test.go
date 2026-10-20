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

package bfe_cluster

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
)

// loadCluster 从临时配置文件装载指定集群，走的是生产同一条 ClusterConfLoad 路径。
func loadCluster(t *testing.T, name, content string) *BfeCluster {
	t.Helper()

	path := filepath.Join(t.TempDir(), "cluster_conf.data")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write cluster conf failed: %v", err)
	}

	conf, err := cluster_conf.ClusterConfLoad(path, nil)
	if err != nil {
		t.Fatalf("ClusterConfLoad failed: %v", err)
	}

	clusterConf, ok := (*conf.Config)[name]
	if !ok {
		t.Fatalf("cluster %q not found in loaded conf", name)
	}

	cluster := NewBfeCluster(name)
	cluster.BasicInit(clusterConf)
	return cluster
}

// 默认配置：未显式设置的字段应取默认值。
func TestBfeClusterDefaults(t *testing.T) {
	cluster := loadCluster(t, "c1", `{
	"Version": "1",
	"Config": {"c1": {}}
}`)

	if got := cluster.Name; got != "c1" {
		t.Errorf("Name = %q, want c1", got)
	}

	// 默认超时（毫秒 -> Duration）
	if got, want := cluster.TimeoutReadClient(), time.Duration(cluster_conf.DefaultReadClientTimeout)*time.Millisecond; got != want {
		t.Errorf("TimeoutReadClient = %v, want %v", got, want)
	}
	if got, want := cluster.TimeoutWriteClient(), time.Duration(cluster_conf.DefaultWriteClientTimeout)*time.Millisecond; got != want {
		t.Errorf("TimeoutWriteClient = %v, want %v", got, want)
	}
	if got, want := cluster.TimeoutReadClientAgain(), time.Duration(cluster_conf.DefaultReadClientAgainTimeout)*time.Millisecond; got != want {
		t.Errorf("TimeoutReadClientAgain = %v, want %v", got, want)
	}

	backendConf := cluster.BackendConf()
	if backendConf == nil || backendConf.Protocol == nil {
		t.Fatalf("BackendConf = %+v, want non-nil with Protocol", backendConf)
	}
	if *backendConf.Protocol != "http" {
		t.Errorf("Protocol = %q, want http", *backendConf.Protocol)
	}
	if cluster.TimeoutConnSrv() <= 0 {
		t.Errorf("TimeoutConnSrv = %d, want > 0", cluster.TimeoutConnSrv())
	}
	if cluster.RetryLevel() < 0 {
		t.Errorf("RetryLevel = %d, want >= 0", cluster.RetryLevel())
	}

	// 集群级开关默认值
	if cluster.DisableHostHeader {
		t.Error("DisableHostHeader should default to false")
	}
	if cluster.CancelOnClientClose() == false && cluster.DefaultSSEFlushInterval() < 0 {
		t.Errorf("unexpected SSE flush interval: %v", cluster.DefaultSSEFlushInterval())
	}
	// 未配置 https 段的集群返回 nil，调用方（createTransport）只在 protocol=https 时才取用
	if cluster.BackendHTTPSConf() != nil {
		t.Error("BackendHTTPSConf should be nil when no https conf is set")
	}
}

// 显式配置：覆盖值应生效。
func TestBfeClusterOverrides(t *testing.T) {
	cluster := loadCluster(t, "c2", `{
	"Version": "1",
	"Config": {
		"c2": {
			"ClusterBasic": {
				"TimeoutReadClient": 1000,
				"TimeoutWriteClient": 2000,
				"TimeoutReadClientAgain": 3000,
				"DisableHostHeader": true,
				"DisableHealthCheck": true
			},
			"BackendConf": {
				"Protocol": "https",
				"TimeoutResponseHeader": 500,
				"MaxIdleConnsPerHost": 0,
				"RetryLevel": 2
			}
		}
	}
}`)

	if got, want := cluster.TimeoutReadClient(), 1000*time.Millisecond; got != want {
		t.Errorf("TimeoutReadClient = %v, want %v", got, want)
	}
	if got, want := cluster.TimeoutWriteClient(), 2000*time.Millisecond; got != want {
		t.Errorf("TimeoutWriteClient = %v, want %v", got, want)
	}
	if got, want := cluster.TimeoutReadClientAgain(), 3000*time.Millisecond; got != want {
		t.Errorf("TimeoutReadClientAgain = %v, want %v", got, want)
	}
	if !cluster.DisableHostHeader {
		t.Error("DisableHostHeader should be true")
	}
	if !cluster.DisableHealthCheck {
		t.Error("DisableHealthCheck should be true")
	}

	backendConf := cluster.BackendConf()
	if *backendConf.Protocol != "https" {
		t.Errorf("Protocol = %q, want https", *backendConf.Protocol)
	}
	if backendConf.TimeoutResponseHeader == nil || *backendConf.TimeoutResponseHeader != 500 {
		t.Errorf("TimeoutResponseHeader = %v, want 500", backendConf.TimeoutResponseHeader)
	}
	if cluster.RetryLevel() != 2 {
		t.Errorf("RetryLevel = %d, want 2", cluster.RetryLevel())
	}
}

// 非法配置在装载阶段就被拒绝。
func TestBfeClusterInvalidConf(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "NoVersion",
			content: `{"Config": {"c3": {}}}`,
		},
		{
			name:    "BadProtocol",
			content: `{"Version": "1", "Config": {"c3": {"BackendConf": {"Protocol": "bogus"}}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cluster_conf.data")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("write cluster conf failed: %v", err)
			}
			if _, err := cluster_conf.ClusterConfLoad(path, nil); err == nil {
				t.Fatalf("%s should be rejected", tt.name)
			}
		})
	}
}
