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

// ReqHostSet sets hostname to request.
func ReqHostSet(req *bfe_basic.Request, hostname string) {
	httpReq := req.HttpRequest
	httpReq.Host = hostname
}

// ReqHostSetFromFirstPathSegment set hostname from first path segment.
//
// example:
// if uri path pattern is /x.baidu.com/xxxx,
//     set host x.baidu.com
//     set uri path  /xxx
// if do not match this pattern, do noting
func ReqHostSetFromFirstPathSegment(req *bfe_basic.Request) {
	path := req.HttpRequest.URL.Path

	// path: /x.baidu.com/xxxx
	// segs[0]: ""
	// segs[1]: "x.baidu.com"
	// segs[2]: "xxx"
	segs := strings.SplitN(path, "/", 3)
	if len(segs) < 3 {
		return
	}

	// set host and trim path prefix
	req.HttpRequest.Host = segs[1]
	req.HttpRequest.URL.Path = "/" + segs[2]
}

// ReqHostSuffixReplace replaces suffix of hostname.
func ReqHostSuffixReplace(req *bfe_basic.Request, originSuffix, newSuffix string) {
	hostname := req.HttpRequest.URL.Host
	if !strings.HasSuffix(hostname, originSuffix) {
		return
	}

	hostname = strings.TrimSuffix(hostname, originSuffix) + newSuffix
	req.HttpRequest.Host = hostname
}
