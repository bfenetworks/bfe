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

package openai

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

// ErrorParser returns the OpenAI-family error parser for client-facing
// normalization (AIConf.NormalizeUpstreamError).
func (a *Adapter) ErrorParser() utils.ErrorParser {
	return a
}

// StreamErrorParser returns the per-event SSE error parser.
func (a *Adapter) StreamErrorParser() utils.StreamErrorParser {
	return a
}

// ParseError parses an OpenAI-style error envelope
// ({"error":{message,type,param,code}}) into a normalized ProtocolError.
// code may be a string or a nested object (some providers nest it); a
// nested object is ignored and the mapping falls back to type/status.
// Returning nil means the body is not a recognizable OpenAI error
// envelope and the caller applies its unrecognized-action.
func (a *Adapter) ParseError(statusCode int, body []byte, header http.Header) *utils.ProtocolError {
	if statusCode < 400 {
		return nil
	}
	env := gjson.GetBytes(body, "error")
	if !env.Exists() || !env.IsObject() {
		return nil
	}
	typ := env.Get("type").String()
	code := env.Get("code").String()
	if env.Get("code").IsObject() {
		code = ""
	}
	param := env.Get("param").String()

	perr := &utils.ProtocolError{
		Code:         mapError(statusCode, code, typ),
		StatusCode:   statusCode,
		IsUpstream:   true,
		Message:      env.Get("message").String(),
		UpstreamCode: utils.FirstNonEmpty(code, typ),
	}
	if param != "" {
		perr.Param = &param
	}
	if statusCode == 429 && header != nil {
		if secs, err := strconv.Atoi(strings.TrimSpace(header.Get("Retry-After"))); err == nil && secs > 0 {
			perr.RetryAfterSeconds = secs
		}
	}
	return perr
}

// ParseStreamError inspects one SSE event. OpenAI signals in-stream
// errors with an event whose data is an {"error":{...}} object (the
// OpenAI adapter is also the registry fallback, so this check doubles as
// the generic rule for unknown protocols). The HTTP status of a stream is
// 200; the mapping is driven by the envelope's code/type fields.
func (a *Adapter) ParseStreamError(ev utils.StreamEvent) *utils.ProtocolError {
	data := strings.TrimSpace(ev.Data)
	if data == "" || data == "[DONE]" {
		return nil
	}
	env := gjson.Get(data, "error")
	if !env.Exists() || !env.IsObject() {
		return nil
	}
	typ := env.Get("type").String()
	code := env.Get("code").String()
	if env.Get("code").IsObject() {
		code = ""
	}
	param := env.Get("param").String()

	perr := &utils.ProtocolError{
		Code:         mapError(200, code, typ),
		StatusCode:   200,
		IsUpstream:   true,
		Message:      env.Get("message").String(),
		UpstreamCode: utils.FirstNonEmpty(code, typ),
	}
	if param != "" {
		perr.Param = &param
	}
	return perr
}

// mapError maps an OpenAI error (status code + error.code/error.type) to
// the unified gateway catalog. statusCode-driven branches handle
// non-streaming responses; code/type-driven branches handle streaming
// error events where the HTTP status is 200.
func mapError(statusCode int, code, typ string) string {
	switch code {
	case "context_length_exceeded":
		return utils.CodeContextLengthExceeded
	case "insufficient_quota":
		return utils.CodeUpstreamQuotaExhausted
	case "model_not_found", "does_not_exist":
		return utils.CodeUpstreamModelNotFound
	case "content_policy_violation", "content_filter":
		return utils.CodeContentFiltered
	case "rate_limit_exceeded":
		return utils.CodeUpstreamRateLimited
	}
	switch typ {
	case "authentication_error", "permission_error":
		return utils.CodeUpstreamAuthError
	case "rate_limit_error":
		return utils.CodeUpstreamRateLimited
	case "overloaded_error":
		return utils.CodeUpstreamOverloaded
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
		// covers 400/422 non-streaming and streaming error events
		// (status 200) without a specific code
		return utils.CodeUpstreamInvalidRequest
	}
}
