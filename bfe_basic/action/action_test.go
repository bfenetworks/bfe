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
	"reflect"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

var actionTestCaseList = []struct {
	name string // case name

	data       string // json
	action     Action
	shouldSucc bool // should unmarshal success or not

	originHeader map[string]string
	expectHeader map[string]string
}{
	{
		"CaseWrongJson",
		`{"Params":[]`,
		Action{
			ActionClose,
			[]string{},
		},
		false,
		map[string]string{},
		map[string]string{},
	},
	{
		"CaseNilCmd",
		`{"Params":[]}`,
		Action{
			ActionClose,
			[]string{},
		},
		false,
		map[string]string{},
		map[string]string{},
	},
	{
		"CloseCaseNormal",
		`{"Cmd": "CLOSE", "Params":[]}`,
		Action{
			ActionClose,
			[]string{},
		},
		true,
		map[string]string{},
		map[string]string{},
	},
	{
		"CloseCaseWrongParams",
		`{"Cmd": "CLOSE", "Params":["colu"]}`,
		Action{
			ActionClose,
			[]string{},
		},
		false,
		map[string]string{},
		map[string]string{},
	},
	{
		"CloseCaseLowerCase",
		`{"Cmd": "close", "Params":[]}`,
		Action{
			ActionClose,
			[]string{},
		},
		true,
		map[string]string{},
		map[string]string{},
	},
	{
		"PassCaseNormal",
		`{"Cmd": "Pass", "Params":[]}`,
		Action{
			ActionPass,
			[]string{},
		},
		true,
		map[string]string{},
		map[string]string{},
	},
	{
		"AddHeaderNormalCase",
		`{"Cmd": "REQ_HEADER_ADD", "Params":["x-bfe-key","value"]}`,
		Action{
			ActionReqHeaderAdd,
			[]string{"x-bfe-key", "value"},
		},
		true,
		map[string]string{},
		map[string]string{
			"x-bfe-key": "value",
		},
	},
	{
		"AddHeaderNormalWrongCase1",
		`{"Cmd": "REQ_HEADER_ADD", "Params":["x-bfe-key"]}`,
		Action{
			ActionReqHeaderAdd,
			[]string{"key"},
		},
		false,
		map[string]string{},
		map[string]string{},
	},
	{
		"SetHeaderNormalCase",
		`{"Cmd": "REQ_HEADER_SET", "Params":["x-bfe-key","value"]}`,
		Action{
			ActionReqHeaderSet,
			[]string{"x-bfe-key", "value"},
		},
		true,
		map[string]string{"x-bfe-key": "key1"},
		map[string]string{"x-bfe-key": "value"},
	},
	{
		"SetHeaderNormalWrongCase1",
		`{"Cmd": "REQ_HEADER_SET", "Params":["key"]}`,
		Action{
			ActionReqHeaderSet,
			[]string{"key"},
		},
		false,
		map[string]string{},
		map[string]string{},
	},
	{
		"DelHeaderNormalCase",
		`{"Cmd": "REQ_HEADER_DEL", "Params":["key"]}`,
		Action{
			ActionReqHeaderDel,
			[]string{"key"},
		},
		true,
		map[string]string{"key": "value"},
		map[string]string{},
	},
	{
		"DelHeaderWrongCase1",
		`{"Cmd": "REQ_HEADER_DEL", "Params":["x-bfe-key", "value"]}`,
		Action{
			ActionReqHeaderDel,
			[]string{"x-bfe-key"},
		},
		false,
		map[string]string{},
		map[string]string{},
	},
	{
		"DelHeaderWrongCase2",
		`{"Cmd": "REQ_HEADER_DEL", "Params":[""]}`,
		Action{
			ActionReqHeaderDel,
			[]string{""},
		},
		false,
		map[string]string{},
		map[string]string{},
	},
}

func buildRequest(header map[string]string) bfe_basic.Request {
	req := bfe_basic.Request{}
	req.HttpRequest = &bfe_http.Request{}
	req.HttpRequest.Header = make(bfe_http.Header)

	for key, value := range header {
		req.HttpRequest.Header.Add(key, value)
	}

	return req
}

func TestUnmarshalJson(t *testing.T) {
	var err error

	for _, actionTestCase := range actionTestCaseList {
		var action Action
		data := []byte(actionTestCase.data)

		name := actionTestCase.name
		// unmarshal
		if err = action.UnmarshalJSON(data); (err == nil) != actionTestCase.shouldSucc {
			shouldSucc := actionTestCase.shouldSucc
			t.Errorf("caseName[%s], should Unmarshal [%v] but err is [%s]", name, shouldSucc, err)
			continue
		}

		if err != nil {
			continue
		}

		// check equal
		if !reflect.DeepEqual(action, actionTestCase.action) {
			t.Errorf("caseName[%s] should equal [%v] [%v]", name, action, actionTestCase.action)
			continue
		}

		req := buildRequest(actionTestCase.originHeader)
		err := action.Do(&req)
		if err != nil {
			t.Errorf("caseName[%s], Do action should success, but err[%s]", name, err)
			continue
		}

		if !reflect.DeepEqual(req, buildRequest(actionTestCase.expectHeader)) {
			t.Errorf("caseName[%s], header is not equal", name)
		}
	}
}
