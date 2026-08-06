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

package mod_errors

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

func TestNewModuleErrors(t *testing.T) {
	m := NewModuleErrors()
	if m == nil {
		t.Fatalf("NewModuleErrors() should not return nil")
	}
	if m.Name() != "mod_errors" {
		t.Errorf("Name() should be mod_errors, not %s", m.Name())
	}
	if m.ruleTable == nil {
		t.Errorf("ruleTable should not be nil")
	}
}

func TestModuleErrorsInitSuccess(t *testing.T) {
	m := NewModuleErrors()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if m.configPath == "" {
		t.Errorf("configPath should not be empty")
	}

	if !strings.HasSuffix(m.configPath, "testdata/mod_errors/errors_rule.data") {
		t.Errorf("configPath should end with testdata/mod_errors/errors_rule.data, not %s", m.configPath)
	}
}

func TestModuleErrorsInitConfNotExist(t *testing.T) {
	m := NewModuleErrors()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata_not_exist")
	if err == nil {
		t.Errorf("Init() should return error when config file not exist")
	}
}

func TestModuleErrorsInitDataLoadError(t *testing.T) {
	confRoot := "./testdata_init_fail"
	if err := os.MkdirAll(filepath.Join(confRoot, "mod_errors"), 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	defer os.RemoveAll(confRoot)

	confContent := "[basic]\nDataPath = mod_errors/missing.data\n"
	confPath := filepath.Join(confRoot, "mod_errors", "mod_errors.conf")
	if err := ioutil.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := NewModuleErrors()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, confRoot)
	if err == nil {
		t.Errorf("Init() should return error when data file load fails")
	}
}

func TestModuleErrorsLoadConfData(t *testing.T) {
	m := NewModuleErrors()
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
	values.Set("path", "./testdata/mod_errors/errors_rule.data")
	if err := m.loadConfData(values); err != nil {
		t.Errorf("loadConfData(path) error: %v", err)
	}

	// reload with invalid path
	values.Set("path", "./testdata/mod_errors/not_exist.data")
	if err := m.loadConfData(values); err == nil {
		t.Errorf("loadConfData(invalid path) should return error")
	}
}

func TestModuleErrorsHandlerNoResponse(t *testing.T) {
	m := NewModuleErrors()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := &bfe_basic.Request{
		Session:      new(bfe_basic.Session),
		HttpRequest:  new(bfe_http.Request),
		Route:        bfe_basic.RequestRoute{Product: "example"},
	}

	ret := m.errorsHandler(req, nil)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("errorsHandler() should return BfeHandlerGoOn, not %d", ret)
	}
}

func TestModuleErrorsHandlerNoMatchedProduct(t *testing.T) {
	m := NewModuleErrors()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := &bfe_basic.Request{
		Session:      new(bfe_basic.Session),
		HttpRequest:  new(bfe_http.Request),
		Route:        bfe_basic.RequestRoute{Product: "not_exist"},
		HttpResponse: &bfe_http.Response{StatusCode: 404},
	}

	ret := m.errorsHandler(req, req.HttpResponse)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("errorsHandler() should return BfeHandlerGoOn, not %d", ret)
	}
}

func TestModuleErrorsHandlerNoMatchedCond(t *testing.T) {
	m := NewModuleErrors()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	resp := &bfe_http.Response{
		StatusCode: 403,
		Header:     make(bfe_http.Header),
	}
	req := &bfe_basic.Request{
		Session:      new(bfe_basic.Session),
		HttpRequest:  new(bfe_http.Request),
		Route:        bfe_basic.RequestRoute{Product: "example"},
		HttpResponse: resp,
	}

	ret := m.errorsHandler(req, resp)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("errorsHandler() should return BfeHandlerGoOn, not %d", ret)
	}
	if resp.StatusCode != 403 {
		t.Errorf("StatusCode should keep 403, not %d", resp.StatusCode)
	}
}

func TestModuleErrorsHandlerReturn(t *testing.T) {
	m := NewModuleErrors()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	resp := &bfe_http.Response{
		StatusCode: 404,
		Header:     make(bfe_http.Header),
		Body:       ioutil.NopCloser(strings.NewReader("origin")),
	}
	req := &bfe_basic.Request{
		Session:      new(bfe_basic.Session),
		HttpRequest:  new(bfe_http.Request),
		Route:        bfe_basic.RequestRoute{Product: "example"},
		HttpResponse: resp,
	}

	ret := m.errorsHandler(req, resp)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("errorsHandler() should return BfeHandlerGoOn, not %d", ret)
	}

	if resp.StatusCode != 200 {
		t.Errorf("StatusCode should be 200, not %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "text/html" {
		t.Errorf("Content-Type should be text/html, not %s", resp.Header.Get("Content-Type"))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	if string(body) != "ERROR" {
		t.Errorf("body should be ERROR, not %s", string(body))
	}
	resp.Body.Close()
}

func TestModuleErrorsHandlerRedirect(t *testing.T) {
	m := NewModuleErrors()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	resp := &bfe_http.Response{
		StatusCode: 500,
		Header:     make(bfe_http.Header),
		Body:       ioutil.NopCloser(strings.NewReader("origin")),
	}
	req := &bfe_basic.Request{
		Session:      new(bfe_basic.Session),
		HttpRequest:  new(bfe_http.Request),
		Route:        bfe_basic.RequestRoute{Product: "example"},
		HttpResponse: resp,
	}

	ret := m.errorsHandler(req, resp)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("errorsHandler() should return BfeHandlerGoOn, not %d", ret)
	}

	if resp.StatusCode != 302 {
		t.Errorf("StatusCode should be 302, not %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "http://example.com/error.html" {
		t.Errorf("Location should be http://example.com/error.html, not %s", resp.Header.Get("Location"))
	}
	resp.Body.Close()
}
