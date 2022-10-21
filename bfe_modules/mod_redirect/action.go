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
)

type ActionFile struct {
	Cmd    *string  // command of action
	Params []string // params of action
}

type Action struct {
	Cmd    string   // command of action
	Params []string // params of action
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

func actionConvert(actionFile ActionFile) Action {
	action := Action{}
	action.Cmd = *actionFile.Cmd
	action.Params = actionFile.Params
	return action
}

func actionsConvert(actionFiles ActionFileList) []Action {
	actions := make([]Action, 0, len(actionFiles))

	for _, actionFile := range actionFiles {
		action := actionConvert(actionFile)
		actions = append(actions, action)
	}

	return actions
}

// do exclusive action to request
func redirectExclusiveActionDo(req *bfe_basic.Request, action Action) {
	switch action.Cmd {
	case "SCHEME_SET":
		ReqSchemeSet(req, action.Params[0])
	// for url
	case "URL_SET":
		ReqUrlSet(req, action.Params[0])
	case "URL_FROM_QUERY":
		ReqUrlFromQuery(req, action.Params[0])
	case "URL_PREFIX_ADD":
		ReqUrlPrefixAdd(req, action.Params[0])
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
