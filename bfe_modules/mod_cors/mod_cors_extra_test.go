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

package mod_cors

import (
	"net/http"
	"net/url"
	"testing"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const testProduct = "test_product"

func newBasicRequest(method, urlStr, origin string) *bfe_basic.Request {
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest, _ = bfe_http.NewRequest(method, urlStr, nil)
	req.HttpRequest.Header = make(bfe_http.Header)
	if origin != "" {
		req.HttpRequest.Header.Set(HeaderOrigin, origin)
	}
	return req
}

func newDefaultTrueRule(allowOriginMap map[string]bool) CorsRule {
	cond, _ := condition.Build("default_t()")
	return CorsRule{
		Cond:                        cond,
		AccessControlAllowOriginMap: allowOriginMap,
	}
}

func newTestModuleWithRule(product string, rule CorsRule) *ModuleCors {
	m := NewModuleCors()
	conf := &CorsRuleConf{
		Version: "20260802000000",
		Config: ProductRuleList{
			product: CorsRuleList{rule},
		},
	}
	m.ruleTable.Update(conf)
	return m
}

func TestNewModuleCors(t *testing.T) {
	m := NewModuleCors()
	if m == nil {
		t.Fatalf("NewModuleCors() should not return nil")
	}

	if m.Name() != ModCors {
		t.Fatalf("module name should be %s, but it's %s", ModCors, m.Name())
	}

	if m.ruleTable == nil {
		t.Fatalf("rule table should be initialized")
	}
}

func TestModuleCorsName(t *testing.T) {
	m := NewModuleCors()
	if m.Name() != ModCors {
		t.Fatalf("module name should be %s, but it's %s", ModCors, m.Name())
	}
}

func TestInitSuccess(t *testing.T) {
	m := NewModuleCors()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Fatalf("Init() should have no error, but error is %v", err)
	}

	if m.conf == nil {
		t.Fatalf("conf should be loaded")
	}
}

func TestInitFailure(t *testing.T) {
	m := NewModuleCors()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata/nonexistent")
	if err == nil {
		t.Fatalf("Init() should return error for missing conf root")
	}
}

func TestLoadRuleDataDefaultPath(t *testing.T) {
	m := NewModuleCors()
	m.conf = &ConfModCors{}
	m.conf.Basic.DataPath = "testdata/mod_cors/cors_rule.data"

	modVersion, err := m.loadRuleData(nil)
	if err != nil {
		t.Fatalf("loadRuleData() should have no error, but error is %v", err)
	}

	expectModVersion := "cors_rule.data=20200508210000"
	if modVersion != expectModVersion {
		t.Fatalf("version should be %s, but it's %s", expectModVersion, modVersion)
	}
}

func TestLoadRuleDataFailure(t *testing.T) {
	m := NewModuleCors()
	query := url.Values{
		"path": []string{"testdata/mod_cors/cors_rule_no_version.data"},
	}

	_, err := m.loadRuleData(query)
	if err == nil {
		t.Fatalf("loadRuleData() should return error for invalid rule file")
	}
}

func TestMatchOriginAllowed(t *testing.T) {
	rule := CorsRule{
		AccessControlAllowOriginMap: map[string]bool{"%origin": true},
	}
	allow, matched := matchOriginAllowed("http://example.org", &rule)
	if !allow || matched != "http://example.org" {
		t.Fatalf("%%origin should allow request origin")
	}

	rule.AccessControlAllowOriginMap = map[string]bool{"*": true}
	allow, matched = matchOriginAllowed("http://example.org", &rule)
	if !allow || matched != "*" {
		t.Fatalf("* should allow any origin")
	}

	rule.AccessControlAllowOriginMap = map[string]bool{"http://example.org": true}
	allow, matched = matchOriginAllowed("http://example.org", &rule)
	if !allow || matched != "http://example.org" {
		t.Fatalf("exact origin should match")
	}

	rule.AccessControlAllowOriginMap = map[string]bool{"http://other.org": true}
	allow, matched = matchOriginAllowed("http://example.org", &rule)
	if allow || matched != "" {
		t.Fatalf("unmatched origin should not be allowed")
	}
}

func TestAddVaryHeader(t *testing.T) {
	header := make(bfe_http.Header)
	addVaryHeader(header)
	if header.Get(HeaderVary) != HeaderOrigin {
		t.Fatalf("Vary header should be %s", HeaderOrigin)
	}

	header.Set(HeaderVary, "*")
	addVaryHeader(header)
	if header.Get(HeaderVary) != "*" {
		t.Fatalf("Vary header should remain *")
	}

	header.Set(HeaderVary, "Accept-Encoding")
	addVaryHeader(header)
	if header.Get(HeaderVary) != "Accept-Encoding,Origin" {
		t.Fatalf("Vary header should append Origin, but it's %s", header.Get(HeaderVary))
	}

	header.Set(HeaderVary, "Accept-Encoding, Origin")
	addVaryHeader(header)
	if header.Get(HeaderVary) != "Accept-Encoding, Origin" {
		t.Fatalf("Vary header should not duplicate Origin")
	}
}

func TestSetRespHeaderForNonPreflightNotAllowed(t *testing.T) {
	m := NewModuleCors()
	req := newBasicRequest(http.MethodGet, "http://example.org", "http://evil.org")
	rspHeader := make(bfe_http.Header)
	rule := newDefaultTrueRule(map[string]bool{"http://example.org": true})

	m.setRespHeaderForNonPreflight(req, rspHeader, &rule)

	if rspHeader.Get(HeaderAccessControlAllowOrigin) != "" {
		t.Fatalf("Allow-Origin should not be set for disallowed origin")
	}

	if rspHeader.Get(HeaderVary) != "" {
		t.Fatalf("Vary should not be set for disallowed origin")
	}
}

func TestSetRespHeaderForNonPreflightWildcard(t *testing.T) {
	m := NewModuleCors()
	req := newBasicRequest(http.MethodGet, "http://example.org", "http://example.org")
	rspHeader := make(bfe_http.Header)
	rule := newDefaultTrueRule(map[string]bool{"*": true})

	m.setRespHeaderForNonPreflight(req, rspHeader, &rule)

	if rspHeader.Get(HeaderAccessControlAllowOrigin) != "*" {
		t.Fatalf("Allow-Origin should be *, but it's %s", rspHeader.Get(HeaderAccessControlAllowOrigin))
	}

	if rspHeader.Get(HeaderVary) != HeaderOrigin {
		t.Fatalf("Vary should be %s", HeaderOrigin)
	}
}

func TestSetRespHeaderForNonPreflightMirrorOrigin(t *testing.T) {
	m := NewModuleCors()
	origin := "http://mirror.example"
	req := newBasicRequest(http.MethodGet, "http://example.org", origin)
	rspHeader := make(bfe_http.Header)
	rule := newDefaultTrueRule(map[string]bool{"%origin": true})

	m.setRespHeaderForNonPreflight(req, rspHeader, &rule)

	if rspHeader.Get(HeaderAccessControlAllowOrigin) != origin {
		t.Fatalf("Allow-Origin should mirror request origin")
	}
}

func TestSetRespHeaderForPreflightAllowed(t *testing.T) {
	m := NewModuleCors()
	origin := "http://example.org"
	req := newBasicRequest(http.MethodOptions, "http://example.org", origin)
	rspHeader := make(bfe_http.Header)
	maxAge := 600
	rule := CorsRule{
		Cond:                          nil,
		AccessControlAllowOriginMap:   map[string]bool{origin: true},
		AccessControlAllowCredentials: true,
		AccessControlAllowMethods:     []string{http.MethodPut, http.MethodDelete},
		AccessControlAllowHeaders:     []string{"X-Bfe-Test"},
		AccessControlMaxAge:           &maxAge,
	}

	m.setRespHeaderForPreflght(req, rspHeader, &rule)

	if rspHeader.Get(HeaderAccessControlAllowOrigin) != origin {
		t.Fatalf("Allow-Origin should be %s", origin)
	}

	if rspHeader.Get(HeaderAccessControlAllowCredentials) != "true" {
		t.Fatalf("Allow-Credentials should be true")
	}

	if rspHeader.Get(HeaderAccessControlAllowMethods) != "PUT,DELETE" {
		t.Fatalf("Allow-Methods is not expected: %s", rspHeader.Get(HeaderAccessControlAllowMethods))
	}

	if rspHeader.Get(HeaderAccessControlAllowHeaders) != "X-Bfe-Test" {
		t.Fatalf("Allow-Headers is not expected")
	}

	if rspHeader.Get(HeaderAccessControlMaxAge) != "600" {
		t.Fatalf("Max-Age is not expected")
	}
}

func TestCorsHandlerNoOrigin(t *testing.T) {
	m := newTestModuleWithRule(testProduct, newDefaultTrueRule(map[string]bool{"*": true}))
	req := newBasicRequest(http.MethodGet, "http://example.org", "")
	req.Route.Product = testProduct
	resp := bfe_basic.CreateInternalResp(req, bfe_http.StatusOK)

	ret := m.corsHandler(req, resp)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("corsHandler should return BfeHandlerGoOn for request without Origin")
	}

	if resp.Header.Get(HeaderAccessControlAllowOrigin) != "" {
		t.Fatalf("Allow-Origin should not be set")
	}
}

func TestCorsHandlerNoRule(t *testing.T) {
	m := NewModuleCors()
	req := newBasicRequest(http.MethodGet, "http://example.org", "http://example.org")
	req.Route.Product = testProduct
	resp := bfe_basic.CreateInternalResp(req, bfe_http.StatusOK)

	ret := m.corsHandler(req, resp)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("corsHandler should return BfeHandlerGoOn when no rule found")
	}

	if resp.Header.Get(HeaderAccessControlAllowOrigin) != "" {
		t.Fatalf("Allow-Origin should not be set when no rule found")
	}
}

func TestCorsHandlerPreflightSkipped(t *testing.T) {
	m := newTestModuleWithRule(testProduct, newDefaultTrueRule(map[string]bool{"*": true}))
	req := newBasicRequest(http.MethodOptions, "http://example.org", "http://example.org")
	req.HttpRequest.Header.Set(HeaderAccessControlRequestMethod, http.MethodPut)
	req.Route.Product = testProduct
	resp := bfe_basic.CreateInternalResp(req, bfe_http.StatusOK)

	ret := m.corsHandler(req, resp)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("corsHandler should return BfeHandlerGoOn for preflight request")
	}

	if resp.Header.Get(HeaderAccessControlAllowOrigin) != "" {
		t.Fatalf("corsHandler should not set CORS headers for preflight request")
	}
}

func TestCorsHandlerActualRequest(t *testing.T) {
	m := newTestModuleWithRule(testProduct, newDefaultTrueRule(map[string]bool{"*": true}))
	req := newBasicRequest(http.MethodGet, "http://example.org", "http://example.org")
	req.Route.Product = testProduct
	resp := bfe_basic.CreateInternalResp(req, bfe_http.StatusOK)

	ret := m.corsHandler(req, resp)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("corsHandler should return BfeHandlerGoOn")
	}

	if resp.Header.Get(HeaderAccessControlAllowOrigin) != "*" {
		t.Fatalf("Allow-Origin should be *, but it's %s", resp.Header.Get(HeaderAccessControlAllowOrigin))
	}

	if resp.Header.Get(HeaderVary) != HeaderOrigin {
		t.Fatalf("Vary should be %s", HeaderOrigin)
	}
}

func TestCorsHandlerActualRequestNotAllowed(t *testing.T) {
	m := newTestModuleWithRule(testProduct, newDefaultTrueRule(map[string]bool{"http://other.org": true}))
	req := newBasicRequest(http.MethodGet, "http://example.org", "http://example.org")
	req.Route.Product = testProduct
	resp := bfe_basic.CreateInternalResp(req, bfe_http.StatusOK)

	ret := m.corsHandler(req, resp)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("corsHandler should return BfeHandlerGoOn")
	}

	if resp.Header.Get(HeaderAccessControlAllowOrigin) != "" {
		t.Fatalf("Allow-Origin should not be set for disallowed origin")
	}
}

func TestCorsPreflightHandler(t *testing.T) {
	m := newTestModuleWithRule(testProduct, newDefaultTrueRule(map[string]bool{"*": true}))
	req := newBasicRequest(http.MethodOptions, "http://example.org", "http://example.org")
	req.HttpRequest.Header.Set(HeaderAccessControlRequestMethod, http.MethodPut)
	req.Route.Product = testProduct

	ret, resp := m.corsPreflightHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("corsPreflightHandler should return BfeHandlerResponse")
	}

	if resp == nil {
		t.Fatalf("corsPreflightHandler should return response")
	}

	if resp.StatusCode != bfe_http.StatusNoContent {
		t.Fatalf("preflight status should be %d, but it's %d", bfe_http.StatusNoContent, resp.StatusCode)
	}

	if resp.Header.Get(HeaderAccessControlAllowOrigin) != "*" {
		t.Fatalf("Allow-Origin should be *")
	}
}

func TestCorsPreflightHandlerNonPreflight(t *testing.T) {
	m := newTestModuleWithRule(testProduct, newDefaultTrueRule(map[string]bool{"*": true}))
	req := newBasicRequest(http.MethodGet, "http://example.org", "http://example.org")
	req.Route.Product = testProduct

	ret, resp := m.corsPreflightHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("corsPreflightHandler should return BfeHandlerGoOn for non-preflight request")
	}

	if resp != nil {
		t.Fatalf("corsPreflightHandler should not return response for non-preflight request")
	}
}

func TestCorsPreflightHandlerNoRule(t *testing.T) {
	m := NewModuleCors()
	req := newBasicRequest(http.MethodOptions, "http://example.org", "http://example.org")
	req.HttpRequest.Header.Set(HeaderAccessControlRequestMethod, http.MethodPut)
	req.Route.Product = testProduct

	ret, resp := m.corsPreflightHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("corsPreflightHandler should return BfeHandlerGoOn when no rule found")
	}

	if resp != nil {
		t.Fatalf("corsPreflightHandler should not return response when no rule found")
	}
}

func TestCreateCorsPreflightResponse(t *testing.T) {
	m := NewModuleCors()
	origin := "http://example.org"
	req := newBasicRequest(http.MethodOptions, "http://example.org", origin)
	rule := newDefaultTrueRule(map[string]bool{origin: true})

	resp := m.createCorsPreflightResponse(req, &rule)
	if resp == nil {
		t.Fatalf("createCorsPreflightResponse should return response")
	}

	if resp.StatusCode != bfe_http.StatusNoContent {
		t.Fatalf("preflight status should be %d", bfe_http.StatusNoContent)
	}

	if resp.Header.Get(HeaderAccessControlAllowOrigin) != origin {
		t.Fatalf("Allow-Origin should be %s", origin)
	}
}
