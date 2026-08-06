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

package mod_errors

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestErrorsConfLoadSuccess(t *testing.T) {
	conf, err := ErrorsConfLoad("./testdata/mod_errors/errors_rule.data")
	if err != nil {
		t.Fatalf("ErrorsConfLoad() error: %v", err)
	}

	if conf.Version != "SCMPF_MODULE_VERSION" {
		t.Errorf("Version should be SCMPF_MODULE_VERSION, not %s", conf.Version)
	}

	rules, ok := conf.Config["example"]
	if !ok {
		t.Fatalf("Config should contain product example")
	}
	if len(*rules) != 2 {
		t.Errorf("product example should have 2 rules, not %d", len(*rules))
	}

	if (*rules)[0].Actions[0].Cmd != RETURN {
		t.Errorf("first action cmd should be %s", RETURN)
	}
	if (*rules)[1].Actions[0].Cmd != REDIRECT {
		t.Errorf("second action cmd should be %s", REDIRECT)
	}
}

func TestErrorsConfLoadFileNotExist(t *testing.T) {
	_, err := ErrorsConfLoad("./testdata/mod_errors/not_exist.data")
	if err == nil {
		t.Errorf("ErrorsConfLoad() should return error for missing file")
	}
}

func TestErrorsConfLoadInvalidJSON(t *testing.T) {
	_, err := ErrorsConfLoad("./testdata/mod_errors/errors_rule_invalid_json.data")
	if err == nil {
		t.Errorf("ErrorsConfLoad() should return error for invalid json")
	}
}

func TestErrorsConfLoadNoVersion(t *testing.T) {
	_, err := ErrorsConfLoad("./testdata/mod_errors/errors_rule_no_version.data")
	if err == nil {
		t.Errorf("ErrorsConfLoad() should return error when version is missing")
	}
}

func TestErrorsConfLoadNoConfig(t *testing.T) {
	_, err := ErrorsConfLoad("./testdata/mod_errors/errors_rule_no_config.data")
	if err == nil {
		t.Errorf("ErrorsConfLoad() should return error when config is missing")
	}
}

func TestErrorsConfLoadNoCond(t *testing.T) {
	_, err := ErrorsConfLoad("./testdata/mod_errors/errors_rule_no_cond.data")
	if err == nil {
		t.Errorf("ErrorsConfLoad() should return error when cond is missing")
	}
}

func TestErrorsConfLoadNoActions(t *testing.T) {
	_, err := ErrorsConfLoad("./testdata/mod_errors/errors_rule_no_actions.data")
	if err == nil {
		t.Errorf("ErrorsConfLoad() should return error when actions is missing")
	}
}

func TestErrorsConfLoadInvalidCmd(t *testing.T) {
	_, err := ErrorsConfLoad("./testdata/mod_errors/errors_rule_invalid_cmd.data")
	if err == nil {
		t.Errorf("ErrorsConfLoad() should return error for invalid command")
	}
}

func TestErrorsConfLoadInvalidStatus(t *testing.T) {
	_, err := ErrorsConfLoad("./testdata/mod_errors/errors_rule_invalid_status.data")
	if err == nil {
		t.Errorf("ErrorsConfLoad() should return error for invalid status code")
	}
}

func TestErrorsConfLoadInvalidRedirect(t *testing.T) {
	_, err := ErrorsConfLoad("./testdata/mod_errors/errors_rule_invalid_redirect.data")
	if err == nil {
		t.Errorf("ErrorsConfLoad() should return error for invalid redirect")
	}
}

func TestActionFileCheck(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "mod_errors_large_*.html")
	if err != nil {
		t.Fatalf("TempFile() error: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	largeData := make([]byte, MaxPageSize+1)
	if _, err := tmpFile.Write(largeData); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	tmpFile.Close()

	tests := []struct {
		name    string
		cmd     string
		params  []string
		wantErr bool
	}{
		{
			name:    "valid RETURN",
			cmd:     RETURN,
			params:  []string{"200", "text/html", "./testdata/mod_errors/test.html"},
			wantErr: false,
		},
		{
			name:    "valid REDIRECT",
			cmd:     REDIRECT,
			params:  []string{"http://example.com/error.html"},
			wantErr: false,
		},
		{
			name:    "nil cmd",
			cmd:     "",
			params:  []string{"200", "text/html", "./testdata/mod_errors/test.html"},
			wantErr: true,
		},
		{
			name:    "invalid cmd",
			cmd:     "UNKNOWN",
			params:  []string{"200", "text/html", "./testdata/mod_errors/test.html"},
			wantErr: true,
		},
		{
			name:    "RETURN params num not 3",
			cmd:     RETURN,
			params:  []string{"200", "text/html"},
			wantErr: true,
		},
		{
			name:    "RETURN invalid status code string",
			cmd:     RETURN,
			params:  []string{"abc", "text/html", "./testdata/mod_errors/test.html"},
			wantErr: true,
		},
		{
			name:    "RETURN invalid status code value",
			cmd:     RETURN,
			params:  []string{"999", "text/html", "./testdata/mod_errors/test.html"},
			wantErr: true,
		},
		{
			name:    "RETURN status code not 2XX/4XX/5XX",
			cmd:     RETURN,
			params:  []string{"301", "text/html", "./testdata/mod_errors/test.html"},
			wantErr: true,
		},
		{
			name:    "RETURN file too large",
			cmd:     RETURN,
			params:  []string{"200", "text/html", tmpFile.Name()},
			wantErr: true,
		},
		{
			name:    "REDIRECT params num not 1",
			cmd:     REDIRECT,
			params:  []string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd
			actionFile := ActionFile{}
			if cmd != "" {
				actionFile.Cmd = &cmd
			}
			actionFile.Params = tt.params

			err := ActionFileCheck(actionFile)
			if tt.wantErr && err == nil {
				t.Errorf("ActionFileCheck() should return error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ActionFileCheck() error: %v", err)
			}
		})
	}
}

func TestActionFileListCheck(t *testing.T) {
	cmd := RETURN
	validAction := ActionFile{
		Cmd:    &cmd,
		Params: []string{"200", "text/html", "./testdata/mod_errors/test.html"},
	}

	emptyList := ActionFileList{}
	if err := ActionFileListCheck(&emptyList); err == nil {
		t.Errorf("ActionFileListCheck() should return error for empty list")
	}

	validList := ActionFileList{validAction}
	if err := ActionFileListCheck(&validList); err != nil {
		t.Errorf("ActionFileListCheck() error: %v", err)
	}

	invalidList := ActionFileList{validAction, validAction}
	if err := ActionFileListCheck(&invalidList); err == nil {
		t.Errorf("ActionFileListCheck() should return error for list with 2 actions")
	}
}

func TestErrorsRuleCheck(t *testing.T) {
	cmd := RETURN
	cond := "res_code_in(\"404\")"
	validActions := &ActionFileList{
		ActionFile{
			Cmd:    &cmd,
			Params: []string{"200", "text/html", "./testdata/mod_errors/test.html"},
		},
	}

	if err := ErrorsRuleCheck(ErrorsRuleFile{Cond: nil, Actions: validActions}); err == nil {
		t.Errorf("ErrorsRuleCheck() should return error when Cond is nil")
	}

	if err := ErrorsRuleCheck(ErrorsRuleFile{Cond: &cond, Actions: nil}); err == nil {
		t.Errorf("ErrorsRuleCheck() should return error when Actions is nil")
	}

	if err := ErrorsRuleCheck(ErrorsRuleFile{Cond: &cond, Actions: validActions}); err != nil {
		t.Errorf("ErrorsRuleCheck() error: %v", err)
	}
}

func TestRuleListCheck(t *testing.T) {
	cmd := RETURN
	cond := "res_code_in(\"404\")"
	validRule := ErrorsRuleFile{
		Cond: &cond,
		Actions: &ActionFileList{
			ActionFile{
				Cmd:    &cmd,
				Params: []string{"200", "text/html", "./testdata/mod_errors/test.html"},
			},
		},
	}

	emptyList := RuleFileList{}
	if err := RuleListCheck(&emptyList); err != nil {
		t.Errorf("RuleListCheck() error: %v", err)
	}

	validList := RuleFileList{validRule}
	if err := RuleListCheck(&validList); err != nil {
		t.Errorf("RuleListCheck() error: %v", err)
	}

	invalidList := RuleFileList{ErrorsRuleFile{Cond: nil, Actions: nil}}
	if err := RuleListCheck(&invalidList); err == nil {
		t.Errorf("RuleListCheck() should return error for invalid rule")
	}
}

func TestProductRulesCheck(t *testing.T) {
	cmd := RETURN
	cond := "res_code_in(\"404\")"
	ruleList := RuleFileList{
		ErrorsRuleFile{
			Cond: &cond,
			Actions: &ActionFileList{
				ActionFile{
					Cmd:    &cmd,
					Params: []string{"200", "text/html", "./testdata/mod_errors/test.html"},
				},
			},
		},
	}

	productRules := ProductRulesFile{
		"example": &ruleList,
	}
	if err := ProductRulesCheck(&productRules); err != nil {
		t.Errorf("ProductRulesCheck() error: %v", err)
	}

	productRulesNil := ProductRulesFile{
		"example": nil,
	}
	if err := ProductRulesCheck(&productRulesNil); err == nil {
		t.Errorf("ProductRulesCheck() should return error when rule list is nil")
	}
}

func TestErrorsConfCheck(t *testing.T) {
	cmd := RETURN
	cond := "res_code_in(\"404\")"
	ruleList := RuleFileList{
		ErrorsRuleFile{
			Cond: &cond,
			Actions: &ActionFileList{
				ActionFile{
					Cmd:    &cmd,
					Params: []string{"200", "text/html", "./testdata/mod_errors/test.html"},
				},
			},
		},
	}
	version := "v1"
	config := ProductRulesFile{"example": &ruleList}

	if err := ErrorsConfCheck(ErrorsConfFile{Version: &version, Config: &config}); err != nil {
		t.Errorf("ErrorsConfCheck() error: %v", err)
	}

	if err := ErrorsConfCheck(ErrorsConfFile{Version: nil, Config: &config}); err == nil {
		t.Errorf("ErrorsConfCheck() should return error when Version is nil")
	}

	if err := ErrorsConfCheck(ErrorsConfFile{Version: &version, Config: nil}); err == nil {
		t.Errorf("ErrorsConfCheck() should return error when Config is nil")
	}
}
