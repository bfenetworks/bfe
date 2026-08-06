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
	"testing"
)

func TestErrorsRuleTableSearch(t *testing.T) {
	conf, err := ErrorsConfLoad("./testdata/mod_errors/errors_rule.data")
	if err != nil {
		t.Fatalf("ErrorsConfLoad() error: %v", err)
	}

	table := NewErrorsRuleTable()
	table.Update(conf)

	rules, ok := table.Search("example")
	if !ok {
		t.Fatalf("Search() should find product example")
	}
	if len(*rules) != 2 {
		t.Errorf("product example should have 2 rules, not %d", len(*rules))
	}

	_, ok = table.Search("not_exist")
	if ok {
		t.Errorf("Search() should not find product not_exist")
	}
}

func TestErrorsRuleTableSearchEmpty(t *testing.T) {
	table := NewErrorsRuleTable()

	_, ok := table.Search("example")
	if ok {
		t.Errorf("Search() should not find any product in empty table")
	}
}
