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

package bfe_util

import (
	"fmt"
	"net"
	"sync"
	"time"
)

import (
	http "github.com/bfenetworks/bfe/bfe_http"
	tls "github.com/bfenetworks/bfe/bfe_tls"
)

// A MockServer is an tls/tcp server listening on a system-chosen port on the
// local loopback interface, for use in end-to-end tests.
type MockServer struct {
	Listener net.Listener

	// optional fields for tls server
	TLS    *tls.Config
	Config *http.Server

	// optional fields for tcp server
	Handler MockHandler
}

// A MockHandler is an handler to process established conn on server side
type MockHandler func(conn net.Conn)

// NewUnstartedServer returns a new Server but doesn't start it.
// After changing its configuration, the caller should call StartTLS.
// The caller should call Close when finished, to shut it down.
func NewUnstartedServer(handler interface{}) *MockServer {
	ms := new(MockServer)
	ms.Listener = newLocalListener()
	ms.Config = &http.Server{
		CloseNotifyCh:           make(chan bool),
		GracefulShutdownTimeout: 3 * time.Second,
		ReadTimeout:             60 * time.Second,
	}

	switch h := handler.(type) {
	case http.Handler:
		ms.Config.Handler = h

	case MockHandler:
		ms.Handler = h
	}

	return ms
}

// StartTLS starts TLS on a server from NewUnstartedServer.
func (s *MockServer) StartTLS() {
	cert, err := tls.X509KeyPair(localhostCert, localhostKey)
	if err != nil {
		panic(fmt.Sprintf("NewTLSServer: %v", err))
	}

	// init tls config and tls listener
	if s.TLS == nil {
		s.TLS = new(tls.Config)
	}
	if len(s.TLS.Certificates) == 0 {
		s.TLS.Certificates = []tls.Certificate{cert}
	}
	tlsListener := tls.NewListener(s.Listener, s.TLS)
	s.Listener = &historyListener{Listener: tlsListener}

	// start tls server
	go s.Serve(s.Listener)
}

// StartTCP starts TCP on a server from NewUnstartedServer.
func (s *MockServer) StartTCP() {
	s.Listener = &historyListener{Listener: s.Listener}
	go s.Serve(s.Listener)
}

func (s *MockServer) Serve(l net.Listener) error {
	defer l.Close()
	for {
		rw, e := l.Accept()
		if e != nil {
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			return e
		}
		c := MockConn{server: s, rwc: rw}
		go c.serve()
	}
}

// Close shuts down the server and blocks until all outstanding
// requests on this server have completed.
func (s *MockServer) Close() {
	s.Listener.Close()
	s.CloseClientConnections()
}

// CloseClientConnections closes any currently open HTTP connections
// to the test Server.
func (s *MockServer) CloseClientConnections() {
	hl, ok := s.Listener.(*historyListener)
	if !ok {
		return
	}
	hl.Lock()
	for _, conn := range hl.history {
		conn.Close()
	}
	hl.Unlock()
}

// historyListener keeps track of all connections that it's ever
// accepted.
type historyListener struct {
	net.Listener
	sync.Mutex // protects history
	history    []net.Conn
}

func (hs *historyListener) Accept() (c net.Conn, err error) {
	c, err = hs.Listener.Accept()
	if err == nil {
		hs.Lock()
		hs.history = append(hs.history, c)
		hs.Unlock()
	}
	return
}

func newLocalListener() net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("failed to listen on a port: %v", err))
	}
	return l
}

type MockConn struct {
	server *MockServer
	rwc    net.Conn
}

// Serve a new connection.
func (c *MockConn) serve() {
	if tlsConn, ok := c.rwc.(*tls.Conn); ok {
		if err := tlsConn.Handshake(); err != nil {
			return
		}
		tlsState := tlsConn.ConnectionState()
		proto := tlsState.NegotiatedProtocol
		if fn := c.server.Config.TLSNextProto[proto]; fn != nil {
			h := c.server.Config.Handler
			fn(c.server.Config, tlsConn, h)
		}
	} else {
		fn := c.server.Handler
		fn(c.rwc)
	}
}

// LocalhostCert is a PEM-encoded TLS cert with SAN IPs
// "127.0.0.1" and "[::1]", expiring at Jan 29 16:00:00 2084 GMT.
// generated from src/crypto/tls:
// go run generate_cert.go  --rsa-bits 1024 --host 127.0.0.1,::1,example.com --ca --start-date "Jan 1 00:00:00 1970" --duration=1000000h
var localhostCert = []byte(`-----BEGIN CERTIFICATE-----
MIICEzCCAXygAwIBAgIQMIMChMLGrR+QvmQvpwAU6zANBgkqhkiG9w0BAQsFADAS
MRAwDgYDVQQKEwdBY21lIENvMCAXDTcwMDEwMTAwMDAwMFoYDzIwODQwMTI5MTYw
MDAwWjASMRAwDgYDVQQKEwdBY21lIENvMIGfMA0GCSqGSIb3DQEBAQUAA4GNADCB
iQKBgQDuLnQAI3mDgey3VBzWnB2L39JUU4txjeVE6myuDqkM/uGlfjb9SjY1bIw4
iA5sBBZzHi3z0h1YV8QPuxEbi4nW91IJm2gsvvZhIrCHS3l6afab4pZBl2+XsDul
rKBxKKtD1rGxlG4LjncdabFn9gvLZad2bSysqz/qTAUStTvqJQIDAQABo2gwZjAO
BgNVHQ8BAf8EBAMCAqQwEwYDVR0lBAwwCgYIKwYBBQUHAwEwDwYDVR0TAQH/BAUw
AwEB/zAuBgNVHREEJzAlggtleGFtcGxlLmNvbYcEfwAAAYcQAAAAAAAAAAAAAAAA
AAAAATANBgkqhkiG9w0BAQsFAAOBgQCEcetwO59EWk7WiJsG4x8SY+UIAA+flUI9
tyC4lNhbcF2Idq9greZwbYCqTTTr2XiRNSMLCOjKyI7ukPoPjo16ocHj+P3vZGfs
h1fIw3cSS2OolhloGw/XM6RWPWtPAlGykKLciQrBru5NAPvCMsb/I1DAceTiotQM
fblo6RBxUQ==
-----END CERTIFICATE-----`)

// LocalhostKey is the private key for localhostCert.
var localhostKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIICXgIBAAKBgQDuLnQAI3mDgey3VBzWnB2L39JUU4txjeVE6myuDqkM/uGlfjb9
SjY1bIw4iA5sBBZzHi3z0h1YV8QPuxEbi4nW91IJm2gsvvZhIrCHS3l6afab4pZB
l2+XsDulrKBxKKtD1rGxlG4LjncdabFn9gvLZad2bSysqz/qTAUStTvqJQIDAQAB
AoGAGRzwwir7XvBOAy5tM/uV6e+Zf6anZzus1s1Y1ClbjbE6HXbnWWF/wbZGOpet
3Zm4vD6MXc7jpTLryzTQIvVdfQbRc6+MUVeLKwZatTXtdZrhu+Jk7hx0nTPy8Jcb
uJqFk541aEw+mMogY/xEcfbWd6IOkp+4xqjlFLBEDytgbIECQQDvH/E6nk+hgN4H
qzzVtxxr397vWrjrIgPbJpQvBsafG7b0dA4AFjwVbFLmQcj2PprIMmPcQrooz8vp
jy4SHEg1AkEA/v13/5M47K9vCxmb8QeD/asydfsgS5TeuNi8DoUBEmiSJwma7FXY
fFUtxuvL7XvjwjN5B30pNEbc6Iuyt7y4MQJBAIt21su4b3sjXNueLKH85Q+phy2U
fQtuUE9txblTu14q3N7gHRZB4ZMhFYyDy8CKrN2cPg/Fvyt0Xlp/DoCzjA0CQQDU
y2ptGsuSmgUtWj3NM9xuwYPm+Z/F84K6+ARYiZ6PYj013sovGKUFfYAqVXVlxtIX
qyUBnu3X9ps8ZfjLZO7BAkEAlT4R5Yl6cGhaJQYZHOde3JEMhNRcVFMO8dJDaFeo
f9Oeos0UUothgiDktdQHxdNEwLjQf7lJJBzV+5OtwswCWA==
-----END RSA PRIVATE KEY-----`)
