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
	"errors"
	"net"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_balance/backend"
	http "github.com/bfenetworks/bfe/bfe_http"
	tls "github.com/bfenetworks/bfe/bfe_tls"
)

import (
	"github.com/baidu/go-lib/web-monitor/metrics"
)

const (
	WebSocket = "websocket"
)

var (
	errBalanceHandler = errors.New("bfe_websocket: balanceHandler uninitial")
	errRetryTooMany   = errors.New("bfe_websocket: proxy retry too many")
)

// CheckUpgradeWebSocket checks whether client request for WebSocket protocol.
func CheckUpgradeWebSocket(req *http.Request) bool {
	if req.Method != "GET" {
		return false
	}
	if strings.ToLower(req.Header.Get("Upgrade")) != WebSocket {
		return false
	}
	if !strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade") {
		return false
	}
	return true
}

// CheckAcceptWebSocket checks whether server accept WebSocket protocol.
func CheckAcceptWebSocket(rsp *http.Response) bool {
	if rsp.StatusCode != http.StatusSwitchingProtocols {
		return false
	}
	if strings.ToLower(rsp.Header.Get("Upgrade")) != WebSocket {
		return false
	}
	if strings.ToLower(rsp.Header.Get("Connection")) != "upgrade" {
		return false
	}
	return true
}

// Scheme returns scheme of current websocket conn.
func Scheme(c net.Conn) string {
	if _, ok := c.(*tls.Conn); ok {
		return "wss" // websocket over https
	}
	return "ws" // websocket over http
}

// BalanceHandler selects backend for current conn.
type BalanceHandler func(req interface{}) (*backend.BfeBackend, error)

// WebSocketState is internal state for WebSocket.
type WebSocketState struct {
	WebSocketErrBalance       *metrics.Counter
	WebSocketErrConnect       *metrics.Counter
	WebSocketErrProxy         *metrics.Counter
	WebSocketErrHandshake     *metrics.Counter
	WebSocketErrBackendReject *metrics.Counter
	WebSocketErrTransfer      *metrics.Counter
	WebSocketPanicConn        *metrics.Counter
	WebSocketBytesRecv        *metrics.Counter
	WebSocketBytesSent        *metrics.Counter
}

var state WebSocketState

// GetWebSocketState returns internal state for WebSocket.
func GetWebSocketState() *WebSocketState {
	return &state
}
