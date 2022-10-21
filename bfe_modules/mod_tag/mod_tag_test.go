// Copyright (c) 2019 The BFE Authors.
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
	"net/url"
	"testing"
)

import (
	"github.com/baidu/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const (
	expectProduct = "example_product"
)

func TestLoadRuleData(t *testing.T) {
	m := new(ModuleTag)
	m.ruleTable = new(TagRuleTable)

	query := url.Values{
		"path": []string{"testdata/mod_tag/tag_rule.data"},
	}

	expectModVersion := "tag_rule.data=20200218210000"
	modVersion, err := m.loadRuleData(query)
	if err != nil {
		t.Fatalf("should have no error, but error is %v", err)
	}

	if modVersion != expectModVersion {
		t.Fatalf("version should be %s, but it's %s", expectModVersion, modVersion)
	}

	expectVersion := "20200218210000"
	if m.ruleTable.version != expectVersion {
		t.Fatalf("version should be %s, but it's %s", expectVersion, m.ruleTable.version)
	}

	ruleList, ok := m.ruleTable.productRule[expectProduct]
	if !ok {
		t.Fatalf("config should have product: %s", expectProduct)
	}

	if len(ruleList) != 1 {
		t.Fatalf("len(ruleList) should be 1, but it's %d", len(ruleList))
	}
}

func TestTagHandlerCase1(t *testing.T) {
	m := NewModuleTag()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	err := m.Init(cb, wh, "./testdata")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// prepare request
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = expectProduct
	req.HttpRequest, err = bfe_http.NewRequest("GET", "http://example.org", nil)
	if err != nil {
		t.Fatalf("bfe_http.NewRequest error: %v", err)
	}
	req.Tags.TagTable = make(map[string][]string)

	m.tagHandler(req)

	if len(req.Tags.TagTable) != 2 {
		t.Fatalf("req.Tags.TagTable should have 2 tag, but it's %d", len(req.Tags.TagTable))
	}

	expectTagName := "tag_test1"
	tagValue := req.Tags.TagTable[expectTagName]
	if len(tagValue) != 1 {
		t.Fatalf("req.Tags.TagTable should have tag[%s], but it's %d", expectTagName,
			len(req.Tags.TagTable[expectTagName]))
	}

	expectTagValue := "bfe_test1"
	if tagValue[0] != expectTagValue {
		t.Fatalf("TagValue should be %s, but it's %s", expectTagValue, req.Tags.TagTable[expectTagName][0])
	}

	expectTagName = "tag_test2"
	tagValue = req.Tags.TagTable[expectTagName]
	if len(tagValue) != 1 {
		t.Fatalf("req.Tags.TagTable should have tag[%s], but it's %d", expectTagName,
			len(req.Tags.TagTable[expectTagName]))
	}

	expectTagValue = "bfe_test2"
	if tagValue[0] != expectTagValue {
		t.Fatalf("TagValue should be %s, but it's %s", expectTagValue, req.Tags.TagTable[expectTagName][0])
	}
}

func TestTagHandlerCase2(t *testing.T) {
	m := new(ModuleTag)
	m.ruleTable = new(TagRuleTable)

	query := url.Values{"path": []string{"testdata/mod_tag/tag_rule.data3"}}
	_, err := m.loadRuleData(query)
	if err != nil {
		t.Fatalf("should have no error, but error is %v", err)
	}

	// prepare request
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = expectProduct
	req.HttpRequest, err = bfe_http.NewRequest("GET", "http://example.org", nil)
	if err != nil {
		t.Fatalf("bfe_http.NewRequest error: %v", err)
	}
	req.Tags.TagTable = make(map[string][]string)

	m.tagHandler(req)

	if len(req.Tags.TagTable) != 1 {
		t.Fatalf("req.Tags.TagTable should have 1 tag, but it's %d", len(req.Tags.TagTable))
	}

	expectTagName := "tag31"
	tagValue := req.Tags.TagTable[expectTagName]
	if len(tagValue) != 1 {
		t.Fatalf("req.Tags.TagTable should have tag[%s], but it's %d", expectTagName,
			len(req.Tags.TagTable[expectTagName]))
	}

	expectTagValue := "bfe31"
	if tagValue[0] != expectTagValue {
		t.Fatalf("TagValue should be %s, but it's %s", expectTagValue, req.Tags.TagTable[expectTagName][0])
	}
}

func TestTagHandlerCase3(t *testing.T) {
	m := new(ModuleTag)
	m.ruleTable = new(TagRuleTable)

	query := url.Values{"path": []string{"testdata/mod_tag/tag_rule.data4"}}
	_, err := m.loadRuleData(query)
	if err != nil {
		t.Fatalf("should have no error, but error is %v", err)
	}

	// prepare request
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = expectProduct
	req.HttpRequest, err = bfe_http.NewRequest("GET", "http://example.org", nil)
	if err != nil {
		t.Fatalf("bfe_http.NewRequest error: %v", err)
	}
	req.Tags.TagTable = make(map[string][]string)

	m.tagHandler(req)

	if len(req.Tags.TagTable) != 1 {
		t.Fatalf("req.Tags.TagTable should have 1 tag, but it's %d", len(req.Tags.TagTable))
	}

	expectTagName := "tag4"
	tagValue := req.Tags.TagTable[expectTagName]
	if len(tagValue) != 2 {
		t.Fatalf("req.Tags.TagTable should have tag[%s] and len(tagValue) is 2, but it's %d", expectTagName,
			len(req.Tags.TagTable[expectTagName]))
	}

	expectTagValue1 := "bfe41"
	if tagValue[0] != expectTagValue1 {
		t.Fatalf("value should be %s, but it's %s", expectTagValue1, tagValue[0])
	}

	expectTagValue2 := "bfe42"
	if tagValue[1] != expectTagValue2 {
		t.Fatalf("value should be %s, but it's %s", expectTagValue2, tagValue[1])
	}
}
