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

package mod_tag

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	invalidRuleData = `{
    "Version": "",
    "Config": {}
}
`
)

// prepareTempConfRoot creates a temporary configuration root that contains
// mod_tag/mod_tag.conf. If ruleData is not empty, it also writes
// mod_tag/tag_rule.data with the given content.
func prepareTempConfRoot(t *testing.T, ruleData string) string {
	t.Helper()

	dir, err := ioutil.TempDir("", "mod_tag_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	modDir := filepath.Join(dir, "mod_tag")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	confContent := `[Basic]
DataPath = mod_tag/tag_rule.data

[Log]
OpenDebug = false
`
	if err := ioutil.WriteFile(filepath.Join(modDir, "mod_tag.conf"), []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if ruleData != "" {
		if err := ioutil.WriteFile(filepath.Join(modDir, "tag_rule.data"), []byte(ruleData), 0644); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}
	}

	return dir
}

func prepareTagRequest(t *testing.T, product string, urlStr string) *bfe_basic.Request {
	t.Helper()

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = product
	req.Tags.TagTable = make(map[string][]string)

	var err error
	req.HttpRequest, err = bfe_http.NewRequest("GET", urlStr, nil)
	if err != nil {
		t.Fatalf("bfe_http.NewRequest() error: %v", err)
	}

	return req
}

func TestNewModuleTagAndName(t *testing.T) {
	m := NewModuleTag()
	if m == nil {
		t.Fatal("NewModuleTag() returns nil")
	}

	if m.Name() != ModTag {
		t.Errorf("Name() should be %s, got %s", ModTag, m.Name())
	}
}

func TestModuleTagInitSuccess(t *testing.T) {
	m := NewModuleTag()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, "./testdata"); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if m.conf == nil {
		t.Error("conf should be set after Init()")
	}
}

func TestModuleTagInitConfNotExist(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_tag_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	m := NewModuleTag()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when config file does not exist")
	}
}

func TestModuleTagInitBadRuleData(t *testing.T) {
	dir := prepareTempConfRoot(t, invalidRuleData)

	m := NewModuleTag()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when rule data file is invalid")
	}
}

func TestTagRuleTableSearch(t *testing.T) {
	table := NewTagRuleTable()

	cond, err := condition.Build("req_host_in(\"example.org\")")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}

	rules := TagRuleList{
		{
			Cond:  cond,
			Param: TagParam{TagName: "tag1", TagValue: "value1"},
			Last:  false,
		},
	}

	table.Update(&TagRuleConf{
		Version: "v1",
		Config: ProductRuleList{
			"example_product": rules,
		},
	})

	got, ok := table.Search("example_product")
	if !ok {
		t.Fatal("Search() should find rules for example_product")
	}
	if len(got) != len(rules) {
		t.Errorf("Search() returns %d rules, want %d", len(got), len(rules))
	}

	if _, ok := table.Search("unknown_product"); ok {
		t.Error("Search() should not find rules for unknown_product")
	}
}

func TestTagHandlerMatch(t *testing.T) {
	m := NewModuleTag()

	cond, err := condition.Build("req_host_in(\"example.org\")")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}

	m.ruleTable.Update(&TagRuleConf{
		Version: "v1",
		Config: ProductRuleList{
			"example_product": {
				{
					Cond:  cond,
					Param: TagParam{TagName: "tag1", TagValue: "value1"},
					Last:  false,
				},
			},
		},
	})

	req := prepareTagRequest(t, "example_product", "http://example.org")
	ret, resp := m.tagHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, got %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("resp should be nil")
	}

	values := req.Tags.TagTable["tag1"]
	if len(values) != 1 || values[0] != "value1" {
		t.Errorf("tag1 should be [value1], got %v", values)
	}
}

func TestTagHandlerNoMatch(t *testing.T) {
	m := NewModuleTag()

	cond, err := condition.Build("req_host_in(\"example.org\")")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}

	m.ruleTable.Update(&TagRuleConf{
		Version: "v1",
		Config: ProductRuleList{
			"example_product": {
				{
					Cond:  cond,
					Param: TagParam{TagName: "tag1", TagValue: "value1"},
					Last:  false,
				},
			},
		},
	})

	req := prepareTagRequest(t, "example_product", "http://other.org")
	ret, resp := m.tagHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, got %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("resp should be nil")
	}
	if len(req.Tags.TagTable) != 0 {
		t.Errorf("no tag should be added, got %v", req.Tags.TagTable)
	}
}

func TestTagHandlerProductNotFound(t *testing.T) {
	m := NewModuleTag()
	m.ruleTable.Update(&TagRuleConf{
		Version: "v1",
		Config:  ProductRuleList{},
	})

	req := prepareTagRequest(t, "unknown_product", "http://example.org")
	ret, resp := m.tagHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, got %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		t.Error("resp should be nil")
	}
	if len(req.Tags.TagTable) != 0 {
		t.Errorf("no tag should be added, got %v", req.Tags.TagTable)
	}
}

func TestTagHandlerLast(t *testing.T) {
	m := NewModuleTag()

	cond, err := condition.Build("req_host_in(\"example.org\")")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}

	m.ruleTable.Update(&TagRuleConf{
		Version: "v1",
		Config: ProductRuleList{
			"example_product": {
				{
					Cond:  cond,
					Param: TagParam{TagName: "tag1", TagValue: "value1"},
					Last:  true,
				},
				{
					Cond:  cond,
					Param: TagParam{TagName: "tag2", TagValue: "value2"},
					Last:  false,
				},
			},
		},
	})

	req := prepareTagRequest(t, "example_product", "http://example.org")
	m.tagHandler(req)

	if _, ok := req.Tags.TagTable["tag2"]; ok {
		t.Error("tag2 should not be added because Last is true on the first rule")
	}
	if _, ok := req.Tags.TagTable["tag1"]; !ok {
		t.Error("tag1 should be added")
	}
}

func TestTagHandlerAppendValues(t *testing.T) {
	m := NewModuleTag()

	cond, err := condition.Build("req_host_in(\"example.org\")")
	if err != nil {
		t.Fatalf("condition.Build() error: %v", err)
	}

	m.ruleTable.Update(&TagRuleConf{
		Version: "v1",
		Config: ProductRuleList{
			"example_product": {
				{
					Cond:  cond,
					Param: TagParam{TagName: "tag1", TagValue: "value1"},
					Last:  false,
				},
				{
					Cond:  cond,
					Param: TagParam{TagName: "tag1", TagValue: "value2"},
					Last:  false,
				},
			},
		},
	})

	req := prepareTagRequest(t, "example_product", "http://example.org")
	m.tagHandler(req)

	values := req.Tags.TagTable["tag1"]
	if len(values) != 2 {
		t.Fatalf("tag1 should have 2 values, got %v", values)
	}
	if values[0] != "value1" || values[1] != "value2" {
		t.Errorf("tag1 values should be [value1 value2], got %v", values)
	}
}
