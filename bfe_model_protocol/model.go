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

package bfe_model_protocol

import (
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_model_protocol/gemini"
)

// ExtractModelFromPath returns the model name carried in the request path for
// protocols that put it there (currently only Gemini:
// /v1beta/models/{model}[:action], see issue #1384). It returns "" for
// protocols that carry the model in the request body (openai/anthropic), for
// unrecognized paths, and for nil requests. This is the single entry point
// for path-based model extraction; future protocols with the same trait
// extend it here.
func ExtractModelFromPath(req *bfe_http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	return gemini.ExtractModelFromPath(req.URL.Path)
}
