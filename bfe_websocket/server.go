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

package bfe_websocket

import (
	"github.com/baidu/go-lib/gotrack"
)

import (
	http "github.com/bfenetworks/bfe/bfe_http"
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

func (s *Server) handleConn(hs *http.Server, rw http.ResponseWriter, req *http.Request) *serverConn {
	sc := new(serverConn)
	sc.srv = s
	sc.hs = hs
	sc.rw = rw
	sc.req = req

	sc.closeNotifyCh = hs.CloseNotifyCh
	sc.errCh = make(chan error, 2)
	sc.serveG = gotrack.NewGoroutineLock()

	return sc
}

// NewProtoHandler returns protocol handler for websocket.
func NewProtoHandler(conf *Server) func(*http.Server, http.ResponseWriter, *http.Request) {
	if conf == nil {
		conf = new(Server)
	}

	protoHandler := func(hs *http.Server, w http.ResponseWriter, r *http.Request) {
		if sc := conf.handleConn(hs, w, r); sc != nil {
			sc.serve()
		}
	}
	return protoHandler
}
