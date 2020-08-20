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
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/queue"
	"github.com/bfenetworks/bfe/bfe_modules/mod_waf/waf_rule"
)

type wafWorker struct {
	concurrency  int               // concurrency for check job
	checkJobList *queue.Queue      //queue for check job
	jobCallback  func(interface{}) //callback function for job

	wafTable *waf_rule.WafRuleTable //waf table, which do the check stuff
}

func NewWafWorker() *wafWorker {
	return new(wafWorker)
}

func (ww *wafWorker) Init(config *ConfModWaf, callback func(interface{})) error {
	ww.concurrency = config.Basic.Concurrency
	ww.checkJobList = new(queue.Queue)
	ww.checkJobList.Init()

	ww.jobCallback = callback

	ww.wafTable = waf_rule.NewWafRuleTable()
	ww.wafTable.Init()

	ww.startAsyncJob()

	return nil
}

func (ww *wafWorker) startAsyncJob() {
	for i := 0; i < ww.concurrency; i++ {
		go ww.doAsyncJob()
	}
}

func (ww *wafWorker) doAsyncJob() {
	for {
		item := ww.checkJobList.Remove()

		job, ok := item.(*wafJob)
		if !ok {
			continue
		}
		wafRule, ok := ww.wafTable.GetRule(job.Rule)
		if !ok {
			continue
		}
		hit := wafRule.Check(job.RuleRequest)
		job.SetHit(hit)
		ww.jobCallback(job)
	}
}

func (ww *wafWorker) pushAsyncJob(job *wafJob) {
	ww.checkJobList.Append(job)
}

func (ww *wafWorker) doSyncJob(job *wafJob) (bool, error) {
	wafRule, ok := ww.wafTable.GetRule(job.Rule)
	if !ok {
		return true, fmt.Errorf("WafWorker.doSyncJob(), err=invalid rule %s", job.Rule)
	}
	log.Logger.Debug("doSyncJob() %v rule=%s", job.RuleRequest, job.Rule)
	hit := wafRule.Check(job.RuleRequest)
	job.SetHit(hit)
	ww.jobCallback(job)
	return hit, nil
}
