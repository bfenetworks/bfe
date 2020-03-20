// Copyright (c) 2019 Baidu, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     bfe_http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mod_doh

import (
	"fmt"
	"net/url"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_http"
	"github.com/baidu/bfe/bfe_module"
)

const (
	ModDoh = "mod_doh"
)

var (
	openDebug = false
)

type ModuleDohState struct {
	DohRequest           *metrics.Counter
	FetchDnsErr          *metrics.Counter
	ConvertToResponseErr *metrics.Counter
}

type ModuleDoh struct {
	name       string
	state      ModuleDohState
	metrics    metrics.Metrics
	conf       *ConfModDoh
	ruleTable  *DohRuleTable
	dnsFetcher DnsFetcherIF
}

func NewModuleDoh() *ModuleDoh {
	m := new(ModuleDoh)
	m.name = ModDoh
	m.metrics.Init(&m.state, ModDoh, 0)
	m.ruleTable = NewDohRuleTable()
	m.dnsFetcher = new(DnsFetcher)
	return m
}

func (m *ModuleDoh) Name() string {
	return m.name
}

func (m *ModuleDoh) loadConfData(query url.Values) error {
	path := query.Get("path")
	if path == "" {
		path = m.conf.Basic.DataPath
	}

	conf, err := DohConfLoad(path)
	if err != nil {
		return fmt.Errorf("error in DohConfLoad(%s): %v", path, err)
	}

	m.ruleTable.Update(conf)
	return nil
}

func (m *ModuleDoh) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleDoh) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleDoh) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModuleDoh) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadConfData,
	}
	return handlers
}

func (m *ModuleDoh) dohHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn, nil
	}

	for _, rule := range *rules {
		if !rule.Cond.Match(req) {
			continue
		}

		m.state.DohRequest.Inc(1)
		msg, err := m.dnsFetcher.Fetch(req.HttpRequest, rule.Net, rule.Address)
		if err != nil {
			if openDebug {
				log.Logger.Debug("%s: fetchDNS error: %v", m.name, err)
			}

			m.state.FetchDnsErr.Inc(1)
			return bfe_module.BfeHandlerResponse, bfe_basic.CreateInternalResp(req, bfe_http.StatusInternalServerError)
		}

		resp, err := DnsMsgToResponse(req, msg)
		if err != nil {
			if openDebug {
				log.Logger.Debug("%s: DnsMsgToResponse error: %v", m.name, err)
			}

			m.state.ConvertToResponseErr.Inc(1)
			return bfe_module.BfeHandlerResponse, bfe_basic.CreateInternalResp(req, bfe_http.StatusInternalServerError)
		}

		return bfe_module.BfeHandlerResponse, resp
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleDoh) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var cfg *ConfModDoh

	confPath := bfe_module.ModConfPath(cr, m.name)
	if cfg, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err: %v", m.name, err)
	}
	openDebug = cfg.Log.OpenDebug
	m.conf = cfg

	if err = m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %v", err)
	}

	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.dohHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.dohHandler): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.monitorHandlers): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(): %v", m.name, err)
	}

	return nil
}