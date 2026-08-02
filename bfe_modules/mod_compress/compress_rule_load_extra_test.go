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

package mod_compress

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

func TestCompressRuleCheckErrors(t *testing.T) {
	cases := []struct {
		name    string
		rule    compressRuleFile
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil cond",
			rule:    compressRuleFile{Cond: nil, Action: &ActionFile{Cmd: strPtr(ActionGzip), Quality: intPtr(5), FlushSize: intPtr(128)}},
			wantErr: true,
			errMsg:  "no Cond",
		},
		{
			name:    "nil action",
			rule:    compressRuleFile{Cond: strPtr("default_t()"), Action: nil},
			wantErr: true,
			errMsg:  "no Action",
		},
		{
			name:    "invalid action",
			rule:    compressRuleFile{Cond: strPtr("default_t()"), Action: &ActionFile{Cmd: strPtr("BAD"), Quality: intPtr(5), FlushSize: intPtr(128)}},
			wantErr: true,
			errMsg:  "invalid cmd",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := compressRuleCheck(tc.rule)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCompressRuleListCheckError(t *testing.T) {
	list := compressRuleFileList{
		{Cond: strPtr("default_t()"), Action: &ActionFile{Cmd: strPtr(ActionGzip), Quality: intPtr(5), FlushSize: intPtr(128)}},
		{Cond: strPtr("default_t()"), Action: &ActionFile{Cmd: strPtr("BAD"), Quality: intPtr(5), FlushSize: intPtr(128)}},
	}

	err := compressRuleListCheck(&list)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "compressRule: 1") {
		t.Fatalf("expected error to contain rule index, got %q", err.Error())
	}
}

func TestProductRulesCheckNilList(t *testing.T) {
	rules := ProductRulesFile{"unittest": nil}
	err := productRulesCheck(&rules)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "no compressRuleList for product: unittest") {
		t.Fatalf("unexpected error: %q", err.Error())
	}
}

func TestProductRuleConfCheckErrors(t *testing.T) {
	cases := []struct {
		name    string
		conf    productRuleConfFile
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil version",
			conf:    productRuleConfFile{Version: nil, Config: &ProductRulesFile{}},
			wantErr: true,
			errMsg:  "no Version",
		},
		{
			name:    "nil config",
			conf:    productRuleConfFile{Version: strPtr("v1"), Config: nil},
			wantErr: true,
			errMsg:  "no Config",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := productRuleConfCheck(tc.conf)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRuleConvertConditionError(t *testing.T) {
	ruleFile := compressRuleFile{Cond: strPtr("invalid_cond()"), Action: &ActionFile{Cmd: strPtr(ActionGzip), Quality: intPtr(5), FlushSize: intPtr(128)}}
	_, err := ruleConvert(ruleFile)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestProductRuleConfLoadErrors(t *testing.T) {
	t.Run("file not exist", func(t *testing.T) {
		_, err := ProductRuleConfLoad("./testdata/mod_compress/not_exist.data")
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "invalid.json")
		if err := ioutil.WriteFile(path, []byte("{ bad json"), 0644); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}
		_, err := ProductRuleConfLoad(path)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalid rule", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "invalid_rule.data")
		content := `{
    "Config": {
        "unittest": [
            {
                "Cond": "default_t()"
            }
        ]
    },
    "Version": "unittest"
}`
		if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}
		_, err := ProductRuleConfLoad(path)
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.Contains(err.Error(), "no Action") {
			t.Fatalf("unexpected error: %q", err.Error())
		}
	})

	t.Run("invalid condition", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "invalid_cond.data")
		content := `{
    "Config": {
        "unittest": [
            {
                "Cond": "default_t()",
                "Action": {
                    "Cmd": "GZIP",
                    "Quality": 5,
                    "FlushSize": 128
                }
            },
            {
                "Cond": "bad_cond()",
                "Action": {
                    "Cmd": "GZIP",
                    "Quality": 5,
                    "FlushSize": 128
                }
            }
        ]
    },
    "Version": "unittest"
}`
		if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}
		_, err := ProductRuleConfLoad(path)
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.Contains(err.Error(), "bad_cond") {
			t.Fatalf("unexpected error: %q", err.Error())
		}
	})
}

func TestCompressRuleTableUpdateAndSearch(t *testing.T) {
	table := NewCompressRuleTable()
	cond, err := condition.Build("default_t()")
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	rules := compressRuleList{{Cond: cond, Action: Action{Cmd: ActionGzip, Quality: 5, FlushSize: 128}}}
	table.Update(productRuleConf{Version: "v1", Config: ProductRules{"unittest": &rules}})

	got, ok := table.Search("unittest")
	if !ok {
		t.Fatalf("expected to find rules for unittest")
	}
	if len(*got) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(*got))
	}

	_, ok = table.Search("not_exist")
	if ok {
		t.Fatalf("expected no rules for not_exist")
	}
}
