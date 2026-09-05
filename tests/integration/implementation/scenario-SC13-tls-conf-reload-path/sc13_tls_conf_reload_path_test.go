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

package sc13

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

// clusterNames mirrors the clusters referenced by the static testdata
// (gslb.data / ai_route.data). Mock backends are required for BFE to build
// a coherent cluster table at startup, even though this scenario never
// proxies traffic.
var clusterNames = []string{
	"cluster_primary_a",
	"cluster_primary_b",
	"cluster_primary_c",
	"cluster_fallback_1",
	"cluster_fallback_2",
	"cluster_entity_default",
	"cluster_global_default",
	"cluster_holder",
}

const (
	// versionDirName mimics a conf-agent generated versioned tls_conf dir.
	versionDirName = "tls_conf_20260904205703"
	sniName        = "example.org"
)

// testEnv holds all resources for a single SC13 integration test.
type testEnv struct {
	t              *testing.T
	processEnv     *common.ProcessEnv
	backends       map[string]*common.MockBackend
	confDir        string
	httpsPort      int
	bfeMonitorPort int
	stopBFE        func()
}

func newTestEnv(t *testing.T) *testEnv {
	e := &testEnv{t: t}

	e.backends = make(map[string]*common.MockBackend)
	for _, name := range clusterNames {
		e.backends[name] = common.NewMockBackend(name, http.StatusOK, fmt.Sprintf("response from %s", name))
	}

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	logDir := filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends:      e.backends,
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	var stop func()
	_, e.bfeMonitorPort, stop = e.processEnv.StartBFE(e.confDir, logDir)
	e.stopBFE = stop

	e.httpsPort = readHTTPSPort(t, filepath.Join(e.confDir, "bfe.conf"))
	return e
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	for _, b := range e.backends {
		b.Close()
	}
}

// readHTTPSPort parses the httpsPort line rewritten by StartBFE.
func readHTTPSPort(t *testing.T, bfeConf string) int {
	t.Helper()
	data, err := os.ReadFile(bfeConf)
	if err != nil {
		t.Fatalf("read bfe.conf failed: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "httpsPort") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			t.Fatalf("parse httpsPort line failed: %s", trimmed)
		}
		port, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			t.Fatalf("parse httpsPort failed: %v", err)
		}
		return port
	}
	t.Fatal("httpsPort not found in bfe.conf")
	return 0
}

// tlsHandshake performs a TLS handshake against the BFE https listener.
// A nil clientCert means the client sends no certificate.
func (e *testEnv) tlsHandshake(clientCert *tls.Certificate) error {
	conf := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // tests verify reload behavior, not server identity
		ServerName:         sniName,
		NextProtos:         []string{"http/1.1"},
	}
	if clientCert != nil {
		conf.Certificates = []tls.Certificate{*clientCert}
	}

	conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", e.httpsPort), conf)
	if err != nil {
		return err
	}
	return conn.Close()
}

// reloadTLSConf calls the BFE monitor endpoint /reload/tls_conf and returns
// the error field of the JSON response (empty means success).
func (e *testEnv) reloadTLSConf(path string) string {
	url := fmt.Sprintf("http://127.0.0.1:%d/reload/tls_conf", e.bfeMonitorPort)
	if path != "" {
		url += "?path=" + path
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		e.t.Fatalf("call reload tls_conf failed: %v", err)
	}
	defer resp.Body.Close()

	rsp := &struct {
		Error string `json:"error"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(rsp); err != nil {
		e.t.Fatalf("decode reload rsp failed: %v", err)
	}
	return rsp.Error
}

// copyDir recursively copies a directory tree.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read dir %s failed: %v", src, err)
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatalf("mkdir %s failed: %v", dst, err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			copyDir(t, srcPath, dstPath)
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("read %s failed: %v", srcPath, err)
		}
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			t.Fatalf("write %s failed: %v", dstPath, err)
		}
	}
}

// buildVersionDir creates a versioned tls_conf dir mimicking what conf-agent
// produces: a full copy of the activated tls_conf, with tls_rule_conf.data
// switched to ClientAuth=true (ClientCAName=example_ca). The client CA file
// is present only when withClientCA is true.
func (e *testEnv) buildVersionDir(withClientCA bool) {
	e.t.Helper()

	activated := filepath.Join(e.confDir, "tls_conf")
	versioned := filepath.Join(e.confDir, versionDirName)
	copyDir(e.t, activated, versioned)

	rule := `{
    "Version": "20260904205703",
    "Config": {
        "example_product": {
            "VipConf": [
                "10.199.4.14"
            ],
            "SniConf": ["example.org"],
            "CertName": "example.org",
            "NextProtos": [
                "http/1.1"
            ],
            "Grade": "C",
            "ClientAuth": true,
            "ClientCAName": "example_ca"
        }
    }
}`
	if err := os.WriteFile(filepath.Join(versioned, "tls_rule_conf.data"), []byte(rule), 0644); err != nil {
		e.t.Fatalf("write versioned tls_rule_conf.data failed: %v", err)
	}

	caFile := filepath.Join(versioned, "client_ca", "example_ca.crt")
	if withClientCA {
		// conf-agent places the new client CA only in the version dir; the
		// activated dir stays stale (this is exactly the conf-agent#19 setup).
		data, err := os.ReadFile(filepath.Join("testdata", "tls_conf", "client_ca", "example_ca.crt"))
		if err != nil {
			e.t.Fatalf("read example_ca.crt from template failed: %v", err)
		}
		if err := os.WriteFile(caFile, data, 0644); err != nil {
			e.t.Fatalf("write versioned client ca failed: %v", err)
		}
	} else if err := os.Remove(caFile); err != nil {
		e.t.Fatalf("remove versioned client ca failed: %v", err)
	}
}

// makeClientCert builds a client certificate signed by example_ca, which the
// server trusts under ClientCAName=example_ca.
func (e *testEnv) makeClientCert() tls.Certificate {
	e.t.Helper()

	caCertPEM, err := os.ReadFile(filepath.Join("testdata", "tls_conf", "client_ca", "example_ca.crt"))
	if err != nil {
		e.t.Fatalf("read ca cert failed: %v", err)
	}
	caKeyPEM, err := os.ReadFile(filepath.Join("testdata", "tls_conf", "client_ca", "example_ca.key"))
	if err != nil {
		e.t.Fatalf("read ca key failed: %v", err)
	}

	caBlock, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		e.t.Fatalf("parse ca cert failed: %v", err)
	}
	keyBlock, _ := pem.Decode(caKeyPEM)
	caKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		e.t.Fatalf("parse ca key failed: %v", err)
	}

	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		e.t.Fatalf("generate client key failed: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "sc13-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		e.t.Fatalf("create client cert failed: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		e.t.Fatalf("build client key pair failed: %v", err)
	}
	return cert
}

// TestTC01_ReloadWithPathUsesVersionDirClientCA is the core regression test
// for rainway-ai-gateway/conf-agent#19: with "path" pointing to a versioned
// tls_conf dir, client CA must be loaded from the version dir even when the
// activated tls_conf dir lacks it.
func TestTC01_ReloadWithPathUsesVersionDirClientCA(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	// Simulate stale activated dir: the referenced client CA is missing.
	if err := os.Remove(filepath.Join(e.confDir, "tls_conf", "client_ca", "example_ca.crt")); err != nil {
		t.Fatalf("remove activated client ca failed: %v", err)
	}

	// Baseline: ClientAuth=false, handshake without client cert succeeds.
	if err := e.tlsHandshake(nil); err != nil {
		t.Fatalf("baseline handshake without client cert should succeed: %v", err)
	}

	e.buildVersionDir(true)

	if errMsg := e.reloadTLSConf(versionDirName); errMsg != "" {
		t.Fatalf("reload with path should succeed, got error: %s", errMsg)
	}

	// After reload: server requires client cert. Handshake without cert fails.
	if err := e.tlsHandshake(nil); err == nil {
		t.Fatal("handshake without client cert should fail after ClientAuth reload")
	} else {
		t.Logf("handshake without client cert failed as expected: %v", err)
	}

	// Handshake with a valid client cert succeeds.
	clientCert := e.makeClientCert()
	if err := e.tlsHandshake(&clientCert); err != nil {
		t.Fatalf("handshake with client cert should succeed: %v", err)
	}
}

// TestTC02_ReloadWithPathMissingClientCA verifies that a missing client CA
// file in the version dir still makes the reload fail (validation logic is
// unchanged, only the base dir changed).
func TestTC02_ReloadWithPathMissingClientCA(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	e.buildVersionDir(false)

	errMsg := e.reloadTLSConf(versionDirName)
	if errMsg == "" {
		t.Fatal("reload with path should fail: example_ca.crt missing in version dir")
	}
	if !strings.Contains(errMsg, "ClientCALoad") {
		t.Fatalf("error should mention ClientCALoad, got: %s", errMsg)
	}
	t.Logf("got expected error: %s", errMsg)
}

// TestTC03_ReloadWithoutPathKeepsActivatedDir verifies that a reload without
// "path" still reads from the activated tls_conf dir and is not affected by
// version dir content.
func TestTC03_ReloadWithoutPathKeepsActivatedDir(t *testing.T) {
	e := newTestEnv(t)
	defer e.Close()

	// Activated dir lacks example_ca.crt; version dir has complete config.
	if err := os.Remove(filepath.Join(e.confDir, "tls_conf", "client_ca", "example_ca.crt")); err != nil {
		t.Fatalf("remove activated client ca failed: %v", err)
	}
	e.buildVersionDir(true)

	if errMsg := e.reloadTLSConf(""); errMsg != "" {
		t.Fatalf("reload without path should succeed, got error: %s", errMsg)
	}

	// Activated config still has ClientAuth=false: handshake without cert works.
	if err := e.tlsHandshake(nil); err != nil {
		t.Fatalf("handshake without client cert should succeed after no-path reload: %v", err)
	}
}
