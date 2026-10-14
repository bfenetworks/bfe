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

// https listener based on standard crypto/tls

package bfe_server

import (
	"crypto/tls"
	"net"
	"sync"
)

type HttpsListener struct {
	tcpListener net.Listener // underlying tcp listener
	config      *tls.Config  // current tls config
	lock        sync.RWMutex
}

func NewHttpsListener(listener net.Listener, config *tls.Config) *HttpsListener {
	return &HttpsListener{
		tcpListener: listener,
		config:      config,
	}
}

func (l *HttpsListener) Accept() (net.Conn, error) {
	c, err := l.tcpListener.Accept()
	if err != nil {
		return nil, err
	}

	// get current tls config
	l.lock.RLock()
	config := l.config
	l.lock.RUnlock()

	return tls.Server(c, config), nil
}

func (l *HttpsListener) Close() error {
	return l.tcpListener.Close()
}

func (l *HttpsListener) Addr() net.Addr {
	return l.tcpListener.Addr()
}

// GetConfigClone returns a clone of the listener's current tls config.
// It reflects in-place updates such as UpdateSessionTicketKey.
func (l *HttpsListener) GetConfigClone() *tls.Config {
	l.lock.RLock()
	defer l.lock.RUnlock()
	return l.config.Clone()
}

// UpdateSessionTicketKey updates session ticket key.
func (l *HttpsListener) UpdateSessionTicketKey(key []byte) {
	l.lock.Lock()
	defer l.lock.Unlock()

	// clone and modify config
	config := l.config.Clone()
	copy(config.SessionTicketKey[:], key)

	// update config for listener
	l.config = config
}
