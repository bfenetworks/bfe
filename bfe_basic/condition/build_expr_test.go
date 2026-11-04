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

/* build_expr_test.go - dual-track build tests (legacy + new-style expr) */

package condition

import (
	"testing"

	"github.com/bfenetworks/bfe/bfe_expr"
)

func TestBuildNewStyleExpr(t *testing.T) {
	c, err := Build(`ReqMethod() in ["GET", "POST"] && ReqPath() startsWith "/api"`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := c.(*bfe_expr.Condition); !ok {
		t.Fatalf("should be *bfe_expr.Condition, got %T", c)
	}
}

func TestBuildLegacyPrimitive(t *testing.T) {
	defer SetEngineMode(EngineLegacyFirst)

	// 默认 LegacyFirst：旧原语串由旧引擎编译
	c, err := Build(`req_method_in("GET|POST")`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := c.(*PrimitiveCond); !ok {
		t.Fatalf("default LegacyFirst should give *PrimitiveCond, got %T", c)
	}

	// ExprCompatible：旧原语串经 legacyPatcher 重写后由新引擎编译
	SetEngineMode(EngineExprCompatible)
	c, err = Build(`req_method_in("GET|POST")`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := c.(*bfe_expr.Condition); !ok {
		t.Fatalf("ExprCompatible should be *bfe_expr.Condition, got %T", c)
	}
}

func TestBuildLegacyComplex(t *testing.T) {
	// 复合旧表达式在 ExprCompatible 下同样走新引擎（不回退）
	defer SetEngineMode(EngineLegacyFirst)
	SetEngineMode(EngineExprCompatible)

	c, err := Build(`req_method_in("GET|POST") && req_path_prefix_in("/api", false)`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := c.(*bfe_expr.Condition); !ok {
		t.Fatalf("should be *bfe_expr.Condition, got %T", c)
	}
}

func TestBuildBothEnginesFail(t *testing.T) {
	if _, err := Build(`req_no_such_primitive("x")`); err == nil {
		t.Errorf("unknown legacy primitive should fail")
	}
	if _, err := Build(`NoSuchFunc() == 1`); err == nil {
		t.Errorf("unknown expr function should fail")
	}
	// 旧语法解析失败且新引擎也编译失败
	if _, err := Build(`req_method_in("GET|POST") &&`); err == nil {
		t.Errorf("broken expression should fail")
	}
}
