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
	"io"
	"strings"
	"testing"

	"github.com/bfenetworks/bfe/bfe_http"
)

// TestNewContentTypeDecoderMediaTypeParams verifies that a media type with
// parameters (issue #1406: "application/json; charset=utf-8") still selects
// the JSON/SSE decoder instead of falling back to the line decoder.
func TestNewContentTypeDecoderMediaTypeParams(t *testing.T) {
	cases := []struct {
		contentType string
		wantJSON    bool
		wantSSE     bool
	}{
		{"application/json", true, false},
		{"application/json; charset=utf-8", true, false},
		{"application/x-ndjson", true, false},
		{"text/event-stream", false, true},
		{"text/event-stream; charset=utf-8", false, true},
		{"application/x-sse", false, true},
		{"text/plain", false, false},
	}

	for _, c := range cases {
		dec, err := NewContentTypeDecoder(strings.NewReader(""), c.contentType)
		if err != nil {
			t.Fatalf("%s: NewContentTypeDecoder failed: %s", c.contentType, err)
		}

		ctd, ok := dec.(*ContentTypeDecoder)
		if !ok {
			t.Fatalf("%s: expected *ContentTypeDecoder, got %T", c.contentType, dec)
		}

		_, isJSON := ctd.dec.(*JsonDecoder)
		_, isSSE := ctd.dec.(*SSEEventDecoder)
		if isJSON != c.wantJSON || isSSE != c.wantSSE {
			t.Errorf("%s: json=%v sse=%v, want json=%v sse=%v",
				c.contentType, isJSON, isSSE, c.wantJSON, c.wantSSE)
		}
	}
}

// TestDoResponseProcessSseContentTypeWithCharset verifies the response path
// selects the SSE decoder from res.IsSse even when the Content-Type carries a
// charset parameter.
func TestDoResponseProcessSseContentTypeWithCharset(t *testing.T) {
	m := NewModuleBodyProcess()
	req := newTestRequest("AI_product")
	req.InitAiBasicInfo()

	res := &bfe_http.Response{
		StatusCode: bfe_http.StatusOK,
		IsSse:      true,
		Body:       io.NopCloser(bytes.NewBufferString("data: {}\n\n")),
		Header:     bfe_http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}},
	}

	bp := m.DoResponseProcess(req, res, nil)
	if bp == nil {
		t.Fatal("expected a BodyProcessor")
	}
	if _, ok := bp.decoder.(*SSEEventDecoder); !ok {
		t.Errorf("expected the SSE event decoder, got %T", bp.decoder)
	}
}

// TestDoResponseProcessCompressedPassThrough verifies a compressed body is not
// routed to a strict decoder: JSON decoding of the compressed bytes would fail
// and truncate the forwarded body. The bytes must be forwarded verbatim
// (issue #1406).
func TestDoResponseProcessCompressedPassThrough(t *testing.T) {
	m := NewModuleBodyProcess()
	req := newTestRequest("AI_product")
	req.InitAiBasicInfo()

	payload := []byte{0x1f, 0x8b, 0x08, 0x00, 'n', 'o', 't', '\n', 'j', 's', 'o', 'n'}
	res := &bfe_http.Response{
		StatusCode: bfe_http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(payload)),
		Header: bfe_http.Header{
			"Content-Type":     {"application/json; charset=utf-8"},
			"Content-Encoding": {"gzip"},
		},
	}

	bp := m.DoResponseProcess(req, res, nil)
	if bp == nil {
		t.Fatal("expected a BodyProcessor")
	}
	if _, ok := bp.decoder.(*LineDecoder); !ok {
		t.Errorf("expected the line (pass-through) decoder, got %T", bp.decoder)
	}

	forwarded, err := io.ReadAll(bp)
	if err != nil {
		t.Fatalf("read forwarded body: %s", err)
	}
	if !bytes.Equal(forwarded, payload) {
		t.Errorf("forwarded body changed: got %q, want %q", forwarded, payload)
	}
}
