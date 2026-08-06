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

package mod_unified_waf

import (
	"net"
	"net/http"
	"net/url"
	"testing"

	bwi "github.com/bfenetworks/bwi/bwi"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

func prepareRequest(product, path string) *bfe_basic.Request {
	req := new(bfe_basic.Request)
	req.HttpRequest = new(bfe_http.Request)
	req.HttpRequest.Header = make(bfe_http.Header)
	req.HttpRequest.Method = "GET"
	req.HttpRequest.URL = &url.URL{Path: path}
	req.HttpRequest.Proto = "HTTP/1.1"
	req.HttpRequest.ProtoMajor = 1
	req.HttpRequest.ProtoMinor = 1
	req.Route.Product = product
	req.Session = new(bfe_basic.Session)
	req.Context = make(map[interface{}]interface{})
	req.LogId = "test-logid"
	return req
}

type fakeWafResult struct {
	flag    int
	eventID string
}

func (r *fakeWafResult) GetResultFlag() int {
	return r.flag
}

func (r *fakeWafResult) GetEventId() string {
	return r.eventID
}

type fakeWafServer struct {
	block   bool
	eventID string
}

func (f *fakeWafServer) DetectRequest(req *http.Request, logId string) (bwi.WafResult, error) {
	flag := bwi.WAF_RESULT_PASS
	if f.block {
		flag = bwi.WAF_RESULT_BLOCK
	}
	return &fakeWafResult{flag: flag, eventID: f.eventID}, nil
}

func (f *fakeWafServer) UpdateSockFactory(socketFactory func() (net.Conn, error)) {
}

func (f *fakeWafServer) Close() {
}

func newFakeWafClient(block bool, eventID string) *WafClient {
	c := &WafClient{}
	c.monitor = NewMonitorStates()
	c.serverAddress = "127.0.0.1:1"
	c.serverIP = "127.0.0.1"
	c.available.Store(true)
	c.globalWafParam = &GlobalParam{}
	c.globalWafParam.WafClient.Concurrency = 1
	c.globalWafParam.WafClient.MaxWaitCount = 1
	c.globalWafParam.WafDetect.ReqTimeout = 1000
	c.globalWafParam.WafDetect.RetryMax = 0
	c.concurrency = 1
	c.concurrencyChan = make(chan int, 1)
	c.concurrencyChan <- 1
	c.maxWaitCount.Store(1)
	c.curWaitCount.Store(0)
	c.client = &fakeWafServer{block: block, eventID: eventID}
	return c
}

func newTestModuleWafWithFakeClient(block bool, eventID string) *ModuleWaf {
	m := &ModuleWaf{
		name:       ModUnifiedWaf,
		monitor:    NewMonitorStates(),
		prodParams: NewProductParamTable(),
	}

	conf, err := ProductParamLoadAndCheck("./testdata/product_param.data")
	if err != nil {
		panic(err)
	}
	m.prodParams.Update(conf.Config, conf.Version)

	pool := &WafClientPool{
		monitor:    m.monitor,
		wafClients: map[string]*WafClient{},
	}
	pool.wafClients["mock"] = newFakeWafClient(block, eventID)
	m.wafClientPool = pool

	return m
}

func TestNewModuleWaf(t *testing.T) {
	m := NewModuleWaf()
	if m == nil {
		t.Fatal("NewModuleWaf() returns nil")
	}
	if m.Name() != ModUnifiedWaf {
		t.Errorf("Name() = %s, want %s", m.Name(), ModUnifiedWaf)
	}
	if m.wafClientPool == nil {
		t.Errorf("wafClientPool is nil")
	}
	if m.prodParams == nil {
		t.Errorf("prodParams is nil")
	}
	if m.monitor == nil {
		t.Errorf("monitor is nil")
	}
}

func TestModuleWafInitSuccess(t *testing.T) {
	m := NewModuleWaf()
	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()

	err := m.Init(cbs, whs, "./testdata")
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if m.Name() != ModUnifiedWaf {
		t.Errorf("Name() = %s, want %s", m.Name(), ModUnifiedWaf)
	}
	if m.conf == nil {
		t.Errorf("conf is nil after Init()")
	}
	if m.isNoneWaf {
		t.Errorf("isNoneWaf should be false for BFEMockWaf")
	}
}

func TestModuleWafInitSuccessNone(t *testing.T) {
	m := NewModuleWaf()
	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()

	err := m.Init(cbs, whs, "./testdata/mod_unified_waf_none")
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if !m.isNoneWaf {
		t.Errorf("isNoneWaf should be true for None")
	}
}

func TestModuleWafInitFailureMissingConf(t *testing.T) {
	m := NewModuleWaf()
	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()

	err := m.Init(cbs, whs, "./testdata/mod_unified_waf_not_exist")
	if err == nil {
		t.Errorf("Init() should return error for missing config")
	}
}

func TestModuleWafInitFailureBadProduct(t *testing.T) {
	m := NewModuleWaf()
	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()

	err := m.Init(cbs, whs, "./testdata/mod_unified_waf_bad_product")
	if err == nil {
		t.Errorf("Init() should return error for unsupported WafProductName")
	}
}

func TestModuleWafInitFailureMissingPath(t *testing.T) {
	m := NewModuleWaf()
	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()

	err := m.Init(cbs, whs, "./testdata/mod_unified_waf_missing_path")
	if err == nil {
		t.Errorf("Init() should return error when ProductParamPath is missing")
	}
}

func TestModuleWafInitFailureBadWafData(t *testing.T) {
	m := NewModuleWaf()
	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()

	err := m.Init(cbs, whs, "./testdata/mod_unified_waf_bad_data")
	if err == nil {
		t.Errorf("Init() should return error when mod_unified_waf.data is invalid")
	}
}

func TestModuleWafHandlerNoWafConfig(t *testing.T) {
	m := newTestModuleWafWithFakeClient(false, "")

	req := prepareRequest("UnknownProduct", "/")
	ret, resp := m.wafHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("wafHandler() ret = %v, want %v", ret, bfe_module.BfeHandlerGoOn)
	}
	if resp != nil {
		t.Errorf("wafHandler() resp should be nil")
	}

	info := bfe_basic.GetWafInfo(req)
	if info.WafStatus != bfe_basic.WAF_NO_CHECK {
		t.Errorf("WafStatus = %d, want %d", info.WafStatus, bfe_basic.WAF_NO_CHECK)
	}
}

func TestModuleWafHandlerPass(t *testing.T) {
	m := newTestModuleWafWithFakeClient(false, "")

	req := prepareRequest("ProductA", "/")
	ret, resp := m.wafHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("wafHandler() ret = %v, want %v", ret, bfe_module.BfeHandlerGoOn)
	}
	if resp != nil {
		t.Errorf("wafHandler() resp should be nil for pass")
	}

	info := bfe_basic.GetWafInfo(req)
	if info.WafStatus != bfe_basic.WAF_PASS {
		t.Errorf("WafStatus = %d, want %d", info.WafStatus, bfe_basic.WAF_PASS)
	}
}

func TestModuleWafHandlerBlock(t *testing.T) {
	m := newTestModuleWafWithFakeClient(true, "evt-123")

	req := prepareRequest("ProductA", "/")
	ret, resp := m.wafHandler(req)
	if ret != bfe_module.BfeHandlerFinish {
		t.Errorf("wafHandler() ret = %v, want %v", ret, bfe_module.BfeHandlerFinish)
	}
	if resp == nil {
		t.Fatalf("wafHandler() resp should not be nil for block")
	}
	if resp.StatusCode != bfe_http.StatusOK {
		t.Errorf("resp.StatusCode = %d, want %d", resp.StatusCode, bfe_http.StatusOK)
	}

	info := bfe_basic.GetWafInfo(req)
	if info.WafStatus != bfe_basic.WAF_FORBIDDEN {
		t.Errorf("WafStatus = %d, want %d", info.WafStatus, bfe_basic.WAF_FORBIDDEN)
	}
}

func TestModuleWafHandlerNoAvailableClient(t *testing.T) {
	m := NewModuleWaf()
	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()

	if err := m.Init(cbs, whs, "./testdata"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	req := prepareRequest("ProductA", "/")
	before := m.monitor.state.GetCounter(bfe_basic.REQ_NO_CHECK)
	ret, resp := m.wafHandler(req)
	after := m.monitor.state.GetCounter(bfe_basic.REQ_NO_CHECK)

	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("wafHandler() ret = %v, want %v", ret, bfe_module.BfeHandlerGoOn)
	}
	if resp != nil {
		t.Errorf("wafHandler() resp should be nil when no available client")
	}
	if after != before+1 {
		t.Errorf("REQ_NO_CHECK counter = %d, want %d", after, before+1)
	}
}
