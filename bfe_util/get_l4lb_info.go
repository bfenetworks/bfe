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

package bfe_util

import (
	"fmt"
	"net"
)

import (
	"github.com/bfenetworks/bfe/bfe_tls"
)

// GetVipPort return vip and port for given conn
func GetVipPort(conn net.Conn) (net.IP, int, error) {
	// get underlying bfe conn, the given net.Conn may be wired like:
	//  - TLS Connection (optional)
	//  - BFE Connection (PROXY, optional)
	//  - TCP Connection
	if tc, ok := conn.(*bfe_tls.Conn); ok {
		conn = tc.GetNetConn()
	}

	// get virtual vip
	if af, ok := conn.(AddressFetcher); ok {
		vaddr := af.VirtualAddr()
		if vaddr == nil {
			return nil, 0, fmt.Errorf("vip unknown")
		}
		return ParseIpAndPort(vaddr.String())
	}

	return nil, 0, fmt.Errorf("can`t get vip and port when Layer4LoadBalancer is not set")
}

// GetVip return vip for given conn
func GetVip(conn net.Conn) net.IP {
	vip, _, err := GetVipPort(conn)
	if err != nil {
		return nil
	}
	return vip
}
