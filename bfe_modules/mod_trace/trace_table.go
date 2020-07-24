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

package mod_trace

import (
	"sync"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

type TraceRuleTable struct {
	lock        sync.RWMutex
	version     string
	productRule ProductRuleList // product => rule list
}

type TraceRule struct {
	Cond   condition.Condition
	Enable bool
}

type ProductRuleList map[string]TraceRuleList // product => list of trace rule list
type TraceRuleList []TraceRule

func NewTraceRuleTable() *TraceRuleTable {
	t := new(TraceRuleTable)
	t.productRule = make(ProductRuleList)
	return t
}

func (t *TraceRuleTable) Update(ruleConf *TraceRuleConf) {
	t.lock.Lock()
	t.version = ruleConf.Version
	t.productRule = ruleConf.Config
	t.lock.Unlock()
}

func (t *TraceRuleTable) Search(product string) (TraceRuleList, bool) {
	t.lock.RLock()
	ruleList, ok := t.productRule[product]
	t.lock.RUnlock()

	return ruleList, ok
}
