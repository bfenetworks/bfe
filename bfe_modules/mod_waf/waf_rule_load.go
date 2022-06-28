// Copyright (c) 2020 The BFE Authors.
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
package mod_waf

import (
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_modules/mod_waf/waf_rule"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type wafRule struct {
	Cond       condition.Condition
	BlockRules []string
	CheckRules []string
}

type ruleList []*wafRule

type productWafRule map[string]*ruleList

type productWafRuleConfig struct {
	Version string
	Config  productWafRule
}

type wafRuleFile struct {
	Cond       string
	BlockRules []string
	CheckRules []string
}

type ruleListFile []*wafRuleFile

type productWafRuleFile map[string]*ruleListFile

type productWafRuleConfigFile struct {
	Version *string
	Config  *productWafRuleFile
}

func wafRuleConvert(ruleFile *wafRuleFile) (*wafRule, error) {
	if ruleFile == nil {
		return nil, fmt.Errorf("wafRuleConvert(), err= empty ruleFile")
	}
	var rule wafRule
	var err error
	rule.Cond, err = condition.Build(ruleFile.Cond)
	if err != nil {
		return nil, err
	}
	rule.BlockRules = make([]string, len(ruleFile.BlockRules))
	rule.CheckRules = make([]string, len(ruleFile.CheckRules))

	copy(rule.BlockRules, ruleFile.BlockRules)
	copy(rule.CheckRules, ruleFile.CheckRules)
	return &rule, nil
}

func productWafRuleConvert(prf *productWafRuleFile) (productWafRule, error) {
	wr := make(productWafRule)
	if prf == nil {
		return nil, fmt.Errorf("ruleConvert(), err= empty productWafRuleFile")
	}

	for product, fruleList := range *prf {
		rlist := make(ruleList, 0)
		for _, frule := range *fruleList {
			rule, err := wafRuleConvert(frule)
			if err != nil {
				return nil, fmt.Errorf("ruleConvert(), err=%s", err)
			}
			rlist = append(rlist, rule)
		}
		wr[product] = &rlist
	}
	return wr, nil
}

func wafRuleFileCheck(conf *wafRuleFile) error {
	if conf == nil {
		return fmt.Errorf("wafRuleFileCheck(), err=nil config")
	}
	if len(conf.Cond) == 0 {
		return fmt.Errorf("wafRuleFileCheck(), err=empty cond")
	}
	if len(conf.BlockRules) == 0 && len(conf.CheckRules) == 0 {
		return fmt.Errorf("wafRuleFileCheck(), err=block rules and check rule both empty")
	}
	if len(conf.BlockRules) != 0 {
		for _, rule := range conf.BlockRules {
			if !waf_rule.IsValidRule(rule) {
				return fmt.Errorf("wafRuleFileCheck(), err:= unknow rule %s", rule)
			}
		}
	}
	if len(conf.CheckRules) != 0 {
		for _, rule := range conf.CheckRules {
			if !waf_rule.IsValidRule(rule) {
				return fmt.Errorf("wafRuleFileCheck(), err:= unknow rule %s", rule)
			}
		}
	}
	return nil
}

func ruleListFileCheck(conf *ruleListFile) error {
	if conf == nil {
		return fmt.Errorf("ruleListFileCheck(), err=nil config")
	}
	for index, rule := range *conf {
		if err := wafRuleFileCheck(rule); err != nil {
			return fmt.Errorf("ruleListFileCheck(), err=%d, %s", index, err)
		}
	}
	return nil
}

func productWafRuleFileCheck(conf *productWafRuleFile) error {
	if conf == nil {
		return fmt.Errorf("productWafRuleFileCheck(), err=nil config")
	}
	for product, ruleList := range *conf {
		if ruleList == nil {
			return fmt.Errorf("productWafRuleFileCheck(), err=product[%s] has empty rulelist", product)
		}
		err := ruleListFileCheck(ruleList)
		if err != nil {
			return err
		}
	}
	return nil
}

func productWafRuleConfFileCheck(conf *productWafRuleConfigFile) error {
	if conf == nil {
		return fmt.Errorf("productWafRuleConfFileCheck(), err=nil config")
	}

	if conf.Version == nil {
		return fmt.Errorf("productWafRuleConfFileCheck(), err=no version")
	}
	if conf.Config == nil {
		return fmt.Errorf("productWafRuleConfFileCheck(), err=no Config")
	}

	return productWafRuleFileCheck(conf.Config)
}

func ProductWafRuleConfLoad(fileName string) (productWafRuleConfig, error) {
	var conf productWafRuleConfig
	var fileConf productWafRuleConfigFile

	f, err := os.Open(fileName)
	if err != nil {
		return conf, fmt.Errorf("ProductWafRuleConfLoad(), err=%s", err)
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(&fileConf)
	if err != nil {
		return conf, fmt.Errorf("ProductWafRuleConfLoad(), err=%s", err)
	}
	err = productWafRuleConfFileCheck(&fileConf)
	if err != nil {
		return conf, fmt.Errorf("ProductWafRuleConfLoad(), err=%s", err)
	}
	pwr, err := productWafRuleConvert(fileConf.Config)
	if err != nil {
		return conf, fmt.Errorf("ProductWafRuleConfLoad(), err=%s", err)
	}
	conf.Version = *fileConf.Version
	conf.Config = pwr
	return conf, nil
}
