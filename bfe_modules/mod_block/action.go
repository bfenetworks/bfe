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

package mod_block

import (
	"errors"
	"fmt"
)

type ActionFile struct {
	Cmd    *string  // command of action
	Params []string // params of action
}

type Action struct {
	Cmd    string   // command of action
	Params []string // params of action
}

func ActionFileCheck(conf *ActionFile) error {
	var paramsLenCheck int

	// check command
	if conf.Cmd == nil {
		return errors.New("no Cmd")
	}

	// validate command, and get how many params should exist for each command
	switch *conf.Cmd {
	case "CLOSE":
		paramsLenCheck = 0
	case "ALLOW":
		paramsLenCheck = 0
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

	return nil
}

func actionConvert(actionFile ActionFile) Action {
	action := Action{}
	action.Cmd = *actionFile.Cmd
	action.Params = actionFile.Params
	return action
}
