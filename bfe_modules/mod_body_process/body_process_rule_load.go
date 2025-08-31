// Copyright (c) 2025 The BFE Authors.
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

package mod_body_process

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

type ProcConf struct {
	Name string
	Params []string
}
type BodyProcessConfig struct {
	Dec  string
	Enc  string
	Proc []ProcConf // processing steps
}

func BodyProcessConfigCheck(config *BodyProcessConfig) error {
	return nil
}

type processRuleFile struct {
	Cond            *string
	RequestProcess  *BodyProcessConfig
	ResponseProcess *BodyProcessConfig
}

type processRule struct {
	Cond            condition.Condition
	RequestProcess  *BodyProcessConfig
	ResponseProcess *BodyProcessConfig
}

type processRuleFileList []processRuleFile
type processRuleList []processRule

type ProductRulesFile map[string]processRuleFileList
type ProductRules map[string]processRuleList

type productRuleConfFile struct {
	Version *string
	Config  *ProductRulesFile
}

type productRuleConf struct {
	Version string
	Config  ProductRules
}

func processRuleCheck(conf processRuleFile) error {
	if conf.Cond == nil {
		return errors.New("no Cond")
	}

	if conf.RequestProcess == nil && conf.ResponseProcess == nil {
		return errors.New("no RequestProcess or ResponseProcess")
	}

	if err := BodyProcessConfigCheck(conf.RequestProcess); err != nil {
		return err
	}

	if err := BodyProcessConfigCheck(conf.ResponseProcess); err != nil {
		return err
	}

	return nil
}

func processRuleListCheck(conf processRuleFileList) error {
	for index, rule := range conf {
		err := processRuleCheck(rule)
		if err != nil {
			return fmt.Errorf("processRule: %d, %v", index, err)
		}
	}

	return nil
}

func productRulesCheck(conf *ProductRulesFile) error {
	for product, ruleList := range *conf {
		if ruleList == nil {
			return fmt.Errorf("no tokenRuleList for product: %s", product)
		}

		err := processRuleListCheck(ruleList)
		if err != nil {
			return fmt.Errorf("ProductRules: %s, %v", product, err)
		}
	}

	return nil
}

func productRuleConfCheck(conf productRuleConfFile) error {
	var err error

	if conf.Version == nil {
		return errors.New("no Version")
	}

	if conf.Config == nil {
		return errors.New("no Config")
	}

	err = productRulesCheck(conf.Config)
	if err != nil {
		return fmt.Errorf("config: %v", err)
	}

	return nil
}

func ruleConvert(ruleFile processRuleFile) (processRule, error) {
	rule := processRule{}

	cond, err := condition.Build(*ruleFile.Cond)
	if err != nil {
		return rule, err
	}
	rule.Cond = cond
	rule.RequestProcess = ruleFile.RequestProcess
	rule.ResponseProcess = ruleFile.ResponseProcess
	return rule, nil
}

func ruleListConvert(ruleFileList processRuleFileList) (processRuleList, error) {
	var ruleList processRuleList

	for _, ruleFile := range ruleFileList {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return nil, err
		}
		ruleList = append(ruleList, rule)
	}

	return ruleList, nil
}

func ProductRuleConfLoad(filename string) (productRuleConf, error) {
	var conf productRuleConf
	var err error

	file, err := os.Open(filename)
	if err != nil {
		return conf, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var config productRuleConfFile
	err = decoder.Decode(&config)
	if err != nil {
		return conf, err
	}

	err = productRuleConfCheck(config)
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
