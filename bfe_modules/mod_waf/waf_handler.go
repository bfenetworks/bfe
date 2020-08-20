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

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_modules/mod_waf/waf_rule"
)

type wafHandler struct {
	wafLogger *wafLogger //waf detail log in the waf logger
	worker    *wafWorker //worker which deal the check thing
}

func NewWafHandler() *wafHandler {
	wh := new(wafHandler)
	wh.wafLogger = NewWafLogger()
	wh.worker = NewWafWorker()
	return wh
}

func (wh *wafHandler) Init(conf *ConfModWaf) error {
	err := wh.wafLogger.Init(conf)
	if err != nil {
		return err
	}
	err = wh.worker.Init(conf, wh.wafLogger.DumpLog)
	if err != nil {
		return err
	}
	return nil
}

func (wh *wafHandler) HandlerCheckJob(rule string, req *bfe_basic.Request) error {
	if !waf_rule.IsValidRule(rule) {
		return fmt.Errorf("HandlerBlockJob() err=unknown rule: %s", rule)
	}
	job := NewWafJob(req, rule, CheckType)
	wh.worker.pushAsyncJob(job)
	return nil
}

func (wh *wafHandler) HandlerBlockJob(rule string, req *bfe_basic.Request) (bool, error) {
	if !waf_rule.IsValidRule(rule) {
		return false, fmt.Errorf("HandlerBlockJob() err=unknown rule: %s", rule)
	}
	job := NewWafJob(req, rule, BlockType)
	return wh.worker.doSyncJob(job)
}
