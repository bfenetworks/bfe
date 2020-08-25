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

// module framework for bfe

package bfe_module

import (
	"fmt"
	"path"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type BfeModule interface {
	// Name return name of module.
	Name() string

	// Init initializes the module.
	//
	// Params:
	//      - cbs: callback handlers. for register call back function
	//      - whs: web monitor handlers. for register web monitor handler
	//      - cr: config root path. for get config path of module
	Init(cbs *BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error
}

// moduleMap holds mappings from mod_name to module.
var moduleMap = make(map[string]BfeModule)

// modulesAll is an ordered list of all module names.
var modulesAll = make([]string, 0)

// modulesEnabled is list of enabled module names.
var modulesEnabled = make([]string, 0)

// AddModule adds module to moduleMap and modulesAll.
func AddModule(module BfeModule) {
	moduleMap[module.Name()] = module
	modulesAll = append(modulesAll, module.Name())
}

type BfeModules struct {
	workModules map[string]BfeModule // work modules, configure in bfe conf file
}

// NewBfeModules create new BfeModules
func NewBfeModules() *BfeModules {
	bfeModules := new(BfeModules)
	bfeModules.workModules = make(map[string]BfeModule)

	return bfeModules
}

// RegisterModule register work module, only work module be inited
func (bm *BfeModules) RegisterModule(name string) error {
	module, ok := moduleMap[name]
	if !ok {
		return fmt.Errorf("no module for %s", name)
	}

	bm.workModules[name] = module

	return nil
}

// GetModule get work module by name.
func (bm *BfeModules) GetModule(name string) BfeModule {
	return bm.workModules[name]
}

// Init initializes bfe modules.
//
// Params:
//     - cbs: BfeCallbacks
//     - whs: WebHandlers
//     - cr : root path for config
func (bm *BfeModules) Init(cbs *BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	// go through ALL available module names
	// It is IMPORTANT to do init by the order defined in modulesAll
	for _, name := range modulesAll {
		// check whether this module is enabled
		module, ok := bm.workModules[name]
		if ok {
			// do init for this module
			err := module.Init(cbs, whs, cr)
			if err != nil {
				log.Logger.Error("Err in module.init() for %s [%s]",
					module.Name(), err.Error())
				return err
			}
			log.Logger.Info("%s:Init() OK", module.Name())

			// add to modulesEnabled
			modulesEnabled = append(modulesEnabled, name)
		}
	}
	return nil
}

// ModConfPath get full path of module config file.
//
// format: confRoot/<modName>/<modName>.conf
//
// e.g., confRoot = "/home/bfe/conf", modName = "mod_access"
// return "/home/bfe/conf/mod_access/mod_access.conf"
func ModConfPath(confRoot string, modName string) string {
	confPath := path.Join(confRoot, modName, modName+".conf")
	return confPath
}

// ModConfDir get dir for module config.
//
// format: confRoot/<modName>
//
// e.g., confRoot = "/home/bfe/conf", modName = "mod_access"
// return "/home/bfe/conf/mod_access"
func ModConfDir(confRoot string, modName string) string {
	confDir := path.Join(confRoot, modName)
	return confDir
}

// ModuleStatusGetJSON get modules Available and modules Enabled.
func ModuleStatusGetJSON() ([]byte, error) {
	status := make(map[string][]string)
	status["available"] = modulesAll
	status["enabled"] = modulesEnabled
	return json.Marshal(status)
}
