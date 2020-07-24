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
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
)

// ReqPathSet sets path to request.
func ReqPathSet(req *bfe_basic.Request, path string) {
	httpReq := req.HttpRequest
	httpReq.URL.Path = path
}

// ReqPathPrefixAdd adds prefix to path.
// e.g., path  "/(.*)" => "/link?$1",
func ReqPathPrefixAdd(req *bfe_basic.Request, prefix string) {
	httpReq := req.HttpRequest
	pathStr := httpReq.URL.Path
	// remove "/" from path
	pathStr = strings.TrimPrefix(pathStr, "/")
	// add prefix to path
	pathStr = prefix + pathStr

	// add "/" to path
	if !strings.HasPrefix(pathStr, "/") {
		pathStr = "/" + pathStr
	}

	// set new path
	httpReq.URL.Path = pathStr
}

// ReqPathPrefixTrim trims prefix of path
// e.g., path "/service/shortcut/(.*)" => "/$1",
func ReqPathPrefixTrim(req *bfe_basic.Request, prefix string) {
	httpReq := req.HttpRequest
	pathStr := httpReq.URL.Path
	// trim prefix from path
	pathStr = strings.TrimPrefix(pathStr, prefix)
	// add "/" to prefix
	if !strings.HasPrefix(pathStr, "/") {
		pathStr = "/" + pathStr
	}

	// set new path
	httpReq.URL.Path = pathStr
}
