// Copyright (c) 2025 The BFE Authors.
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

/* action_hook_test.go - tests for action params compiler injection */

package bfe_expr

import (
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic/action"
)

// 无表达式参数：不生成求值闭包
func TestCompileActionParamsNoExpr(t *testing.T) {
	eval, err := compileActionParams([]string{"key", "value"}, []int{1})
	if err != nil {
		t.Fatalf("compileActionParams: %v", err)
	}
	if eval != nil {
		t.Error("should return nil eval when no expr param")
	}
}

// 下标越界/无表达式：忽略，不报错
func TestCompileActionParamsIgnoreIndex(t *testing.T) {
	eval, err := compileActionParams([]string{"key"}, []int{5})
	if err != nil {
		t.Fatalf("compileActionParams: %v", err)
	}
	if eval != nil {
		t.Error("out-of-range index should be ignored")
	}
}

// 非法表达式：加载期报错
func TestCompileActionParamsInvalidExpr(t *testing.T) {
	if _, err := compileActionParams([]string{"key", "=1+1"}, []int{1}); err == nil {
		t.Error("invalid expr should fail at load time")
	}
}

// 表达式参数：运行期求值，其他参数原样保留
func TestCompileActionParamsEval(t *testing.T) {
	params := []string{"X-Bfe-Key", "=ReqMethod()"}
	eval, err := compileActionParams(params, []int{1})
	if err != nil {
		t.Fatalf("compileActionParams: %v", err)
	}
	if eval == nil {
		t.Fatal("eval should not be nil")
	}

	out := eval(newTestRequest())
	if len(out) != 2 || out[0] != "X-Bfe-Key" || out[1] != "GET" {
		t.Errorf("eval result = %v, want [X-Bfe-Key GET]", out)
	}

	// 原始参数不被修改
	if params[1] != "=ReqMethod()" {
		t.Errorf("params mutated: %v", params)
	}
}

// 求值失败：该参数置空，其他参数照常返回
func TestCompileActionParamsEvalError(t *testing.T) {
	eval, err := compileActionParams([]string{"X-Bfe-Key", "=ReqMethod()"}, []int{1})
	if err != nil {
		t.Fatalf("compileActionParams: %v", err)
	}

	out := eval(nil)
	if len(out) != 2 || out[0] != "X-Bfe-Key" || out[1] != "" {
		t.Errorf("eval(nil) = %v, want [X-Bfe-Key \"\"]", out)
	}
}

// 通过 action.Action 验证注入生效：加载期编译、运行期求值
func TestActionExprParamHook(t *testing.T) {
	var a action.Action
	data := []byte(`{"Cmd":"REQ_HEADER_SET","Params":["X-Bfe-Key","=ReqMethod()"]}`)
	if err := a.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	req := newTestRequest()
	if err := a.Do(req); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := req.HttpRequest.Header.Get("X-Bfe-Key"); got != "GET" {
		t.Errorf("header X-Bfe-Key = %q, want GET", got)
	}
}

// 转义字面量：=="=value" → "=value"，不按表达式求值
func TestActionExprParamEscaped(t *testing.T) {
	var a action.Action
	data := []byte(`{"Cmd":"REQ_HEADER_SET","Params":["X-Bfe-Key","==value"]}`)
	if err := a.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	req := newTestRequest()
	if err := a.Do(req); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := req.HttpRequest.Header.Get("X-Bfe-Key"); got != "=value" {
		t.Errorf("header X-Bfe-Key = %q, want =value", got)
	}
}

// 非法表达式：配置加载失败
func TestActionExprParamCompileError(t *testing.T) {
	var a action.Action
	data := []byte(`{"Cmd":"REQ_HEADER_SET","Params":["X-Bfe-Key","=NoSuchFunc()"]}`)
	if err := a.UnmarshalJSON(data); err == nil {
		t.Error("UnmarshalJSON should fail on invalid expr")
	}
}

// 默认放开：参数未登记白名单的动作（QUERY_DEL_ALL_EXCEPT）也支持表达式
func TestActionExprParamDefaultAll(t *testing.T) {
	var a action.Action
	data := []byte(`{"Cmd":"QUERY_DEL_ALL_EXCEPT","Params":["=ReqMethod()"]}`)
	if err := a.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	// 请求方法 GET 不在 query key 中，因此所有 query 都被删除
	req := newTestRequest()
	if err := a.Do(req); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := req.HttpRequest.URL.RawQuery; got != "" {
		t.Errorf("RawQuery = %q, want empty", got)
	}
}

// header 名保持字面量：REQ_HEADER_SET 的 Params[0] 写表达式应报错
func TestActionExprParamHeaderNameLiteralOnly(t *testing.T) {
	var a action.Action
	data := []byte(`{"Cmd":"REQ_HEADER_SET","Params":["=ReqMethod()","v"]}`)
	if err := a.UnmarshalJSON(data); err == nil {
		t.Error("UnmarshalJSON should reject expr header name")
	}
}
