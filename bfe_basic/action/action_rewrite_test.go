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
	"fmt"
	"strings"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func TestActionFileCheck_1(t *testing.T) {
	// no Cmd
	var err error

	actionFile := ActionFile{}

	err = ActionFileCheck(actionFile)
	if err == nil {
		t.Error("err should not be nil")
		return
	} else {
		fmt.Printf("TestActionFileCheck_1():err=%s\n", err.Error())
	}

	// no Params
	actionFile.Cmd = new(string)
	*actionFile.Cmd = "HOST_SET"
	actionFile.Params = nil

	err = ActionFileCheck(actionFile)
	if err == nil {
		t.Error("err should not be nil")
		return
	} else {
		fmt.Printf("TestActionFileCheck_1():err=%s\n", err.Error())
	}

	// no enough params for "PATH_SET"
	err = ActionFileCheck(actionFile)
	if err == nil {
		t.Error("err should not be nil")
		return
	} else {
		fmt.Printf("TestActionFileCheck_1():err=%s\n", err.Error())
	}
}

func TestActionConvert_1(t *testing.T) {
	// prepare data
	actionFile := ActionFile{}
	actionFile.Cmd = new(string)
	*actionFile.Cmd = "HOST_SET"
	actionFile.Params = []string{"www.baidu.com"}

	// invoke
	action := Action{
		Cmd:    *actionFile.Cmd,
		Params: actionFile.Params,
	}

	if action.Cmd != "HOST_SET" {
		t.Error("action.Cmd should be HOST_SET")
	}

	if len(action.Params) != 1 {
		t.Error("len(action.Params) should be 1")
	}

	if action.Params[0] != "www.baidu.com" {
		t.Error("action.Params[0] should be www.baidu.com")
	}
}

func TestReWriteActionDo_HOST_SET(t *testing.T) {
	// prepare data
	httpReq, err := bfe_http.NewRequest("GET", "http://news.baidu.com/current", nil)
	if err != nil {
		errMsg := fmt.Sprintf("err in bfe_http.NewRequest():%s", err.Error())
		t.Error(errMsg)
		return
	}

	req := &bfe_basic.Request{}
	req.HttpRequest = httpReq

	action := Action{Cmd: "HOST_SET",
		Params: []string{"www.baidu.com"},
	}

	// invoke
	action.Do(req)

	// check
	if req.HttpRequest.Host != "www.baidu.com" {
		t.Error("err in reWriteActionDo(), HOST_SET")
	}
}

func TestReWriteActionDo_PATH_SET(t *testing.T) {
	// prepare data
	httpReq, err := bfe_http.NewRequest("GET", "http://news.baidu.com/current", nil)
	if err != nil {
		errMsg := fmt.Sprintf("err in bfe_http.NewRequest():%s", err.Error())
		t.Error(errMsg)
		return
	}

	req := &bfe_basic.Request{}
	req.HttpRequest = httpReq

	action := Action{Cmd: "PATH_SET",
		Params: []string{"/index"},
	}

	// invoke
	action.Do(req)

	// check
	if req.HttpRequest.URL.Path != "/index" {
		t.Error("err in reWriteActionDo(), PATH_SET")
	}
}

func TestReWriteActionDo_PATH_PREFIX_ADD(t *testing.T) {
	// prepare data
	httpReq, err := bfe_http.NewRequest("GET", "http://news.baidu.com/current", nil)
	if err != nil {
		errMsg := fmt.Sprintf("err in bfe_http.NewRequest():%s", err.Error())
		t.Error(errMsg)
		return
	}

	req := &bfe_basic.Request{}
	req.HttpRequest = httpReq

	action := Action{Cmd: "PATH_PREFIX_ADD",
		Params: []string{"index/"},
	}

	// invoke
	action.Do(req)

	// check
	if req.HttpRequest.URL.Path != "/index/current" {
		t.Error("err in reWriteActionDo(), PATH_PREFIX_ADD")
	}
}

func TestReWriteActionDo_PATH_PREFIX_TRIM(t *testing.T) {
	// prepare data
	httpReq, err := bfe_http.NewRequest("GET", "http://m.baidu.com/service/index.html", nil)
	if err != nil {
		errMsg := fmt.Sprintf("err in bfe_http.NewRequest():%s", err.Error())
		t.Error(errMsg)
		return
	}

	req := &bfe_basic.Request{}
	req.HttpRequest = httpReq

	action := Action{Cmd: "PATH_PREFIX_TRIM",
		Params: []string{"/service"},
	}

	// invoke
	action.Do(req)

	// check
	if req.HttpRequest.URL.Path != "/index.html" {
		t.Error("err in reWriteActionDo(), PATH_PREFIX_TRIM")
	}
}

func TestReWriteActionDo_PATH_PREFIX_TRIM_CASE2(t *testing.T) {
	// prepare data
	httpReq, err := bfe_http.NewRequest("GET", "http://m.baidu.com/service/index.html/?wd=123", nil)
	if err != nil {
		errMsg := fmt.Sprintf("err in bfe_http.NewRequest():%s", err.Error())
		t.Error(errMsg)
		return
	}

	req := &bfe_basic.Request{}
	req.HttpRequest = httpReq

	action := Action{Cmd: "PATH_PREFIX_TRIM",
		Params: []string{"/service"},
	}

	// invoke
	action.Do(req)

	// check
	if req.HttpRequest.URL.Path != "/index.html/" {
		t.Error("err in reWriteActionDo(), PATH_PREFIX_TRIM")
	}
}

func TestReWriteActionDo_PATH_PREFIX_TRIM_CASE3(t *testing.T) {
	// prepare data
	httpReq, err := bfe_http.NewRequest("GET", "http://m.baidu.com/service/index.html/?wd=123", nil)
	if err != nil {
		errMsg := fmt.Sprintf("err in bfe_http.NewRequest():%s", err.Error())
		t.Error(errMsg)
		return
	}

	req := &bfe_basic.Request{}
	req.HttpRequest = httpReq

	ReqPathPrefixTrim(req, "/service")

	// check
	if req.HttpRequest.URL.Path != "/index.html/" {
		t.Error("err in reWriteActionDo(), PATH_PREFIX_TRIM")
	}
}

func TestReWriteActionDo_QUERY_ADD(t *testing.T) {
	// prepare data
	urlStr := "http://m.baidu.com/?a=1"
	httpReq, err := bfe_http.NewRequest("GET", urlStr, nil)
	if err != nil {
		errMsg := fmt.Sprintf("err in bfe_http.NewRequest():%s", err.Error())
		t.Error(errMsg)
		return
	}

	req := &bfe_basic.Request{}
	req.HttpRequest = httpReq

	action := Action{Cmd: "QUERY_ADD",
		Params: []string{"b", "2"},
	}

	// invoke
	action.Do(req)

	// check
	if req.HttpRequest.URL.RawQuery != "a=1&b=2" &&
		req.HttpRequest.URL.RawQuery != "b=2&a=1" {
		t.Error("err in reWriteActionDo(), QUERY_ADD")
	}
}

func TestReWriteActionDo_QUERY_ADD_CASE2(t *testing.T) {
	// prepare data
	urlStr := "http://m.baidu.com/test/?a=1"
	httpReq, err := bfe_http.NewRequest("GET", urlStr, nil)
	if err != nil {
		errMsg := fmt.Sprintf("err in bfe_http.NewRequest():%s", err.Error())
		t.Error(errMsg)
		return
	}

	req := &bfe_basic.Request{}
	req.HttpRequest = httpReq

	action := Action{Cmd: "QUERY_ADD",
		Params: []string{"b", "2"},
	}

	// invoke
	action.Do(req)

	if req.HttpRequest.URL.Path != "/test/" {
		t.Errorf("err in reWriteActionDo(), QUERY_ADD")
	}

	// check
	if req.HttpRequest.URL.RawQuery != "a=1&b=2" &&
		req.HttpRequest.URL.RawQuery != "b=2&a=1" {
		t.Error("err in reWriteActionDo(), QUERY_ADD")
	}
}

func TestReWriteActionDo_QUERY_RENAME(t *testing.T) {
	// prepare data
	urlStr := "http://m.baidu.com/?a=1&a=2"
	httpReq, err := bfe_http.NewRequest("GET", urlStr, nil)
	if err != nil {
		errMsg := fmt.Sprintf("err in bfe_http.NewRequest():%s", err.Error())
		t.Error(errMsg)
		return
	}

	req := &bfe_basic.Request{}
	req.HttpRequest = httpReq

	action := Action{Cmd: "QUERY_RENAME",
		Params: []string{"a", "b"},
	}

	// invoke
	action.Do(req)

	// check
	if req.HttpRequest.URL.RawQuery != "b=1&b=2" &&
		req.HttpRequest.URL.RawQuery != "b=2&b=1" {
		t.Error("err in reWriteActionDo(), QUERY_RENAME")
	}
}

func TestReWriteActionDo_QUERY_DEL(t *testing.T) {
	// prepare data
	urlStr := "http://m.baidu.com/?a=1&b=2&c=3"
	httpReq, err := bfe_http.NewRequest("GET", urlStr, nil)
	if err != nil {
		errMsg := fmt.Sprintf("err in bfe_http.NewRequest():%s", err.Error())
		t.Error(errMsg)
		return
	}

	req := &bfe_basic.Request{}
	req.HttpRequest = httpReq

	action := Action{Cmd: "QUERY_DEL",
		Params: []string{"b", "c"},
	}

	// invoke
	action.Do(req)

	// check
	if req.HttpRequest.URL.RawQuery != "a=1" {
		t.Error("err in reWriteActionDo(), QUERY_DEL")
	}
}

func TestReWriteActionDo_QUERY_DEL_ALL_EXCEPT(t *testing.T) {
	// prepare data
	urlStr := "http://m.baidu.com/?a=1&b=2&c=3"
	httpReq, err := bfe_http.NewRequest("GET", urlStr, nil)
	if err != nil {
		errMsg := fmt.Sprintf("err in bfe_http.NewRequest():%s", err.Error())
		t.Error(errMsg)
		return
	}

	req := &bfe_basic.Request{}
	req.HttpRequest = httpReq

	action := Action{Cmd: "QUERY_DEL_ALL_EXCEPT",
		Params: []string{"a"},
	}

	// invoke
	action.Do(req)

	// check
	if req.HttpRequest.URL.RawQuery != "a=1" {
		t.Error("err in reWriteActionDo(), QUERY_DEL_ALL_EXCEPT")
	}
}

func TestReWriteActionDo_complex_case(t *testing.T) {
	// prepare request
	urlStr := "http://www.baidu.com/?wd=abcdef&a=1&b=2"
	httpReq, err := bfe_http.NewRequest("GET", urlStr, nil)
	if err != nil {
		errMsg := fmt.Sprintf("err in bfe_http.NewRequest():%s", err.Error())
		t.Error(errMsg)
		return
	}

	req := &bfe_basic.Request{}
	req.HttpRequest = httpReq

	// prepare actions
	actions := make([]Action, 0)

	action := Action{Cmd: "QUERY_DEL_ALL_EXCEPT",
		Params: []string{"wd"},
	}
	actions = append(actions, action)

	action = Action{Cmd: "QUERY_RENAME",
		Params: []string{"wd", "word"},
	}
	actions = append(actions, action)

	action = Action{Cmd: "QUERY_ADD",
		Params: []string{"from", "844b", "vit", "fps"},
	}
	actions = append(actions, action)

	action = Action{Cmd: "HOST_SET",
		Params: []string{"m.baidu.com"},
	}
	actions = append(actions, action)

	action = Action{Cmd: "PATH_SET",
		Params: []string{"/s"},
	}
	actions = append(actions, action)

	// invoke
	for _, action = range actions {
		action.Do(req)
	}

	// check
	if req.HttpRequest.Host != "m.baidu.com" {
		t.Error("err in reWriteActionsDo(), HOST_SET")
	}

	if req.HttpRequest.URL.Path != "/s" {
		t.Error("err in reWriteActionsDo(), PATH_SET")
	}

	queries := req.HttpRequest.URL.Query()
	rawQuery := req.HttpRequest.URL.RawQuery
	if len(queries) != 3 ||
		!strings.Contains(rawQuery, "word=abcdef") ||
		!strings.Contains(rawQuery, "from=844b") ||
		!strings.Contains(rawQuery, "vit=fps") {
		t.Errorf("err in reWriteActionsDo(), query rewrite: %s, %s", queries, rawQuery)
	}
}
