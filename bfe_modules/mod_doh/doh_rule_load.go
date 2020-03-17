// Copyright (c) 2019 Baidu, Inc.
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

package mod_doh

import (
	"encoding/json"
	"fmt"
	"os"
)

import (
	"github.com/baidu/bfe/bfe_basic/condition"
)

type DohRuleFile struct {
	Cond    string
	Net     string
	Address string
}

type DohRule struct {
	Cond    condition.Condition
	Net     string
	Address string
}

type RuleFileList []DohRuleFile
type RuleList []DohRule

type ProductRulesFile map[string]*RuleFileList
type ProductRules map[string]*RuleList

type DohConfFile struct {
	Version *string
	Config  *ProductRulesFile
}

type DohConf struct {
	Version string
	Config  ProductRules
}

func DohRuleCheck(conf DohRuleFile) error {
	if len(conf.Cond) == 0 {
		return fmt.Errorf("no Cond")
	}

	if conf.Net != "TCP" && conf.Net != "UDP" {
		return fmt.Errorf("Net should be \"TCP\" or \"UDP\"")
	}

	// TODO: check Address

	return nil
}

func RuleListCheck(conf *RuleFileList) error {
	for index, rule := range *conf {
		err := DohRuleCheck(rule)
		if err != nil {
			return fmt.Errorf("DohRule: %d, %v", index, err)
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

func DohConfCheck(conf DohConfFile) error {
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

func ruleConvert(ruleFile DohRuleFile) (DohRule, error) {
	rule := DohRule{}

	cond, err := condition.Build(ruleFile.Cond)
	if err != nil {
		return rule, err
	}

	rule.Cond = cond
	rule.Net = ruleFile.Net
	rule.Address = ruleFile.Address

	return rule, nil
}

func ruleListConvert(ruleFileList *RuleFileList) (*RuleList, error) {
	ruleList := new(RuleList)
	*ruleList = make([]DohRule, 0)

	for _, ruleFile := range *ruleFileList {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return ruleList, err
		}
		*ruleList = append(*ruleList, rule)
	}

	return ruleList, nil
}

func DohConfLoad(filename string) (DohConf, error) {
	var conf DohConf

	file, err := os.Open(filename)
	if err != nil {
		return conf, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	var config DohConfFile
	err = decoder.Decode(&config)
	if err != nil {
		return conf, err
	}

	err = DohConfCheck(config)
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
