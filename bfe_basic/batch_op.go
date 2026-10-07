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

import (
	"encoding/json"
	"strings"
)

// ClassifyBatchOp maps (method, request path) to a batch operation type. It
// mirrors the endpoint coverage of openAIEndpointModes: the /files and
// /batches entries match their sub-paths by prefix, so /files/{id} and
// /files/{id}/content arrive here with ModeFile, and /batches/{id} and
// /batches/{id}/cancel with ModeBatch. Returns "" for operations not
// intercepted (e.g. DELETE /files/{id}). Shared by mod_ai_batch and
// mod_ai_rate_limit.
func ClassifyBatchOp(method string, path string) string {
	norm := normalizeBatchPath(path)
	segs := strings.Split(strings.Trim(norm, "/"), "/")
	if len(segs) >= 2 && segs[0] == "v1" {
		segs = segs[1:]
	}
	if len(segs) == 0 {
		return ""
	}

	switch segs[0] {
	case "files":
		switch {
		case len(segs) == 1 && method == "POST":
			return BatchOpUpload
		case len(segs) == 2 && method == "GET":
			return BatchOpGet
		case len(segs) == 3 && segs[2] == "content" && method == "GET":
			return BatchOpDownload
		}
	case "batches":
		switch {
		case len(segs) == 1 && method == "POST":
			return BatchOpCreate
		case len(segs) == 1 && method == "GET":
			return BatchOpList
		case len(segs) == 2 && method == "GET":
			return BatchOpGet
		case len(segs) == 3 && segs[2] == "cancel" && method == "POST":
			return BatchOpCancel
		}
	}
	return ""
}

// BatchPathFileId extracts the {id} segment of /files/{id}[/content].
func BatchPathFileId(path string) string {
	segs := strings.Split(strings.Trim(normalizeBatchPath(path), "/"), "/")
	if len(segs) >= 2 && segs[0] == "v1" {
		segs = segs[1:]
	}
	if len(segs) >= 2 && segs[0] == "files" {
		return segs[1]
	}
	return ""
}

// BatchPathBatchId extracts the {id} segment of /batches/{id}[/cancel].
func BatchPathBatchId(path string) string {
	segs := strings.Split(strings.Trim(normalizeBatchPath(path), "/"), "/")
	if len(segs) >= 2 && segs[0] == "v1" {
		segs = segs[1:]
	}
	if len(segs) >= 2 && segs[0] == "batches" {
		return segs[1]
	}
	return ""
}

// normalizeBatchPath reduces provider-native base_url prefixes (e.g.
// /compatible-mode/v1/xxx) to the canonical /v1/xxx form by keeping the
// path from the last "/v1" segment, matching DetectModeFromPath semantics.
func normalizeBatchPath(path string) string {
	idx := strings.LastIndex(path, "/v1/")
	if idx >= 0 {
		return path[idx:]
	}
	return path
}

// ExtractBatchInputFileId parses input_file_id from a buffered batch create
// request body. Returns "" when the body is not fully buffered or does not
// parse. Used by the reverse proxy to derive the batch affinity hint before
// module callbacks run.
func ExtractBatchInputFileId(req *Request) string {
	if req == nil || req.HttpRequest == nil {
		return ""
	}
	accessor, err := req.HttpRequest.GetBodyAccessor()
	if err != nil || accessor == nil {
		return ""
	}
	body, all := accessor.GetBytes()
	if !all || len(body) == 0 {
		return ""
	}
	var v struct {
		InputFileId string `json:"input_file_id"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return ""
	}
	return v.InputFileId
}
