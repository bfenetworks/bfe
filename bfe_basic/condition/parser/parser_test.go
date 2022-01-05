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

package parser

import (
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/json"
)

func TestParserParse(t *testing.T) {
	testCases := []struct {
		condStr   string
		variables []string
		hasError  bool
	}{
		{
			`req_path_prefix_in("/static/", true)&&req_cookie_value_in("BUID", "TEST") && req_host_in(true) && x`,
			[]string{"x"},
			true,
		},
		{
			`req_path_prefix_in("/static/", true)&&req_cookie_value_in("BUID", "TEST", true) && req_host_in(true) && x`,
			[]string{"x"},
			true,
		},
		{
			`req_path_prefix_in("/static/", true)&&req_cookie_value_in("BUID", "TEST", true) && req_host_in("xxx") && x`,
			[]string{"x"},
			false,
		},
		{
			`req_path_prefix_in("/static/", true)&&x&&req_cookie_value_in("BUID", "TEST", true) && req_host_in("xxx") && x`,
			[]string{"x"},
			false,
		},
		{
			`!req_path_prefix_in("/static/", true)&&!x&&!req_cookie_value_in("BUID", "TEST", true)||!req_host_in("xxx") && !x`,
			[]string{"x"},
			false,
		},
	}

	for _, testCase := range testCases {
		node, idents, err := Parse(testCase.condStr)

		if err != nil {
			if testCase.hasError {
				t.Logf("got error as expected [%s]", err)
			} else {
				t.Errorf("got unexpected err [%s]", err)
			}

			continue
		}

		if testCase.hasError {
			t.Errorf("parser should return err")
			continue
		}

		for i, ident := range idents {
			if len(testCase.variables) < i+1 {
				t.Fatalf("cond variables len error %v", idents)
			}

			if ident.Name != testCase.variables[i] {
				t.Fatalf("expect ident %s got %s", testCase.variables, ident.Name)
			}
		}

		b, _ := json.MarshalIndent(node, "", "    ")
		t.Logf("%s", b)
	}
}

func TestParen(t *testing.T) {
	node, _, _ := Parse(`(a && b) && c`)

	b, _ := json.MarshalIndent(node, "", "    ")
	t.Logf("%s", b)
}
