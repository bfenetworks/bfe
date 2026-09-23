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

package gemini

import (
	"strings"
)

// modelsPathPrefix is the path prefix of Gemini native endpoints that carry
// the model name, e.g. /v1beta/models/{model}:generateContent. It is kept in
// sync with the path recognition rule in detect.go.
const modelsPathPrefix = "/v1beta/models/"

// ExtractModelFromPath extracts the model name from a Gemini native request
// path of the form /v1beta/models/{model}[:action], where action is one of
// generateContent / streamGenerateContent / countTokens etc. The model name
// is the segment between the prefix and the first ":" (or the end of the
// path). It returns "" for paths that do not carry a model (issue #1384).
func ExtractModelFromPath(path string) string {
	rest := strings.TrimPrefix(path, modelsPathPrefix)
	if rest == path {
		return ""
	}
	if i := strings.Index(rest, ":"); i >= 0 {
		rest = rest[:i]
	}
	return strings.Trim(rest, " /")
}
