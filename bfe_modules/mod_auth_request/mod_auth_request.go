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

package mod_auth_request

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/delay_counter"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	ModAuthRequest         = "mod_auth_request"
	DiffInterval           = 20 // interval for diff counter (in seconds)
	DelayConterInterval    = 60 // interval for moving current to past (in s)
	DelayCounterBucketSize = 1  // size of delay counter bucket (in ms)
	DelayCounterBucketNum  = 20 // number of delay counter bucket

	XForwardedMethod = "X-Forwarded-Method"
	XForwardedURI    = "X-Forwarded-Uri"
)

var (
	openDebug = false

	ErrAuthRequest = errors.New("AUTH_REQ_FORBIDDEN")
)

type ModuleAuthRequestState struct {
	AuthRequestChecked      *metrics.Counter
	AuthRequestPass         *metrics.Counter
	AuthRequestForbidden    *metrics.Counter
	AuthRequestUnauthorized *metrics.Counter
	AuthRequestFail         *metrics.Counter
	AuthRequestUncertain    *metrics.Counter
}

type ModuleAuthRequest struct {
	name      string
	conf      *ConfModAuthRequest
	ruleTable *AuthRequestRuleTable

	authClient http.Client // auth client, use default roundtrip

	state   ModuleAuthRequestState // module state
	metrics metrics.Metrics

	delay *delay_counter.DelayRecent // delay distribution for auth service
}

func NewModuleAuthRequest() *ModuleAuthRequest {
	m := new(ModuleAuthRequest)
	m.name = ModAuthRequest
	m.ruleTable = NewAuthRequestRuleTable()

	m.metrics.Init(&m.state, ModAuthRequest, DiffInterval)

	m.delay = new(delay_counter.DelayRecent)
	m.delay.Init(DelayConterInterval, DelayCounterBucketSize, DelayCounterBucketNum)
	m.delay.SetKeyPrefix(ModAuthRequest)

	return m
}

func (m *ModuleAuthRequest) Name() string {
	return m.name
}

func (m *ModuleAuthRequest) loadRuleData(query url.Values) (string, error) {
	// get file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.conf.Basic.DataPath
	}

	// load from config file
	conf, err := AuthRequestRuleFileLoad(path)
	if err != nil {
		return "", fmt.Errorf("%s: AuthRequestRuleFileLoad(%s) error: %v", m.name, path, err)
	}

	// update to rule table
	m.ruleTable.Update(conf)

	_, fileName := filepath.Split(path)
	return fmt.Sprintf("%s=%s", fileName, conf.Version), nil
}

func removeHopHeaders(headers http.Header) {
	for _, h := range bfe_basic.HopHeaders {
		headers.Del(h)
	}
}

func (m *ModuleAuthRequest) createAuthRequest(originReq *bfe_basic.Request) *http.Request {
	authReq, _ := http.NewRequest(http.MethodGet, m.conf.Basic.AuthAddress, nil)

	// copy header from origin request header
	bfe_http.CopyHeader(bfe_http.Header(authReq.Header), originReq.HttpRequest.Header)

	// remove hop headers
	removeHopHeaders(authReq.Header)

	// remove Content-Length header
	authReq.Header.Del("Content-Length")

	xMethod := originReq.HttpRequest.Header.Get(XForwardedMethod)
	if xMethod != "" {
		authReq.Header.Set(XForwardedMethod, xMethod)
	} else {
		authReq.Header.Set(XForwardedMethod, originReq.HttpRequest.Method)
	}

	xUri := originReq.HttpRequest.Header.Get(XForwardedURI)
	if xUri != "" {
		authReq.Header.Set(XForwardedURI, xUri)
	} else {
		authReq.Header.Set(XForwardedURI, originReq.HttpRequest.URL.RequestURI())
	}

	if openDebug {
		log.Logger.Info("%s: auth request header: [%v]", m.name, authReq.Header)
	}

	return authReq
}

func (m *ModuleAuthRequest) callAuthService(forwardReq *http.Request) (*http.Response, error) {
	startTime := time.Now()
	defer m.delay.AddBySub(startTime, time.Now())

	resp, err := m.authClient.Do(forwardReq)
	if err != nil {
		log.Logger.Info("%s: auth request failed: %v", m.name, err)
		return resp, err
	}

	return resp, nil
}

func (m *ModuleAuthRequest) genAuthForbiddenResp(req *bfe_basic.Request, resp *http.Response) *bfe_http.Response {
	forbiddenResp := bfe_basic.CreateInternalResp(req, resp.StatusCode)
	if resp.StatusCode == bfe_http.StatusUnauthorized {
		if wwwAuth := resp.Header.Get("WWW-Authenticate"); len(wwwAuth) > 0 {
			forbiddenResp.Header.Set("WWW-Authenticate", wwwAuth)
		}
		m.state.AuthRequestUnauthorized.Inc(1)
		return forbiddenResp
	}

	if resp.StatusCode == bfe_http.StatusForbidden {
		m.state.AuthRequestForbidden.Inc(1)
		return forbiddenResp
	}

	// if the service response code is 2XX, the access is allowed.
	if resp.StatusCode/100 == 2 {
		m.state.AuthRequestPass.Inc(1)
		return nil
	}

	// if any other response code returned is considered an error
	m.state.AuthRequestUncertain.Inc(1)
	if openDebug {
		log.Logger.Info("%s: auth response is not expected, resp code[%d]", m.name, resp.StatusCode)
	}
	return nil
}

// forward request to auth server
func (m *ModuleAuthRequest) forwardAuthServer(req *bfe_basic.Request) *bfe_http.Response {
	// create auth request
	authReq := m.createAuthRequest(req)

	// call auth service
	resp, err := m.callAuthService(authReq)
	if err != nil {
		m.state.AuthRequestFail.Inc(1)
		return nil
	}
	defer resp.Body.Close()

	return m.genAuthForbiddenResp(req, resp)
}

func (m *ModuleAuthRequest) authRequestHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		return bfe_module.BfeHandlerGoOn, nil
	}

	for _, rule := range rules {
		if rule.Enable && rule.Cond.Match(req) {
			m.state.AuthRequestChecked.Inc(1)

			// check request is denied
			if resp := m.forwardAuthServer(req); resp != nil {
				req.ErrCode = ErrAuthRequest
				return bfe_module.BfeHandlerResponse, resp
			}
		}
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleAuthRequest) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadRuleData,
	}
	return handlers
}

func (m *ModuleAuthRequest) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleAuthRequest) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleAuthRequest) getDelay(query url.Values) ([]byte, error) {
	delay := m.delay
	return delay.FormatOutput(query)
}

// all monitor handlers
func (m *ModuleAuthRequest) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:            m.getState,
		m.name + ".diff":  m.getStateDiff,
		m.name + ".delay": m.getDelay,
	}
	return handlers
}

func (m *ModuleAuthRequest) init(conf *ConfModAuthRequest, cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers) error {
	var err error

	_, err = m.loadRuleData(nil)
	if err != nil {
		return err
	}

	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.authRequestHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.authRequestHandler): %v", m.name, err)
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

func (m *ModuleAuthRequest) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	var err error
	var conf *ConfModAuthRequest

	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}

	m.conf = conf
	openDebug = conf.Log.OpenDebug

	m.authClient = http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			// disable redirect
			return http.ErrUseLastResponse
		},
		Timeout: time.Duration(m.conf.Basic.AuthTimeout) * time.Millisecond,
	}
	return m.init(conf, cbs, whs)
}
