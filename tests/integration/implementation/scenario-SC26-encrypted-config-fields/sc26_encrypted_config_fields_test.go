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

// Package sc26 verifies field-level encrypted config at rest (see
// 测试设计文档/scenario-SC26-下发配置字段级加密落盘): token_rule.data Tokens
// keys and cluster_conf.data AIConf.Keys[].Key carry enc$v1$ ciphertext,
// decrypted by BFE at load time via the bfe.conf [Security] keyring.
package sc26

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_util/crypto"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost = "enc.example.org"
	apiPath = "/v1/chat/completions"
	product = "ai_product"
	cluster = "cluster_enc"

	upstreamKey1 = "sk-upstream-k1-plain"
	upstreamKey2 = "sk-upstream-k2-plain"
)

var defaultBody = []byte(`{"model":"gpt-4"}`)

// test key material (fixed, non-secret)
var (
	keyA = testKeyMaterial(1)
	keyB = testKeyMaterial(101)
)

func testKeyMaterial(seed byte) []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = seed + byte(i)
	}
	return key
}

// encryptEnvelope produces an enc$v1$ field envelope, mirroring the control
// plane export encryption (BFE itself never encrypts).
func encryptEnvelope(t *testing.T, plaintext string, key []byte, keyID byte) string {
	t.Helper()

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("cipher.NewGCM: %v", err)
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	raw := append([]byte{keyID}, nonce...)
	raw = gcm.Seal(raw, nonce, []byte(plaintext), nil)
	return crypto.Marker + base64.StdEncoding.EncodeToString(raw)
}

// writeKeyring writes a TOML keyring file (shared format with the control
// plane) with 0600 permissions into dir and returns its path.
func writeKeyring(t *testing.T, dir string, keys map[byte][]byte) string {
	t.Helper()

	ids := make([]int, 0, len(keys))
	for id := range keys {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)

	var b strings.Builder
	fmt.Fprintf(&b, "ActiveKeyID = %d\n[Keys]\n", ids[len(ids)-1])
	for _, id := range ids {
		fmt.Fprintf(&b, "%d = \"%s\"\n", id, base64.StdEncoding.EncodeToString(keys[byte(id)]))
	}

	path := filepath.Join(dir, "export.keys")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir keyring dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		t.Fatalf("write keyring: %v", err)
	}
	return path
}

// setSecurityKeyFile points the [Security] KeyFile line of the generated
// bfe.conf at the given keyring file.
func setSecurityKeyFile(t *testing.T, confDir, keyFile string) {
	t.Helper()

	path := filepath.Join(confDir, "bfe.conf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bfe.conf: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	inSecurity := false
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSecurity = trimmed == "[Security]"
			continue
		}
		if inSecurity && (strings.HasPrefix(trimmed, "KeyFile") || strings.HasPrefix(trimmed, "#KeyFile")) {
			lines[i] = fmt.Sprintf("KeyFile = \"%s\"", keyFile)
			found = true
		}
	}
	if !found {
		t.Fatal("[Security] KeyFile line not found in bfe.conf template")
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatalf("write bfe.conf: %v", err)
	}
}

func plainToken(key, keyID string) common.TokenFile {
	return common.TokenFile{
		Key:            key,
		KeyId:          keyID,
		Enabled:        true,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
		QuotaPlans:     []string{},
	}
}

func tokenRuleData(tokens map[string]common.TokenFile) *common.TokenRuleData {
	return &common.TokenRuleData{
		Version: "20260720150000",
		Config: map[string][]common.TokenRule{
			product: {
				{Cond: "default_t()", Action: common.ActionFile{Cmd: "CHECK_TOKEN"}},
			},
		},
		QuotaPlans: map[string][]common.QuotaPlan{
			product: {
				{Id: "plan-unlimited", Unlimited: true, PassNoQuota: false, ExpiredTime: -1, Quota: 0},
			},
		},
		Tokens: map[string]map[string]common.TokenFile{product: tokens},
	}
}

func aiConfWithKeys(keys ...cluster_conf.AIKey) *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Keys:           keys,
		ModelProtocols: []string{"openai"},
	}
}

func defaultAIConf() *cluster_conf.AIConf {
	return aiConfWithKeys(cluster_conf.AIKey{Name: "k1", Key: upstreamKey1, Weight: 100})
}

// testEnv holds all resources for a single SC26 integration test.
type testEnv struct {
	t              *testing.T
	processEnv     *common.ProcessEnv
	backend        *common.MockBackend
	confDir        string
	logDir         string
	keyringPath    string // WorkDir/export.keys when a keyring is configured
	bfePort        int
	bfeMonitorPort int
	stopBFE        func()
	startErr       error // set when BFE refuses to start (fail-fast cases)
}

type envOpt struct {
	// keys is the keyring content; nil means no [Security] KeyFile at all.
	keys map[byte][]byte
	// tokenRule / aiConf are required (the template has neither file).
	tokenRule *common.TokenRuleData
	aiConf    *cluster_conf.AIConf
}

func newTestEnv(t *testing.T, opt envOpt) *testEnv {
	e := &testEnv{t: t}

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()

	e.backend = common.NewMockBackend(cluster, http.StatusOK, `{"ok":true}`)

	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends:      map[string]*common.MockBackend{cluster: e.backend},
		AIConfs:       map[string]*cluster_conf.AIConf{cluster: opt.aiConf},
		TokenRuleData: opt.tokenRule,
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	if opt.keys != nil {
		// keyring is mock material; production discipline (0600, outside the
		// conf dir) is enforced by deployment, not by BFE. ConfPathProc only
		// resolves single-segment relative paths reliably on Windows, so the
		// keyring lives at conf-root-relative keys/export.keys (0600).
		e.keyringPath = writeKeyring(t, filepath.Join(e.confDir, "keys"), opt.keys)
		setSecurityKeyFile(t, e.confDir, "keys/export.keys")
	}

	e.bfePort, e.bfeMonitorPort, e.stopBFE, e.startErr = e.processEnv.StartBFERaw(e.confDir, e.logDir)
	return e
}

// rotateKeyring overwrites the env keyring file in place (rotation).
func (e *testEnv) rotateKeyring(keys map[byte][]byte) {
	writeKeyring(e.t, filepath.Join(e.confDir, "keys"), keys)
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
	}
	if e.backend != nil {
		e.backend.Close()
	}
}

func (e *testEnv) mustStart() {
	if e.startErr != nil {
		e.logBFEFiles()
		e.t.Fatalf("bfe should start: %v", e.startErr)
	}
}

func (e *testEnv) mustNotStart() {
	if e.startErr == nil {
		e.t.Fatal("bfe should refuse to start, but it started")
	}
	e.t.Logf("bfe refused to start as expected: %v", e.startErr)
}

// logBFEFiles dumps the BFE log dir to the test log (startup diagnostics).
func (e *testEnv) logBFEFiles() {
	entries, err := os.ReadDir(e.logDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(e.logDir, entry.Name()))
		if err == nil && len(data) > 0 {
			e.t.Logf("bfe log %s:\n%s", entry.Name(), string(data))
		}
	}
}

// logsContain reports whether any file under the log dir contains s.
func (e *testEnv) logsContain(s string) bool {
	entries, err := os.ReadDir(e.logDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(e.logDir, entry.Name()))
		if err == nil && strings.Contains(string(data), s) {
			return true
		}
	}
	return false
}

func (e *testEnv) sendRequest(apiKey string) (int, string) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(defaultBody))
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		e.t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("read response: %v", err)
	}
	return resp.StatusCode, string(body)
}

// reload calls a BFE monitor reload endpoint and returns the error field of
// the JSON response (empty means success). When the error message contains
// Windows path backslashes the monitor response is not decodable JSON; the
// raw body is returned in that case.
func (e *testEnv) reload(name string) string {
	url := fmt.Sprintf("http://127.0.0.1:%d/reload/%s", e.bfeMonitorPort, name)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		e.t.Fatalf("call reload %s failed: %v", name, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("read reload rsp failed: %v", err)
	}
	rsp := &struct {
		Error string `json:"error"`
	}{}
	if err := json.Unmarshal(raw, rsp); err != nil {
		return string(raw)
	}
	return rsp.Error
}

// writeTokenRule regenerates mod_ai_token_auth/token_rule.data in the
// running BFE conf dir.
func (e *testEnv) writeTokenRule(data *common.TokenRuleData) {
	raw, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		e.t.Fatalf("marshal token rule: %v", err)
	}
	path := filepath.Join(e.confDir, "mod_ai_token_auth", "token_rule.data")
	if err := os.WriteFile(path, raw, 0644); err != nil {
		e.t.Fatalf("write token_rule.data: %v", err)
	}
}

// rewriteClusterConfKey replaces AIConf.Keys[0].Key of the generated
// cluster_conf.data (the rest of the file is preserved).
func (e *testEnv) rewriteClusterConfKey(newKey string) {
	path := filepath.Join(e.confDir, "cluster_conf", "cluster_conf.data")
	raw, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatalf("read cluster_conf.data: %v", err)
	}
	doc := map[string]interface{}{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		e.t.Fatalf("unmarshal cluster_conf.data: %v", err)
	}
	conf := doc["Config"].(map[string]interface{})[cluster].(map[string]interface{})
	aiConf := conf["AIConf"].(map[string]interface{})
	keys := aiConf["Keys"].([]interface{})
	keys[0].(map[string]interface{})["Key"] = newKey

	out, err := json.MarshalIndent(doc, "", "    ")
	if err != nil {
		e.t.Fatalf("marshal cluster_conf.data: %v", err)
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		e.t.Fatalf("write cluster_conf.data: %v", err)
	}
}

// assertEncryptedOnDisk asserts that the file contains the enc$v1$ marker
// and does not contain any of the given plaintext secrets.
func assertEncryptedOnDisk(t *testing.T, path string, plaintexts ...string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	content := string(data)
	if !strings.Contains(content, crypto.Marker) {
		t.Errorf("%s should contain %s ciphertext", path, crypto.Marker)
	}
	for _, p := range plaintexts {
		if strings.Contains(content, p) {
			t.Errorf("plaintext %q leaked on disk in %s", p, path)
		}
	}
}

// TC-01: token_rule.data with an enc$v1$ Tokens outer key loads and
// authenticates; the on-disk file holds no key plaintext.
func TestTC01_EncryptedTokenRuleLoadAndAuth(t *testing.T) {
	encKey := encryptEnvelope(t, "ak-enc", keyA, 1)
	e := newTestEnv(t, envOpt{
		keys: map[byte][]byte{1: keyA},
		tokenRule: tokenRuleData(map[string]common.TokenFile{
			"ak-123": plainToken("ak-123", "ak-123-id"),
			encKey:   {KeyId: "id-enc-01", Enabled: true, ExpiredTime: -1, UnlimitedQuota: true, QuotaPlans: []string{}},
		}),
		aiConf: defaultAIConf(),
	})
	defer e.Close()
	e.mustStart()

	assertEncryptedOnDisk(t, filepath.Join(e.confDir, "mod_ai_token_auth", "token_rule.data"), "ak-enc")

	if code, _ := e.sendRequest("ak-enc"); code != http.StatusOK {
		t.Errorf("ak-enc should auth (200), got %d", code)
	}
	if code, _ := e.sendRequest("ak-123"); code != http.StatusOK {
		t.Errorf("ak-123 should auth (200), got %d", code)
	}
	if code, _ := e.sendRequest("ak-wrong"); code != http.StatusUnauthorized {
		t.Errorf("ak-wrong should be rejected (401), got %d", code)
	}
}

// TC-02: cluster_conf.data with an enc$v1$ AIConf.Keys[].Key injects the
// plaintext upstream key; the on-disk file holds no key plaintext.
func TestTC02_EncryptedUpstreamKeyInjection(t *testing.T) {
	e := newTestEnv(t, envOpt{
		keys:      map[byte][]byte{1: keyA},
		tokenRule: tokenRuleData(map[string]common.TokenFile{"ak-123": plainToken("ak-123", "ak-123-id")}),
		aiConf: aiConfWithKeys(cluster_conf.AIKey{
			Name:   "k1",
			Key:    encryptEnvelope(t, upstreamKey1, keyA, 1),
			Weight: 100,
		}),
	})
	defer e.Close()
	e.mustStart()

	clusterConfFile := filepath.Join(e.confDir, "cluster_conf", "cluster_conf.data")
	assertEncryptedOnDisk(t, clusterConfFile, upstreamKey1)

	data, err := os.ReadFile(clusterConfFile)
	if err != nil {
		t.Fatalf("read cluster_conf.data: %v", err)
	}
	if !strings.Contains(string(data), `"k1"`) {
		t.Error("non-sensitive field Name=k1 should stay readable on disk")
	}

	if code, _ := e.sendRequest("ak-123"); code != http.StatusOK {
		t.Fatalf("request should succeed (200), got %d", code)
	}
	headers := e.backend.AuthHeaders()
	if len(headers) == 0 {
		t.Fatal("backend observed no Authorization header")
	}
	found := false
	for _, h := range headers {
		if h == "Bearer "+upstreamKey1 {
			found = true
		}
		if strings.Contains(h, crypto.Marker) {
			t.Error("upstream Authorization header must never carry ciphertext")
		}
	}
	if !found {
		t.Errorf("backend should receive plaintext key %q, got %v", upstreamKey1, headers)
	}
}

// TC-03: plaintext and ciphertext tokens/keys coexist in the same files.
func TestTC03_MixedPlaintextCiphertextCoexist(t *testing.T) {
	encToken := encryptEnvelope(t, "ak-enc", keyA, 1)
	e := newTestEnv(t, envOpt{
		keys: map[byte][]byte{1: keyA},
		tokenRule: tokenRuleData(map[string]common.TokenFile{
			"ak-plain": plainToken("ak-plain", "ak-plain-id"),
			encToken:   {KeyId: "id-enc-01", Enabled: true, ExpiredTime: -1, UnlimitedQuota: true, QuotaPlans: []string{}},
		}),
		aiConf: aiConfWithKeys(
			cluster_conf.AIKey{Name: "k1", Key: upstreamKey1, Weight: 50},
			cluster_conf.AIKey{Name: "k2", Key: encryptEnvelope(t, upstreamKey2, keyA, 1), Weight: 50},
		),
	})
	defer e.Close()
	e.mustStart()

	for _, key := range []string{"ak-plain", "ak-enc"} {
		if code, _ := e.sendRequest(key); code != http.StatusOK {
			t.Errorf("%s should auth (200), got %d", key, code)
		}
	}

	// both decrypted/plaintext upstream keys participate in weighted selection
	for i := 0; i < 30; i++ {
		if code, _ := e.sendRequest("ak-plain"); code != http.StatusOK {
			t.Fatalf("request %d should succeed, got %d", i, code)
		}
	}
	saw1, saw2 := false, false
	for _, h := range e.backend.AuthHeaders() {
		if h == "Bearer "+upstreamKey1 {
			saw1 = true
		}
		if h == "Bearer "+upstreamKey2 {
			saw2 = true
		}
	}
	if !saw1 || !saw2 {
		t.Errorf("both upstream keys should be selected, saw k1=%v k2=%v", saw1, saw2)
	}
}

// TC-04: ciphertext config without keyring fails reload and keeps the old
// effective config (canary/rollback safety).
func TestTC04_ReloadCiphertextWithoutKeyringKeepsOldConf(t *testing.T) {
	e := newTestEnv(t, envOpt{
		keys:      nil, // no [Security] at all
		tokenRule: tokenRuleData(map[string]common.TokenFile{"ak-123": plainToken("ak-123", "ak-123-id")}),
		aiConf:    defaultAIConf(),
	})
	defer e.Close()
	e.mustStart()

	if code, _ := e.sendRequest("ak-123"); code != http.StatusOK {
		t.Fatalf("baseline ak-123 should auth, got %d", code)
	}

	// control plane switches to encrypted exports; BFE has no keyring
	encKey := encryptEnvelope(t, "ak-enc", keyA, 1)
	e.writeTokenRule(tokenRuleData(map[string]common.TokenFile{
		encKey: {KeyId: "id-enc-01", Enabled: true, ExpiredTime: -1, UnlimitedQuota: true, QuotaPlans: []string{}},
	}))

	errMsg := e.reload("mod_ai_token_auth")
	if errMsg == "" {
		t.Fatal("reload should fail without keyring")
	}
	if !strings.Contains(errMsg, "keyring not configured") {
		t.Errorf("error should mention keyring not configured, got: %s", errMsg)
	}
	if strings.Contains(errMsg, crypto.Marker) {
		t.Error("reload error must not leak ciphertext")
	}

	// previously effective config is kept
	if code, _ := e.sendRequest("ak-123"); code != http.StatusOK {
		t.Errorf("old config should be kept after failed reload, ak-123 got %d", code)
	}
	if code, _ := e.sendRequest("ak-enc"); code != http.StatusUnauthorized {
		t.Errorf("new ciphertext config must not take effect, ak-enc got %d", code)
	}
}

// TC-05: wrong key material or unknown keyID fails startup (route layer,
// InitDataLoad fail-fast) without leaking ciphertext into logs.
func TestTC05_WrongKeyringDecryptFails(t *testing.T) {
	t.Run("wrong key material", func(t *testing.T) {
		e := newTestEnv(t, envOpt{
			keys:      map[byte][]byte{1: keyB}, // ciphertext made with keyA
			tokenRule: tokenRuleData(map[string]common.TokenFile{"ak-123": plainToken("ak-123", "ak-123-id")}),
			aiConf: aiConfWithKeys(cluster_conf.AIKey{
				Name:   "k1",
				Key:    encryptEnvelope(t, upstreamKey1, keyA, 1),
				Weight: 100,
			}),
		})
		defer e.Close()
		e.mustNotStart()

		if !e.logsContain("decrypt failed") {
			t.Error("log should contain GCM decrypt failure")
		}
		if e.logsContain(crypto.Marker) || e.logsContain(upstreamKey1) {
			t.Error("log must not leak ciphertext or plaintext key material")
		}
	})

	t.Run("unknown keyID", func(t *testing.T) {
		e := newTestEnv(t, envOpt{
			keys:      map[byte][]byte{2: keyA}, // ciphertext carries keyID 1
			tokenRule: tokenRuleData(map[string]common.TokenFile{"ak-123": plainToken("ak-123", "ak-123-id")}),
			aiConf: aiConfWithKeys(cluster_conf.AIKey{
				Name:   "k1",
				Key:    encryptEnvelope(t, upstreamKey1, keyA, 1),
				Weight: 100,
			}),
		})
		defer e.Close()
		e.mustNotStart()

		if !e.logsContain("unknown keyID 1") {
			t.Error("log should contain unknown keyID 1")
		}
		if e.logsContain(crypto.Marker) {
			t.Error("log must not leak ciphertext")
		}
	})
}

// TC-06: first startup (module layer) with ciphertext token config and a
// missing keyring fails fast; the control arm with keyring starts fine.
func TestTC06_FirstStartCiphertextFailFast(t *testing.T) {
	t.Run("no keyring refuses start", func(t *testing.T) {
		encKey := encryptEnvelope(t, "ak-enc", keyA, 1)
		e := newTestEnv(t, envOpt{
			keys: nil,
			tokenRule: tokenRuleData(map[string]common.TokenFile{
				encKey: {KeyId: "id-enc-01", Enabled: true, ExpiredTime: -1, UnlimitedQuota: true, QuotaPlans: []string{}},
			}),
			aiConf: defaultAIConf(),
		})
		defer e.Close()
		e.mustNotStart()

		if !e.logsContain("keyring not configured") {
			e.logBFEFiles()
			t.Error("log should contain keyring not configured")
		}
	})

	t.Run("with keyring starts", func(t *testing.T) {
		encKey := encryptEnvelope(t, "ak-enc", keyA, 1)
		e := newTestEnv(t, envOpt{
			keys: map[byte][]byte{1: keyA},
			tokenRule: tokenRuleData(map[string]common.TokenFile{
				encKey: {KeyId: "id-enc-01", Enabled: true, ExpiredTime: -1, UnlimitedQuota: true, QuotaPlans: []string{}},
			}),
			aiConf: defaultAIConf(),
		})
		defer e.Close()
		e.mustStart()

		if code, _ := e.sendRequest("ak-enc"); code != http.StatusOK {
			t.Errorf("ak-enc should auth (200), got %d", code)
		}
	})
}

// TC-07: rotation hot reload — updating the keyring file plus the existing
// reload endpoints activates the new key without a restart.
func TestTC07_KeyringHotReloadRotation(t *testing.T) {
	encV1 := encryptEnvelope(t, "ak-enc", keyA, 1)
	e := newTestEnv(t, envOpt{
		keys: map[byte][]byte{1: keyA},
		tokenRule: tokenRuleData(map[string]common.TokenFile{
			encV1: {KeyId: "id-enc-01", Enabled: true, ExpiredTime: -1, UnlimitedQuota: true, QuotaPlans: []string{}},
		}),
		aiConf: aiConfWithKeys(cluster_conf.AIKey{
			Name:   "k1",
			Key:    encryptEnvelope(t, upstreamKey1, keyA, 1),
			Weight: 100,
		}),
	})
	defer e.Close()
	e.mustStart()

	if code, _ := e.sendRequest("ak-enc"); code != http.StatusOK {
		t.Fatalf("baseline ak-enc should auth, got %d", code)
	}

	// rotation: keyring gains keyID=2; next export cycle re-encrypts
	// everything with the new key (full regeneration semantics)
	e.rotateKeyring(map[byte][]byte{1: keyA, 2: keyB})
	encV2 := encryptEnvelope(t, "ak-enc-v2", keyB, 2)
	e.writeTokenRule(tokenRuleData(map[string]common.TokenFile{
		encV2: {KeyId: "id-enc-v2-01", Enabled: true, ExpiredTime: -1, UnlimitedQuota: true, QuotaPlans: []string{}},
	}))
	e.rewriteClusterConfKey(encryptEnvelope(t, upstreamKey2, keyB, 2))

	if errMsg := e.reload("mod_ai_token_auth"); errMsg != "" {
		t.Fatalf("reload mod_ai_token_auth should succeed, got: %s", errMsg)
	}
	if errMsg := e.reload("server_data_conf"); errMsg != "" {
		t.Fatalf("reload server_data_conf should succeed, got: %s", errMsg)
	}

	if code, _ := e.sendRequest("ak-enc-v2"); code != http.StatusOK {
		t.Errorf("ak-enc-v2 should auth after rotation (200), got %d", code)
	}
	if code, _ := e.sendRequest("ak-enc"); code != http.StatusUnauthorized {
		t.Errorf("old ak-enc should be replaced after rotation (401), got %d", code)
	}

	e.backendAuthIs(t, upstreamKey2)
}

// TC-08: keyring with two keyIDs decrypts envelopes of both generations.
func TestTC08_MultiKeyIDCoexist(t *testing.T) {
	oldEnc := encryptEnvelope(t, "ak-old", keyA, 1)
	newEnc := encryptEnvelope(t, "ak-new", keyB, 2)
	e := newTestEnv(t, envOpt{
		keys: map[byte][]byte{1: keyA, 2: keyB},
		tokenRule: tokenRuleData(map[string]common.TokenFile{
			oldEnc: {KeyId: "id-old-01", Enabled: true, ExpiredTime: -1, UnlimitedQuota: true, QuotaPlans: []string{}},
			newEnc: {KeyId: "id-new-01", Enabled: true, ExpiredTime: -1, UnlimitedQuota: true, QuotaPlans: []string{}},
		}),
		aiConf: defaultAIConf(),
	})
	defer e.Close()
	e.mustStart()

	for _, key := range []string{"ak-old", "ak-new"} {
		if code, _ := e.sendRequest(key); code != http.StatusOK {
			t.Errorf("%s should auth (200), got %d", key, code)
		}
	}
}

// TC-09: all-plaintext config without [Security] starts and reloads
// normally (baseline / rollback state).
func TestTC09_AllPlaintextNoKeyringBaseline(t *testing.T) {
	e := newTestEnv(t, envOpt{
		keys:      nil,
		tokenRule: tokenRuleData(map[string]common.TokenFile{"ak-123": plainToken("ak-123", "ak-123-id")}),
		aiConf:    defaultAIConf(),
	})
	defer e.Close()
	e.mustStart()

	if code, _ := e.sendRequest("ak-123"); code != http.StatusOK {
		t.Fatalf("ak-123 should auth (200), got %d", code)
	}
	if errMsg := e.reload("mod_ai_token_auth"); errMsg != "" {
		t.Errorf("reload mod_ai_token_auth should succeed, got: %s", errMsg)
	}
	if errMsg := e.reload("server_data_conf"); errMsg != "" {
		t.Errorf("reload server_data_conf should succeed, got: %s", errMsg)
	}
}

// backendAuthIs asserts the latest observed backend Authorization header
// carries the given plaintext upstream key.
func (e *testEnv) backendAuthIs(t *testing.T, want string) {
	t.Helper()

	headers := e.backend.AuthHeaders()
	if len(headers) == 0 {
		t.Fatal("backend observed no Authorization header")
	}
	last := headers[len(headers)-1]
	if last != "Bearer "+want {
		t.Errorf("latest backend Authorization = %q, want %q", last, "Bearer "+want)
	}
}
