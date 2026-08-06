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

package mod_header

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHeaderConfCheckMissingVersion(t *testing.T) {
	conf := HeaderConfFile{Config: &ProductRulesFile{}}
	if err := HeaderConfCheck(conf); err == nil || !strings.Contains(err.Error(), "no Version") {
		t.Errorf("expected no Version error, got %v", err)
	}
}

func TestHeaderConfCheckMissingConfig(t *testing.T) {
	version := "v1"
	conf := HeaderConfFile{Version: &version}
	if err := HeaderConfCheck(conf); err == nil || !strings.Contains(err.Error(), "no Config") {
		t.Errorf("expected no Config error, got %v", err)
	}
}

func TestProductRulesCheckNilRuleList(t *testing.T) {
	version := "v1"
	config := ProductRulesFile{
		"p1": nil,
	}
	conf := HeaderConfFile{Version: &version, Config: &config}
	if err := HeaderConfCheck(conf); err == nil || !strings.Contains(err.Error(), "no RuleList") {
		t.Errorf("expected no RuleList error, got %v", err)
	}
}

func TestRuleListCheckError(t *testing.T) {
	cond := "default_t()"
	last := true
	list := RuleFileList{
		{Cond: &cond, Actions: &ActionFileList{}, Last: &last},
	}
	if err := RuleListCheck(&list); err == nil {
		t.Error("RuleListCheck should fail for empty actions")
	}
}

func TestRuleConvertInvalidCondition(t *testing.T) {
	cond := "invalid_condition()"
	last := true
	cmd := "REQ_HEADER_SET"
	action := ActionFile{Cmd: &cmd, Params: []string{"k", "v"}}
	ruleFile := HeaderRuleFile{
		Cond:    &cond,
		Actions: &ActionFileList{action},
		Last:    &last,
	}
	if _, err := ruleConvert(ruleFile); err == nil {
		t.Error("ruleConvert should fail for invalid condition")
	}
}

func TestRuleConvertNilCond(t *testing.T) {
	last := true
	cmd := "REQ_HEADER_SET"
	action := ActionFile{Cmd: &cmd, Params: []string{"k", "v"}}
	ruleFile := HeaderRuleFile{
		Cond:    nil,
		Actions: &ActionFileList{action},
		Last:    &last,
	}
	if _, err := ruleConvert(ruleFile); err == nil {
		t.Error("ruleConvert should fail for nil condition")
	}
}

func TestHeaderConfLoadInvalidJSON(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_header_test")
	if err != nil {
		t.Fatalf("TempDir error: %v", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "invalid.json")
	if err := ioutil.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	if _, err := HeaderConfLoad(path); err == nil {
		t.Error("HeaderConfLoad should fail for invalid JSON")
	}
}

func TestHeaderConfLoadMissingVersion(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_header_test")
	if err != nil {
		t.Fatalf("TempDir error: %v", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "missing_version.json")
	content := `{"Config": {"p1": []}}`
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	if _, err := HeaderConfLoad(path); err == nil || !strings.Contains(err.Error(), "no Version") {
		t.Errorf("expected no Version error, got %v", err)
	}
}

func TestHeaderConfLoadInvalidAction(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_header_test")
	if err != nil {
		t.Fatalf("TempDir error: %v", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "invalid_action.json")
	content := `{
		"Version": "v1",
		"Config": {
			"p1": [
				{
					"cond": "default_t()",
					"actions": [{"cmd": "INVALID", "params": ["k", "v"]}],
					"last": true
				}
			]
		}
	}`
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	if _, err := HeaderConfLoad(path); err == nil {
		t.Error("HeaderConfLoad should fail for invalid action")
	}
}

func TestHeaderConfLoadClassifyRules(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_header_test")
	if err != nil {
		t.Fatalf("TempDir error: %v", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "classify.json")
	content := `{
		"Version": "v1",
		"Config": {
			"p1": [
				{
					"cond": "default_t()",
					"actions": [
						{"cmd": "REQ_HEADER_SET", "params": ["X-Req", "1"]},
						{"cmd": "RSP_HEADER_SET", "params": ["X-Rsp", "2"]}
					],
					"last": true
				}
			]
		}
	}`
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	conf, err := HeaderConfLoad(path)
	if err != nil {
		t.Fatalf("HeaderConfLoad error: %v", err)
	}

	if conf.Version != "v1" {
		t.Errorf("version = %s, want v1", conf.Version)
	}

	ruleLists := conf.Config["p1"]
	if ruleLists == nil {
		t.Fatal("no rules for p1")
	}
	if len(*ruleLists[ReqHeader]) != 1 || len(*ruleLists[RspHeader]) != 1 {
		t.Errorf("unexpected rule list lengths: req=%d rsp=%d",
			len(*ruleLists[ReqHeader]), len(*ruleLists[RspHeader]))
	}
}

func TestGetHeaderType(t *testing.T) {
	if got := getHeaderType("REQ_HEADER_SET"); got != ReqHeader {
		t.Errorf("getHeaderType(REQ_) = %d, want %d", got, ReqHeader)
	}
	if got := getHeaderType("RSP_HEADER_SET"); got != RspHeader {
		t.Errorf("getHeaderType(RSP_) = %d, want %d", got, RspHeader)
	}
}
