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

package mod_degrade

import (
	"fmt"
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_bufio"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_util"
)

type degradeRuleConfFile struct {
	Cond        *string            // condition for degrade rule
	Action      *DegradeActionFile // action for degrade rule
	Name        *string            // rule name
	Enable      *bool              // check enable, if enable = true, the degradation will be opened
	DegradeRate *int               // degrade rate, for example: degradeRate=10, meaning 10% of request will be degraded
}

type degradeRuleConf struct {
	Cond        condition.Condition // condition for degrade rule
	Action      DegradeAction       // action for degrade rule
	Name        string              // rule name
	Enable      bool                // check enable?
	DegradeRate int                 // degrade rate
}

type degradeRuleConfFileList []*degradeRuleConfFile
type degradeRuleConfList []degradeRuleConf

type productRuleFile map[string]*degradeRuleConfFileList
type productRule map[string]degradeRuleConfList

type productRuleConf struct {
	Version *string     // version of the config
	Config  productRule // product name => list of degrade rules
}

type productRuleConfFile struct {
	Version *string          // version of the config
	Config  *productRuleFile // product name => list of degrade rules
}

func productRuleConfLoad(fileName string) (productRuleConf, error) {
	var (
		config productRuleConfFile
		conf   productRuleConf
		err    error
	)

	// load json file
	if err = bfe_util.LoadJsonFile(fileName, &config); err != nil {
		return conf, fmt.Errorf("LoadJsonFile() err: %s", err.Error())
	}

	// check config
	if err = productRuleConfCheck(&config); err != nil {
		return conf, err
	}

	// covert to productRuleConf
	conf.Version = config.Version
	conf.Config = make(productRule)

	for product, ruleFileList := range *config.Config {
		ruleList, err := ruleListConvert(ruleFileList)
		if err != nil {
			return conf, err
		}

		conf.Config[product] = ruleList

	}

	return conf, nil
}

func ruleListConvert(ruleFileList *degradeRuleConfFileList) (degradeRuleConfList, error) {
	ruleList := make(degradeRuleConfList, 0)

	for _, ruleFile := range *ruleFileList {
		rule, err := ruleConvert(*ruleFile)
		if err != nil {
			return nil, err
		}

		ruleList = append(ruleList, *rule)

	}

	return ruleList, nil
}

func ruleConvert(ruleFile degradeRuleConfFile) (*degradeRuleConf, error) {
	var (
		rule   *degradeRuleConf
		action *DegradeAction
		err    error
	)

	cond, err := condition.Build(*ruleFile.Cond)
	if err != nil {
		return nil, err
	}

	rule = &degradeRuleConf{
		Cond:        cond,
		DegradeRate: *ruleFile.DegradeRate,
		Name:        *ruleFile.Name,
		Enable:      *ruleFile.Enable,
	}

	action, err = covertAction(ruleFile.Action)
	if err != nil {
		return nil, err
	}

	rule.Action = *action
	// check rsp invalid
	rb := strings.NewReader(rule.Action.Rsp)

	_, err = bfe_http.ReadResponse(bfe_bufio.NewReader(rb), nil)
	if err != nil {
		return nil, err
	}

	return rule, nil
}

func covertAction(conf *DegradeActionFile) (*DegradeAction, error) {
	var (
		statusLine StatusLine
		err        error
	)

	rb := strings.Builder{}

	action := &DegradeAction{
		Cmd: *conf.Cmd,
		Rsp: "",
	}

	statusLine, err = NewStatusLineFromConf(conf)
	if err != nil {
		return nil, err
	}

	// generate rsp
	// 1: generate status line
	// if set status line, will ignore StatusText/HttpVersion/StatusCode
	// has already check, if StatusLine is set, it must be valid
	if conf.StatusLine != nil {
		statusLine, _ = parseStatusLine(*conf.StatusLine)
	}

	rb.WriteString(statusLine.String())
	// 2: generate header
	// write content-length into header
	if conf.Body != nil {
		rb.WriteString(fmt.Sprintf("Content-Length: %d \n", len(*conf.Body)))
	}

	// write custom header list
	if conf.Header != nil && len(*conf.Header) > 0 {
		for headerKey, headerValues := range *conf.Header {
			rb.WriteString(headerKey + ": ")

			// if header's value is a string slice, e.g. SESSION = ["value1", "value2", "value3"]
			// will generate: "SESSION": "value1; value2; value3 CRLF"
			for idx, value := range headerValues {
				rb.WriteString(value)

				if idx != len(value)-1 {
					rb.WriteString("; ")
				}
			}

			rb.WriteString("\n")
		}
	}

	// 3: generate body
	rb.WriteString("\n")

	if conf.Body != nil {
		rb.WriteString(*conf.Body)
	}

	// 4: set value
	action.Rsp = rb.String()

	return action, nil
}

func productRuleConfCheck(conf *productRuleConfFile) error {
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

// ProductRulesCheck check ProductRules
func ProductRulesCheck(conf map[string]*degradeRuleConfFileList) error {
	for product, ruleList := range conf {
		if ruleList == nil {
			return fmt.Errorf("no degradeRuleList for product:%s", product)
		}
		if err := DegradeRuleListCheck(ruleList); err != nil {
			return fmt.Errorf("ProductRules:%s, %s", product, err.Error())
		}
	}

	return nil
}

// DegradeRuleListCheck check degradeRuleList
func DegradeRuleListCheck(conf *degradeRuleConfFileList) error {
	// create a rule map
	ruleMap := make(map[string]bool)
	for index, rule := range *conf {
		if err := DegradeRuleCheck(rule); err != nil {
			return fmt.Errorf("degradeRule:%d, %s", index, err.Error())
		}
		if _, ok := ruleMap[*rule.Name]; ok {
			return fmt.Errorf("duplicated rule name[%s]", *rule.Name)
		}

		ruleMap[*rule.Name] = true

	}

	return nil
}

// DegradeRuleCheck check degradeRule
func DegradeRuleCheck(conf *degradeRuleConfFile) error {
	// check nil filed
	if err := bfe_util.CheckNilField(*conf, false); err != nil {
		return err
	}

	// check action
	if _, ok := allowActions[*conf.Action.Cmd]; !ok {
		return fmt.Errorf("mod_degrade action: [%s] not allowed", *conf.Action.Cmd)
	}
	if err := ActionFileCheck(conf.Action); err != nil {
		return err
	}

	// check condition
	if _, err := condition.Build(*conf.Cond); err != nil {
		return fmt.Errorf("cond.Build(): [%s][%s]", *conf.Cond, err.Error())
	}

	if *conf.DegradeRate < 0 || *conf.DegradeRate > 100 {
		return fmt.Errorf("degradeRate should >= 0 and <= 100")
	}

	return nil
}
