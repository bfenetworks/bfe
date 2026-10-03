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

package mod_ai_cache

import (
	"fmt"
	"strings"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util"
)

// cache key strategies
const (
	CacheKeyStrategyLastQuestion = "lastQuestion"
	CacheKeyStrategyAllQuestions = "allQuestions"
	CacheKeyStrategyDisabled     = "disabled"
)

// cache status values recorded in AiBasicInfo and the access log
const (
	CacheStatusHit         = "hit"
	CacheStatusHitSemantic = "hit_semantic"
	CacheStatusMiss        = "miss"
	CacheStatusSkip        = "skip"
)

// semantic threshold relations
const (
	SemanticRelationLt  = "lt"
	SemanticRelationLte = "lte"
	SemanticRelationGt  = "gt"
	SemanticRelationGte = "gte"
)

// semantic global conf defaults (top-level Semantic block of the rule file)
const (
	DefaultSemanticTopK              = 1
	DefaultSemanticThreshold         = 0.15
	DefaultSemanticThresholdRelation = SemanticRelationLt
)

// default GJSON paths for key/value extraction
const (
	DefaultLastQuestionPath   = "messages.@reverse.0.content"
	DefaultCacheValuePath     = "choices.0.message.content"
	DefaultCacheStreamPath    = "choices.0.delta.content"
	DefaultResponseTemplate   = `{"id":"from-cache","object":"chat.completion","created":0,"choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"finish_reason":"stop"}],"usage":null}`
	DefaultStreamResponseTmpl = "data:{\"id\":\"from-cache\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"%s\"}}]}\n\ndata:[DONE]\n\n"
)

// ==================== File representation ====================

type ProductRuleConfFile struct {
	Cond *string `json:"cond"`

	CacheKeyStrategy *string `json:"cacheKeyStrategy"` // lastQuestion / allQuestions / disabled

	CacheTTL *int `json:"cacheTTL"` // cache TTL (seconds)

	// GJSON paths
	CacheKeyFrom       *string `json:"cacheKeyFrom"`         // override key extraction path
	CacheValueFrom     *string `json:"cacheValueFrom"`       // non-streaming answer path
	CacheStreamFrom    *string `json:"cacheStreamValueFrom"` // streaming answer path
	CacheToolCallsFrom *string `json:"cacheToolCallsFrom"`   // reserved for tool calls

	// hit response templates, %s is the placeholder of cached content
	ResponseTemplate       *string `json:"responseTemplate"`
	StreamResponseTemplate *string `json:"streamResponseTemplate"`

	// size limits
	MaxBodyBytes  *int64 `json:"maxBodyBytes"`
	MaxValueBytes *int64 `json:"maxValueBytes"`

	// whether to enable the semantic cache for this rule; ignored when
	// cacheKeyStrategy is "disabled"
	EnableSemanticCache *bool `json:"enableSemanticCache"`
}

// SemanticConfFile is the file representation of the top-level Semantic
// block: module-level global config for the semantic cache, hot reloaded
// together with the rule table.
type SemanticConfFile struct {
	TopK              *int     `json:"topK"`              // number of nearest neighbors
	Threshold         *float64 `json:"threshold"`         // score threshold
	ThresholdRelation *string  `json:"thresholdRelation"` // lt / lte / gt / gte
}

type ProductRuleConfListFile []*ProductRuleConfFile

type ProductRuleConfDataFile struct {
	Version  *string                              `json:"Version"`
	Semantic *SemanticConfFile                    `json:"Semantic"` // optional, global
	Config   *map[string]*ProductRuleConfListFile `json:"Config"`
}

// ==================== Mem types (runtime representation) ====================

type ProductRuleConf struct {
	Cond      string
	CondBuild condition.Condition

	CacheKeyStrategy string
	Disabled         bool

	CacheTTL int

	CacheKeyFrom    string
	CacheValueFrom  string
	CacheStreamFrom string

	ResponseTemplate       string
	StreamResponseTemplate string

	MaxBodyBytes  int64
	MaxValueBytes int64

	EnableSemanticCache bool
}

// SemanticConf is the runtime representation of the top-level Semantic
// block of the rule file (module-level global semantic config).
type SemanticConf struct {
	TopK              int     // number of nearest neighbors
	Threshold         float64 // score threshold
	ThresholdRelation string  // lt / lte / gt / gte
}

type ProductRuleConfList []*ProductRuleConf

type ProductRuleConfData struct {
	Version  *string
	Semantic *SemanticConf // nil when the rule file has no Semantic block
	Config   *map[string]*ProductRuleConfList
}

// ==================== Convert methods ====================

func (f *ProductRuleConfFile) Convert() *ProductRuleConf {
	rule := &ProductRuleConf{
		Cond:                   *f.Cond,
		CacheKeyStrategy:       *f.CacheKeyStrategy,
		CacheTTL:               *f.CacheTTL,
		CacheKeyFrom:           *f.CacheKeyFrom,
		CacheValueFrom:         *f.CacheValueFrom,
		CacheStreamFrom:        *f.CacheStreamFrom,
		ResponseTemplate:       *f.ResponseTemplate,
		StreamResponseTemplate: *f.StreamResponseTemplate,
		MaxBodyBytes:           *f.MaxBodyBytes,
		MaxValueBytes:          *f.MaxValueBytes,
		EnableSemanticCache:    *f.EnableSemanticCache,
	}

	if rule.CacheKeyStrategy == CacheKeyStrategyDisabled {
		rule.Disabled = true
		// enableSemanticCache is ignored on disabled rules (disabled wins)
		rule.EnableSemanticCache = false
	}

	return rule
}

func (f ProductRuleConfListFile) Convert() ProductRuleConfList {
	result := make(ProductRuleConfList, 0, len(f))
	for _, item := range f {
		result = append(result, item.Convert())
	}
	return result
}

func (f *ProductRuleConfDataFile) Convert() *ProductRuleConfData {
	result := &ProductRuleConfData{
		Version: f.Version,
	}

	if f.Semantic != nil {
		result.Semantic = f.Semantic.Convert()
	}

	if f.Config != nil {
		config := make(map[string]*ProductRuleConfList)
		for k, v := range *f.Config {
			list := v.Convert()
			config[k] = &list
		}
		result.Config = &config
	}

	return result
}

// Convert converts the file representation to the runtime conf. Callers must
// have applied setDefaults first so that no pointer field is nil.
func (f *SemanticConfFile) Convert() *SemanticConf {
	return &SemanticConf{
		TopK:              *f.TopK,
		Threshold:         *f.Threshold,
		ThresholdRelation: *f.ThresholdRelation,
	}
}

// ==================== Check methods (on File types) ====================

func (obj *ProductRuleConfFile) Check() error {
	if err := bfe_util.CheckNilField(*obj, false); err != nil {
		return err
	}

	if _, err := condition.Build(*obj.Cond); err != nil {
		return fmt.Errorf("cond.Build(): cond_str[%s][%s]", *obj.Cond, err.Error())
	}

	// A cond referencing req_ai_intent_in triggers intent resolve during
	// cache lookup (the primitive lazily classifies on first evaluation),
	// which negates the lookup-early benefit. Warn but accept, consistent
	// with mod_ai_route's tolerant treatment of question names.
	if strings.Contains(*obj.Cond, "req_ai_intent_in") {
		log.Logger.Warn("mod_ai_cache: rule cond references req_ai_intent_in, "+
			"intent resolve would run during cache lookup and negate the "+
			"lookup-early benefit; cond[%s]", *obj.Cond)
	}

	switch *obj.CacheKeyStrategy {
	case CacheKeyStrategyLastQuestion, CacheKeyStrategyAllQuestions, CacheKeyStrategyDisabled:
	default:
		return fmt.Errorf("cacheKeyStrategy should be one of %s/%s/%s",
			CacheKeyStrategyLastQuestion, CacheKeyStrategyAllQuestions, CacheKeyStrategyDisabled)
	}

	if *obj.CacheTTL < 0 {
		return fmt.Errorf("cacheTTL must >= 0")
	}

	if *obj.MaxBodyBytes <= 0 || *obj.MaxValueBytes <= 0 {
		return fmt.Errorf("maxBodyBytes/maxValueBytes must > 0")
	}

	if !strings.Contains(*obj.ResponseTemplate, "%s") {
		return fmt.Errorf("responseTemplate must contain %%s placeholder")
	}

	if !strings.Contains(*obj.StreamResponseTemplate, "%s") {
		return fmt.Errorf("streamResponseTemplate must contain %%s placeholder")
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

func productRuleConfDataCheck(conf *ProductRuleConfDataFile) error {
	if conf.Version == nil {
		return fmt.Errorf("Version is not set")
	}
	if conf.Config == nil {
		return fmt.Errorf("Config is not set")
	}

	if err := productRulesCheck(conf.Config); err != nil {
		return fmt.Errorf("Config: %s", err.Error())
	}

	if conf.Semantic != nil {
		if err := conf.Semantic.Check(); err != nil {
			return fmt.Errorf("Semantic: %s", err.Error())
		}
	}

	return nil
}

// setDefaults fills default values for optional rule fields before check.
func (f *ProductRuleConfFile) setDefaults(defaultCacheTTL int) {
	if f.CacheKeyStrategy == nil {
		v := CacheKeyStrategyLastQuestion
		f.CacheKeyStrategy = &v
	}
	if f.CacheTTL == nil {
		v := defaultCacheTTL
		f.CacheTTL = &v
	}
	if f.CacheKeyFrom == nil {
		v := ""
		f.CacheKeyFrom = &v
	}
	if f.CacheValueFrom == nil {
		v := DefaultCacheValuePath
		f.CacheValueFrom = &v
	}
	if f.CacheStreamFrom == nil {
		v := DefaultCacheStreamPath
		f.CacheStreamFrom = &v
	}
	if f.CacheToolCallsFrom == nil {
		v := ""
		f.CacheToolCallsFrom = &v
	}
	if f.ResponseTemplate == nil {
		v := DefaultResponseTemplate
		f.ResponseTemplate = &v
	}
	if f.StreamResponseTemplate == nil {
		v := DefaultStreamResponseTmpl
		f.StreamResponseTemplate = &v
	}
	if f.MaxBodyBytes == nil {
		v := int64(DefaultMaxBodyBytes)
		f.MaxBodyBytes = &v
	}
	if f.MaxValueBytes == nil {
		v := int64(DefaultMaxValueBytes)
		f.MaxValueBytes = &v
	}
	if f.EnableSemanticCache == nil {
		v := false
		f.EnableSemanticCache = &v
	}
}

// setDefaults fills default values for the optional global semantic conf.
func (f *SemanticConfFile) setDefaults() {
	if f.TopK == nil {
		v := DefaultSemanticTopK
		f.TopK = &v
	}
	if f.Threshold == nil {
		v := DefaultSemanticThreshold
		f.Threshold = &v
	}
	if f.ThresholdRelation == nil {
		v := DefaultSemanticThresholdRelation
		f.ThresholdRelation = &v
	}
}

// Check validates the global semantic conf (top-level Semantic block).
func (f *SemanticConfFile) Check() error {
	if *f.TopK < 1 || *f.TopK > 10 {
		return fmt.Errorf("topK must be in 1-10")
	}
	if *f.Threshold < 0 || *f.Threshold > 2 {
		return fmt.Errorf("threshold must be in 0-2")
	}
	switch *f.ThresholdRelation {
	case SemanticRelationLt, SemanticRelationLte, SemanticRelationGt, SemanticRelationGte:
	default:
		return fmt.Errorf("thresholdRelation should be one of %s/%s/%s/%s",
			SemanticRelationLt, SemanticRelationLte, SemanticRelationGt, SemanticRelationGte)
	}
	return nil
}

func ProductRuleConfLoad(fileName string, defaultCacheTTL int) (*ProductRuleConfData, error) {
	var config ProductRuleConfDataFile

	if err := bfe_util.LoadJsonFile(fileName, &config); err != nil {
		return nil, fmt.Errorf("LoadJsonFile(): err[%s]", err.Error())
	}

	// fill defaults for optional fields
	if config.Semantic != nil {
		config.Semantic.setDefaults()
	}
	if config.Config != nil {
		for _, ruleList := range *config.Config {
			for _, rule := range *ruleList {
				rule.setDefaults(defaultCacheTTL)
			}
		}
	}

	if err := productRuleConfDataCheck(&config); err != nil {
		return nil, err
	}

	return config.Convert(), nil
}
