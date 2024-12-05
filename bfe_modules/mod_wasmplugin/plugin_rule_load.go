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

type PluginMap map[string]bfe_wasmplugin.WasmPlugin

func buildRuleList(rules []FilterRuleFile, pluginMap PluginMap) (RuleList, error) {
	var rulelist RuleList

	for _, r := range rules {
		rule := FilterRule{}
		cond, err := condition.Build(*r.Cond)
		if err != nil {
			return nil, err
		}

		rule.Cond =cond
		
		for _, pn := range *r.PluginList {
			plug := pluginMap[pn]
			if plug == nil {
				return nil, fmt.Errorf("unknown plugin: %s", pn)
			}
			rule.PluginList = append(rule.PluginList, plug)
		}
		
		rulelist = append(rulelist, rule)
	}

	return rulelist, nil
}

func buildNewPluginMap(conf *map[string]PluginMeta, pmOld PluginMap, 
	pluginPath string) (pmNew PluginMap, unchanged map[string]bool, err error) {

	pmNew = PluginMap{}
	unchanged = map[string]bool{}

	if conf != nil {
		for pn, p := range *conf {
			plugOld := pmOld[pn]
			// check whether plugin version changed.
			if plugOld != nil {
				configOld := plugOld.GetConfig()
				if configOld.WasmVersion == p.WasmVersion && configOld.ConfigVersion == p.ConfVersion {
					// not change, just copy to new map
					pmNew[pn] = plugOld

					// grow instance num if needed
					if p.InstanceNum > plugOld.InstanceNum() {
						actual := plugOld.EnsureInstanceNum(p.InstanceNum)
						if actual != p.InstanceNum {
							err = fmt.Errorf("can not EnsureInstanceNum, plugin:%s, num:%d", pn, p.InstanceNum)
							return
						}
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
			}
			plug, err1 := bfe_wasmplugin.NewWasmPlugin(wasmconf)
			if err1 != nil {
				// build plugin error
				err = err1
				return
			}

			pmNew[pn] = plug
		}
	}

	return
}

func cleanPlugins(pm PluginMap, unchanged map[string]bool, conf *map[string]PluginMeta) {
	for pn, plug := range pm {
		if unchanged[pn] {
			// shink instance num if needed
			confnum := (*conf)[pn].InstanceNum
			if plug.InstanceNum() > confnum {
				plug.EnsureInstanceNum(confnum)
			}
		} else {
			// stop plug
			plug.OnPluginDestroy()
			plug.Clear()
		}
	}
}

func updatePluginConf(t *PluginTable, conf PluginConfFile, pluginPath string) error {
	if conf.Version != nil && *conf.Version != t.GetVersion() {

		// 1. check plugin map
		pm := t.GetPluginMap()
		pluginMapNew, unchanged, err := buildNewPluginMap(conf.PluginMap, pm, pluginPath)
		if err != nil {
			return err
		}

		// 2. construct product rules
		var beforeLocationRulesNew RuleList
		if conf.BeforeLocationRules != nil {
			if rulelist, err := buildRuleList(*conf.BeforeLocationRules, pluginMapNew); err == nil {
				beforeLocationRulesNew = rulelist
			} else {
				return err
			}
		}

		productRulesNew := make(ProductRules)
		if conf.FoundProductRules != nil {
			for product, rules := range *conf.FoundProductRules {
				if rulelist, err := buildRuleList(rules, pluginMapNew); err == nil {
					productRulesNew[product] = rulelist
				} else {
					return err
				}
			}
		}

		// 3. update PluginTable
		t.Update(*conf.Version, beforeLocationRulesNew, productRulesNew, pluginMapNew)

		// 4. stop & clean old plugins
		cleanPlugins(pm, unchanged, conf.PluginMap)
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
