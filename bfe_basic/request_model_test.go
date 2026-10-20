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

package bfe_basic

import (
	"net"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_http"
)

// fakeConn 只提供会话所需的地址信息。
type fakeConn struct {
	net.Conn
}

func (c fakeConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 54321}
}

func (c fakeConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
}

func newSession() *Session {
	return NewSession(&fakeConn{})
}

// NewRequest 的字段装配与 SSE 识别。
func TestNewRequest(t *testing.T) {
	session := newSession()
	httpReq := &bfe_http.Request{
		Method: "GET",
		Host:   "example.org",
		Proto:  "HTTP/1.1",
		Header: bfe_http.Header{},
	}
	stat := NewRequestStat(time.Now())

	req := NewRequest(httpReq, session.Connection, stat, session, nil)

	if req.HttpRequest != httpReq {
		t.Error("HttpRequest should be the same pointer")
	}
	if req.Connection != session.Connection {
		t.Error("Connection should be set from the argument")
	}
	if req.Stat != stat {
		t.Error("Stat should be set from the argument")
	}
	if req.Session != session {
		t.Error("Session should be set from the argument")
	}
	if req.RemoteAddr == nil || req.RemoteAddr.String() != "10.0.0.1:54321" {
		t.Errorf("RemoteAddr = %v, want 10.0.0.1:54321", req.RemoteAddr)
	}
	if req.Context == nil || req.Tags.TagTable == nil {
		t.Error("Context and Tags.TagTable should be initialized")
	}
	if req.IsSse {
		t.Error("IsSse should be false without an event-stream Accept header")
	}

	// Accept 头含 text/event-stream（含大小写与多余空格）则应识别为 SSE
	httpReq.Header.Set("Accept", "application/json, TEXT/EVENT-STREAM ")
	sseReq := NewRequest(httpReq, session.Connection, stat, session, nil)
	if !sseReq.IsSse {
		t.Error("IsSse should be true for an event-stream Accept header")
	}
}

// NewRequest 在 session 为 nil 时不应 panic。
func TestNewRequestNilSession(t *testing.T) {
	httpReq := &bfe_http.Request{Method: "GET", Header: bfe_http.Header{}}
	req := NewRequest(httpReq, nil, NewRequestStat(time.Now()), nil, nil)
	if req.RemoteAddr != nil {
		t.Errorf("RemoteAddr = %v, want nil", req.RemoteAddr)
	}
}

// CachedQuery 首次解析后缓存，不随 URL 变化重新解析。
func TestCachedQuery(t *testing.T) {
	req := newTestRequest(t, "http://example.org/a?x=1&y=2")

	query := req.CachedQuery()
	if got := query.Get("x"); got != "1" {
		t.Errorf("x = %q, want 1", got)
	}

	// 改动 URL 后仍返回缓存值（固化现状）
	req.HttpRequest.URL.RawQuery = "x=9"
	if got := req.CachedQuery().Get("x"); got != "1" {
		t.Errorf("cached x = %q, want 1", got)
	}

	// 无 query 时返回空集合，不 panic
	empty := newTestRequest(t, "http://example.org/")
	if got := empty.CachedQuery(); len(got) != 0 {
		t.Errorf("query = %v, want empty", got)
	}
}

// CachedCookie / Cookie：解析 Cookie 头并按名查找。
func TestCachedCookieAndCookie(t *testing.T) {
	req := newTestRequest(t, "http://example.org/")
	req.HttpRequest.Header.Set("Cookie", "a=1; b=2")

	cookie, ok := req.Cookie("a")
	if !ok {
		t.Fatal("cookie a should exist")
	}
	if cookie.Value != "1" {
		t.Errorf("cookie a = %q, want 1", cookie.Value)
	}

	if _, ok := req.Cookie("missing"); ok {
		t.Error("cookie missing should not exist")
	}

	// 缓存后再次取用不应重新解析
	cached := req.CachedCookie()
	if cached == nil {
		t.Fatal("CachedCookie should not be nil")
	}
	if _, ok := cached.Get("b"); !ok {
		t.Error("cookie b should be present in the cached map")
	}
}

// Protocol：TLS 连接取 session 协议，否则取 HTTP 协议。
func TestProtocol(t *testing.T) {
	req := newTestRequest(t, "http://example.org/")
	req.HttpRequest.Proto = "HTTP/1.1"
	if got := req.Protocol(); got != "HTTP/1.1" {
		t.Errorf("Protocol = %q, want HTTP/1.1", got)
	}

	req.Session.IsSecure = true
	req.Session.Proto = "https"
	if got := req.Protocol(); got != "https" {
		t.Errorf("Protocol = %q, want https", got)
	}
}

// Tags：追加语义与多值顺序。
func TestAddAndGetTags(t *testing.T) {
	req := newTestRequest(t, "http://example.org/")

	req.AddTags("k", []string{"v1", "v2"})
	req.AddTags("k", []string{"v3"})

	got := req.GetTags("k")
	if len(got) != 3 || got[0] != "v1" || got[1] != "v2" || got[2] != "v3" {
		t.Errorf("tags = %v, want [v1 v2 v3]", got)
	}

	// 空切片不改变已有值
	req.AddTags("k", nil)
	if got := req.GetTags("k"); len(got) != 3 {
		t.Errorf("tags after empty add = %v, want 3 values", got)
	}

	if got := req.GetTags("absent"); got != nil {
		t.Errorf("tags of absent key = %v, want nil", got)
	}
}

// Context：支持任意类型 key，未设置返回 nil。
func TestRequestContext(t *testing.T) {
	req := newTestRequest(t, "http://example.org/")

	type ctxKey struct{ name string }
	key := ctxKey{name: "k"}

	if got := req.GetContext(key); got != nil {
		t.Errorf("GetContext = %v, want nil", got)
	}

	req.SetContext(key, 42)
	if got := req.GetContext(key); got != 42 {
		t.Errorf("GetContext = %v, want 42", got)
	}

	// 不同类型的 key 互不干扰
	req.SetContext("k", "string-key")
	if got := req.GetContext(key); got != 42 {
		t.Errorf("typed key overwritten by string key: %v", got)
	}
	if got := req.GetContext("k"); got != "string-key" {
		t.Errorf("string key = %v, want string-key", got)
	}
}

// SetRequestTransport 写入后端与 transport 引用。
func TestSetRequestTransport(t *testing.T) {
	req := newTestRequest(t, "http://example.org/")

	backend := &backend.BfeBackend{}
	transport := &bfe_http.Transport{}
	req.SetRequestTransport(backend, transport)

	if req.Trans.Backend != backend {
		t.Error("Trans.Backend should be set")
	}
	if req.Trans.Transport != transport {
		t.Error("Trans.Transport should be set")
	}
}

// newTestRequest 构造一个最小的 Request。
func newTestRequest(t *testing.T, target string) *Request {
	t.Helper()

	httpReq, err := bfe_http.NewRequest("GET", target, nil)
	if err != nil {
		t.Fatalf("new http request failed: %v", err)
	}
	session := newSession()
	return NewRequest(httpReq, session.Connection, NewRequestStat(time.Now()), session, nil)
}