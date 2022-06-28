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

package mod_auth_request

import (
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type AuthRequestRuleFile struct {
	Version string
	Config  ProductRuleRawList // product => raw rule list
}

type AuthRequestRuleConf struct {
	Version string
	Config  ProductRuleList // product => rule list
}

type AuthRequestRuleRaw struct {
	Cond   string // condition
	Enable bool   // whether enable auth request
}

type ProductRuleRawList map[string]RuleRawList // product => raw rule list
type RuleRawList []AuthRequestRuleRaw          // raw rule list

func AuthRequestRuleCheck(authRequestRuleFile *AuthRequestRuleFile) error {
	if authRequestRuleFile == nil {
		return fmt.Errorf("authRequestRuleFile is nil")
	}

	if len(authRequestRuleFile.Version) == 0 {
		return fmt.Errorf("no Version")
	}

	if authRequestRuleFile.Config == nil {
		return fmt.Errorf("no Config")
	}

	return nil
}

func ruleConvert(rawRule AuthRequestRuleRaw) (*AuthRequestRule, error) {
	cond, err := condition.Build(rawRule.Cond)
	if err != nil {
		return nil, err
	}

	var rule AuthRequestRule
	rule.Cond = cond
	rule.Enable = rawRule.Enable

	return &rule, nil
}

func ruleListConvert(rawRuleList RuleRawList) (AuthRequestRuleList, error) {
	ruleList := AuthRequestRuleList{}
	for i, rawRule := range rawRuleList {
		rule, err := ruleConvert(rawRule)
		if err != nil {
			return nil, fmt.Errorf("rule [%d] error: %v", i, err)
		}

		ruleList = append(ruleList, *rule)
	}

	return ruleList, nil
}

func AuthRequestRuleFileLoad(filename string) (*AuthRequestRuleConf, error) {
	var ruleFile AuthRequestRuleFile
	var ruleConf AuthRequestRuleConf

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	err = decoder.Decode(&ruleFile)
	if err != nil {
		return nil, err
	}

	err = AuthRequestRuleCheck(&ruleFile)
	if err != nil {
		return nil, err
	}

	ruleConf.Version = ruleFile.Version
	ruleConf.Config = make(ProductRuleList)

	for product, ruleFileList := range ruleFile.Config {
		ruleList, err := ruleListConvert(ruleFileList)
		if err != nil {
			return nil, fmt.Errorf("product[%s] rule error: %v", product, err)
		}
		ruleConf.Config[product] = ruleList
	}

	return &ruleConf, nil
}
