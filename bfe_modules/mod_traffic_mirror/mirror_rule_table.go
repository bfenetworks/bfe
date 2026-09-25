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

package mod_traffic_mirror

import (
	"sync"
	"sync/atomic"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

// mirror counters snapshot for prometheus export
type mirrorCounters struct {
	reqTotal      uint64 // requests submitted for mirroring
	submitDrop    uint64 // dropped because semaphore/queue is full
	skipSample    uint64 // not selected by percentage sampling
	skipBodyLimit uint64 // skipped due to body size limit
	skipRewrite   uint64 // body rewrite failed (mirrored with original body)
	sendFail      uint64 // mirror send/drain failures
	circuitOpen   uint64 // dropped because circuit breaker is open
	respTruncated uint64 // responses truncated by drain limits
}

type mirrorRuleTable struct {
	productRules map[string]*MirrorRuleConfList // product => rules
	lock         sync.RWMutex

	counters mirrorCounters
}

func newMirrorRuleTable() *mirrorRuleTable {
	return &mirrorRuleTable{
		productRules: make(map[string]*MirrorRuleConfList),
	}
}

// Search returns the rule list for a product, ok is false if the product
// is not found (the request passes without mirroring, consistent with
// other AI modules).
func (t *mirrorRuleTable) Search(product string) (*MirrorRuleConfList, bool) {
	t.lock.RLock()
	rules, ok := t.productRules[product]
	t.lock.RUnlock()
	return rules, ok
}

// Match returns the first rule whose condition matches the request,
// or nil if no rule matches. An empty condition matches everything.
func (t *mirrorRuleTable) Match(req *bfe_basic.Request, rules *MirrorRuleConfList) *MirrorRuleConf {
	if rules == nil {
		return nil
	}
	for _, rule := range *rules {
		if rule.CondBuild == nil || rule.CondBuild.Match(req) {
			return rule
		}
	}

	return nil
}

func (t *mirrorRuleTable) load(config *MirrorRuleConfData) error {
	productRules := make(map[string]*MirrorRuleConfList)
	if config.Config != nil {
		for product, ruleList := range *config.Config {
			var rules MirrorRuleConfList
			for _, ruleConf := range *ruleList {
				// build condition; empty cond matches every request
				if ruleConf.Cond != "" {
					cond, err := condition.Build(ruleConf.Cond)
					if err != nil {
						return err
					}
					ruleConf.CondBuild = cond
				}
				rules = append(rules, ruleConf)
			}
			productRules[product] = &rules
		}
	}

	t.lock.Lock()
	t.productRules = productRules
	t.lock.Unlock()

	return nil
}

func (t *mirrorRuleTable) incReqTotal() {
	atomic.AddUint64(&t.counters.reqTotal, 1)
}

func (t *mirrorRuleTable) incSubmitDrop() {
	atomic.AddUint64(&t.counters.submitDrop, 1)
}

func (t *mirrorRuleTable) incSkipSample() {
	atomic.AddUint64(&t.counters.skipSample, 1)
}

func (t *mirrorRuleTable) incSkipBodyLimit() {
	atomic.AddUint64(&t.counters.skipBodyLimit, 1)
}

func (t *mirrorRuleTable) incSkipRewrite() {
	atomic.AddUint64(&t.counters.skipRewrite, 1)
}

func (t *mirrorRuleTable) incSendFail() {
	atomic.AddUint64(&t.counters.sendFail, 1)
}

func (t *mirrorRuleTable) incCircuitOpen() {
	atomic.AddUint64(&t.counters.circuitOpen, 1)
}

func (t *mirrorRuleTable) incRespTruncated() {
	atomic.AddUint64(&t.counters.respTruncated, 1)
}

func (t *mirrorRuleTable) snapshotCounters() mirrorCounters {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return mirrorCounters{
		reqTotal:      atomic.LoadUint64(&t.counters.reqTotal),
		submitDrop:    atomic.LoadUint64(&t.counters.submitDrop),
		skipSample:    atomic.LoadUint64(&t.counters.skipSample),
		skipBodyLimit: atomic.LoadUint64(&t.counters.skipBodyLimit),
		skipRewrite:   atomic.LoadUint64(&t.counters.skipRewrite),
		sendFail:      atomic.LoadUint64(&t.counters.sendFail),
		circuitOpen:   atomic.LoadUint64(&t.counters.circuitOpen),
		respTruncated: atomic.LoadUint64(&t.counters.respTruncated),
	}
}
