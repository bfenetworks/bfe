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

// Package bfe_model_protocol hosts the per-protocol (model_protocol)
// adapters that carry all protocol knowledge of the AI gateway: auth header
// injection, supplementary headers (e.g. anthropic-version), usage field
// extraction and upstream error normalization. Adding a new protocol means
// adding one detection rule in detect.go plus one adapter package
// registered in registry.go, without touching the callers.
//
// The concrete UsageFields / ProtocolError / ErrorNormalizer types live in
// the utils sub-package (a leaf with no BFE dependencies) so that adapter
// subpackages can reference them without import cycles; this package
// re-exports them via aliases.
package bfe_model_protocol

import (
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

// Model protocol identifiers.
const (
	ProtocolOpenAI    = utils.ProtocolOpenAI
	ProtocolAnthropic = utils.ProtocolAnthropic
	ProtocolGemini    = utils.ProtocolGemini
	ProtocolUnknown   = utils.ProtocolUnknown
)

// StreamEvent is the protocol-neutral view of a single streaming (SSE)
// event. The canonical definition lives in the utils sub-package; it is
// re-exported here so callers only need to import bfe_model_protocol.
type StreamEvent = utils.StreamEvent

// ProtocolAdapter carries all protocol knowledge of one model_protocol.
// Adapters are stateless singletons held by the compile-time registry;
// per-cluster differences (which key to use) are passed via method
// arguments and never stored on the adapter.
type ProtocolAdapter interface {
	Key() string

	// InjectAuth writes the upstream credential into the outgoing request.
	// An empty key is a no-op.
	InjectAuth(outreq *bfe_http.Request, key string) error

	// ExtraHeaders returns protocol-level supplementary headers (e.g.
	// anthropic-version). It returns nil when there are none.
	ExtraHeaders() map[string]string

	// ExtractUsageFields extracts usage fields from one response body
	// (SSE event data or a complete non-streaming body).
	ExtractUsageFields(data []byte) UsageFields

	// ErrorNormalizer returns the upstream error normalizer for this
	// protocol. The phase-1 default never recognizes errors, so callers
	// keep their existing status-code whitelist.
	ErrorNormalizer() ErrorNormalizer

	// IsStreamTerminal reports whether the given event terminates the
	// response stream (e.g. Anthropic message_stop, OpenAI [DONE]).
	// Protocols without an SSE termination event (Gemini) always return
	// false; their streams end at HTTP EOF, which the callers already
	// handle as a fallback.
	IsStreamTerminal(ev StreamEvent) bool

	// IsFinalUsageEvent reports whether the given event carries the final
	// usage of the response. Callers only consult it for events that
	// already parsed a non-guess usage, and only mark the final usage when
	// completion tokens are present.
	IsFinalUsageEvent(ev StreamEvent) bool
}
