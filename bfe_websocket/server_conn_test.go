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
	"fmt"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"
)

import (
	"golang.org/x/net/websocket"
)

import (
	"github.com/bfenetworks/bfe/bfe_balance/backend"
)

var echoServer = func(ws *websocket.Conn) {
	defer ws.Close()
	io.Copy(ws, ws)
}

func TestWebSocketProxyForEchoServer(t *testing.T) {
	config, _ := websocket.NewConfig("ws://example.org/echo",
		"http://example.org")
	testWebSocketProxy(t,
		func(st *ServerTester) {
			// websocket handshake
			if err := st.WebSocketHandshake(config); err != nil {
				st.t.Fatalf("websocket handshake fail %s", err)
			}
			// websocket client send msg
			msg := []byte("Hello WebSocket")
			if _, err := st.WebSocketWrite(msg); err != nil {
				st.t.Fatalf("websocket write fail %s", err)
			}
			// websocket client recv msg
			data := make([]byte, 512)
			n, err := st.WebSocketRead(data)
			if err != nil {
				st.t.Fatalf("websocket read fail %s", err)
			}
			// check reply from echoServer
			if !reflect.DeepEqual(msg, data[:n]) {
				st.t.Fatalf("want %s, got %s", msg, data[:n])
			}
		},
		websocketHandlers("/echo", echoServer),
		nil,
	)
}

var countServer = func(ws *websocket.Conn) {
	defer ws.Close()
	for {
		n, err := ws.Read(make([]byte, 512))
		if err != nil {
			return
		}
		msg := []byte(fmt.Sprintf("%d", n))
		if _, err := ws.Write(msg); err != nil {
			return
		}
	}
}

func TestWebSocketProxyForCountServer(t *testing.T) {
	config, _ := websocket.NewConfig("ws://example.org/count",
		"http://example.org")
	testWebSocketProxy(t,
		func(st *ServerTester) {
			// websocket handshake
			if err := st.WebSocketHandshake(config); err != nil {
				st.t.Fatalf("websocket handshake fail %s", err)
			}
			// websocket client send msg
			msg := []byte("this message is 24 bytes")
			if _, err := st.WebSocketWrite(msg); err != nil {
				st.t.Fatalf("websocket write fail %s", err)
			}
			// websocket client recv msg
			data := make([]byte, 512)
			n, err := st.WebSocketRead(data)
			if err != nil {
				st.t.Fatalf("websocket read fail %s", err)
			}
			// check reply from echoServer
			if !reflect.DeepEqual([]byte("24"), data[:n]) {
				st.t.Fatalf("want %s, got %s", "24", data[:n])
			}
		},
		websocketHandlers("/count", countServer),
		nil,
	)
}

var heartbeat = []byte("heartbeat")

var pushServer = func(ws *websocket.Conn) {
	defer ws.Close()
	for {
		if _, err := ws.Write(heartbeat); err != nil {
			return
		}
		time.Sleep(time.Second)
	}
}

func TestWebSocketProxyForPushServer(t *testing.T) {
	config, _ := websocket.NewConfig("ws://example.org/push",
		"http://example.org")
	testWebSocketProxy(t,
		func(st *ServerTester) {
			// websocket handshake
			if err := st.WebSocketHandshake(config); err != nil {
				st.t.Fatalf("websocket handshake fail %s", err)
			}
			// websocket client recv msg
			data := make([]byte, 512)
			n, err := st.WebSocketRead(data)
			if err != nil {
				st.t.Fatalf("websocket read fail %s", err)
			}
			// check reply from echoServer
			if !reflect.DeepEqual(heartbeat, data[:n]) {
				st.t.Fatalf("want %s, got %s", heartbeat, data[:n])
			}
		},
		websocketHandlers("/push", pushServer),
		nil,
	)
}

type SimpleHttpServer struct{}

func (s SimpleHttpServer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	rw.Write([]byte("Welcome to HTTP"))
}

func TestWebSocketProxyForHTTPServer(t *testing.T) {
	config, _ := websocket.NewConfig("ws://example.org/http",
		"http://example.org")
	testWebSocketProxy(t,
		func(st *ServerTester) {
			if err := st.WebSocketHandshake(config); err == nil {
				st.t.Fatalf("websocket handshake expect fail %s", err)
			}
		},
		map[string]http.Handler{"/http": new(SimpleHttpServer)},
		nil,
	)
}

func TestWebSocketProxyBadUpgrade(t *testing.T) {
	reqs := []string{
		`GET /bu HTTP/1.1
Connection: upgrade
Upgrade: ws
Sec-Websocket-Version:13
Sec-Websocket-Key:renJjCsv/OHFTYTV2Qg8iA==

`,

		`GET /bu HTTP/1.1
Upgrade: websocket
Sec-Websocket-Version:13
Sec-Websocket-Key:renJjCsv/OHFTYTV2Qg8iA==

`,

		`GET /bu HTTP/1.1
Connection: keep-alive
Sec-Websocket-Version:13
Sec-Websocket-Key:renJjCsv/OHFTYTV2Qg8iA==

`,

		`GET /bu HTTP/1.1
Sec-Websocket-Version:13
Sec-Websocket-Key:renJjCsv/OHFTYTV2Qg8iA==

`,
		`POST /bu HTTP/1.1
Connection: upgrade
Upgrade: websocket
Content-Length: 0
Sec-Websocket-Version:13
Sec-Websocket-Key:renJjCsv/OHFTYTV2Qg8iA==

`,
	}

	for i, req := range reqs {
		testWebSocketProxy(t,
			func(st *ServerTester) {
				// websocket handshake
				if err := st.Write([]byte(req)); err != nil {
					st.t.Fatalf("case %d: write error %s", i, err)
				}
				st.WantError("EOF")
			},
			nil,
			nil,
		)
	}
}

func TestWebSocketProxyBalanceError(t *testing.T) {
	config, _ := websocket.NewConfig("ws://example.org/echo",
		"http://example.org")
	testWebSocketProxy(t,
		func(st *ServerTester) {
			if err := st.WebSocketHandshake(config); err == nil {
				st.t.Fatalf("websocket handshake should fail %s", err)
			}
		},
		nil,
		&Server{
			BalanceHandler: func(req interface{}) (*backend.BfeBackend, error) {
				return nil, fmt.Errorf("balance error")
			},
		},
	)
}

func TestWebSocketProxyBackendUnavailable(t *testing.T) {
	config, _ := websocket.NewConfig("ws://example.org/echo",
		"http://example.org")
	testWebSocketProxy(t,
		func(st *ServerTester) {
			if err := st.WebSocketHandshake(config); err == nil {
				st.t.Fatalf("websocket handshake should fail %s", err)
			}
		},
		nil,
		&Server{
			BalanceHandler: func(req interface{}) (*backend.BfeBackend, error) {
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

func TestWebSocketProxyClientCloseConn(t *testing.T) {
	config, _ := websocket.NewConfig("ws://example.org/test",
		"http://example.org")

	backendErrCh := make(chan error)
	testWebSocketProxy(t,
		func(st *ServerTester) {
			if err := st.WebSocketHandshake(config); err != nil {
				st.t.Fatalf("websocket handshake fail %s", err)
			}
			// client close conn
			st.cc.Close()

			// check backend
			err := <-backendErrCh
			if err == nil || err.Error() != "EOF" {
				st.t.Fatalf("backend expect conn close")
			}
		},
		websocketHandlers("/test", func(ws *websocket.Conn) {
			defer ws.Close()
			_, err := ws.Read(make([]byte, 256))
			backendErrCh <- err
		}),
		nil,
	)
}

func TestWebSocketProxyBackendCloseConn(t *testing.T) {
	config, _ := websocket.NewConfig("ws://example.org/reject",
		"http://example.org")
	testWebSocketProxy(t,
		func(st *ServerTester) {
			if err := st.WebSocketHandshake(config); err != nil {
				st.t.Fatalf("websocket handshake fail %s", err)
			}
			st.WantError("EOF")
		},
		websocketHandlers("/reject", func(ws *websocket.Conn) {
			ws.Close()
		}),
		nil,
	)
}

func TestWebSocketProxyServerShutdown(t *testing.T) {
	config, _ := websocket.NewConfig("ws://example.org/echo",
		"http://example.org")
	testWebSocketProxy(t,
		func(st *ServerTester) {
			if err := st.WebSocketHandshake(config); err != nil {
				st.t.Fatalf("websocket handshake fail: %s", err)
			}

			// shutdown server
			close(st.mp.Config.CloseNotifyCh)

			// check client conn closed
			st.WantError("EOF")
		},
		nil,
		nil,
	)
}

func TestWebSocketProxyServerConnPanic(t *testing.T) {
	config, _ := websocket.NewConfig("ws://example.org/echo",
		"http://example.org")
	testWebSocketProxy(t,
		func(st *ServerTester) {
			if err := st.WebSocketHandshake(config); err == nil {
				st.t.Fatalf("websocket handshake should fail %s", err)
			}
		},
		nil,
		&Server{
			BalanceHandler: func(req interface{}) (*backend.BfeBackend, error) {
				panic("balance panic")
			},
		},
	)
}
