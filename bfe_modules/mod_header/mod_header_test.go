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

package mod_header

import (
	"testing"
)

import (
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func TestGetModuleName(t *testing.T) {
	m := NewModuleHeader()
	if m.Name() != "mod_header" {
		t.Error("module name is wrong, Expect \"mod_header\"")
	}
}

func TestModHeaderSetGlobal(t *testing.T) {
	testReqHandler(t, "https://www.example.org", nil, "pb", true, false, "https", func(
		t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}

		headerValue := req.HttpRequest.Header["X-Ssl-Header"]
		if len(headerValue) == 0 || headerValue[0] != "2" {
			t.Errorf("header set failed for https, Expect header \"X-Ssl-Header:2\"")
		}
	})
}

func TestModHeaderSetSecure(t *testing.T) {
	testReqHandler(t, "https://www.example.org", nil, "pn", true, false, "https", func(
		t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}

		headerValue := req.HttpRequest.Header["X-Ssl-Header"]
		if len(headerValue) == 0 || headerValue[0] != "1" {
			t.Error("header set failed for https, Expect header \"X-Ssl-Header:1\"")
		}
	})
}

func TestModHeaderSetNotSecure(t *testing.T) {
	testReqHandler(t, "http://www.example.org", nil, "pn", false, false, "http", func(
		t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}

		if len(req.HttpRequest.Header["X-Ssl-Header"]) != 0 {
			t.Error("header set failed for http, Expect empty header")
		}
	})
}

func TestModHeaderAdd(t *testing.T) {
	testReqHandler(t, "http://www.example.org", nil, "pb", false, false, "http", func(
		t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}

		headerValue := req.HttpRequest.Header["Header_add_test"]
		if len(headerValue) < 1 {
			t.Errorf("header add failed, headerValue is: %s", headerValue)
		}
	})
}

//NOTE: HEADER_DEL can't delete user defined headers.
func TestModHeaderDel(t *testing.T) {
	header := make(bfe_http.Header)
	header.Add("Host", "www.example.org")
	testReqHandler(t, "http://www.example.org", header, "pb", false, false, "http", func(
		t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}

		headerValue := req.HttpRequest.Header["Host"]
		if len(headerValue) > 0 {
			t.Error("header delete failed, headerValue is:", headerValue)
		}
	})
}

func TestModHeaderTrustIPNotTrust(t *testing.T) {
	header := make(bfe_http.Header)
	header.Add("Host", "www.example.org")
	testReqHandler(t, "http://www.example.org", header, "pb", false, false, "http", func(
		t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}

		headerValue := req.HttpRequest.Header["Trustip"]
		if len(headerValue) == 0 || headerValue[0] != "False" {
			t.Error("header set failed for trust ip, Expect header \"False\"")
		}

	})
}

func TestModHeaderTrustIPTrust(t *testing.T) {
	header := make(bfe_http.Header)
	header.Add("Host", "www.example.org")
	testReqHandler(t, "http://www.example.org", header, "pb", false, true, "http", func(
		t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}

		headerValue := req.HttpRequest.Header["Trustip"]
		if len(headerValue) == 0 || headerValue[0] != "True" {
			t.Error("header set failed for trust ip, Expect header \"True\"")
		}
	})
}

func TestDelXsslHeaderNotTrustIPAndHttp(t *testing.T) {
	header := make(bfe_http.Header)
	header.Add("X-Ssl-Header", "1")
	testReqHandler(t, "http://www.example.org", header, "pb", false, false, "http", func(
		t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}

		headerValue := req.HttpRequest.Header["X-Ssl-Header"]
		if len(headerValue) != 0 {
			t.Errorf("header delete failed for not trust ip when in http")
		}
	})
}

func TestReqSetCookieExist(t *testing.T) {
	header := make(bfe_http.Header)
	header.Add("Cookie", "SET=0")
	testReqHandler(t, "http://www.example.org/cookie_set", header, "p1", false, false, "http", func(
		t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}

		cookie, err := req.HttpRequest.Cookie("SET")
		if err != nil {
			t.Error("request add cookie error")
			return
		}
		if cookie.Value != "1" {
			t.Errorf("cookie value should be \"1\", not %s", cookie.Value)
		}
	})
}

func TestReqSetCookieNotExist(t *testing.T) {
	testReqHandler(t, "http://www.example.org/cookie_set", nil, "p1", false, false, "http", func(
		t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}

		cookie, err := req.HttpRequest.Cookie("SET")
		if err != nil {
			t.Error("request add cookie error")
			return
		}
		if cookie.Value != "1" {
			t.Errorf("cookie value should be \"1\", not %s", cookie.Value)
		}
	})
}

func TestReqDelCookie(t *testing.T) {
	header := make(bfe_http.Header)
	header.Add("Cookie", "DEL=1")
	testReqHandler(t, "http://www.example.org/cookie_del", header, "p1", false, false, "http", func(
		t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}

		_, err := req.HttpRequest.Cookie("DEL")
		if err == nil {
			t.Error("request del cookie error")
		}
	})
}

func TestRspDelCookieExist(t *testing.T) {
	cookies := []bfe_http.Cookie{
		{
			Name:   "COOKIE",
			Value:  "1",
			Path:   "/unittest",
			Domain: "example.org",
			MaxAge: 100,
		},
		{
			Name:   "SECOND",
			Value:  "2",
			Path:   "/unittest",
			Domain: "example.org",
			MaxAge: 100,
		},
	}

	testRspHandler(t, "http://www.example.org/second", "p1", cookies,
		func(t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
			if ret != bfe_module.BfeHandlerGoOn {
				t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
			}

			cookies := req.HttpResponse.Cookies()
			if len(cookies) != 1 {
				t.Errorf("response cookie length error")
				return
			}
			if cookies[0].Name != "COOKIE" {
				t.Errorf("response delete cookie error")
			}
		})
}

func TestRspDelCookieNotExist(t *testing.T) {
	cookies := []bfe_http.Cookie{
		{
			Name:   "COOKIE",
			Value:  "1",
			Path:   "/unittest",
			Domain: "example.org",
			MaxAge: 100,
		},
	}

	testRspHandler(t, "http://www.example.org/second", "p1", cookies,
		func(t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
			if ret != bfe_module.BfeHandlerGoOn {
				t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
			}

			cookies := req.HttpResponse.Cookies()
			if len(cookies) != 1 {
				t.Errorf("response cookie length error")
				return
			}
			if cookies[0].Name != "COOKIE" {
				t.Errorf("response delete cookie error")
			}
		})
}

func TestRspSetCookieExist(t *testing.T) {
	cookies := []bfe_http.Cookie{
		{
			Name:   "SET",
			Value:  "1",
			Path:   "/unittest",
			Domain: "example.org",
			MaxAge: 100,
		},
	}

	testRspHandler(t, "http://www.example.org/rsp_cookie_set", "p2", cookies,
		func(t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
			if ret != bfe_module.BfeHandlerGoOn {
				t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
			}

			cookies := req.HttpResponse.Cookies()
			if len(cookies) != 1 {
				t.Errorf("response cookie length error")
				return
			}
			if cookies[0].Value != "2" {
				t.Errorf("response set cookie error")
			}
		})
}

func TestRspSetCookieNotExist(t *testing.T) {
	cookies := []bfe_http.Cookie{
		{
			Name:   "COOKIE",
			Value:  "1",
			Path:   "/unittest",
			Domain: "example.org",
			MaxAge: 100,
		},
	}

	testRspHandler(t, "http://www.example.org/rsp_cookie_set", "p2", cookies,
		func(t *testing.T, m *ModuleHeader, ret int, req *bfe_basic.Request) {
			if ret != bfe_module.BfeHandlerGoOn {
				t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
			}

			cookies := req.HttpResponse.Cookies()
			if len(cookies) != 2 {
				t.Errorf("response cookie length error")
				return
			}
			if cookies[1].Name != "SET" {
				t.Errorf("response set cookie error")
			}
		})
}

func initTestModuleHeader(t *testing.T, url string, header bfe_http.Header, product string,
	isSecure bool, isTrustIP bool, proto string) (*ModuleHeader, *bfe_basic.Request) {
	// prepare module header
	m := NewModuleHeader()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// prepare request
	req := makeBasicRequest(url)
	req.Session.IsSecure = isSecure
	req.Session.SetTrustSource(isTrustIP)
	req.Session.Proto = proto
	req.Route.Product = product
	if header != nil {
		req.HttpRequest.Header = header
	}

	return m, req
}

func testReqHandler(t *testing.T, url string, header bfe_http.Header, product string,
	isSecure bool, isTrustIP bool, proto string,
	check func(*testing.T, *ModuleHeader, int, *bfe_basic.Request)) {
	m, req := initTestModuleHeader(t, url, header, product, isSecure, isTrustIP, proto)

	// process request and check
	ret, _ := m.reqHeaderHandler(req)
	check(t, m, ret, req)
}

func initResponse(req *bfe_basic.Request, rspCookies []bfe_http.Cookie) {
	req.HttpResponse = new(bfe_http.Response)
	req.HttpResponse.Header = make(bfe_http.Header)
	for _, rspCookie := range rspCookies {
		req.HttpResponse.Header.Add("Set-Cookie", rspCookie.String())
	}
}

func testRspHandler(t *testing.T, url string, product string, rspCookies []bfe_http.Cookie,
	check func(*testing.T, *ModuleHeader, int, *bfe_basic.Request)) {
	m, req := initTestModuleHeader(t, url, nil, product, false, false, "http")
	initResponse(req, rspCookies)

	// process request and check
	ret := m.rspHeaderHandler(req, req.HttpResponse)
	check(t, m, ret, req)
}
