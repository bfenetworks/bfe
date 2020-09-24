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

package mod_compress

import (
	"errors"
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type compressRuleFile struct {
	Cond   *string
	Action *ActionFile
}

type compressRule struct {
	Cond   condition.Condition
	Action Action
}

type compressRuleFileList []compressRuleFile
type compressRuleList []compressRule

type ProductRulesFile map[string]*compressRuleFileList
type ProductRules map[string]*compressRuleList

type productRuleConfFile struct {
	Version *string
	Config  *ProductRulesFile
}

type productRuleConf struct {
	Version string
	Config  ProductRules
}

func compressRuleCheck(conf compressRuleFile) error {
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

func compressRuleListCheck(conf *compressRuleFileList) error {
	for index, rule := range *conf {
		err := compressRuleCheck(rule)
		if err != nil {
			return fmt.Errorf("compressRule: %d, %v", index, err)
		}
	}

	return nil
}

func productRulesCheck(conf *ProductRulesFile) error {
	for product, ruleList := range *conf {
		if ruleList == nil {
			return fmt.Errorf("no compressRuleList for product: %s", product)
		}

		err := compressRuleListCheck(ruleList)
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
		return fmt.Errorf("Config: %v", err)
	}

	return nil
}

func ruleConvert(ruleFile compressRuleFile) (compressRule, error) {
	rule := compressRule{}

	cond, err := condition.Build(*ruleFile.Cond)
	if err != nil {
		return rule, err
	}
	rule.Cond = cond
	rule.Action = actionConvert(*ruleFile.Action)
	return rule, nil
}

func ruleListConvert(ruleFileList *compressRuleFileList) (*compressRuleList, error) {
	ruleList := new(compressRuleList)
	*ruleList = make([]compressRule, 0)

	for _, ruleFile := range *ruleFileList {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return nil, err
		}
		*ruleList = append(*ruleList, rule)
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
