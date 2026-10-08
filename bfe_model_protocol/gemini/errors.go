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

package gemini

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

// ErrorParser returns the Gemini error parser for client-facing
// normalization (AIConf.NormalizeUpstreamError).
func (a *Adapter) ErrorParser() utils.ErrorParser {
	return a
}

// StreamErrorParser returns the per-event SSE error parser.
func (a *Adapter) StreamErrorParser() utils.StreamErrorParser {
	return a
}

// StreamEndsAtEOF reports that Gemini streams (streamGenerateContent)
// carry no SSE termination event: the response ends at HTTP stream EOF,
// so the absence of a terminal event must not be reported as truncation.
func (a *Adapter) StreamEndsAtEOF() bool {
	return true
}

// ParseError parses a Gemini error envelope
// ({"error":{code,message,status}}) into a normalized ProtocolError; the
// google.rpc.Status "status" enum is the primary mapping key. Returning
// nil means the body is not a recognizable Gemini error envelope and the
// caller applies its unrecognized-action.
func (a *Adapter) ParseError(statusCode int, body []byte, header http.Header) *utils.ProtocolError {
	if statusCode < 400 {
		return nil
	}
	env := gjson.GetBytes(body, "error")
	if !env.Exists() || !env.IsObject() {
		return nil
	}
	status := env.Get("status").String()
	msg := env.Get("message").String()

	perr := &utils.ProtocolError{
		Code:         mapError(statusCode, status, msg),
		StatusCode:   statusCode,
		IsUpstream:   true,
		Message:      msg,
		UpstreamCode: utils.FirstNonEmpty(status, env.Get("code").String()),
	}
	if statusCode == 429 && header != nil {
		if secs, err := strconv.Atoi(strings.TrimSpace(header.Get("Retry-After"))); err == nil && secs > 0 {
			perr.RetryAfterSeconds = secs
		}
	}
	return perr
}

// ParseStreamError inspects one SSE event. Gemini signals in-stream
// errors with a chunk whose data carries an "error" object. The HTTP
// status of a stream is 200; the mapping is driven by the error.status
// enum.
func (a *Adapter) ParseStreamError(ev utils.StreamEvent) *utils.ProtocolError {
	data := strings.TrimSpace(ev.Data)
	if data == "" {
		return nil
	}
	env := gjson.Get(data, "error")
	if !env.Exists() || !env.IsObject() {
		return nil
	}
	status := env.Get("status").String()
	msg := env.Get("message").String()

	return &utils.ProtocolError{
		Code:         mapError(200, status, msg),
		StatusCode:   200,
		IsUpstream:   true,
		Message:      msg,
		UpstreamCode: status,
	}
}

// mapError maps a google.rpc.Status enum to the unified gateway catalog.
// RESOURCE_EXHAUSTED is disambiguated by the message: quota exhaustion
// mentions "quota"/"balance", everything else is treated as rate
// limiting. The statusCode fallback covers envelopes without a status
// enum.
func mapError(statusCode int, rpcStatus, msg string) string {
	switch rpcStatus {
	case "INVALID_ARGUMENT", "FAILED_PRECONDITION", "OUT_OF_RANGE":
		return utils.CodeUpstreamInvalidRequest
	case "UNAUTHENTICATED", "PERMISSION_DENIED":
		return utils.CodeUpstreamAuthError
	case "NOT_FOUND":
		return utils.CodeUpstreamModelNotFound
	case "RESOURCE_EXHAUSTED":
		lmsg := strings.ToLower(msg)
		if strings.Contains(lmsg, "quota") || strings.Contains(lmsg, "balance") {
			return utils.CodeUpstreamQuotaExhausted
		}
		return utils.CodeUpstreamRateLimited
	case "DEADLINE_EXCEEDED":
		return utils.CodeBackendTimeout
	case "UNAVAILABLE", "INTERNAL", "DATA_LOSS", "UNKNOWN":
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
