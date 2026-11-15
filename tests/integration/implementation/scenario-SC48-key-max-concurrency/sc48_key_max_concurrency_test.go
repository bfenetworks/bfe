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

package sc48

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

const (
	apiHost     = "key-conc.example.org"
	apiPath     = "/v1/chat/completions"
	clusterName = "cluster_key_conc"

	primaryCluster = "cluster_key_conc_primary"
	fbCluster      = "cluster_key_conc_fb"

	keyA  = "sk-key-a"
	keyB  = "sk-key-b"
	keyFb = "sk-key-fb"

	// client tokens and their stable key ids (one session each)
	akA  = "ak_conc_a"
	akB  = "ak_conc_b"
	akC  = "ak_conc_c"
	kidA = "kid-a"
	kidB = "kid-b"
	kidC = "kid-c"

	akFb   = "ak_conc_fb"
	akFb2  = "ak_conc_fb2"
	kidFb  = "kid-fb"
	kidFb2 = "kid-fb2"

	affinityTTL = 300
)

// concPrefix is the Redis key prefix of the per-Key session slot ZSets.
const concPrefix = "bfe:ai:key_affinity:conc:"

var defaultBody = []byte(`{"model":"gpt-4"}`)

// testEnv holds the resources of one SC48 integration test.
type testEnv struct {
	t           *testing.T
	processEnv  *common.ProcessEnv
	backend     *common.MockBackend
	fbBackend   *common.MockBackend
	redis       *common.RedisServer
	confDir     string
	logDir      string
	bfePort     int
	monitorPort int
	stopBFE     func()
}

func defaultKeys() []cluster_conf.AIKey {
	return []cluster_conf.AIKey{
		{Name: "key-a", Key: keyA, Weight: 50},
		{Name: "key-b", Key: keyB, Weight: 50},
	}
}

func buildAIConf(keys []cluster_conf.AIKey, maxConc int) *cluster_conf.AIConf {
	return &cluster_conf.AIConf{
		Type: 0,
		Keys: keys,
		KeyPolicy: &cluster_conf.AIKeyPolicy{
			Strategy:                      "weighted_random",
			MaxRetries:                    3,
			RetryBackoffInitial:           50,
			RetryBackoffMax:               200,
			SessionAffinity:               true,
			SessionAffinityTTL:            affinityTTL,
			SessionAffinityRedisPrefix:    "bfe:ai:key_affinity",
			SessionAffinityPenaltyEnable:  true,
			SessionAffinityPenaltyTTL429:  60,
			SessionAffinityPenaltyTTLAuth: 300,
		},
		MaxConcurrencyPerKey: maxConc,
	}
}

// newTestEnv starts miniredis, a mock backend and a real BFE process serving
// the default two-Key cluster cluster_key_conc.
func newTestEnv(t *testing.T, maxConc int, keys []cluster_conf.AIKey,
	responseFunc func(r *http.Request, count int) (int, string)) *testEnv {

	t.Helper()
	if keys == nil {
		keys = defaultKeys()
	}

	e := &testEnv{t: t}
	e.backend = common.NewMockBackend(clusterName, http.StatusOK, `{"ok":true}`)
	if responseFunc != nil {
		e.backend.ResponseFunc = responseFunc
	}
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()
	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends: map[string]*common.MockBackend{
			clusterName: e.backend,
			// gslb.data statically lists the fallback-test clusters; they must
			// have backend conf in every environment even when unused here.
			primaryCluster: e.backend,
			fbCluster:      e.backend,
		},
		RedisAddr:           e.redis.Addr(),
		TokenRuleData:       buildTokenRule(false),
		RateLimitPolicyData: buildRateLimitPolicy(false),
		AIConfs: map[string]*cluster_conf.AIConf{
			clusterName: buildAIConf(keys, maxConc),
		},
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
	return e
}

// newFallbackTestEnv starts an environment for the fallback test: the primary
// cluster has a single Key with max=1 and the route (used by the ak_conc_fb
// tokens) declares cluster_key_conc_fb as fallback.
func newFallbackTestEnv(t *testing.T) *testEnv {
	t.Helper()

	e := &testEnv{t: t}
	e.backend = common.NewMockBackend(primaryCluster, http.StatusOK, `{"ok":true}`)
	e.fbBackend = common.NewMockBackend(fbCluster, http.StatusOK, `{"ok":true}`)
	e.redis = common.NewRedisServer(t)

	e.processEnv = common.NewProcessEnv(t)
	e.processEnv.Build()
	e.confDir = filepath.Join(e.processEnv.WorkDir(), "conf")
	e.logDir = filepath.Join(e.processEnv.WorkDir(), "log")

	builder := &common.BFEConfigBuilder{
		TemplateDir:   "testdata",
		TargetConfDir: e.confDir,
		Backends: map[string]*common.MockBackend{
			// BasicRule of the product points at cluster_key_conc; the AI
			// route sends traffic to the primary/fallback clusters instead.
			clusterName:    e.backend,
			primaryCluster: e.backend,
			fbCluster:      e.fbBackend,
		},
		RedisAddr:           e.redis.Addr(),
		TokenRuleData:       buildTokenRule(true),
		RateLimitPolicyData: buildRateLimitPolicy(true),
		AIConfs: map[string]*cluster_conf.AIConf{
			primaryCluster: buildAIConf([]cluster_conf.AIKey{{Name: "key-a", Key: keyA, Weight: 100}}, 1),
			fbCluster:      buildAIConf([]cluster_conf.AIKey{{Name: "key-fb", Key: keyFb, Weight: 100}}, 0),
		},
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("build bfe config failed: %v", err)
	}

	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)
	return e
}

// buildTokenRule configures one client token per session, all sharing a single
// unlimited quota plan, so the only variable under test is the slot cap.
func buildTokenRule(withFB bool) *common.TokenRuleData {
	tokens := map[string]map[string]common.TokenFile{
		"ai_product": {
			akA: tokenFile(akA, kidA),
			akB: tokenFile(akB, kidB),
			akC: tokenFile(akC, kidC),
		},
	}
	if withFB {
		tokens["ai_product"][akFb] = tokenFile(akFb, kidFb)
		tokens["ai_product"][akFb2] = tokenFile(akFb2, kidFb2)
	}

	return &common.TokenRuleData{
		Version: "1.0",
		QuotaPlans: map[string][]common.QuotaPlan{
			"ai_product": {
				{
					Id:          "unlimited_plan",
					Unlimited:   true,
					PassNoQuota: false,
					RedisKey:    "quota:unlimited_plan",
					ExpiredTime: -1,
					Quota:       0,
					Unit:        "total_token",
				},
			},
		},
		Tokens: tokens,
		Config: map[string][]common.TokenRule{
			"ai_product": {
				{
					Cond:   "default_t()",
					Action: common.ActionFile{Cmd: "CHECK_TOKEN"},
				},
			},
		},
	}
}

func tokenFile(key, keyId string) common.TokenFile {
	return common.TokenFile{
		Key:            key,
		KeyId:          keyId,
		Enabled:        true,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
		QuotaPlans:     []string{"unlimited_plan"},
	}
}

// buildRateLimitPolicy installs a pass-through rate limit policy so a local
// 429 never masks the slot-cap behavior under test.
func buildRateLimitPolicy(withFB bool) *common.RateLimitPolicyData {
	bindings := map[string][]string{
		akA: {"rlp-pass"},
		akB: {"rlp-pass"},
		akC: {"rlp-pass"},
	}
	if withFB {
		bindings[akFb] = []string{"rlp-pass"}
		bindings[akFb2] = []string{"rlp-pass"}
	}

	return &common.RateLimitPolicyData{
		Version: "1.0",
		Config: map[string][]common.RateLimitProductRule{
			"ai_product": {
				{
					Cond: "default_t()",
					HitAction: struct {
						Cmd    string   `json:"cmd"`
						Params []string `json:"params,omitempty"`
					}{Cmd: "FINISH"},
				},
			},
		},
		RateLimitPolicies: map[string]common.RateLimitPolicy{
			"rlp-pass": {
				Name:    "rlp-pass",
				Enabled: true,
				Rules: struct {
					TPM            []common.RateLimitRule       `json:"tpm,omitempty"`
					RPM            []common.RateLimitRule       `json:"rpm,omitempty"`
					MaxConcurrency *int64                       `json:"max_concurrency,omitempty"`
					Batch          *common.RateLimitBatchLimits `json:"batch,omitempty"`
				}{
					RPM: []common.RateLimitRule{
						{
							Name:          "rpm-pass",
							WindowMinutes: 1,
							MaxRequests:   1000000,
							Models:        []string{"*"},
							RedisKey:      "RL_RPM_rlp-pass_0",
						},
					},
				},
			},
		},
		ApikeyRateLimitPolicyBindings: bindings,
	}
}

func (e *testEnv) Close() {
	if e.stopBFE != nil {
		e.stopBFE()
		e.stopBFE = nil
	}
	if e.backend != nil {
		e.backend.Close()
	}
	if e.fbBackend != nil {
		e.fbBackend.Close()
	}
	if e.redis != nil {
		e.redis.Close()
	}
}

func (e *testEnv) logBFEException() {
	if data, err := os.ReadFile(filepath.Join(e.logDir, "exception.log")); err == nil && len(data) > 0 {
		e.t.Logf("bfe exception log:\n%s", string(data))
	}
	if data, err := os.ReadFile(filepath.Join(e.logDir, "bfe.log")); err == nil && len(data) > 0 {
		lines := strings.Split(string(data), "\n")
		start := 0
		if len(lines) > 200 {
			start = len(lines) - 200
		}
		e.t.Logf("bfe log tail:\n%s", strings.Join(lines[start:], "\n"))
	}
}

func (e *testEnv) sendRequest(apiKey string) (*http.Response, string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", e.bfePort, apiPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(defaultBody))
	if err != nil {
		return nil, "", err
	}
	req.Host = apiHost
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
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

// concKey returns the Redis key of the per-Key session slot ZSet.
func concKey(cluster, keyName string) string {
	return concPrefix + cluster + ":" + keyName
}

func (e *testEnv) concMembers(cluster, keyName string) []string {
	return e.redis.ZMembers(concKey(cluster, keyName))
}

func (e *testEnv) assertStatus(resp *http.Response, body string, want int) {
	e.t.Helper()
	if resp.StatusCode != want {
		e.logBFEException()
		e.t.Fatalf("expected status %d, got %d, body: %s", want, resp.StatusCode, body)
	}
}

// keyNameOf maps an observed backend Authorization header to the provider key
// name configured in the cluster.
func keyNameOf(authHeader string) string {
	switch authHeader {
	case "Bearer " + keyA:
		return "key-a"
	case "Bearer " + keyB:
		return "key-b"
	case "Bearer " + keyFb:
		return "key-fb"
	}
	return "unknown:" + authHeader
}

// TestTC01_SlotFullSynthetic429 covers the cap contract: once both Keys hold
// one session each, a third session gets the synthetic 429 (NO_AVAILABLE_KEY,
// Retry-After=SessionAffinityTTL) with zero upstream interaction.
func TestTC01_SlotFullSynthetic429(t *testing.T) {
	e := newTestEnv(t, 1, nil, nil)
	defer e.Close()

	respA, bodyA, err := e.sendRequest(akA)
	if err != nil {
		t.Fatalf("request A failed: %v", err)
	}
	e.assertStatus(respA, bodyA, http.StatusOK)

	respB, bodyB, err := e.sendRequest(akB)
	if err != nil {
		t.Fatalf("request B failed: %v", err)
	}
	e.assertStatus(respB, bodyB, http.StatusOK)

	// Whatever the random first picks were, rotation must leave both Keys
	// occupied by exactly one distinct session.
	ma := e.concMembers(clusterName, "key-a")
	mb := e.concMembers(clusterName, "key-b")
	if len(ma) != 1 || len(mb) != 1 {
		e.logBFEException()
		t.Fatalf("expected one session per key, got key-a=%v key-b=%v", ma, mb)
	}
	occupied := map[string]bool{ma[0]: true, mb[0]: true}
	if !occupied[kidA] || !occupied[kidB] || ma[0] == mb[0] {
		t.Fatalf("expected kid-a and kid-b on distinct keys, got key-a=%v key-b=%v", ma, mb)
	}

	hitsBefore := e.backend.Hits()
	if hitsBefore != 2 {
		t.Fatalf("expected 2 upstream hits for A and B, got %d", hitsBefore)
	}

	respC, bodyC, err := e.sendRequest(akC)
	if err != nil {
		t.Fatalf("request C failed: %v", err)
	}
	if respC.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected synthetic 429 for session C, got %d, body: %s", respC.StatusCode, bodyC)
	}
	if got := respC.Header.Get("Retry-After"); got != fmt.Sprintf("%d", affinityTTL) {
		t.Errorf("Retry-After = %q, want %d (SessionAffinityTTL)", got, affinityTTL)
	}
	if respC.Header.Get("X-Bfe-Gw-Error") != "1" {
		t.Error("synthetic 429 must carry the X-Bfe-Gw-Error marker")
	}
	var aiErr struct {
		Error struct {
			Code    string `json:"code"`
			Details *struct {
				RetryAfterSeconds int `json:"retry_after_seconds"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(bodyC), &aiErr); err != nil {
		t.Fatalf("parse error body failed: %v, body: %s", err, bodyC)
	}
	if aiErr.Error.Code != "NO_AVAILABLE_KEY" {
		t.Errorf("error.code = %q, want NO_AVAILABLE_KEY (body: %s)", aiErr.Error.Code, bodyC)
	}
	if aiErr.Error.Details == nil || aiErr.Error.Details.RetryAfterSeconds != affinityTTL {
		t.Errorf("details.retry_after_seconds = %+v, want %d", aiErr.Error.Details, affinityTTL)
	}
	if got := e.backend.Hits(); got != hitsBefore {
		t.Errorf("session C must not reach the upstream, hits %d -> %d", hitsBefore, got)
	}
	if e.redis.Exists("bfe:ai:key_affinity:" + clusterName + ":" + kidC) {
		t.Error("session C must not leave an affinity binding after the synthetic 429")
	}
}

// TestTC02_BoundSessionNeverSelfEvicted covers the affinity-preserving
// guarantee: a session holding a slot keeps hitting its bound Key under
// concurrency, and concurrent requests from the same session count once.
func TestTC02_BoundSessionNeverSelfEvicted(t *testing.T) {
	e := newTestEnv(t, 1, nil, nil)
	defer e.Close()

	resp, body, err := e.sendRequest(akA)
	if err != nil {
		t.Fatalf("warm-up request failed: %v", err)
	}
	e.assertStatus(resp, body, http.StatusOK)
	headers := e.backend.AuthHeaders()
	if len(headers) != 1 {
		t.Fatalf("expected 1 upstream hit, got %d", len(headers))
	}
	keyX := keyNameOf(headers[0])
	if keyX != "key-a" && keyX != "key-b" {
		t.Fatalf("unexpected bound key %q", headers[0])
	}

	// Three concurrent requests from the bound session.
	type result struct {
		resp *http.Response
		body string
		err  error
	}
	results := make(chan result, 3)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, body, err := e.sendRequest(akA)
			results <- result{resp, body, err}
		}()
	}
	wg.Wait()
	close(results)
	for r := range results {
		if r.err != nil {
			t.Fatalf("concurrent request failed: %v", r.err)
		}
		e.assertStatus(r.resp, r.body, http.StatusOK)
	}

	// All concurrent hits must use the bound Key, and the slot set must still
	// hold exactly one member for that session.
	for i, h := range e.backend.AuthHeaders() {
		if got := keyNameOf(h); got != keyX {
			t.Errorf("attempt %d used %q, expected the bound key %s", i, h, keyX)
		}
	}
	if members := e.concMembers(clusterName, keyX); len(members) != 1 || members[0] != kidA {
		t.Errorf("slot set of %s = %v, want exactly [kid-a]", keyX, members)
	}

	// Session B takes the other Key; A must not be evicted.
	respB, bodyB, err := e.sendRequest(akB)
	if err != nil {
		t.Fatalf("request B failed: %v", err)
	}
	e.assertStatus(respB, bodyB, http.StatusOK)
	keyY := keyNameOf(e.backend.AuthHeaders()[len(e.backend.AuthHeaders())-1])
	if keyY == keyX {
		t.Errorf("session B must bind the other key, got %s as well", keyY)
	}
	if members := e.concMembers(clusterName, keyX); len(members) != 1 || members[0] != kidA {
		t.Errorf("session B displaced session A on %s: members %v", keyX, members)
	}
	if members := e.concMembers(clusterName, keyY); len(members) != 1 || members[0] != kidB {
		t.Errorf("slot set of %s = %v, want exactly [kid-b]", keyY, members)
	}
}

// TestTC03_UnbindReleasesSlotImmediately covers the failure-driven release:
// a 401 on the bound Key frees that Key's slot at once (no TTL wait), so a
// new session can take it as soon as the penalty filter no longer excludes
// the Key.
func TestTC03_UnbindReleasesSlotImmediately(t *testing.T) {
	e := newTestEnv(t, 1, nil, func(r *http.Request, count int) (int, string) {
		// key-a suffers a single transient 401 (session A's first attempt,
		// pinned by the planted binding below); after the penalty expires it
		// is healthy again, so every later hit — including session C's — wins.
		if count == 1 {
			return http.StatusUnauthorized, `{"error":"invalid api key"}`
		}
		return http.StatusOK, `{"ok":true}`
	})
	defer e.Close()

	// Pin session A to key-a: 401 on key-a, rotation to key-b within the
	// same request.
	e.redis.Set("bfe:ai:key_affinity:"+clusterName+":"+kidA, "key-a")

	resp, body, err := e.sendRequest(akA)
	if err != nil {
		t.Fatalf("request A failed: %v", err)
	}
	e.assertStatus(resp, body, http.StatusOK)

	headers := e.backend.AuthHeaders()
	if len(headers) != 2 || keyNameOf(headers[0]) != "key-a" || keyNameOf(headers[1]) != "key-b" {
		t.Fatalf("expected key-a (401) then key-b (200), got %v", headers)
	}
	if got := e.concMembers(clusterName, "key-a"); len(got) != 0 {
		t.Errorf("key-a slot must be released immediately after the 401, members %v", got)
	}
	if got := e.concMembers(clusterName, "key-b"); len(got) != 1 || got[0] != kidA {
		t.Errorf("key-b slot members %v, want [kid-a]", got)
	}
	if !e.redis.Exists("bfe:ai:key_affinity:penalty:" + clusterName + ":key-a") {
		t.Error("401 must still write the penalty key (unchanged behavior)")
	}

	// key-a is penalized; expire the penalty so session C can be routed
	// there (key-level TTLs follow FastForward; the ZSet member scores use
	// the real clock and survive).
	e.redis.FastForward(301 * time.Second)
	respC, bodyC, err := e.sendRequest(akC)
	if err != nil {
		t.Fatalf("request C failed: %v", err)
	}
	e.assertStatus(respC, bodyC, http.StatusOK)
	if got := e.concMembers(clusterName, "key-a"); len(got) != 1 || got[0] != kidC {
		t.Errorf("key-a slot members %v, want [kid-c] (released slot reused)", got)
	}
}

// TestTC04_SlotFullTriggersClusterFallback covers the issue #1440 contract:
// the synthetic 429 carries ErrAiNoAvailableKey (not ErrAiRateLimit), so the
// outer fallback loop degrades to the configured fallback cluster.
func TestTC04_SlotFullTriggersClusterFallback(t *testing.T) {
	e := newFallbackTestEnv(t)
	defer e.Close()

	resp, body, err := e.sendRequest(akFb)
	if err != nil {
		t.Fatalf("request A failed: %v", err)
	}
	e.assertStatus(resp, body, http.StatusOK)
	if got := e.concMembers(primaryCluster, "key-a"); len(got) != 1 || got[0] != kidFb {
		t.Fatalf("primary slot members %v, want [kid-fb]", got)
	}

	hitsBefore := e.backend.Hits()
	respB, bodyB, err := e.sendRequest(akFb2)
	if err != nil {
		t.Fatalf("request B failed: %v", err)
	}
	e.assertStatus(respB, bodyB, http.StatusOK)

	if got := e.backend.Hits(); got != hitsBefore {
		t.Errorf("fallback must not call the primary upstream again, hits %d -> %d", hitsBefore, got)
	}
	if got := e.fbBackend.Hits(); got != 1 {
		t.Fatalf("expected 1 hit on the fallback cluster, got %d", got)
	}
	headers := e.fbBackend.AuthHeaders()
	if len(headers) != 1 || keyNameOf(headers[0]) != "key-fb" {
		t.Errorf("fallback request must use key-fb, got %v", headers)
	}
}

// TestTC05_ExpiredMemberReaped covers the idle-timeout release: a member whose
// score lies in the past is reaped by ZREMRANGEBYSCORE and its slot reused.
// miniredis runs the Lua TIME command on the real clock (FastForward does not
// affect it), so the test plants an expired member directly with ZADD.
func TestTC05_ExpiredMemberReaped(t *testing.T) {
	e := newTestEnv(t, 1, nil, nil)
	defer e.Close()

	resp, body, err := e.sendRequest(akA)
	if err != nil {
		t.Fatalf("request A failed: %v", err)
	}
	e.assertStatus(resp, body, http.StatusOK)
	keyX := keyNameOf(e.backend.AuthHeaders()[0])
	keyY := "key-a"
	if keyX == "key-a" {
		keyY = "key-b"
	}

	// Plant: session A's member on key-X is already expired; the other Key is
	// occupied by an unrelated session with a far-future score. Session B must
	// end up on key-X whatever its first pick is.
	e.redis.ZAdd(concKey(clusterName, keyX), 1, kidA)
	e.redis.ZAdd(concKey(clusterName, keyY), 1e15, "sess-other")

	respB, bodyB, err := e.sendRequest(akB)
	if err != nil {
		t.Fatalf("request B failed: %v", err)
	}
	e.assertStatus(respB, bodyB, http.StatusOK)
	if got := keyNameOf(e.backend.AuthHeaders()[len(e.backend.AuthHeaders())-1]); got != keyX {
		t.Errorf("session B must reuse the expired slot on %s, got %s", keyX, got)
	}
	if got := e.concMembers(clusterName, keyX); len(got) != 1 || got[0] != kidB {
		t.Errorf("key-X slot members %v, want [kid-b] (expired member reaped)", got)
	}
}

// TestTC06_RedisAbsentFailsOpen covers the degradation contract: with no
// [AIKeyAffinity] Redis configured the limit silently disables (fail-open)
// and no conc keys are ever written.
func TestTC06_RedisAbsentFailsOpen(t *testing.T) {
	e := newTestEnv(t, 1, nil, nil)
	defer e.Close()

	// Disable the affinity Redis after the config was generated, then restart
	// BFE: initAIKeyAffinityRedis leaves the handle nil and the limiter
	// fail-open must take over.
	e.stopBFE()
	patchBFEConfDisabledAffinity(e)
	e.bfePort, e.monitorPort, e.stopBFE = e.processEnv.StartBFE(e.confDir, e.logDir)

	for _, ak := range []string{akA, akB, akC} {
		resp, body, err := e.sendRequest(ak)
		if err != nil {
			t.Fatalf("request %s failed: %v", ak, err)
		}
		e.assertStatus(resp, body, http.StatusOK)
	}
	for _, k := range e.redis.Keys() {
		if strings.HasPrefix(k, concPrefix) {
			t.Errorf("no conc keys expected when the affinity redis is absent, found %s", k)
		}
	}
}

func patchBFEConfDisabledAffinity(e *testEnv) {
	e.t.Helper()
	path := filepath.Join(e.confDir, "bfe.conf")
	data, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatalf("read bfe.conf failed: %v", err)
	}
	patched := strings.Replace(string(data),
		"[AIKeyAffinity]\r\nDisabled = false", "[AIKeyAffinity]\r\nDisabled = true", 1)
	if patched == string(data) {
		patched = strings.Replace(string(data),
			"[AIKeyAffinity]\nDisabled = false", "[AIKeyAffinity]\nDisabled = true", 1)
	}
	if patched == string(data) {
		e.t.Fatalf("[AIKeyAffinity] section not found in bfe.conf:\n%s", string(data))
	}
	if err := os.WriteFile(path, []byte(patched), 0644); err != nil {
		e.t.Fatalf("write bfe.conf failed: %v", err)
	}
}

// TestTC07_HotReloadLimit covers the hot-update path: raising the cap from 1
// to 2 takes effect on the next request without a restart.
func TestTC07_HotReloadLimit(t *testing.T) {
	e := newTestEnv(t, 1, nil, nil)
	defer e.Close()

	respA, bodyA, err := e.sendRequest(akA)
	if err != nil {
		t.Fatalf("request A failed: %v", err)
	}
	e.assertStatus(respA, bodyA, http.StatusOK)
	respB, bodyB, err := e.sendRequest(akB)
	if err != nil {
		t.Fatalf("request B failed: %v", err)
	}
	e.assertStatus(respB, bodyB, http.StatusOK)

	respC, bodyC, err := e.sendRequest(akC)
	if err != nil {
		t.Fatalf("baseline request C failed: %v", err)
	}
	if respC.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected 429 for session C before reload, got %d, body: %s", respC.StatusCode, bodyC)
	}

	e.patchMaxConcurrencyPerKey(2)
	e.reloadServerDataConf()

	respC2, bodyC2, err := e.sendRequest(akC)
	if err != nil {
		t.Fatalf("request C after reload failed: %v", err)
	}
	e.assertStatus(respC2, bodyC2, http.StatusOK)

	found := false
	for _, kn := range []string{"key-a", "key-b"} {
		for _, m := range e.concMembers(clusterName, kn) {
			if m == kidC {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("kid-c must hold a slot after the cap was raised to 2")
	}
}

// patchMaxConcurrencyPerKey rewrites MaxConcurrencyPerKey in the generated
// cluster_conf.data, emulating a control-plane push.
func (e *testEnv) patchMaxConcurrencyPerKey(limit int) {
	e.t.Helper()

	path := filepath.Join(e.confDir, "cluster_conf", "cluster_conf.data")
	data, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatalf("read cluster_conf.data failed: %v", err)
	}
	var conf map[string]interface{}
	if err := json.Unmarshal(data, &conf); err != nil {
		e.t.Fatalf("parse cluster_conf.data failed: %v", err)
	}
	configs, ok := conf["Config"].(map[string]interface{})
	if !ok {
		e.t.Fatalf("unexpected cluster_conf.data shape: %s", string(data))
	}
	item, ok := configs[clusterName].(map[string]interface{})
	if !ok {
		e.t.Fatalf("cluster %s missing from cluster_conf.data", clusterName)
	}
	aiConf, ok := item["AIConf"].(map[string]interface{})
	if !ok {
		e.t.Fatalf("AIConf missing for cluster %s", clusterName)
	}
	aiConf["MaxConcurrencyPerKey"] = limit

	patched, err := json.MarshalIndent(conf, "", "    ")
	if err != nil {
		e.t.Fatalf("marshal cluster_conf.data failed: %v", err)
	}
	if err := os.WriteFile(path, patched, 0644); err != nil {
		e.t.Fatalf("write cluster_conf.data failed: %v", err)
	}
}

// reloadServerDataConf triggers a server_data_conf hot reload and fails when
// BFE reports an error.
func (e *testEnv) reloadServerDataConf() {
	e.t.Helper()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/reload/server_data_conf", e.monitorPort))
	if err != nil {
		e.t.Fatalf("reload request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("reload status = %d, body: %s", resp.StatusCode, string(body))
	}
	if strings.Contains(string(body), "AIConf") {
		e.t.Fatalf("reload rejected the configuration: %s", string(body))
	}
}

// TestTC08_DefaultZeroNoop covers the backward-compatible path: without the
// field the cluster behaves exactly as before (unlimited) and the limiter
// never touches Redis.
func TestTC08_DefaultZeroNoop(t *testing.T) {
	e := newTestEnv(t, 0, nil, nil)
	defer e.Close()

	for _, ak := range []string{akA, akB, akC} {
		resp, body, err := e.sendRequest(ak)
		if err != nil {
			t.Fatalf("request %s failed: %v", ak, err)
		}
		e.assertStatus(resp, body, http.StatusOK)
	}
	for _, k := range e.redis.Keys() {
		if strings.HasPrefix(k, concPrefix) {
			t.Errorf("no conc keys expected with MaxConcurrencyPerKey=0, found %s", k)
		}
	}
	// Affinity keeps working as before.
	if !e.redis.Exists("bfe:ai:key_affinity:" + clusterName + ":" + kidA) {
		t.Error("affinity binding for kid-a must exist")
	}
}
