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
	"strings"

	"github.com/bfenetworks/bfe/bfe_http"
)

// DetectProtocolAndKey extracts the client API key and the request's
// protocol/auth style from the request headers. Authorization: Bearer is
// preferred (the "Bearer " and "sk-" prefixes are stripped); it falls back
// to the x-api-key header (Anthropic style), then to the x-goog-api-key
// header (Gemini style). When no credential header is present it returns
// empty strings.
func DetectProtocolAndKey(req *bfe_http.Request) (protocol, key string) {
	// 1. prefer Authorization: Bearer <key> for OpenAI style
	authHeader := req.Header.Get("Authorization")
	if authHeader != "" {
		// remove "Bearer " prefix if exists
		authHeader = strings.TrimPrefix(authHeader, "Bearer ")
		authHeader = strings.TrimPrefix(authHeader, "sk-")
		return ProtocolOpenAI, authHeader
	}

	// 2. fallback to x-api-key for Anthropic style
	if xApiKey := req.Header.Get("x-api-key"); xApiKey != "" {
		return ProtocolAnthropic, xApiKey
	}

	// 3. fallback to x-goog-api-key for Gemini style
	if xGoogApiKey := req.Header.Get("x-goog-api-key"); xGoogApiKey != "" {
		return ProtocolGemini, xGoogApiKey
	}

	return "", ""
}

// DetectProtocol infers the AI protocol/auth style from request
// characteristics. It is purely based on request path/headers and does not
// rely on user routing rules. A nil request (or one without a URL) reports
// ProtocolUnknown.
func DetectProtocol(req *bfe_http.Request) string {
	if req == nil || req.URL == nil {
		return ProtocolUnknown
	}

	path := req.URL.Path
	if strings.HasPrefix(path, "/v1/messages") {
		return ProtocolAnthropic
	}

	// Gemini style: the generateContent / streamGenerateContent action
	// suffix or the /v1beta/models/ prefix (checked before the openai
	// default below).
	if strings.Contains(path, ":generateContent") ||
		strings.Contains(path, ":streamGenerateContent") ||
		strings.HasPrefix(path, "/v1beta/models/") {
		return ProtocolGemini
	}

	// x-api-key without Authorization indicates Anthropic style
	if req.Header.Get("x-api-key") != "" &&
		req.Header.Get("Authorization") == "" {
		return ProtocolAnthropic
	}

	// x-goog-api-key without Authorization indicates Gemini style
	if req.Header.Get("x-goog-api-key") != "" &&
		req.Header.Get("Authorization") == "" {
		return ProtocolGemini
	}

	return ProtocolOpenAI
}
