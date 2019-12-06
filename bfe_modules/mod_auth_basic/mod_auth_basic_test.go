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

package mod_auth_basic

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

func TestAuthStaticFileHandler(t *testing.T) {
	m := NewModuleAuthBasic()
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
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org", nil)
	ret, resp := m.authBasicHandler(req)
	if ret != bfe_module.BFE_HANDLER_RESPONSE {
		t.Errorf("ret should be %d, not %d", bfe_module.BFE_HANDLER_RESPONSE, ret)
		return
	}
	if resp.StatusCode != bfe_http.StatusUnauthorized {
		t.Errorf("status code should be %d, not %d", bfe_http.StatusUnauthorized, resp.StatusCode)
		return
	}

	req.HttpRequest.Header.Set("Authorization", "Basic dW5pdHRlc3Q6MTIzNDU2")
	ret, _ = m.authBasicHandler(req)
	if ret != bfe_module.BFE_HANDLER_GOON {
		t.Errorf("ret should be %d, not %d", bfe_module.BFE_HANDLER_GOON, ret)
	}

	req.HttpRequest.Header.Set("Authorization", "Basic dW5pdHRlc3QyOjEyMzQ1Ng==")
	ret, _ = m.authBasicHandler(req)
	if ret != bfe_module.BFE_HANDLER_GOON {
		t.Errorf("ret should be %d, not %d", bfe_module.BFE_HANDLER_GOON, ret)
	}

	req.HttpRequest.Host = "example.org"
	ret, _ = m.authBasicHandler(req)
	if ret != bfe_module.BFE_HANDLER_GOON {
		t.Errorf("ret should be %d, not %d", bfe_module.BFE_HANDLER_GOON, ret)
	}
}
