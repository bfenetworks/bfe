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
)

// ==================== Convert tests ====================

func TestProductRuleConfFileConvert(t *testing.T) {
	cond := "default_t()"
	act := &action.Action{Cmd: "PASS"}
	f := &ProductRuleConfFile{Cond: &cond, HitAction: act}
	result := f.Convert()
	if result.Cond != cond {
		t.Errorf("Cond: expected %s, got %s", cond, result.Cond)
	}
	if result.HitAction != act {
		t.Error("Action pointer mismatch")
	}
}

func TestProductRuleConfListFileConvert(t *testing.T) {
	cond1 := "default_t()"
	cond2 := "req_host_in(\"example.com\")"
	act := &action.Action{Cmd: "PASS"}
	f := ProductRuleConfListFile{
		{Cond: &cond1, HitAction: act},
		{Cond: &cond2, HitAction: act},
	}
	result := f.Convert()
	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}
	if result[0].Cond != cond1 {
		t.Errorf("item 0 Cond: expected %s, got %s", cond1, result[0].Cond)
	}
	if result[1].Cond != cond2 {
		t.Errorf("item 1 Cond: expected %s, got %s", cond2, result[1].Cond)
	}
}

func TestTPMRuleConfFileConvert(t *testing.T) {
	f := &TPMRuleConfFile{WindowMinutes: 5, MaxTokens: 1000, StepMinutes: 2, Models: []string{"gpt-4", "gpt-4o"}}
	result := f.Convert()
	if result.TimeWindow != 300 {
		t.Errorf("TimeWindow: expected 300, got %d", result.TimeWindow)
	}
	if result.Threshold != 1000 {
		t.Errorf("Threshold: expected 1000, got %d", result.Threshold)
	}
	if result.BucketTimeWindow != 60 {
		t.Errorf("BucketTimeWindow: expected 60, got %d", result.BucketTimeWindow)
	}
	if result.BucketThreshold != 1000 {
		t.Errorf("BucketThreshold: expected 1000, got %d", result.BucketThreshold)
	}
	if result.ReservedX != 0 {
		t.Errorf("ReservedX: expected 0, got %f", result.ReservedX)
	}
	if result.ReservedOff != 0 {
		t.Errorf("ReservedOff: expected 0, got %f", result.ReservedOff)
	}
	if len(result.Models) != 2 || result.Models[0] != "gpt-4" || result.Models[1] != "gpt-4o" {
		t.Errorf("Models mismatch: got %v", result.Models)
	}
}

func TestRPMRuleConfFileConvert(t *testing.T) {
	f := &RPMRuleConfFile{WindowMinutes: 2, MaxRequests: 100, Burst: 10, Models: []string{"gpt-4"}}
	result := f.Convert()
	if result.TimeWindow != 120 {
		t.Errorf("TimeWindow: expected 120, got %d", result.TimeWindow)
	}
	if result.MaxRequests != 100 {
		t.Errorf("MaxRequests: expected 100, got %d", result.MaxRequests)
	}
	if result.Burst != 10 {
		t.Errorf("Burst: expected 10, got %d", result.Burst)
	}
	if len(result.Models) != 1 || result.Models[0] != "gpt-4" {
		t.Errorf("Models mismatch: got %v", result.Models)
	}
}

func TestLimitRulesConfFileConvert(t *testing.T) {
	maxConn := int64(50)
	f := &LimitRulesConfFile{
		TPM:            []TPMRuleConfFile{{WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1}},
		RPM:            []RPMRuleConfFile{{WindowMinutes: 1, MaxRequests: 100, Burst: 10}},
		MaxConcurrency: &maxConn,
	}
	result := f.Convert()
	if result.MaxConcurrency != 50 {
		t.Errorf("MaxConcurrency: expected 50, got %d", result.MaxConcurrency)
	}
	if len(result.TPM) != 1 {
		t.Errorf("TPM length: expected 1, got %d", len(result.TPM))
	}
	if len(result.RPM) != 1 {
		t.Errorf("RPM length: expected 1, got %d", len(result.RPM))
	}
}

func TestLimitRulesConfFileConvertNilMaxConcurrency(t *testing.T) {
	f := &LimitRulesConfFile{
		TPM: []TPMRuleConfFile{{WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1}},
	}
	result := f.Convert()
	if result.MaxConcurrency != -1 {
		t.Errorf("MaxConcurrency: expected -1, got %d", result.MaxConcurrency)
	}
}

func TestPolicyConfFileConvert(t *testing.T) {
	f := &PolicyConfFile{
		Name:    "test-policy",
		Enabled: true,
		Models:  []string{"gpt-4"},
		Rules: &LimitRulesConfFile{
			TPM: []TPMRuleConfFile{{WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1}},
		},
	}
	result := f.Convert()
	if result.Name != "test-policy" {
		t.Errorf("Name: expected test-policy, got %s", result.Name)
	}
	if result.Enabled != true {
		t.Errorf("Enabled: expected true, got %v", result.Enabled)
	}
	if len(result.Models) != 1 || result.Models[0] != "gpt-4" {
		t.Errorf("Models mismatch")
	}
	if result.Rules == nil {
		t.Error("Rules should not be nil")
	}
}

func TestPolicyConfFileConvertNilRules(t *testing.T) {
	f := &PolicyConfFile{
		Name:    "test-policy",
		Enabled: true,
	}
	result := f.Convert()
	if result.Rules != nil {
		t.Error("Rules should be nil when not set")
	}
}

func TestAiRateLimitConfFileConvert(t *testing.T) {
	version := "1.0"
	cond := "default_t()"
	act := &action.Action{Cmd: "PASS"}
	f := &AiRateLimitConfFile{
		Version: &version,
		Config: &map[string]*ProductRuleConfListFile{
			"AI_product": {{Cond: &cond, HitAction: act}},
		},
		RateLimitPolicies: &map[string]*PolicyConfFile{
			"rlp-0001": {
				Name:    "rlp-0001",
				Enabled: true,
				Rules: &LimitRulesConfFile{
					TPM: []TPMRuleConfFile{{WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1}},
				},
			},
		},
		ApikeyRatePoliciesBindings: &map[string][]string{
			"ak-123": {"rlp-0001"},
		},
	}
	result := f.Convert()
	if *result.Version != version {
		t.Errorf("Version: expected %s, got %s", version, *result.Version)
	}
	if result.Config == nil {
		t.Error("Config should not be nil")
	}
	if result.RateLimitPolicies == nil {
		t.Error("RateLimitPolicies should not be nil")
	}
	if result.ApikeyRatePoliciesBindings == nil {
		t.Error("ApikeyRatePoliciesBindings should not be nil")
	}
}

func TestAiRateLimitConfFileConvertNilFields(t *testing.T) {
	f := &AiRateLimitConfFile{}
	result := f.Convert()
	if result.Config != nil {
		t.Error("Config should be nil")
	}
	if result.RateLimitPolicies != nil {
		t.Error("RateLimitPolicies should be nil")
	}
}

// ==================== Check tests ====================

func TestProductRuleConfFileCheckValid(t *testing.T) {
	cond := "default_t()"
	act := &action.Action{Cmd: "PASS"}
	f := &ProductRuleConfFile{Cond: &cond, HitAction: act}
	if err := f.Check(); err != nil {
		t.Errorf("expected no error, got: %s", err)
	}
}

func TestProductRuleConfFileCheckNilCond(t *testing.T) {
	act := &action.Action{Cmd: "PASS"}
	f := &ProductRuleConfFile{Cond: nil, HitAction: act}
	if err := f.Check(); err == nil {
		t.Error("expected error for nil Cond")
	}
}

func TestProductRuleConfFileCheckInvalidCond(t *testing.T) {
	cond := "invalid_cond("
	act := &action.Action{Cmd: "PASS"}
	f := &ProductRuleConfFile{Cond: &cond, HitAction: act}
	if err := f.Check(); err == nil {
		t.Error("expected error for invalid condition")
	}
}

func TestProductRuleConfFileCheckNilAction(t *testing.T) {
	cond := "default_t()"
	f := &ProductRuleConfFile{Cond: &cond, HitAction: nil}
	if err := f.Check(); err == nil {
		t.Error("expected error for nil Action")
	}
}

func TestProductRuleConfFileCheckEmptyActionCmd(t *testing.T) {
	cond := "default_t()"
	act := &action.Action{Cmd: ""}
	f := &ProductRuleConfFile{Cond: &cond, HitAction: act}
	if err := f.Check(); err == nil {
		t.Error("expected error for empty Action.Cmd")
	}
}

func TestProductRuleConfFileCheckUnsupportedActionCmd(t *testing.T) {
	cond := "default_t()"
	act := &action.Action{Cmd: "UNSUPPORTED_CMD"}
	f := &ProductRuleConfFile{Cond: &cond, HitAction: act}
	if err := f.Check(); err == nil {
		t.Error("expected error for unsupported Action.Cmd")
	}
}

func TestProductRuleConfListFileCheckValid(t *testing.T) {
	cond1 := "default_t()"
	cond2 := "req_host_in(\"example.com\")"
	act := &action.Action{Cmd: "PASS"}
	f := &ProductRuleConfListFile{
		{Cond: &cond1, HitAction: act},
		{Cond: &cond2, HitAction: act},
	}
	if err := f.Check(); err != nil {
		t.Errorf("expected no error, got: %s", err)
	}
}

func TestProductRuleConfListFileCheckDuplicateCond(t *testing.T) {
	cond := "default_t()"
	act := &action.Action{Cmd: "PASS"}
	f := &ProductRuleConfListFile{
		{Cond: &cond, HitAction: act},
		{Cond: &cond, HitAction: act},
	}
	if err := f.Check(); err == nil {
		t.Error("expected error for duplicate cond")
	}
}

func TestTPMRuleConfFileCheckValid(t *testing.T) {
	f := &TPMRuleConfFile{Name: "abc", WindowMinutes: 10, MaxTokens: 1000, StepMinutes: 5}
	if err := f.Check(); err != nil {
		t.Errorf("expected no error, got: %s", err)
	}
}

func TestTPMRuleConfFileCheckWindowMinutesZero(t *testing.T) {
	f := &TPMRuleConfFile{Name: "abc", WindowMinutes: 0, MaxTokens: 1000, StepMinutes: 1}
	if err := f.Check(); err == nil {
		t.Error("expected error for window_minutes <= 0")
	}
}

func TestTPMRuleConfFileCheckWindowMinutesTooLarge(t *testing.T) {
	f := &TPMRuleConfFile{Name: "abc", WindowMinutes: 361, MaxTokens: 1000, StepMinutes: 1}
	if err := f.Check(); err == nil {
		t.Error("expected error for window_minutes > 360")
	}
}

func TestTPMRuleConfFileCheckStepMinutesGreaterThanWindow(t *testing.T) {
	f := &TPMRuleConfFile{Name: "abc", WindowMinutes: 5, MaxTokens: 1000, StepMinutes: 10}
	if err := f.Check(); err == nil {
		t.Error("expected error for step_minutes > window_minutes")
	}
}

func TestTPMRuleConfFileCheckStepMinutesDefault(t *testing.T) {
	f := &TPMRuleConfFile{Name: "abc", WindowMinutes: 10, MaxTokens: 1000, StepMinutes: 0}
	if err := f.Check(); err != nil {
		t.Errorf("expected no error, got: %s", err)
	}
	if f.StepMinutes != 1 {
		t.Errorf("StepMinutes should default to 1, got %d", f.StepMinutes)
	}
}

func TestRPMRuleConfFileCheckValid(t *testing.T) {
	f := &RPMRuleConfFile{Name: "abc", WindowMinutes: 10, MaxRequests: 100, Burst: 10}
	if err := f.Check(); err != nil {
		t.Errorf("expected no error, got: %s", err)
	}
}

func TestRPMRuleConfFileCheckWindowMinutesZero(t *testing.T) {
	f := &RPMRuleConfFile{Name: "abc", WindowMinutes: 0, MaxRequests: 100, Burst: 10}
	if err := f.Check(); err == nil {
		t.Error("expected error for window_minutes <= 0")
	}
}

func TestRPMRuleConfFileCheckWindowMinutesTooLarge(t *testing.T) {
	f := &RPMRuleConfFile{Name: "abc", WindowMinutes: 361, MaxRequests: 100, Burst: 10}
	if err := f.Check(); err == nil {
		t.Error("expected error for window_minutes > 360")
	}
}

func TestRPMRuleConfFileCheckBurstDefault(t *testing.T) {
	f := &RPMRuleConfFile{Name: "abc", WindowMinutes: 10, MaxRequests: 100, Burst: 0}
	if err := f.Check(); err != nil {
		t.Errorf("expected no error, got: %s", err)
	}
	if f.Burst != 1 {
		t.Errorf("Burst should default to 1, got %d", f.Burst)
	}
}

func TestLimitRulesConfFileCheckValid(t *testing.T) {
	f := &LimitRulesConfFile{
		TPM: []TPMRuleConfFile{{Name: "abc", WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1}},
		RPM: []RPMRuleConfFile{{Name: "abc", WindowMinutes: 1, MaxRequests: 100, Burst: 10}},
	}
	if err := f.Check(); err != nil {
		t.Errorf("expected no error, got: %s", err)
	}
}

func TestLimitRulesConfFileCheckOnlyMaxConcurrency(t *testing.T) {
	maxConn := int64(50)
	f := &LimitRulesConfFile{MaxConcurrency: &maxConn}
	if err := f.Check(); err != nil {
		t.Errorf("expected no error, got: %s", err)
	}
}

func TestLimitRulesConfFileCheckTooManyTPM(t *testing.T) {
	f := &LimitRulesConfFile{
		TPM: []TPMRuleConfFile{
			{Name: "abc0", WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1},
			{Name: "abc1", WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1},
			{Name: "abc2", WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1},
			{Name: "abc3", WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1},
		},
	}
	if err := f.Check(); err == nil {
		t.Error("expected error for too many TPM rules")
	}
}

func TestLimitRulesConfFileCheckTooManyRPM(t *testing.T) {
	f := &LimitRulesConfFile{
		RPM: []RPMRuleConfFile{
			{Name: "abc0", WindowMinutes: 1, MaxRequests: 100, Burst: 10},
			{Name: "abc1", WindowMinutes: 1, MaxRequests: 100, Burst: 10},
			{Name: "abc2", WindowMinutes: 1, MaxRequests: 100, Burst: 10},
			{Name: "abc3", WindowMinutes: 1, MaxRequests: 100, Burst: 10},
		},
	}
	if err := f.Check(); err == nil {
		t.Error("expected error for too many RPM rules")
	}
}

func TestLimitRulesConfFileCheckNoRules(t *testing.T) {
	f := &LimitRulesConfFile{}
	if err := f.Check(); err == nil {
		t.Error("expected error for no rules configured")
	}
}

func TestLimitRulesConfFileCheckInvalidTPM(t *testing.T) {
	f := &LimitRulesConfFile{
		TPM: []TPMRuleConfFile{{Name: "abc", WindowMinutes: 0, MaxTokens: 1000, StepMinutes: 1}},
	}
	if err := f.Check(); err == nil {
		t.Error("expected error for invalid TPM rule")
	}
}

func TestLimitRulesConfFileCheckInvalidRPM(t *testing.T) {
	f := &LimitRulesConfFile{
		RPM: []RPMRuleConfFile{{Name: "abc", WindowMinutes: 0, MaxRequests: 100, Burst: 10}},
	}
	if err := f.Check(); err == nil {
		t.Error("expected error for invalid RPM rule")
	}
}

func TestPolicyConfFileCheckValid(t *testing.T) {
	f := &PolicyConfFile{
		Name:    "test-policy",
		Enabled: true,
		Rules: &LimitRulesConfFile{
			TPM: []TPMRuleConfFile{{Name: "abc", WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1}},
		},
	}
	if err := f.Check(); err != nil {
		t.Errorf("expected no error, got: %s", err)
	}
}

func TestPolicyConfFileCheckNilRules(t *testing.T) {
	f := &PolicyConfFile{
		Name:    "test-policy",
		Enabled: true,
		Rules:   nil,
	}
	if err := f.Check(); err == nil {
		t.Error("expected error for nil Rules")
	}
}

func TestPolicyConfFileCheckInvalidRules(t *testing.T) {
	f := &PolicyConfFile{
		Name:    "test-policy",
		Enabled: true,
		Rules:   &LimitRulesConfFile{},
	}
	if err := f.Check(); err == nil {
		t.Error("expected error for invalid Rules")
	}
}

func TestRateLimitPoliciesCheckValid(t *testing.T) {
	conf := &map[string]*PolicyConfFile{
		"rlp-0001": {
			Name:    "test-policy",
			Enabled: true,
			Rules: &LimitRulesConfFile{
				TPM: []TPMRuleConfFile{{Name: "abc", WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1}},
			},
		},
	}
	if err := ratePoliciesCheck(conf); err != nil {
		t.Errorf("expected no error, got: %s", err)
	}
}

func TestRateLimitPoliciesCheckNilPolicy(t *testing.T) {
	conf := &map[string]*PolicyConfFile{
		"rlp-0001": nil,
	}
	if err := ratePoliciesCheck(conf); err == nil {
		t.Error("expected error for nil policy")
	}
}

func TestRateLimitPoliciesCheckInvalidPolicy(t *testing.T) {
	conf := &map[string]*PolicyConfFile{
		"rlp-0001": {
			Name:    "",
			Enabled: false,
			Rules:   nil,
		},
	}
	if err := ratePoliciesCheck(conf); err == nil {
		t.Error("expected error for invalid policy")
	}
}

func TestProductRulesCheckValid(t *testing.T) {
	cond := "default_t()"
	act := &action.Action{Cmd: "PASS"}
	conf := &map[string]*ProductRuleConfListFile{
		"AI_product": {{Cond: &cond, HitAction: act}},
	}
	if err := productRulesCheck(conf); err != nil {
		t.Errorf("expected no error, got: %s", err)
	}
}

func TestProductRulesCheckNilRuleList(t *testing.T) {
	conf := &map[string]*ProductRuleConfListFile{
		"AI_product": nil,
	}
	if err := productRulesCheck(conf); err == nil {
		t.Error("expected error for nil RuleList")
	}
}

func TestAiRateLimitConfCheckValid(t *testing.T) {
	version := "1.0"
	cond := "default_t()"
	act := &action.Action{Cmd: "PASS"}
	conf := &AiRateLimitConfFile{
		Version: &version,
		Config: &map[string]*ProductRuleConfListFile{
			"AI_product": {{Cond: &cond, HitAction: act}},
		},
		RateLimitPolicies: &map[string]*PolicyConfFile{
			"rlp-0001": {
				Name:    "test-policy",
				Enabled: true,
				Rules: &LimitRulesConfFile{
					TPM: []TPMRuleConfFile{{Name: "abc", WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1}},
				},
			},
		},
		ApikeyRatePoliciesBindings: &map[string][]string{},
	}
	if err := aiRateLimitConfCheck(conf); err != nil {
		t.Errorf("expected no error, got: %s", err)
	}
}

func TestAiRateLimitConfCheckNilVersion(t *testing.T) {
	cond := "default_t()"
	act := &action.Action{Cmd: "PASS"}
	conf := &AiRateLimitConfFile{
		Version: nil,
		Config: &map[string]*ProductRuleConfListFile{
			"AI_product": {{Cond: &cond, HitAction: act}},
		},
		RateLimitPolicies: &map[string]*PolicyConfFile{
			"rlp-0001": {
				Name:    "test-policy",
				Enabled: true,
				Rules: &LimitRulesConfFile{
					TPM: []TPMRuleConfFile{{Name: "abc", WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1}},
				},
			},
		},
	}
	if err := aiRateLimitConfCheck(conf); err == nil {
		t.Error("expected error for nil Version")
	}
}

func TestAiRateLimitConfCheckNilRateLimitPolicies(t *testing.T) {
	version := "1.0"
	conf := &AiRateLimitConfFile{
		Version:           &version,
		RateLimitPolicies: nil,
	}
	if err := aiRateLimitConfCheck(conf); err == nil {
		t.Error("expected error for nil RateLimitPolicies")
	}
}

func TestAiRateLimitConfCheckNilConfig(t *testing.T) {
	cond := "default_t()"
	act := &action.Action{Cmd: "PASS"}
	version := "1.0"
	conf := &AiRateLimitConfFile{
		Version: &version,
		Config: &map[string]*ProductRuleConfListFile{
			"AI_product": {{Cond: &cond, HitAction: act}},
		},
		RateLimitPolicies: &map[string]*PolicyConfFile{
			"rlp-0001": {
				Name:    "test-policy",
				Enabled: true,
				Rules: &LimitRulesConfFile{
					TPM: []TPMRuleConfFile{{Name: "abc", WindowMinutes: 1, MaxTokens: 1000, StepMinutes: 1}},
				},
			},
		},
		ApikeyRatePoliciesBindings: &map[string][]string{},
	}
	if err := aiRateLimitConfCheck(conf); err != nil {
		t.Errorf("expected no error (Config is optional), got: %s", err)
	}
}

// ==================== AiRateLimitConfLoad tests ====================

func TestAiRateLimitConfLoadValid(t *testing.T) {
	fileName := "testdata/mod_ai_rate_limit/ai_rate_limit.data"
	conf, err := AiRateLimitConfLoad(fileName)
	if err != nil {
		t.Fatalf("expected no error, got: %s", err)
	}

	if conf.Version == nil || *conf.Version != "1.0" {
		t.Errorf("Version: expected 1.0, got %v", conf.Version)
	}

	if conf.Config == nil {
		t.Fatal("Config should not be nil")
	}
	if _, ok := (*conf.Config)["AI_product"]; !ok {
		t.Error("Config should contain AI_product")
	}

	if conf.RateLimitPolicies == nil {
		t.Fatal("RateLimitPolicies should not be nil")
	}
	if _, ok := (*conf.RateLimitPolicies)["rlp-0001"]; !ok {
		t.Error("RateLimitPolicies should contain rlp-0001")
	}
	if _, ok := (*conf.RateLimitPolicies)["rlp-0002"]; !ok {
		t.Error("RateLimitPolicies should contain rlp-0002")
	}

	if conf.ApikeyRatePoliciesBindings == nil {
		t.Fatal("ApikeyRatePoliciesBindings should not be nil")
	}
	bindings := *conf.ApikeyRatePoliciesBindings
	if len(bindings["ak-2v8x9k3m7p"]) != 2 {
		t.Errorf("ak-2v8x9k3m7p should have 2 bindings, got %d", len(bindings["ak-2v8x9k3m7p"]))
	}

	rlp1 := (*conf.RateLimitPolicies)["rlp-0001"]
	if rlp1.Name != "ratelimitX" {
		t.Errorf("rlp-0001 Name: expected ratelimitX, got %s", rlp1.Name)
	}
	if !rlp1.Enabled {
		t.Error("rlp-0001 should be enabled")
	}
	if rlp1.Rules == nil {
		t.Fatal("rlp-0001 Rules should not be nil")
	}
	if len(rlp1.Rules.TPM) != 3 {
		t.Errorf("rlp-0001 TPM count: expected 3, got %d", len(rlp1.Rules.TPM))
	}
	if len(rlp1.Rules.TPM[0].Models) != 2 || rlp1.Rules.TPM[0].Models[0] != "gpt-4" || rlp1.Rules.TPM[0].Models[1] != "gpt-4o" {
		t.Errorf("rlp-0001 TPM[0] Models mismatch: got %v", rlp1.Rules.TPM[0].Models)
	}
	if len(rlp1.Rules.TPM[1].Models) != 1 || rlp1.Rules.TPM[1].Models[0] != "gpt-3.5" {
		t.Errorf("rlp-0001 TPM[1] Models mismatch: got %v", rlp1.Rules.TPM[1].Models)
	}
	if len(rlp1.Rules.TPM[2].Models) != 0 {
		t.Errorf("rlp-0001 TPM[2] Models should be empty, got %v", rlp1.Rules.TPM[2].Models)
	}
	if len(rlp1.Rules.RPM) != 1 {
		t.Errorf("rlp-0001 RPM count: expected 1, got %d", len(rlp1.Rules.RPM))
	}
	if len(rlp1.Rules.RPM[0].Models) != 1 || rlp1.Rules.RPM[0].Models[0] != "gpt-4" {
		t.Errorf("rlp-0001 RPM[0] Models mismatch: got %v", rlp1.Rules.RPM[0].Models)
	}
	if rlp1.Rules.MaxConcurrency != 50 {
		t.Errorf("rlp-0001 MaxConcurrency: expected 50, got %d", rlp1.Rules.MaxConcurrency)
	}

	rlp2 := (*conf.RateLimitPolicies)["rlp-0002"]
	if rlp2.Name != "ratelimitBasic" {
		t.Errorf("rlp-0002 Name: expected ratelimitBasic, got %s", rlp2.Name)
	}
	if rlp2.Rules.MaxConcurrency != 10 {
		t.Errorf("rlp-0002 MaxConcurrency: expected 10, got %d", rlp2.Rules.MaxConcurrency)
	}
}

func TestAiRateLimitConfLoadFileNotFound(t *testing.T) {
	fileName := "testdata/mod_ai_rate_limit/non_existent.data"
	if _, err := AiRateLimitConfLoad(fileName); err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestAiRateLimitConfLoadEmpty(t *testing.T) {
	fileName := "testdata/mod_ai_rate_limit/ai_rate_limit_empty.data"
	conf, err := AiRateLimitConfLoad(fileName)
	if err != nil {
		t.Fatalf("expected no error, got: %s", err)
	}
	if conf.Config == nil {
		t.Fatal("Config should not be nil")
	}
	if conf.RateLimitPolicies == nil {
		t.Fatal("RateLimitPolicies should not be nil")
	}
	if conf.ApikeyRatePoliciesBindings == nil {
		t.Fatal("ApikeyRatePoliciesBindings should not be nil")
	}
}
