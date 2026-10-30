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

package bfe_basic

import (
	"encoding/json"
	"io/ioutil"
	"strings"

	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_model_protocol"
)

const (
	REQ_AI_BASIC_CONTEXT  = "__REQ_AI_BASIC_CONTEXT"
	REQ_AI_RATE_LIMIT_HIT = "__REQ_AI_RATE_LIMIT_HIT"
)

const (
	COMPLETION_TOKENS_UNKNOWN = -1
)

// AI request modes, inferred from request path.
const (
	ModeChat               = "chat"
	ModeCompletion         = "completion"
	ModeImageGeneration    = "image_generation"
	ModeImageEdit          = "image_edit"
	ModeEmbedding          = "embedding"
	ModeAudioSpeech        = "audio_speech"
	ModeAudioTranscription = "audio_transcription"
	ModeRerank             = "rerank"
	ModeVideoGeneration    = "video_generation"
	ModeOcr                = "ocr"
	ModeSearch             = "search"
	ModeRealtime           = "realtime"
	ModeResponses          = "responses"
	ModeFile               = "file"
	ModeBatch              = "batch"
)

// Batch operation types (mod_ai_batch), classified from (method, path).
const (
	BatchOpUpload   = "upload"
	BatchOpCreate   = "create"
	BatchOpGet      = "get"
	BatchOpList     = "list"
	BatchOpCancel   = "cancel"
	BatchOpDownload = "download"
)

// Batch quota settlement markers written by mod_ai_batch into AiBasicInfo;
// consumed by mod_ai_token_auth at HandleRequestFinish.
const (
	BatchSettleNone    = "none"
	BatchSettleSettle  = "settle"  // settled by result-file usage at download
	BatchSettleRelease = "release" // reserve released without settlement
)

// AI protocol/auth styles.
const (
	AuthStyleOpenAI    = "openai"
	AuthStyleAnthropic = "anthropic"
	AuthStyleGemini    = "gemini"
	AuthStyleUnknown   = "unknown"
)

type TokenUsage struct {
	PromptTokens       int64 // number of tokens in the prompt (includes cache_read_tokens, audio_input_tokens, image_input_tokens)
	CompletionTokens   int64 // number of tokens in the completion (includes audio_output_tokens)
	CacheReadTokens    int64 // usage.cache_read_tokens, already included in PromptTokens
	CacheWriteTokens   int64 // usage.cache_write_tokens, already included in PromptTokens (normalized for Anthropic)
	CacheWriteTokens1h int64 // 1h-TTL cache write tokens (usage.cache_creation.ephemeral_1h_input_tokens), already included in CacheWriteTokens
	AudioInputTokens   int64 // usage.audio_input_tokens, already included in PromptTokens
	AudioOutputTokens  int64 // usage.audio_output_tokens, already included in CompletionTokens
	ImageInputTokens   int64 // usage.image_input_tokens / input_token_details.image_tokens, already included in PromptTokens
	VideoCount         int64 // number of generated videos for video generation models
	ImageCount         int64 // number of generated images for image generation models
	UsedQuota          int64 // used quota for this request (unit=total_token)
	UsedCost           int64 // used RMB cost for this request, 1 unit = 1e-8 yuan (unit=RMB)
}

type TokenTimeInfo struct {
	TReqEnd     int64 // Unix microsecond timestamp when the gateway finishes receiving the request body
	TFirstToken int64 // Unix microsecond timestamp when the gateway receives the first chunk with valid generated text
	TLastToken  int64 // Unix microsecond timestamp when the last token is read
	TTFT        int64 // Time to first token (microseconds), TFirstToken - TReqEnd
	TPOT        int64 // Average time per output token (microseconds), (TLastToken - TFirstToken) / (CompletionTokens - 1)
}

type ApikeyTag struct {
	TagName  string //eg entity.type
	TagValue string //eg entity.name
	TagLevel int    // entity type level, value range 1~5
}

type AiAuthInfo struct {
	RejectReason     string   // reason for rejection
	RejectQuotaPlans []string // quota plan IDs rejected due to insufficient quota
	HitQuotaPlans    []string // quota plan IDs hit (passed balance check) for successful requests
}

type AiBasicInfo struct {
	ClientApiKey    string
	ClientKeyId     string
	ClientModel     string
	TargetModel     string
	Mode            string // request mode, e.g. chat, image_generation
	Provider        string // upstream model provider, e.g. openai, deepseek
	AuthStyle       string // request protocol/auth style, e.g. openai, anthropic
	RetryCount      uint32 // model invocation retry count (key-level retry)
	CostCurrency    string // cost currency, e.g. RMB, USD
	tokenUsage      TokenUsage
	ApikeyTags      []ApikeyTag
	TokenTimeInfo   TokenTimeInfo
	AiAuthInfo      AiAuthInfo
	ClusterKeyNames []ClusterKeyName // tried (cluster, key) pairs during request processing

	// AI cache (mod_ai_cache) result
	AiCacheHit        bool    // true when the response was served from the AI cache
	AiCacheStatus     string  // cache status: hit / hit_semantic / miss / skip, empty if cache not enabled
	AiCacheKey        string  // cache key, only filled when mod_ai_cache debug is on
	AiCacheSemantic   bool    // true when the hit comes from the semantic (vector) cache
	AiCacheSimilarity float64 // normalized similarity [0,1] of a semantic cache hit, 0 if not applicable

	// AI context compress (mod_ai_context) result
	ContextCompressStatus string // "" = not processed; skip_* / trim / rewrite / repair_rollback (also the fallback idempotency guard)
	ContextTokensBefore   int64  // estimated tokens before context compression (heuristic estimate)
	ContextTokensAfter    int64  // estimated tokens after context compression, 0 if not compressed
	ContextCompressMode   string // effective mode: conservative / balanced / aggressive

	// Traffic mirroring (mod_traffic_mirror) result: only synchronous fields
	// are recorded here; async mirror results (status/usage/latency) are
	// reported via module Prometheus metrics and are not written back
	MirrorHit     bool   // true when the request was selected for mirroring
	MirrorCluster string // mirror target cluster name, empty if not mirrored

	// Upstream error normalization (AIConf.NormalizeUpstreamError) result;
	// all fields stay at zero values when normalization is disabled or the
	// response is not an upstream error
	UpstreamStatus       int32  // original upstream HTTP status, 0 = not recorded
	UpstreamErrCode      string // original upstream error code (redacted)
	ErrNormalized        bool   // true when the upstream error was rewritten into the unified error body
	ErrNormalizeMiss     bool   // true when the parser did not recognize the upstream error body
	StreamErrorRewritten bool   // true when at least one SSE error event of the stream was rewritten
	StreamTruncated      bool   // true when the stream ended without the protocol's terminal event
	// UpstreamKey holds the last injected upstream credential; it is the
	// pattern source for error-body credential redaction and must never be
	// written to logs or responses.
	UpstreamKey string

	// Batch task (mod_ai_batch) context: filled on files/batches operations.
	// BatchSettle carries the quota settlement contract consumed by
	// mod_ai_token_auth: "settle" (price the parsed usage at batch price and
	// release the reserve), "release" (release the reserve only) or "none".
	BatchId       string // batch_xxx, empty for non-batch traffic
	BatchFileId   string // file-xxx, empty when unknown
	BatchOp       string // BatchOp*: upload/create/get/list/cancel/download
	BatchLines    int64  // jsonl line count (upload counting / output parsing)
	BatchBytes    int64  // file bytes (upload counting / provider response)
	BatchStatus   string // provider status snapshot: validating/.../cancelling
	BatchSettle   string // BatchSettle*: none/settle/release
	BatchSettleId string // idempotency key for settle/release (batch_id)

	// Batch effective file limits, computed by mod_ai_rate_limit as
	// min(bound policies' batch_limits, mod_ai_batch.data global hard
	// ceiling); consumed by mod_ai_batch pre-forward checks. 0 = unlimited.
	BatchEffMaxFileBytes int64
	BatchEffMaxFileLines int64

	// BatchUsageByModel carries per-model usage aggregated from a batch
	// result file, keyed by target model; consumed by mod_ai_token_auth to
	// price the settlement at mode=batch rates per model group.
	BatchUsageByModel map[string]TokenUsage

	// BatchSettleApplied reports whether this request actually executed the
	// settle/release contract: mod_ai_token_auth sets it once the
	// BATCH_SETTLED claim is won and the deduction/release ran. mod_ai_batch
	// persists the settlement fields into BATCH_TASK only when it is true, so
	// a batch already settled/released by another path (control-plane
	// reconcile backstop, concurrent download) is never re-marked.
	BatchSettleApplied bool
	// BatchSettledUnits carries the RMB amount (1e-8 fixed-point units)
	// actually charged by the settlement, published alongside
	// BatchSettleApplied so mod_ai_batch can persist BATCH_TASK.settle_units.
	// Zero is a legitimate value (price miss, no usage) and is still written.
	BatchSettledUnits int64

	allowEstimateToken bool

	// responseCompleted marks whether the upstream response finished
	// normally: the termination event was seen for streaming responses
	// (e.g. Anthropic message_stop, OpenAI [DONE]) or the full body was
	// read for non-streaming responses.
	responseCompleted bool
	// upstreamDropped marks that the upstream connection broke before the
	// response body was fully delivered (issue #1474): a mid-stream read
	// error, as opposed to a client abort (write side) or a clean EOF.
	// Billing treats the forwarded-content estimate as billable on this
	// mark, and the stream filter injects a protocol error event into the
	// client stream.
	upstreamDropped bool
	// finalUsageSeen marks whether the final usage was parsed from the
	// response. An initial usage such as Anthropic message_start
	// (output_tokens = 0) does not count.
	finalUsageSeen bool

	// priceMiss marks that the (model, mode) price entry was absent from the
	// cluster price table and the cost was computed by falling back to the
	// model's chat-mode price (or billed 0 when no fallback price exists).
	// Surfaced to the access log as ai_price_miss so under-priced rows stay
	// visible in reconciliation (issue #1408).
	priceMiss bool

	// usageDisposition tells the access log whether the token figures of
	// this request may be reported (issue #1409). It is decided at request
	// finish by mod_ai_token_auth; the zero value means "not judged"
	// (non-token-auth traffic) and keeps the legacy behaviour.
	usageDisposition string

	// usageSource tells the access log where the reported token usage came
	// from: parsed (final usage from the upstream response, including a
	// genuine zero-usage object), estimated (EstimateToken estimate approved
	// for billing), aborted (client disconnected before the final usage,
	// issue #1414) or unrecognized (no final usage and not estimatable:
	// billing skipped, issue #1410). It is set at the same decision point as
	// the billing-yardstick guard; the zero value means "not judged" and the
	// log field stays absent.
	usageSource string

	// zeroCostReason tells the access log why a 2xx request that holds an RMB
	// quota plan was charged 0 (issue #1414), reported as ai_zero_cost_reason.
	// It is marked at the site that produced the zero cost (unrecognized
	// usage guard, client abort, cache hit, batch release, pricing gaps) and
	// read back at the single billing checkpoint, which also counts
	// BillingZeroCost. Empty means "not a zero-cost request" or "not judged".
	zeroCostReason string
}

// ClusterKeyName represents a tried cluster and API-Key pair during request processing
type ClusterKeyName struct {
	ClusterName string
	KeyName     string
}

func (aiinfo *AiBasicInfo) GetTokenUsage() *TokenUsage {
	return &aiinfo.tokenUsage
}

func (aiinfo *AiBasicInfo) SetAllowEstimateToken(allow bool) {
	aiinfo.allowEstimateToken = allow
}

func (aiinfo *AiBasicInfo) IsAllowEstimateToken() bool {
	return aiinfo.allowEstimateToken
}

func (aiinfo *AiBasicInfo) MarkResponseCompleted() {
	aiinfo.responseCompleted = true
}

func (aiinfo *AiBasicInfo) IsResponseCompleted() bool {
	return aiinfo.responseCompleted
}

// MarkUpstreamDropped records that the upstream connection broke before
// the response body was fully delivered (issue #1474).
func (aiinfo *AiBasicInfo) MarkUpstreamDropped() {
	aiinfo.upstreamDropped = true
}

// IsUpstreamDropped reports whether MarkUpstreamDropped was recorded.
func (aiinfo *AiBasicInfo) IsUpstreamDropped() bool {
	return aiinfo.upstreamDropped
}

func (aiinfo *AiBasicInfo) MarkFinalUsageSeen() {
	aiinfo.finalUsageSeen = true
}

func (aiinfo *AiBasicInfo) IsFinalUsageSeen() bool {
	return aiinfo.finalUsageSeen
}

// MarkPriceMiss records that the request's (model, mode) price entry was
// absent and its cost was computed with a fallback (or billed 0); the access
// log reports it as ai_price_miss (issue #1408).
func (aiinfo *AiBasicInfo) MarkPriceMiss() {
	aiinfo.priceMiss = true
}

// IsPriceMiss reports whether MarkPriceMiss was recorded for this request.
func (aiinfo *AiBasicInfo) IsPriceMiss() bool {
	return aiinfo.priceMiss
}

// Token-usage dispositions for the access log (issue #1409).
const (
	// UsageDispositionUnset is the zero value: no disposition was judged
	// (the request did not go through token authentication, or the module
	// was not loaded). The access-log fields keep their legacy behaviour.
	UsageDispositionUnset = ""
	// UsageDispositionNone means the request never successfully called the
	// upstream (non-2xx response), so no model usage can exist: the access
	// log must not report ai_input_tokens / ai_output_tokens /
	// ai_total_tokens, and any auth-time or response-time estimate is
	// residue, not usage.
	UsageDispositionNone = "none"
	// UsageDispositionObserved means the upstream was successfully called
	// and the shared TokenUsage holds the real parsed usage or the
	// EstimateToken estimate of this request (issue #1398 keeps that view
	// observable); the access log reports it as-is.
	UsageDispositionObserved = "observed"
)

// SetUsageDisposition records whether the token usage of this request may
// be reported by the access log (issue #1409).
func (aiinfo *AiBasicInfo) SetUsageDisposition(disposition string) {
	aiinfo.usageDisposition = disposition
}

// UsageDisposition returns the token-usage disposition set at request
// finish (UsageDispositionUnset when it was never judged).
func (aiinfo *AiBasicInfo) UsageDisposition() string {
	return aiinfo.usageDisposition
}

// Usage sources for the access log (issue #1410): where the reported token
// usage came from. Orthogonal to UsageDisposition, which decides whether
// usage is reported at all; the source is set at the same decision point
// as the billing-yardstick guard so the signal can never diverge from the
// billing call.
const (
	// UsageSourceParsed means a final usage was parsed from the upstream
	// response, decided by the presence of the usage object rather than by
	// its values (issue #1414): an upstream that explicitly reports all-zero
	// usage also lands here and is billed at 0, instead of being estimated.
	UsageSourceParsed = "parsed"
	// UsageSourceEstimated means the upstream returned no usage at all but the
	// request is billable by EstimateToken (estimate allowed and response
	// completed); the approved estimate is mirrored into the shared usage
	// (issue #1398).
	UsageSourceEstimated = "estimated"
	// UsageSourceAborted means the client disconnected (RST/close/write
	// failure) before the final usage was seen: billing is skipped per the
	// issue #1352 yardstick, and the reported token fields are either
	// auth-time seeding residue or the initial usage of the stream
	// (issue #1414).
	UsageSourceAborted = "aborted"
	// UsageSourceUnrecognized means no final usage was parsed and the
	// request is not estimatable: billing was skipped and the reported
	// token fields are auth-time seeding residue.
	UsageSourceUnrecognized = "unrecognized"
	// UsageSourceUpstreamDropped means the upstream connection broke
	// mid-response before the final usage was seen (issue #1474). Unlike a
	// client abort, content already forwarded to the client is real
	// consumption: the request is billed by the forwarded-content estimate
	// when EstimateToken allows, or reported unrecognized otherwise.
	UsageSourceUpstreamDropped = "upstream_dropped"
)

// Zero-cost reasons for the access log (issue #1414): why a 2xx request that
// holds an RMB quota plan was charged 0. Reported as ai_zero_cost_reason and
// counted by the BillingZeroCost module counter (AI cache hits are marked but
// not counted: they never call the upstream, so their zero cost is by design).
const (
	// ZeroCostReasonUnrecognized: upstream usage was neither recognized nor
	// estimatable.
	ZeroCostReasonUnrecognized = "unrecognized"
	// ZeroCostReasonUpstreamDropped: the upstream connection broke
	// mid-response and EstimateToken is off, so the forwarded content could
	// not be billed (issue #1474).
	ZeroCostReasonUpstreamDropped = "upstream_dropped"
	// ZeroCostReasonAborted: the client disconnected before the final usage.
	ZeroCostReasonAborted = "aborted"
	// ZeroCostReasonCacheHit: AI cache hit, the upstream was never called.
	ZeroCostReasonCacheHit = "cache_hit"
	// ZeroCostReasonBatchRelease: batch contract released without billing.
	ZeroCostReasonBatchRelease = "batch_release"
	// ZeroCostReasonPriceMiss: no price entry and no fallback price.
	ZeroCostReasonPriceMiss = "price_miss"
	// ZeroCostReasonPriceZero: the price entry exists but the billed
	// dimensions are priced 0.
	ZeroCostReasonPriceZero = "price_zero"
	// ZeroCostReasonModelTableMissing: the cluster model table is missing or
	// unlookupable.
	ZeroCostReasonModelTableMissing = "model_table_missing"
	// ZeroCostReasonConfigMissing: pricing configuration (server data conf /
	// cluster / target model) is unavailable at billing time.
	ZeroCostReasonConfigMissing = "config_missing"
	// ZeroCostReasonNoUsageTokens: no billable token count, although no usage
	// problem was detected (e.g. a genuine zero-usage response, or image /
	// video modes with a zero generated count).
	ZeroCostReasonNoUsageTokens = "no_usage_tokens"
	// ZeroCostReasonCostZero: fallback for a zero cost with no classified
	// cause.
	ZeroCostReasonCostZero = "cost_zero"
)

// SetUsageSource records where the reported token usage came from (issue
// #1410); the access log reports it as ai_usage_source.
func (aiinfo *AiBasicInfo) SetUsageSource(source string) {
	aiinfo.usageSource = source
}

// UsageSource returns the usage source set at request finish (empty when
// it was never judged, e.g. the upstream call never succeeded).
func (aiinfo *AiBasicInfo) UsageSource() string {
	return aiinfo.usageSource
}

// SetZeroCostReason records why a zero-cost request was charged 0 (issue
// #1414); the access log reports it as ai_zero_cost_reason.
func (aiinfo *AiBasicInfo) SetZeroCostReason(reason string) {
	aiinfo.zeroCostReason = reason
}

// ZeroCostReason returns the zero-cost reason (empty when the request was
// billed, when no RMB plan was bound, or when nothing judged it).
func (aiinfo *AiBasicInfo) ZeroCostReason() string {
	return aiinfo.zeroCostReason
}

func GetApiKey(req *Request) string {
	protocol, key := bfe_model_protocol.DetectProtocolAndKey(req.HttpRequest)
	// preserve the legacy side effect: AuthStyle is set only when a
	// credential header was found (Authorization preferred over x-api-key)
	if protocol != "" {
		if ai := req.GetAiBasicInfo(); ai != nil {
			ai.AuthStyle = protocol
		}
	}
	return key
}

// DetectAuthStyle infers the AI protocol/auth style from request characteristics.
// It is purely based on request path/headers and does not rely on user routing rules.
func DetectAuthStyle(req *Request) string {
	if req == nil || req.HttpRequest == nil {
		return AuthStyleUnknown
	}

	return bfe_model_protocol.DetectProtocol(req.HttpRequest)
}

// DetectModeFromPath infers the AI request mode from the request path.
// It defaults to ModeChat for unknown paths to keep backward compatibility.
//
// OpenAI standard endpoints are recognized with or without the /v1 version
// prefix: both /v1/embeddings and /embeddings map to ModeEmbedding, so the
// billing mode never depends on whether the client entry carries /v1
// (consistent with the upstream path rewrite, issue #1379 follow-up).
// Provider-native entries whose prefix ends in a /v1 segment (the OpenAI
// SDK base_url form, e.g. /compatible-mode/v1/responses) reduce to the same
// endpoints (issue #1382).
func DetectModeFromPath(path string) string {
	if mode, ok := lookupOpenAIEndpointMode(normalizeEndpointLookupPath(path)); ok {
		return mode
	}
	return ModeChat
}

// Set user context by key and val.
func (r *Request) InitAiBasicInfo() *AiBasicInfo {
	ret := &AiBasicInfo{}
	ret.tokenUsage.CompletionTokens = COMPLETION_TOKENS_UNKNOWN

	r.SetContext(REQ_AI_BASIC_CONTEXT, ret)
	return ret
}

func (aiinfo *AiBasicInfo) AppendClusterKeyName(clusterName, keyName string) {
	aiinfo.ClusterKeyNames = append(aiinfo.ClusterKeyNames, ClusterKeyName{
		ClusterName: clusterName,
		KeyName:     keyName,
	})
}

func (aiinfo *AiBasicInfo) IncrementRetryCount() {
	aiinfo.RetryCount++
}

// Get user context by key.
func (r *Request) GetAiBasicInfo() *AiBasicInfo {
	ctx := r.GetContext(REQ_AI_BASIC_CONTEXT)
	if ctx == nil {
		return nil
	}

	aiCtx, ok := ctx.(*AiBasicInfo)
	if !ok {
		return nil
	}
	return aiCtx
}

type HitPolicyInfo struct {
	TpmRules      []string `json:"tpm_rules"` //name of limiter
	RpmRules      []string `json:"rpm_rules"` //name of limiter
	IsConcurrency bool     `json:"is_concurrency"`
	IsRedisError  string   `json:"is_redis_error"`
}

type AiRateLimitHitInfo struct {
	HitPolicyDict map[string]*HitPolicyInfo `json:"hit_rate_limit_policys"` //limite policy id -> HitPolicyInfo
}

func (obj *AiRateLimitHitInfo) GetPolicyHitInfo(policyId string) *HitPolicyInfo {
	if obj.HitPolicyDict == nil {
		obj.HitPolicyDict = make(map[string]*HitPolicyInfo)
	}
	item, ok := obj.HitPolicyDict[policyId]
	if !ok {
		item = &HitPolicyInfo{}
		obj.HitPolicyDict[policyId] = item
	}
	return item
}

func (r *Request) InitAiRateLimitHitInfo() *AiRateLimitHitInfo {
	//NOTE: should be called at the beginning of the rate limit module
	ret := &AiRateLimitHitInfo{}
	r.SetContext(REQ_AI_RATE_LIMIT_HIT, ret)
	return ret
}

func (r *Request) GetAiRateLimitHitInfo() *AiRateLimitHitInfo {
	ctx := r.GetContext(REQ_AI_RATE_LIMIT_HIT)
	if ctx == nil {
		return nil
	}
	info, ok := ctx.(*AiRateLimitHitInfo)
	if !ok {
		return nil
	}
	return info
}

func (r *Request) GetApikeyTags() []ApikeyTag {
	aiInfo := r.GetAiBasicInfo()
	if aiInfo == nil {
		return nil
	}
	return aiInfo.ApikeyTags
}

const (
	CodeInvalidRequest   = "INVALID_REQUEST"
	CodeNoApiKey         = "NO_API_KEY"
	CodeInvalidApiKey    = "INVALID_API_KEY"
	CodeKeyDisabled      = "KEY_DISABLED"
	CodeKeyExpired       = "KEY_EXPIRED"
	CodeSubnetNotAllowed = "SUBNET_NOT_ALLOWED"
	CodeModelNotAllowed  = "MODEL_NOT_ALLOWED"

	CodeRpmLimitExceeded         = "RPM_LIMIT_EXCEEDED"
	CodeTpmLimitExceeded         = "TPM_LIMIT_EXCEEDED"
	CodeConcurrencyLimitExceeded = "CONCURRENCY_LIMIT_EXCEEDED"
	CodeRateLimitRedisError      = "RATE_LIMIT_REDIS_ERROR"

	CodeQuotaExhausted       = "QUOTA_EXHAUSTED"
	CodeQuotaExpired         = "QUOTA_EXPIRED"
	CodeQuotaPackageDisabled = "QUOTA_PACKAGE_DISABLED"
	CodeQuotaModelMismatch   = "QUOTA_MODEL_MISMATCH"
	CodeQuotaPlanFailed      = "QUOTA_PLAN_FAILED"
	CodeInternalQuotaError   = "INTERNAL_QUOTA_ERROR"

	CodeContextLengthExceeded = "CONTEXT_LENGTH_EXCEEDED"
	CodeContentFiltered       = "CONTENT_FILTERED"
	CodeModelInternalError    = "MODEL_INTERNAL_ERROR"
	CodeBackendTimeout        = "BACKEND_TIMEOUT"

	// Batch (mod_ai_batch) limits: file size/line hard ceilings, resolved
	// from min(bound policies' batch_limits, mod_ai_batch.data global caps)
	CodeBatchFileTooLarge = "BATCH_FILE_TOO_LARGE"
	// Batch file ownership check failed (file_id bound to another apikey)
	CodeBatchFileForbidden = "BATCH_FILE_FORBIDDEN"

	CodeProviderProtocolMismatch = "PROVIDER_PROTOCOL_MISMATCH"

	// Gateway-side capacity exhaustion (issue #1441): no usable upstream API
	// key could be selected for the target cluster (all penalized / weight 0
	// / dead). Distinct from CodeUpstreamRateLimited (an upstream-returned
	// rate limit, normalized by AIConf.NormalizeUpstreamError) and from
	// RPM/TPM (client quota).
	CodeNoAvailableKey = "NO_AVAILABLE_KEY"

	// Upstream error normalization catalog (AIConf.NormalizeUpstreamError).
	// Values are single-sourced in bfe_model_protocol/utils and shared with
	// the protocol adapters' mapping tables.
	CodeUpstreamInvalidRequest = bfe_model_protocol.CodeUpstreamInvalidRequest
	CodeUpstreamRateLimited    = bfe_model_protocol.CodeUpstreamRateLimited
	CodeUpstreamQuotaExhausted = bfe_model_protocol.CodeUpstreamQuotaExhausted
	CodeUpstreamAuthError      = bfe_model_protocol.CodeUpstreamAuthError
	CodeUpstreamModelNotFound  = bfe_model_protocol.CodeUpstreamModelNotFound
	CodeUpstreamOverloaded     = bfe_model_protocol.CodeUpstreamOverloaded
	CodeUpstreamUnknown        = bfe_model_protocol.CodeUpstreamUnknown

	CodeConfigLoadError    = "CONFIG_LOAD_ERROR"
	CodeBackendUnavailable = "BACKEND_UNAVAILABLE"
	// Gateway-side capacity exhaustion with a legitimately empty instance
	// pool (issue #1453): the balance layer already reported BK_NO_BACKEND,
	// so the response keeps that business code instead of collapsing it into
	// BACKEND_UNAVAILABLE. Clients can then tell "no capacity" apart from
	// "backend failure".
	CodeNoBackend          = "BK_NO_BACKEND"
	CodeInvalidRequestBody = "INVALID_REQUEST_BODY"
	CodeModelParamMissing  = "MODEL_PARAM_MISSING"

	CodeUserQuotaExhausted     = "USER_QUOTA_EXHAUSTED"
	CodeSessionQuotaExhausted  = "SESSION_QUOTA_EXHAUSTED"
	CodeFunctionQuotaExhausted = "FUNCTION_QUOTA_EXHAUSTED"
	CodeCostBudgetExhausted    = "COST_BUDGET_EXHAUSTED"
	CodeGeoRestricted          = "GEO_RESTRICTED"
	CodeTimeWindowRestricted   = "TIME_WINDOW_RESTRICTED"

	// Gateway-side content audit rejection (SC35 TC-02): a request-phase
	// body-process REJECT is a deterministic policy rejection, not a backend
	// fault, so it keeps the HTTP 500 business contract with its own code
	// instead of collapsing into BACKEND_UNAVAILABLE (502) — the same
	// treatment shape as BK_NO_BACKEND (issue #1453).
	CodeContentRejected = "CONTENT_REJECTED"
)

const (
	TypeAuthenticationError = "authentication_error"
	TypeInvalidRequestError = "invalid_request_error"
	TypeRateLimitError      = "rate_limit_error"
	TypeQuotaError          = "quota_error"
	TypeInternalError       = "internal_error"
)

var ErrorCodeToStatusCode = map[string]int{
	CodeInvalidRequest:   400,
	CodeNoApiKey:         401,
	CodeInvalidApiKey:    401,
	CodeKeyDisabled:      403,
	CodeKeyExpired:       403,
	CodeSubnetNotAllowed: 403,
	CodeModelNotAllowed:  400,

	CodeRpmLimitExceeded:         429,
	CodeTpmLimitExceeded:         429,
	CodeConcurrencyLimitExceeded: 429,
	CodeRateLimitRedisError:      500,

	CodeQuotaExhausted:       429,
	CodeQuotaExpired:         429,
	CodeQuotaPackageDisabled: 429,
	CodeQuotaModelMismatch:   429,
	CodeQuotaPlanFailed:      429,
	CodeInternalQuotaError:   500,

	CodeContextLengthExceeded: 400,
	CodeContentFiltered:       400,
	CodeModelInternalError:    500,
	CodeBackendTimeout:        504,

	CodeBatchFileTooLarge:  413,
	CodeBatchFileForbidden: 404,

	CodeConfigLoadError:    500,
	CodeBackendUnavailable: 502,
	CodeNoBackend:          500,
	CodeContentRejected:    500,
	CodeInvalidRequestBody: 400,
	CodeModelParamMissing:  400,

	CodeUserQuotaExhausted:     429,
	CodeSessionQuotaExhausted:  429,
	CodeFunctionQuotaExhausted: 429,
	CodeCostBudgetExhausted:    429,
	CodeGeoRestricted:          403,
	CodeTimeWindowRestricted:   403,

	CodeProviderProtocolMismatch: 400,

	CodeUpstreamInvalidRequest: 400,
	CodeUpstreamRateLimited:    429,
	CodeUpstreamQuotaExhausted: 429,
	CodeUpstreamAuthError:      502,
	CodeUpstreamModelNotFound:  404,
	CodeUpstreamOverloaded:     503,
	CodeUpstreamUnknown:        502,

	CodeNoAvailableKey: 429,
}

// ErrorCodeToErrorType maps every catalog code to its error.type.
var ErrorCodeToErrorType = map[string]string{
	CodeInvalidRequest:   TypeInvalidRequestError,
	CodeNoApiKey:         TypeAuthenticationError,
	CodeInvalidApiKey:    TypeAuthenticationError,
	CodeKeyDisabled:      TypeAuthenticationError,
	CodeKeyExpired:       TypeAuthenticationError,
	CodeSubnetNotAllowed: TypeAuthenticationError,
	CodeModelNotAllowed:  TypeInvalidRequestError,

	CodeRpmLimitExceeded:         TypeRateLimitError,
	CodeTpmLimitExceeded:         TypeRateLimitError,
	CodeConcurrencyLimitExceeded: TypeRateLimitError,
	CodeRateLimitRedisError:      TypeRateLimitError,

	CodeQuotaExhausted:       TypeQuotaError,
	CodeQuotaExpired:         TypeQuotaError,
	CodeQuotaPackageDisabled: TypeQuotaError,
	CodeQuotaModelMismatch:   TypeQuotaError,
	CodeQuotaPlanFailed:      TypeQuotaError,
	CodeInternalQuotaError:   TypeInternalError,

	CodeContextLengthExceeded: TypeInvalidRequestError,
	CodeContentFiltered:       TypeInvalidRequestError,
	CodeModelInternalError:    TypeInternalError,
	CodeBackendTimeout:        TypeInternalError,

	CodeBatchFileTooLarge:  TypeRateLimitError,
	CodeBatchFileForbidden: TypeAuthenticationError,

	CodeConfigLoadError:    TypeInternalError,
	CodeBackendUnavailable: TypeInternalError,
	CodeNoBackend:          TypeInternalError,
	CodeContentRejected:    TypeInvalidRequestError,
	CodeInvalidRequestBody: TypeInvalidRequestError,
	CodeModelParamMissing:  TypeInvalidRequestError,

	CodeUserQuotaExhausted:     TypeQuotaError,
	CodeSessionQuotaExhausted:  TypeQuotaError,
	CodeFunctionQuotaExhausted: TypeQuotaError,
	CodeCostBudgetExhausted:    TypeQuotaError,
	CodeGeoRestricted:          TypeAuthenticationError,
	CodeTimeWindowRestricted:   TypeAuthenticationError,

	CodeProviderProtocolMismatch: TypeInvalidRequestError,

	CodeNoAvailableKey: TypeRateLimitError,

	CodeUpstreamInvalidRequest: TypeInvalidRequestError,
	CodeUpstreamRateLimited:    TypeRateLimitError,
	CodeUpstreamQuotaExhausted: TypeQuotaError,
	CodeUpstreamAuthError:      TypeInternalError,
	CodeUpstreamModelNotFound:  TypeInvalidRequestError,
	CodeUpstreamOverloaded:     TypeInternalError,
	CodeUpstreamUnknown:        TypeInternalError,
}

// ErrorTypeForCode returns the error.type for a catalog code, defaulting
// to internal_error for unknown codes.
func ErrorTypeForCode(code string) string {
	if t, ok := ErrorCodeToErrorType[code]; ok {
		return t
	}
	return TypeInternalError
}

const (
	LimitTypeApiKeyQuota = "api_key_quota"
	LimitTypeRpm         = "rpm"
	LimitTypeTpm         = "tpm"
	LimitTypeConcurrency = "concurrency"
	LimitTypeErrRedis    = "redis_access_error"
	// Batch limits (mod_ai_batch): file bytes/line ceilings
	LimitTypeBatchFile = "batch_file"
)

type AiErrorDetail struct {
	ApiKey            string `json:"api_key"`
	KeyId             string `json:"key_id"`
	QuotaPlanId       string `json:"quota_plan_id"`
	LimitType         string `json:"limit_type"`
	Model             string `json:"model"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
	// UpstreamStatus/UpstreamCode are set by upstream error normalization
	// (AIConf.NormalizeUpstreamError): the vendor's original HTTP status
	// and error code, for cross-checking vendor documentation.
	UpstreamStatus *int   `json:"upstream_status,omitempty"`
	UpstreamCode   string `json:"upstream_code,omitempty"`
}

// HeaderBfeGwError marks gateway-generated error responses. Upstream
// error normalization skips responses carrying this marker (they are
// already in the unified AiError format).
const HeaderBfeGwError = "X-Bfe-Gw-Error"

type AiError struct {
	Code    string         `json:"code"`
	Type    string         `json:"type"`
	Param   *string        `json:"param"`
	Message string         `json:"message"`
	Details *AiErrorDetail `json:"details"`
}
type AiErrorBody struct {
	Error AiError `json:"error"`
}

func NewAiError(code, typeStr, message string) *AiError {
	return &AiError{
		Code:    code,
		Type:    typeStr,
		Message: message,
	}
}

func NewAiErrorWithDetails(code, typeStr, message string, details *AiErrorDetail) *AiError {
	return &AiError{
		Code:    code,
		Type:    typeStr,
		Message: message,
		Details: details,
	}
}

func (aiError *AiError) Error() string {
	return aiError.Message
}

func (aiError *AiError) GenRespBody() *AiErrorBody {
	return &AiErrorBody{
		Error: *aiError,
	}
}

func (aiError *AiError) GenRespBodyStr() string {
	// generate AiErrorBody and marshal to json string
	respBody := aiError.GenRespBody()
	respBodyBytes, err := json.Marshal(respBody)
	if err != nil {
		return ""
	}
	return string(respBodyBytes)
}

func (aiError *AiError) CreateErrorResponse(request *Request) *bfe_http.Response {
	res := new(bfe_http.Response)
	res.StatusCode = ErrorCodeToStatusCode[aiError.Code]
	res.Header = make(bfe_http.Header)
	res.Header.Set("Server", "bfe")
	res.Header.Set("Content-Type", "application/json")
	res.Header.Set(HeaderBfeGwError, "1")
	// issue #1441 (round 2): every gateway-generated error response carries a
	// request correlation id (ai_error_codes.md §3.3), so clients can trace
	// the response back to the access log (logid) and the key selection /
	// penalty trail. CreateErrorResponse is the single common exit of all
	// gateway-generated AI errors, so the headers are written here rather
	// than at individual call sites.
	setCorrelationHeaders(request, res.Header)
	res.Body = ioutil.NopCloser(strings.NewReader(aiError.GenRespBodyStr()))
	request.HttpResponse = res
	return res
}

// setCorrelationHeaders writes the §3.3 correlation headers on a
// gateway-generated error response: X-Bfe-Log-Id is always set (fallback
// chain: req.LogId -> inbound X-Bfe-Log-Id header -> generated, back-filling
// req.LogId and the request header so the access log carries the same id),
// and a client-supplied X-Request-ID is echoed back byte-identical.
func setCorrelationHeaders(request *Request, header bfe_http.Header) {
	if request.HttpRequest == nil {
		return
	}
	logId := request.LogId
	if logId == "" {
		logId = request.HttpRequest.Header.Get(HeaderBfeLogId)
	}
	if logId == "" {
		logId = GenLogId()
	}
	if logId != "" {
		header.Set(HeaderBfeLogId, logId)
		if request.LogId == "" {
			request.LogId = logId
			request.HttpRequest.Header.Set(HeaderBfeLogId, logId)
		}
	}
	if reqId := request.HttpRequest.Header.Get(HeaderXRequestId); reqId != "" {
		header.Set(HeaderXRequestId, reqId)
	}
}
