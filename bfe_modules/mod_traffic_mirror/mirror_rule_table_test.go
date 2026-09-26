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
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func newMatchTestRequest(product, body string) *bfe_basic.Request {
	httpReq, _ := bfe_http.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions",
		ioutil.NopCloser(bytes.NewBufferString(body)))
	httpReq.Header.Set("Content-Type", "application/json")
	req := bfe_basic.NewRequest(httpReq, nil, nil, &bfe_basic.Session{}, nil)
	req.Route = bfe_basic.RequestRoute{Product: product}
	return req
}

func prepareTestRuleTable(t *testing.T) *mirrorRuleTable {
	conf, err := MirrorRuleConfLoad("testdata/mod_traffic_mirror/mirror_rule.data")
	if err != nil {
		t.Fatalf("MirrorRuleConfLoad failed: %v", err)
	}
	table := newMirrorRuleTable()
	if err := table.load(conf); err != nil {
		t.Fatalf("rule table load failed: %v", err)
	}
	return table
}

func TestRuleTableSearchMiss(t *testing.T) {
	table := prepareTestRuleTable(t)

	if _, ok := table.Search("unknown_product"); ok {
		t.Error("unknown product should not be found")
	}
}

func TestRuleTableMatchByCond(t *testing.T) {
	table := prepareTestRuleTable(t)
	rules, _ := table.Search("default")

	// rule 1 matches gpt-4o requests
	req := newMatchTestRequest("default", `{"model":"gpt-4o","messages":[]}`)
	rule := table.Match(req, rules)
	if rule == nil {
		t.Fatal("rule should match gpt-4o request")
	}
	if rule.MirrorCluster != "cluster_shadow_v2" {
		t.Errorf("MirrorCluster = %s, want cluster_shadow_v2", rule.MirrorCluster)
	}

	// other models fall through to the catch-all rule (percentage 0)
	req2 := newMatchTestRequest("default", `{"model":"deepseek-chat","messages":[]}`)
	rule2 := table.Match(req2, rules)
	if rule2 == nil {
		t.Fatal("catch-all rule should match other models")
	}
	if rule2.MirrorCluster != "cluster_shadow_all" {
		t.Errorf("MirrorCluster = %s, want cluster_shadow_all", rule2.MirrorCluster)
	}
}

func TestRuleTableMatchOtherProduct(t *testing.T) {
	table := prepareTestRuleTable(t)
	rules, _ := table.Search("other_product")

	req := newMatchTestRequest("other_product", `{"model":"any"}`)
	rule := table.Match(req, rules)
	if rule == nil {
		t.Fatal("rule should match for other_product")
	}
	if rule.MirrorCluster != "cluster_shadow_other" {
		t.Errorf("MirrorCluster = %s", rule.MirrorCluster)
	}
}
