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

// handler for application layer protocol over TLS connection

package bfe_server

import (
	"sync"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_tls_conf/tls_rule_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_http2"
	"github.com/bfenetworks/bfe/bfe_spdy"
)

type ProtocolHandler struct {
	server    *BfeServer // the server on which the connection arrived
	conn      *conn      // connection for handler
	proto     string     // name of application layer protocol
	closeOnce sync.Once  // for connection close
}

func NewProtocolHandler(conn *conn, proto string) *ProtocolHandler {
	p := new(ProtocolHandler)
	p.server = conn.server
	p.conn = conn
	p.proto = proto
	return p
}

// ServeHTTP processes http request and send http response.
//
// Params:
//	 - w : a response writer
//	 - r : a http request
func (p *ProtocolHandler) ServeHTTP(rw bfe_http.ResponseWriter, request *bfe_http.Request) {
	log.Logger.Debug("ProtocolHandler(%s): start process request", p.proto)
	sf := p.server.GetServerConf()

	reqStat := bfe_basic.NewRequestStat(request.State.StartTime)
	reqStat.ReadReqEnd = time.Now()
	reqInfo := bfe_basic.NewRequest(request, p.conn.rwc, reqStat, p.conn.session, sf)

	// process request
	isKeepAlive := p.conn.serveRequest(rw, reqInfo)

	// close connection if needed
	if !isKeepAlive {
		closeFunc := func() {
			switch p.proto {
			case tls_rule_conf.SPDY31:
				bfe_spdy.CloseConn(request.Body)
			case tls_rule_conf.HTTP2:
				bfe_http2.CloseConn(request.Body)
			/* never go here */
			default:
				return
			}
		}
		p.closeOnce.Do(closeFunc)
	}
}

// CheckSupportMultiplex checks whether protocol support request multiplexing on a conn.
func CheckSupportMultiplex(proto string) bool {
	switch proto {
	case tls_rule_conf.SPDY31:
		return true
	case tls_rule_conf.HTTP2:
		return true
	default:
		return false
	}
}
