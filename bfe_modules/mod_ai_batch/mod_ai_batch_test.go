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

package mod_ai_batch

import (
	"bytes"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
)

func TestBatchDataConfCheck(t *testing.T) {
	cfg := defaultBatchDataConf()
	if err := cfg.check(); err != nil {
		t.Fatalf("default conf should pass check: %v", err)
	}

	bad := defaultBatchDataConf()
	bad.MaxLineParseBytes = 0
	if err := bad.check(); err == nil {
		t.Fatalf("MaxLineParseBytes=0 should fail check")
	}

	bad2 := defaultBatchDataConf()
	bad2.OwnerCheckMissPolicy = "bogus"
	if err := bad2.check(); err == nil {
		t.Fatalf("invalid OwnerCheckMissPolicy should fail check")
	}
}

// countingBody enforces the effective limits while streaming.
func TestCountingBodyLimits(t *testing.T) {
	payload := strings.Repeat("a", 100) + "\n" + strings.Repeat("b", 100) + "\n"
	ctx := &batchContext{}
	aiMeta := &bfe_basic.AiBasicInfo{}
	aiMeta.BatchEffMaxFileBytes = 150 // exceed within the first chunk

	cb := newCountingBody(ioutil.NopCloser(strings.NewReader(payload)), ctx, aiMeta)
	buf := make([]byte, 512)
	_, err := cb.Read(buf)
	if err != errUploadLimitExceeded {
		t.Fatalf("expected errUploadLimitExceeded, got %v", err)
	}
	if !ctx.uploadAborted {
		t.Fatalf("ctx.uploadAborted should be set")
	}
	if ctx.uploadBytes == 0 {
		t.Fatalf("uploadBytes should be counted")
	}
}

func TestCountingBodyLines(t *testing.T) {
	payload := "l1\nl2\nl3\n"
	ctx := &batchContext{}
	aiMeta := &bfe_basic.AiBasicInfo{}
	aiMeta.BatchEffMaxFileLines = 2

	cb := newCountingBody(ioutil.NopCloser(strings.NewReader(payload)), ctx, aiMeta)
	data, err := ioutil.ReadAll(cb)
	// the read that crosses the line limit returns the bytes plus the abort
	// error; total content is still counted before abort
	if ctx.uploadLines != 3 {
		t.Fatalf("uploadLines = %d, want 3", ctx.uploadLines)
	}
	if len(data) == 0 && err == nil {
		t.Fatalf("expected abort error")
	}
}

// usageScanBody parses per-model usage from a result-file stream and
// finalizes the settle contract at EOF, passing bytes through unchanged.
func TestUsageScanBody(t *testing.T) {
	jsonl := `{"custom_id":"1","response":{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}},"error":null}` + "\n" +
		`{"custom_id":"2","response":{"model":"gpt-4o","usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}},"error":null}` + "\n" +
		`{"custom_id":"3","response":{"model":"deepseek-chat","usage":{"prompt_tokens":7,"completion_tokens":1,"total_tokens":8}},"error":null}` + "\n" +
		`not-a-json-line` + "\n"

	ctx := &batchContext{usageByModel: make(map[string]*bfe_basic.TokenUsage)}
	aiMeta := &bfe_basic.AiBasicInfo{}

	usb := newUsageScanBody(ioutil.NopCloser(strings.NewReader(jsonl)), NewModuleAiBatch(), ctx, aiMeta, "batch-abc", 1<<20, int64(len(jsonl)))
	data, err := ioutil.ReadAll(usb)
	if err != nil {
		t.Fatalf("read err: %v", err)
	}
	if string(data) != jsonl {
		t.Fatalf("response bytes must pass through unchanged")
	}
	if !ctx.usageParsed {
		t.Fatalf("usageParsed should be true")
	}
	if got := ctx.usageByModel["gpt-4o"]; got == nil || got.PromptTokens != 13 || got.CompletionTokens != 7 {
		t.Fatalf("gpt-4o usage wrong: %+v", got)
	}
	if got := ctx.usageByModel["deepseek-chat"]; got == nil || got.UsedQuota != 8 {
		t.Fatalf("deepseek-chat usage wrong: %+v", got)
	}
	if aiMeta.BatchSettle != bfe_basic.BatchSettleSettle {
		t.Fatalf("BatchSettle = %q, want settle", aiMeta.BatchSettle)
	}
	if aiMeta.BatchSettleId != "batch-abc" {
		t.Fatalf("BatchSettleId = %q, want batch-abc", aiMeta.BatchSettleId)
	}
	if aiMeta.BatchLines != 3 {
		t.Fatalf("BatchLines = %d, want 3 (unparsable line excluded)", aiMeta.BatchLines)
	}
	if len(aiMeta.BatchUsageByModel) != 2 {
		t.Fatalf("BatchUsageByModel should carry 2 model groups")
	}
}

// a client-aborted stream (EOF never observed with usage) must not finalize.
func TestUsageScanBodyAbortedNoFinalize(t *testing.T) {
	jsonl := `{"custom_id":"1","response":{"model":"m","usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}},"error":null}` + "\n"
	ctx := &batchContext{usageByModel: make(map[string]*bfe_basic.TokenUsage)}
	aiMeta := &bfe_basic.AiBasicInfo{}

	src := &abortReader{data: jsonl, abortAfter: 10}
	usb := newUsageScanBody(src, NewModuleAiBatch(), ctx, aiMeta, "batch-x", 1<<20, int64(len(jsonl)))
	_, _ = ioutil.ReadAll(usb) // client gone: read error before EOF
	if aiMeta.BatchSettle == bfe_basic.BatchSettleSettle {
		t.Fatalf("settle contract must not finalize on aborted stream")
	}
}

type abortReader struct {
	data       string
	off        int
	abortAfter int
}

func (a *abortReader) Read(p []byte) (int, error) {
	if a.off >= a.abortAfter {
		return 0, io.ErrUnexpectedEOF
	}
	rem := a.abortAfter - a.off
	if len(p) > rem {
		p = p[:rem]
	}
	n := copy(p, a.data[a.off:])
	a.off += n
	return n, nil
}
func (a *abortReader) Close() error { return nil }

func TestParseResponses(t *testing.T) {
	fr := parseFileResponse([]byte(`{"id":"file-abc","purpose":"batch","bytes":1024}`))
	if fr == nil || fr.Id != "file-abc" || fr.Purpose != "batch" {
		t.Fatalf("parseFileResponse wrong: %+v", fr)
	}
	if parseFileResponse([]byte(`{"purpose":"batch"}`)) != nil {
		t.Fatalf("file response without id should be nil")
	}
	if parseFileResponse(nil) != nil {
		t.Fatalf("empty body should be nil")
	}

	br := parseBatchResponse([]byte(`{"id":"batch-abc","status":"in_progress","output_file_id":"file-out"}`))
	if br == nil || br.Id != "batch-abc" || br.Status != "in_progress" || br.OutputFileId != "file-out" {
		t.Fatalf("parseBatchResponse wrong: %+v", br)
	}
	if parseBatchResponse([]byte(`not json`)) != nil {
		t.Fatalf("invalid json should be nil")
	}
}

func TestCaptureBody(t *testing.T) {
	payload := strings.Repeat("x", smallJsonCap+100)
	cb := newCaptureBody(ioutil.NopCloser(strings.NewReader(payload)), smallJsonCap, int64(len(payload)), nil)
	data, err := ioutil.ReadAll(cb)
	if err != nil {
		t.Fatalf("read err: %v", err)
	}
	if string(data) != payload {
		t.Fatalf("bytes must pass through")
	}
	if cb.buf.Len() != smallJsonCap {
		t.Fatalf("buffer capped at %d, got %d", smallJsonCap, cb.buf.Len())
	}
}

var _ = bytes.MinRead // keep bytes import if unused after edits
