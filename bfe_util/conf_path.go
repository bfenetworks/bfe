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
	"path/filepath"
)

// ConfPathProc return path of config file
//
// Params:
//   - confPath: origin path for config file
//   - confRoot: root path of ALL config
//
// Returns:
//
//	the final path of config file
//	(1) absolute path (e.g. "/xx" on unix, "C:\xx" on windows),
//	    return path untouched
//	(2) else, it's relative path, return filepath.Join(confRoot, path)
//
// NOTE: use filepath (os-aware) instead of path (slash-only) so that
// relative elements like ".." are resolved correctly on windows, where
// confRoot uses back-slash separators.
func ConfPathProc(confPath string, confRoot string) string {
	if !filepath.IsAbs(confPath) {
		// relative path to confRoot
		confPath = filepath.Join(confRoot, confPath)
	}

	return confPath
}
