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

package mod_static

import (
	"sync"
)

type StaticRuleTable struct {
	lock         sync.RWMutex
	version      string
	productRules ProductRules
}

func NewStaticRuleTable() *StaticRuleTable {
	t := new(StaticRuleTable)
	t.productRules = make(ProductRules)
	return t
}

func (t *StaticRuleTable) Update(conf StaticConf) {
	t.lock.Lock()
	t.version = conf.Version
	t.productRules = conf.Config
	t.lock.Unlock()
}

func (t *StaticRuleTable) Search(product string) (*RuleList, bool) {
	t.lock.RLock()
	productRules := t.productRules
	t.lock.RUnlock()

	rules, ok := productRules[product]
	return rules, ok
}
