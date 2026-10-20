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
	"errors"
	"net"
	"net/url"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_route_conf/host_rule_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_route_conf/route_rule_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_route_conf/vip_rule_conf"
	"github.com/bfenetworks/bfe/bfe_http"
)

func strPtr(s string) *string { return &s }

// buildRouteConf 用内存结构构造路由配置：basic 规则在这里被编译成前缀树，
// advanced 规则里的条件字符串在这里被解析，与配置装载走同一条 Convert 路径。
func buildRouteConf(t *testing.T,
	basic map[string][]route_rule_conf.BasicRouteRuleFile,
	advanced map[string][]route_rule_conf.AdvancedRouteRuleFile) *route_rule_conf.RouteTableConf {
	t.Helper()

	convertedBasic := make(route_rule_conf.ProductBasicRouteRuleFile, len(basic))
	for product, rules := range basic {
		convertedBasic[product] = rules
	}

	fileConf := &route_rule_conf.RouteTableFile{Version: strPtr("1")}
	if basic != nil {
		fileConf.BasicRule = &convertedBasic
	}
	if advanced != nil {
		advFile := make(route_rule_conf.ProductAdvancedRouteRuleFile, len(advanced))
		for product, rules := range advanced {
			advFile[product] = rules
		}
		fileConf.ProductRule = &advFile
	}

	conf, err := route_rule_conf.Convert(fileConf)
	if err != nil {
		t.Fatalf("convert route conf failed: %v", err)
	}
	return conf
}

// newTestHostTable 组装一张路由表。hosts 是 hostname -> hostTag，
// products 是 hostTag -> product。
func newTestHostTable(t *testing.T, hostTagMap host_rule_conf.HostTag2Product,
	hosts host_rule_conf.Host2HostTag, defaultProduct string,
	vipMap vip_rule_conf.Vip2Product, route *route_rule_conf.RouteTableConf) *HostTable {
	t.Helper()

	table := newHostTable()
	table.Update(
		host_rule_conf.HostConf{
			Version:        "1",
			DefaultProduct: defaultProduct,
			HostMap:        hosts,
			HostTagMap:     hostTagMap,
		},
		vip_rule_conf.VipConf{Version: "1", VipMap: vipMap},
		route,
	)
	return table
}

// newRouteRequest 构造只带 Host/Path 的路由查找请求。
func newRouteRequest(host, path string) *bfe_basic.Request {
	return &bfe_basic.Request{
		HttpRequest: &bfe_http.Request{
			Host: host,
			URL:  &url.URL{Path: path},
		},
		Session: new(bfe_basic.Session),
	}
}

// basicHostTable 是多数用例的公共配置：
// example.org -> tag1 -> prod1，默认产品 def_prod，cluster_test 走 basic 规则。
func basicHostTable(t *testing.T) *HostTable {
	t.Helper()

	route := buildRouteConf(t, map[string][]route_rule_conf.BasicRouteRuleFile{
		"prod1":  {{Hostname: []string{"*"}, ClusterName: strPtr("cluster_test")}},
		"def_pr": {{Hostname: []string{"*"}, ClusterName: strPtr("cluster_default")}},
	}, nil)

	return newTestHostTable(t,
		host_rule_conf.HostTag2Product{"tag1": "prod1", "tagD": "def_pr"},
		host_rule_conf.Host2HostTag{"example.org": "tag1", "def.example.org": "tagD"},
		"def_pr", nil, route)
}

// 精确域名匹配：product 与 hostTag 都写回请求。
func TestHostTableLookupExactHost(t *testing.T) {
	table := basicHostTable(t)
	req := newRouteRequest("example.org", "/v1/echo")

	if err := table.LookupHostTagAndProduct(req); err != nil {
		t.Fatalf("LookupHostTagAndProduct failed: %v", err)
	}
	if req.Route.Product != "prod1" {
		t.Errorf("product = %q, want prod1", req.Route.Product)
	}
	if req.Route.HostTag != "tag1" {
		t.Errorf("hostTag = %q, want tag1", req.Route.HostTag)
	}
}

// 大小写不敏感：findHostRoute 会先 ToLower。
func TestHostTableLookupCaseInsensitive(t *testing.T) {
	table := basicHostTable(t)

	for _, host := range []string{"example.org", "EXAMPLE.ORG", "Example.Org"} {
		req := newRouteRequest(host, "/")
		if err := table.LookupHostTagAndProduct(req); err != nil {
			t.Fatalf("host %q should match, got %v", host, err)
		}
		if req.Route.Product != "prod1" {
			t.Errorf("host %q product = %q, want prod1", host, req.Route.Product)
		}
	}
}

// 通配后缀：*.example.org 命中子域名，不命中裸域名。
func TestHostTableLookupWildcard(t *testing.T) {
	route := buildRouteConf(t, map[string][]route_rule_conf.BasicRouteRuleFile{
		"prod1": {{Hostname: []string{"*"}, ClusterName: strPtr("cluster_test")}},
	}, nil)
	table := newTestHostTable(t,
		host_rule_conf.HostTag2Product{"tag1": "prod1"},
		host_rule_conf.Host2HostTag{"*.wild.org": "tag1"},
		"", nil, route)

	req := newRouteRequest("api.wild.org", "/")
	if err := table.LookupHostTagAndProduct(req); err != nil {
		t.Fatalf("subdomain should match wildcard, got %v", err)
	}
	if req.Route.Product != "prod1" {
		t.Errorf("product = %q, want prod1", req.Route.Product)
	}

	// 未注册的裸域名且无默认产品 -> 报错
	bare := newRouteRequest("wild.org", "/")
	if err := table.LookupHostTagAndProduct(bare); err == nil {
		t.Error("bare domain should not match *.wild.org")
	} else if !errors.Is(err, ErrNoProduct) {
		t.Errorf("error = %v, want ErrNoProduct", err)
	}
}

// Host 带端口：LookupCluster 需要切掉端口后再匹配。
func TestHostTableLookupHostWithPort(t *testing.T) {
	table := basicHostTable(t)
	req := newRouteRequest("example.org:8080", "/v1/echo")

	route := table.Lookup(req)
	if route.Error != nil {
		t.Fatalf("Lookup failed for host with port: %v", route.Error)
	}
	if route.ClusterName != "cluster_test" {
		t.Errorf("cluster = %q, want cluster_test", route.ClusterName)
	}
}

// 默认产品兜底：未注册域名落到 DefaultProduct。
func TestHostTableLookupDefaultProduct(t *testing.T) {
	table := basicHostTable(t)
	req := newRouteRequest("unknown.example.net", "/v1/echo")

	route := table.Lookup(req)
	if route.Error != nil {
		t.Fatalf("Lookup should fall back to default product: %v", route.Error)
	}
	if route.Product != "def_pr" {
		t.Errorf("product = %q, want def_pr", route.Product)
	}
	if route.ClusterName != "cluster_default" {
		t.Errorf("cluster = %q, want cluster_default", route.ClusterName)
	}
}

// 无默认产品且未注册：返回 ErrNoProduct。
func TestHostTableLookupNoDefaultProduct(t *testing.T) {
	route := buildRouteConf(t, map[string][]route_rule_conf.BasicRouteRuleFile{
		"prod1": {{Hostname: []string{"*"}, ClusterName: strPtr("cluster_test")}},
	}, nil)
	table := newTestHostTable(t,
		host_rule_conf.HostTag2Product{"tag1": "prod1"},
		host_rule_conf.Host2HostTag{"example.org": "tag1"},
		"", nil, route)

	req := newRouteRequest("unknown.example.net", "/")
	got := table.Lookup(req)
	if !errors.Is(got.Error, ErrNoProduct) {
		t.Fatalf("error = %v, want ErrNoProduct", got.Error)
	}
	if req.Route.Error == nil {
		t.Error("request route error should be recorded")
	}
}

// VIP 兜底：域名未注册时按访问 VIP 找产品。
func TestHostTableLookupByVip(t *testing.T) {
	route := buildRouteConf(t, map[string][]route_rule_conf.BasicRouteRuleFile{
		"prod1": {{Hostname: []string{"*"}, ClusterName: strPtr("cluster_test")}},
		"vip_p": {{Hostname: []string{"*"}, ClusterName: strPtr("cluster_vip")}},
	}, nil)
	table := newTestHostTable(t,
		host_rule_conf.HostTag2Product{"tag1": "prod1"},
		host_rule_conf.Host2HostTag{"example.org": "tag1"},
		"", vip_rule_conf.Vip2Product{"10.10.10.1": "vip_p"}, route)

	req := newRouteRequest("unknown.example.net", "/")
	req.Session.Vip = net.ParseIP("10.10.10.1")

	got := table.Lookup(req)
	if got.Error != nil {
		t.Fatalf("VIP fallback failed: %v", got.Error)
	}
	if got.Product != "vip_p" {
		t.Errorf("product = %q, want vip_p", got.Product)
	}

	// 域名命中时优先域名，不看 VIP
	req2 := newRouteRequest("example.org", "/")
	req2.Session.Vip = net.ParseIP("10.10.10.1")
	if got := table.Lookup(req2); got.Product != "prod1" {
		t.Errorf("host match should win over VIP, product = %q", got.Product)
	}
}

// LookupProduct / LookupProductByVip 的独立入口。
func TestHostTableLookupProduct(t *testing.T) {
	route := buildRouteConf(t, map[string][]route_rule_conf.BasicRouteRuleFile{
		"prod1": {{Hostname: []string{"*"}, ClusterName: strPtr("cluster_test")}},
		"vip_p": {{Hostname: []string{"*"}, ClusterName: strPtr("cluster_vip")}},
	}, nil)
	table := newTestHostTable(t,
		host_rule_conf.HostTag2Product{"tag1": "prod1"},
		host_rule_conf.Host2HostTag{"example.org": "tag1"},
		"", vip_rule_conf.Vip2Product{"10.10.10.1": "vip_p"}, route)

	if product, err := table.LookupProduct("example.org"); err != nil || product != "prod1" {
		t.Errorf("LookupProduct = (%q, %v), want (prod1, nil)", product, err)
	}
	if _, err := table.LookupProduct("unknown.example.net"); err == nil {
		t.Error("LookupProduct should fail for an unregistered host")
	}
	if product, err := table.LookupProductByVip("10.10.10.1"); err != nil || product != "vip_p" {
		t.Errorf("LookupProductByVip = (%q, %v), want (vip_p, nil)", product, err)
	}
	if _, err := table.LookupProductByVip("10.10.10.2"); err == nil {
		t.Error("LookupProductByVip should fail for an unknown vip")
	}
}

// basic 规则优先：basic 命中即返回，不再评估 advanced。
func TestHostTableBasicRuleTakesPrecedence(t *testing.T) {
	route := buildRouteConf(t, map[string][]route_rule_conf.BasicRouteRuleFile{
		"prod1": {{Hostname: []string{"*"}, Path: []string{"/api*"}, ClusterName: strPtr("cluster_basic")}},
	}, map[string][]route_rule_conf.AdvancedRouteRuleFile{
		"prod1": {{Cond: strPtr("default_t()"), ClusterName: strPtr("cluster_advanced")}},
	})
	table := newTestHostTable(t,
		host_rule_conf.HostTag2Product{"tag1": "prod1"},
		host_rule_conf.Host2HostTag{"example.org": "tag1"},
		"", nil, route)

	if got := table.Lookup(newRouteRequest("example.org", "/api/v1")); got.ClusterName != "cluster_basic" {
		t.Errorf("cluster = %q, want cluster_basic (basic rule wins)", got.ClusterName)
	}

	// basic 不命中时回落到 advanced
	if got := table.Lookup(newRouteRequest("example.org", "/other")); got.ClusterName != "cluster_advanced" {
		t.Errorf("cluster = %q, want cluster_advanced (advanced fallback)", got.ClusterName)
	}
}

// advanced 条件匹配取第一条命中的规则。
func TestHostTableAdvancedRuleOrder(t *testing.T) {
	route := buildRouteConf(t, nil, map[string][]route_rule_conf.AdvancedRouteRuleFile{
		"prod1": {
			{Cond: strPtr(`req_path_prefix_in("/nonmatch", false)`), ClusterName: strPtr("cluster_never")},
			{Cond: strPtr(`req_path_prefix_in("/api", false)`), ClusterName: strPtr("cluster_first")},
			{Cond: strPtr("default_t()"), ClusterName: strPtr("cluster_second")},
		},
	})
	table := newTestHostTable(t,
		host_rule_conf.HostTag2Product{"tag1": "prod1"},
		host_rule_conf.Host2HostTag{"example.org": "tag1"},
		"", nil, route)

	if got := table.Lookup(newRouteRequest("example.org", "/api/v1")); got.ClusterName != "cluster_first" {
		t.Errorf("cluster = %q, want cluster_first (first matching rule)", got.ClusterName)
	}
	if got := table.Lookup(newRouteRequest("example.org", "/other")); got.ClusterName != "cluster_second" {
		t.Errorf("cluster = %q, want cluster_second (default_t fallback)", got.ClusterName)
	}
}

// 无匹配规则 / 无产品规则表的两种错误。
func TestHostTableNoMatchErrors(t *testing.T) {
	// 有 advanced 表但都不命中
	route := buildRouteConf(t, nil, map[string][]route_rule_conf.AdvancedRouteRuleFile{
		"prod1": {{Cond: strPtr(`req_path_prefix_in("/api", false)`), ClusterName: strPtr("cluster_first")}},
	})
	table := newTestHostTable(t,
		host_rule_conf.HostTag2Product{"tag1": "prod1"},
		host_rule_conf.Host2HostTag{"example.org": "tag1"},
		"", nil, route)

	if got := table.Lookup(newRouteRequest("example.org", "/other")); !errors.Is(got.Error, ErrNoMatchRule) {
		t.Errorf("error = %v, want ErrNoMatchRule", got.Error)
	}

	// 产品下完全没有路由表
	empty := newTestHostTable(t,
		host_rule_conf.HostTag2Product{"tag1": "prod1"},
		host_rule_conf.Host2HostTag{"example.org": "tag1"},
		"", nil, buildRouteConf(t, map[string][]route_rule_conf.BasicRouteRuleFile{
			"other_prod": {{Hostname: []string{"*"}, ClusterName: strPtr("cluster_x")}},
		}, nil))

	if got := empty.Lookup(newRouteRequest("example.org", "/")); !errors.Is(got.Error, ErrNoProductRule) {
		t.Errorf("error = %v, want ErrNoProductRule", got.Error)
	}
}

// 版本与规模统计。
func TestHostTableStatusAndVersions(t *testing.T) {
	table := basicHostTable(t)

	versions := table.GetVersions()
	if versions.HostTag != "1" || versions.Vip != "1" || versions.ProductRoute != "1" {
		t.Errorf("versions = %+v, want all 1", versions)
	}

	status := table.GetStatus()
	if status.HostTableSize != 2 {
		t.Errorf("HostTableSize = %d, want 2", status.HostTableSize)
	}
	if status.ProductBasicRouteTableSize != 2 {
		t.Errorf("ProductBasicRouteTableSize = %d, want 2", status.ProductBasicRouteTableSize)
	}
	if status.HostTagTableSize != 2 {
		t.Errorf("HostTagTableSize = %d, want 2", status.HostTagTableSize)
	}
}

// 空表上的查找不应 panic。
func TestHostTableEmpty(t *testing.T) {
	table := newHostTable()

	req := newRouteRequest("example.org", "/")
	if err := table.LookupHostTagAndProduct(req); !errors.Is(err, ErrNoProduct) {
		t.Errorf("error = %v, want ErrNoProduct", err)
	}
	if _, err := table.LookupProduct("example.org"); err == nil {
		t.Error("LookupProduct on empty table should fail")
	}
	if got := table.GetStatus(); got.HostTableSize != 0 {
		t.Errorf("HostTableSize = %d, want 0", got.HostTableSize)
	}
}