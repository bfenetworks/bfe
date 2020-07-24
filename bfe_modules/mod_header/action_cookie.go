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

package mod_header

import (
	"strconv"
	"strings"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

const (
	ReqCookieSet = "REQ_COOKIE_SET"
	ReqCookieDel = "REQ_COOKIE_DEL"
	RspCookieSet = "RSP_COOKIE_SET"
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

func reqAddCookie(req *bfe_basic.Request, cookie *bfe_http.Cookie) {
	req.HttpRequest.AddCookie(cookie)
	if req.CookieMap != nil {
		req.CookieMap[cookie.Name] = cookie
	}
}

func isReqCookieExist(req *bfe_http.Request, cookieName string) bool {
	_, err := req.Cookie(cookieName)
	return err == nil
}

func reqSetCookie(req *bfe_basic.Request, cookie *bfe_http.Cookie) {
	if !isReqCookieExist(req.HttpRequest, cookie.Name) {
		reqAddCookie(req, cookie)
		return
	}

	cookies := req.HttpRequest.Cookies()
	req.HttpRequest.Header.Del("Cookie")

	for i := 0; i < len(cookies); i++ {
		if cookies[i].Name == cookie.Name {
			cookies[i].Value = cookie.Value
		}
		req.HttpRequest.AddCookie(cookies[i])
	}

	if req.CookieMap != nil {
		req.CookieMap[cookie.Name] = cookie
	}
}

func reqDelCookie(req *bfe_basic.Request, cookie *bfe_http.Cookie) {
	if !isReqCookieExist(req.HttpRequest, cookie.Name) {
		return
	}

	cookies := req.HttpRequest.Cookies()
	req.HttpRequest.Header.Del("Cookie")

	for i := 0; i < len(cookies); i++ {
		if cookies[i].Name == cookie.Name {
			continue
		}
		req.HttpRequest.AddCookie(cookies[i])
	}

	if req.CookieMap != nil {
		delete(req.CookieMap, cookie.Name)
	}
}

func rspAddCookie(rsp *bfe_http.Response, cookie *bfe_http.Cookie) {
	rsp.Header.Add("Set-Cookie", cookie.String())
}

func isRspCookieExist(rsp *bfe_http.Response, cookie *bfe_http.Cookie) bool {
	cookies := rsp.Cookies()
	for _, rspCookie := range cookies {
		if rspCookie.Name == cookie.Name &&
			rspCookie.Path == cookie.Path &&
			rspCookie.Domain == cookie.Domain {
			return true
		}
	}
	return false
}

func rspSetCookie(rsp *bfe_http.Response, cookie *bfe_http.Cookie) {
	if !isRspCookieExist(rsp, cookie) {
		rspAddCookie(rsp, cookie)
		return
	}

	cookies := rsp.Cookies()
	rsp.Header.Del("Set-Cookie")

	for i := 0; i < len(cookies); i++ {
		if cookies[i].Name == cookie.Name {
			cookies[i] = cookie
		}
		rspAddCookie(rsp, cookies[i])
	}
}

func rspDelCookie(rsp *bfe_http.Response, cookie *bfe_http.Cookie) {
	if !isRspCookieExist(rsp, cookie) {
		return
	}

	cookies := rsp.Cookies()
	rsp.Header.Del("Set-Cookie")

	for i := 0; i < len(cookies); i++ {
		if cookies[i].Name == cookie.Name {
			continue
		}
		rspAddCookie(rsp, cookies[i])
	}
}

func buildCookie(req *bfe_basic.Request, action Action) *bfe_http.Cookie {
	cookie := &bfe_http.Cookie{
		Name: action.Params[0],
	}

	if action.Cmd == ReqCookieDel {
		return cookie
	}

	if action.Cmd == RspCookieDel {
		cookie.Domain = action.Params[1]
		cookie.Path = action.Params[2]
		return cookie
	}

	cookie.Value = getCookieValue(req, action.Params[1])
	if action.Cmd == ReqCookieSet {
		return cookie
	}

	cookie.Domain = action.Params[2]
	cookie.Path = action.Params[3]
	cookie.Expires, _ = time.Parse(time.RFC1123, action.Params[4])
	cookie.MaxAge, _ = strconv.Atoi(action.Params[5])
	cookie.HttpOnly, _ = strconv.ParseBool(action.Params[6])
	cookie.Secure, _ = strconv.ParseBool(action.Params[7])
	return cookie
}

func ReqCookieActionDo(req *bfe_basic.Request, action Action) {
	cookie := buildCookie(req, action)
	switch action.Cmd {
	case ReqCookieSet:
		reqSetCookie(req, cookie)
	case ReqCookieDel:
		reqDelCookie(req, cookie)
	}
}

func RspCookieActionDo(req *bfe_basic.Request, action Action) {
	cookie := buildCookie(req, action)
	switch action.Cmd {
	case RspCookieSet:
		rspSetCookie(req.HttpResponse, cookie)
	case RspCookieDel:
		rspDelCookie(req.HttpResponse, cookie)
	}
}
