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

package mod_redirect

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedirectRuleCheckNoCond(t *testing.T) {
	cmd := "URL_SET"
	rule := RedirectRuleFile{
		Actions: &ActionFileList{{Cmd: &cmd, Params: []string{"http://a.com"}}},
		Status:  intPtr(302),
	}
	if err := redirectRuleCheck(rule); err == nil || !strings.Contains(err.Error(), "no Cond") {
		t.Errorf("redirectRuleCheck() should return no Cond error, got %v", err)
	}
}

func TestRedirectRuleCheckNoActions(t *testing.T) {
	rule := RedirectRuleFile{
		Cond:   strPtr("req_host_in(\"www.example.org\")"),
		Status: intPtr(302),
	}
	if err := redirectRuleCheck(rule); err == nil || !strings.Contains(err.Error(), "no Actions") {
		t.Errorf("redirectRuleCheck() should return no Actions error, got %v", err)
	}
}

func TestRedirectRuleCheckEmptyActions(t *testing.T) {
	emptyActions := ActionFileList{}
	rule := RedirectRuleFile{
		Cond:    strPtr("req_host_in(\"www.example.org\")"),
		Actions: &emptyActions,
		Status:  intPtr(302),
	}
	if err := redirectRuleCheck(rule); err == nil || !strings.Contains(err.Error(), "no Actions") {
		t.Errorf("redirectRuleCheck() should return no Actions error for empty actions, got %v", err)
	}
}

func TestRedirectRuleCheckInvalidAction(t *testing.T) {
	cmd := "INVALID"
	rule := RedirectRuleFile{
		Cond:    strPtr("req_host_in(\"www.example.org\")"),
		Actions: &ActionFileList{{Cmd: &cmd, Params: []string{"value"}}},
		Status:  intPtr(302),
	}
	if err := redirectRuleCheck(rule); err == nil {
		t.Errorf("redirectRuleCheck() should return error for invalid action")
	}
}

func TestRedirectRuleCheckNoStatus(t *testing.T) {
	cmd := "URL_SET"
	rule := RedirectRuleFile{
		Cond:    strPtr("req_host_in(\"www.example.org\")"),
		Actions: &ActionFileList{{Cmd: &cmd, Params: []string{"http://a.com"}}},
	}
	if err := redirectRuleCheck(rule); err == nil || !strings.Contains(err.Error(), "Status") {
		t.Errorf("redirectRuleCheck() should return Status error, got %v", err)
	}
}

func TestRedirectRuleCheckStatusZero(t *testing.T) {
	cmd := "URL_SET"
	rule := RedirectRuleFile{
		Cond:    strPtr("req_host_in(\"www.example.org\")"),
		Actions: &ActionFileList{{Cmd: &cmd, Params: []string{"http://a.com"}}},
		Status:  intPtr(0),
	}
	if err := redirectRuleCheck(rule); err == nil || !strings.Contains(err.Error(), "Status") {
		t.Errorf("redirectRuleCheck() should return Status error for zero status, got %v", err)
	}
}

func TestProductRulesCheckNilRuleList(t *testing.T) {
	config := ProductRulesFile{"pn": nil}
	if err := ProductRulesCheck(&config); err == nil || !strings.Contains(err.Error(), "no RuleList") {
		t.Errorf("ProductRulesCheck() should return no RuleList error, got %v", err)
	}
}

func TestRedirectConfCheckNoVersion(t *testing.T) {
	cmd := "URL_SET"
	config := RedirectConfFile{
		Config: &ProductRulesFile{
			"pn": &RuleFileList{{
				Cond:    strPtr("req_host_in(\"www.example.org\")"),
				Actions: &ActionFileList{{Cmd: &cmd, Params: []string{"http://a.com"}}},
				Status:  intPtr(302),
			}},
		},
	}
	if err := RedirectConfCheck(config); err == nil || !strings.Contains(err.Error(), "no Version") {
		t.Errorf("RedirectConfCheck() should return no Version error, got %v", err)
	}
}

func TestRedirectConfCheckNoConfig(t *testing.T) {
	config := RedirectConfFile{Version: strPtr("1")}
	if err := RedirectConfCheck(config); err == nil || !strings.Contains(err.Error(), "no Config") {
		t.Errorf("RedirectConfCheck() should return no Config error, got %v", err)
	}
}

func TestRuleConvertInvalidCond(t *testing.T) {
	cmd := "URL_SET"
	ruleFile := RedirectRuleFile{
		Cond:    strPtr("not_a_valid_condition"),
		Actions: &ActionFileList{{Cmd: &cmd, Params: []string{"http://a.com"}}},
		Status:  intPtr(302),
	}
	if _, err := ruleConvert(ruleFile); err == nil {
		t.Errorf("ruleConvert() should return error for invalid condition")
	}
}

func TestRuleListConvertInvalidCond(t *testing.T) {
	cmd := "URL_SET"
	validCond := strPtr("req_host_in(\"www.example.org\")")
	invalidCond := strPtr("not_a_valid_condition")
	list := RuleFileList{
		{Cond: validCond, Actions: &ActionFileList{{Cmd: &cmd, Params: []string{"http://a.com"}}}, Status: intPtr(302)},
		{Cond: invalidCond, Actions: &ActionFileList{{Cmd: &cmd, Params: []string{"http://b.com"}}}, Status: intPtr(301)},
	}
	if _, err := ruleListConvert(&list); err == nil {
		t.Errorf("ruleListConvert() should return error for invalid condition")
	}
}

func TestRedirectConfLoadFileNotExist(t *testing.T) {
	_, err := redirectConfLoad("./testdata/not_exist.conf")
	if err == nil {
		t.Errorf("redirectConfLoad() should return error when file not exist")
	}
}

func TestRedirectConfLoadInvalidJSON(t *testing.T) {
	dir := "./testdata_tmp_invalid_json"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "redirect.conf")
	if err := ioutil.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if _, err := redirectConfLoad(path); err == nil {
		t.Errorf("redirectConfLoad() should return error for invalid JSON")
	}
}

func TestRedirectConfLoadVersionMissing(t *testing.T) {
	dir := "./testdata_tmp_no_version"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	defer os.RemoveAll(dir)

	content := `{"Config":{"pn":[]}}`
	path := filepath.Join(dir, "redirect.conf")
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if _, err := redirectConfLoad(path); err == nil || !strings.Contains(err.Error(), "no Version") {
		t.Errorf("redirectConfLoad() should return no Version error, got %v", err)
	}
}

func TestRedirectConfLoadInvalidCond(t *testing.T) {
	dir := "./testdata_tmp_invalid_cond"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	defer os.RemoveAll(dir)

	content := `{"Version":"1","Config":{"pn":[{"cond":"not_valid","actions":[{"cmd":"URL_SET","params":["http://a.com"]}],"status":302}]}}`
	path := filepath.Join(dir, "redirect.conf")
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if _, err := redirectConfLoad(path); err == nil {
		t.Errorf("redirectConfLoad() should return error for invalid condition")
	}
}

func TestRedirectConfLoadConfigMissing(t *testing.T) {
	dir := "./testdata_tmp_no_config"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	defer os.RemoveAll(dir)

	content := `{"Version":"1"}`
	path := filepath.Join(dir, "redirect.conf")
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if _, err := redirectConfLoad(path); err == nil || !strings.Contains(err.Error(), "no Config") {
		t.Errorf("redirectConfLoad() should return no Config error, got %v", err)
	}
}
