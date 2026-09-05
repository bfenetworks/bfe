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

package bfe_server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
)

// generateCertPEM generates a self-signed certificate and returns its PEM
// encoding together with the PEM encoded private key.
func generateCertPEM(t *testing.T, cn string, dnsNames []string, isCA bool) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key fail: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		DNSNames:              dnsNames,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}
	if isCA {
		tmpl.KeyUsage = x509.KeyUsageCertSign
	} else {
		tmpl.KeyUsage = x509.KeyUsageDigitalSignature
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate fail: %v", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key fail: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return certPEM, keyPEM
}

// writeTLSConfTree builds a tls_conf directory tree. When withClientCA is
// true, the tree contains client_ca/testca.crt so that a tls rule with
// ClientAuth can be loaded from it.
func writeTLSConfTree(t *testing.T, confRoot, dir string, withClientCA bool) {
	t.Helper()

	serverCertPEM, serverKeyPEM := generateCertPEM(t, "server", []string{"example.com"}, false)
	clientCAPEM, _ := generateCertPEM(t, "testca", nil, true)

	writeFile := func(rel string, content []byte) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(confRoot, rel)), 0755); err != nil {
			t.Fatalf("mkdir fail: %v", err)
		}
		if err := os.WriteFile(filepath.Join(confRoot, rel), content, 0644); err != nil {
			t.Fatalf("write file fail: %v", err)
		}
	}

	writeFile(filepath.Join(dir, "server_cert_conf.data"), []byte(`{
    "Version": "20260904205703",
    "Config": {
        "Default": "server",
        "CertConf": {
            "server": {
                "ServerCertFile": "certs/server.crt",
                "ServerKeyFile": "certs/server.key"
            }
        }
    }
}`))
	writeFile(filepath.Join(dir, "tls_rule_conf.data"), []byte(`{
    "Version": "20260904205703",
    "Config": {
        "prod": {
            "CertName": "server",
            "SniConf": ["example.com"],
            "ClientAuth": true,
            "ClientCAName": "testca"
        }
    }
}`))

	// shared server cert/key, resolved relative to confRoot
	writeFile("certs/server.crt", serverCertPEM)
	writeFile("certs/server.key", serverKeyPEM)

	if withClientCA {
		writeFile(filepath.Join(dir, "client_ca", "testca.crt"), clientCAPEM)
	} else if err := os.MkdirAll(filepath.Join(confRoot, dir, "client_ca"), 0755); err != nil {
		t.Fatalf("mkdir client_ca fail: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(confRoot, dir, "client_crl"), 0755); err != nil {
		t.Fatalf("mkdir client_crl fail: %v", err)
	}
}

func newTestBfeServer(confRoot string, httpsBasic bfe_conf.ConfigHttpsBasic) *BfeServer {
	srv := &BfeServer{ConfRoot: confRoot}
	srv.Config.HttpsBasic = httpsBasic
	srv.serverStatus = NewServerStatus()
	srv.MultiCert = NewMultiCertMap(srv.serverStatus.ProxyState)
	srv.TLSServerRule = NewTLSServerRuleMap(srv.serverStatus.ProxyState)
	return srv
}

func httpsBasicFor(confRoot, clientCABaseDir string) bfe_conf.ConfigHttpsBasic {
	return bfe_conf.ConfigHttpsBasic{
		ServerCertConf:   filepath.Join(confRoot, "tls_conf", "server_cert_conf.data"),
		TlsRuleConf:      filepath.Join(confRoot, "tls_conf", "tls_rule_conf.data"),
		ClientCABaseDir:  clientCABaseDir,
		ClientCRLBaseDir: filepath.Join(confRoot, "tls_conf", "client_crl"),
	}
}

// TestTLSConfReload_ClientCARelocatedWithPath is the core regression test for
// rainway-ai-gateway/conf-agent#19: with "path" pointing to a versioned config
// dir, client CA must be loaded from the version dir (where conf-agent placed
// it), not from the activated tls_conf dir.
func TestTLSConfReload_ClientCARelocatedWithPath(t *testing.T) {
	confRoot := t.TempDir()
	// activated dir lacks client_ca/testca.crt (stale), version dir has it
	writeTLSConfTree(t, confRoot, "tls_conf", false)
	writeTLSConfTree(t, confRoot, "tls_conf_20260904205703", true)

	srv := newTestBfeServer(confRoot, httpsBasicFor(confRoot, filepath.Join(confRoot, "tls_conf", "client_ca")))
	query := url.Values{"path": {filepath.Join(confRoot, "tls_conf_20260904205703")}}

	if err := srv.TLSConfReload(query); err != nil {
		t.Fatalf("TLSConfReload with path fail: %v", err)
	}
}

// TestTLSConfReload_WithoutPathUsesActivatedDir verifies that a reload without
// "path" still reads client CA from the activated tls_conf dir and fails when
// the referenced CA file is missing there.
func TestTLSConfReload_WithoutPathUsesActivatedDir(t *testing.T) {
	confRoot := t.TempDir()
	writeTLSConfTree(t, confRoot, "tls_conf", false)
	writeTLSConfTree(t, confRoot, "tls_conf_20260904205703", true)

	srv := newTestBfeServer(confRoot, httpsBasicFor(confRoot, filepath.Join(confRoot, "tls_conf", "client_ca")))

	err := srv.TLSConfReload(url.Values{})
	if err == nil {
		t.Fatal("TLSConfReload without path should fail: testca.crt missing in activated dir")
	}
	t.Logf("got expected error: %v", err)
}

// TestTLSConfReload_CustomAbsoluteCABaseDirNotRelocated verifies that a client
// CA base dir outside tls_conf is left untouched by the "path" relocation.
func TestTLSConfReload_CustomAbsoluteCABaseDirNotRelocated(t *testing.T) {
	confRoot := t.TempDir()
	writeTLSConfTree(t, confRoot, "tls_conf", false)
	// version dir intentionally lacks client_ca/testca.crt
	writeTLSConfTree(t, confRoot, "tls_conf_20260904205703", false)

	clientCAPEM, _ := generateCertPEM(t, "testca", nil, true)
	customDir := filepath.Join(confRoot, "custom_ca")
	if err := os.MkdirAll(customDir, 0755); err != nil {
		t.Fatalf("mkdir custom_ca fail: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "testca.crt"), clientCAPEM, 0644); err != nil {
		t.Fatalf("write custom ca fail: %v", err)
	}

	srv := newTestBfeServer(confRoot, httpsBasicFor(confRoot, customDir))
	query := url.Values{"path": {filepath.Join(confRoot, "tls_conf_20260904205703")}}

	if err := srv.TLSConfReload(query); err != nil {
		t.Fatalf("TLSConfReload with custom CA dir fail: %v", err)
	}
}

// TestTLSConfReload_ClientCAMissingInVersionDir verifies that a missing client
// CA file in the version dir still makes the reload fail (validation logic is
// unchanged, only the directory changed).
func TestTLSConfReload_ClientCAMissingInVersionDir(t *testing.T) {
	confRoot := t.TempDir()
	writeTLSConfTree(t, confRoot, "tls_conf", true)
	writeTLSConfTree(t, confRoot, "tls_conf_20260904205703", false)

	srv := newTestBfeServer(confRoot, httpsBasicFor(confRoot, filepath.Join(confRoot, "tls_conf", "client_ca")))
	query := url.Values{"path": {filepath.Join(confRoot, "tls_conf_20260904205703")}}

	err := srv.TLSConfReload(query)
	if err == nil {
		t.Fatal("TLSConfReload with path should fail: testca.crt missing in version dir")
	}
	t.Logf("got expected error: %v", err)
}
