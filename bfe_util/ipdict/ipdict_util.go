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
	"net"
)

type ipStr struct {
	start string
	end   string
}

type ipStrs []ipStr

// util func for unit test
// load ip to IPItems from struct string
func loadIPStr(ips ipStrs) (*IPItems, error) {

	ipItems, err := NewIPItems(1000, 1000)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		startIP := net.ParseIP(ip.start)
		endIP := net.ParseIP(ip.end)
		err := ipItems.InsertPair(startIP, endIP)
		if err != nil {
			return nil, err
		}
	}

	return ipItems, nil
}

func checkIPPair(startIP, endIP net.IP) error {
	startIP16 := startIP.To16()
	if startIP16 == nil {
		return fmt.Errorf("invalid startIP: %s", startIP.String())
	}
	endIP16 := endIP.To16()
	if endIP16 == nil {
		return fmt.Errorf("invalid endIP: %s", endIP.String())
	}

	if (startIP.To4() != nil && endIP.To4() == nil) || (startIP.To4() == nil && endIP.To4() != nil) {
		return fmt.Errorf("startIP and endIP should both be ipv4 or non-ipv4")
	}
	if bytes.Compare(startIP16, endIP16) == 1 {
		return fmt.Errorf("startIPStr %s > endIPStr %s", startIP.String(), endIP.String())
	}
	return nil
}
