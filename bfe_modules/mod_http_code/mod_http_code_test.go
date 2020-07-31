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

package mod_http_code

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

func TestRequestFinish(t *testing.T) {
	m := NewModuleHttpCode()
	var req bfe_basic.Request
	req.HttpRequest = new(bfe_http.Request)
	req.HttpResponse = new(bfe_http.Response)
	req.HttpResponse.StatusCode = 201
	req.Session = new(bfe_basic.Session)

	m.requestFinish(&req, nil)
	if m.state.All2XX.Get() != 1 {
		t.Errorf("counter All2XX should be 1")
	}

	m.requestFinish(&req, nil)

	if m.state.All2XX.Get() != 2 {
		t.Errorf("counter All2XX should be 1")
	}
}

func TestInit(t *testing.T) {
	m := NewModuleHttpCode()
	if m.Name() != ModHttpCode {
		t.Errorf("ModHttpCode Name() should be %s", ModHttpCode)
	}

	cbs := bfe_module.NewBfeCallbacks()
	whs := web_monitor.NewWebHandlers()
	err := m.Init(cbs, whs, "test")
	if err != nil {
		t.Errorf("ModHttpCode Init() error: %v", err)
	}
}
