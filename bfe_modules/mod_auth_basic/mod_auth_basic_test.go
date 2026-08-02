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

package mod_auth_basic

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func TestNewModuleAuthBasic(t *testing.T) {
	m := NewModuleAuthBasic()
	if m == nil {
		t.Fatalf("NewModuleAuthBasic() should not return nil")
	}
	if m.Name() != ModAuthBasic {
		t.Errorf("Name() should be %s, not %s", ModAuthBasic, m.Name())
	}
	if m.ruleTable == nil {
		t.Errorf("ruleTable should not be nil")
	}
}

func TestModuleAuthBasicInitSuccess(t *testing.T) {
	m := NewModuleAuthBasic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if m.configPath == "" {
		t.Errorf("configPath should not be empty")
	}

	if !strings.HasSuffix(m.configPath, "testdata/mod_auth_basic/auth_basic_rule.data") {
		t.Errorf("configPath should end with testdata/mod_auth_basic/auth_basic_rule.data, not %s",
			m.configPath)
	}
}

func TestModuleAuthBasicInitConfNotExist(t *testing.T) {
	m := NewModuleAuthBasic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata_not_exist")
	if err == nil {
		t.Errorf("Init() should return error when config file not exist")
	}
}

func TestModuleAuthBasicInitDataLoadError(t *testing.T) {
	confRoot := "./testdata_init_fail"
	if err := os.MkdirAll(filepath.Join(confRoot, "mod_auth_basic"), 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	defer os.RemoveAll(confRoot)

	confContent := "[basic]\nDataPath = mod_auth_basic/missing.data\n"
	confPath := filepath.Join(confRoot, "mod_auth_basic", "mod_auth_basic.conf")
	if err := ioutil.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := NewModuleAuthBasic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, confRoot)
	if err == nil {
		t.Errorf("Init() should return error when data file load fails")
	}
}

func TestModuleAuthBasicLoadConfData(t *testing.T) {
	m := NewModuleAuthBasic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// reload with default path
	if err := m.loadConfData(nil); err != nil {
		t.Errorf("loadConfData(nil) error: %v", err)
	}

	// reload with explicit path
	values := url.Values{}
	values.Set("path", "./testdata/mod_auth_basic/auth_basic_rule.data")
	if err := m.loadConfData(values); err != nil {
		t.Errorf("loadConfData(path) error: %v", err)
	}

	// reload with invalid path
	values.Set("path", "./testdata/mod_auth_basic/not_exist.data")
	if err := m.loadConfData(values); err == nil {
		t.Errorf("loadConfData(invalid path) should return error")
	}
}

func TestModuleAuthBasicCheckAuthCredentials(t *testing.T) {
	m := NewModuleAuthBasic()

	rule := &AuthBasicRule{
		UserPasswd: map[string]string{
			"unittest":  "$apr1$mI7SilJz$CWwYJyYKbhVDNl26sdUSh/",
			"unittest2": "{SHA}fEqNCco3Yq9h5ZUglD3CZJT4lBs=",
		},
		Realm: "unittest",
	}

	buildReq := func(authHeader string) *bfe_basic.Request {
		req := new(bfe_basic.Request)
		req.Session = new(bfe_basic.Session)
		req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org", nil)
		if authHeader != "" {
			req.HttpRequest.Header.Set("Authorization", authHeader)
		}
		return req
	}

	// no auth header
	req := buildReq("")
	if m.checkAuthCredentials(req, rule) {
		t.Errorf("checkAuthCredentials() should return false when no auth header")
	}

	// wrong username
	req = buildReq("Basic d3Jvbmd1c2VyOjEyMzQ1Ng==")
	if m.checkAuthCredentials(req, rule) {
		t.Errorf("checkAuthCredentials() should return false when username not exists")
	}

	// wrong password
	req = buildReq("Basic dW5pdHRlc3Q6d3JvbmdwYXNz")
	if m.checkAuthCredentials(req, rule) {
		t.Errorf("checkAuthCredentials() should return false when password is wrong")
	}

	// correct apr1 password
	req = buildReq("Basic dW5pdHRlc3Q6MTIzNDU2")
	if !m.checkAuthCredentials(req, rule) {
		t.Errorf("checkAuthCredentials() should return true when password is correct")
	}

	// correct sha password
	req = buildReq("Basic dW5pdHRlc3QyOjEyMzQ1Ng==")
	if !m.checkAuthCredentials(req, rule) {
		t.Errorf("checkAuthCredentials() should return true when password is correct")
	}
}

func TestModuleAuthBasicCreateUnauthorizedResp(t *testing.T) {
	m := NewModuleAuthBasic()

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org", nil)

	rule := &AuthBasicRule{
		Realm: "unittest",
	}

	resp := m.createUnauthorizedResp(req, rule)
	if resp.StatusCode != bfe_http.StatusUnauthorized {
		t.Errorf("status code should be %d, not %d", bfe_http.StatusUnauthorized, resp.StatusCode)
	}

	expected := "Basic realm=\"unittest\""
	if resp.Header.Get("WWW-Authenticate") != expected {
		t.Errorf("WWW-Authenticate should be %s, not %s", expected, resp.Header.Get("WWW-Authenticate"))
	}
}

func TestModuleAuthBasicHandlerNoMatchedProduct(t *testing.T) {
	m := NewModuleAuthBasic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = "not_exist"
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org", nil)

	ret, resp := m.authBasicHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Errorf("resp should be nil when no product matched")
	}
}

func TestModuleAuthBasicHandlerNoMatchedCond(t *testing.T) {
	m := NewModuleAuthBasic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = "unittest"
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org", nil)
	req.HttpRequest.Host = "example.org"

	ret, resp := m.authBasicHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Errorf("resp should be nil when no condition matched")
	}
}

func TestModuleAuthBasicHandlerUnauthorized(t *testing.T) {
	m := NewModuleAuthBasic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = "unittest"
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org", nil)

	ret, resp := m.authBasicHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
		return
	}
	if resp.StatusCode != bfe_http.StatusUnauthorized {
		t.Errorf("status code should be %d, not %d", bfe_http.StatusUnauthorized, resp.StatusCode)
	}
}

func TestModuleAuthBasicHandlerSuccess(t *testing.T) {
	m := NewModuleAuthBasic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = "unittest"
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org", nil)
	req.HttpRequest.Header.Set("Authorization", "Basic dW5pdHRlc3Q6MTIzNDU2")

	ret, resp := m.authBasicHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Errorf("resp should be nil when auth success")
	}
}

func TestModuleAuthBasicHandlerWrongPassword(t *testing.T) {
	m := NewModuleAuthBasic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = "unittest"
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org", nil)
	req.HttpRequest.Header.Set("Authorization", "Basic dW5pdHRlc3Q6d3JvbmdwYXNz")

	ret, resp := m.authBasicHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
		return
	}
	if resp.StatusCode != bfe_http.StatusUnauthorized {
		t.Errorf("status code should be %d, not %d", bfe_http.StatusUnauthorized, resp.StatusCode)
	}
}

func TestAuthStaticFileHandler(t *testing.T) {
	m := NewModuleAuthBasic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Errorf("Init() error: %v", err)
		return
	}

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = "unittest"
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org", nil)
	ret, resp := m.authBasicHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
		return
	}
	if resp.StatusCode != bfe_http.StatusUnauthorized {
		t.Errorf("status code should be %d, not %d", bfe_http.StatusUnauthorized, resp.StatusCode)
		return
	}

	req.HttpRequest.Header.Set("Authorization", "Basic dW5pdHRlc3Q6MTIzNDU2")
	ret, _ = m.authBasicHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}

	req.HttpRequest.Header.Set("Authorization", "Basic dW5pdHRlc3QyOjEyMzQ1Ng==")
	ret, _ = m.authBasicHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}

	req.HttpRequest.Host = "example.org"
	ret, _ = m.authBasicHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
}
