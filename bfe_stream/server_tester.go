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
	"io"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_balance/backend"
	http "github.com/bfenetworks/bfe/bfe_http"
	tls "github.com/bfenetworks/bfe/bfe_tls"
	util "github.com/bfenetworks/bfe/bfe_util"
)

type ServerTester struct {
	t testing.TB

	// for client side
	cc net.Conn

	// for server side
	ms *util.MockServer

	// for backend side
	mb *util.MockServer
}

func NewServerTester(t testing.TB, h util.MockHandler, c *Server) *ServerTester {
	// create ServerTester
	st := &ServerTester{t: t}

	// init backend
	st.mb = util.NewUnstartedServer(h)
	st.mb.StartTCP()

	// init balancer
	if c == nil {
		c = new(Server)
	}
	if c.BalanceHandler == nil {
		c.BalanceHandler = func(conn interface{}) (*backend.BfeBackend, error) {
			laddr := st.mb.Listener.Addr()

			b := backend.NewBfeBackend()
			b.AddrInfo = laddr.String()

			return b, nil
		}
	}

	// init server
	st.ms = util.NewUnstartedServer(nil)
	st.ms.TLS = new(tls.Config)
	st.ms.TLS.NextProtos = append(st.ms.TLS.NextProtos, "stream")
	st.ms.Config.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	st.ms.Config.TLSNextProto["stream"] = NewProtoHandler(c)
	st.ms.StartTLS()

	// init client
	tlsConfig := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"stream"}}
	cc, err := tls.Dial("tcp", st.ms.Listener.Addr().String(), tlsConfig)
	if err != nil {
		t.Fatal(err)
	}
	st.cc = cc

	return st
}

// client read message until timeout
func (st *ServerTester) Read(buf []byte) error {
	st.cc.SetReadDeadline(time.Now().Add(4 * time.Second))
	_, err := io.ReadFull(st.cc, buf)
	return err
}

// client write message
func (st *ServerTester) Write(data []byte) error {
	_, err := st.cc.Write(data)
	return err
}

// WantData makes client read and check message
func (st *ServerTester) WantData(data []byte) {
	buf := make([]byte, len(data))
	if err := st.Read(buf); err != nil {
		st.t.Fatalf("read error: %s", err)
	}

	if !reflect.DeepEqual(buf, data) {
		st.t.Fatalf("read error: got %v, want %v", buf, data)
	}
}

// WantError makes client read and check error
func (st *ServerTester) WantError(e string) {
	err := st.Read(make([]byte, 256))
	if err == nil {
		st.t.Fatalf("Expecting error")
	}
	if !strings.Contains(err.Error(), e) {
		st.t.Fatalf("Expecting error got %v ; want %v", err.Error(), e)
	}
}

func (st *ServerTester) Close() {
	st.cc.Close()
	st.ms.Close()
	st.mb.Close()
}

func testTLSProxy(t *testing.T, f func(st *ServerTester), h util.MockHandler, c *Server) {
	// create server tester
	st := NewServerTester(t, h, c)
	defer st.Close()

	// perform test actions
	f(st)
}
