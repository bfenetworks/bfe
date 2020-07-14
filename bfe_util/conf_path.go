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

package bfe_util

import (
	"path"
	"strings"
)

// ConfPathProc return path of config file
//
// Params:
//      - confPath: origin path for config file
//      - confRoot: root path of ALL config
//
// Returns:
//      the final path of config file
//      (1) path starts with "/", it's absolute path, return path untouched
//      (2) else, it's relative path, return path.Join(confRoot, path)
//
func ConfPathProc(confPath string, confRoot string) string {
	if !strings.HasPrefix(confPath, "/") {
		// relative path to confRoot
		confPath = path.Join(confRoot, confPath)
	}

	return confPath
}
