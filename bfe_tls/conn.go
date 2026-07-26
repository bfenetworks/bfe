// Copyright (c) 2019 The BFE Authors.
// Licensed under the Apache License, Version 2.0

package bfe_tls

import (
	"net"
	"time"

	crypto_tls "crypto/tls"
)

// Conn wraps *crypto/tls.Conn with BFE-specific state (VIP, SNI, JA3, ClientAuth).
type Conn struct {
	*crypto_tls.Conn // nil for synthetic pre-handshake conns

	rawConn    net.Conn // underlying conn when Conn is nil
	vip        net.IP
	serverName string
	clientAuth bool
	clientCA   string
	ja3Raw     string
	ja3Hash    string
}

// newConn wraps an established crypto/tls.Conn.
func newConn(c *crypto_tls.Conn) *Conn { return &Conn{Conn: c} }

// newSyntheticConn creates a pre-handshake Conn for ServerRule lookups.
func newSyntheticConn(raw net.Conn, sni string) *Conn {
	c := &Conn{rawConn: raw, serverName: sni}
	if addr, ok := raw.LocalAddr().(*net.TCPAddr); ok {
		c.vip = addr.IP
	}
	return c
}

// GetVip returns the VIP (local IP) of the connection.
func (c *Conn) GetVip() net.IP {
	if c.vip != nil {
		return c.vip
	}
	if c.Conn != nil {
		if addr, ok := c.Conn.LocalAddr().(*net.TCPAddr); ok {
			return addr.IP
		}
	}
	return nil
}

// GetServerName returns the SNI from the TLS ClientHello.
func (c *Conn) GetServerName() string {
	if c.serverName != "" {
		return c.serverName
	}
	if c.Conn != nil {
		return c.Conn.ConnectionState().ServerName
	}
	return ""
}

// ConnectionState returns BFE's extended TLS connection state.
func (c *Conn) ConnectionState() ConnectionState {
	if c.Conn == nil {
		return ConnectionState{}
	}
	cs := connectionStateFromCrypto(c.Conn.ConnectionState())
	cs.ClientAuth = c.clientAuth
	cs.ClientCAName = c.clientCA
	cs.JA3Raw = c.ja3Raw
	cs.JA3Hash = c.ja3Hash
	return cs
}

// LocalAddr returns the local network address.
func (c *Conn) LocalAddr() net.Addr {
	if c.Conn != nil {
		return c.Conn.LocalAddr()
	}
	return c.rawConn.LocalAddr()
}

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() net.Addr {
	if c.Conn != nil {
		return c.Conn.RemoteAddr()
	}
	return c.rawConn.RemoteAddr()
}

// SetDeadline sets the read and write deadlines.
func (c *Conn) SetDeadline(t time.Time) error {
	if c.Conn != nil {
		return c.Conn.SetDeadline(t)
	}
	return c.rawConn.SetDeadline(t)
}

// SetReadDeadline sets the read deadline.
func (c *Conn) SetReadDeadline(t time.Time) error {
	if c.Conn != nil {
		return c.Conn.SetReadDeadline(t)
	}
	return c.rawConn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	if c.Conn != nil {
		return c.Conn.SetWriteDeadline(t)
	}
	return c.rawConn.SetWriteDeadline(t)
}

// VerifyHostname checks that the peer certificate chain is valid for host.
func (c *Conn) VerifyHostname(host string) error {
	if c.Conn != nil {
		return c.Conn.VerifyHostname(host)
	}
	return nil
}

// GetNetConn returns the underlying net.Conn.
func (c *Conn) GetNetConn() net.Conn {
	if c.Conn != nil {
		return c.Conn.NetConn()
	}
	return c.rawConn
}

// Handshake performs or re-checks the TLS handshake.
func (c *Conn) Handshake() error {
	if c.Conn != nil {
		return c.Conn.Handshake()
	}
	return nil
}

// ConnParam is implemented by objects that can provide VIP information.
type ConnParam interface {
	GetVip() net.IP
}

// SetConnParam attaches a ConnParam to the connection (used to bind session state).
func (c *Conn) SetConnParam(param ConnParam) {
	if param != nil {
		if ip := param.GetVip(); ip != nil {
			c.vip = ip
		}
	}
}
