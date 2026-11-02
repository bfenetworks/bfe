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

/*
legacy_compare_test.go - L1 对拍测试：旧引擎(oracle) vs 新引擎(bfe_expr.CompileLegacy)

DESCRIPTION
	每个用例给出旧条件串与期望结果 want：
	- 旧引擎（parser.Parse + build）结果必须等于 want（保证夹具与 oracle 正确）
	- 新引擎（bfe_expr.CompileLegacy）结果必须等于旧引擎（对拍）
	覆盖 bfe 当前 buildPrimitive 支持的全部原语。
	裁剪说明（相对源仓库）：bfe 旧引擎无 req_vport_in、req_cluster_in、req_sub_cluster_in、
	req_error_in、usr_variable_is_empty/value_in、apikey_in、model_in、bfe_cluster_in
	等原语，对应用例不移植。
*/

package condition

import (
	"net"
	"net/url"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition/parser"
	"github.com/bfenetworks/bfe/bfe_expr"
	"github.com/bfenetworks/bfe/bfe_http"
)

func newCompareRequest() *bfe_basic.Request {
	header := make(bfe_http.Header)
	header.Set("X-Uid", "12345")
	header.Set("User-Agent", "TestAgent/1.0")
	header.Set("X-Channel", "Internal")
	header.Set("Cookie", "sid=abc123")

	resHeader := make(bfe_http.Header)
	resHeader.Set("X-Res", "rv1")

	req := &bfe_basic.Request{
		HttpRequest: &bfe_http.Request{
			Method:     "GET",
			Proto:      "HTTP/1.1",
			Host:       "www.Example.com:8080",
			URL:        &url.URL{Path: "/api/v1/users", RawQuery: "uid=100&tag=a&tag=b&empty="},
			Header:     header,
			RequestURI: "/api/v1/users?uid=100&tag=a&tag=b&empty=",
		},
		HttpResponse: &bfe_http.Response{
			StatusCode: 200,
			Header:     resHeader,
		},
		Session: &bfe_basic.Session{
			Vip:        net.ParseIP("10.1.1.1"),
			Vport:      443,
			RemoteAddr: &net.TCPAddr{IP: net.ParseIP("2.2.2.2"), Port: 9999},
		},
		ClientAddr: &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 5678},
		Context:    make(map[interface{}]interface{}),
		Route:      bfe_basic.RequestRoute{Product: "example", HostTag: "tag1"},
		Backend: bfe_basic.BackendInfo{
			ClusterName:    "clusterA",
			SubclusterName: "subA",
		},
	}
	req.SetContext("k1", "v1")
	req.Tags.TagTable = make(map[string][]string)
	req.AddTags("t1", []string{"tv1:xx", "tv2"})
	return req
}

// buildLegacy 直接走旧引擎（绕过 Build 的新引擎优先逻辑），作为对拍 oracle
func buildLegacy(t *testing.T, condStr string) Condition {
	t.Helper()
	node, identList, err := parser.Parse(condStr)
	if err != nil {
		t.Fatalf("legacy parse %q: %v", condStr, err)
	}
	if len(identList) != 0 {
		t.Fatalf("legacy parse %q: unresolved variables", condStr)
	}
	c, err := build(node)
	if err != nil {
		t.Fatalf("legacy build %q: %v", condStr, err)
	}
	return c
}

func TestLegacyVsNewEquivalence(t *testing.T) {
	cases := []struct {
		name string
		cond string
		want bool
	}{
		// 逻辑与组合
		{"default_t", `default_t()`, true},
		{"not default_t", `!default_t()`, false},
		{"and", `req_method_in("GET") && req_path_prefix_in("/api", false)`, true},
		{"or", `req_method_in("PUT") || req_method_in("GET")`, true},
		{"paren", `(req_method_in("PUT") || req_method_in("GET")) && req_query_exist()`, true},

		// 方法/协议
		{"method in hit", `req_method_in("GET|POST")`, true},
		{"method in fold", `req_method_in("get|post")`, true},
		{"method in miss", `req_method_in("PUT")`, false},
		{"proto match", `req_proto_match("http/1.1")`, true},
		{"proto match miss", `req_proto_match("HTTP/2")`, false},
		{"proto secure", `req_proto_secure()`, false},

		// host
		{"host in hit", `req_host_in("www.example.com|other.com")`, true},
		{"host in case", `req_host_in("WWW.EXAMPLE.COM")`, true},
		{"host in miss", `req_host_in("other.com")`, false},
		{"host regmatch", `req_host_regmatch("^www[.]Example[.]com$")`, true},
		{"host regmatch miss", `req_host_regmatch("^www[.]example[.]com$")`, false},
		{"host tag", `req_host_tag_in("tag1|tag2")`, true},
		{"host tag miss", `req_host_tag_in("tag9")`, false},
		{"host suffix", `req_host_suffix_in("example.com")`, true},
		{"host suffix miss", `req_host_suffix_in("other.com")`, false},

		// path
		{"path in hit", `req_path_in("/api/v1/users", false)`, true},
		{"path in fold", `req_path_in("/API/V1/USERS", true)`, true},
		{"path in miss", `req_path_in("/api", false)`, false},
		{"path prefix", `req_path_prefix_in("/api|/web", false)`, true},
		{"path prefix fold", `req_path_prefix_in("/API", true)`, true},
		{"path prefix miss", `req_path_prefix_in("/web", false)`, false},
		{"path suffix", `req_path_suffix_in("users|list", false)`, true},
		{"path contain", `req_path_contain("v1", false)`, true},
		{"path contain miss", `req_path_contain("v9", false)`, false},
		{"path regmatch", `req_path_regmatch("^/api/.*")`, true},
		{"path regmatch miss", `req_path_regmatch("^/web/.*")`, false},
		{"path element prefix", `req_path_element_prefix_in("/api/v1", false)`, true},
		{"path element prefix miss", `req_path_element_prefix_in("/api/v2", false)`, false},

		// query
		{"query key prefix", `req_query_key_prefix_in("ui|ta")`, true},
		{"query key prefix miss", `req_query_key_prefix_in("zz")`, false},
		{"query key in", `req_query_key_in("uid|sid")`, true},
		{"query key in miss", `req_query_key_in("sid")`, false},
		{"exist", `req_query_exist()`, true},
		{"query value in", `req_query_value_in("uid", "100|200", false)`, true},
		{"query value in miss", `req_query_value_in("uid", "300", false)`, false},
		{"query value empty", `req_query_value_in("empty", "x", false)`, false},
		{"query value prefix", `req_query_value_prefix_in("uid", "10", false)`, true},
		{"query value suffix", `req_query_value_suffix_in("uid", "00", false)`, true},
		{"query value regmatch", `req_query_value_regmatch("uid", "^[0-9]+$")`, true},
		{"query value regmatch miss", `req_query_value_regmatch("uid", "^a")`, false},
		{"query value hash", `req_query_value_hash_in("uid", "0-9999", false)`, true},
		{"query value hash miss", `req_query_value_hash_in("uid", "5000-9999", false)`, false},
		{"query value contain", `req_query_value_contain("uid", "10|99", false)`, true},

		// url / cookie
		{"url regmatch", `req_url_regmatch("^/api/v1/users.*")`, true},
		{"cookie key in", `req_cookie_key_in("sid|other")`, true},
		{"cookie key in miss", `req_cookie_key_in("other")`, false},
		{"cookie value in", `req_cookie_value_in("sid", "abc123|x", false)`, true},
		{"cookie value in miss", `req_cookie_value_in("sid", "x", false)`, false},
		{"cookie value prefix", `req_cookie_value_prefix_in("sid", "abc", false)`, true},
		{"cookie value suffix", `req_cookie_value_suffix_in("sid", "123", false)`, true},
		{"cookie value hash", `req_cookie_value_hash_in("sid", "0-9999", true)`, true},
		{"cookie value contain", `req_cookie_value_contain("sid", "bc1", false)`, true},

		// port / tag / ua
		{"port in", `req_port_in("8080|9090")`, true},
		{"port in miss", `req_port_in("80")`, false},
		{"tag match", `req_tag_match("t1", "tv1")`, true},
		{"tag match full", `req_tag_match("t1", "tv1:xx")`, false},
		{"tag match miss", `req_tag_match("t1", "tv9")`, false},
		{"ua regmatch", `req_ua_regmatch("TestAgent")`, true},
		{"ua regmatch miss", `req_ua_regmatch("Bot")`, false},

		// header
		{"header key in", `req_header_key_in("X-Uid|X-Other")`, true},
		{"header key in miss", `req_header_key_in("X-Other")`, false},
		{"header value in", `req_header_value_in("X-Uid", "12345", false)`, true},
		{"header value in miss", `req_header_value_in("X-Uid", "999", false)`, false},
		{"header value prefix", `req_header_value_prefix_in("X-Uid", "123", false)`, true},
		{"header value suffix", `req_header_value_suffix_in("X-Uid", "345", false)`, true},
		{"header value regmatch", `req_header_value_regmatch("X-Uid", "^123[0-9]+$")`, true},
		{"header value hash", `req_header_value_hash_in("X-Uid", "0-9999", false)`, true},
		{"header value contain", `req_header_value_contain("X-Uid", "234", false)`, true},

		// ip / hash
		{"cip range", `req_cip_range("1.2.3.0", "1.2.3.255")`, true},
		{"cip range miss", `req_cip_range("1.2.4.0", "1.2.4.255")`, false},
		{"cip hash", `req_cip_hash_in("0-9999")`, true},
		{"vip in", `req_vip_in("10.1.1.1|9.9.9.9")`, true},
		{"vip in miss", `req_vip_in("10.1.1.2")`, false},
		{"vip range", `req_vip_range("10.1.1.0", "10.1.1.255")`, true},
		{"ses vip range", `ses_vip_range("10.1.1.0", "10.1.1.255")`, true},
		{"ses sip range", `ses_sip_range("2.2.2.0", "2.2.2.255")`, true},
		{"ses sip range miss", `ses_sip_range("3.3.3.0", "3.3.3.255")`, false},

		// 响应
		{"res code in", `res_code_in("200|404")`, true},
		{"res code in miss", `res_code_in("302")`, false},
		{"res header key in", `res_header_key_in("X-Res")`, true},
		{"res header key miss", `res_header_key_in("X-No")`, false},
		{"res header value in", `res_header_value_in("X-Res", "rv1", false)`, true},
		{"res header value miss", `res_header_value_in("X-Res", "rv2", false)`, false},

		// tls / 可信
		{"cip trusted", `req_cip_trusted()`, false},
		{"tls sni in", `ses_tls_sni_in("example.com")`, false},
		{"tls client ca in", `ses_tls_client_ca_in("ca1")`, false},

		// context / 时间
		{"context value in", `req_context_value_in("k1", "v1|v2", false)`, true},
		{"context value miss", `req_context_value_in("k1", "v9", false)`, false},
		{"context value absent", `req_context_value_in("nope", "v1", false)`, false},
		{"time range", `bfe_time_range("20200101000000Z", "20990101000000Z")`, true},
		{"time range miss", `bfe_time_range("20200101000000Z", "20200102000000Z")`, false},
		{"periodic range", `bfe_periodic_time_range("000000H", "235959H", "")`, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			legacy := buildLegacy(t, c.cond)
			legacyResult := legacy.Match(newCompareRequest())
			if legacyResult != c.want {
				t.Fatalf("legacy engine result = %v, want %v (fixture/oracle problem)", legacyResult, c.want)
			}

			newCond, err := bfe_expr.CompileLegacy(c.cond)
			if err != nil {
				t.Fatalf("CompileLegacy %q: %v", c.cond, err)
			}
			newResult := newCond.Match(newCompareRequest())
			if newResult != legacyResult {
				t.Errorf("divergence: legacy=%v new=%v", legacyResult, newResult)
			}
		})
	}
}

// TestMixedStyleCondition 新旧混写：旧原语与新风格函数写在同一条 Cond 中
func TestMixedStyleCondition(t *testing.T) {
	cond := `req_method_in("GET|POST") && ReqPath() startsWith "/api" && UsrVarSetStr("route", "hit") == "hit"`
	newCond, err := bfe_expr.CompileLegacy(cond)
	if err != nil {
		t.Fatalf("CompileLegacy mixed: %v", err)
	}

	req := newCompareRequest()
	if !newCond.Match(req) {
		t.Errorf("mixed condition should match")
	}
}

// TestBuildEnginePreference 验证 Build 按引擎模式分发：ExprCompatible 时旧串返回
// *bfe_expr.Condition；默认 LegacyFirst 时返回旧引擎 Condition
func TestBuildEnginePreference(t *testing.T) {
	defer SetEngineMode(EngineLegacyFirst)

	SetEngineMode(EngineExprCompatible)
	c, err := Build(`req_method_in("GET|POST")`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := c.(*bfe_expr.Condition); !ok {
		t.Fatalf("ExprCompatible should give expr engine cond, got %T", c)
	}

	SetEngineMode(EngineLegacyFirst)
	c, err = Build(`req_method_in("GET|POST")`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := c.(*PrimitiveCond); !ok {
		t.Fatalf("LegacyFirst should prefer legacy engine, got %T", c)
	}
}
