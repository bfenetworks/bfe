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
// Code carries the gateway catalog code (e.g. UPSTREAM_RATE_LIMITED); the
// legacy retry booleans are filled for future fault-tolerance use and are
// not consumed by the client-facing normalization path.
type ProtocolError struct {
	Code       string // normalized gateway catalog code
	StatusCode int    // upstream HTTP status code
	IsUpstream bool   // true=upstream error (may swap key / fallback), false=gateway-side error
	Retryable  bool   // retry with the same key after backoff
	SwapKey    bool   // rotate to another key (e.g. 429)
	MarkDead   bool   // mark the key as dead (e.g. 401/402/403)
	Message    string
	// UpstreamCode is the vendor's original error code (OpenAI error.code /
	// Anthropic error.type / Gemini error.status), kept for logging and
	// the error.details.upstream_code field.
	UpstreamCode string
	// Param carries the vendor's error.param when present.
	Param *string
	// RetryAfterSeconds is parsed from the upstream Retry-After header
	// (429 only); 0 when absent.
	RetryAfterSeconds int
}

// ErrorNormalizer parses an upstream error body into a ProtocolError.
// Returning nil means the error was not recognized and the caller falls
// back to its existing handling (the status-code whitelist in phase 1).
// body may be nil.
//
// This is the internal fault-tolerance seam (shouldTriggerFallback); it
// stays decoupled from the client-facing ErrorParser below so that
// enabling client-side normalization never changes fallback semantics.
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

// ErrorParser parses a non-streaming upstream error envelope into a
// ProtocolError. Returning nil means the body was not recognized as a
// protocol error envelope; the caller then applies its configured
// unrecognized-action (pass-through by default). Client-facing
// normalization (AIConf.NormalizeUpstreamError) uses this interface.
type ErrorParser interface {
	ParseError(statusCode int, body []byte, header http.Header) *ProtocolError
}

// StreamErrorParser inspects a single streaming (SSE) event. Returning
// non-nil means the event is an error event and carries the normalized
// form; the caller rewrites the event data payload with the unified error
// body. Returning nil means the event is not an error event and is
// passed through unchanged.
type StreamErrorParser interface {
	ParseStreamError(ev StreamEvent) *ProtocolError
}

// StreamEndsAtEOFReporter is an optional adapter capability: when
// implemented and returning true, the protocol's streams end at HTTP EOF
// and absence of a terminal event must NOT be reported as truncation
// (e.g. Gemini streamGenerateContent).
type StreamEndsAtEOFReporter interface {
	StreamEndsAtEOF() bool
}

// Normalized upstream error catalog codes. The string values mirror the
// AiError catalog in bfe_basic/request_ai_basic.go; they live in this leaf
// package so protocol adapters (which never depend on other BFE packages)
// and the bfe_basic catalog share a single source.
const (
	CodeUpstreamInvalidRequest = "UPSTREAM_INVALID_REQUEST"
	CodeUpstreamRateLimited    = "UPSTREAM_RATE_LIMITED"
	CodeUpstreamQuotaExhausted = "UPSTREAM_QUOTA_EXHAUSTED"
	CodeUpstreamAuthError      = "UPSTREAM_AUTH_ERROR"
	CodeUpstreamModelNotFound  = "UPSTREAM_MODEL_NOT_FOUND"
	CodeUpstreamOverloaded     = "UPSTREAM_OVERLOADED"
	CodeUpstreamUnknown        = "UPSTREAM_UNKNOWN"

	// Activated reserved codes (shared catalog, defined in bfe_basic).
	CodeContextLengthExceeded = "CONTEXT_LENGTH_EXCEEDED"
	CodeContentFiltered       = "CONTENT_FILTERED"
	CodeModelInternalError    = "MODEL_INTERNAL_ERROR"
	CodeBackendTimeout        = "BACKEND_TIMEOUT"
)

// FirstNonEmpty returns the first non-empty string of its arguments.
func FirstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
