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
	"errors"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
	"github.com/bfenetworks/go-lib/web-monitor/metrics"
)

// concMockScript records its invocation and returns the canned result.
type concMockScript struct {
	res     interface{}
	err     error
	runCnt  int
	lastKey string
	lastArg []interface{}
}

func (s *concMockScript) Run(key string, args ...interface{}) (interface{}, error) {
	s.runCnt++
	s.lastKey = key
	s.lastArg = args
	return s.res, s.err
}

// concMockRedis implements redis_client.Client with a canned script; every
// other method is unused by the concurrency limiter.
type concMockRedis struct {
	redis_client.Client
	script *concMockScript
}

func (c *concMockRedis) NewScript(src string) redis_client.RedisScript {
	return c.script
}

func concTestState() *ProxyState {
	return &ProxyState{
		ReqAiKeyConcurrencyLimited:  new(metrics.Counter),
		ReqAiKeyConcurrencyRedisErr: new(metrics.Counter),
	}
}

func TestAIKeyConcurrencyRedisKey(t *testing.T) {
	got := aiKeyConcurrencyRedisKey("cluster-a", "key-1", "bfe:ai:key_affinity")
	want := "bfe:ai:key_affinity:conc:cluster-a:key-1"
	if got != want {
		t.Errorf("aiKeyConcurrencyRedisKey = %q, want %q", got, want)
	}
}

func TestAcquireAIKeySessionSlot_NoLimitOrNoClient(t *testing.T) {
	script := &concMockScript{res: int64(1)}
	client := &concMockRedis{script: script}
	state := concTestState()

	// limit <= 0: always allowed, no Redis call
	if !acquireAIKeySessionSlot(client, "c", "k", "s", "p", 0, 600, state) {
		t.Error("limit=0 must always allow")
	}
	if !acquireAIKeySessionSlot(client, "c", "k", "s", "p", -1, 600, state) {
		t.Error("limit<0 must always allow")
	}
	// nil client: fail-open, no Redis call
	if !acquireAIKeySessionSlot(nil, "c", "k", "s", "p", 1, 600, state) {
		t.Error("nil client must fail open")
	}
	if script.runCnt != 0 {
		t.Errorf("no script run expected, got %d", script.runCnt)
	}
}

func TestAcquireAIKeySessionSlot_ScriptVerdicts(t *testing.T) {
	cases := []struct {
		name    string
		res     interface{}
		err     error
		allowed bool
		redisErr int64
	}{
		{"slot granted", int64(1), nil, true, 0},
		{"slot full", int64(0), nil, false, 0},
		{"redis error fails open", nil, errors.New("boom"), true, 1},
		{"wrong reply type fails open", "1", nil, true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := &concMockScript{res: tc.res, err: tc.err}
			client := &concMockRedis{script: script}
			state := concTestState()

			got := acquireAIKeySessionSlot(client, "c", "k", "s", "p", 1, 600, state)
			if got != tc.allowed {
				t.Errorf("allowed = %v, want %v", got, tc.allowed)
			}
			if state.ReqAiKeyConcurrencyRedisErr.Get() != tc.redisErr {
				t.Errorf("redisErr counter = %d, want %d", state.ReqAiKeyConcurrencyRedisErr.Get(), tc.redisErr)
			}
			if script.runCnt != 1 {
				t.Errorf("script runs = %d, want 1", script.runCnt)
			}
		})
	}
}

func TestAcquireAIKeySessionSlot_ArgsAndAnonSession(t *testing.T) {
	script := &concMockScript{res: int64(1)}
	client := &concMockRedis{script: script}
	state := concTestState()

	// key format, limit and ttl forwarding (ttl seconds -> ms)
	if !acquireAIKeySessionSlot(client, "cluster-a", "key-1", "sess-1", "bfe:ai:key_affinity", 2, 300, state) {
		t.Fatal("expected slot granted")
	}
	if script.lastKey != "bfe:ai:key_affinity:conc:cluster-a:key-1" {
		t.Errorf("script key = %q", script.lastKey)
	}
	if len(script.lastArg) != 3 {
		t.Fatalf("script args = %v, want 3 args", script.lastArg)
	}
	if script.lastArg[0] != "sess-1" || script.lastArg[1] != 2 || script.lastArg[2] != 300000 {
		t.Errorf("script args = %v, want [sess-1 2 300000]", script.lastArg)
	}

	// empty sessionID degrades to a request-level anonymous identity
	before := aiKeyConcAnonSeq
	if !acquireAIKeySessionSlot(client, "c", "k", "", "p", 1, 600, state) {
		t.Fatal("expected slot granted")
	}
	member, ok := script.lastArg[0].(string)
	if !ok || !strings.HasPrefix(member, "anon-") {
		t.Errorf("anon member = %v, want anon- prefix", script.lastArg[0])
	}
	if aiKeyConcAnonSeq != before+1 {
		t.Errorf("anon seq = %d, want %d", aiKeyConcAnonSeq, before+1)
	}
}

func TestReleaseAIKeySessionSlot(t *testing.T) {
	// nil client / empty session: no-op
	releaseAIKeySessionSlot(nil, "c", "k", "s", "p", 600, concTestState())
	script := &concMockScript{res: int64(0)}
	client := &concMockRedis{script: script}
	releaseAIKeySessionSlot(client, "c", "k", "", "p", 600, concTestState())
	if script.runCnt != 0 {
		t.Errorf("no script run expected for empty session, got %d", script.runCnt)
	}

	// normal release runs the release script
	releaseAIKeySessionSlot(client, "c", "k", "s", "p", 600, concTestState())
	if script.runCnt != 1 {
		t.Errorf("script runs = %d, want 1", script.runCnt)
	}

	// release error is counted, never propagated
	errScript := &concMockScript{err: errors.New("boom")}
	errClient := &concMockRedis{script: errScript}
	state := concTestState()
	releaseAIKeySessionSlot(errClient, "c", "k", "s", "p", 600, state)
	if state.ReqAiKeyConcurrencyRedisErr.Get() != 1 {
		t.Errorf("redisErr counter = %d, want 1", state.ReqAiKeyConcurrencyRedisErr.Get())
	}
}

func TestAllEligibleKeysSlotFull(t *testing.T) {
	keys := []cluster_conf.AIKey{
		{Name: "a", Weight: 50},
		{Name: "b", Weight: 50},
		{Name: "c", Weight: 0}, // disabled
	}
	newState := func() *aiKeyAttemptState { return newAIKeyAttemptState() }

	// nothing full yet
	if allEligibleKeysSlotFull(keys, newState(), map[int]struct{}{}) {
		t.Error("no slot refused: must not report full")
	}
	// one of two eligible full
	if allEligibleKeysSlotFull(keys, newState(), map[int]struct{}{0: {}}) {
		t.Error("one of two eligible full: must not report full")
	}
	// both eligible full (weight-0 key irrelevant)
	if !allEligibleKeysSlotFull(keys, newState(), map[int]struct{}{0: {}, 1: {}}) {
		t.Error("both eligible full: must report full")
	}
	// dead key is not eligible: slot-full on it alone must not block
	st := newState()
	st.deadSet[1] = struct{}{}
	if !allEligibleKeysSlotFull(keys, st, map[int]struct{}{0: {}}) {
		t.Error("key 1 dead, key 0 full: must report full")
	}
	st2 := newState()
	st2.deadSet[0] = struct{}{}
	if allEligibleKeysSlotFull(keys, st2, map[int]struct{}{0: {}}) {
		t.Error("key 0 dead and full, key 1 clean: must not report full")
	}
}

func TestNewNoAvailableKey429Resp(t *testing.T) {
	basicReq := &bfe_basic.Request{}
	res := newNoAvailableKey429Resp(basicReq, "deepseek-chat", 600, "no available upstream api key (session slots full)")
	if res.StatusCode != bfe_http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", res.StatusCode)
	}
	if got := res.Header.Get("Retry-After"); got != "600" {
		t.Errorf("Retry-After = %q, want 600", got)
	}
	if res.Header.Get("X-Bfe-Gw-Error") != "1" {
		t.Error("missing X-Bfe-Gw-Error gateway marker")
	}

	// non-positive retryAfter falls back to 60
	res = newNoAvailableKey429Resp(basicReq, "", 0, "x")
	if got := res.Header.Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After fallback = %q, want 60", got)
	}
}
