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
	"strings"

	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

// IsStreamTerminal carries the legacy hard-coded termination check that
// lived in mod_body_process.SSEEvent.GetQuotaUsage: message_stop ends an
// Anthropic stream. The [DONE] marker is kept as an additional terminator
// so that requests identified as Anthropic behave exactly as before the
// decision moved into the adapter.
func (a *Adapter) IsStreamTerminal(ev utils.StreamEvent) bool {
	return ev.Type == "message_stop" || strings.TrimSpace(ev.Data) == "[DONE]"
}

// IsFinalUsageEvent carries the legacy hard-coded final-usage check:
// Anthropic streams deliver the final usage in the message_delta event,
// the non-streaming body has the top-level type "message", and a
// protocol-agnostic final usage chunk carries no top-level type. The
// usage in message_start is initial only (output_tokens = 0) and its
// event type is not listed here, so it is never treated as final.
func (a *Adapter) IsFinalUsageEvent(ev utils.StreamEvent) bool {
	return ev.Type == "message_delta" || ev.Type == "message" || ev.Type == ""
}
