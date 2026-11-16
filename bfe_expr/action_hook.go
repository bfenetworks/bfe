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

/* action_hook.go - inject action expr-params compiler into bfe_basic/action
/*
DESCRIPTION
	bfe_basic/action 不能反向依赖本包（inner_env.go 依赖其 VariableHandlers，
	构成依赖环），故通过 SetExprParamCompiler 注入表达式参数编译器。
	动作参数表达式规范见 docs/zh_cn/sys_design/expr_condition_engine.md §8：
	"=" 前缀为表达式（返回值 string），"==" 转义字面量，加载期严格报错。
*/

package bfe_expr

import (
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/action"
	"github.com/bfenetworks/go-lib/log"
)

func init() {
	action.SetExprParamCompiler(compileActionParams)
}

// compileActionParams 实现 action.SetExprParamCompiler 的注入签名：
// 编译 indexes 下标中以 '=' 开头的参数；无表达式参数返回 (nil, nil)。
// 编译失败返回错误（加载期严格报错）。
func compileActionParams(params []string, indexes []int) (func(req *bfe_basic.Request) []string, error) {
	compiled := make(map[int]*Param)
	for _, i := range indexes {
		if i >= 0 && i < len(params) && strings.HasPrefix(params[i], exprPrefix) {
			p, err := CompileParam(params[i])
			if err != nil {
				return nil, err
			}
			compiled[i] = p
		}
	}
	if len(compiled) == 0 {
		return nil, nil
	}

	return func(req *bfe_basic.Request) []string {
		out := make([]string, len(params))
		copy(out, params)
		for i, p := range compiled {
			v, err := p.Eval(req)
			if err != nil {
				logid := ""
				if req != nil {
					logid = req.LogId
				}
				// 求值失败 → 置空继续
				log.Logger.Warn("bfe_expr: eval action param %q error: %v (logid: %s)", p.src, err, logid)
				v = ""
			}
			out[i] = v
		}
		return out
	}, nil
}
