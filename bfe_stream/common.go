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
	"errors"
	"net"
)

import (
	"github.com/baidu/go-lib/web-monitor/metrics"
)

import (
	"github.com/bfenetworks/bfe/bfe_balance/backend"
)

var (
	errBalanceHandler = errors.New("bfe_stream: balanceHandler uninitial")
	errRetryTooMany   = errors.New("bfe_stream: proxy retry too many")
)

// FindProductHandler gets product by conn vip.
type FindProductHandler func(c net.Conn) string

// BalanceHandler selects backend for current conn.
type BalanceHandler func(c interface{}) (*backend.BfeBackend, error)

// ProxyHandler forwards data between client and backend.
type ProxyHandler func(s *Server, c net.Conn, b net.Conn, errCh chan error)

// StreamState is internal state for stream.
type StreamState struct {
	StreamErrBalance  *metrics.Counter
	StreamErrConnect  *metrics.Counter
	StreamErrProxy    *metrics.Counter
	StreamErrTransfer *metrics.Counter
	StreamPanicConn   *metrics.Counter
	StreamBytesRecv   *metrics.Counter
	StreamBytesSent   *metrics.Counter
}

var state StreamState

// GetStreamState returns internal state for stream.
func GetStreamState() *StreamState {
	return &state
}
