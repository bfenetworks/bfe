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

/* var_env.go - request context & user variables
/*
DESCRIPTION
	用户变量读写：typed setter 把原生类型（int/float64/bool）直接存入请求
	Context（key 前缀与 legacy USR_VAR_SET 相同，存储类型不同互不覆盖）；
	typed getter 类型断言优先、string 存储解析兜底，缺失/解析失败返回零值。
	legacy GetUsrVariable 只认 string 存储，不会误读 typed 变量。
*/

package bfe_expr

import (
	"strconv"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
)

type VarEnv struct {
	req *bfe_basic.Request
}

// ReqContext 按 key 读取请求 Context 值，不存在返回 nil
func (e *VarEnv) ReqContext(key interface{}) interface{} {
	if e.req == nil || e.req.Context == nil {
		return nil
	}
	return e.req.GetContext(key)
}

func (e *VarEnv) usrVarKey(name string) string {
	return bfe_basic.UsrVaraibleNamePrifix + name
}

func (e *VarEnv) getUsrVar(name string) (interface{}, bool) {
	if e.req == nil || e.req.Context == nil {
		return nil, false
	}
	val, ok := e.req.Context[e.usrVarKey(name)]
	return val, ok
}

// UsrVarStr 读取 string 型用户变量，不存在或非 string 存储返回 ""
func (e *VarEnv) UsrVarStr(name string) string {
	val, ok := e.getUsrVar(name)
	if !ok {
		return ""
	}
	str, ok := val.(string)
	if !ok {
		return ""
	}
	return str
}

// UsrVarInt 读取 int 型用户变量：原生 int 直取；string 存储则 Atoi 解析；
// 缺失/类型不符/解析失败返回 0
func (e *VarEnv) UsrVarInt(name string) int {
	val, ok := e.getUsrVar(name)
	if !ok {
		return 0
	}
	switch v := val.(type) {
	case int:
		return v
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		return 0
	default:
		return 0
	}
}

// UsrVarFloat 读取 float64 型用户变量：原生 float64 直取；string 存储则 ParseFloat 解析；
// 缺失/类型不符/解析失败返回 0
func (e *VarEnv) UsrVarFloat(name string) float64 {
	val, ok := e.getUsrVar(name)
	if !ok {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return v
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		return 0
	default:
		return 0
	}
}

// UsrVarBool 读取 bool 型用户变量：原生 bool 直取；string 存储按 "true"/"false" 解析；
// 缺失/类型不符/解析失败返回 false
func (e *VarEnv) UsrVarBool(name string) bool {
	val, ok := e.getUsrVar(name)
	if !ok {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case string:
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
		return false
	default:
		return false
	}
}

// UsrVarSetStr 写入 string 型用户变量（与 legacy USR_VAR_SET 同一存储），返回写入值
func (e *VarEnv) UsrVarSetStr(name, value string) string {
	if e.req != nil {
		e.req.SetUsrVariable(name, value)
	}
	return value
}

// UsrVarSetInt 写入 int 型用户变量（原生类型入 Context），返回写入值
func (e *VarEnv) UsrVarSetInt(name string, value int) int {
	if e.req != nil && e.req.Context != nil {
		e.req.SetContext(e.usrVarKey(name), value)
	}
	return value
}

// UsrVarSetFloat 写入 float64 型用户变量，返回写入值
func (e *VarEnv) UsrVarSetFloat(name string, value float64) float64 {
	if e.req != nil && e.req.Context != nil {
		e.req.SetContext(e.usrVarKey(name), value)
	}
	return value
}

// UsrVarSetBool 写入 bool 型用户变量，返回写入值
func (e *VarEnv) UsrVarSetBool(name string, value bool) bool {
	if e.req != nil && e.req.Context != nil {
		e.req.SetContext(e.usrVarKey(name), value)
	}
	return value
}

// UsrVar 是 UsrVarStr 的简写
func (e *VarEnv) UsrVar(name string) string {
	return e.UsrVarStr(name)
}

// UsrVarSet 是 UsrVarSetStr 的简写
func (e *VarEnv) UsrVarSet(name, value string) string {
	return e.UsrVarSetStr(name, value)
}
