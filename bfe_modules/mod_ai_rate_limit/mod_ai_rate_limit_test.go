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
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/action"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

func TestMatchModelEmptyPolicyModels(t *testing.T) {
	if !matchModel(nil, "gpt-4") {
		t.Error("empty policyModels should match any model")
	}
	if !matchModel([]string{}, "gpt-4") {
		t.Error("empty policyModels should match any model")
	}
}

func TestMatchModelEmptyClientModel(t *testing.T) {
	if matchModel([]string{"gpt-4"}, "") {
		t.Error("empty client model should not match")
	}
}

func TestMatchModelWildcard(t *testing.T) {
	if !matchModel([]string{"*"}, "gpt-4") {
		t.Error("wildcard should match any model")
	}
	if !matchModel([]string{"gpt-4", "*"}, "gpt-4o") {
		t.Error("wildcard should match any model when present in list")
	}
}

func TestMatchModelExactMatch(t *testing.T) {
	if !matchModel([]string{"gpt-4"}, "gpt-4") {
		t.Error("exact model name should match")
	}
	if !matchModel([]string{"gpt-4", "gpt-4o"}, "gpt-4o") {
		t.Error("model should match one in the list")
	}
}

func TestMatchModelNoMatch(t *testing.T) {
	if matchModel([]string{"gpt-4"}, "gpt-4o") {
		t.Error("different model should not match")
	}
	if matchModel([]string{"gpt-4", "gpt-4o"}, "gpt-3.5") {
		t.Error("model not in list should not match")
	}
}

func TestMatchModelWildcardFirst(t *testing.T) {
	if !matchModel([]string{"*", "gpt-4"}, "gpt-4o") {
		t.Error("wildcard at first position should match")
	}
}

func TestMatchModelSingleModelList(t *testing.T) {
	if !matchModel([]string{"claude-3"}, "claude-3") {
		t.Error("single model list should match same model")
	}
	if matchModel([]string{"claude-3"}, "claude-3.5") {
		t.Error("single model list should not match different model")
	}
}

func newTestRequest(product, apiKey, model string) *bfe_basic.Request {
	httpReq, _ := bfe_http.NewRequest(http.MethodGet, "http://example.com/v1/chat/completions", nil)
	req := bfe_basic.NewRequest(httpReq, nil, nil, nil, nil)
	req.Route = bfe_basic.RequestRoute{Product: product}
	ai := req.InitAiBasicInfo()
	ai.ClientApiKey = apiKey
	ai.ClientModel = model
	return req
}

func prepareTestModule(t *testing.T) *ModuleAiRateLimit {
	m := NewModuleAiRateLimit()
	m.productConfPath = "testdata/mod_ai_rate_limit/ai_rate_limit.data"
	if _, err := m.loadProductRuleTable(nil); err != nil {
		t.Fatalf("loadProductRuleTable failed: %s", err)
	}
	return m
}

func TestNewModuleAiRateLimitAndName(t *testing.T) {
	m := NewModuleAiRateLimit()
	if m == nil {
		t.Fatal("NewModuleAiRateLimit should not return nil")
	}
	if m.Name() != ModAiRateLimit {
		t.Errorf("expected name %s, got %s", ModAiRateLimit, m.Name())
	}
	if m.state == nil {
		t.Error("module state should be initialized")
	}
	if m.productTable == nil {
		t.Error("productTable should be initialized")
	}
	if m.limiterManager == nil {
		t.Error("limiterManager should be initialized")
	}
	if m.pmsStates == nil {
		t.Error("pmsStates should be initialized")
	}
}

func TestModuleLoadProductRuleTableDefaultPath(t *testing.T) {
	m := NewModuleAiRateLimit()
	m.productConfPath = "testdata/mod_ai_rate_limit/ai_rate_limit.data"
	msg, err := m.loadProductRuleTable(nil)
	if err != nil {
		t.Fatalf("expected no error, got %s", err)
	}
	if !strings.Contains(msg, "ai_rate_limit.data=1.0") {
		t.Errorf("unexpected load message: %s", msg)
	}
	if v := m.state.GetState("Version"); v != "1.0" {
		t.Errorf("expected version 1.0, got %v", v)
	}
}

func TestModuleLoadProductRuleTableWithQueryPath(t *testing.T) {
	m := NewModuleAiRateLimit()
	query := url.Values{}
	query.Set("path", "testdata/mod_ai_rate_limit/ai_rate_limit_empty.data")
	msg, err := m.loadProductRuleTable(query)
	if err != nil {
		t.Fatalf("expected no error, got %s", err)
	}
	if !strings.Contains(msg, "ai_rate_limit_empty.data=1.0") {
		t.Errorf("unexpected load message: %s", msg)
	}
}

func TestModuleLoadProductRuleTableFailure(t *testing.T) {
	m := NewModuleAiRateLimit()
	query := url.Values{}
	query.Set("path", "testdata/mod_ai_rate_limit/non_existent.data")
	if _, err := m.loadProductRuleTable(query); err == nil {
		t.Error("expected error for non-existent data file")
	}
}

func TestModuleMonitorAndReloadHandlers(t *testing.T) {
	m := NewModuleAiRateLimit()
	mon := m.monitorHandlers()
	if mon == nil {
		t.Fatal("monitorHandlers should not return nil")
	}
	if _, ok := mon[ModAiRateLimit]; !ok {
		t.Error("missing state handler")
	}
	if _, ok := mon[ModAiRateLimit+".diff"]; !ok {
		t.Error("missing diff handler")
	}
	if _, ok := mon[ModAiRateLimit+".prometheus"]; !ok {
		t.Error("missing prometheus handler")
	}

	reload := m.reloadHandlers()
	if reload == nil {
		t.Fatal("reloadHandlers should not return nil")
	}
	if _, ok := reload[ModAiRateLimit]; !ok {
		t.Error("missing reload handler")
	}
}

func TestModuleGetState(t *testing.T) {
	m := prepareTestModule(t)
	state := m.getState()
	if state == nil {
		t.Fatal("getState should not return nil")
	}
	if v := state.States["Version"]; v != "1.0" {
		t.Errorf("expected version 1.0 in state, got %v", v)
	}
}

func TestModuleGetStateDiff(t *testing.T) {
	m := prepareTestModule(t)
	diff := m.getStateDiff()
	if diff == nil {
		t.Fatal("getStateDiff should not return nil")
	}
}

func TestModuleGetPrometheus(t *testing.T) {
	m := prepareTestModule(t)
	data, err := m.getPrometheus()
	if err != nil {
		t.Fatalf("getPrometheus failed: %s", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty prometheus output")
	}
	if !strings.Contains(string(data), "tpm_match_total") {
		t.Error("expected tpm_match_total in prometheus output")
	}
}

func TestLimitFoundProductHandlerNoAiBasicInfo(t *testing.T) {
	m := prepareTestModule(t)
	httpReq, _ := bfe_http.NewRequest(http.MethodGet, "http://example.com/v1/chat/completions", nil)
	req := bfe_basic.NewRequest(httpReq, nil, nil, nil, nil)
	req.Route = bfe_basic.RequestRoute{Product: "AI_product"}

	ret, resp := m.limitFoundProductHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
}

func TestLimitFoundProductHandlerNoRulesForProduct(t *testing.T) {
	m := prepareTestModule(t)
	req := newTestRequest("unknown_product", "ak-2v8x9k3m7p", "gpt-4")

	ret, resp := m.limitFoundProductHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
}

func TestLimitFoundProductHandlerNoApiKey(t *testing.T) {
	m := prepareTestModule(t)
	req := newTestRequest("AI_product", "", "gpt-4")

	ret, resp := m.limitFoundProductHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
}

func TestLimitFoundProductHandlerNoPolicyBinding(t *testing.T) {
	m := prepareTestModule(t)
	req := newTestRequest("AI_product", "ak-unbound", "gpt-4")

	ret, resp := m.limitFoundProductHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
}

func TestLimitFoundProductHandlerPolicyDisabled(t *testing.T) {
	m := prepareTestModule(t)
	m.productTable.lock.Lock()
	m.productTable.apiKeyBinding["ak-disabled"] = []string{"rlp-0001"}
	m.productTable.ratePolicies["rlp-0001"].Enabled = false
	m.productTable.lock.Unlock()

	req := newTestRequest("AI_product", "ak-disabled", "gpt-4")
	ret, resp := m.limitFoundProductHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
}

func TestLimitFoundProductHandlerModelMismatch(t *testing.T) {
	m := prepareTestModule(t)
	m.productTable.lock.Lock()
	m.productTable.apiKeyBinding["ak-model-mismatch"] = []string{"rlp-0001"}
	m.productTable.ratePolicies["rlp-0001"].Models = []string{"gpt-4"}
	m.productTable.lock.Unlock()

	req := newTestRequest("AI_product", "ak-model-mismatch", "not-in-policy")
	ret, resp := m.limitFoundProductHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response")
	}
}

func TestExecutePolicyActionClose(t *testing.T) {
	m := NewModuleAiRateLimit()
	req := newTestRequest("AI_product", "ak-123", "gpt-4")
	meta := req.GetAiBasicInfo()
	rule := &productRule{hitAction: &action.Action{Cmd: action.ActionClose}}

	ret, resp := m.executePolicyAction(req, meta, "rlp-0001", &PolicyConf{Name: "test"}, rule)
	if ret != bfe_module.BfeHandlerClose {
		t.Errorf("expected BfeHandlerClose, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response for close action")
	}
}

func TestExecutePolicyActionPass(t *testing.T) {
	m := NewModuleAiRateLimit()
	req := newTestRequest("AI_product", "ak-123", "gpt-4")
	meta := req.GetAiBasicInfo()
	rule := &productRule{hitAction: &action.Action{Cmd: action.ActionPass}}

	ret, resp := m.executePolicyAction(req, meta, "rlp-0001", &PolicyConf{Name: "test"}, rule)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
	if resp != nil {
		t.Error("expected nil response for pass action")
	}
}

func TestExecutePolicyActionFinish(t *testing.T) {
	m := NewModuleAiRateLimit()
	req := newTestRequest("AI_product", "ak-123", "gpt-4")
	meta := req.GetAiBasicInfo()
	rule := &productRule{hitAction: &action.Action{Cmd: action.ActionFinish}}

	ret, resp := m.executePolicyAction(req, meta, "rlp-0001", &PolicyConf{Name: "test"}, rule)
	if ret != bfe_module.BfeHandlerFinish {
		t.Errorf("expected BfeHandlerFinish, got %d", ret)
	}
	if resp == nil {
		t.Fatal("expected non-nil response for finish action")
	}
	if resp.StatusCode != 429 {
		t.Errorf("expected status 429, got %d", resp.StatusCode)
	}
}

func TestLimitRequestFinishHandlerNoContext(t *testing.T) {
	m := NewModuleAiRateLimit()
	req := newTestRequest("AI_product", "ak-123", "gpt-4")

	ret := m.limitRequestFinishHandler(req, nil)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
}

func TestLimitRequestFinishHandlerWithEmptyContext(t *testing.T) {
	m := NewModuleAiRateLimit()
	req := newTestRequest("AI_product", "ak-123", "gpt-4")
	setPolicyLimiterContext(req, &PolicyLimiterContext{})

	ret := m.limitRequestFinishHandler(req, nil)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("expected BfeHandlerGoOn, got %d", ret)
	}
}

func TestModuleInit(t *testing.T) {
	m := NewModuleAiRateLimit()
	err := m.Init(bfe_module.NewBfeCallbacks(), web_monitor.NewWebHandlers(), "testdata")
	if err == nil {
		t.Error("expected Init to fail because default conf references missing product rule path")
	}
}
