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

package mod_body_process

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestBodyProcessConfigCheck(t *testing.T) {
	if err := BodyProcessConfigCheck(nil); err != nil {
		t.Errorf("nil config should pass: %s", err)
	}

	cfg := &BodyProcessConfig{Dec: "line", Enc: "json"}
	if err := BodyProcessConfigCheck(cfg); err != nil {
		t.Errorf("valid config failed: %s", err)
	}

	cfg = &BodyProcessConfig{Dec: "invalid"}
	if err := BodyProcessConfigCheck(cfg); err == nil {
		t.Error("invalid Dec should fail")
	}

	cfg = &BodyProcessConfig{Enc: "invalid"}
	if err := BodyProcessConfigCheck(cfg); err == nil {
		t.Error("invalid Enc should fail")
	}

	cfg = &BodyProcessConfig{Proc: []ProcConf{{Name: "invalid"}}}
	if err := BodyProcessConfigCheck(cfg); err == nil {
		t.Error("invalid Proc name should fail")
	}

	cfg = &BodyProcessConfig{Proc: []ProcConf{{Name: "textfilter", Params: []string{}}}}
	if err := BodyProcessConfigCheck(cfg); err == nil {
		t.Error("textfilter without params should fail")
	}
}

func TestProcessRuleCheck(t *testing.T) {
	cond := "default_t()"
	rule := processRuleFile{Cond: &cond}
	if err := processRuleCheck(rule); err != nil {
		t.Errorf("valid rule failed: %s", err)
	}

	rule = processRuleFile{}
	if err := processRuleCheck(rule); err == nil {
		t.Error("rule without Cond should fail")
	}
}

func TestProcessRuleListCheck(t *testing.T) {
	cond := "default_t()"
	list := processRuleFileList{{Cond: &cond}}
	if err := processRuleListCheck(list); err != nil {
		t.Errorf("valid list failed: %s", err)
	}
}

func TestProductRulesCheck(t *testing.T) {
	cond := "default_t()"
	rules := ProductRulesFile{
		"product": {{Cond: &cond}},
	}
	if err := productRulesCheck(&rules); err != nil {
		t.Errorf("valid rules failed: %s", err)
	}

	badRules := ProductRulesFile{
		"product": nil,
	}
	if err := productRulesCheck(&badRules); err == nil {
		t.Error("nil rule list should fail")
	}
}

func TestProductRuleConfCheck(t *testing.T) {
	version := "1.0"
	cond := "default_t()"
	conf := productRuleConfFile{
		Version: &version,
		Config: &ProductRulesFile{
			"product": {{Cond: &cond}},
		},
	}
	if err := productRuleConfCheck(conf); err != nil {
		t.Errorf("valid conf failed: %s", err)
	}

	noVersion := conf
	noVersion.Version = nil
	if err := productRuleConfCheck(noVersion); err == nil {
		t.Error("missing Version should fail")
	}

	noConfig := conf
	noConfig.Config = nil
	if err := productRuleConfCheck(noConfig); err == nil {
		t.Error("missing Config should fail")
	}
}

func TestRuleConvert(t *testing.T) {
	cond := "default_t()"
	ruleFile := processRuleFile{Cond: &cond}
	rule, err := ruleConvert(ruleFile)
	if err != nil {
		t.Fatalf("ruleConvert failed: %s", err)
	}
	if rule.Cond == nil {
		t.Error("expected condition to be built")
	}
}

func TestRuleConvertInvalidCond(t *testing.T) {
	cond := "invalid_cond()"
	ruleFile := processRuleFile{Cond: &cond}
	_, err := ruleConvert(ruleFile)
	if err == nil {
		t.Error("expected error for invalid condition")
	}
}

func TestRuleListConvert(t *testing.T) {
	cond := "default_t()"
	list := processRuleFileList{{Cond: &cond}}
	rules, err := ruleListConvert(list)
	if err != nil {
		t.Fatalf("ruleListConvert failed: %s", err)
	}
	if len(rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(rules))
	}
}

func TestProductRuleConfLoad(t *testing.T) {
	conf, err := ProductRuleConfLoad("./testdata/mod_body_process/body_process.data")
	if err != nil {
		t.Fatalf("ProductRuleConfLoad failed: %s", err)
	}
	if conf.Version != "1.0" {
		t.Errorf("expected version 1.0, got %s", conf.Version)
	}
	if _, ok := conf.Config["AI_product"]; !ok {
		t.Error("expected AI_product in config")
	}
}

func TestProductRuleConfLoadInvalid(t *testing.T) {
	_, err := ProductRuleConfLoad("./testdata/mod_body_process/body_process_invalid.data")
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestProductRuleConfLoadNotExist(t *testing.T) {
	_, err := ProductRuleConfLoad("./testdata/mod_body_process/not_exist.data")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestProductRuleConfLoadBadJson(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_body_process")
	if err != nil {
		t.Fatalf("TempDir failed: %s", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "bad.json")
	if err := ioutil.WriteFile(path, []byte("{invalid"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %s", err)
	}

	_, err = ProductRuleConfLoad(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
