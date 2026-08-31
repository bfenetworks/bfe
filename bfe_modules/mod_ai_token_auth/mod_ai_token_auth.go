// Copyright (c) 2025 The BFE Authors.
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
	"time"

	"github.com/bfenetworks/go-lib/log"
	"github.com/bfenetworks/go-lib/quota"
	"github.com/bfenetworks/go-lib/web-monitor/metrics"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
	"github.com/tidwall/gjson"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
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
	ReqTotal    *metrics.Counter
	ReqAuth     *metrics.Counter
	ReqAuthFail *metrics.Counter
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

	m.ruleTable.Update(conf)

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

func UpdateCtxByUsage(ctx *TokenAuthContext, data []byte) {
	var used, prompt, completion, cacheRead, cacheWrite, audioInput, audioOutput, imageInput, imageCount, videoCount int64

	used = gjson.GetBytes(data, "usage.total_tokens").Int()
	prompt = gjson.GetBytes(data, "usage.prompt_tokens").Int()
	completion = gjson.GetBytes(data, "usage.completion_tokens").Int()
	cacheRead = gjson.GetBytes(data, "usage.cache_read_tokens").Int()
	cacheWrite = gjson.GetBytes(data, "usage.cache_write_tokens").Int()
	audioInput = gjson.GetBytes(data, "usage.audio_input_tokens").Int()
	audioOutput = gjson.GetBytes(data, "usage.audio_output_tokens").Int()
	imageInput = gjson.GetBytes(data, "usage.input_token_details.image_tokens").Int()
	if imageInput == 0 {
		imageInput = gjson.GetBytes(data, "usage.image_input_tokens").Int()
	}
	imageCount = gjson.GetBytes(data, "usage.image_count").Int()
	if imageCount == 0 {
		imageCount = gjson.GetBytes(data, "data.#").Int()
	}
	videoCount = gjson.GetBytes(data, "usage.video_count").Int()
	if videoCount == 0 {
		videoCount = gjson.GetBytes(data, "data.#").Int()
	}

	// DeepSeek fallback: prompt_cache_hit_tokens / prompt_tokens_details.cached_tokens
	if cacheRead == 0 {
		cacheRead = gjson.GetBytes(data, "usage.prompt_cache_hit_tokens").Int()
	}
	if cacheRead == 0 {
		cacheRead = gjson.GetBytes(data, "usage.prompt_tokens_details.cached_tokens").Int()
	}

	// Responses API fallback: input_token_details.cached_tokens
	if cacheRead == 0 {
		cacheRead = gjson.GetBytes(data, "usage.input_token_details.cached_tokens").Int()
	}

	// Claude fallback: input_tokens / output_tokens / cache_read_input_tokens / cache_creation_input_tokens
	if prompt == 0 && completion == 0 {
		prompt = gjson.GetBytes(data, "usage.input_tokens").Int()
		completion = gjson.GetBytes(data, "usage.output_tokens").Int()
		if cacheRead == 0 {
			cacheRead = gjson.GetBytes(data, "usage.cache_read_input_tokens").Int()
		}
		if cacheWrite == 0 {
			cacheWrite = gjson.GetBytes(data, "usage.cache_creation_input_tokens").Int()
		}
		if used == 0 {
			used = prompt + completion
		}
	}

	tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
	if used > 0 {
		tokenUsage.UsedQuota = used
		tokenUsage.PromptTokens = prompt
		tokenUsage.CompletionTokens = completion
		tokenUsage.CacheReadTokens = cacheRead
		tokenUsage.CacheWriteTokens = cacheWrite
		tokenUsage.AudioInputTokens = audioInput
		tokenUsage.AudioOutputTokens = audioOutput
		tokenUsage.ImageInputTokens = imageInput
		tokenUsage.ImageCount = imageCount
		tokenUsage.VideoCount = videoCount
	} else if prompt > 0 || completion > 0 || imageCount > 0 || videoCount > 0 {
		if imageCount > 0 {
			tokenUsage.UsedQuota = imageCount
		} else if videoCount > 0 {
			tokenUsage.UsedQuota = videoCount
		} else {
			tokenUsage.UsedQuota = prompt + completion
		}
		tokenUsage.PromptTokens = prompt
		tokenUsage.CompletionTokens = completion
		tokenUsage.CacheReadTokens = cacheRead
		tokenUsage.CacheWriteTokens = cacheWrite
		tokenUsage.AudioInputTokens = audioInput
		tokenUsage.AudioOutputTokens = audioOutput
		tokenUsage.ImageInputTokens = imageInput
		tokenUsage.ImageCount = imageCount
		tokenUsage.VideoCount = videoCount
	}
}

func (m *ModuleAITokenAuth) tokenReadResponseHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	ctx := GetTokenAuthContext(req) // ensure token auth context is set
	if ctx == nil {
		return bfe_module.BfeHandlerGoOn
	}
	tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
	if res.StatusCode == bfe_http.StatusOK && res.ContentLength >= 0 {
		if bodyAccessor, err := res.GetBodyAccessor(); err == nil {
			body, _ := bodyAccessor.GetBytes()
			UpdateCtxByUsage(ctx, body)
		}
		if tokenUsage.UsedQuota <= 0 && ctx.aiBasicInfo.IsAllowEstimateToken() {
			tokenUsage.CompletionTokens = int64(res.ContentLength) / 4                                         // estimate completion tokens
			tokenUsage.UsedQuota = CalcReqUsedQuota(req, tokenUsage.PromptTokens, tokenUsage.CompletionTokens) // calculate used quota
		}
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
	// Skip token-count endpoints which should never be billed.
	if strings.Contains(req.HttpRequest.RequestURI, "/count_tokens") {
		return bfe_module.BfeHandlerGoOn
	}

	if res == nil || res.StatusCode != bfe_http.StatusOK {
		// only count used quota for successful requests
		return bfe_module.BfeHandlerGoOn
	}

	ctx := GetTokenAuthContext(req) // ensure token auth context is set
	if ctx == nil {
		return bfe_module.BfeHandlerGoOn
	}

	// Prevent duplicate deduction when HandleRequestFinish is triggered more than once.
	if ctx.deducted {
		return bfe_module.BfeHandlerGoOn
	}

	tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
	if tokenUsage.UsedQuota <= 0 && ctx.aiBasicInfo.IsAllowEstimateToken() {
		tokenUsage.UsedQuota = CalcReqUsedQuota(req, tokenUsage.PromptTokens, tokenUsage.CompletionTokens) // calculate used quota
	}

	// calculate RMB cost at request finish time using token usage already populated
	// by mod_body_process (streaming) or tokenReadResponseHandler (non-streaming).
	if tokenUsage.UsedCost <= 0 && hasRMBPlan(ctx.Token.QuotaPlans) {
		tokenUsage.UsedCost = m.calcCostUnits(req, ctx.serverConf, tokenUsage)
	}

	costUnits := tokenUsage.UsedCost

	if tokenUsage.UsedQuota > 0 || costUnits > 0 {
		for _, plan := range ctx.Token.QuotaPlans {
			if plan.Unlimited {
				continue
			}
			if quota.IsRMB(plan.Unit) {
				if costUnits > 0 {
					_, err := plan.Deduct(m.redisClient, costUnits)
					if err != nil {
						log.Logger.Warn("deduct rmb quota failed: %v", err)
					}
				}
			} else {
				if tokenUsage.UsedQuota > 0 {
					_, err := plan.Deduct(m.redisClient, tokenUsage.UsedQuota)
					if err != nil {
						log.Logger.Warn("deduct token quota failed: %v", err)
					}
				}
			}
		}
	}

	ctx.deducted = true
	return bfe_module.BfeHandlerGoOn
}

func SetApiKey(req *bfe_http.Request, apiKey string, authStyle string) {
	// set api key according to protocol/auth style
	if apiKey == "" {
		return
	}

	switch authStyle {
	case bfe_basic.AuthStyleAnthropic:
		req.Header.Set("x-api-key", apiKey)
	default:
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}
}

func GetApiKey(req *bfe_basic.Request) string {
	return bfe_basic.GetApiKey(req)
}

// found product handler
func (m *ModuleAITokenAuth) tokenFoundProductHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	meta := req.GetAiBasicInfo()
	if meta == nil {
		return bfe_module.BfeHandlerGoOn, nil
	}

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
		resp := err.CreateErrorResponse(req)
		return bfe_module.BfeHandlerResponse, resp
	}

	promptToken := 0
	if meta.IsAllowEstimateToken() {
		promptToken = int(GetPromptToken(req))
	}
	SetTokenAuthContext(req, tok, int64(promptToken), tok.Tags)

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

type TokenAuthContext struct {
	Token       *Token
	aiBasicInfo *bfe_basic.AiBasicInfo
	// serverConf caches the SvrDataConf before it is cleared by the reverse proxy.
	// It is used for RMB cost calculation at request finish time.
	serverConf bfe_basic.ServerDataConfInterface
	// deducted marks whether the quota/cost deduction has already been executed
	// for this request, preventing duplicate charges when HandleRequestFinish is
	// triggered multiple times.
	deducted bool
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

// GetImageCountFromReq reads the request body "n" field for image generation requests.
// It returns at least 1 to avoid under-billing when the field is missing or invalid.
func GetImageCountFromReq(req *bfe_basic.Request) int64 {
	bodyAccessor, _ := req.HttpRequest.GetBodyAccessor()
	if bodyAccessor == nil {
		return 1
	}
	body, _ := bodyAccessor.GetBytes()
	n := gjson.GetBytes(body, "n").Int()
	if n <= 0 {
		return 1
	}
	return n
}

// GetVideoCountFromReq reads the request body "n" field for video generation requests.
// It returns at least 1 to avoid under-billing when the field is missing or invalid.
func GetVideoCountFromReq(req *bfe_basic.Request) int64 {
	bodyAccessor, _ := req.HttpRequest.GetBodyAccessor()
	if bodyAccessor == nil {
		return 1
	}
	body, _ := bodyAccessor.GetBytes()
	n := gjson.GetBytes(body, "n").Int()
	if n <= 0 {
		return 1
	}
	return n
}

// SetTokenAuthContext sets the token authentication context in the request
func SetTokenAuthContext(req *bfe_basic.Request, tok *Token, promptToken int64, tags []bfe_basic.ApikeyTag) {
	aiBasicInfo := req.GetAiBasicInfo()
	if aiBasicInfo != nil {
		aiBasicInfo.ClientKeyId = tok.KeyId
		tusage := aiBasicInfo.GetTokenUsage()
		tusage.PromptTokens = promptToken
		tusage.CompletionTokens = bfe_basic.COMPLETION_TOKENS_UNKNOWN // -1 - unknown
		if aiBasicInfo.Mode == bfe_basic.ModeImageGeneration {
			tusage.ImageCount = GetImageCountFromReq(req)
		}
		if aiBasicInfo.Mode == bfe_basic.ModeVideoGeneration {
			tusage.VideoCount = GetVideoCountFromReq(req)
		}
		aiBasicInfo.ApikeyTags = tags
	}

	tokenCtx := &TokenAuthContext{
		Token:       tok,
		aiBasicInfo: aiBasicInfo,
		serverConf:  req.SvrDataConf,
	}
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

func hasRMBPlan(plans []*QuotaPlan) bool {
	for _, plan := range plans {
		if quota.IsRMB(plan.Unit) {
			return true
		}
	}
	return false
}

func (m *ModuleAITokenAuth) calcCostUnits(req *bfe_basic.Request, serverConf bfe_basic.ServerDataConfInterface, usage *bfe_basic.TokenUsage) int64 {
	aiMeta := req.GetAiBasicInfo()
	if aiMeta == nil || usage == nil {
		return 0
	}

	clusterName := req.Route.ClusterName
	targetModel := aiMeta.TargetModel
	mode := aiMeta.Mode
	if mode == "" {
		mode = bfe_basic.ModeChat
	}
	if clusterName == "" || targetModel == "" {
		return 0
	}

	if serverConf == nil {
		return 0
	}
	cluster, err := serverConf.ClusterTableLookup(clusterName)
	if err != nil || cluster == nil || cluster.AIConf == nil || cluster.AIConf.ModelTable == nil {
		log.Logger.Warn("model table not found for cluster %s", clusterName)
		return 0
	}

	entry := cluster_conf.LookupModelPrice(cluster.AIConf.ModelTable, targetModel, mode)
	if entry == nil {
		log.Logger.Warn("model price not found for cluster %s model %s mode %s", clusterName, targetModel, mode)
		return 0
	}

	tierName := ""
	if cluster.AIConf.ModelTable != nil {
		tierName = cluster.AIConf.ModelTable.ActiveTierName(time.Now())
	}

	switch mode {
	case bfe_basic.ModeImageGeneration:
		return calcImageGenerationCost(entry, usage, tierName)
	case bfe_basic.ModeVideoGeneration:
		return calcVideoGenerationCost(entry, usage, tierName)
	case bfe_basic.ModeResponses:
		return calcResponsesCost(entry, usage, tierName)
	default:
		return calcChatCost(entry, usage, tierName)
	}
}

func calcResponsesCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
	return calcChatCost(entry, usage, tierName)
}

func calcVideoGenerationCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
	videoCount := usage.VideoCount
	if videoCount < 0 {
		videoCount = 0
	}

	costPerVideo := entry.GetPriceInt(tierName, cluster_conf.PriceOutputCostPerVideoInt)
	if costPerVideo < 0 {
		log.Logger.Warn("invalid model price for video generation model %s", entry.Model)
		return 0
	}

	return videoCount * costPerVideo
}

func calcImageGenerationCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
	imageCount := usage.ImageCount
	if imageCount < 0 {
		imageCount = 0
	}

	costPerImage := entry.GetPriceInt(tierName, cluster_conf.PriceOutputCostPerImageInt)
	inputImageTokenCost := entry.GetPriceInt(tierName, cluster_conf.PriceInputCostPerImageTokenInt)
	if costPerImage < 0 || inputImageTokenCost < 0 {
		log.Logger.Warn("invalid model price for image generation model %s", entry.Model)
		return 0
	}

	return imageCount*costPerImage + usage.ImageInputTokens*inputImageTokenCost
}

func calcChatCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
	inputCost := entry.GetPriceInt(tierName, cluster_conf.PriceInputCostPerTokenInt)
	outputCost := entry.GetPriceInt(tierName, cluster_conf.PriceOutputCostPerTokenInt)
	if inputCost < 0 || outputCost < 0 {
		log.Logger.Warn("invalid model price for model %s", entry.Model)
		return 0
	}

	promptTokens := usage.PromptTokens
	completionTokens := usage.CompletionTokens
	cacheReadTokens := usage.CacheReadTokens
	cacheWriteTokens := usage.CacheWriteTokens
	audioInputTokens := usage.AudioInputTokens
	audioOutputTokens := usage.AudioOutputTokens

	// sanitize sub-token usage to avoid negative normal input/output or negative charges
	if cacheReadTokens < 0 {
		cacheReadTokens = 0
	}
	if cacheWriteTokens < 0 {
		cacheWriteTokens = 0
	}
	if audioInputTokens < 0 {
		audioInputTokens = 0
	}
	if audioInputTokens > promptTokens-cacheReadTokens {
		audioInputTokens = promptTokens - cacheReadTokens
	}
	if audioOutputTokens < 0 {
		audioOutputTokens = 0
	}
	if audioOutputTokens > completionTokens {
		audioOutputTokens = completionTokens
	}

	cacheReadCost := entry.GetPriceInt(tierName, cluster_conf.PriceCacheReadInputTokenCostInt)
	cacheWriteCost := entry.GetPriceInt(tierName, cluster_conf.PriceCacheCreationInputTokenCostInt)
	audioInputCost := entry.GetPriceInt(tierName, cluster_conf.PriceInputCostPerAudioTokenInt)
	audioOutputCost := entry.GetPriceInt(tierName, cluster_conf.PriceOutputCostPerAudioTokenInt)
	imageInputCost := entry.GetPriceInt(tierName, cluster_conf.PriceInputCostPerImageTokenInt)

	// normal input/output start as the full totals
	normalInput := promptTokens
	normalOutput := completionTokens

	// cache-aware billing: split cache read from prompt
	if cacheReadCost > 0 || cacheWriteCost > 0 {
		normalInput = promptTokens - cacheReadTokens
		if normalInput < 0 {
			normalInput = 0
		}
	}

	// image-aware billing: split image input from normal input
	imageInputTokens := usage.ImageInputTokens
	if imageInputCost > 0 {
		if imageInputTokens > normalInput {
			imageInputTokens = normalInput
		}
		normalInput = normalInput - imageInputTokens
		if normalInput < 0 {
			normalInput = 0
		}
	} else {
		// no image input price configured: bill image input as normal input
		imageInputTokens = 0
	}

	// audio-aware billing: split audio input from normal input
	if audioInputCost > 0 {
		if audioInputTokens > normalInput {
			audioInputTokens = normalInput
		}
		normalInput = normalInput - audioInputTokens
		if normalInput < 0 {
			normalInput = 0
		}
	} else {
		// no audio input price configured: bill audio input as normal input
		audioInputTokens = 0
	}

	// audio-aware billing: split audio output from completion
	if audioOutputCost > 0 {
		if audioOutputTokens > completionTokens {
			audioOutputTokens = completionTokens
		}
		normalOutput = completionTokens - audioOutputTokens
		if normalOutput < 0 {
			normalOutput = 0
		}
	} else {
		// no audio output price configured: bill audio output as normal output
		audioOutputTokens = 0
	}

	var cost int64
	if cacheReadCost > 0 || cacheWriteCost > 0 || audioInputCost > 0 || audioOutputCost > 0 || imageInputCost > 0 {
		cost = normalInput*inputCost +
			cacheReadTokens*cacheReadCost +
			cacheWriteTokens*cacheWriteCost +
			audioInputTokens*audioInputCost +
			imageInputTokens*imageInputCost +
			normalOutput*outputCost +
			audioOutputTokens*audioOutputCost
	} else {
		// fallback to legacy billing when no cache/audio/image price is configured
		cost = promptTokens*inputCost + completionTokens*outputCost
	}

	return cost
}
