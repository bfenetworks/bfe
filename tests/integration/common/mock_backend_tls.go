// Copyright (c) 2026 The BFE Authors.
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

package common

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TLSOptions configures a TLS mock backend.
type TLSOptions struct {
	// CertPEM/KeyPEM is the server certificate presented to BFE.
	CertPEM []byte
	KeyPEM  []byte
	// ClientCAs, when non-nil, makes the backend require and verify a client
	// certificate against the pool (mutual TLS).
	ClientCAs *x509.CertPool
	// SNIRecorder, if non-nil, receives the SNI server name of each handshake.
	SNIRecorder func(serverName string)
	// ClientCertRecorder, if non-nil, receives the client certificate common
	// name of each request (empty when no client cert was presented).
	ClientCertRecorder func(commonName string)
}

// NewMockBackendTLS starts an HTTPS mock backend with a caller-provided
// certificate, reusing the recording handler of MockBackend.
func NewMockBackendTLS(t *testing.T, clusterName string, response int, body string, opts TLSOptions) *MockBackend {
	t.Helper()

	cert, err := tls.X509KeyPair(opts.CertPEM, opts.KeyPEM)
	if err != nil {
		t.Fatalf("load TLS mock backend key pair failed: %v", err)
	}

	b := &MockBackend{
		ClusterName: clusterName,
		Response:    response,
		Body:        body,
	}

	handler := b.handler()
	if opts.ClientCertRecorder != nil {
		recorder := opts.ClientCertRecorder
		base := handler
		handler = func(w http.ResponseWriter, r *http.Request) {
			cn := ""
			if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
				cn = r.TLS.PeerCertificates[0].Subject.CommonName
			}
			recorder(cn)
			base(w, r)
		}
	}

	srv := httptest.NewUnstartedServer(handler)
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	if opts.SNIRecorder != nil {
		recorder := opts.SNIRecorder
		cfg.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			recorder(hello.ServerName)
			return nil, nil
		}
	}
	if opts.ClientCAs != nil {
		cfg.ClientCAs = opts.ClientCAs
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	srv.TLS = cfg
	srv.StartTLS()

	b.server = srv
	return b
}
