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

package action

import (
	"net/url"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func buildTestReq(uri string) *bfe_basic.Request {
	req := &bfe_basic.Request{
		HttpRequest: &bfe_http.Request{},
	}

	req.HttpRequest.URL, _ = url.Parse(uri)

	return req
}

var rewriteCases = []struct {
	name   string
	inUri  string
	action Action
	outUri string
}{
	{
		"query_add_case1",
		"http://example.org/s?word=123&word=321&id=1234",
		Action{
			Cmd: "QUERY_ADD",
			Params: []string{
				"key",
				"value",
			},
		},
		"http://example.org/s?word=123&word=321&id=1234&key=value",
	},
	{
		"query_add_case2",
		"http://example.org/",
		Action{
			Cmd: "QUERY_ADD",
			Params: []string{
				"key",
				"value",
			},
		},
		"http://example.org/?key=value",
	},
	{
		"query_rename_case1",
		"http://example.org/",
		Action{
			Cmd: "QUERY_RENAME",
			Params: []string{
				"key",
				"value",
			},
		},
		"http://example.org/",
	},
	{
		"query_rename_case2",
		"http://example.org/s?word=123",
		Action{
			Cmd: "QUERY_RENAME",
			Params: []string{
				"word",
				"wd",
			},
		},
		"http://example.org/s?wd=123",
	},
	{
		"query_rename_case3",
		"http://example.org/s?word=123&wd=123",
		Action{
			Cmd: "QUERY_RENAME",
			Params: []string{
				"word",
				"wd",
			},
		},
		"http://example.org/s?wd=123&wd=123",
	},
	{
		"query_rename_case4",
		"http://example.org/s?word=123&wd=123",
		Action{
			Cmd: "QUERY_RENAME",
			Params: []string{
				"wd",
				"word",
			},
		},
		"http://example.org/s?word=123&word=123",
	},
	{
		"query_rename_case5",
		"http://example.org/s?word=123&wd=123&id=32",
		Action{
			Cmd: "QUERY_RENAME",
			Params: []string{
				"wd",
				"word",
			},
		},
		"http://example.org/s?word=123&word=123&id=32",
	},
	{
		"query_del_case0",
		"http://example.org/",
		Action{
			Cmd: "QUERY_DEL",
			Params: []string{
				"wd",
			},
		},
		"http://example.org/",
	},
	{
		"query_del_case1",
		"http://example.org/s?word=123&wd=456&id=789",
		Action{
			Cmd: "QUERY_DEL",
			Params: []string{
				"wd",
			},
		},
		"http://example.org/s?word=123&id=789",
	},
	{
		"query_del_case2",
		"http://example.org/s?word=123&wd=123&id=32",
		Action{
			Cmd: "QUERY_DEL",
			Params: []string{
				"word",
			},
		},
		"http://example.org/s?wd=123&id=32",
	},
	{
		"query_del_case3",
		"http://example.org/s?word=123&wd=123&id=32",
		Action{
			Cmd: "QUERY_DEL",
			Params: []string{
				"id",
			},
		},
		"http://example.org/s?word=123&wd=123",
	},
	{
		"query_del_case4",
		"http://example.org/s?word=123",
		Action{
			Cmd: "QUERY_DEL",
			Params: []string{
				"word",
			},
		},
		"http://example.org/s",
	},
	{
		"query_del_all_except_case1",
		"http://example.org/s?word=123&wd=123&id=32",
		Action{
			Cmd: "QUERY_DEL_ALL_EXCEPT",
			Params: []string{
				"id",
			},
		},
		"http://example.org/s?id=32",
	},
	{
		"query_del_all_except_case2",
		"http://example.org/s?word=123&wd=123&id=32",
		Action{
			Cmd: "QUERY_DEL_ALL_EXCEPT",
			Params: []string{
				"word",
			},
		},
		"http://example.org/s?word=123",
	},
	{
		"query_del_all_except_case3",
		"http://example.org/s?word=123&wd=123&id=32",
		Action{
			Cmd: "QUERY_DEL_ALL_EXCEPT",
			Params: []string{
				"wd",
			},
		},
		"http://example.org/s?wd=123",
	},
	{
		"query_del_all_except_case4",
		"http://example.org/s?word=123",
		Action{
			Cmd: "QUERY_DEL_ALL_EXCEPT",
			Params: []string{
				"wd",
			},
		},
		"http://example.org/s",
	},
	{
		"query_del_all_except_case6",
		"http://example.org/client/?word=123",
		Action{
			Cmd: "QUERY_DEL_ALL_EXCEPT",
			Params: []string{
				"wd",
			},
		},
		"http://example.org/client/",
	},
	{
		"query_del_all_except_case7",
		"http://example.org/client/?word=123",
		Action{
			Cmd: "QUERY_DEL_ALL_EXCEPT",
			Params: []string{
				"wd",
			},
		},
		"http://example.org/client/",
	},
}

func TestReqQueryAdd(t *testing.T) {
	for _, testCase := range rewriteCases {
		uri := testCase.inUri
		req := buildTestReq(uri)
		testCase.action.Do(req)

		if req.HttpRequest.URL.String() != testCase.outUri {
			t.Errorf("testCase: %s expect[%s] while[%s]", testCase.name, testCase.outUri, req.HttpRequest.URL.String())
		}
	}
}
