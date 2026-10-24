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

package sc33

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost = "sticky.example.org"
	apiPath = "/v1/chat/completions"

	clusterSticky = "cluster_sticky"

	maskMain    = "mainmask"
	maskStandby = "standbymask"

	cookieKey = "bfe_ssbl"
)

var defaultBody = []byte(`{"model":"gpt-4"}`)
var defaultResponse = `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`

type stickyEnv struct {
	t          *testing.T
	processEnv *common.ProcessEnv
	backendA   *common.MockBackend
	backendB   *common.MockBackend
	redis      *common.RedisServer

	confDir     string
	logDir      string
	bfePort     int
	monitorPort int
	stopBFE     func()
}

// stickyOptions controls the generated module conf/data.
type stickyOptions struct {
	// cacheType is "local" (default) or "redis".
	cacheType string
	// rule is the single product rule written for ai_product. Ignored when
	// data is set.
	rule map[string]interface{}
	// data, when non-nil, is written verbatim as session_sticky.data.
	data map[string]interface{}
}

func newStickyEnv(t *testing.T, opts stickyOptions) *stickyEnv {
	e := &stickyEnv{t: t}

	e.backendA = common.NewMockBackend(clusterSticky, http.StatusOK, defaultResponse)
	e.backendB = common.NewMockBackend(clusterSticky, http.StatusOK, defaultResponse)
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		BackendGroups: map[string][]*common.MockBackend{
			clusterSticky: {e.backendA, e.backendB},
		},
	}
	if opts.cacheType == "redis" {
		builder.RedisAddr = e.redis.Addr()
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	if opts.cacheType == "redis" {
		confPath := filepath.Join(e.confDir, "mod_session_sticky", "mod_session_sticky.conf")
		if err := common.RewriteConfLine(confPath, "CacheType", "redis"); err != nil {
			t.Fatalf("set CacheType=redis failed: %v", err)
		}
	}

	data := opts.data
	if data == nil {
		data = map[string]interface{}{
			"Version": "v1",
			"Config": map[string][]map[string]interface{}{
				"ai_product": {opts.rule},
			},
		}
	}
	e.writeData(data)

	e.startBFE()
	return e
}

func (e *stickyEnv) startBFE() {
	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
}

func (e *stickyEnv) restartBFE() {
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	e.startBFE()
}

func (e *stickyEnv) writeData(data map[string]interface{}) {
	e.t.Helper()
	path := filepath.Join(e.confDir, "mod_session_sticky", "session_sticky.data")
	if err := common.WriteJSONFile(path, data); err != nil {
		e.t.Fatalf("write session_sticky.data failed: %v", err)
	}
}

// getWithRetry polls url until the monitor web server responds. The monitor
// listener starts right after the HTTP listener (which is all StartBFE waits
// for), so on a slow/loaded machine the first reload request can hit
// connection refused (observed on WSL for TC07).
func getWithRetry(url string) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < 50; attempt++ {
		if attempt > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		resp, err := http.Get(url)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (e *stickyEnv) reloadModule() {
	e.t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/reload/mod_session_sticky", e.monitorPort)
	resp, err := getWithRetry(url)
	if err != nil {
		e.t.Fatalf("reload mod_session_sticky failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("reload mod_session_sticky status=%d body=%s", resp.StatusCode, body)
	}
}

func (e *stickyEnv) reloadGslbData() {
	e.t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/reload/gslb_data_conf", e.monitorPort)
	resp, err := getWithRetry(url)
	if err != nil {
		e.t.Fatalf("reload gslb_data_conf failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("reload gslb_data_conf status=%d body=%s", resp.StatusCode, body)
	}
}

func (e *stickyEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	if e.backendA != nil {
		e.backendA.Close()
	}
	if e.backendB != nil {
		e.backendB.Close()
	}
	if e.redis != nil {
		e.redis.Close()
	}
}

func (e *stickyEnv) logBFEException() {
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

// send issues one request. cookie may be nil; headers are added verbatim.
func (e *stickyEnv) send(path string, headers map[string]string, cookie *http.Cookie, body []byte) (*http.Response, string, error) {
	if path == "" {
		path = apiPath
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, path)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}

	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return resp, string(respBody), nil
}

func (e *stickyEnv) mustStatus(resp *http.Response, body string, err error, want int) {
	e.t.Helper()
	if err != nil {
		e.t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != want {
		e.logBFEException()
		e.t.Fatalf("expected %d, got %d, body: %s", want, resp.StatusCode, body)
	}
}

// setCookie returns the Set-Cookie header for name, or "" when absent.
func setCookie(resp *http.Response, name string) string {
	for _, raw := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(raw, name+"=") {
			return raw
		}
	}
	return ""
}

// cookieValue returns the cookie value for name, or "".
func cookieValue(resp *http.Response, name string) string {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func encodeCookieValue(mask, payload string) string {
	maskBytes := []byte(mask)
	data := make([]byte, len(payload))
	for i := 0; i < len(payload); i++ {
		data[i] = payload[i] ^ maskBytes[i%len(maskBytes)]
	}
	return base64.RawStdEncoding.EncodeToString(data)
}

func decodeCookieValue(t *testing.T, mask, value string) string {
	t.Helper()
	raw, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode cookie base64 failed: %v", err)
	}
	maskBytes := []byte(mask)
	data := make([]byte, len(raw))
	for i := range raw {
		data[i] = raw[i] ^ maskBytes[i%len(maskBytes)]
	}
	return string(data)
}

func backendCookie(t *testing.T, mask string, backend *common.MockBackend) string {
	t.Helper()
	host, port := backend.HostPort()
	payload := fmt.Sprintf(`{"ip":%q,"port":%d,"subcluster":"sub_sub"}`, host, port)
	return encodeCookieValue(mask, payload)
}

func cookiePort(t *testing.T, mask, value string) int {
	t.Helper()
	var decoded struct {
		IP   string `json:"ip"`
		Port int    `json:"port"`
		Sub  string `json:"subcluster"`
	}
	if err := json.Unmarshal([]byte(decodeCookieValue(t, mask, value)), &decoded); err != nil {
		t.Fatalf("decode cookie JSON failed: %v", err)
	}
	if decoded.IP == "" || decoded.Port == 0 || decoded.Sub == "" {
		t.Fatalf("cookie JSON missing fields: %+v", decoded)
	}
	return decoded.Port
}

func cookieRule(extra map[string]interface{}) map[string]interface{} {
	rule := map[string]interface{}{
		"Cond":            "default_t()",
		"Type":            "Cookie",
		"CookieKey":       cookieKey,
		"MaskCode":        maskMain,
		"StandbyMaskCode": maskStandby,
		"MaxAge":          3600,
		"RenewWindow":     1800,
	}
	for k, v := range extra {
		rule[k] = v
	}
	return rule
}

func stickyRule(extra map[string]interface{}) map[string]interface{} {
	rule := map[string]interface{}{
		"Cond":      "default_t()",
		"Type":      "Sticky",
		"CookieKey": cookieKey,
	}
	for k, v := range extra {
		rule[k] = v
	}
	return rule
}

// boundBackend returns the backend whose port matches the cookie value.
func (e *stickyEnv) boundBackend(t *testing.T, mask, value string) *common.MockBackend {
	t.Helper()
	_, portA := e.backendA.HostPort()
	if cookiePort(t, mask, value) == portA {
		return e.backendA
	}
	return e.backendB
}

func (e *stickyEnv) otherBackend(backend *common.MockBackend) *common.MockBackend {
	if backend == e.backendA {
		return e.backendB
	}
	return e.backendA
}

// TestTC01_CookieModeInjectsAndBinds verifies the first cookie-mode request
// binds a backend and returns an encrypted backend cookie.
func TestTC01_CookieModeInjectsAndBinds(t *testing.T) {
	e := newStickyEnv(t, stickyOptions{rule: cookieRule(nil)})
	defer e.Close()

	resp, body, err := e.send("", nil, nil, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)

	raw := setCookie(resp, cookieKey)
	if raw == "" {
		t.Fatalf("expected Set-Cookie %s, got %v", cookieKey, resp.Header.Values("Set-Cookie"))
	}
	value := cookieValue(resp, cookieKey)
	if strings.Contains(value, "127.0.0.1") {
		t.Fatalf("cookie value must be masked, got %q", value)
	}
	bound := e.boundBackend(t, maskMain, value)
	if bound.Hits() != 1 {
		t.Fatalf("expected the bound backend to serve 1 request, got %d", bound.Hits())
	}
	if other := e.otherBackend(bound); other.Hits() != 0 {
		t.Fatalf("expected the other backend idle, got %d", other.Hits())
	}
}

// TestTC02_CookieKeepsSession verifies subsequent requests carrying the cookie
// always hit the same backend and do not re-issue the cookie.
func TestTC02_CookieKeepsSession(t *testing.T) {
	e := newStickyEnv(t, stickyOptions{rule: cookieRule(nil)})
	defer e.Close()

	resp, body, err := e.send("", nil, nil, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	value := cookieValue(resp, cookieKey)
	if value == "" {
		t.Fatalf("expected an initial cookie")
	}
	bound := e.boundBackend(t, maskMain, value)

	for i := 0; i < 3; i++ {
		resp, body, err = e.send("", nil, &http.Cookie{Name: cookieKey, Value: value}, defaultBody)
		e.mustStatus(resp, body, err, http.StatusOK)
		if raw := setCookie(resp, cookieKey); raw != "" {
			t.Fatalf("request %d should not re-issue the cookie, got %q", i+2, raw)
		}
	}

	if bound.Hits() != 4 {
		t.Fatalf("expected the bound backend to serve 4 requests, got %d", bound.Hits())
	}
	if other := e.otherBackend(bound); other.Hits() != 0 {
		t.Fatalf("expected the other backend idle, got %d", other.Hits())
	}
}

// TestTC03_CookieAttributes verifies MaxAge/Domain/Path/Secure/HttpOnly are
// emitted as configured.
func TestTC03_CookieAttributes(t *testing.T) {
	e := newStickyEnv(t, stickyOptions{rule: cookieRule(map[string]interface{}{
		"CookieKey":   "sess",
		"Domain":      ".example.org",
		"Path":        "/",
		"MaxAge":      600,
		"RenewWindow": 600,
		"Secure":      true,
		"HttpOnly":    true,
	})})
	defer e.Close()

	resp, body, err := e.send("", nil, nil, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)

	raw := setCookie(resp, "sess")
	if raw == "" {
		t.Fatalf("expected Set-Cookie sess, got %v", resp.Header.Values("Set-Cookie"))
	}
	// net/http serializes Domain without the leading dot (browsers ignore it),
	// so accept the canonical form.
	for _, want := range []string{"Domain=example.org", "Path=/", "Max-Age=600", "Secure", "HttpOnly"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("Set-Cookie %q missing %q", raw, want)
		}
	}
}

// TestTC04_TamperedCookieFallsBack verifies a corrupted cookie is ignored,
// the request is balanced normally and a fresh cookie is issued.
func TestTC04_TamperedCookieFallsBack(t *testing.T) {
	e := newStickyEnv(t, stickyOptions{rule: cookieRule(nil)})
	defer e.Close()

	resp, body, err := e.send("", nil, &http.Cookie{Name: cookieKey, Value: "!!!not-base64!!!"}, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)

	if total := e.backendA.Hits() + e.backendB.Hits(); total != 1 {
		t.Fatalf("expected exactly 1 backend hit, got %d", total)
	}
	if raw := setCookie(resp, cookieKey); raw == "" {
		t.Fatalf("expected a fresh cookie after a corrupted one")
	}
}

// TestTC05_StandbyMaskCompatibility verifies a cookie signed with the standby
// mask is accepted (mask rotation compatibility).
func TestTC05_StandbyMaskCompatibility(t *testing.T) {
	e := newStickyEnv(t, stickyOptions{rule: cookieRule(map[string]interface{}{
		"MaskCode":        "newmask",
		"StandbyMaskCode": "oldmask",
	})})
	defer e.Close()

	value := backendCookie(t, "oldmask", e.backendA)
	resp, body, err := e.send("", nil, &http.Cookie{Name: cookieKey, Value: value}, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)

	if e.backendA.Hits() != 1 {
		t.Fatalf("expected the cookie (standby mask) to pin backend A, hits=%d", e.backendA.Hits())
	}
	if e.backendB.Hits() != 0 {
		t.Fatalf("expected backend B idle, hits=%d", e.backendB.Hits())
	}
}

// TestTC06_RenewWindowResignsCookie verifies the cookie is re-issued once the
// renewal window is reached and not before.
func TestTC06_RenewWindowResignsCookie(t *testing.T) {
	e := newStickyEnv(t, stickyOptions{rule: cookieRule(nil)})
	defer e.Close()

	host, port := e.backendA.HostPort()

	// Expired renewal window: must re-issue.
	renewDue := fmt.Sprintf(`{"ip":%q,"port":%d,"subcluster":"sub_sub","renewtime":%d}`,
		host, port, time.Now().Unix()-1)
	value := encodeCookieValue(maskMain, renewDue)
	resp, body, err := e.send("", nil, &http.Cookie{Name: cookieKey, Value: value}, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if raw := setCookie(resp, cookieKey); raw == "" {
		t.Fatalf("expected a renewed cookie once RenewTime is reached")
	}

	// Far-future renewal window: must not re-issue.
	renewLater := fmt.Sprintf(`{"ip":%q,"port":%d,"subcluster":"sub_sub","renewtime":%d}`,
		host, port, time.Now().Unix()+3600)
	value = encodeCookieValue(maskMain, renewLater)
	resp, body, err = e.send("", nil, &http.Cookie{Name: cookieKey, Value: value}, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if raw := setCookie(resp, cookieKey); raw != "" {
		t.Fatalf("cookie should not be renewed before the window, got %q", raw)
	}
}

// TestTC07_UnavailableStickyBackendResigns verifies a cookie pointing to a
// backend that can no longer be selected is re-balanced and re-signed.
func TestTC07_UnavailableStickyBackendResigns(t *testing.T) {
	e := newStickyEnv(t, stickyOptions{rule: cookieRule(nil)})
	defer e.Close()

	value := backendCookie(t, maskMain, e.backendA)
	if err := e.zeroBackendWeight(e.backendA); err != nil {
		t.Fatalf("zero backend weight failed: %v", err)
	}
	e.reloadGslbData()

	resp, body, err := e.send("", nil, &http.Cookie{Name: cookieKey, Value: value}, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)

	if e.backendA.Hits() != 0 {
		t.Fatalf("backend A must be unselectable, hits=%d", e.backendA.Hits())
	}
	if e.backendB.Hits() != 1 {
		t.Fatalf("expected backend B to serve the request, hits=%d", e.backendB.Hits())
	}
	newValue := cookieValue(resp, cookieKey)
	if newValue == "" {
		t.Fatalf("expected a re-signed cookie")
	}
	if got := cookiePort(t, maskMain, newValue); got != portOf(t, e.backendB) {
		t.Fatalf("re-signed cookie should point to backend B (%d), got %d", portOf(t, e.backendB), got)
	}
}

// zeroBackendWeight rewrites cluster_table.data so the given backend has
// weight 0 (no longer selectable).
func (e *stickyEnv) zeroBackendWeight(backend *common.MockBackend) error {
	path := filepath.Join(e.confDir, "cluster_conf", "cluster_table.data")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var table map[string]interface{}
	if err := json.Unmarshal(data, &table); err != nil {
		return err
	}
	host, port := backend.HostPort()
	config := table["Config"].(map[string]interface{})
	cluster := config[clusterSticky].(map[string]interface{})
	for sub, rawEntries := range cluster {
		entries, ok := rawEntries.([]interface{})
		if !ok {
			continue
		}
		updated := false
		for _, raw := range entries {
			entry := raw.(map[string]interface{})
			if entry["Addr"] == host && int(entry["Port"].(float64)) == port {
				entry["Weight"] = 0
				updated = true
			}
		}
		if updated {
			cluster[sub] = entries
		}
	}
	return common.WriteJSONFile(path, table)
}

func portOf(t *testing.T, backend *common.MockBackend) int {
	t.Helper()
	_, port := backend.HostPort()
	return port
}

// seedSticky writes a backend mapping into redis (redis cache mode).
func (e *stickyEnv) seedSticky(id string, backend *common.MockBackend) {
	e.t.Helper()
	host, port := backend.HostPort()
	payload := fmt.Sprintf(`{"addr":%q,"port":%d,"sub_cluster":"sub_sub","created_at":%d}`,
		host, port, time.Now().Unix())
	e.redis.Set("bfe:stickyid:"+id, payload)
}

// TestTC08_StickySourcePriority verifies the decode priority
// Cookie > Header > URIParam > JSON request field using redis-seeded mappings.
func TestTC08_StickySourcePriority(t *testing.T) {
	e := newStickyEnv(t, stickyOptions{
		cacheType: "redis",
		rule: stickyRule(map[string]interface{}{
			"Header":             "X-Sticky-Id",
			"URIParam":           "sticky_id",
			"StickyRequestField": "previous_response_id",
		}),
	})
	defer e.Close()

	e.seedSticky("sid-cookie", e.backendA)
	e.seedSticky("sid-header", e.backendB)
	e.seedSticky("sid-uri", e.backendA)
	e.seedSticky("sid-json", e.backendB)

	body := []byte(`{"model":"gpt-4","previous_response_id":"sid-json"}`)

	// All four sources conflict: the cookie wins.
	resp, respBody, err := e.send("/v1/chat/completions?sticky_id=sid-uri",
		map[string]string{"X-Sticky-Id": "sid-header"},
		&http.Cookie{Name: cookieKey, Value: "sid-cookie"}, body)
	e.mustStatus(resp, respBody, err, http.StatusOK)
	if e.backendA.Hits() != 1 || e.backendB.Hits() != 0 {
		t.Fatalf("cookie must win: A=%d B=%d", e.backendA.Hits(), e.backendB.Hits())
	}

	// No cookie: the header wins over URI param and JSON field.
	resp, respBody, err = e.send("/v1/chat/completions?sticky_id=sid-uri",
		map[string]string{"X-Sticky-Id": "sid-header"}, nil, body)
	e.mustStatus(resp, respBody, err, http.StatusOK)
	if e.backendA.Hits() != 1 || e.backendB.Hits() != 1 {
		t.Fatalf("header must win: A=%d B=%d", e.backendA.Hits(), e.backendB.Hits())
	}

	// No cookie/header: the URI param wins over the JSON field.
	resp, respBody, err = e.send("/v1/chat/completions?sticky_id=sid-uri", nil, nil, body)
	e.mustStatus(resp, respBody, err, http.StatusOK)
	if e.backendA.Hits() != 2 || e.backendB.Hits() != 1 {
		t.Fatalf("uri param must win: A=%d B=%d", e.backendA.Hits(), e.backendB.Hits())
	}

	// Only the JSON field remains.
	resp, respBody, err = e.send("/v1/chat/completions", nil, nil, body)
	e.mustStatus(resp, respBody, err, http.StatusOK)
	if e.backendA.Hits() != 2 || e.backendB.Hits() != 2 {
		t.Fatalf("JSON request field must be honoured: A=%d B=%d", e.backendA.Hits(), e.backendB.Hits())
	}
}

// TestTC09_StickyJsonResponseField verifies the sticky id is captured from the
// JSON response body and reused by later requests.
func TestTC09_StickyJsonResponseField(t *testing.T) {
	e := newStickyEnv(t, stickyOptions{
		rule: stickyRule(map[string]interface{}{
			"Header":              "X-Sticky-Id",
			"StickyResponseField": "response_id",
		}),
	})
	defer e.Close()

	responseWithID := `{"response_id":"resp-9","choices":[]}`
	e.backendA.Body = responseWithID
	e.backendB.Body = responseWithID

	resp, body, err := e.send("", nil, nil, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if total := e.backendA.Hits() + e.backendB.Hits(); total != 1 {
		t.Fatalf("expected 1 backend hit, got %d", total)
	}
	bound := e.backendA
	if e.backendB.Hits() == 1 {
		bound = e.backendB
	}

	resp, body, err = e.send("", map[string]string{"X-Sticky-Id": "resp-9"}, nil, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if bound.Hits() != 2 {
		t.Fatalf("expected the JSON response id to pin the backend, hits=%d", bound.Hits())
	}
	if other := e.otherBackend(bound); other.Hits() != 0 {
		t.Fatalf("expected the other backend idle, hits=%d", other.Hits())
	}
}

// TestTC10_RedisCacheSurvivesRestart verifies the sticky mapping is shared via
// redis across a BFE restart and expires with ExpireSeconds.
func TestTC10_RedisCacheSurvivesRestart(t *testing.T) {
	e := newStickyEnv(t, stickyOptions{
		cacheType: "redis",
		rule: stickyRule(map[string]interface{}{
			"Header":              "X-Sticky-Id",
			"StickyResponseField": "response_id",
		}),
	})
	defer e.Close()

	responseWithID := `{"response_id":"resp-10","choices":[]}`
	e.backendA.Body = responseWithID
	e.backendB.Body = responseWithID

	resp, body, err := e.send("", nil, nil, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	bound := e.backendA
	if e.backendB.Hits() == 1 {
		bound = e.backendB
	}
	if got := e.redis.Get("bfe:stickyid:resp-10"); got == "" {
		t.Fatalf("expected the sticky mapping in redis, keys=%v", e.redis.Keys())
	}

	e.restartBFE()

	resp, body, err = e.send("", map[string]string{"X-Sticky-Id": "resp-10"}, nil, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if bound.Hits() != 2 {
		t.Fatalf("expected the redis mapping to survive the restart, bound hits=%d", bound.Hits())
	}

	// Expire the mapping (ExpireSeconds=60) and verify it is gone.
	e.redis.AdvanceTime(61 * time.Second)
	if e.redis.Exists("bfe:stickyid:resp-10") {
		t.Fatalf("expected the sticky mapping to expire, keys=%v", e.redis.Keys())
	}
	resp, body, err = e.send("", map[string]string{"X-Sticky-Id": "resp-10"}, nil, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
}

// TestTC11_RuleConditionIsolation verifies a rule only applies to matching
// paths: no cookie/cache is written for non-matching requests.
func TestTC11_RuleConditionIsolation(t *testing.T) {
	e := newStickyEnv(t, stickyOptions{rule: cookieRule(map[string]interface{}{
		"Cond": `req_path_prefix_in("/api", true)`,
	})})
	defer e.Close()

	resp, body, err := e.send("", nil, nil, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if raw := setCookie(resp, cookieKey); raw != "" {
		t.Fatalf("rule must not apply to /v1/..., got Set-Cookie %q", raw)
	}

	resp, body, err = e.send("/api/chat", nil, nil, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if raw := setCookie(resp, cookieKey); raw == "" {
		t.Fatalf("expected a cookie for the matching path, got %v", resp.Header.Values("Set-Cookie"))
	}
}

// TestTC12_RuleHotReload verifies /reload/mod_session_sticky takes effect
// without a restart.
func TestTC12_RuleHotReload(t *testing.T) {
	e := newStickyEnv(t, stickyOptions{rule: cookieRule(nil)})
	defer e.Close()

	resp, body, err := e.send("", nil, nil, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if raw := setCookie(resp, cookieKey); raw == "" {
		t.Fatalf("expected a cookie before the reload")
	}

	// Reload with an empty rule list: the module must stop issuing cookies.
	e.writeData(map[string]interface{}{
		"Version": "v2",
		"Config": map[string][]map[string]interface{}{
			"ai_product": {},
		},
	})
	e.reloadModule()

	resp, body, err = e.send("", nil, nil, defaultBody)
	e.mustStatus(resp, body, err, http.StatusOK)
	if raw := setCookie(resp, cookieKey); raw != "" {
		t.Fatalf("rule should be gone after reload, got Set-Cookie %q", raw)
	}
}

// TestTC13_InvalidCacheTypeFailsFast verifies a bogus CacheType is rejected at
// startup.
func TestTC13_InvalidCacheTypeFailsFast(t *testing.T) {
	e := &stickyEnv{t: t}
	e.backendA = common.NewMockBackend(clusterSticky, http.StatusOK, defaultResponse)
	e.backendB = common.NewMockBackend(clusterSticky, http.StatusOK, defaultResponse)
	defer e.backendA.Close()
	defer e.backendB.Close()
	e.redis = common.NewRedisServer(t)
	defer e.redis.Close()

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()
	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		BackendGroups: map[string][]*common.MockBackend{
			clusterSticky: {e.backendA, e.backendB},
		},
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}
	e.writeData(map[string]interface{}{
		"Version": "v1",
		"Config": map[string][]map[string]interface{}{
			"ai_product": {},
		},
	})
	confPath := filepath.Join(e.confDir, "mod_session_sticky", "mod_session_sticky.conf")
	if err := common.RewriteConfLine(confPath, "CacheType", "bogus"); err != nil {
		t.Fatalf("rewrite CacheType failed: %v", err)
	}

	if _, _, stop, err := e.processEnv.StartBFERaw(e.confDir, e.logDir); err == nil {
		if stop != nil {
			stop()
		}
		t.Fatalf("expected BFE to fail fast on CacheType=bogus")
	}
}
