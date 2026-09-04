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

package bfe_model_protocol

import (
	"fmt"

	"github.com/bfenetworks/bfe/bfe_model_protocol/anthropic"
	"github.com/bfenetworks/bfe/bfe_model_protocol/openai"
)

// adapters holds the registered protocol adapters. The built-in adapters
// are registered at init time; Register is also the extension point for
// future protocols.
var adapters = map[string]ProtocolAdapter{}

func init() {
	Register(openai.New())
	Register(anthropic.New())
}

// Register adds a protocol adapter to the registry.
func Register(a ProtocolAdapter) {
	adapters[a.Key()] = a
}

// Get returns the adapter for the given protocol. Unknown or empty
// protocol names fall back to the openai adapter, matching the previous
// hard-coded fallback behavior.
func Get(protocol string) ProtocolAdapter {
	if a, ok := adapters[protocol]; ok {
		return a
	}
	return adapters[ProtocolOpenAI]
}

// Supports reports whether protocol p is in the protocols list. An empty
// list defaults to openai only, preserving backward compatibility of
// AIConf.ModelProtocols.
func Supports(protocols []string, p string) bool {
	if len(protocols) == 0 {
		return p == ProtocolOpenAI
	}
	for _, v := range protocols {
		if v == p {
			return true
		}
	}
	return false
}

// ValidateProtocols checks that every value is a protocol known to the
// registry. An empty list is valid (callers default it to ["openai"]).
func ValidateProtocols(protocols []string) error {
	for _, p := range protocols {
		if _, ok := adapters[p]; !ok {
			return fmt.Errorf("unknown model protocol %q", p)
		}
	}
	return nil
}
