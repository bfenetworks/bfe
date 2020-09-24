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

package mod_auth_basic

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type AuthBasicRuleFile struct {
	Cond     string
	UserFile string
	Realm    string
}

type AuthBasicRule struct {
	Cond       condition.Condition
	UserPasswd map[string]string
	Realm      string
}

type RuleFileList []AuthBasicRuleFile
type RuleList []AuthBasicRule

type ProductRulesFile map[string]*RuleFileList
type ProductRules map[string]*RuleList

type AuthBasicConfFile struct {
	Version *string
	Config  *ProductRulesFile
}

type AuthBasicConf struct {
	Version string
	Config  ProductRules
}

func readUserFile(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	userPasswd := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.Trim(scanner.Text(), " \t")
		if len(line) == 0 || strings.Contains(line, "#") {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) != 2 && len(parts) != 3 {
			return nil, fmt.Errorf("Format error, \"%s\".", line)
		}

		userPasswd[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	return userPasswd, nil
}

func AuthBasicRuleCheck(conf AuthBasicRuleFile) error {
	if len(conf.Cond) == 0 {
		return fmt.Errorf("Cond empty.")
	}

	if len(conf.UserFile) == 0 {
		return fmt.Errorf("UserFile empty.")
	}

	return nil
}

func RuleListCheck(conf *RuleFileList) error {
	for index, rule := range *conf {
		err := AuthBasicRuleCheck(rule)
		if err != nil {
			return fmt.Errorf("AuthBasicRule: %d, %v", index, err)
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

func AuthBasicConfCheck(conf AuthBasicConfFile) error {
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

func ruleConvert(ruleFile AuthBasicRuleFile) (AuthBasicRule, error) {
	rule := AuthBasicRule{}

	cond, err := condition.Build(ruleFile.Cond)
	if err != nil {
		return rule, err
	}

	rule.Cond = cond
	rule.UserPasswd, err = readUserFile(ruleFile.UserFile)
	if err != nil {
		return rule, err
	}
	rule.Realm = ruleFile.Realm
	if len(rule.Realm) == 0 {
		rule.Realm = "Restricted"
	}

	return rule, nil
}

func ruleListConvert(ruleFileList *RuleFileList) (*RuleList, error) {
	ruleList := new(RuleList)
	*ruleList = make([]AuthBasicRule, 0)

	for _, ruleFile := range *ruleFileList {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return ruleList, err
		}
		*ruleList = append(*ruleList, rule)
	}

	return ruleList, nil
}

func AuthBasicConfLoad(filename string) (AuthBasicConf, error) {
	var conf AuthBasicConf

	file, err := os.Open(filename)
	if err != nil {
		return conf, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	var config AuthBasicConfFile
	err = decoder.Decode(&config)
	if err != nil {
		return conf, err
	}

	err = AuthBasicConfCheck(config)
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
