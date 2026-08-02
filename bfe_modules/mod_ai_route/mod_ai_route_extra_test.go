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

package mod_ai_route

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const testRouteData = `{
    "Version": "20260720150000",
    "route_rules": {
        "apikey_ak_user_a": {
            "type": "apikey",
            "owner": "ak_user_a",
            "rules": [
                {
                    "name": "user_a-rule1",
                    "Cond": "req_host_in(\"api.example.org\")",
                    "targets": [
                        {
                            "ClusterName": "cluster_deepseek_a",
                            "Model": "deepseek-v4-pro",
                            "Weight": 70
                        },
                        {
                            "ClusterName": "cluster_deepseek_b",
                            "Model": "deepseek-v4-pro",
                            "Weight": 30
                        }
                    ],
                    "fallbacks": [
                        {
                            "ClusterName": "cluster_deepseek_c",
                            "Model": "deepseek-v3.2"
                        }
                    ]
                }
            ]
        },
        "global_default": {
            "type": "global",
            "owner": "global",
            "rules": [
                {
                    "name": "global-default",
                    "Cond": "default_t()",
                    "targets": [
                        {
                            "ClusterName": "cluster_global",
                            "Model": "",
                            "Weight": 100
                        }
                    ],
                    "fallbacks": []
                }
            ]
        }
    },
    "ApikeyRouteTableBindings": {
        "ak_user_a": [
            "apikey_ak_user_a",
            "global_default"
        ]
    }
}`

// prepareTempConf creates a temporary conf root containing
// mod_ai_route/mod_ai_route.conf and mod_ai_route/ai_route.data.
// The caller is responsible for cleaning up the returned directory.
func prepareTempConf(t *testing.T) string {
	t.Helper()

	dir, err := ioutil.TempDir("", "mod_ai_route_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	modDir := filepath.Join(dir, "mod_ai_route")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	dataPath := filepath.Join(modDir, "ai_route.data")
	if err := ioutil.WriteFile(dataPath, []byte(testRouteData), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	confContent := `[basic]
RouteRulePath = ./mod_ai_route/ai_route.data

[log]
OpenDebug = true
`
	confPath := filepath.Join(modDir, "mod_ai_route.conf")
	if err := ioutil.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return dir
}

func setOpenDebug(t *testing.T, v bool) {
	t.Helper()
	old := openDebug
	openDebug = v
	t.Cleanup(func() { openDebug = old })
}

func TestNewModuleAiRouteAndName(t *testing.T) {
	m := NewModuleAiRoute()
	if m == nil {
		t.Fatal("NewModuleAiRoute() returns nil")
	}
	if m.Name() != ModAiRoute {
		t.Errorf("Name() should be %s, got %s", ModAiRoute, m.Name())
	}
}

func TestModuleAiRouteInitSuccess(t *testing.T) {
	dir := prepareTempConf(t)

	m := NewModuleAiRoute()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err != nil {
		t.Errorf("Init() error: %v", err)
	}

	if m.conf == nil {
		t.Fatal("conf should be loaded")
	}
	if !m.conf.Log.OpenDebug {
		t.Error("OpenDebug should be true")
	}
}

func TestModuleAiRouteInitConfNotExist(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_ai_route_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	m := NewModuleAiRoute()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when config file does not exist")
	}
}

func TestModuleAiRouteInitDataLoadFail(t *testing.T) {
	dir := prepareTempConf(t)

	// overwrite data file with invalid JSON
	dataPath := filepath.Join(dir, "mod_ai_route", "ai_route.data")
	if err := ioutil.WriteFile(dataPath, []byte("invalid json data"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := NewModuleAiRoute()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when route data file is invalid")
	}
}

func TestModuleAiRouteInitAddFilterError(t *testing.T) {
	dir := prepareTempConf(t)

	m := NewModuleAiRoute()
	cb := &bfe_module.BfeCallbacks{}
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when AddFilter returns error")
	}
}

func TestModuleAiRouteInitRegisterHandlersError(t *testing.T) {
	dir := prepareTempConf(t)

	m := NewModuleAiRoute()
	cb := bfe_module.NewBfeCallbacks()

	if err := m.Init(cb, nil, dir); err == nil {
		t.Error("Init() should fail when RegisterHandlers receives nil WebHandlers")
	}
}

func TestModuleAiRouteInitReloadRegisterError(t *testing.T) {
	dir := prepareTempConf(t)

	m := NewModuleAiRoute()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	// pre-register a reload handler with the same name to force collision
	err := wh.RegisterHandler(web_monitor.WebHandleReload, ModAiRoute, func(url.Values) error { return nil })
	if err != nil {
		t.Fatalf("pre-register handler error: %v", err)
	}

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when reload RegisterHandlers returns error")
	}
}

func TestModuleAiRouteGetState(t *testing.T) {
	m := NewModuleAiRoute()
	m.state.ReqTotal.Inc(1)

	b, err := m.getState(nil)
	if err != nil {
		t.Errorf("getState() error: %v", err)
	}
	if len(b) == 0 {
		t.Error("getState() returns empty response")
	}
}

func TestLoadRouteRuleConfWithQueryPath(t *testing.T) {
	dir := prepareTempConf(t)
	dataPath := filepath.Join(dir, "mod_ai_route", "ai_route.data")

	m := NewModuleAiRoute()
	m.conf = &ConfModAiRoute{}

	query := url.Values{}
	query.Set("path", dataPath)
	if err := m.loadRouteRuleConf(query); err != nil {
		t.Errorf("loadRouteRuleConf() with query path error: %v", err)
	}
}

func TestLoadRouteRuleConfWithDefaultPath(t *testing.T) {
	dir := prepareTempConf(t)
	dataPath := filepath.Join(dir, "mod_ai_route", "ai_route.data")

	m := NewModuleAiRoute()
	m.conf = &ConfModAiRoute{}
	m.conf.Basic.RouteRulePath = dataPath

	if err := m.loadRouteRuleConf(nil); err != nil {
		t.Errorf("loadRouteRuleConf() with default path error: %v", err)
	}
}

func TestLoadRouteRuleConfInvalidData(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_ai_route_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	invalidPath := filepath.Join(dir, "invalid.data")
	if err := ioutil.WriteFile(invalidPath, []byte("not json"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := NewModuleAiRoute()
	m.conf = &ConfModAiRoute{}

	query := url.Values{}
	query.Set("path", invalidPath)
	if err := m.loadRouteRuleConf(query); err == nil {
		t.Error("loadRouteRuleConf() should fail for invalid data")
	}
}

func TestLoadRouteRuleConfUpdateFail(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_ai_route_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "invalid_weight.data")
	content := `{
    "Version": "v1",
    "route_rules": {
        "global_default": {
            "type": "global",
            "owner": "global",
            "rules": [
                {
                    "name": "global-default",
                    "Cond": "default_t()",
                    "targets": [
                        {"ClusterName": "c1", "Weight": 60},
                        {"ClusterName": "c2", "Weight": 30}
                    ],
                    "fallbacks": []
                }
            ]
        }
    },
    "ApikeyRouteTableBindings": {}
}`
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := NewModuleAiRoute()
	m.conf = &ConfModAiRoute{}

	query := url.Values{}
	query.Set("path", path)
	if err := m.loadRouteRuleConf(query); err == nil {
		t.Error("loadRouteRuleConf() should fail when routeTable.Update fails")
	}
}

func TestRouteFoundProductHandlerWithOpenDebug(t *testing.T) {
	setOpenDebug(t, true)

	m := NewModuleAiRoute()
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}
	if err := m.routeTable.Update(data); err != nil {
		t.Fatalf("update table failed: %s", err)
	}

	req, _ := bfe_http.NewRequest(http.MethodGet, "http://api.example.org/v1/chat/completions", nil)
	basicReq := bfe_basic.NewRequest(req, nil, nil, nil, nil)
	basicReq.InitAiBasicInfo()

	ret, resp := m.routeFoundProductHandler(basicReq)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}

	req2 := newTestRequestWithApiKey("api.example.org", "ak_no_binding")
	ret, resp = m.routeFoundProductHandler(req2)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
	if req2.GetAiRouteResult() != nil {
		t.Error("expected no AiRouteResult when route miss")
	}
}

func TestRouteFoundProductHandlerHitEntity(t *testing.T) {
	m := NewModuleAiRoute()
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}
	if err := m.routeTable.Update(data); err != nil {
		t.Fatalf("update table failed: %s", err)
	}

	req := newTestRequestWithApiKey("other.example.org", "ak_user_b")
	ret, resp := m.routeFoundProductHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}

	result := req.GetAiRouteResult()
	if result == nil {
		t.Fatal("expected AiRouteResult set in context")
	}
	if result.RouteType != RouteTypeEntity {
		t.Errorf("RouteType: expected entity, got %s", result.RouteType)
	}
	if result.Owner != "dept_ai" {
		t.Errorf("Owner: expected dept_ai, got %s", result.Owner)
	}
}

func TestRouteFoundProductHandlerHitGlobal(t *testing.T) {
	m := NewModuleAiRoute()
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}
	if err := m.routeTable.Update(data); err != nil {
		t.Fatalf("update table failed: %s", err)
	}

	req := newTestRequestWithApiKey("unknown.example.org", "ak_user_a")
	ret, resp := m.routeFoundProductHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}

	result := req.GetAiRouteResult()
	if result == nil {
		t.Fatal("expected AiRouteResult set in context")
	}
	if result.RouteType != RouteTypeGlobal {
		t.Errorf("RouteType: expected global, got %s", result.RouteType)
	}
	if result.Owner != "global" {
		t.Errorf("Owner: expected global, got %s", result.Owner)
	}
}

func TestAiRouteTableSearchMissingTableWithOpenDebug(t *testing.T) {
	setOpenDebug(t, true)

	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}

	table := NewAiRouteTable()
	if err := table.Update(data); err != nil {
		t.Fatalf("update failed: %s", err)
	}

	req := newTestBasicRequest("api.example.org")
	result := table.Search("ak_unknown", req)
	if result != nil {
		t.Error("expected nil when all bound tables missing")
	}
}

func TestAiRouteTableSearchEmptyBinding(t *testing.T) {
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}

	table := NewAiRouteTable()
	if err := table.Update(data); err != nil {
		t.Fatalf("update failed: %s", err)
	}

	// directly inject an empty binding
	table.lock.Lock()
	table.bindings["ak_empty_binding"] = []string{}
	table.lock.Unlock()

	req := newTestBasicRequest("api.example.org")
	result := table.Search("ak_empty_binding", req)
	if result != nil {
		t.Error("expected nil for apikey with empty binding")
	}
}

func TestAiRouteDataFileUnmarshalCanonical(t *testing.T) {
	content := `{
    "Version": "v1",
    "route_rules": {
        "global_default": {
            "type": "global",
            "owner": "global",
            "rules": []
        }
    },
    "ApikeyRouteTableBindings": {}
}`
	var file AiRouteDataFile
	if err := json.Unmarshal([]byte(content), &file); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if file.Version != "v1" {
		t.Errorf("Version mismatch: expected v1, got %s", file.Version)
	}
	if _, ok := file.RouteRules["global_default"]; !ok {
		t.Error("expected global_default route rule")
	}
}

func TestAiRouteDataFileUnmarshalUpperRouteRules(t *testing.T) {
	content := `{
    "Version": "v1",
    "RouteRules": {
        "global_default": {
            "type": "global",
            "owner": "global",
            "rules": []
        }
    },
    "ApikeyRouteTableBindings": {}
}`
	var file AiRouteDataFile
	if err := json.Unmarshal([]byte(content), &file); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if _, ok := file.RouteRules["global_default"]; !ok {
		t.Error("expected global_default route rule from RouteRules")
	}
}

func TestAiRouteDataFileUnmarshalCanonicalTakesPrecedence(t *testing.T) {
	content := `{
    "Version": "v1",
    "route_rules": {
        "canonical": {
            "type": "global",
            "owner": "global",
            "rules": []
        }
    },
    "RouteRules": {
        "upper": {
            "type": "global",
            "owner": "global",
            "rules": []
        }
    },
    "ApikeyRouteTableBindings": {}
}`
	var file AiRouteDataFile
	if err := json.Unmarshal([]byte(content), &file); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if _, ok := file.RouteRules["canonical"]; !ok {
		t.Error("expected canonical route rule")
	}
	if _, ok := file.RouteRules["upper"]; ok {
		t.Error("upper route rule should be ignored when canonical present")
	}
}

func TestAiRouteDataFileUnmarshalApiKeyNormalization(t *testing.T) {
	content := `{
    "Version": "v1",
    "route_rules": {
        "apikey_ak": {
            "type": "api_key",
            "owner": "ak",
            "rules": []
        }
    },
    "ApikeyRouteTableBindings": {}
}`
	var file AiRouteDataFile
	if err := json.Unmarshal([]byte(content), &file); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	table := file.RouteRules["apikey_ak"]
	if table.Type != RouteTypeApikey {
		t.Errorf("type should be normalized to %s, got %s", RouteTypeApikey, table.Type)
	}
}

func TestAiRouteDataFileUnmarshalInvalidJSON(t *testing.T) {
	content := `{"Version": "v1", "route_rules": }`
	var file AiRouteDataFile
	if err := json.Unmarshal([]byte(content), &file); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestValidateRouteTableEmptyCond(t *testing.T) {
	table := RouteTable{
		Type: RouteTypeGlobal,
		Rules: []RouteRule{
			{
				Name:    "empty-cond",
				CondStr: "",
				Targets: []bfe_basic.AiRouteTarget{
					{ClusterName: "cluster_global", Weight: 100},
				},
			},
		},
	}
	if err := ValidateRouteTable(&table); err == nil {
		t.Error("expected error for empty Cond")
	}
}

func TestRouteTableMatchNilCond(t *testing.T) {
	table := RouteTable{
		Type: RouteTypeGlobal,
		Rules: []RouteRule{
			{
				Name:    "nil-cond",
				CondStr: "req_host_in(\"api.example.org\")",
				Targets: []bfe_basic.AiRouteTarget{
					{ClusterName: "cluster_a", Weight: 100},
				},
			},
			{
				Name:    "default",
				CondStr: "default_t()",
				Targets: []bfe_basic.AiRouteTarget{
					{ClusterName: "cluster_default", Weight: 100},
				},
			},
		},
	}
	if err := ValidateRouteTable(&table); err != nil {
		t.Fatalf("validate failed: %s", err)
	}

	// simulate a rule whose condition failed to compile by clearing Cond
	table.Rules[0].Cond = nil

	matched := table.Match(newTestRequest("api.example.org"))
	if matched == nil || matched.Name != "default" {
		t.Errorf("expected default rule when first rule has nil Cond, got %v", matched)
	}
}

func TestAiRouteResultTargetsAndFallbacks(t *testing.T) {
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}

	table := NewAiRouteTable()
	if err := table.Update(data); err != nil {
		t.Fatalf("update failed: %s", err)
	}

	req := newTestBasicRequest("api.example.org")
	result := table.Search("ak_user_a", req)
	if result == nil {
		t.Fatal("expected hit route")
	}

	if len(result.Targets) != 2 {
		t.Errorf("Targets count: expected 2, got %d", len(result.Targets))
	}
	if result.Targets[0].ClusterName != "cluster_deepseek_a" {
		t.Errorf("first target cluster: expected cluster_deepseek_a, got %s", result.Targets[0].ClusterName)
	}
	if result.Targets[0].Weight != 70 {
		t.Errorf("first target weight: expected 70, got %d", result.Targets[0].Weight)
	}

	if len(result.Fallbacks) != 1 {
		t.Errorf("Fallbacks count: expected 1, got %d", len(result.Fallbacks))
	}
	if result.Fallbacks[0].ClusterName != "cluster_deepseek_c" {
		t.Errorf("fallback cluster: expected cluster_deepseek_c, got %s", result.Fallbacks[0].ClusterName)
	}
}

func TestConfLoadPathRelativeToConfRoot(t *testing.T) {
	dir := prepareTempConf(t)
	confPath := filepath.Join(dir, "mod_ai_route", "mod_ai_route.conf")

	cfg, err := ConfLoad(confPath, dir)
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}
	if cfg == nil {
		t.Fatal("ConfLoad() returns nil config")
	}

	expectedSuffix := "/mod_ai_route/ai_route.data"
	if !strings.HasSuffix(cfg.Basic.RouteRulePath, expectedSuffix) {
		t.Errorf("RouteRulePath should end with %s, got %s", expectedSuffix, cfg.Basic.RouteRulePath)
	}
}

func TestConfModAiRouteCheckEmptyRouteRulePath(t *testing.T) {
	cfg := &ConfModAiRoute{}
	if err := cfg.Check(""); err == nil {
		t.Error("Check() should fail when RouteRulePath is empty")
	}
}
