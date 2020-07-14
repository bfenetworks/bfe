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
	"net"
	"testing"
)

/* Update provides for thread-safe switching items */
func TestUpdate(t *testing.T) {
	table := NewIPTable()

	ipItems, err := NewIPItems(1000, 1000)
	if err != nil {
		t.Error(err.Error())
	}

	ipItems.items = append(ipItems.items, ipPair{net.IPv6zero, net.IPv6zero})
	table.Update(ipItems)

	if ipItems.Length() != 1 {
		t.Errorf("TestItemLength): itemNum [%d] != 1", ipItems.Length())
	}

}

/* Search provides for binary search IP in dict */
func TestSearch(t *testing.T) {
	// Create table
	table := NewIPTable()

	ips := ipStrs{
		{
			"10.26.74.55",
			"10.26.74.255",
		},
		{
			"10.21.34.5",
			"10.23.77.100",
		},
		{
			"10.12.14.2",
			"10.12.14.50",
		},
		{
			"2.2.2.2",
			"2.2.2.2",
		},
		{
			"1::1",
			"1::F",
		},
		{
			"1::FFF",
			"1::FFFF",
		},
	}

	ipItems, err := loadIPStr(ips)
	if err != nil {
		t.Error(err.Error())
	}

	ipItems.Sort()

	// Switch items
	table.Update(ipItems)

	// Search items
	if !table.Search(net.ParseIP("10.12.14.12")) {
		t.Errorf("TestSearch(): 10.12.14.12 not hit")
	}

	if !table.Search(net.ParseIP("2.2.2.2")) {
		t.Errorf("TestSearch(): 2.2.2.2 not hit")
	}
	if !table.Search(net.ParseIP("1::F")) {
		t.Errorf("TestSearch(): 1::F not hit")
	}
	if table.Search(net.ParseIP("1.1.1.1")) {
		t.Errorf("TestSearch(): 1.1.1.1 hit")
	}
	if table.Search(net.ParseIP("1::FF")) {
		t.Errorf("TestSearch(): 1::FF hit")
	}
}
