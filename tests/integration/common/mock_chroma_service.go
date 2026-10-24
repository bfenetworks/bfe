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

package common

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// chroma API paths consumed by the mod_ai_cache vector provider.
const (
	chromaHeartbeatPath    = "/api/v1/heartbeat"
	chromaCollectionsPath  = "/api/v1/collections"
	chromaCollectionPrefix = "/api/v1/collections/"
	chromaMetaTenant       = "tenant_id"
	chromaMetaAnswer       = "answer"
	chromaMetaCreatedAt    = "created_at"
)

// ChromaRecord describes a single vector record of a collection. It is used
// by UpsertRecord to inject records directly into the in-memory store (e.g.
// a record with an expired created_at for TTL tests), bypassing HTTP.
type ChromaRecord struct {
	ID        string    // unique record id (upsert replaces on conflict)
	Tenant    string    // tenant_id metadata (mandatory filter)
	Question  string    // document payload
	Answer    string    // answer inlined in the metadata
	Embedding []float64 // vector
	CreatedAt time.Time // created_at metadata (unix seconds, TTL filter)
}

// chromaStoredRecord is the in-memory representation of one vector record.
type chromaStoredRecord struct {
	id        string
	document  string
	embedding []float64
	metadata  map[string]interface{}
}

// mockChromaCollection holds the records of one collection.
type mockChromaCollection struct {
	records map[string]*chromaStoredRecord
}

// MockChromaService is an in-process httptest server implementing the Chroma
// HTTP API subset consumed by the mod_ai_cache vector provider: heartbeat,
// collection get_or_create, record upsert and vector query. The query
// endpoint evaluates the where conjunction for real (tenant_id $eq,
// created_at $gt and friends) and ranks candidates by the true cosine
// distance of the stored vectors, so tenant isolation and TTL semantics can
// be asserted against it. Counters, a global failure switch (HTTP 500), an
// empty-result switch and direct record injection support the test cases.
type MockChromaService struct {
	t      *testing.T
	server *httptest.Server

	mu              sync.Mutex
	collections     map[string]*mockChromaCollection
	status          int // forced status code for all responses, 0 means off
	emptyResults    bool
	heartbeatHits   int
	collectionsHits int
	upsertHits      int
	queryHits       int
	queryFilters    []map[string]interface{}
}

// NewMockChromaService starts a local Chroma mock with an empty store.
func NewMockChromaService(t *testing.T) *MockChromaService {
	m := &MockChromaService{
		t:           t,
		collections: make(map[string]*mockChromaCollection),
	}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

func (m *MockChromaService) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	status := m.status
	m.mu.Unlock()
	if status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"mock chroma service failure"}`))
		return
	}

	path := r.URL.Path
	switch {
	case path == chromaHeartbeatPath && r.Method == http.MethodGet:
		m.handleHeartbeat(w)
	case path == chromaCollectionsPath && r.Method == http.MethodPost:
		m.handleGetOrCreateCollection(w, r)
	case strings.HasPrefix(path, chromaCollectionPrefix) && r.Method == http.MethodPost:
		rest := strings.TrimPrefix(path, chromaCollectionPrefix)
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) != 2 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		name, err := url.PathUnescape(parts[0])
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch parts[1] {
		case "upsert":
			m.handleUpsert(w, r, name)
		case "query":
			m.handleQuery(w, r, name)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (m *MockChromaService) handleHeartbeat(w http.ResponseWriter) {
	m.mu.Lock()
	m.heartbeatHits++
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"nanosecond heartbeat":0}`))
}

func (m *MockChromaService) handleGetOrCreateCollection(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Name == "" {
		http.Error(w, "invalid collection request", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.collectionsHits++
	if _, ok := m.collections[req.Name]; !ok {
		m.collections[req.Name] = &mockChromaCollection{records: make(map[string]*chromaStoredRecord)}
	}
	m.mu.Unlock()

	resp := map[string]interface{}{
		"name": req.Name,
		"id":   "mock-collection-" + req.Name,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (m *MockChromaService) handleUpsert(w http.ResponseWriter, r *http.Request, collection string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req struct {
		Ids        []string                 `json:"ids"`
		Embeddings [][]float64              `json:"embeddings"`
		Documents  []string                 `json:"documents"`
		Metadatas  []map[string]interface{} `json:"metadatas"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid upsert request", http.StatusBadRequest)
		return
	}
	if len(req.Ids) == 0 || len(req.Ids) != len(req.Embeddings) {
		http.Error(w, "ids/embeddings length mismatch", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.upsertHits++
	coll := m.collectionLocked(collection)
	for i, id := range req.Ids {
		rec := &chromaStoredRecord{
			id:        id,
			embedding: append([]float64(nil), req.Embeddings[i]...),
			metadata:  deepCopyMetadata(atIndex(req.Metadatas, i)),
		}
		if i < len(req.Documents) {
			rec.document = req.Documents[i]
		}
		coll.records[id] = rec
	}
	m.mu.Unlock()

	resp := map[string]interface{}{"ids": req.Ids}
	data, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (m *MockChromaService) handleQuery(w http.ResponseWriter, r *http.Request, collection string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req struct {
		QueryEmbeddings [][]float64            `json:"query_embeddings"`
		NResults        int                    `json:"n_results"`
		Where           map[string]interface{} `json:"where"`
		Include         []string               `json:"include"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid query request", http.StatusBadRequest)
		return
	}
	if len(req.QueryEmbeddings) == 0 {
		http.Error(w, "query_embeddings is empty", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.queryHits++
	m.queryFilters = append(m.queryFilters, deepCopyMetadata(req.Where))
	empty := m.emptyResults
	coll := m.collectionLocked(collection)

	// response layout mirrors the real Chroma API: one inner array per query
	// vector of the batch. Result computation runs under the lock so a
	// concurrent upsert can never mutate the record map mid-query.
	ids := make([][]string, len(req.QueryEmbeddings))
	distances := make([][]float64, len(req.QueryEmbeddings))
	metadatas := make([][]map[string]interface{}, len(req.QueryEmbeddings))
	documents := make([][]string, len(req.QueryEmbeddings))
	for i := range req.QueryEmbeddings {
		ids[i] = []string{}
		distances[i] = []float64{}
		metadatas[i] = []map[string]interface{}{}
		documents[i] = []string{}
		if empty {
			continue
		}
		for _, hit := range coll.query(req.QueryEmbeddings[i], req.NResults, req.Where) {
			ids[i] = append(ids[i], hit.record.id)
			distances[i] = append(distances[i], hit.distance)
			metadatas[i] = append(metadatas[i], deepCopyMetadata(hit.record.metadata))
			documents[i] = append(documents[i], hit.record.document)
		}
	}
	m.mu.Unlock()

	resp := map[string]interface{}{
		"ids":       ids,
		"distances": distances,
		"metadatas": metadatas,
		"documents": documents,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// scoredRecord is a query candidate with its computed cosine distance.
type scoredRecord struct {
	record   *chromaStoredRecord
	distance float64
}

// query filters the collection by the where conjunction, ranks the remaining
// records by true cosine distance of their stored vectors and keeps the top
// nResults closest. The caller must hold no lock.
func (c *mockChromaCollection) query(embedding []float64, nResults int,
	where map[string]interface{}) []scoredRecord {
	candidates := make([]scoredRecord, 0, len(c.records))
	for _, rec := range c.records {
		if !chromaWhereMatch(rec.metadata, where) {
			continue
		}
		candidates = append(candidates, scoredRecord{
			record:   rec,
			distance: cosineDistance(embedding, rec.embedding),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distance != candidates[j].distance {
			return candidates[i].distance < candidates[j].distance
		}
		return candidates[i].record.id < candidates[j].record.id
	})
	if nResults < 1 {
		nResults = 1
	}
	if len(candidates) > nResults {
		candidates = candidates[:nResults]
	}
	return candidates
}

// chromaWhereMatch evaluates a Chroma where expression against record
// metadata. Supported: field operators ($eq/$ne/$gt/$gte/$lt/$lte) and the
// logical combinators $and / $or. An empty where matches everything.
func chromaWhereMatch(meta map[string]interface{}, where map[string]interface{}) bool {
	for key, cond := range where {
		switch strings.ToLower(key) {
		case "$and":
			items, ok := cond.([]interface{})
			if !ok {
				return false
			}
			for _, item := range items {
				m, ok := item.(map[string]interface{})
				if !ok || !chromaWhereMatch(meta, m) {
					return false
				}
			}
		case "$or":
			items, ok := cond.([]interface{})
			if !ok {
				return false
			}
			matched := false
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok && chromaWhereMatch(meta, m) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		default:
			ops, ok := cond.(map[string]interface{})
			if !ok {
				return false
			}
			for op, expect := range ops {
				if !chromaMetaMatch(meta[key], op, expect) {
					return false
				}
			}
		}
	}
	return true
}

// chromaMetaMatch evaluates one field operator. Numbers are compared as
// float64 (the JSON wire format); strings support $eq/$ne.
func chromaMetaMatch(actual interface{}, op string, expect interface{}) bool {
	switch strings.ToLower(op) {
	case "$eq":
		return chromaValueEqual(actual, expect)
	case "$ne":
		return !chromaValueEqual(actual, expect)
	case "$gt", "$gte", "$lt", "$lte":
		a, okA := chromaValueFloat(actual)
		b, okB := chromaValueFloat(expect)
		if !okA || !okB {
			return false
		}
		switch strings.ToLower(op) {
		case "$gt":
			return a > b
		case "$gte":
			return a >= b
		case "$lt":
			return a < b
		default:
			return a <= b
		}
	}
	return false
}

func chromaValueEqual(a, b interface{}) bool {
	if fa, ok := chromaValueFloat(a); ok {
		fb, ok := chromaValueFloat(b)
		return ok && fa == fb
	}
	sa, okA := a.(string)
	sb, okB := b.(string)
	return okA && okB && sa == sb
}

func chromaValueFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// cosineDistance returns 1 - cosine similarity of two vectors (Chroma
// convention: 0 is identical, 2 is opposite). Degenerate inputs yield the
// maximum distance.
func cosineDistance(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 2
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 2
	}
	return 1 - dot/(math.Sqrt(normA)*math.Sqrt(normB))
}

// collectionLocked returns the named collection, creating it on first use.
// The caller must hold m.mu.
func (m *MockChromaService) collectionLocked(name string) *mockChromaCollection {
	coll, ok := m.collections[name]
	if !ok {
		coll = &mockChromaCollection{records: make(map[string]*chromaStoredRecord)}
		m.collections[name] = coll
	}
	return coll
}

func atIndex(items []map[string]interface{}, i int) map[string]interface{} {
	if i < len(items) {
		return items[i]
	}
	return nil
}

// deepCopyMetadata copies a metadata map (or where expression) through JSON
// so stored/shared maps are never aliased into responses or assertions.
func deepCopyMetadata(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	data, err := json.Marshal(in)
	if err != nil {
		return in
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return in
	}
	return out
}

// UpsertRecord injects one record directly into the in-memory store of the
// given collection, materializing the same metadata (tenant_id, answer,
// created_at) the real upload path sends. It bypasses HTTP and the failure
// switches, which makes it suitable for seeding expired records.
func (m *MockChromaService) UpsertRecord(collection string, rec ChromaRecord) {
	metadata := map[string]interface{}{
		chromaMetaTenant:    rec.Tenant,
		chromaMetaAnswer:    rec.Answer,
		chromaMetaCreatedAt: rec.CreatedAt.Unix(),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	coll := m.collectionLocked(collection)
	coll.records[rec.ID] = &chromaStoredRecord{
		id:        rec.ID,
		document:  rec.Question,
		embedding: append([]float64(nil), rec.Embedding...),
		metadata:  metadata,
	}
}

// RecordCount returns the number of records stored in the collection.
func (m *MockChromaService) RecordCount(collection string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.collectionLocked(collection).records)
}

// HeartbeatHits returns the number of heartbeat calls received.
func (m *MockChromaService) HeartbeatHits() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.heartbeatHits
}

// CollectionsHits returns the number of collection get_or_create calls received.
func (m *MockChromaService) CollectionsHits() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.collectionsHits
}

// UpsertHits returns the number of upsert calls received.
func (m *MockChromaService) UpsertHits() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.upsertHits
}

// QueryHits returns the number of query calls received.
func (m *MockChromaService) QueryHits() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.queryHits
}

// QueryFilters returns deep copies of all where expressions received by the
// query endpoint, in arrival order.
func (m *MockChromaService) QueryFilters() []map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]map[string]interface{}, len(m.queryFilters))
	for i, f := range m.queryFilters {
		result[i] = deepCopyMetadata(f)
	}
	return result
}

// SetStatus forces the given HTTP status code for all subsequent responses
// (0 disables the override).
func (m *MockChromaService) SetStatus(status int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = status
}

// SetFailAll switches the mock between global failure (every response is
// HTTP 500) and normal behavior.
func (m *MockChromaService) SetFailAll(fail bool) {
	if fail {
		m.SetStatus(http.StatusInternalServerError)
		return
	}
	m.SetStatus(0)
}

// SetEmptyResults forces the query endpoint to return empty result sets
// while still accepting and recording requests.
func (m *MockChromaService) SetEmptyResults(empty bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emptyResults = empty
}

// URL returns the http address of the mock chroma service.
func (m *MockChromaService) URL() string {
	return m.server.URL
}

// Addr returns the host:port of the mock chroma service.
func (m *MockChromaService) Addr() string {
	u, _ := url.Parse(m.server.URL)
	return u.Host
}

// Close shuts down the mock chroma service.
func (m *MockChromaService) Close() {
	m.server.Close()
}
