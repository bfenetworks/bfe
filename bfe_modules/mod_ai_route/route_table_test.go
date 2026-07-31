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
)

func newTestBasicRequest(host string) *bfe_basic.Request {
	req, _ := bfe_http.NewRequest(http.MethodGet, "http://"+host+"/v1/chat/completions", nil)
	return bfe_basic.NewRequest(req, nil, nil, bfe_basic.NewSession(nil), nil)
}

func TestAiRouteTableUpdateValid(t *testing.T) {
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}

	table := NewAiRouteTable()
	if err := table.Update(data); err != nil {
		t.Errorf("update should success, got: %s", err)
	}
}

func TestAiRouteTableUpdateInvalidWeight(t *testing.T) {
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data.invalid_weight")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}

	table := NewAiRouteTable()
	if err := table.Update(data); err == nil {
		t.Error("expected error for invalid weight sum")
	}
}

func TestAiRouteTableUpdateInvalidCond(t *testing.T) {
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data.invalid_cond")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}

	table := NewAiRouteTable()
	if err := table.Update(data); err == nil {
		t.Error("expected error for invalid condition")
	}
}

func TestAiRouteTableUpdateEmptyTargets(t *testing.T) {
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data.empty_targets")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}

	table := NewAiRouteTable()
	if err := table.Update(data); err == nil {
		t.Error("expected error for empty targets")
	}
}

func TestAiRouteTableSearchHitApikey(t *testing.T) {
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}

	table := NewAiRouteTable()
	if err := table.Update(data); err != nil {
		t.Fatalf("update failed: %s", err)
	}

	req := newTestBasicRequest("api.example.org")
	result := table.Search("ak_user_a", req)
	if result == nil {
		t.Fatal("expected hit apikey route")
	}
	if result.RouteType != RouteTypeApikey {
		t.Errorf("RouteType: expected apikey, got %s", result.RouteType)
	}
	if result.Owner != "ak_user_a" {
		t.Errorf("Owner: expected ak_user_a, got %s", result.Owner)
	}
	if result.RuleName != "user_a-rule1" {
		t.Errorf("RuleName: expected user_a-rule1, got %s", result.RuleName)
	}
	if len(result.Targets) != 2 {
		t.Errorf("Targets count: expected 2, got %d", len(result.Targets))
	}
	if len(result.Fallbacks) != 1 {
		t.Errorf("Fallbacks count: expected 1, got %d", len(result.Fallbacks))
	}
}

func TestAiRouteTableSearchHitEntity(t *testing.T) {
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}

	table := NewAiRouteTable()
	if err := table.Update(data); err != nil {
		t.Fatalf("update failed: %s", err)
	}

	req := newTestBasicRequest("other.example.org")
	result := table.Search("ak_user_b", req)
	if result == nil {
		t.Fatal("expected hit entity route")
	}
	if result.RouteType != RouteTypeEntity {
		t.Errorf("RouteType: expected entity, got %s", result.RouteType)
	}
	if result.Owner != "dept_ai" {
		t.Errorf("Owner: expected dept_ai, got %s", result.Owner)
	}
}

func TestAiRouteTableSearchHitGlobal(t *testing.T) {
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}

	table := NewAiRouteTable()
	if err := table.Update(data); err != nil {
		t.Fatalf("update failed: %s", err)
	}

	req := newTestBasicRequest("unknown.example.org")
	result := table.Search("ak_user_a", req)
	if result == nil {
		t.Fatal("expected hit global route")
	}
	if result.RouteType != RouteTypeGlobal {
		t.Errorf("RouteType: expected global, got %s", result.RouteType)
	}
	if result.Owner != "global" {
		t.Errorf("Owner: expected global, got %s", result.Owner)
	}
}

func TestAiRouteTableSearchNoBinding(t *testing.T) {
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}

	table := NewAiRouteTable()
	if err := table.Update(data); err != nil {
		t.Fatalf("update failed: %s", err)
	}

	req := newTestBasicRequest("api.example.org")
	result := table.Search("ak_no_binding", req)
	if result != nil {
		t.Error("expected nil for apikey without binding")
	}
}

func TestAiRouteTableSearchMissingTable(t *testing.T) {
	data, err := AiRouteDataLoad("testdata/mod_ai_route/ai_route.data")
	if err != nil {
		t.Fatalf("load data failed: %s", err)
	}

	table := NewAiRouteTable()
	if err := table.Update(data); err != nil {
		t.Fatalf("update failed: %s", err)
	}

	req := newTestBasicRequest("api.example.org")
	result := table.Search("ak_unknown", req)
	if result != nil {
		t.Error("expected nil when all bound tables missing")
	}
}
