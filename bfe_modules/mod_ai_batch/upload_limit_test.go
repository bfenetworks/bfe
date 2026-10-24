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
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

// jsonlOneLine is the smallest realistic batch input file: one request line.
const jsonlOneLine = `{"custom_id":"1","method":"POST","url":"/v1/chat/completions",` +
	`"body":{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}}` + "\n"

// newUploadRequest builds the request/context pair preCheckUpload works on.
// HttpRequest and OutRequest share the same *bfe_http.Request, mirroring the
// reverse proxy's shallow copy (the transport reads OutRequest).
func newUploadRequest(contentType string, body []byte) (*ModuleAiBatch,
	*bfe_basic.Request, *batchContext) {

	httpReq := &bfe_http.Request{
		Method:        "POST",
		Header:        bfe_http.Header{},
		Body:          ioutil.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	req := &bfe_basic.Request{HttpRequest: httpReq, OutRequest: httpReq}
	return NewModuleAiBatch(), req, &batchContext{op: bfe_basic.BatchOpUpload}
}

// limitBytes returns a content string with exactly n jsonl lines.
func limitLines(n int) string {
	return strings.Repeat(jsonlOneLine, n)
}

// A multipart upload whose FILE content is within both limits must be accepted
// and must reach the provider untouched: the envelope (11 lines / 502 bytes for
// a 1-line file) is never measured (issue #1434).
func TestPreCheckUploadMultipartFileWithinLimits(t *testing.T) {
	contentType, body := buildMultipartBody(t, "batch.jsonl", jsonlOneLine)
	aiMeta := &bfe_basic.AiBasicInfo{BatchEffMaxFileLines: 10, BatchEffMaxFileBytes: 1 << 20}
	m, req, ctx := newUploadRequest(contentType, body)

	ret, res := m.preCheckUpload(req, ctx, aiMeta)
	if ret != bfe_module.BfeHandlerGoOn || res != nil {
		t.Fatalf("file within limits: ret=%d res=%v, want GoOn", ret, res)
	}
	if !ctx.uploadPartOK {
		t.Fatalf("file part should be located")
	}
	if ctx.uploadPartLines != 1 || ctx.uploadPartBytes != int64(len(jsonlOneLine)) {
		t.Fatalf("part stats = %d lines / %d bytes, want 1 / %d",
			ctx.uploadPartLines, ctx.uploadPartBytes, len(jsonlOneLine))
	}
	if _, gated := req.OutRequest.Body.(*multipartLimitBody); gated {
		t.Fatalf("pre-checked upload must not carry a streaming gate")
	}
	got, err := ioutil.ReadAll(req.OutRequest.Body)
	if err != nil {
		t.Fatalf("accepted upload must stream through: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("body changed: %d bytes, want %d", len(got), len(body))
	}
}

// The envelope is bigger than max_file_bytes while the file content is not:
// the request must not be rejected on the Content-Length (issue #1434).
func TestPreCheckUploadMultipartEnvelopeOverByteLimit(t *testing.T) {
	contentType, body := buildMultipartBody(t, "batch.jsonl", jsonlOneLine)
	if int64(len(body)) <= 400 {
		t.Fatalf("envelope (%d) must exceed the limit used by this test", len(body))
	}
	aiMeta := &bfe_basic.AiBasicInfo{BatchEffMaxFileBytes: 400}
	m, req, ctx := newUploadRequest(contentType, body)

	if ret, res := m.preCheckUpload(req, ctx, aiMeta); ret != bfe_module.BfeHandlerGoOn || res != nil {
		t.Fatalf("envelope must not be measured against the file limit: ret=%d res=%v", ret, res)
	}
	if ctx.uploadPartBytes != int64(len(jsonlOneLine)) {
		t.Fatalf("part bytes = %d, want %d", ctx.uploadPartBytes, len(jsonlOneLine))
	}
}

// The file content itself over a limit is still rejected, with the file's
// numbers in the error and the operation switched to reject (413).
func TestPreCheckUploadMultipartFileOverLimit(t *testing.T) {
	tenLines := limitLines(10)

	cases := []struct {
		name   string
		aiMeta *bfe_basic.AiBasicInfo
		lines  int
		want   int
	}{
		{"lines exactly at limit", &bfe_basic.AiBasicInfo{BatchEffMaxFileLines: 10}, 10,
			bfe_module.BfeHandlerGoOn},
		{"lines one over limit", &bfe_basic.AiBasicInfo{BatchEffMaxFileLines: 10}, 11,
			bfe_module.BfeHandlerFinish},
		{"bytes exactly at limit",
			&bfe_basic.AiBasicInfo{BatchEffMaxFileBytes: int64(len(tenLines))}, 10,
			bfe_module.BfeHandlerGoOn},
		{"bytes one over limit",
			&bfe_basic.AiBasicInfo{BatchEffMaxFileBytes: int64(len(tenLines)) - 1}, 10,
			bfe_module.BfeHandlerFinish},
	}
	for _, c := range cases {
		content := limitLines(c.lines)
		contentType, body := buildMultipartBody(t, "batch.jsonl", content)
		m, req, ctx := newUploadRequest(contentType, body)

		ret, res := m.preCheckUpload(req, ctx, c.aiMeta)
		if ret != c.want {
			t.Fatalf("%s: ret=%d, want %d (lines=%d bytes=%d limit=%+v)",
				c.name, ret, c.want, ctx.uploadPartLines, ctx.uploadPartBytes, c.aiMeta)
		}
		if c.want == bfe_module.BfeHandlerFinish {
			if res == nil || res.StatusCode != bfe_http.StatusRequestEntityTooLarge {
				t.Fatalf("%s: want 413, got %v", c.name, res)
			}
			if !ctx.overSized {
				t.Fatalf("%s: ctx.overSized should be set", c.name)
			}
			if got := m.state.GetCounter("UPLOAD_REJECT_SIZE"); got != 1 {
				t.Fatalf("%s: UPLOAD_REJECT_SIZE = %d, want 1", c.name, got)
			}
		}
	}
}

// Raw (non-multipart) uploads are measured on the whole body: the early
// Content-Length rejection must keep working (SC28 TC-04a semantics).
func TestPreCheckUploadRawBodyUnchanged(t *testing.T) {
	raw := bytes.Repeat([]byte("a"), 200)
	aiMeta := &bfe_basic.AiBasicInfo{BatchEffMaxFileBytes: 100}
	m, req, ctx := newUploadRequest("application/octet-stream", raw)
	if ret, res := m.preCheckUpload(req, ctx, aiMeta); ret != bfe_module.BfeHandlerFinish ||
		res == nil || res.StatusCode != bfe_http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized raw upload: ret=%d res=%v, want 413", ret, res)
	}

	ok := bytes.Repeat([]byte("a"), 100)
	aiMeta = &bfe_basic.AiBasicInfo{BatchEffMaxFileBytes: 100}
	m, req, ctx = newUploadRequest("application/jsonl", ok)
	if ret, res := m.preCheckUpload(req, ctx, aiMeta); ret != bfe_module.BfeHandlerGoOn || res != nil {
		t.Fatalf("raw upload exactly at limit: ret=%d res=%v, want GoOn", ret, res)
	}
}

// When the `file` part does not fit in the buffered prefix, the streaming gate
// takes over and measures the FILE, not the envelope.
func TestPreCheckUploadStreamedMultipart(t *testing.T) {
	defer bfe_http.SetAccessibleBodySize(bfe_http.DefaultAccessibleBodySize)
	// a buffer smaller than the file part forces the streaming path
	bfe_http.SetAccessibleBodySize(64)

	content := limitLines(2) // 2 lines / 274 bytes
	contentType, body := buildMultipartBody(t, "batch.jsonl", content)

	// within the limits: the whole envelope streams through, stats stay exact
	aiMeta := &bfe_basic.AiBasicInfo{BatchEffMaxFileLines: 2, BatchEffMaxFileBytes: 1 << 20}
	m, req, ctx := newUploadRequest(contentType, body)
	if ret, res := m.preCheckUpload(req, ctx, aiMeta); ret != bfe_module.BfeHandlerGoOn || res != nil {
		t.Fatalf("streamed upload within limits: ret=%d res=%v", ret, res)
	}
	if _, gated := req.OutRequest.Body.(*multipartLimitBody); !gated {
		t.Fatalf("streamed upload should carry the multipart gate")
	}
	got, err := ioutil.ReadAll(req.OutRequest.Body)
	if err != nil {
		t.Fatalf("streamed upload within limits must not abort: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("gate must forward the body unchanged: %d bytes, want %d", len(got), len(body))
	}
	if !ctx.uploadPartOK || ctx.uploadPartLines != 2 || ctx.uploadPartBytes != int64(len(content)) {
		t.Fatalf("streamed part stats = ok:%v %d lines / %d bytes, want 2 / %d",
			ctx.uploadPartOK, ctx.uploadPartLines, ctx.uploadPartBytes, len(content))
	}

	// over the line limit: aborted mid-stream, envelope lines never inspected
	aiMeta = &bfe_basic.AiBasicInfo{BatchEffMaxFileLines: 1}
	m, req, ctx = newUploadRequest(contentType, body)
	if ret, res := m.preCheckUpload(req, ctx, aiMeta); ret != bfe_module.BfeHandlerGoOn || res != nil {
		t.Fatalf("streamed upload cannot be judged before forwarding: ret=%d res=%v", ret, res)
	}
	if _, err := ioutil.ReadAll(req.OutRequest.Body); err != errUploadLimitExceeded {
		t.Fatalf("over-limit stream: err=%v, want errUploadLimitExceeded", err)
	}
	if !ctx.uploadAborted {
		t.Fatalf("ctx.uploadAborted should be set")
	}
}

// multipartLimitBody counts the `file` part content across chunk boundaries,
// including boundary look-alikes inside the content and multi-byte boundaries.
func TestMultipartLimitBodyCountsFilePart(t *testing.T) {
	boundary := "----bfeBoundary42"
	content := "line-1\r\n----bfeBoundary42x\nline-3\n"
	body := buildEnvelope(boundary, "batch.jsonl", content, "batch")

	cases := []struct {
		name       string
		aiMeta     *bfe_basic.AiBasicInfo
		wantErr    error
		wantOK     bool
		wantLines  int64
		wantBytes  int64
		stepBuffer int
	}{
		{
			name:      "envelope lines ignored",
			aiMeta:    &bfe_basic.AiBasicInfo{BatchEffMaxFileLines: 3},
			wantOK:    true,
			wantLines: 3,
			wantBytes: int64(len(content)),
		},
		{
			name:       "one byte reads",
			aiMeta:     &bfe_basic.AiBasicInfo{BatchEffMaxFileLines: 3},
			wantOK:     true,
			wantLines:  3,
			wantBytes:  int64(len(content)),
			stepBuffer: 1,
		},
		{
			name:      "file content over line limit",
			aiMeta:    &bfe_basic.AiBasicInfo{BatchEffMaxFileLines: 2},
			wantErr:   errUploadLimitExceeded,
			wantLines: 3,
		},
		{
			name:      "file content over byte limit",
			aiMeta:    &bfe_basic.AiBasicInfo{BatchEffMaxFileBytes: int64(len(content)) - 1},
			wantErr:   errUploadLimitExceeded,
			wantBytes: int64(len(content)),
		},
		{
			name:      "exactly at both limits",
			aiMeta:    &bfe_basic.AiBasicInfo{BatchEffMaxFileLines: 3, BatchEffMaxFileBytes: int64(len(content))},
			wantOK:    true,
			wantLines: 3,
			wantBytes: int64(len(content)),
		},
	}
	for _, c := range cases {
		ctx := &batchContext{}
		gate := newUploadLimitBody(ioutil.NopCloser(bytes.NewReader(body)), boundary, ctx, c.aiMeta)
		got, err := readAllStep(gate, c.stepBuffer)
		if c.wantErr != nil {
			if err != c.wantErr {
				t.Fatalf("%s: err=%v, want %v", c.name, err, c.wantErr)
			}
			if !ctx.uploadAborted {
				t.Fatalf("%s: ctx.uploadAborted should be set", c.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: unexpected err %v", c.name, err)
		}
		if !bytes.Equal(got, body) {
			t.Fatalf("%s: gate must forward the body unchanged", c.name)
		}
		if ctx.uploadPartOK != c.wantOK {
			t.Fatalf("%s: uploadPartOK = %v, want %v", c.name, ctx.uploadPartOK, c.wantOK)
		}
		if ctx.uploadPartLines != c.wantLines || ctx.uploadPartBytes != c.wantBytes {
			t.Fatalf("%s: part stats = %d lines / %d bytes, want %d / %d",
				c.name, ctx.uploadPartLines, ctx.uploadPartBytes, c.wantLines, c.wantBytes)
		}
		if ctx.uploadFilename != "batch.jsonl" {
			t.Fatalf("%s: filename = %q", c.name, ctx.uploadFilename)
		}
		// envelope counters stay whole-body (approximate binding fallback)
		if ctx.uploadLines <= ctx.uploadPartLines {
			t.Fatalf("%s: envelope lines (%d) should exceed the file lines (%d)",
				c.name, ctx.uploadLines, ctx.uploadPartLines)
		}
	}
}

// A multipart body whose boundary cannot be parsed degrades to whole-body
// counting, the documented conservative fallback.
func TestMultipartLimitBodyDegrades(t *testing.T) {
	content := "a\nb\n"
	body := buildEnvelope("----bfeBoundary42", "f.jsonl", content, "batch")

	// no boundary known: the caller gets the envelope counter
	ctx := &batchContext{}
	gate := newUploadLimitBody(ioutil.NopCloser(bytes.NewReader(body)), "", ctx,
		&bfe_basic.AiBasicInfo{BatchEffMaxFileBytes: 10})
	if _, ok := gate.(*envelopeCountingBody); !ok {
		t.Fatalf("missing boundary must fall back to envelope counting")
	}

	// wrong boundary: the parser never sees a part, so nothing is counted and
	// the upload is not aborted by the file limits
	ctx = &batchContext{}
	gate = newUploadLimitBody(ioutil.NopCloser(bytes.NewReader(body)), "other-boundary", ctx,
		&bfe_basic.AiBasicInfo{BatchEffMaxFileBytes: 10})
	if _, err := ioutil.ReadAll(gate); err != nil {
		t.Fatalf("unparsable envelope must not abort: %v", err)
	}
	if ctx.uploadPartOK {
		t.Fatalf("unparsable envelope must not publish file stats")
	}
}

// buildEnvelope assembles a multipart body with the file part first and the
// purpose field after it, the shape of every OpenAI-compatible client.
func buildEnvelope(boundary, filename, content, purpose string) []byte {
	var buf bytes.Buffer
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString(`Content-Disposition: form-data; name="file"; filename="` + filename + `"` + "\r\n")
	buf.WriteString("Content-Type: application/jsonl\r\n\r\n")
	buf.WriteString(content)
	buf.WriteString("\r\n--" + boundary + "\r\n")
	buf.WriteString(`Content-Disposition: form-data; name="purpose"` + "\r\n\r\n")
	buf.WriteString(purpose)
	buf.WriteString("\r\n--" + boundary + "--\r\n")
	return buf.Bytes()
}

// readAllStep reads r with a fixed buffer size (1 for the byte-at-a-time
// parser stress test) and returns everything that was handed out.
func readAllStep(r interface {
	Read([]byte) (int, error)
}, step int) ([]byte, error) {
	if step <= 0 {
		step = 32 * 1024
	}
	buf := make([]byte, step)
	var out bytes.Buffer
	for {
		n, err := r.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
		}
		if err != nil {
			if err == io.EOF {
				return out.Bytes(), nil
			}
			return out.Bytes(), err
		}
	}
}
