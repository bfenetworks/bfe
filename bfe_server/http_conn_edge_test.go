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

package bfe_server

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_websocket"
)

// TC-08 Expect: 100-continue —— 先回 100 Continue，转发后再回最终响应。
// 用 Connection: close + 读到 EOF 的方式拿原始报文，避免中途的 1xx 干扰结构化解析。
func TestForwardExpectContinue(t *testing.T) {
	upstream := newTestUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Body-Len", strconv.Itoa(len(body)))
		_, _ = io.WriteString(w, "done")
	})
	srv := newForwardServer(t, upstream.Listener.Addr().String(), nil)

	c := dialServer(t, srv)
	const payload = "hello-body"
	c.send("POST /v1/echo HTTP/1.1\r\n" +
		"Host: " + testHost + "\r\n" +
		"Connection: close\r\n" +
		"Expect: 100-continue\r\n" +
		"Content-Length: " + strconv.Itoa(len(payload)) + "\r\n\r\n" + payload)

	raw := c.readRawUntilEOF()

	contIdx := strings.Index(raw, "HTTP/1.1 100 Continue")
	okIdx := strings.Index(raw, "HTTP/1.1 200 OK")
	if contIdx < 0 {
		t.Fatalf("expected 100 Continue before the final response, got: %q", raw)
	}
	if okIdx < 0 {
		t.Fatalf("expected a 200 response, got: %q", raw)
	}
	if contIdx > okIdx {
		t.Errorf("100 Continue should precede the final response, got: %q", raw)
	}
	if got := srv.serverStatus.ProxyState.ClientConnUse100Continue.Get(); got != 1 {
		t.Errorf("ClientConnUse100Continue = %d, want 1", got)
	}

	// 上游应收到完整 body
	if _, lastBody := upstream.lastRequest(); lastBody != payload {
		t.Errorf("upstream body = %q, want %q", lastBody, payload)
	}
}

// TC-08b Expect + Content-Length: 0 —— 直接 400 并关闭连接。
func TestForwardExpectContinueZeroLength(t *testing.T) {
	upstream := newTestUpstream(t, nil)
	srv := newForwardServer(t, upstream.Listener.Addr().String(), nil)

	c := dialServer(t, srv)
	c.send("POST /v1/echo HTTP/1.1\r\n" +
		"Host: " + testHost + "\r\n" +
		"Expect: 100-continue\r\n" +
		"Content-Length: 0\r\n\r\n")

	raw := c.readRawUntilEOF()
	if !strings.Contains(raw, "400 Bad Request") {
		t.Fatalf("expected 400, got: %q", raw)
	}
	if got := upstream.hits.Load(); got != 0 {
		t.Errorf("upstream hits = %d, want 0", got)
	}
	if got := srv.serverStatus.ProxyState.ErrClientZeroContentlen.Get(); got != 1 {
		t.Errorf("ErrClientZeroContentlen = %d, want 1", got)
	}
}

// TC-08c 不支持的 Expect —— 417 Expectation Failed。
func TestForwardUnsupportedExpect(t *testing.T) {
	upstream := newTestUpstream(t, nil)
	srv := newForwardServer(t, upstream.Listener.Addr().String(), nil)

	c := dialServer(t, srv)
	c.send(getRequest("/v1/echo", "Expect: 200-ok"))

	raw := c.readRawUntilEOF()
	if !strings.Contains(raw, "417 Expectation Failed") {
		t.Fatalf("expected 417, got: %q", raw)
	}
	if got := upstream.hits.Load(); got != 0 {
		t.Errorf("upstream hits = %d, want 0", got)
	}
	if got := srv.serverStatus.ProxyState.ErrClientExpectFail.Get(); got != 1 {
		t.Errorf("ErrClientExpectFail = %d, want 1", got)
	}
}

// TC-09 上游连不上 —— 内部 500 响应 + 计入连接类错误。
func TestForwardUpstreamUnreachable(t *testing.T) {
	// upstreamAddr 留空 -> 使用一个已关闭的端口
	srv := newForwardServerWith(t, forwardOpts{})

	c := dialServer(t, srv)
	resp, _ := c.do(getRequest("/v1/echo"), "GET")

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if got := srv.serverStatus.ProxyState.ErrBkConnectBackend.Get(); got < 1 {
		t.Errorf("ErrBkConnectBackend = %d, want >= 1", got)
	}
	if got := srv.serverStatus.ProxyState.ErrBkRequestBackend.Get(); got < 1 {
		t.Errorf("ErrBkRequestBackend = %d, want >= 1", got)
	}
}

// TC-10 上游响应头超时 —— 按集群的 TimeoutResponseHeader 失败并计数。
func TestForwardUpstreamResponseHeaderTimeout(t *testing.T) {
	release := make(chan struct{})
	upstream := newTestUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = io.WriteString(w, "late")
	})
	defer close(release)

	srv := newForwardServerWith(t, forwardOpts{
		upstreamAddr: upstream.Listener.Addr().String(),
		// 集群级超时 50ms；BFE 的默认值是 60s，这里显式收紧
		clusterExtra: `"BackendConf": {"TimeoutResponseHeader": 50}`,
	})

	c := dialServer(t, srv)

	start := time.Now()
	resp, _ := c.do(getRequest("/v1/echo"), "GET")
	elapsed := time.Since(start)

	if resp.StatusCode < 500 {
		t.Fatalf("status = %d, want 5xx", resp.StatusCode)
	}
	if elapsed > 2*time.Second {
		t.Errorf("request took %v, want well under 2s (cluster timeout is 50ms)", elapsed)
	}
	if got := srv.serverStatus.ProxyState.ErrBkRespHeaderTimeout.Get(); got < 1 {
		t.Errorf("ErrBkRespHeaderTimeout = %d, want >= 1", got)
	}
}

// TC-11 HTTP Upgrade —— checkHttpUpgrade 识别与 next-proto 分发。
func TestCheckHttpUpgrade(t *testing.T) {
	upgradeReq := &bfe_basic.Request{HttpRequest: &bfe_http.Request{Method: "GET", Header: bfe_http.Header{}}}
	upgradeReq.HttpRequest.Header.Set("Connection", "Upgrade")
	upgradeReq.HttpRequest.Header.Set("Upgrade", "websocket")
	if got := checkHttpUpgrade(upgradeReq); got != bfe_websocket.WebSocket {
		t.Errorf("checkHttpUpgrade = %q, want %q", got, bfe_websocket.WebSocket)
	}

	plainReq := &bfe_basic.Request{HttpRequest: &bfe_http.Request{Method: "GET", Header: bfe_http.Header{}}}
	if got := checkHttpUpgrade(plainReq); got != "" {
		t.Errorf("checkHttpUpgrade = %q, want empty for a plain request", got)
	}
}

// TC-11b Upgrade 请求被交给注册的 next-proto 处理器。
func TestForwardUpgradeDispatch(t *testing.T) {
	upstream := newTestUpstream(t, nil)
	srv := newForwardServer(t, upstream.Listener.Addr().String(), nil)

	handled := make(chan struct{})
	srv.HTTPNextProto = map[string]func(*bfe_http.Server, bfe_http.ResponseWriter, *bfe_http.Request){
		bfe_websocket.WebSocket: func(_ *bfe_http.Server, w bfe_http.ResponseWriter, r *bfe_http.Request) {
			// 真实升级处理器会接管连接；这里同样 Hijack 后写原始响应，
			// 因为 serve() 在分发后直接 return，不再走 response.finishRequest()。
			rw, _, err := w.(*response).Hijack()
			if err == nil {
				_, _ = io.WriteString(rw,
					"HTTP/1.1 101 Switching Protocols\r\n"+
						"Upgrade: websocket\r\n"+
						"X-Upgrade-Handler: called\r\n\r\n")
			}
			close(handled)
		},
	}

	c := dialServer(t, srv)
	c.send(getRequest("/v1/echo",
		"Connection: Upgrade",
		"Upgrade: websocket"))

	select {
	case <-handled:
	case <-time.After(3 * time.Second):
		t.Fatal("upgrade handler was not dispatched")
	}

	raw := c.readRawUntilEOF()
	if !strings.Contains(raw, "X-Upgrade-Handler: called") {
		t.Errorf("response should come from the upgrade handler, got: %q", raw)
	}
	// 升级后不再走反向代理
	if got := upstream.hits.Load(); got != 0 {
		t.Errorf("upstream hits = %d, want 0 (upgrade bypasses the proxy)", got)
	}
}

// TC-12 客户端提前断开 —— 服务端不挂死、不 panic。
func TestForwardClientDisconnect(t *testing.T) {
	upstream := newTestUpstream(t, nil)
	srv := newForwardServer(t, upstream.Listener.Addr().String(), nil)

	c := dialServer(t, srv)
	// 只发头部（声明 100 字节 body），随后直接断开
	c.send("POST /v1/echo HTTP/1.1\r\n" +
		"Host: " + testHost + "\r\n" +
		"Content-Length: 100\r\n\r\n" +
		"partial")
	if err := c.conn.Close(); err != nil {
		t.Fatalf("close client conn failed: %v", err)
	}

	// serve() 必须在合理时间内退出，且不触发 panic 兜底
	c.waitDone(5 * time.Second)
	if got := srv.serverStatus.ProxyState.PanicClientConnServe.Get(); got != 0 {
		t.Errorf("PanicClientConnServe = %d, want 0", got)
	}
}

// TC-14 纯函数：statusLine 与 appendTime。
func TestStatusLine(t *testing.T) {
	req11 := &bfe_http.Request{Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1}
	req10 := &bfe_http.Request{Proto: "HTTP/1.0", ProtoMajor: 1, ProtoMinor: 0}

	tests := []struct {
		name string
		req  *bfe_http.Request
		code int
		want string
	}{
		{name: "OK11", req: req11, code: 200, want: "HTTP/1.1 200 OK\r\n"},
		{name: "OK10", req: req10, code: 200, want: "HTTP/1.0 200 OK\r\n"},
		{name: "NoContent", req: req11, code: 204, want: "HTTP/1.1 204 No Content\r\n"},
		{name: "NotModified", req: req11, code: 304, want: "HTTP/1.1 304 Not Modified\r\n"},
		{name: "NotFound", req: req11, code: 404, want: "HTTP/1.1 404 Not Found\r\n"},
		{name: "UnknownCode", req: req11, code: 599, want: "HTTP/1.1 599 status code 599\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusLine(tt.req, tt.code); got != tt.want {
				t.Errorf("statusLine = %q, want %q", got, tt.want)
			}
			// 第二次走缓存分支，结果必须一致
			if got := statusLine(tt.req, tt.code); got != tt.want {
				t.Errorf("statusLine (cached) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppendTime(t *testing.T) {
	ts := time.Date(2026, time.October, 20, 8, 9, 10, 0, time.UTC)
	want := "Tue, 20 Oct 2026 08:09:10 GMT"

	if got := string(appendTime(nil, ts)); got != want {
		t.Errorf("appendTime = %q, want %q", got, want)
	}

	// 非 UTC 时间应先转 UTC
	zone := time.FixedZone("UTC+8", 8*3600)
	if got := string(appendTime(nil, ts.In(zone))); got != want {
		t.Errorf("appendTime with zone = %q, want %q", got, want)
	}

	// 应追加到已有内容之后
	if got := string(appendTime([]byte("Date: "), ts)); got != "Date: "+want {
		t.Errorf("appendTime append = %q, want %q", got, "Date: "+want)
	}
}
