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

package mod_body_process

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

// issue #1468: a compressed SSE response must be parsed for usage from a
// decompressed copy while the bytes forwarded to the client stay identical.

const openAISSEUsageChunk = `data: {"id":"chatcmpl-x","object":"chat.completion.chunk","choices":[],` +
	`"usage":{"prompt_tokens":296476,"completion_tokens":160,"total_tokens":296636,` +
	`"prompt_tokens_details":{"cached_tokens":296320}}}` + "\n\n"

const openAISSEDone = "data: [DONE]\n\n"

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("gzip write: %s", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %s", err)
	}
	return buf.Bytes()
}

func zlibBytes(t *testing.T, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("zlib write: %s", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %s", err)
	}
	return buf.Bytes()
}

// newCompressedSSEResponse builds the response shape a compressed SSE upstream
// produces: 200, chunked (ContentLength = -1), SSE media type and a
// Content-Encoding header.
func newCompressedSSEResponse(encoding string, body []byte) *bfe_http.Response {
	res := &bfe_http.Response{
		StatusCode:    bfe_http.StatusOK,
		ContentLength: -1,
		IsSse:         true,
		Body:          io.NopCloser(bytes.NewReader(body)),
		Header:        make(bfe_http.Header),
	}
	res.Header.Set("Content-Type", "text/event-stream")
	if encoding != "" {
		res.Header.Set("Content-Encoding", encoding)
	}
	return res
}

// newStreamTestRequest builds an AI request whose auth style is already
// identified, as in production when the response body is processed.
func newStreamTestRequest(style string) (*bfe_basic.Request, *bfe_basic.AiBasicInfo) {
	httpReq, _ := bfe_http.NewRequest("POST", "http://example.com/v1/chat/completions", nil)
	req := bfe_basic.NewRequest(httpReq, nil, nil, nil, nil)
	ai := req.InitAiBasicInfo()
	ai.AuthStyle = style
	return req, ai
}

func TestDoResponseProcessCompressedSSEGzipUsage(t *testing.T) {
	m := NewModuleBodyProcess()
	req, ai := newStreamTestRequest(bfe_basic.AuthStyleOpenAI)
	ai.SetAllowEstimateToken(true)

	plain := []byte(openAISSEUsageChunk + openAISSEDone)
	compressed := gzipBytes(t, plain)
	res := newCompressedSSEResponse("gzip", compressed)

	bp := m.DoResponseProcess(req, res, nil)
	if bp == nil {
		t.Fatal("expected a BodyProcessor for an AI response")
	}
	forwarded, err := io.ReadAll(bp)
	if err != nil {
		t.Fatalf("read processed body failed: %s", err)
	}

	// Forwarding stays byte-identical: the client receives the very gzip bytes
	// plus the Content-Encoding header (nothing was decoded into the stream).
	if !bytes.Equal(forwarded, compressed) {
		t.Fatalf("forwarded %d bytes, want the original %d compressed bytes",
			len(forwarded), len(compressed))
	}
	if got := res.Header.Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}

	// Usage came from the decompressed plaintext.
	usage := ai.GetTokenUsage()
	if usage.PromptTokens != 296476 {
		t.Errorf("PromptTokens = %d, want 296476", usage.PromptTokens)
	}
	if usage.CompletionTokens != 160 {
		t.Errorf("CompletionTokens = %d, want 160", usage.CompletionTokens)
	}
	if usage.CacheReadTokens != 296320 {
		t.Errorf("CacheReadTokens = %d, want 296320", usage.CacheReadTokens)
	}
	if !ai.IsFinalUsageSeen() {
		t.Error("IsFinalUsageSeen() = false, want true for a usage-bearing stream")
	}
}

func TestDoResponseProcessCompressedSSEDeflateUsage(t *testing.T) {
	m := NewModuleBodyProcess()
	req, ai := newStreamTestRequest(bfe_basic.AuthStyleOpenAI)
	ai.SetAllowEstimateToken(true)

	plain := []byte(openAISSEUsageChunk + openAISSEDone)
	compressed := zlibBytes(t, plain)
	res := newCompressedSSEResponse("deflate", compressed)

	bp := m.DoResponseProcess(req, res, nil)
	if bp == nil {
		t.Fatal("expected a BodyProcessor for an AI response")
	}
	forwarded, err := io.ReadAll(bp)
	if err != nil {
		t.Fatalf("read processed body failed: %s", err)
	}
	if !bytes.Equal(forwarded, compressed) {
		t.Fatal("forwarded bytes differ from the compressed input")
	}
	if got := ai.GetTokenUsage().CacheReadTokens; got != 296320 {
		t.Errorf("CacheReadTokens = %d, want 296320", got)
	}
}

func TestDoResponseProcessCompressedSSEAnthropicCacheRead(t *testing.T) {
	m := NewModuleBodyProcess()
	req, ai := newStreamTestRequest(bfe_basic.AuthStyleAnthropic)
	ai.SetAllowEstimateToken(true)

	plain := []byte("event: message_start\n" +
		`data: {"type":"message_start","message":{"usage":{"input_tokens":425,"cache_read_input_tokens":36096,"cache_creation_input_tokens":0,"output_tokens":0}}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","usage":{"output_tokens":52}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n")
	compressed := gzipBytes(t, plain)
	res := newCompressedSSEResponse("gzip", compressed)

	bp := m.DoResponseProcess(req, res, nil)
	if bp == nil {
		t.Fatal("expected a BodyProcessor for an AI response")
	}
	if _, err := io.ReadAll(bp); err != nil {
		t.Fatalf("read processed body failed: %s", err)
	}

	usage := ai.GetTokenUsage()
	if usage.CacheReadTokens != 36096 {
		t.Errorf("CacheReadTokens = %d, want 36096", usage.CacheReadTokens)
	}
	if usage.CompletionTokens != 52 {
		t.Errorf("CompletionTokens = %d, want 52", usage.CompletionTokens)
	}
	if !ai.IsFinalUsageSeen() {
		t.Error("IsFinalUsageSeen() = false, want true (message_delta usage)")
	}
	if !ai.IsResponseCompleted() {
		t.Error("IsResponseCompleted() = false, want true (message_stop)")
	}
}

// oneByteReader returns a single byte per Read to exercise the incremental
// decompression and event boundaries of the tap.
type oneByteReader struct {
	r io.Reader
}

func (o *oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return o.r.Read(p[:1])
}

func TestCompressedStreamDecoderSplitReads(t *testing.T) {
	plain := []byte(openAISSEUsageChunk + openAISSEDone)
	compressed := gzipBytes(t, plain)

	dec, err := NewCompressedStreamDecoder(&oneByteReader{r: bytes.NewReader(compressed)},
		"gzip", NewSSEEventDecoder)
	if err != nil {
		t.Fatalf("NewCompressedStreamDecoder: %s", err)
	}
	defer dec.Close()

	req, ai := newStreamTestRequest(bfe_basic.AuthStyleOpenAI)
	ai.SetAllowEstimateToken(true)
	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	proc := NewQuotaUsageProcessor(req, res)

	var forwarded []byte
	for {
		events, derr := dec.Decode()
		if derr != nil {
			t.Fatalf("Decode: %s", derr)
		}
		if len(events) == 0 {
			break
		}
		if _, perr := proc.Process(events); perr != nil {
			t.Fatalf("Process: %s", perr)
		}
		for _, ev := range events {
			forwarded = append(forwarded, ev.ToBytes()...)
		}
	}

	if !bytes.Equal(forwarded, compressed) {
		t.Fatalf("forwarded %d bytes, want the original %d compressed bytes",
			len(forwarded), len(compressed))
	}
	if got := ai.GetTokenUsage().CacheReadTokens; got != 296320 {
		t.Errorf("CacheReadTokens = %d, want 296320", got)
	}
}

func TestCompressedStreamDecoderCorruptFailsOpen(t *testing.T) {
	// A gzip header followed by garbage: the decoder fails mid-stream, so the
	// tap must switch to pass-through instead of truncating the forward.
	truncated := gzipBytes(t, []byte(openAISSEUsageChunk))[:20]

	dec, err := NewCompressedStreamDecoder(bytes.NewReader(truncated), "gzip", NewSSEEventDecoder)
	if err != nil {
		t.Fatalf("NewCompressedStreamDecoder: %s", err)
	}
	defer dec.Close()

	var forwarded []byte
	for {
		events, derr := dec.Decode()
		if derr != nil {
			t.Fatalf("Decode must fail open, got: %s", derr)
		}
		if len(events) == 0 {
			break
		}
		for _, ev := range events {
			forwarded = append(forwarded, ev.ToBytes()...)
		}
	}

	if !bytes.Equal(forwarded, truncated) {
		t.Fatalf("forwarded %d bytes, want the %d input bytes", len(forwarded), len(truncated))
	}
}

func TestCompressedStreamDecoderEstimateUsesPlaintext(t *testing.T) {
	// A quantitiy of plaintext far larger than its compressed form, with no
	// usage object: the estimate must follow the plaintext length, not the
	// compressed one (issue #1468 side effect on the estimate yardstick).
	var plain bytes.Buffer
	for i := 0; i < 200; i++ {
		plain.WriteString("data: {\"choices\":[{\"delta\":{\"content\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"}}]}\n\n")
	}
	plain.WriteString(openAISSEDone)
	compressed := gzipBytes(t, plain.Bytes())
	if len(compressed)*3 > len(plain.Bytes()) {
		t.Fatalf("test data is not compressible enough: %d -> %d", len(plain.Bytes()), len(compressed))
	}

	dec, err := NewCompressedStreamDecoder(bytes.NewReader(compressed), "gzip", NewSSEEventDecoder)
	if err != nil {
		t.Fatalf("NewCompressedStreamDecoder: %s", err)
	}
	defer dec.Close()

	req, ai := newStreamTestRequest(bfe_basic.AuthStyleOpenAI)
	ai.SetAllowEstimateToken(true)
	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	proc := NewQuotaUsageProcessor(req, res)

	for {
		events, derr := dec.Decode()
		if derr != nil {
			t.Fatalf("Decode: %s", derr)
		}
		if len(events) == 0 {
			break
		}
		if _, perr := proc.Process(events); perr != nil {
			t.Fatalf("Process: %s", perr)
		}
	}

	usage := ai.GetTokenUsage()
	if ai.IsFinalUsageSeen() {
		t.Error("IsFinalUsageSeen() = true, want false: the stream carries no usage")
	}
	// Estimation roughly plaintext/4. The compressed length is more than 3x
	// smaller, so an estimate based on compressed bytes cannot reach this value.
	minWant := int64(len(plain.Bytes()) / 8)
	if usage.CompletionTokens < minWant {
		t.Errorf("CompletionTokens = %d, want >= %d (estimated from the decompressed bytes)",
			usage.CompletionTokens, minWant)
	}
}

func TestCompressedStreamDecoderRawOnlyCallback(t *testing.T) {
	truncated := gzipBytes(t, []byte("data: {}\n\n"))[:20]

	dec, err := NewCompressedStreamDecoder(bytes.NewReader(truncated), "gzip", NewSSEEventDecoder)
	if err != nil {
		t.Fatalf("NewCompressedStreamDecoder: %s", err)
	}

	calls := 0
	dec.SetRawOnlyCallback(func() { calls++ })

	for {
		events, derr := dec.Decode()
		if derr != nil {
			t.Fatalf("Decode: %s", derr)
		}
		if len(events) == 0 {
			break
		}
	}

	if calls != 1 {
		t.Errorf("raw-only callback called %d times, want exactly 1", calls)
	}
}

func TestNewCompressedStreamDecoderUnsupported(t *testing.T) {
	if _, err := NewCompressedStreamDecoder(bytes.NewReader([]byte("x")), "zstd", NewSSEEventDecoder); err == nil {
		t.Fatal("expected an error for an unsupported content-encoding")
	}
}

// TestDoResponseProcessCompressedNonSSEStaysLineDecoder pins issue #1406: a
// compressed non-streaming body is still passed through byte-exact (its usage is
// parsed from the decoded copy in mod_ai_token_auth).
func TestDoResponseProcessCompressedNonSSEStaysLineDecoder(t *testing.T) {
	m := NewModuleBodyProcess()
	req, ai := newStreamTestRequest(bfe_basic.AuthStyleOpenAI)
	ai.SetAllowEstimateToken(true)

	plain := []byte(`{"usage":{"prompt_tokens":10,"completion_tokens":2}}`)
	compressed := gzipBytes(t, plain)
	res := &bfe_http.Response{
		StatusCode:    bfe_http.StatusOK,
		ContentLength: -1,
		Body:          io.NopCloser(bytes.NewReader(compressed)),
		Header:        make(bfe_http.Header),
	}
	res.Header.Set("Content-Type", "application/json; charset=utf-8")
	res.Header.Set("Content-Encoding", "gzip")

	bp := m.DoResponseProcess(req, res, nil)
	if bp == nil {
		t.Fatal("expected a BodyProcessor for an AI response")
	}
	forwarded, err := io.ReadAll(bp)
	if err != nil {
		t.Fatalf("read processed body failed: %s", err)
	}
	if !bytes.Equal(forwarded, compressed) {
		t.Fatal("forwarded bytes differ from the compressed input")
	}
	if _, ok := bp.decoder.(*CompressedStreamDecoder); ok {
		t.Error("a compressed non-SSE body must not use the streaming tap")
	}
}

// TestDoResponseProcessCompressedUnsupportedEncoding pins the fail-open path:
// an encoding outside the decodable set keeps the issue #1406 pass-through and
// the request falls back to the estimate.
func TestDoResponseProcessCompressedUnsupportedEncoding(t *testing.T) {
	m := NewModuleBodyProcess()
	req, ai := newStreamTestRequest(bfe_basic.AuthStyleOpenAI)
	ai.SetAllowEstimateToken(true)

	body := []byte("not really zstd but never decoded either")
	res := newCompressedSSEResponse("zstd", body)

	bp := m.DoResponseProcess(req, res, nil)
	if bp == nil {
		t.Fatal("expected a BodyProcessor for an AI response")
	}
	forwarded, err := io.ReadAll(bp)
	if err != nil {
		t.Fatalf("read processed body failed: %s", err)
	}
	if !bytes.Equal(forwarded, body) {
		t.Fatal("forwarded bytes differ from the input")
	}
	if _, ok := bp.decoder.(*CompressedStreamDecoder); ok {
		t.Error("an unsupported content-encoding must not use the streaming tap")
	}
}

// TestDoResponseProcessCompressedNoQuotaProcessor pins that the tap is only
// built when something actually reads usage: a non-2xx response has no
// QuotaUsageProcessor, so a configured audit rule must keep the byte-exact
// pass-through instead of paying for a decompression nobody consumes.
func TestDoResponseProcessCompressedNoQuotaProcessor(t *testing.T) {
	m := NewModuleBodyProcess()
	req, _ := newStreamTestRequest(bfe_basic.AuthStyleOpenAI)

	plain := []byte(openAISSEUsageChunk)
	compressed := gzipBytes(t, plain)
	res := newCompressedSSEResponse("gzip", compressed)
	res.StatusCode = bfe_http.StatusInternalServerError

	conf := &BodyProcessConfig{Dec: "sse"}
	bp := m.DoResponseProcess(req, res, conf)
	if bp == nil {
		t.Fatal("expected a BodyProcessor with an explicit response config")
	}
	if _, err := io.ReadAll(bp); err != nil {
		t.Fatalf("read processed body failed: %s", err)
	}
	if _, ok := bp.decoder.(*CompressedStreamDecoder); ok {
		t.Error("the tap must not be built without a quota usage processor")
	}
}
