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

package mod_header

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"net"
	"testing"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_modules/mod_geo"
	"github.com/bfenetworks/bfe/bfe_tls"
	"github.com/bfenetworks/bfe/bfe_util"
)

type testConn struct {
	net.Conn
	local  net.Addr
	remote net.Addr
}

func (c *testConn) LocalAddr() net.Addr {
	return c.local
}

func (c *testConn) RemoteAddr() net.Addr {
	return c.remote
}

type testAddrFetcher struct {
	net.Conn
	local      net.Addr
	remote     net.Addr
	virtual    net.Addr
	balancer   net.Addr
}

func (f *testAddrFetcher) LocalAddr() net.Addr    { return f.local }
func (f *testAddrFetcher) RemoteAddr() net.Addr   { return f.remote }
func (f *testAddrFetcher) VirtualAddr() net.Addr  { return f.virtual }
func (f *testAddrFetcher) BalancerAddr() net.Addr { return f.balancer }

func makeRequestForVars() *bfe_basic.Request {
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Context = make(map[interface{}]interface{})
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org/path", nil)
	req.HttpRequest.Host = "www.example.org"
	req.HttpRequest.Proto = "HTTP/1.1"
	req.HttpRequest.State = &bfe_http.RequestState{
		SerialNumber:  2,
		H2Fingerprint: "h2fp",
	}
	req.ClientAddr = &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 12345}
	req.RemoteAddr = &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 54321}
	req.Session.SessionId = "session-123"
	req.LogId = "log-456"
	req.Session.Vip = net.ParseIP("127.0.0.1")
	req.Session.Proto = "h2"
	req.Session.IsSecure = true
	req.Route.ClusterName = "clusterA"
	req.Backend = bfe_basic.BackendInfo{
		ClusterName:    "clusterA",
		SubclusterName: "subA",
		BackendName:    "backend1",
		BackendAddr:    "10.0.0.2:8080",
	}
	req.Stat = &bfe_basic.RequestStat{
		ReadReqStart: time.Now().Add(-time.Second),
		BackendEnd:   time.Now(),
	}
	return req
}

func TestUint16ToStr(t *testing.T) {
	if got := uint16ToStr(0x1234); got != "1234" {
		t.Errorf("uint16ToStr(0x1234) = %s, want 1234", got)
	}
}

func TestGetClientIp(t *testing.T) {
	req := makeRequestForVars()
	if got := getClientIp(req); got != "10.0.0.1" {
		t.Errorf("getClientIp = %s, want 10.0.0.1", got)
	}

	req.ClientAddr = nil
	if got := getClientIp(req); got != "" {
		t.Errorf("getClientIp(nil) = %s, want empty", got)
	}
}

func TestGetClientPort(t *testing.T) {
	req := makeRequestForVars()
	if got := getClientPort(req); got != "12345" {
		t.Errorf("getClientPort = %s, want 12345", got)
	}

	req.ClientAddr = nil
	if got := getClientPort(req); got != "" {
		t.Errorf("getClientPort(nil) = %s, want empty", got)
	}
}

func TestGetRequestHost(t *testing.T) {
	req := makeRequestForVars()
	if got := getRequestHost(req); got != "www.example.org" {
		t.Errorf("getRequestHost = %s, want www.example.org", got)
	}
}

func TestGetProto(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"spdy/2", "20"},
		{"spdy/3", "30"},
		{"spdy/3.1", "31"},
		{"h2", "h2"},
		{"stream", "st"},
		{"http/1.1", "00"},
	}

	for _, tc := range cases {
		if got := getProto(tc.in); got != tc.want {
			t.Errorf("getProto(%s) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestGetReqTime(t *testing.T) {
	req := makeRequestForVars()
	if got := getReqTime(req); got <= 0 {
		t.Errorf("getReqTime = %d, want > 0", got)
	}

	req.Stat.BackendEnd = req.Stat.ReadReqStart.Add(-time.Second)
	if got := getReqTime(req); got != 0 {
		t.Errorf("getReqTime negative = %d, want 0", got)
	}
}

func TestGetConnReused(t *testing.T) {
	req := makeRequestForVars()
	if got := getConnReused(req); got != "R" {
		t.Errorf("getConnReused = %s, want R", got)
	}

	req.HttpRequest.State.SerialNumber = 1
	if got := getConnReused(req); got != "N" {
		t.Errorf("getConnReused = %s, want N", got)
	}

	req.HttpRequest.State = nil
	if got := getConnReused(req); got != "U" {
		t.Errorf("getConnReused(nil state) = %s, want U", got)
	}
}

func TestGetConnResume(t *testing.T) {
	state := &bfe_tls.ConnectionState{DidResume: true}
	if got := getConnResume(state); got != "R" {
		t.Errorf("getConnResume(true) = %s, want R", got)
	}

	state.DidResume = false
	if got := getConnResume(state); got != "N" {
		t.Errorf("getConnResume(false) = %s, want N", got)
	}
}

func TestGetBfeSslFunctions(t *testing.T) {
	req := makeRequestForVars()
	req.Session.TlsState = &bfe_tls.ConnectionState{
		Version:     bfe_tls.VersionTLS12,
		CipherSuite: bfe_tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		DidResume:   true,
		JA3Raw:      "raw",
		JA3Hash:     "hash",
	}

	if got := getBfeSslResume(req); got != "R" {
		t.Errorf("getBfeSslResume = %s, want R", got)
	}
	if got := getBfeSslCipher(req); got == "" {
		t.Error("getBfeSslCipher empty")
	}
	if got := getBfeSslVersion(req); got == "" {
		t.Error("getBfeSslVersion empty")
	}
	if got := getBfeSslJa3Raw(req); got != "raw" {
		t.Errorf("getBfeSslJa3Raw = %s, want raw", got)
	}
	if got := getBfeSslJa3Hash(req); got != "hash" {
		t.Errorf("getBfeSslJa3Hash = %s, want hash", got)
	}

	req.Session.TlsState = nil
	if got := getBfeSslResume(req); got != "" {
		t.Errorf("getBfeSslResume(nil) = %s, want empty", got)
	}
}

func TestGetBfeProtocol(t *testing.T) {
	req := makeRequestForVars()
	req.Session.IsSecure = true
	req.Session.Proto = "h2"
	if got := getBfeProtocol(req); got != "h2" {
		t.Errorf("getBfeProtocol = %s, want h2", got)
	}

	req.Session.IsSecure = false
	if got := getBfeProtocol(req); got != req.HttpRequest.Proto {
		t.Errorf("getBfeProtocol = %s, want %s", got, req.HttpRequest.Proto)
	}
}

func TestGetClientCertFunctions(t *testing.T) {
	req := makeRequestForVars()

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(12345),
		Subject: pkix.Name{
			CommonName:         "unittest",
			Organization:       []string{"org1", "org2"},
			OrganizationalUnit: []string{"ou1"},
			Province:           []string{"province1"},
			Country:            []string{"country1"},
			Locality:           []string{"locality1"},
			Names: []pkix.AttributeTypeAndValue{
				{Type: oidTitle, Value: "title1"},
			},
		},
		Extensions: []pkix.Extension{
			{Id: asn1.ObjectIdentifier{2, 5, 4, 12}, Value: []byte{0x01, 0x02}},
		},
	}

	req.Session.TlsState = &bfe_tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}

	if got := getClientCertSerialNumber(req); got != "12345" {
		t.Errorf("getClientCertSerialNumber = %s, want 12345", got)
	}
	if got := getClientCertSubjectCommonName(req); got != "unittest" {
		t.Errorf("getClientCertSubjectCommonName = %s, want unittest", got)
	}
	if got := getClientCertSubjectOrganization(req); got != "org1" {
		t.Errorf("getClientCertSubjectOrganization = %s, want org1", got)
	}
	if got := getClientCertSubjectOrganizationalUnit(req); got != "ou1" {
		t.Errorf("getClientCertSubjectOrganizationalUnit = %s, want ou1", got)
	}
	if got := getClientCertSubjectProvince(req); got != "province1" {
		t.Errorf("getClientCertSubjectProvince = %s, want province1", got)
	}
	if got := getClientCertSubjectCountry(req); got != "country1" {
		t.Errorf("getClientCertSubjectCountry = %s, want country1", got)
	}
	if got := getClientCertSubjectLocality(req); got != "locality1" {
		t.Errorf("getClientCertSubjectLocality = %s, want locality1", got)
	}
	if got := getClientCertSubjectTitle(req); got != "title1" {
		t.Errorf("getClientCertSubjectTitle = %s, want title1", got)
	}

	// nil cert
	req.Session.TlsState.PeerCertificates = nil
	if got := getClientCertSerialNumber(req); got != "" {
		t.Errorf("getClientCertSerialNumber(nil cert) = %s, want empty", got)
	}
}

func TestGetClientCertSubjectTitleNonString(t *testing.T) {
	req := makeRequestForVars()
	cert := &x509.Certificate{
		Subject: pkix.Name{
			Names: []pkix.AttributeTypeAndValue{
				{Type: oidTitle, Value: 123},
			},
		},
	}
	req.Session.TlsState = &bfe_tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}
	if got := getClientCertSubjectTitle(req); got != "" {
		t.Errorf("getClientCertSubjectTitle = %s, want empty", got)
	}
}

func TestGetClientCertSubjectEmpty(t *testing.T) {
	req := makeRequestForVars()
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: ""}}
	req.Session.TlsState = &bfe_tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}
	if got := getClientCertSubjectCommonName(req); got != "" {
		t.Errorf("getClientCertSubjectCommonName = %s, want empty", got)
	}
	if got := getClientCertSubjectOrganization(req); got != "" {
		t.Errorf("getClientCertSubjectOrganization = %s, want empty", got)
	}
	if got := getClientCertSubjectOrganizationalUnit(req); got != "" {
		t.Errorf("getClientCertSubjectOrganizationalUnit = %s, want empty", got)
	}
	if got := getClientCertSubjectProvince(req); got != "" {
		t.Errorf("getClientCertSubjectProvince = %s, want empty", got)
	}
	if got := getClientCertSubjectCountry(req); got != "" {
		t.Errorf("getClientCertSubjectCountry = %s, want empty", got)
	}
	if got := getClientCertSubjectLocality(req); got != "" {
		t.Errorf("getClientCertSubjectLocality = %s, want empty", got)
	}
}

func TestGetBfeSslJa3NilState(t *testing.T) {
	req := makeRequestForVars()
	req.Session.TlsState = nil
	if got := getBfeSslJa3Raw(req); got != "" {
		t.Errorf("getBfeSslJa3Raw(nil) = %s, want empty", got)
	}
	if got := getBfeSslJa3Hash(req); got != "" {
		t.Errorf("getBfeSslJa3Hash(nil) = %s, want empty", got)
	}
}

func TestGetBfeCluster(t *testing.T) {
	req := makeRequestForVars()
	if got := getBfeCluster(req); got != "clusterA" {
		t.Errorf("getBfeCluster = %s, want clusterA", got)
	}
}

func TestGetBfeVip(t *testing.T) {
	req := makeRequestForVars()
	if got := getBfeVip(req); got != "127.0.0.1" {
		t.Errorf("getBfeVip = %s, want 127.0.0.1", got)
	}

	req.Session.Vip = nil
	if got := getBfeVip(req); got != Unknown {
		t.Errorf("getBfeVip(nil) = %s, want %s", got, Unknown)
	}
}

func TestGetAddressFetcher(t *testing.T) {
	base := &testConn{local: &net.TCPAddr{IP: net.ParseIP("1.1.1.1"), Port: 80}}
	tlsConn := &bfe_tls.Conn{}
	if got := getAddressFetcher(tlsConn); got != nil {
		t.Error("getAddressFetcher(bfe_tls.Conn without AddressFetcher) should be nil")
	}

	fetcher := &testAddrFetcher{balancer: &net.TCPAddr{IP: net.ParseIP("2.2.2.2"), Port: 80}}
	if got := getAddressFetcher(fetcher); got == nil {
		t.Error("getAddressFetcher(AddressFetcher) should not be nil")
	}

	_ = base
}

func TestGetBfeBip(t *testing.T) {
	req := makeRequestForVars()
	fetcher := &testAddrFetcher{
		balancer: &net.TCPAddr{IP: net.ParseIP("2.2.2.2"), Port: 8080},
	}
	req.Session.Connection = fetcher
	if got := getBfeBip(req); got != "2.2.2.2" {
		t.Errorf("getBfeBip = %s, want 2.2.2.2", got)
	}

	req.Session.Connection = &testConn{}
	if got := getBfeBip(req); got != Unknown {
		t.Errorf("getBfeBip(no fetcher) = %s, want %s", got, Unknown)
	}

	fetcherNilBalancer := &testAddrFetcher{}
	req.Session.Connection = fetcherNilBalancer
	if got := getBfeBip(req); got != Unknown {
		t.Errorf("getBfeBip(nil balancer) = %s, want %s", got, Unknown)
	}
}

func TestGetBfeRip(t *testing.T) {
	req := makeRequestForVars()
	req.Session.Connection = &testConn{
		local: &net.TCPAddr{IP: net.ParseIP("3.3.3.3"), Port: 443},
	}
	if got := getBfeRip(req); got != "3.3.3.3" {
		t.Errorf("getBfeRip = %s, want 3.3.3.3", got)
	}
}

func TestGetBfeBackendInfo(t *testing.T) {
	req := makeRequestForVars()
	want := "ClusterName:clusterA,SubClusterName:subA,BackendName:backend1(10.0.0.2:8080)"
	if got := getBfeBackendInfo(req); got != want {
		t.Errorf("getBfeBackendInfo = %s, want %s", got, want)
	}
}

func TestGetBfeServerName(t *testing.T) {
	req := makeRequestForVars()
	got := getBfeServerName(req)
	if got == "" || got == Unknown {
		t.Errorf("getBfeServerName = %s, want non-empty hostname", got)
	}
}

func TestGetSessionIdAndLogId(t *testing.T) {
	req := makeRequestForVars()
	if got := getSessionId(req); got != "session-123" {
		t.Errorf("getSessionId = %s, want session-123", got)
	}
	if got := getLogId(req); got != "log-456" {
		t.Errorf("getLogId = %s, want log-456", got)
	}
}

func TestGetClientGeoFunctions(t *testing.T) {
	req := makeRequestForVars()
	req.SetContext(mod_geo.CtxCountryIsoCode, "CN")
	req.SetContext(mod_geo.CtxSubdivisionIsoCode, "BJ")
	req.SetContext(mod_geo.CtxCityName, "Beijing")
	req.SetContext(mod_geo.CtxLatitude, "39.9")
	req.SetContext(mod_geo.CtxLongitude, "116.4")

	if got := getClientGeoCountryIsoCode(req); got != "CN" {
		t.Errorf("getClientGeoCountryIsoCode = %s, want CN", got)
	}
	if got := getClientGeoSubdivisionIsoCode(req); got != "BJ" {
		t.Errorf("getClientGeoSubdivisionIsoCode = %s, want BJ", got)
	}
	if got := getClientGeoCityName(req); got != "Beijing" {
		t.Errorf("getClientGeoCityName = %s, want Beijing", got)
	}
	if got := getClientGeoLatitude(req); got != "39.9" {
		t.Errorf("getClientGeoLatitude = %s, want 39.9", got)
	}
	if got := getClientGeoLongitude(req); got != "116.4" {
		t.Errorf("getClientGeoLongitude = %s, want 116.4", got)
	}

	// missing context
	req2 := makeRequestForVars()
	if got := getClientGeoCountryIsoCode(req2); got != "" {
		t.Errorf("getClientGeoCountryIsoCode(missing) = %s, want empty", got)
	}
}

func TestGetBfeHTTP2Fingerprint(t *testing.T) {
	req := makeRequestForVars()
	if got := getBfeHTTP2Fingerprint(req); got != "h2fp" {
		t.Errorf("getBfeHTTP2Fingerprint = %s, want h2fp", got)
	}
}

func TestVariableHandlersKeys(t *testing.T) {
	// sanity check that common variables are registered
	for _, key := range []string{"bfe_client_ip", "bfe_vip", "bfe_log_id", "bfe_ssl_cipher"} {
		if _, ok := VariableHandlers[key]; !ok {
			t.Errorf("VariableHandlers missing %s", key)
		}
	}
}

func TestGetClientCertExtVal(t *testing.T) {
	req := makeRequestForVars()
	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: oidTitle, Value: []byte{0xAB, 0xCD}},
		},
	}
	req.Session.TlsState = &bfe_tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}
	if got := getClientCertExtVal(req, oidTitle); got != "abcd" {
		t.Errorf("getClientCertExtVal = %s, want abcd", got)
	}

	if got := getClientCertExtVal(req, asn1.ObjectIdentifier{1, 2, 3}); got != "nil" {
		t.Errorf("getClientCertExtVal(missing) = %s, want nil", got)
	}

	req.Session.TlsState.PeerCertificates = nil
	if got := getClientCertExtVal(req, oidTitle); got != "" {
		t.Errorf("getClientCertExtVal(nil cert) = %s, want empty", got)
	}
}

func TestGetAddressFetcherWithTlsConn(t *testing.T) {
	// bfe_tls.Conn.GetNetConn returns nil in test if not set,
	// so getAddressFetcher returns nil
	tlsConn := &bfe_tls.Conn{}
	if got := getAddressFetcher(tlsConn); got != nil {
		t.Error("getAddressFetcher(bfe_tls.Conn) should be nil when underlying conn is nil")
	}
}

// ensure interfaces are satisfied
var _ bfe_util.AddressFetcher = (*testAddrFetcher)(nil)
