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
	"strings"
	"testing"
)

// new case
func TestNewIpLocationTable(t *testing.T) {
	_, err := NewIpLocationTable(0, 1)
	if err == nil {
		t.Errorf("NewIpLocationTable should return err!=nil but return nil")
	}

	_, err = NewIpLocationTable(1, 0)
	if err == nil {
		t.Errorf("NewIpLocationTable should return err!=nil but return nil")
	}

	_, err = NewIpLocationTable(1000001, 10)
	if err == nil {
		t.Errorf("NewIpLocationTable should return err!=nil but not")
	}

	_, err = NewIpLocationTable(1000, 1025)
	if err == nil {
		t.Errorf("NewIpLocationTable should return err!=nil but not")
	}
}

//  Add case
func TestLocAdd(t *testing.T) {
	locTable, err := NewIpLocationTable(1, 48)
	if err != nil {
		t.Errorf("NewIpLocationTable should return err==nil but return not")
	}

	ipStrS := "223.255.192.0"
	ipStrE := "223.255.223.255"
	ipLoc := "KR:None:None"
	//223.255.192.0|223.255.223.255|KR|None|None|None|None|80|0|0|0|0
	ipS := net.ParseIP(ipStrS)
	ipE := net.ParseIP(ipStrE)
	err = locTable.Add(ipS, ipE, ipLoc)
	if err != nil {
		t.Errorf("locTable Add should return err==nil but return not")
	}

	//223.255.128.0|223.255.191.255|HK|None|XIANGGANG|XIANGGANG|None|80|0|80|80|0
	ipStrS = "223.255.128.0"
	ipStrE = "223.255.191.255"
	ipLoc = "HK:XIANGGANG:XIANGGANG"
	ipS = net.ParseIP(ipStrS)
	ipE = net.ParseIP(ipStrE)
	err = locTable.Add(ipS, ipE, ipLoc)
	if err == nil {
		t.Errorf("locTable Add should return err!=nil but return nil")
	}

	//1::1|1::FFFF|HK|None|XIANGGANG|XIANGGANG|None|80|0|80|80|0
	ipStrS = "1::1"
	ipStrE = "1::FFFF"
	ipLoc = "HK:XIANGGANG:XIANGGANG"
	ipS = net.ParseIP(ipStrS)
	ipE = net.ParseIP(ipStrE)
	err = locTable.Add(ipS, ipE, ipLoc)
	if err == nil {
		t.Errorf("locTable Add should return err!=nil but return nil")
	}
}

//  Search case
func TestLocSearch(t *testing.T) {
	locTable, err := NewIpLocationTable(3, 48)
	if err != nil {
		t.Errorf("NewIpLocationTable should return err==nil but not")
	}

	//223.255.192.0|223.255.223.255|KR|None|None|None|None|80|0|0|0|0
	ipStrS := "223.255.192.0"
	ipStrE := "223.255.223.255"
	ipLoc := "KR:None:None"
	ipS := net.ParseIP(ipStrS)
	ipE := net.ParseIP(ipStrE)
	err = locTable.Add(ipS, ipE, ipLoc)
	if err != nil {
		t.Errorf("locTable Add should return err==nil but not")
	}

	ip := net.ParseIP(ipStrS)
	var loc string
	loc, err = locTable.Search(ip)
	if err != nil {
		t.Errorf("locTable Search should return err==nil but not")
	}

	if !strings.EqualFold("KR:None:None", loc) {
		t.Errorf("locTable Search should return KR:None:None but is %s", loc)
	}

	//223.255.128.0|223.255.191.255|HK|None|XIANGGANG|XIANGGANG|None|80|0|80|80|0
	ipStrS = "223.255.128.0"
	ipStrE = "223.255.191.255"
	ipLoc = "HK:XIANGGANG:XIANGGANG"
	ipS = net.ParseIP(ipStrS)
	ipE = net.ParseIP(ipStrE)
	locTable.Add(ipS, ipE, ipLoc)
	if err != nil {
		t.Errorf("locTable Add should return err==nil but not")
	}

	ip = net.ParseIP(ipStrE)
	loc, err = locTable.Search(ip)
	if err != nil {
		t.Errorf("locTable Search should return err==nil but not")
	}
	if !strings.EqualFold("HK:XIANGGANG:XIANGGANG", loc) {
		t.Errorf("locTable Search should return HK:XIANGGANG:XIANGGANG but is %s", loc)
	}

	//1::1|1::FFFF|HK|None|XIANGGANG|XIANGGANG|None|80|0|80|80|0
	ipStrS = "1::1"
	ipStrE = "1::FFFF"
	ipLoc = "HK:XIANGGANG:XIANGGANG"
	ipS = net.ParseIP(ipStrS)
	ipE = net.ParseIP(ipStrE)
	locTable.Add(ipS, ipE, ipLoc)
	if err != nil {
		t.Errorf("locTable Add should return err==nil but not")
	}

	ip = net.ParseIP(ipStrE)
	loc, err = locTable.Search(ip)
	if err != nil {
		t.Errorf("locTable Search should return err==nil but not")
	}
	if !strings.EqualFold("HK:XIANGGANG:XIANGGANG", loc) {
		t.Errorf("locTable Search should return HK:XIANGGANG:XIANGGANG but is %s", loc)
	}
}
