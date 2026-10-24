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
	"encoding/json"
	"io"
	"io/ioutil"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
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

// envelopeCountingBody enforces the effective limits on the whole body while
// streaming (degraded path: body is the file, or the envelope is unparsable).
func TestEnvelopeCountingBodyLimits(t *testing.T) {
	payload := strings.Repeat("a", 100) + "\n" + strings.Repeat("b", 100) + "\n"
	ctx := &batchContext{}
	aiMeta := &bfe_basic.AiBasicInfo{}
	aiMeta.BatchEffMaxFileBytes = 150 // exceed within the first chunk

	cb := newEnvelopeCountingBody(ioutil.NopCloser(strings.NewReader(payload)), ctx, aiMeta)
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

func TestEnvelopeCountingBodyLines(t *testing.T) {
	payload := "l1\nl2\nl3\n"
	ctx := &batchContext{}
	aiMeta := &bfe_basic.AiBasicInfo{}
	aiMeta.BatchEffMaxFileLines = 2

	cb := newEnvelopeCountingBody(ioutil.NopCloser(strings.NewReader(payload)), ctx, aiMeta)
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
// Usage is read from response.body.usage (the OpenAI batch result contract);
// a flat response.usage row is still accepted, error rows are skipped, and
// lines without usage / malformed lines are counted separately.
func TestUsageScanBody(t *testing.T) {
	jsonl := `{"custom_id":"1","response":{"status_code":200,"body":{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}},"error":null}` + "\n" +
		`{"custom_id":"2","response":{"status_code":200,"body":{"model":"gpt-4o","usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}},"error":null}` + "\n" +
		`{"custom_id":"3","response":{"model":"deepseek-chat","usage":{"prompt_tokens":7,"completion_tokens":1,"total_tokens":8}},"error":null}` + "\n" +
		`{"custom_id":"4","response":{"status_code":500,"body":{}},"error":{"message":"boom"}}` + "\n" +
		`{"custom_id":"5","response":null,"error":{"message":"failed"}}` + "\n" +
		`not-a-json-line` + "\n"

	ctx := &batchContext{usageByModel: make(map[string]*bfe_basic.TokenUsage)}
	m := NewModuleAiBatch()
	aiMeta := &bfe_basic.AiBasicInfo{}

	usb := newUsageScanBody(ioutil.NopCloser(strings.NewReader(jsonl)), m, ctx, aiMeta, "batch-abc", 1<<20, int64(len(jsonl)))
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
	// legacy flat shape still bills
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
		t.Fatalf("BatchLines = %d, want 3 (error/no-usage/unparsable lines excluded)", aiMeta.BatchLines)
	}
	if len(aiMeta.BatchUsageByModel) != 2 {
		t.Fatalf("BatchUsageByModel should carry 2 model groups")
	}
	if got := m.state.GetCounter("SETTLE_USAGE_PARSE"); got != 1 {
		t.Fatalf("SETTLE_USAGE_PARSE = %d, want 1", got)
	}
	// one row carries a response object without usage; response:null rows are
	// plain error rows and are not counted, one line is malformed
	if got := m.state.GetCounter("SETTLE_NO_USAGE"); got != 1 {
		t.Fatalf("SETTLE_NO_USAGE = %d, want 1", got)
	}
	if got := m.state.GetCounter("SETTLE_USAGE_PARSE_ERR"); got != 1 {
		t.Fatalf("SETTLE_USAGE_PARSE_ERR = %d, want 1", got)
	}
	// the settlement write-back moved to the finish handler: the scanner must
	// not touch BATCH_TASK by itself
	if got := m.state.GetCounter("TASK_SETTLE_WRITE"); got != 0 {
		t.Fatalf("TASK_SETTLE_WRITE = %d, scanner must not write task records", got)
	}
}

// a client-aborted stream (EOF never observed with usage) must not finalize.
func TestUsageScanBodyAbortedNoFinalize(t *testing.T) {
	jsonl := `{"custom_id":"1","response":{"body":{"model":"m","usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}},"error":null}` + "\n"
	ctx := &batchContext{usageByModel: make(map[string]*bfe_basic.TokenUsage)}
	aiMeta := &bfe_basic.AiBasicInfo{}

	src := &abortReader{data: jsonl, abortAfter: 10}
	usb := newUsageScanBody(src, NewModuleAiBatch(), ctx, aiMeta, "batch-x", 1<<20, int64(len(jsonl)))
	_, _ = ioutil.ReadAll(usb) // client gone: read error before EOF
	if aiMeta.BatchSettle == bfe_basic.BatchSettleSettle {
		t.Fatalf("settle contract must not finalize on aborted stream")
	}
}

// multipartFilePartStats reports the file CONTENT, never the envelope.
func TestMultipartFilePartStats(t *testing.T) {
	content := `{"custom_id":"1","method":"POST","url":"/v1/chat/completions"}` + "\n"
	contentType, body := buildMultipartBody(t, "batch-input.jsonl", content)
	boundary := multipartBoundaryOf(t, contentType)

	stats, ok := multipartFilePartStats(boundary, body)
	if !ok {
		t.Fatalf("file part should be located")
	}
	if stats.Filename != "batch-input.jsonl" {
		t.Fatalf("filename = %q", stats.Filename)
	}
	if stats.Lines != 1 {
		t.Fatalf("lines = %d, want 1", stats.Lines)
	}
	if stats.Bytes != int64(len(content)) {
		t.Fatalf("bytes = %d, want %d", stats.Bytes, len(content))
	}
	// the envelope is strictly bigger: headers, boundary lines and CRLFs
	if int64(len(body)) <= stats.Bytes {
		t.Fatalf("envelope (%d) should exceed the file content (%d)", len(body), stats.Bytes)
	}

	// a truncated prefix that still contains the whole file part stays exact
	// (the part itself was not cut); a prefix cutting into the part does not
	if stats, ok := multipartFilePartStats(boundary, body[:len(body)-8]); !ok || stats.Bytes != int64(len(content)) {
		t.Fatalf("prefix with a complete file part: ok=%v stats=%+v", ok, stats)
	}
	if _, ok := multipartFilePartStats(boundary, body[:len(body)/2]); ok {
		t.Fatalf("truncated file part must not yield part stats")
	}

	if _, ok := multipartFilePartStats(boundary, nil); ok {
		t.Fatalf("empty body must not yield part stats")
	}
	if _, ok := multipartFilePartStats("", body); ok {
		t.Fatalf("missing boundary must not yield part stats")
	}

	// a multipart body without a file part (field only) is not locatable
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("purpose", "batch")
	w.Close()
	if _, ok := multipartFilePartStats(w.Boundary(), buf.Bytes()); ok {
		t.Fatalf("field-only multipart must not yield part stats")
	}
}

func TestBuildUploadFileFields(t *testing.T) {
	aiMeta := &bfe_basic.AiBasicInfo{ClientKeyId: "ak-1"}

	// exact: file part located, provider File object confirms bytes/filename
	ctx := &batchContext{
		uploadPartOK:    true,
		uploadFilename:  "local.jsonl",
		uploadPartBytes: 206,
		uploadPartLines: 1,
		uploadBytes:     509,
		uploadLines:     11,
	}
	fields := buildUploadFileFields(ctx, aiMeta, "key-a", &fileResponse{Id: "f1", Purpose: "batch", Bytes: 206})
	if fields["lines"] != "1" || fields["bytes"] != "206" {
		t.Fatalf("exact content stats expected, got %v", fields)
	}
	if fields["lines_src"] != fileSrcExact || fields["bytes_src"] != fileSrcExact {
		t.Fatalf("sources should be exact: %v", fields)
	}
	if fields["filename"] != "local.jsonl" {
		t.Fatalf("filename = %q", fields["filename"])
	}
	if fields["dir"] != "input" || fields["purpose"] != "batch" {
		t.Fatalf("binding shape wrong: %v", fields)
	}

	// provider File object wins for bytes; lines stay from the part
	fields = buildUploadFileFields(ctx, aiMeta, "key-a", &fileResponse{Id: "f1", Bytes: 300, Filename: "provider.jsonl"})
	if fields["bytes"] != "300" || fields["lines"] != "1" {
		t.Fatalf("provider bytes should win: %v", fields)
	}
	if fields["filename"] != "provider.jsonl" {
		t.Fatalf("provider filename should win: %q", fields["filename"])
	}

	// no file part: envelope counters only, marked approx
	fields = buildUploadFileFields(&batchContext{uploadBytes: 509, uploadLines: 11}, aiMeta, "key-a", &fileResponse{Id: "f1"})
	if fields["lines"] != "11" || fields["bytes"] != "509" {
		t.Fatalf("approx fallback expected: %v", fields)
	}
	if fields["lines_src"] != fileSrcApprox || fields["bytes_src"] != fileSrcApprox {
		t.Fatalf("sources should be approx: %v", fields)
	}
	if _, ok := fields["filename"]; ok {
		t.Fatalf("filename must be absent when unknown")
	}
}

func TestBuildCreateTaskFields(t *testing.T) {
	aiMeta := &bfe_basic.AiBasicInfo{ClientKeyId: "ak-1"}
	ctx := &batchContext{inputFileId: "file-in", estLines: 1, endpoint: "/v1/chat/completions", reserved: true, rmbUnits: 12345}
	br := parseBatchResponse([]byte(`{"id":"batch-1","status":"validated","created_at":1760000000,"request_counts":{"completed":0,"failed":0,"total":1}}`))

	fields := buildCreateTaskFields(ctx, aiMeta, "key-a", "cluster-x", br)
	if fields["input_file_id"] != "file-in" || fields["est_lines"] != "1" {
		t.Fatalf("identity fields wrong: %v", fields)
	}
	if fields["reserved_units"] != "12345" || fields["reserved"] != "1" {
		t.Fatalf("reserve fields wrong: %v", fields)
	}
	if fields["endpoint"] != "/v1/chat/completions" {
		t.Fatalf("endpoint should come from the create body: %v", fields)
	}
	if fields["request_counts"] != `{"completed":0,"failed":0,"total":1}` {
		t.Fatalf("request_counts = %q", fields["request_counts"])
	}
	if fields["created_at"] != "1760000000" {
		t.Fatalf("created_at = %q", fields["created_at"])
	}
	if _, ok := fields["output_file_id"]; ok {
		t.Fatalf("output_file_id must be absent when the provider omits it")
	}

	// reserve failed / not applied: amount is zero, marker says so
	ctx.reserved = false
	fields = buildCreateTaskFields(ctx, aiMeta, "key-a", "cluster-x", br)
	if fields["reserved_units"] != "0" || fields["reserved"] != "0" {
		t.Fatalf("unreserved create should record zero: %v", fields)
	}

	// provider endpoint (poll response) overrides the create-body value
	br.Endpoint = "/v1/embeddings"
	fields = buildCreateTaskFields(ctx, aiMeta, "key-a", "cluster-x", br)
	if fields["endpoint"] != "/v1/embeddings" {
		t.Fatalf("provider endpoint should win: %v", fields)
	}
}

func TestBuildGetTaskFields(t *testing.T) {
	br := parseBatchResponse([]byte(`{"id":"batch-1","status":"completed","output_file_id":"file-out","endpoint":"/v1/chat/completions","request_counts":{"completed":1,"failed":0,"total":1}}`))
	fields := buildGetTaskFields(br)
	for k, want := range map[string]string{
		"status":         "completed",
		"output_file_id": "file-out",
		"endpoint":       "/v1/chat/completions",
		"request_counts": `{"completed":1,"failed":0,"total":1}`,
	} {
		if fields[k] != want {
			t.Fatalf("%s = %q, want %q", k, fields[k], want)
		}
	}

	// a poll without the optional fields must not blank out the record
	bare := buildGetTaskFields(&batchResponse{Status: "in_progress"})
	if len(bare) != 1 || bare["status"] != "in_progress" {
		t.Fatalf("only status should be written: %v", bare)
	}

	// malformed request_counts is dropped instead of stored
	br.RequestCounts = json.RawMessage(`not-json`)
	if _, ok := buildGetTaskFields(br)["request_counts"]; ok {
		t.Fatalf("malformed request_counts must not be written")
	}
}

func TestBuildSettleTaskFields(t *testing.T) {
	aiMeta := &bfe_basic.AiBasicInfo{
		BatchSettle:       bfe_basic.BatchSettleSettle,
		BatchSettledUnits: 2700,
		BatchUsageByModel: map[string]bfe_basic.TokenUsage{
			"gpt-4o":     {PromptTokens: 10, CompletionTokens: 5},
			"deepseek":   {PromptTokens: 3, CompletionTokens: 2},
			"unrelated?": {},
		},
	}
	fields := buildSettleTaskFields(aiMeta)
	if fields["usage_in"] != "13" || fields["usage_out"] != "7" {
		t.Fatalf("usage totals wrong: %v", fields)
	}
	if fields["settle_units"] != "2700" || fields["settle_status"] != "settled" {
		t.Fatalf("settlement fields wrong: %v", fields)
	}
	if fields["usage_source"] != "download" {
		t.Fatalf("usage_source = %q, want download", fields["usage_source"])
	}

	aiMeta.BatchSettle = bfe_basic.BatchSettleRelease
	fields = buildSettleTaskFields(aiMeta)
	if len(fields) != 1 || fields["settle_status"] != "released" {
		t.Fatalf("release should only mark settle_status: %v", fields)
	}

	aiMeta.BatchSettle = bfe_basic.BatchSettleNone
	if fields := buildSettleTaskFields(aiMeta); fields != nil {
		t.Fatalf("none contract must write nothing: %v", fields)
	}

	// zero settlement (price miss) is still recorded
	zero := &bfe_basic.AiBasicInfo{BatchSettle: bfe_basic.BatchSettleSettle}
	if fields := buildSettleTaskFields(zero); fields["settle_units"] != "0" || fields["settle_status"] != "settled" {
		t.Fatalf("zero settlement must be recorded: %v", fields)
	}
}

func TestParseResponses(t *testing.T) {
	fr := parseFileResponse([]byte(`{"id":"file-abc","purpose":"batch","bytes":1024}`))
	if fr == nil || fr.Id != "file-abc" || fr.Purpose != "batch" || fr.Bytes != 1024 {
		t.Fatalf("parseFileResponse wrong: %+v", fr)
	}
	fr = parseFileResponse([]byte(`{"id":"file-abc","filename":"input.jsonl"}`))
	if fr == nil || fr.Filename != "input.jsonl" {
		t.Fatalf("parseFileResponse filename wrong: %+v", fr)
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
	br = parseBatchResponse([]byte(`{"id":"batch-abc","status":"completed","endpoint":"/v1/chat/completions","created_at":1760000000,"request_counts":{"total":1}}`))
	if br == nil || br.Endpoint != "/v1/chat/completions" || br.CreatedAt != 1760000000 {
		t.Fatalf("parseBatchResponse extra fields wrong: %+v", br)
	}
	if got, ok := compactJSONObject(br.RequestCounts); !ok || got != `{"total":1}` {
		t.Fatalf("request_counts compaction wrong: %q ok=%v", got, ok)
	}
	if parseBatchResponse([]byte(`not json`)) != nil {
		t.Fatalf("invalid json should be nil")
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

// buildMultipartBody assembles a multipart upload the way a real client does:
// one `file` part followed by the `purpose` field. Returns the Content-Type
// (with boundary) and the whole envelope body.
func buildMultipartBody(t *testing.T, filename, content string) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := w.WriteField("purpose", "batch"); err != nil {
		t.Fatalf("write purpose field: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return w.FormDataContentType(), buf.Bytes()
}

// multipartBoundaryOf extracts the boundary from a multipart Content-Type.
func multipartBoundaryOf(t *testing.T, contentType string) string {
	t.Helper()
	h := bfe_http.Header{}
	h.Set("Content-Type", contentType)
	boundary, ok := bfe_basic.IsMultipartUpload(h)
	if !ok {
		t.Fatalf("content type %q is not a multipart upload", contentType)
	}
	return boundary
}
