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

package mod_redirect

import (
	"errors"
	"fmt"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_expr"
	"github.com/bfenetworks/go-lib/log"
)

type ActionFile struct {
	Cmd    *string  // command of action
	Params []string // params of action
}

type Action struct {
	Cmd    string   // command of action
	Params []string // params of action

	// param0 加载期编译的 Params[0] 表达式（URL_SET / URL_PREFIX_ADD）；
	// nil 或字面量时运行期直接使用 Params[0]
	param0 *bfe_expr.Param
}

type ActionFileList []ActionFile

var EXCLUSIVE_ACTIONS = map[string]interface{}{
	"SCHEME_SET":     nil,
	"URL_SET":        nil,
	"URL_FROM_QUERY": nil,
	"URL_PREFIX_ADD": nil,
}

func ActionFileCheck(conf ActionFile) error {
	var paramsLenCheck int

	// check command
	if conf.Cmd == nil {
		return errors.New("no Cmd")
	}

	// validate command, and get how many params should exist for each command
	switch *conf.Cmd {
	// commands for url
	case "URL_SET", "URL_FROM_QUERY", "URL_PREFIX_ADD", "SCHEME_SET":
		paramsLenCheck = 1
	default:
		return fmt.Errorf("invalid cmd:%s", *conf.Cmd)
	}

	// check params
	if conf.Params == nil {
		return errors.New("no Params")
	}

	if paramsLenCheck != -1 {
		paramsLen := len(conf.Params)
		if paramsLenCheck != paramsLen {
			return fmt.Errorf("num of params:[ok:%d, now:%d]", paramsLenCheck, paramsLen)
		}
	}

	// currently only http|https scheme supported.
	if *conf.Cmd == "SCHEME_SET" {
		scheme := strings.ToLower(conf.Params[0])
		if scheme != "http" && scheme != "https" {
			return fmt.Errorf("scheme %s invalid, only http|https supported now", conf.Params[0])
		}
		conf.Params[0] = scheme
	}

	return nil
}

func ActionFileListCheck(conf *ActionFileList) error {
	if len(*conf) > 1 {
		return fmt.Errorf("ActionFileList: currently only support exclusive action!")
	}

	for index, action := range *conf {
		err := ActionFileCheck(action)
		if err != nil {
			return fmt.Errorf("ActionFileList:%d, %s", index, err.Error())
		}
	}

	return nil
}

func actionConvert(actionFile ActionFile) (Action, error) {
	action := Action{}
	action.Cmd = *actionFile.Cmd
	action.Params = actionFile.Params

	// URL_SET / URL_PREFIX_ADD 的 Params[0] 支持 "=" 前缀表达式，加载期编译
	switch action.Cmd {
	case "URL_SET", "URL_PREFIX_ADD":
		p, err := bfe_expr.CompileParam(action.Params[0])
		if err != nil {
			return action, err
		}
		action.param0 = p
	}

	return action, nil
}

func actionsConvert(actionFiles ActionFileList) ([]Action, error) {
	actions := make([]Action, 0, len(actionFiles))

	for _, actionFile := range actionFiles {
		action, err := actionConvert(actionFile)
		if err != nil {
			return actions, err
		}
		actions = append(actions, action)
	}

	return actions, nil
}

// actionParam0 返回运行期 Params[0]：表达式求值（失败置空 + warn），
// 字面量（含 "==" 转义）经 Param 返回
func actionParam0(req *bfe_basic.Request, action Action) string {
	if action.param0 == nil {
		return action.Params[0]
	}
	v, err := action.param0.Eval(req)
	if err != nil {
		logid := ""
		if req != nil {
			logid = req.LogId
		}
		log.Logger.Warn("bfe_expr: eval redirect param %q error: %v (logid: %s)",
			action.param0.String(), err, logid)
		return ""
	}
	return v
}

// do exclusive action to request
func redirectExclusiveActionDo(req *bfe_basic.Request, action Action) {
	switch action.Cmd {
	case "SCHEME_SET":
		ReqSchemeSet(req, action.Params[0])
	// for url
	case "URL_SET":
		ReqUrlSet(req, actionParam0(req, action))
	case "URL_FROM_QUERY":
		ReqUrlFromQuery(req, action.Params[0])
	case "URL_PREFIX_ADD":
		ReqUrlPrefixAdd(req, actionParam0(req, action))
	}
}

// check if exclusive action
func checkExclusiveAction(actions []Action) bool {
	if len(actions) != 1 {
		return false
	}

	action := actions[0]
	_, ok := EXCLUSIVE_ACTIONS[action.Cmd]
	return ok
}

// do actions to request
func redirectActionsDo(req *bfe_basic.Request, actions []Action) {
	// for exclusive action
	if checkExclusiveAction(actions) {
		redirectExclusiveActionDo(req, actions[0])
		return
	}
}
