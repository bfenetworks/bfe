// Copyright (c) 2026 The BFE Authors.
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

package mod_ai_route

import (
	"fmt"
	"net/url"

	"github.com/bfenetworks/go-lib/log"
	"github.com/bfenetworks/go-lib/web-monitor/metrics"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const ModAiRoute = "mod_ai_route"

var openDebug = false

type ModuleAiRouteState struct {
	ReqTotal     *metrics.Counter
	ReqHitApikey *metrics.Counter
	ReqHitEntity *metrics.Counter
	ReqHitGlobal *metrics.Counter
	ReqMiss      *metrics.Counter
	ReqFallback  *metrics.Counter
}

type ModuleAiRoute struct {
	name       string
	conf       *ConfModAiRoute
	routeTable *AiRouteTable
	state      ModuleAiRouteState
	metrics    metrics.Metrics
}

func NewModuleAiRoute() *ModuleAiRoute {
	m := new(ModuleAiRoute)
	m.name = ModAiRoute
	m.metrics.Init(&m.state, ModAiRoute, 0)
	m.routeTable = NewAiRouteTable()
	return m
}

func (m *ModuleAiRoute) Name() string {
	return m.name
}

func (m *ModuleAiRoute) loadRouteRuleConf(query url.Values) error {
	path := query.Get("path")
	if path == "" {
		path = m.conf.Basic.RouteRulePath
	}

	data, err := AiRouteDataLoad(path)
	if err != nil {
		return fmt.Errorf("err in AiRouteDataLoad(%s): %s", path, err)
	}

	if err := m.routeTable.Update(data); err != nil {
		return fmt.Errorf("err in routeTable.Update: %s", err)
	}

	return nil
}

func (m *ModuleAiRoute) routeFoundProductHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	m.state.ReqTotal.Inc(1)

	aiMeta := req.GetAiBasicInfo()
	if aiMeta == nil {
		return bfe_module.BfeHandlerGoOn, nil
	}

	apiKey := aiMeta.ClientApiKey
	if apiKey == "" {
		if openDebug {
			log.Logger.Debug("%s: api key empty, skip", m.name)
		}
		return bfe_module.BfeHandlerGoOn, nil
	}

	result := m.routeTable.Search(apiKey, req)
	if result == nil {
		m.state.ReqMiss.Inc(1)
		if openDebug {
			log.Logger.Debug("%s: no route hit for apiKey[%s]", m.name, apiKey)
		}
		return bfe_module.BfeHandlerGoOn, nil
	}

	switch result.RouteType {
	case RouteTypeApikey:
		m.state.ReqHitApikey.Inc(1)
	case RouteTypeEntity:
		m.state.ReqHitEntity.Inc(1)
	case RouteTypeGlobal:
		m.state.ReqHitGlobal.Inc(1)
	}

	req.SetAiRouteResult(result)

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleAiRoute) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	confPath := bfe_module.ModConfPath(cr, m.name)
	var err error
	if m.conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %v", m.name, err)
	}
	openDebug = m.conf.Log.OpenDebug

	if err := m.loadRouteRuleConf(nil); err != nil {
		return fmt.Errorf("%s: loadRouteRuleConf err %v", m.name, err)
	}

	if err := cbs.AddFilter(bfe_module.HandleFoundProduct, m.routeFoundProductHandler); err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(routeFoundProductHandler): %s", m.name, err.Error())
	}

	monitorHandlers := map[string]interface{}{
		m.name: m.getState,
	}
	if err := web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, monitorHandlers); err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(monitor): %v", m.name, err)
	}

	reloadHandlers := map[string]interface{}{
		m.name: m.loadRouteRuleConf,
	}
	if err := web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, reloadHandlers); err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(reload): %v", m.name, err)
	}

	return nil
}

func (m *ModuleAiRoute) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}
