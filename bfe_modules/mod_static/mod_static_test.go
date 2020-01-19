// Copyright (c) 2019 Baidu, Inc.
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
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_http"
	"github.com/baidu/bfe/bfe_module"
)

func TestStaticFileHandler(t *testing.T) {
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Errorf("Init() error: %v", err)
		return
	}
	m.enableCompress = false

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = "unittest"

	// Case 1.
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org", nil)
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.StatusCode != bfe_http.StatusOK {
		t.Errorf("status code should be %d, not %d", bfe_http.StatusOK, resp.StatusCode)
	}
	resp.Body.Close()

	// Case 2.
	req.HttpRequest.Host = "example.org"
	ret, _ = m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}

	// Case 3.
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org/empty", nil)
	ret, resp = m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.StatusCode != bfe_http.StatusNotFound {
		t.Errorf("status code should be %d, not %d", bfe_http.StatusNotFound, resp.StatusCode)
	}
	resp.Body.Close()

	// Case 4.
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org/directory", nil)
	ret, resp = m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.StatusCode != bfe_http.StatusInternalServerError {
		t.Errorf("status code should be %d, not %d",
			bfe_http.StatusInternalServerError, resp.StatusCode)
	}
	resp.Body.Close()

	// Case 5.
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org/hello", nil)
	ret, resp = m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
	}
	if resp.StatusCode != bfe_http.StatusOK {
		t.Errorf("status code should be %d, not %d",
			bfe_http.StatusInternalServerError, resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type should be \"text/plain; charset=utf-8\", not %s",
			resp.Header.Get("Content-Type"))
	}
	resp.Body.Close()

	// Check state.
	fileCurrentOpened := m.state.FileCurrentOpened.Get()
	if fileCurrentOpened != 0 {
		t.Errorf("fileCurrentOpened should be 0, not %d", fileCurrentOpened)
	}
	fileBrowseNotExist := m.state.FileBrowseNotExist.Get()
	if fileBrowseNotExist != 1 {
		t.Errorf("fileBrowseNotExist should be 1, not %d", fileBrowseNotExist)
	}
}

func TestStaticFileHandler_Compressed(t *testing.T) {
	m := NewModuleStatic()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Errorf("Init() error: %v", err)
		return
	}

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = "unittest"
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org/index.html", nil)
	req.HttpRequest.Header.Set("Accept-Encoding", "gzip")
	ret, resp := m.staticFileHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
		return
	}
	if resp.StatusCode != bfe_http.StatusOK {
		t.Errorf("status code should be %d, not %d", bfe_http.StatusOK, resp.StatusCode)
		return
	}
	if resp.Header.Get("Content-Type") != "application/gzip" {
		t.Errorf("Content-Type should be \"application/gzip\", not %s",
			resp.Header.Get("Content-Type"))
	}
}
