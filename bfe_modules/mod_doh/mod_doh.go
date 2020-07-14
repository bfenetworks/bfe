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

package mod_doh

import (
	"fmt"
)

import (
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	ModDoh = "mod_doh"
)

var (
	openDebug = false
)

type ModuleDohState struct {
	DohRequest          *metrics.Counter
	DohRequestNotSecure *metrics.Counter
	FetchDnsErr         *metrics.Counter
}

type ModuleDoh struct {
	name       string
	state      ModuleDohState
	metrics    metrics.Metrics
	conf       *ConfModDoh
	cond       condition.Condition
	dnsFetcher DnsFetcher
}

func NewModuleDoh() *ModuleDoh {
	m := new(ModuleDoh)
	m.name = ModDoh
	m.metrics.Init(&m.state, ModDoh, 0)
	return m
}

func (m *ModuleDoh) Name() string {
	return m.name
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

func (m *ModuleDoh) dohHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	if !m.cond.Match(req) {
		return bfe_module.BfeHandlerGoOn, nil
	}

	m.state.DohRequest.Inc(1)
	if !req.Session.IsSecure {
		m.state.DohRequestNotSecure.Inc(1)
		return bfe_module.BfeHandlerResponse,
			bfe_basic.CreateInternalResp(req, bfe_http.StatusForbidden)
	}

	resp, err := m.dnsFetcher.Fetch(req)
	if err != nil {
		m.state.FetchDnsErr.Inc(1)
		return bfe_module.BfeHandlerResponse,
			bfe_basic.CreateInternalResp(req, bfe_http.StatusInternalServerError)
	}

	return bfe_module.BfeHandlerResponse, resp
}

func (m *ModuleDoh) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var cfg *ConfModDoh

	confPath := bfe_module.ModConfPath(cr, m.name)
	if cfg, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s.Init(): conf load err: %v", m.name, err)
	}
	openDebug = cfg.Log.OpenDebug
	m.conf = cfg
	m.dnsFetcher = NewDnsClient(&cfg.Dns)

	if m.cond, err = condition.Build(cfg.Basic.Cond); err != nil {
		return fmt.Errorf("%s.Init(): err in condition Build(): %v", m.name, err)
	}

	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.dohHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.dohHandler): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.monitorHandlers): %v", m.name, err)
	}

	return nil
}
