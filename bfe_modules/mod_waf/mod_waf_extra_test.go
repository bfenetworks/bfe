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

package mod_waf

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_module"
)

const validWafProductRule = `{
    "Version": "2019-12-10184356",
    "Config": {
        "pn": [
            {
                "Cond": "default_t()",
                "BlockRules": [
                    "RuleBashCmd"
                ]
            }
        ]
    }
}
`

// prepareTempWafConf creates a temporary conf root that contains
// mod_waf/mod_waf.conf and mod_waf/waf_rule.data. The caller does not need to
// clean up the returned directory.
func prepareTempWafConf(t *testing.T, ruleContent string) string {
	t.Helper()

	dir, err := ioutil.TempDir("", "mod_waf_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	modDir := filepath.Join(dir, "mod_waf")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	if ruleContent != "" {
		if err := ioutil.WriteFile(filepath.Join(modDir, "waf_rule.data"), []byte(ruleContent), 0644); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}
	}

	confContent := `[basic]
ProductRulePath = mod_waf/waf_rule.data

[Log]
LogPrefix = waf
LogDir = log
RotateWhen = NEXTHOUR
BackupCount = 24
`
	if err := ioutil.WriteFile(filepath.Join(modDir, "mod_waf.conf"), []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return dir
}

func TestNewModuleWafAndName(t *testing.T) {
	m := NewModuleWaf()
	if m == nil {
		t.Fatal("NewModuleWaf() returns nil")
	}

	if m.Name() != ModWaf {
		t.Errorf("Name() should be %s, got %s", ModWaf, m.Name())
	}
}

func TestModuleWafInitSuccess(t *testing.T) {
	dir := prepareTempWafConf(t, validWafProductRule)

	m := NewModuleWaf()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err != nil {
		t.Errorf("Init() error: %v", err)
	}
}

func TestModuleWafInitConfNotExist(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_waf_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	m := NewModuleWaf()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when config file does not exist")
	}
}

func TestModuleWafInitBadProductRule(t *testing.T) {
	dir := prepareTempWafConf(t, "invalid product rule content")

	m := NewModuleWaf()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when product rule file is invalid")
	}
}

func updateRuleTable(m *ModuleWaf, condStr string, blockRules, checkRules []string) error {
	cond, err := condition.Build(condStr)
	if err != nil {
		return err
	}

	m.ruleTable.Update(&productWafRuleConfig{
		Version: "v1",
		Config: productWafRule{
			"pn": &ruleList{&wafRule{
				Cond:       cond,
				BlockRules: blockRules,
				CheckRules: checkRules,
			}},
		},
	})
	return nil
}

func TestModuleWafHandleWafProductNotFound(t *testing.T) {
	m := NewModuleWaf()

	req := prepareRequest("not_exist", "/")
	ret, resp := m.handleWaf(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("resp should be nil when product is not found")
	}
}

func TestModuleWafHandleWafCondNotMatch(t *testing.T) {
	m := NewModuleWaf()
	if err := updateRuleTable(m, "req_host_in(\"n.example.org\")", []string{"RuleBashCmd"}, nil); err != nil {
		t.Fatalf("updateRuleTable() error: %v", err)
	}

	req := prepareRequest("pn", "/")
	req.HttpRequest.Host = "other.example.org"
	ret, resp := m.handleWaf(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("resp should be nil when condition does not match")
	}
}

func TestModuleWafHandleWafBlock(t *testing.T) {
	dir := prepareTempWafConf(t, validWafProductRule)

	m := NewModuleWaf()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if err := updateRuleTable(m, "default_t()", []string{"RuleBashCmd"}, nil); err != nil {
		t.Fatalf("updateRuleTable() error: %v", err)
	}

	req := prepareRequest("pn", "/")
	req.HttpRequest.Header["User-Agent"] = []string{"() { :; }; echo; echo; rm -rf ./*"}
	ret, resp := m.handleWaf(req)
	if ret != bfe_module.BfeHandlerFinish {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerFinish, ret)
	}
	if resp != nil {
		t.Error("resp should be nil for block action")
	}
	if req.ErrCode != ErrWaf {
		t.Errorf("ErrCode should be %v, got %v", ErrWaf, req.ErrCode)
	}
}

func TestModuleWafHandleWafCheckHit(t *testing.T) {
	dir := prepareTempWafConf(t, validWafProductRule)

	m := NewModuleWaf()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if err := updateRuleTable(m, "default_t()", nil, []string{"RuleBashCmd"}); err != nil {
		t.Fatalf("updateRuleTable() error: %v", err)
	}

	req := prepareRequest("pn", "/")
	req.HttpRequest.Header["User-Agent"] = []string{"() { :; }; echo; echo; rm -rf ./*"}
	ret, resp := m.handleWaf(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("resp should be nil for check action")
	}
}

func TestModuleWafHandleWafInvalidRule(t *testing.T) {
	dir := prepareTempWafConf(t, validWafProductRule)

	m := NewModuleWaf()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if err := updateRuleTable(m, "default_t()", []string{"UnknownRule"}, nil); err != nil {
		t.Fatalf("updateRuleTable() error: %v", err)
	}

	req := prepareRequest("pn", "/")
	ret, resp := m.handleWaf(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("resp should be nil when rule is invalid")
	}
}

func TestModuleWafHandleWafBlockContinueWhenNotHit(t *testing.T) {
	dir := prepareTempWafConf(t, validWafProductRule)

	m := NewModuleWaf()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if err := updateRuleTable(m, "default_t()", []string{"RuleBashCmd"}, nil); err != nil {
		t.Fatalf("updateRuleTable() error: %v", err)
	}

	req := prepareRequest("pn", "/")
	req.HttpRequest.Header["User-Agent"] = []string{"normal user agent"}
	ret, resp := m.handleWaf(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("resp should be nil when block rule does not hit")
	}
}

func TestModuleWafGetState(t *testing.T) {
	m := NewModuleWaf()
	m.state.CheckedReq.Inc(1)

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

func TestModuleWafLoadProductRuleConfWithPath(t *testing.T) {
	dir := prepareTempWafConf(t, validWafProductRule)

	m := NewModuleWaf()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// reload with explicit path parameter
	query := map[string][]string{"path": {filepath.Join(dir, "mod_waf", "waf_rule.data")}}
	if err := m.loadProductRuleConf(query); err != nil {
		t.Errorf("loadProductRuleConf() error: %v", err)
	}
}
