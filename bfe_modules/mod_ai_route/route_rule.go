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

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

// route table types
const (
	RouteTypeApikey = "apikey"
	RouteTypeEntity = "entity"
	RouteTypeGlobal = "global"
)

type RouteRule struct {
	Name      string `json:"name"`
	CondStr   string `json:"Cond"`
	Cond      condition.Condition           `json:"-"`
	Targets   []bfe_basic.AiRouteTarget   `json:"targets"`
	Fallbacks []bfe_basic.AiRouteFallback `json:"fallbacks"`
}

type RouteTable struct {
	Type  string      `json:"type"`
	Owner string      `json:"owner"`
	Rules []RouteRule `json:"rules"`
}

type AiRouteData struct {
	Version                  string                `json:"Version"`
	RouteRules               map[string]RouteTable `json:"route_rules"`
	ApikeyRouteTableBindings map[string][]string   `json:"ApikeyRouteTableBindings"`
}

func (rt *RouteTable) Match(req *bfe_basic.Request) *RouteRule {
	for i := range rt.Rules {
		rule := &rt.Rules[i]
		if rule.Cond != nil && rule.Cond.Match(req) {
			return rule
		}
	}
	return nil
}

func ValidateRouteTable(table *RouteTable) error {
	switch table.Type {
	case RouteTypeApikey, RouteTypeEntity, RouteTypeGlobal:
	default:
		return fmt.Errorf("invalid route table type: %s", table.Type)
	}

	for i := range table.Rules {
		rule := &table.Rules[i]
		if rule.Name == "" {
			return fmt.Errorf("rule name empty")
		}
		if rule.CondStr == "" {
			return fmt.Errorf("rule[%s] Cond empty", rule.Name)
		}
		cond, err := condition.Build(rule.CondStr)
		if err != nil {
			return fmt.Errorf("rule[%s] build cond[%s] err: %s", rule.Name, rule.CondStr, err)
		}
		rule.Cond = cond

		if len(rule.Targets) == 0 {
			return fmt.Errorf("rule[%s] targets empty", rule.Name)
		}

		totalWeight := 0
		for _, target := range rule.Targets {
			totalWeight += target.Weight
		}
		if totalWeight != 100 {
			return fmt.Errorf("rule[%s] total weight %d != 100", rule.Name, totalWeight)
		}
	}
	return nil
}
