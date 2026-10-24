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

package mod_traffic_mirror

import (
	"bufio"
	"io"
	"strings"

	"github.com/tidwall/gjson"
)

const (
	// sseDoneMarker terminates a server-sent events stream
	sseDoneMarker = "[DONE]"

	// sseContentTypePrefix marks streaming (SSE) responses
	sseContentTypePrefix = "text/event-stream"

	// maxParseBytes caps how much of a non-streaming body is kept in memory
	// for parsing; larger bodies are still drained but not parsed
	maxParseBytes = 1024 * 1024
)

// mirrorRespResult carries the lightweight semantics parsed while draining
// a mirror response. The response body itself is always discarded.
type mirrorRespResult struct {
	FinishReason     string // choices[].finish_reason (last seen)
	ErrorType        string // OpenAI error.type
	ErrorCode        string // OpenAI error.code
	PromptTokens     int64  // usage.prompt_tokens (last seen)
	CompletionTokens int64  // usage.completion_tokens (last seen)
	Bytes            int64  // total bytes drained
	Truncated        bool   // hit the byte limit
	Completed        bool   // stream read to [DONE] or body EOF
}

// drainAndParseBody fully drains the mirror response body while lightly
// parsing usage/error/finish_reason (FR-8). SSE streams are read event by
// event until [DONE]; plain responses are read to EOF. The byte limit caps
// the amount read; the time limit is enforced by the request context, which
// turns an overdue read into an error (treated as incomplete, like EOF
// without [DONE]).
func drainAndParseBody(body io.Reader, contentType string, maxBytes int64) *mirrorRespResult {
	result := &mirrorRespResult{}

	if strings.HasPrefix(contentType, sseContentTypePrefix) {
		drainSSE(body, maxBytes, result)
		return result
	}

	drainPlain(body, maxBytes, result)
	return result
}

// drainPlain reads a non-streaming body to EOF (or the byte limit), keeping
// up to maxParseBytes of payload for JSON parsing.
func drainPlain(body io.Reader, maxBytes int64, result *mirrorRespResult) {
	limited := &io.LimitedReader{R: body, N: maxBytes + 1}

	var payload []byte
	buf := make([]byte, 32*1024)
	for {
		n, err := limited.Read(buf)
		result.Bytes += int64(n)
		if len(payload) < maxParseBytes {
			payload = append(payload, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	if result.Bytes > maxBytes {
		result.Truncated = true
		result.Bytes = maxBytes
		result.Completed = false
		return
	}

	result.Completed = true
	parseJSONPayload(result, payload)
}

// drainSSE reads an SSE stream line by line until [DONE] or EOF, parsing
// each event payload. A blank line terminates one SSE event; an EOF with a
// pending partial event flushes it first.
func drainSSE(body io.Reader, maxBytes int64, result *mirrorRespResult) {
	reader := bufio.NewReaderSize(body, 64*1024)
	var data strings.Builder

	flushEvent := func() {
		payload := strings.TrimSpace(data.String())
		data.Reset()
		if payload == "" {
			return
		}
		if payload == sseDoneMarker {
			result.Completed = true
			return
		}
		parseJSONPayload(result, []byte(payload))
	}

	for {
		line, err := reader.ReadBytes('\n')
		result.Bytes += int64(len(line))
		if result.Bytes > maxBytes {
			result.Truncated = true
			result.Bytes = maxBytes
			result.Completed = false
			return
		}

		trimmed := strings.TrimRight(string(line), "\r\n")
		switch {
		case strings.HasPrefix(trimmed, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(trimmed[len("data:"):]))
		case trimmed == "":
			// blank line terminates one SSE event
			flushEvent()
		default:
			// comment lines (": ...") and field lines are ignored
		}

		if err != nil {
			// EOF or read error: flush a trailing event without terminator
			flushEvent()
			return
		}
	}
}

// parseJSONPayload extracts usage/error/finish_reason from one JSON payload
// (an SSE data line or a whole non-streaming body). The last seen values win.
// Parse failures are ignored by design: the response is drained regardless.
func parseJSONPayload(result *mirrorRespResult, payload []byte) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return
	}

	root := gjson.ParseBytes(payload)

	if usage := root.Get("usage"); usage.Exists() {
		if v := usage.Get("prompt_tokens"); v.Exists() {
			result.PromptTokens = v.Int()
		}
		if v := usage.Get("completion_tokens"); v.Exists() {
			result.CompletionTokens = v.Int()
		}
	}

	if errObj := root.Get("error"); errObj.Exists() {
		result.ErrorType = errObj.Get("type").String()
		result.ErrorCode = errObj.Get("code").String()
	}

	// finish_reason appears per choice; take the last non-empty one
	root.Get("choices").ForEach(func(_, choice gjson.Result) bool {
		if fr := choice.Get("finish_reason"); fr.Exists() && fr.String() != "" {
			result.FinishReason = fr.String()
		}
		return true
	})
}
