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

/* action_expr_test.go - tests for action expr-param compiler hook */

package action

import (
	"errors"
	"reflect"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func buildExprRequest() *bfe_basic.Request {
	return &bfe_basic.Request{
		HttpRequest: &bfe_http.Request{Header: make(bfe_http.Header)},
	}
}

// 未注入编译器：表达式参数按字面量处理（旧行为）
func TestExprParamCompilerNotInjected(t *testing.T) {
	var ac Action
	if err := ac.UnmarshalJSON([]byte(`{"Cmd":"REQ_HEADER_SET","Params":["X-Bfe-Key","=value"]}`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if ac.exprEval != nil {
		t.Error("exprEval should be nil without compiler")
	}

	req := buildExprRequest()
	if err := ac.Do(req); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := req.HttpRequest.Header.Get("X-Bfe-Key"); got != "=value" {
		t.Errorf("header = %q, want =value", got)
	}
}

// 注入编译器：按 cmd 的参数下标编译，Do 使用求值结果
func TestExprParamCompilerInjection(t *testing.T) {
	var gotParams []string
	var gotIndexes []int

	SetExprParamCompiler(func(params []string, indexes []int) (func(*bfe_basic.Request) []string, error) {
		gotParams, gotIndexes = params, indexes
		return func(req *bfe_basic.Request) []string {
			out := make([]string, len(params))
			copy(out, params)
			for _, i := range indexes {
				out[i] = "EVAL"
			}
			return out
		}, nil
	})
	defer SetExprParamCompiler(nil)

	var ac Action
	if err := ac.UnmarshalJSON([]byte(`{"Cmd":"REQ_HEADER_SET","Params":["X-Bfe-Key","=expr"]}`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if ac.exprEval == nil {
		t.Fatal("exprEval should be set")
	}
	if !reflect.DeepEqual(gotParams, []string{"X-Bfe-Key", "=expr"}) {
		t.Errorf("compiler params = %v", gotParams)
	}
	if !reflect.DeepEqual(gotIndexes, []int{1}) {
		t.Errorf("compiler indexes = %v, want [1]", gotIndexes)
	}

	req := buildExprRequest()
	if err := ac.Do(req); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := req.HttpRequest.Header.Get("X-Bfe-Key"); got != "EVAL" {
		t.Errorf("header = %q, want EVAL", got)
	}
}

// 编译器返回错误：配置加载失败
func TestExprParamCompilerError(t *testing.T) {
	SetExprParamCompiler(func(params []string, indexes []int) (func(*bfe_basic.Request) []string, error) {
		return nil, errors.New("bad expr")
	})
	defer SetExprParamCompiler(nil)

	var ac Action
	err := ac.UnmarshalJSON([]byte(`{"Cmd":"REQ_HEADER_SET","Params":["X-Bfe-Key","=bad"]}`))
	if err == nil {
		t.Error("UnmarshalJSON should return compiler error")
	}
}

// 不在表达式参数表内的 cmd：不调用编译器
func TestExprParamCompilerSkipped(t *testing.T) {
	called := false
	SetExprParamCompiler(func(params []string, indexes []int) (func(*bfe_basic.Request) []string, error) {
		called = true
		return nil, nil
	})
	defer SetExprParamCompiler(nil)

	var ac Action
	if err := ac.UnmarshalJSON([]byte(`{"Cmd":"CLOSE","Params":[]}`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if called {
		t.Error("compiler should not be called for CLOSE")
	}
	if ac.exprEval != nil {
		t.Error("exprEval should be nil for CLOSE")
	}
}
