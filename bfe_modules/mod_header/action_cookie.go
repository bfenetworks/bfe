// Copyright (c) 2019 Baidu, Inc.
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

package mod_header

import (
	"strings"
)

import (
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_http"
)

const (
	ReqCookieAdd = "REQ_COOKIE_ADD"
	RspCookieDel = "RSP_COOKIE_DEL"
)

func getCookieValue(req *bfe_basic.Request, value string) string {
	if strings.HasPrefix(value, "%") {
		if cookieHandler, ok := VariableHandlers[value[1:]]; ok {
			return cookieHandler(req)
		}
	}

	return value
}

// reqAddCookie adds cookie for request if this cookie key not exists
func reqAddCookie(req *bfe_basic.Request, cookie bfe_http.Cookie) {
	httpRequest := req.HttpRequest
	_, err := httpRequest.Cookie(cookie.Name)
	if err != bfe_http.ErrNoCookie {
		// cookie already exists
		return
	}

	// add cookie
	httpRequest.AddCookie(&cookie)
	if req.CookieMap != nil {
		// add to cached cookie map if cookies have beed parsed
		req.CookieMap[cookie.Name] = &cookie
	}
}

func setCookie(rspHeader bfe_http.Header, cookie bfe_http.Cookie) {
	if v := cookie.String(); v != "" {
		rspHeader.Add("Set-Cookie", v)
	}
}

func rspFindCookie(resp *bfe_http.Response, cookieName string) (*bfe_http.Cookie, bool) {
	cookies := resp.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == cookieName {
			return cookie, true
		}
	}
	return nil, false
}

func rspDelCookie(resp *bfe_http.Response, cookie bfe_http.Cookie) {
	if cookieExist, ok := rspFindCookie(resp, cookie.Name); ok {
		cookieExist.MaxAge = -1
		setCookie(resp.Header, *cookieExist)
	}
}

func ReqCookieActionDo(req *bfe_basic.Request, action Action) {
	cookie := bfe_http.Cookie{
		Name:  action.Params[0],
		Value: getCookieValue(req, action.Params[1]),
	}
	if action.Cmd == ReqCookieAdd {
		reqAddCookie(req, cookie)
	}
}

func ReqCookieActionsDo(req *bfe_basic.Request, actions []Action) {
	for _, action := range actions {
		ReqCookieActionDo(req, action)
	}
}

func RspCookieActionDo(req *bfe_basic.Request, action Action) {
	cookie := bfe_http.Cookie{
		Name: action.Params[0],
	}
	if action.Cmd == RspCookieDel {
		rspDelCookie(req.HttpResponse, cookie)
	}
}

func RspCookieActionsDo(req *bfe_basic.Request, actions []Action) {
	for _, action := range actions {
		RspCookieActionDo(req, action)
	}
}
