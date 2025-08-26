// Copyright (c) 2025 The BFE Authors.
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

package mod_body_process

import (
	"fmt"
	"net/url"

	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	ModBodyProcess = "mod_body_process"
	BodyProcessResponseConfigKey = "mod_body_process.response_config"
)

var (
	openDebug = false
)

type ModuleBodyProcessState struct {
	ReqTotal           *metrics.Counter
	ReqProcess         *metrics.Counter
	ResProcess         *metrics.Counter
}

type ModuleBodyProcess struct {
	name      string
	conf      *ConfModBodyProcess
	ruleTable *ProcessRuleTable
	state     ModuleBodyProcessState
	metrics   metrics.Metrics
}

func NewModuleBodyProcess() *ModuleBodyProcess {
	m := new(ModuleBodyProcess)
	m.name = ModBodyProcess
	m.metrics.Init(&m.state, ModBodyProcess, 0)
	m.ruleTable = NewTokenRuleTable()
	return m
}

func (m *ModuleBodyProcess) Name() string {
	return m.name
}

func (m *ModuleBodyProcess) loadProductRuleConf(query url.Values) error {
	path := query.Get("path")
	if path == "" {
		path = m.conf.Basic.ProductRulePath
	}

	conf, err := ProductRuleConfLoad(path)
	if err != nil {
		return fmt.Errorf("err in ProductRuleConfLoad(%s): %s", path, err)
	}

	m.ruleTable.Update(conf)
	return nil
}

func (m *ModuleBodyProcess) matchProcessRule(req *bfe_basic.Request) *processRule {
	if openDebug {
		log.Logger.Debug("%s check request", m.name)
	}
	m.state.ReqTotal.Inc(1)

	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		if openDebug {
			log.Logger.Debug("%s product %s not found, just pass", m.name, req.Route.Product)
		}
		return nil
	}

	for _, rule := range rules {
		if openDebug {
			log.Logger.Debug("%s process rule: %v", m.name, rule)
		}

		if rule.Cond.Match(req) {
			return &rule
		}
	}

	return nil
}

// found product handler
func (m *ModuleBodyProcess) afterLocationHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	matchedRule := m.matchProcessRule(req)
	if matchedRule == nil {
		// no rule, just pass
		return bfe_module.BfeHandlerGoOn, nil
	}

	// add body processor
	if openDebug {
		log.Logger.Debug("%s found matched rule: %v", m.name, matchedRule)
	}

	m.DoRequestProcess(req, matchedRule.RequestProcess)

	if matchedRule.ResponseProcess != nil {
		req.SetContext(BodyProcessResponseConfigKey, matchedRule.ResponseProcess)
	}
	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleBodyProcess) readResponseHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	var conf *BodyProcessConfig
	// get response config from request context
	data := req.GetContext(BodyProcessResponseConfigKey)
	if data != nil {
		var ok bool
		conf, ok = data.(*BodyProcessConfig)
		if !ok {
			log.Logger.Warn("%s: type assertion fail, %v", m.name, data)
		}
	}
	
	m.DoResponseProcess(req, res, conf)

	return bfe_module.BfeHandlerGoOn
}
/*
func (m *ModuleBodyProcess) readResponseHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	data := req.GetContext(BodyProcessResponseConfigKey)
	if data != nil {
		conf, ok := data.(*BodyProcessConfig)
		if !ok {
			log.Logger.Warn("%s: type assertion fail, %v", m.name, data)
			return bfe_module.BfeHandlerGoOn
		}
		m.DoResponseProcess(req, res, conf)
	}
	return bfe_module.BfeHandlerGoOn
}
*/
func (m *ModuleBodyProcess) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleBodyProcess) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleBodyProcess) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModuleBodyProcess) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadProductRuleConf,
	}
	return handlers
}

func (m *ModuleBodyProcess) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error

	confPath := bfe_module.ModConfPath(cr, m.name)
	if m.conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %v", m.name, err)
	}
	openDebug = m.conf.Log.OpenDebug

	if err = m.loadProductRuleConf(nil); err != nil {
		return fmt.Errorf("%s: loadProductRuleConf() err %v", m.name, err)
	}

	err = cbs.AddFilter(bfe_module.HandleAfterLocation, m.afterLocationHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.foundProductHandler): %s", m.name, err.Error())
	}

	err = cbs.AddFilter(bfe_module.HandleReadResponse, m.readResponseHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.readResponseHandler): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.monitorHandlers): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.reloadHandlerr): %v", m.name, err)
	}

	return nil
}
