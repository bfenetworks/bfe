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
	"fmt"
	"os"
	"path"

	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
	"github.com/bfenetworks/bfe/bfe_wasmplugin"
)

type PluginConfFile struct {
	Version *string // version of the config
	BeforeLocationRules *[]FilterRuleFile	// rule list for BeforeLocation
	FoundProductRules *map[string][]FilterRuleFile // product --> rule list for FoundProduct
	PluginMap *map[string]PluginMeta
}

type FilterRuleFile struct {
	Cond    *string         // condition for plugin
	PluginList *[]string
}

type PluginMeta struct {
	Name string
	WasmVersion string
	ConfVersion string
	InstanceNum int
	Product string
}

type FilterRule struct {
	Cond    condition.Condition // condition for plugin
	PluginList []bfe_wasmplugin.WasmPlugin
}

type RuleList []FilterRule
type ProductRules map[string]RuleList // product => list of filter rules

func updatePluginConf(t *PluginTable, conf PluginConfFile, pluginPath string) error {
	if conf.Version != nil && *conf.Version != t.GetVersion() {
		pluginMapNew := make(map[string]bfe_wasmplugin.WasmPlugin)
		var beforeLocationRulesNew RuleList
		productRulesNew := make(ProductRules)

		// 1. check plugin map
		unchanged := make(map[string]bool)

		pm := t.GetPluginMap()
		if conf.PluginMap != nil {
			for pn, p := range *conf.PluginMap {
				plugOld := pm[pn]
				// check whether plugin version changed.
				if plugOld != nil {
					configOld := plugOld.GetConfig()
					if configOld.WasmVersion == p.WasmVersion && configOld.ConfigVersion == p.ConfVersion {
						// not change, just copy to new map
						pluginMapNew[pn] = plugOld

						// ensure instance num
						actual := plugOld.EnsureInstanceNum(p.InstanceNum)
						if actual != p.InstanceNum {
							return fmt.Errorf("can not EnsureInstanceNum, plugin:%s, num:%d", pn, p.InstanceNum)
						}

						unchanged[pn] = true
						continue
					}
				}
				// if changed, construct a new plugin.
				wasmconf := bfe_wasmplugin.WasmPluginConfig {
					PluginName: pn,
					WasmVersion: p.WasmVersion,
					ConfigVersion: p.ConfVersion,
					InstanceNum: p.InstanceNum,
					Path: path.Join(pluginPath, pn),
					// Md5: p.Md5,
				}
				plug, err := bfe_wasmplugin.NewWasmPlugin(wasmconf)
				if err != nil {
					// build plugin error
					return err
				}

				// plug.OnPluginStart()

				pluginMapNew[pn] = plug
			}
		}

		// 2. construct product rules
		if conf.BeforeLocationRules != nil {
			for _, r := range *conf.BeforeLocationRules {
				rule := FilterRule{}
				cond, err := condition.Build(*r.Cond)
				if err != nil {
					return err
				}
				rule.Cond =cond
				for _, pn := range *r.PluginList {
					plug := pluginMapNew[pn]
					if plug == nil {
						return fmt.Errorf("unknown plugin: %s", pn)
					}
					rule.PluginList = append(rule.PluginList, plug)
				}
				beforeLocationRulesNew = append(beforeLocationRulesNew, rule)
			}
		}

		if conf.FoundProductRules != nil {
			for product, rules := range *conf.FoundProductRules {
				var rulelist RuleList
				for _, r := range rules {
					rule := FilterRule{}
					cond, err := condition.Build(*r.Cond)
					if err != nil {
						return err
					}
					rule.Cond =cond
					for _, pn := range *r.PluginList {
						plug := pluginMapNew[pn]
						if plug == nil {
							return fmt.Errorf("unknown plugin: %s", pn)
						}
						rule.PluginList = append(rule.PluginList, plug)
					}
					rulelist = append(rulelist, rule)
				}
				productRulesNew[product] = rulelist
			}
		}

		// 3. update PluginTable
		t.Update(*conf.Version, beforeLocationRulesNew, productRulesNew, pluginMapNew)

		// 4. stop & clear old plugins
		for pn, plug := range pm {
			if _, ok := unchanged[pn]; !ok {
				// stop plug
				plug.OnPluginDestroy()
				plug.Clear()
			}
		}
	}
	return nil
}

func pluginConfLoad(filename string) (PluginConfFile, error) {
	var conf PluginConfFile

	/* open the file */
	file, err := os.Open(filename)

	if err != nil {
		return conf, err
	}

	/* decode the file  */
	decoder := json.NewDecoder(file)

	err = decoder.Decode(&conf)
	file.Close()

	if err != nil {
		return conf, err
	}

	return conf, nil
}
