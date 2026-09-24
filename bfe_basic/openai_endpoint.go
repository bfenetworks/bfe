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
	"strings"
)

// openAIEndpointModes maps OpenAI API endpoints (in the form with any /v1
// version prefix already stripped) to their billing modes. Endpoints without
// a dedicated price mode ("/models", "/moderations", "/audio/translations")
// map to ModeChat, preserving the default-mode semantics of DetectModeFromPath.
//
// This table is the single definition of "what an OpenAI standard endpoint
// is": the upstream path rewrite (bfe_server) and the billing mode detection
// (DetectModeFromPath) both consult it, so the two can never disagree on
// endpoint compatibility (e.g. an endpoint accepted by the rewrite but
// mis-modeled for billing, or vice versa).
var openAIEndpointModes = map[string]string{
	"/audio/speech":         ModeAudioSpeech,
	"/audio/transcriptions": ModeAudioTranscription,
	"/audio/translations":   ModeChat, // no dedicated mode; same as the default
	"/chat/completions":     ModeChat,
	"/completions":          ModeCompletion,
	"/embeddings":           ModeEmbedding,
	"/images/edits":         ModeImageEdit,
	"/images/generations":   ModeImageGeneration,
	"/models":               ModeChat, // no dedicated mode; same as the default
	"/moderations":          ModeChat, // no dedicated mode; same as the default
	"/responses":            ModeResponses,
	"/rerank":               ModeRerank,
	"/video/generations":    ModeVideoGeneration,
}

// StripV1Prefix strips a leading "/v1" version prefix from an OpenAI-style
// request path: "/v1/chat/completions" -> "/chat/completions"; paths without
// the prefix (including "/v10/xxx" and provider-native paths such as
// "/compatible-mode/v1/chat/completions") are returned unchanged. Shared by
// DetectModeFromPath and the bfe_server path rewrite so both treat the
// optional /v1 entry prefix identically.
func StripV1Prefix(path string) string {
	if strings.HasPrefix(path, "/v1/") {
		return path[len("/v1"):]
	}
	return path
}

// IsOpenAIEndpoint reports whether path (with any /v1 prefix already stripped)
// is a recognized OpenAI API endpoint, i.e. listed in openAIEndpointModes.
// Used by the bfe_server upstream path rewrite to decide whether the
// configured base path applies.
func IsOpenAIEndpoint(path string) bool {
	_, ok := lookupOpenAIEndpointMode(path)
	return ok
}

// lookupOpenAIEndpointMode returns the billing mode of a recognized OpenAI
// endpoint path, matching either the exact endpoint or an endpoint subpath
// (e.g. "/models/{model}"). The second return value reports whether the path
// matched any endpoint.
func lookupOpenAIEndpointMode(path string) (string, bool) {
	if mode, ok := openAIEndpointModes[path]; ok {
		return mode, true
	}
	for ep, mode := range openAIEndpointModes {
		if strings.HasPrefix(path, ep+"/") {
			return mode, true
		}
	}
	return "", false
}

// normalizeEndpointLookupPath reduces a client entry path to its OpenAI
// endpoint form for billing-mode lookup: a leading /v1 is stripped
// (standard entry), and a provider-native prefix ending in a /v1 segment
// (the OpenAI SDK base_url form, e.g. "/compatible-mode/v1", issue #1382)
// is reduced to the part after that segment. Mode detection only; the
// upstream path rewrite keeps its own StripV1Prefix semantics so
// provider-native paths keep passing through unchanged.
func normalizeEndpointLookupPath(path string) string {
	if rest := StripV1Prefix(path); rest != path {
		return rest
	}
	if i := strings.Index(path, "/v1/"); i >= 0 {
		return StripV1Prefix(path[i+len("/v1"):])
	}
	return path
}
