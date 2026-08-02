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

package mod_header

import (
	"io/ioutil"
	"net/url"
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

func TestModuleHeaderInitConfNotExist(t *testing.T) {
	m := NewModuleHeader()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata_not_exist")
	if err == nil {
		t.Error("Init() should return error when config not exist")
	}
}

func TestModuleHeaderInitDataLoadError(t *testing.T) {
	// create a temp conf root with a conf file pointing to missing data
	dir := t.TempDir()
	confPath := bfe_module.ModConfPath(dir, "mod_header")

	content := "[basic]\nDataPath = mod_header/missing.data\n"
	if err := writeFile(confPath, content); err != nil {
		t.Fatalf("writeFile error: %v", err)
	}

	m := NewModuleHeader()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, dir)
	if err == nil {
		t.Error("Init() should return error when data file load fails")
	}
}

func TestModuleHeaderInitAddFilterError(t *testing.T) {
	m := NewModuleHeader()
	// callbacks map is nil, so AddFilter will fail for HandleAfterLocation
	cb := &bfe_module.BfeCallbacks{}
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata")
	if err == nil {
		t.Error("Init() should return error when AddFilter fails")
	}
}

func TestModuleHeaderInitRegisterHandlerError(t *testing.T) {
	m := NewModuleHeader()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	// register twice to trigger duplicate handler error
	_ = m.Init(cb, wh, "./testdata")
	err := m.Init(cb, wh, "./testdata")
	if err == nil {
		t.Error("Init() should return error when RegisterHandler fails")
	}
}

func TestModuleHeaderLoadConfData(t *testing.T) {
	m := NewModuleHeader()
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
	values.Set("path", "./testdata/mod_header/header_rule.data")
	if err := m.loadConfData(values); err != nil {
		t.Errorf("loadConfData(path) error: %v", err)
	}

	// reload with invalid path
	values.Set("path", "./testdata/mod_header/not_exist.data")
	if err := m.loadConfData(values); err == nil {
		t.Error("loadConfData(invalid path) should return error")
	}
}

func TestModuleHeaderReqHeaderHandlerDisableDefault(t *testing.T) {
	m := NewModuleHeader()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	m.disableDefaultHeader = true

	req := makeBasicRequest("http://www.example.org")
	req.Route.Product = "pb"

	ret, _ := m.reqHeaderHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}

	// default headers should not be set
	if req.HttpRequest.Header.Get(bfe_basic.HeaderForwardedHost) != "" {
		t.Error("default header should not be set when disabled")
	}
}

func TestModuleHeaderApplyProductRuleNotFound(t *testing.T) {
	m := NewModuleHeader()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := makeBasicRequest("http://www.example.org")
	req.Route.Product = "not_exist"

	m.applyProductRule(req, ReqHeader, req.Route.Product)
	// should not panic and should not modify headers
	if req.HttpRequest.Header.Get("Header_Add_Test") != "" {
		t.Error("product not found should not modify headers")
	}
}

func TestModuleHeaderRspHeaderHandler(t *testing.T) {
	m := NewModuleHeader()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := makeBasicRequest("http://www.example.org")
	req.Route.Product = "p2"
	req.HttpResponse = &bfe_http.Response{
		Header: make(bfe_http.Header),
	}

	ret := m.rspHeaderHandler(req, req.HttpResponse)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestApplyProductRuleOpenDebug(t *testing.T) {
	old := openDebug
	openDebug = true
	defer func() { openDebug = old }()

	m := NewModuleHeader()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := makeBasicRequest("http://www.example.org")
	req.Route.Product = "pb"
	m.applyProductRule(req, ReqHeader, req.Route.Product)
}

func TestSetDefaultHeaderOpenDebug(t *testing.T) {
	old := openDebug
	openDebug = true
	defer func() { openDebug = old }()

	m := NewModuleHeader()
	req := makeDefaultHeaderRequest()
	m.setDefaultHeader(req)
}

func TestDoHeaderLast(t *testing.T) {
	cond := "default_t()"
	c, err := condition.Build(cond)
	if err != nil {
		t.Fatalf("condition build error: %v", err)
	}

	list := RuleList{
		{
			Cond:    c,
			Actions: []Action{{Cmd: "REQ_HEADER_SET", Params: []string{"X-First", "1"}}},
			Last:    true,
		},
		{
			Cond:    c,
			Actions: []Action{{Cmd: "REQ_HEADER_SET", Params: []string{"X-Second", "2"}}},
			Last:    false,
		},
	}

	req := makeBasicRequest("http://www.example.org")
	DoHeader(req, ReqHeader, &list)

	if req.HttpRequest.Header.Get("X-First") != "1" {
		t.Error("first rule should run")
	}
	if req.HttpRequest.Header.Get("X-Second") != "" {
		t.Error("second rule should not run due to last=true")
	}
}

func TestDoHeaderCondNotMatch(t *testing.T) {
	cond := "req_path_prefix_in(\"/notmatch\", false)"
	c, err := condition.Build(cond)
	if err != nil {
		t.Fatalf("condition build error: %v", err)
	}

	list := RuleList{
		{
			Cond:    c,
			Actions: []Action{{Cmd: "REQ_HEADER_SET", Params: []string{"X-Key", "v"}}},
			Last:    false,
		},
	}

	req := makeBasicRequest("http://www.example.org")
	DoHeader(req, ReqHeader, &list)

	if req.HttpRequest.Header.Get("X-Key") != "" {
		t.Error("rule should not run when condition does not match")
	}
}

func writeFile(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return ioutil.WriteFile(path, []byte(content), 0644)
}
