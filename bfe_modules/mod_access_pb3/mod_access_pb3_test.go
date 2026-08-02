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
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
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

func newTestConn() net.Conn {
	return &testConn{
		localAddr:  &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080},
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345},
	}
}

func TestNewModuleAccessPb3(t *testing.T) {
	m := NewModuleAccessPb3()
	if m == nil {
		t.Fatal("NewModuleAccessPb3() returns nil")
	}
	if m.Name() != "mod_access_pb3" {
		t.Errorf("module name error, got: %s, want: mod_access_pb3", m.Name())
	}
}

func TestModuleAccessPb3Close(t *testing.T) {
	m := NewModuleAccessPb3()
	err := m.Close()
	if err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func prepareModule(t *testing.T) (*ModuleAccessPb3, string) {
	dir, err := ioutil.TempDir("", "mod_access_pb3_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}

	modConfDir := filepath.Join(dir, "mod_access_pb3")
	if err := os.MkdirAll(modConfDir, 0755); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("MkdirAll() error: %v", err)
	}

	confPath := filepath.Join(modConfDir, "mod_access_pb3.conf")
	content := `[Log]
LogPrefix = pb_access3
LogDir = ./
RotateWhen = NEXTHOUR
BackupCount = 2

[BasicConf]
OpenDebug = true
`
	if err := ioutil.WriteFile(confPath, []byte(content), 0644); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := NewModuleAccessPb3()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err = m.Init(cb, wh, dir)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("Init() error: %v", err)
	}

	return m, dir
}

func makeBasicRequest(t *testing.T) (*bfe_basic.Request, *bfe_http.Response) {
	conn := newTestConn()

	session := bfe_basic.NewSession(conn)
	session.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}
	session.Vip = net.ParseIP("10.0.0.1")
	session.StartTime = time.Now().Add(-time.Second)
	session.SetTrustSource(true)

	req, err := bfe_http.NewRequest("GET", "http://www.example.org/path?key=value", nil)
	if err != nil {
		t.Fatalf("NewRequest() error: %v", err)
	}
	req.RequestURI = "/path?key=value"
	req.Header.Set("User-Agent", "test-agent")
	req.Header.Set("Referer", "http://referer.example.org")
	req.State = &bfe_http.RequestState{SerialNumber: 2}

	stat := bfe_basic.NewRequestStat(time.Now().Add(-500 * time.Millisecond))
	stat.ReadReqEnd = time.Now().Add(-400 * time.Millisecond)
	stat.ClusterStart = time.Now().Add(-300 * time.Millisecond)
	stat.ClusterEnd = time.Now().Add(-200 * time.Millisecond)
	stat.BackendStart = time.Now().Add(-250 * time.Millisecond)
	stat.BackendEnd = time.Now().Add(-150 * time.Millisecond)
	stat.ResponseStart = time.Now().Add(-100 * time.Millisecond)
	stat.ResponseEnd = time.Now().Add(-50 * time.Millisecond)
	stat.BackendFirst = time.Now().Add(-390 * time.Millisecond)
	stat.HeaderLenIn = 100
	stat.BodyLenIn = 200
	stat.HeaderLenOut = 80
	stat.BodyLenOut = 160

	bfeReq := bfe_basic.NewRequest(req, conn, stat, session, nil)
	bfeReq.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 54321}
	bfeReq.LogId = "12345"
	bfeReq.Route.Product = "unittest"
	bfeReq.Backend = bfe_basic.BackendInfo{
		ClusterName:    "cluster1",
		SubclusterName: "subcluster1",
		BackendAddr:    "10.0.0.3",
		BackendPort:    8080,
		BackendName:    "backend1",
	}
	bfeReq.RetryTime = 1

	res := &bfe_http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Proto:      "HTTP/1.1",
		Header:     make(bfe_http.Header),
	}
	res.Header.Set("Content-Type", "text/html")

	return bfeReq, res
}

func TestInit(t *testing.T) {
	m, dir := prepareModule(t)
	defer os.RemoveAll(dir)

	if m.conf == nil {
		t.Error("m.conf should not be nil after Init")
	}
	if m.logger == nil {
		t.Error("m.logger should not be nil after Init")
	}
}

func TestInitConfNotExist(t *testing.T) {
	m := NewModuleAccessPb3()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./test_data/not_exist")
	if err == nil {
		t.Error("Init() should return error when config file not exist")
	}
}

func TestRequestFinishHandler(t *testing.T) {
	m, dir := prepareModule(t)
	defer os.RemoveAll(dir)

	req, res := makeBasicRequest(t)
	ret := m.requestFinishHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("requestFinishHandler() ret error, got: %d, want: %d", ret, bfe_module.BfeHandlerGoOn)
	}
	if m.state.AllReqLogCount.Get() != 1 {
		t.Errorf("AllReqLogCount should be 1, got: %d", m.state.AllReqLogCount.Get())
	}
}

func TestSessionFinishHandler(t *testing.T) {
	m, dir := prepareModule(t)
	defer os.RemoveAll(dir)

	conn := newTestConn()
	session := bfe_basic.NewSession(conn)
	session.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}
	session.SessionId = "100"
	session.StartTime = time.Now().Add(-time.Second)
	session.EndTime = time.Now()
	session.SetReqNum(3)
	session.UpdateReadTotal(1024)
	session.UpdateWriteTotal(2048)

	ret := m.sessionFinishHandler(session)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("sessionFinishHandler() ret error, got: %d, want: %d", ret, bfe_module.BfeHandlerGoOn)
	}
	if m.state.AllSesLogCount.Get() != 1 {
		t.Errorf("AllSesLogCount should be 1, got: %d", m.state.AllSesLogCount.Get())
	}
}

func TestGetState(t *testing.T) {
	m := NewModuleAccessPb3()
	_, err := m.getState(nil)
	if err != nil {
		t.Errorf("getState() error: %v", err)
	}
}

func TestGetStateDiff(t *testing.T) {
	m := NewModuleAccessPb3()
	_, err := m.getStateDiff(nil)
	if err != nil {
		t.Errorf("getStateDiff() error: %v", err)
	}
}
