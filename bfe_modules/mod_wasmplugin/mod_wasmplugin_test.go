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

package mod_wasmplugin

import (
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
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_wasmplugin"
)

func newBasicRequest() *bfe_basic.Request {
	req, _ := bfe_http.NewRequest("GET", "http://www.example.org", nil)
	return &bfe_basic.Request{
		HttpRequest: req,
		Context:     make(map[interface{}]interface{}),
	}
}

func TestNewModuleWasm(t *testing.T) {
	m := NewModuleWasm()
	if m == nil {
		t.Fatalf("NewModuleWasm() should not return nil")
	}

	if m.Name() != ModWasm {
		t.Errorf("Name() should be %s, not %s", ModWasm, m.Name())
	}

	if m.pluginTable == nil {
		t.Errorf("pluginTable should not be nil")
	}
}

func TestModuleWasmInitSuccess(t *testing.T) {
	m := NewModuleWasm()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if m.wasmPluginPath == "" {
		t.Errorf("wasmPluginPath should not be empty")
	}

	if m.configPath == "" {
		t.Errorf("configPath should not be empty")
	}

	if !strings.HasSuffix(m.configPath, "testdata/mod_wasm/wasm.data") {
		t.Errorf("configPath should end with testdata/mod_wasmplugin/wasm.data, not %s", m.configPath)
	}
}

func TestModuleWasmInitConfNotExist(t *testing.T) {
	m := NewModuleWasm()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata_not_exist")
	if err == nil {
		t.Errorf("Init() should return error when config file not exist")
	}
}

func TestModuleWasmInitDataLoadError(t *testing.T) {
	confRoot := "./testdata_init_fail"
	if err := os.MkdirAll(filepath.Join(confRoot, "mod_wasmplugin"), 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	defer os.RemoveAll(confRoot)

	confContent := "[basic]\nDataPath = mod_wasmplugin/missing.data\n"
	confPath := filepath.Join(confRoot, "mod_wasmplugin", "mod_wasmplugin.conf")
	if err := os.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := NewModuleWasm()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, confRoot)
	if err == nil {
		t.Errorf("Init() should return error when data file load fails")
	}
}

func TestModuleWasmLoadConfData(t *testing.T) {
	m := NewModuleWasm()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if err := m.loadConfData(nil); err != nil {
		t.Errorf("loadConfData(nil) error: %v", err)
	}

	values := url.Values{}
	values.Set("path", "./testdata/mod_wasm/wasm.data")
	if err := m.loadConfData(values); err != nil {
		t.Errorf("loadConfData(valid path) error: %v", err)
	}

	values.Set("path", "./testdata/mod_wasm/not_exist.data")
	if err := m.loadConfData(values); err == nil {
		t.Errorf("loadConfData(invalid path) should return error")
	}
}

func TestWasmBeforeLocationHandlerNoMatch(t *testing.T) {
	m := NewModuleWasm()

	req := newBasicRequest()
	ret, resp := m.wasmBeforeLocationHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Errorf("resp should be nil when no rule matched")
	}
}

func TestWasmBeforeLocationHandlerMatchEmptyPlugins(t *testing.T) {
	m := NewModuleWasm()

	cond, err := condition.Build("default_t()")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}

	rule := FilterRule{
		Cond:       cond,
		PluginList: []bfe_wasmplugin.WasmPlugin{},
	}
	m.pluginTable.Update("v1", RuleList{rule}, nil, nil)

	req := newBasicRequest()
	ret, resp := m.wasmBeforeLocationHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Errorf("resp should be nil")
	}

	val, ok := req.Context[ModWasmBeforeLocationKey]
	if !ok {
		t.Fatalf("context should contain ModWasmBeforeLocationKey")
	}
	if _, ok := val.([]*bfe_wasmplugin.Filter); !ok {
		t.Errorf("context value should be []*bfe_wasmplugin.Filter")
	}
}

func TestWasmRequestHandlerNoMatch(t *testing.T) {
	m := NewModuleWasm()

	req := newBasicRequest()
	req.Route.Product = "prod1"

	ret, resp := m.wasmRequestHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Errorf("resp should be nil when no rule matched")
	}
}

func TestWasmRequestHandlerMatchEmptyPlugins(t *testing.T) {
	m := NewModuleWasm()

	cond, err := condition.Build("default_t()")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}

	rule := FilterRule{
		Cond:       cond,
		PluginList: []bfe_wasmplugin.WasmPlugin{},
	}
	m.pluginTable.Update("v1", nil, ProductRules{"prod1": RuleList{rule}}, nil)

	req := newBasicRequest()
	req.Route.Product = "prod1"

	ret, resp := m.wasmRequestHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Errorf("resp should be nil")
	}

	val, ok := req.Context[ModWasmBeforeLocationKey]
	if !ok {
		t.Fatalf("context should contain ModWasmBeforeLocationKey")
	}
	if _, ok := val.([]*bfe_wasmplugin.Filter); !ok {
		t.Errorf("context value should be []*bfe_wasmplugin.Filter")
	}
}

func TestWasmResponseHandlerNoContext(t *testing.T) {
	m := NewModuleWasm()
	req := newBasicRequest()

	ret := m.wasmResponseHandler(req, nil)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
}

func TestWasmResponseHandlerBadContextType(t *testing.T) {
	m := NewModuleWasm()
	req := newBasicRequest()
	req.Context[ModWasmBeforeLocationKey] = "not a filter list"

	ret := m.wasmResponseHandler(req, nil)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
}

func TestWasmResponseHandlerEmptyFilters(t *testing.T) {
	m := NewModuleWasm()
	req := newBasicRequest()
	req.Context[ModWasmBeforeLocationKey] = []*bfe_wasmplugin.Filter{}

	ret := m.wasmResponseHandler(req, nil)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
}
