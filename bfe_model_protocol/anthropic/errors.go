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

package anthropic

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

// ErrorParser returns the Anthropic error parser for client-facing
// normalization (AIConf.NormalizeUpstreamError).
func (a *Adapter) ErrorParser() utils.ErrorParser {
	return a
}

// StreamErrorParser returns the per-event SSE error parser.
func (a *Adapter) StreamErrorParser() utils.StreamErrorParser {
	return a
}

// ParseError parses an Anthropic error envelope
// ({"type":"error","error":{type,message}}) into a normalized
// ProtocolError. Returning nil means the body is not a recognizable
// Anthropic error envelope and the caller applies its
// unrecognized-action.
func (a *Adapter) ParseError(statusCode int, body []byte, header http.Header) *utils.ProtocolError {
	if statusCode < 400 {
		return nil
	}
	if !strings.EqualFold(gjson.GetBytes(body, "type").String(), "error") {
		return nil
	}
	env := gjson.GetBytes(body, "error")
	if !env.Exists() || !env.IsObject() {
		return nil
	}
	typ := env.Get("type").String()

	perr := &utils.ProtocolError{
		Code:         mapError(statusCode, typ),
		StatusCode:   statusCode,
		IsUpstream:   true,
		Message:      env.Get("message").String(),
		UpstreamCode: typ,
	}
	if statusCode == 429 && header != nil {
		if secs, err := strconv.Atoi(strings.TrimSpace(header.Get("Retry-After"))); err == nil && secs > 0 {
			perr.RetryAfterSeconds = secs
		}
	}
	return perr
}

// ParseStreamError inspects one SSE event. Anthropic signals in-stream
// errors with an error event whose data payload carries the top-level
// type "error". The HTTP status of a stream is 200; the mapping is driven
// by the envelope's error.type field.
func (a *Adapter) ParseStreamError(ev utils.StreamEvent) *utils.ProtocolError {
	if ev.Type != "error" {
		return nil
	}
	env := gjson.Get(ev.Data, "error")
	if !env.Exists() || !env.IsObject() {
		return nil
	}
	typ := env.Get("type").String()

	return &utils.ProtocolError{
		Code:         mapError(200, typ),
		StatusCode:   200,
		IsUpstream:   true,
		Message:      env.Get("message").String(),
		UpstreamCode: typ,
	}
}

// mapError maps an Anthropic error type (the error envelope carries no
// numeric code) to the unified gateway catalog. The statusCode fallback
// covers non-streaming responses whose type is unexpected.
func mapError(statusCode int, typ string) string {
	switch typ {
	case "invalid_request_error":
		return utils.CodeUpstreamInvalidRequest
	case "request_too_large":
		return utils.CodeContextLengthExceeded
	case "authentication_error", "permission_error":
		return utils.CodeUpstreamAuthError
	case "not_found_error":
		return utils.CodeUpstreamModelNotFound
	case "rate_limit_error":
		return utils.CodeUpstreamRateLimited
	case "overloaded_error":
		return utils.CodeUpstreamOverloaded
	case "api_error":
		return utils.CodeModelInternalError
	}
	switch statusCode {
	case 401, 402, 403:
		return utils.CodeUpstreamAuthError
	case 404:
		return utils.CodeUpstreamModelNotFound
	case 408, 504:
		return utils.CodeBackendTimeout
	case 429:
		return utils.CodeUpstreamRateLimited
	case 500, 501, 502, 503, 505:
		return utils.CodeModelInternalError
	default:
		return utils.CodeUpstreamInvalidRequest
	}
}
