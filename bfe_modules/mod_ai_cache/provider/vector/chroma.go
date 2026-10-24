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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// maxChromaRespBody bounds the chroma response body (8MB).
const maxChromaRespBody = 8 << 20

// chroma metadata keys
const (
	chromaMetaTenant    = "tenant_id"
	chromaMetaAnswer    = "answer"
	chromaMetaCreatedAt = "created_at"
)

// Config carries the connection info of a Chroma server.
type Config struct {
	ServiceHost string
	ServicePort int
	ApiKey      string // bearer credential, never logged
	Collection  string
	TimeoutMs   int
}

type chromaProvider struct {
	base       string // http://host:port
	apiKey     string
	collection string
	// resource is the collection identifier used in the request paths. Chroma
	// 0.6.x accepts only the collection UUID for /collections/{x}/query etc.
	// (a name yields 400 InvalidUUID), so init() resolves the id from the
	// get_or_create response and falls back to the configured name when the
	// server returns no UUID (e.g. older servers or test doubles).
	resource string
	timeout  time.Duration
	client   *http.Client
}

// NewChromaProvider connects to a Chroma server: it probes the heartbeat,
// gets or creates the collection (cosine space) and returns the provider.
// Any failure is returned to the caller, which is expected to disable the
// semantic cache (fail-open).
func NewChromaProvider(cfg Config) (*chromaProvider, error) {
	if cfg.ServiceHost == "" {
		return nil, fmt.Errorf("chroma service host is empty")
	}
	if cfg.ServicePort < 1 || cfg.ServicePort > 65535 {
		return nil, fmt.Errorf("chroma service port %d invalid", cfg.ServicePort)
	}
	if cfg.Collection == "" {
		return nil, fmt.Errorf("chroma collection is empty")
	}
	timeout := cfg.TimeoutMs
	if timeout <= 0 {
		timeout = 300
	}

	p := &chromaProvider{
		base:       fmt.Sprintf("http://%s:%d", cfg.ServiceHost, cfg.ServicePort),
		apiKey:     cfg.ApiKey,
		collection: cfg.Collection,
		resource:   cfg.Collection,
		timeout:    time.Duration(timeout) * time.Millisecond,
		client:     &http.Client{},
	}

	if err := p.init(); err != nil {
		return nil, err
	}
	return p, nil
}

// init probes the heartbeat and gets or creates the collection. When the
// server returns a UUID collection id it becomes the request-path resource
// (Chroma 0.6.x requires the id for query/upsert/count).
func (p *chromaProvider) init() error {
	if err := p.get("/api/v1/heartbeat"); err != nil {
		return fmt.Errorf("chroma heartbeat err: %s", err.Error())
	}

	body, err := json.Marshal(map[string]interface{}{
		"name":          p.collection,
		"metadata":      map[string]string{"hnsw:space": "cosine"},
		"get_or_create": true,
	})
	if err != nil {
		return err
	}
	respBody, err := p.postResponse("/api/v1/collections", body)
	if err != nil {
		return fmt.Errorf("chroma get_or_create collection err: %s", err.Error())
	}
	var coll struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &coll); err == nil {
		if _, err := uuid.Parse(coll.ID); err == nil {
			p.resource = coll.ID
		}
	}
	return nil
}

// whereCond builds the metadata filter of a query. Tenant isolation and the
// TTL filter are always enforced together at the query expression level.
func whereCond(tenant string, ttl time.Duration) map[string]interface{} {
	tenantCond := map[string]interface{}{
		chromaMetaTenant: map[string]interface{}{"$eq": tenant},
	}
	if ttl <= 0 {
		return tenantCond
	}
	return map[string]interface{}{
		"$and": []interface{}{
			tenantCond,
			map[string]interface{}{
				chromaMetaCreatedAt: map[string]interface{}{"$gt": time.Now().Add(-ttl).Unix()},
			},
		},
	}
}

// Query returns the topK nearest records of the embedding within the tenant
// and the ttl. Chroma cosine distance is in [0,2], the smaller the more
// similar; Similarity = 1 - distance.
func (p *chromaProvider) Query(embedding []float32, tenant string, topK int, ttl time.Duration) ([]Result, error) {
	if topK < 1 {
		topK = 1
	}
	body, err := json.Marshal(map[string]interface{}{
		"query_embeddings": [][]float32{embedding},
		"n_results":        topK,
		"where":            whereCond(tenant, ttl),
		"include":          []string{"metadatas", "documents", "distances"},
	})
	if err != nil {
		return nil, err
	}

	respBody, err := p.postResponse(fmt.Sprintf("/api/v1/collections/%s/query", p.resource), body)
	if err != nil {
		return nil, err
	}

	var queryResp struct {
		Ids       [][]string                 `json:"ids"`
		Distances [][]float64                `json:"distances"`
		Documents [][]string                 `json:"documents"`
		Metadatas [][]map[string]interface{} `json:"metadatas"`
	}
	if err := json.Unmarshal(respBody, &queryResp); err != nil {
		return nil, fmt.Errorf("parse query response err: %s", err.Error())
	}
	if len(queryResp.Ids) == 0 || len(queryResp.Ids[0]) == 0 {
		return nil, nil
	}

	results := make([]Result, 0, len(queryResp.Ids[0]))
	for i, id := range queryResp.Ids[0] {
		result := Result{ID: id}
		if len(queryResp.Distances) > 0 && i < len(queryResp.Distances[0]) {
			result.Score = queryResp.Distances[0][i]
			result.Similarity = 1 - result.Score
		}
		if len(queryResp.Documents) > 0 && i < len(queryResp.Documents[0]) {
			result.Question = queryResp.Documents[0][i]
		}
		if len(queryResp.Metadatas) > 0 && i < len(queryResp.Metadatas[0]) {
			if answer, ok := queryResp.Metadatas[0][i][chromaMetaAnswer].(string); ok {
				result.Answer = answer
			}
		}
		results = append(results, result)
	}
	return results, nil
}

// Upload inserts or updates one record; the document is the question text,
// the answer is inlined in the metadata together with tenant_id (mandatory
// filter) and created_at (unix seconds, TTL filter).
func (p *chromaProvider) Upload(item Item) error {
	metadata := map[string]interface{}{
		chromaMetaTenant:    item.Tenant,
		chromaMetaAnswer:    item.Answer,
		chromaMetaCreatedAt: item.CreatedAt.Unix(),
	}
	body, err := json.Marshal(map[string]interface{}{
		"ids":        []string{item.ID},
		"embeddings": [][]float32{item.Embedding},
		"documents":  []string{item.Question},
		"metadatas":  []map[string]interface{}{metadata},
	})
	if err != nil {
		return err
	}

	return p.post(fmt.Sprintf("/api/v1/collections/%s/upsert", p.resource), body)
}

// get issues a GET request and expects status 200.
func (p *chromaProvider) get(path string) error {
	_, err := p.doRequest(http.MethodGet, path, nil)
	return err
}

// post issues a POST request and expects status 200.
func (p *chromaProvider) post(path string, body []byte) error {
	_, err := p.doRequest(http.MethodPost, path, body)
	return err
}

// postResponse issues a POST request and returns the response body.
func (p *chromaProvider) postResponse(path string, body []byte) ([]byte, error) {
	return p.doRequest(http.MethodPost, path, body)
}

func (p *chromaProvider) doRequest(method, path string, body []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, p.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request err: %s", err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request err: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	respBody, err := ioutil.ReadAll(http.MaxBytesReader(nil, resp.Body, maxChromaRespBody))
	if err != nil {
		return nil, fmt.Errorf("read response err: %s", err.Error())
	}
	return respBody, nil
}
