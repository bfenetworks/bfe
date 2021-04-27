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

package mod_errors

import (
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func prepareAction() *Action {
	return &Action{
		Cmd:         RETURN,
		StatusCode:  200,
		ContentType: "text/html",
		ContentData: "hello world",
	}
}

func TestDoReturn(t *testing.T) {
	req := bfe_basic.Request{
		HttpResponse: new(bfe_http.Response),
	}
	action := prepareAction()

	doReturn(&req, *action)

	res := req.HttpResponse
	if res.StatusCode != action.StatusCode {
		t.Errorf("response errors should return %d", action.StatusCode)
	}
}

func TestActionConvert(t *testing.T) {
	cmd := RETURN
	actionFile := ActionFile{
		Cmd:    &cmd,
		Params: []string{"200", "text/html", "./testdata/mod_errors/test.html"},
	}
	action := actionConvert(actionFile)

	if action.StatusCode != 200 {
		t.Errorf("StatusCode should be 200")
	}

	if action.ContentData != "ERROR" {
		t.Errorf("ContentData should be ERROR")
	}
}
