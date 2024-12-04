// Copyright (c) 2019 The BFE Authors.
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
	"sync"

	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util"
	"github.com/bfenetworks/bfe/bfe_util/json"
	"github.com/bfenetworks/bfe/bfe_wasmplugin"
	gcfg "gopkg.in/gcfg.v1"
)

type ConfModWasm struct {
	Basic struct {
		WasmPluginPath string // path of Wasm plugins
		DataPath string // path of config data
	}

	Log struct {
		OpenDebug bool
	}
}

// ConfLoad loads config from config file
func ConfLoad(filePath string, confRoot string) (*ConfModWasm, error) {
	var cfg ConfModWasm
	var err error

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return &cfg, err
	}

	// check conf of mod_redirect
	err = cfg.Check(confRoot)
	if err != nil {
		return &cfg, err
	}

	return &cfg, nil
}

func (cfg *ConfModWasm) Check(confRoot string) error {
	if cfg.Basic.WasmPluginPath == "" {
		log.Logger.Warn("ModWasm.WasmPluginPath not set, use default value")
		cfg.Basic.WasmPluginPath = "mod_wasm"
	}
	cfg.Basic.WasmPluginPath = bfe_util.ConfPathProc(cfg.Basic.WasmPluginPath, confRoot)

	if cfg.Basic.DataPath == "" {
		log.Logger.Warn("ModWasm.DataPath not set, use default value")
		cfg.Basic.WasmPluginPath = "mod_wasm/wasm.data"
	}
	cfg.Basic.DataPath = bfe_util.ConfPathProc(cfg.Basic.DataPath, confRoot)

	return nil
}

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
	// Md5 string
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
