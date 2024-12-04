// Copyright (c) 2024 The BFE Authors.
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
	"sync"

	"github.com/bfenetworks/bfe/bfe_wasmplugin"
)

type PluginTable struct {
	lock         sync.RWMutex
	version      string
	beforeLocationRules RuleList
	productRules ProductRules
	pluginMap map[string]bfe_wasmplugin.WasmPlugin
}

func NewPluginTable() *PluginTable {
	t := new(PluginTable)
	t.productRules = make(ProductRules)
	t.pluginMap = make(map[string]bfe_wasmplugin.WasmPlugin)
	return t
}

func (t *PluginTable) Update(version string, beforeLocationRules RuleList, productRules ProductRules, pluginMap map[string]bfe_wasmplugin.WasmPlugin) {
	t.lock.Lock()

	t.version = version
	t.beforeLocationRules = beforeLocationRules
	t.productRules = productRules
	t.pluginMap = pluginMap

	t.lock.Unlock()
}

func (t *PluginTable) GetVersion() string {
	defer t.lock.RUnlock()
	t.lock.RLock()
	return t.version
}

func (t *PluginTable) GetPluginMap() map[string]bfe_wasmplugin.WasmPlugin {
	defer t.lock.RUnlock()
	t.lock.RLock()
	return t.pluginMap
}

func (t *PluginTable) GetBeforeLocationRules() RuleList {
	defer t.lock.RUnlock()
	t.lock.RLock()
	return t.beforeLocationRules
}

func (t *PluginTable) Search(product string) (RuleList, bool) {
	t.lock.RLock()
	productRules := t.productRules
	t.lock.RUnlock()

	rules, ok := productRules[product]
	return rules, ok
}
