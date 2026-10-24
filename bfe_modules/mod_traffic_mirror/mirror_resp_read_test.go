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

package mod_traffic_mirror

import (
	"io"
	"strings"
	"testing"
)

// chunkReader returns at most n bytes per read to exercise partial-line
// handling in the SSE parser
type chunkReader struct {
	r io.Reader
	n int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(p) > c.n {
		p = p[:c.n]
	}
	return c.r.Read(p)
}

const sseFullStream = `data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"hel"}}]}

data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}

data: [DONE]

`

func TestDrainSSEFullStream(t *testing.T) {
	result := drainAndParseBody(strings.NewReader(sseFullStream), "text/event-stream", 1<<20)

	if !result.Completed {
		t.Error("stream should be completed ([DONE] seen)")
	}
	if result.Truncated {
		t.Error("stream should not be truncated")
	}
	if result.FinishReason != "stop" {
		t.Errorf("FinishReason = %s, want stop", result.FinishReason)
	}
	if result.PromptTokens != 11 || result.CompletionTokens != 7 {
		t.Errorf("usage = %d/%d, want 11/7", result.PromptTokens, result.CompletionTokens)
	}
	if result.Bytes != int64(len(sseFullStream)) {
		t.Errorf("Bytes = %d, want %d", result.Bytes, len(sseFullStream))
	}
}

func TestDrainSSEChunkedReads(t *testing.T) {
	reader := &chunkReader{r: strings.NewReader(sseFullStream), n: 7}
	result := drainAndParseBody(reader, "text/event-stream", 1<<20)

	if !result.Completed {
		t.Error("chunked stream should be completed")
	}
	if result.PromptTokens != 11 || result.CompletionTokens != 7 {
		t.Errorf("usage = %d/%d, want 11/7", result.PromptTokens, result.CompletionTokens)
	}
}

func TestDrainSSEWithoutDone(t *testing.T) {
	stream := `data: {"choices":[{"delta":{"content":"hi"}}],"usage":{"prompt_tokens":3,"completion_tokens":1}}
`
	result := drainAndParseBody(strings.NewReader(stream), "text/event-stream", 1<<20)

	if result.Completed {
		t.Error("stream without [DONE] should not be marked completed")
	}
	if result.PromptTokens != 3 {
		t.Errorf("PromptTokens = %d, want 3", result.PromptTokens)
	}
}

func TestDrainSSETruncated(t *testing.T) {
	result := drainAndParseBody(strings.NewReader(sseFullStream), "text/event-stream", 64)

	if !result.Truncated {
		t.Error("stream should be truncated by the byte limit")
	}
	if result.Completed {
		t.Error("truncated stream should not be completed")
	}
}

func TestDrainNonStreamJson(t *testing.T) {
	body := `{"id":"chatcmpl-9","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"length"}],"usage":{"prompt_tokens":5,"completion_tokens":9,"total_tokens":14}}`
	result := drainAndParseBody(strings.NewReader(body), "application/json", 1<<20)

	if !result.Completed {
		t.Error("non-stream body should be completed at EOF")
	}
	if result.FinishReason != "length" {
		t.Errorf("FinishReason = %s, want length", result.FinishReason)
	}
	if result.PromptTokens != 5 || result.CompletionTokens != 9 {
		t.Errorf("usage = %d/%d, want 5/9", result.PromptTokens, result.CompletionTokens)
	}
}

func TestDrainErrorJson(t *testing.T) {
	body := `{"error":{"message":"rate limit","type":"rate_limit_exceeded","code":"rate_limit"}}`
	result := drainAndParseBody(strings.NewReader(body), "application/json", 1<<20)

	if result.ErrorType != "rate_limit_exceeded" {
		t.Errorf("ErrorType = %s", result.ErrorType)
	}
	if result.ErrorCode != "rate_limit" {
		t.Errorf("ErrorCode = %s", result.ErrorCode)
	}
}

func TestDrainNonStreamTruncated(t *testing.T) {
	body := strings.Repeat("a", 1000)
	result := drainAndParseBody(strings.NewReader(body), "text/plain", 100)

	if !result.Truncated {
		t.Error("body should be truncated by the byte limit")
	}
	if result.Bytes != 100 {
		t.Errorf("Bytes = %d, want 100", result.Bytes)
	}
}

func TestDrainInvalidJsonIgnored(t *testing.T) {
	result := drainAndParseBody(strings.NewReader(`{invalid`), "application/json", 1<<20)

	if !result.Completed {
		t.Error("drain should complete even when parsing fails")
	}
	if result.PromptTokens != 0 {
		t.Errorf("no tokens should be parsed, got %d", result.PromptTokens)
	}
}
