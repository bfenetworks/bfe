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

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// HTTP server.  See RFC 2616.

package bfe_server

import (
	"net"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
)

func delayCalc(delay time.Duration) time.Duration {
	if delay == 0 {
		delay = 5 * time.Millisecond
	} else {
		delay *= 2
	}
	if max := 1 * time.Second; delay > max {
		delay = max
	}
	return delay
}

func isTimeout(err error) bool {
	e, ok := err.(net.Error)
	return ok && e.Timeout()
}

// ServeHttp accept incoming http connections
func (srv *BfeServer) ServeHttp(ln net.Listener) error {
	return srv.Serve(ln, ln, "HTTP")
}

// ServeHttps accept incoming https connections
func (srv *BfeServer) ServeHttps(ln *HttpsListener) error {
	return srv.Serve(ln.tlsListener, ln.tcpListener, "HTTPS")
}

// Serve accepts incoming connections on the Listener l, creating a
// new service goroutine for each.  The service goroutines read requests and
// then call srv.Handler to reply to them.
//
// Params
//     - l  : net listener
//     - raw: underlying tcp listener (different from `l` in HTTPS)
//
// Return
//     - err: error
func (srv *BfeServer) Serve(l net.Listener, raw net.Listener, proto string) error {
	var tempDelay time.Duration // how long to sleep on accept failure
	proxyState := srv.serverStatus.ProxyState

	for {
		// accept new connection
		rw, e := l.Accept()
		if e != nil {
			if isTimeout(e) {
				proxyState.ErrClientTimeout.Inc(1)
				continue
			}
			proxyState.ErrClientConnAccept.Inc(1)

			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				tempDelay = delayCalc(tempDelay)

				log.Logger.Error("http: Accept error: %v; retrying in %v", e, tempDelay)
				time.Sleep(tempDelay)
				continue
			}

			// if in GraceShutdown state, exit accept loop after timeout
			if srv.CheckGracefulShutdown() {
				shutdownTimeout := srv.Config.Server.GracefulShutdownTimeout
				time.Sleep(time.Duration(shutdownTimeout) * time.Second)
			}

			return e
		}

		// start go-routine for new connection
		go func(rwc net.Conn, srv *BfeServer) {
			// create data structure for new connection
			c, err := newConn(rwc, srv)
			if err != nil {
				// current, here is unreachable
				return
			}

			// process new connection
			c.serve()
		}(rw, srv)
	}
}
