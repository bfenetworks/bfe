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

package mod_ai_cache

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"

	"github.com/bfenetworks/go-lib/log"
	"github.com/bfenetworks/go-lib/web-monitor/module_state2"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_modules/mod_ai_cache/provider/embedding"
	"github.com/bfenetworks/bfe/bfe_modules/mod_ai_cache/provider/vector"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

var (
	openDebug = false
)

const (
	DiffCounterInterval = 20 // interval for diff counter (in seconds)
)

const (
	ModAiCache     = "mod_ai_cache"
	ModAiCacheDiff = "mod_ai_cache_diff"

	CtxAiCache = "mod_ai_cache.ctx"
)

// key for counter of mod_ai_cache
var CounterKeys = []string{
	"REQ_TOTAL",
	"CACHE_HIT",
	"SEMANTIC_HIT",
	"CACHE_MISS",
	"CACHE_SKIP",
	"REDIS_ERR",
	"EMBEDDING_ERR",
	"VECTOR_ERR",
	"LATENCY_MS",
	"EMBEDDING_LATENCY_MS",
	"VECTOR_LATENCY_MS",
	"VALUE_TOO_LARGE",
	"SEMANTIC_SKIPPED",
}

type ModuleAiCache struct {
	name      string                     // name of module
	state     *module_state2.State       // module state
	stateDiff module_state2.CounterSlice // diff counter for module state

	productConfPath string // path for product rule
	cacheKeyPrefix  string // prefix of cache key
	defaultCacheTTL int    // default cache TTL (seconds)

	ruleTable *cacheRuleTable // product rule table

	pmsStates *PrometheusStates
	lock      sync.RWMutex

	redisClient redis_client.Client // redis client
	redisCache  *redisCache         // cache wrapper over redis client

	semantic *semanticCache // semantic cache (nil when not configured or init failed)
}

func NewModuleAiCache() *ModuleAiCache {
	m := new(ModuleAiCache)
	m.name = ModAiCache
	m.state = new(module_state2.State)

	// init module state
	m.state.Init()
	m.state.CountersInit(CounterKeys)
	m.state.SetKeyPrefix(ModAiCache)

	// init module state diff
	m.stateDiff.Init(m.state, DiffCounterInterval)
	m.stateDiff.SetKeyPrefix(ModAiCacheDiff)

	// create rule table
	m.ruleTable = newCacheRuleTable()

	m.pmsStates = newPrometheusStates()

	return m
}

func (m *ModuleAiCache) Name() string {
	return m.name
}

func (m *ModuleAiCache) getState() *module_state2.StateData {
	return m.state.GetAll()
}

func (m *ModuleAiCache) getStateDiff() *module_state2.CounterDiff {
	stateDiff := m.stateDiff.Get()
	return &stateDiff
}

func (m *ModuleAiCache) getPrometheus() ([]byte, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	snap := m.ruleTable.snapshotCounters()

	m.pmsStates.reqTotal.Set(float64(snap.reqTotal))
	m.pmsStates.cacheHit.Set(float64(snap.cacheHit))
	m.pmsStates.semanticHit.Set(float64(snap.semanticHit))
	m.pmsStates.cacheMiss.Set(float64(snap.cacheMiss))
	m.pmsStates.cacheSkip.Set(float64(snap.cacheSkip))
	m.pmsStates.redisErr.Set(float64(snap.redisErr))
	m.pmsStates.embeddingErr.Set(float64(snap.embeddingErr))
	m.pmsStates.vectorErr.Set(float64(snap.vectorErr))
	m.pmsStates.latencyMs.Set(float64(snap.latencyMs))
	m.pmsStates.embeddingLatencyMs.Set(float64(snap.embeddingLatencyMs))
	m.pmsStates.vectorLatencyMs.Set(float64(snap.vectorLatencyMs))
	m.pmsStates.valueTooLarge.Set(float64(snap.valueTooLarge))
	m.pmsStates.semanticSkipped.Set(float64(snap.semanticSkipped))

	return m.pmsStates.toString()
}

// load product rule table
func (m *ModuleAiCache) loadProductRuleTable(query url.Values) (string, error) {
	// get file path
	path := query.Get("path")
	if path == "" {
		path = m.productConfPath // use default
	}
	log.Logger.Info("%s: begin load ProductRuleTable, path:%s", m.name, path)

	// load productRule conf
	productConf, err := ProductRuleConfLoad(path, m.defaultCacheTTL)
	if err != nil {
		return "", fmt.Errorf("%s: product conf load err %s", m.name, err.Error())
	}

	if err = m.ruleTable.load(productConf); err != nil {
		return "", fmt.Errorf("%s: build ai cache rule err %s", m.name, err.Error())
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
func (m *ModuleAiCache) monitorHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name:                 web_monitor.CreateStateDataHandler(m.getState),
		m.name + ".diff":       web_monitor.CreateCounterDiffHandler(m.getStateDiff),
		m.name + ".prometheus": m.getPrometheus,
	}
	return handlers
}

// all reload handlers
func (m *ModuleAiCache) reloadHandlers() map[string]interface{} {
	handlers := map[string]interface{}{
		m.name: m.loadProductRuleTable,
	}

	return handlers
}

// module Init
func (m *ModuleAiCache) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
	// load module config
	confPath := bfe_module.ModConfPath(cr, m.name)
	conf, err := ConfLoad(confPath, cr)
	if err != nil {
		return fmt.Errorf("%s: conf load err %s", m.name, err.Error())
	}
	m.productConfPath = conf.Basic.ProductRulePath
	m.cacheKeyPrefix = conf.Basic.CacheKeyPrefix
	m.defaultCacheTTL = conf.Basic.DefaultCacheTTL
	openDebug = conf.Log.OpenDebug

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

	m.redisClient = redis_client.NewRedisClient(options)
	m.redisCache = newRedisCache(m.name, m.redisClient, m.ruleTable)

	// init the semantic cache (embedding + vector store). This is fail-open:
	// any problem (sections missing, invalid values, provider init failure)
	// only disables the semantic capability with a warning; the exact-match
	// cache and the whole module keep working.
	m.initSemantic(conf)

	// load product rule table
	if _, err := m.loadProductRuleTable(nil); err != nil {
		return fmt.Errorf("%s.Init(): loadProductRuleTable(): %s", m.name, err.Error())
	}

	// register filter for cache lookup at HandleFoundProduct, ordered between
	// mod_ai_token_auth and mod_ai_route (bfe_modules.go), so a cache hit
	// short-circuits before route rule evaluation — and the lazy intent
	// resolve triggered by req_ai_intent_in — saving classification latency
	// on the hit path. req.Route.Product is resolved before HandleFoundProduct
	// fires, so Search(product) is unaffected. A hit also short-circuits
	// before mod_body_process, and AiCacheStatus is still set before
	// mod_access_pb3 logging.
	err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.cacheRequestHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.cacheRequestHandler): %s", m.name, err.Error())
	}

	// register filter for cache fill at HandleReadResponse: the response body
	// is wrapped to capture the upstream answer while it streams to the client.
	err = cbs.AddFilter(bfe_module.HandleReadResponse, m.cacheResponseHandler)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.cacheResponseHandler): %v", m.name, err)
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

// initSemantic initializes the semantic cache from the [embedding]/[vector]
// sections of the module conf. It is strictly fail-open: when the sections
// are absent the module stays in pure exact-match mode; when they are
// invalid or the providers cannot be initialized, a warning is logged and
// the semantic capability is globally disabled (all enableSemanticCache
// rule switches are ignored). Init never fails because of semantic conf.
func (m *ModuleAiCache) initSemantic(conf *ConfModAiCache) {
	conn, err := conf.SemanticConnect()
	if err != nil {
		log.Logger.Warn("%s: invalid semantic conf, semantic cache disabled, err[%v]", m.name, err)
		return
	}
	if conn == nil {
		// [embedding]/[vector] not configured: pure exact-match cache mode
		return
	}

	embProvider, err := embedding.NewOpenAIProvider(embedding.Config{
		ServiceHost: conn.Embedding.ServiceHost,
		ServicePort: conn.Embedding.ServicePort,
		UseHttps:    conn.Embedding.UseHttps,
		ApiKey:      conn.Embedding.ApiKey,
		Model:       conn.Embedding.Model,
		TimeoutMs:   conn.Embedding.TimeoutMs,
	})
	if err != nil {
		log.Logger.Warn("%s: init embedding provider failed, semantic cache disabled, err[%v]", m.name, err)
		return
	}

	vecProvider, err := vector.NewChromaProvider(vector.Config{
		ServiceHost: conn.Vector.ServiceHost,
		ServicePort: conn.Vector.ServicePort,
		ApiKey:      conn.Vector.ApiKey,
		Collection:  conn.Vector.Collection,
		TimeoutMs:   conn.Vector.TimeoutMs,
	})
	if err != nil {
		log.Logger.Warn("%s: init vector provider failed, semantic cache disabled, err[%v]", m.name, err)
		return
	}

	m.semantic = newSemanticCache(embProvider, vecProvider, conn.Vector.MaxQuestionBytes, m.ruleTable)
	log.Logger.Info("%s: semantic cache enabled, collection[%s], maxQuestionBytes[%d]",
		m.name, conn.Vector.Collection, m.semantic.maxQuestionBytes)
}

// setCacheStatus records the cache status of this request in AiBasicInfo so
// that the access log and the billing logic of mod_ai_token_auth can
// recognize cache hits. A semantic hit is also a cache hit for billing: it
// sets AiCacheHit so that mod_ai_token_auth skips the deduction without
// any change on its side.
func (m *ModuleAiCache) setCacheStatus(req *bfe_basic.Request, status string) {
	aiInfo := req.GetAiBasicInfo()
	if aiInfo == nil {
		return
	}
	aiInfo.AiCacheStatus = status
	if status == CacheStatusHit {
		aiInfo.AiCacheHit = true
	}
	if status == CacheStatusHitSemantic {
		aiInfo.AiCacheHit = true
		aiInfo.AiCacheSemantic = true
	}
}

// setCacheSimilarity records the normalized similarity of a semantic cache
// hit in AiBasicInfo for the access log and threshold tuning.
func (m *ModuleAiCache) setCacheSimilarity(req *bfe_basic.Request, similarity float64) {
	aiInfo := req.GetAiBasicInfo()
	if aiInfo == nil {
		return
	}
	aiInfo.AiCacheSimilarity = similarity
}

// setCacheKey records the cache key in AiBasicInfo for debug logging; it is
// only filled when the module debug log is enabled.
func setCacheKey(req *bfe_basic.Request, key string) {
	aiInfo := req.GetAiBasicInfo()
	if aiInfo == nil {
		return
	}
	if openDebug {
		aiInfo.AiCacheKey = key
	}
}

func setAiCacheContext(request *bfe_basic.Request, ctx *aiCacheContext) {
	request.SetContext(CtxAiCache, ctx)
}

func getAiCacheContext(request *bfe_basic.Request) *aiCacheContext {
	val := request.GetContext(CtxAiCache)
	if val == nil {
		return nil
	}

	ctx, ok := val.(*aiCacheContext)
	if !ok {
		return nil
	}
	return ctx
}
