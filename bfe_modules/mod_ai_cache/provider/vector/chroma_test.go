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

package vector

import (
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

// chromaMock records the requests of a fake Chroma server and replies with
// scripted responses.
type chromaMock struct {
	srv *httptest.Server

	createBody string // body of get_or_create collection
	upsertBody string // body of the last upsert
	queryBody  string // body of the last query

	failHeartbeat bool
	failQuery     bool
	failUpsert    bool
	queryResp     string
}

func newChromaMock(t *testing.T) *chromaMock {
	m := &chromaMock{}
	m.queryResp = `{"ids":[["id1"]],"distances":[[0.1]],"documents":[["question text"]],` +
		`"metadatas":[[{"tenant_id":"key_001","answer":"cached answer","created_at":1700000000}]]}`
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		switch r.URL.Path {
		case "/api/v1/heartbeat":
			if m.failHeartbeat {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write([]byte(`{"nanosecond heartbeat":0}`))
		case "/api/v1/collections":
			m.createBody = string(body)
			w.Write([]byte(`{"id":"collection-id-1","name":"ai_cache_semantic"}`))
		case "/api/v1/collections/ai_cache_semantic/query":
			m.queryBody = string(body)
			if m.failQuery {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write([]byte(m.queryResp))
		case "/api/v1/collections/ai_cache_semantic/upsert":
			m.upsertBody = string(body)
			if m.failUpsert {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return m
}

func (m *chromaMock) close() {
	m.srv.Close()
}

func (m *chromaMock) config() Config {
	u, _ := url.Parse(m.srv.URL)
	h, p, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(p)
	return Config{ServiceHost: h, ServicePort: port, Collection: "ai_cache_semantic", TimeoutMs: 500}
}

func TestChromaProviderInit(t *testing.T) {
	mock := newChromaMock(t)
	defer mock.close()

	p, err := NewChromaProvider(mock.config())
	if err != nil {
		t.Fatalf("NewChromaProvider error: %v", err)
	}
	if p == nil {
		t.Fatal("provider should not be nil")
	}

	// collection must be created with the cosine space
	var createReq map[string]interface{}
	if err := json.Unmarshal([]byte(mock.createBody), &createReq); err != nil {
		t.Fatalf("create collection body is not json: %v", err)
	}
	if createReq["name"] != "ai_cache_semantic" || createReq["get_or_create"] != true {
		t.Errorf("unexpected create collection body: %s", mock.createBody)
	}
	metadata, ok := createReq["metadata"].(map[string]interface{})
	if !ok || metadata["hnsw:space"] != "cosine" {
		t.Errorf("collection must be created with hnsw:space=cosine, got: %s", mock.createBody)
	}

	// heartbeat failure must fail the init (caller degrades, fail-open)
	mock2 := newChromaMock(t)
	defer mock2.close()
	mock2.failHeartbeat = true
	if _, err := NewChromaProvider(mock2.config()); err == nil {
		t.Error("heartbeat failure should fail the provider init")
	}
}

func TestChromaProviderQueryWhereClause(t *testing.T) {
	mock := newChromaMock(t)
	defer mock.close()

	p, err := NewChromaProvider(mock.config())
	if err != nil {
		t.Fatalf("NewChromaProvider error: %v", err)
	}

	results, err := p.Query([]float32{0.1, 0.2}, "key_001", 5, time.Hour)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// the where clause must contain BOTH the tenant condition and the TTL
	// condition (tenant isolation is enforced at the query level)
	var queryReq map[string]interface{}
	if err := json.Unmarshal([]byte(mock.queryBody), &queryReq); err != nil {
		t.Fatalf("query body is not json: %v", err)
	}
	where, ok := queryReq["where"].(map[string]interface{})
	if !ok {
		t.Fatalf("query must carry a where clause, got: %s", mock.queryBody)
	}
	and, ok := where["$and"].([]interface{})
	if !ok || len(and) != 2 {
		t.Fatalf("where must be a $and of tenant + ttl conditions, got: %s", mock.queryBody)
	}
	tenantCond, ok := and[0].(map[string]interface{})["tenant_id"].(map[string]interface{})
	if !ok || tenantCond["$eq"] != "key_001" {
		t.Errorf("where must contain tenant_id $eq key_001, got: %s", mock.queryBody)
	}
	ttlCond, ok := and[1].(map[string]interface{})["created_at"].(map[string]interface{})
	if !ok {
		t.Fatalf("where must contain created_at condition, got: %s", mock.queryBody)
	}
	cutoff, ok := ttlCond["$gt"].(float64)
	if !ok || cutoff <= 0 {
		t.Errorf("created_at $gt must be a unix timestamp in the past, got: %v", ttlCond["$gt"])
	}
	if int64(cutoff) > time.Now().Add(-time.Hour).Unix() {
		t.Errorf("created_at $gt must be now-ttl, got: %d", int64(cutoff))
	}

	// distance is the native score; similarity = 1 - distance
	if results[0].Score != 0.1 {
		t.Errorf("expected score 0.1, got %f", results[0].Score)
	}
	if results[0].Similarity != 0.9 {
		t.Errorf("expected similarity 0.9, got %f", results[0].Similarity)
	}
	if results[0].Answer != "cached answer" || results[0].Question != "question text" {
		t.Errorf("unexpected result: %+v", results[0])
	}
}

func TestChromaProviderQueryNoTtl(t *testing.T) {
	mock := newChromaMock(t)
	defer mock.close()

	p, err := NewChromaProvider(mock.config())
	if err != nil {
		t.Fatalf("NewChromaProvider error: %v", err)
	}

	// ttl <= 0 means no expiry: only the tenant condition
	if _, err := p.Query([]float32{0.1}, "key_001", 1, 0); err != nil {
		t.Fatalf("Query error: %v", err)
	}
	var queryReq map[string]interface{}
	if err := json.Unmarshal([]byte(mock.queryBody), &queryReq); err != nil {
		t.Fatalf("query body is not json: %v", err)
	}
	where := queryReq["where"].(map[string]interface{})
	if _, hasAnd := where["$and"]; hasAnd {
		t.Errorf("no ttl must not add a $and, got: %s", mock.queryBody)
	}
	tenantCond, ok := where["tenant_id"].(map[string]interface{})
	if !ok || tenantCond["$eq"] != "key_001" {
		t.Errorf("where must contain tenant_id $eq, got: %s", mock.queryBody)
	}
}

func TestChromaProviderQueryEmptyAndError(t *testing.T) {
	mock := newChromaMock(t)
	defer mock.close()
	mock.queryResp = `{"ids":[[]],"distances":[[]]}`

	p, err := NewChromaProvider(mock.config())
	if err != nil {
		t.Fatalf("NewChromaProvider error: %v", err)
	}

	results, err := p.Query([]float32{0.1}, "key_001", 1, time.Hour)
	if err != nil {
		t.Fatalf("empty result should not be an error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results, got %d", len(results))
	}

	mock.failQuery = true
	if _, err := p.Query([]float32{0.1}, "key_001", 1, time.Hour); err == nil {
		t.Error("query failure should be reported")
	}
}

func TestChromaProviderUpload(t *testing.T) {
	mock := newChromaMock(t)
	defer mock.close()

	p, err := NewChromaProvider(mock.config())
	if err != nil {
		t.Fatalf("NewChromaProvider error: %v", err)
	}

	item := Item{
		ID:        ItemID("key_001", "what is bfe"),
		Tenant:    "key_001",
		Question:  "what is bfe",
		Answer:    "BFE is a layer-7 load balancer",
		Embedding: []float32{0.1, 0.2},
		CreatedAt: time.Unix(1700000000, 0),
	}
	if err := p.Upload(item); err != nil {
		t.Fatalf("Upload error: %v", err)
	}

	// data layout: document is the question, metadata carries tenant_id,
	// answer and created_at (unix seconds); id is deterministic
	var upsertReq map[string]interface{}
	if err := json.Unmarshal([]byte(mock.upsertBody), &upsertReq); err != nil {
		t.Fatalf("upsert body is not json: %v", err)
	}
	ids := upsertReq["ids"].([]interface{})
	if ids[0] != ItemID("key_001", "what is bfe") {
		t.Errorf("upsert id must be deterministic (sha256(tenant\\nquestion)), got: %v", ids[0])
	}
	if ItemID("key_001", "what is bfe") == ItemID("key_002", "what is bfe") {
		t.Error("different tenants must produce different ids")
	}
	if ItemID("key_001", "what is bfe") == ItemID("key_001", "other question") {
		t.Error("different questions must produce different ids")
	}
	docs := upsertReq["documents"].([]interface{})
	if docs[0] != "what is bfe" {
		t.Errorf("document must be the question text, got: %v", docs[0])
	}
	metadatas := upsertReq["metadatas"].([]interface{})
	meta := metadatas[0].(map[string]interface{})
	if meta["tenant_id"] != "key_001" || meta["answer"] != "BFE is a layer-7 load balancer" {
		t.Errorf("unexpected metadata: %v", meta)
	}
	if meta["created_at"] != float64(1700000000) {
		t.Errorf("created_at must be unix seconds, got: %v", meta["created_at"])
	}

	mock.failUpsert = true
	if err := p.Upload(item); err == nil {
		t.Error("upsert failure should be reported")
	}
}

func TestChromaProviderConfigCheck(t *testing.T) {
	base := Config{ServiceHost: "127.0.0.1", ServicePort: 8000, Collection: "ai_cache_semantic"}

	// config errors are rejected before any network access
	cfg := base
	cfg.ServiceHost = ""
	if _, err := NewChromaProvider(cfg); err == nil {
		t.Error("empty host should be rejected")
	}
	cfg = base
	cfg.ServicePort = 0
	if _, err := NewChromaProvider(cfg); err == nil {
		t.Error("invalid port should be rejected")
	}
	cfg = base
	cfg.Collection = ""
	if _, err := NewChromaProvider(cfg); err == nil {
		t.Error("empty collection should be rejected")
	}
}
