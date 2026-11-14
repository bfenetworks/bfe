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

/* conf_env.go - expression access to bfe.data tables (stub in phase 2)
/*
DESCRIPTION
	本期仅保留与 go-bfe-pro 一致的 StrTable/MapTable 接口面，实现为 stub：
	调用记 warn（未实现）并返回零值，不改变规则执行结果。
	表数据的配置加载 API（ApplyExprConf 等）本期不暴露。
*/

package bfe_expr

// ConfEnv 提供配置表数据的表达式访问（本期 stub）
type ConfEnv struct{}

// StrTable 按名字取 []string 表（本期 stub，返回 nil）
func (e *ConfEnv) StrTable(name string) []string {
	warnExprStub("StrTable")
	return nil
}

// MapTable 按名字取映射表中 key 的值（本期 stub，返回 ""）
func (e *ConfEnv) MapTable(name, key string) string {
	warnExprStub("MapTable")
	return ""
}
