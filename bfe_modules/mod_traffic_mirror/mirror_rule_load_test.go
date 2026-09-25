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
	"testing"
)

func TestMirrorRuleConfLoad(t *testing.T) {
	conf, err := MirrorRuleConfLoad("testdata/mod_traffic_mirror/mirror_rule.data")
	if err != nil {
		t.Fatalf("MirrorRuleConfLoad failed: %v", err)
	}

	if conf.Version == nil || *conf.Version != "1.0" {
		t.Errorf("Version = %v, want 1.0", conf.Version)
	}

	if conf.Config == nil {
		t.Fatal("Config should not be nil")
	}

	rules, ok := (*conf.Config)["default"]
	if !ok || rules == nil {
		t.Fatal("default product rules not found")
	}
	if len(*rules) != 2 {
		t.Fatalf("default product should have 2 rules, got %d", len(*rules))
	}

	rule := (*rules)[0]
	if rule.MirrorCluster != "cluster_shadow_v2" {
		t.Errorf("MirrorCluster = %s", rule.MirrorCluster)
	}
	if rule.Percentage != 100 {
		t.Errorf("Percentage = %d, want 100", rule.Percentage)
	}
	if rule.Cond == "" {
		t.Error("Cond should not be empty for first rule")
	}
	if len(rule.RemoveHeaders) != 2 {
		t.Errorf("RemoveHeaders len = %d, want 2", len(rule.RemoveHeaders))
	}
	if rule.SetHeaders["X-Env"] != "shadow" {
		t.Errorf("SetHeaders[X-Env] = %s", rule.SetHeaders["X-Env"])
	}
	if len(rule.BodyRewrites) != 1 {
		t.Fatalf("BodyRewrites len = %d, want 1", len(rule.BodyRewrites))
	}
	if rule.BodyRewrites[0].Path != "model" || rule.BodyRewrites[0].Value != "deepseek-v3" {
		t.Errorf("BodyRewrites[0] = %+v", rule.BodyRewrites[0])
	}

	// second rule: empty cond matches all, default percentage applied
	rule2 := (*rules)[1]
	if rule2.Cond != "" {
		t.Errorf("rule2 Cond = %s, want empty", rule2.Cond)
	}
	if rule2.Percentage != 0 {
		t.Errorf("rule2 Percentage = %d, want 0", rule2.Percentage)
	}
	if rule2.PathRewrite != "" {
		t.Errorf("rule2 PathRewrite = %s, want empty", rule2.PathRewrite)
	}
}

func TestMirrorRuleConfLoadInvalid(t *testing.T) {
	if _, err := MirrorRuleConfLoad("testdata/mod_traffic_mirror/mirror_rule_invalid.data"); err == nil {
		t.Error("load should fail for percentage > 100")
	}
}

func TestMirrorRuleConfLoadNotFound(t *testing.T) {
	if _, err := MirrorRuleConfLoad("testdata/mod_traffic_mirror/not_exist.data"); err == nil {
		t.Error("load should fail for missing file")
	}
}
