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

package mod_body_process

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

func newRawEvent(data string) *RawEvent {
	e := RawEvent([]byte(data))
	return &e
}

type unsupportedEvent struct{}

func (unsupportedEvent) ToBytes() []byte           { return nil }
func (unsupportedEvent) GetQuotaUsage() QuotaUsage { return QuotaUsage{} }

func newTestRequest(product string) *bfe_basic.Request {
	httpReq, _ := bfe_http.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions", nil)
	req := bfe_basic.NewRequest(httpReq, nil, nil, nil, nil)
	req.Route = bfe_basic.RequestRoute{Product: product}
	return req
}

func newTestRequestWithBody(product string, body string) *bfe_basic.Request {
	httpReq, _ := bfe_http.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions",
		io.NopCloser(bytes.NewBufferString(body)))
	httpReq.Header.Set("Content-Type", "application/json")
	req := bfe_basic.NewRequest(httpReq, nil, nil, nil, nil)
	req.Route = bfe_basic.RequestRoute{Product: product}
	return req
}

func TestNewModuleBodyProcessAndName(t *testing.T) {
	m := NewModuleBodyProcess()
	if m == nil {
		t.Fatal("NewModuleBodyProcess should not return nil")
	}
	if m.Name() != ModBodyProcess {
		t.Errorf("expected name %s, got %s", ModBodyProcess, m.Name())
	}
	if m.ruleTable == nil {
		t.Error("ruleTable should be initialized")
	}
}

func TestModuleInitSuccess(t *testing.T) {
	m := NewModuleBodyProcess()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if m.conf == nil {
		t.Fatal("conf should be loaded")
	}
}

func TestModuleInitConfNotExist(t *testing.T) {
	m := NewModuleBodyProcess()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata_not_exist")
	if err == nil {
		t.Error("Init() should return error when config file not exist")
	}
}

func TestModuleInitDataLoadError(t *testing.T) {
	confRoot, err := ioutil.TempDir("", "mod_body_process_init_fail")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(confRoot)

	modDir := filepath.Join(confRoot, "mod_body_process")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	confContent := "[basic]\nProductRulePath = mod_body_process/missing.data\n[log]\nOpenDebug = false\n"
	confPath := filepath.Join(modDir, "mod_body_process.conf")
	if err := ioutil.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := NewModuleBodyProcess()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err = m.Init(cb, wh, confRoot)
	if err == nil {
		t.Error("Init() should return error when data file load fails")
	}
}

func TestLoadProductRuleConfDefaultPath(t *testing.T) {
	m := NewModuleBodyProcess()
	m.conf = &ConfModBodyProcess{}
	m.conf.Basic.ProductRulePath = "./testdata/mod_body_process/body_process.data"

	if err := m.loadProductRuleConf(nil); err != nil {
		t.Fatalf("loadProductRuleConf(nil) error: %v", err)
	}
}

func TestLoadProductRuleConfWithQueryPath(t *testing.T) {
	m := NewModuleBodyProcess()
	m.conf = &ConfModBodyProcess{}
	m.conf.Basic.ProductRulePath = "./testdata/mod_body_process/body_process.data"

	query := url.Values{}
	query.Set("path", "./testdata/mod_body_process/body_process.data")
	if err := m.loadProductRuleConf(query); err != nil {
		t.Fatalf("loadProductRuleConf(path) error: %v", err)
	}
}

func TestLoadProductRuleConfFailure(t *testing.T) {
	m := NewModuleBodyProcess()
	m.conf = &ConfModBodyProcess{}
	m.conf.Basic.ProductRulePath = "./testdata/mod_body_process/body_process.data"

	query := url.Values{}
	query.Set("path", "./testdata/mod_body_process/not_exist.data")
	if err := m.loadProductRuleConf(query); err == nil {
		t.Error("loadProductRuleConf(invalid path) should return error")
	}
}

func TestMatchProcessRule(t *testing.T) {
	m := prepareTestModule(t)

	req := newTestRequest("AI_product")
	rule := m.matchProcessRule(req)
	if rule == nil {
		t.Fatal("expected matched rule")
	}

	req = newTestRequest("unknown_product")
	rule = m.matchProcessRule(req)
	if rule != nil {
		t.Error("expected no rule for unknown product")
	}
}

func TestAfterLocationHandlerNoAiBasicInfo(t *testing.T) {
	m := prepareTestModule(t)
	req := newTestRequest("AI_product")

	ret, resp := m.afterLocationHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
}

func TestAfterLocationHandlerNoMatchedRule(t *testing.T) {
	m := prepareTestModule(t)
	req := newTestRequest("unknown_product")
	req.InitAiBasicInfo()

	ret, resp := m.afterLocationHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
}

func TestAfterLocationHandlerMatchedRule(t *testing.T) {
	m := prepareTestModule(t)
	req := newTestRequestWithBody("AI_product", `{"prompt":"hello"}`)
	req.InitAiBasicInfo()

	ret, resp := m.afterLocationHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
	if req.HttpRequest.Body == nil {
		t.Error("expected request body replaced by BodyProcessor")
	}
}

func TestReadResponseHandler(t *testing.T) {
	m := prepareTestModule(t)
	req := newTestRequest("AI_product")
	req.InitAiBasicInfo()
	req.SetContext(BodyProcessResponseConfigKey, &BodyProcessConfig{Dec: "line"})

	res := &bfe_http.Response{
		StatusCode: bfe_http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("hello\nworld\n")),
		Header:     make(bfe_http.Header),
	}

	ret := m.readResponseHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if res.Body == nil {
		t.Error("expected response body replaced by BodyProcessor")
	}
}

func TestReadResponseHandlerNoAiBasicInfo(t *testing.T) {
	m := prepareTestModule(t)
	req := newTestRequest("AI_product")
	res := &bfe_http.Response{Body: io.NopCloser(bytes.NewBufferString("hello"))}

	ret := m.readResponseHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
}

func TestRequestFinishHandler(t *testing.T) {
	m := prepareTestModule(t)
	req := newTestRequest("AI_product")
	ai := req.InitAiBasicInfo()
	ai.GetTokenUsage().CompletionTokens = 5
	ai.SetAllowEstimateToken(true)

	ret := m.requestFinishHandler(req, nil)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
}

func TestCalcTokenTime(t *testing.T) {
	aiInfo := &bfe_basic.AiBasicInfo{}
	aiInfo.GetTokenUsage().CompletionTokens = 5
	aiInfo.TokenTimeInfo.TReqEnd = 100
	aiInfo.TokenTimeInfo.TFirstToken = 150
	aiInfo.TokenTimeInfo.TLastToken = 250

	calcTokenTime(aiInfo)
	if aiInfo.TokenTimeInfo.TTFT != 50 {
		t.Errorf("expected TTFT 50, got %d", aiInfo.TokenTimeInfo.TTFT)
	}
	if aiInfo.TokenTimeInfo.TPOT != 25 {
		t.Errorf("expected TPOT 25, got %d", aiInfo.TokenTimeInfo.TPOT)
	}
}

func TestGetState(t *testing.T) {
	m := prepareTestModule(t)
	_, err := m.getState(nil)
	if err != nil {
		t.Fatalf("getState failed: %s", err)
	}
}

func TestGetStateDiff(t *testing.T) {
	m := prepareTestModule(t)
	_, err := m.getStateDiff(nil)
	if err != nil {
		t.Fatalf("getStateDiff failed: %s", err)
	}
}

func TestMonitorHandlers(t *testing.T) {
	m := NewModuleBodyProcess()
	handlers := m.monitorHandlers()
	if handlers == nil {
		t.Fatal("monitorHandlers should not return nil")
	}
	if _, ok := handlers[ModBodyProcess]; !ok {
		t.Error("missing state handler")
	}
	if _, ok := handlers[ModBodyProcess+".diff"]; !ok {
		t.Error("missing diff handler")
	}
}

func TestReloadHandlers(t *testing.T) {
	m := NewModuleBodyProcess()
	handlers := m.reloadHandlers()
	if handlers == nil {
		t.Fatal("reloadHandlers should not return nil")
	}
	if _, ok := handlers[ModBodyProcess]; !ok {
		t.Error("missing reload handler")
	}
}

func prepareTestModule(t *testing.T) *ModuleBodyProcess {
	m := NewModuleBodyProcess()
	m.conf = &ConfModBodyProcess{}
	m.conf.Basic.ProductRulePath = "./testdata/mod_body_process/body_process.data"
	if err := m.loadProductRuleConf(nil); err != nil {
		t.Fatalf("loadProductRuleConf failed: %s", err)
	}
	return m
}

func TestConfLoadSuccess(t *testing.T) {
	cfg, err := ConfLoad("./testdata/mod_body_process/mod_body_process.conf", "./testdata")
	if err != nil {
		t.Fatalf("ConfLoad failed: %s", err)
	}
	if cfg == nil {
		t.Fatal("ConfLoad returned nil config")
	}
}

func TestConfLoadFailure(t *testing.T) {
	_, err := ConfLoad("./testdata/mod_body_process/not_exist.conf", "./testdata")
	if err == nil {
		t.Error("ConfLoad should return error for non-existent file")
	}
}

func TestConfCheckDefaultPath(t *testing.T) {
	cfg := &ConfModBodyProcess{}
	if err := cfg.Check("./testdata"); err != nil {
		t.Fatalf("Check failed: %s", err)
	}
	if !strings.HasSuffix(cfg.Basic.ProductRulePath, "mod_body_process/body_process.data") {
		t.Errorf("unexpected ProductRulePath: %s", cfg.Basic.ProductRulePath)
	}
}
