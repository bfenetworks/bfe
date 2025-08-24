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

package mod_ai_token_auth

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/metrics"
	"github.com/baidu/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

const (
	ModAITokenAuth = "mod_ai_token_auth"
)

var (
	openDebug = false
)

type ModuleAITokenAuthState struct {
	ReqTotal           *metrics.Counter
	ReqAuth            *metrics.Counter
	ReqAuthFail        *metrics.Counter
}

type ModuleAITokenAuth struct {
	name      string
	conf      *ConfModAITokenAuth
	ruleTable *TokenRuleTable
	state     ModuleAITokenAuthState
	metrics   metrics.Metrics

	redisClient redis_client.Client // redis client
}

func NewModuleAITokenAuth() *ModuleAITokenAuth {
	m := new(ModuleAITokenAuth)
	m.name = ModAITokenAuth
	m.metrics.Init(&m.state, ModAITokenAuth, 0)
	m.ruleTable = NewTokenRuleTable()
	return m
}

func (m *ModuleAITokenAuth) Name() string {
	return m.name
}

func (m *ModuleAITokenAuth) loadProductRuleConf(query url.Values) error {
	path := query.Get("path")
	if path == "" {
		path = m.conf.Basic.ProductRulePath
	}

	conf, err := ProductRuleConfLoad(path)
	if err != nil {
		return fmt.Errorf("err in ProductRuleConfLoad(%s): %s", path, err)
	}

	oldtokens := m.ruleTable.Update(conf)
	// clean old tokens' used quota in redis
	for _, t := range oldtokens {
		key := usedQuotaKey(t.Key, t.UpdateTime)
		m.redisClient.Expire(key, 3600)
	}

	return nil
}

func (m *ModuleAITokenAuth) matchTokenRule(req *bfe_basic.Request) bool {
	if openDebug {
		log.Logger.Debug("%s check request", m.name)
	}
	m.state.ReqTotal.Inc(1)

	rules, ok := m.ruleTable.Search(req.Route.Product)
	if !ok {
		if openDebug {
			log.Logger.Debug("%s product %s not found, just pass", m.name, req.Route.Product)
		}
		return false
	}

	for _, rule := range *rules {
		if openDebug {
			log.Logger.Debug("%s process rule: %v", m.name, rule)
		}

		if rule.Cond.Match(req) {
			return true
		}
	}

	return false
}

func (m *ModuleAITokenAuth) tokenReadResponseHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	ctx := GetTokenAuthContext(req) // ensure token auth context is set
	if ctx == nil {
		// log.Logger.Warn("%s: token auth context not set", m.name)
		return bfe_module.BfeHandlerGoOn
	}

	if res.ContentLength >= 0 {
		ctx.CompletionTokens = int64(res.ContentLength) / 4 // estimate completion tokens
		// ctx.UsedQuota = CalcReqUsedQuota(req, ctx.PromptTokens, ctx.CompletionTokens) // calculate used quota
	}
	return bfe_module.BfeHandlerGoOn
}

func CalcReqUsedQuota(req *bfe_basic.Request, promptTokens, completionTokens int64) int64 {
	// calculate used quota based on prompt and completion tokens
	if promptTokens < 0 || completionTokens < 0 {
		return 0
	}
	return promptTokens + completionTokens
}

func (m *ModuleAITokenAuth) tokenRequestFinishHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	ctx := GetTokenAuthContext(req) // ensure token auth context is set
	if ctx == nil {
		return bfe_module.BfeHandlerGoOn
	}

	ctx.UsedQuota = CalcReqUsedQuota(req, ctx.PromptTokens, ctx.CompletionTokens) // calculate used quota
	if ctx.UsedQuota > 0 {
		m.IncrTokenUsedQuotaBy(ctx.Token, ctx.UsedQuota) // increment token used quota
	}

	return bfe_module.BfeHandlerGoOn
}

func SetApiKey(req *bfe_http.Request, apiKey string) {
	// set api key to Authorization header
	if apiKey == "" {
		return
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
}

func GetApiKey(req *bfe_basic.Request) string {
	// get api key from Authorization header
	authHeader := req.HttpRequest.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	// remove "Bearer " prefix if exists
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	authHeader = strings.TrimPrefix(authHeader, "sk-")

	// split by "-" and return the first part as api key
	parts := strings.Split(authHeader, "-")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// found product handler
func (m *ModuleAITokenAuth) tokenFoundProductHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	matched := m.matchTokenRule(req)
	if !matched {
		// no rule, just pass
		return bfe_module.BfeHandlerGoOn, nil
	}

	// do token authentication
	m.state.ReqAuth.Inc(1)
	tok, err := m.ValidateUserTokenByReq(req)
	if err != nil {
		m.state.ReqAuthFail.Inc(1)
		resp := bfe_basic.CreateSpecifiedContentResp(req, bfe_http.StatusUnauthorized, "text/plain",
			fmt.Sprintf("token authentication failed: %s", err.Error()))
		return bfe_module.BfeHandlerResponse, resp
	}

	promptToken := GetPromptToken(req)
	SetTokenAuthContext(req, &TokenAuthContext{
		Token: tok,
		PromptTokens: promptToken,
		CompletionTokens: -1, // -1 - unknown
	})

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleAITokenAuth) getState(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetAll()
	return s.Format(params)
}

func (m *ModuleAITokenAuth) getStateDiff(params map[string][]string) ([]byte, error) {
	s := m.metrics.GetDiff()
	return s.Format(params)
}

func (m *ModuleAITokenAuth) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:           m.getState,
		m.name + ".diff": m.getStateDiff,
	}
	return handlers
}

func (m *ModuleAITokenAuth) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadProductRuleConf,
	}
	return handlers
}

func (m *ModuleAITokenAuth) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers,
	cr string) error {
	var err error

	confPath := bfe_module.ModConfPath(cr, m.name)
	if m.conf, err = ConfLoad(confPath, cr); err != nil {
		return fmt.Errorf("%s: conf load err %v", m.name, err)
	}
	openDebug = m.conf.Log.OpenDebug

	// new Redis Client
	r := m.conf.Redis
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

	if err = m.loadProductRuleConf(nil); err != nil {
		return fmt.Errorf("%s: loadProductRuleConf() err %v", m.name, err)
	}

	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.tokenFoundProductHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.tokenFoundProductHandler): %s", m.name, err.Error())
	}

	err = cbs.AddFilter(bfe_module.HandleReadResponse, m.tokenReadResponseHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.tokenReadResponseHandler): %v", m.name, err)
	}

	err = cbs.AddFilter(bfe_module.HandleRequestFinish, m.tokenRequestFinishHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.tokenReadResponseHandler): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.monitorHandlers): %v", m.name, err)
	}

	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init(): RegisterHandlers(m.reloadHandlerr): %v", m.name, err)
	}

	return nil
}

func usedQuotaKey(key string, updatetime int64) string {
	return fmt.Sprintf("usedquota_%s:%d", key, updatetime)
}

type TokenAuthContext struct {
	Token *Token
	PromptTokens int64 // number of tokens in the prompt
	CompletionTokens int64 // number of tokens in the completion
	UsedQuota int64 // used quota for this request
}
const REQ_TOKEN_AUTH_CONTEXT = "tokenauth_ctx"
func GetTokenAuthContext(req *bfe_basic.Request) *TokenAuthContext {
	ctx := req.GetContext(REQ_TOKEN_AUTH_CONTEXT)
	tokenCtx, ok := ctx.(*TokenAuthContext)
	if !ok {
		return nil
	}

	return tokenCtx
}
// SetTokenAuthContext sets the token authentication context in the request
func SetTokenAuthContext(req *bfe_basic.Request, tokenCtx *TokenAuthContext) {
	req.SetContext(REQ_TOKEN_AUTH_CONTEXT, tokenCtx)
}

func GetPromptToken(req *bfe_basic.Request) int64 {
	// get prompt token from request body
	// just a simple implementation here, only consider content length
	// just a simple estimation: 1 token ~ 4 bytes
	if req.HttpRequest.ContentLength > 0 {
		return req.HttpRequest.ContentLength / 4
	}
	
	// if content length is not set, try to peek the body
	bodyAccessor, _ := req.HttpRequest.GetBodyAccessor()
	if bodyAccessor == nil {
		return 0
	}

	body, _ := bodyAccessor.GetBytes()
	return int64(len(body)) / 4
}
