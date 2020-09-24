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
package mod_markdown

import (
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type markdownRuleFile struct {
	Cond string
}

type markdownRule struct {
	Cond *condition.Condition
}

type markdownRuleFiles []*markdownRuleFile
type markdownRules []*markdownRule

type productRulesFile map[string]*markdownRuleFiles
type productRules map[string]*markdownRules

type productRuleConfFile struct {
	Version string
	Config  *productRulesFile
}

type productRuleConf struct {
	Version string
	Config  productRules
}

func ruleConvert(ruleFile *markdownRuleFile) (*markdownRule, error) {
	if ruleFile == nil {
		return nil, fmt.Errorf("ruleConvert(): nil ruleFile")
	}
	rule := new(markdownRule)
	cond, err := condition.Build(ruleFile.Cond)
	if err != nil {
		return nil, err
	}

	rule.Cond = &cond
	return rule, nil
}

func rulesConvert(ruleFiles *markdownRuleFiles) (*markdownRules, error) {
	if ruleFiles == nil {
		return nil, fmt.Errorf("rulesConvert():nil markdownRuleFiles")
	}
	rules := new(markdownRules)
	*rules = make([]*markdownRule, 0)
	for _, ruleFile := range *ruleFiles {
		rule, err := ruleConvert(ruleFile)
		if err != nil {
			return nil, err
		}
		*rules = append(*rules, rule)
	}
	return rules, nil
}

func mdRuleCheck(rule *markdownRuleFile) error {
	if rule == nil {
		return fmt.Errorf("empty rule")
	}
	if rule.Cond == "" {
		return fmt.Errorf("empty rule cond")
	}
	return nil
}

func rulesCheck(mdRuleFiles *markdownRuleFiles) error {
	if mdRuleFiles == nil {
		return fmt.Errorf("empty rules")
	}
	for index, rule := range *mdRuleFiles {
		err := mdRuleCheck(rule)
		if err != nil {
			return fmt.Errorf("Invalid markdown rule, index[%d], err[%s]", index, err)
		}
	}
	return nil
}

func productRulesFileCheck(cfg *productRulesFile) error {
	if cfg == nil {
		return fmt.Errorf("nil product rules file")
	}
	for product, productRules := range *cfg {
		if productRules == nil {
			return fmt.Errorf("no product rules for product[%s]", product)
		}

		err := rulesCheck(productRules)
		if err != nil {
			return fmt.Errorf("check product[%s] rules err[%v]", product, err)
		}
	}
	return nil
}

func productRuleFileCheck(cfg productRuleConfFile) error {
	if cfg.Version == "" {
		return fmt.Errorf("no version")
	}

	if cfg.Config == nil {
		return fmt.Errorf("no Config")
	}
	err := productRulesFileCheck(cfg.Config)
	if err != nil {
		return fmt.Errorf("Config err[%v]", err)
	}
	return nil
}

func ProductRuleConfLoad(fileName string) (productRuleConf, error) {
	var confFile productRuleConfFile
	var conf productRuleConf

	file, err := os.Open(fileName)
	if err != nil {
		return conf, err
	}
	defer file.Close()

	err = json.NewDecoder(file).Decode(&confFile)
	if err != nil {
		return conf, err
	}
	err = productRuleFileCheck(confFile)
	if err != nil {
		return conf, err
	}

	conf.Version = confFile.Version
	conf.Config = make(productRules)

	for product, productConfFile := range *confFile.Config {
		productRules, err := rulesConvert(productConfFile)
		if err != nil {
			return conf, err
		}
		conf.Config[product] = productRules
	}
	return conf, nil
}
