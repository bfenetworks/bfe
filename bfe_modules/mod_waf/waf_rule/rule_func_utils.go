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

import (
	"strings"
)

// check if a char is a special char
func isSpecialChar(ch byte, chars []byte) bool {
	match := false
	for _, v := range chars {
		if ch == v {
			match = true
			break
		}
	}

	return match
}

// check if the given string contains a special word
func containWords(str string, words []string) bool {
	match := false
	for _, w := range words {
		if strings.Contains(str, w) {
			match = true
			break
		}
	}

	return match
}

// check if the given string match a special word
func matchWords(str string, words []string) bool {
	match := false
	for _, w := range words {
		if str == w {
			match = true
			break
		}
	}

	return match
}

// check "referer" and "x-requested-with" in headers
func hasReferer(req *RuleRequestInfo) bool {
	// check header "referer"
	referer, found := req.Headers["referer"]
	if !found || len(referer) == 0 {
		// try "x-requested-with"
		referer, found = req.Headers["x-requested-with"]
	}

	if found && len(referer) > 0 {
		return true
	}

	return false
}

// get url path from raw uri
func getUrlPath(req *RuleRequestInfo) string {
	urlPath := ""

	s := strings.Index(req.Uri, "/")

	if s >= 0 {
		e := strings.Index(req.Uri, "?")
		if e > 0 {
			urlPath = req.Uri[s:e]
		} else {
			urlPath = req.Uri[s:]
		}
	}

	return urlPath
}

// check if a string can be converted to a decimal number
func isInteger(s string) bool {
	if len(s) == 0 {
		return false
	}

	if s[0] == '+' || s[0] == '-' {
		s = s[1:]
	}

	if len(s) == 0 {
		return false
	}

	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			continue
		} else {
			return false
		}
	}

	return true
}

// split path from value string
func splitPath(value string) string {
	pos := strings.LastIndex(value, "/")

	if pos < 0 {
		return ""
	} else {
		path_pos := strings.LastIndex(value[0:pos], "/")
		if path_pos < 0 {
			return value[0:pos]
		} else {
			return value[path_pos+1 : pos]
		}
	}
}

// convert value to lower case
func convertToLower(keyValues map[string][]string) map[string][]string {
	lowerKeyValues := make(map[string][]string)
	for key, values := range keyValues {
		for _, value := range values {
			lowerCase := strings.ToLower(value)
			lowerKeyValues[key] = append(lowerKeyValues[key], lowerCase)
		}
	}

	return lowerKeyValues
}

// check char whether is [a-z_] or not
func isAZ(ch byte) bool {
	if (ch >= 'a' && ch <= 'z') || (ch == '_') {
		return true
	}

	return false
}

// check char whether is [a-z0-9_] or not
func isAZ09(ch byte) bool {
	if isAZ(ch) || (ch >= '0' && ch <= '9') {
		return true
	}

	return false
}

// check string whether is consist of [a-z0-9_] or not
func isAZ09s(value string) bool {
	lenWord := len(value)
	if lenWord == 0 {
		return false
	}

	for i := 0; i < lenWord; i++ {
		if !isAZ09(value[i]) {
			return false
		}
	}

	return true
}

// isBlankSubString - check substring whether is consist of blank charactors or not
//
// Params:
//      - str   : master string
//      - first : substring start index
//      - last  : substring end index + 1
//
// Returns:
//      - bool  : is consist of blank chars, return true; else truen false
func isBlankSubString(str string, first, last int) bool {
	if first >= last || len(str) <= first || first < 0 {
		return false
	}

	if len(str) < last {
		last = len(str)
	}
	blanks := []byte{' ', '\t', '\n', '\r', '\f', '\v'}

	for i := first; i < last; i++ {
		if !isSpecialChar(str[i], blanks) {
			return false
		}
	}

	return true
}

// findLastSuffix - find token from last to begin in string slice
//
// Params:
//      - words : destination strings
//      - token : search string
//
// Return:
//      - int   : find it , return index; else -1
func findLastSuffix(words []string, token string) int {
	if len(words) == 0 || len(token) == 0 {
		return -1
	}

	for i := len(words) - 1; i >= 0; i-- {
		if strings.HasSuffix(words[i], token) {
			return i
		}
	}

	return -1
}

// getLevel - get level for rule RULE_SQL_INJECTION(_POST) and RULE_FILE_INCLUDE
//
// Params:
//      - req : request
//      - runeName : rule name
//
// Return:
//      - int: level
func getLevel(req *RuleRequestInfo, ruleName string) int {
	level := 1

	// check "referer"
	if hasReferer(req) {
		level = 2
	}

	return level
}

// remove space, \t, \n, \r
func removeWhitespace(str string) []byte {
	newStr := make([]byte, len(str))
	pos := 0
	for _, c := range str {
		// ignore space, \t, \n, \r
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			newStr[pos] = byte(c)
			pos++
		}
	}

	return newStr
}
