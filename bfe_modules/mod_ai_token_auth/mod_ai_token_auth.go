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
	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	modelprotocol "github.com/bfenetworks/bfe/bfe_model_protocol"
	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util"
	"github.com/bfenetworks/bfe/bfe_util/crypto"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

const (
	ModAITokenAuth = "mod_ai_token_auth"
)

var (
	openDebug = false

	// defaultTokenAuthModule backs the exported batch quota primitives
	// (BatchPreCheck/BatchReserve/BatchRelease) used by mod_ai_batch. The
	// module is a singleton in practice: bfe_modules.go constructs it once.
	defaultTokenAuthModule *ModuleAITokenAuth
)

type ModuleAITokenAuthState struct {
	ReqTotal    *metrics.Counter
	ReqAuth     *metrics.Counter
	ReqAuthFail *metrics.Counter
	// PriceLookupMiss counts billing attempts whose model price lookup
	// missed with no fallback available (unknown cluster/model, or the model
	// has no chat-mode baseline entry to fall back to). Such misses bill 0;
	// non-chat misses with a chat baseline fall back instead (issue #1408).
	// The counter is the monitoring signal for misconfigured price tables.
	PriceLookupMiss *metrics.Counter
	// PriceFallbackUsed counts billing attempts whose (model, mode) price
	// entry was missing but the cost was recovered by falling back to the
	// model's chat-mode price (issue #1408 product decision: an unpriced mode
	// is billed the same as chat). Together with the per-request ai_price_miss
	// access-log marker it keeps the price-table gap visible.
	PriceFallbackUsed *metrics.Counter
	// Non2xxEstimateDropped counts requests whose upstream call never
	// succeeded (non-2xx) and whose EstimateToken residue (auth-time prompt
	// seeding / response-time completion guessing) was dropped from the
	// shared TokenUsage so it could not be reported as model usage
	// (issue #1409).
	Non2xxEstimateDropped *metrics.Counter
	// Non2xxRealUsageKept counts non-2xx requests that still carried a real
	// parsed usage. Such observations are kept in the shared TokenUsage but
	// never reported by the access log (issue #1409); a non-zero value means
	// an upstream returned usage on an error response.
	Non2xxRealUsageKept *metrics.Counter
	// UsageUnrecognized counts 2xx requests whose upstream usage was never
	// recognized (no final usage parsed) and which are not billable by
	// EstimateToken (estimate disabled, or response not completed). Their
	// billing fields are zeroed and no RMB deduction happens. Together with
	// the per-request ai_usage_source access-log marker this is the
	// product-side signal for the billing bypass (issue #1410); a genuine
	// zero-usage request parses successfully and never lands here.
	UsageUnrecognized *metrics.Counter
	// AbortBillingSkipped counts client aborts (RST/close/write failure)
	// detected before the final usage was seen: billing is skipped entirely
	// per the issue #1352 yardstick, whether or not an initial usage (e.g.
	// Anthropic message_start) was already parsed. It was counted by
	// UsageUnrecognized until issue #1414 split the two causes, so that an
	// upstream/parsing problem and a client-side disconnect stay separable:
	// only the former should be alerted on, while the latter tracks the
	// client-abort rate.
	AbortBillingSkipped *metrics.Counter
	// UpstreamDropBilled counts 2xx requests whose upstream connection
	// broke mid-response (issue #1474) and which were billed by the
	// forwarded-content estimate. Unlike a client abort, content already
	// delivered to the client is real consumption and must be charged;
	// this counter keeps the two zero-billing-adjacent shapes separable
	// from AbortBillingSkipped (client aborts stay zero-billed per
	// issue #1352) and from the issue #1415 clean-EOF estimate billing.
	UpstreamDropBilled *metrics.Counter
	// BillingZeroCost counts 2xx requests that hold an RMB quota plan and were
	// charged 0, excluding AI cache hits (which never call the upstream, so
	// their zero cost is by design). It is the structural counterpart of
	// UsageUnrecognized: instead of covering one billing bypass, it covers
	// every zero-cost 2xx request — unrecognized usage, client aborts, batch
	// releases, price-table gaps, deduction-time configuration loss — so a new
	// bypass shape cannot stay silent. The per-request cause is reported as
	// ai_zero_cost_reason (issue #1414).
	BillingZeroCost *metrics.Counter
	// DeductFailed counts quota deductions that the Redis write rejected.
	// Such requests were billed in the log but not charged; the failure used
	// to produce only a log line (issue #1414).
	DeductFailed *metrics.Counter
	// QuotaOverage counts quota deductions that drove the balance negative
	// (issue #1476): the plan is in arrears and the overrun is fully
	// recorded in the balance. Since the fix, balances are never truncated
	// at zero, so a negative balance means cumulative consumption has
	// exceeded the plan quota — normal right after a large request exhausts
	// a small plan, but a sustained growth under PassNoQuota signals the
	// pass-through mode is accumulating unpaid usage.
	QuotaOverage *metrics.Counter
}

type ModuleAITokenAuth struct {
	name      string
	conf      *ConfModAITokenAuth
	ruleTable *TokenRuleTable
	state     ModuleAITokenAuthState
	metrics   metrics.Metrics

	redisClient redis_client.Client // redis client

	// keyFile is the file-encryption keyring path from bfe.conf
	// [Security].KeyFile; empty means decryption disabled. The keyring
	// itself is re-read on every rule reload (rotation hot reload).
	keyFile string
}

func NewModuleAITokenAuth() *ModuleAITokenAuth {
	m := new(ModuleAITokenAuth)
	m.name = ModAITokenAuth
	m.metrics.Init(&m.state, ModAITokenAuth, 0)
	m.ruleTable = NewTokenRuleTable()
	defaultTokenAuthModule = m
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

	// Re-read the keyring on every load: rotation adds the new key to all
	// decrypt endpoints first, so a reload must pick it up without a
	// restart. A broken keyring fails this reload and the previously
	// effective config is kept.
	var kr *crypto.Keyring
	if m.keyFile != "" {
		var err error
		kr, err = crypto.LoadKeyringFile(m.keyFile)
		if err != nil {
			return fmt.Errorf("err in LoadKeyringFile(%s): %s", m.keyFile, err)
		}
	}

	conf, err := ProductRuleConfLoad(path, kr)
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

// UpdateCtxByUsage writes the usage observed in a complete non-streaming body
// into the shared TokenUsage. It reports whether the upstream carried a usage
// object at all (issue #1414): presence, not "some value is > 0", is what
// distinguishes a genuine all-zero usage report from a missing one, and the
// caller marks the final usage / gates the estimate on it.
func UpdateCtxByUsage(ctx *TokenAuthContext, data []byte) bool {
	// The protocol/auth style is identified per request before the response
	// is processed (GetApiKey / DetectAuthStyle), so the adapter for
	// aiMeta.AuthStyle carries the same usage chain the legacy all-chain
	// extraction applied to this response.
	fields := modelprotocol.Get(ctx.aiBasicInfo.AuthStyle).ExtractUsageFields(data)
	if !fields.UsagePresent ||
		(fields.UsedQuota == 0 && fields.PromptTokens == 0 && fields.CompletionTokens == 0 &&
			fields.ImageCount == 0 && fields.VideoCount == 0) {
		// Auth style / response format mismatch (e.g. a Bearer key detected
		// as openai while the backend returns an Anthropic body): the single
		// adapter found no usable usage (possibly a usage object in another
		// protocol's field names). Fall back to the composed cross-protocol
		// chain so the final usage is still recognized (issue #1364);
		// otherwise the final-usage mark is never set and the request-finish
		// guard would zero the usage. A present-but-all-zero usage object also
		// re-runs the chain, which is harmless: the fallback keeps presence
		// (issue #1414).
		fields = utils.ParseUsageFieldsCrossProtocol(data)
	}
	if !fields.UsagePresent {
		return false
	}

	tokenUsage := ctx.aiBasicInfo.GetTokenUsage()

	// Generated counts are not part of every upstream's usage object: keep the
	// request-derived fallback installed at auth time (image_generation /
	// video_generation) exactly as the streaming accumulator does, so an
	// object that omits image_count cannot zero a billable count.
	if fields.ImageCount == 0 && fields.VideoCount == 0 {
		fields.ImageCount = tokenUsage.ImageCount
		fields.VideoCount = tokenUsage.VideoCount
	}

	// total_tokens is absent in some shapes (Anthropic): derive the quota from
	// the parts, with the generated count as the quota unit for image / video
	// modes (legacy derivation, issue #1414 keeps it).
	if fields.UsedQuota == 0 {
		switch {
		case fields.ImageCount > 0:
			fields.UsedQuota = fields.ImageCount
		case fields.VideoCount > 0:
			fields.UsedQuota = fields.VideoCount
		default:
			fields.UsedQuota = fields.PromptTokens + fields.CompletionTokens
		}
	}

	// The upstream reported usage: its values are the observation, including
	// an explicit all-zero report (billed at 0, classified as parsed).
	tokenUsage.UsedQuota = fields.UsedQuota
	tokenUsage.PromptTokens = fields.PromptTokens
	tokenUsage.CompletionTokens = fields.CompletionTokens
	tokenUsage.CacheReadTokens = fields.CacheReadTokens
	tokenUsage.CacheWriteTokens = fields.CacheWriteTokens
	tokenUsage.CacheWriteTokens1h = fields.CacheWriteTokens1h
	tokenUsage.AudioInputTokens = fields.AudioInputTokens
	tokenUsage.AudioOutputTokens = fields.AudioOutputTokens
	tokenUsage.ImageInputTokens = fields.ImageInputTokens
	tokenUsage.ImageCount = fields.ImageCount
	tokenUsage.VideoCount = fields.VideoCount

	return true
}

// isJSONMediaType reports whether a Content-Type header value denotes JSON,
// ignoring media type parameters such as "; charset=utf-8".
func isJSONMediaType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(mediaType, ';'); i >= 0 {
		mediaType = strings.TrimSpace(mediaType[:i])
	}
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

// isCompleteNonStreamBody reports whether res carries a complete, non-streaming
// body that can be read whole during the read-response phase.
//
// A fixed-length response (ContentLength >= 0) is always complete. A chunked
// response (ContentLength == -1, e.g. gzip + chunked upstream) is read whole
// only when it is neither an SSE stream nor a non-JSON type that may stream
// incrementally; everything else is left to mod_body_process so a real stream
// is never blocked here (issue #1406).
func isCompleteNonStreamBody(res *bfe_http.Response) bool {
	if res.ContentLength >= 0 {
		return true
	}
	return !res.IsSse && isJSONMediaType(res.Header.Get("Content-Type"))
}

func (m *ModuleAITokenAuth) tokenReadResponseHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	ctx := GetTokenAuthContext(req) // ensure token auth context is set
	if ctx == nil {
		return bfe_module.BfeHandlerGoOn
	}
	tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
	if res.StatusCode == bfe_http.StatusOK && isCompleteNonStreamBody(res) {
		// The full non-streaming body was read: the response is complete.
		ctx.aiBasicInfo.MarkResponseCompleted()
		bodyLen := 0
		usagePresent := false
		if bodyAccessor, err := res.GetBodyAccessor(); err == nil {
			body, _ := bodyAccessor.GetBytes()
			// Decode a parse-only copy: res.Body and res.Header are left
			// untouched so the bytes forwarded to the client, and the
			// Content-Encoding response header, stay exactly as the upstream
			// sent them (issue #1406: gzip responses were never decoded).
			if decoded, derr := bfe_util.DecodeContentEncoding(res.Header.Get("Content-Encoding"), body); derr == nil {
				body = decoded
			} else {
				log.Logger.Warn("%s: decode content-encoding %q failed: %v",
					m.name, res.Header.Get("Content-Encoding"), derr)
			}
			bodyLen = len(body)
			usagePresent = UpdateCtxByUsage(ctx, body)
		}
		if usagePresent {
			// The upstream carried a usage object — including an explicit
			// all-zero report, which is a real observation and must not be
			// re-estimated (issue #1414).
			ctx.aiBasicInfo.MarkFinalUsageSeen()
		}
		if !usagePresent && ctx.aiBasicInfo.IsAllowEstimateToken() {
			// A chunked response has ContentLength == -1, so estimate from the
			// decoded body length instead of the (missing) Content-Length.
			if bodyLen == 0 && res.ContentLength > 0 {
				bodyLen = int(res.ContentLength)
			}
			tokenUsage.CompletionTokens = int64(bodyLen) / 4                                                   // estimate completion tokens
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

// isClientAbortErr reports whether err is a client-side abort error:
// the client reset/closed the connection or the response could not be
// written to the client.
func isClientAbortErr(err error) bool {
	return err == bfe_basic.ErrClientWrite ||
		err == bfe_basic.ErrClientClose ||
		err == bfe_basic.ErrClientReset
}

func (m *ModuleAITokenAuth) tokenRequestFinishHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	if res == nil || res.StatusCode != bfe_http.StatusOK {
		// only count used quota for successful requests; a non-2xx response
		// means the upstream was never successfully called, so the request
		// must not report model usage (issue #1409).
		m.dropUnbilledEstimate(req, res)
		return bfe_module.BfeHandlerGoOn
	}

	// Skip token-count endpoints which should never be billed.
	if strings.Contains(req.HttpRequest.RequestURI, "/count_tokens") {
		return bfe_module.BfeHandlerGoOn
	}

	ctx := GetTokenAuthContext(req) // ensure token auth context is set
	if ctx == nil {
		return bfe_module.BfeHandlerGoOn
	}

	// The upstream was successfully called: the shared TokenUsage holds the
	// observed usage (real values, or the EstimateToken estimate approved by
	// the billing yardstick). It may be reported by the access log
	// (issue #1398 / #1409).
	ctx.aiBasicInfo.SetUsageDisposition(bfe_basic.UsageDispositionObserved)

	// Batch quota contract (mod_ai_batch): a download settlement or a
	// terminal release published in AiBasicInfo. Executed exactly once and
	// never falls through to the standard usage pricing below.
	if m.batchContractHandler(req, ctx) {
		return bfe_module.BfeHandlerGoOn
	}

	// AI cache hit (mod_ai_cache): the upstream was never called and no
	// usage exists, so token/cost deduction must be skipped entirely.
	if ctx.aiBasicInfo.AiCacheHit {
		// issue #1414: mark why this 2xx request is free. Cache hits are the
		// one zero-cost shape that is legitimate by construction, so they are
		// reported per request but deliberately not counted by BillingZeroCost
		// (counting them would drown the anomalies).
		ctx.aiBasicInfo.SetZeroCostReason(bfe_basic.ZeroCostReasonCacheHit)
		ctx.deducted = true
		return bfe_module.BfeHandlerGoOn
	}

	// Prevent duplicate deduction when HandleRequestFinish is triggered more than once.
	if ctx.deducted {
		return bfe_module.BfeHandlerGoOn
	}

	// Client aborted (RST/close/write-fail) before the final usage was
	// seen: never bill by full-request estimation (issue #1352).
	aborted := isClientAbortErr(req.ErrCode)
	if aborted && !ctx.aiBasicInfo.IsFinalUsageSeen() {
		// The abort zero-deduction is a billing bypass too. issue #1414 split
		// it out of UsageUnrecognized (a client-side disconnect is not an
		// upstream/parsing problem and must not pollute that alert), gave it a
		// per-request marker and counted it in BillingZeroCost. Counter-only on
		// purpose: a warn per aborted request would just track the client-abort
		// rate, so this branch is also excluded from the checkpoint's warn set.
		m.state.AbortBillingSkipped.Inc(1)
		ctx.aiBasicInfo.SetUsageSource(bfe_basic.UsageSourceAborted)
		if hasRMBPlan(ctx.Token.QuotaPlans) {
			ctx.aiBasicInfo.SetZeroCostReason(bfe_basic.ZeroCostReasonAborted)
			m.state.BillingZeroCost.Inc(1)
		}
		ctx.deducted = true
		return bfe_module.BfeHandlerGoOn
	}

	tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
	// Values populated by EstimateToken (prompt tokens seeded from request
	// size, completion tokens accumulated from response content) are only
	// billable when the response completed normally. Reset them otherwise:
	// calcCostUnits below bills from the token fields directly, so guarding
	// UsedQuota alone is not enough. All billing fields must be cleared:
	// leaving sub-token fields (cache/audio/image) behind would let
	// calcCostUnits bill the survivors alone (issue #1364: an unrecognized
	// final usage was reduced to CacheReadTokens and billed cache-read only).
	//
	// issue #1398: the reset only applies to a billing copy, never to the
	// shared TokenUsage. mod_access_pb3 reads the same object for the access
	// log (AiTotalTokens = UsedQuota), so zeroing it here silently wiped the
	// statistics/billing of requests whose backend really produced tokens
	// (42.7% of 200 responses landed total_tokens=0 with TTFT > 0). Only the
	// billing result UsedCost is written back.
	billingUsage := *tokenUsage
	// issue #1474: an upstream that dropped the connection mid-response
	// (marked upstreamDropped in the send path) is billable by the
	// forwarded-content estimate just like a completed response: content
	// delivered to the client is real consumption. A client abort never
	// reaches this gate (the issue #1352 abort branch above returns
	// first).
	estimateBillable := ctx.aiBasicInfo.IsAllowEstimateToken() &&
		(ctx.aiBasicInfo.IsResponseCompleted() || ctx.aiBasicInfo.IsUpstreamDropped())
	unrecognized := !ctx.aiBasicInfo.IsFinalUsageSeen() && !estimateBillable
	if unrecognized {
		billingUsage.PromptTokens = 0
		billingUsage.CompletionTokens = 0
		billingUsage.CacheReadTokens = 0
		billingUsage.CacheWriteTokens = 0
		billingUsage.CacheWriteTokens1h = 0
		billingUsage.AudioInputTokens = 0
		billingUsage.AudioOutputTokens = 0
		billingUsage.ImageInputTokens = 0
		billingUsage.VideoCount = 0
		billingUsage.ImageCount = 0
		billingUsage.UsedQuota = 0

		// issue #1410: this branch silently skipped billing (no deduction,
		// total=0 indistinguishable from a genuine zero-usage row in the
		// access log). Make the bypass observable: count it (alertable via
		// monitorHandlers) and warn with the sub-criteria so the cause is
		// readable without going back to the source.
		m.state.UsageUnrecognized.Inc(1)
		log.Logger.Warn("%s: usage unrecognized and not estimatable, billing skipped: cluster=%s key_id=%s model=%s response_completed=%v allow_estimate=%v",
			m.name, req.Route.ClusterName, ctx.aiBasicInfo.ClientKeyId, ctx.aiBasicInfo.TargetModel,
			ctx.aiBasicInfo.IsResponseCompleted(), ctx.aiBasicInfo.IsAllowEstimateToken())
		// issue #1414: the cause is consumed by the billing checkpoint below,
		// which counts BillingZeroCost for this request. issue #1474: keep
		// an upstream drop (estimate disabled) distinguishable from a
		// generic unrecognized shape.
		if ctx.aiBasicInfo.IsUpstreamDropped() {
			ctx.aiBasicInfo.SetZeroCostReason(bfe_basic.ZeroCostReasonUpstreamDropped)
		} else {
			ctx.aiBasicInfo.SetZeroCostReason(bfe_basic.ZeroCostReasonUnrecognized)
		}
	}
	// issue #1410 / #1414: record where the reported token usage came from at
	// the same decision point, so the ai_usage_source log marker can never
	// diverge from the billing call. Since #1414 the final-usage mark is
	// presence-driven, so an upstream reporting all-zero usage lands in the
	// parsed branch (billed at 0) instead of being mistaken for missing usage.
	switch {
	case ctx.aiBasicInfo.IsFinalUsageSeen():
		ctx.aiBasicInfo.SetUsageSource(bfe_basic.UsageSourceParsed)
	case unrecognized:
		ctx.aiBasicInfo.SetUsageSource(bfe_basic.UsageSourceUnrecognized)
	case ctx.aiBasicInfo.IsUpstreamDropped():
		// issue #1474: billed (or billable) by the forwarded-content
		// estimate after an upstream mid-stream drop, distinct from both a
		// clean-EOF estimate and a client abort.
		ctx.aiBasicInfo.SetUsageSource(bfe_basic.UsageSourceUpstreamDropped)
		m.state.UpstreamDropBilled.Inc(1)
	default:
		ctx.aiBasicInfo.SetUsageSource(bfe_basic.UsageSourceEstimated)
	}
	// issue #1414 (EstimateToken yardstick): the estimate backs only a request
	// from which no usage object was ever collected. A final usage that
	// reports all zeros is an observation, not a gap.
	if !ctx.aiBasicInfo.IsFinalUsageSeen() && billingUsage.UsedQuota <= 0 && estimateBillable {
		billingUsage.UsedQuota = CalcReqUsedQuota(req, billingUsage.PromptTokens, billingUsage.CompletionTokens) // calculate used quota
	}
	if !ctx.aiBasicInfo.IsFinalUsageSeen() && estimateBillable && tokenUsage.UsedQuota <= 0 && billingUsage.UsedQuota > 0 {
		// Mirror the approved estimate into the shared object so the access
		// log (AiTotalTokens) records the estimate instead of 0 (issue
		// #1398: "missing usage" must be observable, never silently zeroed).
		tokenUsage.UsedQuota = billingUsage.UsedQuota
	}

	// calculate RMB cost at request finish time using token usage already populated
	// by mod_body_process (streaming) or tokenReadResponseHandler (non-streaming).
	if billingUsage.UsedCost <= 0 && hasRMBPlan(ctx.Token.QuotaPlans) {
		billingUsage.UsedCost = m.calcCostUnits(req, ctx.serverConf, &billingUsage)
	}
	// Write back the billing result only; the token fields of the shared
	// object keep their observed/estimated values for the access log.
	tokenUsage.UsedCost = billingUsage.UsedCost

	costUnits := billingUsage.UsedCost

	// issue #1414: single checkpoint for "an RMB plan was bound but nothing was
	// charged". Each bypass site marks a reason on the request, and this block
	// turns it into the alertable counter and the per-request log marker, so a
	// future bypass shape only needs to mark its reason to become visible.
	//
	// Two shapes are deliberately excluded because their zero cost is by
	// design, not an anomaly:
	//   - AI cache hits, which returned earlier (the upstream was never called);
	//   - requests that never became a billable model call: batch / files
	//     management endpoints (upload, create, get, list, cancel, download) and
	//     anything else without a resolved target model. Counting them would
	//     inflate the alert baseline of every batch lifecycle; the batch
	//     settlement itself is counted separately in batchSettle, and
	//     calcCostUnits marks no reason for these shapes either.
	if costUnits == 0 && hasRMBPlan(ctx.Token.QuotaPlans) &&
		ctx.aiBasicInfo.TargetModel != "" && !isBatchManagementMode(ctx.aiBasicInfo.Mode) {
		reason := ctx.aiBasicInfo.ZeroCostReason()
		// issue #1474: an upstream drop with estimation disabled zeroed every
		// billing field before pricing ran, so a pricing-config gap recorded
		// by calcCostUnits must not mask the actual cause of the zero cost.
		if unrecognized && ctx.aiBasicInfo.IsUpstreamDropped() {
			reason = bfe_basic.ZeroCostReasonUpstreamDropped
		}
		if reason == "" {
			if noBillableTokens(&billingUsage) {
				reason = bfe_basic.ZeroCostReasonNoUsageTokens
			} else {
				reason = bfe_basic.ZeroCostReasonCostZero
			}
		}
		ctx.aiBasicInfo.SetZeroCostReason(reason)
		m.state.BillingZeroCost.Inc(1)
		// Shapes that already logged a line at their own site (unrecognized
		// guard, abort branch, price lookup, model table lookup) are not
		// logged twice here.
		if zeroCostNeedsWarn(reason) {
			log.Logger.Warn("%s: RMB plan bound but cost is 0: cluster=%s key_id=%s model=%s mode=%s reason=%s",
				m.name, req.Route.ClusterName, ctx.aiBasicInfo.ClientKeyId,
				ctx.aiBasicInfo.TargetModel, ctx.aiBasicInfo.Mode, reason)
		}
	}

	if billingUsage.UsedQuota > 0 || costUnits > 0 {
		for _, plan := range ctx.Token.QuotaPlans {
			if plan.Unlimited {
				continue
			}
			if quota.IsRMB(plan.Unit) {
				if costUnits > 0 {
					_, err := plan.Deduct(m.redisClient, costUnits)
					if err != nil {
						m.state.DeductFailed.Inc(1)
						log.Logger.Warn("deduct rmb quota failed: %v", err)
					}
				}
			} else {
				if billingUsage.UsedQuota > 0 {
					_, err := plan.Deduct(m.redisClient, billingUsage.UsedQuota)
					if err != nil {
						m.state.DeductFailed.Inc(1)
						log.Logger.Warn("deduct token quota failed: %v", err)
					}
				}
			}
		}
	}

	ctx.deducted = true
	return bfe_module.BfeHandlerGoOn
}

// dropUnbilledEstimate handles the request-finish path of a response whose
// upstream call never succeeded (res == nil or non-2xx, issue #1409).
//
// Such a request cannot have produced model usage, yet the shared TokenUsage
// may still carry EstimateToken residue: the prompt tokens seeded at auth
// time (mod_ai_token_auth.tokenFoundProductHandler, gated only by
// Server.EstimateToken) and the completion tokens accumulated from response
// bytes by mod_body_process.QuotaUsageProcessor. Because the access log
// (mod_access_pb3) reads that shared object directly, the residue used to be
// written as ai_input_tokens/ai_output_tokens while ai_total_tokens stayed 0,
// polluting every token-based report.
//
// The residue is dropped here and the request is marked so the access log
// reports no token field at all. Real usage parsed from the response is
// never dropped: issue #1398 keeps observations visible on the shared
// object, so it is only marked (and counted) as not-reportable.
func (m *ModuleAITokenAuth) dropUnbilledEstimate(req *bfe_basic.Request, res *bfe_http.Response) {
	aiBasicInfo := req.GetAiBasicInfo()
	if aiBasicInfo == nil {
		return
	}

	status := 0
	if res != nil {
		status = res.StatusCode
	}

	aiBasicInfo.SetUsageDisposition(bfe_basic.UsageDispositionNone)

	if aiBasicInfo.IsFinalUsageSeen() {
		// A real usage was parsed even though the final status is not 2xx
		// (e.g. an upstream returning usage on an error response): keep the
		// observation but never report it as usage of a failed request.
		m.state.Non2xxRealUsageKept.Inc(1)
		log.Logger.Warn("%s: non-2xx response carries real usage, not reported: status=%d cluster=%s key_id=%s",
			m.name, status, req.Route.ClusterName, aiBasicInfo.ClientKeyId)
		return
	}

	usage := aiBasicInfo.GetTokenUsage()
	if usage.PromptTokens == 0 && usage.CompletionTokens <= 0 && usage.UsedQuota == 0 {
		return // no residue to drop
	}

	usage.PromptTokens = 0
	usage.CompletionTokens = 0
	usage.CacheReadTokens = 0
	usage.CacheWriteTokens = 0
	usage.CacheWriteTokens1h = 0
	usage.AudioInputTokens = 0
	usage.AudioOutputTokens = 0
	usage.ImageInputTokens = 0
	usage.ImageCount = 0
	usage.VideoCount = 0
	usage.UsedQuota = 0

	m.state.Non2xxEstimateDropped.Inc(1)
	log.Logger.Warn("%s: drop estimated usage of non-2xx response: status=%d cluster=%s model=%s req_body_len=%d",
		m.name, status, req.Route.ClusterName, aiBasicInfo.TargetModel, req.HttpRequest.ContentLength)
}

func SetApiKey(req *bfe_http.Request, apiKey string, authStyle string) {
	// set api key according to protocol/auth style
	_ = modelprotocol.Get(authStyle).InjectAuth(req, apiKey)
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

	// resolve the file-encryption keyring path from bfe.conf [Security];
	// the keyring itself is re-read on every rule reload (rotation hot
	// reload). Empty KeyFile disables decryption (plaintext pass-through).
	secConf, err := bfe_conf.LoadSecurityConf(cr)
	if err != nil {
		return fmt.Errorf("%s: load security conf err %v", m.name, err)
	}
	m.keyFile = secConf.KeyFile

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

	// issue #1387: the target model allow/block check runs on the AI
	// forwarding stage callback (target model resolved), re-validated per
	// cluster attempt. Registered before mod_ai_rate_limit so that the
	// allow/block check always precedes rate limiting on this point.
	err = cbs.AddFilter(bfe_module.HandleAfterAITargetModel, m.targetModelCheckFilter)
	if err != nil {
		return fmt.Errorf("%s.Init(): AddFilter(m.targetModelCheckFilter): %s", m.name, err.Error())
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

// isBatchManagementMode reports whether an AI mode belongs to a batch / files
// management endpoint. Such requests carry the API key and its quota plan but
// never invoke a model, so a zero cost is by design and must be neither
// counted nor attributed (issue #1414).
func isBatchManagementMode(mode string) bool {
	return mode == bfe_basic.ModeFile || mode == bfe_basic.ModeBatch
}

// noBillableTokens reports whether every billed dimension of the usage is 0
// (issue #1414). It distinguishes a zero cost that the observation itself
// explains from one caused by a pricing or configuration problem.
func noBillableTokens(u *bfe_basic.TokenUsage) bool {
	return u.PromptTokens == 0 && u.CompletionTokens == 0 && u.CacheReadTokens == 0 &&
		u.CacheWriteTokens == 0 && u.CacheWriteTokens1h == 0 && u.AudioInputTokens == 0 &&
		u.AudioOutputTokens == 0 && u.ImageInputTokens == 0 && u.ImageCount == 0 && u.VideoCount == 0
}

// zeroCostNeedsWarn reports whether a zero-cost reason still needs a log line
// from the billing checkpoint. Unrecognized usage, client aborts, price
// lookups and model-table lookups already log at their own site with more
// context (issue #1414).
func zeroCostNeedsWarn(reason string) bool {
	switch reason {
	case bfe_basic.ZeroCostReasonPriceZero, bfe_basic.ZeroCostReasonNoUsageTokens,
		bfe_basic.ZeroCostReasonCostZero, bfe_basic.ZeroCostReasonBatchRelease,
		bfe_basic.ZeroCostReasonConfigMissing:
		return true
	}
	return false
}

func (m *ModuleAITokenAuth) calcCostUnits(req *bfe_basic.Request, serverConf bfe_basic.ServerDataConfInterface, usage *bfe_basic.TokenUsage) int64 {
	aiMeta := req.GetAiBasicInfo()
	// issue #1414: every early return that bills 0 marks why, so the billing
	// checkpoint can report the cause instead of a bare zero. Returns that are
	// excluded from the checkpoint (see below) deliberately mark nothing, so
	// the per-request reason can never contradict the counter.
	if aiMeta == nil || usage == nil {
		return 0
	}

	clusterName := req.Route.ClusterName
	targetModel := aiMeta.TargetModel
	mode := aiMeta.Mode
	if mode == "" {
		mode = bfe_basic.ModeChat
	}
	if isBatchManagementMode(mode) {
		// Batch / files management endpoints (upload, create, get, list,
		// cancel, download) carry the API key and its quota plan but never
		// call a model: there is nothing to price, and their zero cost is by
		// design rather than a billing anomaly.
		return 0
	}
	if targetModel == "" {
		// No target model was resolved, so the request never became a billable
		// model call: nothing to price and nothing to attribute.
		return 0
	}
	if clusterName == "" {
		// A model call that resolved no cluster cannot be priced: a genuine
		// billing-configuration gap.
		aiMeta.SetZeroCostReason(bfe_basic.ZeroCostReasonConfigMissing)
		return 0
	}

	if serverConf == nil {
		aiMeta.SetZeroCostReason(bfe_basic.ZeroCostReasonConfigMissing)
		return 0
	}
	cluster, err := serverConf.ClusterTableLookup(clusterName)
	if err != nil || cluster == nil || cluster.AIConf == nil || cluster.AIConf.ModelTable == nil {
		aiMeta.SetZeroCostReason(bfe_basic.ZeroCostReasonModelTableMissing)
		log.Logger.Warn("model table not found for cluster %s", clusterName)
		return 0
	}

	entry := cluster_conf.LookupModelPrice(cluster.AIConf.ModelTable, targetModel, mode)
	priceMissed := false
	if entry == nil {
		// Semantic fallback (issue #1408, product decision): when the
		// (model, mode) entry is absent, bill at the model's chat-mode price
		// (an unpriced mode is billed the same as chat). The chat mode itself
		// never falls back: a missing chat baseline stays fail-loud (bill 0).
		// The miss is never silent: the request is marked ai_price_miss and
		// counted, so the price-table gap stays visible in reconciliation.
		if mode != bfe_basic.ModeChat {
			if chatEntry := cluster_conf.LookupModelPrice(cluster.AIConf.ModelTable, targetModel, bfe_basic.ModeChat); chatEntry != nil {
				entry = chatEntry
				priceMissed = true
			}
		}
		if entry == nil {
			m.state.PriceLookupMiss.Inc(1)
			aiMeta.MarkPriceMiss()
			aiMeta.SetZeroCostReason(bfe_basic.ZeroCostReasonPriceMiss)
			log.Logger.Warn("model price not found for cluster %s model %s mode %s", clusterName, targetModel, mode)
			return 0
		}
	}
	if priceMissed {
		m.state.PriceFallbackUsed.Inc(1)
		aiMeta.MarkPriceMiss()
		log.Logger.Warn("model price fell back to chat price: cluster=%s model=%s mode=%s", clusterName, targetModel, mode)
	}

	tierName := ""
	if cluster.AIConf.ModelTable != nil {
		tierName = cluster.AIConf.ModelTable.ActiveTierName(time.Now())
	}

	var cost int64
	switch mode {
	case bfe_basic.ModeImageGeneration:
		cost = calcImageGenerationCost(entry, usage, tierName)
	case bfe_basic.ModeVideoGeneration:
		cost = calcVideoGenerationCost(entry, usage, tierName)
	case bfe_basic.ModeResponses:
		cost = calcResponsesCost(entry, usage, tierName)
	default:
		cost = calcChatCost(entry, usage, tierName)
	}
	if cost == 0 && !noBillableTokens(usage) {
		// issue #1414: a real observation that costs nothing is impossible
		// once the price entry was found, so the entry exists but every billed
		// dimension is priced 0 (an unconfigured image/video price key, or a
		// rate configured as 0 — the config check only rejects negatives).
		aiMeta.SetZeroCostReason(bfe_basic.ZeroCostReasonPriceZero)
	}
	return cost
}

func calcResponsesCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
	return calcChatCost(entry, usage, tierName)
}

func calcVideoGenerationCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
	// Billing method is entry-driven (issue #1420): a token-priced
	// video_generation entry (token unit prices but no output_cost_per_video)
	// bills by the token formula; mode alone must not pick the formula.
	if !entry.HasPrice(tierName, cluster_conf.PriceOutputCostPerVideo) {
		return calcChatCost(entry, usage, tierName)
	}

	videoCount := usage.VideoCount
	if videoCount < 0 {
		videoCount = 0
	}

	// Prices stay float64 (yuan per unit); quota.CalcCostUnits converts each
	// billing item to a fixed-point integer with rounding, so prices with
	// more than 8 decimal places keep their precision.
	return quota.CalcCostUnits(videoCount, entry.GetPrice(tierName, cluster_conf.PriceOutputCostPerVideo))
}

func calcImageGenerationCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
	// Billing method is entry-driven (issue #1420): a token-priced
	// image_generation entry (token unit prices but no output_cost_per_image)
	// bills by the token formula; mode alone must not pick the formula. The
	// per-image key is checked for presence, not value, so an explicit
	// output_cost_per_image = 0 stays per-image billing at zero cost.
	if !entry.HasPrice(tierName, cluster_conf.PriceOutputCostPerImage) {
		return calcChatCost(entry, usage, tierName)
	}

	imageCount := usage.ImageCount
	if imageCount < 0 {
		imageCount = 0
	}

	cost := quota.CalcCostUnits(imageCount, entry.GetPrice(tierName, cluster_conf.PriceOutputCostPerImage))
	cost += quota.CalcCostUnits(usage.ImageInputTokens, entry.GetPrice(tierName, cluster_conf.PriceInputCostPerImageToken))
	return cost
}

func calcChatCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
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
	if audioInputTokens > promptTokens-cacheReadTokens-cacheWriteTokens {
		audioInputTokens = promptTokens - cacheReadTokens - cacheWriteTokens
		if audioInputTokens < 0 {
			audioInputTokens = 0
		}
	}
	if audioOutputTokens < 0 {
		audioOutputTokens = 0
	}
	if audioOutputTokens > completionTokens {
		audioOutputTokens = completionTokens
	}

	cacheReadPrice := entry.GetPrice(tierName, cluster_conf.PriceCacheReadInputTokenCost)
	cacheWritePrice := entry.GetPrice(tierName, cluster_conf.PriceCacheCreationInputTokenCost)
	cacheWritePrice1h := entry.GetPrice(tierName, cluster_conf.PriceCacheCreationInputTokenCost1h)
	audioInputPrice := entry.GetPrice(tierName, cluster_conf.PriceInputCostPerAudioToken)
	audioOutputPrice := entry.GetPrice(tierName, cluster_conf.PriceOutputCostPerAudioToken)
	imageInputPrice := entry.GetPrice(tierName, cluster_conf.PriceInputCostPerImageToken)

	// Length-tier billing: pick input/output price by the total input token
	// count (promptTokens, including cache read/write); exceeding a tier
	// threshold bills the whole request at that tier. A tier side whose key
	// is not configured (-1) keeps the base price on that side. With no tier
	// key configured (ok=false) the base prices are used, exactly as before.
	inputPrice := entry.GetPrice(tierName, cluster_conf.PriceInputCostPerToken)
	outputPrice := entry.GetPrice(tierName, cluster_conf.PriceOutputCostPerToken)
	if tierIn, tierOut, ok := entry.GetLengthTierPrice(tierName, promptTokens); ok {
		if tierIn >= 0 {
			inputPrice = tierIn
		}
		if tierOut >= 0 {
			outputPrice = tierOut
		}
	}

	// normal input/output start as the full totals
	normalInput := promptTokens
	normalOutput := completionTokens

	// cache-aware billing: split cache read/write from prompt.
	// PromptTokens is the total input (for Anthropic it is normalized to
	// input_tokens + cache read + cache write at parse time), so both cache
	// parts must be removed to get the normal (fresh) input tokens.
	if cacheReadPrice > 0 || cacheWritePrice > 0 {
		normalInput = promptTokens - cacheReadTokens - cacheWriteTokens
		if normalInput < 0 {
			normalInput = 0
		}
	}

	// image-aware billing: split image input from normal input
	imageInputTokens := usage.ImageInputTokens
	if imageInputPrice > 0 {
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
	if audioInputPrice > 0 {
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
	if audioOutputPrice > 0 {
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

	// 1h-TTL cache write split: bill the 1h portion at its own price, the
	// remainder at the 5m (base cache_creation) price. This runs after the
	// normal-input split above so the full cache write amount is removed
	// from the normal input exactly as before; the 1h portion is then moved
	// from the 5m remainder to its own billing item.
	cacheWriteTokens1h := usage.CacheWriteTokens1h
	if cacheWriteTokens1h < 0 {
		cacheWriteTokens1h = 0
	}
	if cacheWritePrice1h > 0 {
		if cacheWriteTokens1h > cacheWriteTokens {
			cacheWriteTokens1h = cacheWriteTokens
		}
		cacheWriteTokens -= cacheWriteTokens1h
	} else {
		// no 1h price configured: bill all cache writes at the base price
		cacheWriteTokens1h = 0
	}

	// Each billing item is converted to a fixed-point integer separately and
	// the integers are summed, so floating point only participates in single
	// multiplications well below 2^53. Items with an unconfigured price (0)
	// contribute nothing, which preserves the legacy fallback semantics of
	// billing unconfigured sub-token usage at the normal input/output price.
	cost := quota.CalcCostUnits(normalInput, inputPrice)
	cost += quota.CalcCostUnits(cacheReadTokens, cacheReadPrice)
	cost += quota.CalcCostUnits(cacheWriteTokens, cacheWritePrice)
	cost += quota.CalcCostUnits(cacheWriteTokens1h, cacheWritePrice1h)
	cost += quota.CalcCostUnits(audioInputTokens, audioInputPrice)
	cost += quota.CalcCostUnits(imageInputTokens, imageInputPrice)
	cost += quota.CalcCostUnits(normalOutput, outputPrice)
	cost += quota.CalcCostUnits(audioOutputTokens, audioOutputPrice)

	return cost
}
