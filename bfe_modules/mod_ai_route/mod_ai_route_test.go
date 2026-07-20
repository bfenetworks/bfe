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

package mod_ai_route

import (
	"net/http"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func newTestRequestWithApiKey(host, apiKey string) *bfe_basic.Request {
	req, _ := bfe_http.NewRequest(http.MethodGet, "http://"+host+"/v1/chat/completions", nil)
	basicReq := bfe_basic.NewRequest(req, nil, nil, nil, nil)
	aiMeta := basicReq.InitAiBasicInfo()
	aiMeta.ClientApiKey = apiKey
	return basicReq
}

func TestRouteFoundProductHandlerHit(t *testing.T) {
	m := NewModuleAiRoute()
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}
	if err := m.routeTable.Update(data); err != nil {
		t.Fatalf("update table failed: %s", err)
	}

	req := newTestRequestWithApiKey("api.example.org", "ak_user_a")
	ret, resp := m.routeFoundProductHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}

	result := req.GetAiRouteResult()
	if result == nil {
		t.Fatal("expected AiRouteResult set in context")
	}
	if result.RouteType != RouteTypeApikey {
		t.Errorf("RouteType: expected apikey, got %s", result.RouteType)
	}
	if result.RuleName != "user_a-rule1" {
		t.Errorf("RuleName: expected user_a-rule1, got %s", result.RuleName)
	}
}

func TestRouteFoundProductHandlerNoAiBasicInfo(t *testing.T) {
	m := NewModuleAiRoute()

	req, _ := bfe_http.NewRequest(http.MethodGet, "http://api.example.org/v1/chat/completions", nil)
	basicReq := bfe_basic.NewRequest(req, nil, nil, nil, nil)

	ret, resp := m.routeFoundProductHandler(basicReq)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
	if basicReq.GetAiRouteResult() != nil {
		t.Error("expected no AiRouteResult when AiBasicInfo is nil")
	}
}

func TestRouteFoundProductHandlerEmptyApiKey(t *testing.T) {
	m := NewModuleAiRoute()

	req, _ := bfe_http.NewRequest(http.MethodGet, "http://api.example.org/v1/chat/completions", nil)
	basicReq := bfe_basic.NewRequest(req, nil, nil, nil, nil)
	basicReq.InitAiBasicInfo()

	ret, resp := m.routeFoundProductHandler(basicReq)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
}

func TestRouteFoundProductHandlerMiss(t *testing.T) {
	m := NewModuleAiRoute()
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}
	if err := m.routeTable.Update(data); err != nil {
		t.Fatalf("update table failed: %s", err)
	}

	req := newTestRequestWithApiKey("api.example.org", "ak_no_binding")
	ret, resp := m.routeFoundProductHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
	if req.GetAiRouteResult() != nil {
		t.Error("expected no AiRouteResult when route miss")
	}
}
