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

package mod_userid

import (
	"encoding/hex"
	"net/url"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

func TestNewModuleUserID(t *testing.T) {
	m := NewModuleUserID()
	if m == nil {
		t.Fatal("NewModuleUserID() returned nil")
	}
	if got, want := m.Name(), ModName; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestGenUid(t *testing.T) {
	uid := genUid()
	if uid == "" {
		t.Error("genUid() returned empty string")
	}
	if _, err := hex.DecodeString(uid); err != nil {
		t.Errorf("genUid() value %q is not valid hex: %v", uid, err)
	}
}

func TestModuleUserIDReqSetUid(t *testing.T) {
	moduleWithConfig := func(path string) *ModuleUserID {
		m := NewModuleUserID()
		if _, err := m.loadConfData(url.Values{"path": []string{path}}); err != nil {
			t.Fatalf("loadConfData(%q) failed: %v", path, err)
		}
		return m
	}

	tests := []struct {
		name      string
		m         *ModuleUserID
		req       *bfe_basic.Request
		wantCode  int
		wantResp  *bfe_http.Response
		check     func(*testing.T, *bfe_basic.Request) bool
		checkDesc string
	}{
		{
			name: "nil config",
			m: func() *ModuleUserID {
				m := NewModuleUserID()
				m.setConfig(nil)
				return m
			}(),
			req: &bfe_basic.Request{
				Context:     map[interface{}]interface{}{},
				HttpRequest: &bfe_http.Request{Header: make(bfe_http.Header)},
			},
			wantCode: bfe_module.BfeHandlerGoOn,
			check: func(t *testing.T, req *bfe_basic.Request) bool {
				return req.GetContext(UidCtxKey) == nil
			},
			checkDesc: "UidCtxKey should not be set",
		},
		{
			name: "no matching rules and no global",
			m:    moduleWithConfig("./testdata/mod_userid/userid_rule.data"),
			req: func() *bfe_basic.Request {
				req := &bfe_basic.Request{
					Context: map[interface{}]interface{}{},
					HttpRequest: &bfe_http.Request{
						Header: make(bfe_http.Header),
						URL:    &url.URL{Path: "/other"},
					},
				}
				req.Route.Product = "unknown_product"
				return req
			}(),
			wantCode: bfe_module.BfeHandlerGoOn,
			check: func(t *testing.T, req *bfe_basic.Request) bool {
				return req.GetContext(UidCtxKey) == nil
			},
			checkDesc: "UidCtxKey should not be set",
		},
		{
			name: "matching path rule",
			m:    moduleWithConfig("./testdata/mod_userid/userid_rule_global.data"),
			req: func() *bfe_basic.Request {
				req := &bfe_basic.Request{
					Context: map[interface{}]interface{}{},
					Session: &bfe_basic.Session{},
					HttpRequest: &bfe_http.Request{
						Header: make(bfe_http.Header),
						URL:    &url.URL{Path: "/abc/xyz"},
					},
					CookieMap: make(bfe_http.CookieMap),
				}
				req.Route.Product = "example_product"
				return req
			}(),
			wantCode: bfe_module.BfeHandlerGoOn,
			check: func(t *testing.T, req *bfe_basic.Request) bool {
				c, ok := req.Cookie("bfe_userid_abc")
				if !ok {
					t.Log("cookie bfe_userid_abc not found")
					return false
				}
				if c.Path != "/abc" {
					t.Logf("cookie path = %q, want /abc", c.Path)
					return false
				}
				if c.Domain != "abc.example.com" {
					t.Logf("cookie domain = %q, want abc.example.com", c.Domain)
					return false
				}
				if c.MaxAge == 0 {
					t.Log("cookie MaxAge is 0")
					return false
				}
				if req.GetContext(UidCtxKey) == nil {
					t.Log("context is nil")
					return false
				}
				return true
			},
			checkDesc: "path rule cookie should be set",
		},
		{
			name: "existing userid cookie does not generate new one",
			m:    moduleWithConfig("./testdata/mod_userid/userid_rule_global.data"),
			req: func() *bfe_basic.Request {
				req := &bfe_basic.Request{
					Context: map[interface{}]interface{}{},
					Session: &bfe_basic.Session{},
					HttpRequest: &bfe_http.Request{
						Header: make(bfe_http.Header),
						URL:    &url.URL{Path: "/abc"},
					},
				}
				req.Route.Product = "example_product"
				req.HttpRequest.AddCookie(&bfe_http.Cookie{
					Name:  "bfe_userid_abc",
					Value: "existing_value",
				})
				return req
			}(),
			wantCode: bfe_module.BfeHandlerGoOn,
			check: func(t *testing.T, req *bfe_basic.Request) bool {
				c, _ := req.Cookie("bfe_userid_abc")
				if c.Value != "existing_value" {
					t.Logf("cookie value = %q, want existing_value", c.Value)
					return false
				}
				return req.GetContext(UidCtxKey) == nil
			},
			checkDesc: "existing cookie should be kept and no context set",
		},
		{
			name: "global fallback when product has no rules",
			m:    moduleWithConfig("./testdata/mod_userid/userid_rule_global.data"),
			req: func() *bfe_basic.Request {
				req := &bfe_basic.Request{
					Context: map[interface{}]interface{}{},
					HttpRequest: &bfe_http.Request{
						Header: make(bfe_http.Header),
						URL:    &url.URL{Path: "/anything"},
					},
					CookieMap: make(bfe_http.CookieMap),
				}
				req.Route.Product = "other_product"
				return req
			}(),
			wantCode: bfe_module.BfeHandlerGoOn,
			check: func(t *testing.T, req *bfe_basic.Request) bool {
				c, ok := req.Cookie("bfe_userid")
				if !ok {
					t.Log("global cookie not found")
					return false
				}
				if c.Path != "/" {
					t.Logf("cookie path = %q, want /", c.Path)
					return false
				}
				return req.GetContext(UidCtxKey) != nil
			},
			checkDesc: "global fallback cookie should be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotResp := tt.m.reqSetUid(tt.req)
			if got != tt.wantCode {
				t.Errorf("reqSetUid() code = %v, want %v", got, tt.wantCode)
			}
			if gotResp != tt.wantResp {
				t.Errorf("reqSetUid() response = %v, want %v", gotResp, tt.wantResp)
			}
			if tt.check != nil && !tt.check(t, tt.req) {
				t.Errorf("check failed: %s", tt.checkDesc)
			}
		})
	}
}

func TestModuleUserIDRspSetUid(t *testing.T) {
	uidCookie := &bfe_http.Cookie{
		Name:  "bfe_userid",
		Value: "test_value",
	}

	tests := []struct {
		name      string
		req       *bfe_basic.Request
		resp      *bfe_http.Response
		want      int
		check     func(*testing.T, *bfe_http.Response) bool
		checkDesc string
	}{
		{
			name: "no context",
			req: &bfe_basic.Request{
				Context: map[interface{}]interface{}{},
			},
			resp: &bfe_http.Response{Header: make(bfe_http.Header)},
			want: bfe_module.BfeHandlerGoOn,
			check: func(t *testing.T, resp *bfe_http.Response) bool {
				return len(resp.Cookies()) == 0
			},
			checkDesc: "no Set-Cookie should be added",
		},
		{
			name: "context is not a cookie",
			req: &bfe_basic.Request{
				Context: map[interface{}]interface{}{
					UidCtxKey: "not a cookie",
				},
			},
			resp: &bfe_http.Response{Header: make(bfe_http.Header)},
			want: bfe_module.BfeHandlerGoOn,
			check: func(t *testing.T, resp *bfe_http.Response) bool {
				return len(resp.Cookies()) == 0
			},
			checkDesc: "no Set-Cookie should be added",
		},
		{
			name: "valid uid cookie",
			req: &bfe_basic.Request{
				Context: map[interface{}]interface{}{
					UidCtxKey: uidCookie,
				},
			},
			resp: &bfe_http.Response{Header: make(bfe_http.Header)},
			want: bfe_module.BfeHandlerGoOn,
			check: func(t *testing.T, resp *bfe_http.Response) bool {
				cs := resp.Cookies()
				if len(cs) != 1 {
					t.Logf("len(cookies) = %d, want 1", len(cs))
					return false
				}
				if cs[0].Name != uidCookie.Name || cs[0].Value != uidCookie.Value {
					t.Logf("cookie = %v, want %v", cs[0], uidCookie)
					return false
				}
				return true
			},
			checkDesc: "Set-Cookie should contain the uid cookie",
		},
	}

	m := NewModuleUserID()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.rspSetUid(tt.req, tt.resp); got != tt.want {
				t.Errorf("rspSetUid() = %v, want %v", got, tt.want)
			}
			if tt.check != nil && !tt.check(t, tt.resp) {
				t.Errorf("check failed: %s", tt.checkDesc)
			}
		})
	}
}

func TestModuleUserIDLoadConfData(t *testing.T) {
	m := NewModuleUserID()
	m.confFile = "./testdata/mod_userid/userid_rule.data"

	got, err := m.loadConfData(url.Values{})
	if err != nil {
		t.Fatalf("loadConfData() error = %v", err)
	}
	if !strings.HasPrefix(got, "userid_rule.data=") {
		t.Errorf("loadConfData() = %q, want prefix userid_rule.data=", got)
	}

	got, err = m.loadConfData(url.Values{
		"path": []string{"./testdata/mod_userid/userid_rule_global.data"},
	})
	if err != nil {
		t.Fatalf("loadConfData(query path) error = %v", err)
	}
	if !strings.HasPrefix(got, "userid_rule_global.data=2026") {
		t.Errorf("loadConfData(query path) = %q, want prefix userid_rule_global.data=2026", got)
	}

	if _, err := m.loadConfData(url.Values{
		"path": []string{"./testdata/mod_userid/userid_rule_missing.data"},
	}); err == nil {
		t.Error("loadConfData(missing path) expected error, got nil")
	}
}

func TestModuleUserIDInitFailures(t *testing.T) {
	tests := []struct {
		name string
		cr   string
	}{
		{
			name: "missing conf root",
			cr:   "./testdata/missing_root",
		},
		{
			name: "missing data file",
			cr:   "./testdata/mod_userid_bad",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cbs := bfe_module.NewBfeCallbacks()
			whs := web_monitor.NewWebHandlers()
			m := NewModuleUserID()
			if err := m.Init(cbs, whs, tt.cr); err == nil {
				t.Error("Init() expected error, got nil")
			}
		})
	}
}

func TestNewConfigFromFileFailures(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "invalid json",
			path: "./testdata/mod_userid/userid_rule_invalid_json.data",
		},
		{
			name: "bad condition",
			path: "./testdata/mod_userid/userid_rule_bad_cond.data",
		},
		{
			name: "empty params",
			path: "./testdata/mod_userid/userid_rule_empty_params.data",
		},
		{
			name: "empty product",
			path: "./testdata/mod_userid/userid_rule_empty_product.data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewConfigFromFile(tt.path); err == nil {
				t.Errorf("NewConfigFromFile(%q) expected error, got nil", tt.path)
			}
		})
	}
}
