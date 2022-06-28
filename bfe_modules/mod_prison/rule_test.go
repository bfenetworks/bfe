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
	"crypto/md5"
	"testing"
	"time"
)

import (
	"github.com/baidu/go-lib/lru_cache"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

func newPrisonRuleConfTest() (*PrisonRuleConf, error) {
	jsonConf := []byte(` {
            "Name": "ip cookie prison 1",
            "cond": "default_t()",
            "accessSignConf": {
                "socketip": true,
                "url": true,
                "header": [
                    "User-Agent",
                    "Referer"
                ],
                "Cookie": [
                    "UID"
                ]
            },
            "action": {
                "cmd": "CLOSE",
                "params": []
            },
            "checkPeriod": 10,
            "threshold": 2,
            "stayPeriod": 10,
            "accessDictSize": 1000,
            "prisonDictSize": 1000
       } `)
	ruleConf := PrisonRuleConf{}
	if err := json.Unmarshal(jsonConf, &ruleConf); err != nil {
		return nil, err
	}
	// check conf
	PrisonRuleCheck(&ruleConf)
	return &ruleConf, nil
}

func newPrisonRuleTest() (*prisonRule, error) {
	ruleConf, err := newPrisonRuleConfTest()
	if err != nil {
		return nil, err
	}

	rule, err := newPrisonRule(*ruleConf)
	if err != nil {
		return nil, err
	}

	return rule, nil
}

func TestNewPrisonRule(t *testing.T) {
	prisonRuleConf, err := newPrisonRuleConfTest()
	if err != nil {
		t.Error("should success")
		return
	}

	rule, err := newPrisonRule(*prisonRuleConf)
	if err != nil {
		t.Error("should create success")
		return
	}

	// check name
	if rule.name != "ip cookie prison 1" {
		t.Error("name is wrong")
		return
	}
}

func TestInitDictCase1(t *testing.T) {
	rule, err := newPrisonRuleTest()
	if err != nil {
		t.Error(err)
		return
	}

	// init dict
	rule.initDict(nil)
	if rule.accessDict == nil || rule.prisonDict == nil {
		t.Error("accessDict/prisonDict shouldn't be nil")
		return
	}

	// check len
	if rule.accessDict.Len() != 0 || rule.prisonDict.Len() != 0 {
		t.Error("len of accessDict/prisonDict should be 0")
		return
	}
}

func TestInitDictCase2(t *testing.T) {
	rule, err := newPrisonRuleTest()
	if err != nil {
		t.Error(err)
		return
	}

	rule1 := prisonRule{
		accessDict: lru_cache.NewLRUCache(1),
		prisonDict: lru_cache.NewLRUCache(1),
	}
	rule1.accessDict.Add(1, 2)

	// init dict
	rule.initDict(&rule1)
	if rule.accessDict == nil || rule.prisonDict == nil {
		t.Error("accessDict/prisonDict shouldn't be nil")
		return
	}

	if rule.accessDict.Len() != 1 || rule.prisonDict.Len() != 0 {
		t.Error("len of accessDict/prisonDict should be 0")
		return
	}

	val, ok := rule.accessDict.Get(1)
	if !ok {
		t.Error("should get success")
		return
	}

	value := val.(int)
	if value != 2 {
		t.Error("val should be 2")
		return
	}
}

func TestRecordAccess(t *testing.T) {
	rule, err := newPrisonRuleTest()
	if err != nil {
		t.Error(err)
		return
	}
	rule.initDict(nil)

	sign := AccessSign(md5.Sum([]byte("12334")))
	req := &bfe_basic.Request{}
	req.Route.Product = "testProduct"

	// recordAccess
	rule.recordAccess(sign)
	value, ok := rule.accessDict.Get(sign)
	if !ok {
		t.Error("should get success")
		return
	}

	f := value.(*AccessCounter)
	if f.count != 1 {
		t.Error("count for 12334 should be 1")
		return
	}

	rule.recordAccess(sign)
	if f.count != 2 {
		t.Error("count for 12334 should be 2")
		return
	}

	// meet threshod, should be zero
	rule.recordAccess(sign)

	_, ok = rule.accessDict.Get(sign)
	if ok {
		t.Errorf("access counter should be deleted")
		return
	}

	// should get failed
	_, ok = rule.accessDict.Get(AccessSign(md5.Sum([]byte("1234"))))
	if ok {
		t.Error("should get failed")
		return
	}
}

func TestShouldDeny(t *testing.T) {
	rule, err := newPrisonRuleTest()
	if err != nil {
		t.Error(err)
		return
	}
	rule.initDict(nil)

	req := &bfe_basic.Request{
		Context: make(map[interface{}]interface{}),
	}
	sign := AccessSign(md5.Sum([]byte("12334")))
	rule.recordAccess(sign)
	if rule.shouldDeny(sign, req) {
		t.Error("shouldn't deny")
		return
	}
	rule.recordAccess(sign)
	rule.recordAccess(sign)
	if !rule.shouldDeny(sign, req) {
		t.Error("should deny")
		return
	}
}

func newPrisonRuleConfTestNew() (*PrisonRuleConf, error) {
	jsonConf := []byte(` {
            "Name": "ip cookie prison 1",
            "cond": "default_t()",
            "accessSignConf": {
                "socketip": true,
                "url": true,
                "header": [
                    "User-Agent",
                    "Referer"
                ],
                "Cookie": [
                    "UID"
                ]
            },
            "action": {
                "cmd": "CLOSE",
                "params": []
            },
            "checkPeriod": 1,
            "threshold": 0,
            "stayPeriod": 0,
            "accessDictSize": 1000,
            "prisonDictSize": 1000
       } `)
	ruleConf := PrisonRuleConf{}
	if err := json.Unmarshal(jsonConf, &ruleConf); err != nil {
		return nil, err
	}
	// check conf
	PrisonRuleCheck(&ruleConf)
	return &ruleConf, nil
}

func newPrisonRuleTestNew() (*prisonRule, error) {
	ruleConf, err := newPrisonRuleConfTestNew()
	if err != nil {
		return nil, err
	}

	rule, err := newPrisonRule(*ruleConf)
	if err != nil {
		return nil, err
	}

	return rule, nil
}

func TestShouldDenyCase1(t *testing.T) {
	rule, err := newPrisonRuleTestNew()
	if err != nil {
		t.Error(err)
		return
	}
	rule.initDict(nil)

	req := &bfe_basic.Request{
		Context: make(map[interface{}]interface{}),
	}
	sign := AccessSign(md5.Sum([]byte("12334")))

	rule.recordAccess(sign)
	if !rule.shouldDeny(sign, req) {
		t.Error("shouldn deny")
		return
	}
}

func newPrisonRuleConfTestCase2() (*PrisonRuleConf, error) {
	jsonConf := []byte(` {
            "Name": "ip cookie prison 1",
            "cond": "default_t()",
            "accessSignConf": {
                "socketip": true,
                "url": true,
                "header": [
                    "User-Agent",
                    "Referer"
                ],
                "Cookie": [
                    "UID"
                ]
            },
            "action": {
                "cmd": "CLOSE",
                "params": []
            },
            "checkPeriod": 1,
            "threshold": 1,
            "stayPeriod": 0,
            "accessDictSize": 1000,
            "prisonDictSize": 1000
       } `)
	ruleConf := PrisonRuleConf{}
	if err := json.Unmarshal(jsonConf, &ruleConf); err != nil {
		return nil, err
	}
	// check conf
	PrisonRuleCheck(&ruleConf)
	return &ruleConf, nil
}

func newPrisonRuleTestCase2() (*prisonRule, error) {
	ruleConf, err := newPrisonRuleConfTestCase2()
	if err != nil {
		return nil, err
	}

	rule, err := newPrisonRule(*ruleConf)
	if err != nil {
		return nil, err
	}

	return rule, nil
}

func TestShouldDenyCase2(t *testing.T) {
	rule, err := newPrisonRuleTestCase2()
	if err != nil {
		t.Error(err)
		return
	}
	rule.initDict(nil)

	req := &bfe_basic.Request{
		Context: make(map[interface{}]interface{}),
	}
	sign := AccessSign(md5.Sum([]byte("12334")))

	rule.recordAccess(sign)
	if rule.shouldDeny(sign, req) {
		t.Error("1st check shouldn't deny")
		return
	}

	rule.recordAccess(sign)
	if !rule.shouldDeny(sign, req) {
		t.Error("2nd check should deny")
		return
	}

	time.Sleep(1 * time.Second)
	rule.recordAccess(sign)
	if rule.shouldDeny(sign, req) {
		t.Error("after 1 second 1st check shouldn't deny")
		return
	}

	rule.recordAccess(sign)
	if !rule.shouldDeny(sign, req) {
		t.Error("ater 1 second 2nd check should deny")
		return
	}
}
