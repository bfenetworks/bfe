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

package mod_session_sticky

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/lru_cache"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

var (
	openDebug = false

	ModSessionStickyKey = "mod_session_sticky_key"
)

type ModuleSessionState struct {
	Version *metrics.State
}

type ModuleSessionSticky struct {
	name    string // name of module
	state   ModuleSessionState
	metrics metrics.Metrics

	configPath string            // path of config file
	ruleTable  *ProductRuleTable // table of sticky rules

	jsessioncache *lru_cache.LRUCache
}

func NewModuleSessionSticky() *ModuleSessionSticky {
	m := new(ModuleSessionSticky)
	m.name = "mod_session_sticky"
	// init module state
	m.metrics.Init(&m.state, m.name, 20)
	m.ruleTable = NewProductRuleTable()
	return m
}

func (m *ModuleSessionSticky) Name() string {
	return m.name
}

// for monitor state
func (m *ModuleSessionSticky) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleSessionSticky) decodeHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
	rules, ok := m.ruleTable.Search(request.Route.Product)
	if !ok {
		rules, ok = m.ruleTable.Search(bfe_basic.GlobalProduct)
	}

	if !ok {
		return bfe_module.BfeHandlerGoOn, nil
	}
	rule := m.FindStickyRule(request, rules)
	if rule == nil {
		return bfe_module.BfeHandlerGoOn, nil
	}

	debuglogWithSwitch(fmt.Sprintf("%s:decodeHandler:before:host=%s, path=%s, query=%s, rule=%v", m.name,
		request.HttpRequest.Host, request.HttpRequest.URL.Path,
		request.HttpRequest.URL.RawQuery, rule))

	// do decode to request, according to rules
	m.processDecode(request, *rule)

	debuglogWithSwitch(fmt.Sprintf("%s:decodeHandler:after:host=%s, path=%s, query=%s, rule=%v", m.name,
		request.HttpRequest.Host, request.HttpRequest.URL.Path,
		request.HttpRequest.URL.RawQuery, rule))
	return bfe_module.BfeHandlerGoOn, nil

}

// OBSOLETE!
// find and cache sticky rule which can match request;
func (m *ModuleSessionSticky) FindAndCacheStickyRule(request *bfe_basic.Request, rules *StickyRuleList) *StickyRule {
	if request == nil {
		return nil
	}
	if val, ok := request.Context[ModSessionStickyKey]; ok {
		sticky, ok := val.(StickyRule)
		if ok {
			return &sticky
		}
	}
	if rules == nil {
		return nil
	}
	for _, rule := range *rules {
		if rule.Cond.Match(request) {
			request.Context[ModSessionStickyKey] = rule
			return &rule
		}
	}
	return nil
}

type SessionStickyData struct {
	bk   *bfe_basic.SessionStickyBackend
	rule *StickyRule
}

// find sticky rule which can match request;
func (m *ModuleSessionSticky) FindStickyRule(request *bfe_basic.Request, rules *StickyRuleList) *StickyRule {
	if request == nil {
		return nil
	}
	if rules == nil {
		return nil
	}
	for _, rule := range *rules {
		if rule.Cond.Match(request) {
			return &rule
		}
	}
	return nil
}

func (m *ModuleSessionSticky) encodeHandler(request *bfe_basic.Request, res *bfe_http.Response) int {

	// in order to make sure the consistency of session sticky's config between decode and encode,
	// we try to get rule from context of request.
	val, ok := request.Context[ModSessionStickyKey]
	if !ok {
		return bfe_module.BfeHandlerGoOn
	}

	sessiondata, ok := val.(SessionStickyData)
	if !ok {
		return bfe_module.BfeHandlerGoOn
	}

	debuglogWithSwitch(fmt.Sprintf("%s:encodeHandler:before:host=%s, path=%s, query=%s, rule=%v", m.name,
		request.HttpRequest.Host, request.HttpRequest.URL.Path,
		request.HttpRequest.URL.RawQuery, sessiondata.rule))

	// do encode handler to request, according to rules
	m.processEncode(request, res, sessiondata)

	debuglogWithSwitch(fmt.Sprintf("%s:encodeHandler:after:host=%s, path=%s, query=%s, rule=%v", m.name,
		request.HttpRequest.Host, request.HttpRequest.URL.Path,
		request.HttpRequest.URL.RawQuery, sessiondata.rule))
	return bfe_module.BfeHandlerGoOn
}

func (m *ModuleSessionSticky) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error
	var conf *ConfModSessionSticky

	confPath := bfe_module.ModConfPath(cr, m.name)
	if conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}

	openDebug = conf.Log.OpenDebug

	return m.init(conf, cbs, whs)
}
func (m *ModuleSessionSticky) loadConfData(query url.Values) error {
	// get file path
	path := query.Get("path")
	if path == "" {
		// use default
		path = m.configPath
	}

	// load from config file
	conf, err := ProductRuleConfLoad(path)
	if err != nil {
		return fmt.Errorf("err in ProductRuleConfLoad(%s):%s", path, err.Error())
	}

	// update to rule table
	m.ruleTable.Update(conf)

	// set module version
	m.state.Version.Set(conf.Version)

	return nil
}

func (m *ModuleSessionSticky) init(cfg *ConfModSessionSticky, cbs *bfe_module.BfeCallbacks,
	whs *web_monitor.WebHandlers) error {

	openDebug = cfg.Log.OpenDebug

	m.configPath = cfg.Basic.DataPath
	m.jsessioncache = lru_cache.NewLRUCache(cfg.Basic.CacheSize)

	// load from config file to rule table
	if err := m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %s", err.Error())
	}

	// register handler
	err := cbs.AddFilter(bfe_module.HandleAfterLocation, m.decodeHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.decodeHandler): %s", m.name, err.Error())
	}

	// register handler
	err = cbs.AddFilter(bfe_module.HandleReadResponse, m.encodeHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.encodeHandler): %s", m.name, err.Error())
	}

	// register web handler
	err = whs.RegisterHandler(web_monitor.WebHandleMonitor, m.name, m.getState)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.getState): %s", m.name, err.Error())
	}

	err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandler(m.loadConfData): %s", m.name, err.Error())
	}
	return nil
}

// decode session sticky info in http request with given rules.
func (m *ModuleSessionSticky) processDecode(req *bfe_basic.Request, rule StickyRule) {
	var bk *bfe_basic.SessionStickyBackend

	defer func() {
		// save sticky data to request context
		req.Context[ModSessionStickyKey] = SessionStickyData{bk: bk, rule: &rule}
	}()

	switch rule.Type {
	case RuleTypeCookie:
		// cause check cookie is quic than check cond, so
		// we check cookie first
		cookie, ok := req.Cookie(rule.CookieKey)
		if !ok {
			return
		}

		val, err := doDecode(cookie.Value, []byte(rule.MaskCode))
		if err != nil {
			debuglogWithSwitch(fmt.Sprintf("processDecode() try maskcode fail: cookie [%s] maskcode[%s] err[%s]", val, rule.MaskCode, err))
			// try decode cookie by stand by maskcode
			val, err = doDecode(cookie.Value, []byte(rule.StandbyMaskCode))
			if err != nil {
				debuglogWithSwitch(fmt.Sprintf("processDecode() try standby maskcode fail: cookie [%s] standbyMaskcode[%s] err[%s]",
					val, rule.StandbyMaskCode, err))
				return
			}
		}

		bk, err = getStickyBackend(val)
		if err != nil {
			debuglogWithSwitch(fmt.Sprintf("processDecode(): cookie [%s] sticky bk[%v] err[%s]", val, bk, err))
			return
		}

	case RuleTypeJsession:
		var jsessionid string
		// check jsessionid in cookie
		if cookie, ok := req.Cookie(rule.CookieKey); ok {
			jsessionid = cookie.Value
		}

		// check header if no jsessionid in cookie
		if jsessionid == "" && rule.Header != "" {
			h, ok := req.HttpRequest.Header[rule.Header]
			if ok && len(h) > 0 {
				jsessionid = h[0]
			}
		}

		// check uriparam if no jsession in cookie or header
		if jsessionid == "" && rule.URIParam != "" {
			jsessionid = req.CachedQuery().Get(rule.URIParam)
		}

		if jsessionid == "" {
			// no jsessionid found in request
			return
		}

		val, ok := m.jsessioncache.Get(jsessionid)
		if !ok {
			return
		}
		bk = val.(*bfe_basic.SessionStickyBackend)
	}

	req.Context[bfe_basic.SessionStickyBackendKey] = bk
}

func getStickyBackend(code string) (*bfe_basic.SessionStickyBackend, error) {
	var bk bfe_basic.SessionStickyBackend
	err := json.Unmarshal([]byte(code), &bk)
	if err != nil {
		return nil, err
	}
	if bk.Addr == nil || bk.Port == nil || bk.SubCluster == nil {
		return nil, fmt.Errorf("getStickyBackend(): bk[%v] has empty field", bk)
	}
	return &bk, err
}

// encode session sticky info in  http request with given rules.
func (m *ModuleSessionSticky) processEncode(req *bfe_basic.Request, res *bfe_http.Response, sessiondata SessionStickyData) {
	// just in case the backend is nil while blackhole subcluser or backend unavailable
	if req.Trans.Backend == nil {
		debuglogWithSwitch(fmt.Sprintf("processEncode(): backend nil"))
		return
	}

	rule := sessiondata.rule
	sesbk := sessiondata.bk

	var jsessionid string
	if rule.Type == RuleTypeJsession {
		if cookie, err := res.Cookie(rule.CookieKey); err == nil {
			jsessionid = cookie.Value
		}
		if jsessionid == "" {
			// no Jsessionid in response, exit
			return
		}
	}

	bk := &bfe_basic.SessionStickyBackend{
		Addr:       &req.Trans.Backend.Addr,
		Port:       &req.Trans.Backend.Port,
		SubCluster: &req.Backend.SubclusterName,
	}

	switch rule.Type {
	case RuleTypeCookie:
		// in the following cases, we should set cookie:
		// 1. no session sticky info in request
		// 2. backend changed
		// 3. need renew
		needSetCookie := false
		if sesbk == nil {
			needSetCookie = true
			debuglogWithSwitch("processEncode(): need set cookie because sessiondata.bk is nil")
		} else if *sesbk.Addr != *bk.Addr || *sesbk.Port != *bk.Port ||
			*sesbk.SubCluster != *bk.SubCluster {
			needSetCookie = true
			debuglogWithSwitch("processEncode(): need set cookie because backend changed")
		} else if sesbk.RenewTime != nil {
			now := time.Now().Unix()
			if now >= *(sesbk.RenewTime) {
				needSetCookie = true
				debuglogWithSwitch("processEncode(): need set cookie because need renew")
			}
		}

		if !needSetCookie {
			return
		}

		if rule.RenewWindow > 0 && rule.MaxAge > 0 {
			renew := time.Now().Unix() + int64(rule.MaxAge) - int64(rule.RenewWindow)
			if renew > 0 {
				bk.RenewTime = &renew
			}
		}
		bkStr, err := getStickyBackendStr(bk)
		if err != nil {
			debuglogWithSwitch(fmt.Sprintf("processEncode(): get sticky backend info[%v] err[%s]", *bk, err))
			return
		}

		// just encode by active maskcode
		val := doEncode(bkStr, []byte(rule.MaskCode))
		cookie := bfe_http.Cookie{
			Name:     rule.CookieKey,
			Value:    val,
			MaxAge:   rule.MaxAge,
			HttpOnly: rule.HttpOnly,
			Secure:   rule.Secure,
		}
		if rule.Domain != "" {
			cookie.Domain = rule.Domain
		}
		if rule.Path != "" {
			cookie.Path = rule.Path
		}

		setCookie(&cookie, res)
		debuglogWithSwitch(fmt.Sprintf("EncodeHandler(): encode cookie [%v]", cookie))
	case RuleTypeJsession:
		// save backend for Jessionid
		m.jsessioncache.Add(jsessionid, bk)
	}
}

func setCookie(cookie *bfe_http.Cookie, res *bfe_http.Response) {
	if v := cookie.String(); v != "" {
		res.Header.Add("Set-Cookie", v)
	}
}

func getStickyBackendStr(bk *bfe_basic.SessionStickyBackend) (string, error) {
	val, err := json.Marshal(bk)
	return string(val), err
}

func doMask(maskCode []byte, val string) string {
	valBytes := []byte(val)
	data := make([]byte, len(val))

	for i := range data {
		data[i] = valBytes[i] ^ maskCode[i%len(maskCode)]
	}
	return string(data)
}

func doEncode(src string, maskCode []byte) string {
	str := doMask(maskCode, src)
	str = base64.RawStdEncoding.EncodeToString([]byte(str))
	return str
}

func doDecode(src string, maskCode []byte) (string, error) {
	str, err := base64.RawStdEncoding.DecodeString(src)
	if err != nil {
		return "", err
	}
	val := doMask(maskCode, string(str))
	return val, err
}

func debuglogWithSwitch(str string) {
	if openDebug {
		log.Logger.Debug(str)
	}
}
