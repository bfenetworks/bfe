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

package mod_key_log

import (
	"errors"
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type KeyLogRuleFile struct {
	Cond *string // condition for key_log
}

type KeyLogRule struct {
	Cond condition.Condition // condition for key_log
}

type RuleFileList []KeyLogRuleFile
type RuleList []KeyLogRule

type ProductRulesFile map[string]*RuleFileList // product => list of key_log rules
type ProductRules map[string]*RuleList         // product => list of key_log rules

type KeyLogConfFile struct {
	Version *string // version of the config
	Config  *ProductRulesFile
}

type keyLogConf struct {
	Version string       // version of the config
	Config  ProductRules // product rules for key_log
}

func keyLogRuleCheck(conf KeyLogRuleFile) error {
	// check Cond
	if conf.Cond == nil {
		return errors.New("no Cond")
	}

	return nil
}

func RuleListCheck(conf *RuleFileList) error {
	for index, rule := range *conf {
		err := keyLogRuleCheck(rule)
		if err != nil {
			return fmt.Errorf("keyLogRule:%d, %s", index, err.Error())
		}
	}

	return nil
}

func ProductRulesCheck(conf *ProductRulesFile) error {
	for product, ruleList := range *conf {
		if ruleList == nil {
			return fmt.Errorf("no RuleList for product:%s", product)
		}

		err := RuleListCheck(ruleList)
		if err != nil {
			return fmt.Errorf("ProductRules:%s, %s", product, err.Error())
		}
	}

	return nil
}

func KeyLogConfCheck(conf KeyLogConfFile) error {
	var err error

	// check Version
	if conf.Version == nil {
		return errors.New("no Version")
	}

	// check Config
	if conf.Config == nil {
		return errors.New("no Config")
	}

	err = ProductRulesCheck(conf.Config)
	if err != nil {
		return fmt.Errorf("Config:%s", err.Error())
	}

	return nil
}

func ruleConvert(ruleFile KeyLogRuleFile) (KeyLogRule, error) {
	rule := KeyLogRule{}

	cond, err := condition.Build(*ruleFile.Cond)
	if err != nil {
		return rule, err
	}
	rule.Cond = cond

	return rule, nil
}

func ruleListConvert(ruleFileList *RuleFileList) (*RuleList, error) {
	ruleList := new(RuleList)
	*ruleList = make([]KeyLogRule, 0)

	for _, ruleFile := range *ruleFileList {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return ruleList, err
		}
		*ruleList = append(*ruleList, rule)
	}

	return ruleList, nil
}

// keyLogConfLoad loads config of key_log from file.
func keyLogConfLoad(filename string) (keyLogConf, error) {
	var conf keyLogConf
	var err error

	// open the file
	file, err1 := os.Open(filename)
	if err1 != nil {
		return conf, err1
	}
	defer file.Close()

	// decode the file
	decoder := json.NewDecoder(file)
	var config KeyLogConfFile
	err = decoder.Decode(&config)
	if err != nil {
		return conf, err
	}

	// check config
	err = KeyLogConfCheck(config)
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
