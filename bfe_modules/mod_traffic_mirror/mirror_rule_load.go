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

package mod_traffic_mirror

import (
	"fmt"

	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util"
)

// default injected mirror marker headers
const (
	DefaultMirrorHeader = "X-Bfe-Mirror"
	MirrorHeaderValue   = "true"
)

// supported body rewrite paths (phase 1: model field only, see design doc)
const (
	BodyRewritePathModel = "model"
)

// ==================== File representation ====================

type MirrorBodyRewriteConfFile struct {
	Path  *string `json:"path"`
	Value *string `json:"value"`
}

type MirrorRuleConfFile struct {
	Cond          *string `json:"cond"`          // condition expression, empty matches all
	MirrorCluster *string `json:"mirrorCluster"` // cluster name in cluster_table.data
	Percentage    *int    `json:"percentage"`    // 0-100 sampling percentage

	RemoveHeaders []string                     `json:"removeHeaders"` // sensitive headers to strip
	SetHeaders    map[string]string            `json:"setHeaders"`    // extra headers to inject
	BodyRewrites  []*MirrorBodyRewriteConfFile `json:"bodyRewrites"`  // body json field rewrites

	PathRewrite *string `json:"pathRewrite"` // optional path rewrite for mirror target
}

type MirrorRuleConfListFile []*MirrorRuleConfFile

type MirrorRuleConfDataFile struct {
	Version *string                             `json:"Version"`
	Config  *map[string]*MirrorRuleConfListFile `json:"Config"`
}

// ==================== Mem types (runtime representation) ====================

type MirrorBodyRewriteConf struct {
	Path  string
	Value string
}

type MirrorRuleConf struct {
	Cond          string
	CondBuild     condition.Condition
	MirrorCluster string
	Percentage    int

	RemoveHeaders []string
	SetHeaders    map[string]string
	BodyRewrites  []*MirrorBodyRewriteConf

	PathRewrite string // empty means no rewrite
}

type MirrorRuleConfList []*MirrorRuleConf

type MirrorRuleConfData struct {
	Version *string
	Config  *map[string]*MirrorRuleConfList
}

// ==================== Convert methods ====================

func (f *MirrorBodyRewriteConfFile) Convert() *MirrorBodyRewriteConf {
	return &MirrorBodyRewriteConf{
		Path:  *f.Path,
		Value: *f.Value,
	}
}

func (f *MirrorRuleConfFile) Convert() *MirrorRuleConf {
	rule := &MirrorRuleConf{
		Cond:          *f.Cond,
		MirrorCluster: *f.MirrorCluster,
		Percentage:    *f.Percentage,
		RemoveHeaders: f.RemoveHeaders,
		SetHeaders:    f.SetHeaders,
		PathRewrite:   "",
	}

	for _, rw := range f.BodyRewrites {
		rule.BodyRewrites = append(rule.BodyRewrites, rw.Convert())
	}

	if f.PathRewrite != nil {
		rule.PathRewrite = *f.PathRewrite
	}

	return rule
}

func (f MirrorRuleConfListFile) Convert() MirrorRuleConfList {
	result := make(MirrorRuleConfList, 0, len(f))
	for _, item := range f {
		result = append(result, item.Convert())
	}
	return result
}

func (f *MirrorRuleConfDataFile) Convert() *MirrorRuleConfData {
	result := &MirrorRuleConfData{
		Version: f.Version,
	}

	if f.Config != nil {
		config := make(map[string]*MirrorRuleConfList)
		for k, v := range *f.Config {
			list := v.Convert()
			config[k] = &list
		}
		result.Config = &config
	}

	return result
}

// ==================== Check methods (on File types) ====================

func (obj *MirrorBodyRewriteConfFile) Check() error {
	if err := bfe_util.CheckNilField(*obj, false); err != nil {
		return err
	}
	if *obj.Path != BodyRewritePathModel {
		return fmt.Errorf("bodyRewrites path only supports \"%s\" in phase 1, got[%s]",
			BodyRewritePathModel, *obj.Path)
	}
	return nil
}

func (obj *MirrorRuleConfFile) Check() error {
	// map/slice fields are optional; only pointer fields are required
	if err := bfe_util.CheckNilField(*obj, true); err != nil {
		return err
	}

	if *obj.MirrorCluster == "" {
		return fmt.Errorf("mirrorCluster is empty")
	}

	if *obj.Percentage < 0 || *obj.Percentage > 100 {
		return fmt.Errorf("percentage must be in [0, 100], got[%d]", *obj.Percentage)
	}

	// empty cond matches everything; otherwise it must be buildable
	if *obj.Cond != "" {
		if _, err := condition.Build(*obj.Cond); err != nil {
			return fmt.Errorf("cond.Build(): cond_str[%s][%s]", *obj.Cond, err.Error())
		}
	}

	for _, rw := range obj.BodyRewrites {
		if err := rw.Check(); err != nil {
			return err
		}
	}

	return nil
}

func (obj MirrorRuleConfListFile) Check() error {
	ruleMap := make(map[string]bool, 0)
	for index, rule := range obj {
		if rule == nil {
			return fmt.Errorf("rule:%d is nil", index)
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

func mirrorRulesCheck(conf *map[string]*MirrorRuleConfListFile) error {
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

func mirrorRuleConfDataCheck(conf *MirrorRuleConfDataFile) error {
	if err := bfe_util.CheckNilField(*conf, true); err != nil {
		return err
	}

	if conf.Config != nil {
		if err := mirrorRulesCheck(conf.Config); err != nil {
			return fmt.Errorf("Config: %s", err.Error())
		}
	}

	return nil
}

// setDefaults fills default values for optional rule fields before check.
func (f *MirrorRuleConfFile) setDefaults() {
	if f.Cond == nil {
		v := ""
		f.Cond = &v
	}
	if f.Percentage == nil {
		v := 100
		f.Percentage = &v
	}
	if f.RemoveHeaders == nil {
		f.RemoveHeaders = []string{}
	}
	if f.SetHeaders == nil {
		f.SetHeaders = map[string]string{}
	}
	if f.BodyRewrites == nil {
		f.BodyRewrites = []*MirrorBodyRewriteConfFile{}
	}
	if f.PathRewrite == nil {
		v := ""
		f.PathRewrite = &v
	}
}

func MirrorRuleConfLoad(fileName string) (*MirrorRuleConfData, error) {
	var config MirrorRuleConfDataFile

	if err := bfe_util.LoadJsonFile(fileName, &config); err != nil {
		return nil, fmt.Errorf("LoadJsonFile(): err[%s]", err.Error())
	}

	// fill defaults for optional fields
	if config.Config != nil {
		for _, ruleList := range *config.Config {
			for _, rule := range *ruleList {
				rule.setDefaults()
			}
		}
	}

	if err := mirrorRuleConfDataCheck(&config); err != nil {
		return nil, err
	}

	return config.Convert(), nil
}
