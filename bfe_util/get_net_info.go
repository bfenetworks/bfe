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

package bfe_util

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"reflect"
)

import (
	"github.com/bfenetworks/bfe/bfe_tls"
)

// CloseWriter is the interface that wraps the basic CloseWrite method.
type CloseWriter interface {
	CloseWrite() error
}

// AddressFetcher is the interface that group the address related method.
type AddressFetcher interface {
	// RemoteAddr returns the remote network address.
	RemoteAddr() net.Addr

	// LocalAddr returns the local network address.
	LocalAddr() net.Addr

	// VirtualAddr returns the virtual network address.
	VirtualAddr() net.Addr

	// BalancerAddr return the balancer network address. May be nil.
	BalancerAddr() net.Addr
}

// ConnFetcher is the interface that wrap the GetNetConn
type ConnFetcher interface {
	// GetNetConn returns the underlying net.Conn
	GetNetConn() net.Conn
}

// GetTCPConn returns underlying TCPConn of given conn.
func GetTCPConn(conn net.Conn) (*net.TCPConn, error) {
	switch value := conn.(type) {
	case *bfe_tls.Conn:
		c := value.GetNetConn()
		return c.(*net.TCPConn), nil
	case *net.TCPConn:
		return value, nil
	default:
		return nil, fmt.Errorf("GetTCPConn(): conn type not support %s", reflect.TypeOf(conn))
	}
}

// GetConnFile get a copy of underlying os.File of tcp conn
func GetConnFile(conn net.Conn) (*os.File, error) {
	// get underlying net.Conn
	if c, ok := conn.(ConnFetcher); ok {
		conn = c.GetNetConn()
		return GetConnFile(conn)
	}

	// the fd is tcpConn.fd.sysfd
	if c, ok := conn.(*net.TCPConn); ok {
		return c.File()
	}

	return nil, fmt.Errorf("GetConnFd(): conn type not support %s", reflect.TypeOf(conn))
}

// ParseIpAndPort return parsed ip address
func ParseIpAndPort(addr string) (net.IP, int, error) {
	taddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, 0, err
	}
	return taddr.IP, taddr.Port, nil
}

func NativeUint16(data []byte) uint16 {
	if IsBigEndian() {
		return binary.BigEndian.Uint16(data)
	} else {
		return binary.LittleEndian.Uint16(data)
	}
}

// IsBigEndian check machine is big endian or not
func IsBigEndian() bool {
	var i int32 = 0x12345678
	return byte(i) == 0x12
}
