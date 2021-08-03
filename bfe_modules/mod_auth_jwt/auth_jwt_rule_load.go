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

package mod_auth_jwt

import (
	"fmt"
	"io/ioutil"
	"os"
)

import (
	"github.com/golang-jwt/jwt"
	jose "gopkg.in/square/go-jose.v2"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type AuthJWTRuleFile struct {
	Cond    string
	KeyFile string // JSON Web Key file
	Realm   string // security realm
}

type keyProvider struct {
	key *jose.JSONWebKey
}

func (p *keyProvider) provideKey(token *jwt.Token) (interface{}, error) {
	return p.key.Key, nil
}

type AuthJWTRule struct {
	Cond  condition.Condition
	Keys  []keyProvider
	Realm string
}

type RuleFileList []AuthJWTRuleFile
type RuleList []AuthJWTRule

type ProductRulesFile map[string]*RuleFileList
type ProductRules map[string]*RuleList

type AuthJWTConfFile struct {
	Version *string
	Config  *ProductRulesFile
}

type AuthJWTConf struct {
	Version string
	Config  ProductRules
}

// Read JSON Web Key file.
// The file must follow the format described by https://tools.ietf.org/html/rfc7517s
func readKeyFile(filename string) ([]keyProvider, error) {
	var keyProviders []keyProvider
	var keys []*jose.JSONWebKey

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return keyProviders, err
	}

	err = json.Unmarshal(data, &keys)
	if err != nil {
		return keyProviders, err
	}

	for _, key := range keys {
		keyProviders = append(keyProviders, keyProvider{key: key})
	}

	return keyProviders, nil
}

func AuthJWTRuleCheck(conf AuthJWTRuleFile) error {
	if len(conf.Cond) == 0 {
		return fmt.Errorf("Cond empty.")
	}

	if len(conf.KeyFile) == 0 {
		return fmt.Errorf("KeyFile empty.")
	}

	return nil
}

func RuleListCheck(conf *RuleFileList) error {
	for index, rule := range *conf {
		err := AuthJWTRuleCheck(rule)
		if err != nil {
			return fmt.Errorf("AuthJWTRule: %d, %v", index, err)
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

func AuthJWTConfCheck(conf AuthJWTConfFile) error {
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

func ruleConvert(ruleFile AuthJWTRuleFile) (AuthJWTRule, error) {
	rule := AuthJWTRule{}

	cond, err := condition.Build(ruleFile.Cond)
	if err != nil {
		return rule, err
	}

	rule.Cond = cond
	rule.Keys, err = readKeyFile(ruleFile.KeyFile)
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
	*ruleList = make([]AuthJWTRule, 0)

	for _, ruleFile := range *ruleFileList {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return ruleList, err
		}
		*ruleList = append(*ruleList, rule)
	}

	return ruleList, nil
}

func AuthJWTConfLoad(filename string) (AuthJWTConf, error) {
	var conf AuthJWTConf

	file, err := os.Open(filename)
	if err != nil {
		return conf, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	var config AuthJWTConfFile
	err = decoder.Decode(&config)
	if err != nil {
		return conf, err
	}

	err = AuthJWTConfCheck(config)
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
