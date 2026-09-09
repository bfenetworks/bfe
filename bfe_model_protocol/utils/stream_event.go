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

package utils

// StreamEvent is the protocol-neutral view of a single streaming (SSE)
// event used by the adapters' stream-termination and final-usage
// decisions. Type is the top-level "type" field of the event data payload
// ("" when absent, e.g. OpenAI usage chunks); Data is the raw event data.
// It deliberately lives in this leaf package so that ProtocolAdapter
// implementations never depend on BFE module types such as
// mod_body_process.SSEEvent.
type StreamEvent struct {
	Type string
	Data string
}
