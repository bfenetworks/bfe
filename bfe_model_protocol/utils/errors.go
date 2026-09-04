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

package utils

import (
	"net/http"
)

// ProtocolError is the normalized form of an upstream error response.
type ProtocolError struct {
	Code       string // auth_invalid | rate_limited | model_not_found | server_error | unknown
	StatusCode int    // upstream HTTP status code
	IsUpstream bool   // true=upstream error (may swap key / fallback), false=gateway-side error
	Retryable  bool   // retry with the same key after backoff
	SwapKey    bool   // rotate to another key (e.g. 429)
	MarkDead   bool   // mark the key as dead (e.g. 401/402/403)
	Message    string
}

// ErrorNormalizer parses an upstream error body into a ProtocolError.
// Returning nil means the error was not recognized and the caller falls
// back to its existing handling (the status-code whitelist in phase 1).
// body may be nil.
type ErrorNormalizer interface {
	Normalize(statusCode int, body []byte, header http.Header) *ProtocolError
}

// DefaultErrorNormalizer is the phase-1 error normalizer: it never
// recognizes an error, so callers keep using their existing status-code
// whitelist. Protocol adapters return it until a protocol-specific
// normalization is implemented.
type DefaultErrorNormalizer struct{}

func (DefaultErrorNormalizer) Normalize(statusCode int, body []byte, header http.Header) *ProtocolError {
	return nil
}
