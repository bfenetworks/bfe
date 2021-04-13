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

package mod_block

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
	"github.com/bfenetworks/bfe/bfe_util/ipdict"
)

const (
	ModBlock     = "mod_block"
	CtxBlockInfo = "mod_block.block_info"
)

var (
	ErrBlock = errors.New("BLOCK")
)

var (
	openDebug = false
)

type ModuleBlockState struct {
	ConnTotal    *metrics.Counter // all connnetion checked
	ConnAccept   *metrics.Counter // connection passed
	ConnRefuse   *metrics.Counter // connection refused
	ReqTotal     *metrics.Counter // all request in
	ReqAccept    *metrics.Counter // request accepted
	ReqRefuse    *metrics.Counter // request refused
	WrongCommand *metrics.Counter // request with condition satisfied, but wrong command
}

type BlockInfo struct {
	BlockRuleName string // block rule name
}

type ModuleBlock struct {
	name    string           // name of module
	state   ModuleBlockState // module state
	metrics metrics.Metrics

	productRulePath string // path of block rule data file
	ipBlocklistPath string // path of ip blocklist data file

	ruleTable *ProductRuleTable // table for product block rules
	ipTable   *ipdict.IPTable   // table for global ip blocklist
}

func NewModuleBlock() *ModuleBlock {
	m := new(ModuleBlock)
	m.name = ModBlock
	m.metrics.Init(&m.state, ModBlock, 0)

	m.ruleTable = NewProductRuleTable()
	m.ipTable = ipdict.NewIPTable()

	return m
}

func (m *ModuleBlock) Name() string {
	return m.name
}

// loadGlobalIPTable loads global ip blocklist.
func (m *ModuleBlock) loadGlobalIPTable(query url.Values) error {
	// get reload file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.ipBlocklistPath
	}

	// load data
	items, err := GlobalIPTableLoad(path)
	if err != nil {
		return fmt.Errorf("err in GlobalIPTableLoad(%s):%s", path, err)
	}

	m.ipTable.Update(items)
	return nil
}

// loadProductRuleConf load from config file.
func (m *ModuleBlock) loadProductRuleConf(query url.Values) error {
	// get path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.productRulePath
	}

	// load file
	conf, err := ProductRuleConfLoad(path)
	if err != nil {
		return fmt.Errorf("err in ProductRuleConfLoad(%s):%s", path, err)
	}

	m.ruleTable.Update(conf)
	return nil
}

// globalBlockHandler is a handler for doing global block.
func (m *ModuleBlock) globalBlockHandler(session *bfe_basic.Session) int {
	if openDebug {
		log.Logger.Debug("%s check connection (remote: %v)",
			m.name, session.RemoteAddr)
	}
	m.state.ConnTotal.Inc(1)

	clientIP := session.RemoteAddr.IP
	if m.ipTable.Search(clientIP) {
		session.SetError(ErrBlock, "connection blocked")
		log.Logger.Debug("%s refuse connection (remote: %v)",
			m.name, session.RemoteAddr)
		m.state.ConnRefuse.Inc(1)
		return bfe_module.BfeHandlerClose
	}

	if openDebug {
		log.Logger.Debug("%s accept connection (remote: %v)",
			m.name, session.RemoteAddr)
	}
	m.state.ConnAccept.Inc(1)
	return bfe_module.BfeHandlerGoOn
}

// productBlockHandler is a handler for doing product block.
func (m *ModuleBlock) productBlockHandler(request *bfe_basic.Request) (
	int, *bfe_http.Response) {
	if openDebug {
		log.Logger.Debug("%s check request", m.name)
	}
	m.state.ReqTotal.Inc(1)

	// check global rules for given request
	rules, ok := m.ruleTable.Search(bfe_basic.GlobalProduct)
	if ok { // rules found
		retVal, isMatch, resp := m.productRulesProcess(request, rules)
		if isMatch {
			return retVal, resp
		}
	}
	// check product rules for given request
	rules, ok = m.ruleTable.Search(request.Route.Product)
	if !ok { // no rules found
		if openDebug {
			log.Logger.Debug("%s product %s not found, just pass",
				m.name, request.Route.Product)
		}
		return bfe_module.BfeHandlerGoOn, nil
	}

	retVal, isMatch, resp := m.productRulesProcess(request, rules)
	if !isMatch {
		m.state.ReqAccept.Inc(1)
	}
	return retVal, resp
}

func (m *ModuleBlock) productRulesProcess(req *bfe_basic.Request, rules *blockRuleList) (
	int, bool, *bfe_http.Response) {
	for _, rule := range *rules {
		if openDebug {
			log.Logger.Debug("%s process rule: %v", m.name, rule)
		}

		// rule condition is satisfied ?
		if rule.Cond.Match(req) {
			// set block info name
			blockInfo := &BlockInfo{BlockRuleName: rule.Name}
			req.SetContext(CtxBlockInfo, blockInfo)

			switch rule.Action.Cmd {
			case "ALLOW":
				if openDebug {
					log.Logger.Debug("%s accept request", m.name)
				}
				m.state.ReqAccept.Inc(1)
				return bfe_module.BfeHandlerGoOn, true, nil
			case "CLOSE":
				req.ErrCode = ErrBlock
				log.Logger.Debug("%s block connection (rule:%v, remote:%s)",
					m.name, rule, req.RemoteAddr)
				m.state.ReqRefuse.Inc(1)
				return bfe_module.BfeHandlerClose, true, nil
			default:
				if openDebug {
					log.Logger.Debug("%s unknown block command (%s), just pass",
						rule.Action.Cmd)
				}
				m.state.WrongCommand.Inc(1)
			}
		}
	}

	if openDebug {
		log.Logger.Debug("%s accept request", m.name)
	}
	return bfe_module.BfeHandlerGoOn, false, nil
}

func (m *ModuleBlock) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleBlock) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleBlock) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModuleBlock) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name + ".global_ip_table":    m.loadGlobalIPTable,
		m.name + ".product_rule_table": m.loadProductRuleConf,
	}
	return handlers
}

func (m *ModuleBlock) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var conf *ConfModBlock
	var err error

	// load module config
	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}

	m.productRulePath = conf.Basic.ProductRulePath
	m.ipBlocklistPath = conf.Basic.IPBlocklistPath
	openDebug = conf.Log.OpenDebug

	// load conf data
	if err = m.loadGlobalIPTable(nil); err != nil {
		return fmt.Errorf("%s: loadGlobalIPTable() err %s", m.name, err.Error())
	}
	if err = m.loadProductRuleConf(nil); err != nil {
		return fmt.Errorf("%s: loadProductRuleConf() err %s", m.name, err.Error())
	}

	// register handler
	err = cbs.AddFilter(bfe_module.HandleAccept, m.globalBlockHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.globalBlockHandler): %s", m.name, err.Error())
	}

	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.productBlockHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.productBlockHandler): %s", m.name, err.Error())
	}

	// register web handler for monitor
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.monitorHandlers): %s", m.name, err.Error())
	}
	// register web handler for reload
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.reloadHandlers): %s", m.name, err.Error())
	}

	return nil
}
