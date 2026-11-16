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

/*
action_variable.go - variable name legality check and variable handlers
*/
package action

import (
	"fmt"
	"net/textproto"
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic"
)

const variableCharset = "abcdefghijklmnopqrstuvwxyz0123456789_"

const (
	ParamTypeLiteral = iota
	ParamTypeVarLegacy
	ParamTypeVarNew
)

/*
format string contains the following style substring:
ParamTypeVarLegacy: eg:%bfe_cluster
ParamTypeVarNew:
--new not-param var: eg:%{bfe_cluster},
--new param var: eg:%{bfe_req_header::headername}, %{usr_variable::rname}
ParamTypeLiteral:
*/
func ParamType(param string) int {
	if strings.HasPrefix(param, "%") {
		if strings.HasPrefix(param, "%%") {
			return ParamTypeLiteral
		}
		if strings.HasPrefix(param, "%{") {
			// if strings.HasSuffix(param, "%}") {
			// 	return ParamTypeLiteral
			// } else
			if strings.HasSuffix(param, "}") {
				return ParamTypeVarNew
			} else {
				return ParamTypeLiteral
			}
		}
		return ParamTypeVarLegacy
	}
	return ParamTypeLiteral
}

func IsLegalVarName(varName string) bool {
	if len(varName) <= 0 {
		return false
	}

	for _, c := range varName {
		if !strings.Contains(variableCharset, string(c)) {
			return false
		}
	}

	return true
}

// expectPercent returnes index of '%', otherwise return
// last index of str
func expectPercent(str string) int {
	index := 0
	for _, c := range str {
		if c != '%' {
			index++
			continue
		}
		break
	}

	return index
}

// expectRightBrace returnes index of '}' + 1 or size of str
func expectRightBrace(str string) int {
	index := 0
	for _, c := range str {
		if c != '}' {
			index++
			continue
		}

		//%}
		// if index > 0 && str[index-1] == '%' {
		// 	index++
		// 	continue
		// }

		index++
		break
	}
	return index
}

func expectVariableParam(str string) int {
	index := 0
	// variable param, variable now only has
	// characters [a-z] and '_'
	for _, c := range str {
		if !strings.Contains(variableCharset, string(c)) {
			break
		}
		index++
	}

	return index
}

// splitParam splits string "__bsi=%bfe_ssl_info;max-age=3600" to separate strings
// "__bsi="
// "%bfe_ssl_info"
// ";max-age=3600"
// escape literal: %%
func splitFormatStr(param string) []string {
	params := make([]string, 0)
	paramBegin := 0
	index := 0

	for {
		paramBegin = index
		if paramBegin >= len(param) {
			break
		}
		// normal text
		if param[index] != '%' {
			index++
			index += expectPercent(param[index:])
		} else if index++; index < len(param) {
			if param[index] == '%' {
				// escape text, start with %%...
				index++
				index += expectPercent(param[index:])
			} else if param[index] == '{' {
				// newstyle variable param, maybe
				index++
				index += expectRightBrace(param[index:])
			} else {
				// lagacy style variable param
				index += expectVariableParam(param[index:])
			}
		}

		params = append(params, param[paramBegin:index])
	}

	return params
}

func PreprocessFormatStr(param string) ([]string, error) {
	params := splitFormatStr(param)

	for idx, _ := range params {
		pt := ParamType(params[idx])
		var err error
		switch pt {
		case ParamTypeVarLegacy:
			err = checkLegacyStyleVar(params[idx])
		case ParamTypeVarNew:
			err = checkNewStyleVar(params[idx])
			if err == nil {
				p2 := adjustNewStyleVarName(params[idx])
				params[idx] = p2
			}
		default:
			err = nil
		}
		if err != nil {
			return params, err
		}
	}

	return params, nil
}

func EvalFormatStr(req *bfe_basic.Request, params []string) string {
	var value string

	if len(params) < 1 {
		return ""
	}

	// concatenate sepate strings to a whole one
	// if a variable param, decode it first
	for _, p := range params {
		pt := ParamType(p)
		var tmp string
		switch pt {
		case ParamTypeVarLegacy:
			tmp = evalLegacyStyleVar(p, req)
		case ParamTypeVarNew:
			tmp = evalNewStyleVar(p, req)
		default:
			if strings.HasPrefix(p, "%") {
				tmp = p[1:]
			} else {
				tmp = p
			}
		}

		value += tmp
	}

	return value
}

type VarEvaluator interface {
	check(param string) error
	eval(param string, req *bfe_basic.Request) string
}

type innerVarEvaluator struct {
}

func (p *innerVarEvaluator) check(param string) error {
	if len(param) <= 3 {
		return fmt.Errorf("command's param is not valid: %s", param)
	}
	if param[0:2] != "%{" {
		return fmt.Errorf("command's param is not valid: %s", param)
	}

	varName := param[2 : len(param)-1]
	if _, found := VariableHandlers[varName]; !found {
		// not valid header value variable
		return fmt.Errorf("command's param is not valid: %s", param)
	}
	return nil
}

func (p *innerVarEvaluator) eval(param string, req *bfe_basic.Request) string {
	varName := param[2 : len(param)-1]
	if handler, found := VariableHandlers[varName]; found {
		return handler(req)
	}
	return ""
}

func isParamVar(param string) bool {
	idx := strings.Index(param, "::")
	return (idx != -1)
}

func splitParamVar(param string) (string, string, error) {
	if len(param) <= 6 {
		return "", "", fmt.Errorf("paramVar is not valid: %s", param)
	}
	if param[0:2] != "%{" {
		return "", "", fmt.Errorf("paramVar is not valid: %s", param)
	}

	varName := param[2 : len(param)-1]
	idx := strings.Index(varName, "::")
	if idx == -1 {
		return "", "", fmt.Errorf("paramVar is not valid: %s", param)
	}
	sname := varName[0:idx]
	fname := varName[idx+2:]
	if len(sname) < 1 || len(fname) < 1 {
		return "", "", fmt.Errorf("paramVar is not valid: %s", param)
	}

	return sname, fname, nil
}

func joinParamVar(sname, fname string) string {
	return fmt.Sprintf("%%{%s::%s}", sname, fname)
}

type innerParamVarEvaluator struct {
}

func (p *innerParamVarEvaluator) adjustParVarName(param string) string {
	sname, fname, err := splitParamVar(param)
	nfname := fname

	if err != nil {
		return param
	}

	switch sname {
	case bfe_basic.VarNameScopeUser:
	case bfe_basic.VarNameScopeBfeReqHeader:
		nfname = textproto.CanonicalMIMEHeaderKey(fname)
	case bfe_basic.VarNameScopeBfeResHeader:
		nfname = textproto.CanonicalMIMEHeaderKey(fname)
	case bfe_basic.VarNameScopeBfeReqCookie:
	case bfe_basic.VarNameScopeBfeResCookie:
	case bfe_basic.VarNameScopeBfeReqQuery:
	default:
	}

	return joinParamVar(sname, nfname)
}

func (p *innerParamVarEvaluator) check(param string) error {
	sname, fname, err := splitParamVar(param)

	if err != nil {
		return err
	}

	candidates := []string{bfe_basic.VarNameScopeBfeReqHeader,
		bfe_basic.VarNameScopeBfeResHeader,
		bfe_basic.VarNameScopeBfeReqCookie,
		bfe_basic.VarNameScopeBfeResCookie,
		bfe_basic.VarNameScopeBfeReqQuery,
		bfe_basic.VarNameScopeUser}
	found := false
	for _, item := range candidates {
		if sname == item {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("paramVar is not valid: %s, illegal sname:%s, candidates are %s", param, sname, strings.Join(candidates, ","))
	}

	if sname == bfe_basic.VarNameScopeUser {
		if !IsLegalVarName(fname) {
			return fmt.Errorf("illegal user variable name:%s, illegal part:%s", param, fname)
		}
	}

	return nil
}

func (p *innerParamVarEvaluator) eval(param string, req *bfe_basic.Request) string {
	sname, fname, err := splitParamVar(param)

	if err != nil {
		return ""
	}
	ret := ""
	switch sname {
	case bfe_basic.VarNameScopeUser:
		ret = req.GetUsrVariable(fname)
	case bfe_basic.VarNameScopeBfeReqHeader:
		if req.HttpRequest != nil {
			ret = req.HttpRequest.Header.Get(fname)
		}
	case bfe_basic.VarNameScopeBfeResHeader:
		if req.HttpResponse != nil {
			ret = req.HttpResponse.Header.Get(fname)
		}
	case bfe_basic.VarNameScopeBfeReqCookie:
		if req.HttpRequest != nil {
			cookie, err := req.HttpRequest.Cookie(fname)
			if err != nil {
				ret = ""
			} else {
				ret = cookie.Value
			}
		}
	case bfe_basic.VarNameScopeBfeResCookie:
		if req.HttpResponse != nil {
			cookie, err := req.HttpResponse.Cookie(fname)
			if err != nil {
				ret = ""
			} else {
				ret = cookie.Value
			}
		}
	case bfe_basic.VarNameScopeBfeReqQuery:
		ret = GetReqQueryKey(req, fname)
	default:
	}

	return ret
}

func checkLegacyStyleVar(param string) error {
	varHeaderVal := strings.ToLower(param)
	if _, found := VariableHandlers[varHeaderVal[1:]]; !found {
		// not valid header value variable
		err := fmt.Errorf("legacy style paramVar is not valid: %s", param)
		return err
	}
	return nil
}

func checkNewStyleVar(param string) error {
	if isParamVar(param) {
		evaluator := innerParamVarEvaluator{}
		return evaluator.check(param)
	}

	evaluator := innerVarEvaluator{}
	return evaluator.check(param)
}

func adjustNewStyleVarName(param string) string {
	if isParamVar(param) {
		evaluator := innerParamVarEvaluator{}
		return evaluator.adjustParVarName(param)
	}

	return param
}

func evalLegacyStyleVar(param string, req *bfe_basic.Request) string {
	varHeaderVal := strings.ToLower(param)
	if handler, found := VariableHandlers[varHeaderVal[1:]]; found {
		return handler(req)
	} else {
		return ""
	}
}

func evalNewStyleVar(param string, req *bfe_basic.Request) string {
	if isParamVar(param) {
		evaluator := innerParamVarEvaluator{}
		return evaluator.eval(param, req)
	}

	evaluator := innerVarEvaluator{}
	return evaluator.eval(param, req)
}

func PreprocessSetUsrVarParams(params []string) ([]string, error) {
	retParams := []string{}

	if len(params) != 2 {
		return nil, fmt.Errorf("illegal params size. should be 2, actual:%d", len(params))
	}

	if !IsLegalVarName(params[0]) {
		return nil, fmt.Errorf("illegal user variable name:%s", params[0])
	}
	retParams = append(retParams, params[0])
	args, err := PreprocessFormatStr(params[1])
	if err != nil {
		return nil, err
	}
	retParams = append(retParams, args...)

	return retParams, nil
}

func SetUsrVariable(req *bfe_basic.Request, varName string, params []string) {
	val := EvalFormatStr(req, params)
	req.SetUsrVariable(varName, val)
}
