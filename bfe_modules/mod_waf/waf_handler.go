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
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_modules/mod_waf/waf_rule"
)

type wafHandler struct {
	wafLogger *wafLogger // waf detail log in the waf logger

	wafTable *waf_rule.WafRuleTable // waf table, which do the check stuff
}

func NewWafHandler() *wafHandler {
	wh := new(wafHandler)
	return wh
}

func (wh *wafHandler) Init(conf *ConfModWaf) error {
	wh.wafLogger = NewWafLogger()
	err := wh.wafLogger.Init(conf)
	if err != nil {
		return err
	}

	wh.wafTable = waf_rule.NewWafRuleTable()
	wh.wafTable.Init()
	return err
}

func (wh *wafHandler) HandleBlockJob(rule string, req *bfe_basic.Request) (bool, error) {
	if !waf_rule.IsValidRule(rule) {
		return false, fmt.Errorf("HandleBlockJob() err=unknown rule: %s", rule)
	}
	job := NewWafJob(req, rule, BlockType)
	return wh.doJob(job)
}

func (wh *wafHandler) HandleCheckJob(rule string, req *bfe_basic.Request) (bool, error) {
	if !waf_rule.IsValidRule(rule) {
		return false, fmt.Errorf("HandleCheckJob() err=unknown rule: %s", rule)
	}
	job := NewWafJob(req, rule, CheckType)
	return wh.doJob(job)
}

func (wh *wafHandler) doJob(job *wafJob) (bool, error) {
	wafRule, ok := wh.wafTable.GetRule(job.Rule)
	if !ok {
		return true, fmt.Errorf("wafHandler.doJob(), err=invalid rule %s", job.Rule)
	}
	log.Logger.Debug("wafHandler.doJob() %v rule=%s", job.RuleRequest, job.Rule)
	hit := wafRule.Check(job.RuleRequest)
	job.SetHit(hit)
	wh.wafLogger.DumpLog(job)
	return hit, nil
}
