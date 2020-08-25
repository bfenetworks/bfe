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
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type StaticRuleFile struct {
	Cond   string
	Action *ActionFile
}

type StaticRule struct {
	Cond   condition.Condition
	Action Action
}

type RuleFileList []StaticRuleFile
type RuleList []StaticRule

type ProductRulesFile map[string]*RuleFileList
type ProductRules map[string]*RuleList

type StaticConfFile struct {
	Version *string
	Config  *ProductRulesFile
}

type StaticConf struct {
	Version string
	Config  ProductRules
}

func StaticRuleCheck(conf StaticRuleFile) error {
	if len(conf.Cond) == 0 {
		return fmt.Errorf("no Cond")
	}

	if conf.Action == nil {
		return fmt.Errorf("no Action")
	}

	if err := ActionFileCheck(conf.Action); err != nil {
		return fmt.Errorf("Action: %v", err)
	}

	return nil
}

func RuleListCheck(conf *RuleFileList) error {
	for index, rule := range *conf {
		err := StaticRuleCheck(rule)
		if err != nil {
			return fmt.Errorf("StaticRule: %d, %v", index, err)
		}
	}

	return nil
}

func ProductRulesCheck(conf *ProductRulesFile) error {
	for product, ruleList := range *conf {
		if ruleList == nil {
			return fmt.Errorf("no RuleList for product: %s", product)
		}

		err := RuleListCheck(ruleList)
		if err != nil {
			return fmt.Errorf("invalid product rules:%s, %v", product, err)
		}
	}

	return nil
}

func StaticConfCheck(conf StaticConfFile) error {
	var err error

	if conf.Version == nil {
		return fmt.Errorf("no Version")
	}

	if conf.Config == nil {
		return fmt.Errorf("no Config")
	}

	err = ProductRulesCheck(conf.Config)
	if err != nil {
		return fmt.Errorf("Config: %v", err)
	}

	return nil
}

func ruleConvert(ruleFile StaticRuleFile) (StaticRule, error) {
	rule := StaticRule{}

	cond, err := condition.Build(ruleFile.Cond)
	if err != nil {
		return rule, err
	}

	rule.Cond = cond
	rule.Action = actionConvert(*ruleFile.Action)

	return rule, nil
}

func ruleListConvert(ruleFileList *RuleFileList) (*RuleList, error) {
	ruleList := new(RuleList)
	*ruleList = make([]StaticRule, 0)

	for _, ruleFile := range *ruleFileList {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return ruleList, err
		}
		*ruleList = append(*ruleList, rule)
	}

	return ruleList, nil
}

func StaticConfLoad(filename string) (StaticConf, error) {
	var conf StaticConf

	file, err := os.Open(filename)
	if err != nil {
		return conf, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	var config StaticConfFile
	err = decoder.Decode(&config)
	if err != nil {
		return conf, err
	}

	err = StaticConfCheck(config)
	if err != nil {
		return conf, err
	}

	conf.Version = *config.Version
	conf.Config = make(ProductRules)

	for product, ruleFileList := range *config.Config {
		ruleList, err := ruleListConvert(ruleFileList)
		if err != nil {
			return conf, err
		}
		conf.Config[product] = ruleList
	}

	return conf, nil
}
