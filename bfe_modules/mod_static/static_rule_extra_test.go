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

package mod_static

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func prepareRuleFile(t *testing.T, content string) string {
	t.Helper()
	dir, err := os.MkdirTemp("./testdata", "mod_static_rule_")
	if err != nil {
		t.Fatalf("MkdirTemp() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "static_rule.data")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	return filepath.ToSlash(path)
}

func rootForRuleTests(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("./testdata", "mod_static_rule_root_")
	if err != nil {
		t.Fatalf("MkdirTemp() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	staticDir := filepath.Join(dir, "static")
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return filepath.ToSlash(staticDir)
}

func TestStaticConfLoadFileNotExist(t *testing.T) {
	if _, err := StaticConfLoad("./testdata/mod_static/static_rule.data.not_exist"); err == nil {
		t.Errorf("StaticConfLoad() should return error when file does not exist")
	}
}

func TestStaticConfLoadInvalidJSON(t *testing.T) {
	path := prepareRuleFile(t, "not a json")
	if _, err := StaticConfLoad(path); err == nil {
		t.Errorf("StaticConfLoad() should return error for invalid JSON")
	}
}

func TestStaticConfLoadMissingVersion(t *testing.T) {
	root := rootForRuleTests(t)
	path := prepareRuleFile(t, fmt.Sprintf(`{
    "Config": {
        "unittest": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [%q, "index.html"]
                }
            }
        ]
    }
}`, root))
	if _, err := StaticConfLoad(path); err == nil {
		t.Errorf("StaticConfLoad() should return error when Version is missing")
	}
}

func TestStaticConfLoadMissingConfig(t *testing.T) {
	path := prepareRuleFile(t, `{"Version": "unittest"}`)
	if _, err := StaticConfLoad(path); err == nil {
		t.Errorf("StaticConfLoad() should return error when Config is missing")
	}
}

func TestStaticConfLoadNilRuleList(t *testing.T) {
	path := prepareRuleFile(t, `{
    "Config": {
        "unittest": null
    },
    "Version": "unittest"
}`)
	if _, err := StaticConfLoad(path); err == nil {
		t.Errorf("StaticConfLoad() should return error when product rule list is nil")
	}
}

func TestStaticConfLoadEmptyCond(t *testing.T) {
	root := rootForRuleTests(t)
	path := prepareRuleFile(t, fmt.Sprintf(`{
    "Config": {
        "unittest": [
            {
                "Cond": "",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [%q, "index.html"]
                }
            }
        ]
    },
    "Version": "unittest"
}`, root))
	if _, err := StaticConfLoad(path); err == nil {
		t.Errorf("StaticConfLoad() should return error when Cond is empty")
	}
}

func TestStaticConfLoadInvalidActionCmd(t *testing.T) {
	root := rootForRuleTests(t)
	path := prepareRuleFile(t, fmt.Sprintf(`{
    "Config": {
        "unittest": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "Action": {
                    "Cmd": "INDEX",
                    "Params": [%q, "index.html"]
                }
            }
        ]
    },
    "Version": "unittest"
}`, root))
	if _, err := StaticConfLoad(path); err == nil {
		t.Errorf("StaticConfLoad() should return error for invalid action cmd")
	}
}

func TestStaticConfLoadInvalidParamsLength(t *testing.T) {
	root := rootForRuleTests(t)
	path := prepareRuleFile(t, fmt.Sprintf(`{
    "Config": {
        "unittest": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [%q]
                }
            }
        ]
    },
    "Version": "unittest"
}`, root))
	if _, err := StaticConfLoad(path); err == nil {
		t.Errorf("StaticConfLoad() should return error for invalid params length")
	}
}

func TestStaticConfLoadDirNotExist(t *testing.T) {
	path := prepareRuleFile(t, `{
    "Config": {
        "unittest": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": ["./testdata/mod_static/not_exist_dir", "index.html"]
                }
            }
        ]
    },
    "Version": "unittest"
}`)
	if _, err := StaticConfLoad(path); err == nil {
		t.Errorf("StaticConfLoad() should return error when directory does not exist")
	}
}

func TestStaticConfLoadDefaultFileNotExist(t *testing.T) {
	root := rootForRuleTests(t)
	path := prepareRuleFile(t, fmt.Sprintf(`{
    "Config": {
        "unittest": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [%q, "missing.html"]
                }
            }
        ]
    },
    "Version": "unittest"
}`, root))
	if _, err := StaticConfLoad(path); err == nil {
		t.Errorf("StaticConfLoad() should return error when default file does not exist")
	}
}

func TestStaticConfLoadCondBuildError(t *testing.T) {
	root := rootForRuleTests(t)
	path := prepareRuleFile(t, fmt.Sprintf(`{
    "Config": {
        "unittest": [
            {
                "Cond": "req_host_in(\"www.example.org\") && invalid_func()",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [%q, "index.html"]
                }
            }
        ]
    },
    "Version": "unittest"
}`, root))
	if _, err := StaticConfLoad(path); err == nil {
		t.Errorf("StaticConfLoad() should return error when condition build fails")
	}
}

func TestStaticRuleCheckNoAction(t *testing.T) {
	root := rootForRuleTests(t)
	err := StaticRuleCheck(StaticRuleFile{
		Cond:   "req_host_in(\"www.example.org\")",
		Action: nil,
	})
	if err == nil {
		t.Errorf("StaticRuleCheck() should return error when Action is nil")
	}

	browse := ActionBrowse
	err = StaticRuleCheck(StaticRuleFile{
		Cond: "",
		Action: &ActionFile{
			Cmd:    &browse,
			Params: []string{root, "index.html"},
		},
	})
	if err == nil {
		t.Errorf("StaticRuleCheck() should return error when Cond is empty")
	}
}

func TestStaticRuleTableUpdateAndSearch(t *testing.T) {
	table := NewStaticRuleTable()
	if table == nil {
		t.Fatal("NewStaticRuleTable() returns nil")
	}

	rules := RuleList{}
	table.Update(StaticConf{
		Version: "v1",
		Config: ProductRules{
			"unittest": &rules,
		},
	})

	got, ok := table.Search("unittest")
	if !ok || got != &rules {
		t.Errorf("Search(unittest) should return stored rules")
	}

	_, ok = table.Search("notexist")
	if ok {
		t.Errorf("Search(notexist) should return false")
	}
}

func TestRuleListCheckErrorMessage(t *testing.T) {
	root := rootForRuleTests(t)
	browse := ActionBrowse
	ruleList := RuleFileList{
		StaticRuleFile{
			Cond: "req_host_in(\"www.example.org\")",
			Action: &ActionFile{
				Cmd:    &browse,
				Params: []string{root, "index.html"},
			},
		},
		StaticRuleFile{
			Cond:   "req_host_in(\"www.example.org\")",
			Action: nil,
		},
	}

	err := RuleListCheck(&ruleList)
	if err == nil {
		t.Fatalf("RuleListCheck() should return error")
	}
	if !strings.Contains(err.Error(), "StaticRule: 1") {
		t.Errorf("error message should contain rule index, got %v", err)
	}
}
