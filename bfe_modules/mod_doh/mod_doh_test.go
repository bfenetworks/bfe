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

package mod_doh

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

type TestDnsFetcher struct{}

func (f *TestDnsFetcher) Fetch(req *bfe_basic.Request) (*bfe_http.Response, error) {
	return DnsMsgToResponse(req, buildDnsMsg())
}

func TestDohHandlerSecure(t *testing.T) {
	m := NewModuleDoh()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	m.dnsFetcher = new(TestDnsFetcher)

	req := buildDohRequest("GET", t)
	req.Session = new(bfe_basic.Session)
	req.Session.IsSecure = true

	ret, resp := m.dohHandler(req)
	if ret != bfe_module.BfeHandlerResponse {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
		return
	}
	if resp.StatusCode != bfe_http.StatusOK {
		t.Errorf("status code should be %d, not %d", bfe_http.StatusOK, resp.StatusCode)
	}
}
