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
	"bytes"
	"fmt"
	"net"
)

//IpRange - a structure that holds the start and end of a range of ip addresses
type IpRange struct {
	start net.IP
	end   net.IP
}

// private ip range
var privateRanges = []IpRange{
	{
		start: net.ParseIP("10.0.0.0").To4(),
		end:   net.ParseIP("10.255.255.255").To4(),
	},
	{
		start: net.ParseIP("172.16.0.0").To4(),
		end:   net.ParseIP("172.31.255.255").To4(),
	},
	{
		start: net.ParseIP("192.168.0.0").To4(),
		end:   net.ParseIP("192.168.255.255").To4(),
	},
}

// InRange checks whether a given ip address is within a range given
func InRange(r IpRange, ip net.IP) bool {
	return bytes.Compare(ip, r.start) >= 0 && bytes.Compare(ip, r.end) <= 0
}

// ParseIPv4 parse IP addr from string to net.IP
//
// Params:
//     - s: IP addr in string, e.g., "1.2.3.4"
//
// Returns:
//     IP addr in net.IP
func ParseIPv4(s string) net.IP {
	ip := net.ParseIP(s)

	if ip != nil {
		ip = ip.To4()
	}

	return ip
}

// IPv4ToUint32 convert net.IP to uint32
//
// e.g., 1.2.3.4 to 0x01020304
//
// Params:
//     - ipBytes: IPv4 addr in net.IP
//
// Returns:
//     IPv4 addr in uint32
func IPv4ToUint32(ipBytes net.IP) (uint32, error) {
	if len(ipBytes) != 4 {
		return 0, fmt.Errorf("ip bytes len: %d", len(ipBytes))
	}

	var ipNum uint32
	var tmp uint32

	for i, b := range ipBytes {
		tmp = uint32(b)
		ipNum |= (tmp << uint((3-i)*8))
	}

	return ipNum, nil
}

// IPv4StrToUint32 convert IPv4 string to uint32
//
// e.g., "1.2.3.4" to 0x01020304
//
// Params:
//     - ipStr: IPv4 addr in string
//
// Returns:
//     IPv4 addr in uint32
func IPv4StrToUint32(ipStr string) (uint32, error) {
	ip := ParseIPv4(ipStr)
	if ip == nil {
		return 0, fmt.Errorf("invalid IPv4 addr string: %s", ipStr)
	}

	return IPv4ToUint32(ip)
}

// Uint32ToIPv4 convert uint32 net.IP
//
// e.g., 0x01020304 to 1.2.3.4
//
// Params:
//     - ipNum: IPv4 addr in uint32
//
// Returns:
//     IPv4 addr in net.IP
func Uint32ToIPv4(ipNum uint32) net.IP {
	var ipBytes [4]byte

	for i := 0; i < 4; i++ {
		ipBytes[3-i] = byte(ipNum & 0xFF)
		ipNum >>= 8
	}

	return net.IPv4(ipBytes[0], ipBytes[1], ipBytes[2], ipBytes[3]).To4()
}

// Uint32ToIPv4Str convert uint32 to str
//
// e.g., 0x01020304 to "1.2.3.4"
//
// Params:
//     - ipNum: IPv4 addr in uint32
//
// Returns:
//     IPv4 addr in string
func Uint32ToIPv4Str(ipNum uint32) string {
	str := fmt.Sprintf("%d.%d.%d.%d", byte(ipNum>>24), byte(ipNum>>16), byte(ipNum>>8), byte(ipNum))

	return str
}

// IsIPv4Address Check input is ipv4 address or not.
//
// param:
//     - input: a string
// return:
//     bool
func IsIPv4Address(input string) bool {
	return net.ParseIP(input).To4() != nil
}

// IsPrivateIp Check to see if an ip is in a private subnet.
//
// param:
//     - input: an ip string
// return:
//     bool
func IsPrivateIp(input string) bool {
	if ip := net.ParseIP(input).To4(); ip != nil {
		for _, r := range privateRanges {
			if InRange(r, ip) {
				return true
			}
		}
	}
	return false
}
