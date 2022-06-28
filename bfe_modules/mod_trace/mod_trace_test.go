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
package mod_trace

import (
	"net/url"
	"testing"
)

import (
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"github.com/opentracing/opentracing-go"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

func TestLoadRuleData(t *testing.T) {
	m := new(ModuleTrace)
	m.ruleTable = new(TraceRuleTable)

	query := url.Values{
		"path": []string{"testdata/mod_trace/trace_rule.data"},
	}

	expectModVersion := "trace_rule.data=20200316215500"
	modVersion, err := m.loadRuleData(query)
	if err != nil {
		t.Fatalf("should have no error, but error is %v", err)
	}

	if modVersion != expectModVersion {
		t.Fatalf("version should be %s, but it's %s", expectModVersion, modVersion)
	}

	expectVersion := "20200316215500"
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

func TestStartTrace(t *testing.T) {
	m := NewModuleTrace()
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
	req.HttpRequest.Header = make(bfe_http.Header)
	req.Context = make(map[interface{}]interface{})

	m.startTrace(req)

	value := req.GetContext(CtxSpan)
	span := value.(opentracing.Span)
	if span == nil {
		t.Fatalf("GetContext %s failed", CtxSpan)
	}
}
