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

// variable condition implementation
/**
Add variable condition and set in bfe`s condition match;
Diff from primitive condition which fetch and match, variable condition deal request in three steps:
- fetch info from request;
- match request with the fetched info
- extract variable from request
Here is the example:
`req_path_with_vars_in("/users/{userId}")` and request's path is /users/123, then the steps are:
- get path from request. path=/users/123;
- match path. /users/123 match /users/{userId};
- extract vars. get vars userId=123 and store vars in request context(detail see: req.SetVar);
*/

package condition

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_basic/condition/parser"
)

// VariableMatcher, which not only match request bu also extract vars from request;
type VariableMatcher interface {
	Match(interface{}) bool                             // match func
	ExtractVariable(interface{}) map[string]interface{} //extract func
}

// VariableCond, diff from primitive cond, which also extract and store vars
type VariableCond struct {
	name    string
	node    *parser.CallExpr
	fetcher Fetcher
	matcher VariableMatcher
}

func (c *VariableCond) String() string {
	return c.node.String()
}

func (c *VariableCond) Match(req *bfe_basic.Request) bool {
	if req == nil || req.Session == nil || req.HttpRequest == nil || req.Context == nil {
		return false
	}

	// step1: fetch
	fetched, err := c.fetcher.Fetch(req)
	if err != nil {
		return false
	}

	// step2: match
	matched := c.matcher.Match(fetched)
	if !matched {
		return false
	}
	// step3: extract and store
	vars := c.matcher.ExtractVariable(fetched)
	if vars == nil {
		return false
	}
	c.setRequestVars(req, vars)
	return true
}

func (c *VariableCond) setRequestVars(req *bfe_basic.Request, vars map[string]interface{}) {
	for key, value := range vars {
		req.SetVar(key, value)
	}
}

// variableRegexp, store the original str and the regexp after build
type variableRegexp struct {
	originalStr string         // original str which record the str we want to build; lke /user/{userId}
	patternStr  string         // pattern str which record the str  we used to build; like /user/(?P<v0>[^/]+);
	varNames    []string       // varNames which record the name of user defined; like ["userId"]
	regexp      *regexp.Regexp //regexp which is the regexp after build the pattern str;
}

func (reg *variableRegexp) Match(s string) bool {
	if reg == nil || reg.regexp == nil {
		return false
	}
	return reg.regexp.MatchString(s)
}

func (reg *variableRegexp) ExtractVariable(s string) map[string]interface{} {
	if reg == nil || reg.regexp == nil {
		return nil
	}

	vIndex := reg.regexp.FindStringSubmatchIndex(s)

	if vIndex == nil {
		return nil
	}

	if len(reg.varNames) != len(vIndex)/2-1 {
		return nil
	}

	var varMap = make(map[string]interface{})
	for i, name := range reg.varNames {
		varMap[name] = s[vIndex[2*i+2]:vIndex[2*i+3]]
	}
	return varMap
}

func NewPathVariableMatcher(tpl string, isPrefixType bool) (VariableMatcher, error) {
	p := new(pathVariableMatcher)
	err := p.Init(tpl, isPrefixType)
	if err != nil {
		return nil, err
	}
	return p, err
}

type pathVariableMatcher struct {
	vRegexp *variableRegexp
}

func (p *pathVariableMatcher) Init(tpl string, isPrefixType bool) error {
	vRegexp, err := buildPathVariableRegexp(tpl, isPrefixType)
	if err != nil {
		return err
	}
	p.vRegexp = vRegexp
	return nil
}

func (p *pathVariableMatcher) Match(path interface{}) bool {
	pathStr, ok := path.(string)
	if !ok {
		return false
	}
	if p.vRegexp == nil || p.vRegexp.regexp == nil {
		return false
	}
	return p.vRegexp.Match(pathStr)
}

func (p *pathVariableMatcher) ExtractVariable(path interface{}) map[string]interface{} {
	pathStr, ok := path.(string)
	if !ok {
		return nil
	}
	if p.vRegexp == nil {
		return nil
	}
	return p.vRegexp.ExtractVariable(pathStr)
}

func braceIndices(s string) ([]int, error) {
	var indices = make([]int, 0)
	var err = fmt.Errorf("braceIndices() error: invalid brace sequence[%s]", s)
	left, right := 0, 1
	braceIndex := left
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			if braceIndex == left {
				indices = append(indices, i)
				braceIndex = right
			} else {
				return nil, err
			}
		case '}':
			if braceIndex == right {
				indices = append(indices, i)
				braceIndex = left
			} else {
				return nil, err
			}
		}
	}
	if braceIndex != left {
		return nil, err
	}
	return indices, nil
}

// buildPathVariableRegexp is used to build varRegexp. This function is inspired by gorilla/mux.
// tpl is the str like /user/{userId}, note: we just support to use brace to wrapper variable name.
// here is the brief description:
// 1. get all the location of char '{' and '}'
// 2. get the raw str before '{', varName between '{' and '}'
// 3. varname store in varNames and we change the varname to regexp str '(?P<vN>[^/]+)', note: avoid of some unexpected
// groupName, we user vN as the variable group name, like v0, v1
// 4. we build the regexp str to build variableRegexp
func buildPathVariableRegexp(tpl string, isPrefixType bool) (*variableRegexp, error) {
	braces, err := braceIndices(tpl)
	if err != nil {
		return nil, err
	}

	pattern := bytes.NewBufferString("")
	pattern.WriteByte('^')
	defaultRegPattern := "[^/]+"

	headerIndex := 0
	varNameMap := make(map[string]bool)

	var vars = make([]string, len(braces)/2)
	for i := 0; i < len(braces)/2; i++ {
		leftBraceIndex := braces[2*i]
		rightBraceIndex := braces[2*i+1]

		rawStr := tpl[headerIndex:leftBraceIndex]
		varName := tpl[leftBraceIndex+1 : rightBraceIndex]

		if varName == "" {
			return nil, fmt.Errorf("buildPathVariableRegexp() empty varname, tpl: %s", tpl)
		}
		if _, exists := varNameMap[varName]; exists {
			return nil, fmt.Errorf("buildPathVariableRegexp() duplicate varname, tpl: %s", tpl)
		}
		varNameMap[varName] = true

		headerIndex = rightBraceIndex + 1
		vars[i] = varName
		patternV := "v" + strconv.Itoa(i)
		fmt.Fprintf(pattern, "%s(?P<%s>%s)", regexp.QuoteMeta(rawStr), patternV, defaultRegPattern)
	}

	endStr := tpl[headerIndex:]
	fmt.Fprintf(pattern, "%s", endStr)

	if !isPrefixType {
		fmt.Fprintf(pattern, "$")
	}

	reg, err := regexp.Compile(pattern.String())
	if err != nil {
		return nil, err
	}
	return &variableRegexp{
		originalStr: tpl,
		varNames:    vars,
		regexp:      reg,
		patternStr:  pattern.String(),
	}, nil
}
