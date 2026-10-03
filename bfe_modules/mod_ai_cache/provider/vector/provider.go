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

// Package vector abstracts the vector store used by the semantic cache of
// mod_ai_cache.
package vector

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Item is a single cache record written to the vector store.
type Item struct {
	ID        string // deterministic id: sha256(tenant + "\n" + question), upsert is idempotent
	Tenant    string // = ClientKeyId
	Question  string // question text
	Answer    string // final answer (stored inline, returned on semantic hit)
	Embedding []float32
	CreatedAt time.Time
}

// Result is a single nearest neighbor returned by the vector store.
type Result struct {
	ID         string
	Question   string
	Answer     string
	Score      float64 // native score of the store (distance or similarity)
	Similarity float64 // normalized to [0,1], the higher the more similar, for logging only
}

// VectorProvider abstracts a vector store backend. Implementations must
// enforce the configured timeout internally.
type VectorProvider interface {
	// Query returns at most topK nearest records of the embedding. The
	// results must be limited to the tenant, and only records whose
	// created_at is within ttl may be returned (ttl <= 0 means no expiry).
	// Results are ordered best-first; only the first result is judged
	// against the threshold by the caller.
	Query(embedding []float32, tenant string, topK int, ttl time.Duration) ([]Result, error)
	// Upload inserts or updates a record (same id overwrites).
	Upload(item Item) error
}

// ItemID returns the deterministic record id for a (tenant, question) pair:
// sha256(tenant + "\n" + question) in hex. Uploading the same pair twice
// overwrites the same record, which makes the write-back idempotent.
func ItemID(tenant, question string) string {
	sum := sha256.Sum256([]byte(tenant + "\n" + question))
	return hex.EncodeToString(sum[:])
}
