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
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
)

// TC-01 基本 GET 转发：状态码、响应头、body 与上游命中次数。
func TestForwardBasicGet(t *testing.T) {
	upstream := newTestUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "hit")
		_, _ = io.WriteString(w, "hello-from-upstream")
	})
	srv := newForwardServer(t, upstream.Listener.Addr().String(), nil)

	c := dialServer(t, srv)
	resp, body := c.do(getRequest("/v1/echo"), "GET")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if body != "hello-from-upstream" {
		t.Errorf("body = %q, want hello-from-upstream", body)
	}
	if got := resp.Header.Get("X-Upstream"); got != "hit" {
		t.Errorf("X-Upstream = %q, want hit", got)
	}
	if got := upstream.hits.Load(); got != 1 {
		t.Errorf("upstream hits = %d, want 1", got)
	}

	// 上游应收到原始 Host 与路径
	lastHost, _ := upstream.lastRequest()
	if lastHost != testHost {
		t.Errorf("upstream Host = %q, want %q", lastHost, testHost)
	}
}

// TC-02 Keep-Alive：同一连接上的两个请求都被服务，连接不重建。
func TestForwardKeepAliveReusesConnection(t *testing.T) {
	upstream := newTestUpstream(t, nil)
	srv := newForwardServer(t, upstream.Listener.Addr().String(), nil)

	c := dialServer(t, srv)

	for i := 1; i <= 2; i++ {
		resp, body := c.do(getRequest("/v1/echo"), "GET")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request #%d status = %d, want 200", i, resp.StatusCode)
		}
		if body != "upstream-body" {
			t.Errorf("request #%d body = %q, want upstream-body", i, body)
		}
	}

	if got := upstream.hits.Load(); got != 2 {
		t.Errorf("upstream hits = %d, want 2 (both requests forwarded)", got)
	}

	// 两个响应来自同一条客户端连接：连接计数只应加 1
	if got := srv.serverStatus.ProxyState.ClientConnServed.Get(); got != 1 {
		t.Errorf("ClientConnServed = %d, want 1 (connection reused)", got)
	}
	if got := srv.serverStatus.ProxyState.ClientReqServed.Get(); got != 2 {
		t.Errorf("ClientReqServed = %d, want 2", got)
	}
}

// TC-03 Connection: close：响应带关闭语义，之后连接被关闭。
func TestForwardConnectionClose(t *testing.T) {
	upstream := newTestUpstream(t, nil)
	srv := newForwardServer(t, upstream.Listener.Addr().String(), nil)

	c := dialServer(t, srv)
	resp, body := c.do(getRequest("/v1/echo", "Connection: close"), "GET")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if body != "upstream-body" {
		t.Errorf("body = %q, want upstream-body", body)
	}
	if !resp.Close {
		t.Error("resp.Close should be true for a Connection: close request")
	}
	if raw := c.raw(); !strings.Contains(raw, "Connection: close") {
		t.Errorf("response header should carry Connection: close, got: %q", raw)
	}

	c.expectClosed()
}

// TC-04 上游 chunked 响应透传：客户端收到分块编码，body 完整。
func TestForwardUpstreamChunked(t *testing.T) {
	upstream := newTestUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		// 不设置 Content-Length 并主动 Flush，强制上游用 chunked 响应
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "chunk-1")
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, "-chunk-2")
	})
	srv := newForwardServer(t, upstream.Listener.Addr().String(), nil)

	c := dialServer(t, srv)
	resp, body := c.do(getRequest("/v1/echo"), "GET")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if body != "chunk-1-chunk-2" {
		t.Errorf("body = %q, want chunk-1-chunk-2", body)
	}

	raw := c.raw()
	if !strings.Contains(raw, "Transfer-Encoding: chunked") {
		t.Errorf("client should receive chunked encoding, got raw: %q", raw)
	}
	if strings.Contains(raw, "Content-Length:") {
		t.Errorf("chunked response must not carry Content-Length, got raw: %q", raw)
	}
}

// TC-05 HEAD 请求：不返回 body，但保留上游的 Content-Length。
func TestForwardHeadRequest(t *testing.T) {
	upstream := newTestUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, "hello")
		}
	})
	srv := newForwardServer(t, upstream.Listener.Addr().String(), nil)

	c := dialServer(t, srv)
	resp, body := c.do(getRequest("/v1/echo"), "HEAD")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
	if resp.ContentLength != 5 {
		t.Errorf("ContentLength = %d, want 5 (preserved from upstream)", resp.ContentLength)
	}
}

// TC-06 请求头超限：返回 413，不转发并关闭连接。
func TestForwardHeaderTooLarge(t *testing.T) {
	upstream := newTestUpstream(t, nil)
	srv := newForwardServer(t, upstream.Listener.Addr().String(), func(cfg *bfe_conf.BfeConfig) {
		cfg.Server.MaxHeaderBytes = 64
	})

	c := dialServer(t, srv)

	// 头部远超 MaxHeaderBytes（服务端读限额为 MaxHeaderBytes+4096）
	c.send(getRequest("/v1/echo", "X-Padding: "+strings.Repeat("a", 8<<10)))

	raw := c.readRawUntilEOF()
	if !strings.Contains(raw, "413 Request Entity Too Large") {
		t.Fatalf("expected 413, got raw response: %q", raw)
	}
	if got := upstream.hits.Load(); got != 0 {
		t.Errorf("upstream hits = %d, want 0 (rejected before forwarding)", got)
	}
	if got := srv.serverStatus.ProxyState.ErrClientLongHeader.Get(); got != 1 {
		t.Errorf("ErrClientLongHeader = %d, want 1", got)
	}
}

// TC-07 请求 URI 超限：返回 414 并关闭连接。
//
// 说明：方案文档原定本用例覆盖"请求体超限"，但该路径（chunk_writer 的
// maxPostHandlerReadBytes）在正常转发下不可达——transport 转发时已把请求体读完，
// w.req.Body 处于 EOF。故替换为同一 serve() 分支族、确定性更强的 URI 超限。
func TestForwardUriTooLong(t *testing.T) {
	upstream := newTestUpstream(t, nil)
	srv := newForwardServer(t, upstream.Listener.Addr().String(), func(cfg *bfe_conf.BfeConfig) {
		cfg.Server.MaxHeaderUriBytes = 32
	})

	c := dialServer(t, srv)
	c.send(getRequest("/v1/echo/" + strings.Repeat("x", 128)))

	raw := c.readRawUntilEOF()
	if !strings.Contains(raw, "414 Request-URI Too Long") {
		t.Fatalf("expected 414, got raw response: %q", raw)
	}
	if got := upstream.hits.Load(); got != 0 {
		t.Errorf("upstream hits = %d, want 0", got)
	}
	if got := srv.serverStatus.ProxyState.ErrClientLongUrl.Get(); got != 1 {
		t.Errorf("ErrClientLongUrl = %d, want 1", got)
	}
}
