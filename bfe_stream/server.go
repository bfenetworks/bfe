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

package bfe_stream

import (
	"net"
)

import (
	"github.com/baidu/go-lib/gotrack"
)

import (
	http "github.com/bfenetworks/bfe/bfe_http"
	tls "github.com/bfenetworks/bfe/bfe_tls"
)

const (
	defaultConnectTimeout  = 1000 // ms
	defaultConnectRetryMax = 3
)

type Server struct {
	// ConnectTimeout optionally specifies the timeout value (ms) to
	// connect backend. If zero, a default value is used.
	ConnectTimeout int

	// ConnectRetryMax optionally specifies the upper limit of connect
	// retris. If zero, a default value is used
	ConnectRetryMax int

	// BalanceHandler optionally specifies the handler for backends balance
	// BalanceHandler should not be nil.
	BalanceHandler BalanceHandler

	// FindProductHandler finds product name for stream proxy
	FindProductHandler FindProductHandler

	// ProxyHandler optionally specifies the handler for process client conn
	// and backend conn. If nil, a default value is used.
	ProxyHandler ProxyHandler

	// ProxyConfig optionally specifies the config for ProxyHandler
	ProxyConfig interface{}
}

func (s *Server) connectTimeout() int {
	if v := s.ConnectTimeout; v > 0 {
		return v
	}
	return defaultConnectTimeout
}

func (s *Server) connectRetryMax() int {
	if v := s.ConnectRetryMax; v > 0 {
		return v
	}
	return defaultConnectRetryMax
}

func (s *Server) balanceHandler() BalanceHandler {
	return s.BalanceHandler
}

func (s *Server) proxyHandler() ProxyHandler {
	if v := s.ProxyHandler; v != nil {
		return v
	}
	return TLSProxyHandler
}

func (s *Server) handleConn(hs *http.Server, c net.Conn, h http.Handler) *serverConn {
	sc := new(serverConn)
	sc.srv = s
	sc.hs = hs
	sc.conn = c
	if tc, ok := c.(*tls.Conn); ok {
		sc.tlsState = new(tls.ConnectionState)
		*sc.tlsState = tc.ConnectionState()
		if serverRule != nil {
			sc.rule = serverRule.GetStreamRule(tc)
		}

	}

	sc.closeNotifyCh = hs.CloseNotifyCh
	sc.copyErrCh = make(chan error, 2)
	sc.serveG = gotrack.NewGoroutineLock()

	return sc
}

// FindProduct finds product by conn vip.
func (s *Server) FindProduct(c net.Conn) string {
	productHandler := s.FindProductHandler
	if productHandler == nil {
		return ""
	}

	return productHandler(c)
}

// NewProtoHandler creates a protocol handler for stream.
func NewProtoHandler(conf *Server) func(*http.Server, *tls.Conn, http.Handler) {
	if conf == nil {
		conf = new(Server)
	}

	protoHandler := func(hs *http.Server, c *tls.Conn, h http.Handler) {
		if sc := conf.handleConn(hs, c, h); sc != nil {
			sc.serve()
		}
	}
	return protoHandler
}

// Rule is the customized config for specific conn in server side.
type Rule struct {
	ProxyProtocol int
}

type ServerRule interface {
	GetStreamRule(conn *tls.Conn) *Rule
}

var serverRule ServerRule

func SetServerRule(r ServerRule) {
	serverRule = r
}
