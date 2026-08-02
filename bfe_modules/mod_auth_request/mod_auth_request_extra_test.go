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

package mod_auth_request

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestNewModuleAuthRequestAndName(t *testing.T) {
	m := NewModuleAuthRequest()
	if m == nil {
		t.Fatal("NewModuleAuthRequest() should not return nil")
	}

	if got, want := m.Name(), ModAuthRequest; got != want {
		t.Fatalf("Name() = %s, want %s", got, want)
	}
}

func TestModuleAuthRequestInit(t *testing.T) {
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	// success
	m := NewModuleAuthRequest()
	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// non-existent conf root
	m = NewModuleAuthRequest()
	if err := m.Init(cb, wh, "./testdata_not_exist"); err == nil {
		t.Fatal("Init() should fail with non-existent conf root")
	}

	// missing AuthAddress
	m = NewModuleAuthRequest()
	cr := prepareConfRoot(t, `[Basic]
DataPath = mod_auth_request/auth_request_rule.data
AuthTimeout = 100
`)
	if err := m.Init(cb, wh, cr); err == nil {
		t.Fatal("Init() should fail when AuthAddress is missing")
	}

	// invalid AuthTimeout
	m = NewModuleAuthRequest()
	cr = prepareConfRoot(t, `[Basic]
DataPath = mod_auth_request/auth_request_rule.data
AuthAddress = http://example.org
AuthTimeout = 0
`)
	if err := m.Init(cb, wh, cr); err == nil {
		t.Fatal("Init() should fail when AuthTimeout <= 0")
	}

	// invalid AuthAddress URL
	m = NewModuleAuthRequest()
	cr = prepareConfRoot(t, `[Basic]
DataPath = mod_auth_request/auth_request_rule.data
AuthAddress = "://invalid-url"
AuthTimeout = 100
`)
	if err := m.Init(cb, wh, cr); err == nil {
		t.Fatal("Init() should fail with invalid AuthAddress")
	}

	// invalid rule data path
	m = NewModuleAuthRequest()
	cr = prepareConfRoot(t, `[Basic]
DataPath = mod_auth_request/not_exist.data
AuthAddress = http://example.org
AuthTimeout = 100
`)
	if err := m.Init(cb, wh, cr); err == nil {
		t.Fatal("Init() should fail with invalid rule data path")
	}
}

func prepareConfRoot(t *testing.T, confContent string) string {
	t.Helper()

	tmpDir, err := ioutil.TempDir("", "mod_auth_request_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	modDir := filepath.Join(tmpDir, "mod_auth_request")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	confPath := filepath.Join(modDir, "mod_auth_request.conf")
	if err := ioutil.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return tmpDir
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestForwardAuthServer(t *testing.T) {
	req, _ := bfe_http.NewRequest(http.MethodGet, "http://example.org", nil)
	basicReq := bfe_basic.NewRequest(req, nil, nil, nil, nil)

	// auth service returns 200
	m := NewModuleAuthRequest()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	m.conf = &ConfModAuthRequest{}
	m.conf.Basic.AuthAddress = ts.URL

	if resp := m.forwardAuthServer(basicReq); resp != nil {
		t.Fatalf("forwardAuthServer() should return nil for 200, got %v", resp)
	}

	// auth service returns 403
	m = NewModuleAuthRequest()
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()
	m.conf = &ConfModAuthRequest{}
	m.conf.Basic.AuthAddress = ts.URL

	resp := m.forwardAuthServer(basicReq)
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("forwardAuthServer() should return 403, got %v", resp)
	}

	// auth service returns 500
	m = NewModuleAuthRequest()
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	m.conf = &ConfModAuthRequest{}
	m.conf.Basic.AuthAddress = ts.URL

	if resp := m.forwardAuthServer(basicReq); resp != nil {
		t.Fatalf("forwardAuthServer() should return nil for unexpected code, got %v", resp)
	}

	// auth service unreachable
	m = NewModuleAuthRequest()
	m.conf = &ConfModAuthRequest{}
	m.conf.Basic.AuthAddress = "http://127.0.0.1:1/"

	if resp := m.forwardAuthServer(basicReq); resp != nil {
		t.Fatalf("forwardAuthServer() should return nil on error, got %v", resp)
	}

	// mock transport error
	m = NewModuleAuthRequest()
	m.conf = &ConfModAuthRequest{}
	m.conf.Basic.AuthAddress = "http://example.org"
	m.authClient = http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("mock transport error")
		}),
	}

	if resp := m.forwardAuthServer(basicReq); resp != nil {
		t.Fatalf("forwardAuthServer() should return nil on transport error, got %v", resp)
	}
}

func TestGenAuthForbiddenResp(t *testing.T) {
	m := NewModuleAuthRequest()

	req, _ := bfe_http.NewRequest(http.MethodGet, "http://example.org", nil)
	basicReq := bfe_basic.NewRequest(req, nil, nil, nil, nil)

	resp := &http.Response{
		Header: make(http.Header),
	}

	// 403 Forbidden
	resp.StatusCode = http.StatusForbidden
	if got := m.genAuthForbiddenResp(basicReq, resp); got == nil || got.StatusCode != http.StatusForbidden {
		t.Fatalf("genAuthForbiddenResp(403) = %v, want 403 response", got)
	}

	// 500 Internal Server Error
	resp.StatusCode = http.StatusInternalServerError
	if got := m.genAuthForbiddenResp(basicReq, resp); got != nil {
		t.Fatalf("genAuthForbiddenResp(500) = %v, want nil", got)
	}
}

func TestAuthRequestHandlerRuleMatching(t *testing.T) {
	cond, err := condition.Build(`req_path_prefix_in("/auth_request", false)`)
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}

	tests := []struct {
		name       string
		product    string
		path       string
		ruleEnable bool
		authCode   int
		wantAction int
	}{
		{
			name:       "product_not_found",
			product:    "other_product",
			path:       "/auth_request",
			ruleEnable: true,
			authCode:   http.StatusForbidden,
			wantAction: bfe_module.BfeHandlerGoOn,
		},
		{
			name:       "path_not_match",
			product:    expectProduct,
			path:       "/other",
			ruleEnable: true,
			authCode:   http.StatusForbidden,
			wantAction: bfe_module.BfeHandlerGoOn,
		},
		{
			name:       "rule_disabled",
			product:    expectProduct,
			path:       "/auth_request",
			ruleEnable: false,
			authCode:   http.StatusForbidden,
			wantAction: bfe_module.BfeHandlerGoOn,
		},
		{
			name:       "rule_match_forbidden",
			product:    expectProduct,
			path:       "/auth_request",
			ruleEnable: true,
			authCode:   http.StatusForbidden,
			wantAction: bfe_module.BfeHandlerResponse,
		},
		{
			name:       "rule_match_pass",
			product:    expectProduct,
			path:       "/auth_request",
			ruleEnable: true,
			authCode:   http.StatusOK,
			wantAction: bfe_module.BfeHandlerGoOn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModuleAuthRequest()
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.authCode)
			}))
			defer ts.Close()

			m.conf = &ConfModAuthRequest{}
			m.conf.Basic.AuthAddress = ts.URL

			m.ruleTable.Update(&AuthRequestRuleConf{
				Version: "v1",
				Config: ProductRuleList{
					expectProduct: AuthRequestRuleList{
						{Cond: cond, Enable: tt.ruleEnable},
					},
				},
			})

			req, _ := bfe_http.NewRequest(http.MethodGet, "http://example.org"+tt.path, nil)
			basicReq := bfe_basic.NewRequest(req, nil, nil, new(bfe_basic.Session), nil)
			basicReq.Route.Product = tt.product

			action, resp := m.authRequestHandler(basicReq)
			if action != tt.wantAction {
				t.Fatalf("authRequestHandler() action = %d, want %d", action, tt.wantAction)
			}
			if action == bfe_module.BfeHandlerResponse && resp == nil {
				t.Fatal("authRequestHandler() should return response")
			}
			if action == bfe_module.BfeHandlerResponse && basicReq.ErrCode != ErrAuthRequest {
				t.Fatalf("ErrCode = %v, want %v", basicReq.ErrCode, ErrAuthRequest)
			}
		})
	}
}

func TestCreateAuthRequestWithForwardedHeaders(t *testing.T) {
	m := NewModuleAuthRequest()
	m.conf = &ConfModAuthRequest{}
	m.conf.Basic.AuthAddress = "http://example.org/auth_request"

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = expectProduct
	req.HttpRequest, _ = bfe_http.NewRequest(http.MethodPost, "http://example.org/foo?bar=1", nil)
	req.HttpRequest.Header.Set(XForwardedMethod, "PUT")
	req.HttpRequest.Header.Set(XForwardedURI, "/orig-uri")

	authReq := m.createAuthRequest(req)

	if got := authReq.Header.Get(XForwardedMethod); got != "PUT" {
		t.Fatalf("%s = %s, want PUT", XForwardedMethod, got)
	}
	if got := authReq.Header.Get(XForwardedURI); got != "/orig-uri" {
		t.Fatalf("%s = %s, want /orig-uri", XForwardedURI, got)
	}
}
