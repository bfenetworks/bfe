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
	"sync"
	"sync/atomic"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

// cache counters snapshot for prometheus export
type cacheCounters struct {
	reqTotal      uint64
	cacheHit      uint64
	cacheMiss     uint64
	cacheSkip     uint64
	redisErr      uint64
	latencyMs     uint64
	valueTooLarge uint64
}

type cacheRuleTable struct {
	productRules map[string]ProductRuleConfList // product => rules
	lock         sync.RWMutex

	counters cacheCounters
}

func newCacheRuleTable() *cacheRuleTable {
	return &cacheRuleTable{
		productRules: make(map[string]ProductRuleConfList),
	}
}

// Search returns the rule list for a product, ok is false if the product
// is not found (the request should pass without caching, consistent with
// other AI modules).
func (t *cacheRuleTable) Search(product string) (ProductRuleConfList, bool) {
	t.lock.RLock()
	rules, ok := t.productRules[product]
	t.lock.RUnlock()
	return rules, ok
}

// Match returns the first rule whose condition matches the request,
// or nil if no rule matches. A rule with cacheKeyStrategy=disabled also
// returns nil after recording the match: it means "do not cache".
func (t *cacheRuleTable) Match(req *bfe_basic.Request, rules ProductRuleConfList) *ProductRuleConf {
	for _, rule := range rules {
		if rule.CondBuild.Match(req) {
			if rule.Disabled {
				return nil
			}
			return rule
		}
	}

	return nil
}

func (t *cacheRuleTable) load(config *ProductRuleConfData) error {
	productRules := make(map[string]ProductRuleConfList)
	if config.Config != nil {
		for product, ruleList := range *config.Config {
			var rules ProductRuleConfList
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
	t.lock.Unlock()

	return nil
}

func (t *cacheRuleTable) incReqTotal() {
	atomic.AddUint64(&t.counters.reqTotal, 1)
}

func (t *cacheRuleTable) incCacheHit() {
	atomic.AddUint64(&t.counters.cacheHit, 1)
}

func (t *cacheRuleTable) incCacheMiss() {
	atomic.AddUint64(&t.counters.cacheMiss, 1)
}

func (t *cacheRuleTable) incCacheSkip() {
	atomic.AddUint64(&t.counters.cacheSkip, 1)
}

func (t *cacheRuleTable) incRedisErr() {
	atomic.AddUint64(&t.counters.redisErr, 1)
}

func (t *cacheRuleTable) addLatencyMs(ms int64) {
	if ms > 0 {
		atomic.AddUint64(&t.counters.latencyMs, uint64(ms))
	}
}

func (t *cacheRuleTable) incValueTooLarge() {
	atomic.AddUint64(&t.counters.valueTooLarge, 1)
}

func (t *cacheRuleTable) snapshotCounters() cacheCounters {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return cacheCounters{
		reqTotal:      atomic.LoadUint64(&t.counters.reqTotal),
		cacheHit:      atomic.LoadUint64(&t.counters.cacheHit),
		cacheMiss:     atomic.LoadUint64(&t.counters.cacheMiss),
		cacheSkip:     atomic.LoadUint64(&t.counters.cacheSkip),
		redisErr:      atomic.LoadUint64(&t.counters.redisErr),
		latencyMs:     atomic.LoadUint64(&t.counters.latencyMs),
		valueTooLarge: atomic.LoadUint64(&t.counters.valueTooLarge),
	}
}
