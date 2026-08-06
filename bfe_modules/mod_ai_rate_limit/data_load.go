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
	"fmt"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_basic/action"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util"
)

// ==================== File representation ====================

type ProductRuleConfFile struct {
	Cond      *string        `json:"cond"`
	HitAction *action.Action `json:"hit_action"`
}

type ProductRuleConfListFile []*ProductRuleConfFile

type TPMRuleConfFile struct {
	Name          string   `json:"name"`
	WindowMinutes int      `json:"window_minutes"`
	MaxTokens     int64    `json:"max_tokens"`
	StepMinutes   int      `json:"step_minutes"`
	Models        []string `json:"models"`
}

type RPMRuleConfFile struct {
	Name          string   `json:"name"`
	WindowMinutes int      `json:"window_minutes"`
	MaxRequests   int64    `json:"max_requests"`
	Burst         int64    `json:"burst"`
	Models        []string `json:"models"`
}

type LimitRulesConfFile struct {
	TPM            []TPMRuleConfFile `json:"tpm"`
	RPM            []RPMRuleConfFile `json:"rpm"`
	MaxConcurrency *int64            `json:"max_concurrency"`
}

type PolicyConfFile struct {
	Name    string              `json:"name"`
	Enabled bool                `json:"enabled"`
	Models  []string            `json:"models"`
	Rules   *LimitRulesConfFile `json:"rules"`
}

type AiRateLimitConfFile struct {
	Version                    *string                              `json:"Version"`
	Config                     *map[string]*ProductRuleConfListFile `json:"Config"`
	RateLimitPolicies          *map[string]*PolicyConfFile          `json:"RateLimitPolicies"`
	ApikeyRatePoliciesBindings *map[string][]string                 `json:"ApikeyRateLimitPolicyBindings"`
}

// ==================== Mem types (runtime representation) ====================

type ProductRuleConf struct {
	Cond      string
	HitAction *action.Action
}

type ProductRuleConfList []*ProductRuleConf

type TPMRuleConf struct {
	Name             string
	TimeWindow       int64   // TPMRuleConfFile.WindowMinutes * 60
	Threshold        int64   // TPMRuleConfFile.MaxTokens
	BucketTimeWindow int64   // 60s
	BucketThreshold  int64   // TPMRuleConfFile.MaxTokens
	ReservedX        float64 // 0
	ReservedOff      float64 // 0
	Models           []string
}

type RPMRuleConf struct {
	Name        string
	TimeWindow  int   // RPMRuleConfFile.WindowMinutes * 60
	MaxRequests int64 // RPMRuleConfFile.MaxRequests
	Burst       int64 // RPMRuleConfFile.Burst
	Models      []string
}

type LimitRulesConf struct {
	TPM            []*TPMRuleConf
	RPM            []*RPMRuleConf
	MaxConcurrency *int64
}

type PolicyConf struct {
	Name    string
	Enabled bool
	Models  []string
	Rules   *LimitRulesConf
}

type AiRateLimitConf struct {
	Version                    *string
	Config                     *map[string]*ProductRuleConfList
	RateLimitPolicies          *map[string]*PolicyConf
	ApikeyRatePoliciesBindings *map[string][]string
}

// ==================== Convert methods ====================

func (f *ProductRuleConfFile) Convert() *ProductRuleConf {
	return &ProductRuleConf{
		Cond:      *f.Cond,
		HitAction: f.HitAction,
	}
}

func (f ProductRuleConfListFile) Convert() ProductRuleConfList {
	result := make(ProductRuleConfList, 0, len(f))
	for _, item := range f {
		result = append(result, item.Convert())
	}
	return result
}

func (f *TPMRuleConfFile) Convert() *TPMRuleConf {
	bucketTimeWindow := int64(60)
	bucketThreshold := f.MaxTokens
	return &TPMRuleConf{
		Name:             f.Name,
		TimeWindow:       int64(f.WindowMinutes) * 60,
		Threshold:        f.MaxTokens,
		BucketTimeWindow: bucketTimeWindow,
		BucketThreshold:  bucketThreshold,
		ReservedX:        0,
		ReservedOff:      0,
		Models:           f.Models,
	}
}

func (f *RPMRuleConfFile) Convert() *RPMRuleConf {
	return &RPMRuleConf{
		Name:        f.Name,
		TimeWindow:  f.WindowMinutes * 60,
		MaxRequests: f.MaxRequests,
		Burst:       f.Burst,
		Models:      f.Models,
	}
}

func (f *LimitRulesConfFile) Convert() *LimitRulesConf {
	result := &LimitRulesConf{
		MaxConcurrency: f.MaxConcurrency,
	}

	for _, tpm := range f.TPM {
		result.TPM = append(result.TPM, tpm.Convert())
	}
	for _, rpm := range f.RPM {
		result.RPM = append(result.RPM, rpm.Convert())
	}
	return result
}

func (f *PolicyConfFile) Convert() *PolicyConf {

	result := &PolicyConf{
		Name:    f.Name,
		Enabled: f.Enabled,
		Models:  f.Models,
	}
	if f.Rules != nil {
		result.Rules = f.Rules.Convert()
	}
	return result
}

func (f *AiRateLimitConfFile) Convert() *AiRateLimitConf {
	result := &AiRateLimitConf{
		Version:                    f.Version,
		ApikeyRatePoliciesBindings: f.ApikeyRatePoliciesBindings,
	}

	if f.Config != nil {
		config := make(map[string]*ProductRuleConfList)
		for k, v := range *f.Config {
			list := v.Convert()
			config[k] = &list
		}
		result.Config = &config
	}

	if f.RateLimitPolicies != nil {
		policies := make(map[string]*PolicyConf)
		for k, v := range *f.RateLimitPolicies {
			policies[k] = v.Convert()
		}
		result.RateLimitPolicies = &policies
	}

	return result
}

// ==================== Check methods (on File types) ====================

func (obj *ProductRuleConfFile) Check() error {
	if err := bfe_util.CheckNilField(*obj, false); err != nil {
		return err
	}

	if _, err := condition.Build(*obj.Cond); err != nil {
		return fmt.Errorf("cond.Build(): cond_str[%s][%s]", *obj.Cond, err.Error())
	}

	if obj.HitAction == nil || len(obj.HitAction.Cmd) <= 0 {
		return fmt.Errorf("Action.Cmd is nil or empty")
	}

	if err := obj.HitAction.Check(allowReqActions); err != nil {
		return fmt.Errorf("Action: %s", err.Error())
	}

	return nil
}

func (obj *ProductRuleConfListFile) Check() error {
	ruleMap := make(map[string]bool, 0)
	for index, rule := range *obj {
		if err := rule.Check(); err != nil {
			return fmt.Errorf("rule:%d, %s", index, err.Error())
		}

		if _, ok := ruleMap[*rule.Cond]; ok {
			return fmt.Errorf("can't have same cond[%s]", *rule.Cond)
		}
		ruleMap[*rule.Cond] = true
	}

	return nil
}

func (obj *TPMRuleConfFile) Check() error {
	if obj.WindowMinutes <= 0 || obj.WindowMinutes > 360 {
		return fmt.Errorf("window_minutes should be in [1, 360]")
	}
	if len(obj.Name) <= 0 {
		return fmt.Errorf("Please set name")
	}

	if obj.StepMinutes <= 0 {
		obj.StepMinutes = 1
	}
	if obj.StepMinutes > obj.WindowMinutes {
		return fmt.Errorf("step_minutes should <= window_minutes")
	}

	return nil
}

func (obj *RPMRuleConfFile) Check() error {
	if obj.WindowMinutes <= 0 || obj.WindowMinutes > 360 {
		return fmt.Errorf("window_minutes should be in [1, 360]")
	}

	if len(obj.Name) <= 0 {
		return fmt.Errorf("Please set name")
	}

	if obj.Burst < 1 {
		log.Logger.Warn("obj.Burst:%d, set to 1", obj.Burst)
		obj.Burst = 1
	}

	return nil
}

func (obj *LimitRulesConfFile) Check() error {
	hasRule := false

	if obj.TPM != nil {
		if len(obj.TPM) > 3 {
			return fmt.Errorf("tpm rules should <= 3")
		}
		for i := range obj.TPM {
			if err := obj.TPM[i].Check(); err != nil {
				return fmt.Errorf("tpm[%d]: %s", i, err.Error())
			}
		}
		if len(obj.TPM) > 0 {
			hasRule = true
		}

		tpmNames := make(map[string]bool)
		for i := range obj.TPM {
			name := obj.TPM[i].Name
			if name == "" {
				continue
			}
			if tpmNames[name] {
				return fmt.Errorf("tpm[%d]: duplicate name %q", i, name)
			}
			tpmNames[name] = true
		}
	}

	if obj.RPM != nil {
		if len(obj.RPM) > 3 {
			return fmt.Errorf("rpm rules should <= 3")
		}
		for i := range obj.RPM {
			if err := obj.RPM[i].Check(); err != nil {
				return fmt.Errorf("rpm[%d]: %s", i, err.Error())
			}
		}
		if len(obj.RPM) > 0 {
			hasRule = true
		}
		rpmNames := make(map[string]bool)
		for i := range obj.RPM {
			name := obj.RPM[i].Name
			if name == "" {
				continue
			}
			if rpmNames[name] {
				return fmt.Errorf("rpm[%d]: duplicate name %q", i, name)
			}
			rpmNames[name] = true
		}
	}

	if obj.MaxConcurrency != nil {
		hasRule = true
	}

	if !hasRule {
		return fmt.Errorf("at least one of tpm/rpm/max_concurrency should be configured")
	}

	return nil
}

func (obj *PolicyConfFile) Check() error {
	if err := bfe_util.CheckNilField(*obj, true); err != nil {
		return err
	}

	if obj.Rules == nil {
		return fmt.Errorf("Rules is nil")
	}

	if err := obj.Rules.Check(); err != nil {
		return fmt.Errorf("Rules: %s", err.Error())
	}

	return nil
}

func ratePoliciesCheck(conf *map[string]*PolicyConfFile) error {
	for policyId, policy := range *conf {
		if len(policyId) <= 0 {
			return fmt.Errorf("policyId is empty")
		}
		if policy == nil {
			return fmt.Errorf("policy[%s] is nil", policyId)
		}
		if err := policy.Check(); err != nil {
			return fmt.Errorf("policy[%s]: %s", policyId, err.Error())
		}
	}
	return nil
}

func productRulesCheck(conf *map[string]*ProductRuleConfListFile) error {
	for product, ruleList := range *conf {
		if ruleList == nil {
			return fmt.Errorf("no RuleList for product:%s", product)
		}
		if err := ruleList.Check(); err != nil {
			return fmt.Errorf("product[%s]: %s", product, err.Error())
		}
	}
	return nil
}

func aiRateLimitConfCheck(conf *AiRateLimitConfFile) error {
	if err := bfe_util.CheckNilField(*conf, true); err != nil {
		return err
	}

	if conf.Config != nil {
		if err := productRulesCheck(conf.Config); err != nil {
			return fmt.Errorf("Config: %s", err.Error())
		}
	}

	if conf.RateLimitPolicies == nil {
		return fmt.Errorf("RateLimitPolicies is nil")
	}
	if err := ratePoliciesCheck(conf.RateLimitPolicies); err != nil {
		return fmt.Errorf("RateLimitPolicies: %s", err.Error())
	}

	if conf.ApikeyRatePoliciesBindings != nil {
		for key, policies := range *conf.ApikeyRatePoliciesBindings {
			if len(key) <= 0 {
				return fmt.Errorf("key is empty in ApikeyRatePoliciesBindings")
			}
			if policies == nil {
				return fmt.Errorf("policies in ApikeyRatePoliciesBindings is nil for key:%s", key)
			}
		}
	}

	return nil
}

func AiRateLimitConfLoad(fileName string) (*AiRateLimitConf, error) {
	var config AiRateLimitConfFile

	if err := bfe_util.LoadJsonFile(fileName, &config); err != nil {
		return nil, fmt.Errorf("LoadJsonFile(): err[%s]", err.Error())
	}

	if err := aiRateLimitConfCheck(&config); err != nil {
		return nil, err
	}

	return config.Convert(), nil
}
