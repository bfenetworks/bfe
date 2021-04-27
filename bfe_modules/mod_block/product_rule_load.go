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

package mod_block

import (
	"errors"
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type blockRuleFile struct {
	Cond   *string     // condition for block
	Name   *string     // block rule name
	Action *ActionFile // action for block
}

type blockRule struct {
	Cond   condition.Condition // condition for block
	Name   string              // block rule name
	Action Action              // action for block
}

type blockRuleFileList []blockRuleFile
type blockRuleList []blockRule

type ProductRulesFile map[string]*blockRuleFileList // product => list of block rules
type ProductRules map[string]*blockRuleList

type productRuleConfFile struct {
	Version *string // version of the config
	Config  *ProductRulesFile
}

type productRuleConf struct {
	Version string       // version of the config
	Config  ProductRules // product rules for block
}

func blockRuleCheck(conf blockRuleFile) error {
	// check Cond
	if conf.Cond == nil {
		return errors.New("no Cond")
	}

	// check Name
	if conf.Name == nil {
		return errors.New("no Name")
	}

	// check Actions
	if conf.Action == nil {
		return errors.New("no Action")
	}

	if err := ActionFileCheck(conf.Action); err != nil {
		return fmt.Errorf("Action:%s", err.Error())
	}

	return nil
}

func blockRuleListCheck(conf *blockRuleFileList) error {
	ruleNameMap := make(map[string]bool)
	for index, rule := range *conf {
		err := blockRuleCheck(rule)
		if err != nil {
			return fmt.Errorf("blockRule:%d, %s", index, err.Error())
		}

		// check rule name for one product
		if _, ok := ruleNameMap[*rule.Name]; ok {
			return fmt.Errorf("blockRule:%d, two rules have same name[%s]!", index, *rule.Name)
		}
		ruleNameMap[*rule.Name] = true
	}

	return nil
}

func productRulesCheck(conf *ProductRulesFile) error {
	for product, ruleList := range *conf {
		if ruleList == nil {
			return fmt.Errorf("no blockRuleList for product:%s", product)
		}

		err := blockRuleListCheck(ruleList)
		if err != nil {
			return fmt.Errorf("ProductRules:%s, %s", product, err.Error())
		}
	}

	return nil
}

func productRuleConfCheck(conf productRuleConfFile) error {
	var err error

	// check Version
	if conf.Version == nil {
		return errors.New("no Version")
	}

	// check Config
	if conf.Config == nil {
		return errors.New("no Config")
	}

	err = productRulesCheck(conf.Config)
	if err != nil {
		return fmt.Errorf("Config:%s", err.Error())
	}

	return nil
}

func ruleConvert(ruleFile blockRuleFile) (blockRule, error) {
	rule := blockRule{}

	cond, err := condition.Build(*ruleFile.Cond)
	if err != nil {
		return rule, err
	}
	rule.Cond = cond
	rule.Name = *ruleFile.Name
	rule.Action = actionConvert(*ruleFile.Action)
	return rule, nil
}

func ruleListConvert(ruleFileList *blockRuleFileList) (*blockRuleList, error) {
	ruleList := new(blockRuleList)
	*ruleList = make([]blockRule, 0)

	for _, ruleFile := range *ruleFileList {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return nil, err
		}
		*ruleList = append(*ruleList, rule)
	}

	return ruleList, nil
}

// ProductRuleConfLoad load block rule config from file.
func ProductRuleConfLoad(filename string) (productRuleConf, error) {
	var conf productRuleConf
	var err error

	// open the file
	file, err := os.Open(filename)
	if err != nil {
		return conf, err
	}
	defer file.Close()

	// decode the file
	decoder := json.NewDecoder(file)
	var config productRuleConfFile
	err = decoder.Decode(&config)
	if err != nil {
		return conf, err
	}

	// check config
	err = productRuleConfCheck(config)
	if err != nil {
		return conf, err
	}

	// convert config
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
