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
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"testing"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_proxy"
	tls "github.com/bfenetworks/bfe/bfe_tls"
)

func TestTLSProxyForEchoServer(t *testing.T) {
	testTLSProxy(t,
		func(st *ServerTester) {
			// generate random msg
			len := rand.Intn(4096) + 1
			msg := make([]byte, len)
			rand.Read(msg)

			// client send msg
			err := st.Write(msg)
			if err != nil {
				st.t.Fatalf("write error: %s", err)
			}

			// client check msg recv
			st.WantData(msg)
		},
		func(conn net.Conn) {
			io.Copy(conn, conn)
		},
		nil,
	)
}

func TestTLSProxyForHTTPServer(t *testing.T) {
	req := []byte(`GET / HTTP/1.1\r\n\r\n`)
	rsp := []byte(`200 HTTP/1.1 OK\r\n\r\n`)

	testTLSProxy(t,
		func(st *ServerTester) {
			// client send http req
			err := st.Write(req)
			if err != nil {
				st.t.Fatalf("write error: %s", err)
			}

			// client check resp recv
			st.WantData(rsp)
		},
		func(conn net.Conn) {
			// backend send http rsp
			conn.Write(rsp)
		},
		nil,
	)
}

func TestTLSProxyClientCloseConn(t *testing.T) {
	errCh := make(chan error)

	testTLSProxy(t,
		func(st *ServerTester) {
			// client close conn
			st.cc.Close()
			st.WantError("closed")

			// check conn to backend closed
			timer := time.NewTimer(2 * time.Second)
			defer timer.Stop()
			select {
			case err := <-errCh:
				if err != io.EOF {
					st.t.Fatalf("expect error io.EOF")
				}
			case <-timer.C:
				st.t.Fatalf("expect error bfeore timeout")
			}
		},
		func(conn net.Conn) {
			// backend try read
			_, err := conn.Read(make([]byte, 256))
			errCh <- err
		},
		nil,
	)
}

func TestTLSProxyBackendCloseConn(t *testing.T) {
	testTLSProxy(t,
		func(st *ServerTester) {
			// client check conn to proxy closed
			st.WantError("EOF")
		},
		func(conn net.Conn) {
			// backend close conn
			conn.Close()
		},
		nil,
	)
}

func TestTLSProxyBackendUnavailable(t *testing.T) {
	testTLSProxy(t,
		func(st *ServerTester) {
			// check client conn error
			d, err := ioutil.ReadAll(st.cc)
			if len(d) > 0 || err != nil {
				st.t.Fatalf("expect read 0 bytes: got %d:%s", len(d), err)
			}
		},
		func(conn net.Conn) {
			io.Copy(conn, conn)
		},
		&Server{
			BalanceHandler: func(conn interface{}) (*backend.BfeBackend, error) {
				b := backend.NewBfeBackend()
				b.AddrInfo = "8.8.8.8:12345"
				// balancer return unavailable backend
				return b, nil
			},
			ConnectTimeout:  500,
			ConnectRetryMax: 2,
		},
	)
}

func TestTLSProxyBalancerError(t *testing.T) {
	testTLSProxy(t,
		func(st *ServerTester) {
			// check client conn closed
			st.WantError("EOF")
		},
		func(conn net.Conn) {
		},
		&Server{
			BalanceHandler: func(conn interface{}) (*backend.BfeBackend, error) {
				b := backend.NewBfeBackend()

				// balancer return unavailable backend
				return b, fmt.Errorf("balance error")
			},
		},
	)
}

func TestTLSProxyServerConnPanic(t *testing.T) {
	testTLSProxy(t,
		func(st *ServerTester) {
			// check client conn closed
			st.WantError("EOF")
		},
		func(conn net.Conn) {
			io.Copy(conn, conn)
		},
		&Server{
			BalanceHandler: func(conn interface{}) (*backend.BfeBackend, error) {
				panic("panic balancer")
			},
		},
	)
}

func TestTLSProxyServerShutdown(t *testing.T) {
	testTLSProxy(t,
		func(st *ServerTester) {
			// shutdown server
			close(st.ms.Config.CloseNotifyCh)

			// check client conn closed
			st.WantError("EOF")
		},
		func(conn net.Conn) {
			io.Copy(conn, conn)
		},
		nil,
	)
}

type testServerRule struct {
	proxyProtocolVersion int
}

func (t *testServerRule) GetStreamRule(conn *tls.Conn) *Rule {
	return &Rule{ProxyProtocol: t.proxyProtocolVersion}
}

func TestTLSProxyUsingProxyProtocolToBackend(t *testing.T) {
	sr := testServerRule{proxyProtocolVersion: 2}
	SetServerRule(&sr)

	testTLSProxy(t,
		func(st *ServerTester) {
			// generate random msg
			len := rand.Intn(4096) + 1
			msg := make([]byte, len)
			rand.Read(msg)

			// client send msg
			err := st.Write(msg)
			if err != nil {
				st.t.Fatalf("write error: %s", err)
			}

			// client check msg recv
			st.WantData(msg)
		},
		func(conn net.Conn) {
			pc := bfe_proxy.NewConn(conn, 0, 0)
			io.Copy(pc, pc)
			if pc.BalancerAddr() != conn.RemoteAddr() {
				t.Errorf("balancer address[%v] should be equal to %v", pc.BalancerAddr(), conn.RemoteAddr())
			}
		},
		nil,
	)
}
