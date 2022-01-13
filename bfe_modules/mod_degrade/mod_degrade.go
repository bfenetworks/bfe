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

package mod_degrade

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net/url"
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic/action"

	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_bufio"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/spaolacci/murmur3"
)

const (
	ModDegrade = "mod_degrade"
)

type ModuleDegradeState struct {
	ReqTotal     *metrics.Counter // all request in
	ReqDegrade   *metrics.Counter // request degraded
	ReqGoOn      *metrics.Counter // request go on
	WrongCommand *metrics.Counter // request with condition satisfied, but wrong command
}

type ModuleDegrade struct {
	name    string             // name of module
	state   ModuleDegradeState // module state
	metrics metrics.Metrics

	ruleTable       *ProductRuleTable // table for product degrade rules
	productRulePath string
}

func NewModuleDegrade() *ModuleDegrade {
	m := new(ModuleDegrade)
	m.name = ModDegrade
	m.metrics.Init(&m.state, ModDegrade, 0)
	m.ruleTable = NewProductRuleTable()
	return m
}

func (m *ModuleDegrade) Name() string {
	return m.name
}

func (m *ModuleDegrade) processDegradeHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	var (
		ok    bool
		rules degradeRuleConfList
	)

	m.state.ReqTotal.Inc(1)
	// check product rules for given request
	rules, ok = m.ruleTable.Search(request.Route.Product)
	if !ok { // no rules found
		m.state.ReqGoOn.Inc(1)

		return bfe_module.BfeHandlerGoOn, nil
	}

	return m.processProductRuleHandler(request, rules)
}

func (m *ModuleDegrade) checkRequestDegrade(req *bfe_basic.Request, rules degradeRuleConfList) (*DegradeAction, bool) {
	for _, rule := range rules {
		// if current rule unable, fast skip it
		if !rule.Enable {
			continue
		}
		// hash value >= degrade rate, will skip it
		if m.getHash(m.getHashKey(req), 100) >= rule.DegradeRate {
			continue
		}
		// condition not match
		if !rule.Cond.Match(req) {
			continue
		}

		return &rule.Action, true
	}

	return nil, false

}

// cal hash key
// TODO: support more hashkey method
func (m *ModuleDegrade) getHashKey(req *bfe_basic.Request) []byte {
	var hashKey []byte
	if req.ClientAddr != nil {
		hashKey = req.ClientAddr.IP
	}
	if len(hashKey) == 0 {
		hashKey = make([]byte, 8)
		binary.BigEndian.PutUint64(hashKey, rand.Uint64())
	}
	return hashKey
}

func (m *ModuleDegrade) getHash(value []byte, base uint) int {
	var hash uint64

	if value == nil {
		hash = uint64(rand.Uint32())
	} else {
		hash = murmur3.Sum64(value)
	}

	return int(hash % uint64(base))
}

func (m *ModuleDegrade) processProductRuleHandler(req *bfe_basic.Request, rules degradeRuleConfList) (
	int, *bfe_http.Response) {
	var (
		resp    *bfe_http.Response
		daction *DegradeAction
		ok      bool
		err     error
	)
	daction, ok = m.checkRequestDegrade(req, rules)
	if !ok {
		m.state.ReqGoOn.Inc(1)
		return bfe_module.BfeHandlerGoOn, nil
	}

	if daction.Cmd == action.ActionClose {
		m.state.ReqDegrade.Inc(1)
		return bfe_module.BfeHandlerClose, nil
	}

	resp, err = m.newResponse(req, daction)
	if err != nil {
		m.state.ReqGoOn.Inc(1)
		log.Logger.Warn("%s process response is not excepted, error=%s, degrade will skip it", m.name, err)
		return bfe_module.BfeHandlerGoOn, nil
	}
	m.state.ReqDegrade.Inc(1)
	return bfe_module.BfeHandlerResponse, resp
}

func (m *ModuleDegrade) newResponse(req *bfe_basic.Request, action *DegradeAction) (*bfe_http.Response, error) {
	r := bfe_bufio.NewReader(strings.NewReader(action.Rsp))
	return bfe_http.ReadResponse(r, req.HttpRequest)
}

func (m *ModuleDegrade) reloadHandler() map[string]interface{} {
	return map[string]interface{}{
		m.name + ".product_rule_table": m.loadProductRuleConf,
	}
}

func (m *ModuleDegrade) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleDegrade) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleDegrade) monitorHandlers() map[string]interface{} {
	return map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
}

func (m *ModuleDegrade) loadProductRuleConf(query url.Values) error {
	// get path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.productRulePath
	}
	conf, err := productRuleConfLoad(path)
	if err != nil {
		return fmt.Errorf("err in ProductRuleConfLoad(%s):%s", path, err)
	}
	m.ruleTable.Update(conf)
	return nil
}

func (m *ModuleDegrade) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	var (
		err  error
		conf *ConfModDegrade
	)

	// load module config
	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}
	m.productRulePath = conf.Basic.ProductRulePath

	if err = m.loadProductRuleConf(nil); err != nil {
		return fmt.Errorf("%s: loadProductRuleConf() err %s", m.name, err.Error())
	}

	// register handler
	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.processDegradeHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.processDegradeHandler): %s", m.name, err.Error())
	}

	// register web handler for monitor
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.monitorHandlers): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandler())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.reloadHandlers): %s", m.name, err.Error())
	}

	return nil
}
