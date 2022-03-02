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

package mod_cors

import (
	"net/http"
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
	m := NewModuleCors()

	query := url.Values{
		"path": []string{"testdata/mod_cors/cors_rule.data"},
	}

	expectModVersion := "cors_rule.data=20200508210000"
	modVersion, err := m.loadRuleData(query)
	if err != nil {
		t.Fatalf("should have no error, but error is %v", err)
	}

	if modVersion != expectModVersion {
		t.Fatalf("version should be %s, but it's %s", expectModVersion, modVersion)
	}

	expectVersion := "20200508210000"
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

func TestCorsHandlerCase1(t *testing.T) {
	m := NewModuleCors()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// prepare request
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = expectProduct
	req.HttpRequest, err = bfe_http.NewRequest(http.MethodGet, "http://example.org", nil)
	if err != nil {
		t.Fatalf("bfe_http.NewRequest error: %v", err)
	}
	req.HttpRequest.Header = make(bfe_http.Header)
	req.HttpRequest.Header.Set(HeaderOrigin, "http://hello-world.example")

	// prepare respnose
	resp := bfe_basic.CreateInternalResp(req, bfe_http.StatusOK)

	m.corsHandler(req, resp)

	origin := resp.Header.Get(HeaderAccessControlAllowOrigin)
	if origin != "*" {
		t.Fatalf("response header %s is not expected, it's %s", HeaderAccessControlAllowOrigin, origin)
	}
}

func TestCheckCorsPreflight(t *testing.T) {
	req := new(bfe_basic.Request)
	req.HttpRequest, _ = bfe_http.NewRequest(http.MethodOptions, "http://example.org", nil)
	if checkCorsPreflight(req) {
		t.Fatalf("checkCorsPreflight should return false")
	}

	req.HttpRequest.Header = make(bfe_http.Header)
	req.HttpRequest.Header.Set(HeaderOrigin, "http://hello-world.example")
	if checkCorsPreflight(req) {
		t.Fatalf("checkCorsPreflight should return false")
	}

	req.HttpRequest.Header.Set(HeaderAccessControlRequestMethod, http.MethodPut)
	if !checkCorsPreflight(req) {
		t.Fatalf("checkCorsPreflight should return true")
	}
}

func TestSetRespCorsHeader(t *testing.T) {
	allowedOrigin := "http://hello-world.example"
	exposeHeader := "X-Bfe-Test"

	m := NewModuleCors()

	req := new(bfe_basic.Request)
	req.HttpRequest, _ = bfe_http.NewRequest(http.MethodGet, "http://example.org", nil)
	req.HttpRequest.Header.Set(HeaderOrigin, allowedOrigin)
	rspHeader := make(bfe_http.Header)

	maxAge := 10
	rule := CorsRule{
		AccessControlAllowOriginMap:   map[string]bool{allowedOrigin: true},
		AccessControlAllowCredentials: true,
		AccessControlExposeHeaders:    []string{exposeHeader},
		AccessControlAllowMethods:     []string{http.MethodPut},
		AccessControlAllowHeaders:     []string{exposeHeader},
		AccessControlMaxAge:           &maxAge,
	}

	// preflight is false
	m.setRespHeaderForNonPreflight(req, rspHeader, &rule)

	if rspHeader.Get(HeaderAccessControlAllowOrigin) != allowedOrigin {
		t.Fatalf("response header %s is not expected", HeaderAccessControlAllowOrigin)
	}

	if rspHeader.Get(HeaderAccessControlAllowCredentials) != "true" {
		t.Fatalf("response header %s is not expected", HeaderAccessControlAllowCredentials)
	}

	if rspHeader.Get(HeaderAccessControlExposeHeaders) != exposeHeader {
		t.Fatalf("response header %s is not expected", HeaderAccessControlExposeHeaders)
	}

	if rspHeader.Get(HeaderVary) != HeaderOrigin {
		t.Fatalf("response header %s is not expected", HeaderVary)
	}

	if len(rspHeader.Get(HeaderAccessControlAllowMethods)) != 0 {
		t.Fatalf("response header %s is not expected", HeaderAccessControlAllowMethods)
	}

	if len(rspHeader.Get(HeaderAccessControlMaxAge)) != 0 {
		t.Fatalf("response header %s is not expected", HeaderAccessControlMaxAge)
	}

	// preflight is true
	m.setRespHeaderForPreflght(req, rspHeader, &rule)

	if rspHeader.Get(HeaderAccessControlAllowMethods) != http.MethodPut {
		t.Fatalf("response header %s is not expected", HeaderAccessControlAllowMethods)
	}

	if rspHeader.Get(HeaderAccessControlMaxAge) != "10" {
		t.Fatalf("response header %s is not expected", HeaderAccessControlMaxAge)
	}
}
