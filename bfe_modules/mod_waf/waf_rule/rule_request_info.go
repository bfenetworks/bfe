// Copyright (c) 2020 The BFE Authors.
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
package waf_rule

import (
	"net/url"
)
import (
	"github.com/bfenetworks/bfe/bfe_basic"
)

type RuleRequestInfo struct {
	Method  string // "GET", "POST", "PUT", "DELETE"
	Version string // "HTTP_1_0", "HTTP_1_1"

	Headers map[string][]string //Header

	Uri        string   // uri
	UriUnquote string   // unquoted uri
	UriParsed  *url.URL // parsed uri

	QueryValues url.Values // parsed query string values
}

func NewRuleRequestInfo(req *bfe_basic.Request) *RuleRequestInfo {
	wj := new(RuleRequestInfo)
	wj.Method = req.HttpRequest.Method
	wj.Uri = req.HttpRequest.RequestURI
	wj.Headers = req.HttpRequest.Header

	wj.UriUnquote, _ = url.QueryUnescape(wj.Uri)
	wj.UriParsed, _ = url.Parse(wj.Uri)
	wj.QueryValues = wj.UriParsed.Query()
	return wj
}
