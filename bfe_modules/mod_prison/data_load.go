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

package mod_prison

import (
	"fmt"
	"regexp"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/action"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util"
)

type AccessSignConf struct {
	UseSocketIP  bool     // if true, add socketip to input of signature
	UseClientIP  bool     // if true, add clientip to input of signature
	UseConnectID bool     // if true, add connect id to input of signature
	UseUrl       bool     // if true, add url to input of signature
	UseHost      bool     // if true, add host to input of signature
	UsePath      bool     // if true, add path to input of signature
	UseHeaders   bool     // if true, use all request headers
	UrlRegexp    string   // url regmatch str
	Query        []string // query key list
	Header       []string // header key list
	Cookie       []string // cookie key list
}

type PrisonRuleConf struct {
	Cond           *string         // condition for prison rule
	Action         *action.Action  // action for prison rule
	AccessSignConf *AccessSignConf // sign conf for prison rule
	Name           *string         // rule name
	CheckPeriod    *int64          // check period in seconds
	StayPeriod     *int64          // stayPeriod in seconds
	Threshold      *int32          // threshold
	AccessDictSize *int            // size of access dict
	PrisonDictSize *int            // size of prison dict
}

type PrisonRuleConfList []*PrisonRuleConf

type ProductRuleConf struct {
	Version *string                         // version of the config
	Config  *map[string]*PrisonRuleConfList // product name => list of prison rules
}

// PrisonRuleCheck check prisonRule
func PrisonRuleCheck(conf *PrisonRuleConf) error {
	// check nil filed
	if err := bfe_util.CheckNilField(*conf, false); err != nil {
		return err
	}

	// check action
	if _, ok := allowActions[conf.Action.Cmd]; !ok {
		return fmt.Errorf("mod_prison action: [%s] not allowed", conf.Action.Cmd)
	}

	// check condition
	if _, err := condition.Build(*conf.Cond); err != nil {
		return fmt.Errorf("cond.Build(): [%s][%s]", *conf.Cond, err.Error())
	}

	// check AccessSignConf
	if len(conf.AccessSignConf.UrlRegexp) != 0 {
		urlRegexp, err := regexp.Compile(conf.AccessSignConf.UrlRegexp)
		if err != nil {
			return fmt.Errorf("regexp.Compile: %s:%s", conf.AccessSignConf.UrlRegexp, err.Error())
		}

		// check subExprNum
		subExpNum := urlRegexp.NumSubexp()
		if subExpNum <= 0 {
			return fmt.Errorf("regExpr[%s] without subexpression", conf.AccessSignConf.UrlRegexp)
		}
	}

	// check CheckPeriod
	if *conf.CheckPeriod <= 0 {
		return fmt.Errorf("checkPeriod should > 0")
	}

	// check Threshold
	if *conf.Threshold < 0 {
		return fmt.Errorf("threshold should >= 0")
	}

	// if num of requests is over conf.Threshold in check period,
	// the time duration of stay period should be :
	//    conf.StayPeriod + conf.CheckPeriod - SecondsElapsedInCheckPeriod
	if *conf.StayPeriod < 0 {
		return fmt.Errorf("stayPeriod should >= 0")
	}

	if *conf.AccessDictSize <= 0 || *conf.PrisonDictSize <= 0 {
		return fmt.Errorf("accessDictSize/prisonDictSize should > 0")
	}

	return nil
}

// PrisonRuleListCheck check prisonRuleList
func PrisonRuleListCheck(conf *PrisonRuleConfList) error {
	// create a rule map
	ruleMap := make(map[string]bool)
	for index, rule := range *conf {
		if err := PrisonRuleCheck(rule); err != nil {
			return fmt.Errorf("prisonRule:%d, %s", index, err.Error())
		}
		if _, ok := ruleMap[*rule.Name]; ok {
			return fmt.Errorf("duplicated rule name[%s]", *rule.Name)
		}
		ruleMap[*rule.Name] = true
	}

	return nil
}

// ProductRulesCheck check ProductRules
func ProductRulesCheck(conf map[string]*PrisonRuleConfList) error {
	for product, ruleList := range conf {
		if ruleList == nil {
			return fmt.Errorf("no prisonRuleList for product:%s", product)
		}
		if err := PrisonRuleListCheck(ruleList); err != nil {
			return fmt.Errorf("ProductRules:%s, %s", product, err.Error())
		}
	}

	return nil
}

func productRuleConfCheck(conf *ProductRuleConf) error {
	// check nil filed
	if err := bfe_util.CheckNilField(*conf, false); err != nil {
		return err
	}

	// check productRules
	if err := ProductRulesCheck(*(conf.Config)); err != nil {
		return fmt.Errorf("check config: %s", err.Error())
	}

	return nil
}

func productRuleConfLoad(fileName string) (ProductRuleConf, error) {
	var config ProductRuleConf

	// load json file
	if err := bfe_util.LoadJsonFile(fileName, &config); err != nil {
		return config, fmt.Errorf("LoadJsonFile() err: %s", err.Error())
	}

	// check config
	if err := productRuleConfCheck(&config); err != nil {
		return config, err
	}

	return config, nil
}
