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

package anthropic

import (
	"strings"
	"testing"
)

// TestStreamErrorEventBuilder pins the issue #1474 wire format of the
// gateway-injected stream error events and the truncation markers.
func TestStreamErrorEventBuilder(t *testing.T) {
	a := New()

	native := string(a.NativeStreamErrorEvent("boom"))
	if native != "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":\"boom\"}}\n\n" {
		t.Errorf("native event format drifted: %q", native)
	}

	wrapped := string(a.WrapStreamErrorPayload(`{"error":{}}`))
	if wrapped != "event: error\ndata: {\"error\":{}}\n\n" {
		t.Errorf("wrapped event format drifted: %q", wrapped)
	}

	markers := a.StreamTerminalMarkers()
	joined := ""
	for _, m := range markers {
		joined += string(m) + "|"
	}
	if !strings.Contains(joined, "event: message_stop") {
		t.Errorf("anthropic markers must cover message_stop: %q", joined)
	}
}
