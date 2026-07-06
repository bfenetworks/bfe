// Copyright (c) 2026 The BFE Authors.
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

package mod_access_pb3

import (
	"errors"
	"fmt"
	"strconv"
)

const (
	ACTION_SAVE_REQ_HEADER = "SAVE_REQ_HEADER" // save req header to access log
	ACTION_SAVE_REQ_BODY   = "SAVE_REQ_BODY"   // save req body to access log
	ACTION_SAVE_REQ_COOKIE = "SAVE_REQ_COOKIE" // save req cookie to access log
	ACTION_SAVE_RES_HEADER = "SAVE_RES_HEADER" // save res header to access log
)

const (
	// The max size of req body to be logged.
	MAX_LOG_BODY_SIZE = 4096
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

// check ActionFile
func ActionFileCheck(conf ActionFile) error {
	var paramsLenCheck int

	// check command
	if conf.Cmd == nil {
		return errors.New("no Cmd")
	}

	// validate command, and get how many params should exist for each command
	switch *conf.Cmd {
	case ACTION_SAVE_REQ_HEADER:
		paramsLenCheck = -1
	case ACTION_SAVE_REQ_BODY:
		paramsLenCheck = 1
	case ACTION_SAVE_REQ_COOKIE:
		// unsured cookie number
		paramsLenCheck = -1
	case ACTION_SAVE_RES_HEADER:
		paramsLenCheck = -1
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

	switch *conf.Cmd {
	case ACTION_SAVE_REQ_HEADER:
		if len(conf.Params) < 1 {
			return fmt.Errorf("At least one req header key should be set")
		}
	case ACTION_SAVE_REQ_BODY:
		maxReqBodySize, err := strconv.Atoi(conf.Params[0])
		if err != nil || maxReqBodySize < 0 || maxReqBodySize > MAX_LOG_BODY_SIZE {
			return fmt.Errorf("MaxBodySize should be [0, %d]", MAX_LOG_BODY_SIZE)
		}
	case ACTION_SAVE_REQ_COOKIE:
		if len(conf.Params) < 1 {
			return fmt.Errorf("At least one cookie key should be set")
		}
	case ACTION_SAVE_RES_HEADER:
		if len(conf.Params) < 1 {
			return fmt.Errorf("At least one res header key should be set")
		}
	default:
		return fmt.Errorf("invalid cmd:%s", *conf.Cmd)
	}

	return nil
}

// check ActionFileList
func ActionFileListCheck(conf *ActionFileList) error {
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
	actions := make([]Action, 0)

	for _, actionFile := range actionFiles {
		action := actionConvert(actionFile)
		actions = append(actions, action)
	}

	return actions
}
