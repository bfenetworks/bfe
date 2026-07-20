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

func newTestRequest(host string) *bfe_basic.Request {
	req, _ := bfe_http.NewRequest(http.MethodGet, "http://"+host+"/v1/chat/completions", nil)
	return bfe_basic.NewRequest(req, nil, nil, nil, nil)
}

func TestValidateRouteTableValid(t *testing.T) {
	table := RouteTable{
		Type:  RouteTypeGlobal,
		Owner: "global",
		Rules: []RouteRule{
			{
				Name:    "global-default",
				CondStr: "default_t()",
				Targets: []bfe_basic.AiRouteTarget{
					{ClusterName: "cluster_global", Model: "", Weight: 100},
				},
				Fallbacks: []bfe_basic.AiRouteFallback{},
			},
		},
	}
	if err := ValidateRouteTable(&table); err != nil {
		t.Errorf("expected no error, got: %s", err)
	}
	if table.Rules[0].Cond == nil {
		t.Error("Cond should be compiled")
	}
}

func TestValidateRouteTableInvalidType(t *testing.T) {
	table := RouteTable{
		Type:  "unknown",
		Owner: "global",
		Rules: []RouteRule{
			{
				Name:    "global-default",
				CondStr: "default_t()",
				Targets: []bfe_basic.AiRouteTarget{
					{ClusterName: "cluster_global", Model: "", Weight: 100},
				},
			},
		},
	}
	if err := ValidateRouteTable(&table); err == nil {
		t.Error("expected error for invalid route table type")
	}
}

func TestValidateRouteTableEmptyRuleName(t *testing.T) {
	table := RouteTable{
		Type: RouteTypeGlobal,
		Rules: []RouteRule{
			{
				Name:    "",
				CondStr: "default_t()",
				Targets: []bfe_basic.AiRouteTarget{
					{ClusterName: "cluster_global", Model: "", Weight: 100},
				},
			},
		},
	}
	if err := ValidateRouteTable(&table); err == nil {
		t.Error("expected error for empty rule name")
	}
}

func TestValidateRouteTableInvalidCond(t *testing.T) {
	table := RouteTable{
		Type: RouteTypeGlobal,
		Rules: []RouteRule{
			{
				Name:    "global-default",
				CondStr: "invalid_cond(",
				Targets: []bfe_basic.AiRouteTarget{
					{ClusterName: "cluster_global", Model: "", Weight: 100},
				},
			},
		},
	}
	if err := ValidateRouteTable(&table); err == nil {
		t.Error("expected error for invalid condition")
	}
}

func TestValidateRouteTableEmptyTargets(t *testing.T) {
	table := RouteTable{
		Type: RouteTypeGlobal,
		Rules: []RouteRule{
			{
				Name:    "global-default",
				CondStr: "default_t()",
				Targets: []bfe_basic.AiRouteTarget{},
			},
		},
	}
	if err := ValidateRouteTable(&table); err == nil {
		t.Error("expected error for empty targets")
	}
}

func TestValidateRouteTableInvalidWeight(t *testing.T) {
	table := RouteTable{
		Type: RouteTypeGlobal,
		Rules: []RouteRule{
			{
				Name:    "global-default",
				CondStr: "default_t()",
				Targets: []bfe_basic.AiRouteTarget{
					{ClusterName: "cluster_global", Model: "", Weight: 60},
					{ClusterName: "cluster_global_2", Model: "", Weight: 30},
				},
			},
		},
	}
	if err := ValidateRouteTable(&table); err == nil {
		t.Error("expected error for invalid total weight")
	}
}

func TestRouteTableMatch(t *testing.T) {
	table := RouteTable{
		Type:  RouteTypeApikey,
		Owner: "ak_user_a",
		Rules: []RouteRule{
			{
				Name:    "host-match",
				CondStr: "req_host_in(\"api.example.org\")",
				Targets: []bfe_basic.AiRouteTarget{
					{ClusterName: "cluster_a", Model: "", Weight: 100},
				},
			},
			{
				Name:    "default",
				CondStr: "default_t()",
				Targets: []bfe_basic.AiRouteTarget{
					{ClusterName: "cluster_default", Model: "", Weight: 100},
				},
			},
		},
	}
	if err := ValidateRouteTable(&table); err != nil {
		t.Fatalf("validate failed: %s", err)
	}

	matched := table.Match(newTestRequest("api.example.org"))
	if matched == nil || matched.Name != "host-match" {
		t.Errorf("expected host-match rule, got %v", matched)
	}

	matched = table.Match(newTestRequest("other.example.org"))
	if matched == nil || matched.Name != "default" {
		t.Errorf("expected default rule, got %v", matched)
	}
}
