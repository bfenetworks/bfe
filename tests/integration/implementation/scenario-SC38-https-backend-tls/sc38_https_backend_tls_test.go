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

package sc38

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost = "tls.example.org"
	apiPath = "/v1/chat/completions"

	clusterTLS = "cluster_tls"

	caPath      = "tls_conf/backend_rs/sc38_ca.crt"
	wrongCAPath = "tls_conf/backend_rs/sc38_wrong_ca.crt"
	clientCert  = "tls_conf/backend_rs/sc38_client.crt"
	clientKey   = "tls_conf/backend_rs/sc38_client.key"
	backendHost = "backend.example.org"
)

var defaultBody = []byte(`{"model":"gpt-4"}`)
var okBody = `{"id":"chatcmpl-sc38","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`

// recorder collects strings from TLS callbacks.
type recorder struct {
	mu     sync.Mutex
	values []string
}

func (r *recorder) add(v string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values = append(r.values, v)
}

func (r *recorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.values...)
}

type tlsEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backend    *common.MockBackend

	confDir string
	logDir  string
	bfePort int
	stopBFE func()
}

type envOptions struct {
	serverCertPEM []byte
	serverKeyPEM  []byte
	tlsOpts       common.TLSOptions
	httpsConf     map[string]interface{}
	backendConf   map[string]interface{}
	// files are written into the conf dir after generation (CA/client PEMs).
	files map[string][]byte
}

func newTLSEnv(t *testing.T, opts envOptions) *tlsEnv {
	e := &tlsEnv{t: t}

	opts.tlsOpts.CertPEM = opts.serverCertPEM
	opts.tlsOpts.KeyPEM = opts.serverKeyPEM
	e.backend = common.NewMockBackendTLS(t, clusterTLS, http.StatusOK, okBody, opts.tlsOpts)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	backendConf := map[string]interface{}{"Protocol": "https"}
	for k, v := range opts.backendConf {
		backendConf[k] = v
	}

	httpsConf := make(map[string]interface{}, len(opts.httpsConf)+1)
	for k, v := range opts.httpsConf {
		httpsConf[k] = v
	}
	// BFE dereferences RSInsecureSkipVerify unconditionally when a custom
	// RSCAList is used (bfe_http/transport.go:652); an omitted field panics the
	// process. Write the explicit default so the fixtures describe a valid
	// production config and the tests stay focused on the TLS semantics.
	if _, ok := httpsConf["RSInsecureSkipVerify"]; !ok {
		httpsConf["RSInsecureSkipVerify"] = false
	}

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends: map[string]*common.MockBackend{
			clusterTLS: e.backend,
		},
		ClusterOverrides: map[string]*common.ClusterOverride{
			clusterTLS: {
				BackendConf: backendConf,
				HTTPSConf:   httpsConf,
			},
		},
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	for rel, data := range opts.files {
		if err := common.WritePEMFile(filepath.Join(e.confDir, rel), data); err != nil {
			t.Fatalf("write %s failed: %v", rel, err)
		}
	}

	if err := common.RewriteConfLine(filepath.Join(e.confDir, "bfe.conf"),
		"EnableAiGateway", "false"); err != nil {
		t.Fatalf("disable AI gateway failed: %v", err)
	}

	e.bfePort, _, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
	return e
}

func (e *tlsEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	if e.backend != nil {
		e.backend.Close()
	}
}

func (e *tlsEnv) logBFEException() {
	data, err := os.ReadFile(filepath.Join(e.logDir, "exception.log"))
	if err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}

	logPath := filepath.Join(e.logDir, "bfe.log")
	logData, err := os.ReadFile(logPath)
	if err == nil && len(logData) > 0 {
		lines := strings.Split(string(logData), "\n")
		start := 0
		if len(lines) > 120 {
			start = len(lines) - 120
		}
		e.t.Logf("bfe log tail:\n%s", strings.Join(lines[start:], "\n"))
	}
}

func (e *tlsEnv) sendRequest() (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(defaultBody))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return resp, string(body), nil
}

func (e *tlsEnv) mustSucceed() string {
	e.t.Helper()
	resp, body, err := e.sendRequest()
	if err != nil {
		e.t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		e.t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
	}
	return body
}

func (e *tlsEnv) mustFail() {
	e.t.Helper()
	resp, body, err := e.sendRequest()
	if err != nil {
		e.t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode == http.StatusOK {
		e.t.Fatalf("expected a gateway error for a failed TLS handshake, got 200, body: %s", body)
	}
}

// TestTC01_RSCAListAndRSHost verifies a trusted CA plus RSHost makes the
// handshake succeed with SNI set to the configured host name.
func TestTC01_RSCAListAndRSHost(t *testing.T) {
	ca := common.NewTestCA(t, "sc38-ca")
	serverCert, serverKey := ca.IssueServerCert(t, []string{backendHost}, nil)

	sni := &recorder{}
	e := newTLSEnv(t, envOptions{
		serverCertPEM: serverCert,
		serverKeyPEM:  serverKey,
		tlsOpts:       common.TLSOptions{SNIRecorder: sni.add},
		httpsConf: map[string]interface{}{
			"RSHost":   backendHost,
			"RSCAList": []string{caPath},
		},
		files: map[string][]byte{caPath: caPEM(t, ca)},
	})
	defer e.Close()

	e.mustSucceed()
	if got := sni.all(); len(got) == 0 || got[0] != backendHost {
		t.Fatalf("expected SNI=%s, got %v", backendHost, got)
	}
}

// TestTC02_RSHostFallbackSemantics verifies that without RSHost BFE verifies
// against the backend address (127.0.0.1), so a DNS-only certificate fails;
// adding RSHost makes the same topology succeed.
func TestTC02_RSHostFallbackSemantics(t *testing.T) {
	ca := common.NewTestCA(t, "sc38-ca")
	serverCert, serverKey := ca.IssueServerCert(t, []string{backendHost}, nil)

	t.Run("without RSHost", func(t *testing.T) {
		e := newTLSEnv(t, envOptions{
			serverCertPEM: serverCert,
			serverKeyPEM:  serverKey,
			httpsConf: map[string]interface{}{
				"RSCAList": []string{caPath},
			},
			files: map[string][]byte{caPath: caPEM(t, ca)},
		})
		defer e.Close()
		e.mustFail()
	})

	t.Run("with RSHost", func(t *testing.T) {
		sni := &recorder{}
		e := newTLSEnv(t, envOptions{
			serverCertPEM: serverCert,
			serverKeyPEM:  serverKey,
			tlsOpts:       common.TLSOptions{SNIRecorder: sni.add},
			httpsConf: map[string]interface{}{
				"RSHost":   backendHost,
				"RSCAList": []string{caPath},
			},
			files: map[string][]byte{caPath: caPEM(t, ca)},
		})
		defer e.Close()
		e.mustSucceed()
		if got := sni.all(); len(got) == 0 || got[0] != backendHost {
			t.Fatalf("expected SNI=%s, got %v", backendHost, got)
		}
	})
}

// TestTC03_InsecureSkipVerify verifies RSInsecureSkipVerify accepts a
// certificate that is not trusted by the configured CA.
func TestTC03_InsecureSkipVerify(t *testing.T) {
	untrusted := common.NewTestCA(t, "sc38-untrusted")
	serverCert, serverKey := untrusted.IssueServerCert(t, []string{"other.example.org"}, nil)

	e := newTLSEnv(t, envOptions{
		serverCertPEM: serverCert,
		serverKeyPEM:  serverKey,
		httpsConf: map[string]interface{}{
			"RSHost":               backendHost,
			"RSInsecureSkipVerify": true,
		},
	})
	defer e.Close()

	e.mustSucceed()
}

// TestTC04_VerificationFailure verifies that a certificate from a private CA
// is rejected when no RSCAList is configured (system CA pool is used).
func TestTC04_VerificationFailure(t *testing.T) {
	untrusted := common.NewTestCA(t, "sc38-untrusted")
	serverCert, serverKey := untrusted.IssueServerCert(t, []string{backendHost}, nil)

	e := newTLSEnv(t, envOptions{
		serverCertPEM: serverCert,
		serverKeyPEM:  serverKey,
		httpsConf: map[string]interface{}{
			"RSHost":   backendHost,
			"RSCAList": []string{},
		},
	})
	defer e.Close()

	e.mustFail()
}

// TestTC05_RSCAListSelection verifies RSCAList replaces the system CA pool:
// the matching CA succeeds, an unrelated CA fails.
func TestTC05_RSCAListSelection(t *testing.T) {
	trusted := common.NewTestCA(t, "sc38-trusted")
	other := common.NewTestCA(t, "sc38-other")
	serverCert, serverKey := trusted.IssueServerCert(t, []string{backendHost}, nil)

	t.Run("correct CA", func(t *testing.T) {
		e := newTLSEnv(t, envOptions{
			serverCertPEM: serverCert,
			serverKeyPEM:  serverKey,
			httpsConf: map[string]interface{}{
				"RSHost":   backendHost,
				"RSCAList": []string{caPath},
			},
			files: map[string][]byte{caPath: caPEM(t, trusted)},
		})
		defer e.Close()
		e.mustSucceed()
	})

	t.Run("unrelated CA", func(t *testing.T) {
		e := newTLSEnv(t, envOptions{
			serverCertPEM: serverCert,
			serverKeyPEM:  serverKey,
			httpsConf: map[string]interface{}{
				"RSHost":   backendHost,
				"RSCAList": []string{wrongCAPath},
			},
			files: map[string][]byte{wrongCAPath: caPEM(t, other)},
		})
		defer e.Close()
		e.mustFail()
	})
}

// TestTC06_MutualTLS verifies BFE presents its client certificate when
// BFECertFile/BFEKeyFile are configured; without them the mTLS backend
// rejects the handshake.
func TestTC06_MutualTLS(t *testing.T) {
	ca := common.NewTestCA(t, "sc38-mtls-ca")
	serverCert, serverKey := ca.IssueServerCert(t, []string{backendHost}, nil)
	clientCertPEM, clientKeyPEM := ca.IssueClientCert(t, "bfe-client")

	t.Run("client cert configured", func(t *testing.T) {
		cns := &recorder{}
		e := newTLSEnv(t, envOptions{
			serverCertPEM: serverCert,
			serverKeyPEM:  serverKey,
			tlsOpts: common.TLSOptions{
				ClientCAs:          ca.Pool(),
				ClientCertRecorder: cns.add,
			},
			httpsConf: map[string]interface{}{
				"RSHost":      backendHost,
				"RSCAList":    []string{caPath},
				"BFECertFile": clientCert,
				"BFEKeyFile":  clientKey,
			},
			files: map[string][]byte{
				caPath:     caPEM(t, ca),
				clientCert: clientCertPEM,
				clientKey:  clientKeyPEM,
			},
		})
		defer e.Close()

		e.mustSucceed()
		if got := cns.all(); len(got) == 0 || got[0] != "bfe-client" {
			t.Fatalf("expected the backend to see client cert CN bfe-client, got %v", got)
		}
	})

	t.Run("client cert absent", func(t *testing.T) {
		e := newTLSEnv(t, envOptions{
			serverCertPEM: serverCert,
			serverKeyPEM:  serverKey,
			tlsOpts:       common.TLSOptions{ClientCAs: ca.Pool()},
			httpsConf: map[string]interface{}{
				"RSHost":   backendHost,
				"RSCAList": []string{caPath},
			},
			files: map[string][]byte{caPath: caPEM(t, ca)},
		})
		defer e.Close()
		e.mustFail()
	})
}

// TestTC07_TLSFailureBaseline pins the TLS failure handling: the request fails
// fast, the backend never receives business traffic, and the gateway keeps
// serving subsequent requests.
func TestTC07_TLSFailureBaseline(t *testing.T) {
	ca := common.NewTestCA(t, "sc38-ca")
	other := common.NewTestCA(t, "sc38-other")
	serverCert, serverKey := ca.IssueServerCert(t, []string{backendHost}, nil)

	e := newTLSEnv(t, envOptions{
		serverCertPEM: serverCert,
		serverKeyPEM:  serverKey,
		httpsConf: map[string]interface{}{
			"RSHost":   backendHost,
			"RSCAList": []string{wrongCAPath},
		},
		files: map[string][]byte{wrongCAPath: caPEM(t, other)},
	})
	defer e.Close()

	start := time.Now()
	e.mustFail()
	e.mustFail()
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("TLS failures should fail fast, took %v", elapsed)
	}
	if hits := e.backend.Hits(); hits != 0 {
		t.Fatalf("backend handler must not be reached on TLS failure, hits=%d", hits)
	}
}

// caPEM returns the CA certificate PEM.
func caPEM(t *testing.T, ca *common.TestCA) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := ca.WriteCAPEM(path); err != nil {
		t.Fatalf("write CA PEM failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read CA PEM failed: %v", err)
	}
	return data
}
