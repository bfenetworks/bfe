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

package mod_ai_rate_limit

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"

	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/web-monitor/module_state2"
	"github.com/baidu/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/action"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util"
	"github.com/bfenetworks/bfe/bfe_util/limit_rate"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

var (
	openDebug = false
)

const (
	DiffCounterInterval = 20 // interval for diff counter (in seconds)
)

const (
	ModAiRateLimit     = "mod_ai_rate_limit"
	ModAiRateLimitDiff = "mod_ai_rate_limit_diff"

	CtxPolicyLimiter = "mod_ai_rate_limit.policy_limiter_ctx"
)

var (
	ErrAiRateLimit = errors.New("AI_RATE_LIMIT") // deny by mod_ai_rate_limit
)

// key for counter of mod_ai_rate_limit
var CounterKeys = []string{
	"REQ_AI_RATE_MEET_THRESHOLD",
}

type ModuleAiRateLimit struct {
	name      string                     // name of module
	state     *module_state2.State       // module state
	stateDiff module_state2.CounterSlice // diff counter for module state

	productConfPath string // path for product rule

	productTable *productRuleTable // product rule table

	pmsStates *PrometheusStates
	lock      sync.RWMutex

	redisClient redis_client.Client // redis client
	redisAgent  *limit_rate.RedisLRAgent

	limiterManager       *policyLimiterManager
	isRejectOnRedisError bool
}

func NewModuleAiRateLimit() *ModuleAiRateLimit {
	m := new(ModuleAiRateLimit)
	m.name = ModAiRateLimit
	m.state = new(module_state2.State)

	// init module state
	m.state.Init()
	m.state.CountersInit(CounterKeys)
	m.state.SetKeyPrefix(ModAiRateLimit)

	// init module state diff
	m.stateDiff.Init(m.state, DiffCounterInterval)
	m.stateDiff.SetKeyPrefix(ModAiRateLimitDiff)

	// create product Table
	m.productTable = newProductRuleTable(m.state)

	m.pmsStates = newPrometheusState()

	m.limiterManager = newPolicyLimiterManager()
	return m
}

func (m *ModuleAiRateLimit) Name() string {
	return m.name
}

func (m *ModuleAiRateLimit) limitFoundProductHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	meta := req.GetAiBasicInfo()
	if meta == nil {
		return bfe_module.BfeHandlerGoOn, nil
	}

	req.InitAiRateLimitHitInfo()
	ret, res := m.runProductRules(req, meta)
	return ret, res
}

func (m *ModuleAiRateLimit) runProductRules(req *bfe_basic.Request, meta *bfe_basic.AiBasicInfo) (int, *bfe_http.Response) {
	product := req.Route.Product
	rules := m.productTable.getProductRules(product)
	if rules == nil {
		if openDebug {
			log.Logger.Debug("product[%s] have no ai rate limit rules, pass", product)
		}
		return bfe_module.BfeHandlerGoOn, nil
	}

	ctx := &PolicyLimiterContext{}
	setPolicyLimiterContext(req, ctx)

	for _, rule := range rules {
		if !rule.cond.Match(req) {
			continue
		}

		if openDebug {
			log.Logger.Debug("mod_ai_rate_limit: cond[%s] matched for product[%s]", rule.condStr, product)
		}

		ret, res := m.executeCheckLimitPolicy(req, meta, rule, ctx)
		if ret != bfe_module.BfeHandlerGoOn {
			req.ErrCode = ErrAiRateLimit
			return ret, res
		}
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleAiRateLimit) executeCheckLimitPolicy(req *bfe_basic.Request, meta *bfe_basic.AiBasicInfo, rule *productRule, ctx *PolicyLimiterContext) (int, *bfe_http.Response) {
	apiKey := meta.ClientApiKey
	if apiKey == "" {
		if openDebug {
			log.Logger.Debug("mod_ai_rate_limit: no api key found, pass")
		}
		return bfe_module.BfeHandlerGoOn, nil
	}

	policyIds := m.productTable.getPolicyIds(apiKey)
	if len(policyIds) == 0 {
		if openDebug {
			log.Logger.Debug("mod_ai_rate_limit: no policies bound to apiKey[%s], pass", apiKey)
		}
		return bfe_module.BfeHandlerGoOn, nil
	}

	clientModel := meta.ClientModel

	for _, policyId := range policyIds {
		policy := m.productTable.getPolicy(policyId)
		if policy == nil {
			if openDebug {
				log.Logger.Debug("mod_ai_rate_limit: policy[%s] not found, skip", policyId)
			}
			continue
		}

		if !policy.Enabled {
			if openDebug {
				log.Logger.Debug("mod_ai_rate_limit: policy[%s] disabled, skip", policyId)
			}
			continue
		}

		if !matchModel(policy.Models, clientModel) {
			if openDebug {
				log.Logger.Debug("mod_ai_rate_limit: policy[%s] models[%v] != clientModel[%s], skip", policyId, policy.Models, clientModel)
			}
			continue
		}

		ls := m.limiterManager.getLimiterPolicySet(policyId)

		// check Concurrency
		if !ls.checkConcurrency(req, meta, m.redisAgent, ctx, clientModel, m.isRejectOnRedisError) {
			return m.executePolicyAction(req, meta, policyId, policy, rule)
		}

		// check RPM
		if !ls.checkRPM(req, meta, m.redisAgent, ctx, clientModel, m.isRejectOnRedisError) {
			return m.executePolicyAction(req, meta, policyId, policy, rule)
		}

		// check TPM
		if !ls.checkTPM(req, meta, m.redisAgent, ctx, clientModel, m.isRejectOnRedisError) {
			return m.executePolicyAction(req, meta, policyId, policy, rule)
		}
	}

	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleAiRateLimit) executePolicyAction(req *bfe_basic.Request, meta *bfe_basic.AiBasicInfo, policyId string, policy *PolicyConf, rule *productRule) (int, *bfe_http.Response) {
	if rule.hitAction.Cmd == action.ActionClose {
		return bfe_module.BfeHandlerClose, nil
	}

	if rule.hitAction.Cmd == action.ActionFinish {
		apiKey := meta.ClientApiKey

		var errorCode, limitType string
		hitInfo := req.GetAiRateLimitHitInfo()
		if hitInfo != nil {
			policyHitInfo := hitInfo.GetPolicyHitInfo(policyId)
			if policyHitInfo.IsConcurrency {
				errorCode = bfe_basic.CodeConcurrencyLimitExceeded
				limitType = bfe_basic.LimitTypeConcurrency
			} else if len(policyHitInfo.RpmRules) > 0 {
				errorCode = bfe_basic.CodeRpmLimitExceeded
				limitType = bfe_basic.LimitTypeRpm
			} else if len(policyHitInfo.TpmRules) > 0 {
				errorCode = bfe_basic.CodeTpmLimitExceeded
				limitType = bfe_basic.LimitTypeTpm
			}
		}
		if errorCode == "" {
			errorCode = bfe_basic.CodeRpmLimitExceeded
			limitType = bfe_basic.LimitTypeRpm
		}

		aiError := bfe_basic.NewAiErrorWithDetails(
			errorCode,
			bfe_basic.TypeRateLimitError,
			fmt.Sprintf("Rate limit exceeded for policy %s", policy.Name),
			&bfe_basic.AiErrorDetail{
				ApiKey:    apiKey,
				LimitType: limitType,
			},
		)

		resp := aiError.CreateErrorResponse(req)
		return bfe_module.BfeHandlerFinish, resp
	}

	if rule.hitAction.Cmd == action.ActionPass {
		return bfe_module.BfeHandlerGoOn, nil
	}

	log.Logger.Warn("%s unsupported action:%s", m.name, rule.hitAction.Cmd)
	return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleAiRateLimit) limitRequestFinishHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	meta := req.GetAiBasicInfo()
	if meta == nil {
		return bfe_module.BfeHandlerGoOn
	}

	ctx := getPolicyLimiterContext(req)
	if ctx == nil {
		return bfe_module.BfeHandlerGoOn
	}

	tokenUsage := meta.GetTokenUsage()
	for _, tpmData := range ctx.TpmLimiterDataList {
		if !tpmData.IsAllowed {
			continue
		}
		tokenDelta := tokenUsage.UsedQuota - tpmData.PreConsumeToken
		if tokenDelta != 0 {
			if err := tpmData.Limiter.UpdateTokenUsage(tpmData.BucketTimeSec, tokenDelta, m.redisAgent); err != nil {
				log.Logger.Warn("mod_ai_rate_limit: UpdateTokenUsage failed, err:%v", err)
			}
		}
		tpmData.Item.tokenCount.Add(uint64(tokenUsage.UsedQuota))
	}

	// release concurrency
	for _, limiter := range ctx.ConLimiters {
		if _, err := limiter.ConnRelease(m.redisAgent); err != nil {
			log.Logger.Warn("mod_ai_rate_limit: ConnRelease failed, err:%v", err)
		}
	}

	return bfe_module.BfeHandlerGoOn
}

func matchModel(policyModels []string, model string) bool {
	if len(policyModels) == 0 {
		return true
	}
	if model == "" {
		return false
	}
	for _, m := range policyModels {
		if m == "*" {
			return true
		}
		if m == model {
			return true
		}
	}
	return false
}

func (m *ModuleAiRateLimit) getState() *module_state2.StateData {
	return m.state.GetAll()
}

func (m *ModuleAiRateLimit) getStateDiff() *module_state2.CounterDiff {
	stateDiff := m.stateDiff.Get()
	return &stateDiff
}

func (m *ModuleAiRateLimit) getPrometheus() ([]byte, error) {
	tpmStats, rpmStats, conStats := m.limiterManager.getLimiterStats()

	m.lock.Lock()
	defer m.lock.Unlock()

	tpmMatchTotal := uint64(0)
	tpmHitTotal := uint64(0)
	tpmTokenTotal := uint64(0)
	for _, s := range tpmStats {
		pid := bfe_util.FormatAsPrometheusLabelValue(s.PolicyId)
		iid := bfe_util.FormatAsPrometheusLabelValue(s.InstId)
		tpmMatchTotal += s.MatchCount
		tpmHitTotal += s.HitCount
		tpmTokenTotal += s.TokenCount
		m.pmsStates.tpmMatchVec.WithLabelValues(pid, iid).Add(float64(s.MatchCount))
		m.pmsStates.tpmHitVec.WithLabelValues(pid, iid).Add(float64(s.HitCount))
		m.pmsStates.tpmTokenVec.WithLabelValues(pid, iid).Add(float64(s.TokenCount))
	}
	m.pmsStates.tpmMatchTotal.Set(float64(tpmMatchTotal))
	m.pmsStates.tpmHitTotal.Set(float64(tpmHitTotal))
	m.pmsStates.tpmTokenTotal.Set(float64(tpmTokenTotal))

	rpmMatchTotal := uint64(0)
	rpmHitTotal := uint64(0)
	for _, s := range rpmStats {
		pid := bfe_util.FormatAsPrometheusLabelValue(s.PolicyId)
		iid := bfe_util.FormatAsPrometheusLabelValue(s.InstId)
		rpmMatchTotal += s.MatchCount
		rpmHitTotal += s.HitCount
		m.pmsStates.rpmMatchVec.WithLabelValues(pid, iid).Add(float64(s.MatchCount))
		m.pmsStates.rpmHitVec.WithLabelValues(pid, iid).Add(float64(s.HitCount))
	}
	m.pmsStates.rpmMatchTotal.Set(float64(rpmMatchTotal))
	m.pmsStates.rpmHitTotal.Set(float64(rpmHitTotal))

	conMatchTotal := uint64(0)
	conHitTotal := uint64(0)
	for _, s := range conStats {
		pid := bfe_util.FormatAsPrometheusLabelValue(s.PolicyId)
		iid := bfe_util.FormatAsPrometheusLabelValue(s.InstId)
		conMatchTotal += s.MatchCount
		conHitTotal += s.HitCount
		m.pmsStates.conMatchVec.WithLabelValues(pid, iid).Add(float64(s.MatchCount))
		m.pmsStates.conHitVec.WithLabelValues(pid, iid).Add(float64(s.HitCount))
	}
	m.pmsStates.conMatchTotal.Set(float64(conMatchTotal))
	m.pmsStates.conHitTotal.Set(float64(conHitTotal))

	ret, err := m.pmsStates.toString()

	m.pmsStates.resetVec()

	return ret, err
}

// load product rule Table
func (m *ModuleAiRateLimit) loadProductRuleTable(query url.Values) (string, error) {
	// get file path
	path := query.Get("path")
	if path == "" {
		path = m.productConfPath // use default
	}
	log.Logger.Info("%s: begin load ProductRuleTable, path:%s", m.name, path)

	// load productRule conf
	productConf, err := AiRateLimitConfLoad(path)
	if err != nil {
		return "", fmt.Errorf("%s: product conf load err %s", m.name, err.Error())
	}

	if err = m.productTable.load(productConf); err != nil {
		return "", fmt.Errorf("%s: build AI rate limit err %s", m.name, err.Error())
	}

	// update limiter args for existing limiters
	if productConf.RateLimitPolicies != nil {
		m.limiterManager.updateLimiters(*productConf.RateLimitPolicies)
	}

	// set module version
	version := *productConf.Version
	m.state.Set("Version", version)

	log.Logger.Info("%s: load ProductRuleTable done, version[%s]", m.name, version)

	confbytes, _ := json.Marshal(productConf)
	m.state.Set("ProductRuleTable", string(confbytes))

	_, fileName := filepath.Split(path)
	return fmt.Sprintf("%s=%s", fileName, version), nil
}

// all monitor handlers
func (m *ModuleAiRateLimit) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:                 web_monitor.CreateStateDataHandler(m.getState),
		m.name + ".diff":       web_monitor.CreateCounterDiffHandler(m.getStateDiff),
		m.name + ".prometheus": m.getPrometheus,
	}
	return handlers
}

// all reload handlers
func (m *ModuleAiRateLimit) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadProductRuleTable,
	}

	return handlers
}

// module Init
func (m *ModuleAiRateLimit) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	// load module config
	confPath := bfe_module.ModConfPath(cr, m.name)
	conf, err := ConfLoad(confPath, cr)
	if err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}
	m.productConfPath = conf.Basic.ProductRulePath
	openDebug = conf.Log.OpenDebug
	m.isRejectOnRedisError = conf.Basic.IsRejectOnRedisError

	r := conf.Redis
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
	m.redisAgent = limit_rate.NewRedisLRAgent(m.redisClient)

	// load product rule table
	if _, err := m.loadProductRuleTable(nil); err != nil {
		return fmt.Errorf("%s.Init(): loadProductRuleTable(): %s", m.name, err.Error())
	}

	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.limitFoundProductHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.limitFoundProductHandler): %s", m.name, err.Error())
	}

	err = cbs.AddFilter(bfe_module.HandleRequestFinish, m.limitRequestFinishHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.limitRequestFinishHandler): %v", m.name, err)
	}

	// register web handler for monitor
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.monitorHandlers): %s", m.name, err.Error())
	}

	// register web handler for reload
	err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, m.reloadHandlers())
	if err != nil {
		return fmt.Errorf("%s.Init():RegisterHandlers(m.reloadHandlers): %s", m.name, err.Error())
	}

	return nil
}

func setPolicyLimiterContext(request *bfe_basic.Request, ctx *PolicyLimiterContext) {
	request.SetContext(CtxPolicyLimiter, ctx)
}

func getPolicyLimiterContext(request *bfe_basic.Request) *PolicyLimiterContext {
	val := request.GetContext(CtxPolicyLimiter)
	if val == nil {
		return nil
	}

	return val.(*PolicyLimiterContext)
}
