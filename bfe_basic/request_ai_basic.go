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

import "strings"

const (
	REQ_AI_BASIC_CONTEXT  = "__REQ_AI_BASIC_CONTEXT"
	REQ_AI_RATE_LIMIT_HIT = "__REQ_AI_RATE_LIMIT_HIT"
)

type TokenUsage struct {
	PromptTokens     int64 // number of tokens in the prompt
	CompletionTokens int64 // number of tokens in the completion
	UsedQuota        int64 // used quota for this request
}

type ApikeyTag struct {
	TagName   string   //eg entity.type
	TagValues []string //eg entity.name
}

type AiBasicInfo struct {
	ClientApiKey string
	ClientModel  string
	TargetModel  string
	tokenUsage   TokenUsage
	ApikeyTags   []ApikeyTag
}

func (aiinfo *AiBasicInfo) GetTokenUsage() *TokenUsage {
	return &aiinfo.tokenUsage
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
	//TODO:This should be call in initial Request stage
	ret := &AiBasicInfo{}
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
	IsConcurrency bool     `json:"is_conncurrency"`
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
	var ret *AiRateLimitHitInfo
	ctx := r.GetContext(REQ_AI_RATE_LIMIT_HIT)
	if ctx != nil {
		ret, _ = ctx.(*AiRateLimitHitInfo)
	}
	return ret
}
