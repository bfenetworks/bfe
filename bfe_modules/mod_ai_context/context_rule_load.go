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

package mod_ai_context

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

// context compress modes (rule entries, mode is required)
const (
	ModeOff          = "off"
	ModeConservative = "conservative"
	ModeBalanced     = "balanced"
	ModeAggressive   = "aggressive"
)

// thinking block policies (Defaults.thinkingPolicy)
const (
	ThinkingPolicyTrimAllButLast = "trim-all-but-last"
	ThinkingPolicyKeep           = "keep"
)

// rewrite strengths (Defaults.rewrite.strength)
const (
	RewriteStrengthLite = "lite"
	RewriteStrengthFull = "full"
)

// defaults for the Defaults block of context_rule.data
const (
	DefaultTriggerRatio          = 0.7
	DefaultKeepLatestImages      = 2
	DefaultToolResultMaxChars    = 2000
	DefaultThinkingPolicy        = ThinkingPolicyTrimAllButLast
	DefaultCharsPerToken         = 4
	DefaultImageTokenEstimate    = 1200
	DefaultRewriteStrength       = RewriteStrengthLite
	DefaultProtectedSurvivalRate = 0.95
)

// ==================== File representation ====================
//
// The rule file is parsed through map[string]json.RawMessage on purpose:
// unknown fields (e.g. phase-2 "override" on rule entries and "summary" in
// the Defaults block) must be ignored and counted for CTX_CFG_UNKNOWN_FIELD
// instead of rejecting the file, so a phase-2 config degrades safely on an
// old data plane (design-changes.md 4.3).

type ContextRuleConfFile struct {
	Cond *string `json:"cond"`
	Mode *string `json:"mode"`

	MaxContextTokens *int64 `json:"maxContextTokens"` // 0/absent = use model window
	ReserveTokens    *int64 `json:"reserveTokens"`    // 0/absent = auto clamp(window*15%, 256, 16000)
}

type RewriteConfFile struct {
	Strength              *string  `json:"strength"`
	ProtectedSurvivalRate *float64 `json:"protectedSurvivalRate"`
}

type DefaultsConfFile struct {
	TriggerRatio       *float64         `json:"triggerRatio"`
	KeepLatestImages   *int             `json:"keepLatestImages"`
	ToolResultMaxChars *int             `json:"toolResultMaxChars"`
	ThinkingPolicy     *string          `json:"thinkingPolicy"`
	CharsPerToken      *int             `json:"charsPerToken"`
	ImageTokenEstimate *int             `json:"imageTokenEstimate"`
	Rewrite            *RewriteConfFile `json:"rewrite"`
}

type ContextRuleConfListFile []*ContextRuleConfFile

type ContextRuleConfDataFile struct {
	Version  *string                              `json:"Version"`
	Defaults *DefaultsConfFile                    `json:"Defaults"`
	Config   *map[string]*ContextRuleConfListFile `json:"Config"`
}

// ==================== Mem types (runtime representation) ====================

// ContextParams is the per-request parameter set: the Defaults block merged
// with the rule-level budget overrides. All pipeline tuning flows through it,
// so a rule reload takes effect on the next request.
type ContextParams struct {
	TriggerRatio          float64 // proactive trigger threshold (ratio of budget)
	KeepLatestImages      int     // keep latest N inline images (0 = no image trim)
	ToolResultMaxChars    int     // max chars of a single tool result (0 = no truncate)
	ThinkingPolicy        string  // trim-all-but-last / keep
	CharsPerToken         int     // bytes per token for heuristic estimation
	ImageTokenEstimate    int     // fixed token estimate per inline base64 image
	RewriteStrength       string  // lite / full
	ProtectedSurvivalRate float64 // fidelity gate threshold for protected tokens

	MaxContextTokens int64 // rule override of the model context window (0 = use window)
	ReserveTokens    int64 // rule override of the output reserve (0 = auto)
}

func (p *ContextParams) estimateParams() EstimateParams {
	return EstimateParams{
		CharsPerToken:      p.CharsPerToken,
		ImageTokenEstimate: p.ImageTokenEstimate,
	}
}

type ContextRuleConf struct {
	Cond      string
	CondBuild condition.Condition

	Mode             string // off / conservative / balanced / aggressive
	MaxContextTokens int64
	ReserveTokens    int64
}

type ContextRuleConfList []*ContextRuleConf

type ContextDefaults struct {
	TriggerRatio          float64
	KeepLatestImages      int
	ToolResultMaxChars    int
	ThinkingPolicy        string
	CharsPerToken         int
	ImageTokenEstimate    int
	RewriteStrength       string
	ProtectedSurvivalRate float64
}

// toParams materializes the Defaults block as request parameters; rule-level
// budget overrides are applied by the caller.
func (d *ContextDefaults) toParams() ContextParams {
	return ContextParams{
		TriggerRatio:          d.TriggerRatio,
		KeepLatestImages:      d.KeepLatestImages,
		ToolResultMaxChars:    d.ToolResultMaxChars,
		ThinkingPolicy:        d.ThinkingPolicy,
		CharsPerToken:         d.CharsPerToken,
		ImageTokenEstimate:    d.ImageTokenEstimate,
		RewriteStrength:       d.RewriteStrength,
		ProtectedSurvivalRate: d.ProtectedSurvivalRate,
	}
}

type ContextRuleConfData struct {
	Version  *string
	Defaults *ContextDefaults
	Config   *map[string]*ContextRuleConfList

	// UnknownFields counts ignored unknown config fields (rule entries and
	// the Defaults block) for the CTX_CFG_UNKNOWN_FIELD forward-compat alert
	UnknownFields int
}

// ==================== Load with forward-compatible field checking ====================

// knownField marks whether key is in the known set and counts it otherwise.
func knownField(key string, known map[string]bool, unknown *int) bool {
	if known[key] {
		return true
	}
	*unknown++
	return false
}

var knownTopFields = map[string]bool{
	"Version":  true,
	"Defaults": true,
	"Config":   true,
}

var knownDefaultsFields = map[string]bool{
	"triggerRatio":       true,
	"keepLatestImages":   true,
	"toolResultMaxChars": true,
	"thinkingPolicy":     true,
	"charsPerToken":      true,
	"imageTokenEstimate": true,
	"rewrite":            true,
}

var knownRewriteFields = map[string]bool{
	"strength":              true,
	"protectedSurvivalRate": true,
}

var knownRuleFields = map[string]bool{
	"cond":             true,
	"mode":             true,
	"maxContextTokens": true,
	"reserveTokens":    true,
}

// unmarshalKnown unmarshaps raw into a struct of pointer fields; keys outside
// the known set are counted (not rejected) for the forward-compat alert.
func unmarshalKnown(raw json.RawMessage, known map[string]bool, unknown *int, out interface{}) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	for k := range m {
		knownField(k, known, unknown)
	}
	return json.Unmarshal(raw, out)
}

// parseDefaultsFile parses the Defaults block; a missing block yields all
// default values. Unknown fields are counted, known fields are validated.
func parseDefaultsFile(raw json.RawMessage, unknown *int) (*DefaultsConfFile, error) {
	d := &DefaultsConfFile{}
	if len(raw) == 0 {
		d.setDefaults()
		return d, nil
	}
	if err := unmarshalKnown(raw, knownDefaultsFields, unknown, d); err != nil {
		return nil, fmt.Errorf("Defaults: %s", err.Error())
	}
	if d.Rewrite != nil {
		rw := &RewriteConfFile{}
		if err := unmarshalKnown(rawRewrite(raw), knownRewriteFields, unknown, rw); err != nil {
			return nil, fmt.Errorf("Defaults.rewrite: %s", err.Error())
		}
		d.Rewrite = rw
	}
	d.setDefaults()
	if err := d.Check(); err != nil {
		return nil, fmt.Errorf("Defaults: %s", err.Error())
	}
	return d, nil
}

// rawRewrite extracts the raw rewrite sub-block of the Defaults block.
func rawRewrite(defaultsRaw json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(defaultsRaw, &m); err != nil {
		return nil
	}
	return m["rewrite"]
}

// ==================== Check methods (on File types) ====================

func (obj *ContextRuleConfFile) Check() error {
	if obj.Cond == nil || *obj.Cond == "" {
		return fmt.Errorf("cond is required")
	}
	if _, err := condition.Build(*obj.Cond); err != nil {
		return fmt.Errorf("cond.Build(): cond_str[%s][%s]", *obj.Cond, err.Error())
	}

	if obj.Mode == nil || *obj.Mode == "" {
		return fmt.Errorf("mode is required")
	}
	switch *obj.Mode {
	case ModeOff, ModeConservative, ModeBalanced, ModeAggressive:
	default:
		return fmt.Errorf("mode should be one of %s/%s/%s/%s",
			ModeOff, ModeConservative, ModeBalanced, ModeAggressive)
	}

	if obj.MaxContextTokens != nil && *obj.MaxContextTokens < 0 {
		return fmt.Errorf("maxContextTokens must >= 0")
	}
	if obj.ReserveTokens != nil && *obj.ReserveTokens < 0 {
		return fmt.Errorf("reserveTokens must >= 0")
	}

	return nil
}

func (obj ContextRuleConfListFile) Check() error {
	ruleMap := make(map[string]bool, 0)
	for index, rule := range obj {
		if rule == nil {
			return fmt.Errorf("rule:%d is null", index)
		}
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

func (obj *DefaultsConfFile) Check() error {
	if *obj.TriggerRatio <= 0 || *obj.TriggerRatio > 1 {
		return fmt.Errorf("triggerRatio must be in (0, 1]")
	}
	if *obj.KeepLatestImages < 0 {
		return fmt.Errorf("keepLatestImages must >= 0")
	}
	if *obj.ToolResultMaxChars < 0 {
		return fmt.Errorf("toolResultMaxChars must >= 0")
	}
	switch *obj.ThinkingPolicy {
	case ThinkingPolicyTrimAllButLast, ThinkingPolicyKeep:
	default:
		return fmt.Errorf("thinkingPolicy should be one of %s/%s",
			ThinkingPolicyTrimAllButLast, ThinkingPolicyKeep)
	}
	if *obj.CharsPerToken < 1 {
		return fmt.Errorf("charsPerToken must >= 1")
	}
	if *obj.ImageTokenEstimate < 0 {
		return fmt.Errorf("imageTokenEstimate must >= 0")
	}
	if obj.Rewrite != nil {
		if err := obj.Rewrite.Check(); err != nil {
			return fmt.Errorf("rewrite: %s", err.Error())
		}
	}
	return nil
}

func (obj *RewriteConfFile) Check() error {
	switch *obj.Strength {
	case RewriteStrengthLite, RewriteStrengthFull:
	default:
		return fmt.Errorf("strength should be one of %s/%s", RewriteStrengthLite, RewriteStrengthFull)
	}
	if *obj.ProtectedSurvivalRate <= 0 || *obj.ProtectedSurvivalRate > 1 {
		return fmt.Errorf("protectedSurvivalRate must be in (0, 1]")
	}
	return nil
}

// ==================== Defaults ====================

func (f *ContextRuleConfFile) setDefaults() {
	if f.MaxContextTokens == nil {
		v := int64(0)
		f.MaxContextTokens = &v
	}
	if f.ReserveTokens == nil {
		v := int64(0)
		f.ReserveTokens = &v
	}
}

func (f *DefaultsConfFile) setDefaults() {
	if f.TriggerRatio == nil {
		v := DefaultTriggerRatio
		f.TriggerRatio = &v
	}
	if f.KeepLatestImages == nil {
		v := DefaultKeepLatestImages
		f.KeepLatestImages = &v
	}
	if f.ToolResultMaxChars == nil {
		v := DefaultToolResultMaxChars
		f.ToolResultMaxChars = &v
	}
	if f.ThinkingPolicy == nil {
		v := DefaultThinkingPolicy
		f.ThinkingPolicy = &v
	}
	if f.CharsPerToken == nil {
		v := DefaultCharsPerToken
		f.CharsPerToken = &v
	}
	if f.ImageTokenEstimate == nil {
		v := DefaultImageTokenEstimate
		f.ImageTokenEstimate = &v
	}
	if f.Rewrite == nil {
		f.Rewrite = &RewriteConfFile{}
	}
	f.Rewrite.setDefaults()
}

func (f *RewriteConfFile) setDefaults() {
	if f.Strength == nil {
		v := DefaultRewriteStrength
		f.Strength = &v
	}
	if f.ProtectedSurvivalRate == nil {
		v := DefaultProtectedSurvivalRate
		f.ProtectedSurvivalRate = &v
	}
}

// ==================== Convert methods ====================

func (f *ContextRuleConfFile) Convert() *ContextRuleConf {
	return &ContextRuleConf{
		Cond:             *f.Cond,
		Mode:             *f.Mode,
		MaxContextTokens: *f.MaxContextTokens,
		ReserveTokens:    *f.ReserveTokens,
	}
}

func (f ContextRuleConfListFile) Convert() ContextRuleConfList {
	result := make(ContextRuleConfList, 0, len(f))
	for _, item := range f {
		result = append(result, item.Convert())
	}
	return result
}

func (f *DefaultsConfFile) Convert() *ContextDefaults {
	return &ContextDefaults{
		TriggerRatio:          *f.TriggerRatio,
		KeepLatestImages:      *f.KeepLatestImages,
		ToolResultMaxChars:    *f.ToolResultMaxChars,
		ThinkingPolicy:        *f.ThinkingPolicy,
		CharsPerToken:         *f.CharsPerToken,
		ImageTokenEstimate:    *f.ImageTokenEstimate,
		RewriteStrength:       *f.Rewrite.Strength,
		ProtectedSurvivalRate: *f.Rewrite.ProtectedSurvivalRate,
	}
}

// ==================== Load ====================

func ContextRuleConfLoad(fileName string) (*ContextRuleConfData, error) {
	content, err := os.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("ReadFile(): err[%s]", err.Error())
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(content, &top); err != nil {
		return nil, fmt.Errorf("rule file is not a json object: %s", err.Error())
	}

	data := &ContextRuleConfData{}
	unknown := 0

	// Version (required)
	versionRaw, ok := top["Version"]
	if !ok {
		return nil, fmt.Errorf("Version is not set")
	}
	var version string
	if err := json.Unmarshal(versionRaw, &version); err != nil {
		return nil, fmt.Errorf("Version: %s", err.Error())
	}
	data.Version = &version

	// top-level unknown fields are counted too (consistent forward-compat)
	for k := range top {
		knownField(k, knownTopFields, &unknown)
	}

	// Defaults (optional block, all fields optional)
	defaultsFile, err := parseDefaultsFile(top["Defaults"], &unknown)
	if err != nil {
		return nil, err
	}
	data.Defaults = defaultsFile.Convert()

	// Config (required, may be an empty map)
	configRaw, ok := top["Config"]
	if !ok {
		return nil, fmt.Errorf("Config is not set")
	}
	var configMap map[string]json.RawMessage
	if err := json.Unmarshal(configRaw, &configMap); err != nil {
		return nil, fmt.Errorf("Config: %s", err.Error())
	}
	config := make(map[string]*ContextRuleConfList, len(configMap))
	for product, listRaw := range configMap {
		var ruleRaws []json.RawMessage
		if err := json.Unmarshal(listRaw, &ruleRaws); err != nil {
			return nil, fmt.Errorf("product[%s]: %s", product, err.Error())
		}
		if ruleRaws == nil {
			return nil, fmt.Errorf("no RuleList for product:%s", product)
		}
		ruleFiles := make(ContextRuleConfListFile, 0, len(ruleRaws))
		for index, ruleRaw := range ruleRaws {
			ruleFile := &ContextRuleConfFile{}
			if err := unmarshalKnown(ruleRaw, knownRuleFields, &unknown, ruleFile); err != nil {
				return nil, fmt.Errorf("product[%s] rule:%d: %s", product, index, err.Error())
			}
			ruleFile.setDefaults()
			if err := ruleFile.Check(); err != nil {
				return nil, fmt.Errorf("product[%s] rule:%d, %s", product, index, err.Error())
			}
			ruleFiles = append(ruleFiles, ruleFile)
		}
		if err := ruleFiles.Check(); err != nil {
			return nil, fmt.Errorf("product[%s]: %s", product, err.Error())
		}
		list := ruleFiles.Convert()
		config[product] = &list
	}
	data.Config = &config
	data.UnknownFields = unknown

	return data, nil
}
