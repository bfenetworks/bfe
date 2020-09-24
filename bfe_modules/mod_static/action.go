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

package mod_static

import (
	"fmt"
	"os"
	"path"
)

const (
	ActionBrowse = "BROWSE"
)

type ActionFile struct {
	Cmd    *string
	Params []string
}

type Action struct {
	Cmd    string
	Params []string
}

func ActionFileCheck(conf *ActionFile) error {
	if conf.Cmd == nil {
		return fmt.Errorf("no Cmd")
	}

	switch *conf.Cmd {
	case ActionBrowse:
		if len(conf.Params) != 2 {
			return fmt.Errorf("Params num of %s should be 2", ActionBrowse)
		}

		if _, err := os.Stat(conf.Params[0]); err != nil {
			return fmt.Errorf("Directory[%s] error: %v", conf.Params[0], err)
		}
		if len(conf.Params[1]) != 0 {
			defaultFilePath := path.Join(conf.Params[0], conf.Params[1])
			if _, err := os.Stat(defaultFilePath); err != nil {
				return fmt.Errorf("Default File[%s] error: %v", defaultFilePath, err)
			}
		}
	default:
		return fmt.Errorf("invalid cmd: %s", *conf.Cmd)
	}

	return nil
}

func actionConvert(actionFile ActionFile) Action {
	action := Action{}
	action.Cmd = *actionFile.Cmd
	action.Params = actionFile.Params
	return action
}
