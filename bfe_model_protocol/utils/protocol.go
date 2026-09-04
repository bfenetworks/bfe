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

// Package utils holds protocol-neutral helpers shared by the model-protocol
// adapters: protocol name constants, the neutral usage result structure,
// upstream-error normalization types and the gjson usage extraction chains.
// It intentionally depends only on the standard library and third-party
// packages so that both bfe_model_protocol and its adapter subpackages can
// import it without cycles.
package utils

// Model protocol identifiers.
const (
	ProtocolOpenAI    = "openai"
	ProtocolAnthropic = "anthropic"
	ProtocolUnknown   = "unknown"
)
