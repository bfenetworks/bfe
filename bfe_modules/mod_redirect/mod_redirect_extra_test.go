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

package mod_redirect

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func TestNewModuleRedirect(t *testing.T) {
	m := NewModuleRedirect()
	if m == nil {
		t.Fatalf("NewModuleRedirect() should not return nil")
	}
	if m.Name() != "mod_redirect" {
		t.Errorf("Name() should be mod_redirect, not %s", m.Name())
	}
	if m.ruleTable == nil {
		t.Errorf("ruleTable should not be nil")
	}
}

func TestModuleRedirectInitSuccess(t *testing.T) {
	m := NewModuleRedirect()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if !strings.HasSuffix(m.configPath, "testdata/mod_redirect/redirect.json") {
		t.Errorf("configPath should end with testdata/mod_redirect/redirect.json, not %s", m.configPath)
	}
}

func TestModuleRedirectInitConfNotExist(t *testing.T) {
	m := NewModuleRedirect()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata_not_exist")
	if err == nil {
		t.Errorf("Init() should return error when config file not exist")
	}
}

func TestModuleRedirectInitDataLoadError(t *testing.T) {
	confRoot := "./testdata_init_fail"
	if err := os.MkdirAll(filepath.Join(confRoot, "mod_redirect"), 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	defer os.RemoveAll(confRoot)

	confContent := "[basic]\nDataPath = mod_redirect/missing.data\n"
	confPath := filepath.Join(confRoot, "mod_redirect", "mod_redirect.conf")
	if err := ioutil.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := NewModuleRedirect()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, confRoot)
	if err == nil {
		t.Errorf("Init() should return error when data file load fails")
	}
}

func TestLoadConfDataWithPath(t *testing.T) {
	m := NewModuleRedirect()
	m.configPath = "./testdata/redirect_1.conf"

	values := url.Values{}
	values.Set("path", "./testdata/redirect_1.conf")
	result, err := m.loadConfData(values)
	if err != nil {
		t.Fatalf("loadConfData(path) error: %v", err)
	}
	if !strings.HasPrefix(result, "redirect_1.conf=") {
		t.Errorf("loadConfData result should start with redirect_1.conf=, got %s", result)
	}
}

func TestLoadConfDataDefaultPath(t *testing.T) {
	m := NewModuleRedirect()
	m.configPath = "./testdata/redirect_1.conf"

	result, err := m.loadConfData(nil)
	if err != nil {
		t.Fatalf("loadConfData(nil) error: %v", err)
	}
	if !strings.HasPrefix(result, "redirect_1.conf=") {
		t.Errorf("loadConfData result should start with redirect_1.conf=, got %s", result)
	}
}

func TestLoadConfDataInvalidPath(t *testing.T) {
	m := NewModuleRedirect()
	m.configPath = "./testdata/redirect_1.conf"

	values := url.Values{}
	values.Set("path", "./testdata/not_exist.conf")
	if _, err := m.loadConfData(values); err == nil {
		t.Errorf("loadConfData(invalid path) should return error")
	}
}

func TestPrepareReqRedirectNoMatch(t *testing.T) {
	emptyRules := RuleList{}
	req := newBasicRequest("http://www.example.org/index/", "www.example.org")

	if PrepareReqRedirect(req, &emptyRules) {
		t.Errorf("PrepareReqRedirect() should return false when no rules match")
	}
}

func TestPrepareReqRedirectUrlSet(t *testing.T) {
	cond, _ := condition.Build("req_host_in(\"www.example.org\")")
	rules := RuleList{{
		Cond:    cond,
		Actions: []Action{{Cmd: "URL_SET", Params: []string{"http://new.example.org"}}},
		Status:  302,
	}}
	req := newBasicRequest("http://www.example.org/index/", "www.example.org")

	if !PrepareReqRedirect(req, &rules) {
		t.Errorf("PrepareReqRedirect() should return true")
		return
	}
	if req.Redirect.Url != "http://new.example.org" {
		t.Errorf("redirect url should be http://new.example.org, got %s", req.Redirect.Url)
	}
	if req.Redirect.Code != 302 {
		t.Errorf("redirect code should be 302, got %d", req.Redirect.Code)
	}
}

func TestPrepareReqRedirectUrlPrefixAdd(t *testing.T) {
	cond, _ := condition.Build("req_host_in(\"www.example.org\")")
	rules := RuleList{{
		Cond:    cond,
		Actions: []Action{{Cmd: "URL_PREFIX_ADD", Params: []string{"https://secure.example.org"}}},
		Status:  301,
	}}
	req := newBasicRequest("http://www.example.org/path?k=v", "www.example.org")

	if !PrepareReqRedirect(req, &rules) {
		t.Errorf("PrepareReqRedirect() should return true")
		return
	}
	expected := "https://secure.example.org/path?k=v"
	if req.Redirect.Url != expected {
		t.Errorf("redirect url should be %s, got %s", expected, req.Redirect.Url)
	}
}

func TestPrepareReqRedirectUrlFromQuery(t *testing.T) {
	cond, _ := condition.Build("req_host_in(\"www.example.org\")")
	rules := RuleList{{
		Cond:    cond,
		Actions: []Action{{Cmd: "URL_FROM_QUERY", Params: []string{"target"}}},
		Status:  307,
	}}
	req := newBasicRequest("http://www.example.org/?target=http://dest.example.org", "www.example.org")

	if !PrepareReqRedirect(req, &rules) {
		t.Errorf("PrepareReqRedirect() should return true")
		return
	}
	if req.Redirect.Url != "http://dest.example.org" {
		t.Errorf("redirect url should be http://dest.example.org, got %s", req.Redirect.Url)
	}
}

func TestPrepareReqRedirectSchemeSet(t *testing.T) {
	cond, _ := condition.Build("req_host_in(\"www.example.org\")")
	rules := RuleList{{
		Cond:    cond,
		Actions: []Action{{Cmd: "SCHEME_SET", Params: []string{"https"}}},
		Status:  308,
	}}
	req := newBasicRequest("http://www.example.org/path", "www.example.org")

	if !PrepareReqRedirect(req, &rules) {
		t.Errorf("PrepareReqRedirect() should return true")
		return
	}
	expected := "https://www.example.org/path"
	if req.Redirect.Url != expected {
		t.Errorf("redirect url should be %s, got %s", expected, req.Redirect.Url)
	}
}

func TestRedirectHandlerProductNotFound(t *testing.T) {
	m, err := prepareModuleRedirect()
	if err != nil {
		t.Fatalf("prepareModuleRedirect() error: %v", err)
	}

	req := newBasicRequest("http://www.example.org/index/?space=true", "www.example.org")
	req.Route.Product = "not_exist"

	ret, _ := m.redirectHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
}

func TestRedirectHandlerNoConditionMatch(t *testing.T) {
	m, err := prepareModuleRedirect()
	if err != nil {
		t.Fatalf("prepareModuleRedirect() error: %v", err)
	}

	req := newBasicRequest("http://www.example.org/other/", "www.example.org")
	req.Route.Product = "pn"

	ret, _ := m.redirectHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
}

func TestRedirectHandlerWithDebug(t *testing.T) {
	oldDebug := openDebug
	openDebug = true
	defer func() { openDebug = oldDebug }()

	m, err := prepareModuleRedirect()
	if err != nil {
		t.Fatalf("prepareModuleRedirect() error: %v", err)
	}

	req := newBasicRequest("http://www.example.org/index/?space=true", "www.example.org")
	req.Route.Product = "pn"

	ret, _ := m.redirectHandler(req)
	if ret != bfe_module.BfeHandlerRedirect {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerRedirect, ret)
	}
}

func TestReqSchemeSetHostFromRequest(t *testing.T) {
	req := newBasicRequest("http:///path", "")
	req.HttpRequest.URL.Host = ""
	ReqSchemeSet(req, "https")
	expected := "https:///path"
	if req.Redirect.Url != expected {
		t.Errorf("redirect url should be %s, got %s", expected, req.Redirect.Url)
	}
}

func newBasicRequest(urlStr string, host string) *bfe_basic.Request {
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest = new(bfe_http.Request)
	req.HttpRequest.Host = host
	req.HttpRequest.URL, _ = url.Parse(urlStr)
	return req
}
