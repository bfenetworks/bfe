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

package mod_ai_cache

import (
	"errors"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_module"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

// errRedisClient fails every Get to drive the redis error path (issue #1499).
type errRedisClient struct {
	*fakeRedisClient
}

func (e *errRedisClient) Get(key string) (interface{}, error) {
	e.getCalls++
	return nil, errors.New("redis down")
}

// Issue #1499: the JSON monitor source (module state) must mirror the
// prometheus source (rule table counters) on the redis error/latency paths.
// Previously the state counters were declared but never incremented, so
// /monitor/mod_ai_cache showed REDIS_ERR=0 while
// /monitor/mod_ai_cache.prometheus showed a growing redis_err.
func TestRedisErrAndLatencyDualWrite(t *testing.T) {
	table := newCacheRuleTable()
	state := newTestModuleState()
	rc := newRedisCache("test", &errRedisClient{newFakeRedisClient("")}, table, state)

	if _, hit := rc.Get("k"); hit {
		t.Fatal("a failing redis must surface as a miss (fail-open)")
	}

	snap := table.snapshotCounters()
	if got := state.GetAll().SCounters["REDIS_ERR"]; got != int64(snap.redisErr) || got != 1 {
		t.Errorf("REDIS_ERR = %d, want 1 and equal to the rule table redisErr %d", got, snap.redisErr)
	}
	// The two sources must agree exactly; the value itself may be 0 on a
	// fast machine.
	if got := state.GetAll().SCounters["LATENCY_MS"]; got != int64(snap.latencyMs) {
		t.Errorf("LATENCY_MS = %d, want equal to the rule table latencyMs %d", got, snap.latencyMs)
	}

	// A start time in the past forces ms > 0 through the same helper every
	// redis operation uses.
	rc.recordLatency(time.Now().Add(-10 * time.Millisecond))
	if got := state.GetAll().SCounters["LATENCY_MS"]; got <= 0 {
		t.Errorf("LATENCY_MS = %d, want > 0 after a forced-latency record", got)
	}
}

// Issue #1499: the semantic cache error/latency counters must be dual-written
// too (embedding err, vector err, embedding latency, vector latency).
func TestSemanticErrAndLatencyDualWrite(t *testing.T) {
	// embedding failure arm
	table := newCacheRuleTable()
	state := newTestModuleState()
	sc := newSemanticCache(&fakeEmbeddingProvider{err: errors.New("embedding down")},
		&fakeVectorProvider{}, 0, table, state)

	if _, _, _, ok := sc.Lookup("q", "tenant", 0, &SemanticConf{TopK: 1}); ok {
		t.Fatal("an embedding failure must degrade to a miss")
	}
	snap := table.snapshotCounters()
	if got := state.GetAll().SCounters["EMBEDDING_ERR"]; got != int64(snap.embeddingErr) || got != 1 {
		t.Errorf("EMBEDDING_ERR = %d, want 1 and equal to the rule table embeddingErr %d", got, snap.embeddingErr)
	}
	if got := state.GetAll().SCounters["EMBEDDING_LATENCY_MS"]; got != int64(snap.embeddingLatencyMs) {
		t.Errorf("EMBEDDING_LATENCY_MS = %d, want equal to the rule table embeddingLatencyMs %d",
			got, snap.embeddingLatencyMs)
	}

	// vector query failure arm
	table2 := newCacheRuleTable()
	state2 := newTestModuleState()
	sc2 := newSemanticCache(&fakeEmbeddingProvider{vec: []float32{0.1, 0.2}},
		&fakeVectorProvider{err: errors.New("vector down")}, 0, table2, state2)

	if _, _, _, ok := sc2.Lookup("q", "tenant", 0, &SemanticConf{TopK: 1}); ok {
		t.Fatal("a vector failure must degrade to a miss")
	}
	snap2 := table2.snapshotCounters()
	if got := state2.GetAll().SCounters["VECTOR_ERR"]; got != int64(snap2.vectorErr) || got != 1 {
		t.Errorf("VECTOR_ERR = %d, want 1 and equal to the rule table vectorErr %d", got, snap2.vectorErr)
	}
	if got := state2.GetAll().SCounters["VECTOR_LATENCY_MS"]; got != int64(snap2.vectorLatencyMs) {
		t.Errorf("VECTOR_LATENCY_MS = %d, want equal to the rule table vectorLatencyMs %d",
			got, snap2.vectorLatencyMs)
	}
}

// Issue #1499 machine-checkable criterion: after exercising the request
// hit/miss paths and the fixed error paths, every CounterKeys entry in the
// module state (JSON monitor) equals the rule table snapshot (prometheus
// monitor) — the two endpoints cannot diverge on any counter.
func TestMonitorCountersJSONMatchPrometheus(t *testing.T) {
	m := prepareTestModule(t)
	m.redisCache = newRedisCache(m.name, newFakeRedisClient(""), m.ruleTable, m.state)

	// One miss and one hit through the real request handler (REQ_TOTAL +2,
	// CACHE_MISS +1, CACHE_HIT +1 on the two sources).
	req := newTestRequest("default", chatBody("what is bfe", false))
	req.InitAiBasicInfo().ClientKeyId = "key_001"
	if ret, _ := m.cacheRequestHandler(req); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected pass on miss, got ret=%d", ret)
	}
	fake, ok := m.redisCache.client.(*fakeRedisClient)
	if !ok {
		t.Fatalf("expected *fakeRedisClient, got %T", m.redisCache.client)
	}
	fake.value = "BFE is a layer-7 load balancer"
	req2 := newTestRequest("default", chatBody("what is bfe", false))
	req2.InitAiBasicInfo().ClientKeyId = "key_001"
	if ret, _ := m.cacheRequestHandler(req2); ret != bfe_module.BfeHandlerFinish {
		t.Fatalf("expected finish on hit, got ret=%d", ret)
	}

	// One redis error through the real wrapper (REDIS_ERR +1 both sides).
	rc := newRedisCache("test", &errRedisClient{newFakeRedisClient("")}, m.ruleTable, m.state)
	if _, hit := rc.Get("k"); hit {
		t.Fatal("a failing redis must surface as a miss (fail-open)")
	}
	rc.recordLatency(time.Now().Add(-10 * time.Millisecond))

	snap := m.ruleTable.snapshotCounters()
	jsonCounters := m.state.GetAll().SCounters

	want := map[string]uint64{
		"REQ_TOTAL":     snap.reqTotal,
		"CACHE_MISS":    snap.cacheMiss,
		"CACHE_HIT":     snap.cacheHit,
		"REDIS_ERR":     snap.redisErr,
		"LATENCY_MS":    snap.latencyMs,
		"EMBEDDING_ERR": snap.embeddingErr,
		"VECTOR_ERR":    snap.vectorErr,
		// Latency counters may be 0 on a fast machine; both sides must agree.
		"EMBEDDING_LATENCY_MS": snap.embeddingLatencyMs,
		"VECTOR_LATENCY_MS":    snap.vectorLatencyMs,
	}
	for _, key := range CounterKeys {
		var wantVal uint64
		if v, ok := want[key]; ok {
			wantVal = v
		}
		if got := jsonCounters[key]; uint64(got) != wantVal {
			t.Errorf("counter %s: JSON monitor = %d, want equal to prometheus source %d",
				key, got, wantVal)
		}
	}

	// The exercised events must actually have counted (guards against a
	// vacuous all-zero pass).
	if jsonCounters["REQ_TOTAL"] != 2 || jsonCounters["CACHE_MISS"] != 1 || jsonCounters["CACHE_HIT"] != 1 {
		t.Errorf("request counters = REQ_TOTAL:%d CACHE_MISS:%d CACHE_HIT:%d, want 2/1/1",
			jsonCounters["REQ_TOTAL"], jsonCounters["CACHE_MISS"], jsonCounters["CACHE_HIT"])
	}
	if jsonCounters["REDIS_ERR"] != 1 {
		t.Errorf("REDIS_ERR = %d, want 1", jsonCounters["REDIS_ERR"])
	}
}

var _ redis_client.Client = (*errRedisClient)(nil)
