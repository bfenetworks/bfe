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

package mod_cors

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
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
	ModCors                             = "mod_cors"
	HeaderAccessControlAllowOrigin      = "Access-Control-Allow-Origin"
	HeaderAccessControlAllowCredentials = "Access-Control-Allow-Credentials"
	HeaderAccessControlAllowMethods     = "Access-Control-Allow-Methods"
	HeaderAccessControlAllowHeaders     = "Access-Control-Allow-Headers"
	HeaderAccessControlMaxAge           = "Access-Control-Max-Age"
	HeaderAccessControlExposeHeaders    = "Access-Control-Expose-Headers"
	HeaderAccessControlRequestMethod    = "Access-Control-Request-Method"
	HeaderOrigin                        = "Origin"
	HeaderVary                          = "Vary"
)

type ModuleCorsState struct {
	ReqCorsRuleHit       *metrics.Counter
	ReqPreFlightHit      *metrics.Counter
	ReqAllowOriginHit    *metrics.Counter
	ReqNotAllowOriginHit *metrics.Counter
}

var (
	openDebug = false
)

type ModuleCors struct {
	name      string
	conf      *ConfModCors
	ruleTable *CorsRuleTable
	state     ModuleCorsState
	metrics   metrics.Metrics
}

func NewModuleCors() *ModuleCors {
	m := new(ModuleCors)
	m.name = ModCors
	m.ruleTable = NewCorsRuleTable()
	m.metrics.Init(&m.state, ModCors, 0)
	return m
}

func (m *ModuleCors) Name() string {
	return m.name
}

func (m *ModuleCors) loadRuleData(query url.Values) (string, error) {
	// get file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.conf.Basic.DataPath
	}

	// load from config file
	conf, err := CorsRuleFileLoad(path)
	if err != nil {
		return "", fmt.Errorf("%s: CorsRuleFileLoad(%s) error: %v", m.name, path, err)
	}

	// update to rule table
	m.ruleTable.Update(conf)

	_, fileName := filepath.Split(path)
	return fmt.Sprintf("%s=%s", fileName, conf.Version), nil
}

// Add `Origin` in the Vary response header, to indicate to clients that server responses will differ based on the value
// of the Origin request header
func addVaryHeader(rspHeader bfe_http.Header) {
	varyValue := rspHeader.Get(HeaderVary)
	if len(varyValue) == 0 {
		rspHeader.Set(HeaderVary, HeaderOrigin)
		return
	}

	if varyValue == "*" {
		return
	}

	needAddOrigin := true
	items := strings.Split(varyValue, ",")
	for _, item := range items {
		if strings.TrimSpace(item) == HeaderOrigin {
			needAddOrigin = false
			break
		}
	}

	if needAddOrigin {
		varyValue += fmt.Sprintf(",%s", HeaderOrigin)
	}
}

// set response header for preflight request
func (m *ModuleCors) setRespHeaderForPreflght(request *bfe_basic.Request, rspHeader bfe_http.Header, rule *CorsRule) {
	origin := request.HttpRequest.Header.Get(HeaderOrigin)
	allow, matchedOrigin := matchOriginAllowed(origin, rule)
	if !allow {
		m.state.ReqNotAllowOriginHit.Inc(1)
		return
	}
	m.state.ReqAllowOriginHit.Inc(1)

	rspHeader.Set(HeaderAccessControlAllowOrigin, matchedOrigin)

	if rule.AccessControlAllowCredentials {
		rspHeader.Set(HeaderAccessControlAllowCredentials, "true")
	}

	if len(rule.AccessControlAllowMethods) > 0 {
		rspHeader.Set(HeaderAccessControlAllowMethods, strings.Join(rule.AccessControlAllowMethods, ","))
	}

	if len(rule.AccessControlAllowHeaders) > 0 {
		rspHeader.Set(HeaderAccessControlAllowHeaders, strings.Join(rule.AccessControlAllowHeaders, ","))
	}

	if rule.AccessControlMaxAge != nil {
		rspHeader.Set(HeaderAccessControlMaxAge, strconv.Itoa(*rule.AccessControlMaxAge))
	}

	addVaryHeader(rspHeader)
}

// set response header for non-preflight request
func (m *ModuleCors) setRespHeaderForNonPreflight(request *bfe_basic.Request, rspHeader bfe_http.Header, rule *CorsRule) {
	origin := request.HttpRequest.Header.Get(HeaderOrigin)
	allow, matchedOrigin := matchOriginAllowed(origin, rule)
	if !allow {
		m.state.ReqNotAllowOriginHit.Inc(1)
		return
	}
	m.state.ReqAllowOriginHit.Inc(1)

	rspHeader.Set(HeaderAccessControlAllowOrigin, matchedOrigin)

	if rule.AccessControlAllowCredentials {
		rspHeader.Set(HeaderAccessControlAllowCredentials, "true")
	}

	if len(rule.AccessControlExposeHeaders) > 0 {
		rspHeader.Set(HeaderAccessControlExposeHeaders, strings.Join(rule.AccessControlExposeHeaders, ","))
	}

	addVaryHeader(rspHeader)
}

func (m *ModuleCors) corsHandler(request *bfe_basic.Request, response *bfe_http.Response) int {
	// cors request must carry header "origin"
	if request.HttpRequest.Header.Get(HeaderOrigin) == "" {
		return bfe_module.BfeHandlerGoOn
	}

	// preflight request has processed by corsPreflightHandler, no need to deal it
	if checkCorsPreflight(request) {
		return bfe_module.BfeHandlerGoOn
	}

	rules, ok := m.ruleTable.Search(request.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn
	}

	for index, rule := range rules {
		if rule.Cond.Match(request) {
			if openDebug {
				log.Logger.Info("%s hit product[%s] cors rule[%d]",
					request.HttpRequest.Host+request.HttpRequest.URL.RequestURI(), request.Route.Product, index)
			}

			m.state.ReqCorsRuleHit.Inc(1)

			// set cors header for response
			m.setRespHeaderForNonPreflight(request, response.Header, &rule)
			break
		}
	}

	return bfe_module.BfeHandlerGoOn
}

func checkCorsPreflight(request *bfe_basic.Request) bool {
	if request.HttpRequest.Method != http.MethodOptions {
		return false
	}

	if request.HttpRequest.Header.Get(HeaderOrigin) == "" {
		return false
	}

	if _, ok := supportedMethod[request.HttpRequest.Header.Get(HeaderAccessControlRequestMethod)]; !ok {
		return false
	}

	return true
}

func matchOriginAllowed(origin string, rule *CorsRule) (bool, string) {
	if _, ok := rule.AccessControlAllowOriginMap["%origin"]; ok {
		return true, origin
	}

	if _, ok := rule.AccessControlAllowOriginMap["*"]; ok {
		return true, "*"
	}

	if _, ok := rule.AccessControlAllowOriginMap[origin]; ok {
		return true, origin
	}

	return false, ""
}

func (m *ModuleCors) createCorsPreflightResponse(request *bfe_basic.Request, rule *CorsRule) *bfe_http.Response {
	m.state.ReqPreFlightHit.Inc(1)

	resp := bfe_basic.CreateInternalResp(request, bfe_http.StatusNoContent)

	m.setRespHeaderForPreflght(request, resp.Header, rule)

	return resp
}

func (m *ModuleCors) corsPreflightHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	if !checkCorsPreflight(request) {
		return bfe_module.BfeHandlerGoOn, nil
	}

	rules, ok := m.ruleTable.Search(request.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn, nil
	}

	for index, rule := range rules {
		if rule.Cond.Match(request) {
			if openDebug {
				log.Logger.Info("%s hit product[%s] cors rule[%d]",
					request.HttpRequest.Host+request.HttpRequest.URL.String(), request.Route.Product, index)
			}

			m.state.ReqCorsRuleHit.Inc(1)

			resp := m.createCorsPreflightResponse(request, &rule)

			return bfe_module.BfeHandlerResponse, resp
		}
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleCors) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleCors) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleCors) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModuleCors) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadRuleData,
	}
	return handlers
}

func (m *ModuleCors) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	var err error
	var conf *ConfModCors

	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}

	m.conf = conf
	openDebug = conf.Log.OpenDebug

	_, err = m.loadRuleData(nil)
	if err != nil {
		return err
	}

	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.corsPreflightHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.corsPreflightHandler): %v", m.name, err)
	}

	err = cbs.AddFilter(bfe_module.HandleReadResponse, m.corsHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.corsHandler): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.reloadHandlers): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.reloadHandlers): %v", m.name, err)
	}

	return nil
}
