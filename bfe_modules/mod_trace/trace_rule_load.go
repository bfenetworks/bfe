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
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type TraceRuleFile struct {
	Version string             // version
	Config  ProductRuleRawList // product -> raw rule list
}

type TraceRuleConf struct {
	Version string          // version
	Config  ProductRuleList // product -> rule list
}

type TraceRuleRaw struct {
	Cond   string // condition
	Enable bool   // enable trace
}

type ProductRuleRawList map[string]RuleRawList // product => raw rule list
type RuleRawList []TraceRuleRaw

func TraceRuleCheck(traceRuleFile *TraceRuleFile) error {
	if traceRuleFile == nil {
		return fmt.Errorf("traceRuleFile is nil")
	}

	if len(traceRuleFile.Version) == 0 {
		return fmt.Errorf("no Version")
	}

	if traceRuleFile.Config == nil {
		return fmt.Errorf("no Config")
	}

	return nil
}

func ruleConvert(rawRule TraceRuleRaw) (*TraceRule, error) {
	cond, err := condition.Build(rawRule.Cond)
	if err != nil {
		return nil, err
	}

	var rule TraceRule
	rule.Cond = cond
	rule.Enable = rawRule.Enable

	return &rule, nil
}

func ruleListConvert(rawRuleList RuleRawList) (TraceRuleList, error) {
	ruleList := TraceRuleList{}
	for i, rawRule := range rawRuleList {
		rule, err := ruleConvert(rawRule)
		if err != nil {
			return nil, fmt.Errorf("rule [%d] error: %v", i, err)
		}

		ruleList = append(ruleList, *rule)
	}

	return ruleList, nil
}

func TraceRuleFileLoad(filename string) (*TraceRuleConf, error) {
	var traceRuleFile TraceRuleFile
	var traceRuleConf TraceRuleConf

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	err = decoder.Decode(&traceRuleFile)
	if err != nil {
		return nil, err
	}

	err = TraceRuleCheck(&traceRuleFile)
	if err != nil {
		return nil, err
	}

	traceRuleConf.Version = traceRuleFile.Version
	traceRuleConf.Config = make(ProductRuleList)

	for product, ruleFileList := range traceRuleFile.Config {
		ruleList, err := ruleListConvert(ruleFileList)
		if err != nil {
			return nil, fmt.Errorf("product[%s] rule error: %v", product, err)
		}
		traceRuleConf.Config[product] = ruleList
	}

	return &traceRuleConf, nil
}
