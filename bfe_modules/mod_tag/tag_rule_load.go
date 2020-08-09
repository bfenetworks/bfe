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

package mod_tag

import (
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type TagRuleFile struct {
	Version string             // version
	Config  ProductRuleRawList // product -> raw rule list
}

type TagRuleConf struct {
	Version string          // version
	Config  ProductRuleList // product -> rule list
}

type TagRuleRaw struct {
	Cond  string   // condition
	Param TagParam // tag param
	Last  bool     // if true, not to check the next rule in the list if the condition is satisfied
}

type ProductRuleRawList map[string]RuleRawList // product => raw rule list
type RuleRawList []TagRuleRaw

func TagRuleCheck(tagRuleFile *TagRuleFile) error {
	if tagRuleFile == nil {
		return fmt.Errorf("tagRuleFile is nil")
	}

	if len(tagRuleFile.Version) == 0 {
		return fmt.Errorf("no Version")
	}

	if tagRuleFile.Config == nil {
		return fmt.Errorf("no Config")
	}

	return nil
}

func ruleConvert(rawRule TagRuleRaw) (*TagRule, error) {
	cond, err := condition.Build(rawRule.Cond)
	if err != nil {
		return nil, err
	}

	if len(rawRule.Param.TagName) == 0 {
		return nil, fmt.Errorf("TagName may be empty")
	}

	if len(rawRule.Param.TagValue) == 0 {
		return nil, fmt.Errorf("TagValue may be empty")
	}

	var rule TagRule
	rule.Cond = cond
	rule.Param = rawRule.Param
	rule.Last = rawRule.Last

	return &rule, nil
}

func ruleListConvert(rawRuleList RuleRawList) (TagRuleList, error) {
	ruleList := TagRuleList{}
	for i, rawRule := range rawRuleList {
		rule, err := ruleConvert(rawRule)
		if err != nil {
			return nil, fmt.Errorf("rule [%d] error: %v", i, err)
		}

		ruleList = append(ruleList, *rule)
	}

	return ruleList, nil
}

func TagRuleFileLoad(filename string) (*TagRuleConf, error) {
	var tagRuleFile TagRuleFile
	var tagRuleConf TagRuleConf

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	err = decoder.Decode(&tagRuleFile)
	if err != nil {
		return nil, err
	}

	err = TagRuleCheck(&tagRuleFile)
	if err != nil {
		return nil, err
	}

	tagRuleConf.Version = tagRuleFile.Version
	tagRuleConf.Config = make(ProductRuleList)

	for product, ruleFileList := range tagRuleFile.Config {
		ruleList, err := ruleListConvert(ruleFileList)
		if err != nil {
			return nil, fmt.Errorf("product[%s] rule error: %v", product, err)
		}
		tagRuleConf.Config[product] = ruleList
	}

	return &tagRuleConf, nil
}
