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

package bfe_basic

// AiBatchRouteHint carries the batch/file identifiers of the current request
// from mod_ai_batch (which classifies the operation) to the reverse proxy
// (which performs batch-level key affinity in chooseAIKeyWithAffinity). It
// lives in bfe_basic so that bfe_server does not import module packages.
type AiBatchRouteHint struct {
	BatchId string // batch_xxx for /v1/batches/{id}[...] operations
	FileId  string // file-xxx for /v1/files/{id}[...] operations
}

type ctxKeyAiBatchRouteHint struct{}

// AIKeyAffinityBatchPrefix is the Redis namespace of batch-level key
// affinity bindings, written by mod_ai_batch and read by
// chooseAIKeyWithAffinity (bfe_server). It intentionally does not follow the
// configurable [AIKeyAffinity] prefix: batch bindings are an independent
// namespace so the two mechanisms cannot shadow each other.
const AIKeyAffinityBatchPrefix = "bfe:ai:key_affinity:batch"

// BatchAffinityKey builds the binding key for a batch or file identifier.
func BatchAffinityKey(cluster, id string) string {
	return AIKeyAffinityBatchPrefix + ":" + cluster + ":" + id
}

// SetAiBatchRouteHint attaches the hint to the request context.
func (req *Request) SetAiBatchRouteHint(hint *AiBatchRouteHint) {
	if hint == nil {
		return
	}
	req.SetContext(ctxKeyAiBatchRouteHint{}, hint)
}

// GetAiBatchRouteHint returns the hint set by mod_ai_batch, or nil.
func (req *Request) GetAiBatchRouteHint() *AiBatchRouteHint {
	if v := req.GetContext(ctxKeyAiBatchRouteHint{}); v != nil {
		if hint, ok := v.(*AiBatchRouteHint); ok {
			return hint
		}
	}
	return nil
}
