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

package mod_block

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

const validIPBlocklist = `# {"version": "1234", "pairIPNum":1, "singleIPNum":2}
# global ip blocklist

10.1.1.0 10.1.1.254
192.168.1.100
1::1
`

const validProductRule = `{
    "Config": {
        "pn": [
            {
                "action": {
                    "cmd": "CLOSE",
                    "params": []
                },
                "cond": "req_host_in(\"n.example.org\")",
                "name": "pn_block_rule"
            }
        ],
        "pt": []
    },
    "Version": "1234"
}
`

// prepareTempConf creates a temporary conf root that contains
// mod_block/mod_block.conf and optionally mod_block/ip_blocklist.data and
// mod_block/block_rules.data. The caller does not need to clean up the
// returned directory.
func prepareTempConf(t *testing.T, ipBlocklistContent, productRuleContent string) string {
	t.Helper()

	dir, err := ioutil.TempDir("", "mod_block_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	modDir := filepath.Join(dir, "mod_block")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	if ipBlocklistContent != "" {
		if err := ioutil.WriteFile(filepath.Join(modDir, "ip_blocklist.data"), []byte(ipBlocklistContent), 0644); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}
	}

	if productRuleContent != "" {
		if err := ioutil.WriteFile(filepath.Join(modDir, "block_rules.data"), []byte(productRuleContent), 0644); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}
	}

	confContent := `[basic]
ProductRulePath = mod_block/block_rules.data
IPBlocklistPath = mod_block/ip_blocklist.data

[log]
OpenDebug = true
`
	if err := ioutil.WriteFile(filepath.Join(modDir, "mod_block.conf"), []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return dir
}

func prepareRequestForProduct() *bfe_basic.Request {
	req := new(bfe_basic.Request)
	req.HttpRequest = new(bfe_http.Request)
	req.HttpRequest.Header = make(bfe_http.Header)
	req.HttpRequest.URL = &url.URL{}
	req.Session = new(bfe_basic.Session)
	req.Context = make(map[interface{}]interface{})
	return req
}

func TestNewModuleBlockAndName(t *testing.T) {
	m := NewModuleBlock()
	if m == nil {
		t.Fatal("NewModuleBlock() returns nil")
	}

	if m.Name() != ModBlock {
		t.Errorf("Name() should be %s, got %s", ModBlock, m.Name())
	}
}

func TestModuleBlockInitSuccess(t *testing.T) {
	dir := prepareTempConf(t, validIPBlocklist, validProductRule)

	m := NewModuleBlock()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err != nil {
		t.Errorf("Init() error: %v", err)
	}
}

func TestModuleBlockInitConfNotExist(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_block_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	m := NewModuleBlock()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when config file does not exist")
	}
}

func TestModuleBlockInitBadGlobalIPTable(t *testing.T) {
	dir := prepareTempConf(t, "invalid ip blocklist content", validProductRule)

	m := NewModuleBlock()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when global ip blocklist file is invalid")
	}
}

func TestModuleBlockInitBadProductRule(t *testing.T) {
	dir := prepareTempConf(t, validIPBlocklist, "invalid product rule content")

	m := NewModuleBlock()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when product rule file is invalid")
	}
}

func TestModuleBlockGetState(t *testing.T) {
	m := NewModuleBlock()
	m.state.ReqTotal.Inc(1)

	b, err := m.getState(nil)
	if err != nil {
		t.Errorf("getState() error: %v", err)
	}
	if len(b) == 0 {
		t.Error("getState() returns empty response")
	}

	b, err = m.getStateDiff(nil)
	if err != nil {
		t.Errorf("getStateDiff() error: %v", err)
	}
	if len(b) == 0 {
		t.Error("getStateDiff() returns empty response")
	}
}

func TestLoadGlobalIPTableWithPath(t *testing.T) {
	dir := prepareTempConf(t, validIPBlocklist, validProductRule)

	m := NewModuleBlock()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// reload with explicit path parameter
	query := url.Values{}
	query.Set("path", filepath.Join(dir, "mod_block", "ip_blocklist.data"))
	if err := m.loadGlobalIPTable(query); err != nil {
		t.Errorf("loadGlobalIPTable() error: %v", err)
	}
}

func TestLoadProductRuleConfWithPath(t *testing.T) {
	dir := prepareTempConf(t, validIPBlocklist, validProductRule)

	m := NewModuleBlock()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// reload with explicit path parameter
	query := url.Values{}
	query.Set("path", filepath.Join(dir, "mod_block", "block_rules.data"))
	if err := m.loadProductRuleConf(query); err != nil {
		t.Errorf("loadProductRuleConf() error: %v", err)
	}
}

func TestProductBlockHandlerGlobalAllow(t *testing.T) {
	m := NewModuleBlock()

	cond, err := condition.Build("req_host_in(\"n.example.org\")")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}

	m.ruleTable.Update(productRuleConf{
		Version: "v1",
		Config: ProductRules{
			bfe_basic.GlobalProduct: &blockRuleList{
				{
					Cond:   cond,
					Name:   "global_allow_rule",
					Action: Action{Cmd: "ALLOW"},
				},
			},
		},
	})

	req := prepareRequestForProduct()
	req.HttpRequest.Host = "n.example.org"
	req.Route.Product = "pn"

	ret, resp := m.productBlockHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("resp should be nil for ALLOW action")
	}

	val := req.GetContext(CtxBlockInfo)
	if val == nil {
		t.Fatal("CtxBlockInfo should be set")
	}
	blockInfo, ok := val.(*BlockInfo)
	if !ok {
		t.Fatal("CtxBlockInfo should be *BlockInfo")
	}
	if blockInfo.BlockRuleName != "global_allow_rule" {
		t.Errorf("BlockRuleName should be global_allow_rule, got %s", blockInfo.BlockRuleName)
	}
}

func TestProductBlockHandlerProductAllow(t *testing.T) {
	m := NewModuleBlock()

	cond, err := condition.Build("req_host_in(\"n.example.org\")")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}

	m.ruleTable.Update(productRuleConf{
		Version: "v1",
		Config: ProductRules{
			"pn": &blockRuleList{
				{
					Cond:   cond,
					Name:   "pn_allow_rule",
					Action: Action{Cmd: "ALLOW"},
				},
			},
		},
	})

	req := prepareRequestForProduct()
	req.HttpRequest.Host = "n.example.org"
	req.Route.Product = "pn"

	ret, resp := m.productBlockHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("resp should be nil for ALLOW action")
	}

	blockInfo, ok := req.GetContext(CtxBlockInfo).(*BlockInfo)
	if !ok || blockInfo.BlockRuleName != "pn_allow_rule" {
		t.Error("CtxBlockInfo should contain pn_allow_rule")
	}
}

func TestProductBlockHandlerProductClose(t *testing.T) {
	m := NewModuleBlock()

	cond, err := condition.Build("req_host_in(\"n.example.org\")")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}

	m.ruleTable.Update(productRuleConf{
		Version: "v1",
		Config: ProductRules{
			"pn": &blockRuleList{
				{
					Cond:   cond,
					Name:   "pn_close_rule",
					Action: Action{Cmd: "CLOSE"},
				},
			},
		},
	})

	req := prepareRequestForProduct()
	req.HttpRequest.Host = "n.example.org"
	req.Route.Product = "pn"

	ret, resp := m.productBlockHandler(req)
	if ret != bfe_module.BfeHandlerClose {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerClose, ret)
	}
	if resp != nil {
		t.Error("resp should be nil for CLOSE action")
	}

	if req.ErrCode != ErrBlock {
		t.Error("req.ErrCode should be ErrBlock")
	}

	blockInfo, ok := req.GetContext(CtxBlockInfo).(*BlockInfo)
	if !ok || blockInfo.BlockRuleName != "pn_close_rule" {
		t.Error("CtxBlockInfo should contain pn_close_rule")
	}
}

func TestProductBlockHandlerUnknownCommand(t *testing.T) {
	m := NewModuleBlock()

	cond, err := condition.Build("default_t()")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}

	m.ruleTable.Update(productRuleConf{
		Version: "v1",
		Config: ProductRules{
			"pn": &blockRuleList{
				{
					Cond:   cond,
					Name:   "unknown_cmd_rule",
					Action: Action{Cmd: "UNKNOWN"},
				},
			},
		},
	})

	req := prepareRequestForProduct()
	req.Route.Product = "pn"

	ret, resp := m.productBlockHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("resp should be nil when unknown command is ignored")
	}
}

func TestProductBlockHandlerNoMatch(t *testing.T) {
	m := NewModuleBlock()

	cond, err := condition.Build("req_host_in(\"n.example.org\")")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}

	m.ruleTable.Update(productRuleConf{
		Version: "v1",
		Config: ProductRules{
			"pn": &blockRuleList{
				{
					Cond:   cond,
					Name:   "pn_close_rule",
					Action: Action{Cmd: "CLOSE"},
				},
			},
		},
	})

	req := prepareRequestForProduct()
	req.HttpRequest.Host = "other.example.org"
	req.Route.Product = "pn"

	ret, resp := m.productBlockHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("resp should be nil when no rule matches")
	}

	if req.GetContext(CtxBlockInfo) != nil {
		t.Error("CtxBlockInfo should not be set when no rule matches")
	}
}

func TestProductBlockHandlerProductNotFound(t *testing.T) {
	m := NewModuleBlock()
	m.ruleTable.Update(productRuleConf{
		Version: "v1",
		Config:  ProductRules{},
	})

	req := prepareRequestForProduct()
	req.Route.Product = "not_exist"

	ret, resp := m.productBlockHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("resp should be nil when product is not found")
	}
}
