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

package mod_auth_basic

import (
	"fmt"
	"net/url"
)

import (
	auth "github.com/abbot/go-http-auth"
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
	ModAuthBasic = "mod_auth_basic"
)

type ModuleAuthBasicState struct {
	ReqAuthRuleHit   *metrics.Counter
	ReqAuthChallenge *metrics.Counter
	ReqAuthSuccess   *metrics.Counter
	ReqAuthFailure   *metrics.Counter
}

type ModuleAuthBasic struct {
	name       string
	state      ModuleAuthBasicState
	metrics    metrics.Metrics
	configPath string
	ruleTable  *AuthBasicRuleTable
}

var (
	openDebug = false
)

func NewModuleAuthBasic() *ModuleAuthBasic {
	m := new(ModuleAuthBasic)
	m.name = ModAuthBasic
	m.metrics.Init(&m.state, ModAuthBasic, 0)
	m.ruleTable = NewAuthBasicRuleTable()
	return m
}

func (m *ModuleAuthBasic) Name() string {
	return m.name
}

func (m *ModuleAuthBasic) loadConfData(query url.Values) error {
	path := query.Get("path")
	if path == "" {
		path = m.configPath
	}

	conf, err := AuthBasicConfLoad(path)
	if err != nil {
		return fmt.Errorf("error in AuthBasicConfLoad(%s): %v", path, err)
	}

	m.ruleTable.Update(conf)
	return nil
}

func (m *ModuleAuthBasic) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleAuthBasic) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleAuthBasic) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModuleAuthBasic) checkAuthCredentials(req *bfe_basic.Request,
	rule *AuthBasicRule) bool {
	httpRequest := req.HttpRequest
	username, passwd, ok := httpRequest.BasicAuth()
	if !ok {
		m.state.ReqAuthChallenge.Inc(1)
		return false
	}
	if openDebug {
		log.Logger.Debug("%s check auth, username[%s], passwd[%s]", m.name, username, passwd)
	}

	hashedPasswd, ok := rule.UserPasswd[username]
	if !ok {
		if openDebug {
			log.Logger.Debug("%s check passwd, no username[%s]", m.name, username)
		}
		m.state.ReqAuthFailure.Inc(1)
		return false
	}

	if !auth.CheckSecret(passwd, hashedPasswd) {
		m.state.ReqAuthFailure.Inc(1)
		return false
	}

	m.state.ReqAuthSuccess.Inc(1)
	return true
}

func (m *ModuleAuthBasic) createUnauthorizedResp(req *bfe_basic.Request,
	rule *AuthBasicRule) *bfe_http.Response {
	resp := bfe_basic.CreateInternalResp(req, bfe_http.StatusUnauthorized)
	resp.Header.Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"%s\"", rule.Realm))
	return resp
}

func (m *ModuleAuthBasic) authBasicHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn, nil
	}

	for _, rule := range *rules {
		if rule.Cond.Match(req) {
			m.state.ReqAuthRuleHit.Inc(1)

			if !m.checkAuthCredentials(req, &rule) {
				return bfe_module.BfeHandlerResponse, m.createUnauthorizedResp(req, &rule)
			}
			return bfe_module.BfeHandlerGoOn, nil
		}
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleAuthBasic) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var cfg *ConfModAuthBasic

	confPath := bfe_module.ModConfPath(cr, m.name)
	if cfg, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err: %v", m.name, err)
	}

	m.configPath = cfg.Basic.DataPath
	openDebug = cfg.Log.OpenDebug

	if err = m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %v", err)
	}

	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.authBasicHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.authBasicHandler): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.monitorHandlers): %v", m.name, err)
	}

	err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %v", m.name, err)
	}

	return nil
}
