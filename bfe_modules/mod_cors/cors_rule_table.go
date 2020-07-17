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

package mod_cors

import (
	"sync"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

type CorsRuleTable struct {
	lock        sync.RWMutex
	version     string
	productRule ProductRuleList // product => rule list
}

type CorsRule struct {
	Cond                          condition.Condition
	AccessControlAllowOriginMap   map[string]bool
	AccessControlAllowCredentials bool
	AccessControlExposeHeaders    []string
	AccessControlAllowHeaders     []string
	AccessControlAllowMethods     []string
	AccessControlMaxAge           *int
}

type ProductRuleList map[string]CorsRuleList // product => list of cors rule list
type CorsRuleList []CorsRule

func NewCorsRuleTable() *CorsRuleTable {
	t := new(CorsRuleTable)
	t.productRule = make(ProductRuleList)
	return t
}

func (t *CorsRuleTable) Update(ruleConf *CorsRuleConf) {
	t.lock.Lock()
	t.version = ruleConf.Version
	t.productRule = ruleConf.Config
	t.lock.Unlock()
}

func (t *CorsRuleTable) Search(product string) (CorsRuleList, bool) {
	t.lock.RLock()
	ruleList, ok := t.productRule[product]
	t.lock.RUnlock()

	return ruleList, ok
}
