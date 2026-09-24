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
	CacheStatusHit  = "hit"
	CacheStatusMiss = "miss"
	CacheStatusSkip = "skip"
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
}

type ProductRuleConfListFile []*ProductRuleConfFile

type ProductRuleConfDataFile struct {
	Version *string                              `json:"Version"`
	Config  *map[string]*ProductRuleConfListFile `json:"Config"`
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
}

type ProductRuleConfList []*ProductRuleConf

type ProductRuleConfData struct {
	Version *string
	Config  *map[string]*ProductRuleConfList
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
	}

	if rule.CacheKeyStrategy == CacheKeyStrategyDisabled {
		rule.Disabled = true
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

// ==================== Check methods (on File types) ====================

func (obj *ProductRuleConfFile) Check() error {
	if err := bfe_util.CheckNilField(*obj, false); err != nil {
		return err
	}

	if _, err := condition.Build(*obj.Cond); err != nil {
		return fmt.Errorf("cond.Build(): cond_str[%s][%s]", *obj.Cond, err.Error())
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
	if err := bfe_util.CheckNilField(*conf, true); err != nil {
		return err
	}

	if conf.Config != nil {
		if err := productRulesCheck(conf.Config); err != nil {
			return fmt.Errorf("Config: %s", err.Error())
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
}

func ProductRuleConfLoad(fileName string, defaultCacheTTL int) (*ProductRuleConfData, error) {
	var config ProductRuleConfDataFile

	if err := bfe_util.LoadJsonFile(fileName, &config); err != nil {
		return nil, fmt.Errorf("LoadJsonFile(): err[%s]", err.Error())
	}

	// fill defaults for optional fields
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
