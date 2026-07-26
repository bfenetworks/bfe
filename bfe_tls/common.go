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

package bfe_tls

import (
	"crypto"
	"crypto/rand"
	crypto_tls "crypto/tls"
	"crypto/x509"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/ocsp"
)

// TLS version constants.
const (
	VersionSSL30 = 0x0300
	VersionTLS10 = 0x0301
	VersionTLS11 = 0x0302
	VersionTLS12 = 0x0303
	VersionTLS13 = 0x0304
)

// Security grade constants (per SSLLabs definitions).
const (
	GradeAPlus = "A+"
	GradeA     = "A"
	GradeB     = "B"
	GradeC     = "C"
)

// ClientAuthType re-exported from crypto/tls.
type ClientAuthType = crypto_tls.ClientAuthType

const (
	NoClientCert               = crypto_tls.NoClientCert
	RequestClientCert          = crypto_tls.RequestClientCert
	RequireAnyClientCert       = crypto_tls.RequireAnyClientCert
	VerifyClientCertIfGiven    = crypto_tls.VerifyClientCertIfGiven
	RequireAndVerifyClientCert = crypto_tls.RequireAndVerifyClientCert
)

// ClientSessionCache re-exported from crypto/tls.
type ClientSessionCache = crypto_tls.ClientSessionCache

// ServerSessionCache is a server-side TLS session resumption cache backed
// by an external store (e.g. Redis). Implemented by bfe_server.ServerSessionCache.
type ServerSessionCache interface {
	Get(sessionKey string) (sessionState []byte, ok bool)
	Put(sessionKey string, sessionState []byte) error
}

// VersionTextForOpenSSL returns a human-readable TLS version string.
func VersionTextForOpenSSL(ver uint16) string {
	switch ver {
	case VersionTLS13:
		return "TLSv1.3"
	case VersionTLS12:
		return "TLSv1.2"
	case VersionTLS11:
		return "TLSv1.1"
	case VersionTLS10:
		return "TLSv1"
	case VersionSSL30:
		return "SSLv3"
	default:
		return "unknown"
	}
}

// MultiCertificate selects a Certificate based on connection properties.
type MultiCertificate interface {
	Get(c *Conn) *Certificate
}

// NextProtoConf selects ALPN protocols per connection.
type NextProtoConf interface {
	Get(c *Conn) []string
	Mandatory(c *Conn) (string, bool)
}

// ServerRule selects per-VIP/SNI TLS rules for incoming connections.
type ServerRule interface {
	Get(c *Conn) *Rule
}

// KeyPairLoader can load certificate key pairs from files.
type KeyPairLoader interface {
	LoadX509KeyPair(certFile, keyFile string) (cert Certificate, err error)
}

// Rule holds per-VIP/SNI TLS policy overrides applied during handshake.
type Rule struct {
	NextProtos    NextProtoConf
	Grade         string
	ClientAuth    bool
	ClientCAs     *x509.CertPool
	ClientCAName  string
	ClientCRLPool *CRLPool
	Chacha20      bool
	DynamicRecord bool
}

// Config holds BFE TLS server configuration.
type Config struct {
	Rand                     io.Reader
	Time                     func() time.Time
	Certificates             []Certificate
	NameToCertificate        map[string]*Certificate
	VerifyPeerCertificate    func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error
	MultiCert                MultiCertificate
	RootCAs                  *x509.CertPool
	NextProtos               []string
	ServerName               string
	ClientAuth               ClientAuthType
	ClientCAs                *x509.CertPool
	InsecureSkipVerify       bool
	CipherSuites             []uint16
	CipherSuitesPriority     []uint16
	PreferServerCipherSuites bool
	Ssl3PoodleProofed        bool
	SessionTicketsDisabled   bool
	SessionTicketKey         [32]byte
	SessionTicketKeyName     [16]byte
	ClientSessionCache       ClientSessionCache
	ServerSessionCache       ServerSessionCache
	SessionCacheDisabled     bool
	MinVersion               uint16
	MaxVersion               uint16
	CurvePreferences         []CurveID
	EnableSslv2ClientHello   bool
	ServerRule               ServerRule

	serverInitOnce sync.Once
}

// Clone returns a shallow copy of Config.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	c2 := *c
	return &c2
}

// BuildNameToCertificate populates NameToCertificate from Certificates.
func (c *Config) BuildNameToCertificate() {
	m := make(map[string]*Certificate)
	for i := range c.Certificates {
		cert := &c.Certificates[i]
		if cert.Leaf == nil {
			if len(cert.Certificate) == 0 {
				continue
			}
			var err error
			cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				continue
			}
		}
		names := append([]string{cert.Leaf.Subject.CommonName}, cert.Leaf.DNSNames...)
		for _, name := range names {
			m[name] = cert
		}
	}
	c.NameToCertificate = m
}

func (c *Config) randReader() io.Reader {
	if c.Rand != nil {
		return c.Rand
	}
	return rand.Reader
}

// Certificate holds a TLS certificate chain and private key.
type Certificate struct {
	Certificate [][]byte
	PrivateKey  crypto.PrivateKey
	OCSPStaple  []byte
	OCSPParse   *ocsp.Response
	Leaf        *x509.Certificate
}

// toCryptoTLS converts to the standard crypto/tls.Certificate type.
func (cert *Certificate) toCryptoTLS() crypto_tls.Certificate {
	return crypto_tls.Certificate{
		Certificate: cert.Certificate,
		PrivateKey:  cert.PrivateKey,
		OCSPStaple:  cert.OCSPStaple,
		Leaf:        cert.Leaf,
	}
}

// ConnectionState contains details about an established TLS connection.
// It extends the standard crypto/tls.ConnectionState with BFE-specific fields.
type ConnectionState struct {
	Version                    uint16
	HandshakeComplete          bool
	DidResume                  bool
	CipherSuite                uint16
	OcspStaple                 bool
	NegotiatedProtocolIsMutual bool
	NegotiatedProtocol         string
	ServerName                 string
	HandshakeTime              time.Duration
	PeerCertificates           []*x509.Certificate
	VerifiedChains             [][]*x509.Certificate
	// ClientRandom and ServerRandom are not exposed by crypto/tls; reserved for
	// future access via tls.Conn.ConnectionState() key material exports.
	ClientRandom  []byte
	ServerRandom  []byte
	MasterSecret  []byte
	ClientCiphers []uint16
	ClientAuth    bool
	ClientCAName  string
	JA3Raw        string
	JA3Hash       string
}

// connectionStateFromCrypto maps a crypto/tls.ConnectionState to bfe_tls.ConnectionState.
func connectionStateFromCrypto(cs crypto_tls.ConnectionState) ConnectionState {
	return ConnectionState{
		Version:                    cs.Version,
		HandshakeComplete:          cs.HandshakeComplete,
		DidResume:                  cs.DidResume,
		CipherSuite:                cs.CipherSuite,
		OcspStaple:                 len(cs.OCSPResponse) > 0,
		NegotiatedProtocolIsMutual: cs.NegotiatedProtocolIsMutual,
		NegotiatedProtocol:         cs.NegotiatedProtocol,
		ServerName:                 cs.ServerName,
		PeerCertificates:           cs.PeerCertificates,
		VerifiedChains:             cs.VerifiedChains,
	}
}

// OcspTimeRangeCheck verifies the OCSP response is within its valid time window
// (allowing one hour of tolerance on each end).
func OcspTimeRangeCheck(parse *ocsp.Response) bool {
	const delta = time.Hour
	now := time.Now()
	return now.Sub(parse.ThisUpdate) >= delta && parse.NextUpdate.Sub(now) >= delta
}
