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

package mod_secure_link

import (
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const secureLinkRuleData = `{
	"Version": "v1",
	"Config": {
		"p1": [{
			"cond": "default_t()",
			"ChecksumKey": "sign",
			"ExpiresKey": "time",
			"ExpressionNodes": [
				{"Type": "query", "Param": "time"},
				{"Type": "uri"},
				{"Type": "remote_addr"},
				{"Type": "label", "Param": " secret"}
			]
		}]
	}
}`

func prepareTempConfRoot(t *testing.T, missingData bool) string {
	t.Helper()

	dir := strings.ReplaceAll(t.TempDir(), "\\", "/")
	modDir := dir + "/mod_secure_link"
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	confContent := `[basic]
DataPath = mod_secure_link/secure_link.data

[log]
OpenDebug = true
`
	if err := os.WriteFile(modDir+"/mod_secure_link.conf", []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if !missingData {
		if err := os.WriteFile(modDir+"/secure_link.data", []byte(secureLinkRuleData), 0644); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}
	}

	return dir
}

func emptyCallbacks(t *testing.T) *bfe_module.BfeCallbacks {
	t.Helper()
	cb := bfe_module.NewBfeCallbacks()
	v := reflect.ValueOf(cb).Elem().FieldByName("callbacks")
	v = reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
	v.Set(reflect.MakeMap(v.Type()))
	return cb
}

func TestModuleSecureLinkInitAddFilterError(t *testing.T) {
	confRoot := prepareTempConfRoot(t, false)
	m := NewModuleSecureLink()
	cb := emptyCallbacks(t)
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, confRoot); err == nil {
		t.Errorf("Init() want error when AddFilter fails")
	}
}

func TestModuleSecureLinkName(t *testing.T) {
	m := NewModuleSecureLink()
	if m.Name() != "mod_secure_link" {
		t.Errorf("Name() want mod_secure_link, got %s", m.Name())
	}
}

func TestModuleSecureLinkInitSuccess(t *testing.T) {
	confRoot := prepareTempConfRoot(t, false)
	m := NewModuleSecureLink()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if !strings.HasSuffix(m.configPath, "mod_secure_link/secure_link.data") {
		t.Errorf("configPath should end with mod_secure_link/secure_link.data, got %s", m.configPath)
	}
}

func TestModuleSecureLinkInitConfNotExist(t *testing.T) {
	m := NewModuleSecureLink()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata_not_exist"); err == nil {
		t.Errorf("Init() want error when config file not exist")
	}
}

func TestModuleSecureLinkInitRegisterHandlerError(t *testing.T) {
	confRoot := prepareTempConfRoot(t, false)
	m := NewModuleSecureLink()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	// Pre-register the reload handler to force Init's RegisterHandler to fail.
	err := wh.RegisterHandler(web_monitor.WebHandleReload, "mod_secure_link", func(url.Values) error { return nil })
	if err != nil {
		t.Fatalf("RegisterHandler() error: %v", err)
	}

	if err := m.Init(cb, wh, confRoot); err == nil {
		t.Errorf("Init() want error when reload handler already registered")
	}
}

func TestModuleSecureLinkInitDataLoadError(t *testing.T) {
	confRoot := prepareTempConfRoot(t, true)
	m := NewModuleSecureLink()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, confRoot); err == nil {
		t.Errorf("Init() want error when data file load fails")
	}
}

func TestModuleSecureLinkLoadConfData(t *testing.T) {
	confRoot := prepareTempConfRoot(t, false)
	m := NewModuleSecureLink()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// reload with the default config path
	if err := m.loadConfData(nil); err != nil {
		t.Errorf("loadConfData(nil) error: %v", err)
	}

	// reload with an explicit path
	values := url.Values{}
	values.Set("path", "testdata/mod_secure_link/secure_link_rule.data")
	if err := m.loadConfData(values); err != nil {
		t.Errorf("loadConfData(path) error: %v", err)
	}

	// reload with an invalid path
	values.Set("path", "testdata/mod_secure_link/not_exist.data")
	if err := m.loadConfData(values); err == nil {
		t.Errorf("loadConfData(invalid path) want error")
	}
}

func newModuleWithRuleData(t *testing.T, dataPath string) *ModuleSecureLink {
	t.Helper()

	m := NewModuleSecureLink()
	if err := m.loadConfData(url.Values{
		"path": {dataPath},
	}); err != nil {
		t.Fatalf("loadConfData(%s) error: %v", dataPath, err)
	}
	return m
}

func buildRequest(product string, query url.Values) *bfe_basic.Request {
	return &bfe_basic.Request{
		HttpRequest: &bfe_http.Request{
			Host:       "example.com",
			RequestURI: "/a/b",
			RemoteAddr: "127.0.0.1",
			Header:     bfe_http.Header{},
		},
		Query: query,
		Route: bfe_basic.RequestRoute{
			Product: product,
		},
	}
}

func TestValidateHandlerNoProduct(t *testing.T) {
	m := newModuleWithRuleData(t, "testdata/mod_secure_link/secure_link_rule.data")
	req := buildRequest("not_exist", url.Values{})

	ret, resp := m.validateHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("validateHandler() want %d, got %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Errorf("validateHandler() resp should be nil when product not found")
	}
}

func TestValidateHandlerNoCondMatch(t *testing.T) {
	dir := strings.ReplaceAll(t.TempDir(), "\\", "/")
	dataPath := dir + "/no_match.data"
	dataContent := `{
		"Version": "v1",
		"Config": {
			"p1": [{
				"cond": "req_host_in(\"other.com\")",
				"ChecksumKey": "sign",
				"ExpiresKey": "time",
				"ExpressionNodes": [
					{"Type": "query", "Param": "time"},
					{"Type": "uri"},
					{"Type": "remote_addr"},
					{"Type": "label", "Param": " secret"}
				]
			}]
		}
	}`
	if err := os.WriteFile(dataPath, []byte(dataContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := newModuleWithRuleData(t, dataPath)
	req := buildRequest("p1", url.Values{
		"sign": []string{"x"},
		"time": []string{"9999999999"},
	})

	ret, resp := m.validateHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("validateHandler() want %d, got %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Errorf("validateHandler() resp should be nil when no condition matched")
	}
}

func TestValidateHandlerWithoutExpiresKey(t *testing.T) {
	m := newModuleWithRuleData(t, "testdata/mod_secure_link/secure_link_rule.data")
	req := buildRequest("p1", url.Values{
		"sign": []string{"x"},
	})

	before := m.state.ReqWithoutExpiresKey.Get()
	ret, resp := m.validateHandler(req)
	after := m.state.ReqWithoutExpiresKey.Get()

	if ret != bfe_module.BfeHandlerResponse {
		t.Errorf("validateHandler() want %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp == nil || resp.StatusCode != 403 {
		t.Errorf("validateHandler() want 403 response")
	}
	if after != before+1 {
		t.Errorf("ReqWithoutExpiresKey counter should increment")
	}
}

func TestValidateHandlerInvalidExpiresValue(t *testing.T) {
	m := newModuleWithRuleData(t, "testdata/mod_secure_link/secure_link_rule.data")
	req := buildRequest("p1", url.Values{
		"sign": []string{"x"},
		"time": []string{"abc"},
	})

	before := m.state.ReqInvalidExpiresValue.Get()
	ret, resp := m.validateHandler(req)
	after := m.state.ReqInvalidExpiresValue.Get()

	if ret != bfe_module.BfeHandlerResponse {
		t.Errorf("validateHandler() want %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp == nil || resp.StatusCode != 403 {
		t.Errorf("validateHandler() want 403 response")
	}
	if after != before+1 {
		t.Errorf("ReqInvalidExpiresValue counter should increment")
	}
}

func TestValidateHandlerWithoutChecksumKey(t *testing.T) {
	m := newModuleWithRuleData(t, "testdata/mod_secure_link/secure_link_rule.data")
	req := buildRequest("p2", url.Values{
		"time": []string{"9999999999"},
	})

	before := m.state.ReqWithoutChecksumKey.Get()
	ret, resp := m.validateHandler(req)
	after := m.state.ReqWithoutChecksumKey.Get()

	if ret != bfe_module.BfeHandlerResponse {
		t.Errorf("validateHandler() want %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp == nil || resp.StatusCode != 403 {
		t.Errorf("validateHandler() want 403 response")
	}
	if after != before+1 {
		t.Errorf("ReqWithoutChecksumKey counter should increment")
	}
}

func TestValidateHandlerValidNoExpires(t *testing.T) {
	m := newModuleWithRuleData(t, "testdata/mod_secure_link/secure_link_rule.data")

	// p2 has no ExpiresKey. Compute the expected checksum for the expression
	// (empty time query) + uri + remote_addr + label " secret".
	checker, err := NewChecker(&CheckerConfig{
		ChecksumKey: "md5",
		ExpressionNodes: []ExpressionNodeFile{
			{Type: "query", Param: "time"},
			{Type: "uri"},
			{Type: "remote_addr"},
			{Type: "label", Param: " secret"},
		},
	})
	if err != nil {
		t.Fatalf("NewChecker() error: %v", err)
	}
	want := checker.encode("/a/b127.0.0.1 secret")

	req := buildRequest("p2", url.Values{
		"md5": []string{want},
	})

	ret, resp := m.validateHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("validateHandler() want %d, got %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Errorf("validateHandler() resp should be nil when checksum is valid")
	}
}
