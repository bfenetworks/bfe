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

package mod_secure_link

import (
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

// RuleFile define how to validate secure link
type RuleFile struct {
	Cond *string

	ChecksumKey     *string
	ExpiresKey      *string
	ExpressionNodes []ExpressionNodeFile
}

type Rule struct {
	Cond condition.Condition // condition for header

	ChecksumKey string
	ExpiresKey  string

	Expression *Expression

	Checker *Checker
}

type ExpressionNodeFile struct {
	Type  string
	Param string
}

func (rule *Rule) Check(req *bfe_basic.Request) error {
	return rule.Checker.Check(req)
}

type ProductRulesFile map[string][]*RuleFile
type ProductRules map[string][]*Rule

type DataFile struct {
	Version *string // version of the config
	Config  ProductRulesFile
}

type Data struct {
	Version string       // version of the config
	Config  ProductRules // product rules for header
}

// DataLoad loads config of header from file.
func DataLoad(filename string) (*Data, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	cf := &DataFile{}
	err = decoder.Decode(cf)
	if err != nil {
		return nil, err
	}

	return NewData(cf)
}

func NewData(cf *DataFile) (*Data, error) {
	if cf.Version == nil {
		return nil, fmt.Errorf("bad Version node")
	}

	if cf.Config == nil {
		return nil, fmt.Errorf("bad Config node")
	}

	conf := &Data{
		Version: *cf.Version,
		Config:  make(map[string][]*Rule, len(cf.Config)),
	}

	for product, ruleFiles := range cf.Config {
		rules := make([]*Rule, len(ruleFiles))
		for i, ruleFile := range ruleFiles {
			if ruleFile == nil {
				return nil, fmt.Errorf("bad Config[%d] node", i)
			}

			rule, err := NewRule(ruleFile)
			if err != nil {
				return nil, fmt.Errorf("bad Config[%d] node: %v", i, err)
			}
			rules[i] = rule
		}
		conf.Config[product] = rules
	}

	return conf, nil
}

// NewRule get Rule by RuleFile
func NewRule(rf *RuleFile) (*Rule, error) {
	if rf.Cond == nil {
		return nil, fmt.Errorf("bad Cond node")
	}
	cond, err := condition.Build(*rf.Cond)
	if err != nil {
		return nil, err
	}

	rule := &Rule{
		Cond:        cond,
		ChecksumKey: "md5",
	}

	if val := rf.ChecksumKey; val != nil {
		rule.ChecksumKey = *val
		if rule.ChecksumKey == "" {
			return nil, fmt.Errorf("bad ChecksumKey node: empty string")
		}
	}
	if val := rf.ExpiresKey; val != nil {
		rule.ExpiresKey = *val
	}

	if rf.ExpressionNodes == nil {
		return nil, fmt.Errorf("bad SecureLink node")
	}
	rule.Checker, err = NewChecker(&CheckerConfig{
		ExpressionNodes: rf.ExpressionNodes,
		ChecksumKey:     rule.ChecksumKey,
		ExpiresKey:      rule.ExpiresKey,
	})

	if err != nil {
		return nil, fmt.Errorf("bad SecureLink node, err: %v", err)
	}

	return rule, nil
}
