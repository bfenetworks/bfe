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
	"fmt"
	"hash/fnv"
	"net"
	"sort"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/hash_set"
)

const (
	IP_LENGTH = 16
)

// Hash is a hash method which convert net.IP to type uint64.
func Hash(ip []byte) uint64 {
	hash64 := fnv.New64()
	hash64.Write(ip)
	return hash64.Sum64()
}

type ipPair struct {
	startIP net.IP
	endIP   net.IP
}

type ipPairs []ipPair

// IPItems manage single IP(hashSet) and ipPairs
type IPItems struct {
	ipSet   *hash_set.HashSet
	items   ipPairs
	Version string
}

// NewIPItems creates new IPItems
func NewIPItems(maxSingleIPNum int, maxPairIPNum int) (*IPItems, error) {
	// maxSingleIPNum && maxPairIPNum must >= 0
	if maxSingleIPNum < 0 || maxPairIPNum < 0 {
		return nil, fmt.Errorf("SingleIPNum/PairIPNum must >= 0")
	}

	var err error
	ipItems := new(IPItems)

	// create a hashSet for single IPs
	isFixedSize := true // ip address is fixed size(IP_LENGTH)
	maxSingleIPNum += 1 // +1, hash_set don't support maxSingleIPNum == 0
	ipItems.ipSet, err = hash_set.NewHashSet(maxSingleIPNum, IP_LENGTH, isFixedSize, Hash)
	if err != nil {
		return nil, err
	}

	// create item array for pair IPs
	ipItems.items = make(ipPairs, 0, maxPairIPNum)
	return ipItems, nil
}

// Len returns num of items
func (items ipPairs) Len() int {
	return len(items)
}

// Less compares specified items
func (items ipPairs) Less(i, j int) bool {
	return bytes.Compare(items[i].startIP, items[j].startIP) >= 0
}

// Swap swaps specified items
func (items ipPairs) Swap(i, j int) {
	items[i], items[j] = items[j], items[i]
}

// checkMerge merge items between index i and j in sorted items.
// If items[i] and items[j] can merge, then merge all items between index i and j
// Others do not merge.
// Constraint: j > i, items[j].endIP >= items[i].startIP
func (ipItems *IPItems) checkMerge(i, j int) int {
	var mergedNum int

	items := ipItems.items

	if bytes.Compare(items[j].endIP, items[i].startIP) >= 0 {
		items[i].startIP = items[j].startIP
		if bytes.Compare(items[j].endIP, items[i].endIP) >= 0 {
			items[i].endIP = items[j].endIP
		}

		items[j].startIP = net.IPv6zero
		items[j].endIP = net.IPv6zero

		mergedNum++

		// Merge items [i+1, j)
		for k := i + 1; k < j; k++ {
			if items[k].endIP.Equal(net.IPv6zero) || items[k].endIP.Equal(net.IPv4zero) {
				continue
			}

			items[k].startIP = net.IPv6zero
			items[k].endIP = net.IPv6zero
			mergedNum++
		}
	}

	return mergedNum
}

// mergeItems provides for merging sorted items
// 1. Sorted dict
//  startIPStr   endIPStr
// ------------------------
// 10.26.74.55 10.26.74.255
// 10.23.77.88 10.23.77.240
// 10.21.34.5  10.23.77.100
// 10.12.14.2  10.12.14.50
// ------------------------
// 2. Merged sorted dict
//  startIPStr   endIPStr
// ------------------------
// 10.26.74.55 10.26.74.255
// 10.21.34.5  10.23.77.240
// 10.12.14.2  10.12.14.50
// 0.0.0.0     0.0.0.0
// ------------------------
func (ipItems *IPItems) mergeItems() int {
	var mergedNum int

	items := ipItems.items
	length := len(items)

	for i := 0; i < length-1; i++ {

		if items[i].endIP.Equal(net.IPv6zero) || items[i].endIP.Equal(net.IPv4zero) {
			continue
		}

		for j := i + 1; j < length; j++ {
			if items[j].endIP.Equal(net.IPv6zero) || items[i].endIP.Equal(net.IPv4zero) {
				continue
			}

			mergedNum += ipItems.checkMerge(i, j)
		}
	}

	return mergedNum
}

// InsertPair provides insert startIP,endIP into IpItems
func (ipItems *IPItems) InsertPair(startIP, endIP net.IP) error {
	if err := checkIPPair(startIP, endIP); err != nil {
		return fmt.Errorf("InsertPair failed: %s", err.Error())
	}

	startIP16 := startIP.To16()
	endIP16 := endIP.To16()

	ipItems.items = append(ipItems.items, ipPair{startIP16, endIP16})
	return nil
}

// InsertSingle insert single ip into ipitems
func (ipItems *IPItems) InsertSingle(ip net.IP) error {
	ip16 := ip.To16()
	if ip16 == nil {
		return fmt.Errorf("InsertSingle(): err, invalid ip: %s", ip.String())
	}
	return ipItems.ipSet.Add(ip16)
}

// Sort provides for sorting dict according startIP by descending order
// 1. Origin dict
//  startIPStr   endIPStr
// ------------------------
// 10.26.74.55 10.26.74.255
// 10.12.14.2  10.12.14.50
// 10.21.34.5  10.23.77.100
// 10.23.77.88 10.23.77.240
// ------------------------
// 2. Sorted dict
//  startIPStr   endIPStr
// ------------------------
// 10.26.74.55 10.26.74.255
// 10.23.77.88 10.23.77.240
// 10.21.34.5  10.23.77.100
// 10.12.14.2  10.12.14.50
// ------------------------
// 3. Merged sorted dict
//  startIPStr   endIPStr
// ------------------------
// 10.26.74.55 10.26.74.255
// 10.21.34.5  10.23.77.240
// 10.12.14.2  10.12.14.50
// 0.0.0.0     0.0.0.0
// ------------------------
// 4. Dict after resliced
//  startIPStr   endIPStr
// ------------------------
// 10.26.74.55 10.26.74.255
// 10.21.34.5  10.23.77.240
// 10.12.14.2  10.12.14.50
// ------------------------
func (ipItems *IPItems) Sort() {

	// Sort items according startIP by descending order
	sort.Sort(ipItems.items)

	// Merge item lines
	mergedNum := ipItems.mergeItems()
	length := len(ipItems.items) - mergedNum

	// Sort items according startIP by descending order
	sort.Sort(ipItems.items)

	// Reslice
	ipItems.items = ipItems.items[0:length]
}

// Length return num of IPItems
func (ipItems *IPItems) Length() int {
	num := len(ipItems.items)
	num += ipItems.ipSet.Len()

	return num
}
