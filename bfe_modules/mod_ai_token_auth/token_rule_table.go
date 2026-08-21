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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

type TokenRuleTable struct {
	lock          sync.RWMutex
	version       string
	productRules  ProductRules
	productTokens ProductTokens
}

func NewTokenRuleTable() *TokenRuleTable {
	t := new(TokenRuleTable)
	t.productRules = make(ProductRules)
	t.productTokens = make(ProductTokens)
	return t
}

func (t *TokenRuleTable) Update(conf productRuleConf) {
	t.lock.Lock()
	t.version = conf.Version
	t.productRules = conf.Config
	t.productTokens = conf.Tokens
	t.lock.Unlock()
}

func (t *TokenRuleTable) Search(product string) (*tokenRuleList, bool) {
	t.lock.RLock()
	productRules := t.productRules
	t.lock.RUnlock()

	rules, ok := productRules[product]
	return rules, ok
}

func (t *TokenRuleTable) GetToken(product, key string) (*Token, bool) {
	t.lock.RLock()
	tokenMap := t.productTokens[product]
	t.lock.RUnlock()

	if tokenMap == nil {
		return nil, false
	}
	tok, ok := (*tokenMap)[key]
	return tok, ok
}

func (t *TokenRuleTable) ValidateUserToken(product, key string) (token *Token, err error) {
	if key == "" {
		return nil, errors.New("no token")
	}
	var ok bool
	token, ok = t.GetToken(product, key)
	if !ok {
		return nil, errors.New("token not found")
	}

	if !token.Enabled {
		return nil, fmt.Errorf("token %s disabled", token.KeyId)
	}

	if token.ExpiredTime != -1 && token.ExpiredTime < time.Now().Unix() {
		return nil, fmt.Errorf("token %s expired", token.KeyId)
	}

	return token, nil
}

func SetAiAuthInfo(req *bfe_basic.Request, rejectReason string, rejectQuotaPlans []string) {
	aiBasicInfo := req.GetAiBasicInfo()
	if aiBasicInfo != nil {
		aiBasicInfo.AiAuthInfo.RejectReason = rejectReason
		aiBasicInfo.AiAuthInfo.RejectQuotaPlans = rejectQuotaPlans
	}
}

func (m *ModuleAITokenAuth) ValidateUserTokenByReq(req *bfe_basic.Request) (token *Token, err *bfe_basic.AiError) {
	key := bfe_basic.GetApiKey(req)
	if key == "" {
		SetAiAuthInfo(req, bfe_basic.CodeNoApiKey, nil)
		return nil, bfe_basic.NewAiError(bfe_basic.CodeNoApiKey, bfe_basic.TypeAuthenticationError, "no api key in request")
	}
	product := req.Route.Product
	if product == "" {
		SetAiAuthInfo(req, bfe_basic.CodeInvalidRequest, nil)
		return nil, bfe_basic.NewAiError(bfe_basic.CodeInvalidRequest, bfe_basic.TypeInvalidRequestError, "product not found")
	}

	var ok bool
	token, ok = m.ruleTable.GetToken(product, key)
	if !ok {
		SetAiAuthInfo(req, bfe_basic.CodeInvalidApiKey, nil)
		return nil, bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeInvalidApiKey, bfe_basic.TypeAuthenticationError, fmt.Sprintf("Invalid API key: %s. Key not found in system.", key),
			&bfe_basic.AiErrorDetail{
				ApiKey: key,
			})
	}

	// record key_id into AiBasicInfo as early as possible so that access log
	// can still identify the token even if the request is later rejected.
	if aiBasicInfo := req.GetAiBasicInfo(); aiBasicInfo != nil {
		aiBasicInfo.ClientKeyId = token.KeyId
	}

	if !token.Enabled {
		SetAiAuthInfo(req, bfe_basic.CodeKeyDisabled, nil)
		return nil, bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeKeyDisabled, bfe_basic.TypeAuthenticationError, fmt.Sprintf("Invalid API key: %s. disabled.", key),
			&bfe_basic.AiErrorDetail{
				ApiKey: key,
				KeyId:  token.KeyId,
			})
	}

	if token.ExpiredTime != -1 && token.ExpiredTime < time.Now().Unix() {
		SetAiAuthInfo(req, bfe_basic.CodeKeyExpired, nil)
		return nil, bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeKeyExpired, bfe_basic.TypeAuthenticationError, fmt.Sprintf("Invalid API key: %s. expired.", key),
			&bfe_basic.AiErrorDetail{
				ApiKey: key,
				KeyId:  token.KeyId,
			})
	}

	if !token.UnlimitedQuota {
		// check token quotaPlans, deduct quota from redis key of each quotaPlan
		for _, plan := range token.QuotaPlans {
			if plan.Unlimited || plan.PassNoQuota {
				continue
			}
			if plan.ExpiredTime != -1 && plan.ExpiredTime < time.Now().Unix() {
				// plan quota expired
				SetAiAuthInfo(req, bfe_basic.CodeQuotaExpired, []string{plan.Id})
				return nil, bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeQuotaExpired, bfe_basic.TypeQuotaError, fmt.Sprintf("Quota plan %s expired.", plan.Id),
					&bfe_basic.AiErrorDetail{
						ApiKey:      key,
						KeyId:       token.KeyId,
						QuotaPlanId: plan.Id,
						LimitType:   bfe_basic.LimitTypeApiKeyQuota,
					})
			}
			hasBalance, _, err := plan.HasBalance(m.redisClient)
			if err != nil {
				SetAiAuthInfo(req, bfe_basic.CodeInternalQuotaError, []string{plan.Id})
				return nil, bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeInternalQuotaError, bfe_basic.TypeInternalError, fmt.Sprintf("Internal error during quota deduction for plan %s: %v", plan.Id, err),
					&bfe_basic.AiErrorDetail{
						ApiKey:      key,
						KeyId:       token.KeyId,
						QuotaPlanId: plan.Id,
					})
			}
			if !hasBalance {
				SetAiAuthInfo(req, bfe_basic.CodeQuotaExhausted, []string{plan.Id})
				return nil, bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeQuotaExhausted, bfe_basic.TypeQuotaError, fmt.Sprintf("Quota plan %s exhausted.", plan.Id),
					&bfe_basic.AiErrorDetail{
						ApiKey:      key,
						KeyId:       token.KeyId,
						QuotaPlanId: plan.Id,
						LimitType:   bfe_basic.LimitTypeApiKeyQuota,
					})
			}

			// record quota plan that passed balance check
			if aiBasicInfo := req.GetAiBasicInfo(); aiBasicInfo != nil {
				aiBasicInfo.AiAuthInfo.HitQuotaPlans = append(aiBasicInfo.AiAuthInfo.HitQuotaPlans, plan.Id)
			}
		}
	}

	if len(token.Models) > 0 || len(token.BlockModels) > 0 {
		model, err := condition.ReqBodyJsonFetch(req, "model", nil)
		if err != nil || model == "" {
			SetAiAuthInfo(req, bfe_basic.CodeInvalidRequest, nil)
			return nil, bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeInvalidRequest, bfe_basic.TypeInvalidRequestError, fmt.Sprintf("Model not found in request body: %v", err),
				&bfe_basic.AiErrorDetail{
					ApiKey: key,
					KeyId:  token.KeyId,
				})
		}
		model = strings.TrimSpace(model)
		if len(token.BlockModels) > 0 {
			for _, blockModel := range token.BlockModels {
				if blockModel == model {
					SetAiAuthInfo(req, bfe_basic.CodeModelNotAllowed, nil)
					return nil, bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeModelNotAllowed, bfe_basic.TypeInvalidRequestError, fmt.Sprintf("Model %s blocked by key %s", model, key),
						&bfe_basic.AiErrorDetail{
							ApiKey: key,
							KeyId:  token.KeyId,
							Model:  model,
						})
				}
			}
		}

		if len(token.Models) > 0 {
			inModels := false
			for _, m := range token.Models {
				if m == model {
					inModels = true
					break
				}
			}
			if !inModels {
				SetAiAuthInfo(req, bfe_basic.CodeModelNotAllowed, nil)
				return nil, bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeModelNotAllowed, bfe_basic.TypeInvalidRequestError, fmt.Sprintf("Model %s not allowed by key %s", model, key),
					&bfe_basic.AiErrorDetail{
						ApiKey: key,
						KeyId:  token.KeyId,
						Model:  model,
					})
			}
		}
	}

	if len(token.Subnet) > 0 {
		inSubnet := false
		for _, subnet := range token.Subnet {
			if req.ClientAddr != nil && subnet.Contains(req.ClientAddr.IP) {
				inSubnet = true
				break
			} else if req.RemoteAddr != nil && subnet.Contains(req.RemoteAddr.IP) {
				inSubnet = true
				break
			}
		}
		if !inSubnet {
			SetAiAuthInfo(req, bfe_basic.CodeSubnetNotAllowed, nil)
			return nil, bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeSubnetNotAllowed, bfe_basic.TypeAuthenticationError, fmt.Sprintf("Client IP not in subnet of key %s", key),
				&bfe_basic.AiErrorDetail{
					ApiKey: key,
					KeyId:  token.KeyId,
				})
		}
	}
	return token, nil
}
