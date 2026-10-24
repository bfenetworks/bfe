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
	"strings"
	"unicode/utf8"
)

// lossless trim layers of the degradation pipeline (design-changes.md 7.4).
// All layers skip the last user message (cache key stability, design 7.3) and
// never touch system messages.

const (
	// toolTruncatedMarker is appended to a truncated tool result
	toolTruncatedMarker = "...[truncated]"

	// imageRemovedPlaceholder replaces dropped inline images
	imageRemovedPlaceholder = "[Earlier image removed to fit context window]"
)

// trimToolResults is L1: truncate each tool result (role=tool message) to
// maxChars runes, appending the truncated marker. maxChars <= 0 disables the
// layer. Returns whether any message changed.
func trimToolResults(msgs []Message, maxChars int) bool {
	if maxChars <= 0 {
		return false
	}
	changed := false
	for i := range msgs {
		if msgs[i].Role != "tool" {
			continue
		}
		changed = truncateMessageContent(&msgs[i], maxChars) || changed
	}
	return changed
}

// truncateMessageContent truncates the text content of a single message.
// Both plain-string content and text parts of typed-parts content are
// handled; non-text parts are left alone.
func truncateMessageContent(m *Message, maxChars int) bool {
	if s, ok := m.contentString(); ok {
		if runeLen(s) <= maxChars {
			return false
		}
		m.setContentString(truncateRunes(s, maxChars) + toolTruncatedMarker)
		return true
	}

	parts, ok := m.contentParts()
	if !ok {
		return false
	}
	changed := false
	for j := range parts {
		if parts[j].Type != "text" || runeLen(parts[j].Text) <= maxChars {
			continue
		}
		parts[j].Text = truncateRunes(parts[j].Text, maxChars) + toolTruncatedMarker
		changed = true
	}
	if changed {
		m.setContentParts(parts)
	}
	return changed
}

// trimOldImages is L1.5: keep only the latest keepLatest inline images of
// the conversation; older inline images are replaced by a placeholder text
// part. keepLatest <= 0 disables the layer (design 4.2: 0 = no image trim).
// The protected message (last user message) is never touched, so its images
// always survive. Returns whether any message changed.
func trimOldImages(msgs []Message, keepLatest int, protectedIdx int) bool {
	if keepLatest <= 0 {
		return false
	}
	changed := false
	kept := 0
	// scan from the newest message so "latest" means latest in the array
	for i := len(msgs) - 1; i >= 0; i-- {
		if i == protectedIdx {
			continue
		}
		parts, ok := msgs[i].contentParts()
		if !ok {
			continue
		}
		msgChanged := false
		for j := len(parts) - 1; j >= 0; j-- {
			if !parts[j].isInlineImage() {
				continue
			}
			if kept < keepLatest {
				kept++
				continue
			}
			parts[j] = ContentPart{Type: "text", Text: imageRemovedPlaceholder}
			msgChanged = true
		}
		if msgChanged {
			msgs[i].setContentParts(parts)
			changed = true
		}
	}
	return changed
}

// trimThinkingBlocks is L2: remove thinking blocks (reasoning_content and
// thinking/reasoning content parts) from all assistant messages except the
// last one. thinkingPolicy=keep disables the layer. Returns whether any
// message changed.
func trimThinkingBlocks(msgs []Message, policy string) bool {
	if policy == ThinkingPolicyKeep {
		return false
	}
	lastAsst := lastIndexOfRole(msgs, "assistant")
	if lastAsst < 0 {
		return false
	}
	changed := false
	for i := range msgs {
		if msgs[i].Role != "assistant" || i == lastAsst {
			continue
		}
		if msgs[i].ReasoningContent != "" {
			msgs[i].ReasoningContent = ""
			changed = true
		}
		parts, ok := msgs[i].contentParts()
		if !ok {
			continue
		}
		filtered := make([]ContentPart, 0, len(parts))
		msgChanged := false
		for j := range parts {
			if isThinkingPart(&parts[j]) {
				msgChanged = true
				continue
			}
			filtered = append(filtered, parts[j])
		}
		if msgChanged {
			msgs[i].setContentParts(filtered)
			changed = true
		}
	}
	return changed
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

// truncateRunes truncates s to at most max runes.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

// messageText returns the concatenated plain text of a message (string
// content, or the text/thinking parts), used by the rewrite marker checks.
func messageText(m *Message) string {
	if s, ok := m.contentString(); ok {
		return s
	}
	parts, ok := m.contentParts()
	if !ok {
		return ""
	}
	var sb strings.Builder
	for i := range parts {
		if parts[i].Type == "text" {
			sb.WriteString(parts[i].Text)
		}
		if isThinkingPart(&parts[i]) {
			sb.WriteString(parts[i].Thinking)
		}
	}
	return sb.String()
}
