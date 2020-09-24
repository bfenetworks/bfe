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

// module for marking rewrite

package mod_rewrite

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

type ModuleReWrite struct {
	name string // name of module

	configPath string        // path of config file
	ruleTable  *ReWriteTable // table of rewrite rules
}

func NewModuleReWrite() *ModuleReWrite {
	m := new(ModuleReWrite)
	m.name = "mod_rewrite"
	m.ruleTable = NewReWriteTable()

	return m
}

func (m *ModuleReWrite) Name() string {
	return m.name
}

func (m *ModuleReWrite) loadConfData(query url.Values) error {
	// get file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.configPath
	}

	// load from config file
	conf, err := ReWriteConfLoad(path)
	if err != nil {
		return fmt.Errorf("err in ReWriteConfLoad(%s):%s", path, err.Error())
	}

	// update to rule table
	m.ruleTable.Update(conf)

	return nil
}

// ReqReWrite do rewrite to http request, with given rewrite rules.
func ReqReWrite(req *bfe_basic.Request, rules *RuleList) {
	for _, rule := range *rules {
		// rule condition is satisfied ?
		if rule.Cond.Match(req) {
			// do actions of the rule
			reWriteActionsDo(req, rule.Actions)

			// flag of last is true?
			if rule.Last {
				break
			}
		}
	}
}

// rewriteHandler is a handler for doing rewrite.
func (m *ModuleReWrite) rewriteHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	// find rewrite rules for given request
	rules, ok := m.ruleTable.Search(request.Route.Product)

	if ok {
		if openDebug {
			log.Logger.Debug("%s:before:host=%s, path=%s, query=%s, rules=%v", m.name,
				request.HttpRequest.Host, request.HttpRequest.URL.Path,
				request.HttpRequest.URL.RawQuery, rules)
		}

		// do rewrite to request, according to rules
		ReqReWrite(request, rules)

		if openDebug {
			log.Logger.Debug("%s:after:host=%s, path=%s, query=%s", m.name,
				request.HttpRequest.Host, request.HttpRequest.URL.Path,
				request.HttpRequest.URL.RawQuery)
		}
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleReWrite) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var conf *ConfModReWrite

	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}

	return m.init(conf, cbs, whs)
}

func (m *ModuleReWrite) init(cfg *ConfModReWrite, cbs *bfe_module.BfeCallbacks,
	whs *web_monitor.WebHandlers) error {
	openDebug = cfg.Log.OpenDebug

	m.configPath = cfg.Basic.DataPath

	// load from config file to rule table
	if err := m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %s", err.Error())
	}

	// register handler
	err := cbs.AddFilter(bfe_module.HandleAfterLocation, m.rewriteHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.rewriteHandler): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %s", m.name, err.Error())
	}

	return nil
}
