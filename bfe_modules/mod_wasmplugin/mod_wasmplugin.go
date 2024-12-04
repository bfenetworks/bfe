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
	"net/url"

	_ "github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_wasmplugin"
)

const (
	ModWasm = "mod_wasm"
	ModWasmBeforeLocationKey = "mod_wasm_before_location_key"
)

var (
	openDebug = false
)

type ModuleWasm struct {
	name          string
	wasmPluginPath string // path of Wasm plugins
	configPath string // path of config file
	pluginTable   *PluginTable
}

func NewModuleWasm() *ModuleWasm {
	m := new(ModuleWasm)
	m.name = ModWasm
	m.pluginTable = NewPluginTable()
	return m
}

func (m *ModuleWasm) Name() string {
	return m.name
}

func (m *ModuleWasm) loadConfData(query url.Values) error {
	path := query.Get("path")
	if path == "" {
		path = m.configPath
	}

	// load from config file
	conf, err := pluginConfLoad(path)

	if err != nil {
		return fmt.Errorf("err in pluginConfLoad(%s):%s", path, err.Error())
	}

	// update to plugin table
	return updatePluginConf(m.pluginTable, conf, m.wasmPluginPath)
}

func (m *ModuleWasm) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var conf *ConfModWasm

	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: cond load err %s", m.name, err.Error())
	}

	// init wasm engine

	return m.init(conf, cbs, whs)
}

func (m *ModuleWasm) init(cfg *ConfModWasm, cbs *bfe_module.BfeCallbacks,
	whs *web_monitor.WebHandlers) error {
	openDebug = cfg.Log.OpenDebug
	
	m.wasmPluginPath = cfg.Basic.WasmPluginPath
	m.configPath = cfg.Basic.DataPath

	// load plugins from WasmPluginPath
	// construct plugin list
	if err := m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %s", err.Error())
	}

	// register handler
	err := cbs.AddFilter(bfe_module.HandleBeforeLocation, m.wasmBeforeLocationHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.wasmBeforeLocationHandler): %s", m.name, err.Error())
	}

	// register handler
	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.wasmRequestHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.HandleFoundProduct): %s", m.name, err.Error())
	}

	// register handler
	err = cbs.AddFilter(bfe_module.HandleReadResponse, m.wasmResponseHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.HandleReadResponse): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %s", m.name, err.Error())
	}
	
	return nil
}

// 
func (m *ModuleWasm) wasmBeforeLocationHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	var pl []bfe_wasmplugin.WasmPlugin
	rl := m.pluginTable.GetBeforeLocationRules()
	for _, rule := range rl {
		if rule.Cond.Match(request) {
			// find pluginlist
			pl = rule.PluginList
			break
		}
	}

	var resp *bfe_http.Response
	if pl != nil {
		// do the pluginlist
		retCode := bfe_module.BfeHandlerGoOn
		var fl []*bfe_wasmplugin.Filter
		for _, plug := range pl {
			filter := bfe_wasmplugin.NewFilter(plug, request)
			var ret int
			ret, resp = filter.RequestHandler(request)
			fl = append(fl, filter)
			if ret != bfe_module.BfeHandlerGoOn {
				retCode = ret
				break
			}
		}

		request.Context[ModWasmBeforeLocationKey] = fl
		return retCode, resp
	}

	return bfe_module.BfeHandlerGoOn, resp
}

// 
func (m *ModuleWasm) wasmRequestHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	var pl []bfe_wasmplugin.WasmPlugin
	rl, _ := m.pluginTable.Search(request.Route.Product)
	for _, rule := range rl {
		if rule.Cond.Match(request) {
			// find pluginlist
			pl = rule.PluginList
			break
		}
	}

	var resp *bfe_http.Response
	if pl != nil {
		// do the pluginlist
		retCode := bfe_module.BfeHandlerGoOn
		var fl []*bfe_wasmplugin.Filter
		for _, plug := range pl {
			filter := bfe_wasmplugin.NewFilter(plug, request)
			var ret int
			ret, resp = filter.RequestHandler(request)
			fl = append(fl, filter)
			if ret != bfe_module.BfeHandlerGoOn {
				retCode = ret
				break
			}
		}

		var fl0 []*bfe_wasmplugin.Filter
		val, ok := request.Context[ModWasmBeforeLocationKey]
		if ok {
			fl0 = val.([]*bfe_wasmplugin.Filter)
		}

		fl0 = append(fl0, fl...)
		request.Context[ModWasmBeforeLocationKey] = fl0
		return retCode, resp
	}

	return bfe_module.BfeHandlerGoOn, resp
}

//
func (m *ModuleWasm) wasmResponseHandler(request *bfe_basic.Request, res *bfe_http.Response) int {
	val, ok := request.Context[ModWasmBeforeLocationKey]

	if ok {
		fl, matched := val.([]*bfe_wasmplugin.Filter)
		if !matched {
			// error
			return bfe_module.BfeHandlerGoOn
		}

		n := len(fl)
		for i := n-1; i >= 0; i-- {
			fl[i].ResponseHandler(request)
			fl[i].OnDestroy()
		}
	}

	return bfe_module.BfeHandlerGoOn
}
