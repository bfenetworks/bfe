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
	"fmt"
	"strconv"
)

// streamTruncatedCode is the error.code used in gateway-injected stream
// error events (issue #1474).
const streamTruncatedCode = "upstream_stream_truncated"

// NativeStreamErrorEvent builds an OpenAI-style stream error event:
// the data payload is the {"error":{message,type,param,code}} envelope with
// type "server_error" and no event: line (OpenAI chat streams are untyped).
func (a *Adapter) NativeStreamErrorEvent(message string) []byte {
	return []byte(fmt.Sprintf("data: {\"error\":{\"message\":%s,\"type\":\"server_error\",\"param\":null,\"code\":%s}}\n\n",
		strconv.Quote(message), strconv.Quote(streamTruncatedCode)))
}

// WrapStreamErrorPayload wraps an already-serialized error JSON payload
// (e.g. the unified AiError body) into an OpenAI stream error event.
func (a *Adapter) WrapStreamErrorPayload(payload string) []byte {
	return []byte(fmt.Sprintf("data: %s\n\n", payload))
}

// StreamTerminalMarkers returns the wire-level markers that terminate an
// OpenAI-family stream (issue #1474 truncation detection).
func (a *Adapter) StreamTerminalMarkers() [][]byte {
	return [][]byte{[]byte("data: [DONE]"), []byte("data:[DONE]")}
}
