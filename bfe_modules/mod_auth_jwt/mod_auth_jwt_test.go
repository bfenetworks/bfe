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

package mod_auth_jwt

import (
	"fmt"
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

func TestAuthJWTFileHandlerRuleNotMatched(t *testing.T) {
	testModAuthJWT(t, "example.org", "", func(
		t *testing.T, m *ModuleAuthJWT, ret int, resp *bfe_http.Response) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}
	})
}

func TestAuthJWTFileHandlerNoAuthorization(t *testing.T) {
	testModAuthJWT(t, "www.example.org", "", func(
		t *testing.T, m *ModuleAuthJWT, ret int, resp *bfe_http.Response) {
		if ret != bfe_module.BfeHandlerResponse {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
			return
		}
		if resp.StatusCode != bfe_http.StatusUnauthorized {
			t.Errorf("status code should be %d, not %d", bfe_http.StatusUnauthorized, resp.StatusCode)
			return
		}
	})
}

func TestAuthJWTFileHandlerTokenInvalid(t *testing.T) {
	testModAuthJWT(t, "www.example.org", "INVALID", func(
		t *testing.T, m *ModuleAuthJWT, ret int, resp *bfe_http.Response) {
		if ret != bfe_module.BfeHandlerResponse {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerResponse, ret)
			return
		}
		if resp.StatusCode != bfe_http.StatusUnauthorized {
			t.Errorf("status code should be %d, not %d", bfe_http.StatusUnauthorized, resp.StatusCode)
			return
		}
	})
}

func TestAuthJWTFileHandlerCorrect(t *testing.T) {
	header := "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiIsImtpZCI6IjAwMDEifQ"
	payload := "eyJuYW1lIjoiVW5pdHRlc3QiLCJzdWIiOiJVbml0dGVzdCIsImlzcyI6IkJGRSBHYXRld2F5In0"
	signature := "NZcVkR0hcJLFrmtFZGlrMUye5wFB2twCKDDRHwn4QQ4"
	token := fmt.Sprintf("%s.%s.%s", header, payload, signature)

	testModAuthJWT(t, "www.example.org", token, func(
		t *testing.T, m *ModuleAuthJWT, ret int, resp *bfe_http.Response) {
		if ret != bfe_module.BfeHandlerGoOn {
			t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
		}
	})
}

func testModAuthJWT(t *testing.T, host string, token string,
	check func(*testing.T, *ModuleAuthJWT, int, *bfe_http.Response)) {
	// prepare module
	m := NewModuleAuthJWT()
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
	req.HttpRequest, _ = bfe_http.NewRequest("GET", fmt.Sprintf("http://%s", host), nil)
	if token != "" {
		req.HttpRequest.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	// process request and check
	ret, resp := m.authJWTHandler(req)
	check(t, m, ret, resp)
	if resp != nil {
		resp.Body.Close()
	}
}
