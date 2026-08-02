// Copyright (c) 2026 The BFE Authors.
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

package mod_access_pb3

import (
	"net"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
)

func TestConnAddrInfoGen(t *testing.T) {
	conn := newTestConn()
	session := bfe_basic.NewSession(conn)
	session.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}
	session.Vip = net.ParseIP("10.0.0.1")
	session.SetTrustSource(true)

	info := ConnAddrInfoGen(session)
	if info == nil {
		t.Fatal("ConnAddrInfoGen() returns nil")
	}
	if info.BfeIp == nil || *info.BfeIp == 0 {
		t.Error("BfeIp should not be zero")
	}
	if info.SockSrcIp == nil || *info.SockSrcIp == 0 {
		t.Error("SockSrcIp should not be zero")
	}
	if info.IsTrustSrcIp == nil || !*info.IsTrustSrcIp {
		t.Error("IsTrustSrcIp should be true")
	}
	if info.Vip == nil || *info.Vip == 0 {
		t.Error("Vip should not be zero")
	}
}

func TestConnAddrInfoGenIPv6(t *testing.T) {
	conn := &testConn{
		localAddr:  &net.TCPAddr{IP: net.ParseIP("::1"), Port: 8080},
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("::1"), Port: 12345},
	}
	session := bfe_basic.NewSession(conn)
	session.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("::1"), Port: 12345}
	session.Vip = net.ParseIP("2001:db8::1")
	session.SetTrustSource(false)

	info := ConnAddrInfoGen(session)
	if info == nil {
		t.Fatal("ConnAddrInfoGen() returns nil")
	}
	if info.Vip6 == nil || *info.Vip6 == "" {
		t.Error("Vip6 should be set for IPv6 vip")
	}
}

func TestConnAddrInfoGenNilVip(t *testing.T) {
	conn := newTestConn()
	session := bfe_basic.NewSession(conn)
	session.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}
	session.Vip = nil

	info := ConnAddrInfoGen(session)
	if info == nil {
		t.Fatal("ConnAddrInfoGen() returns nil")
	}
	if info.Vip != nil && *info.Vip != 0 {
		t.Error("Vip should be zero when session.Vip is nil")
	}
}
