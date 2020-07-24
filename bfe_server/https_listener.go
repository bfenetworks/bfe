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

// wrapper of tls.listener

package bfe_server

import (
	"net"
	"sync"
)

import (
	"github.com/bfenetworks/bfe/bfe_tls"
)

type HttpsListener struct {
	tlsListener net.Listener // listener for https
	tcpListener net.Listener // underlying tcp listener

	config *bfe_tls.Config // tls config for listener
	lock   sync.Mutex
}

func NewHttpsListener(listener net.Listener, config *bfe_tls.Config) *HttpsListener {
	httpsListener := &HttpsListener{
		tcpListener: listener,
		config:      config,
		tlsListener: bfe_tls.NewListener(listener, config),
	}
	return httpsListener
}

// UpdateSessionTicketKey updates session ticket key.
func (l *HttpsListener) UpdateSessionTicketKey(key []byte) {
	l.lock.Lock()
	defer l.lock.Unlock()

	// clone and modify config
	config := l.config.Clone()
	copy(config.SessionTicketKeyName[:], key[:16])
	copy(config.SessionTicketKey[:], key[16:])

	// update config for listener
	l.config = config
	bfe_tls.UpdateListener(l.tlsListener, config)
}
