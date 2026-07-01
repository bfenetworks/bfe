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
	"sync"

	"github.com/baidu/go-lib/web-monitor/module_state2"
	"github.com/bfenetworks/bfe/bfe_basic/action"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

type productRule struct {
	cond      condition.Condition
	condStr   string
	hitAction *action.Action
}

type productRuleTable struct {
	productRules  map[string][]*productRule // product => rules
	ratePolicies  map[string]*PolicyConf    // policyId => policy config
	apiKeyBinding map[string][]string       // apiKey => policyIds

	state *module_state2.State
	lock  sync.RWMutex
}

func newProductRuleTable(state *module_state2.State) *productRuleTable {
	return &productRuleTable{
		productRules:  make(map[string][]*productRule),
		ratePolicies:  make(map[string]*PolicyConf),
		apiKeyBinding: make(map[string][]string),
		state:         state,
	}
}

func (p *productRuleTable) getProductRules(product string) []*productRule {
	p.lock.RLock()
	rules := p.productRules[product]
	p.lock.RUnlock()
	return rules
}

func (p *productRuleTable) getPolicy(policyId string) *PolicyConf {
	p.lock.RLock()
	policy := p.ratePolicies[policyId]
	p.lock.RUnlock()
	return policy
}

func (p *productRuleTable) getPolicyIds(apiKey string) []string {
	p.lock.RLock()
	ids := p.apiKeyBinding[apiKey]
	p.lock.RUnlock()
	return ids
}

func (p *productRuleTable) load(config *AiRateLimitConf) error {
	productRules := make(map[string][]*productRule)
	if config.Config != nil {
		for product, ruleList := range *config.Config {
			var rules []*productRule
			for _, ruleConf := range *ruleList {
				cond, err := condition.Build(ruleConf.Cond)
				if err != nil {
					return err
				}
				rules = append(rules, &productRule{
					cond:      cond,
					condStr:   ruleConf.Cond,
					hitAction: ruleConf.HitAction,
				})
			}
			productRules[product] = rules
		}
	}

	ratePolicies := make(map[string]*PolicyConf)
	if config.RateLimitPolicies != nil {
		for policyId, policy := range *config.RateLimitPolicies {
			ratePolicies[policyId] = policy
		}
	}

	apiKeyBinding := make(map[string][]string)
	if config.ApikeyRatePoliciesBindings != nil {
		for apiKey, policyIds := range *config.ApikeyRatePoliciesBindings {
			apiKeyBinding[apiKey] = policyIds
		}
	}

	p.lock.Lock()
	p.productRules = productRules
	p.ratePolicies = ratePolicies
	p.apiKeyBinding = apiKeyBinding
	p.lock.Unlock()

	return nil
}
