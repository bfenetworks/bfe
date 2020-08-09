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

// load route table from json file

package route_rule_conf

import (
	"errors"
	"fmt"
	"os"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

// RouteRule is composed by a condition and cluster to serve
type RouteRule struct {
	Cond        condition.Condition
	ClusterName string
}

type RouteRuleFile struct {
	Cond        *string
	ClusterName *string
}

// RouteRules is a list of rule.
type RouteRules []RouteRule
type RouteRuleFiles []RouteRuleFile

// ProductRouteRule holds mapping from product to rules.
type ProductRouteRule map[string]RouteRules
type ProductRouteRuleFile map[string]RouteRuleFiles

type RouteTableFile struct {
	Version     *string               // version of the config
	ProductRule *ProductRouteRuleFile // product => rules
}

type RouteTableConf struct {
	Version string // version of the config
	RuleMap ProductRouteRule
}

func convert(fileConf *RouteTableFile) (*RouteTableConf, error) {
	conf := &RouteTableConf{
		RuleMap: make(ProductRouteRule),
	}

	if fileConf.Version == nil {
		return nil, errors.New("no Version")
	}

	if fileConf.ProductRule == nil {
		return nil, errors.New("no product rule")
	}

	conf.Version = *fileConf.Version

	for product, ruleFiles := range *fileConf.ProductRule {
		rules := make(RouteRules, len(ruleFiles))
		for i, ruleFile := range ruleFiles {
			if ruleFile.ClusterName == nil {
				return nil, errors.New("no cluster name")
			}

			if ruleFile.Cond == nil {
				return nil, errors.New("no cond")
			}

			rules[i].ClusterName = *ruleFile.ClusterName
			cond, err := condition.Build(*ruleFile.Cond)
			if err != nil {
				return nil, fmt.Errorf("error build [%s] [%s]", *ruleFile.Cond, err)
			}
			rules[i].Cond = cond
		}

		conf.RuleMap[product] = rules
	}

	return conf, nil
}

func (conf *RouteTableConf) LoadAndCheck(filename string) (string, error) {
	var fileConf RouteTableFile

	// open file
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// decode the file
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&fileConf); err != nil {
		return "", err
	}

	pConf, err := convert(&fileConf)
	if err != nil {
		return "", err
	}

	*conf = *pConf

	return conf.Version, nil
}

// RouteConfLoad loads config of route table from file.
func RouteConfLoad(filename string) (*RouteTableConf, error) {
	var conf RouteTableConf
	if _, err := conf.LoadAndCheck(filename); err != nil {
		return nil, err
	}

	return &conf, nil
}
