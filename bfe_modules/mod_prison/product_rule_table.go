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

package mod_prison

import (
	"sync"
)

type productRuleTable struct {
	ruleTable map[string]*prisonRules // productName => prison rules
	lock      sync.RWMutex
}

func newProductRuleTable() *productRuleTable {
	return &productRuleTable{
		ruleTable: make(map[string]*prisonRules),
	}
}

func (p *productRuleTable) getRules(product string) (*prisonRules, bool) {
	p.lock.RLock()
	rules, ok := p.ruleTable[product]
	p.lock.RUnlock()

	return rules, ok
}

func (p *productRuleTable) setTable(ruleTable map[string]*prisonRules) {
	p.lock.Lock()
	p.ruleTable = ruleTable
	p.lock.Unlock()
}

func (p *productRuleTable) getTable() map[string]*prisonRules {
	p.lock.RLock()
	ruleTable := p.ruleTable
	p.lock.RUnlock()

	return ruleTable
}

func (p *productRuleTable) newRuleTable(config ProductRuleConf) (map[string]*prisonRules, error) {
	oldRuleTable := p.getTable()

	ruleTable := make(map[string]*prisonRules)
	for product, ruleConfList := range *config.Config {
		// create new Prison Rule
		rules, err := newPrisonRules(*ruleConfList)
		if err != nil {
			return nil, err
		}

		// init accessDict and prisonDict
		oldRules, ok := oldRuleTable[product]
		if !ok {
			rules.initDict(nil)
		} else {
			rules.initDict(oldRules)
		}

		ruleTable[product] = rules
	}

	return ruleTable, nil
}

func (p *productRuleTable) load(config ProductRuleConf) error {
	ruleTable, err := p.newRuleTable(config)
	if err != nil {
		return err
	}

	p.setTable(ruleTable)
	return nil
}
