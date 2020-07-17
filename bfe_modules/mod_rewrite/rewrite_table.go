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

package mod_rewrite

import (
	"sync"
)

type ReWriteTable struct {
	lock         sync.RWMutex
	version      string
	productRules ProductRules
}

func NewReWriteTable() *ReWriteTable {
	t := new(ReWriteTable)
	t.productRules = make(ProductRules)
	return t
}

func (t *ReWriteTable) Update(conf ReWriteConf) {
	t.lock.Lock()

	t.version = conf.Version
	t.productRules = conf.Config

	t.lock.Unlock()
}

func (t *ReWriteTable) Search(product string) (*RuleList, bool) {
	t.lock.RLock()
	productRules := t.productRules
	t.lock.RUnlock()

	//  find rules for given product
	rules, ok := productRules[product]
	return rules, ok
}
