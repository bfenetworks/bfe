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

package mod_prison

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/action"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

var (
	openDebug = false
)

const (
	ModPrison        = "mod_prison"
	ReqCtxPrisonInfo = "mod_prison.prison_info"
)

var (
	ErrPrison = errors.New("PRISON") // deny by mod_prison
)

type ModulePrisonState struct {
	AllChecked *metrics.Counter // count of checked requests
	AllPrison  *metrics.Counter // count of blocked requests
}

type PrisonInfo struct {
	PrisonType string    // type of prison, mod_prison
	PrisonName string    // name of prison rule
	FreeTime   time.Time // free time
	IsExpired  bool      // is expired
	Action     string    // action
}

type ModulePrison struct {
	name            string            // name of module
	state           ModulePrisonState // module state
	metrics         metrics.Metrics
	productConfPath string            // path for prodct rule
	productTable    *productRuleTable // product rule table
}

func NewModulePrison() *ModulePrison {
	m := new(ModulePrison)
	m.name = ModPrison
	m.metrics.Init(&m.state, ModPrison, 0)
	m.productTable = newProductRuleTable()
	return m
}

func (m *ModulePrison) Name() string {
	return m.name
}

func (m *ModulePrison) prisonHandler(req *bfe_basic.Request) (
	int, *bfe_http.Response) {
	// process global prison rules
	product := bfe_basic.GlobalProduct
	ret, res := m.processProductRules(req, product)
	if ret != bfe_module.BfeHandlerGoOn {
		return ret, res
	}

	// process product prison rules
	product = req.Route.Product
	ret, res = m.processProductRules(req, product)
	return ret, res
}

func (m *ModulePrison) processProductRules(req *bfe_basic.Request, product string) (int, *bfe_http.Response) {
	rules, ok := m.productTable.getRules(product)
	if !ok {
		if openDebug {
			log.Logger.Debug("product[%s] without prison rules, pass", product)
		}
		return bfe_module.BfeHandlerGoOn, nil
	}

	return m.processRules(req, rules)
}

func (m *ModulePrison) processRules(req *bfe_basic.Request, rules *prisonRules) (int, *bfe_http.Response) {
	for _, rule := range rules.ruleList {
		if !rule.cond.Match(req) {
			continue
		}

		m.state.AllChecked.Inc(1)
		if !rule.recordAndCheck(req) {
			continue
		}

		m.state.AllPrison.Inc(1)
		switch rule.action.Cmd {
		case action.ActionClose:
			req.ErrCode = ErrPrison
			return bfe_module.BfeHandlerClose, nil
		case action.ActionFinish:
			req.ErrCode = ErrPrison
			return bfe_module.BfeHandlerFinish, nil
		default:
			rule.action.Do(req)
		}
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModulePrison) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModulePrison) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModulePrison) loadProductRuleTable(query url.Values) (string, error) {
	// get reload file path
	path := query.Get("path")
	if path == "" {
		path = m.productConfPath // use default
	}

	// load and update rules
	productConf, err := productRuleConfLoad(path)
	if err != nil {
		return "", fmt.Errorf("%s: load product rule err %s", m.name, err.Error())
	}
	if err = m.productTable.load(productConf); err != nil {
		return "", fmt.Errorf("%s: load prison err %s", m.name, err.Error())
	}

	version := *productConf.Version
	_, fileName := filepath.Split(path)
	return fmt.Sprintf("%s=%s", fileName, version), nil
}

func (m *ModulePrison) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModulePrison) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	// load module config
	confPath := bfe_module.ModConfPath(cr, m.name)
	conf, err := ConfLoad(confPath, cr)
	if err != nil {
		return fmt.Errorf("%s.Init():load conf err %s", m.name, err.Error())
	}
	m.productConfPath = conf.Basic.ProductRulePath
	openDebug = conf.Log.OpenDebug

	// load product rule table
	if _, err := m.loadProductRuleTable(nil); err != nil {
		return fmt.Errorf("%s.Init():loadProductRuleTable(): %s", m.name, err.Error())
	}

	// register handler for prison
	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.prisonHandler)
	if err != nil {
		return fmt.Errorf("%s.Init():AddFilter(m.prisonHandler): %s", m.name, err.Error())
	}

	// register web handler for monitor
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.monitorHandlers): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadProductRuleTable)
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.reloadHandlers): %s", m.name, err.Error())
	}

	return nil
}
