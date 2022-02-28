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

package mod_auth_request

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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

const (
	expectProduct = "example_product"
)

func TestLoadRuleData(t *testing.T) {
	m := new(ModuleAuthRequest)
	m.ruleTable = new(AuthRequestRuleTable)

	query := url.Values{
		"path": []string{"testdata/mod_auth_request/auth_request_rule.data"},
	}

	expectModVersion := "auth_request_rule.data=auth_request_rule_version"
	modVersion, err := m.loadRuleData(query)
	if err != nil {
		t.Fatalf("should have no error, but error is %v", err)
	}

	if modVersion != expectModVersion {
		t.Fatalf("version should be %s, but it's %s", expectModVersion, modVersion)
	}

	expectVersion := "auth_request_rule_version"
	if m.ruleTable.version != expectVersion {
		t.Fatalf("version should be %s, but it's %s", expectVersion, m.ruleTable.version)
	}

	ruleList, ok := m.ruleTable.productRule[expectProduct]
	if !ok {
		t.Fatalf("config should have product: %s", expectProduct)
	}

	if len(ruleList) != 1 {
		t.Fatalf("len(ruleList) should be 1, but it's %d", len(ruleList))
	}
}

func TestCreateAuthRequest(t *testing.T) {
	m := NewModuleAuthRequest()
	m.conf = new(ConfModAuthRequest)
	m.conf.Basic.AuthAddress = "http://example.org/auth_request"

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = expectProduct
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://example.org", nil)

	// test for copy header
	testCopyHeader := "X-Bfe-Test"
	testCopyHeaderValue := "test"
	req.HttpRequest.Header.Set(testCopyHeader, testCopyHeaderValue)

	// test for hop header
	for _, h := range bfe_basic.HopHeaders {
		req.HttpRequest.Header.Set(h, "hop")
	}

	authReq := m.createAuthRequest(req)

	for _, h := range bfe_basic.HopHeaders {
		if authReq.Header.Get(h) != "" {
			t.Fatalf("auth request should not have header :%s", h)
		}
	}

	if authReq.Header.Get(testCopyHeader) != testCopyHeaderValue {
		t.Fatalf("auth request should not have header :%s", testCopyHeaderValue)
	}

	if authReq.Header.Get(XForwardedMethod) != http.MethodGet {
		t.Fatalf("%s should be %s, but it's %s", XForwardedMethod, http.MethodGet, authReq.Header.Get(XForwardedMethod))
	}

	if authReq.Header.Get(XForwardedURI) != "/" {
		t.Fatalf("%s should be %s, but it's %s", XForwardedURI, "/", authReq.Header.Get(XForwardedURI))
	}
}

func TestCheckAuthForbidden(t *testing.T) {
	m := NewModuleAuthRequest()

	req, _ := bfe_http.NewRequest(http.MethodGet, "http://example.org", nil)
	basicReq := bfe_basic.NewRequest(req, nil, nil, nil, nil)
	resp := new(http.Response)
	resp.Header = make(http.Header)

	resp.StatusCode = http.StatusUnauthorized
	var forbiddenResp *bfe_http.Response
	if forbiddenResp = m.genAuthForbiddenResp(basicReq, resp); forbiddenResp == nil {
		t.Fatalf("forbiddenResp should be nil")
	}
	if forbiddenResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status_code should be %d, but it's %d", http.StatusUnauthorized, forbiddenResp.StatusCode)
	}

	resp.StatusCode = http.StatusCreated
	if forbiddenResp = m.genAuthForbiddenResp(basicReq, resp); forbiddenResp != nil {
		t.Fatalf("forbasicResp should be nil")
	}

	resp.StatusCode = http.StatusMovedPermanently
	if forbiddenResp = m.genAuthForbiddenResp(basicReq, resp); forbiddenResp != nil {
		t.Fatalf("checkAuthForbidden should nil")
	}
}

func TestAuthRequestHandler(t *testing.T) {
	m := NewModuleAuthRequest()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("WWW-Authenticate", "Basic realm=testforbfe")
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	m.conf.Basic.AuthAddress = ts.URL

	// prepare request
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = expectProduct
	req.HttpRequest, err = bfe_http.NewRequest("GET", "http://example.org/auth_request", nil)
	if err != nil {
		t.Fatalf("bfe_http.NewRequest error: %v", err)
	}

	action, resp := m.authRequestHandler(req)
	if action != bfe_module.BfeHandlerResponse {
		t.Fatalf("m.authRequestHandler should return %d, but it's %d", bfe_module.BfeHandlerResponse, action)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("resp code should be %d, but it's %d", http.StatusUnauthorized, resp.StatusCode)
	}

	wwwAuth := resp.Header.Get("WWW-Authenticate")
	if resp.Header.Get("WWW-Authenticate") != "Basic realm=testforbfe" {
		t.Fatalf("resp header[WWW-Authenticate] should be Basic realm=testforbfe, but it's %s", wwwAuth)
	}
}
