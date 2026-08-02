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

package mod_tcp_keepalive

import (
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

func setDebug(t *testing.T, v bool) {
	t.Helper()
	old := openDebug
	openDebug = v
	t.Cleanup(func() { openDebug = old })
}

func prepareTempConfRoot(t *testing.T, dataContent string) string {
	t.Helper()

	dir, err := ioutil.TempDir("", "mod_tcp_keepalive_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	modDir := filepath.Join(dir, "mod_tcp_keepalive")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	if err := ioutil.WriteFile(filepath.Join(modDir, "tcp_keepalive.data"), []byte(dataContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	confContent := `[basic]
DataPath = ./mod_tcp_keepalive/tcp_keepalive.data

[log]
OpenDebug = true
`
	if err := ioutil.WriteFile(filepath.Join(modDir, "mod_tcp_keepalive.conf"), []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return filepath.ToSlash(dir)
}

func newTCPConn(t *testing.T) *net.TCPConn {
	t.Helper()

	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("ListenTCP() error: %v", err)
	}
	t.Cleanup(func() { l.Close() })

	serverCh := make(chan *net.TCPConn, 1)
	go func() {
		c, err := l.AcceptTCP()
		if err == nil {
			serverCh <- c
		}
		close(serverCh)
	}()

	client, err := net.DialTCP("tcp", nil, l.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatalf("DialTCP() error: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	server := <-serverCh
	if server == nil {
		t.Fatal("AcceptTCP() failed")
	}
	t.Cleanup(func() { server.Close() })

	return server
}

func TestNewModuleTcpKeepAlive(t *testing.T) {
	m := NewModuleTcpKeepAlive()
	if m == nil {
		t.Fatal("NewModuleTcpKeepAlive() returns nil")
	}
	if m.Name() != ModTcpKeepAlive {
		t.Errorf("Name() = %q, want %q", m.Name(), ModTcpKeepAlive)
	}
}

func TestModuleTcpKeepAliveInitSuccess(t *testing.T) {
	m := NewModuleTcpKeepAlive()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
}

func TestModuleTcpKeepAliveInitConfNotExist(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_tcp_keepalive_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	m := NewModuleTcpKeepAlive()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, filepath.ToSlash(dir)); err == nil {
		t.Error("Init() should fail when config file does not exist")
	}
}

func TestModuleTcpKeepAliveInitDataLoadFail(t *testing.T) {
	dir := prepareTempConfRoot(t, "invalid json data")

	m := NewModuleTcpKeepAlive()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when rule data file is invalid")
	}
}

func TestModuleTcpKeepAliveInitNilWebHandlers(t *testing.T) {
	dir := prepareTempConfRoot(t, `{
    "Config": {"product1": [{"VipConf": ["10.1.1.1"], "KeepAliveParam": {}}]},
    "Version": "v1"
}`)

	m := NewModuleTcpKeepAlive()
	cb := bfe_module.NewBfeCallbacks()

	if err := m.Init(cb, nil, dir); err == nil {
		t.Error("Init() should fail when WebHandlers is nil")
	}
}

func TestModuleTcpKeepAliveLoadConfDataWithPath(t *testing.T) {
	validData := `{
    "Config": {
        "product1": [
            {
                "VipConf": ["10.1.1.1"],
                "KeepAliveParam": {"KeepIdle": 10}
            }
        ]
    },
    "Version": "v2"
}`
	dir := prepareTempConfRoot(t, validData)

	m := NewModuleTcpKeepAlive()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if m.ruleTable.version != "v2" {
		t.Errorf("initial version = %q, want v2", m.ruleTable.version)
	}

	reloadData := `{
    "Config": {
        "product1": [
            {
                "VipConf": ["10.1.1.1"],
                "KeepAliveParam": {"KeepIdle": 20}
            }
        ]
    },
    "Version": "v3"
}`
	reloadPath := filepath.Join(dir, "mod_tcp_keepalive", "reload.data")
	if err := ioutil.WriteFile(reloadPath, []byte(reloadData), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	query := url.Values{}
	query.Set("path", filepath.ToSlash(reloadPath))
	if err := m.loadConfData(query); err != nil {
		t.Fatalf("loadConfData() error: %v", err)
	}

	if m.ruleTable.version != "v3" {
		t.Errorf("reloaded version = %q, want v3", m.ruleTable.version)
	}
}

func TestModuleTcpKeepAliveLoadConfDataDefaultPath(t *testing.T) {
	validData := `{
    "Config": {"product1": [{"VipConf": ["10.1.1.1"], "KeepAliveParam": {}}]},
    "Version": "v1"
}`
	dir := prepareTempConfRoot(t, validData)

	m := NewModuleTcpKeepAlive()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if err := m.loadConfData(nil); err != nil {
		t.Fatalf("loadConfData(nil) error: %v", err)
	}
}

func TestModuleTcpKeepAliveLoadConfDataFail(t *testing.T) {
	validData := `{
    "Config": {"product1": [{"VipConf": ["10.1.1.1"], "KeepAliveParam": {}}]},
    "Version": "v1"
}`
	dir := prepareTempConfRoot(t, validData)

	m := NewModuleTcpKeepAlive()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	query := url.Values{}
	query.Set("path", filepath.ToSlash(filepath.Join(dir, "not_exist.data")))
	if err := m.loadConfData(query); err == nil {
		t.Error("loadConfData() should fail for non-existent data file")
	}
}

func TestModuleTcpKeepAliveHandlers(t *testing.T) {
	m := NewModuleTcpKeepAlive()

	mon := m.monitorHandlers()
	if mon == nil {
		t.Fatal("monitorHandlers() returns nil")
	}
	if _, ok := mon[ModTcpKeepAlive]; !ok {
		t.Error("monitorHandlers() missing module entry")
	}
	if _, ok := mon[ModTcpKeepAlive+".diff"]; !ok {
		t.Error("monitorHandlers() missing diff entry")
	}

	reload := m.reloadHandlers()
	if reload == nil {
		t.Fatal("reloadHandlers() returns nil")
	}
	if _, ok := reload[ModTcpKeepAlive]; !ok {
		t.Error("reloadHandlers() missing module entry")
	}
}

func TestModuleTcpKeepAliveGetState(t *testing.T) {
	m := NewModuleTcpKeepAlive()
	m.state.ConnToSet.Inc(1)

	b, err := m.getState(nil)
	if err != nil {
		t.Fatalf("getState() error: %v", err)
	}
	if len(b) == 0 {
		t.Error("getState() returns empty response")
	}

	b, err = m.getStateDiff(nil)
	if err != nil {
		t.Fatalf("getStateDiff() error: %v", err)
	}
	if len(b) == 0 {
		t.Error("getStateDiff() returns empty response")
	}
}

func TestDisableKeepAlive(t *testing.T) {
	m := NewModuleTcpKeepAlive()
	conn := newTCPConn(t)

	if err := m.disableKeepAlive(conn); err != nil {
		t.Errorf("disableKeepAlive() error: %v", err)
	}
}

func TestSetKeepAliveParam(t *testing.T) {
	m := NewModuleTcpKeepAlive()
	conn := newTCPConn(t)

	param := KeepAliveParam{
		KeepIdle:  10,
		KeepIntvl: 5,
		KeepCnt:   3,
	}

	if err := m.setKeepAliveParam(conn, param); err != nil {
		t.Errorf("setKeepAliveParam() error: %v", err)
	}
}

func TestDisableKeepAliveError(t *testing.T) {
	m := NewModuleTcpKeepAlive()
	conn := newTCPConn(t)
	conn.Close()

	if err := m.disableKeepAlive(conn); err == nil {
		t.Error("disableKeepAlive() should fail for closed connection")
	}
}

func TestSetKeepAliveParamFileError(t *testing.T) {
	m := NewModuleTcpKeepAlive()
	conn := newTCPConn(t)
	conn.Close()

	param := KeepAliveParam{KeepIdle: 10, KeepIntvl: 5, KeepCnt: 3}
	if err := m.setKeepAliveParam(conn, param); err == nil {
		t.Error("setKeepAliveParam() should fail for closed connection")
	}
}

func TestHandleTcpKeepAliveDisable(t *testing.T) {
	m := NewModuleTcpKeepAlive()
	conn := newTCPConn(t)

	if err := m.handleTcpKeepAlive(conn, KeepAliveParam{Disable: true}); err != nil {
		t.Errorf("handleTcpKeepAlive() disable error: %v", err)
	}
}

func TestHandleTcpKeepAliveSet(t *testing.T) {
	m := NewModuleTcpKeepAlive()
	conn := newTCPConn(t)

	param := KeepAliveParam{KeepIdle: 10, KeepIntvl: 5, KeepCnt: 3}
	if err := m.handleTcpKeepAlive(conn, param); err != nil {
		t.Errorf("handleTcpKeepAlive() set error: %v", err)
	}
}

func TestHandleTcpKeepAliveSetError(t *testing.T) {
	m := NewModuleTcpKeepAlive()

	var closedConn *net.TCPConn
	func() {
		conn := newTCPConn(t)
		conn.Close()
		closedConn = conn
	}()

	param := KeepAliveParam{KeepIdle: 10, KeepIntvl: 5, KeepCnt: 3}
	if err := m.handleTcpKeepAlive(closedConn, param); err == nil {
		t.Error("handleTcpKeepAlive() should fail for closed connection")
	}
}

func TestSetKeepAliveFunctions(t *testing.T) {
	conn := newTCPConn(t)
	f, err := conn.File()
	if err != nil {
		t.Fatalf("conn.File() error: %v", err)
	}
	defer f.Close()
	fd := int(f.Fd())

	setDebug(t, true)

	if err := setIdle(fd, 10); err != nil {
		t.Errorf("setIdle() error: %v", err)
	}
	if err := setInterval(fd, 5); err != nil {
		t.Errorf("setInterval() error: %v", err)
	}
	if err := setCount(fd, 3); err != nil {
		t.Errorf("setCount() error: %v", err)
	}
	if err := setNonblock(fd); err != nil {
		t.Errorf("setNonblock() error: %v", err)
	}

	setDebug(t, false)
	if err := setIdle(fd, 10); err != nil {
		t.Errorf("setIdle() error when debug off: %v", err)
	}
}

func TestGetTcpConnDirect(t *testing.T) {
	m := NewModuleTcpKeepAlive()
	conn := newTCPConn(t)

	tcpConn, err := m.getTcpConn(conn)
	if err != nil {
		t.Fatalf("getTcpConn() error: %v", err)
	}
	if tcpConn != conn {
		t.Error("getTcpConn() should return the same TCPConn")
	}
}

type connFetcher struct {
	net.Conn
}

func (c *connFetcher) GetNetConn() net.Conn {
	return c.Conn
}

func TestGetTcpConnFetcher(t *testing.T) {
	m := NewModuleTcpKeepAlive()
	conn := newTCPConn(t)

	wrapper := &connFetcher{conn}
	tcpConn, err := m.getTcpConn(wrapper)
	if err != nil {
		t.Fatalf("getTcpConn() error: %v", err)
	}
	if tcpConn != conn {
		t.Error("getTcpConn() should return underlying TCPConn")
	}
}

func TestGetTcpConnNonTCP(t *testing.T) {
	m := NewModuleTcpKeepAlive()
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	if _, err := m.getTcpConn(c1); err == nil {
		t.Error("getTcpConn() should fail for non-TCP connection")
	}
}

func prepareSession(t *testing.T, conn net.Conn, product string, vip string) *bfe_basic.Session {
	t.Helper()

	session := new(bfe_basic.Session)
	session.Connection = conn
	session.Product = product
	session.Vip = net.ParseIP(vip)
	if conn != nil {
		if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
			session.RemoteAddr = addr
		} else {
			session.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
		}
	}
	return session
}

const testRuleData = `{
    "Config": {
        "product1": [
            {
                "VipConf": ["10.1.1.1"],
                "KeepAliveParam": {
                    "KeepIdle": 10,
                    "KeepIntvl": 5,
                    "KeepCnt": 3
                }
            },
            {
                "VipConf": ["10.1.1.3"],
                "KeepAliveParam": {
                    "Disable": true
                }
            }
        ]
    },
    "Version": "v1"
}`

func prepareModuleWithData(t *testing.T, dataContent string) *ModuleTcpKeepAlive {
	t.Helper()

	dir := prepareTempConfRoot(t, dataContent)
	m := NewModuleTcpKeepAlive()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	return m
}

func TestHandleAcceptProductNotFound(t *testing.T) {
	m := prepareModuleWithData(t, testRuleData)

	conn := newTCPConn(t)
	session := prepareSession(t, conn, "notexist", "10.1.1.1")

	ret := m.HandleAccept(session)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestHandleAcceptVipNotFound(t *testing.T) {
	m := prepareModuleWithData(t, testRuleData)

	conn := newTCPConn(t)
	session := prepareSession(t, conn, "product1", "10.9.9.9")

	ret := m.HandleAccept(session)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestHandleAcceptVipFoundSet(t *testing.T) {
	m := prepareModuleWithData(t, testRuleData)

	conn := newTCPConn(t)
	session := prepareSession(t, conn, "product1", "10.1.1.1")

	ret := m.HandleAccept(session)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestHandleAcceptVipFoundDisable(t *testing.T) {
	m := prepareModuleWithData(t, testRuleData)

	conn := newTCPConn(t)
	session := prepareSession(t, conn, "product1", "10.1.1.3")

	ret := m.HandleAccept(session)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestHandleAcceptGetTcpConnError(t *testing.T) {
	m := prepareModuleWithData(t, testRuleData)

	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	session := prepareSession(t, c1, "product1", "10.1.1.1")

	ret := m.HandleAccept(session)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestHandleAcceptHandleError(t *testing.T) {
	m := prepareModuleWithData(t, testRuleData)

	conn := newTCPConn(t)
	conn.Close()

	session := prepareSession(t, conn, "product1", "10.1.1.1")

	ret := m.HandleAccept(session)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestHandleAcceptProductNotFoundDebugOff(t *testing.T) {
	setDebug(t, false)
	m := prepareModuleWithData(t, testRuleData)

	conn := newTCPConn(t)
	session := prepareSession(t, conn, "notexist", "10.1.1.1")

	ret := m.HandleAccept(session)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestHandleAcceptVipNotFoundDebugOff(t *testing.T) {
	setDebug(t, false)
	m := prepareModuleWithData(t, testRuleData)

	conn := newTCPConn(t)
	session := prepareSession(t, conn, "product1", "10.9.9.9")

	ret := m.HandleAccept(session)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestHandleAcceptVipFoundDebugOff(t *testing.T) {
	setDebug(t, false)
	m := prepareModuleWithData(t, testRuleData)

	conn := newTCPConn(t)
	session := prepareSession(t, conn, "product1", "10.1.1.1")

	ret := m.HandleAccept(session)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestHandleAcceptWithDebug(t *testing.T) {
	setDebug(t, true)
	m := prepareModuleWithData(t, testRuleData)

	conn := newTCPConn(t)
	session := prepareSession(t, conn, "product1", "10.1.1.1")

	ret := m.HandleAccept(session)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestMonitorAndReloadHandlerInvocation(t *testing.T) {
	validData := `{
    "Config": {"product1": [{"VipConf": ["10.1.1.1"], "KeepAliveParam": {}}]},
    "Version": "v1"
}`
	dir := prepareTempConfRoot(t, validData)

	m := NewModuleTcpKeepAlive()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	reloadHandler, ok := m.reloadHandlers()[ModTcpKeepAlive].(func(url.Values) error)
	if !ok {
		t.Fatal("reload handler type mismatch")
	}
	if err := reloadHandler(nil); err != nil {
		t.Errorf("reload handler error: %v", err)
	}

	stateHandler, ok := m.monitorHandlers()[ModTcpKeepAlive].(func(map[string][]string) ([]byte, error))
	if !ok {
		t.Fatal("state handler type mismatch")
	}
	b, err := stateHandler(nil)
	if err != nil {
		t.Errorf("state handler error: %v", err)
	}
	if len(b) == 0 {
		t.Error("state handler returns empty response")
	}

	diffHandler, ok := m.monitorHandlers()[ModTcpKeepAlive+".diff"].(func(map[string][]string) ([]byte, error))
	if !ok {
		t.Fatal("diff handler type mismatch")
	}
	b, err = diffHandler(nil)
	if err != nil {
		t.Errorf("diff handler error: %v", err)
	}
	if len(b) == 0 {
		t.Error("diff handler returns empty response")
	}
}
