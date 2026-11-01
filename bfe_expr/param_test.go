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

/* param_test.go - tests for action param expression */

package bfe_expr

import (
	"testing"
)

// 普通字面量：不做任何解析，零开销
func TestCompileParamLiteral(t *testing.T) {
	p, err := CompileParam("http://www.example.org")
	if err != nil {
		t.Fatalf("CompileParam literal: %v", err)
	}
	if p.IsExpr() {
		t.Error("literal should not be expr")
	}
	if p.String() != "http://www.example.org" {
		t.Errorf("String() = %q", p.String())
	}
	v, err := p.Eval(nil)
	if err != nil {
		t.Fatalf("Eval literal: %v", err)
	}
	if v != "http://www.example.org" {
		t.Errorf("Eval literal = %q", v)
	}
}

// "==" 转义：首个 '=' 转义为字面量
func TestCompileParamEscaped(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"==foo", "=foo"},
		{"==", "="},
		{"===foo", "==foo"},
	}

	for _, c := range cases {
		p, err := CompileParam(c.src)
		if err != nil {
			t.Errorf("CompileParam(%q): %v", c.src, err)
			continue
		}
		if p.IsExpr() {
			t.Errorf("CompileParam(%q) should be literal", c.src)
		}
		v, err := p.Eval(nil)
		if err != nil {
			t.Errorf("Eval(%q): %v", c.src, err)
			continue
		}
		if v != c.want {
			t.Errorf("Eval(%q) = %q, want %q", c.src, v, c.want)
		}
	}
}

// 表达式：加载期编译，运行期经 GetEnv 求值
func TestCompileParamExpr(t *testing.T) {
	p, err := CompileParam("=ReqMethod()")
	if err != nil {
		t.Fatalf("CompileParam expr: %v", err)
	}
	if !p.IsExpr() {
		t.Error("should be expr")
	}
	if p.String() != "=ReqMethod()" {
		t.Errorf("String() = %q", p.String())
	}

	v, err := p.Eval(newTestRequest())
	if err != nil {
		t.Fatalf("Eval expr: %v", err)
	}
	if v != "GET" {
		t.Errorf("Eval expr = %q, want GET", v)
	}
}

// 空表达式（单独的 "="）编译失败
func TestCompileParamEmptyExpr(t *testing.T) {
	if _, err := CompileParam("="); err == nil {
		t.Error("CompileParam(\"=\") should fail")
	}
}

// 返回值不是 string：编译期即失败（AsKind）
func TestCompileParamNonStringExpr(t *testing.T) {
	if _, err := CompileParam("=1+1"); err == nil {
		t.Error("CompileParam(\"=1+1\") should fail for non-string result")
	}
}

// 非法表达式（未知函数）编译失败
func TestCompileParamInvalidExpr(t *testing.T) {
	if _, err := CompileParam("=NoSuchFunc()"); err == nil {
		t.Error("CompileParam unknown func should fail")
	}
}

// nil 请求：GetEnv 返回 nil，Eval 报错
func TestParamEvalNilRequest(t *testing.T) {
	p, err := CompileParam("=ReqMethod()")
	if err != nil {
		t.Fatalf("CompileParam: %v", err)
	}
	if _, err := p.Eval(nil); err == nil {
		t.Error("Eval(nil) should fail")
	}
}

// 运行期求值错误（int 解析失败）向上传播
func TestParamEvalRuntimeError(t *testing.T) {
	p, err := CompileParam(`=string(int(ReqQuery("uid")))`)
	if err != nil {
		t.Fatalf("CompileParam: %v", err)
	}

	req := newTestRequest()
	req.HttpRequest.URL.RawQuery = "uid=abc"
	if _, err := p.Eval(req); err == nil {
		t.Error("Eval should fail on non-numeric uid")
	}
}

// 批量编译：任一失败即返回错误
func TestCompileParams(t *testing.T) {
	ps, err := CompileParams([]string{"literal", "=ReqMethod()"})
	if err != nil {
		t.Fatalf("CompileParams: %v", err)
	}
	if len(ps) != 2 || ps[0].IsExpr() || !ps[1].IsExpr() {
		t.Errorf("unexpected params: %+v", ps)
	}

	if _, err := CompileParams([]string{"ok", "="}); err == nil {
		t.Error("CompileParams should fail on invalid expr")
	}
}

// 批量求值：单个失败置空并继续
func TestEvalParamsFallback(t *testing.T) {
	ps, err := CompileParams([]string{"literal", "=ReqMethod()"})
	if err != nil {
		t.Fatalf("CompileParams: %v", err)
	}

	// nil 请求 → 表达式求值失败，置空；字面量保持
	values := EvalParams(nil, ps)
	if len(values) != 2 || values[0] != "literal" || values[1] != "" {
		t.Errorf("EvalParams(nil) = %v, want [literal \"]", values)
	}

	values = EvalParams(newTestRequest(), ps)
	if values[1] != "GET" {
		t.Errorf("EvalParams expr = %q, want GET", values[1])
	}
}
