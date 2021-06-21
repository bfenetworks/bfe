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

package mod_trust_clientip

import (
	"net"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_util/ipdict"
	"github.com/bfenetworks/bfe/bfe_util/net_util"
)

func TestAcceptHandler_1(t *testing.T) {
	var err error

	m := NewModuleTrustClientIP()
	m.trustTable = ipdict.NewIPTable()

	// load conf from file
	m.configPath = "./testdata/trust_ip_1.conf"
	err = m.loadConfData(nil)
	if err != nil {
		t.Errorf("get err from m.loadConfData():%s", err.Error())
		return
	}

	// create session for test
	s := bfe_basic.Session{}
	s.RemoteAddr = &net.TCPAddr{}

	// case 1: ip addr is trusted
	s.RemoteAddr.IP = net_util.ParseIPv4("119.75.215.1")
	m.acceptHandler(&s)
	if !s.TrustSource() {
		t.Error("119.75.215.1 should be trusted")
	}

	// case 2: ip addr is not trusted
	s.RemoteAddr.IP = net_util.ParseIPv4("119.76.215.1")
	m.acceptHandler(&s)
	if s.TrustSource() {
		t.Error("119.76.215.1 should not be trusted")
	}

	// case 3: ip addr is trusted
	s.RemoteAddr.IP = net_util.ParseIPv4("127.0.0.1")
	m.acceptHandler(&s)
	if !s.TrustSource() {
		t.Error("127.0.0.1 should be trusted")
	}

	// case 4: ip addr is trusted
	s.RemoteAddr.IP = net.ParseIP("::1")
	m.acceptHandler(&s)
	if !s.TrustSource() {
		t.Error("::1 should be trusted")
	}

	// case 5: ip addr is not trusted
	s.RemoteAddr.IP = net.ParseIP("1::")
	m.acceptHandler(&s)
	if s.TrustSource() {
		t.Error("1:: should not be trusted")
	}

}

func TestLoadConfData_case1(t *testing.T) {
	conf, err := TrustIPConfLoad("testdata/trust_ip_3.conf")
	if err != nil {
		t.Errorf("TrustIPConfLoad failed! err %s", err.Error())
		return
	}

	if _, err = ipItemsMake(conf); err == nil {
		t.Error("ipItemsMake should be err")
		return
	}

}
