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

package bfe_route

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfFiles 在临时目录里落一套最小服务器配置，返回各文件路径。
// route 与 cluster 内容可按用例覆盖，便于构造校验失败的组合。
func writeConfFiles(t *testing.T, route, cluster string) (hostFile, vipFile, routeFile, clusterFile string) {
	t.Helper()

	dir := t.TempDir()
	write := func(name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s failed: %v", name, err)
		}
		return path
	}

	hostFile = write("host.data", `{
	"Version": "1",
	"Hosts": {"tag1": ["example.org"]},
	"HostTags": {"prod1": ["tag1"]}
}`)

	vipFile = write("vip.data", `{
	"Version": "1",
	"Vips": {}
}`)

	if route == "" {
		route = `{
	"Version": "1",
	"BasicRule": {"prod1": [{"Hostname": ["*"], "ClusterName": "cluster_test"}]}
}`
	}
	routeFile = write("route.data", route)

	if cluster == "" {
		cluster = `{
	"Version": "1",
	"Config": {
		"cluster_test": {
			"sub_test": [{"Name": "n0", "Addr": "127.0.0.1", "Port": 8080, "Weight": 10}]
		}
	}
}`
	}
	clusterFile = write("cluster_table.data", cluster)

	return
}

// 正常装载：路由表与集群表都可用。
func TestLoadServerDataConf(t *testing.T) {
	hostFile, vipFile, routeFile, clusterFile := writeConfFiles(t, "", "")

	conf, err := LoadServerDataConf(hostFile, vipFile, routeFile, clusterFile, nil)
	if err != nil {
		t.Fatalf("LoadServerDataConf failed: %v", err)
	}

	product, err := conf.HostTableLookup("example.org")
	if err != nil {
		t.Fatalf("HostTableLookup failed: %v", err)
	}
	if product != "prod1" {
		t.Errorf("product = %q, want prod1", product)
	}

	cluster, err := conf.ClusterTableLookup("cluster_test")
	if err != nil {
		t.Fatalf("ClusterTableLookup failed: %v", err)
	}
	if cluster.Name != "cluster_test" {
		t.Errorf("cluster name = %q, want cluster_test", cluster.Name)
	}

	if _, err := conf.ClusterTableLookup("no_such_cluster"); err == nil {
		t.Error("ClusterTableLookup should fail for an unknown cluster")
	}

	// 版本信息应被记录下来
	if versions := conf.HostTable.GetVersions(); versions.HostTag != "1" {
		t.Errorf("host tag version = %q, want 1", versions.HostTag)
	}
	if versions := conf.ClusterTable.GetVersions(); versions.ClusterConfVer != "1" {
		t.Errorf("cluster conf version = %q, want 1", versions.ClusterConfVer)
	}
}

// 校验：路由引用的集群必须存在于集群表。
func TestLoadServerDataConfMissingCluster(t *testing.T) {
	route := `{
	"Version": "1",
	"BasicRule": {"prod1": [{"Hostname": ["*"], "ClusterName": "cluster_missing"}]}
}`
	hostFile, vipFile, routeFile, clusterFile := writeConfFiles(t, route, "")

	_, err := LoadServerDataConf(hostFile, vipFile, routeFile, clusterFile, nil)
	if err == nil {
		t.Fatal("expected error for route referencing a missing cluster")
	}
	if !strings.Contains(err.Error(), "should exist in cluster_conf") {
		t.Errorf("unexpected error: %v", err)
	}
}

// 校验：路由里的产品必须存在于 host 表。
func TestLoadServerDataConfUnknownProduct(t *testing.T) {
	route := `{
	"Version": "1",
	"BasicRule": {"prod_unknown": [{"Hostname": ["*"], "ClusterName": "cluster_test"}]}
}`
	hostFile, vipFile, routeFile, clusterFile := writeConfFiles(t, route, "")

	_, err := LoadServerDataConf(hostFile, vipFile, routeFile, clusterFile, nil)
	if err == nil {
		t.Fatal("expected error for route product missing in host table")
	}
	if !strings.Contains(err.Error(), "should exist in host") {
		t.Errorf("unexpected error: %v", err)
	}
}

// 文件缺失或内容非法时返回错误，不 panic。
func TestLoadServerDataConfBadFiles(t *testing.T) {
	hostFile, vipFile, routeFile, clusterFile := writeConfFiles(t, "", "")
	missing := filepath.Join(t.TempDir(), "not-exist.data")

	tests := []struct {
		name                      string
		host, vip, route, cluster string
		wantErr                   string
	}{
		{
			name: "MissingHostFile", host: missing, vip: vipFile, route: routeFile,
			cluster: clusterFile, wantErr: "hostTableLoad",
		},
		{
			name: "MissingVipFile", host: hostFile, vip: missing, route: routeFile,
			cluster: clusterFile, wantErr: "hostTableLoad",
		},
		{
			name: "MissingRouteFile", host: hostFile, vip: vipFile, route: missing,
			cluster: clusterFile, wantErr: "hostTableLoad",
		},
		{
			name: "MissingClusterFile", host: hostFile, vip: vipFile, route: routeFile,
			cluster: missing, wantErr: "clusterTableLoad",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadServerDataConf(tt.host, tt.vip, tt.route, tt.cluster, nil)
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
