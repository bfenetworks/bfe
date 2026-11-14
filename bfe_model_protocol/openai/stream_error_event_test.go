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

package openai

import (
	"strings"
	"testing"
)

// TestStreamErrorEventBuilder pins the issue #1474 wire format of the
// gateway-injected stream error events and the truncation markers.
func TestStreamErrorEventBuilder(t *testing.T) {
	a := New()

	native := string(a.NativeStreamErrorEvent("boom"))
	if native != `data: {"error":{"message":"boom","type":"server_error","param":null,"code":"upstream_stream_truncated"}}`+"\n\n" {
		t.Errorf("native event format drifted: %q", native)
	}

	wrapped := string(a.WrapStreamErrorPayload(`{"error":{}}`))
	if wrapped != "data: {\"error\":{}}\n\n" {
		t.Errorf("wrapped event format drifted: %q", wrapped)
	}

	markers := a.StreamTerminalMarkers()
	if len(markers) == 0 {
		t.Fatalf("no terminal markers declared")
	}
	joined := string(markers[0])
	for _, m := range markers[1:] {
		joined += "|" + string(m)
	}
	if !strings.Contains(joined, "[DONE]") {
		t.Errorf("openai markers must cover [DONE]: %q", joined)
	}
}
