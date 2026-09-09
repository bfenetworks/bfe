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
	"strings"

	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

// IsStreamTerminal carries the legacy hard-coded termination check that
// lived in mod_body_process.SSEEvent.GetQuotaUsage: an Anthropic-style
// message_stop event or the OpenAI [DONE] marker ends the stream. The
// OpenAI adapter is also the registry fallback (unknown/empty auth
// styles), so it keeps the full combined rule to preserve the previous
// behavior for every request that does not identify a concrete protocol.
func (a *Adapter) IsStreamTerminal(ev utils.StreamEvent) bool {
	return ev.Type == "message_stop" || strings.TrimSpace(ev.Data) == "[DONE]"
}

// IsFinalUsageEvent carries the legacy hard-coded final-usage check: the
// Anthropic non-streaming top-level type "message", or a protocol-agnostic
// final usage chunk (an OpenAI stream_options.include_usage chunk has no
// top-level type). Without "message" the final usage of a non-streaming
// Anthropic JSON body would never be recognized (issue #1364).
func (a *Adapter) IsFinalUsageEvent(ev utils.StreamEvent) bool {
	return ev.Type == "message" || ev.Type == ""
}
