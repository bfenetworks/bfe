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
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_modules/mod_waf/waf_rule"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

const (
	CheckType = "Check" // check job is async job, which just check and log, but never block
	BlockType = "Block" // block job is sync job, which check, log and maybe block
)

type wafJob struct {
	Rule        string                    // rule name of this job
	Type        string                    // type of this job
	Hit         bool                      // is job hit rule
	RuleRequest *waf_rule.RuleRequestInfo // waf check request info
}

func NewWafJob(req *bfe_basic.Request, rule string, jtype string) *wafJob {
	wj := new(wafJob)
	wj.Rule = rule
	wj.Type = jtype
	wj.RuleRequest = waf_rule.NewRuleRequestInfo(req)
	return wj
}

func (j *wafJob) SetHit(hit bool) { j.Hit = hit }

func (j *wafJob) String() string {
	bytes, _ := json.Marshal(*j)
	return string(bytes)
}
