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

package mod_ai_context

// FidelityGate is the P2 quality gate (design 7.4): the rewrite output is
// accepted only when the token-weighted survival rate of the tombstone
// protected fragments reaches the configured threshold; otherwise the
// pipeline falls back to the lossless trim-layer product (GATE_FALLBACK).
type FidelityGate struct {
	threshold float64
}

func NewFidelityGate(threshold float64) *FidelityGate {
	return &FidelityGate{threshold: threshold}
}

// Pass reports whether the protected content survived the rewrite well
// enough. A conversation without protected content always passes.
func (g *FidelityGate) Pass(survivedTokens, totalTokens int64) bool {
	if totalTokens <= 0 {
		return true
	}
	threshold := g.threshold
	if threshold <= 0 {
		threshold = DefaultProtectedSurvivalRate
	}
	return float64(survivedTokens)/float64(totalTokens) >= threshold
}
