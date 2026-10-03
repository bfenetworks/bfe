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

package mod_ai_context

import (
	"sync"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

// contextRuleTable is the product rule table of mod_ai_context: per-product
// ordered rule lists plus the global Defaults block. Follows the mod_ai_cache
// rule model: locate the rule list by product, then return the first rule
// whose condition matches the request.
type contextRuleTable struct {
	productRules map[string]ContextRuleConfList // product => rules
	defaults     *ContextDefaults               // global tuning params (Defaults block)
	lock         sync.RWMutex
}

func newContextRuleTable() *contextRuleTable {
	return &contextRuleTable{
		productRules: make(map[string]ContextRuleConfList),
		defaults:     defaultContextDefaults(),
	}
}

// defaultContextDefaults materializes the built-in defaults used before the
// first rule file load.
func defaultContextDefaults() *ContextDefaults {
	f := &DefaultsConfFile{}
	f.setDefaults()
	return f.Convert()
}

// Search returns the rule list for a product, ok is false if the product is
// not found (the request passes without compression, consistent with other
// AI modules).
func (t *contextRuleTable) Search(product string) (ContextRuleConfList, bool) {
	t.lock.RLock()
	rules, ok := t.productRules[product]
	t.lock.RUnlock()
	return rules, ok
}

// Defaults returns the global tuning parameters of the loaded rule file.
func (t *contextRuleTable) Defaults() *ContextDefaults {
	t.lock.RLock()
	d := t.defaults
	t.lock.RUnlock()
	return d
}

// Match returns the first rule whose condition matches the request, merged
// with the Defaults parameters. ok is false when no product/rule matches.
// A matched rule with mode=off still returns ok=true; the caller records
// SKIP_NO_RULE (design-changes.md 7.1-2: no rule hit or mode=off).
func (t *contextRuleTable) Match(req *bfe_basic.Request) (*ContextRuleConf, ContextParams, bool) {
	rules, ok := t.Search(req.Route.Product)
	if !ok {
		return nil, ContextParams{}, false
	}

	for _, rule := range rules {
		if rule.CondBuild.Match(req) {
			params := t.Defaults().toParams()
			params.MaxContextTokens = rule.MaxContextTokens
			params.ReserveTokens = rule.ReserveTokens
			return rule, params, true
		}
	}

	return nil, ContextParams{}, false
}

// load builds the runtime rule table from the parsed rule data. The cond
// expressions were already compiled during load (a compile failure rejects
// the whole file before this point).
func (t *contextRuleTable) load(data *ContextRuleConfData) error {
	productRules := make(map[string]ContextRuleConfList)
	if data.Config != nil {
		for product, ruleList := range *data.Config {
			rules := make(ContextRuleConfList, 0, len(*ruleList))
			for _, ruleConf := range *ruleList {
				cond, err := condition.Build(ruleConf.Cond)
				if err != nil {
					return err
				}
				ruleConf.CondBuild = cond
				rules = append(rules, ruleConf)
			}
			productRules[product] = rules
		}
	}

	t.lock.Lock()
	t.productRules = productRules
	if data.Defaults != nil {
		t.defaults = data.Defaults
	}
	t.lock.Unlock()

	return nil
}
