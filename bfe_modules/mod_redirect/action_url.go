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
	"github.com/bfenetworks/bfe/bfe_basic"
)

// ReqUrlSet sets redirect url
func ReqUrlSet(req *bfe_basic.Request, url string) {
	req.Redirect.Url = url
}

// ReqUrlFromQuery sets redirect url to value of given field in query
// e.g., url "http://service?url=(.*)" => "$1",
func ReqUrlFromQuery(req *bfe_basic.Request, key string) {
	if req.Query == nil {
		req.Query = req.HttpRequest.URL.Query()
	}

	req.Redirect.Url = req.Query.Get(key)
}

// ReqUrlPrefixAdd specify redirect url by adding prefix to original uri(path+query)
// e.g., url  "/(.*)" => "link$1",
func ReqUrlPrefixAdd(req *bfe_basic.Request, prefix string) {
	rawUrl := req.HttpRequest.URL
	uri := rawUrl.RequestURI()
	req.Redirect.Url = prefix + uri
}

// ReqSchemeSet specify redirect url to absolute one with scheme user defined
// e.g., url  scheme://host/path, usually scheme is https
func ReqSchemeSet(req *bfe_basic.Request, scheme string) {
	rawUrl := req.HttpRequest.URL
	uri := rawUrl.RequestURI()

	host := rawUrl.Host
	if host == "" {
		host = req.HttpRequest.Host
	}

	req.Redirect.Url = scheme + "://" + host + uri
}
