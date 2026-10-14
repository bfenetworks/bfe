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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	clusterName = "cluster_sc20"
	sniOrg      = "example.org"
	sniNet      = "example.net"

	// serialOK / serialRevoked are fixed client certificate serial numbers;
	// the CRL generated for TC-05 revokes serialRevoked only.
	serialOK      = 1001
	serialRevoked = 1002
)

// envOptions customizes the generated BFE configuration before startup.
type envOptions struct {
	// clientAuth rewrites tls_rule_conf.data so that the example.org product
	// requires client certificate verification (legacy bool literal format).
	clientAuth bool
	// clientCRL writes a CRL revoking serialRevoked into
	// tls_conf/client_crl/example_ca/revoke.crl (requires clientAuth).
	clientCRL bool
	// extraBfeConf is appended verbatim to bfe.conf (TC-07 deprecated fields).
	extraBfeConf string
	// modules replaces the Modules line in bfe.conf (TC-08: mod_key_log).
	modules string
}

// testEnv holds all resources for a single SC20 integration test.
type testEnv struct {
	t           *testing.T
	processEnv  *common.ProcessEnv
	backend     *common.MockBackend
	confDir     string
	logDir      string
	httpsPort   int
	monitorPort int
	stopBFE     func()

	serverCA *testCA
	clientCA *testCA
}

func newTestEnv(t *testing.T, opts *envOptions) *testEnv {
	if opts == nil {
		opts = &envOptions{}
	}
	e := &testEnv{t: t}

	e.backend = common.NewMockBackend(clusterName, http.StatusOK, fmt.Sprintf("response from %s", clusterName))

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends:      map[string]*common.MockBackend{clusterName: e.backend},
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	e.generatePKI(opts)

	if opts.clientAuth {
		e.writeTLSRuleConf(true)
	}
	if opts.extraBfeConf != "" {
		e.appendBFEConf(opts.extraBfeConf)
	}
	if opts.modules != "" {
		e.rewriteModules(opts.modules)
	}

	var stop func()
	_, e.monitorPort, stop = e.processEnv.StartBFE(e.confDir, e.logDir)
	e.stopBFE = stop

	e.httpsPort = readHTTPSPort(t, filepath.Join(e.confDir, "bfe.conf"))
	return e
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	e.backend.Close()
}

// generatePKI creates the server CA (two SAN server certificates) and the
// client CA, and writes them into the built conf dir. Optionally writes the
// client CRL for TC-05.
func (e *testEnv) generatePKI(opts *envOptions) {
	e.t.Helper()

	e.serverCA = newTestCA(e.t, "sc20-server-ca")
	certsDir := filepath.Join(e.confDir, "tls_conf", "certs")
	if err := os.MkdirAll(certsDir, 0755); err != nil {
		e.t.Fatalf("mkdir certs dir failed: %v", err)
	}
	for _, name := range []string{sniOrg, sniNet} {
		certPEM, keyPEM := e.serverCA.newServerCert(e.t, name, []string{name})
		if err := os.WriteFile(filepath.Join(certsDir, name+".crt"), certPEM, 0644); err != nil {
			e.t.Fatalf("write server cert failed: %v", err)
		}
		if err := os.WriteFile(filepath.Join(certsDir, name+".key"), keyPEM, 0644); err != nil {
			e.t.Fatalf("write server key failed: %v", err)
		}
	}

	e.clientCA = newTestCA(e.t, "sc20-client-ca")
	caDir := filepath.Join(e.confDir, "tls_conf", "client_ca")
	if err := os.MkdirAll(caDir, 0755); err != nil {
		e.t.Fatalf("mkdir client ca dir failed: %v", err)
	}
	e.clientCA.writeCertTo(e.t, filepath.Join(caDir, "example_ca.crt"))

	if opts.clientCRL {
		crlDir := filepath.Join(e.confDir, "tls_conf", "client_crl", "example_ca")
		if err := os.MkdirAll(crlDir, 0755); err != nil {
			e.t.Fatalf("mkdir client crl dir failed: %v", err)
		}
		crlDER := e.clientCA.newCRL(e.t, serialRevoked)
		if err := os.WriteFile(filepath.Join(crlDir, "revoke.crl"), crlDER, 0644); err != nil {
			e.t.Fatalf("write crl failed: %v", err)
		}
	}
}

// writeTLSRuleConf rewrites tls_rule_conf.data. When clientAuth is true the
// example.org product uses the legacy bool literal ("ClientAuth": true) to
// verify backward-compatible parsing.
func (e *testEnv) writeTLSRuleConf(clientAuth bool) {
	e.t.Helper()
	rule := fmt.Sprintf(`{
    "Version": "13",
    "DefaultNextProtos": ["http/1.1"],
    "DefaultChacha20": true,
    "Config": {
        "example_product": {
            "VipConf": [
                "10.199.4.14"
            ],
            "SniConf": ["example.org"],
            "CertName": "example.org",
            "NextProtos": [
                "h2;rate=100;isw=65535;mcs=200;level=0",
                "http/1.1"
            ],
            "Grade": "A+",
            "Chacha20": true,
            "DynamicRecord": true,
            "ClientAuth": %t,
            "ClientCAName": "example_ca"
        },
        "example_product_net": {
            "VipConf": [
                "10.199.4.15"
            ],
            "SniConf": ["example.net"],
            "CertName": "example.net",
            "NextProtos": [
                "http/1.1"
            ],
            "Grade": "C",
            "ClientAuth": false,
            "ClientCAName": "example_ca"
        }
    }
}`, clientAuth)
	path := filepath.Join(e.confDir, "tls_conf", "tls_rule_conf.data")
	if err := os.WriteFile(path, []byte(rule), 0644); err != nil {
		e.t.Fatalf("write tls_rule_conf.data failed: %v", err)
	}
}

func (e *testEnv) appendBFEConf(extra string) {
	e.t.Helper()
	path := filepath.Join(e.confDir, "bfe.conf")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		e.t.Fatalf("open bfe.conf failed: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString("\n" + extra + "\n"); err != nil {
		e.t.Fatalf("append bfe.conf failed: %v", err)
	}
}

func (e *testEnv) rewriteModules(modules string) {
	e.t.Helper()
	path := filepath.Join(e.confDir, "bfe.conf")
	data, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatalf("read bfe.conf failed: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Modules") {
			lines[i] = "Modules = " + modules
		}
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		e.t.Fatalf("write bfe.conf failed: %v", err)
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

// tlsDial performs a TLS handshake against the BFE https listener and returns
// the established connection (caller closes). serverName may be empty to omit
// SNI; nextProtos nil means no ALPN.
func (e *testEnv) tlsDial(serverName string, clientCert *tls.Certificate,
	maxVersion uint16, nextProtos []string, sessionCache tls.ClientSessionCache) (*tls.Conn, error) {
	conf := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // tests verify protocol behavior, not server identity
		ServerName:         serverName,
		NextProtos:         nextProtos,
		MaxVersion:         maxVersion,
		ClientSessionCache: sessionCache,
	}
	if clientCert != nil {
		conf.Certificates = []tls.Certificate{*clientCert}
	}
	return tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", e.httpsPort), conf)
}

// httpGet writes a raw HTTP/1.1 request on an established TLS connection and
// returns the response head. It fails if the response does not start with a
// 200 status line.
func httpGet(conn *tls.Conn, host string) (string, error) {
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: " + host + "\r\nConnection: close\r\n\r\n")); err != nil {
		return "", err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}
	head := string(buf[:n])
	if !strings.HasPrefix(head, "HTTP/") {
		return head, fmt.Errorf("unexpected response: %q", head)
	}
	return head, nil
}

// httpsRoundTrip performs a TLS handshake followed by a raw HTTP/1.1 request.
// It is used for TLS 1.3 client-auth enforcement, where tls.Dial may return
// before the server's post-handshake "certificate required" alert arrives;
// the alert (or connection close) is surfaced by the first read/write
// instead. A received HTTP response means the connection was accepted.
func (e *testEnv) httpsRoundTrip(serverName string, clientCert *tls.Certificate, maxVersion uint16) error {
	conn, err := e.tlsDial(serverName, clientCert, maxVersion, []string{"http/1.1"}, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = httpGet(conn, serverName)
	return err
}

// reload calls a BFE monitor /reload/<name> endpoint with the given raw query
// and returns the error field of the JSON response (empty means success).
func (e *testEnv) reload(name, rawQuery string) string {
	url := fmt.Sprintf("http://127.0.0.1:%d/reload/%s", e.monitorPort, name)
	if rawQuery != "" {
		url += "?" + rawQuery
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		e.t.Fatalf("call reload %s failed: %v", name, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("read reload rsp failed: %v", err)
	}
	rsp := &struct {
		Error string `json:"error"`
	}{}
	if err := json.Unmarshal(body, rsp); err != nil {
		// go-lib/web_monitor builds the response with fmt.Sprintf without
		// JSON escaping; on Windows a raw backslash in a file path makes
		// the payload invalid JSON. Fall back to the raw body so error
		// assertions stay cross-platform.
		return string(body)
	}
	return rsp.Error
}
