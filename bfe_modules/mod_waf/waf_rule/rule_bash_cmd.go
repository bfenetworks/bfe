// Copyright (c) 2020 The BFE Authors.
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

package waf_rule

import "strings"

type RuleBashCmdExe struct {
}

func NewRuleBashCmdExe() *RuleBashCmdExe {
	rule := new(RuleBashCmdExe)
	return rule
}

func (rule *RuleBashCmdExe) Init() error {
	return nil
}

func (rule *RuleBashCmdExe) Check(req *RuleRequestInfo) bool {
	return ruleBashCmdExeCheck(req)
}

func (rule *RuleBashCmdExe) CheckString(pStr *string) bool {
	return checkHeaderValue(*pStr)
}

// check if first non-space/tab char is ";"
func checkSemicolon(value string) bool {
	length := len(value)

	for i := 0; i < length; i++ {
		if value[i] == ' ' || value[i] == '\t' {
			continue
		} else if value[i] != ';' {
			return false
		} else {
			return true
		}
	}

	return false
}

// check if header value content matches the spedific rules
func checkHeaderValueContent(value string) bool {
	index := strings.Index(value, "}")
	if index != -1 {
		if checkSemicolon(value[index+1:]) {
			return true
		}
	}

	return false
}

// check if value started with the specific char
func checkSpecificChar(value string, c string) (int, bool) {
	length := len(value)

	for i := 0; i < length; i++ {
		if value[i] == ' ' || value[i] == '\t' {
			continue
		} else if value[i] != c[0] {
			return -1, false
		} else {
			return i, true
		}
	}

	return -1, false
}

// check if header value matches "^\s+\(\s+\)\s+{"
func checkHeaderValuePrefix(value string) (int, bool) {
	var index, gIndex int
	var hit bool

	index, hit = checkSpecificChar(value[gIndex:], "(")
	if !hit {
		return -1, false
	}

	gIndex += index + 1
	index, hit = checkSpecificChar(value[gIndex:], ")")
	if !hit {
		return -1, false
	}

	gIndex += index + 1
	index, hit = checkSpecificChar(value[gIndex:], "{")
	if !hit {
		return -1, false
	}

	gIndex += index
	return gIndex, true
}

// check header value
func checkHeaderValue(value string) bool {

	index, hit := checkHeaderValuePrefix(value)
	if hit {
		if checkHeaderValueContent(value[index+1:]) {
			return true
		}
	}

	return false
}

func ruleBashCmdExeCheck(req *RuleRequestInfo) bool {

	for _, values := range req.Headers {
		for _, value := range values {
			if checkHeaderValue(value) {
				return true
			}
		}
	}

	return false
}
