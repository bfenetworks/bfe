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

package net_util

import (
	"testing"
)

func TestParseIPv4(t *testing.T) {
	ip := ParseIPv4("127.0.0.1")

	if len(ip) != 4 || ip[0] != 127 || ip[1] != 0 || ip[2] != 0 || ip[3] != 1 {
		t.Errorf("err in ParseIPv4()")
	}
}

func TestIPv4ToUint32(t *testing.T) {
	ip := ParseIPv4("1.2.3.4")

	ipNum, err := IPv4ToUint32(ip)
	if err != nil || ipNum != 0x01020304 {
		t.Errorf("ipNum should be 0x01020304, now it's %x", ipNum)
	}
}

func TestUint32ToIPv4(t *testing.T) {
	ip := Uint32ToIPv4(0x12345678)
	if len(ip) != 4 || ip[0] != 0x12 || ip[1] != 0x34 || ip[2] != 0x56 || ip[3] != 0x78 {
		t.Errorf("err in Uint32ToIPv4(), %s, len=%d", ip.String(), len(ip))
	}
}

func TestUint32ToIPv4Str(t *testing.T) {
	ipStr := Uint32ToIPv4Str(0x01020304)

	if ipStr != "1.2.3.4" {
		t.Errorf("err in Uint32ToIPv4Str(), %d is converted to %s", 0x01020304, ipStr)
	}
}

func TestIPv4StrToUint32(t *testing.T) {
	testStr := "0.0.0.1"
	if ipInt, err := IPv4StrToUint32(testStr); err != nil {
		t.Errorf("err in convert %s to uint 32: %v", testStr, err)
	} else {
		confirmInt := uint32(1)
		if ipInt != confirmInt {
			t.Errorf("ip %s should be converted to %d, but %d get",
				testStr, ipInt, confirmInt)
		}
	}

	testStr = "0.0.1.1"
	if ipInt, err := IPv4StrToUint32(testStr); err != nil {
		t.Errorf("err in convert %s to uint 32: %v", testStr, err)
	} else {
		confirmInt := uint32(1)<<8 + uint32(1)
		if ipInt != confirmInt {
			t.Errorf("ip %s should be converted to %d, but %d get",
				testStr, ipInt, confirmInt)
		}
	}

	testStr = "0.1.1.1"
	if ipInt, err := IPv4StrToUint32(testStr); err != nil {
		t.Errorf("err in convert %s to uint 32: %v", testStr, err)
	} else {
		confirmInt := uint32(1)<<16 + uint32(1)<<8 + uint32(1)
		if ipInt != confirmInt {
			t.Errorf("ip %s should be converted to %d, but %d get",
				testStr, ipInt, confirmInt)
		}
	}

	testStr = "1.1.1.1"
	if ipInt, err := IPv4StrToUint32(testStr); err != nil {
		t.Errorf("err in convert %s to uint 32: %v", testStr, err)
	} else {
		confirmInt := uint32(1)<<24 + uint32(1)<<16 + uint32(1)<<8 + uint32(1)
		if ipInt != confirmInt {
			t.Errorf("ip %s should be converted to %d, but %d get",
				testStr, ipInt, confirmInt)
		}
	}

	testStr = "256.1.1.1"
	if _, err := IPv4StrToUint32(testStr); err == nil {
		t.Errorf("err should happen in convert %s to uint32", testStr)
	}

	testStr = "2001::1"
	if _, err := IPv4StrToUint32(testStr); err == nil {
		t.Errorf("err should happen in convert %s to uint32", testStr)
	}
}

func TestIsIPv4Address(t *testing.T) {
	isIpv4 := IsIPv4Address("127.0.0.1")
	if !isIpv4 {
		t.Errorf("err in IsIPv4Address(), 127.0.0.1 is ipv4")
	}

	isIpv4 = IsIPv4Address("127.0.1")
	if isIpv4 {
		t.Errorf("err in IsIPv4Address(), 127.0.1 is not ipv4")
	}
}

func TestIsPrivateIp(t *testing.T) {
	isPrivate := IsPrivateIp("192.168.0.1")
	if !isPrivate {
		t.Errorf("err in IsPrivateIp(), 192.168.0.1 is private")
	}

	isPrivate = IsPrivateIp("202.113.12.9")
	if isPrivate {
		t.Errorf("err in IsPrivateIp(), 202.113.12.9 is not private")
	}
}
