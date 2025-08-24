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

package mod_ai_token_auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

type tokenRuleFile struct {
        Cond   *string
        Action *ActionFile
}

type tokenRule struct {
        Cond   condition.Condition
        Action Action
}

type tokenFileMap map[string]*TokenFile
type tokenMap map[string]*Token

type ProductTokenFiles map[string]*tokenFileMap
type ProductTokens map[string]*tokenMap

type tokenRuleFileList []tokenRuleFile
type tokenRuleList []tokenRule

type ProductRulesFile map[string]*tokenRuleFileList
type ProductRules map[string]*tokenRuleList

type productRuleConfFile struct {
	Version *string
	Tokens  *ProductTokenFiles
	Config  *ProductRulesFile
}

type productRuleConf struct {
	Version string
	Tokens  ProductTokens
	Config  ProductRules
}

func tokenMapCheck(conf *tokenFileMap) error {
	if conf == nil {
		return errors.New("no tokenMap")
	}

	for key, token := range *conf {
		if err := tokenCheck(token); err != nil {
			return fmt.Errorf("token %s: %v", key, err)
		}
	}

	return nil
}
func productTokensCheck(conf *ProductTokenFiles) error {
	for product, tokenMap := range *conf {
		if err := tokenMapCheck(tokenMap); err != nil {
			return fmt.Errorf("ProductTokens %s: %v", product, err)
		}
	}

	return nil
}

func tokenRuleCheck(conf tokenRuleFile) error {
	if conf.Cond == nil {
		return errors.New("no Cond")
	}

	if conf.Action == nil {
		return errors.New("no Action")
	}
	if err := ActionFileCheck(conf.Action); err != nil {
		return err
	}

	return nil
}

func tokenRuleListCheck(conf *tokenRuleFileList) error {
	for index, rule := range *conf {
		err := tokenRuleCheck(rule)
		if err != nil {
			return fmt.Errorf("tokenRule: %d, %v", index, err)
		}
	}

	return nil
}

func productRulesCheck(conf *ProductRulesFile) error {
	for product, ruleList := range *conf {
		if ruleList == nil {
			return fmt.Errorf("no tokenRuleList for product: %s", product)
		}

		err := tokenRuleListCheck(ruleList)
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

	if conf.Tokens == nil {
		return errors.New("no Tokens")
	}

	err = productTokensCheck(conf.Tokens)
	if err != nil {
		return fmt.Errorf("tokens: %v", err)
	}

	err = productRulesCheck(conf.Config)
	if err != nil {
		return fmt.Errorf("config: %v", err)
	}

	return nil
}

func ruleConvert(ruleFile tokenRuleFile) (tokenRule, error) {
	rule := tokenRule{}

	cond, err := condition.Build(*ruleFile.Cond)
	if err != nil {
		return rule, err
	}
	rule.Cond = cond
	rule.Action = actionConvert(*ruleFile.Action)
	return rule, nil
}

func ruleListConvert(ruleFileList *tokenRuleFileList) (*tokenRuleList, error) {
	var ruleList tokenRuleList

	for _, ruleFile := range *ruleFileList {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return nil, err
		}
		ruleList = append(ruleList, rule)
	}

	return &ruleList, nil
}

func tokenMapConvert(tokenFileMap *tokenFileMap) (*tokenMap, error) {
	tokenMap := make(tokenMap)

	for key, tokenFile := range *tokenFileMap {
		token := tokenConvert(*tokenFile)
		tokenMap[key] = &token
	}

	return &tokenMap, nil
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

	conf.Tokens = make(ProductTokens)
	if config.Tokens != nil {
		for product, tokenMap := range *config.Tokens {
			tokenMap, err := tokenMapConvert(tokenMap)
			if err != nil {
				return conf, err
			}
			conf.Tokens[product] = tokenMap
		}
	}

	return conf, nil
}
