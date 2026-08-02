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

package mod_access

import (
	"bytes"
	"errors"
	"net"
	"testing"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

type testConn struct {
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (c *testConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (c *testConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (c *testConn) Close() error                       { return nil }
func (c *testConn) LocalAddr() net.Addr                { return c.localAddr }
func (c *testConn) RemoteAddr() net.Addr               { return c.remoteAddr }
func (c *testConn) SetDeadline(t time.Time) error      { return nil }
func (c *testConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *testConn) SetWriteDeadline(t time.Time) error { return nil }

func newTestConn() net.Conn {
	return &testConn{
		localAddr:  &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080},
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345},
	}
}

func prepareRequestLogTest(t *testing.T) (*bfe_basic.Request, *bfe_http.Response, *bytes.Buffer) {
	conn := newTestConn()

	session := bfe_basic.NewSession(conn)
	session.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}
	session.Vip = net.ParseIP("10.0.0.1")
	session.StartTime = time.Now().Add(-time.Second)
	session.SetTrustSource(true)

	req, err := bfe_http.NewRequest("GET", "http://www.example.org/path?key=value", nil)
	if err != nil {
		t.Fatalf("NewRequest() error: %v", err)
	}
	req.RequestURI = "/path?key=value"
	req.Header.Set("User-Agent", "test-agent")
	req.Header.Set("Referer", "http://referer.example.org")
	req.AddCookie(&bfe_http.Cookie{Name: "sid", Value: "abc123"})
	req.State = &bfe_http.RequestState{SerialNumber: 2}

	outReq, _ := bfe_http.NewRequest("GET", "http://backend/path?key=value", nil)
	outReq.State = &bfe_http.RequestState{
		ConnectBackendStart: time.Now().Add(-300 * time.Millisecond),
		ConnectBackendEnd:   time.Now().Add(-200 * time.Millisecond),
	}

	stat := bfe_basic.NewRequestStat(time.Now().Add(-500 * time.Millisecond))
	stat.ReadReqEnd = time.Now().Add(-400 * time.Millisecond)
	stat.ClusterStart = time.Now().Add(-300 * time.Millisecond)
	stat.ClusterEnd = time.Now().Add(-200 * time.Millisecond)
	stat.BackendStart = time.Now().Add(-250 * time.Millisecond)
	stat.BackendEnd = time.Now().Add(-150 * time.Millisecond)
	stat.BackendFirst = time.Now().Add(-390 * time.Millisecond)
	stat.ResponseStart = time.Now().Add(-100 * time.Millisecond)
	stat.ResponseEnd = time.Now().Add(-50 * time.Millisecond)
	stat.HeaderLenIn = 100
	stat.BodyLenIn = 200
	stat.HeaderLenOut = 80
	stat.BodyLenOut = 160

	bfeReq := bfe_basic.NewRequest(req, conn, stat, session, nil)
	bfeReq.OutRequest = outReq
	bfeReq.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 54321}
	bfeReq.LogId = "logid-123"
	bfeReq.Route.Product = "unittest"
	bfeReq.ErrCode = errors.New("err001")
	bfeReq.ErrMsg = "timeout"
	bfeReq.Backend = bfe_basic.BackendInfo{
		ClusterName:    "cluster1",
		SubclusterName: "subcluster1",
		BackendAddr:    "10.0.0.3",
		BackendName:    "backend1",
	}
	bfeReq.RetryTime = 1
	bfeReq.Redirect = bfe_basic.RedirectInfo{Url: "http://redirect.example.org", Code: 302}

	res := &bfe_http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Proto:      "HTTP/1.1",
		Header:     make(bfe_http.Header),
	}
	res.Header.Set("Content-Type", "text/html")
	res.Header.Set("Set-Cookie", "session=xyz")

	return bfeReq, res, bytes.NewBuffer(nil)
}

func TestOnLogFmtRequestLine(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtRequestLine(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtRequestLine() error: %v", err)
	}
	want := "GET /path?key=value HTTP/1.1"
	if buff.String() != want {
		t.Errorf("onLogFmtRequestLine() got: %s, want: %s", buff.String(), want)
	}
}

func TestOnLogFmtHost(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtHost(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtHost() error: %v", err)
	}
	if buff.String() != "www.example.org" {
		t.Errorf("onLogFmtHost() got: %s", buff.String())
	}
}

func TestOnLogFmtStatusCode(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtStatusCode(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtStatusCode() error: %v", err)
	}
	if buff.String() != "200" {
		t.Errorf("onLogFmtStatusCode() got: %s", buff.String())
	}
}

func TestOnLogFmtStatusCodeNilRes(t *testing.T) {
	req, _, buff := prepareRequestLogTest(t)
	err := onLogFmtStatusCode(nil, &LogFmtItem{}, buff, req, nil)
	if err != nil {
		t.Errorf("onLogFmtStatusCode() error: %v", err)
	}
	if buff.String() != "-" {
		t.Errorf("onLogFmtStatusCode() nil res got: %s, want: -", buff.String())
	}
}

func TestOnLogFmtProduct(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtProduct(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtProduct() error: %v", err)
	}
	if buff.String() != "unittest" {
		t.Errorf("onLogFmtProduct() got: %s", buff.String())
	}
}

func TestOnLogFmtClientIp(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtClientIp(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtClientIp() error: %v", err)
	}
	if buff.String() != "10.0.0.2" {
		t.Errorf("onLogFmtClientIp() got: %s", buff.String())
	}
}

func TestOnLogFmtLogId(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtLogId(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtLogId() error: %v", err)
	}
	if buff.String() != "logid-123" {
		t.Errorf("onLogFmtLogId() got: %s", buff.String())
	}
}

func TestOnLogFmtRetryNum(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtRetryNum(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtRetryNum() error: %v", err)
	}
	if buff.String() != "1" {
		t.Errorf("onLogFmtRetryNum() got: %s", buff.String())
	}
}

func TestOnLogFmtBackend(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtBackend(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtBackend() error: %v", err)
	}
	want := "cluster1,subcluster1,10.0.0.3,backend1"
	if buff.String() != want {
		t.Errorf("onLogFmtBackend() got: %s, want: %s", buff.String(), want)
	}
}

func TestOnLogFmtClusterName(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtClusterName(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtClusterName() error: %v", err)
	}
	if buff.String() != "cluster1" {
		t.Errorf("onLogFmtClusterName() got: %s", buff.String())
	}
}

func TestOnLogFmtSubclusterName(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtSubclusterName(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtSubclusterName() error: %v", err)
	}
	if buff.String() != "subcluster1" {
		t.Errorf("onLogFmtSubclusterName() got: %s", buff.String())
	}
}

func TestOnLogFmtRequestUri(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtRequestUri(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtRequestUri() error: %v", err)
	}
	if buff.String() != "/path?key=value" {
		t.Errorf("onLogFmtRequestUri() got: %s", buff.String())
	}
}

func TestOnLogFmtUrl(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtUrl(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtUrl() error: %v", err)
	}
	if buff.String() != "http://www.example.org/path?key=value" {
		t.Errorf("onLogFmtUrl() got: %s", buff.String())
	}
}

func TestOnLogFmtRequestHeader(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtRequestHeader(nil, &LogFmtItem{Key: "User-Agent"}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtRequestHeader() error: %v", err)
	}
	if buff.String() != "test-agent" {
		t.Errorf("onLogFmtRequestHeader() got: %s", buff.String())
	}
}

func TestOnLogFmtRequestHeaderNotExist(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtRequestHeader(nil, &LogFmtItem{Key: "X-Not-Exist"}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtRequestHeader() error: %v", err)
	}
	if buff.String() != "-" {
		t.Errorf("onLogFmtRequestHeader() not exist got: %s, want: -", buff.String())
	}
}

func TestOnLogFmtResponseHeader(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtResponseHeader(nil, &LogFmtItem{Key: "Content-Type"}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtResponseHeader() error: %v", err)
	}
	if buff.String() != "text/html" {
		t.Errorf("onLogFmtResponseHeader() got: %s", buff.String())
	}
}

func TestOnLogFmtResponseHeaderNilRes(t *testing.T) {
	req, _, buff := prepareRequestLogTest(t)
	err := onLogFmtResponseHeader(nil, &LogFmtItem{Key: "Content-Type"}, buff, req, nil)
	if err != nil {
		t.Errorf("onLogFmtResponseHeader() error: %v", err)
	}
	if buff.String() != "-" {
		t.Errorf("onLogFmtResponseHeader() nil res got: %s, want: -", buff.String())
	}
}

func TestOnLogFmtReqCookie(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtReqCookie(nil, &LogFmtItem{Key: "sid"}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtReqCookie() error: %v", err)
	}
	if buff.String() != "abc123" {
		t.Errorf("onLogFmtReqCookie() got: %s", buff.String())
	}
}

func TestOnLogFmtReqCookieNotExist(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtReqCookie(nil, &LogFmtItem{Key: "notexist"}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtReqCookie() error: %v", err)
	}
	if buff.String() != "-" {
		t.Errorf("onLogFmtReqCookie() not exist got: %s, want: -", buff.String())
	}
}

func TestOnLogFmtResCookie(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtResCookie(nil, &LogFmtItem{Key: "session"}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtResCookie() error: %v", err)
	}
	if buff.String() != "xyz" {
		t.Errorf("onLogFmtResCookie() got: %s", buff.String())
	}
}

func TestOnLogFmtResCookieNilRes(t *testing.T) {
	req, _, buff := prepareRequestLogTest(t)
	err := onLogFmtResCookie(nil, &LogFmtItem{Key: "session"}, buff, req, nil)
	if err != nil {
		t.Errorf("onLogFmtResCookie() error: %v", err)
	}
	if buff.String() != "-" {
		t.Errorf("onLogFmtResCookie() nil res got: %s, want: -", buff.String())
	}
}

func TestOnLogFmtErrorCode(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtErrorCode(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtErrorCode() error: %v", err)
	}
	want := "err001,timeout"
	if buff.String() != want {
		t.Errorf("onLogFmtErrorCode() got: %s, want: %s", buff.String(), want)
	}
}

func TestOnLogFmtRedirect(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtRedirect(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtRedirect() error: %v", err)
	}
	want := "http://redirect.example.org,302"
	if buff.String() != want {
		t.Errorf("onLogFmtRedirect() got: %s, want: %s", buff.String(), want)
	}
}

func TestOnLogFmtResProto(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtResProto(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtResProto() error: %v", err)
	}
	if buff.String() != "HTTP/1.1" {
		t.Errorf("onLogFmtResProto() got: %s", buff.String())
	}
}

func TestOnLogFmtResStatus(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtResStatus(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtResStatus() error: %v", err)
	}
	if buff.String() != "200 OK" {
		t.Errorf("onLogFmtResStatus() got: %s", buff.String())
	}
}

func TestOnLogFmtVip(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtVip(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtVip() error: %v", err)
	}
	if buff.String() != "10.0.0.1" {
		t.Errorf("onLogFmtVip() got: %s", buff.String())
	}
}

func TestOnLogFmtIsTrustip(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtIsTrustip(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtIsTrustip() error: %v", err)
	}
	if buff.String() != "true" {
		t.Errorf("onLogFmtIsTrustip() got: %s", buff.String())
	}
}

func TestOnLogFmtNthReqInSession(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtNthReqInSession(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtNthReqInSession() error: %v", err)
	}
	if buff.String() != "2" {
		t.Errorf("onLogFmtNthReqInSession() got: %s", buff.String())
	}
}

func TestOnLogFmtReqBodyLen(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtReqBodyLen(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtReqBodyLen() error: %v", err)
	}
	if buff.String() != "200" {
		t.Errorf("onLogFmtReqBodyLen() got: %s", buff.String())
	}
}

func TestOnLogFmtResBodyLen(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtResBodyLen(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtResBodyLen() error: %v", err)
	}
	if buff.String() != "160" {
		t.Errorf("onLogFmtResBodyLen() got: %s", buff.String())
	}
}

func TestOnLogFmtReqHeaderLen(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtReqHeaderLen(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtReqHeaderLen() error: %v", err)
	}
	if buff.String() != "100" {
		t.Errorf("onLogFmtReqHeaderLen() got: %s", buff.String())
	}
}

func TestOnLogFmtResHeaderLen(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtResHeaderLen(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtResHeaderLen() error: %v", err)
	}
	if buff.String() != "80" {
		t.Errorf("onLogFmtResHeaderLen() got: %s", buff.String())
	}
}

func TestOnLogFmtResLen(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtResLen(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtResLen() error: %v", err)
	}
	if buff.String() != "240" {
		t.Errorf("onLogFmtResLen() got: %s", buff.String())
	}
}

func TestOnLogFmtTime(t *testing.T) {
	buff := bytes.NewBuffer(nil)
	err := onLogFmtTime(nil, buff)
	if err != nil {
		t.Errorf("onLogFmtTime() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtTime() output is empty")
	}
}

func TestOnLogFmtRequestTime(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtRequestTime(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtRequestTime() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtRequestTime() output is empty")
	}
}

func TestOnLogFmtAllServeTime(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtAllServeTime(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtAllServeTime() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtAllServeTime() output is empty")
	}
}

func TestOnLogFmtReadReqDuration(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtReadReqDuration(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtReadReqDuration() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtReadReqDuration() output is empty")
	}
}

func TestOnLogFmtClusterDuration(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtClusterDuration(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtClusterDuration() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtClusterDuration() output is empty")
	}
}

func TestOnLogFmtLastBackendDuration(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtLastBackendDuration(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtLastBackendDuration() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtLastBackendDuration() output is empty")
	}
}

func TestOnLogFmtWriteSrvTime(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtWriteSrvTime(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtWriteSrvTime() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtWriteSrvTime() output is empty")
	}
}

func TestOnLogFmtResDuration(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtResDuration(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtResDuration() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtResDuration() output is empty")
	}
}

func TestOnLogFmtProxyDelayTime(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtProxyDelayTime(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtProxyDelayTime() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtProxyDelayTime() output is empty")
	}
}

func TestOnLogFmtReadWriteSrvTime(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtReadWriteSrvTime(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtReadWriteSrvTime() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtReadWriteSrvTime() output is empty")
	}
}

func TestOnLogFmtConnectBackendTime(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtConnectBackendTime(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtConnectBackendTime() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtConnectBackendTime() output is empty")
	}
}

func TestOnLogFmtSinceSessionTime(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtSinceSessionTime(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtSinceSessionTime() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtSinceSessionTime() output is empty")
	}
}

func TestOnLogFmtServerAddr(t *testing.T) {
	req, res, buff := prepareRequestLogTest(t)
	err := onLogFmtServerAddr(nil, &LogFmtItem{}, buff, req, res)
	if err != nil {
		t.Errorf("onLogFmtServerAddr() error: %v", err)
	}
	if buff.Len() == 0 {
		t.Error("onLogFmtServerAddr() output is empty")
	}
}

func TestOnLogFmtNilReq(t *testing.T) {
	buff := bytes.NewBuffer(nil)

	tests := []struct {
		name string
		fn   func(*ModuleAccess, *LogFmtItem, *bytes.Buffer, *bfe_basic.Request, *bfe_http.Response) error
	}{
		{"AllServeTime", onLogFmtAllServeTime},
		{"RequestTime", onLogFmtRequestTime},
		{"RequestLine", onLogFmtRequestLine},
		{"Backend", onLogFmtBackend},
		{"ReqBodyLen", onLogFmtReqBodyLen},
		{"ResBodyLen", onLogFmtResBodyLen},
		{"ClusterDuration", onLogFmtClusterDuration},
		{"ClusterName", onLogFmtClusterName},
		{"ConnectBackendTime", onLogFmtConnectBackendTime},
		{"Host", onLogFmtHost},
		{"IsTrustip", onLogFmtIsTrustip},
		{"LastBackendDuration", onLogFmtLastBackendDuration},
		{"LogId", onLogFmtLogId},
		{"NthReqInSession", onLogFmtNthReqInSession},
		{"StatusCode", onLogFmtStatusCode},
		{"Product", onLogFmtProduct},
		{"ProxyDelayTime", onLogFmtProxyDelayTime},
		{"ReadReqDuration", onLogFmtReadReqDuration},
		{"ReadWriteSrvTime", onLogFmtReadWriteSrvTime},
		{"Redirect", onLogFmtRedirect},
		{"ClientIp", onLogFmtClientIp},
		{"ReqCookie", onLogFmtReqCookie},
		{"ErrorCode", onLogFmtErrorCode},
		{"ReqHeaderLen", onLogFmtReqHeaderLen},
		{"RequestHeader", onLogFmtRequestHeader},
		{"RequestUri", onLogFmtRequestUri},
		{"ResCookie", onLogFmtResCookie},
		{"ResDuration", onLogFmtResDuration},
		{"ResProto", onLogFmtResProto},
		{"ResponseHeader", onLogFmtResponseHeader},
		{"ResHeaderLen", onLogFmtResHeaderLen},
		{"ResLen", onLogFmtResLen},
		{"ResStatus", onLogFmtResStatus},
		{"RetryNum", onLogFmtRetryNum},
		{"ServerAddr", onLogFmtServerAddr},
		{"SinceSessionTime", onLogFmtSinceSessionTime},
		{"SubclusterName", onLogFmtSubclusterName},
		{"Vip", onLogFmtVip},
		{"Url", onLogFmtUrl},
		{"WriteSrvTime", onLogFmtWriteSrvTime},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buff.Reset()
			err := tt.fn(nil, &LogFmtItem{}, buff, nil, nil)
			if err == nil {
				t.Errorf("onLogFmt%s() should return error when req is nil", tt.name)
			}
		})
	}
}

func TestOnLogFmtNilStat(t *testing.T) {
	conn := newTestConn()
	session := bfe_basic.NewSession(conn)
	req, _ := bfe_http.NewRequest("GET", "http://www.example.org/", nil)
	req.RequestURI = "/"
	bfeReq := bfe_basic.NewRequest(req, nil, nil, session, nil)
	buff := bytes.NewBuffer(nil)

	tests := []struct {
		name string
		fn   func(*ModuleAccess, *LogFmtItem, *bytes.Buffer, *bfe_basic.Request, *bfe_http.Response) error
	}{
		{"AllServeTime", onLogFmtAllServeTime},
		{"ReqBodyLen", onLogFmtReqBodyLen},
		{"ResBodyLen", onLogFmtResBodyLen},
		{"ClusterDuration", onLogFmtClusterDuration},
		{"LastBackendDuration", onLogFmtLastBackendDuration},
		{"ProxyDelayTime", onLogFmtProxyDelayTime},
		{"ReadReqDuration", onLogFmtReadReqDuration},
		{"ReadWriteSrvTime", onLogFmtReadWriteSrvTime},
		{"ReqHeaderLen", onLogFmtReqHeaderLen},
		{"ResDuration", onLogFmtResDuration},
		{"WriteSrvTime", onLogFmtWriteSrvTime},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buff.Reset()
			err := tt.fn(nil, &LogFmtItem{}, buff, bfeReq, nil)
			if err == nil {
				t.Errorf("onLogFmt%s() should return error when req.Stat is nil", tt.name)
			}
		})
	}
}

func TestOnLogFmtNilHttpRequest(t *testing.T) {
	conn := newTestConn()
	session := bfe_basic.NewSession(conn)
	bfeReq := &bfe_basic.Request{Session: session}
	buff := bytes.NewBuffer(nil)

	tests := []struct {
		name string
		fn   func(*ModuleAccess, *LogFmtItem, *bytes.Buffer, *bfe_basic.Request, *bfe_http.Response) error
	}{
		{"RequestLine", onLogFmtRequestLine},
		{"Host", onLogFmtHost},
		{"RequestHeader", onLogFmtRequestHeader},
		{"RequestUri", onLogFmtRequestUri},
		{"Url", onLogFmtUrl},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buff.Reset()
			err := tt.fn(nil, &LogFmtItem{}, buff, bfeReq, nil)
			if err == nil {
				t.Errorf("onLogFmt%s() should return error when req.HttpRequest is nil", tt.name)
			}
		})
	}
}

func TestOnLogFmtSinceSessionTimeNilSession(t *testing.T) {
	req, _ := bfe_http.NewRequest("GET", "http://www.example.org/", nil)
	req.RequestURI = "/"
	bfeReq := bfe_basic.NewRequest(req, nil, nil, nil, nil)
	buff := bytes.NewBuffer(nil)

	err := onLogFmtSinceSessionTime(nil, &LogFmtItem{}, buff, bfeReq, nil)
	if err == nil {
		t.Error("onLogFmtSinceSessionTime() should return error when req.Session is nil")
	}
}
