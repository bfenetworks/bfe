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

package sc41

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/bfenetworks/bfe-access-pb/b2log"
	bfe_access_pb "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost = "log.example.org"
	apiPath = "/v1/chat/completions"

	clusterLog = "cluster_log"

	tlsServerName = "example.org"
)

var okBody = `{"id":"chatcmpl-sc41","choices":[]}`

type logEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backend    *common.MockBackend

	confDir     string
	logDir      string
	bfePort     int
	httpsPort   int
	monitorPort int
	stopBFE     func()
}

// newLogEnv starts BFE in plain-forwarding mode with mod_access_pb3 loaded.
// requireClientCert makes the HTTPS listener demand a client certificate;
// enableProxy makes the listeners expect a PROXY protocol header (used to
// exercise the VIP -> product session tag).
func newLogEnv(t *testing.T, requireClientCert, enableProxy bool) *logEnv {
	e := &logEnv{t: t}

	e.backend = common.NewMockBackend(clusterLog, http.StatusOK, okBody)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends: map[string]*common.MockBackend{
			clusterLog: e.backend,
		},
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	if requireClientCert {
		if err := e.enableClientAuth(); err != nil {
			t.Fatalf("enable client auth failed: %v", err)
		}
	}

	if err := common.RewriteConfLine(filepath.Join(e.confDir, "bfe.conf"),
		"EnableAiGateway", "false"); err != nil {
		t.Fatalf("disable AI gateway failed: %v", err)
	}
	if enableProxy {
		if err := common.RewriteConfLine(filepath.Join(e.confDir, "bfe.conf"),
			"Layer4LoadBalancer", `"PROXY"`); err != nil {
			t.Fatalf("enable PROXY protocol failed: %v", err)
		}
	}

	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
	e.httpsPort = e.readConfInt("httpsPort")
	return e
}

// enableClientAuth flips ClientAuth to true in tls_rule_conf.data.
func (e *logEnv) enableClientAuth() error {
	path := filepath.Join(e.confDir, "tls_conf", "tls_rule_conf.data")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var conf map[string]interface{}
	if err := json.Unmarshal(data, &conf); err != nil {
		return err
	}
	configs, _ := conf["Config"].(map[string]interface{})
	changed := false
	for _, raw := range configs {
		product, _ := raw.(map[string]interface{})
		if _, ok := product["ClientAuth"]; ok {
			product["ClientAuth"] = true
			changed = true
		}
	}
	if !changed {
		return fmt.Errorf("no ClientAuth field found in %s", path)
	}
	return common.WriteJSONFile(path, conf)
}

// readConfInt reads an integer value from bfe.conf (e.g. httpsPort).
func (e *logEnv) readConfInt(key string) int {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(e.confDir, "bfe.conf"))
	if err != nil {
		e.t.Fatalf("read bfe.conf failed: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		idx := strings.Index(trimmed, "=")
		if idx < 0 || !strings.EqualFold(strings.TrimSpace(trimmed[:idx]), key) {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(trimmed[idx+1:]))
		if err != nil {
			e.t.Fatalf("parse %s failed: %v", key, err)
		}
		return v
	}
	e.t.Fatalf("%s not found in bfe.conf", key)
	return 0
}

func (e *logEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	if e.backend != nil {
		e.backend.Close()
	}
}

func (e *logEnv) logBFEException() {
	data, err := os.ReadFile(filepath.Join(e.logDir, "exception.log"))
	if err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}

	logPath := filepath.Join(e.logDir, "pb_access3.log")
	if _, err := os.Stat(logPath); err != nil {
		e.t.Logf("access log %s missing: %v", logPath, err)
	}
}

func (e *logEnv) httpClient() *http.Client {
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
}

func (e *logEnv) doHTTP() (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	resp, err := e.httpClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp, string(body), err
}

func (e *logEnv) doHTTPS() (*http.Response, string, error) {
	url := fmt.Sprintf("https://127.0.0.1:%d%s", e.httpsPort, apiPath)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         tlsServerName,
			},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp, string(body), err
}

// doProxyHTTP sends a PROXY protocol v1 header followed by an HTTP/1.1
// request on the same TCP connection. The PROXY destination address becomes
// the session VIP, which is what maps to a product via vip_rule.data.
func (e *logEnv) doProxyHTTP() (int, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", e.bfePort))
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	header := fmt.Sprintf("PROXY TCP4 203.0.113.9 127.0.0.1 45678 %d\r\n", e.bfePort)
	request := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", apiPath, apiHost)
	if _, err := conn.Write([]byte(header + request)); err != nil {
		return 0, err
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// stopAndParseLogs stops BFE (flushing the access log) and returns all
// decoded BfeLog records.
func (e *logEnv) stopAndParseLogs() []*bfe_access_pb.BfeLog {
	e.t.Helper()
	time.Sleep(500 * time.Millisecond)
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	logs, err := common.ParseBfeLogsAfterStop(e.logDir)
	if err != nil {
		e.t.Fatalf("parse access log failed: %v", err)
	}
	return logs
}

func sessionLogs(logs []*bfe_access_pb.BfeLog) []*bfe_access_pb.BfeLog {
	var out []*bfe_access_pb.BfeLog
	for _, l := range logs {
		if l.GetLogType() == bfe_access_pb.BfeLogType_Session && l.SessionLog != nil {
			out = append(out, l)
		}
	}
	return out
}

// completedSessions filters sessions that served at least one request. It
// drops the TCP readiness probe connection created by the harness, which
// produces a CLIENT_CLOSE session with req_num=0.
func completedSessions(logs []*bfe_access_pb.BfeLog) []*bfe_access_pb.BfeLog {
	var out []*bfe_access_pb.BfeLog
	for _, l := range sessionLogs(logs) {
		if l.SessionLog.GetReqNum() > 0 {
			out = append(out, l)
		}
	}
	return out
}

func requestLogs(logs []*bfe_access_pb.BfeLog) []*bfe_access_pb.BfeLog {
	var out []*bfe_access_pb.BfeLog
	for _, l := range logs {
		if l.GetLogType() == bfe_access_pb.BfeLogType_Request && l.RequestLog != nil {
			out = append(out, l)
		}
	}
	return out
}

// TestTC01_SessionLogBasicFields verifies the session-level record fields.
func TestTC01_SessionLogBasicFields(t *testing.T) {
	t.Run("plain http without vip", func(t *testing.T) {
		e := newLogEnv(t, false, false)

		if resp, body, err := e.doHTTP(); err != nil || resp.StatusCode != http.StatusOK {
			if err != nil {
				t.Fatalf("send request failed: %v", err)
			}
			e.logBFEException()
			t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
		}

		logs := e.stopAndParseLogs()
		sessions := completedSessions(logs)
		if len(sessions) != 1 {
			t.Fatalf("expected exactly 1 completed session, got %d (all=%d)", len(sessions), len(logs))
		}
		logEntry := sessions[0]
		// Without a VIP (Layer4LoadBalancer unset) session.Product is empty,
		// so the tag falls back to the "bfe" product placeholder.
		if got := logEntry.GetLogTag(); got != "session_bfe" {
			t.Fatalf("expected log_tag session_bfe, got %q", got)
		}
		if got := logEntry.GetLogid(); got == 0 {
			t.Fatalf("expected a non-zero session logid")
		}
		if got := logEntry.GetTimestamp(); got == 0 {
			t.Fatalf("expected a non-zero timestamp")
		}
		ses := logEntry.SessionLog
		if got := ses.GetProto(); got != "http" {
			t.Fatalf("expected proto http, got %q", got)
		}
		if got := ses.GetReqNum(); got < 1 {
			t.Fatalf("expected at least 1 request in the session, got %d", got)
		}
	})

	t.Run("proxy vip maps product", func(t *testing.T) {
		e := newLogEnv(t, false, true)

		status, err := e.doProxyHTTP()
		if err != nil {
			t.Fatalf("send PROXY request failed: %v", err)
		}
		if status != http.StatusOK {
			e.logBFEException()
			t.Fatalf("expected 200, got %d", status)
		}

		logs := e.stopAndParseLogs()
		sessions := completedSessions(logs)
		if len(sessions) != 1 {
			t.Fatalf("expected exactly 1 completed session, got %d", len(sessions))
		}
		logEntry := sessions[0]
		if got := logEntry.GetLogTag(); got != "session_ai_vip" {
			t.Fatalf("expected log_tag session_ai_vip, got %q", got)
		}
		if got := logEntry.SessionLog.GetProduct(); got != "ai_vip" {
			t.Fatalf("expected session product ai_vip, got %q", got)
		}
	})
}

// TestTC02_RequestSessionCorrelation verifies the request log's session_id
// matches the session log's logid while the request log keeps its own logid.
func TestTC02_RequestSessionCorrelation(t *testing.T) {
	e := newLogEnv(t, false, false)

	if resp, body, err := e.doHTTP(); err != nil || resp.StatusCode != http.StatusOK {
		if err != nil {
			t.Fatalf("send request failed: %v", err)
		}
		e.logBFEException()
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
	}

	logs := e.stopAndParseLogs()
	sessions := completedSessions(logs)
	requests := requestLogs(logs)
	if len(sessions) != 1 || len(requests) != 1 {
		t.Fatalf("expected 1 completed session and 1 request log, got sessions=%d requests=%d", len(sessions), len(requests))
	}

	sessionID := sessions[0].GetLogid()
	reqSessionID := requests[0].RequestLog.GetSessionId()
	if reqSessionID == 0 {
		t.Fatalf("expected a non-zero request session_id")
	}
	if reqSessionID != sessionID {
		t.Fatalf("request session_id=%d must equal session logid=%d", reqSessionID, sessionID)
	}
	if reqLogID := requests[0].GetLogid(); reqLogID == 0 {
		t.Fatalf("expected a non-zero request logid")
	}
}

// TestTC03_TlsSessionInfo verifies TLS version/cipher are recorded on the
// session log for HTTPS connections. The protobuf has no SNI field, so the
// assertion is limited to the fields the module actually populates.
func TestTC03_TlsSessionInfo(t *testing.T) {
	e := newLogEnv(t, false, false)

	if resp, body, err := e.doHTTPS(); err != nil || resp.StatusCode != http.StatusOK {
		if err != nil {
			t.Fatalf("send HTTPS request failed: %v", err)
		}
		e.logBFEException()
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, body)
	}

	logs := e.stopAndParseLogs()
	sessions := completedSessions(logs)
	if len(sessions) != 1 {
		t.Fatalf("expected exactly 1 completed session, got %d", len(sessions))
	}
	ses := sessions[0].SessionLog
	if got := ses.GetProto(); got != "https" {
		t.Fatalf("expected proto https, got %q", got)
	}
	if got := ses.GetTlsVersion(); got == 0 {
		t.Fatalf("expected a non-zero tls_version")
	}
	if got := ses.GetCipherSuite(); got == 0 {
		t.Fatalf("expected a non-zero cipher_suite")
	}
	if ses.HandshakeTime == nil {
		t.Fatalf("expected handshake_time to be set")
	}
}

// TestTC04_HandshakeFailureSession verifies a failed TLS handshake produces a
// session_err_* log with a non-empty error code and message.
func TestTC04_HandshakeFailureSession(t *testing.T) {
	e := newLogEnv(t, true, false)

	conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", e.httpsPort), &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         tlsServerName,
	})
	if err == nil {
		_ = conn.Close()
		t.Fatalf("expected the handshake to fail without a client certificate")
	}

	logs := e.stopAndParseLogs()
	sessions := sessionLogs(logs)
	if len(sessions) == 0 {
		t.Fatalf("expected a session log for the failed handshake, got 0 (all=%d)", len(logs))
	}
	logEntry := sessions[len(sessions)-1]
	if got := logEntry.GetLogTag(); got != "session_err_bfe" {
		t.Fatalf("expected log_tag session_err_bfe, got %q", got)
	}
	ses := logEntry.SessionLog
	if got := ses.GetErrCode(); got == "" {
		t.Fatalf("expected a non-empty err_code for the failed handshake")
	}
	if got := ses.GetErrMsg(); got == "" {
		t.Fatalf("expected a non-empty err_msg for the failed handshake")
	}
}

// TestTC05_B2logFraming verifies every log record is a well-formed b2log frame
// (magic/version/lengths) whose payload decodes as a BfeLog.
func TestTC05_B2logFraming(t *testing.T) {
	e := newLogEnv(t, false, false)

	if resp, _, err := e.doHTTP(); err != nil || resp.StatusCode != http.StatusOK {
		if err != nil {
			t.Fatalf("send HTTP request failed: %v", err)
		}
		t.Fatalf("HTTP request status = %d", resp.StatusCode)
	}
	if resp, _, err := e.doHTTPS(); err != nil || resp.StatusCode != http.StatusOK {
		if err != nil {
			t.Fatalf("send HTTPS request failed: %v", err)
		}
		t.Fatalf("HTTPS request status = %d", resp.StatusCode)
	}

	time.Sleep(500 * time.Millisecond)
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}

	path := filepath.Join(e.logDir, "pb_access3.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read access log failed: %v", err)
	}

	records := 0
	sessionRecords := 0
	requestRecords := 0
	rest := data
	for len(rest) > 0 {
		if len(rest) < b2log.HEADER_SIZE {
			t.Fatalf("trailing %d bytes are shorter than a b2log header", len(rest))
		}
		var header b2log.Header
		if err := binary.Read(bytes.NewReader(rest[:b2log.HEADER_SIZE]), binary.LittleEndian, &header); err != nil {
			t.Fatalf("decode b2log header failed: %v", err)
		}
		if header.MagicNumber != b2log.MAGIC_NUMBER {
			t.Fatalf("record %d: invalid magic number 0x%x", records, header.MagicNumber)
		}
		if header.Version != b2log.HEADER_VERSION {
			t.Fatalf("record %d: unexpected header version %d", records, header.Version)
		}
		// b2log does not compress records today: CompressLen must be 0 and the
		// payload length comes from UnCompressLen.
		if header.CompressLen != 0 {
			t.Fatalf("record %d: unexpected compressed record (compress_len=%d)",
				records, header.CompressLen)
		}
		if header.UnCompressLen == 0 || header.UnCompressLen > b2log.MAX_RECORD_LEN {
			t.Fatalf("record %d: invalid payload length %d", records, header.UnCompressLen)
		}
		end := b2log.HEADER_SIZE + int(header.UnCompressLen)
		if end > len(rest) {
			t.Fatalf("record %d: payload length %d exceeds remaining bytes %d",
				records, header.UnCompressLen, len(rest)-b2log.HEADER_SIZE)
		}
		bfeLog := new(bfe_access_pb.BfeLog)
		if err := proto.Unmarshal(rest[b2log.HEADER_SIZE:end], bfeLog); err != nil {
			t.Fatalf("record %d: payload is not a BfeLog: %v", records, err)
		}
		switch bfeLog.GetLogType() {
		case bfe_access_pb.BfeLogType_Session:
			sessionRecords++
		case bfe_access_pb.BfeLogType_Request:
			requestRecords++
		default:
			t.Fatalf("record %d: unexpected log type %v", records, bfeLog.GetLogType())
		}
		records++
		rest = rest[end:]
	}

	if records < 2 {
		t.Fatalf("expected at least 2 b2log records, got %d", records)
	}
	if sessionRecords == 0 || requestRecords == 0 {
		t.Fatalf("expected both session and request records, got session=%d request=%d",
			sessionRecords, requestRecords)
	}
	if len(rest) != 0 {
		t.Fatalf("expected no trailing bytes, got %d", len(rest))
	}
}
