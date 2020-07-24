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

package mod_errors

import (
	"fmt"
	"net/url"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

var (
	openDebug = false
)

const (
	MaxPageSize = 2 * 1024 * 1024
)

type ModuleErrors struct {
	name       string           // name of module
	configPath string           // path of config file
	ruleTable  *ErrorsRuleTable // table of errors rules
}

func NewModuleErrors() *ModuleErrors {
	m := new(ModuleErrors)
	m.name = "mod_errors"
	m.ruleTable = NewErrorsRuleTable()
	return m
}

func (m *ModuleErrors) Name() string {
	return m.name
}

func (m *ModuleErrors) loadConfData(query url.Values) error {
	// get file path
	path := query.Get("path")
	if path == "" {
		path = m.configPath // use default
	}

	// load from config file
	conf, err := ErrorsConfLoad(path)
	if err != nil {
		return fmt.Errorf("err in ErrorsConfLoad(%s):%s", path, err.Error())
	}

	// update to rule table
	m.ruleTable.Update(conf)
	return nil
}

func (m *ModuleErrors) errorsHandler(req *bfe_basic.Request, resp *bfe_http.Response) int {
	if req.HttpResponse == nil {
		// never go here
		log.Logger.Debug("%s:errorsHandler(): no response found", m.name)
		return bfe_module.BfeHandlerGoOn
	}

	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn
	}

	for _, rule := range *rules {
		// condition is satisfiled?
		if rule.Cond.Match(req) {
			// do actions of the rule
			ErrorsActionsDo(req, rule.Actions)
			return bfe_module.BfeHandlerGoOn
		}
	}
	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleErrors) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var cfg *ConfModErrors

	confPath := bfe_module.ModConfPath(cr, m.name)
	if cfg, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}

	openDebug = cfg.Log.OpenDebug

	m.configPath = cfg.Basic.DataPath

	// load from config file to rule table
	if err = m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %s", err.Error())
	}

	// register handler
	err = cbs.AddFilter(bfe_module.HandleReadResponse, m.errorsHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.errorsHandler): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %s", m.name, err.Error())
	}

	return nil
}
