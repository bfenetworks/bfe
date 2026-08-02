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

package mod_body_process

import (
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

func TestNewTokenRuleTable(t *testing.T) {
	table := NewTokenRuleTable()
	if table == nil {
		t.Fatal("NewTokenRuleTable should not return nil")
	}
}

func TestProcessRuleTableUpdateAndSearch(t *testing.T) {
	table := NewTokenRuleTable()
	cond, _ := condition.Build("default_t()")
	conf := productRuleConf{
		Version: "1.0",
		Config: ProductRules{
			"product": {{Cond: cond}},
		},
	}

	table.Update(conf)
	rules, ok := table.Search("product")
	if !ok {
		t.Fatal("expected product found")
	}
	if len(rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(rules))
	}

	_, ok = table.Search("unknown")
	if ok {
		t.Error("expected unknown product not found")
	}
}
