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

package mod_static

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

type staticConfFixture struct {
	confRoot string
	static   string
}

func prepareStaticConf(t *testing.T, enableCompress bool) *staticConfFixture {
	t.Helper()

	dir, err := os.MkdirTemp("./testdata", "mod_static_module_")
	if err != nil {
		t.Fatalf("MkdirTemp() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	modDir := filepath.Join(dir, "mod_static")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	staticDir := filepath.Join(dir, "static")
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	// plain files
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html></html>\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "unknown.xyz"), []byte("unknown"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "empty"), []byte{}, 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "zero.html"), []byte("zero"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	// directory used for fallback test
	if err := os.MkdirAll(filepath.Join(staticDir, "subdir"), 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	// gzip compressed variant
	var gzBuf bytes.Buffer
	gzWriter := gzip.NewWriter(&gzBuf)
	if _, err := gzWriter.Write([]byte("<html></html>\n")); err != nil {
		t.Fatalf("gzip Write() error: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("gzip Close() error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "index.html.gz"), gzBuf.Bytes(), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	// brotli variant (content is not actually brotli-compressed, but module only checks file existence)
	if err := os.WriteFile(filepath.Join(staticDir, "index.html.br"), []byte("brotli bytes"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	relStatic, err := filepath.Rel(".", staticDir)
	if err != nil {
		t.Fatalf("filepath.Rel() error: %v", err)
	}
	relStatic = filepath.ToSlash(relStatic)

	ruleData := fmt.Sprintf(`{
    "Config": {
        "unittest": [
            {
                "Cond": "req_host_in(\"www.example.org\") && req_path_prefix_in(\"/empty\", false)",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [%q, ""]
                }
            },
            {
                "Cond": "req_host_in(\"www.example.org\") && req_path_prefix_in(\"/notfound\", false)",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [%q, ""]
                }
            },
            {
                "Cond": "req_host_in(\"www.example.org\") && req_path_prefix_in(\"/subdir\", false)",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [%q, "index.html"]
                }
            },
            {
                "Cond": "req_host_in(\"www.example.org\") && req_path_prefix_in(\"/zero\", false)",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [%q, ""]
                }
            },
            {
                "Cond": "req_host_in(\"www.example.org\") && req_path_prefix_in(\"/unknown\", false)",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [%q, ""]
                }
            },
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [%q, "index.html"]
                }
            }
        ]
    },
    "Version": "unittest"
}`, relStatic, relStatic, relStatic, relStatic, relStatic, relStatic)

	if err := os.WriteFile(filepath.Join(modDir, "static_rule.data"), []byte(ruleData), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	mimeData := `{
    "Version": "unittest",
    "Config": {
        ".html": "text/html"
    }
}`
	if err := os.WriteFile(filepath.Join(modDir, "mime_type.data"), []byte(mimeData), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	confContent := fmt.Sprintf(`[basic]
DataPath = ./mod_static/static_rule.data
MimeTypePath = ./mod_static/mime_type.data
EnableCompress = %t

[log]
OpenDebug = true
`, enableCompress)
	if err := os.WriteFile(filepath.Join(modDir, "mod_static.conf"), []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return &staticConfFixture{
		confRoot: filepath.ToSlash(dir),
		static:   relStatic,
	}
}

func buildRequest(t *testing.T, method, rawURL string, header bfe_http.Header, product string) *bfe_basic.Request {
	t.Helper()
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = product
	var err error
	req.HttpRequest, err = bfe_http.NewRequest(method, rawURL, nil)
	if err != nil {
		t.Fatalf("bfe_http.NewRequest() error: %v", err)
	}
	req.HttpRequest.Header = header
	return req
}

func TestNewModuleStaticAndName(t *testing.T) {
	m := NewModuleStatic()
	if m == nil {
		t.Fatal("NewModuleStatic() returns nil")
	}
	if m.Name() != ModStatic {
		t.Errorf("Name() should be %s, got %s", ModStatic, m.Name())
	}
	if m.ruleTable == nil {
		t.Errorf("ruleTable should not be nil")
	}
	if m.mimeTypeTable == nil {
		t.Errorf("mimeTypeTable should not be nil")
	}
}

func TestModuleStaticInitSuccess(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if m.conf == nil {
		t.Errorf("conf should not be nil after Init")
	}
}

func TestModuleStaticInitConfNotExist(t *testing.T) {
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata_not_exist"); err == nil {
		t.Errorf("Init() should return error when config file does not exist")
	}
}

func TestModuleStaticInitDataLoadFail(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	// remove rule data to make loadConfData fail
	if err := os.Remove(filepath.Join(fixture.confRoot, "mod_static", "static_rule.data")); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err == nil {
		t.Errorf("Init() should return error when data file load fails")
	}
}

func TestModuleStaticInitMimeTypeLoadFail(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	// corrupt mime type file
	if err := os.WriteFile(filepath.Join(fixture.confRoot, "mod_static", "mime_type.data"), []byte("invalid"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err == nil {
		t.Errorf("Init() should return error when mime type file load fails")
	}
}

func TestModuleStaticLoadConfData(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if err := m.loadConfData(nil); err != nil {
		t.Errorf("loadConfData(nil) error: %v", err)
	}

	values := url.Values{}
	values.Set("path", filepath.Join(fixture.confRoot, "mod_static", "static_rule.data"))
	if err := m.loadConfData(values); err != nil {
		t.Errorf("loadConfData(valid path) error: %v", err)
	}

	values.Set("path", filepath.Join(fixture.confRoot, "mod_static", "not_exist.data"))
	if err := m.loadConfData(values); err == nil {
		t.Errorf("loadConfData(invalid path) should return error")
	}
}

func TestModuleStaticLoadMimeType(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if err := m.loadMimeType(nil); err != nil {
		t.Errorf("loadMimeType(nil) error: %v", err)
	}

	values := url.Values{}
	values.Set("path", filepath.Join(fixture.confRoot, "mod_static", "mime_type.data"))
	if err := m.loadMimeType(values); err != nil {
		t.Errorf("loadMimeType(valid path) error: %v", err)
	}

	values.Set("path", filepath.Join(fixture.confRoot, "mod_static", "not_exist.data"))
	if err := m.loadMimeType(values); err == nil {
		t.Errorf("loadMimeType(invalid path) should return error")
	}
}

func TestModuleStaticGetStateAndDiff(t *testing.T) {
	m := NewModuleStatic()
	m.state.FileBrowseCount.Inc(1)

	b, err := m.getState(nil)
	if err != nil {
		t.Errorf("getState() error: %v", err)
	}
	if len(b) == 0 {
		t.Errorf("getState() returns empty response")
	}

	b, err = m.getStateDiff(nil)
	if err != nil {
		t.Errorf("getStateDiff() error: %v", err)
	}
	if len(b) == 0 {
		t.Errorf("getStateDiff() returns empty response")
	}
}

func TestModuleStaticHandlers(t *testing.T) {
	m := NewModuleStatic()

	monitors := m.monitorHandlers()
	if _, ok := monitors[ModStatic]; !ok {
		t.Errorf("monitorHandlers() should contain key %s", ModStatic)
	}
	if _, ok := monitors[ModStatic+".diff"]; !ok {
		t.Errorf("monitorHandlers() should contain key %s.diff", ModStatic)
	}

	reloads := m.reloadHandlers()
	if _, ok := reloads[ModStatic]; !ok {
		t.Errorf("reloadHandlers() should contain key %s", ModStatic)
	}
	if _, ok := reloads[ModStatic+".mime_type"]; !ok {
		t.Errorf("reloadHandlers() should contain key %s.mime_type", ModStatic)
	}
}

func TestStaticFileHandlerNoMatchedProduct(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := buildRequest(t, "GET", "http://www.example.org/index.html", nil, "not_exist")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, got %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Errorf("resp should be nil when product not matched")
		resp.Body.Close()
	}
}

func TestStaticFileHandlerNoMatchedCond(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := buildRequest(t, "GET", "http://example.org/index.html", nil, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, got %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Errorf("resp should be nil when condition not matched")
		resp.Body.Close()
	}
}

func TestStaticFileHandlerDefaultAction(t *testing.T) {
	m := NewModuleStatic()
	m.conf = &ConfModStatic{Basic: struct {
		DataPath       string
		MimeTypePath   string
		EnableCompress bool
	}{EnableCompress: false}}

	cond, err := condition.Build("req_host_in(\"www.example.org\")")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}
	rules := RuleList{
		StaticRule{
			Cond:   cond,
			Action: Action{Cmd: "UNKNOWN"},
		},
	}
	m.ruleTable.Update(StaticConf{
		Version: "v1",
		Config: ProductRules{
			"unittest": &rules,
		},
	})

	req := buildRequest(t, "GET", "http://www.example.org/index.html", nil, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, got %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Errorf("resp should be nil for unknown action")
		resp.Body.Close()
	}
}

func TestStaticFileHandlerNormalFileExtra(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := buildRequest(t, "GET", "http://www.example.org/index.html", nil, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.StatusCode != bfe_http.StatusOK {
		t.Errorf("status code should be %d, got %d", bfe_http.StatusOK, resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "text/html" {
		t.Errorf("Content-Type should be text/html, got %s", resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("Content-Length") != "14" {
		t.Errorf("Content-Length should be 14, got %s", resp.Header.Get("Content-Length"))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	if string(body) != "<html></html>\n" {
		t.Errorf("body mismatch, got %q", string(body))
	}
	resp.Body.Close()

	if m.state.FileCurrentOpened.Get() != 0 {
		t.Errorf("FileCurrentOpened should be 0, got %d", m.state.FileCurrentOpened.Get())
	}
}

func TestStaticFileHandlerEmptyFile(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := buildRequest(t, "GET", "http://www.example.org/empty", nil, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.StatusCode != bfe_http.StatusOK {
		t.Errorf("status code should be %d, got %d", bfe_http.StatusOK, resp.StatusCode)
	}
	if resp.Header.Get("Content-Length") != "0" {
		t.Errorf("Content-Length should be 0, got %s", resp.Header.Get("Content-Length"))
	}
	resp.Body.Close()
}

func TestStaticFileHandlerNotFound(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := buildRequest(t, "GET", "http://www.example.org/notfound/missing.txt", nil, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.StatusCode != bfe_http.StatusNotFound {
		t.Errorf("status code should be %d, got %d", bfe_http.StatusNotFound, resp.StatusCode)
	}
	if m.state.FileBrowseNotExist.Get() != 1 {
		t.Errorf("FileBrowseNotExist should be 1, got %d", m.state.FileBrowseNotExist.Get())
	}
	resp.Body.Close()
}

func TestStaticFileHandlerDirectoryFallback(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := buildRequest(t, "GET", "http://www.example.org/subdir", nil, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.StatusCode != bfe_http.StatusOK {
		t.Errorf("status code should be %d, got %d", bfe_http.StatusOK, resp.StatusCode)
	}
	if resp.Header.Get("Content-Length") != "14" {
		t.Errorf("Content-Length should be 14, got %s", resp.Header.Get("Content-Length"))
	}
	if m.state.FileBrowseFallbackDefault.Get() != 1 {
		t.Errorf("FileBrowseFallbackDefault should be 1, got %d", m.state.FileBrowseFallbackDefault.Get())
	}
	resp.Body.Close()
}

func TestStaticFileHandlerUnknownContentType(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := buildRequest(t, "GET", "http://www.example.org/unknown.xyz", nil, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.StatusCode != bfe_http.StatusOK {
		t.Errorf("status code should be %d, got %d", bfe_http.StatusOK, resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "" {
		t.Errorf("Content-Type should be empty for unknown extension, got %s", resp.Header.Get("Content-Type"))
	}
	if m.state.FileBrowseContentTypeError.Get() != 1 {
		t.Errorf("FileBrowseContentTypeError should be 1, got %d", m.state.FileBrowseContentTypeError.Get())
	}
	resp.Body.Close()
}

func TestStaticFileHandlerLastModifiedZero(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	zeroPath := filepath.Join(fixture.static, "zero.html")
	if err := os.Chtimes(zeroPath, unixEpochTime, unixEpochTime); err != nil {
		t.Fatalf("Chtimes() error: %v", err)
	}

	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := buildRequest(t, "GET", "http://www.example.org/zero.html", nil, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.Header.Get("Last-Modified") != "" {
		t.Errorf("Last-Modified should be empty for zero mod time, got %s", resp.Header.Get("Last-Modified"))
	}
	resp.Body.Close()
}

func TestStaticFileHandlerGzipCompressed(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	header := make(bfe_http.Header)
	header.Set("Accept-Encoding", "gzip")
	req := buildRequest(t, "GET", "http://www.example.org/index.html", header, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.StatusCode != bfe_http.StatusOK {
		t.Errorf("status code should be %d, got %d", bfe_http.StatusOK, resp.StatusCode)
	}
	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("Content-Encoding should be gzip, got %s", resp.Header.Get("Content-Encoding"))
	}
	if resp.Header.Get("Content-Length") == "" {
		t.Errorf("Content-Length should not be empty")
	}
	resp.Body.Close()
}

func TestStaticFileHandlerBrotliCompressed(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	header := make(bfe_http.Header)
	header.Set("Accept-Encoding", "br")
	req := buildRequest(t, "GET", "http://www.example.org/index.html", header, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.Header.Get("Content-Encoding") != "br" {
		t.Errorf("Content-Encoding should be br, got %s", resp.Header.Get("Content-Encoding"))
	}
	resp.Body.Close()
}

func TestStaticFileHandlerEnableCompressFalse(t *testing.T) {
	fixture := prepareStaticConf(t, false)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	header := make(bfe_http.Header)
	header.Set("Accept-Encoding", "gzip")
	req := buildRequest(t, "GET", "http://www.example.org/index.html", header, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.Header.Get("Content-Encoding") != "" {
		t.Errorf("Content-Encoding should be empty when compression disabled, got %s", resp.Header.Get("Content-Encoding"))
	}
	if resp.Header.Get("Content-Length") != "14" {
		t.Errorf("Content-Length should be 14, got %s", resp.Header.Get("Content-Length"))
	}
	resp.Body.Close()
}

func TestStaticFileHandlerHeadMethodExtra(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := buildRequest(t, "HEAD", "http://www.example.org/index.html", nil, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.StatusCode != bfe_http.StatusOK {
		t.Errorf("status code should be %d, got %d", bfe_http.StatusOK, resp.StatusCode)
	}
	if resp.Header.Get("Content-Length") != "14" {
		t.Errorf("Content-Length should be 14, got %s", resp.Header.Get("Content-Length"))
	}
	if resp.Body != nil {
		resp.Body.Close()
	}

	if m.state.FileCurrentOpened.Get() != 0 {
		t.Errorf("FileCurrentOpened should be 0 after HEAD, got %d", m.state.FileCurrentOpened.Get())
	}
}

func TestStaticFileHandlerInvalidMethodExtra(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := buildRequest(t, "POST", "http://www.example.org/index.html", nil, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.StatusCode != bfe_http.StatusMethodNotAllowed {
		t.Errorf("status code should be %d, got %d", bfe_http.StatusMethodNotAllowed, resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCreateRespFromStaticFileContentLength(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := buildRequest(t, "GET", "http://www.example.org/index.html", nil, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	expectedLength := fmt.Sprintf("%d", len(body))
	if resp.Header.Get("Content-Length") != expectedLength {
		t.Errorf("Content-Length should be %s, got %s", expectedLength, resp.Header.Get("Content-Length"))
	}
	resp.Body.Close()
}

func TestCreateRespFromStaticFileLastModified(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := buildRequest(t, "GET", "http://www.example.org/index.html", nil, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.Header.Get("Last-Modified") == "" {
		t.Errorf("Last-Modified should be set for non-zero mod time")
	}
	resp.Body.Close()
}

func TestLoadConfDataAbsolutePathInQuery(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	values := url.Values{}
	values.Set("path", filepath.ToSlash(filepath.Join(fixture.confRoot, "mod_static", "static_rule.data")))
	if err := m.loadConfData(values); err != nil {
		t.Errorf("loadConfData(absolute path) error: %v", err)
	}
}

func TestOpenStaticFileFallbackOnNotExist(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req, err := bfe_http.NewRequest("GET", "http://www.example.org/missing.txt", nil)
	if err != nil {
		t.Fatalf("bfe_http.NewRequest() error: %v", err)
	}

	file, err := m.openStaticFile(req, fixture.static, "index.html")
	if err != nil {
		t.Fatalf("openStaticFile() error: %v", err)
	}
	if file == nil {
		t.Fatal("openStaticFile() returns nil file")
	}
	file.Close()

	if m.state.FileBrowseFallbackDefault.Get() != 1 {
		t.Errorf("FileBrowseFallbackDefault should be 1, got %d", m.state.FileBrowseFallbackDefault.Get())
	}
}

func TestOpenStaticFileNoFallbackWhenDefaultEmpty(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req, err := bfe_http.NewRequest("GET", "http://www.example.org/missing.txt", nil)
	if err != nil {
		t.Fatalf("bfe_http.NewRequest() error: %v", err)
	}

	_, err = m.openStaticFile(req, fixture.static, "")
	if err == nil {
		t.Errorf("openStaticFile() should return error when file not exist and default empty")
	}
}

func TestStaticFileHandlerMetricsCounters(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := buildRequest(t, "GET", "http://www.example.org/index.html", nil, "unittest")
	_, resp := m.staticFileHandler(req)
	if resp != nil {
		resp.Body.Close()
	}

	if m.state.FileBrowseCount.Get() != 1 {
		t.Errorf("FileBrowseCount should be 1, got %d", m.state.FileBrowseCount.Get())
	}
	if m.state.FileBrowseSize.Get() != 14 {
		t.Errorf("FileBrowseSize should be 14, got %d", m.state.FileBrowseSize.Get())
	}
}

func TestProcessContentTypeUsesMimeTypeTable(t *testing.T) {
	m := NewModuleStatic()
	m.mimeTypeTable.Update(MimeTypeConf{
		Version: "v1",
		Config: MimeType{
			".xyz": "application/xyz",
		},
	})

	resp := &bfe_http.Response{Header: make(bfe_http.Header)}
	file := &staticFile{extension: ".xyz"}
	m.processContentType(resp, file)
	if resp.Header.Get("Content-Type") != "application/xyz" {
		t.Errorf("Content-Type should be application/xyz, got %s", resp.Header.Get("Content-Type"))
	}
}

func TestProcessContentEncodingDefault(t *testing.T) {
	m := NewModuleStatic()
	resp := &bfe_http.Response{Header: make(bfe_http.Header)}
	file := &staticFile{extension: ".txt", encoding: "identity"}
	m.processContentEncoding(resp, file)
	if resp.Header.Get("Content-Encoding") != "" {
		t.Errorf("Content-Encoding should be empty for unknown encoding")
	}
}

func TestLoadMimeTypeAbsolutePathInQuery(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	values := url.Values{}
	values.Set("path", filepath.ToSlash(filepath.Join(fixture.confRoot, "mod_static", "mime_type.data")))
	if err := m.loadMimeType(values); err != nil {
		t.Errorf("loadMimeType(absolute path) error: %v", err)
	}
}

func TestStaticFileHandlerTimeFormat(t *testing.T) {
	fixture := prepareStaticConf(t, true)
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, fixture.confRoot); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// set a known modification time in the future
	indexPath := filepath.Join(fixture.static, "index.html")
	future := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(indexPath, future, future); err != nil {
		t.Fatalf("Chtimes() error: %v", err)
	}

	req := buildRequest(t, "GET", "http://www.example.org/index.html", nil, "unittest")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret should be %d, got %d", bfe_module.BfeHandlerResponse, ret)
	}
	lm := resp.Header.Get("Last-Modified")
	if lm == "" {
		t.Errorf("Last-Modified should be set")
	}
	if !strings.Contains(lm, "2030") {
		t.Errorf("Last-Modified should contain 2030, got %s", lm)
	}
	resp.Body.Close()
}
