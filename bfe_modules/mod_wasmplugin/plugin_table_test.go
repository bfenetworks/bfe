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
	"testing"
)

func TestNewPluginTable(t *testing.T) {
	tbl := NewPluginTable()
	if tbl == nil {
		t.Fatalf("NewPluginTable() should not return nil")
	}

	if tbl.productRules == nil {
		t.Errorf("productRules should be initialized")
	}

	if tbl.pluginMap == nil {
		t.Errorf("pluginMap should be initialized")
	}
}

func TestPluginTableUpdateAndGet(t *testing.T) {
	tbl := NewPluginTable()

	beforeRules := RuleList{}
	productRules := ProductRules{"prod1": RuleList{}}
	pluginMap := PluginMap{}

	tbl.Update("v2", beforeRules, productRules, pluginMap)

	if tbl.GetVersion() != "v2" {
		t.Errorf("GetVersion() should be v2, not %s", tbl.GetVersion())
	}

	if tbl.GetBeforeLocationRules() == nil {
		t.Errorf("GetBeforeLocationRules() should not be nil")
	}

	if tbl.GetPluginMap() == nil {
		t.Errorf("GetPluginMap() should not be nil")
	}

	rules, ok := tbl.Search("prod1")
	if !ok {
		t.Errorf("Search(prod1) should return ok")
	}
	if rules == nil {
		t.Errorf("Search(prod1) rules should not be nil")
	}

	_, ok = tbl.Search("prod2")
	if ok {
		t.Errorf("Search(prod2) should not return ok")
	}
}
