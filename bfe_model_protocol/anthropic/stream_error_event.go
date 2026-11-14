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
	"fmt"
	"strconv"
)

// NativeStreamErrorEvent builds an Anthropic-style stream error event: an
// explicit "event: error" block whose data is the {"type":"error",
// "error":{type,message}} envelope (issue #1474).
func (a *Adapter) NativeStreamErrorEvent(message string) []byte {
	return []byte(fmt.Sprintf("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":%s}}\n\n",
		strconv.Quote(message)))
}

// WrapStreamErrorPayload wraps an already-serialized error JSON payload
// (e.g. the unified AiError body) into an Anthropic stream error event.
func (a *Adapter) WrapStreamErrorPayload(payload string) []byte {
	return []byte(fmt.Sprintf("event: error\ndata: %s\n\n", payload))
}

// StreamTerminalMarkers returns the wire-level markers that terminate an
// Anthropic-family stream (issue #1474 truncation detection). Anthropic's
// own termination is event: message_stop; anthropic-compatible servers may
// additionally send the OpenAI-style [DONE] frame.
func (a *Adapter) StreamTerminalMarkers() [][]byte {
	return [][]byte{
		[]byte("event: message_stop"),
		[]byte("data: [DONE]"),
		[]byte("data:[DONE]"),
	}
}
