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
	"net/url"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

func prepareRequest(urlStr string) *bfe_basic.Request {
	req := new(bfe_http.Request)
	req.URL, _ = url.Parse(urlStr)

	freq := bfe_basic.NewRequest(req, nil, nil, nil, nil)
	return freq
}

func TestReqUrlSet(t *testing.T) {
	url := "http://www.example.org/unknown"
	req := prepareRequest(url)

	redirectUrl := "http://www.example.org/more"
	ReqUrlSet(req, redirectUrl)

	if req.Redirect.Url != redirectUrl {
		errStr := "req.Redirect.Url should be " + redirectUrl
		t.Error(errStr)
		return
	}
}

func TestReqUrlFromQuery(t *testing.T) {
	url := "http://www.example.org/redirect?url=http://n.example.org"
	req := prepareRequest(url)

	ReqUrlFromQuery(req, "url")

	redirectUrl := "http://n.example.org"
	if req.Redirect.Url != redirectUrl {
		errStr := "req.Redirect.Url should be " + redirectUrl + ", but " + req.Redirect.Url
		t.Error(errStr)
		return
	}
}

func TestReqUrlPrefixAdd(t *testing.T) {
	url := "http://n.example.org/yule/test.html"
	req := prepareRequest(url)

	prefix := "http://n1.example.com/redirect"
	ReqUrlPrefixAdd(req, prefix)

	redirectUrl := prefix + "/yule/test.html"
	if req.Redirect.Url != redirectUrl {
		errStr := "req.Redirect.Url should be " + redirectUrl
		t.Error(errStr)
		return
	}
}

func TestReqSchemeSet(t *testing.T) {
	url := "http://n.example.org/test.html"
	req := prepareRequest(url)

	ReqSchemeSet(req, "https")

	redirectUrl := "https://n.example.org/test.html"
	if req.Redirect.Url != redirectUrl {
		errStr := "req.Redirect.Url should be " + redirectUrl + " but is " + req.Redirect.Url
		t.Error(errStr)
		return
	}
}
