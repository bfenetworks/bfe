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

package mod_rewrite

import (
	"testing"

	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/action"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func newTestRequest(host, path string) *bfe_basic.Request {
	httpReq, _ := bfe_http.NewRequest("GET", "http://"+host+path, nil)
	session := &bfe_basic.Session{}
	req := bfe_basic.NewRequest(httpReq, nil, nil, session, nil)
	return req
}

func TestNewModuleReWrite(t *testing.T) {
	m := NewModuleReWrite()
	if m == nil {
		t.Fatal("NewModuleReWrite() should not return nil")
	}
	if m.Name() != "mod_rewrite" {
		t.Errorf("Name() = %s, want mod_rewrite", m.Name())
	}
	if m.ruleTable == nil {
		t.Error("ruleTable should be initialized")
	}
}

func TestModuleReWriteName(t *testing.T) {
	m := NewModuleReWrite()
	if got := m.Name(); got != "mod_rewrite" {
		t.Errorf("Name() = %s, want mod_rewrite", got)
	}
}

func TestReqReWriteMatchAndAction(t *testing.T) {
	cond, err := condition.Build(`req_host_in("www.example.org")`)
	if err != nil {
		t.Fatalf("build condition err: %s", err)
	}

	rules := RuleList{
		ReWriteRule{
			Cond: cond,
			Actions: []action.Action{
				{Cmd: action.ActionHostSet, Params: []string{"new.example.org"}},
				{Cmd: action.ActionPathSet, Params: []string{"/rewritten"}},
			},
			Last: true,
		},
	}

	req := newTestRequest("www.example.org", "/old")
	ReqReWrite(req, &rules)

	if req.HttpRequest.Host != "new.example.org" {
		t.Errorf("Host = %s, want new.example.org", req.HttpRequest.Host)
	}
	if req.HttpRequest.URL.Path != "/rewritten" {
		t.Errorf("Path = %s, want /rewritten", req.HttpRequest.URL.Path)
	}
}

func TestReqReWriteNoMatch(t *testing.T) {
	cond, err := condition.Build(`req_host_in("www.example.org")`)
	if err != nil {
		t.Fatalf("build condition err: %s", err)
	}

	rules := RuleList{
		ReWriteRule{
			Cond:    cond,
			Actions: []action.Action{{Cmd: action.ActionHostSet, Params: []string{"new.example.org"}}},
			Last:    true,
		},
	}

	req := newTestRequest("www.other.org", "/old")
	ReqReWrite(req, &rules)

	if req.HttpRequest.Host != "www.other.org" {
		t.Errorf("Host = %s, want www.other.org", req.HttpRequest.Host)
	}
}

func TestReqReWriteLastStop(t *testing.T) {
	condTrue, _ := condition.Build(`default_t()`)
	condFalse, _ := condition.Build(`req_host_in("www.never.org")`)

	rules := RuleList{
		ReWriteRule{
			Cond:    condTrue,
			Actions: []action.Action{{Cmd: action.ActionHostSet, Params: []string{"first.example.org"}}},
			Last:    true,
		},
		ReWriteRule{
			Cond:    condFalse,
			Actions: []action.Action{{Cmd: action.ActionHostSet, Params: []string{"second.example.org"}}},
			Last:    true,
		},
	}

	req := newTestRequest("www.example.org", "/")
	ReqReWrite(req, &rules)

	if req.HttpRequest.Host != "first.example.org" {
		t.Errorf("Host = %s, want first.example.org", req.HttpRequest.Host)
	}
}

func TestRewriteHandler(t *testing.T) {
	cond, _ := condition.Build(`default_t()`)
	conf := ReWriteConf{
		Version: "test-1",
		Config: ProductRules{
			"pn": &RuleList{
				ReWriteRule{
					Cond:    cond,
					Actions: []action.Action{{Cmd: action.ActionPathSet, Params: []string{"/handled"}}},
					Last:    true,
				},
			},
		},
	}

	m := NewModuleReWrite()
	m.ruleTable.Update(conf)

	req := newTestRequest("www.example.org", "/origin")
	req.Route.Product = "pn"

	status, resp := m.rewriteHandler(req)
	if status != bfe_module.BfeHandlerGoOn {
		t.Errorf("status = %d, want BfeHandlerGoOn", status)
	}
	if resp != nil {
		t.Error("resp should be nil")
	}
	if req.HttpRequest.URL.Path != "/handled" {
		t.Errorf("Path = %s, want /handled", req.HttpRequest.URL.Path)
	}
}

func TestRewriteHandlerProductNotFound(t *testing.T) {
	m := NewModuleReWrite()
	req := newTestRequest("www.example.org", "/origin")
	req.Route.Product = "not_exist"

	status, resp := m.rewriteHandler(req)
	if status != bfe_module.BfeHandlerGoOn {
		t.Errorf("status = %d, want BfeHandlerGoOn", status)
	}
	if resp != nil {
		t.Error("resp should be nil")
	}
	if req.HttpRequest.URL.Path != "/origin" {
		t.Errorf("Path should not be modified, got %s", req.HttpRequest.URL.Path)
	}
}

func TestInitSuccess(t *testing.T) {
	m := NewModuleReWrite()
	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()

	err := m.Init(cbs, whs, "./testdata/init")
	if err != nil {
		t.Fatalf("Init() err: %s", err)
	}

	if m.configPath == "" {
		t.Error("configPath should be set")
	}

	hl := cbs.GetHandlerList(bfe_module.HandleAfterLocation)
	if hl == nil {
		t.Error("handler list for HandleAfterLocation should not be nil")
	}

	_, err = whs.GetHandler(web_monitor.WebHandleReload, "mod_rewrite")
	if err != nil {
		t.Errorf("reload handler not registered: %s", err)
	}
}

func TestInitConfNotFound(t *testing.T) {
	m := NewModuleReWrite()
	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()

	err := m.Init(cbs, whs, "./testdata/not_exist")
	if err == nil {
		t.Error("Init() should fail when config file not found")
	}
}

func TestInitDataNotFound(t *testing.T) {
	m := NewModuleReWrite()
	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()

	err := m.Init(cbs, whs, "./testdata/init_bad")
	if err == nil {
		t.Error("Init() should fail when rewrite data file not found")
	}
}

func TestInitRegisterDuplicate(t *testing.T) {
	m1 := NewModuleReWrite()
	cbs1 := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()

	if err := m1.Init(cbs1, whs, "./testdata/init"); err != nil {
		t.Fatalf("first Init() err: %s", err)
	}

	m2 := NewModuleReWrite()
	cbs2 := bfe_module.NewBfeCallbacks()

	err := m2.Init(cbs2, whs, "./testdata/init")
	if err == nil {
		t.Error("Init() should fail when registering duplicate web handler")
	}
}
