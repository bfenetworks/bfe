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

package mod_redirect

import (
	"errors"
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type RedirectRuleFile struct {
	Cond    *string         // condition for redirect
	Actions *ActionFileList // list of actions
	Status  *int            // redirect code
}

type RedirectRule struct {
	Cond    condition.Condition // condition for redirect
	Actions []Action            // list of actions
	Status  int                 // redirect code
}

type RuleFileList []RedirectRuleFile
type RuleList []RedirectRule

type ProductRulesFile map[string]*RuleFileList // product => list of redirect rules
type ProductRules map[string]*RuleList         // product => list of redirect rules

type RedirectConfFile struct {
	Version *string // version of the config
	Config  *ProductRulesFile
}

type redirectConf struct {
	Version string       // version of the config
	Config  ProductRules // product rules for redirect
}

func redirectRuleCheck(conf RedirectRuleFile) error {
	var err error

	// check Cond
	if conf.Cond == nil {
		return errors.New("no Cond")
	}

	// check Actions
	if conf.Actions == nil || len(*conf.Actions) == 0 {
		return errors.New("no Actions")
	}

	err = ActionFileListCheck(conf.Actions)
	if err != nil {
		return fmt.Errorf("Actions:%s", err.Error())
	}

	// check redirect code
	if conf.Status == nil || *conf.Status == 0 {
		return fmt.Errorf("Status: redirect code not provided")
	}

	return nil
}

func RuleListCheck(conf *RuleFileList) error {
	for index, rule := range *conf {
		err := redirectRuleCheck(rule)
		if err != nil {
			return fmt.Errorf("redirectRule:%d, %s", index, err.Error())
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

func RedirectConfCheck(conf RedirectConfFile) error {
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

func ruleConvert(ruleFile RedirectRuleFile) (RedirectRule, error) {
	rule := RedirectRule{}

	cond, err := condition.Build(*ruleFile.Cond)
	if err != nil {
		return rule, err
	}
	rule.Cond = cond

	rule.Actions = actionsConvert(*ruleFile.Actions)
	rule.Status = *ruleFile.Status
	return rule, nil
}

func ruleListConvert(ruleFileList *RuleFileList) (*RuleList, error) {
	ruleList := new(RuleList)
	*ruleList = make([]RedirectRule, 0)

	for _, ruleFile := range *ruleFileList {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return ruleList, err
		}
		*ruleList = append(*ruleList, rule)
	}

	return ruleList, nil
}

// redirectConfLoad loads config of redirect from file.
func redirectConfLoad(filename string) (redirectConf, error) {
	var conf redirectConf
	var err error

	/* open the file    */
	file, err1 := os.Open(filename)

	if err1 != nil {
		return conf, err1
	}

	/* decode the file  */
	decoder := json.NewDecoder(file)

	var config RedirectConfFile
	err = decoder.Decode(&config)
	file.Close()

	if err != nil {
		return conf, err
	}

	// check config
	err = RedirectConfCheck(config)
	if err != nil {
		return conf, err
	}

	/* convert config   */
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
