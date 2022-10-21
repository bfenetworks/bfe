// Copyright (c) 2019 The BFE Authors.
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

package condition

import (
	"net"
	"reflect"
	"regexp"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_tls"
	"github.com/bfenetworks/bfe/bfe_util/net_util"
)

var (
	req bfe_basic.Request = bfe_basic.Request{
		Session:     &bfe_basic.Session{},
		HttpRequest: &bfe_http.Request{},
	}
)

func stringAddr(x string) *string {
	return &x
}

func intAddr(x int) *int {
	return &x
}

func compileRegExpr(expr string) *regexp.Regexp {
	reg, err := regexp.Compile(expr)
	if err != nil {
		return nil
	}

	return reg
}

func IPv4ToUint32(ipStr string) uint32 {
	ipUint32, _ := net_util.IPv4StrToUint32(ipStr)
	return ipUint32
}

var buildPrimitiveTests = []struct {
	name      string
	cond      string
	buildCond Condition
	buildErr  bool
}{
	{
		"testWrongName",
		"req_path_in1",
		nil,
		true,
	},
	{
		"testDefaultBoolParam",
		"req_path_in(\"/ABC\", false)",
		&PrimitiveCond{
			name:    "req_path_in",
			fetcher: &PathFetcher{},
			matcher: &InMatcher{
				patterns: []string{"/ABC"},
				foldCase: false},
		},
		false,
	},
	{
		"testWrongParamType",
		"req_path_in(\"notbool\")",
		nil,
		true,
	},
	{
		"testWrongVariable",
		"a && b",
		nil,
		true,
	},
	{
		"testDefaultTrue",
		"default_t()",
		&DefaultTrueCond{},
		false,
	},
	{
		"testBuildReqPatIn",
		"req_path_in(\"/abc\", true)",
		&PrimitiveCond{
			name:    "req_path_in",
			fetcher: &PathFetcher{},
			matcher: &InMatcher{
				patterns: []string{"/ABC"},
				foldCase: true},
		},
		false,
	},
	{
		"testBuildReqPathElementPrefixIn",
		"req_path_element_prefix_in(\"/abc\", true)",
		&PrimitiveCond{
			name:    "req_path_element_prefix_in",
			fetcher: &PathFetcher{},
			matcher: &PathElementPrefixMatcher{
				patterns: []string{"/ABC/"},
				foldCase: true,
			},
		},
		false,
	},
	{
		"testBuildQueRegMatch",
		"req_query_value_regmatch(\"abc\", \"123\")",
		&PrimitiveCond{
			name: "req_query_value_regmatch",
			fetcher: &QueryValueFetcher{
				key: "abc",
			},
			matcher: &RegMatcher{
				regex: compileRegExpr("123"),
			},
		},
		false,
	},
	{
		"testQueryExist",
		"req_query_exist()",
		&QueryExistMatcher{},
		false,
	},
	{
		"testBuildUrlRegMatch",
		"req_url_regmatch(\"123\")",
		&PrimitiveCond{
			name:    "req_url_regmatch",
			fetcher: &UrlFetcher{},
			matcher: &RegMatcher{
				regex: compileRegExpr("123"),
			},
		},
		false,
	},
	{
		"testBuildUrlRegMatchcase1",
		"req_url_regmatch(`123`)",
		&PrimitiveCond{
			name:    "req_url_regmatch",
			fetcher: &UrlFetcher{},
			matcher: &RegMatcher{
				regex: compileRegExpr("123"),
			},
		},
		false,
	},
	{
		"testBuildVIPIn",
		"req_vip_in(\"1.1.1.1|2001:DB8:2de::e13\")",
		&PrimitiveCond{
			name:    "req_vip_in",
			fetcher: &VIPFetcher{},
			matcher: &IpInMatcher{
				patterns: []net.IP{net.ParseIP("1.1.1.1"), net.ParseIP("2001:DB8:2de::e13")},
			},
		},
		false,
	},
	{
		"testBuildVIPInWrongCase",
		"req_vip_in(\"1.1.1.1|2001::DB8:2de::e13\")",
		nil,
		true,
	},
	{
		"testBuildCIPMatch",
		"req_cip_range(\"1.1.1.1\", \"2.2.2.2\")",
		&PrimitiveCond{
			name:    "req_cip_range",
			fetcher: &CIPFetcher{},
			matcher: &IPMatcher{
				startIP: net.ParseIP("1.1.1.1"),
				endIP:   net.ParseIP("2.2.2.2"),
			},
		},
		false,
	},
	{
		"testBuildCIPMatchIpv6",
		"req_cip_range(\"2001:DB8:2de::e13\", \"2002:DB8:2de::e13\")",
		&PrimitiveCond{
			name:    "req_cip_range",
			fetcher: &CIPFetcher{},
			matcher: &IPMatcher{
				startIP: net.ParseIP("2001:DB8:2de::e13"),
				endIP:   net.ParseIP("2002:DB8:2de::e13"),
			},
		},
		false,
	},
	{
		"testBuildCIPMatch_wrongCase1",
		"req_cip_range(\"1.1.1.1\", \"1.1.1.0\")",
		nil,
		true,
	},
	{
		"testBuildCIPMatch_wrongCase1_notip",
		"req_cip_range(\"1.1.1\", \"1.1.1.0\")",
		nil,
		true,
	},
	{
		"testBuildCIPMatch_wrongCase_ipv4_ipv6",
		"req_cip_range(\"1.1.1.1\", \"2001:DB8:2de::e13\")",
		nil,
		true,
	},
	{
		"testBuildCIPMatch_wrongCase_ipv6",
		"req_cip_range(\"2002:DB8:2de::e13\", \"2001:DB8:2de::e13\")",
		nil,
		true,
	},
	{
		"testBuildHashInErr1",
		"req_header_value_hash_in(\"X-Bfe-Uid\", \"abc\", true)",
		nil,
		true,
	},
	{
		"testBuildHashInErr2",
		"req_header_value_hash_in(\"X-Bfe-Uid\", \"100-200|300-\", true)",
		nil,
		true,
	},
	{
		"testBuildHashInErr3",
		"req_header_value_hash_in(\"X-Bfe-Uid\", \"100-200|300-400-500\", true)",
		nil,
		true,
	},
	{
		"testBuildHashInErr4",
		"req_header_value_hash_in(\"X-Bfe-Uid\", \"100-10000\", true)",
		nil,
		true,
	},
	{
		"testBuildHashInErr4",
		"req_header_value_hash_in(\"X-Bfe-Uid\", \"100-99\", true)",
		nil,
		true,
	},
}

func TestBuildPrimitive(t *testing.T) {
	for _, buildPrimitiveTest := range buildPrimitiveTests {
		t.Logf("run test %s", buildPrimitiveTest.name)
		buildC, err := Build(buildPrimitiveTest.cond)

		if buildPrimitiveTest.buildErr {
			if err == nil {
				t.Errorf("build primitive should return err")
			}
			t.Logf("build err as expected [%s]", err)
		} else {
			if err != nil {
				t.Errorf("build should success %s", err)
			}
			// check equal
			// hack:ignore node field compare
			if c, ok := buildC.(*PrimitiveCond); ok {
				c.node = nil
			}
			if !reflect.DeepEqual(buildC, buildPrimitiveTest.buildCond) {
				t.Errorf("build cond not equal [%v] [%v]", buildC, buildPrimitiveTest.buildCond)
			}
		}
	}
}

func TestBuildReqVipIn(t *testing.T) {
	buildC, err := Build("req_vip_in(\"1.1.1.1|2001:DB8:2de::e13\")")
	if err != nil {
		t.Errorf("build failed, req_vip_in(\"1.1.1.1|2001:DB8:2de::e13\")")
	}
	req.Session.Vip = net.IPv4(1, 1, 1, 1).To4()
	if !buildC.Match(&req) {
		t.Errorf("1.1.1.1 not match req_vip_in(\"1.1.1.1|2001:DB8:2de::e13\")")
	}
	req.Session.Vip = net.ParseIP("2001:0DB8:02de:0::e13")
	if !buildC.Match(&req) {
		t.Errorf("2001:DB8:2de::e13 not match req_vip_in(\"1.1.1.1|2001:DB8:2de::e13\")")
	}
	req.Session.Vip = net.ParseIP("2002:0DB8:02de:0::e13")
	if buildC.Match(&req) {
		t.Errorf("2002:DB8:2de::e13 not match req_vip_in(\"1.1.1.1|2001:DB8:2de::e13\")")
	}
}

func TestBuildReqVipRange(t *testing.T) {
	buildC, err := Build("req_vip_range(\"1.1.1.1\", \"4.4.4.4\")")
	if err != nil {
		t.Errorf("build failed, req_vip_range(\"1.1.1.1\", \"4.4.4.4\")")
	}
	req.Session.Vip = net.IPv4(3, 255, 255, 255).To4()
	if !buildC.Match(&req) {
		t.Errorf("3.255.255.255 not match req_vip_range(\"1.1.1.1\", \"4.4.4.4\")")
	}

	buildC, err = Build("req_vip_range(\"2001:0DB8:02de:0::e13\", \"2002:0DB8:02de:0::e13\")")
	if err != nil {
		t.Errorf("build failed, req_vip_range(\"2001:0DB8:02de:0::e13\", \"2002:0DB8:02de:0::e13\")")
	}
	req.Session.Vip = net.ParseIP("2001:ffff::ffff")
	if !buildC.Match(&req) {
		t.Errorf("2001:ffff::ffff not match req_vip_range(\"2001:0DB8:02de:0::e13\", \"2002:0DB8:02de:0::e13\")")
	}
}

func TestBuildHeaderValueHashIn(t *testing.T) {
	buildC, err := Build("req_header_value_hash_in(\"X-Bfe-Uid\", \"4073|5000-9999\", true)")
	if err != nil {
		t.Errorf("build failed, req_header_value_hash_in(\"X-Bfe-Uid\", \"4073|5000-9999\", true), err(%s)",
			err.Error())
	}
	req.HttpRequest.Header = make(map[string][]string)
	req.HttpRequest.Header["X-Bfe-Uid"] = []string{"TEST-uid-0001"} // hash_bucket of "test-uid-0001": 5968
	if !buildC.Match(&req) {
		t.Errorf("TEST-uid-0001 not match req_header_value_hash_in(\"X-Bfe-Uid\", \"4073|5000-9999\", true)")
	}
	req.HttpRequest.Header["X-Bfe-Uid"] = []string{"test-uid-0002"} // hash_bucket of "test-uid-0002": 4073
	if !buildC.Match(&req) {
		t.Errorf("test-uid-0002 not match req_header_value_hash_in(\"X-Bfe-Uid\", \"4073|5000-9999\", true)")
	}
	req.HttpRequest.Header["X-Bfe-Uid"] = []string{"test-uid-0003"} // hash_bucket of "test-uid-0003: 4055
	if buildC.Match(&req) {
		t.Errorf("test-uid-0003 match req_header_value_hash_in(\"X-Bfe-Uid\", \"4073|5000-9999\", true)")
	}
	req.HttpRequest.Header["X-Bfe-Uid"] = []string{"test-uid-0004"} // hash_bucket of "test-uid-0004: 9683
	if !buildC.Match(&req) {
		t.Errorf("test-uid-0004 not match req_header_value_hash_in(\"X-Bfe-Uid\", \"4073|5000-9999\", true)")
	}
}

func TestBuildTlsSniIn(t *testing.T) {
	buildTlsSniIn, err := Build("ses_tls_sni_in(\"test.com\")")
	if err != nil {
		t.Errorf("build failed, ses_tls_sni_in(\"test.com\"), err(%s)",
			err.Error())
	}

	req.Session = &bfe_basic.Session{TlsState: &bfe_tls.ConnectionState{ServerName: "test.com"}, IsSecure: true}
	if !buildTlsSniIn.Match(&req) {
		t.Errorf("sni not match ses_tls_sni_in(\"test.com\")")
	}

	req.Session = &bfe_basic.Session{TlsState: &bfe_tls.ConnectionState{ServerName: "test.com"}}
	if buildTlsSniIn.Match(&req) {
		t.Errorf("sni match ses_tls_sni_in(\"test.com\")")
	}
}

func TestBuildTlsClientAuth(t *testing.T) {
	buildTlsClientAuth, err := Build("ses_tls_client_auth()")
	if err != nil {
		t.Errorf("build failed, ses_tls_client_auth(), err(%s)",
			err.Error())
	}

	req.Session = &bfe_basic.Session{TlsState: &bfe_tls.ConnectionState{ClientAuth: true}, IsSecure: true}
	if !buildTlsClientAuth.Match(&req) {
		t.Errorf("clientauth not match ses_tls_client_auth()")
	}

	req.Session = &bfe_basic.Session{TlsState: &bfe_tls.ConnectionState{ClientAuth: false}, IsSecure: true}
	if buildTlsClientAuth.Match(&req) {
		t.Errorf("clientauth match ses_tls_client_auth()")
	}
}

func TestBuildTlsClientCAIn(t *testing.T) {
	buildTlsClientCAIn, err := Build("ses_tls_client_ca_in(\"clientCa\")")
	if err != nil {
		t.Errorf("build failed, ses_tls_client_ca_in(\"clientCa\"), err(%s)",
			err.Error())
	}

	req.Session = &bfe_basic.Session{TlsState: &bfe_tls.ConnectionState{ClientAuth: true, ClientCAName: "clientCa"}, IsSecure: true}
	if !buildTlsClientCAIn.Match(&req) {
		t.Errorf("ca not match ses_tls_client_ca_in(\"clientCa\")")
	}

	req.Session = &bfe_basic.Session{TlsState: &bfe_tls.ConnectionState{ClientAuth: false, ClientCAName: "clientCa"}, IsSecure: true}
	if buildTlsClientCAIn.Match(&req) {
		t.Errorf("ca match ses_tls_client_ca_in(\"clientCa\")")
	}
}

func TestBuildHostTagIn(t *testing.T) {
	cond, err := Build("req_host_tag_in(\"host_tag1|host_tag2\")")
	if err != nil {
		t.Fatalf("should have no error")
	}
	req.Route.HostTag = "host_tag1"
	if !cond.Match(&req) {
		t.Fatalf("should match host tag %s", req.Route.HostTag)
	}
	req.Route.HostTag = "host_tag2"
	if !cond.Match(&req) {
		t.Fatalf("should match host tag %s", req.Route.HostTag)
	}
	req.Route.HostTag = "host_tag3"
	if cond.Match(&req) {
		t.Fatalf("should not match host tag %s", req.Route.HostTag)
	}
}
