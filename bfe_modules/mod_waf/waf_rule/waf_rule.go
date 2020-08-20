// Copyright (c) 2020 The BFE Authors.
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
package waf_rule

const (
	RuleBashCmd = "RuleBashCmd" // bash cmd
)

var implementedRule = map[string]WafRule{
	RuleBashCmd: NewRuleBashCmdExe(),
}

func IsValidRule(rule string) bool {
	_, ok := implementedRule[rule]
	return ok
}

type WafRule interface {
	Init() error
	Check(req *RuleRequestInfo) bool
}

type WafRuleTable struct {
	rules map[string]WafRule // name to WafRule
}

func NewWafRuleTable() *WafRuleTable {
	wafRules := new(WafRuleTable)

	wafRules.rules = make(map[string]WafRule)
	return wafRules
}

func (wr *WafRuleTable) Init() {
	for k, v := range implementedRule {
		wr.rules[k] = v
	}
	for _, v := range wr.rules {
		v.Init()
	}
}

func (wr *WafRuleTable) GetRule(ruleName string) (WafRule, bool) {
	rule, ok := wr.rules[ruleName]
	return rule, ok
}
