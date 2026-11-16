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

/* program.go - compile & run of new-style condition expressions
/*
DESCRIPTION
	新风格条件表达式的编译与执行：
	- 编译期 expr.Env(&Env{}) 强类型检查 + AsBool 结果约束，配置加载时出错；
	- 运行期每请求取请求级 Env（GetEnv），求值出错按 false 处理并输出 warn 日志。
*/

package bfe_expr

import (
	"fmt"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/go-lib/log"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Condition 新风格条件表达式，实现 condition.Condition 接口（Match）
type Condition struct {
	prog *vm.Program
	src  string
}

// Compile 编译新风格条件表达式；结果必须是 bool 类型
func Compile(src string) (*Condition, error) {
	return compile(src)
}

// CompileLegacy 编译兼容旧原语的条件串：
// legacyPatcher 把旧原语名重写为 Env 方法后统一走新引擎，旧/新/混写均可。
// 先经 ValidateLegacyArgs 做旧原语字面量参数校验（对齐旧实现加载期拒绝非法配置的行为）。
// 编译失败时由调用方决定是否回退旧引擎。
func CompileLegacy(src string) (*Condition, error) {
	if err := ValidateLegacyArgs(src); err != nil {
		return nil, err
	}
	return compile(src, expr.Patch(&legacyPatcher{}))
}

func compile(src string, opts ...expr.Option) (*Condition, error) {
	opts = append([]expr.Option{expr.Env(&Env{}), expr.AsBool()}, opts...)
	prog, err := expr.Compile(src, opts...)
	if err != nil {
		return nil, fmt.Errorf("expr compile %q: %v", src, err)
	}
	return &Condition{prog: prog, src: src}, nil
}

// MustCompile 同 Compile，编译失败时 panic（仅用于测试和确定的常量表达式）
func MustCompile(src string) *Condition {
	c, err := Compile(src)
	if err != nil {
		panic(err)
	}
	return c
}

func (c *Condition) String() string {
	return c.src
}

// Match 求值：运行期错误按 false 处理并输出 warn 日志，不影响请求处理
func (c *Condition) Match(req *bfe_basic.Request) bool {
	env := GetEnv(req)
	if env == nil {
		return false
	}
	out, err := expr.Run(c.prog, env)
	if err != nil {
		log.Logger.Warn("bfe_expr: eval %q error: %v (logid: %s)", c.src, err, req.LogId)
		return false
	}
	result, ok := out.(bool)
	if !ok {
		log.Logger.Warn("bfe_expr: eval %q result is not bool (logid: %s)", c.src, req.LogId)
		return false
	}
	return result
}
