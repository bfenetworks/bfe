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

package mod_static

import (
	"testing"
)

import (
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func TestStaticFileHandlerNormalFile(t *testing.T) {
	testModuleStatic(t, "GET", "http://www.example.org", nil, func(
		t *testing.T, m *ModuleStatic, ret int, resp *bfe_http.Response) {
		if ret != bfe_module.BfeHandlerResponse {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
		}
		if resp.StatusCode != bfe_http.StatusOK {
			t.Errorf("status code should be %d, not %d", bfe_http.StatusOK, resp.StatusCode)
		}
		if resp.Header.Get("Content-Length") != "53" {
			t.Errorf("content-length should be 53, not %s", resp.Header.Get("Content-Length"))
		}
	})
}

func TestStaticFileHandlerNoMatchedRule(t *testing.T) {
	testModuleStatic(t, "GET", "http://example.org", nil, func(
		t *testing.T, m *ModuleStatic, ret int, resp *bfe_http.Response) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}
	})
}

func TestStaticFileHandlerInvalidMethod(t *testing.T) {
	testModuleStatic(t, "POST", "http://www.example.org", nil, func(
		t *testing.T, m *ModuleStatic, ret int, resp *bfe_http.Response) {
		if ret != bfe_module.BfeHandlerResponse {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
		}
		if resp.StatusCode != bfe_http.StatusMethodNotAllowed {
			t.Errorf("status code should be %d, not %d", bfe_http.StatusMethodNotAllowed, resp.StatusCode)
		}
	})
}

func TestStaticFileHandlerFileEmpty(t *testing.T) {
	testModuleStatic(t, "GET", "http://www.example.org/empty", nil, func(
		t *testing.T, m *ModuleStatic, ret int, resp *bfe_http.Response) {
		if ret != bfe_module.BfeHandlerResponse {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
		}
		if resp.StatusCode != bfe_http.StatusOK {
			t.Errorf("status code should be %d, not %d", bfe_http.StatusNotFound, resp.StatusCode)
		}
		if resp.Header.Get("Content-Length") != "0" {
			t.Errorf("content-length should be 0, not %s", resp.Header.Get("Content-Length"))
		}
	})
}

func TestStaticFileHandlerDir(t *testing.T) {
	testModuleStatic(t, "GET", "http://www.example.org/directory", nil, func(
		t *testing.T, m *ModuleStatic, ret int, resp *bfe_http.Response) {
		if ret != bfe_module.BfeHandlerResponse {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
		}
		if resp.StatusCode != bfe_http.StatusInternalServerError {
			t.Errorf("status code should be %d, not %d",
				bfe_http.StatusInternalServerError, resp.StatusCode)
		}
	})
}

func TestStaticFileHandlerFileNotExist(t *testing.T) {
	testModuleStatic(t, "GET", "http://www.example.org/notfound", nil, func(
		t *testing.T, m *ModuleStatic, ret int, resp *bfe_http.Response) {
		if ret != bfe_module.BfeHandlerResponse {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
		}
		if resp.StatusCode != bfe_http.StatusNotFound {
			t.Errorf("status code should be %d, not %d",
				bfe_http.StatusNotFound, resp.StatusCode)
		}

		fileBrowseNotExist := m.state.FileBrowseNotExist.Get()
		if fileBrowseNotExist != 1 {
			t.Errorf("fileBrowseNotExist should be 1, not %d", fileBrowseNotExist)
		}
	})
}

func TestStaticFileHandlerFileNotExistUseDefault(t *testing.T) {
	testModuleStatic(t, "GET", "http://www.example.org/fallbackdefault", nil, func(
		t *testing.T, m *ModuleStatic, ret int, resp *bfe_http.Response) {
		if ret != bfe_module.BfeHandlerResponse {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
		}
		if resp.StatusCode != bfe_http.StatusOK {
			t.Errorf("status code should be %d, not %d",
				bfe_http.StatusOK, resp.StatusCode)
		}

		fileBrowseNotExist := m.state.FileBrowseNotExist.Get()
		if fileBrowseNotExist != 1 {
			t.Errorf("fileBrowseNotExist should be 1, not %d", fileBrowseNotExist)
		}
		fileBrowseFallbackDefault := m.state.FileBrowseFallbackDefault.Get()
		if fileBrowseFallbackDefault != 1 {
			t.Errorf("fileBrowseFallbackDefault should be 1, not %d", fileBrowseFallbackDefault)
		}
	})
}

func TestStaticFileHandlerCompressed(t *testing.T) {
	header := make(bfe_http.Header)
	header.Set("Accept-Encoding", "gzip")
	testModuleStatic(t, "GET", "http://www.example.org/index.html", header, func(
		t *testing.T, m *ModuleStatic, ret int, resp *bfe_http.Response) {
		if ret != bfe_module.BfeHandlerResponse {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
			return
		}
		if resp.StatusCode != bfe_http.StatusOK {
			t.Errorf("status code should be %d, not %d", bfe_http.StatusOK, resp.StatusCode)
			return
		}
		if resp.Header.Get("Content-Encoding") != "gzip" {
			t.Errorf("Content-Encoding should be \"gzip\", not %s", resp.Header.Get("Content-Encoding"))
		}
		if resp.Header.Get("Content-Length") != "70" {
			t.Errorf("content-length should be 70, not %s", resp.Header.Get("Content-Length"))
		}
	})
}

func TestStaticFileHandlerHeadMethod(t *testing.T) {
	header := make(bfe_http.Header)
	testModuleStatic(t, "HEAD", "http://www.example.org/index.html", header, func(
		t *testing.T, m *ModuleStatic, ret int, resp *bfe_http.Response) {
		if ret != bfe_module.BfeHandlerResponse {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
			return
		}
		if resp.StatusCode != bfe_http.StatusOK {
			t.Errorf("status code should be %d, not %d", bfe_http.StatusOK, resp.StatusCode)
			return
		}
		if resp.Header.Get("Content-Length") != "53" {
			t.Errorf("content-length should be 53, not %s", resp.Header.Get("Content-Length"))
		}
	})
}

func testModuleStatic(t *testing.T, method string, url string, header bfe_http.Header,
	check func(*testing.T, *ModuleStatic, int, *bfe_http.Response)) {
	// prepare module static
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// prepare request
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = "unittest"
	req.HttpRequest, err = bfe_http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("bfe_http.NewRequest error: %v", err)
	}
	req.HttpRequest.Header = header

	// process request and check
	ret, resp := m.staticFileHandler(req)
	check(t, m, ret, resp)
	if resp != nil {
		resp.Body.Close()
	}

	fileCurrentOpened := m.state.FileCurrentOpened.Get()
	if fileCurrentOpened != 0 {
		t.Errorf("fileCurrentOpened should be 0, not %d", fileCurrentOpened)
	}
}
