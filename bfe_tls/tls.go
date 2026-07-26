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

// Package bfe_tls wraps Go's standard crypto/tls to add BFE-specific
// features: per-VIP/SNI rule selection, grade-based cipher policy,
// CRL checking, and OCSP stapling.
package bfe_tls

import (
	"crypto/tls"
	"net"
	"sync"

	crypto_tls "crypto/tls"
)

// CurveID and curve constants re-exported from crypto/tls.
type CurveID = crypto_tls.CurveID

const (
	CurveP256 = crypto_tls.CurveP256
	CurveP384 = crypto_tls.CurveP384
	CurveP521 = crypto_tls.CurveP521
	X25519    = crypto_tls.X25519
)



// X509KeyPair parses a PEM-encoded certificate and private key.
func X509KeyPair(certPEMBlock, keyPEMBlock []byte) (Certificate, error) {
	tlsCert, err := crypto_tls.X509KeyPair(certPEMBlock, keyPEMBlock)
	if err != nil {
		return Certificate{}, err
	}
	return Certificate{
		Certificate: tlsCert.Certificate,
		PrivateKey:  tlsCert.PrivateKey,
		Leaf:        tlsCert.Leaf,
	}, nil
}

// LoadX509KeyPair reads and parses a certificate/key pair from files.
func LoadX509KeyPair(certFile, keyFile string) (Certificate, error) {
	tlsCert, err := crypto_tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return Certificate{}, err
	}
	return Certificate{
		Certificate: tlsCert.Certificate,
		PrivateKey:  tlsCert.PrivateKey,
		Leaf:        tlsCert.Leaf,
	}, nil
}

// NewLRUClientSessionCache creates a new LRU client session cache.
var NewLRUClientSessionCache = crypto_tls.NewLRUClientSessionCache

// Server returns a TLS server-side Conn using conn as the underlying transport.
func Server(conn net.Conn, config *Config) *Conn {
	tlsCfg := config.toCryptoTLS(nil)
	return newConn(crypto_tls.Server(conn, tlsCfg))
}

// Client returns a TLS client-side Conn using conn as the underlying transport.
func Client(conn net.Conn, config *Config) *Conn {
	tlsCfg := config.toCryptoTLS(nil)
	return newConn(crypto_tls.Client(conn, tlsCfg))
}

// listener wraps a net.Listener and accepts TLS connections using a
// live-reloadable BFE Config.
type listener struct {
	net.Listener
	mu     sync.RWMutex
	config *Config
}

// Accept waits for and returns the next incoming TLS connection.
func (l *listener) Accept() (net.Conn, error) {
	raw, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	l.mu.RLock()
	cfg := l.config
	l.mu.RUnlock()

	tlsCfg := cfg.toCryptoTLS(cfg.ServerRule)
	tlsConn := crypto_tls.Server(raw, tlsCfg)
	return newConn(tlsConn), nil
}

// NewListener creates a Listener that wraps each accepted connection in TLS.
func NewListener(inner net.Listener, config *Config) net.Listener {
	l := &listener{Listener: inner, config: config}
	return l
}

// UpdateListener hot-reloads the TLS config on an existing listener.
func UpdateListener(l net.Listener, config *Config) {
	if tl, ok := l.(*listener); ok {
		tl.mu.Lock()
		tl.config = config
		tl.mu.Unlock()
	}
}

// toCryptoTLS converts a BFE Config to a standard crypto/tls.Config,
// wiring per-connection rule selection via GetConfigForClient.
func (c *Config) toCryptoTLS(rule ServerRule) *crypto_tls.Config {
	tlsCerts := make([]tls.Certificate, 0, len(c.Certificates))
	for _, cert := range c.Certificates {
		tlsCerts = append(tlsCerts, cert.toCryptoTLS())
	}

	cfg := &crypto_tls.Config{
		Rand:                     c.Rand,
		Time:                     c.Time,
		Certificates:             tlsCerts,
		NextProtos:               c.NextProtos,
		ServerName:               c.ServerName,
		InsecureSkipVerify:       c.InsecureSkipVerify,
		CipherSuites:             c.CipherSuites,
		PreferServerCipherSuites: c.PreferServerCipherSuites,
		SessionTicketsDisabled:   c.SessionTicketsDisabled,
		SessionTicketKey:         c.SessionTicketKey,
		ClientSessionCache:       c.ClientSessionCache,
		MinVersion:               VersionTLS12, // floor: no SSL3/TLS1.0/TLS1.1
		MaxVersion:               0,            // 0 = crypto/tls default = TLS 1.3
		CurvePreferences:         c.CurvePreferences,
		VerifyPeerCertificate:    c.VerifyPeerCertificate,
	}

	if c.RootCAs != nil {
		cfg.RootCAs = c.RootCAs
	}
	if c.ClientCAs != nil {
		cfg.ClientCAs = c.ClientCAs
		cfg.ClientAuth = c.ClientAuth
	}

	if rule == nil {
		return cfg
	}

	// Wire per-connection config selection.
	cfg.GetConfigForClient = func(chi *crypto_tls.ClientHelloInfo) (*crypto_tls.Config, error) {
		synth := newSyntheticConn(chi.Conn, chi.ServerName)
		r := rule.Get(synth)
		if r == nil {
			return nil, nil // use base config
		}
		perConn := cfg.Clone()
		applyRule(perConn, r)
		return perConn, nil
	}

	// Multi-cert selection: pick the right leaf cert by SNI/VIP.
	if c.MultiCert != nil {
		cfg.GetCertificate = func(chi *crypto_tls.ClientHelloInfo) (*crypto_tls.Certificate, error) {
			synth := newSyntheticConn(chi.Conn, chi.ServerName)
			cert := c.MultiCert.Get(synth)
			if cert == nil {
				return nil, nil // fall back to Certificates list
			}
			tlsCert := cert.toCryptoTLS()
			return &tlsCert, nil
		}
	}

	return cfg
}

// applyRule applies a BFE TLS Rule to a per-connection crypto/tls.Config.
func applyRule(cfg *crypto_tls.Config, r *Rule) {
	cfg.MinVersion = gradeMinVersion(r.Grade)
	cfg.MaxVersion = 0 // always allow TLS 1.3
	cfg.CipherSuites = gradeCipherSuites(r.Grade, r.Chacha20)

	if r.ClientAuth {
		cfg.ClientAuth = crypto_tls.RequireAndVerifyClientCert
		if r.ClientCAs != nil {
			cfg.ClientCAs = r.ClientCAs
		}
	}
}

// Dial connects to addr using TLS on the named network.
func Dial(network, addr string, config *Config) (*Conn, error) {
	tlsCfg := config.toCryptoTLS(config.ServerRule)
	c, err := crypto_tls.Dial(network, addr, tlsCfg)
	if err != nil {
		return nil, err
	}
	return newConn(c), nil
}
