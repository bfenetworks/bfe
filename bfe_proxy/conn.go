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

package bfe_proxy

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	bufio "github.com/bfenetworks/bfe/bfe_bufio"
)

const (
	// defaultProxyHeaderTimeout is the read timeout of PROXY header.
	// This can be overridden by setting ProxyHeaderTimeout.
	defaultProxyHeaderTimeout = 30 * time.Second

	// defaultProxyHeaderBytes is the maximum permitted size of the PROXY headers
	//
	// Maximum length of header in PROXY v1
	//   See: Proxy Protocol 2.1. Human-readable header format (Version 1)
	//   "So a 108-byte buffer is always enough to store all the line and a trailing zero."
	//
	// Maximum length of header in PROXY V2
	//   See: Proxy Protocol 2.2. Binary header format (version 2)
	//   "The sender must ensure that all the protocol header is sent at once. This block
	//   is always smaller than an MSS, so there is no reason for it to be segmented at
	//   the beginning of the connection."
	defaultMaxProxyHeaderBytes int64 = 2048

	// noLimit is an effective infinite upper bound for io.LimitedReader
	noLimit int64 = (1 << 63) - 1
)

// Conn is used to wrap an underlying connection which
// may be speaking the Proxy Protocol. If it is, the RemoteAddr() will
// return the address of the client instead of the proxy address.
type Conn struct {
	conn          net.Conn // underlying TCP connection
	lmtReader     *io.LimitedReader
	bufReader     *bufio.Reader
	headerTimeout time.Duration // timeout for reading proxy header
	headerLimit   int64         // maximum bytes of proxy header accepted
	headerErr     error         // error when parsing proxy header
	dstAddr       *net.TCPAddr  // real dst address (i.e. virtual address)
	srcAddr       *net.TCPAddr  // real src address (i.e. real client address)
	once          sync.Once
}

// NewConn is used to wrap a net.Conn that may be speaking
// the proxy protocol
func NewConn(conn net.Conn, headerTimeout time.Duration, maxProxyHeaderBytes int64) *Conn {
	if headerTimeout <= 0 {
		headerTimeout = defaultProxyHeaderTimeout
	}
	if maxProxyHeaderBytes <= 0 {
		maxProxyHeaderBytes = defaultMaxProxyHeaderBytes
	}

	pConn := new(Conn)
	pConn.headerTimeout = headerTimeout
	pConn.headerLimit = maxProxyHeaderBytes
	pConn.conn = conn
	pConn.lmtReader = io.LimitReader(conn, pConn.headerLimit).(*io.LimitedReader)
	pConn.bufReader = bufio.NewReader(pConn.lmtReader)
	return pConn
}

// Read reads data from the connection.
// It check for the proxy protocol header when doing
// the initial read. If there is an error parsing the header,
// it is returned and the socket is closed.
func (p *Conn) Read(b []byte) (int, error) {
	p.checkProxyHeaderOnce()
	if p.headerErr != nil {
		return 0, p.headerErr
	}
	return p.bufReader.Read(b)
}

// Write writes data to the connection.
func (p *Conn) Write(b []byte) (int, error) {
	return p.conn.Write(b)
}

// Close closes the connection.
func (p *Conn) Close() error {
	return p.conn.Close()
}

// LocalAddr returns the local network address.
func (p *Conn) LocalAddr() net.Addr {
	return p.conn.LocalAddr()
}

// RemoteAddr returns the address of the client if the proxy
// protocol is being used, otherwise just returns the address of
// the socket peer. If there is an error parsing the header, the
// address of the client is not returned, and the socket is closed.
func (p *Conn) RemoteAddr() net.Addr {
	p.checkProxyHeaderOnce()
	if p.srcAddr != nil {
		return p.srcAddr
	}
	return p.conn.RemoteAddr()
}

// VirtualAddr returns the virtual address
func (p *Conn) VirtualAddr() net.Addr {
	p.checkProxyHeaderOnce()
	if p.dstAddr != nil {
		return p.dstAddr
	}
	return nil
}

// BalancerAddr returns the address of balancer
func (p *Conn) BalancerAddr() net.Addr {
	p.checkProxyHeaderOnce()
	if p.dstAddr != nil {
		return p.conn.RemoteAddr()
	}
	return nil
}

// GetNetConn returns the underlying connection
func (p *Conn) GetNetConn() net.Conn {
	return p.conn
}

// SetDeadline implements the Conn.SetDeadline method
func (p *Conn) SetDeadline(t time.Time) error {
	return p.conn.SetDeadline(t)
}

// SetReadDeadline implements the Conn.SetReadDeadline method
func (p *Conn) SetReadDeadline(t time.Time) error {
	return p.conn.SetReadDeadline(t)
}

// SetWriteDeadline implements the Conn.SetWriteDeadline method
func (p *Conn) SetWriteDeadline(t time.Time) error {
	return p.conn.SetWriteDeadline(t)
}

func (p *Conn) checkProxyHeaderOnce() {
	p.once.Do(func() {
		if err := p.checkProxyHeader(); err != nil {
			log.Logger.Error("bfe_proxy: Failed to read proxy header: %v", err)
		}
	})
}

func (p *Conn) checkProxyHeader() error {
	// set read timeout for proxy header
	p.conn.SetReadDeadline(time.Now().Add(p.headerTimeout))

	// reset timeout and read limit for conn
	defer func() {
		p.conn.SetReadDeadline(time.Time{})
		p.lmtReader.N = noLimit
	}()

	// read and parse proxy header
	hdr, err := Read(p.bufReader)
	if err == ErrNoProxyProtocol { // ignore ErrNoProxyProtocol
		return nil
	}
	if err != nil {
		p.Close()
		p.headerErr = err
		return err
	}

	// initial real src/dst address
	srcAddr := net.JoinHostPort(hdr.SourceAddress.String(), fmt.Sprintf("%d", hdr.SourcePort))
	p.srcAddr, err = net.ResolveTCPAddr(hdr.TransportProtocol.String(), srcAddr)
	if err != nil { /* never go here */
		p.Close()
		return err
	}

	dstAddr := net.JoinHostPort(hdr.DestinationAddress.String(), fmt.Sprintf("%d", hdr.DestinationPort))
	p.dstAddr, err = net.ResolveTCPAddr(hdr.TransportProtocol.String(), dstAddr)
	if err != nil { /* never go here */
		p.Close()
		return err
	}

	return nil
}
