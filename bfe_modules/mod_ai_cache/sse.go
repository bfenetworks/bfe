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

package mod_ai_cache

import (
	"strings"

	"github.com/tidwall/gjson"
)

// sseDone marks the end of an SSE stream.
const sseDone = "[DONE]"

// extractStreamAnswer parses the accumulated SSE payload and extracts the
// answer content using path (a GJSON path, e.g. choices.0.delta.content).
//
// Boundary cases handled:
//   - events split across multiple network chunks (partial messages): the
//     caller only invokes this on the fully accumulated buffer, so framing
//     is recomputed over the whole payload;
//   - "[DONE]" in the same package as the last content chunk: data after the
//     last content chunk is ignored;
//   - first chunk without delta.content (role-only): contributes nothing.
func extractStreamAnswer(payload []byte, path string) string {
	var sb strings.Builder

	events := splitSSEEvents(string(payload))
	for _, event := range events {
		data := sseEventData(event)
		if data == "" {
			continue
		}
		if data == sseDone {
			break
		}
		if !gjson.Valid(data) {
			continue
		}
		sb.WriteString(gjson.Get(data, path).String())
	}

	return sb.String()
}

// splitSSEEvents splits an SSE payload into events. Events are separated by
// a blank line; a trailing partial event (no terminating blank line) is
// still returned as an event so that a final content chunk followed by
// "[DONE]" in the same package is handled.
func splitSSEEvents(payload string) []string {
	payload = strings.ReplaceAll(payload, "\r\n", "\n")
	// an event ends with a blank line; the final event may lack it
	rawEvents := strings.Split(payload, "\n\n")

	events := make([]string, 0, len(rawEvents))
	for _, e := range rawEvents {
		e = strings.TrimRight(e, "\n")
		if strings.TrimSpace(e) == "" {
			continue
		}
		events = append(events, e)
	}
	return events
}

// sseEventData returns the concatenated data payload of a single SSE event:
// the data: field lines joined with newlines (per the SSE spec, multiple
// data: lines are concatenated with "\n").
func sseEventData(event string) string {
	var lines []string
	for _, line := range strings.Split(event, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimPrefix(data, " ")
		lines = append(lines, data)
	}

	if len(lines) == 0 {
		return ""
	}

	return strings.Join(lines, "\n")
}

// buildSSEEvent builds one SSE event for the given data payload.
func buildSSEEvent(data string) string {
	return "data:" + data + "\n\n"
}
