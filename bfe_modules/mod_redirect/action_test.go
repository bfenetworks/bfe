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

package mod_redirect

import (
	"net/http"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func TestActionFileCheckNoCmd(t *testing.T) {
	conf := ActionFile{Params: []string{"value"}}
	if err := ActionFileCheck(conf); err == nil || !strings.Contains(err.Error(), "no Cmd") {
		t.Errorf("ActionFileCheck() should return no Cmd error, got %v", err)
	}
}

func TestActionFileCheckNoParams(t *testing.T) {
	cmd := "URL_SET"
	conf := ActionFile{Cmd: &cmd}
	if err := ActionFileCheck(conf); err == nil || !strings.Contains(err.Error(), "no Params") {
		t.Errorf("ActionFileCheck() should return no Params error, got %v", err)
	}
}

func TestActionFileCheckInvalidCmd(t *testing.T) {
	cmd := "UNKNOWN_CMD"
	conf := ActionFile{Cmd: &cmd, Params: []string{"value"}}
	if err := ActionFileCheck(conf); err == nil || !strings.Contains(err.Error(), "invalid cmd") {
		t.Errorf("ActionFileCheck() should return invalid cmd error, got %v", err)
	}
}

func TestActionFileCheckWrongParamsLength(t *testing.T) {
	cmd := "URL_SET"
	conf := ActionFile{Cmd: &cmd, Params: []string{"a", "b"}}
	if err := ActionFileCheck(conf); err == nil || !strings.Contains(err.Error(), "num of params") {
		t.Errorf("ActionFileCheck() should return params length error, got %v", err)
	}
}

func TestActionFileCheckInvalidScheme(t *testing.T) {
	cmd := "SCHEME_SET"
	conf := ActionFile{Cmd: &cmd, Params: []string{"ftp"}}
	if err := ActionFileCheck(conf); err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Errorf("ActionFileCheck() should return scheme error, got %v", err)
	}
}

func TestActionFileCheckSchemeLowercase(t *testing.T) {
	cmd := "SCHEME_SET"
	params := []string{"HTTPS"}
	conf := ActionFile{Cmd: &cmd, Params: params}
	if err := ActionFileCheck(conf); err != nil {
		t.Errorf("ActionFileCheck() should not return error, got %v", err)
		return
	}
	if params[0] != "https" {
		t.Errorf("scheme should be lowercased to https, got %s", params[0])
	}
}

func TestActionFileCheckValidActions(t *testing.T) {
	cases := []struct {
		cmd    string
		params []string
	}{
		{"URL_SET", []string{"http://www.example.org"}},
		{"URL_FROM_QUERY", []string{"url"}},
		{"URL_PREFIX_ADD", []string{"https://www.example.org"}},
		{"SCHEME_SET", []string{"https"}},
	}

	for _, c := range cases {
		conf := ActionFile{Cmd: &c.cmd, Params: c.params}
		if err := ActionFileCheck(conf); err != nil {
			t.Errorf("ActionFileCheck(%s) should not return error, got %v", c.cmd, err)
		}
	}
}

func TestActionFileListCheckMultipleActions(t *testing.T) {
	cmd1 := "URL_SET"
	cmd2 := "URL_PREFIX_ADD"
	list := ActionFileList{
		{Cmd: &cmd1, Params: []string{"http://a.com"}},
		{Cmd: &cmd2, Params: []string{"http://b.com"}},
	}
	if err := ActionFileListCheck(&list); err == nil || !strings.Contains(err.Error(), "exclusive action") {
		t.Errorf("ActionFileListCheck() should return exclusive action error, got %v", err)
	}
}

func TestActionFileListCheckEmpty(t *testing.T) {
	list := ActionFileList{}
	if err := ActionFileListCheck(&list); err != nil {
		t.Errorf("ActionFileListCheck() should not return error for empty list, got %v", err)
	}
}

func TestActionFileListCheckInvalidAction(t *testing.T) {
	cmd := "INVALID"
	list := ActionFileList{{Cmd: &cmd, Params: []string{"value"}}}
	if err := ActionFileListCheck(&list); err == nil {
		t.Errorf("ActionFileListCheck() should return error for invalid action")
	}
}

func TestRedirectExclusiveActionDo(t *testing.T) {
	req, _ := bfe_http.NewRequest(http.MethodGet, "http://example.org", nil)
	freq := bfe_basic.NewRequest(req, nil, nil, nil, nil)

	redirectExclusiveActionDo(freq, Action{Cmd: "SCHEME_SET", Params: []string{"https"}})
	if freq.Redirect.Url != "https://example.org/" {
		t.Errorf("SCHEME_SET result mismatch, got %s", freq.Redirect.Url)
	}

	freq.Redirect.Url = ""
	redirectExclusiveActionDo(freq, Action{Cmd: "URL_SET", Params: []string{"http://new.example.org"}})
	if freq.Redirect.Url != "http://new.example.org" {
		t.Errorf("URL_SET result mismatch, got %s", freq.Redirect.Url)
	}
}

func TestCheckExclusiveAction(t *testing.T) {
	if checkExclusiveAction([]Action{}) {
		t.Errorf("checkExclusiveAction([]) should be false")
	}

	if checkExclusiveAction([]Action{{Cmd: "URL_SET"}, {Cmd: "URL_PREFIX_ADD"}}) {
		t.Errorf("checkExclusiveAction(multi) should be false")
	}

	if !checkExclusiveAction([]Action{{Cmd: "URL_SET"}}) {
		t.Errorf("checkExclusiveAction(URL_SET) should be true")
	}

	if checkExclusiveAction([]Action{{Cmd: "UNKNOWN"}}) {
		t.Errorf("checkExclusiveAction(UNKNOWN) should be false")
	}
}

func newRedirectRequest(rawURL string) *bfe_basic.Request {
	req, _ := bfe_http.NewRequest(http.MethodGet, rawURL, nil)
	return bfe_basic.NewRequest(req, nil, nil, nil, nil)
}

// URL_SET 的表达式参数：加载期编译，运行期求值
func TestActionConvertExprParam(t *testing.T) {
	cmd := "URL_SET"
	action, err := actionConvert(ActionFile{Cmd: &cmd, Params: []string{`="http://new.example.org"`}})
	if err != nil {
		t.Fatalf("actionConvert: %v", err)
	}
	if action.param0 == nil || !action.param0.IsExpr() {
		t.Fatal("param0 should be compiled expr")
	}

	freq := newRedirectRequest("http://example.org")
	redirectExclusiveActionDo(freq, action)
	if freq.Redirect.Url != "http://new.example.org" {
		t.Errorf("URL_SET expr result = %s", freq.Redirect.Url)
	}
}

// URL_PREFIX_ADD 的表达式参数
func TestActionConvertUrlPrefixAddExpr(t *testing.T) {
	cmd := "URL_PREFIX_ADD"
	action, err := actionConvert(ActionFile{Cmd: &cmd, Params: []string{`="https://cdn.example.org"`}})
	if err != nil {
		t.Fatalf("actionConvert: %v", err)
	}

	freq := newRedirectRequest("http://example.org/path?q=1")
	redirectExclusiveActionDo(freq, action)
	if freq.Redirect.Url != "https://cdn.example.org/path?q=1" {
		t.Errorf("URL_PREFIX_ADD expr result = %s", freq.Redirect.Url)
	}
}

// "==" 转义为字面量
func TestActionConvertExprParamEscaped(t *testing.T) {
	cmd := "URL_SET"
	action, err := actionConvert(ActionFile{Cmd: &cmd, Params: []string{"==http://lit.example.org"}})
	if err != nil {
		t.Fatalf("actionConvert: %v", err)
	}
	if action.param0.IsExpr() {
		t.Error("escaped param should be literal")
	}

	freq := newRedirectRequest("http://example.org")
	redirectExclusiveActionDo(freq, action)
	if freq.Redirect.Url != "=http://lit.example.org" {
		t.Errorf("escaped literal result = %s", freq.Redirect.Url)
	}
}

// 非法表达式：加载期报错
func TestActionConvertExprParamInvalid(t *testing.T) {
	cases := []struct {
		cmd  string
		expr string
	}{
		{"URL_SET", "=1+1"},
		{"URL_PREFIX_ADD", "=NoSuchFunc()"},
	}

	for _, c := range cases {
		cmd := c.cmd
		if _, err := actionConvert(ActionFile{Cmd: &cmd, Params: []string{c.expr}}); err == nil {
			t.Errorf("actionConvert(%s, %s) should fail", c.cmd, c.expr)
		}
	}
}

// 运行期求值失败：置空
func TestActionParam0EvalError(t *testing.T) {
	cmd := "URL_SET"
	action, err := actionConvert(ActionFile{Cmd: &cmd, Params: []string{"=ReqMethod()"}})
	if err != nil {
		t.Fatalf("actionConvert: %v", err)
	}

	if got := actionParam0(nil, action); got != "" {
		t.Errorf("actionParam0(nil) = %q, want empty", got)
	}
}
