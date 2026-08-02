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

package mod_access

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

func TestNewModuleAccess(t *testing.T) {
	m := NewModuleAccess()
	if m == nil {
		t.Fatal("NewModuleAccess() returns nil")
	}
	if m.Name() != "mod_access" {
		t.Errorf("module name error, got: %s, want: mod_access", m.Name())
	}
}

func TestParseConfig(t *testing.T) {
	m := NewModuleAccess()
	conf := &ConfModAccess{}
	conf.Template.RequestTemplate = "$time $status_code"
	conf.Template.SessionTemplate = "$ses_clientip $ses_start_time"

	err := m.ParseConfig(conf)
	if err != nil {
		t.Errorf("ParseConfig() error: %v", err)
	}
	if len(m.reqFmts) == 0 {
		t.Error("reqFmts should not be empty")
	}
	if len(m.sessionFmts) == 0 {
		t.Error("sessionFmts should not be empty")
	}
}

func TestParseConfigInvalidRequestTemplate(t *testing.T) {
	m := NewModuleAccess()
	conf := &ConfModAccess{}
	conf.Template.RequestTemplate = "$unknown"
	conf.Template.SessionTemplate = "$ses_clientip"

	err := m.ParseConfig(conf)
	if err == nil {
		t.Error("ParseConfig() should return error for invalid request template")
	}
}

func TestParseConfigInvalidSessionTemplate(t *testing.T) {
	m := NewModuleAccess()
	conf := &ConfModAccess{}
	conf.Template.RequestTemplate = "$time"
	conf.Template.SessionTemplate = "$unknown"

	err := m.ParseConfig(conf)
	if err == nil {
		t.Error("ParseConfig() should return error for invalid session template")
	}
}

func TestCheckLogFormat(t *testing.T) {
	m := NewModuleAccess()
	conf := &ConfModAccess{}
	conf.Template.RequestTemplate = "$time $status_code"
	conf.Template.SessionTemplate = "$ses_clientip $ses_start_time"

	if err := m.ParseConfig(conf); err != nil {
		t.Fatalf("ParseConfig() error: %v", err)
	}

	if err := m.CheckLogFormat(); err != nil {
		t.Errorf("CheckLogFormat() error: %v", err)
	}
}

func TestCheckLogFormatSessionItemInRequest(t *testing.T) {
	m := NewModuleAccess()
	conf := &ConfModAccess{}
	conf.Template.RequestTemplate = "$ses_clientip"
	conf.Template.SessionTemplate = "$ses_clientip"

	if err := m.ParseConfig(conf); err != nil {
		t.Fatalf("ParseConfig() error: %v", err)
	}

	err := m.CheckLogFormat()
	if err == nil {
		t.Error("CheckLogFormat() should return error when session item used in request log")
	}
}

func TestCheckLogFormatRequestItemInSession(t *testing.T) {
	m := NewModuleAccess()
	conf := &ConfModAccess{}
	conf.Template.RequestTemplate = "$time"
	conf.Template.SessionTemplate = "$status_code"

	if err := m.ParseConfig(conf); err != nil {
		t.Fatalf("ParseConfig() error: %v", err)
	}

	err := m.CheckLogFormat()
	if err == nil {
		t.Error("CheckLogFormat() should return error when request item used in session log")
	}
}

func TestInit(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_access_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	modConfDir := filepath.Join(dir, "mod_access")
	if err := os.MkdirAll(modConfDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	confPath := filepath.Join(modConfDir, "mod_access.conf")
	content := `[Log]
LogFile = access.log

[Template]
RequestTemplate = "REQUEST_LOG $time $status_code"
SessionTemplate = "SESSION_LOG $time $ses_clientip"
`
	if err := ioutil.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := NewModuleAccess()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err = m.Init(cb, wh, dir)
	if err != nil {
		t.Errorf("Init() error: %v", err)
	}
}

func prepareModuleAccess(t *testing.T) (*ModuleAccess, string) {
	dir, err := ioutil.TempDir("", "mod_access_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}

	logFile, err := ioutil.TempFile(dir, "access.log")
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("TempFile() error: %v", err)
	}
	logFile.Close()

	conf := &ConfModAccess{}
	conf.Log.LogFile = logFile.Name()
	conf.Template.RequestTemplate = "REQUEST_LOG $time clientip: $remote_addr host: $host product: $product status: $status_code error: $error"
	conf.Template.SessionTemplate = "SESSION_LOG $time clientip: $ses_clientip start_time: $ses_start_time end_time: $ses_end_time keepalive_num: $ses_keepalive_num error: $ses_error"

	m := NewModuleAccess()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err = m.init(conf, cb, wh)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("init() error: %v", err)
	}

	return m, dir
}

func makeBasicRequest() (*bfe_basic.Request, *bfe_basic.Session) {
	session := bfe_basic.NewSession(nil)
	session.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}
	session.StartTime = time.Now().Add(-time.Second)
	session.EndTime = time.Now()
	session.SetReqNum(3)

	req, _ := bfe_http.NewRequest("GET", "http://www.example.org/path?key=value", nil)
	req.Header.Set("User-Agent", "test-agent")
	req.State = &bfe_http.RequestState{SerialNumber: 2}

	stat := bfe_basic.NewRequestStat(time.Now().Add(-500 * time.Millisecond))
	stat.ReadReqEnd = time.Now().Add(-400 * time.Millisecond)
	stat.ClusterStart = time.Now().Add(-300 * time.Millisecond)
	stat.ClusterEnd = time.Now().Add(-200 * time.Millisecond)
	stat.BackendStart = time.Now().Add(-250 * time.Millisecond)
	stat.BackendEnd = time.Now().Add(-150 * time.Millisecond)
	stat.ResponseStart = time.Now().Add(-100 * time.Millisecond)
	stat.ResponseEnd = time.Now().Add(-50 * time.Millisecond)
	stat.HeaderLenIn = 100
	stat.BodyLenIn = 200
	stat.HeaderLenOut = 80
	stat.BodyLenOut = 160

	bfeReq := bfe_basic.NewRequest(req, nil, stat, session, nil)
	bfeReq.RemoteAddr = session.RemoteAddr
	bfeReq.LogId = "logid-123"
	bfeReq.Route.Product = "unittest"
	bfeReq.Backend = bfe_basic.BackendInfo{
		ClusterName:    "cluster1",
		SubclusterName: "subcluster1",
		BackendAddr:    "10.0.0.1",
		BackendName:    "backend1",
	}
	bfeReq.RetryTime = 1

	return bfeReq, session
}

func TestRequestLogHandler(t *testing.T) {
	m, dir := prepareModuleAccess(t)
	defer os.RemoveAll(dir)

	req, _ := makeBasicRequest()
	res := &bfe_http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Proto:      "HTTP/1.1",
		Header:     make(bfe_http.Header),
	}
	res.Header.Set("Content-Type", "text/html")

	ret := m.requestLogHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("requestLogHandler() ret error, got: %d, want: %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestSessionLogHandler(t *testing.T) {
	m, dir := prepareModuleAccess(t)
	defer os.RemoveAll(dir)

	_, session := makeBasicRequest()

	ret := m.sessionLogHandler(session)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("sessionLogHandler() ret error, got: %d, want: %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestRequestLogHandlerNilReq(t *testing.T) {
	m, dir := prepareModuleAccess(t)
	defer os.RemoveAll(dir)

	ret := m.requestLogHandler(nil, nil)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("requestLogHandler() ret error, got: %d, want: %d", ret, bfe_module.BfeHandlerGoOn)
	}
}

func TestSessionLogHandlerNilSession(t *testing.T) {
	m, dir := prepareModuleAccess(t)
	defer os.RemoveAll(dir)

	ret := m.sessionLogHandler(nil)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("sessionLogHandler() ret error, got: %d, want: %d", ret, bfe_module.BfeHandlerGoOn)
	}
}
