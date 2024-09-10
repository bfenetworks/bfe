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

type prisonRules struct {
	ruleList []prisonRule           // prison rule list for one product
	ruleMap  map[string]*prisonRule // name => prison rule
}

func newPrisonRules(ruleConfList PrisonRuleConfList) (*prisonRules, error) {
	// make new rule list
	ruleList, err := newPrisonRuleList(ruleConfList)
	if err != nil {
		return nil, err
	}

	// build rule map
	ruleMap := buildPrisonRuleMap(ruleList)

	return &prisonRules{
		ruleList: ruleList,
		ruleMap:  ruleMap,
	}, nil
}

func newPrisonRuleList(ruleConfList PrisonRuleConfList) ([]prisonRule, error) {
	var ruleList []prisonRule
	for _, ruleConf := range ruleConfList {
		rule, err := newPrisonRule(*ruleConf)
		if err != nil {
			return nil, err
		}

		ruleList = append(ruleList, *rule)
	}

	return ruleList, nil
}

func buildPrisonRuleMap(ruleList []prisonRule) map[string]*prisonRule {
	ruleMap := make(map[string]*prisonRule, len(ruleList))
	for i := range ruleList {
		name := ruleList[i].name
		ruleMap[name] = &ruleList[i]
	}

	return ruleMap
}

func (r *prisonRules) initDict(oldRules *prisonRules) {
	oldRuleMap := make(map[string]*prisonRule)
	if oldRules != nil {
		oldRuleMap = oldRules.ruleMap
	}

	// compare rules
	ruleAdd, ruleMod, _ := compareRules(r.ruleMap, oldRuleMap)

	// add new rule
	for _, name := range ruleAdd {
		rule := r.ruleMap[name]
		rule.initDict(nil)
	}

	// use old dict
	for _, name := range ruleMod {
		rule := r.ruleMap[name]
		rule.initDict(oldRuleMap[name])
	}
}

func compareRules(newRuleMap, oldRuleMap map[string]*prisonRule) ([]string, []string, []string) {
	var ruleAdd, ruleMod, ruleDel []string

	for name := range newRuleMap {
		if _, ok := oldRuleMap[name]; !ok {
			ruleAdd = append(ruleAdd, name)
		} else {
			ruleMod = append(ruleMod, name)
		}
	}

	for name := range oldRuleMap {
		if _, ok := newRuleMap[name]; !ok {
			ruleDel = append(ruleDel, name)
		}
	}

	return ruleAdd, ruleMod, ruleDel
}
