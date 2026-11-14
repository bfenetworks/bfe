// Copyright (c) 2019 The BFE Authors.
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

package action

import (
	"bytes"
	"fmt"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

const (
	// connection actions
	ActionClose  = "CLOSE"  // close, close the connection directly
	ActionPass   = "PASS"   // pass, do nothing
	ActionFinish = "FINISH" // finish, close connection after reply

	// header actions
	ActionReqHeaderAdd = "REQ_HEADER_ADD" // add request header
	ActionReqHeaderSet = "REQ_HEADER_SET" // set request header
	ActionReqHeaderDel = "REQ_HEADER_DEL" // del request header

	// host actions
	ActionHostSetFromPathPrefix = "HOST_SET_FROM_PATH_PREFIX" // set host from path prefix
	ActionHostSet               = "HOST_SET"                  // set host
	ActionHostSuffixReplace     = "HOST_SUFFIX_REPLACE"       // set host replaced suffx

	// path actions
	ActionPathSet        = "PATH_SET"         // set path
	ActionPathPrefixAdd  = "PATH_PREFIX_ADD"  // add path prefix
	ActionPathPrefixTrim = "PATH_PREFIX_TRIM" // trim path prefix

	// query actions
	ActionQueryAdd          = "QUERY_ADD"            // add query
	ActionQueryDel          = "QUERY_DEL"            // del query
	ActionQueryRename       = "QUERY_RENAME"         // rename query
	ActionQueryDelAllExcept = "QUERY_DEL_ALL_EXCEPT" // del query except given query key
)

type ActionFile struct {
	Cmd    *string // command of action
	Params []string
}

type Action struct {
	Cmd    string   // command of action
	Params []string // params of action

	// exprEval 动作参数表达式运行期求值闭包（加载期编译）；nil 表示无表达式参数
	exprEval func(req *bfe_basic.Request) []string
}

// ExprParamCompiler 动作参数表达式编译器签名：
// 入参为参数列表与「可作为表达式的下标集合」，返回运行期求值闭包与加载期错误。
type ExprParamCompiler func(params []string, indexes []int) (func(req *bfe_basic.Request) []string, error)

var exprParamCompiler ExprParamCompiler

// SetExprParamCompiler 注入动作参数表达式编译器。
// 由 bfe_expr 包在 init 中调用，避免 bfe_basic/action 反向依赖 bfe_expr（依赖环）。
// 未注入时表达式参数按字面量处理，保持旧行为。
func SetExprParamCompiler(c ExprParamCompiler) {
	exprParamCompiler = c
}

// exprLiteralOnly 各 cmd 中必须保持字面量的参数下标；其余参数默认都允许写成
// "=expr"（未加前缀的参数仍按字面量处理，零开销，因此不会改变既有配置行为）。
// 只登记受加载期格式校验、或运行期动态取值有安全风险的参数：
//   - REQ_HEADER_ADD/SET 的 Params[0] 是 header 名，ActionFileCheck 要求其以
//     X-Bfe- 开头；若支持表达式则需在运行期校验动态 header 名，会引入请求头
//     注入风险。
var exprLiteralOnly = map[string]map[int]bool{
	ActionReqHeaderAdd: {0: true},
	ActionReqHeaderSet: {0: true},
}

// exprParamIndexes 返回该 cmd 允许编译为表达式的参数下标（literal-only 之外的全部下标）
func exprParamIndexes(cmd string, paramNum int) []int {
	skip := exprLiteralOnly[cmd]
	indexes := make([]int, 0, paramNum)
	for i := 0; i < paramNum; i++ {
		if skip[i] {
			continue
		}
		indexes = append(indexes, i)
	}
	return indexes
}

// compileExprParams 编译动作参数中以 "=" 开头的表达式参数；无表达式参数返回 nil
func (ac *Action) compileExprParams() error {
	ac.exprEval = nil
	if exprParamCompiler == nil {
		return nil
	}
	indexes := exprParamIndexes(ac.Cmd, len(ac.Params))
	if len(indexes) == 0 {
		return nil
	}
	eval, err := exprParamCompiler(ac.Params, indexes)
	if err != nil {
		return err
	}
	ac.exprEval = eval
	return nil
}

// UnmarshalJSON decodes given data in json format
func (ac *Action) UnmarshalJSON(data []byte) error {
	var actionFile ActionFile

	// decode
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&actionFile); err != nil {
		return fmt.Errorf("decode Action err: %s", err)
	}

	// check
	if err := ActionFileCheck(actionFile); err != nil {
		return fmt.Errorf("actionFileCheck err: %s", err)
	}

	// convert to action
	ac.Cmd = *actionFile.Cmd
	ac.Params = actionFile.Params

	// compile expr params ("=" prefix) at load time; fail fast on invalid expr
	if err := ac.compileExprParams(); err != nil {
		return fmt.Errorf("compile expr params err: %s", err)
	}

	return nil
}

func (ac *Action) Check(allowActions map[string]bool) error {
	cmd := ac.Cmd

	if _, ok := allowActions[cmd]; !ok {
		// cmd not allowed
		return fmt.Errorf("action cmd[%s], is not allowed", cmd)
	}

	return nil
}

// Do process action for request
func (ac *Action) Do(req *bfe_basic.Request) error {
	params := ac.Params
	if ac.exprEval != nil {
		params = ac.exprEval(req)
	}

	switch ac.Cmd {
	// for req header
	case ActionReqHeaderAdd:
		req.HttpRequest.Header.Add(params[0], params[1])
	case ActionReqHeaderSet:
		req.HttpRequest.Header.Set(params[0], params[1])
	case ActionReqHeaderDel:
		req.HttpRequest.Header.Del(params[0])

	// for host
	case ActionHostSet:
		ReqHostSet(req, params[0])
	case ActionHostSetFromPathPrefix:
		ReqHostSetFromFirstPathSegment(req)
	case ActionHostSuffixReplace:
		ReqHostSuffixReplace(req, params[0], params[1])

	// for path
	case ActionPathSet:
		ReqPathSet(req, params[0])
	case ActionPathPrefixAdd:
		ReqPathPrefixAdd(req, params[0])
	case ActionPathPrefixTrim:
		ReqPathPrefixTrim(req, params[0])

	// for query
	case ActionQueryAdd:
		ReqQueryAdd(req, params)
	case ActionQueryRename:
		ReqQueryRename(req, params[0], params[1])
	case ActionQueryDel:
		ReqQueryDel(req, params)
	case ActionQueryDelAllExcept:
		ReqQueryDelAllExcept(req, params)
	case ActionClose, ActionPass, ActionFinish:
		// pass
	default:
		return fmt.Errorf("unknown cmd[%s]", ac.Cmd)
	}

	return nil
}

const HeaderPrefix = "X-BFE-"

func ActionFileCheck(conf ActionFile) error {
	var paramsLenCheck int

	// check command
	if conf.Cmd == nil {
		return fmt.Errorf("no Cmd")
	}

	// validate command
	*conf.Cmd = strings.ToUpper(*conf.Cmd)
	switch *conf.Cmd {
	case ActionReqHeaderAdd, ActionReqHeaderSet:
		paramsLenCheck = 2
	case ActionReqHeaderDel:
		paramsLenCheck = 1
	case ActionClose, ActionPass, ActionFinish:
		paramsLenCheck = 0
	case ActionHostSetFromPathPrefix:
		paramsLenCheck = 0
	case ActionHostSet:
		paramsLenCheck = 1
	case ActionPathSet, ActionPathPrefixAdd, ActionPathPrefixTrim:
		paramsLenCheck = 1
	case ActionQueryAdd, ActionQueryRename:
		paramsLenCheck = 2
	case ActionQueryDel, ActionQueryDelAllExcept:
		paramsLenCheck = -1 // any is OK
	default:
		return fmt.Errorf("invalid cmd:%s", *conf.Cmd)
	}

	if paramsLenCheck != -1 && len(conf.Params) != paramsLenCheck {
		return fmt.Errorf("num of params:[ok:%d, now:%d]", paramsLenCheck, len(conf.Params))
	}

	for _, p := range conf.Params {
		if len(p) == 0 {
			return fmt.Errorf("empty Params")
		}
	}

	if *conf.Cmd == ActionReqHeaderSet || *conf.Cmd == ActionReqHeaderAdd {
		header := strings.ToUpper(conf.Params[0])

		if !strings.HasPrefix(header, HeaderPrefix) {
			return fmt.Errorf("add/set header key must start with X-Bfe-, got %s", header)
		}
	}

	return nil
}
