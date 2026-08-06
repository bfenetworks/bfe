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
	"os"
	"path/filepath"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_wasmplugin"
	"github.com/bfenetworks/proxy-wasm-go-host/proxywasm/common"
)

// stubWasmPlugin is a fake WasmPlugin used to exercise rule loading paths
// without requiring a real Wasm runtime.
type stubWasmPlugin struct {
	config            bfe_wasmplugin.WasmPluginConfig
	instanceNum       int
	destroyed         bool
	cleared           bool
	forceEnsureResult bool
	nextEnsureResult  int
}

func newStubWasmPlugin(name, wasmVer, confVer string, num int) *stubWasmPlugin {
	return &stubWasmPlugin{
		config: bfe_wasmplugin.WasmPluginConfig{
			PluginName:    name,
			WasmVersion:   wasmVer,
			ConfigVersion: confVer,
			InstanceNum:   num,
		},
		instanceNum: num,
	}
}

func (s *stubWasmPlugin) PluginName() string {
	return s.config.PluginName
}

func (s *stubWasmPlugin) GetPluginConfig() []byte {
	return nil
}

func (s *stubWasmPlugin) GetConfig() bfe_wasmplugin.WasmPluginConfig {
	return s.config
}

func (s *stubWasmPlugin) EnsureInstanceNum(num int) int {
	if s.forceEnsureResult {
		s.forceEnsureResult = false
		s.instanceNum = s.nextEnsureResult
		return s.instanceNum
	}
	s.instanceNum = num
	return s.instanceNum
}

func (s *stubWasmPlugin) InstanceNum() int {
	return s.instanceNum
}

func (s *stubWasmPlugin) GetInstance() common.WasmInstance {
	return nil
}

func (s *stubWasmPlugin) ReleaseInstance(instance common.WasmInstance) {}

func (s *stubWasmPlugin) Exec(f func(instance common.WasmInstance) bool) {}

func (s *stubWasmPlugin) Clear() {
	s.cleared = true
}

func (s *stubWasmPlugin) OnPluginStart() {}

func (s *stubWasmPlugin) OnPluginDestroy() {
	s.destroyed = true
}

func (s *stubWasmPlugin) GetRootContextID() int32 {
	return 0
}

func strPtr(s string) *string {
	return &s
}

func newTestRequest() *bfe_basic.Request {
	return &bfe_basic.Request{
		HttpRequest: nil,
		Context:     make(map[interface{}]interface{}),
	}
}

func TestPluginConfLoad(t *testing.T) {
	content := `{
		"Version": "v1",
		"PluginMap": {
			"p1": {
				"Name": "p1",
				"WasmVersion": "v1",
				"ConfVersion": "c1",
				"InstanceNum": 1,
				"Product": "prod1"
			}
		}
	}`

	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "wasm.data")
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	conf, err := pluginConfLoad(file)
	if err != nil {
		t.Fatalf("pluginConfLoad() error: %v", err)
	}

	if conf.Version == nil || *conf.Version != "v1" {
		t.Errorf("Version should be v1")
	}

	if conf.PluginMap == nil {
		t.Fatalf("PluginMap should not be nil")
	}

	if _, ok := (*conf.PluginMap)["p1"]; !ok {
		t.Errorf("PluginMap should contain p1")
	}
}

func TestPluginConfLoadFileNotExist(t *testing.T) {
	_, err := pluginConfLoad("./testdata/mod_wasmplugin/not_exist.data")
	if err == nil {
		t.Errorf("pluginConfLoad() should return error for missing file")
	}
}

func TestPluginConfLoadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "wasm.data")
	if err := os.WriteFile(file, []byte("not json"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	_, err := pluginConfLoad(file)
	if err == nil {
		t.Errorf("pluginConfLoad() should return error for invalid json")
	}
}

func TestBuildRuleListSuccess(t *testing.T) {
	condStr := "default_t()"
	pluginList := []string{"p1"}
	pluginMap := PluginMap{
		"p1": newStubWasmPlugin("p1", "v1", "c1", 1),
	}

	rules := []FilterRuleFile{
		{
			Cond:       strPtr(condStr),
			PluginList: &pluginList,
		},
	}

	rl, err := buildRuleList(rules, pluginMap)
	if err != nil {
		t.Fatalf("buildRuleList() error: %v", err)
	}

	if len(rl) != 1 {
		t.Fatalf("rule list length should be 1, not %d", len(rl))
	}

	req := newTestRequest()
	if !rl[0].Cond.Match(req) {
		t.Errorf("condition should match")
	}

	if len(rl[0].PluginList) != 1 {
		t.Errorf("plugin list length should be 1, not %d", len(rl[0].PluginList))
	}
}

func TestBuildRuleListUnknownPlugin(t *testing.T) {
	condStr := "default_t()"
	pluginList := []string{"unknown"}

	rules := []FilterRuleFile{
		{
			Cond:       strPtr(condStr),
			PluginList: &pluginList,
		},
	}

	_, err := buildRuleList(rules, PluginMap{})
	if err == nil {
		t.Errorf("buildRuleList() should return error for unknown plugin")
	}
}

func TestBuildRuleListInvalidCond(t *testing.T) {
	condStr := "not_a_real_cond()"
	pluginList := []string{"p1"}

	rules := []FilterRuleFile{
		{
			Cond:       strPtr(condStr),
			PluginList: &pluginList,
		},
	}

	_, err := buildRuleList(rules, PluginMap{"p1": newStubWasmPlugin("p1", "v1", "c1", 1)})
	if err == nil {
		t.Errorf("buildRuleList() should return error for invalid condition")
	}
}

func TestBuildRuleListEmpty(t *testing.T) {
	rl, err := buildRuleList(nil, PluginMap{})
	if err != nil {
		t.Fatalf("buildRuleList() error: %v", err)
	}
	if len(rl) != 0 {
		t.Errorf("rule list should be empty")
	}
}

func TestBuildNewPluginMapUnchanged(t *testing.T) {
	oldPlug := newStubWasmPlugin("p1", "v1", "c1", 1)
	oldMap := PluginMap{"p1": oldPlug}

	conf := map[string]PluginMeta{
		"p1": {
			Name:        "p1",
			WasmVersion: "v1",
			ConfVersion: "c1",
			InstanceNum: 1,
		},
	}

	newMap, unchanged, err := buildNewPluginMap(&conf, oldMap, "")
	if err != nil {
		t.Fatalf("buildNewPluginMap() error: %v", err)
	}

	if newMap["p1"] != oldPlug {
		t.Errorf("unchanged plugin should be reused")
	}

	if !unchanged["p1"] {
		t.Errorf("p1 should be marked unchanged")
	}
}

func TestBuildNewPluginMapGrow(t *testing.T) {
	oldPlug := newStubWasmPlugin("p1", "v1", "c1", 1)
	oldMap := PluginMap{"p1": oldPlug}

	conf := map[string]PluginMeta{
		"p1": {
			Name:        "p1",
			WasmVersion: "v1",
			ConfVersion: "c1",
			InstanceNum: 3,
		},
	}

	_, unchanged, err := buildNewPluginMap(&conf, oldMap, "")
	if err != nil {
		t.Fatalf("buildNewPluginMap() error: %v", err)
	}

	if !unchanged["p1"] {
		t.Errorf("p1 should be marked unchanged")
	}

	if oldPlug.InstanceNum() != 3 {
		t.Errorf("instance num should grow to 3, not %d", oldPlug.InstanceNum())
	}
}

func TestBuildNewPluginMapGrowFail(t *testing.T) {
	oldPlug := newStubWasmPlugin("p1", "v1", "c1", 1)
	oldMap := PluginMap{"p1": oldPlug}

	conf := map[string]PluginMeta{
		"p1": {
			Name:        "p1",
			WasmVersion: "v1",
			ConfVersion: "c1",
			InstanceNum: 5,
		},
	}

	// Simulate EnsureInstanceNum returning a smaller number than requested.
	oldPlug.forceEnsureResult = true
	oldPlug.nextEnsureResult = 3
	_, _, err := buildNewPluginMap(&conf, oldMap, "")
	if err == nil {
		t.Errorf("buildNewPluginMap() should return error when EnsureInstanceNum fails")
	}
}

func TestBuildNewPluginMapShrink(t *testing.T) {
	oldPlug := newStubWasmPlugin("p1", "v1", "c1", 5)
	oldMap := PluginMap{"p1": oldPlug}

	conf := map[string]PluginMeta{
		"p1": {
			Name:        "p1",
			WasmVersion: "v1",
			ConfVersion: "c1",
			InstanceNum: 2,
		},
	}

	_, unchanged, err := buildNewPluginMap(&conf, oldMap, "")
	if err != nil {
		t.Fatalf("buildNewPluginMap() error: %v", err)
	}

	if !unchanged["p1"] {
		t.Errorf("p1 should be marked unchanged")
	}

	// buildNewPluginMap does not shrink unchanged plugins; shrink happens later
	// in cleanPlugins.
	if oldPlug.InstanceNum() != 5 {
		t.Errorf("instance num should remain 5 before cleanPlugins, not %d", oldPlug.InstanceNum())
	}
}

func TestBuildNewPluginMapNewPluginError(t *testing.T) {
	tmpDir := t.TempDir()
	oldMap := PluginMap{}

	conf := map[string]PluginMeta{
		"p1": {
			Name:        "p1",
			WasmVersion: "v1",
			ConfVersion: "c1",
			InstanceNum: 1,
		},
	}

	_, _, err := buildNewPluginMap(&conf, oldMap, tmpDir)
	if err == nil {
		t.Errorf("buildNewPluginMap() should return error when NewWasmPlugin fails")
	}
}

func TestBuildNewPluginMapEmptyConf(t *testing.T) {
	newMap, unchanged, err := buildNewPluginMap(nil, PluginMap{}, "")
	if err != nil {
		t.Fatalf("buildNewPluginMap() error: %v", err)
	}
	if len(newMap) != 0 {
		t.Errorf("new plugin map should be empty")
	}
	if len(unchanged) != 0 {
		t.Errorf("unchanged map should be empty")
	}
}

func TestCleanPlugins(t *testing.T) {
	unchangedPlug := newStubWasmPlugin("p1", "v1", "c1", 5)
	removedPlug := newStubWasmPlugin("p2", "v1", "c1", 1)

	pm := PluginMap{
		"p1": unchangedPlug,
		"p2": removedPlug,
	}

	unchanged := map[string]bool{"p1": true}
	conf := &map[string]PluginMeta{
		"p1": {Name: "p1", InstanceNum: 2},
		"p2": {Name: "p2", InstanceNum: 1},
	}

	cleanPlugins(pm, unchanged, conf)

	if unchangedPlug.InstanceNum() != 2 {
		t.Errorf("unchanged plugin instance num should shrink to 2, not %d", unchangedPlug.InstanceNum())
	}

	if unchangedPlug.destroyed {
		t.Errorf("unchanged plugin should not be destroyed")
	}

	if !removedPlug.destroyed {
		t.Errorf("removed plugin should be destroyed")
	}

	if !removedPlug.cleared {
		t.Errorf("removed plugin should be cleared")
	}
}

func TestUpdatePluginConfSameVersion(t *testing.T) {
	tbl := NewPluginTable()
	tbl.Update("v1", nil, nil, nil)

	conf := PluginConfFile{
		Version: strPtr("v1"),
	}

	if err := updatePluginConf(tbl, conf, ""); err != nil {
		t.Fatalf("updatePluginConf() error: %v", err)
	}

	if tbl.GetVersion() != "v1" {
		t.Errorf("version should remain v1")
	}
}

func TestUpdatePluginConfNewVersionEmpty(t *testing.T) {
	tbl := NewPluginTable()

	conf := PluginConfFile{
		Version: strPtr("v1"),
	}

	if err := updatePluginConf(tbl, conf, ""); err != nil {
		t.Fatalf("updatePluginConf() error: %v", err)
	}

	if tbl.GetVersion() != "v1" {
		t.Errorf("version should be v1, not %s", tbl.GetVersion())
	}

	if len(tbl.GetBeforeLocationRules()) != 0 {
		t.Errorf("before location rules should be empty")
	}

	if _, ok := tbl.Search("prod1"); ok {
		t.Errorf("product rules should be empty")
	}
}

func TestUpdatePluginConfNewVersionWithRules(t *testing.T) {
	oldPlug := newStubWasmPlugin("p1", "v1", "c1", 1)
	tbl := NewPluginTable()
	tbl.Update("", nil, nil, PluginMap{"p1": oldPlug})

	condStr := "default_t()"
	pluginList := []string{"p1"}
	beforeRules := []FilterRuleFile{
		{Cond: strPtr(condStr), PluginList: &pluginList},
	}
	foundRules := map[string][]FilterRuleFile{
		"prod1": {{Cond: strPtr(condStr), PluginList: &pluginList}},
	}
	pluginMetas := map[string]PluginMeta{
		"p1": {
			Name:        "p1",
			WasmVersion: "v1",
			ConfVersion: "c1",
			InstanceNum: 1,
		},
	}

	conf := PluginConfFile{
		Version:             strPtr("v2"),
		BeforeLocationRules: &beforeRules,
		FoundProductRules:   &foundRules,
		PluginMap:           &pluginMetas,
	}

	if err := updatePluginConf(tbl, conf, ""); err != nil {
		t.Fatalf("updatePluginConf() error: %v", err)
	}

	if tbl.GetVersion() != "v2" {
		t.Errorf("version should be v2, not %s", tbl.GetVersion())
	}

	if len(tbl.GetBeforeLocationRules()) != 1 {
		t.Errorf("before location rules length should be 1, not %d", len(tbl.GetBeforeLocationRules()))
	}

	rules, ok := tbl.Search("prod1")
	if !ok {
		t.Fatalf("Search(prod1) should return ok")
	}
	if len(rules) != 1 {
		t.Errorf("prod1 rules length should be 1, not %d", len(rules))
	}
}

func TestUpdatePluginConfRuleBuildError(t *testing.T) {
	oldPlug := newStubWasmPlugin("p1", "v1", "c1", 1)
	tbl := NewPluginTable()
	tbl.Update("", nil, nil, PluginMap{"p1": oldPlug})

	pluginList := []string{"unknown"}
	beforeRules := []FilterRuleFile{
		{Cond: strPtr("default_t()"), PluginList: &pluginList},
	}
	pluginMetas := map[string]PluginMeta{
		"p1": {
			Name:        "p1",
			WasmVersion: "v1",
			ConfVersion: "c1",
			InstanceNum: 1,
		},
	}

	conf := PluginConfFile{
		Version:             strPtr("v2"),
		BeforeLocationRules: &beforeRules,
		PluginMap:           &pluginMetas,
	}

	if err := updatePluginConf(tbl, conf, ""); err == nil {
		t.Errorf("updatePluginConf() should return error for unknown plugin in rule")
	}
}

func TestUpdatePluginConfInvalidCond(t *testing.T) {
	oldPlug := newStubWasmPlugin("p1", "v1", "c1", 1)
	tbl := NewPluginTable()
	tbl.Update("", nil, nil, PluginMap{"p1": oldPlug})

	pluginList := []string{"p1"}
	beforeRules := []FilterRuleFile{
		{Cond: strPtr("bad_cond()"), PluginList: &pluginList},
	}
	pluginMetas := map[string]PluginMeta{
		"p1": {
			Name:        "p1",
			WasmVersion: "v1",
			ConfVersion: "c1",
			InstanceNum: 1,
		},
	}

	conf := PluginConfFile{
		Version:             strPtr("v2"),
		BeforeLocationRules: &beforeRules,
		PluginMap:           &pluginMetas,
	}

	if err := updatePluginConf(tbl, conf, ""); err == nil {
		t.Errorf("updatePluginConf() should return error for invalid condition")
	}
}

func TestUpdatePluginConfNewPluginError(t *testing.T) {
	tmpDir := t.TempDir()
	tbl := NewPluginTable()

	pluginMetas := map[string]PluginMeta{
		"p1": {
			Name:        "p1",
			WasmVersion: "v1",
			ConfVersion: "c1",
			InstanceNum: 1,
		},
	}

	conf := PluginConfFile{
		Version:   strPtr("v2"),
		PluginMap: &pluginMetas,
	}

	if err := updatePluginConf(tbl, conf, tmpDir); err == nil {
		t.Errorf("updatePluginConf() should return error when NewWasmPlugin fails")
	}
}

func TestConditionHelper(t *testing.T) {
	cond, err := condition.Build("default_t()")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}
	if !cond.Match(newTestRequest()) {
		t.Errorf("default_t() should match")
	}
}
