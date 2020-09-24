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

// Internal data structure for http request

package bfe_basic

import (
	"net"
	"net/url"
)

import (
	"github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_http"
)

type BackendInfo struct {
	ClusterName    string // name of cluster
	SubclusterName string // name of sub-cluster
	BackendAddr    string // backend ip address
	BackendPort    uint32 // backend's port
	BackendName    string // backend name
}

type RedirectInfo struct {
	Url    string          // URL
	Code   int             // HTTP status code
	Header bfe_http.Header // Extra header
}

type RequestRoute struct {
	Error       error  // error in request-route
	HostTag     string // tags
	Product     string // name of product
	ClusterName string // clustername req should route to
}

type RequestTags struct {
	Error    error               // error in request-tag
	TagTable map[string][]string // type-tags pairs
}

type RequestTransport struct {
	Backend   *backend.BfeBackend   // destination backend for request
	Transport bfe_http.RoundTripper // transport to backend
}

// Request is a wrapper of HTTP request
type Request struct {
	Connection net.Conn
	Session    *Session

	RemoteAddr *net.TCPAddr // address of remote peer
	ClientAddr *net.TCPAddr // address of real client. Maybe nil if request is from
	// upstream proxy but without a valid Clientip header

	HttpRequest  *bfe_http.Request  // incoming request
	OutRequest   *bfe_http.Request  // forwarded request
	HttpResponse *bfe_http.Response // corresponding response

	CookieMap bfe_http.CookieMap // cookie map
	Query     url.Values         // save url query

	LogId         string // log id for each request
	ReqBody       []byte // req body, size is limited
	ReqBodyPeeked bool   // whether req body has been peeked

	Route RequestRoute // for get backend cluster based on host/path/query/header/...

	Tags RequestTags // request tag info

	Trans RequestTransport // request transport

	BfeStatusCode int // request directly return by bfe

	ErrCode error  // error code for handling request
	ErrMsg  string // additional error msg

	Stat *RequestStat // time, data length, etc.

	RetryTime int         // times of retry
	Backend   BackendInfo // backend info

	Redirect RedirectInfo // redirect info

	SvrDataConf ServerDataConfInterface // interface for ServerDataConf

	// User context associated with this request
	Context map[interface{}]interface{}
}

// NewRequest creates and initializes a new request.
func NewRequest(request *bfe_http.Request, conn net.Conn, stat *RequestStat,
	session *Session, svrDataConf ServerDataConfInterface) *Request {
	fReq := new(Request)

	fReq.ErrCode = nil
	fReq.Connection = conn
	fReq.HttpRequest = request
	fReq.Stat = stat
	fReq.Session = session
	fReq.Context = make(map[interface{}]interface{})
	fReq.Tags.TagTable = make(map[string][]string)

	if session != nil {
		fReq.RemoteAddr = session.RemoteAddr
	}

	fReq.SvrDataConf = svrDataConf

	return fReq
}

func (req *Request) CachedQuery() url.Values {
	if req.Query == nil {
		req.Query = req.HttpRequest.URL.Query()
	}

	return req.Query
}

func (req *Request) CachedCookie() bfe_http.CookieMap {
	// parse all cookies if needed
	if req.CookieMap == nil {
		cookies := req.HttpRequest.Cookies()
		req.CookieMap = bfe_http.CookieMapGet(cookies)
	}

	return req.CookieMap
}

func (req *Request) Cookie(name string) (*bfe_http.Cookie, bool) {
	if req.CookieMap == nil {
		req.CachedCookie() // lazily parse cookie
	}
	return req.CookieMap.Get(name)
}

func (req *Request) SetRequestTransport(backend *backend.BfeBackend,
	transport bfe_http.RoundTripper) {
	req.Trans.Backend = backend
	req.Trans.Transport = transport
}

func (req *Request) Protocol() string {
	if req.Session.IsSecure {
		return req.Session.Proto
	}
	return req.HttpRequest.Proto
}

func (req *Request) AddTags(name string, ntags []string) {
	if len(ntags) == 0 {
		return
	}

	tags := req.Tags.TagTable[name]
	tags = append(tags, ntags...)
	req.Tags.TagTable[name] = tags
}

func (req *Request) GetTags(name string) []string {
	return req.Tags.TagTable[name]
}

func (req *Request) SetContext(key, val interface{}) {
	req.Context[key] = val
}

func (req *Request) GetContext(key interface{}) interface{} {
	return req.Context[key]
}
