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

package mod_header

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type HeaderRuleFile struct {
	Cond    *string         // condition for header
	Actions *ActionFileList // list of actions
	Last    *bool           // if true, not to check the next rule in the list if
	// the condition is satisfied
}

type HeaderRule struct {
	Cond    condition.Condition // condition for header
	Actions []Action            // list of actions
	Last    bool                // if true, not to check the next rule in the list if
	// the condition is satisfied
}

type RuleFileList []HeaderRuleFile
type RuleList []HeaderRule

type ProductRulesFile map[string]*RuleFileList // product => list of header rules
type ProductRules map[string][]*RuleList       // product => list of header rules

type HeaderConfFile struct {
	Version *string // version of the config
	Config  *ProductRulesFile
}

type HeaderConf struct {
	Version string       // version of the config
	Config  ProductRules // product rules for header
}

func HeaderRuleCheck(conf HeaderRuleFile) error {
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

	// check Last
	if conf.Last == nil {
		return errors.New("no Last")
	}

	return nil
}

func RuleListCheck(conf *RuleFileList) error {
	for index, rule := range *conf {
		err := HeaderRuleCheck(rule)
		if err != nil {
			return fmt.Errorf("HeaderRule:%d, %s", index, err.Error())
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

func HeaderConfCheck(conf HeaderConfFile) error {
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

func ruleConvert(ruleFile HeaderRuleFile) (HeaderRule, error) {
	rule := HeaderRule{}

	if ruleFile.Cond == nil {
		return rule, fmt.Errorf("cond not set")
	}
	cond, err := condition.Build(*ruleFile.Cond)
	if err != nil {
		return rule, err
	}

	rule.Cond = cond
	rule.Actions, err = actionsConvert(*ruleFile.Actions)
	if err != nil {
		return rule, err
	}
	rule.Last = *ruleFile.Last

	return rule, nil
}

func ruleListConvert(ruleFileList *RuleFileList) (*RuleList, error) {
	ruleList := new(RuleList)
	*ruleList = make([]HeaderRule, 0)

	for _, ruleFile := range *ruleFileList {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return ruleList, err
		}
		*ruleList = append(*ruleList, rule)
	}

	return ruleList, nil
}

func initRuleLists() []*RuleList {
	ruleLists := make([]*RuleList, TotalType)

	for i := 0; i < len(ruleLists); i++ {
		ruleList := make(RuleList, 0)
		ruleLists[i] = &ruleList
	}

	return ruleLists
}

func getHeaderType(cmd string) int {
	if strings.HasPrefix(cmd, "REQ_") {
		return ReqHeader
	}

	return RspHeader
}

func classifyRuleByAction(rule HeaderRule) RuleList {
	ruleList := make(RuleList, TotalType)
	for i := 0; i < len(ruleList); i++ {
		ruleList[i].Cond = rule.Cond
		ruleList[i].Last = rule.Last
	}

	for _, action := range rule.Actions {
		headerType := getHeaderType(action.Cmd)
		ruleList[headerType].Actions = append(ruleList[headerType].Actions, action)
	}

	return ruleList
}

func classifyRules(ruleList *RuleList) []*RuleList {
	ruleLists := initRuleLists()

	for _, rule := range *ruleList {
		ruleList := classifyRuleByAction(rule)

		for i := 0; i < len(ruleLists); i++ {
			if len(ruleList[i].Actions) != 0 {
				*ruleLists[i] = append(*ruleLists[i], ruleList[i])
			}
		}
	}

	return ruleLists
}

// HeaderConfLoad loads config of header from file.
func HeaderConfLoad(filename string) (HeaderConf, error) {
	var conf HeaderConf
	var err error

	/* open the file    */
	file, err1 := os.Open(filename)

	if err1 != nil {
		return conf, err1
	}

	/* decode the file  */
	decoder := json.NewDecoder(file)

	var config HeaderConfFile
	err = decoder.Decode(&config)
	file.Close()

	if err != nil {
		return conf, err
	}

	// check config
	err = HeaderConfCheck(config)
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

		conf.Config[product] = classifyRules(ruleList)
	}

	return conf, nil
}
