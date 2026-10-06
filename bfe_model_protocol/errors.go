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
	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

// ProtocolError is the normalized form of an upstream error response. The
// canonical definition lives in the utils sub-package; it is re-exported
// here so callers only need to import bfe_model_protocol.
type ProtocolError = utils.ProtocolError

// ErrorNormalizer parses an upstream error body into a ProtocolError.
// The canonical definition lives in the utils sub-package; it is
// re-exported here so callers only need to import bfe_model_protocol.
type ErrorNormalizer = utils.ErrorNormalizer

// ErrorParser parses a non-streaming upstream error envelope into a
// ProtocolError (client-facing normalization path).
type ErrorParser = utils.ErrorParser

// StreamErrorParser inspects a single SSE event for error semantics.
type StreamErrorParser = utils.StreamErrorParser

// StreamEndsAtEOFReporter is the optional adapter capability marking
// protocols whose streams end at HTTP EOF (no truncation reporting).
type StreamEndsAtEOFReporter = utils.StreamEndsAtEOFReporter

// Normalized upstream error catalog codes (single source in utils).
const (
	CodeUpstreamInvalidRequest = utils.CodeUpstreamInvalidRequest
	CodeUpstreamRateLimited    = utils.CodeUpstreamRateLimited
	CodeUpstreamQuotaExhausted = utils.CodeUpstreamQuotaExhausted
	CodeUpstreamAuthError      = utils.CodeUpstreamAuthError
	CodeUpstreamModelNotFound  = utils.CodeUpstreamModelNotFound
	CodeUpstreamOverloaded     = utils.CodeUpstreamOverloaded
	CodeUpstreamUnknown        = utils.CodeUpstreamUnknown

	CodeContextLengthExceeded = utils.CodeContextLengthExceeded
	CodeContentFiltered       = utils.CodeContentFiltered
	CodeModelInternalError    = utils.CodeModelInternalError
	CodeBackendTimeout        = utils.CodeBackendTimeout
)
