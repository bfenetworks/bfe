// Copyright (c) 2021 The BFE Authors.
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

package mod_tcp_keepalive

import (
	"sync"
)

type KeepAliveTable struct {
	lock         sync.RWMutex
	version      string
	productRules ProductRules
}

func NewKeepAliveTable() *KeepAliveTable {
	t := new(KeepAliveTable)
	t.productRules = make(ProductRules)

	return t
}

func (t *KeepAliveTable) Update(data ProductRuleData) {
	t.lock.Lock()
	t.version = data.Version
	t.productRules = data.Config
	t.lock.Unlock()
}

func (t *KeepAliveTable) Search(product string) (KeepAliveRules, bool) {
	t.lock.RLock()
	productRules := t.productRules
	t.lock.RUnlock()

	rules, ok := productRules[product]
	return rules, ok
}
