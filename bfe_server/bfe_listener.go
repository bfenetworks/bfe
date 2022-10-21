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

// BfeListener is a wrapper of TCP listener which accept connections behind
// a load balancer (PROXY/NONE)
//
//  Note: The TLS listener is wired together like:
//  1. TCP listener
//  2. BFE listener (PROXY)
//  3. TLS listener

package bfe_server

import (
	"net"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
	"github.com/bfenetworks/bfe/bfe_proxy"
)

import (
	"github.com/baidu/go-lib/log"
)

// BfeListener is used to wrap an underlying TCP listener, which accept connections
// behind a layer4 load balancer (PROXY)
type BfeListener struct {
	// Listener is the underlying tcp listener
	Listener net.Listener

	// BalancerType is the type of Layer4 load balancer
	BalancerType string

	// ProxyHeaderTimeout Optionally specifies the timeout value to
	// receive the Proxy Protocol Header. Zero means no timeout.
	ProxyHeaderTimeout time.Duration

	// ProxyHeaderLimit Optionally specifies the maximum bytes to
	// receive the Proxy Protocol Header. Zero means default value.
	ProxyHeaderLimit int64
}

// NewBfeListener return bfe listener according to config
func NewBfeListener(listener net.Listener, config bfe_conf.BfeConfig) *BfeListener {
	l := new(BfeListener)
	l.Listener = listener
	l.BalancerType = config.Server.Layer4LoadBalancer
	l.ProxyHeaderTimeout = time.Duration(config.Server.ClientReadTimeout) * time.Second
	l.ProxyHeaderLimit = int64(config.Server.MaxProxyHeaderBytes)

	return l
}

// Accept implements the Accept method in the Listener interface;
// it waits for the next call and returns a generic net.Conn.
func (l *BfeListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		log.Logger.Debug("BfeListener: accept error %s", err)
		return nil, err
	}

	switch l.BalancerType {
	case bfe_conf.BalancerProxy:
		conn = bfe_proxy.NewConn(conn, l.ProxyHeaderTimeout, l.ProxyHeaderLimit)
		log.Logger.Debug("BfeListener: accept connection via PROXY")
	}

	return conn, nil
}

// Close closes the underlying listener.
func (l *BfeListener) Close() error {
	return l.Listener.Close()
}

// Addr returns the underlying listener's network address.
func (l *BfeListener) Addr() net.Addr {
	return l.Listener.Addr()
}
