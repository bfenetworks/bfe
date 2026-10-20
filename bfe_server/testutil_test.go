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
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_bufio"
	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_route"
)

// 本文件是 bfe_server 主链路的测试骨架：
//   - newTestUpstream 提供可计数的上游后端；
//   - newForwardServer 用运行期生成的配置拼出一个最小可服务的 BfeServer；
//   - dialServer 通过 loopback TCP 把连接交给 conn.serve()，客户端拿到原始字节流。
//
// 选 loopback TCP 而不是 net.Pipe 的原因：net.Pipe 是同步无缓冲的，客户端写请求与
// 服务端写响应会互相等待；真实 TCP 由内核缓冲，且 GetVipPort 能正常解析出 VIP。

const (
	testProduct    = "prod_test"
	testHostTag    = "tag_test"
	testCluster    = "cluster_test"
	testSubCluster = "cluster_test.a"
	testHost       = "example.org"
)

// testUpstream 是带命中计数的 httptest 后端。
type testUpstream struct {
	*httptest.Server
	hits atomic.Int64

	mu       sync.Mutex
	lastHost string
	lastBody string
}

// lastRequest 返回最近一次收到的 Host 与 body，供断言使用。
func (u *testUpstream) lastRequest() (string, string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastHost, u.lastBody
}

// newTestUpstream 启动一个 mock 后端。handler 为 nil 时返回固定响应。
func newTestUpstream(t *testing.T, handler http.HandlerFunc) *testUpstream {
	t.Helper()

	u := &testUpstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		u.hits.Add(1)
		u.mu.Lock()
		u.lastHost = r.Host
		u.lastBody = string(body)
		u.mu.Unlock()

		if handler != nil {
			handler(w, r)
			return
		}
		w.Header().Set("X-Upstream", "ok")
		_, _ = io.WriteString(w, "upstream-body")
	}))
	t.Cleanup(u.Close)
	return u
}

// forwardOpts 描述一个测试用转发服务。
type forwardOpts struct {
	// upstreamAddr 是上游地址；为空时使用一个已关闭的端口（模拟连不上后端）
	upstreamAddr string
	// clusterExtra 注入 cluster_conf.data 中 cluster_test 对象的额外 JSON 字段，
	// 不要结尾逗号，例如 `"BackendConf": {"TimeoutResponseHeader": 50}`
	clusterExtra string
	// tweak 在 NewBfeServer 之前覆盖 bfe.conf 中的字段（如 MaxHeaderBytes）
	tweak func(*bfe_conf.BfeConfig)
}

// newForwardServer 构造一个最小可服务的 BfeServer：
// 主机 example.org -> 产品 prod_test -> 集群 cluster_test -> upstreamAddr。
func newForwardServer(t *testing.T, upstreamAddr string, tweak func(*bfe_conf.BfeConfig)) *BfeServer {
	t.Helper()
	return newForwardServerWith(t, forwardOpts{upstreamAddr: upstreamAddr, tweak: tweak})
}

// closedAddr 返回一个当前没有监听者的地址，用于构造后端不可达的场景。
func closedAddr(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener failed: %v", err)
	}
	return addr
}

func newForwardServerWith(t *testing.T, opts forwardOpts) *BfeServer {
	t.Helper()

	upstreamAddr := opts.upstreamAddr
	if upstreamAddr == "" {
		upstreamAddr = closedAddr(t)
	}

	host, portStr, err := net.SplitHostPort(upstreamAddr)
	if err != nil {
		t.Fatalf("split upstream addr %q failed: %v", upstreamAddr, err)
	}

	dir := t.TempDir()
	writeConf := func(name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s failed: %v", name, err)
		}
		return path
	}

	hostFile := writeConf("host.data", fmt.Sprintf(`{
	"Version": "1",
	"Hosts": {"%s": ["%s"]},
	"HostTags": {"%s": ["%s"]}
}`, testHostTag, testHost, testProduct, testHostTag))

	// VIP 表把回环地址映射到同一产品，使 VIP 兜底路径与域名路径都可用
	vipFile := writeConf("vip.data", fmt.Sprintf(`{
	"Version": "1",
	"Vips": {"%s": ["127.0.0.1"]}
}`, testProduct))

	routeFile := writeConf("route.data", fmt.Sprintf(`{
	"Version": "1",
	"BasicRule": {
		"%s": [{"Hostname": ["*"], "ClusterName": "%s"}]
	}
}`, testProduct, testCluster))

	// cluster_conf.data 承载集群级参数（ClusterBasic/BackendConf/CheckConf），
	// 与只描述 subcluster/backends 的 cluster_table.data 是两类文件。
	clusterConfFile := writeConf("cluster_conf.data", fmt.Sprintf(`{
	"Version": "1",
	"Config": {
		"%s": {%s}
	}
}`, testCluster, opts.clusterExtra))

	clusterTableFile := writeConf("cluster_table.data", fmt.Sprintf(`{
	"Version": "1",
	"Config": {
		"%s": {
			"%s": [{"Name": "test_backend_0", "Addr": "%s", "Port": %s, "Weight": 10}]
		}
	}
}`, testCluster, testSubCluster, host, portStr))

	gslbFile := writeConf("gslb.data", fmt.Sprintf(`{
	"Version": "1",
	"Clusters": {
		"%s": {"GSLB_BLACKHOLE": 0, "%s": 100}
	},
	"Hostname": "gslb-test",
	"Ts": "1"
}`, testCluster, testSubCluster))

	cfg := bfe_conf.BfeConfig{}
	cfg.Server.SetDefaultConf()
	// 关掉 AI 网关，走传统 ServeHTTP 路径；后续用例可按需覆盖
	cfg.Server.EnableAiGateway = false
	if opts.tweak != nil {
		opts.tweak(&cfg)
	}

	srv := NewBfeServer(cfg, dir, "unit-test")

	conf, err := bfe_route.LoadServerDataConf(hostFile, vipFile, routeFile, clusterConfFile, nil)
	if err != nil {
		t.Fatalf("load server data conf failed: %v", err)
	}
	srv.ServerConf = conf

	if err := srv.balTable.Init(gslbFile, clusterTableFile); err != nil {
		t.Fatalf("init balance table failed: %v", err)
	}

	// 清理顺序（LIFO）：先由 dialServer 注册的 cleanup 关闭客户端连接，
	// 再回到这里等待 conn.serve() 退出，避免 goroutine 泄漏到后续用例。
	t.Cleanup(func() { waitConns(t, srv) })

	return srv
}

// recordingConn 记录读到的所有字节，便于对原始报文（chunked 分帧、状态行）做断言。
type recordingConn struct {
	net.Conn

	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *recordingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.mu.Lock()
		c.buf.Write(p[:n])
		c.mu.Unlock()
	}
	return n, err
}

// raw 返回客户端已读到的原始响应字节。
func (c *recordingConn) raw() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// testClient 是面向 conn.serve() 的测试客户端。
type testClient struct {
	t    *testing.T
	conn *recordingConn
	br   *bfe_bufio.Reader
}

// dialServer 建立一条 loopback 连接并交给 conn.serve() 处理。
func dialServer(t *testing.T, srv *BfeServer) *testClient {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	accepted := make(chan struct{})
	go func() {
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		c, err := newConn(raw, srv)
		if err != nil {
			_ = raw.Close()
			return
		}
		close(accepted)
		c.serve()
	}()

	raw, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })

	select {
	case <-accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not accept the connection")
	}

	rc := &recordingConn{Conn: raw}
	return &testClient{t: t, conn: rc, br: bfe_bufio.NewReader(rc)}
}

// send 写出原始请求报文。
func (c *testClient) send(raw string) {
	c.t.Helper()
	if _, err := io.WriteString(c.conn, raw); err != nil {
		c.t.Fatalf("write request failed: %v (raw response so far: %q)", err, c.conn.raw())
	}
}

// readResponse 读取一个响应。method 影响 HEAD 的 body 语义。
func (c *testClient) readResponse(method string) *bfe_http.Response {
	c.t.Helper()

	req := &bfe_http.Request{Method: method, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1}
	resp, err := bfe_http.ReadResponse(c.br, req)
	if err != nil {
		c.t.Fatalf("read response failed: %v (raw response: %q)", err, c.conn.raw())
	}
	return resp
}

// do 发送请求并读取响应，body 会被完整读出后关闭。
func (c *testClient) do(raw, method string) (*bfe_http.Response, string) {
	c.t.Helper()

	c.send(raw)
	resp := c.readResponse(method)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("read body failed: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		c.t.Fatalf("close body failed: %v", err)
	}
	return resp, string(body)
}

// raw 返回客户端已读到的原始字节。
func (c *testClient) raw() string { return c.conn.raw() }

// expectClosed 断言服务端已关闭连接（读不到更多数据）。
func (c *testClient) expectClosed() {
	c.t.Helper()

	_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	n, err := c.conn.Read(buf)
	if err == nil {
		c.t.Fatalf("connection should be closed, but read %d more byte(s): %q", n, buf[:n])
	}
	// 只有 EOF 才算"对端主动关闭"；超时说明连接还开着，只是没有数据
	if !errors.Is(err, io.EOF) {
		c.t.Fatalf("connection should be closed by the peer, got err=%v", err)
	}
}

// readRawUntilEOF 读到连接关闭为止，返回原始响应字节。
// 用于服务端不发送合法 HTTP 响应（如 413/414 的裸状态行）或需要看分帧细节的用例。
func (c *testClient) readRawUntilEOF() string {
	c.t.Helper()

	_ = c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _ = io.ReadAll(c.br)
	return c.conn.raw()
}

// waitConns 等待服务端连接处理完毕，避免 goroutine 泄漏到后续用例。
func waitConns(t *testing.T, srv *BfeServer) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		srv.connWaitGroup.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("client connections did not finish in 5s")
	}
}

// getRequest 拼一个最小 GET 请求。
func getRequest(path string, extraHeaders ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "GET %s HTTP/1.1\r\nHost: %s\r\n", path, testHost)
	for _, h := range extraHeaders {
		b.WriteString(h)
		b.WriteString("\r\n")
	}
	b.WriteString("\r\n")
	return b.String()
}
