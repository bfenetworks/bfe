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
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

type TokenRuleTable struct {
	lock         sync.RWMutex
	version      string
	productRules ProductRules
	productTokens ProductTokens
	// productApiKeys ProductTokens
}

func NewTokenRuleTable() *TokenRuleTable {
	t := new(TokenRuleTable)
	t.productRules = make(ProductRules)
	t.productTokens = make(ProductTokens)
	// t.productApiKeys = make(ProductTokens)
	return t
}

func (t *TokenRuleTable) Update(conf productRuleConf) (oldtokens []*Token) {
	// check token update time, if the token is not updated, we keep the old used quota
	for prod, tokenmap := range t.productTokens {
		newTokenMap, ok := conf.Tokens[prod]
		if !ok {
			// product not in new conf, all these tokens are removed
			for _, t := range *tokenmap {
				oldtokens = append(oldtokens, t)
			}
		} else {
			// product in new conf, check each token
			for k, t := range *tokenmap {
				newToken, ok := (*newTokenMap)[k]
				if !ok {
					// token not in new conf, remove it
					oldtokens = append(oldtokens, t)
				} else if t.UpdateTime == newToken.UpdateTime {
					// token not updated, keep the old used quota
					newToken.UsedQuota = t.UsedQuota
				} else {
					// token updated, reset used quota
					newToken.UsedQuota = &atomic.Uint64{}
					oldtokens = append(oldtokens, t)
				}
			}
		}
	}
	// init new tokens' UsedQuota
	for _, tokenmap := range conf.Tokens {
		for _, t := range *tokenmap {
			if t.UsedQuota == nil {
				t.UsedQuota = &atomic.Uint64{}
			}
		}
	}

	t.lock.Lock()
	t.version = conf.Version
	t.productRules = conf.Config
	t.productTokens = conf.Tokens
	// t.productApiKeys = apiKeys
	t.lock.Unlock()

	return
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

	switch token.Status {
	case TokenStatusExhausted:
		return nil, fmt.Errorf("token %s quota exhausted", token.Name)
	case TokenStatusExpired:
		return nil, fmt.Errorf("token %s expired", token.Name)
	case TokenStatusDisabled:
		return nil, fmt.Errorf("token %s disabled", token.Name)
	}

	if token.ExpiredTime != -1 && token.ExpiredTime < time.Now().Unix() {
		token.Status = TokenStatusExpired
		return nil, fmt.Errorf("token %s expired", token.Name)
	}

	if !token.UnlimitedQuota && token.RemainQuota <= 0 {
		token.Status = TokenStatusExhausted
		return nil, fmt.Errorf("token %s quota exhausted", token.Name)
	}
	return token, nil
}

func (m *ModuleAITokenAuth) ValidateUserTokenByReq(req *bfe_basic.Request) (token *Token, err error) {
	key := GetApiKey(req)
	if key == "" {
		return nil, errors.New("no token")
	}
	product := req.Route.Product
	if product == "" {
		return nil, errors.New("no product")
	}

	var ok bool
	token, ok = m.ruleTable.GetToken(product, key)
	if !ok {
		return nil, errors.New("token not found")
	}

	switch token.Status {
	case TokenStatusExhausted:
		return nil, fmt.Errorf("token %s quota exhausted", token.Name)
	case TokenStatusExpired:
		return nil, fmt.Errorf("token %s expired", token.Name)
	case TokenStatusDisabled:
		return nil, fmt.Errorf("token %s disabled", token.Name)
	}

	if token.ExpiredTime != -1 && token.ExpiredTime < time.Now().Unix() {
		token.Status = TokenStatusExpired
		return nil, fmt.Errorf("token %s expired", token.Name)
	}

	if !token.UnlimitedQuota {
		if token.RemainQuota <= 0 {
			token.Status = TokenStatusExhausted
			return nil, fmt.Errorf("token %s quota exhausted", token.Name)
		} else {
			used := m.GetTokenUsedQuota(token)
			if used >= token.RemainQuota {
				token.Status = TokenStatusExhausted
				return nil, fmt.Errorf("token %s quota exhausted", token.Name)
			}
		}
	}

	if len(token.Models) > 0 {
		model, err := condition.ReqBodyJsonFetch(req, "model")
		if err != nil || model == "" {
			return nil, fmt.Errorf("model not found in request body: %v", err)
		}
		model = strings.TrimSpace(model)
		inModels := false
		for _, m := range token.Models {
			if m == model {
				inModels = true
				break
			}
		}
		if !inModels {
			return nil, fmt.Errorf("model %s not allowed by token %s", model, token.Name)
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
			return nil, fmt.Errorf("client IP not in subnet of token %s", token.Name)
		}
	}
	return token, nil
}

func (m *ModuleAITokenAuth) GetTokenUsedQuota(t *Token) int64 {
	if t == nil {
		return 0
	}
	key := usedQuotaKey(t.Key, t.UpdateTime)
	val, err := m.redisClient.GetInt64(key)
	if err != nil {
		return 0
	}
	return val
}

func (m *ModuleAITokenAuth) IncrTokenUsedQuotaBy(t *Token, delta int64) int64 {
	if t == nil {
		return 0
	}
	key := usedQuotaKey(t.Key, t.UpdateTime)
	val, err := m.redisClient.IncrBy(key, delta)
	if err != nil {
		return 0
	}
	return val
}
