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
	"testing"
)

func TestGetKnownProtocols(t *testing.T) {
	if got := Get(ProtocolOpenAI).Key(); got != ProtocolOpenAI {
		t.Errorf("Get(openai).Key() = %q", got)
	}
	if got := Get(ProtocolAnthropic).Key(); got != ProtocolAnthropic {
		t.Errorf("Get(anthropic).Key() = %q", got)
	}
}

func TestGetUnknownFallsBackToOpenAI(t *testing.T) {
	// Unknown / empty protocol names fall back to the openai adapter,
	// matching the previous hard-coded fallback behavior.
	for _, p := range []string{"", "gemini", ProtocolUnknown} {
		if got := Get(p).Key(); got != ProtocolOpenAI {
			t.Errorf("Get(%q).Key() = %q, want openai", p, got)
		}
	}
}

func TestSupports(t *testing.T) {
	if !Supports(nil, ProtocolOpenAI) {
		t.Error("empty list should default to supporting openai")
	}
	if Supports(nil, ProtocolAnthropic) {
		t.Error("empty list should not support anthropic")
	}
	if !Supports([]string{ProtocolOpenAI, ProtocolAnthropic}, ProtocolAnthropic) {
		t.Error("anthropic should be supported when listed")
	}
	if Supports([]string{ProtocolAnthropic}, ProtocolOpenAI) {
		t.Error("openai should not be supported when only anthropic is listed")
	}
}

func TestValidateProtocols(t *testing.T) {
	if err := ValidateProtocols(nil); err != nil {
		t.Errorf("empty list should be valid, got %v", err)
	}
	if err := ValidateProtocols([]string{}); err != nil {
		t.Errorf("empty list should be valid, got %v", err)
	}
	if err := ValidateProtocols([]string{ProtocolOpenAI, ProtocolAnthropic}); err != nil {
		t.Errorf("known protocols should be valid, got %v", err)
	}
	if err := ValidateProtocols([]string{ProtocolOpenAI, "gemini"}); err == nil {
		t.Error("unknown protocol should be rejected")
	}
}

func TestErrorNormalizerDefaultNil(t *testing.T) {
	// Phase 1: every adapter's default normalizer returns nil so callers
	// keep their existing status-code whitelist.
	for _, p := range []string{ProtocolOpenAI, ProtocolAnthropic} {
		if pe := Get(p).ErrorNormalizer().Normalize(429, nil, nil); pe != nil {
			t.Errorf("default normalizer should return nil, got %+v", pe)
		}
	}
}
