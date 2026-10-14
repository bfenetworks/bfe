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
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

// TestTC01_TLSVersionNegotiation verifies the TLS version boundaries after
// the fork removal: TLS 1.3 by default, TLS 1.2 for capped clients, and
// TLS 1.0/1.1 rejected at the protocol layer (removed from the standard
// library).
func TestTC01_TLSVersionNegotiation(t *testing.T) {
	e := newTestEnv(t, nil)
	defer e.Close()

	// Default client: TLS 1.3.
	conn, err := e.tlsDial(sniOrg, nil, 0, []string{"http/1.1"}, nil)
	if err != nil {
		t.Fatalf("default tls handshake failed: %v", err)
	}
	if version := conn.ConnectionState().Version; version != tls.VersionTLS13 {
		t.Fatalf("default handshake should negotiate TLS 1.3, got %x", version)
	}
	head, err := httpGet(conn, sniOrg)
	conn.Close()
	if err != nil {
		t.Fatalf("tls1.3 http request failed: %v", err)
	}
	if !strings.Contains(head, "200") {
		t.Fatalf("tls1.3 http request should return 200, got: %q", head)
	}

	// Client capped at TLS 1.2: still works.
	conn, err = e.tlsDial(sniOrg, nil, tls.VersionTLS12, []string{"http/1.1"}, nil)
	if err != nil {
		t.Fatalf("tls1.2 handshake failed: %v", err)
	}
	if version := conn.ConnectionState().Version; version != tls.VersionTLS12 {
		t.Fatalf("capped handshake should negotiate TLS 1.2, got %x", version)
	}
	head, err = httpGet(conn, sniOrg)
	conn.Close()
	if err != nil {
		t.Fatalf("tls1.2 http request failed: %v", err)
	}
	if !strings.Contains(head, "200") {
		t.Fatalf("tls1.2 http request should return 200, got: %q", head)
	}

	// TLS 1.1 / 1.0 are not supported by the standard library any more.
	for _, version := range []uint16{tls.VersionTLS11, tls.VersionTLS10} {
		if _, err := e.tlsDial(sniOrg, nil, version, []string{"http/1.1"}, nil); err == nil {
			t.Fatalf("handshake with max version %x should fail", version)
		} else {
			t.Logf("handshake with max version %x failed as expected: %v", version, err)
		}
	}
}

// TestTC02_MultiCertSNISelection verifies that the standard library
// GetCertificate callback preserves the MultiCert behavior: certificate
// selection by SNI with fallback to the default certificate.
func TestTC02_MultiCertSNISelection(t *testing.T) {
	e := newTestEnv(t, nil)
	defer e.Close()

	checkSNI := func(serverName, wantDNS string) {
		conn, err := e.tlsDial(serverName, nil, 0, []string{"http/1.1"}, nil)
		if err != nil {
			t.Fatalf("handshake with sni %s failed: %v", serverName, err)
		}
		defer conn.Close()
		peer := conn.ConnectionState().PeerCertificates
		if len(peer) == 0 {
			t.Fatalf("sni %s: no peer certificate", serverName)
		}
		found := false
		for _, name := range peer[0].DNSNames {
			if name == wantDNS {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("sni %s: expect cert with DNSName %s, got %v", serverName, wantDNS, peer[0].DNSNames)
		}
	}

	checkSNI(sniOrg, sniOrg)
	checkSNI(sniNet, sniNet)
	// Unknown SNI falls back to the default certificate (example.org).
	checkSNI("unknown.example", sniOrg)
}

// TestTC03_ALPNH2Negotiation verifies the NextProtos gray-scale logic and
// the global h2 switch delivered per connection via GetConfigForClient, and
// that an end-to-end HTTP/2 request is proxied correctly.
func TestTC03_ALPNH2Negotiation(t *testing.T) {
	e := newTestEnv(t, nil)
	defer e.Close()

	// ALPN [h2, http/1.1] with rate=100 gray-scale: h2 wins.
	conn, err := e.tlsDial(sniOrg, nil, 0, []string{"h2", "http/1.1"}, nil)
	if err != nil {
		t.Fatalf("handshake with alpn h2 failed: %v", err)
	}
	if proto := conn.ConnectionState().NegotiatedProtocol; proto != "h2" {
		t.Fatalf("expect negotiated protocol h2, got %q", proto)
	}
	conn.Close()

	// A real HTTP/2 request over the negotiated h2 connection is proxied.
	tr := &http2.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // tests verify protocol behavior, not server identity
			ServerName:         sniOrg,
			NextProtos:         []string{"h2"},
		},
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://127.0.0.1:%d/", e.httpsPort), nil)
	if err != nil {
		t.Fatalf("build http2 request failed: %v", err)
	}
	// :authority defaults to the URL host; set it explicitly so host routing
	// matches example_product (same as the Host header in raw http/1.1 tests).
	req.Host = sniOrg
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("http2 request failed: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read http2 response failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("http2 request should return 200, got %d", resp.StatusCode)
	}
	if resp.ProtoMajor != 2 {
		t.Fatalf("expect HTTP/2 response, got proto %s", resp.Proto)
	}
	if !strings.Contains(string(body), clusterName) {
		t.Fatalf("http2 response should come from mock backend, got: %q", string(body))
	}

	// Client declaring only http/1.1: negotiated http/1.1 and request works.
	conn, err = e.tlsDial(sniOrg, nil, 0, []string{"http/1.1"}, nil)
	if err != nil {
		t.Fatalf("handshake with alpn http/1.1 failed: %v", err)
	}
	if proto := conn.ConnectionState().NegotiatedProtocol; proto != "http/1.1" {
		t.Fatalf("expect negotiated protocol http/1.1, got %q", proto)
	}
	head, err := httpGet(conn, sniOrg)
	conn.Close()
	if err != nil {
		t.Fatalf("http/1.1 request failed: %v", err)
	}
	if !strings.Contains(head, "200") {
		t.Fatalf("http/1.1 request should return 200, got: %q", head)
	}

	// Disable h2 globally via the monitor reload endpoint: ALPN falls back to
	// http/1.1 even though the rule still advertises h2.
	if errMsg := e.reload("tls_conf", "enable=-h2"); errMsg != "" {
		t.Fatalf("reload tls_conf with enable=-h2 failed: %s", errMsg)
	}
	conn, err = e.tlsDial(sniOrg, nil, 0, []string{"h2", "http/1.1"}, nil)
	if err != nil {
		t.Fatalf("handshake after -h2 reload failed: %v", err)
	}
	if proto := conn.ConnectionState().NegotiatedProtocol; proto != "http/1.1" {
		t.Fatalf("h2 globally disabled, expect negotiated protocol http/1.1, got %q", proto)
	}
	conn.Close()
}

// TestTC04_ClientAuthAtStartup verifies client certificate verification with
// ClientAuth enabled at startup (legacy bool literal mapping to
// tls.RequireAndVerifyClientCert), covering the TLS 1.2 handshake-level and
// TLS 1.3 first-request-level observability difference.
func TestTC04_ClientAuthAtStartup(t *testing.T) {
	e := newTestEnv(t, &envOptions{clientAuth: true})
	defer e.Close()

	// TLS 1.2 without a client certificate: handshake fails synchronously.
	if _, err := e.tlsDial(sniOrg, nil, tls.VersionTLS12, []string{"http/1.1"}, nil); err == nil {
		t.Fatal("tls1.2 handshake without client cert should fail")
	} else {
		t.Logf("tls1.2 handshake without client cert failed as expected: %v", err)
	}

	// TLS 1.3 without a client certificate: tls.Dial may succeed (the server
	// sends Finished first), enforcement surfaces on the first request.
	if err := e.httpsRoundTrip(sniOrg, nil, 0); err == nil {
		t.Fatal("tls1.3 request without client cert should fail")
	} else {
		t.Logf("tls1.3 request without client cert failed as expected: %v", err)
	}

	// Valid client certificate issued by the trusted example_ca.
	clientCert := e.clientCA.newClientCert(t, serialOK, "sc20-client")
	if err := e.httpsRoundTrip(sniOrg, &clientCert, tls.VersionTLS12); err != nil {
		t.Fatalf("tls1.2 request with client cert should succeed: %v", err)
	}
	if err := e.httpsRoundTrip(sniOrg, &clientCert, 0); err != nil {
		t.Fatalf("tls1.3 request with client cert should succeed: %v", err)
	}

	// Client certificate issued by an untrusted CA: rejected.
	untrustedCA := newTestCA(t, "sc20-untrusted-ca")
	badCert := untrustedCA.newClientCert(t, serialOK, "sc20-bad-client")
	if _, err := e.tlsDial(sniOrg, &badCert, tls.VersionTLS12, []string{"http/1.1"}, nil); err == nil {
		t.Fatal("tls1.2 handshake with untrusted client cert should fail")
	} else {
		t.Logf("tls1.2 handshake with untrusted client cert failed as expected: %v", err)
	}
}

// TestTC05_ClientCertCRLRevoked verifies CRL revocation checks implemented
// via tls.Config.VerifyConnection: a revoked serial is rejected, a clean one
// passes. TLS 1.2 is used for deterministic handshake-level failure.
func TestTC05_ClientCertCRLRevoked(t *testing.T) {
	e := newTestEnv(t, &envOptions{clientAuth: true, clientCRL: true})
	defer e.Close()

	// Serial 1001 is not revoked: request succeeds.
	okCert := e.clientCA.newClientCert(t, serialOK, "sc20-client-ok")
	if err := e.httpsRoundTrip(sniOrg, &okCert, tls.VersionTLS12); err != nil {
		t.Fatalf("request with non-revoked client cert should succeed: %v", err)
	}

	// Serial 1002 is revoked by tls_conf/client_crl/example_ca/revoke.crl.
	revokedCert := e.clientCA.newClientCert(t, serialRevoked, "sc20-client-revoked")
	if _, err := e.tlsDial(sniOrg, &revokedCert, tls.VersionTLS12, []string{"http/1.1"}, nil); err == nil {
		t.Fatal("handshake with revoked client cert should fail")
	} else {
		t.Logf("handshake with revoked client cert failed as expected: %v", err)
	}
}

// TestTC06_SessionTicketResumptionAndKeyRotation verifies standard library
// in-process session ticket resumption (TLS 1.3 and 1.2), and that rotating
// the ticket key via /reload/tls_session_ticket_key invalidates old tickets.
func TestTC06_SessionTicketResumptionAndKeyRotation(t *testing.T) {
	e := newTestEnv(t, nil)
	defer e.Close()

	// --- TLS 1.3 resumption ---
	cache13 := tls.NewLRUClientSessionCache(8)
	// First connection: a full request round trip so the client reads and
	// caches the post-handshake session tickets.
	conn, err := e.tlsDial(sniOrg, nil, 0, []string{"http/1.1"}, cache13)
	if err != nil {
		t.Fatalf("tls1.3 first handshake failed: %v", err)
	}
	if _, err := httpGet(conn, sniOrg); err != nil {
		t.Fatalf("tls1.3 first request failed: %v", err)
	}
	conn.Close()

	conn, err = e.tlsDial(sniOrg, nil, 0, []string{"http/1.1"}, cache13)
	if err != nil {
		t.Fatalf("tls1.3 second handshake failed: %v", err)
	}
	// Note: ConnectionState.DidResume is set client-side for TLS 1.3 PSK
	// acceptance (verified on go1.24); DidTLS13Resume only exists in go1.25+.
	if !conn.ConnectionState().DidResume {
		t.Fatal("tls1.3 second connection should resume from session ticket")
	}
	conn.Close()

	// --- TLS 1.2 resumption ---
	cache12 := tls.NewLRUClientSessionCache(8)
	conn, err = e.tlsDial(sniOrg, nil, tls.VersionTLS12, []string{"http/1.1"}, cache12)
	if err != nil {
		t.Fatalf("tls1.2 first handshake failed: %v", err)
	}
	if _, err := httpGet(conn, sniOrg); err != nil {
		t.Fatalf("tls1.2 first request failed: %v", err)
	}
	conn.Close()

	conn, err = e.tlsDial(sniOrg, nil, tls.VersionTLS12, []string{"http/1.1"}, cache12)
	if err != nil {
		t.Fatalf("tls1.2 second handshake failed: %v", err)
	}
	if !conn.ConnectionState().DidResume {
		t.Fatal("tls1.2 second connection should resume from session ticket")
	}
	conn.Close()

	// --- Rotate the session ticket key ---
	newKey := `{
    "Version": "rotated version",
    "SessionTicketKey": "ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100"
}`
	keyPath := filepath.Join(e.confDir, "tls_conf", "session_ticket_key.data")
	if err := os.WriteFile(keyPath, []byte(newKey), 0644); err != nil {
		t.Fatalf("write rotated session ticket key failed: %v", err)
	}
	if errMsg := e.reload("tls_session_ticket_key", ""); errMsg != "" {
		t.Fatalf("reload tls_session_ticket_key failed: %s", errMsg)
	}

	// Old tickets can no longer be decrypted: full handshakes are required,
	// but the connections still succeed.
	conn, err = e.tlsDial(sniOrg, nil, 0, []string{"http/1.1"}, cache13)
	if err != nil {
		t.Fatalf("tls1.3 handshake after key rotation failed: %v", err)
	}
	if conn.ConnectionState().DidResume {
		t.Fatal("tls1.3 session should not resume after ticket key rotation")
	}
	conn.Close()

	conn, err = e.tlsDial(sniOrg, nil, tls.VersionTLS12, []string{"http/1.1"}, cache12)
	if err != nil {
		t.Fatalf("tls1.2 handshake after key rotation failed: %v", err)
	}
	if conn.ConnectionState().DidResume {
		t.Fatal("tls1.2 session should not resume after ticket key rotation")
	}
	conn.Close()
}

// TestTC07_DeprecatedFieldsCompat verifies that removed fork features keep
// their configuration parseable and inert: Grade/Chacha20/DynamicRecord in
// tls_rule_conf.data (already in the base template), plus the bfe.conf
// [SessionCache] section and EnableSslv2ClientHello.
func TestTC07_DeprecatedFieldsCompat(t *testing.T) {
	deprecatedConf := `
[HttpsBasic]
EnableSslv2ClientHello = true

[SessionCache]
SessionCacheDisabled = true
servers = "10.1.2.3:9000"
connectTimeout = 10
readTimeout = 10
writeTimeout = 10
maxIdle = 10
sessionExpire = 600000
`
	e := newTestEnv(t, &envOptions{extraBfeConf: deprecatedConf})
	defer e.Close()

	// BFE started successfully with the legacy configuration; both TLS 1.3
	// and 1.2 round trips work (session cache fields are ignored, no Redis
	// connection is attempted).
	if err := e.httpsRoundTrip(sniOrg, nil, 0); err != nil {
		t.Fatalf("tls1.3 round trip with deprecated config failed: %v", err)
	}
	if err := e.httpsRoundTrip(sniOrg, nil, tls.VersionTLS12); err != nil {
		t.Fatalf("tls1.2 round trip with deprecated config failed: %v", err)
	}
}

// TestTC08_KeyLogWriterNSSFormat verifies that mod_key_log, now wired as
// tls.Config.KeyLogWriter, emits NSS key log lines during the handshake:
// TLS 1.3 traffic secrets and the TLS 1.2 CLIENT_RANDOM line.
func TestTC08_KeyLogWriterNSSFormat(t *testing.T) {
	e := newTestEnv(t, &envOptions{modules: "mod_key_log"})
	defer e.Close()

	if err := e.httpsRoundTrip(sniOrg, nil, 0); err != nil {
		t.Fatalf("tls1.3 round trip failed: %v", err)
	}
	if err := e.httpsRoundTrip(sniOrg, nil, tls.VersionTLS12); err != nil {
		t.Fatalf("tls1.2 round trip failed: %v", err)
	}

	keyLogPath := filepath.Join(e.logDir, "key.log")
	data, err := os.ReadFile(keyLogPath)
	if err != nil {
		t.Fatalf("read key log file failed: %v", err)
	}
	content := string(data)
	if content == "" {
		t.Fatal("key log file should not be empty")
	}
	for _, label := range []string{
		"CLIENT_HANDSHAKE_TRAFFIC_SECRET",
		"SERVER_HANDSHAKE_TRAFFIC_SECRET",
		"CLIENT_TRAFFIC_SECRET_0",
		"CLIENT_RANDOM",
	} {
		if !strings.Contains(content, label) {
			t.Fatalf("key log should contain %s line, got:\n%s", label, content)
		}
	}
}
