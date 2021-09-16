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
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

const (
	AdvancedMode = "ADVANCED_MODE"
)

// BasicRouteRule is for host+path routing
// [host, path] -> cluster
type BasicRouteRule struct {
	Hostname    []string
	Path        []string
	ClusterName string
}

// AdvancedRouteRule is composed by a condition and cluster to serve
type AdvancedRouteRule struct {
	Cond        condition.Condition
	ClusterName string
}

type BasicRouteRuleFile struct {
	Hostname    []string
	Path        []string
	ClusterName *string
}

type AdvancedRouteRuleFile struct {
	Cond        *string
	ClusterName *string
}

type BasicRouteRules []BasicRouteRule
type AdvancedRouteRules []AdvancedRouteRule

type BasicRouteRuleFiles []BasicRouteRuleFile
type AdvancedRouteRuleFiles []AdvancedRouteRuleFile

type ProductBasicRouteRule map[string]BasicRouteRules
type ProductBasicRouteTree map[string]*BasicRouteRuleTree
type ProductAdvancedRouteRule map[string]AdvancedRouteRules

type ProductAdvancedRouteRuleFile map[string]AdvancedRouteRuleFiles
type ProductBasicRouteRuleFile map[string]BasicRouteRuleFiles

type RouteTableFile struct {
	Version *string // version of the config

	// product => rules (basic rule)
	BasicRule *ProductBasicRouteRuleFile

	// product => rules (advanced rule)
	ProductRule *ProductAdvancedRouteRuleFile
}

type RouteTableConf struct {
	Version         string // version of the config
	BasicRuleMap    ProductBasicRouteRule
	BasicRuleTree   ProductBasicRouteTree
	AdvancedRuleMap ProductAdvancedRouteRule
}

func Convert(fileConf *RouteTableFile) (*RouteTableConf, error) {
	if fileConf.Version == nil {
		return nil, errors.New("no Version")
	}

	if fileConf.BasicRule == nil && fileConf.ProductRule == nil {
		return nil, errors.New("no product rule")
	}

	// convert basic rule
	productBasicRules, productRuleTree, err := convertBasicRule(fileConf.BasicRule)
	if err != nil {
		return nil, err
	}

	// convert advanced rule
	productAdvancedRules, err := convertAdvancedRule(fileConf.ProductRule)
	if err != nil {
		return nil, err
	}

	conf := &RouteTableConf{
		BasicRuleMap:    productBasicRules,
		BasicRuleTree:   productRuleTree,
		AdvancedRuleMap: productAdvancedRules,
	}

	conf.Version = *fileConf.Version

	return conf, nil
}

func convertAdvancedRule(ProductRule *ProductAdvancedRouteRuleFile) (ProductAdvancedRouteRule, error) {
	productRules := make(ProductAdvancedRouteRule)
	if ProductRule == nil {
		return productRules, nil
	}

	// convert advanced rule
	for product, ruleFiles := range *ProductRule {
		rules := make(AdvancedRouteRules, len(ruleFiles))
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

		productRules[product] = rules
	}
	return productRules, nil
}

func convertBasicRule(ProductRule *ProductBasicRouteRuleFile) (ProductBasicRouteRule, ProductBasicRouteTree, error) {
	productRuleMap := make(ProductBasicRouteRule)
	productRuleTree := make(ProductBasicRouteTree)

	if ProductRule == nil {
		return productRuleMap, productRuleTree, nil
	}

	for product, ruleFiles := range *ProductRule {
		ruleTrees := NewBasicRouteRuleTree()
		ruleList := make(BasicRouteRules, len(ruleFiles))

		for i, ruleFile := range ruleFiles {

			if ruleFile.ClusterName == nil {
				return nil, nil, fmt.Errorf("no cluster name in basic route rule for (%s, %d)", product, i)
			}

			if len(ruleFile.Hostname) == 0 && len(ruleFile.Path) == 0 {
				return nil, nil, fmt.Errorf("no hostname or path in basic route rule for (%s, %d)", product, i)
			}

			for _, host := range ruleFile.Hostname {
				if err := checkHostInBasicRule(host); err != nil {
					return nil, nil, fmt.Errorf("host[%s] is invalid for (%s, %d), err: %s ", host, product, i, err.Error())
				}
			}

			for _, path := range ruleFile.Path {
				if err := checkPathInBasicRule(path); err != nil {
					return nil, nil, fmt.Errorf("path[%s] is invalid (%s, %d), err: %s ", path, product, i, err.Error())
				}
			}

			ruleList[i].Hostname = ruleFile.Hostname
			ruleList[i].Path = ruleFile.Path
			ruleList[i].ClusterName = *ruleFile.ClusterName

			if err := ruleTrees.Insert(&ruleFile); err != nil {
				return nil, nil, err
			}
		}
		productRuleMap[product] = ruleList
		productRuleTree[product] = ruleTrees
	}

	return productRuleMap, productRuleTree, nil
}

// checkHostInBasicRule verify host's wildcard pattern
// only one * is allowed, eg: *.foo.com
func checkHostInBasicRule(host string) error {
	if host == "" {
		return errors.New("hostname is nil or empty")
	}

	if strings.Count(host, "*") > 1 {
		return errors.New("only one * is allowed in a hostname")
	}

	if strings.Count(host, "*") == 1 {
		if host != "*" && !strings.HasPrefix(host, "*.") {
			return errors.New("format error in wildcard host")
		}
	}

	return nil
}

// checkPathInBasicRule verify path's wildcard pattern
// only one * at end of path is allowed
func checkPathInBasicRule(path string) error {
	if path == "" {
		return errors.New("path is nil or empty")
	}
	if strings.Count(path, "*") > 1 {
		return errors.New("only one * is allowed in path")
	}

	if strings.Count(path, "*") == 1 && path[len(path)-1] != '*' {
		return errors.New("* must appear as last character of path")
	}
	return nil
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

	pConf, err := Convert(&fileConf)
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
