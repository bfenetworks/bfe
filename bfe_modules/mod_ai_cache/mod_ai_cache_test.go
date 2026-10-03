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

package mod_ai_cache

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_module"
)

// prepareInitConf creates a temporary conf root containing
// mod_ai_cache/mod_ai_cache.conf and mod_ai_cache/mod_ai_cache_rule.data.
// The caller is responsible for cleaning up the returned directory.
func prepareInitConf(t *testing.T) string {
	t.Helper()

	dir, err := ioutil.TempDir("", "mod_ai_cache_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	modDir := filepath.Join(dir, "mod_ai_cache")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	ruleData := `{
  "Version": "1.0",
  "Config": {
    "default": [
      { "cond": "default_t()", "cacheKeyStrategy": "disabled" }
    ]
  }
}`
	if err := ioutil.WriteFile(filepath.Join(modDir, "mod_ai_cache_rule.data"),
		[]byte(ruleData), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	confContent := `[basic]
ProductRulePath = mod_ai_cache/mod_ai_cache_rule.data
CacheKeyPrefix = ai_cache
DefaultCacheTTL = 3600

[redis]
bns = BFE.poc-redis-wx
connectTimeout = 20
readTimeout = 20
writeTimeout = 20
maxIdle = 20

[log]
OpenDebug = false
`
	if err := ioutil.WriteFile(filepath.Join(modDir, "mod_ai_cache.conf"),
		[]byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return dir
}

// TestInitRegistersLookupAtFoundProduct locks the lookup-before-route
// placement (lazy intent resolve sinking): the cache lookup filter must be
// registered at HandleFoundProduct so a hit short-circuits before
// mod_ai_route evaluates rules, and must NOT stay at HandleAfterLocation.
func TestInitRegistersLookupAtFoundProduct(t *testing.T) {
	dir := prepareInitConf(t)

	m := NewModuleAiCache()
	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()

	if err := m.Init(cbs, whs, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	raw, err := cbs.ModuleHandlersGetJSON()
	if err != nil {
		t.Fatalf("ModuleHandlersGetJSON() error: %v", err)
	}
	var handlers map[string][]string
	if err := json.Unmarshal(raw, &handlers); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	if n := len(handlers["3#HandleFoundProduct"]); n != 1 {
		t.Errorf("HandleFoundProduct should have 1 handler (cacheRequestHandler), got %d", n)
	}
	if n := len(handlers["4#HandleAfterLocation"]); n != 0 {
		t.Errorf("HandleAfterLocation should have no cache lookup handler, got %d", n)
	}
	if n := len(handlers["6#HandleReadResponse"]); n != 1 {
		t.Errorf("HandleReadResponse should have 1 handler (cacheResponseHandler), got %d", n)
	}
}

// TestProductRuleConfLoadWithIntentCond verifies that a rule cond referencing
// req_ai_intent_in is still accepted at load time (it only emits a warning;
// the reference would trigger intent resolve during cache lookup and negate
// the lookup-early benefit, but must not break loading).
func TestProductRuleConfLoadWithIntentCond(t *testing.T) {
	conf, err := ProductRuleConfLoad(
		"testdata/mod_ai_cache/mod_ai_cache_rule_intent_cond.data", DefaultCacheTTL)
	if err != nil {
		t.Fatalf("ProductRuleConfLoad() error: %v", err)
	}
	if conf == nil || conf.Config == nil {
		t.Fatal("conf should be loaded")
	}
}
