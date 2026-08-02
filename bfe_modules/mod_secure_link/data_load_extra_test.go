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

package mod_secure_link

import (
	"net/url"
	"strings"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func TestDataLoadFileNotExist(t *testing.T) {
	_, err := DataLoad("testdata/mod_secure_link/not_exist.data")
	if err == nil {
		t.Errorf("DataLoad() want error for missing file, got nil")
	}
}

func TestNewDataVersionNil(t *testing.T) {
	_, err := NewData(&DataFile{
		Config: ProductRulesFile{},
	})
	if err == nil || !strings.Contains(err.Error(), "Version") {
		t.Errorf("NewData() want Version error, got %v", err)
	}
}

func TestNewDataConfigNil(t *testing.T) {
	version := "v1"
	_, err := NewData(&DataFile{
		Version: &version,
	})
	if err == nil || !strings.Contains(err.Error(), "Config") {
		t.Errorf("NewData() want Config error, got %v", err)
	}
}

func TestNewDataNilRule(t *testing.T) {
	version := "v1"
	_, err := NewData(&DataFile{
		Version: &version,
		Config: ProductRulesFile{
			"p1": []*RuleFile{nil},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "Config[0]") {
		t.Errorf("NewData() want Config[0] error, got %v", err)
	}
}

func TestNewRuleCondNil(t *testing.T) {
	_, err := NewRule(&RuleFile{
		ExpressionNodes: []ExpressionNodeFile{
			{Type: "label", Param: "secret"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "Cond") {
		t.Errorf("NewRule() want Cond error, got %v", err)
	}
}

func TestNewRuleBadCond(t *testing.T) {
	badCond := "bad_cond()"
	_, err := NewRule(&RuleFile{
		Cond: &badCond,
		ExpressionNodes: []ExpressionNodeFile{
			{Type: "label", Param: "secret"},
		},
	})
	if err == nil {
		t.Errorf("NewRule() want error for bad condition, got nil")
	}
}

func TestNewRuleEmptyChecksumKey(t *testing.T) {
	cond := "default_t()"
	emptyKey := ""
	_, err := NewRule(&RuleFile{
		Cond:        &cond,
		ChecksumKey: &emptyKey,
		ExpressionNodes: []ExpressionNodeFile{
			{Type: "label", Param: "secret"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "ChecksumKey") {
		t.Errorf("NewRule() want ChecksumKey error, got %v", err)
	}
}

func TestNewRuleNilExpressionNodes(t *testing.T) {
	cond := "default_t()"
	_, err := NewRule(&RuleFile{
		Cond: &cond,
	})
	if err == nil || !strings.Contains(err.Error(), "SecureLink") {
		t.Errorf("NewRule() want SecureLink error, got %v", err)
	}
}

func TestNewRuleBadExpressionNode(t *testing.T) {
	cond := "default_t()"
	_, err := NewRule(&RuleFile{
		Cond: &cond,
		ExpressionNodes: []ExpressionNodeFile{
			{Type: "bad_type"},
		},
	})
	if err == nil {
		t.Errorf("NewRule() want error for bad expression node, got nil")
	}
}

func TestNewDataBadRule(t *testing.T) {
	version := "v1"
	badCond := "bad_cond()"
	_, err := NewData(&DataFile{
		Version: &version,
		Config: ProductRulesFile{
			"p1": []*RuleFile{{Cond: &badCond}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "bad Config[0] node") {
		t.Errorf("NewData() want bad Config[0] node error, got %v", err)
	}
}

func TestRuleCheck(t *testing.T) {
	cond := "default_t()"
	rule, err := NewRule(&RuleFile{
		Cond: &cond,
		ExpressionNodes: []ExpressionNodeFile{
			{Type: "label", Param: "secret"},
		},
	})
	if err != nil {
		t.Fatalf("NewRule() error: %v", err)
	}

	req := &bfe_basic.Request{
		HttpRequest: &bfe_http.Request{},
		Query: url.Values{
			"md5": []string{rule.Checker.encode("secret")},
		},
	}
	if err := rule.Check(req); err != nil {
		t.Errorf("Rule.Check() want nil, got %v", err)
	}
}
