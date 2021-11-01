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

package ipdict

import (
	"bytes"
	"net"
	"sort"
	"sync"
)

type IPTable struct {
	lock    sync.RWMutex
	ipItems *IPItems
}

func NewIPTable() *IPTable {
	table := new(IPTable)
	return table
}

func (t *IPTable) Version() string {
	t.lock.RLock()
	ipItems := t.ipItems
	t.lock.RUnlock()

	if ipItems != nil {
		return ipItems.Version
	}
	return ""
}

// Update provides for thread-safe switching items
func (t *IPTable) Update(items *IPItems) {
	t.lock.Lock()
	t.ipItems = items
	t.lock.Unlock()
}

// Search provides for binary search IP in dict
func (t *IPTable) Search(srcIP net.IP) bool {
	var hit bool
	t.lock.RLock()
	ipItems := t.ipItems
	t.lock.RUnlock()

	// check ipItems
	if ipItems == nil {
		return false
	}
	// convert ip to ipv6
	ip16 := srcIP.To16()

	if ip16 == nil {
		return false
	}

	// 1. check at the ip set
	if ipItems.ipSet.Exist(ip16) {
		return true
	}

	// 2. check at the item array
	items := ipItems.items
	itemsLen := len(items)

	i := sort.Search(itemsLen,
		func(i int) bool { return bytes.Compare(items[i].startIP, ip16) <= 0 })

	if i < itemsLen {
		if bytes.Compare(items[i].endIP, ip16) >= 0 {
			hit = true
		}
	}

	return hit
}
