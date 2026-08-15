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
)

const (
	REQ_AI_BASIC_CONTEXT  = "__REQ_AI_BASIC_CONTEXT"
	REQ_AI_RATE_LIMIT_HIT = "__REQ_AI_RATE_LIMIT_HIT"
)

const (
	COMPLETION_TOKENS_UNKNOWN = -1
)

type TokenUsage struct {
	PromptTokens     int64 // number of tokens in the prompt
	CompletionTokens int64 // number of tokens in the completion
	UsedQuota        int64 // used quota for this request (unit=total_token)
	UsedCost         int64 // used RMB cost for this request, 1 unit = 1e-8 yuan (unit=RMB)
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
}

type AiAuthInfo struct {
	RejectReason     string   // reason for rejection
	RejectQuotaPlans []string // quota plan IDs rejected due to insufficient quota
}

type AiBasicInfo struct {
	ClientApiKey  string
	ClientModel   string
	TargetModel   string
	tokenUsage    TokenUsage
	ApikeyTags    []ApikeyTag
	TokenTimeInfo TokenTimeInfo
	AiAuthInfo    AiAuthInfo

	allowEstimateToken bool
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

func GetApiKey(req *Request) string {
	// get api key from Authorization header
	authHeader := req.HttpRequest.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	// remove "Bearer " prefix if exists
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	authHeader = strings.TrimPrefix(authHeader, "sk-")

	return authHeader
}

// Set user context by key and val.
func (r *Request) InitAiBasicInfo() *AiBasicInfo {
	ret := &AiBasicInfo{}
	ret.tokenUsage.CompletionTokens = COMPLETION_TOKENS_UNKNOWN

	r.SetContext(REQ_AI_BASIC_CONTEXT, ret)
	return ret
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

	CodeConfigLoadError    = "CONFIG_LOAD_ERROR"
	CodeBackendUnavailable = "BACKEND_UNAVAILABLE"
	CodeInvalidRequestBody = "INVALID_REQUEST_BODY"
	CodeModelParamMissing  = "MODEL_PARAM_MISSING"

	CodeUserQuotaExhausted     = "USER_QUOTA_EXHAUSTED"
	CodeSessionQuotaExhausted  = "SESSION_QUOTA_EXHAUSTED"
	CodeFunctionQuotaExhausted = "FUNCTION_QUOTA_EXHAUSTED"
	CodeCostBudgetExhausted    = "COST_BUDGET_EXHAUSTED"
	CodeGeoRestricted          = "GEO_RESTRICTED"
	CodeTimeWindowRestricted   = "TIME_WINDOW_RESTRICTED"
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

	CodeConfigLoadError:    500,
	CodeBackendUnavailable: 502,
	CodeInvalidRequestBody: 400,
	CodeModelParamMissing:  400,

	CodeUserQuotaExhausted:     429,
	CodeSessionQuotaExhausted:  429,
	CodeFunctionQuotaExhausted: 429,
	CodeCostBudgetExhausted:    429,
	CodeGeoRestricted:          403,
	CodeTimeWindowRestricted:   403,
}

const (
	LimitTypeApiKeyQuota = "api_key_quota"
	LimitTypeRpm         = "rpm"
	LimitTypeTpm         = "tpm"
	LimitTypeConcurrency = "concurrency"
	LimitTypeErrRedis    = "redis_access_error"
)

type AiErrorDetail struct {
	ApiKey            string `json:"api_key"`
	QuotaPlanId       string `json:"quota_plan_id"`
	LimitType         string `json:"limit_type"`
	Model             string `json:"model"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
}
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
	res.Body = ioutil.NopCloser(strings.NewReader(aiError.GenRespBodyStr()))
	request.HttpResponse = res
	return res
}
