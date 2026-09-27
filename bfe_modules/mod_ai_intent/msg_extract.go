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

package mod_ai_intent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic"
)

// ExtractLastUserMessage extracts the last user message from an AI request
// body of the given protocol (openai / anthropic / gemini). Unknown
// protocols are parsed tolerantly in openai format.
func ExtractLastUserMessage(body []byte, protocol string) (string, error) {
	if len(body) == 0 {
		return "", fmt.Errorf("empty body")
	}

	switch protocol {
	case bfe_basic.AuthStyleGemini:
		return extractGeminiLastUser(body)
	case bfe_basic.AuthStyleOpenAI, bfe_basic.AuthStyleAnthropic,
		bfe_basic.AuthStyleUnknown, "":
		return extractOpenAILastUser(body)
	default:
		// tolerate unrecognized protocol values as openai format
		return extractOpenAILastUser(body)
	}
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

type aiChatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func extractOpenAILastUser(body []byte) (string, error) {
	var req struct {
		Messages []aiChatMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", fmt.Errorf("parse messages err: %s", err)
	}
	if len(req.Messages) == 0 {
		return "", fmt.Errorf("no messages")
	}

	last := req.Messages[len(req.Messages)-1]
	if last.Role != "user" {
		return "", fmt.Errorf("last message role is %q", last.Role)
	}
	return messageContentToText(last.Content)
}

func messageContentToText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("message content empty")
	}

	// content as plain string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return "", fmt.Errorf("message text empty")
		}
		return s, nil
	}

	// content as array of typed parts; take text segments
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("parse message content err: %s", err)
	}
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == "text" {
			sb.WriteString(p.Text)
		}
	}
	if sb.Len() == 0 {
		return "", fmt.Errorf("no text part in message content")
	}
	return sb.String(), nil
}

func extractGeminiLastUser(body []byte) (string, error) {
	var req struct {
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", fmt.Errorf("parse contents err: %s", err)
	}
	if len(req.Contents) == 0 {
		return "", fmt.Errorf("no contents")
	}

	last := req.Contents[len(req.Contents)-1]
	if last.Role != "" && last.Role != "user" {
		return "", fmt.Errorf("last content role is %q", last.Role)
	}
	var sb strings.Builder
	for _, p := range last.Parts {
		sb.WriteString(p.Text)
	}
	if sb.Len() == 0 {
		return "", fmt.Errorf("no text part in contents")
	}
	return sb.String(), nil
}
