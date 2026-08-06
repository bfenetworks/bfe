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

package mod_trust_clientip

import (
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util/net_util"
)

const validTrustIPConf = `{
    "Config": {
        "test": [
            {
                "begin": "127.0.0.1",
                "end": "127.0.0.1"
            },
            {
                "begin": "10.0.0.0",
                "end": "10.0.0.255"
            },
            {
                "begin": "192.168.1.0",
                "end": "192.168.1.255"
            },
            {
                "begin": "::1",
                "end": "::1"
            }
        ]
    },
    "Version": "test-version"
}
`

// prepareTempConf creates a temporary conf root that contains
// mod_trust_clientip/mod_trust_clientip.conf and optionally
// mod_trust_clientip/trust_client_ip.data. The caller does not need to clean
// up the returned directory.
func prepareTempConf(t *testing.T, trustIPContent string) string {
	t.Helper()

	dir, err := ioutil.TempDir("", "mod_trust_clientip_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	modDir := filepath.Join(dir, "mod_trust_clientip")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	if trustIPContent != "" {
		if err := ioutil.WriteFile(filepath.Join(modDir, "trust_client_ip.data"), []byte(trustIPContent), 0644); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}
	}

	confContent := `[basic]
DataPath = mod_trust_clientip/trust_client_ip.data

[log]
OpenDebug = false
`
	if err := ioutil.WriteFile(filepath.Join(modDir, "mod_trust_clientip.conf"), []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return dir
}

func prepareSession(ip string) *bfe_basic.Session {
	s := new(bfe_basic.Session)
	s.RemoteAddr = &net.TCPAddr{}
	s.RemoteAddr.IP = net_util.ParseIPv4(ip)
	return s
}

func TestNewModuleTrustClientIPAndName(t *testing.T) {
	m := NewModuleTrustClientIP()
	if m == nil {
		t.Fatal("NewModuleTrustClientIP() returns nil")
	}

	if m.Name() != ModTrustClientIP {
		t.Errorf("Name() should be %s, got %s", ModTrustClientIP, m.Name())
	}
}

func TestModuleTrustClientIPInitSuccess(t *testing.T) {
	dir := prepareTempConf(t, validTrustIPConf)

	m := NewModuleTrustClientIP()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err != nil {
		t.Errorf("Init() error: %v", err)
	}
}

func TestModuleTrustClientIPInitConfNotExist(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_trust_clientip_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	m := NewModuleTrustClientIP()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when config file does not exist")
	}
}

func TestModuleTrustClientIPInitBadTrustIPConf(t *testing.T) {
	badTrustIPConf := `{
    "Config": {
        "test": [
            {
                "begin": "10.0.0.0",
                "end": "10.0.0.abc"
            }
        ]
    },
    "Version": "bad-version"
}
`
	dir := prepareTempConf(t, badTrustIPConf)

	m := NewModuleTrustClientIP()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when trust ip config file is invalid")
	}
}

func TestModuleTrustClientIPInitDataPathNotExist(t *testing.T) {
	dir := prepareTempConf(t, "")

	// make sure the data file does not exist
	dataPath := filepath.Join(dir, "mod_trust_clientip", "trust_client_ip.data")
	os.Remove(dataPath)

	m := NewModuleTrustClientIP()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when trust ip data file does not exist")
	}
}

func TestModuleTrustClientIPGetState(t *testing.T) {
	m := NewModuleTrustClientIP()
	m.state.ConnTotal.Inc(1)

	b, err := m.getState(nil)
	if err != nil {
		t.Errorf("getState() error: %v", err)
	}
	if len(b) == 0 {
		t.Error("getState() returns empty response")
	}

	b, err = m.getStateDiff(nil)
	if err != nil {
		t.Errorf("getStateDiff() error: %v", err)
	}
	if len(b) == 0 {
		t.Error("getStateDiff() returns empty response")
	}

	if m.monitorHandlers() == nil {
		t.Error("monitorHandlers() returns nil")
	}
}

func TestModuleTrustClientIPAcceptHandlerTrusted(t *testing.T) {
	dir := prepareTempConf(t, validTrustIPConf)

	m := NewModuleTrustClientIP()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	testCases := []string{
		"127.0.0.1",
		"10.0.0.1",
		"10.0.0.255",
		"192.168.1.1",
		"192.168.1.255",
	}

	for _, ip := range testCases {
		s := prepareSession(ip)
		ret := m.acceptHandler(s)
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("%s: acceptHandler() should return BfeHandlerGoOn, got %d", ip, ret)
		}
		if !s.TrustSource() {
			t.Errorf("%s should be trusted", ip)
		}
	}

	// IPv6 trusted address
	s := new(bfe_basic.Session)
	s.RemoteAddr = &net.TCPAddr{}
	s.RemoteAddr.IP = net.ParseIP("::1")
	ret := m.acceptHandler(s)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("::1: acceptHandler() should return BfeHandlerGoOn, got %d", ret)
	}
	if !s.TrustSource() {
		t.Error("::1 should be trusted")
	}
}

func TestModuleTrustClientIPAcceptHandlerNotTrusted(t *testing.T) {
	dir := prepareTempConf(t, validTrustIPConf)

	m := NewModuleTrustClientIP()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	testCases := []string{
		"128.0.0.1",
		"192.168.0.1",
		"10.1.0.1",
		"8.8.8.8",
	}

	for _, ip := range testCases {
		s := prepareSession(ip)
		ret := m.acceptHandler(s)
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("%s: acceptHandler() should return BfeHandlerGoOn, got %d", ip, ret)
		}
		if s.TrustSource() {
			t.Errorf("%s should not be trusted", ip)
		}
	}

	// IPv6 not trusted address
	s := new(bfe_basic.Session)
	s.RemoteAddr = &net.TCPAddr{}
	s.RemoteAddr.IP = net.ParseIP("1::1")
	ret := m.acceptHandler(s)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("1::1: acceptHandler() should return BfeHandlerGoOn, got %d", ret)
	}
	if s.TrustSource() {
		t.Error("1::1 should not be trusted")
	}
}

func TestModuleTrustClientIPAcceptHandlerInternal(t *testing.T) {
	dir := prepareTempConf(t, validTrustIPConf)

	m := NewModuleTrustClientIP()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// 10.0.0.1 is internal and also trusted
	s := prepareSession("10.0.0.1")
	m.acceptHandler(s)
	if !s.TrustSource() {
		t.Error("10.0.0.1 should be trusted")
	}

	// 10.1.0.1 is internal but not trusted
	s = prepareSession("10.1.0.1")
	m.acceptHandler(s)
	if s.TrustSource() {
		t.Error("10.1.0.1 should not be trusted")
	}
}

func TestModuleTrustClientIPLoadConfDataWithPath(t *testing.T) {
	dir := prepareTempConf(t, validTrustIPConf)

	m := NewModuleTrustClientIP()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// reload with explicit path parameter
	query := url.Values{}
	query.Set("path", filepath.Join(dir, "mod_trust_clientip", "trust_client_ip.data"))
	if err := m.loadConfData(query); err != nil {
		t.Errorf("loadConfData() error: %v", err)
	}
}
