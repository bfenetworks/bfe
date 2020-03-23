// Copyright (c) 2019 Baidu, Inc.
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

package bfe_plugin

import (
	"fmt"
	goplugin "plugin"
)

import (
	"github.com/baidu/bfe/bfe_module"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

type Plugins struct {
	workPlugins map[string]*PluginInfo // work plugins, configure in bfe conf file
}

// NewPlugins create new Plugins
func NewPlugins() *Plugins {
	pl := new(Plugins)
	pl.workPlugins = make(map[string]*PluginInfo)

	return pl
}

// RegisterPlugin loads a plugin created with `go build -buildmode=plugin`
func (p *Plugins) RegisterPlugin(path string) error {
	plugin, err := goplugin.Open(path)
	if err != nil {
		return fmt.Errorf("RegisterPlugin Open plugin path %v err:%v", path, err)
	}

	nameSym, err := plugin.Lookup("Name")
	if err != nil {
		return fmt.Errorf("RegisterPlugin Lookup Name err:%v", err)
	}

	initSym, err := plugin.Lookup("Init")
	if err != nil {
		return fmt.Errorf("RegisterPlugin Lookup Init err:%v", err)
	}

	pluginInfo := &PluginInfo{
		Name: *nameSym.(*string),
		Init: initSym.(func(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error),
	}
	p.workPlugins[pluginInfo.Name] = pluginInfo

	return nil
}

// Init initializes bfe plugins.
//
// Params:
//     - cbs: BfeCallbacks
//     - whs: WebHandlers
//     - cr : root path for config
func (p *Plugins) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	for _, pl := range p.workPlugins {
		if err := pl.Init(cbs, whs, cr); err != nil {
			log.Logger.Error("Err in plugin.init() for %s [%s]",
				pl.Name, err.Error())
			return err
		}

		log.Logger.Info("%s:Init() OK", pl.Name)
	}

	return nil
}
