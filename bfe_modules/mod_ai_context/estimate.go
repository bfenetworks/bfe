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

import (
	"encoding/json"
	"strings"
)

// EstimateParams carries the estimation coefficients. They come from the
// merged rule parameters and are passed in on every call, so a hot reload of
// the rule data takes effect immediately; Init does not bake them in.
type EstimateParams struct {
	CharsPerToken      int // bytes per token for text estimation
	ImageTokenEstimate int // fixed token estimate per inline base64 image
}

// TokenEstimator abstracts the token estimation of a messages array. The
// default HeuristicEstimator is used in phase 1; a tiktoken-class
// implementation can replace it later without changing the interface.
type TokenEstimator interface {
	// EstimateMessages estimates the tokens of the whole messages array
	// (inline base64 images are counted at their fixed placeholder estimate)
	EstimateMessages(msgs []Message, p EstimateParams) int64
	// EstimateText estimates the tokens of a plain text string
	EstimateText(s string, p EstimateParams) int64
}

// HeuristicEstimator estimates tokens as len(json)/CharsPerToken, aligned
// with the bfe convention (GetPromptToken, len/4) and OmniRoute's
// estimateTokens. Inline base64 images are not counted by their raw bytes;
// each inline image counts ImageTokenEstimate tokens instead.
type HeuristicEstimator struct{}

func NewHeuristicEstimator() *HeuristicEstimator {
	return &HeuristicEstimator{}
}

func charsPerToken(p EstimateParams) int {
	if p.CharsPerToken < 1 {
		return DefaultCharsPerToken
	}
	return p.CharsPerToken
}

func (e *HeuristicEstimator) EstimateText(s string, p EstimateParams) int64 {
	if s == "" {
		return 0
	}
	cpt := int64(charsPerToken(p))
	return (int64(len(s)) + cpt - 1) / cpt
}

func (e *HeuristicEstimator) EstimateMessages(msgs []Message, p EstimateParams) int64 {
	var total int64
	for i := range msgs {
		total += e.estimateMessage(&msgs[i], p)
	}
	return total
}

func (e *HeuristicEstimator) estimateMessage(m *Message, p EstimateParams) int64 {
	chars := 0
	images := 0

	if s, ok := m.contentString(); ok {
		chars += len(s)
	} else if parts, ok := m.contentParts(); ok {
		for i := range parts {
			if parts[i].isInlineImage() {
				// inline base64 image: fixed placeholder estimate
				images++
				continue
			}
			// text/thinking/remote-url parts: count their serialized length
			if b, err := json.Marshal(&parts[i]); err == nil {
				chars += len(b)
			}
		}
	}

	// role, name, tool ids and tool calls participate as plain text
	chars += len(m.Role) + len(m.Name) + len(m.ToolCallID) + len(m.ReasoningContent)
	if len(m.ToolCalls) > 0 {
		if b, err := json.Marshal(m.ToolCalls); err == nil {
			chars += len(b)
		}
	}

	cpt := int64(charsPerToken(p))
	tokens := (int64(chars) + cpt - 1) / cpt
	if images > 0 {
		tokens += int64(images) * int64(p.ImageTokenEstimate)
	}
	return tokens
}

// isInlineImage reports whether the part is an inline base64 image
// (image_url with a data: URI), as opposed to a remote image reference.
func (p *ContentPart) isInlineImage() bool {
	return p.Type == "image_url" && p.ImageURL != nil && strings.HasPrefix(p.ImageURL.URL, "data:image")
}
