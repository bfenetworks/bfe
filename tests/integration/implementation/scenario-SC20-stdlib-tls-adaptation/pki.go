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

package sc20

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"
)

// testCA is a programmatically generated self-signed CA used by this
// scenario. All certificate material is created on the fly because Go >= 1.15
// rejects legacy CN-only certificates for name verification, so historical
// PEM constants cannot be reused.
type testCA struct {
	cert    *x509.Certificate
	key     *rsa.PrivateKey
	certPEM []byte
}

// newTestCA creates a self-signed CA with the given common name.
func newTestCA(t *testing.T, cn string) *testCA {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate ca key failed: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create ca cert failed: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse ca cert failed: %v", err)
	}
	return &testCA{
		cert:    cert,
		key:     key,
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
	}
}

// writeCertTo writes the CA certificate (PEM) to the given file.
func (ca *testCA) writeCertTo(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, ca.certPEM, 0644); err != nil {
		t.Fatalf("write ca cert to %s failed: %v", path, err)
	}
}

// newServerCert issues a SAN server certificate signed by this CA and returns
// cert/key in PEM form.
func (ca *testCA) newServerCert(t *testing.T, cn string, dnsNames []string) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate server key failed: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dnsNames,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatalf("create server cert failed: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}

// newClientCert issues a client certificate with the given serial number
// signed by this CA. The fixed serial allows CRL revocation assertions.
func (ca *testCA) newClientCert(t *testing.T, serial int64, cn string) tls.Certificate {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate client key failed: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatalf("create client cert failed: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("build client key pair failed: %v", err)
	}
	return cert
}

// newCRL signs a CRL revoking the given serial numbers and returns DER bytes.
// BFE's ClientCRLLoad parses .crl files with x509.ParseCRL, which expects DER.
func (ca *testCA) newCRL(t *testing.T, revokedSerials ...int64) []byte {
	t.Helper()

	now := time.Now()
	entries := make([]x509.RevocationListEntry, 0, len(revokedSerials))
	for _, serial := range revokedSerials {
		entries = append(entries, x509.RevocationListEntry{
			SerialNumber:   big.NewInt(serial),
			RevocationTime: now,
		})
	}
	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number:                    big.NewInt(1),
		ThisUpdate:                now.Add(-time.Hour),
		NextUpdate:                now.Add(24 * time.Hour),
		RevokedCertificateEntries: entries,
	}, ca.cert, ca.key)
	if err != nil {
		t.Fatalf("create crl failed: %v", err)
	}
	return der
}
