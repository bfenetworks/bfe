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
	"fmt"
	"net"
	"testing"
)

// util func for unit test
// check whether two ipPairs is equal
// return true if equal else return false
func checkEqual(src, dst ipPairs) bool {
	if len(src) != len(dst) {
		fmt.Println("checkEqual(): len not equal!")
		return false
	}
	for i := 0; i < len(src); i++ {
		if !src[i].startIP.Equal(dst[i].startIP) {
			fmt.Printf("checkEqual(): start element [%d] and [%d] are not equal!\n",
				src[i].startIP, dst[i].startIP)
			return false
		}

		if !src[i].endIP.Equal(dst[i].endIP) {
			fmt.Printf("checkEqual(): end element [%d] and [%d] are not equal!\n",
				src[i].endIP, dst[i].endIP)
			return false
		}

	}

	return true
}

// < case
func TestLess_Case0(t *testing.T) {
	var p ipPairs

	ipStr1 := "1.1.1.1"
	ipStr2 := "2.2.2.2"

	ip1 := net.ParseIP(ipStr1).To16()
	ip2 := net.ParseIP(ipStr2).To16()

	p = append(p, ipPair{ip1, ip1})
	p = append(p, ipPair{ip2, ip2})

	if p.Less(0, 1) {
		t.Errorf("Less(): %s >= %s", ipStr1, ipStr2)
	}

	var p2 ipPairs

	ipStr1 = "1::1"
	ipStr2 = "1::FFFF"

	ip1 = net.ParseIP(ipStr1).To16()
	ip2 = net.ParseIP(ipStr2).To16()

	p2 = append(p2, ipPair{ip1, ip1})
	p2 = append(p2, ipPair{ip2, ip2})

	if p2.Less(0, 1) {
		t.Errorf("Less(): %s >= %s", ipStr1, ipStr2)
	}
}

//  = case
func TestLess_Case1(t *testing.T) {
	var p ipPairs

	ipStr := "1.1.1.1"

	ip := net.ParseIP(ipStr).To16()

	p = append(p, ipPair{ip, ip})
	p = append(p, ipPair{ip, ip})

	if !p.Less(0, 1) {
		t.Errorf("Less(): %s != %s", ipStr, ipStr)
	}

	var p2 ipPairs
	ipStr = "1::1"

	ip = net.ParseIP(ipStr).To16()

	p2 = append(p2, ipPair{ip, ip})
	p2 = append(p2, ipPair{ip, ip})

	if !p2.Less(0, 1) {
		t.Errorf("Less(): %s != %s", ipStr, ipStr)
	}
}

//  > case
func TestLess_Case2(t *testing.T) {
	var p ipPairs

	ipStr1 := "2.2.2.2"
	ipStr2 := "1.1.1.1"

	ip1 := net.ParseIP(ipStr1).To16()
	ip2 := net.ParseIP(ipStr2).To16()

	p = append(p, ipPair{ip1, ip1})
	p = append(p, ipPair{ip2, ip2})

	if !p.Less(0, 1) {
		t.Errorf("Less(): %s < %s", ipStr1, ipStr2)
	}

	var p2 ipPairs

	ipStr1 = "1::FFFF"
	ipStr2 = "1::1"

	ip1 = net.ParseIP(ipStr1).To16()
	ip2 = net.ParseIP(ipStr2).To16()

	p2 = append(p2, ipPair{ip1, ip1})
	p2 = append(p2, ipPair{ip2, ip2})

	if !p2.Less(0, 1) {
		t.Errorf("Less(): %s < %s", ipStr1, ipStr2)
	}
}

// normal case
func TestSwap_Case0(t *testing.T) {
	var p ipPairs

	ipStr1 := "1.1.1.1"
	ipStr2 := "2.2.2.2"

	ip1 := net.ParseIP(ipStr1).To16()
	ip2 := net.ParseIP(ipStr2).To16()

	p = append(p, ipPair{ip1, ip1})
	p = append(p, ipPair{ip2, ip2})

	p.Swap(0, 1)

	if !ip1.Equal(p[1].startIP) || !ip1.Equal(p[1].endIP) {
		t.Errorf("Swap(): %s and %s swap failed!", ipStr1, ipStr2)
	}

	if !ip2.Equal(p[0].startIP) || !ip2.Equal(p[0].endIP) {
		t.Errorf("Swap(): %s and %s swap failed!", ipStr1, ipStr2)
	}

	var p2 ipPairs

	ipStr1 = "1.1.1.1"
	ipStr2 = "2.2.2.2"

	ip1 = net.ParseIP(ipStr1).To16()
	ip2 = net.ParseIP(ipStr2).To16()

	p2 = append(p2, ipPair{ip1, ip1})
	p2 = append(p2, ipPair{ip2, ip2})

	p2.Swap(0, 1)

	if !ip1.Equal(p2[1].startIP) || !ip1.Equal(p2[1].endIP) {
		t.Errorf("Swap(): %s and %s swap failed!", ipStr1, ipStr2)
	}

	if !ip2.Equal(p2[0].startIP) || !ip2.Equal(p2[0].endIP) {
		t.Errorf("Swap(): %s and %s swap failed!", ipStr1, ipStr2)
	}
}

// startIP < endIP case
func TestInsert_Case0(t *testing.T) {
	ipItems, err := NewIPItems(1000, 1000)
	if err != nil {
		t.Error(err.Error())
	}

	startIPStr := "1.1.1.1"
	endIPStr := "2.2.2.2"

	startIP := net.ParseIP(startIPStr)
	endIP := net.ParseIP(endIPStr)

	err = ipItems.InsertPair(startIP, endIP)
	if err != nil {
		t.Errorf("insert(): %s!", err.Error())
	}

	startIPStr = "1::1"
	endIPStr = "1::FFFF"

	startIP = net.ParseIP(startIPStr)
	endIP = net.ParseIP(endIPStr)

	err = ipItems.InsertPair(startIP, endIP)
	if err != nil {
		t.Errorf("insert(): %s!", err.Error())
	}
}

// startIP = endIP case
func TestInsert_Case1(t *testing.T) {
	ipItems, err := NewIPItems(1000, 1000)
	if err != nil {
		t.Error(err.Error())
	}

	startIPStr := "1.1.1.1"
	endIPStr := "1.1.1.1"

	startIP := net.ParseIP(startIPStr)
	endIP := net.ParseIP(endIPStr)

	err = ipItems.InsertPair(startIP, endIP)
	if err != nil {
		t.Error(err.Error())
	}

	startIPStr = "1::1"
	endIPStr = "1::1"

	startIP = net.ParseIP(startIPStr)
	endIP = net.ParseIP(endIPStr)

	err = ipItems.InsertPair(startIP, endIP)
	if err != nil {
		t.Error(err.Error())
	}
}

// startIP > endIP case
func TestInsert_Case2(t *testing.T) {
	ipItems, err := NewIPItems(1000, 1000)
	if err != nil {
		t.Error(err.Error())
	}

	startIPStr := "2.2.2.2"
	endIPStr := "1.1.1.1"

	startIP := net.ParseIP(startIPStr)
	endIP := net.ParseIP(endIPStr)

	err = ipItems.InsertPair(startIP, endIP)
	if err == nil {
		t.Error(err.Error())
	}

	startIPStr = "1::FFFF"
	endIPStr = "1::1"

	startIP = net.ParseIP(startIPStr)
	endIP = net.ParseIP(endIPStr)

	err = ipItems.InsertPair(startIP, endIP)
	if err == nil {
		t.Error(err.Error())
	}
}

// mixed IPv4 and IPv6
func TestInsert_Case3(t *testing.T) {
	ipItems, err := NewIPItems(1000, 1000)
	if err != nil {
		t.Error(err.Error())
	}

	startIPStr := "1.1.1.1"
	endIPStr := "1::1"

	startIP := net.ParseIP(startIPStr)
	endIP := net.ParseIP(endIPStr)

	err = ipItems.InsertPair(startIP, endIP)
	if err == nil {
		t.Error(err.Error())
	}

	startIPStr = "0::1"
	endIPStr = "1.1.1.1"

	startIP = net.ParseIP(startIPStr)
	endIP = net.ParseIP(endIPStr)

	err = ipItems.InsertPair(startIP, endIP)
	if err == nil {
		t.Error(err.Error())
	}
}

// bad ip
func TestInsert_Case4(t *testing.T) {
	ipItems, err := NewIPItems(1000, 1000)
	if err != nil {
		t.Error(err.Error())
	}

	startIPStr := "1::1"
	endIPStr := "1::FFFF"

	startIP := net.ParseIP(startIPStr)
	endIP := net.ParseIP(endIPStr)
	startIP = startIP[:10]

	err = ipItems.InsertPair(startIP, endIP)
	if err == nil {
		t.Error(err.Error())
	}

	err = ipItems.InsertSingle(startIP)
	if err == nil {
		t.Error(err.Error())
	}
}

func TestCheckMerge_Case0(t *testing.T) {
	ips := ipStrs{
		{
			"10.26.74.55",
			"10.26.74.255",
		},
		{
			"0.0.0.0",
			"0.0.0.0",
		},
		{
			"10.12.14.2",
			"10.26.74.105",
		},
	}

	ipItems, err := loadIPStr(ips)
	if err != nil {
		t.Error(err.Error())
	}

	ret := ipItems.checkMerge(0, 2)
	if ret != 1 {
		t.Errorf("checkMerge(): failed! ret:%d", ret)
	}

	ips = ipStrs{
		{
			"1::2",
			"1::4",
		},
		{
			"0::0",
			"0::0",
		},
		{
			"1::1",
			"1::3",
		},
	}

	ipItems, err = loadIPStr(ips)
	if err != nil {
		t.Error(err.Error())
	}

	ret = ipItems.checkMerge(0, 2)
	if ret != 1 {
		t.Errorf("checkMerge(): failed! ret:%d", ret)
	}

}

// len 0 case
func TestMergeItems_Case0(t *testing.T) {
	ips := ipStrs{}

	ipItems, err := loadIPStr(ips)
	if err != nil {
		t.Error(err.Error())
	}

	if ipItems.mergeItems() != 0 {
		t.Errorf("mergeItems(): failed!")
	}
}

// normal case
func TestSort_Case0(t *testing.T) {
	ips := ipStrs{
		{
			"10.26.74.55",
			"10.26.74.255",
		},
		{
			"1::1",
			"1::FFFF",
		},
		{
			"10.21.34.5",
			"10.23.77.100",
		},
		{
			"10.12.14.2",
			"10.12.14.50",
		},
	}

	IPs := ipPairs{
		{
			net.ParseIP("1::1").To16(),
			net.ParseIP("1::FFFF").To16(),
		},
		{
			net.ParseIP("10.26.74.55").To16(),
			net.ParseIP("10.26.74.255").To16(),
		},
		{
			net.ParseIP("10.21.34.5").To16(),
			net.ParseIP("10.23.77.100").To16(),
		},
		{
			net.ParseIP("10.12.14.2").To16(),
			net.ParseIP("10.12.14.50").To16(),
		},
	}

	ipItems, err := loadIPStr(ips)
	if err != nil {
		t.Error(err.Error())
	}

	ipItems.Sort()

	if !checkEqual(ipItems.items, IPs) {
		t.Errorf("checkEqual(): failed!")
	}
}

// merge case
func TestSort_Case1(t *testing.T) {

	ips := ipStrs{
		{
			"10.26.74.55",
			"10.26.74.255",
		},
		{
			"1::1",
			"1::FFFF",
		},
		{
			"10.23.77.88",
			"10.23.77.240",
		},
		{
			"10.21.34.5",
			"10.23.77.100",
		},
		{
			"1::F",
			"1::FF",
		},
		{
			"10.12.14.2",
			"10.12.14.50",
		},
	}

	IPs := ipPairs{
		{
			net.ParseIP("1::1").To16(),
			net.ParseIP("1::FFFF").To16(),
		},
		{
			net.ParseIP("10.26.74.55").To16(),
			net.ParseIP("10.26.74.255").To16(),
		},
		{
			net.ParseIP("10.21.34.5").To16(),
			net.ParseIP("10.23.77.240").To16(),
		},
		{
			net.ParseIP("10.12.14.2").To16(),
			net.ParseIP("10.12.14.50").To16(),
		},
	}

	ipItems, err := loadIPStr(ips)
	if err != nil {
		t.Error(err.Error())
	}

	ipItems.Sort()

	if !checkEqual(ipItems.items, IPs) {
		t.Errorf("checkEqual(): failed!")
	}

}

// merge case
func TestSort_Case2(t *testing.T) {

	ips := ipStrs{
		{
			"10.26.74.55",
			"10.26.74.255",
		},
		{
			"10.23.74.8",
			"10.26.74.55",
		},
		{
			"1::1",
			"1::FF",
		},
		{
			"1::FF",
			"1::FFFF",
		},
	}

	IPs := ipPairs{
		{
			net.ParseIP("1::1").To16(),
			net.ParseIP("1::FFFF").To16(),
		},
		{
			net.ParseIP("10.23.74.8").To16(),
			net.ParseIP("10.26.74.255").To16(),
		},
	}

	ipItems, err := loadIPStr(ips)
	if err != nil {
		t.Error(err)
	}

	ipItems.Sort()

	if !checkEqual(ipItems.items, IPs) {
		t.Errorf("checkEqual(): failed!")
	}

}

// total merge case
func TestSort_Case3(t *testing.T) {

	ips := ipStrs{
		{
			"10.26.74.55",
			"10.26.74.255",
		},
		{
			"10.23.77.88",
			"10.23.77.240",
		},
		{
			"10.21.34.5",
			"10.23.77.100",
		},
		{
			"10.12.14.2",
			"10.30.74.5",
		},
	}

	IPs := ipPairs{
		{
			net.ParseIP("10.12.14.2").To16(),
			net.ParseIP("10.30.74.5").To16(),
		},
	}

	ipItems, err := loadIPStr(ips)
	if err != nil {
		t.Error(err.Error())
	}

	ipItems.Sort()

	if !checkEqual(ipItems.items, IPs) {
		t.Errorf("checkEqual(): failed!")
	}
}
