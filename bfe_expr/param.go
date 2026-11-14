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

/* param.go - action params expression
/*
DESCRIPTION
	动作参数三态语法：
	- "=expr"：expr 表达式，编译期 AsKind(String) 强类型检查，结果必须为 string；
	- "==lit"：转义字面量（首个 '=' 转义），"==" → "="；
	- 其他：普通字面量，原样使用（零开销）。
	加载期预编译，运行期经 GetEnv(req) 求值；单个参数求值失败按 "" 处理并记 warn。
*/

package bfe_expr

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/go-lib/log"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// exprPrefix 动作参数表达式前缀
const exprPrefix = "="

// Param 代表一个动作参数：expr 表达式（加载期预编译）或字面量
type Param struct {
	prog    *vm.Program // isExpr 时非 nil
	literal string      // 字面量值（isExpr=false）
	isExpr  bool
	src     string // 原始配置串（日志/调试）
}

// CompileParam 加载期编译单个动作参数。
// "=" 前缀按 expr 表达式编译（返回值必须 string）；"==" 开头转义为字面量；
// 其余为普通字符串。编译失败返回错误（由调用方使配置加载失败，不回退）。
func CompileParam(src string) (*Param, error) {
	if !strings.HasPrefix(src, exprPrefix) {
		return &Param{literal: src, src: src}, nil
	}
	if strings.HasPrefix(src, exprPrefix+exprPrefix) {
		// 转义：首个 '=' 转义为字面量，"==foo" → "=foo"，"==" → "="
		return &Param{literal: src[1:], src: src}, nil
	}

	code := src[len(exprPrefix):]
	// expr.AsKind(string)：编译期强类型检查返回值必须为 string
	prog, err := expr.Compile(code, expr.Env(&Env{}), expr.AsKind(reflect.String))
	if err != nil {
		return nil, fmt.Errorf("expr compile param %q: %v", src, err)
	}
	return &Param{prog: prog, isExpr: true, src: src}, nil
}

// IsExpr 该参数是否为表达式
func (p *Param) IsExpr() bool {
	return p.isExpr
}

// String 返回原始配置串
func (p *Param) String() string {
	return p.src
}

// Eval 运行期求值。字面量直接返回；表达式经 GetEnv(req) 求值。
// 错误向上传播，由调用方按约定处理（EvalParams 为置空继续）。
func (p *Param) Eval(req *bfe_basic.Request) (string, error) {
	if !p.isExpr {
		return p.literal, nil
	}
	env := GetEnv(req)
	if env == nil {
		return "", fmt.Errorf("nil env")
	}
	out, err := expr.Run(p.prog, env)
	if err != nil {
		return "", err
	}
	s, ok := out.(string)
	if !ok {
		return "", fmt.Errorf("result is %T, not string", out)
	}
	return s, nil
}

// CompileParams 加载期批量编译（对应 []string params）
func CompileParams(params []string) ([]*Param, error) {
	ps := make([]*Param, len(params))
	for i, s := range params {
		p, err := CompileParam(s)
		if err != nil {
			return nil, err
		}
		ps[i] = p
	}
	return ps, nil
}

// EvalParams 运行期批量求值。单个参数求值失败 → 置空 + warn，继续执行；
// 数据缺失返回 ""（Env 约定，非错误）。
func EvalParams(req *bfe_basic.Request, params []*Param) []string {
	values := make([]string, len(params))
	for i, p := range params {
		v, err := p.Eval(req)
		if err != nil {
			logid := ""
			if req != nil {
				logid = req.LogId
			}
			log.Logger.Warn("bfe_expr: eval param %q error: %v (logid: %s)", p.src, err, logid)
			v = ""
		}
		values[i] = v
	}
	return values
}
