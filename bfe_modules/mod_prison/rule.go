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
	"time"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/lru_cache"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/action"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

type prisonRule struct {
	name           string              // rule name
	cond           condition.Condition // condition parsed
	condStr        string              // condition string
	action         action.Action       // action
	accessSigner   AccessSigner        // access signer
	checkPeriodNs  int64               // check period in nanoSeconds
	stayPeriodNs   int64               // stay period in nanoSeconds
	threshold      int32               // threshold period
	accessDict     *lru_cache.LRUCache // dict store access info
	prisonDict     *lru_cache.LRUCache // dict store prison info
	accessDictSize int                 // access dict size
	prisonDictSize int                 // prison dict size
}

func newPrisonRule(ruleConf PrisonRuleConf) (*prisonRule, error) {
	// build condition
	cond, err := condition.Build(*ruleConf.Cond)
	if err != nil {
		return nil, fmt.Errorf("build condition err: condStr[%s], err[%s]", *ruleConf.Cond, err)
	}

	regExp, _ := regexp.Compile(ruleConf.AccessSignConf.UrlRegexp)

	// create new rule
	rule := new(prisonRule)
	rule.cond = cond
	rule.condStr = *ruleConf.Cond
	rule.action = *ruleConf.Action
	rule.name = *ruleConf.Name
	rule.accessSigner = AccessSigner{
		AccessSignConf: *ruleConf.AccessSignConf,
		UrlReg:         regExp,
	}
	rule.checkPeriodNs = *ruleConf.CheckPeriod * 1e9
	rule.stayPeriodNs = *ruleConf.StayPeriod * 1e9
	rule.threshold = *ruleConf.Threshold
	rule.accessDictSize = *ruleConf.AccessDictSize
	rule.prisonDictSize = *ruleConf.PrisonDictSize

	return rule, nil
}

func (r *prisonRule) initDict(oldRule *prisonRule) {
	if oldRule == nil {
		// if oldRule is nil, create new dict
		r.accessDict = lru_cache.NewLRUCache(r.accessDictSize)
		r.prisonDict = lru_cache.NewLRUCache(r.prisonDictSize)
	} else {
		// use old dict instead
		r.accessDict = oldRule.accessDict
		r.prisonDict = oldRule.prisonDict

		// resize dict
		r.accessDict.EnlargeCapacity(r.accessDictSize)
		r.prisonDict.EnlargeCapacity(r.prisonDictSize)
	}
}

func (r *prisonRule) recordAndCheck(req *bfe_basic.Request) bool {
	if openDebug {
		log.Logger.Debug("begin process rule %s", r.name)
	}

	// get sign
	sign, err := r.accessSigner.Sign(r.condStr, req)
	if err != nil {
		return false
	}

	// check whether the access should be denied directyly
	if deny := r.shouldDeny(sign, req); deny {
		return deny
	}

	// record and check
	r.recordAccess(sign)
	return r.shouldDeny(sign, req)
}

func (r *prisonRule) recordAccess(sign AccessSign) {
	var f *AccessCounter

	// check access dict
	value, ok := r.accessDict.Get(sign)
	if !ok {
		f = NewAccessCounter()
		r.accessDict.Add(sign, f)
	} else {
		f = value.(*AccessCounter)
	}

	// check threshod
	if block, restTimeNs := f.IncAndCheck(r.checkPeriodNs, r.threshold); block {
		// should block the access, update prisonDict and accessDict
		freeTimeNs := r.stayPeriodNs + restTimeNs + time.Now().UnixNano()
		r.prisonDict.Add(sign, freeTimeNs)
		r.accessDict.Del(sign)
	}
}

func (r *prisonRule) shouldDeny(sign AccessSign, req *bfe_basic.Request) bool {
	// find prison record for this sign
	freeTimeNs, ok := r.prisonDict.Get(sign)
	if !ok {
		return false
	}

	// check prison time
	if time.Now().UnixNano() < freeTimeNs.(int64) {
		prisonInfo := &PrisonInfo{
			PrisonType: ModPrison,
			PrisonName: r.name,
			FreeTime:   time.Unix(0, freeTimeNs.(int64)),
			IsExpired:  false,
			Action:     r.action.Cmd,
		}
		req.SetContext(ReqCtxPrisonInfo, prisonInfo)
		return true
	}

	// remove prison record if expired
	r.prisonDict.Del(sign)

	// set prisoninfo
	prisonInfo := &PrisonInfo{
		PrisonType: ModPrison,
		PrisonName: r.name,
		FreeTime:   time.Unix(0, freeTimeNs.(int64)),
		IsExpired:  true,
		Action:     r.action.Cmd,
	}
	req.SetContext(ReqCtxPrisonInfo, prisonInfo)

	return false
}
