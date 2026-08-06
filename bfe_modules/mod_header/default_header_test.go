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
	"net"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

type defaultHeaderConn struct {
	net.Conn
	local net.Addr
}

func (c *defaultHeaderConn) LocalAddr() net.Addr {
	return c.local
}

func (c *defaultHeaderConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 12345}
}

func makeDefaultHeaderRequest() *bfe_basic.Request {
	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Session.SetTrustSource(true)
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org/path", nil)
	req.HttpRequest.Host = "www.example.org"
	req.HttpRequest.RemoteAddr = "10.0.0.1:12345"
	req.Connection = &defaultHeaderConn{
		local: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080},
	}
	req.Session.Connection = req.Connection
	req.ClientAddr = &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 12345}
	return req
}

func TestModHeaderForwardedAddr(t *testing.T) {
	req := makeDefaultHeaderRequest()
	modHeaderForwardedAddr(req)

	if got := req.HttpRequest.Header.Get(bfe_basic.HeaderForwardedHost); got != "www.example.org" {
		t.Errorf("HeaderForwardedHost = %s, want www.example.org", got)
	}
	if got := req.HttpRequest.Header.Get(bfe_basic.HeaderForwardedFor); got != "10.0.0.1" {
		t.Errorf("HeaderForwardedFor = %s, want 10.0.0.1", got)
	}
	if got := req.HttpRequest.Header.Get(bfe_basic.HeaderForwardedPort); got != "12345" {
		t.Errorf("HeaderForwardedPort = %s, want 12345", got)
	}
}

func TestModHeaderForwardedAddrWithPrior(t *testing.T) {
	req := makeDefaultHeaderRequest()
	req.HttpRequest.Header.Set(bfe_basic.HeaderForwardedHost, "prior.example.org")
	req.HttpRequest.Header.Set(bfe_basic.HeaderForwardedFor, "192.168.0.1")
	req.HttpRequest.Header.Set(bfe_basic.HeaderForwardedPort, "8080")

	modHeaderForwardedAddr(req)

	if got := req.HttpRequest.Header.Get(bfe_basic.HeaderForwardedHost); got != "prior.example.org, www.example.org" {
		t.Errorf("HeaderForwardedHost = %s, want prior.example.org, www.example.org", got)
	}
	if got := req.HttpRequest.Header.Get(bfe_basic.HeaderForwardedFor); got != "192.168.0.1, 10.0.0.1" {
		t.Errorf("HeaderForwardedFor = %s, want 192.168.0.1, 10.0.0.1", got)
	}
	if got := req.HttpRequest.Header.Get(bfe_basic.HeaderForwardedPort); got != "8080, 12345" {
		t.Errorf("HeaderForwardedPort = %s, want 8080, 12345", got)
	}
}

func TestSetHeaderRealAddr(t *testing.T) {
	req := makeDefaultHeaderRequest()
	setHeaderRealAddr(req, "10.0.0.1", "12345")

	if got := req.HttpRequest.Header.Get(bfe_basic.HeaderRealIP); got != "10.0.0.1" {
		t.Errorf("HeaderRealIP = %s, want 10.0.0.1", got)
	}
	if got := req.HttpRequest.Header.Get(bfe_basic.HeaderRealPort); got != "12345" {
		t.Errorf("HeaderRealPort = %s, want 12345", got)
	}
}

func TestSetHeaderBfeIP(t *testing.T) {
	req := makeDefaultHeaderRequest()
	setHeaderBfeIP(req)

	if got := req.HttpRequest.Header.Get(bfe_basic.HeaderBfeIP); got != "127.0.0.1" {
		t.Errorf("HeaderBfeIP = %s, want 127.0.0.1", got)
	}
}

func TestSetDefaultHeader(t *testing.T) {
	m := NewModuleHeader()
	req := makeDefaultHeaderRequest()
	m.setDefaultHeader(req)

	if got := req.HttpRequest.Header.Get(bfe_basic.HeaderForwardedHost); got != "www.example.org" {
		t.Errorf("HeaderForwardedHost = %s, want www.example.org", got)
	}
	if got := req.HttpRequest.Header.Get(bfe_basic.HeaderRealIP); got != "10.0.0.1" {
		t.Errorf("HeaderRealIP = %s, want 10.0.0.1", got)
	}
	if got := req.HttpRequest.Header.Get(bfe_basic.HeaderBfeIP); got != "127.0.0.1" {
		t.Errorf("HeaderBfeIP = %s, want 127.0.0.1", got)
	}
}

func TestSetDefaultHeaderNilClientAddr(t *testing.T) {
	m := NewModuleHeader()
	req := makeDefaultHeaderRequest()
	req.ClientAddr = nil
	m.setDefaultHeader(req)

	if got := req.HttpRequest.Header.Get(bfe_basic.HeaderRealIP); got != "" {
		t.Errorf("HeaderRealIP(nil client) = %s, want empty", got)
	}
}
