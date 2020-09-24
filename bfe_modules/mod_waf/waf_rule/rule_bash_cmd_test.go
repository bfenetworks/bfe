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

import "testing"

// hit cases
func TestCheckSemicolonCase0(t *testing.T) {
	var s string

	s = ";"
	if !checkSemicolon(s) {
		t.Errorf("TestCheckSemicolonCase0(): string \"%s\" should hit!", s)
	}

	s = " ;"
	if !checkSemicolon(s) {
		t.Errorf("TestCheckSemicolonCase0(): string \"%s\" should hit!", s)
	}

	s = "\t;"
	if !checkSemicolon(s) {
		t.Errorf("TestCheckSemicolonCase0(): string \"%s\" should hit!", s)
	}

	s = " \t \t;"
	if !checkSemicolon(s) {
		t.Errorf("TestCheckSemicolonCase0(): string \"%s\" should hit!", s)
	}
}

// no hit cases
func TestCheckSemicolonCase1(t *testing.T) {
	var s string

	s = ""
	if checkSemicolon(s) {
		t.Errorf("TestCheckSemicolonCase1(): string \"%s\" should not hit!", s)
	}

	s = "123"
	if checkSemicolon(s) {
		t.Errorf("TestCheckSemicolonCase1(): string \"%s\" should not hit!", s)
	}

	s = "a;"
	if checkSemicolon(s) {
		t.Errorf("TestCheckSemicolonCase1(): string \"%s\" should not hit!", s)
	}

	s = "a ;"
	if checkSemicolon(s) {
		t.Errorf("TestCheckSemicolonCase1(): string \"%s\" should not hit!", s)
	}
}

// hit cases
func TestCheckHeaderValueContentCase0(t *testing.T) {
	var s string

	s = "};"
	if !checkHeaderValueContent(s) {
		t.Errorf("TestCheckHeaderValueContentCase0(): string \"%s\" should hit!", s)
	}

	s = "12} ;"
	if !checkHeaderValueContent(s) {
		t.Errorf("TestCheckHeaderValueContentCase0(): string \"%s\" should hit!", s)
	}

	s = " }\t;"
	if !checkHeaderValueContent(s) {
		t.Errorf("TestCheckHeaderValueContentCase0(): string \"%s\" should hit!", s)
	}
}

// no hit cases
func TestCheckHeaderValueContentCase1(t *testing.T) {
	var s string

	s = ""
	if checkHeaderValueContent(s) {
		t.Errorf("TestCheckHeaderValueContentCase1(): string \"%s\" should not hit!", s)
	}

	s = "}"
	if checkHeaderValueContent(s) {
		t.Errorf("TestCheckHeaderValueContentCase1(): string \"%s\" should not hit!", s)
	}

	s = " }\t1;"
	if checkHeaderValueContent(s) {
		t.Errorf("TestCheckHeaderValueContentCase1(): string \"%s\" should not hit!", s)
	}

	s = " }1;"
	if checkHeaderValueContent(s) {
		t.Errorf("TestCheckHeaderValueContentCase1(): string \"%s\" should not hit!", s)
	}
}

// hit cases
func TestCheckSpecificCharCase0(t *testing.T) {
	var s string
	var c string
	var i int
	var hit bool

	c = "("

	s = "("
	i, hit = checkSpecificChar(s, c)
	if !hit || i != 0 {
		t.Errorf("TestCheckSpecificCharCase0(): string \"%s\" should hit!", s)
	}

	s = " ("
	i, hit = checkSpecificChar(s, c)
	if !hit || i != 1 {
		t.Errorf("TestCheckSpecificCharCase0(): string \"%s\" should hit!", s)
	}

	s = "\t("
	i, hit = checkSpecificChar(s, c)
	if !hit || i != 1 {
		t.Errorf("TestCheckSpecificCharCase0(): string \"%s\" should hit!", s)
	}

	s = " \t("
	i, hit = checkSpecificChar(s, c)
	if !hit || i != 2 {
		t.Errorf("TestCheckSpecificCharCase0(): string \"%s\" should hit!", s)
	}
}

// no hit cases
func TestCheckSpecificCharCase1(t *testing.T) {
	var s string
	var c string
	var hit bool

	c = "("

	s = ""
	_, hit = checkSpecificChar(s, c)
	if hit {
		t.Errorf("TestCheckSpecificCharCase1(): string \"%s\" should  no thit!", s)
	}

	s = "i"
	_, hit = checkSpecificChar(s, c)
	if hit {
		t.Errorf("TestCheckSpecificCharCase1(): string \"%s\" should  no thit!", s)
	}

	s = "1("
	_, hit = checkSpecificChar(s, c)
	if hit {
		t.Errorf("TestCheckSpecificCharCase1(): string \"%s\" should  no thit!", s)
	}

	s = " 1("
	_, hit = checkSpecificChar(s, c)
	if hit {
		t.Errorf("TestCheckSpecificCharCase1(): string \"%s\" should  no thit!", s)
	}

	s = "1 ("
	_, hit = checkSpecificChar(s, c)
	if hit {
		t.Errorf("TestCheckSpecificCharCase1(): string \"%s\" should  no thit!", s)
	}
}

// hit cases
func TestCheckHeaderValuePrefixCase0(t *testing.T) {
	var s string
	var hit bool
	var i int

	s = "(){"
	i, hit = checkHeaderValuePrefix(s)
	if !hit || i != 2 {
		t.Errorf("TestCheckHeaderValuePrefixCase0(): string \"%s\" should hit!", s)
	}

	s = " (){"
	i, hit = checkHeaderValuePrefix(s)
	if !hit || i != 3 {
		t.Errorf("TestCheckHeaderValuePrefixCase0(): string \"%s\" should hit!", s)
	}

	s = " (\t){"
	i, hit = checkHeaderValuePrefix(s)
	if !hit || i != 4 {
		t.Errorf("TestCheckHeaderValuePrefixCase0(): string \"%s\" should hit!", s)
	}

	s = " ( )\t{"
	i, hit = checkHeaderValuePrefix(s)
	if !hit || i != 5 {
		t.Errorf("TestCheckHeaderValuePrefixCase0(): string \"%s\" should hit!", s)
	}

	s = "\t( ) { "
	i, hit = checkHeaderValuePrefix(s)
	if !hit || i != 5 {
		t.Errorf("TestCheckHeaderValuePrefixCase0(): string \"%s\" should hit!", s)
	}
}

// no hit cases
func TestCheckHeaderValuePrefix_case1(t *testing.T) {
	var s string
	var hit bool

	s = ""
	_, hit = checkHeaderValuePrefix(s)
	if hit {
		t.Errorf("TestCheckHeaderValuePrefix_case1(): string \"%s\" should not hit!", s)
	}

	s = "1(){"
	_, hit = checkHeaderValuePrefix(s)
	if hit {
		t.Errorf("TestCheckHeaderValuePrefix_case1(): string \"%s\" should not hit!", s)
	}

	s = " (1){"
	_, hit = checkHeaderValuePrefix(s)
	if hit {
		t.Errorf("TestCheckHeaderValuePrefix_case1(): string \"%s\" should not hit!", s)
	}

	s = " ()x{"
	_, hit = checkHeaderValuePrefix(s)
	if hit {
		t.Errorf("TestCheckHeaderValuePrefix_case1(): string \"%s\" should not hit!", s)
	}
}

// hit cases
func TestCheckHeaderValue_case0(t *testing.T) {
	var s string
	var hit bool

	s = "(){};"
	hit = checkHeaderValue(s)
	if !hit {
		t.Errorf("TestCheckHeaderValuePrefixCase0(): string \"%s\" should hit!", s)
	}

	s = "(){xx};"
	hit = checkHeaderValue(s)
	if !hit {
		t.Errorf("TestCheckHeaderValuePrefixCase0(): string \"%s\" should hit!", s)
	}

	s = "(){xx} ;"
	hit = checkHeaderValue(s)
	if !hit {
		t.Errorf("TestCheckHeaderValuePrefixCase0(): string \"%s\" should hit!", s)
	}

	s = "(){xx}\t;"
	hit = checkHeaderValue(s)
	if !hit {
		t.Errorf("TestCheckHeaderValuePrefixCase0(): string \"%s\" should hit!", s)
	}

	s = "(){xx} \t;"
	hit = checkHeaderValue(s)
	if !hit {
		t.Errorf("TestCheckHeaderValuePrefixCase0(): string \"%s\" should hit!", s)
	}
}

// no hit cases
func TestCheckHeaderValueCase1(t *testing.T) {
	var s string
	var hit bool

	s = "(){}1;"
	hit = checkHeaderValue(s)
	if hit {
		t.Errorf("TestCheckHeaderValuePrefixCase0(): string \"%s\" should not hit!", s)
	}

	s = "(){;"
	hit = checkHeaderValue(s)
	if hit {
		t.Errorf("TestCheckHeaderValuePrefixCase0(): string \"%s\" should not hit!", s)
	}

	s = "(){}"
	hit = checkHeaderValue(s)
	if hit {
		t.Errorf("TestCheckHeaderValuePrefixCase0(): string \"%s\" should not hit!", s)
	}
}
