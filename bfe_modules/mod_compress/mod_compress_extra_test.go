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

package mod_compress

import (
	"bytes"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

import (
	"github.com/andybalholm/brotli"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func prepareRequestWithEncoding(enc string) *bfe_basic.Request {
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest = new(bfe_http.Request)
	req.HttpRequest.Header = make(bfe_http.Header)
	if enc != "" {
		req.HttpRequest.Header.Set("Accept-Encoding", enc)
	}
	req.HttpRequest.URL, _ = url.Parse("http://www.example.org/")
	req.Route.Product = "unittest"
	req.Context = make(map[interface{}]interface{})
	return req
}

func prepareResponseWithBody(body []byte) *bfe_http.Response {
	res := new(bfe_http.Response)
	res.StatusCode = 200
	res.Header = make(bfe_http.Header)
	res.Body = ioutil.NopCloser(bytes.NewReader(body))
	return res
}

func writeTempRuleData(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "compress_rule.data")
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	return path
}

func loadRuleData(t *testing.T, m *ModuleCompress, path string) {
	t.Helper()
	values := url.Values{}
	values.Set("path", path)
	if err := m.loadProductRuleConf(values); err != nil {
		t.Fatalf("loadProductRuleConf() error: %v", err)
	}
}

func TestModuleCompressName(t *testing.T) {
	m := NewModuleCompress()
	if m.Name() != ModCompress {
		t.Errorf("Name() should be %s, not %s", ModCompress, m.Name())
	}
}

func TestCheckSupportCompress(t *testing.T) {
	cases := []struct {
		enc      string
		expected bool
	}{
		{"gzip", true},
		{"br", true},
		{"gzip, deflate", true},
		{"br;q=1.0", false},
		{"deflate", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.enc, func(t *testing.T) {
			if got := checkSupportCompress(tc.enc); got != tc.expected {
				t.Errorf("checkSupportCompress(%q) = %v, want %v", tc.enc, got, tc.expected)
			}
		})
	}
}

func TestCheckSupportBrotliCompress(t *testing.T) {
	if !checkSupportBrotliCompress("br") {
		t.Errorf("checkSupportBrotliCompress(br) should be true")
	}
	if checkSupportBrotliCompress("gzip") {
		t.Errorf("checkSupportBrotliCompress(gzip) should be false")
	}
}

func TestGetCompressRuleNoProduct(t *testing.T) {
	openDebug = true
	defer func() { openDebug = false }()

	m := NewModuleCompress()
	req := prepareRequestWithEncoding(EncodeGzip)

	rule, err := m.getCompressRule(req)
	if err == nil {
		t.Fatalf("expected error")
	}
	if rule != nil {
		t.Fatalf("rule should be nil")
	}
}

func TestGetCompressRuleNoMatchedCond(t *testing.T) {
	openDebug = true
	defer func() { openDebug = false }()

	m := NewModuleCompress()
	cond, err := condition.Build("req_header_value_in(\"X-Never\", \"yes\", true)")
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	rules := compressRuleList{{Cond: cond, Action: Action{Cmd: ActionGzip, Quality: 5, FlushSize: 128}}}
	m.ruleTable.Update(productRuleConf{Version: "v1", Config: ProductRules{"unittest": &rules}})

	req := prepareRequestWithEncoding(EncodeGzip)
	rule, err := m.getCompressRule(req)
	if err == nil {
		t.Fatalf("expected error")
	}
	if rule != nil {
		t.Fatalf("rule should be nil")
	}
}

func TestCompressHandlerNoAcceptEncoding(t *testing.T) {
	m := prepareModule()
	data := []byte("hello world")
	req := prepareRequestWithEncoding("")
	res := prepareResponseWithBody(data)

	ret := m.compressHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("ret should be BfeHandlerGoOn, got %d", ret)
	}
	if res.Header.Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding should be empty")
	}
}

func TestCompressHandlerAlreadyEncoded(t *testing.T) {
	m := prepareModule()
	data := []byte("hello world")
	req := prepareRequestWithEncoding(EncodeGzip)
	res := prepareResponseWithBody(data)
	res.Header.Set("Content-Encoding", "br")

	ret := m.compressHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("ret should be BfeHandlerGoOn, got %d", ret)
	}
	if res.Header.Get("Content-Encoding") != "br" {
		t.Fatalf("Content-Encoding should remain br")
	}
}

func TestCompressHandlerContentEncodingIdentity(t *testing.T) {
	m := prepareModule()
	data := []byte("hello world hello world hello world")
	req := prepareRequestWithEncoding(EncodeGzip)
	res := prepareResponseWithBody(data)
	res.Header.Set("Content-Encoding", EncodeIdentity)

	ret := m.compressHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("ret should be BfeHandlerGoOn, got %d", ret)
	}
	if res.Header.Get("Content-Encoding") != EncodeGzip {
		t.Fatalf("Content-Encoding should be gzip, got %s", res.Header.Get("Content-Encoding"))
	}
}

func TestCompressHandlerBrotli(t *testing.T) {
	m := prepareModule()
	path := writeTempRuleData(t, `{
    "Config": {
        "unittest": [
            {
                "Cond": "default_t()",
                "Action": {
                    "Cmd": "BROTLI",
                    "Quality": 4,
                    "FlushSize": 512
                }
            }
        ]
    },
    "Version": "v1"
}`)
	loadRuleData(t, m, path)

	data := prepareTestData(16 * 1024)
	req := prepareRequestWithEncoding(EncodeBrotli)
	res := prepareResponseWithBody(data)

	ret := m.compressHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("ret should be BfeHandlerGoOn, got %d", ret)
	}
	if res.Header.Get("Content-Encoding") != EncodeBrotli {
		t.Fatalf("Content-Encoding should be br, got %s", res.Header.Get("Content-Encoding"))
	}

	compressed, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	res.Body.Close()

	got, err := ioutil.ReadAll(brotli.NewReader(bytes.NewReader(compressed)))
	if err != nil {
		t.Fatalf("decompress error: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("decompressed data does not match original")
	}
}

func TestCompressHandlerNoSupportGzipForBrAction(t *testing.T) {
	m := prepareModule()
	path := writeTempRuleData(t, `{
    "Config": {
        "unittest": [
            {
                "Cond": "default_t()",
                "Action": {
                    "Cmd": "BROTLI",
                    "Quality": 4,
                    "FlushSize": 512
                }
            }
        ]
    },
    "Version": "v1"
}`)
	loadRuleData(t, m, path)

	data := []byte("hello world")
	req := prepareRequestWithEncoding(EncodeGzip)
	res := prepareResponseWithBody(data)

	ret := m.compressHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("ret should be BfeHandlerGoOn, got %d", ret)
	}
	if res.Header.Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding should be empty")
	}
}

func TestCompressHandlerNoSupportBrForGzipAction(t *testing.T) {
	m := prepareModule()
	data := []byte("hello world")
	req := prepareRequestWithEncoding(EncodeBrotli)
	res := prepareResponseWithBody(data)

	ret := m.compressHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("ret should be BfeHandlerGoOn, got %d", ret)
	}
	if res.Header.Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding should be empty")
	}
}

func TestCompressHandlerNoMatchedRule(t *testing.T) {
	m := prepareModule()
	path := writeTempRuleData(t, `{
    "Config": {
        "unittest": [
            {
                "Cond": "req_header_value_in(\"X-Never\", \"yes\", true)",
                "Action": {
                    "Cmd": "GZIP",
                    "Quality": 5,
                    "FlushSize": 512
                }
            }
        ]
    },
    "Version": "v1"
}`)
	loadRuleData(t, m, path)

	data := []byte("hello world")
	req := prepareRequestWithEncoding(EncodeGzip)
	res := prepareResponseWithBody(data)

	ret := m.compressHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("ret should be BfeHandlerGoOn, got %d", ret)
	}
	if res.Header.Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding should be empty")
	}
}

func TestCompressHandlerInvalidAction(t *testing.T) {
	m := prepareModule()
	cond, err := condition.Build("default_t()")
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	rules := compressRuleList{{Cond: cond, Action: Action{Cmd: "UNKNOWN", Quality: 5, FlushSize: 512}}}
	m.ruleTable.Update(productRuleConf{Version: "v1", Config: ProductRules{"unittest": &rules}})

	data := []byte("hello world")
	req := prepareRequestWithEncoding(EncodeGzip)
	res := prepareResponseWithBody(data)

	ret := m.compressHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("ret should be BfeHandlerGoOn, got %d", ret)
	}
	if res.Header.Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding should be empty for invalid action")
	}
}

func TestCompressHandlerNewGzipFilterError(t *testing.T) {
	m := prepareModule()
	cond, err := condition.Build("default_t()")
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	// Invalid quality will cause NewGzipFilter to fail.
	rules := compressRuleList{{Cond: cond, Action: Action{Cmd: ActionGzip, Quality: 100, FlushSize: 512}}}
	m.ruleTable.Update(productRuleConf{Version: "v1", Config: ProductRules{"unittest": &rules}})

	data := []byte("hello world")
	req := prepareRequestWithEncoding(EncodeGzip)
	res := prepareResponseWithBody(data)

	ret := m.compressHandler(req, res)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("ret should be BfeHandlerGoOn, got %d", ret)
	}
	if res.Header.Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding should be empty after filter error")
	}
}

func TestLoadProductRuleConf(t *testing.T) {
	m := prepareModule()

	// default path
	if err := m.loadProductRuleConf(nil); err != nil {
		t.Fatalf("loadProductRuleConf(nil) error: %v", err)
	}

	// explicit valid path
	values := url.Values{}
	values.Set("path", "./testdata/mod_compress/compress_rule.data")
	if err := m.loadProductRuleConf(values); err != nil {
		t.Fatalf("loadProductRuleConf(valid path) error: %v", err)
	}

	// invalid path
	values.Set("path", "./testdata/mod_compress/not_exist.data")
	if err := m.loadProductRuleConf(values); err == nil {
		t.Fatalf("loadProductRuleConf(invalid path) should return error")
	}
}

func TestModuleCompressGetState(t *testing.T) {
	m := prepareModule()

	state, err := m.getState(nil)
	if err != nil {
		t.Fatalf("getState() error: %v", err)
	}
	if len(state) == 0 {
		t.Fatalf("getState() should return non-empty state")
	}

	diff, err := m.getStateDiff(nil)
	if err != nil {
		t.Fatalf("getStateDiff() error: %v", err)
	}
	if len(diff) == 0 {
		t.Fatalf("getStateDiff() should return non-empty diff")
	}
}

func TestModuleCompressInitConfNotExist(t *testing.T) {
	m := NewModuleCompress()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, "./testdata_not_exist")
	if err == nil {
		t.Fatalf("Init() should return error when config file not exist")
	}
}

func TestModuleCompressInitRegisterHandlersError(t *testing.T) {
	m := NewModuleCompress()
	cb := bfe_module.NewBfeCallbacks()

	err := m.Init(cb, nil, "./testdata")
	if err == nil {
		t.Fatalf("Init() should return error when WebHandlers is nil")
	}
}

func TestModuleCompressInitMonitorRegisterError(t *testing.T) {
	m := NewModuleCompress()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	// Pre-register the monitor handler so Init fails when it tries to register again.
	wh.RegisterHandler(web_monitor.WebHandleMonitor, ModCompress, func(map[string][]string) ([]byte, error) {
		return nil, nil
	})

	err := m.Init(cb, wh, "./testdata")
	if err == nil {
		t.Fatalf("Init() should return error when monitor handler registration fails")
	}
}

func TestModuleCompressInitReloadRegisterError(t *testing.T) {
	m := NewModuleCompress()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	// Pre-register the reload handler so Init fails when it tries to register again.
	wh.RegisterHandler(web_monitor.WebHandleReload, ModCompress, func(url.Values) error {
		return nil
	})

	err := m.Init(cb, wh, "./testdata")
	if err == nil {
		t.Fatalf("Init() should return error when reload handler registration fails")
	}
}

func TestModuleCompressInitDataLoadError(t *testing.T) {
	confRoot := "./testdata_init_fail"
	if err := os.MkdirAll(filepath.Join(confRoot, "mod_compress"), 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	defer os.RemoveAll(confRoot)

	confContent := "[basic]\nProductRulePath = mod_compress/missing.data\n"
	confPath := filepath.Join(confRoot, "mod_compress", "mod_compress.conf")
	if err := ioutil.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := NewModuleCompress()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err := m.Init(cb, wh, confRoot)
	if err == nil {
		t.Fatalf("Init() should return error when data file load fails")
	}
}
