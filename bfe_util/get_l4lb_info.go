// Copyright (c) 2019 Baidu, Inc.
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
	"errors"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"
)

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/baidu/bfe/bfe_tls"
)

const (
	TCP_OPT_CIP_ANY = 230 // get cip from tcp option
	TCP_OPT_VIP_ANY = 229 // get vip from tcp option
)

var (
	ErrAddressFormat = errors.New("address format error")
)

// GetVipPort return vip and port for given conn
func GetVipPort(conn net.Conn) (net.IP, int, error) {
	// get underlying bfe conn, the given net.Conn may be wired like:
	//  - TLS Connection (optional)
	//  - BFE Connection (BGW/PROXY, optional)
	//  - TCP Connection
	if tc, ok := conn.(*bfe_tls.Conn); ok {
		conn = tc.GetNetConn()
	}

	// get virtual vip
	if af, ok := conn.(AddressFetcher); ok {
		vaddr := af.VirtualAddr()
		if vaddr == nil {
			return nil, 0, fmt.Errorf("vip unknown")
		}
		return ParseIpAndPort(vaddr.String())
	}

	return nil, 0, fmt.Errorf("cann`t get vip and port when Layer4LoadBalancer is not set")
}

// GetVip return vip for given conn
func GetVip(conn net.Conn) net.IP {
	vip, _, err := GetVipPort(conn)
	if err != nil {
		return nil
	}
	return vip
}

// getVipPortViaBGW gets vip/port from tcp conn via BGW
func getVipPortViaBGW(conn net.Conn) (net.IP, int, error) {
	// get conn fd
	f, err := GetConnFile(conn)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	fd := int(f.Fd())

	// get vip/port
	rawAddr, err := GetsockoptMutiByte(fd, syscall.IPPROTO_TCP, TCP_OPT_VIP_ANY)
	if err != nil {
		log.Logger.Debug("GetsockoptMutiByte() fail: TCP_OPT_VIP_ANY: %v", err)
		return nil, 0, err
	}
	log.Logger.Debug("getVipPortViaBGW(): VIP raw : %v", rawAddr)

	// parse vip/port
	return parseSockAddr(rawAddr)
}

// getCipPortViaBGW gets cip/port from tcp conn via BGW
func getCipPortViaBGW(conn net.Conn) (net.IP, int, error) {
	// get conn fd
	f, err := GetConnFile(conn)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	fd := int(f.Fd())

	// get cip/port
	rawAddr, err := GetsockoptMutiByte(fd, syscall.IPPROTO_TCP, TCP_OPT_CIP_ANY)
	if err != nil {
		log.Logger.Debug("GetsockoptMutiByte fail: TCP_OPT_CIP_ANY: %v", err)
		return nil, 0, err
	}
	log.Logger.Debug("getCipPortViaBGW(): CIP raw : %v", rawAddr)

	// parse cip/port
	return parseSockAddr(rawAddr)
}

// parseSockAddr parses addr from data with format sockaddr_in/sockaddr_in6
//
// Note: Address format of sockaddr_in:
//   struct sockaddr_in {
//       sa_family_t     sin_family;    /* address family: AF_INET */
//       in_port_t       sin_port;      /* port in network byte order */
//       struct in_addr  sin_addr;      /* internet address */
//   };
//   struct in_addr {
//       uint32_t        s_addr;        /* address in network byte order */
//   };
//
// Note: Address format of sockaddr_in6:
//   struct sockaddr_in6 {
//       sa_family_t     sin6_family;   /* AF_INET6 */
//       in_port_t       sin6_port;     /* port number */
//       uint32_t        sin6_flowinfo; /* IPv6 flow information */
//       struct in6_addr sin6_addr;     /* IPv6 address */
//       uint32_t        sin6_scope_id; /* Scope ID (new in 2.4) */
//   };
//   struct in6_addr {
//       unsigned char   s6_addr[16];   /* IPv6 address */
//   };
//
func parseSockAddr(rawAddr []byte) (net.IP, int, error) {
	family := NativeUint16(rawAddr[0:2])

	// parse ip
	var ip net.IP
	switch family {
	case syscall.AF_INET:
		ip = net.IPv4(rawAddr[4], rawAddr[5], rawAddr[6], rawAddr[7]).To4()
	case syscall.AF_INET6:
		ip = net.IP(rawAddr[8:24]).To16()
	default:
		return nil, 0, ErrAddressFormat
	}
	if ip == nil {
		return nil, 0, ErrAddressFormat
	}

	// parse port
	port := binary.BigEndian.Uint16(rawAddr[2:4])

	return ip, int(port), nil
}

var _ AddressFetcher = new(BgwConn)

// BgwConn is used to wrap an underlying tcp connection which
// may be speaking the bgw Protocol. If it is, the RemoteAddr() will
// return the address of the client.
type BgwConn struct {
	conn *net.TCPConn

	// srcAddr is address of real client
	// Note: srcAddr is different from conn.RemoteAddr() under BGW64
	srcAddr *net.TCPAddr

	// dstAddr is address of virtual server
	dstAddr *net.TCPAddr
	once    sync.Once
}

// NewBgwConn is used to wrap a net.TCPConn via BGW
func NewBgwConn(conn *net.TCPConn) *BgwConn {
	bConn := &BgwConn{
		conn: conn,
	}
	return bConn
}

// Read reads data from the connection.
func (c *BgwConn) Read(b []byte) (int, error) {
	return c.conn.Read(b)
}

// Write writes data to the connection.
func (c *BgwConn) Write(b []byte) (int, error) {
	return c.conn.Write(b)
}

// Close closes the connection.
func (c *BgwConn) Close() error {
	return c.conn.Close()
}

func (c *BgwConn) CloseWrite() error {
	return c.conn.CloseWrite()
}

// LocalAddr returns the local network address.
func (c *BgwConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr returns the address of the client if the bgw
// protocol is being used, otherwise just returns the address of
// the socket peer.
func (c *BgwConn) RemoteAddr() net.Addr {
	c.checkTtmInfoOnce()
	if c.srcAddr != nil {
		return c.srcAddr
	}
	return c.conn.RemoteAddr()
}

// VirtualAddr returns the visited address by client
func (c *BgwConn) VirtualAddr() net.Addr {
	c.checkTtmInfoOnce()
	if c.dstAddr != nil {
		return c.dstAddr
	}
	return nil
}

// BalancerAddr returns the address of balancer
func (c *BgwConn) BalancerAddr() net.Addr {
	// Note: Not implement, just ignore
	return nil
}

// GetNetConn returns the underlying connection
func (c *BgwConn) GetNetConn() net.Conn {
	return c.conn
}

// SetDeadline implements the Conn.SetDeadline method
func (c *BgwConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

// SetReadDeadline implements the Conn.SetReadDeadline method
func (c *BgwConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline implements the Conn.SetWriteDeadline method
func (c *BgwConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func (c *BgwConn) checkTtmInfoOnce() {
	c.once.Do(func() {
		c.checkTtmInfo()
	})
}

func (c *BgwConn) checkTtmInfo() {
	c.initSrcAddr()
	c.initDstAddr()
}

func (c *BgwConn) initSrcAddr() {
	cip, cport, err := getCipPortViaBGW(c)
	if err != nil {
		log.Logger.Debug("BgwConn getCipPortViaBGW failed, err:%s", err.Error())
		return
	}

	c.srcAddr = &net.TCPAddr{
		IP:   cip,
		Port: cport,
	}
}

func (c *BgwConn) initDstAddr() {
	vip, vport, err := getVipPortViaBGW(c)
	if err != nil {
		log.Logger.Debug("BgwConn getVipPortViaBGW failed, err:%s", err.Error())
		return
	}

	c.dstAddr = &net.TCPAddr{
		IP:   vip,
		Port: vport,
	}
}
