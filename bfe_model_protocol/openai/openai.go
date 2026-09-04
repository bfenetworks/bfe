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

// Package openai implements the model_protocol adapter for the OpenAI
// protocol family (OpenAI chat/completions, DeepSeek, Groq and the
// OpenAI Responses API).
package openai

import (
	"github.com/bfenetworks/bfe/bfe_model_protocol/utils"
)

// Adapter is the OpenAI-family protocol adapter. It is stateless.
type Adapter struct{}

// New returns the OpenAI-family protocol adapter.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Key() string {
	return utils.ProtocolOpenAI
}

// ExtraHeaders returns nil: the OpenAI protocol needs no supplementary
// headers.
func (a *Adapter) ExtraHeaders() map[string]string {
	return nil
}

// ErrorNormalizer returns the phase-1 default normalizer (never recognizes
// errors, callers keep their status-code whitelist).
func (a *Adapter) ErrorNormalizer() utils.ErrorNormalizer {
	return utils.DefaultErrorNormalizer{}
}
