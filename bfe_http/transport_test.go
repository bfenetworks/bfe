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

package bfe_http

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// dialRecorder 是可注入的 Dial：真实连接到 upstream，同时记录拨号次数与目标地址，
// 从而在不监听额外端口的前提下断言连接复用、代理寻址等行为。
type dialRecorder struct {
	upstreamAddr string

	dials atomic.Int64

	mu    sync.Mutex
	addrs []string
}

func newDialRecorder(upstreamAddr string) *dialRecorder {
	return &dialRecorder{upstreamAddr: upstreamAddr}
}

func (d *dialRecorder) dial(network, addr string) (net.Conn, error) {
	d.dials.Add(1)
	d.mu.Lock()
	d.addrs = append(d.addrs, addr)
	d.mu.Unlock()
	return net.Dial("tcp", d.upstreamAddr)
}

func (d *dialRecorder) count() int64 { return d.dials.Load() }

func (d *dialRecorder) lastAddr() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.addrs) == 0 {
		return ""
	}
	return d.addrs[len(d.addrs)-1]
}

// newTransportRequest 构造一个可直接送入 RoundTrip 的请求。
// 注意：RoundTrip 会写 req.State，必须初始化，否则空指针（见方案文档 D3）。
func newTransportRequest(t *testing.T, target string) *Request {
	t.Helper()

	u, err := url.Parse(target)
	if err != nil {
		t.Fatalf("parse %q failed: %v", target, err)
	}
	return &Request{
		Method:     "GET",
		URL:        u,
		Host:       u.Host,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(Header),
		State:      new(RequestState),
	}
}

// drainBody 读完并关闭响应体，使连接能回到空闲池（复用前提）。
func drainBody(t *testing.T, resp *Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body failed: %v", err)
	}
	return string(body)
}

// TC-01 连接复用：连续两个请求只拨号一次。
func TestTransportReusesConnection(t *testing.T) {
	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	dr := newDialRecorder(upstream.Listener.Addr().String())
	tr := &Transport{Dial: dr.dial, MaxIdleConnsPerHost: 4}
	defer tr.CloseIdleConnections()

	for i := 1; i <= 2; i++ {
		resp, err := tr.RoundTrip(newTransportRequest(t, "http://backend.test/echo"))
		if err != nil {
			t.Fatalf("RoundTrip #%d failed: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("RoundTrip #%d status = %d, want 200", i, resp.StatusCode)
		}
		if body := drainBody(t, resp); body != "ok" {
			t.Errorf("RoundTrip #%d body = %q, want ok", i, body)
		}
	}

	if got := dr.count(); got != 1 {
		t.Errorf("dial count = %d, want 1 (second request should reuse the conn)", got)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("upstream hits = %d, want 2", got)
	}
	// 拨号目标是 URL 推导出的后端地址，不是 Dial 实际连的 upstream
	if got, want := dr.lastAddr(), "backend.test:80"; got != want {
		t.Errorf("dial addr = %q, want %q", got, want)
	}
}

// TC-02 DisableKeepAlives：每个请求各建一条连接。
func TestTransportDisableKeepAlives(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	dr := newDialRecorder(upstream.Listener.Addr().String())
	tr := &Transport{Dial: dr.dial, DisableKeepAlives: true}
	defer tr.CloseIdleConnections()

	for i := 1; i <= 2; i++ {
		resp, err := tr.RoundTrip(newTransportRequest(t, "http://backend.test/echo"))
		if err != nil {
			t.Fatalf("RoundTrip #%d failed: %v", i, err)
		}
		drainBody(t, resp)

		if got := dr.count(); got != int64(i) {
			t.Errorf("after request #%d dial count = %d, want %d", i, got, i)
		}
	}
}

// TC-03 拨号失败：包装成 ConnectError 并带上目标地址。
func TestTransportDialError(t *testing.T) {
	wantErr := errors.New("dial boom")
	tr := &Transport{Dial: func(network, addr string) (net.Conn, error) {
		return nil, wantErr
	}}
	defer tr.CloseIdleConnections()

	_, err := tr.RoundTrip(newTransportRequest(t, "http://backend.test/echo"))
	if err == nil {
		t.Fatal("RoundTrip should fail when dial fails")
	}

	var connectErr ConnectError
	if !errors.As(err, &connectErr) {
		t.Fatalf("error type = %T, want ConnectError: %v", err, err)
	}
	if !errors.Is(connectErr.Err, wantErr) {
		t.Errorf("wrapped error = %v, want %v", connectErr.Err, wantErr)
	}
	if connectErr.Addr != "backend.test:80" {
		t.Errorf("ConnectError.Addr = %q, want backend.test:80", connectErr.Addr)
	}
}

// TC-04 响应头超时：上游迟迟不回包时返回 RespHeaderTimeoutError。
func TestTransportResponseHeaderTimeout(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = io.WriteString(w, "late")
	}))
	defer upstream.Close()
	defer close(release)

	dr := newDialRecorder(upstream.Listener.Addr().String())
	tr := &Transport{Dial: dr.dial, ResponseHeaderTimeout: 50 * time.Millisecond}
	defer tr.CloseIdleConnections()

	start := time.Now()
	_, err := tr.RoundTrip(newTransportRequest(t, "http://backend.test/echo"))
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("RoundTrip should time out")
	}
	var timeoutErr RespHeaderTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("error type = %T, want RespHeaderTimeoutError: %v", err, err)
	}
	if elapsed > time.Second {
		t.Errorf("timeout took %v, want well under 1s", elapsed)
	}
}

// TC-05 CloseIdleConnections 后必须重新拨号。
func TestTransportCloseIdleConnections(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	dr := newDialRecorder(upstream.Listener.Addr().String())
	tr := &Transport{Dial: dr.dial}
	defer tr.CloseIdleConnections()

	resp, err := tr.RoundTrip(newTransportRequest(t, "http://backend.test/echo"))
	if err != nil {
		t.Fatalf("first RoundTrip failed: %v", err)
	}
	drainBody(t, resp)

	tr.CloseIdleConnections()

	resp, err = tr.RoundTrip(newTransportRequest(t, "http://backend.test/echo"))
	if err != nil {
		t.Fatalf("second RoundTrip failed: %v", err)
	}
	drainBody(t, resp)

	if got := dr.count(); got != 2 {
		t.Errorf("dial count = %d, want 2 (idle conn was closed)", got)
	}
}

// TC-06 上游提前断开：读响应头即 EOF，包装成 ReadRespHeaderError。
//
// 说明：TransportBrokenError 是"请求已写出但连接静默死亡、100ms 内没有响应事件"
// 的兜底（transport.go 的 failTicker 分支），依赖时序，无法稳定复现；上游直接
// 断连的确定性结果是 ReadRespHeaderError。上层两处（reverseproxy.go:501、
// :942）都按"传输层错误"统一处理，故此处固化前者。
func TestTransportBrokenConnection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	// 接受连接后立即关闭，模拟上游在回包前断开
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_ = conn.Close()
	}()

	dr := newDialRecorder(ln.Addr().String())
	tr := &Transport{Dial: dr.dial}
	defer tr.CloseIdleConnections()

	_, err = tr.RoundTrip(newTransportRequest(t, "http://backend.test/echo"))
	if err == nil {
		t.Fatal("RoundTrip should fail when the upstream closes early")
	}
	var readErr ReadRespHeaderError
	if !errors.As(err, &readErr) {
		t.Fatalf("error type = %T, want ReadRespHeaderError: %v", err, err)
	}
}

// TC-07 MaxConnsPerHost：并发超过上限时快速失败。
func TestTransportMaxConnsPerHost(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(entered) })
		<-release
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()
	defer releaseAll()

	dr := newDialRecorder(upstream.Listener.Addr().String())
	tr := &Transport{Dial: dr.dial, MaxConnsPerHost: 1}
	defer tr.CloseIdleConnections()

	firstDone := make(chan error, 1)
	go func() {
		resp, err := tr.RoundTrip(newTransportRequest(t, "http://backend.test/echo"))
		if err == nil {
			drainBody(t, resp)
		}
		firstDone <- err
	}()

	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first request never reached the upstream")
	}

	// 占满上限后再发一个请求：应在拨号前被拒绝
	_, err := tr.RoundTrip(newTransportRequest(t, "http://backend.test/echo"))
	if err == nil {
		t.Fatal("second concurrent request should be rejected by MaxConnsPerHost")
	}
	if !strings.Contains(err.Error(), "greater than max conns") {
		t.Errorf("unexpected error: %v", err)
	}

	releaseAll()
	if err := <-firstDone; err != nil {
		t.Fatalf("first request failed: %v", err)
	}
}

// TC-08 Proxy：请求发往代理地址，且请求行使用 absolute-form。
func TestTransportProxy(t *testing.T) {
	var gotRequestURI string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	dr := newDialRecorder(upstream.Listener.Addr().String())
	proxyURL, err := url.Parse("http://proxy.local:1234")
	if err != nil {
		t.Fatalf("parse proxy url failed: %v", err)
	}

	tr := &Transport{
		Dial: dr.dial,
		Proxy: func(*Request) (*url.URL, error) {
			return proxyURL, nil
		},
	}
	defer tr.CloseIdleConnections()

	resp, err := tr.RoundTrip(newTransportRequest(t, "http://backend.test/echo?x=1"))
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	drainBody(t, resp)

	if got := dr.lastAddr(); got != "proxy.local:1234" {
		t.Errorf("dial addr = %q, want proxy.local:1234 (proxy address)", got)
	}
	if !strings.HasPrefix(gotRequestURI, "http://backend.test/echo") {
		t.Errorf("upstream RequestURI = %q, want absolute-form", gotRequestURI)
	}
}

// 入参校验：URL/Header 缺失与不支持的 scheme 都返回错误。
func TestTransportInvalidRequest(t *testing.T) {
	tr := &Transport{Dial: func(network, addr string) (net.Conn, error) {
		return nil, errors.New("should not dial")
	}}
	defer tr.CloseIdleConnections()

	tests := []struct {
		name    string
		req     *Request
		wantErr string
	}{
		{
			name:    "NilURL",
			req:     &Request{Method: "GET", Header: make(Header), State: new(RequestState)},
			wantErr: "nil Request.URL",
		},
		{
			name:    "NilHeader",
			req:     &Request{Method: "GET", URL: &url.URL{Scheme: "http", Host: "h"}, State: new(RequestState)},
			wantErr: "nil Request.Header",
		},
		{
			name: "NoHost",
			req: &Request{
				Method: "GET", URL: &url.URL{Scheme: "http"},
				Header: make(Header), State: new(RequestState),
			},
			wantErr: "no Host in request URL",
		},
		{
			name: "UnsupportedScheme",
			req: &Request{
				Method: "GET", URL: &url.URL{Scheme: "ftp", Host: "h"},
				Header: make(Header), State: new(RequestState),
			},
			wantErr: "unsupported protocol scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tr.RoundTrip(tt.req); err == nil {
				t.Fatalf("RoundTrip should fail for %s", tt.name)
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// RegisterProtocol：http/https 不允许覆盖，其他 scheme 走注册的 RoundTripper。
func TestTransportRegisterProtocol(t *testing.T) {
	tr := &Transport{}

	if !panics(func() { tr.RegisterProtocol("http", nil) }) {
		t.Error("registering http should panic")
	}
	if !panics(func() { tr.RegisterProtocol("https", nil) }) {
		t.Error("registering https should panic")
	}

	called := false
	tr.RegisterProtocol("mock", roundTripperFunc(func(req *Request) (*Response, error) {
		called = true
		return &Response{StatusCode: 200, Header: make(Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	}))

	if !panics(func() { tr.RegisterProtocol("mock", nil) }) {
		t.Error("registering the same scheme twice should panic")
	}

	resp, err := tr.RoundTrip(&Request{
		Method: "GET",
		URL:    &url.URL{Scheme: "mock", Host: "h"},
		Header: make(Header),
		State:  new(RequestState),
	})
	if err != nil {
		t.Fatalf("RoundTrip on registered scheme failed: %v", err)
	}
	if !called {
		t.Error("registered RoundTripper was not called")
	}
	if resp == nil || resp.StatusCode != 200 {
		t.Errorf("unexpected response: %+v", resp)
	}
}

// panics 报告 fn 是否 panic。
func panics(fn func()) (panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	fn()
	return
}

// roundTripperFunc 让测试能把函数当作 RoundTripper 使用。
type roundTripperFunc func(*Request) (*Response, error)

func (f roundTripperFunc) RoundTrip(req *Request) (*Response, error) { return f(req) }
