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

package mod_prison

import (
	"net"
	"testing"
	"time"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func TestNewModulePrison(t *testing.T) {
	m := NewModulePrison()
	if m == nil {
		t.Fatal("NewModulePrison() returns nil")
	}
	if m.Name() != ModPrison {
		t.Errorf("module name error, got: %s, want: %s", m.Name(), ModPrison)
	}
}

func prepareModulePrison(t *testing.T) (*ModulePrison, string) {
	m := NewModulePrison()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	return m, ""
}

func makePrisonRequest(product string, uri string) *bfe_basic.Request {
	conn := &testConn{
		localAddr:  &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080},
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345},
	}

	session := bfe_basic.NewSession(conn)
	session.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}

	req, _ := bfe_http.NewRequest("GET", "http://www.example.org"+uri, nil)
	req.RequestURI = uri
	req.Header.Set("User-Agent", "test-agent")

	bfeReq := bfe_basic.NewRequest(req, conn, nil, session, nil)
	bfeReq.Route.Product = product
	return bfeReq
}

func TestPrisonHandlerNoRules(t *testing.T) {
	m, _ := prepareModulePrison(t)
	req := makePrisonRequest("unknown_product", "/path")

	ret, res := m.prisonHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be BfeHandlerGoOn, got: %d", ret)
	}
	if res != nil {
		t.Error("res should be nil")
	}
}

func loadActionRules(t *testing.T, m *ModulePrison) {
	_, err := m.loadProductRuleTable(nil)
	if err != nil {
		t.Fatalf("loadProductRuleTable() error: %v", err)
	}
	// switch to policy_test.data (without global product) by setting productConfPath
	m.productConfPath = "./testdata/mod_prison/policy_test.data"
	_, err = m.loadProductRuleTable(nil)
	if err != nil {
		t.Fatalf("loadProductRuleTable(test) error: %v", err)
	}
}

func TestPrisonHandlerGlobalRule(t *testing.T) {
	m, _ := prepareModulePrison(t)
	m.productConfPath = "./testdata/mod_prison/policy_actions.data"
	_, err := m.loadProductRuleTable(nil)
	if err != nil {
		t.Fatalf("loadProductRuleTable(actions) error: %v", err)
	}
	req := makePrisonRequest("global", "/global")

	ret, res := m.prisonHandler(req)
	if ret != bfe_module.BfeHandlerClose {
		t.Errorf("ret should be BfeHandlerClose, got: %d", ret)
	}
	if res != nil {
		t.Error("res should be nil")
	}
	if req.ErrCode != ErrPrison {
		t.Error("ErrCode should be ErrPrison")
	}
}

func TestPrisonHandlerCloseRule(t *testing.T) {
	m, _ := prepareModulePrison(t)
	loadActionRules(t, m)
	req := makePrisonRequest("close", "/close")

	ret, _ := m.prisonHandler(req)
	if ret != bfe_module.BfeHandlerClose {
		t.Errorf("ret should be BfeHandlerClose, got: %d", ret)
	}
}

func TestPrisonHandlerFinishRule(t *testing.T) {
	m, _ := prepareModulePrison(t)
	loadActionRules(t, m)
	req := makePrisonRequest("finish", "/finish")

	ret, _ := m.prisonHandler(req)
	if ret != bfe_module.BfeHandlerFinish {
		t.Errorf("ret should be BfeHandlerFinish, got: %d", ret)
	}
}

func TestPrisonHandlerPassRule(t *testing.T) {
	m, _ := prepareModulePrison(t)
	loadActionRules(t, m)
	req := makePrisonRequest("pass", "/pass")

	ret, _ := m.prisonHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be BfeHandlerGoOn, got: %d", ret)
	}
}

func TestPrisonHandlerNoMatch(t *testing.T) {
	m, _ := prepareModulePrison(t)
	loadActionRules(t, m)
	req := makePrisonRequest("nomatch", "/nomatch")

	ret, _ := m.prisonHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be BfeHandlerGoOn, got: %d", ret)
	}
}

func TestLoadProductRuleTable(t *testing.T) {
	m, _ := prepareModulePrison(t)

	version, err := m.loadProductRuleTable(nil)
	if err != nil {
		t.Errorf("loadProductRuleTable() error: %v", err)
	}
	if version == "" {
		t.Error("version should not be empty")
	}
}

func TestGetState(t *testing.T) {
	m := NewModulePrison()
	_, err := m.getState(nil)
	if err != nil {
		t.Errorf("getState() error: %v", err)
	}
}

func TestGetStateDiff(t *testing.T) {
	m := NewModulePrison()
	_, err := m.getStateDiff(nil)
	if err != nil {
		t.Errorf("getStateDiff() error: %v", err)
	}
}

type testConn struct {
	localAddr  *net.TCPAddr
	remoteAddr *net.TCPAddr
}

func (c *testConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (c *testConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (c *testConn) Close() error                       { return nil }
func (c *testConn) LocalAddr() net.Addr                { return c.localAddr }
func (c *testConn) RemoteAddr() net.Addr               { return c.remoteAddr }
func (c *testConn) SetDeadline(t time.Time) error      { return nil }
func (c *testConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *testConn) SetWriteDeadline(t time.Time) error { return nil }
