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

package mod_auth_jwt

import (
	"fmt"
	"net/url"
	"strings"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/golang-jwt/jwt"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	ModAuthJWT = "mod_auth_jwt"
)

type ModuleAuthJWTState struct {
	ReqAuthRuleHit                *metrics.Counter
	ReqAuthNoAuthorization        *metrics.Counter
	ReqAuthAuthorizationFormatErr *metrics.Counter
	ReqAuthSuccess                *metrics.Counter
	ReqAuthFailure                *metrics.Counter
}

type ModuleAuthJWT struct {
	name       string
	state      ModuleAuthJWTState
	metrics    metrics.Metrics
	configPath string
	ruleTable  *AuthJWTRuleTable
}

var (
	openDebug = false
)

func NewModuleAuthJWT() *ModuleAuthJWT {
	m := new(ModuleAuthJWT)
	m.name = ModAuthJWT
	m.metrics.Init(&m.state, ModAuthJWT, 0)
	m.ruleTable = NewAuthJWTRuleTable()
	return m
}

func (m *ModuleAuthJWT) Name() string {
	return m.name
}

func (m *ModuleAuthJWT) loadConfData(query url.Values) error {
	path := query.Get("path")
	if path == "" {
		path = m.configPath
	}

	conf, err := AuthJWTConfLoad(path)
	if err != nil {
		return fmt.Errorf("error in AuthJWTConfLoad(%s): %v", path, err)
	}

	m.ruleTable.Update(conf)
	return nil
}

func (m *ModuleAuthJWT) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleAuthJWT) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleAuthJWT) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModuleAuthJWT) getToken(req *bfe_basic.Request) (string, error) {
	authHeader := req.HttpRequest.Header.Get("Authorization")
	if authHeader == "" {
		m.state.ReqAuthNoAuthorization.Inc(1)
		return "", fmt.Errorf("No Authorization header.")
	}

	authValue := strings.Split(authHeader, " ")
	if len(authValue) != 2 {
		m.state.ReqAuthAuthorizationFormatErr.Inc(1)
		return "", fmt.Errorf("Authorization header format error.")
	}

	if authValue[0] != "Bearer" {
		m.state.ReqAuthAuthorizationFormatErr.Inc(1)
		return "", fmt.Errorf("Authorization type[%s] error.", authValue[0])
	}

	return authValue[1], nil
}

func (m *ModuleAuthJWT) validateToken(token string, rule *AuthJWTRule) error {
	for _, key := range rule.Keys {
		parsedToken, err := jwt.Parse(token, key.provideKey)
		if err != nil {
			if openDebug {
				log.Logger.Debug("%s: parse token error: %v, kid: %s", m.name, err, key.key.KeyID)
			}
			continue
		}

		// Both signature and time based claims "exp, iat, nbf" are valid.
		if parsedToken.Valid && parsedToken.Claims.Valid() == nil {
			return nil
		}
	}

	return fmt.Errorf("token[%s] invalid", token)
}

func (m *ModuleAuthJWT) checkAuthCredentials(req *bfe_basic.Request, rule *AuthJWTRule) error {
	token, err := m.getToken(req)
	if err != nil {
		return err
	}

	return m.validateToken(token, rule)
}

func (m *ModuleAuthJWT) createUnauthorizedResp(req *bfe_basic.Request,
	rule *AuthJWTRule) *bfe_http.Response {
	resp := bfe_basic.CreateInternalResp(req, bfe_http.StatusUnauthorized)
	resp.Header.Set("WWW-Authenticate", fmt.Sprintf("Bearer realm=\"%s\"", rule.Realm))
	return resp
}

func (m *ModuleAuthJWT) authJWTHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn, nil
	}

	for _, rule := range *rules {
		if rule.Cond.Match(req) {
			m.state.ReqAuthRuleHit.Inc(1)

			err := m.checkAuthCredentials(req, &rule)
			if err != nil {
				if openDebug {
					log.Logger.Debug("%s: check auth jwt error: %v", m.name, err)
				}

				m.state.ReqAuthFailure.Inc(1)
				return bfe_module.BfeHandlerResponse, m.createUnauthorizedResp(req, &rule)
			}

			m.state.ReqAuthSuccess.Inc(1)
			return bfe_module.BfeHandlerGoOn, nil
		}
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleAuthJWT) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var cfg *ConfModAuthJWT

	confPath := bfe_module.ModConfPath(cr, m.name)
	if cfg, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err: %v", m.name, err)
	}

	m.configPath = cfg.Basic.DataPath
	openDebug = cfg.Log.OpenDebug

	if err = m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %v", err)
	}

	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.authJWTHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.authJWTHandler): %v", m.name, err)
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
