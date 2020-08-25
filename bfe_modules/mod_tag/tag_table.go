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

package mod_tag

import (
	"sync"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

type TagRuleTable struct {
	lock        sync.RWMutex
	version     string
	productRule ProductRuleList // product => rule list
}

type TagRule struct {
	Cond  condition.Condition
	Param TagParam
	Last  bool // if true, not to check the next rule in the list if the condition is satisfied
}

type TagParam struct {
	TagName  string `json:"TagName"`
	TagValue string `json:"TagValue"`
}

type ProductRuleList map[string]TagRuleList // product => list of tag rule list
type TagRuleList []TagRule

func NewTagRuleTable() *TagRuleTable {
	t := new(TagRuleTable)
	t.productRule = make(ProductRuleList)
	return t
}

func (t *TagRuleTable) Update(ruleConf *TagRuleConf) {
	t.lock.Lock()
	t.version = ruleConf.Version
	t.productRule = ruleConf.Config
	t.lock.Unlock()
}

func (t *TagRuleTable) Search(product string) (TagRuleList, bool) {
	t.lock.RLock()
	ruleList, ok := t.productRule[product]
	t.lock.RUnlock()

	return ruleList, ok
}
