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

package httputil

import (
	"bytes"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	http "github.com/bfenetworks/bfe/bfe_http"
)

// fakeConn 提供可回放的读端与可断言的写端。相比 net.Pipe，它没有同步耦合，
// 不会因为一端未并发读写而阻塞，适合状态机的确定性测试。
type fakeConn struct {
	in       *strings.Reader
	out      bytes.Buffer
	closed   bool
	writeErr error // 非空时 Write 直接返回该错误，用于构造写失败分支
}

func newFakeConn(input string) *fakeConn {
	return &fakeConn{in: strings.NewReader(input)}
}

func (c *fakeConn) Read(p []byte) (int, error) { return c.in.Read(p) }

func (c *fakeConn) Write(p []byte) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	return c.out.Write(p)
}

func (c *fakeConn) Close() error {
	c.closed = true
	return nil
}

func (c *fakeConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
}

func (c *fakeConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 54321}
}

func (c *fakeConn) SetDeadline(t time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(t time.Time) error { return nil }

func (c *fakeConn) written() string { return c.out.String() }

func newTestServerConn(t *testing.T, input string) (*ServerConn, *fakeConn) {
	t.Helper()
	c := newFakeConn(input)
	return NewServerConn(c, nil), c
}

func mustReadRequest(t *testing.T, sc *ServerConn) *http.Request {
	t.Helper()
	req, err := sc.Read()
	if req == nil {
		t.Fatalf("Read returned nil request, err=%v", err)
	}
	return req
}

func newTestResponse(req *http.Request, status int, body string) *http.Response {
	resp := &http.Response{
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		ContentLength: int64(len(body)),
		Body:          io.NopCloser(strings.NewReader(body)),
		Request:       req,
	}
	resp.Header["Content-Length"] = []string{strconv.Itoa(len(body))}
	if body == "" {
		resp.Body = nil
	}
	return resp
}

// TC-01 读取单个请求：方法/URI/头/长度/待应答计数。
func TestServerConnReadSingleRequest(t *testing.T) {
	sc, _ := newTestServerConn(t, "GET /a?b=1 HTTP/1.1\r\nHost: example.org\r\nX-Test: v\r\n\r\n")

	req, err := sc.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if req == nil {
		t.Fatal("Read returned nil request")
	}
	if req.Method != "GET" {
		t.Errorf("method = %q, want GET", req.Method)
	}
	if req.RequestURI != "/a?b=1" {
		t.Errorf("requestURI = %q, want /a?b=1", req.RequestURI)
	}
	if req.Host != "example.org" {
		t.Errorf("host = %q, want example.org", req.Host)
	}
	if got := req.Header.Get("X-Test"); got != "v" {
		t.Errorf("X-Test = %q, want v", got)
	}
	if req.ContentLength != 0 {
		t.Errorf("contentLength = %d, want 0", req.ContentLength)
	}
	if got := sc.Pending(); got != 1 {
		t.Errorf("Pending = %d, want 1", got)
	}
}

// TC-02 pipelining：请求可乱序读出，Pending 反映未应答数。
func TestServerConnPipelining(t *testing.T) {
	sc, _ := newTestServerConn(t,
		"GET /one HTTP/1.1\r\nHost: h\r\n\r\n"+
			"GET /two HTTP/1.1\r\nHost: h\r\n\r\n")

	req1 := mustReadRequest(t, sc)
	req2 := mustReadRequest(t, sc)

	if req1.RequestURI != "/one" || req2.RequestURI != "/two" {
		t.Fatalf("unexpected order: %q, %q", req1.RequestURI, req2.RequestURI)
	}
	if got := sc.Pending(); got != 2 {
		t.Errorf("Pending after 2 reads = %d, want 2", got)
	}

	resp := newTestResponse(req1, 200, "ok")
	if err := sc.Write(req1, resp); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if got := sc.Pending(); got != 1 {
		t.Errorf("Pending after 1 response = %d, want 1", got)
	}
}

// TC-03 坏请求：返回错误并被记住，后续 Read 直接复用该错误。
func TestServerConnBadRequest(t *testing.T) {
	sc, _ := newTestServerConn(t, "BOGUS\r\n\r\n")

	req, err := sc.Read()
	if err == nil {
		t.Fatalf("expected error for malformed request, got req=%+v", req)
	}

	// 错误是粘性的：连接已不可用，再次 Read 不应重新解析
	if _, err2 := sc.Read(); err2 == nil {
		t.Fatal("second Read should return the remembered error")
	}
}

// TC-04 请求体：Content-Length 与 chunked 两种编码均能完整读出。
func TestServerConnReadRequestBody(t *testing.T) {
	t.Run("ContentLength", func(t *testing.T) {
		sc, _ := newTestServerConn(t,
			"POST /u HTTP/1.1\r\nHost: h\r\nContent-Length: 5\r\n\r\nhello")
		req := mustReadRequest(t, sc)
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		if string(body) != "hello" {
			t.Errorf("body = %q, want hello", body)
		}
		if err := req.Body.Close(); err != nil {
			t.Errorf("close body failed: %v", err)
		}
	})

	t.Run("Chunked", func(t *testing.T) {
		sc, _ := newTestServerConn(t,
			"POST /u HTTP/1.1\r\nHost: h\r\nTransfer-Encoding: chunked\r\n\r\n"+
				"5\r\nhello\r\n0\r\n\r\n")
		req := mustReadRequest(t, sc)
		if len(req.TransferEncoding) != 1 || req.TransferEncoding[0] != "chunked" {
			t.Fatalf("transferEncoding = %v, want [chunked]", req.TransferEncoding)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		if string(body) != "hello" {
			t.Errorf("body = %q, want hello", body)
		}
	})
}

// TC-05 写定长响应：状态行、Content-Length 与正文。
func TestServerConnWriteFixedLengthResponse(t *testing.T) {
	sc, conn := newTestServerConn(t, "GET / HTTP/1.1\r\nHost: h\r\n\r\n")
	req := mustReadRequest(t, sc)

	resp := newTestResponse(req, 200, "hello")
	if err := sc.Write(req, resp); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	out := conn.written()
	if !strings.HasPrefix(out, "HTTP/1.1 200 OK\r\n") {
		t.Errorf("unexpected status line: %q", firstLine(out))
	}
	if !strings.Contains(out, "Content-Length: 5\r\n") {
		t.Errorf("missing Content-Length in %q", out)
	}
	if !strings.HasSuffix(out, "\r\n\r\nhello") {
		t.Errorf("unexpected body tail: %q", out)
	}
}

// TC-06 写 chunked 响应：Transfer-Encoding 与结尾空块。
func TestServerConnWriteChunkedResponse(t *testing.T) {
	sc, conn := newTestServerConn(t, "GET / HTTP/1.1\r\nHost: h\r\n\r\n")
	req := mustReadRequest(t, sc)

	resp := newTestResponse(req, 200, "hello")
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
	resp.TransferEncoding = []string{"chunked"}

	if err := sc.Write(req, resp); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	out := conn.written()
	if !strings.Contains(out, "Transfer-Encoding: chunked\r\n") {
		t.Errorf("missing Transfer-Encoding in %q", out)
	}
	if !strings.HasSuffix(out, "0\r\n\r\n") {
		t.Errorf("unexpected chunked tail: %q", out)
	}
	if !strings.Contains(out, "5\r\nhello\r\n") {
		t.Errorf("missing chunk payload in %q", out)
	}
}

// TC-07 HEAD 响应：不写正文，但保留 Content-Length。
func TestServerConnWriteHeadResponse(t *testing.T) {
	sc, conn := newTestServerConn(t, "HEAD / HTTP/1.1\r\nHost: h\r\n\r\n")
	req := mustReadRequest(t, sc)

	resp := &http.Response{
		StatusCode:    200,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Length": []string{"5"}},
		ContentLength: 5,
		Request:       req,
	}
	if err := sc.Write(req, resp); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	out := conn.written()
	if !strings.Contains(out, "Content-Length: 5\r\n") {
		t.Errorf("HEAD should keep Content-Length, got %q", out)
	}
	if !strings.HasSuffix(out, "\r\n\r\n") {
		t.Errorf("HEAD should not write a body, got %q", out)
	}
}

// TC-08 Hijack 与 Close：Hijack 后交出连接，Close 幂等且不重复关闭。
func TestServerConnHijackAndClose(t *testing.T) {
	sc, _ := newTestServerConn(t, "GET / HTTP/1.1\r\nHost: h\r\n\r\n")

	c, r := sc.Hijack()
	if c == nil || r == nil {
		t.Fatal("Hijack should return both conn and reader")
	}
	if c2, r2 := sc.Hijack(); c2 != nil || r2 != nil {
		t.Error("second Hijack should return nil, nil")
	}

	// Hijack 后底层连接已交出，Close 不应再关闭它
	if err := sc.Close(); err != nil {
		t.Errorf("Close after Hijack should be no-op, got %v", err)
	}

	// 已交出读端，再 Read 属于 API 误用
	if _, err := sc.Read(); err == nil {
		t.Error("Read after Hijack should fail")
	}
}

// 边界：对端在请求中途关闭被视为优雅关闭（ErrPersistEOF）。
func TestServerConnGracefulCloseMidRequest(t *testing.T) {
	sc, _ := newTestServerConn(t, "GET / HTTP/1.1\r\nHost: h\r\n")

	req, err := sc.Read()
	if err != ErrPersistEOF {
		t.Fatalf("err = %v, want ErrPersistEOF", err)
	}
	if req != nil {
		t.Errorf("req should be nil on graceful close, got %+v", req)
	}
}

// 边界：未读过请求就写响应属于流水线错误。
func TestServerConnWriteWithoutRead(t *testing.T) {
	sc, _ := newTestServerConn(t, "GET / HTTP/1.1\r\nHost: h\r\n\r\n")

	req := &http.Request{
		Method:     "GET",
		RequestURI: "/",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}
	err := sc.Write(req, newTestResponse(req, 200, ""))
	if err != ErrPipeline {
		t.Fatalf("err = %v, want ErrPipeline", err)
	}
}

// 边界：响应带 Close 时，之后的 Read 立即返回 ErrPersistEOF。
func TestServerConnCloseResponseStopsKeepAlive(t *testing.T) {
	sc, _ := newTestServerConn(t, "GET / HTTP/1.1\r\nHost: h\r\n\r\n")
	req := mustReadRequest(t, sc)

	resp := newTestResponse(req, 200, "bye")
	resp.Close = true
	if err := sc.Write(req, resp); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if _, err := sc.Read(); err != ErrPersistEOF {
		t.Fatalf("err = %v, want ErrPersistEOF after close response", err)
	}
}

// newClientRequest 构造可直接写入 ClientConn 的请求。
// 注意：bfe_http.NewRequest 不初始化 Request.State，而 Request.write 会写
// req.State.BodySize，因此这里必须显式补齐，否则 Write 会空指针。
func newClientRequest(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.State = new(http.RequestState)
	return req
}

// TC-09 ClientConn 往返：Write 发出请求，Read 取回响应。
func TestClientConnDoRoundTrip(t *testing.T) {
	conn := newFakeConn("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
	cc := NewClientConn(conn, nil)

	req := newClientRequest(t, "http://example.org/a")

	if err := cc.Write(req); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if got := cc.Pending(); got != 1 {
		t.Errorf("Pending after write = %d, want 1", got)
	}

	resp, err := cc.Read(req)
	if resp == nil {
		t.Fatalf("Read returned nil response, err=%v", err)
	}
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
	if got := cc.Pending(); got != 0 {
		t.Errorf("Pending after read = %d, want 0", got)
	}
	if !strings.Contains(conn.written(), "GET /a HTTP/1.1\r\n") {
		t.Errorf("unexpected request line: %q", firstLine(conn.written()))
	}
}

// TC-10 ClientConn 关闭语义：req.Close 之后写侧被标记为已关闭。
//
// 这里直接断言 cc.we 而不再发第二次 Write：ClientConn.Write 在走到错误返回时，
// 其 defer 会调用 StartResponse(id) 等待上一次响应结束，若上一次写入是 Close
// 请求且响应未读，就会永久阻塞（流水线等待）。该路径在 BFE 内无调用方，见
// 方案文档 §4.6 的说明，此处不做触发死锁的用例。
func TestClientConnCloseRequestStopsWrites(t *testing.T) {
	conn := newFakeConn("")
	cc := NewClientConn(conn, nil)

	req := newClientRequest(t, "http://example.org/a")
	req.Close = true
	if err := cc.Write(req); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	if cc.we != ErrPersistEOF {
		t.Fatalf("write side error = %v, want ErrPersistEOF", cc.we)
	}
}

// TC-10b ClientConn 误用：Hijack 之后写请求返回 errClosed。
func TestClientConnWriteAfterHijack(t *testing.T) {
	conn := newFakeConn("")
	cc := NewClientConn(conn, nil)
	cc.Hijack()

	req := newClientRequest(t, "http://example.org/a")
	if err := cc.Write(req); err != errClosed {
		t.Fatalf("err = %v, want errClosed", err)
	}
}

// TC-11 ClientConn 误用：未 Write 就 Read 返回 ErrPipeline。
func TestClientConnReadWithoutWrite(t *testing.T) {
	conn := newFakeConn("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	cc := NewClientConn(conn, nil)

	req, _ := http.NewRequest("GET", "http://example.org/a", nil)
	if _, err := cc.Read(req); err != ErrPipeline {
		t.Fatalf("err = %v, want ErrPipeline", err)
	}
}

// TC-12 ProxyClientConn：请求行使用 absolute-form。
func TestProxyClientConnWriteAbsoluteURI(t *testing.T) {
	conn := newFakeConn("")
	cc := NewProxyClientConn(conn, nil)

	req := newClientRequest(t, "http://example.org/a?q=1")
	if err := cc.Write(req); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if !strings.HasPrefix(conn.written(), "GET http://example.org/a?q=1 HTTP/1.1\r\n") {
		t.Errorf("unexpected request line: %q", firstLine(conn.written()))
	}
}

// TC-13 Close 语义：Close 关闭底层连接，重复调用不再报错。
func TestServerConnClose(t *testing.T) {
	sc, conn := newTestServerConn(t, "GET / HTTP/1.1\r\nHost: h\r\n\r\n")

	if err := sc.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !conn.closed {
		t.Error("underlying conn should be closed")
	}
	if err := sc.Close(); err != nil {
		t.Errorf("second Close should be no-op, got %v", err)
	}
}

// TC-14 写失败被记住：首次写返回错误，后续写与读都直接返回同一错误。
func TestServerConnWriteErrorIsRemembered(t *testing.T) {
	sc, conn := newTestServerConn(t,
		"GET /1 HTTP/1.1\r\nHost: h\r\n\r\n"+
			"GET /2 HTTP/1.1\r\nHost: h\r\n\r\n")

	// 先把两个请求读出来（流水线允许先读后写），否则 Write 会因未读而返回 ErrPipeline
	req1 := mustReadRequest(t, sc)
	req2 := mustReadRequest(t, sc)

	writeErr := errors.New("write boom")
	conn.writeErr = writeErr

	if err := sc.Write(req1, newTestResponse(req1, 200, "ok")); !errors.Is(err, writeErr) {
		t.Fatalf("Write err = %v, want %v", err, writeErr)
	}

	// 写侧已损坏，第二个响应直接返回同一错误
	if err := sc.Write(req2, newTestResponse(req2, 200, "ok")); !errors.Is(err, writeErr) {
		t.Fatalf("second Write err = %v, want %v", err, writeErr)
	}

	// 读侧也应立即失败：写侧损坏后不再接收新请求
	if _, err := sc.Read(); !errors.Is(err, writeErr) {
		t.Errorf("Read err = %v, want %v", err, writeErr)
	}
}

// TC-15 ClientConn Close：关闭底层连接，重复调用为 no-op。
func TestClientConnClose(t *testing.T) {
	conn := newFakeConn("")
	cc := NewClientConn(conn, nil)

	if err := cc.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !conn.closed {
		t.Error("underlying conn should be closed")
	}
	if err := cc.Close(); err != nil {
		t.Errorf("second Close should be no-op, got %v", err)
	}
}

// TC-16 ClientConn.Do：一次调用完成写请求 + 读响应。
func TestClientConnDo(t *testing.T) {
	conn := newFakeConn("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
	cc := NewClientConn(conn, nil)

	resp, err := cc.Do(newClientRequest(t, "http://example.org/a"))
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if resp == nil || resp.StatusCode != 200 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
}

// TC-17 响应要求关闭后，再写请求返回读侧错误（不再复用连接）。
func TestClientConnWriteAfterCloseResponse(t *testing.T) {
	conn := newFakeConn("HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Length: 0\r\n\r\n")
	cc := NewClientConn(conn, nil)

	req := newClientRequest(t, "http://example.org/a")
	if err := cc.Write(req); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if _, err := cc.Read(req); err != ErrPersistEOF {
		t.Fatalf("Read err = %v, want ErrPersistEOF", err)
	}

	// 读侧已确认连接关闭，后续写请求应直接返回该错误
	req2 := newClientRequest(t, "http://example.org/b")
	if err := cc.Write(req2); err != ErrPersistEOF {
		t.Fatalf("Write err = %v, want ErrPersistEOF", err)
	}
}

func firstLine(s string) string {
	if i := strings.Index(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}
