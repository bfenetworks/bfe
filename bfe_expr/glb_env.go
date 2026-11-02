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

/* glb_env.go - process-level user variables (stub in phase 2)
/*
DESCRIPTION
	本期仅保留与 go-bfe-pro 一致的 GlbVar* 接口面，实现为 stub：
	调用记 warn（未实现）并返回零值，不改变规则执行结果。
*/

package bfe_expr

// GlbEnv 进程级用户变量（本期 stub）
type GlbEnv struct{}

// GlbVar 读取 string 型变量（本期 stub，返回 ""）
func (e *GlbEnv) GlbVar(name string) string {
	warnExprStub("GlbVar")
	return ""
}

// GlbVarStr 读取 string 型变量（本期 stub，返回 ""）
func (e *GlbEnv) GlbVarStr(name string) string {
	warnExprStub("GlbVarStr")
	return ""
}

// GlbVarInt 读取 int 型变量（本期 stub，返回 0）
func (e *GlbEnv) GlbVarInt(name string) int {
	warnExprStub("GlbVarInt")
	return 0
}

// GlbVarFloat 读取 float64 型变量（本期 stub，返回 0）
func (e *GlbEnv) GlbVarFloat(name string) float64 {
	warnExprStub("GlbVarFloat")
	return 0
}

// GlbVarBool 读取 bool 型变量（本期 stub，返回 false）
func (e *GlbEnv) GlbVarBool(name string) bool {
	warnExprStub("GlbVarBool")
	return false
}

// GlbVarSetStr 写入 string 型变量（本期 stub，返回写入值）
func (e *GlbEnv) GlbVarSetStr(name, value string) string {
	warnExprStub("GlbVarSetStr")
	return value
}

// GlbVarSetInt 写入 int 型变量（本期 stub，返回写入值）
func (e *GlbEnv) GlbVarSetInt(name string, value int) int {
	warnExprStub("GlbVarSetInt")
	return value
}

// GlbVarSetFloat 写入 float64 型变量（本期 stub，返回写入值）
func (e *GlbEnv) GlbVarSetFloat(name string, value float64) float64 {
	warnExprStub("GlbVarSetFloat")
	return value
}

// GlbVarSetBool 写入 bool 型变量（本期 stub，返回写入值）
func (e *GlbEnv) GlbVarSetBool(name string, value bool) bool {
	warnExprStub("GlbVarSetBool")
	return value
}

// GlbVarIncr 计数加一并返回新值（本期 stub，返回 0）
func (e *GlbEnv) GlbVarIncr(name string) int {
	warnExprStub("GlbVarIncr")
	return 0
}

// GlbVarIncrBy 计数加 delta 并返回新值（本期 stub，返回 0）
func (e *GlbEnv) GlbVarIncrBy(name string, delta int) int {
	warnExprStub("GlbVarIncrBy")
	return 0
}

// GlbVarDecr 计数减一并返回新值（本期 stub，返回 0）
func (e *GlbEnv) GlbVarDecr(name string) int {
	warnExprStub("GlbVarDecr")
	return 0
}
