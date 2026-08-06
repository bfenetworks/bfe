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

package mod_doh

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func prepareConfRoot(t *testing.T, confContent string) string {
	t.Helper()

	tmpDir, err := ioutil.TempDir("", "mod_doh_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	modDir := filepath.Join(tmpDir, "mod_doh")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	confPath := filepath.Join(modDir, "mod_doh.conf")
	if err := ioutil.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return tmpDir
}

type errorDnsFetcher struct{}

func (f *errorDnsFetcher) Fetch(req *bfe_basic.Request) (*bfe_http.Response, error) {
	return nil, errors.New("mock fetch error")
}

func TestNewModuleDohAndName(t *testing.T) {
	m := NewModuleDoh()
	if m == nil {
		t.Fatal("NewModuleDoh() should not return nil")
	}
	if got, want := m.Name(), ModDoh; got != want {
		t.Fatalf("Name() = %s, want %s", got, want)
	}
}

func TestModuleDohInit(t *testing.T) {
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	// success
	m := NewModuleDoh()
	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// non-existent conf root
	m = NewModuleDoh()
	tmpDir, err := ioutil.TempDir("", "mod_doh_empty")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	if err := m.Init(cb, wh, tmpDir); err == nil {
		t.Fatal("Init() should fail with non-existent conf root")
	}

	// invalid condition
	m = NewModuleDoh()
	cr := prepareConfRoot(t, `[Basic]
Cond = "bad_cond()"
[Dns]
Address = "127.0.0.1:53"
Timeout = 1000
`)
	if err := m.Init(cb, wh, cr); err == nil {
		t.Fatal("Init() should fail with invalid condition")
	}

	// invalid RetryMax
	m = NewModuleDoh()
	cr = prepareConfRoot(t, `[Basic]
Cond = "default_t()"
[Dns]
Address = "127.0.0.1:53"
RetryMax = -1
Timeout = 1000
`)
	if err := m.Init(cb, wh, cr); err == nil {
		t.Fatal("Init() should fail with invalid RetryMax")
	}
}

func TestDohHandlerNotSecure(t *testing.T) {
	m := NewModuleDoh()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	m.dnsFetcher = new(TestDnsFetcher)

	req := buildDohRequest("GET", t)
	req.Session = &bfe_basic.Session{IsSecure: false}

	ret, resp := m.dohHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret = %d, want %d", ret, bfe_module.BfeHandlerResponse)
	}
	if resp == nil || resp.StatusCode != bfe_http.StatusForbidden {
		t.Fatalf("resp = %v, want 403 Forbidden", resp)
	}
}

func TestDohHandlerCondNotMatch(t *testing.T) {
	m := NewModuleDoh()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	cond, err := condition.Build(`req_path_prefix_in("/doh", true)`)
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}
	m.cond = cond

	req := buildDohRequest("GET", t)
	req.Session = &bfe_basic.Session{IsSecure: true}
	req.HttpRequest.URL.Path = "/other"

	ret, resp := m.dohHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("ret = %d, want %d", ret, bfe_module.BfeHandlerGoOn)
	}
	if resp != nil {
		t.Fatalf("resp should be nil, got %v", resp)
	}
}

func TestDohHandlerFetchError(t *testing.T) {
	m := NewModuleDoh()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	m.dnsFetcher = new(errorDnsFetcher)

	req := buildDohRequest("GET", t)
	req.Session = &bfe_basic.Session{IsSecure: true}

	ret, resp := m.dohHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Fatalf("ret = %d, want %d", ret, bfe_module.BfeHandlerResponse)
	}
	if resp == nil || resp.StatusCode != bfe_http.StatusInternalServerError {
		t.Fatalf("resp = %v, want 500 Internal Server Error", resp)
	}
}
