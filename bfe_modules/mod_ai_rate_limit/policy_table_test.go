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

package mod_ai_rate_limit

import (
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic/action"
	"github.com/bfenetworks/go-lib/web-monitor/module_state2"
)

func TestNewProductRuleTable(t *testing.T) {
	state := new(module_state2.State)
	state.Init()

	table := newProductRuleTable(state)
	if table == nil {
		t.Fatal("newProductRuleTable should not return nil")
	}
	if table.state != state {
		t.Error("productRuleTable state mismatch")
	}
	if len(table.productRules) != 0 {
		t.Errorf("productRules should be empty, got %d", len(table.productRules))
	}
	if len(table.ratePolicies) != 0 {
		t.Errorf("ratePolicies should be empty, got %d", len(table.ratePolicies))
	}
	if len(table.apiKeyBinding) != 0 {
		t.Errorf("apiKeyBinding should be empty, got %d", len(table.apiKeyBinding))
	}
}

func TestProductRuleTableLoadAndLookup(t *testing.T) {
	conf, err := AiRateLimitConfLoad("testdata/mod_ai_rate_limit/ai_rate_limit.data")
	if err != nil {
		t.Fatalf("load conf failed: %s", err)
	}

	state := new(module_state2.State)
	state.Init()
	table := newProductRuleTable(state)

	if err := table.load(conf); err != nil {
		t.Fatalf("load table failed: %s", err)
	}

	rules := table.getProductRules("AI_product")
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule for AI_product, got %d", len(rules))
	}
	if rules[0].condStr != "default_t()" {
		t.Errorf("expected cond default_t(), got %s", rules[0].condStr)
	}
	if rules[0].hitAction == nil || rules[0].hitAction.Cmd != action.ActionPass {
		t.Errorf("expected PASS action, got %v", rules[0].hitAction)
	}

	if rules := table.getProductRules("not_exist"); rules != nil {
		t.Error("non-existent product should return nil rules")
	}

	policy := table.getPolicy("rlp-0001")
	if policy == nil {
		t.Fatal("expected policy rlp-0001")
	}
	if policy.Name != "ratelimitX" {
		t.Errorf("expected policy name ratelimitX, got %s", policy.Name)
	}
	if !policy.Enabled {
		t.Error("expected policy rlp-0001 enabled")
	}
	if table.getPolicy("not_exist") != nil {
		t.Error("non-existent policy should return nil")
	}

	ids := table.getPolicyIds("ak-2v8x9k3m7p")
	if len(ids) != 2 {
		t.Fatalf("expected 2 policy ids, got %d", len(ids))
	}
	if ids[0] != "rlp-0001" || ids[1] != "rlp-0002" {
		t.Errorf("unexpected policy ids: %v", ids)
	}
	if len(table.getPolicyIds("not_exist")) != 0 {
		t.Error("non-existent api key should return empty policy ids")
	}
}

func TestProductRuleTableLoadEmpty(t *testing.T) {
	conf, err := AiRateLimitConfLoad("testdata/mod_ai_rate_limit/ai_rate_limit_empty.data")
	if err != nil {
		t.Fatalf("load empty conf failed: %s", err)
	}

	state := new(module_state2.State)
	state.Init()
	table := newProductRuleTable(state)

	if err := table.load(conf); err != nil {
		t.Fatalf("load empty table failed: %s", err)
	}

	if table.getProductRules("AI_product") != nil {
		t.Error("empty table should return nil product rules")
	}
	if table.getPolicy("rlp-0001") != nil {
		t.Error("empty table should return nil policy")
	}
	if len(table.getPolicyIds("ak-2v8x9k3m7p")) != 0 {
		t.Error("empty table should return empty policy ids")
	}
}

func TestProductRuleTableLoadInvalidCond(t *testing.T) {
	version := "1.0"
	cond := "invalid_cond("
	act := &action.Action{Cmd: action.ActionPass}
	conf := &AiRateLimitConf{
		Version: &version,
		Config: &map[string]*ProductRuleConfList{
			"AI_product": {{Cond: cond, HitAction: act}},
		},
		RateLimitPolicies: &map[string]*PolicyConf{},
	}

	state := new(module_state2.State)
	state.Init()
	table := newProductRuleTable(state)

	if err := table.load(conf); err == nil {
		t.Error("expected error for invalid condition")
	}
}
