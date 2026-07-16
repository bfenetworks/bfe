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
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

var (
	openDebug = false

	ModSessionStickyKey = "mod_session_sticky_key"

	// StickyRedisKeyPrefix is the Redis key prefix for sticky mapping
	StickyRedisKeyPrefix = "bfe:stickyid:"
)

func (c CacheType) String() string {
	switch c {
	case CacheTypeLocal:
		return "local"
	case CacheTypeRedis:
		return "redis"
	default:
		return "unknown"
	}
}

// ParseCacheType converts string to CacheType
func ParseCacheType(s string) (CacheType, error) {
	switch s {
	case "local":
		return CacheTypeLocal, nil
	case "redis":
		return CacheTypeRedis, nil
	default:
		return CacheTypeLocal, fmt.Errorf("invalid cache type: %s", s)
	}
}

// CacheType defines the cache type for session sticky
type CacheType int

const (
	CacheTypeLocal CacheType = iota // local LRU cache
	CacheTypeRedis                  // Redis distributed cache
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

	// New fields for Redis support
	cacheType     CacheType // cache type: local or redis
	stickyCache   *lru_cache.LRUCache
	redisClient   redis_client.Client // Redis client (redis mode)
	expireSeconds int                 // cache expire time
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

// RedisSessionData is used for JSON serialization when storing in Redis
type RedisSessionData struct {
	Addr       string `json:"addr"`
	Port       int    `json:"port"`
	SubCluster string `json:"sub_cluster"`
	CreatedAt  int64  `json:"created_at"`
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
	cacheType, err := ParseCacheType(cfg.Basic.CacheType)
	if err != nil {
		return fmt.Errorf("invalid cache type: %s", err.Error())
	}
	m.cacheType = cacheType
	m.expireSeconds = cfg.Redis.ExpireSeconds

	// Initialize cache based on CacheType
	if m.cacheType == CacheTypeRedis {
		// Create Redis client
		r := cfg.Redis
		options := &redis_client.Options{
			ServiceConf:    r.Bns,
			MaxIdle:        r.MaxIdle,
			MaxActive:      r.MaxActive,
			Wait:           false,
			ConnTimeoutMs:  r.ConnectTimeout,
			ReadTimeoutMs:  r.ReadTimeout,
			WriteTimeoutMs: r.WriteTimeout,
			Password:       r.Password,
		}

		client := redis_client.NewRedisClient(options)
		m.redisClient = client
	} else {
		// Local LRU cache
		m.stickyCache = lru_cache.NewLRUCache(cfg.Basic.CacheSize)
	}

	// load from config file to rule table
	if err = m.loadConfData(nil); err != nil {
		return fmt.Errorf("err in loadConfData(): %s", err.Error())
	}

	// register handler
	err = cbs.AddFilter(bfe_module.HandleAfterLocation, m.decodeHandler)
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

	case RuleTypeSticky:
		var stickyid string
		// check stickyid in cookie
		if cookie, ok := req.Cookie(rule.CookieKey); ok {
			stickyid = cookie.Value
		}

		// check header if no stickyid in cookie
		if stickyid == "" && rule.Header != "" {
			h, ok := req.HttpRequest.Header[rule.Header]
			if ok && len(h) > 0 {
				stickyid = h[0]
			}
		}

		// check uriparam if no stickyid in cookie or header
		if stickyid == "" && rule.URIParam != "" {
			stickyid = req.CachedQuery().Get(rule.URIParam)
		}

		// check JSON request body field (for OpenAI responses interface)
		if stickyid == "" && rule.StickyRequestField != "" {
			stickyid = extractStickyIDFromRequestBody(req, rule.StickyRequestField)
		}

		if stickyid == "" {
			// no stickyid found in request
			return
		}

		// use unified cache interface
		var ok bool
		bk, ok = m.getBackendFromCache(stickyid)
		if !ok {
			return
		}
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

// extractStickyIDFromRequestBody extracts sticky ID from JSON request body
// Reuse condition.ReqBodyJsonFetch for consistent implementation
func extractStickyIDFromRequestBody(req *bfe_basic.Request, fieldName string) string {
	val, err := condition.ReqBodyJsonFetch(req, fieldName, nil)
	if err != nil || val == "" {
		return ""
	}
	return val
}

// extractStickyIDFromResponseBody extracts sticky ID from JSON response body
// Reuse condition.HttpRespBodyJsonGet for consistent implementation
func extractStickyIDFromResponseBody(res *bfe_http.Response, fieldName string) string {
	val, err := condition.HttpRespBodyJsonGet(res, fieldName)
	if err != nil || val == "" {
		return ""
	}
	return val
}

// getBackendFromCache retrieves backend info from cache
func (m *ModuleSessionSticky) getBackendFromCache(stickyid string) (*bfe_basic.SessionStickyBackend, bool) {
	if m.cacheType == CacheTypeRedis {
		return m.getBackendFromRedis(stickyid)
	}
	return m.getBackendFromLocal(stickyid)
}

// setBackendToCache stores backend info to cache
func (m *ModuleSessionSticky) setBackendToCache(stickyid string, bk *bfe_basic.SessionStickyBackend) error {
	if m.cacheType == CacheTypeRedis {
		return m.setBackendToRedis(stickyid, bk)
	}
	m.setBackendToLocal(stickyid, bk)
	return nil
}

// getBackendFromLocal retrieves backend info from local LRU cache
func (m *ModuleSessionSticky) getBackendFromLocal(stickyid string) (*bfe_basic.SessionStickyBackend, bool) {
	val, ok := m.stickyCache.Get(stickyid)
	if !ok {
		return nil, false
	}
	bk, ok := val.(*bfe_basic.SessionStickyBackend)
	return bk, ok
}

// setBackendToLocal stores backend info to local LRU cache
func (m *ModuleSessionSticky) setBackendToLocal(stickyid string, bk *bfe_basic.SessionStickyBackend) {
	m.stickyCache.Add(stickyid, bk)
}

// getBackendFromRedis retrieves backend info from Redis
func (m *ModuleSessionSticky) getBackendFromRedis(stickyid string) (*bfe_basic.SessionStickyBackend, bool) {
	key := StickyRedisKeyPrefix + stickyid

	val, err := m.redisClient.Get(key)
	if err != nil || val == nil {
		return nil, false
	}

	v, ok := val.([]byte)
	if !ok {
		return nil, false
	}

	var data RedisSessionData
	if err := json.Unmarshal(v, &data); err != nil {
		return nil, false
	}

	bk := &bfe_basic.SessionStickyBackend{
		Addr:       &data.Addr,
		Port:       &data.Port,
		SubCluster: &data.SubCluster,
	}
	return bk, true
}

// setBackendToRedis stores backend info to Redis
func (m *ModuleSessionSticky) setBackendToRedis(stickyid string, bk *bfe_basic.SessionStickyBackend) error {
	key := StickyRedisKeyPrefix + stickyid

	data := RedisSessionData{
		Addr:       *bk.Addr,
		Port:       *bk.Port,
		SubCluster: *bk.SubCluster,
		CreatedAt:  time.Now().Unix(),
	}

	val, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return m.redisClient.Setex(key, val, m.expireSeconds)
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

	var stickyid string
	if rule.Type == RuleTypeSticky {
		if cookie, err := res.Cookie(rule.CookieKey); err == nil {
			stickyid = cookie.Value
		}
		// If not found in cookie, try JSON response body (for OpenAI responses interface)
		if stickyid == "" && rule.StickyResponseField != "" {
			stickyid = extractStickyIDFromResponseBody(res, rule.StickyResponseField)
		}
		if stickyid == "" {
			// no stickyid in response, exit
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
	case RuleTypeSticky:
		// save backend for sticky_id using unified cache interface
		if err := m.setBackendToCache(stickyid, bk); err != nil {
			debuglogWithSwitch(fmt.Sprintf("setBackendToCache failed: %v", err))
		}
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
