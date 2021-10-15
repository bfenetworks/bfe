// Copyright (c) 2020 The BFE Authors.
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
package mod_waf

import (
	"errors"
	"fmt"
	"net/url"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	ModWaf = "mod_waf" // mod waf
)

var (
	ErrWaf = errors.New("WAF") // deny by Waf
)

type ModuleWafState struct {
	CheckedReq *metrics.Counter // record how many requests check waf rule

	HitBlockedReq  *metrics.Counter // record how many requests check waf rule
	HitCheckedRule *metrics.Counter // hit checked rule

	BlockedRuleError *metrics.Counter //err times of check blocked rule
	CheckedRuleError *metrics.Counter // err times of check checked rule
}

type ModuleWaf struct {
	name      string          // mod name
	conf      *ConfModWaf     // mod waf config
	handler   *wafHandler     // mod waf handler
	state     ModuleWafState  // state of waf
	ruleTable *WarRuleTable   // rule table of waf
	metrics   metrics.Metrics // metric info of waf
}

func NewModuleWaf() *ModuleWaf {
	m := new(ModuleWaf)
	m.name = ModWaf
	m.handler = NewWafHandler()
	m.metrics.Init(&m.state, m.name, 0)
	m.ruleTable = NewWarRuleTable()
	return m
}

func (m *ModuleWaf) Name() string {
	return m.name
}

func (m *ModuleWaf) loadProductRuleConf(query url.Values) error {
	// get file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.conf.Basic.ProductRulePath
	}

	// load from config file
	conf, err := ProductWafRuleConfLoad(path)
	if err != nil {
		return fmt.Errorf("%s: loadProductRuleConf(%s) error: %v", m.name, path, err)
	}

	// update to rule table
	m.ruleTable.Update(&conf)
	return nil
}

func (m *ModuleWaf) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleWaf) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleWaf) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModuleWaf) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadProductRuleConf,
	}
	return handlers
}

func (m *ModuleWaf) handleWaf(req *bfe_basic.Request) (int, *bfe_http.Response) {
	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn, nil
	}
	for _, rule := range *rules {
		if !rule.Cond.Match(req) {
			continue
		}
		m.state.CheckedReq.Inc(1)
		for _, blockRule := range rule.BlockRules {
			blocked, err := m.handler.HandleBlockJob(blockRule, req)
			if err != nil {
				m.state.BlockedRuleError.Inc(1)
				log.Logger.Debug("ModuleWaf.handleWaf() block job err=%v, rule=%s", err, blockRule)
				continue
			}
			if blocked {
				req.ErrCode = ErrWaf
				m.state.HitBlockedReq.Inc(1)
				return bfe_module.BfeHandlerFinish, nil
			}
		}
		for _, checkRule := range rule.CheckRules {
			hit, err := m.handler.HandleCheckJob(checkRule, req)
			if err != nil {
				m.state.CheckedRuleError.Inc(1)
				log.Logger.Debug("ModuleWaf.handleWaf() checkjob err=%v, rule=%s", err, checkRule)
				continue
			}
			if hit {
				m.state.HitCheckedRule.Inc(1)
			}
		}
		break
	}
	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleWaf) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	var err error

	confPath := bfe_module.ModConfPath(cr, m.Name())
	if m.conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %v", m.name, err)
	}

	if err = m.loadProductRuleConf(nil); err != nil {
		return fmt.Errorf("%s: loadProductRuleConf() err %v", m.Name(), err)
	}

	if err = m.handler.Init(m.conf); err != nil {
		return fmt.Errorf("%s: handler.Init() err %v", m.Name(), err)
	}

	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.handleWaf)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.handleWaf): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.monitorHandlers): %v", m.Name(), err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.reloadHandlerr): %v", m.Name(), err)
	}
	return nil
}
