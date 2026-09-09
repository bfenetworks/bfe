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
	"github.com/tidwall/gjson"

	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

// IsStreamTerminal always returns false: Gemini streams
// (streamGenerateContent) carry no SSE termination event; the response
// ends at HTTP stream EOF, which the callers already handle as the
// completion fallback.
func (a *Adapter) IsStreamTerminal(ev utils.StreamEvent) bool {
	return false
}

// IsFinalUsageEvent reports whether the event data carries a
// usageMetadata object. Every Gemini streaming chunk carries the
// accumulated usage, so this is true for (intermediate and) final chunks
// alike; the caller's "last event wins" accumulation guarantees the
// final chunk's values are the ones billed, while a chunk without
// usageMetadata (e.g. a bare content candidate) is never mistaken for a
// usage event.
func (a *Adapter) IsFinalUsageEvent(ev utils.StreamEvent) bool {
	return gjson.Get(ev.Data, "usageMetadata").Exists()
}
