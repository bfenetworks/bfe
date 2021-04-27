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

package mod_redirect

import (
	"net/url"
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

func prepareModuleRedirect() (*ModuleRedirect, error) {
	m := NewModuleRedirect()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, "./testdata"); err != nil {
		return nil, err
	}
	return m, nil
}

func TestRedirectHandler(t *testing.T) {
	m, err := prepareModuleRedirect()
	if err != nil {
		t.Errorf("TestRedirectHandler(): %s", err)
		return
	}

	// case 1
	req1 := new(bfe_basic.Request)
	req1.Session = new(bfe_basic.Session)
	req1.Route.Product = "pn"
	req1.HttpRequest = new(bfe_http.Request)
	req1.HttpRequest.Host = "www.example.org"
	req1.HttpRequest.URL, _ = url.Parse("/index/?space=true")

	result, _ := m.redirectHandler(req1)
	if result != bfe_module.BfeHandlerRedirect {
		t.Errorf("Should return BfeHandlerRedirect")
	}

	// case 2
	req2 := new(bfe_basic.Request)
	req2.Route.Product = "pb"
	req2.HttpRequest = new(bfe_http.Request)
	req2.HttpRequest.Host = "www.example.org"
	req2.HttpRequest.URL, _ = url.Parse("/index/?space=true")

	result, _ = m.redirectHandler(req2)
	if result == bfe_module.BfeHandlerRedirect {
		t.Errorf("Should return BfeHandlerGoOn")
	}
}
