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

package mod_ai_route

import (
	"fmt"
	"sync"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_basic"
)

type AiRouteTable struct {
	lock sync.RWMutex

	// routeRules key: route table key (<type>_<owner>)
	// routeRules value: pointer to the route table
	routeRules map[string]*RouteTable

	// bindings key: API-Key string
	// bindings value: ordered list of route table keys to search
	bindings map[string][]string
}

func NewAiRouteTable() *AiRouteTable {
	return &AiRouteTable{
		routeRules: make(map[string]*RouteTable),
		bindings:   make(map[string][]string),
	}
}

func (t *AiRouteTable) Update(data *AiRouteData) error {
	// validate and compile conditions (outside the lock)
	rules := make(map[string]*RouteTable)
	for key, table := range data.RouteRules {
		if err := ValidateRouteTable(&table); err != nil {
			return fmt.Errorf("validate route table[%s] err: %s", key, err)
		}
		tableCopy := table
		rules[key] = &tableCopy
	}

	// only lock when swapping the atomic references
	t.lock.Lock()
	t.routeRules = rules
	t.bindings = data.ApikeyRouteTableBindings
	t.lock.Unlock()

	return nil
}

func (t *AiRouteTable) Search(apiKey string, req *bfe_basic.Request) *bfe_basic.AiRouteResult {
	t.lock.RLock()

	tableKeys, ok := t.bindings[apiKey]
	if !ok || len(tableKeys) == 0 {
		t.lock.RUnlock()
		return nil
	}

	// copy table references under lock; table.Match() may be expensive,
	// so we release the lock before matching.
	tables := make([]*RouteTable, 0, len(tableKeys))
	for _, key := range tableKeys {
		if table, ok := t.routeRules[key]; ok {
			tables = append(tables, table)
		} else if openDebug {
			log.Logger.Debug("mod_ai_route: route table[%s] not found", key)
		}
	}
	t.lock.RUnlock()

	// match outside the lock to reduce critical section
	for _, table := range tables {
		rule := table.Match(req)
		if rule != nil {
			return &bfe_basic.AiRouteResult{
				RouteType: table.Type,
				Owner:     table.Owner,
				RuleName:  rule.Name,
				Targets:   rule.Targets,
				Fallbacks: rule.Fallbacks,
			}
		}
	}

	return nil
}
