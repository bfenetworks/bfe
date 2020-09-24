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

package mod_errors

import (
	"errors"
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type ErrorsRuleFile struct {
	Cond    *string         // condition for errors
	Actions *ActionFileList // list of actions
}

type ErrorsRule struct {
	Cond    condition.Condition // condition for errors
	Actions []Action            // list of actions
}

type RuleFileList []ErrorsRuleFile
type RuleList []ErrorsRule

type ProductRulesFile map[string]*RuleFileList // product => list of errors rules
type ProductRules map[string]*RuleList         // product => list of errors rules

type ErrorsConfFile struct {
	Version *string // version of the config
	Config  *ProductRulesFile
}

type ErrorsConf struct {
	Version string       // version of the config
	Config  ProductRules // product rules for errors
}

// ErrorsRuleCheck check errors rule
func ErrorsRuleCheck(conf ErrorsRuleFile) error {
	var err error

	// check Cond
	if conf.Cond == nil {
		return errors.New("no Cond")
	}

	// check Actions
	if conf.Actions == nil {
		return errors.New("no Actions")
	}

	err = ActionFileListCheck(conf.Actions)
	if err != nil {
		return fmt.Errorf("Actions:%s", err.Error())
	}

	return nil
}

// RuleListCheck check RuleList
func RuleListCheck(conf *RuleFileList) error {
	for index, rule := range *conf {
		err := ErrorsRuleCheck(rule)
		if err != nil {
			return fmt.Errorf("ErrorsRule:%d, %s", index, err.Error())
		}
	}

	return nil
}

// ProductRulesCheck check ProductRules
func ProductRulesCheck(conf *ProductRulesFile) error {
	for product, ruleList := range *conf {
		if ruleList == nil {
			return fmt.Errorf("no RuleList for product:%s", product)
		}

		err := RuleListCheck(ruleList)
		if err != nil {
			return fmt.Errorf("invalid product rules:%s, %s", product, err.Error())
		}
	}

	return nil
}

// ErrorsConfCheck check ErrorsConf
func ErrorsConfCheck(conf ErrorsConfFile) error {
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

func ruleConvert(ruleFile ErrorsRuleFile) (ErrorsRule, error) {
	rule := ErrorsRule{}

	if ruleFile.Cond == nil {
		return rule, fmt.Errorf("cond not set")
	}
	cond, err := condition.Build(*ruleFile.Cond)
	if err != nil {
		return rule, err
	}

	rule.Cond = cond
	rule.Actions = actionsConvert(*ruleFile.Actions)

	return rule, nil
}

func ruleListConvert(ruleFileList *RuleFileList) (*RuleList, error) {
	ruleList := new(RuleList)
	*ruleList = make([]ErrorsRule, 0)

	for _, ruleFile := range *ruleFileList {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return ruleList, err
		}
		*ruleList = append(*ruleList, rule)
	}

	return ruleList, nil
}

// ErrorsConfLoad load errors config from file
func ErrorsConfLoad(filename string) (ErrorsConf, error) {
	var conf ErrorsConf

	// open the file
	file, err := os.Open(filename)
	if err != nil {
		return conf, err
	}
	defer file.Close()

	// decode the file
	decoder := json.NewDecoder(file)

	var config ErrorsConfFile
	err = decoder.Decode(&config)
	if err != nil {
		return conf, err
	}

	// check config
	err = ErrorsConfCheck(config)
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
