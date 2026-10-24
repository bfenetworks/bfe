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

package bfe_util

import (
	"bytes"
	"compress/gzip"
	"testing"
)

func gzipTestData(t *testing.T, data []byte) []byte {
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

func TestDecodeContentEncodingGzip(t *testing.T) {
	plain := []byte(`{"usage":{"input_tokens":425,"output_tokens":52}}`)

	got, err := DecodeContentEncoding("gzip", gzipTestData(t, plain))
	if err != nil {
		t.Fatalf("decode gzip: %s", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("decoded = %q, want %q", got, plain)
	}
}

func TestDecodeContentEncodingPassThrough(t *testing.T) {
	plain := []byte("hello")

	for _, encoding := range []string{"", "identity", "   "} {
		got, err := DecodeContentEncoding(encoding, plain)
		if err != nil {
			t.Fatalf("encoding %q: unexpected error: %s", encoding, err)
		}
		if !bytes.Equal(got, plain) {
			t.Errorf("encoding %q: got %q, want %q", encoding, got, plain)
		}
	}
}

func TestDecodeContentEncodingEmptyData(t *testing.T) {
	got, err := DecodeContentEncoding("gzip", nil)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(got) != 0 {
		t.Errorf("got %q, want empty", got)
	}
}

func TestDecodeContentEncodingUnknownFailsOpen(t *testing.T) {
	plain := []byte("hello")

	got, err := DecodeContentEncoding("br", plain)
	if err == nil {
		t.Fatal("expected an error for an unsupported encoding")
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("got %q, want the input returned unchanged %q", got, plain)
	}
}

func TestDecodeContentEncodingInvalidGzipFailsOpen(t *testing.T) {
	plain := []byte("this is not a gzip payload")

	got, err := DecodeContentEncoding("gzip", plain)
	if err == nil {
		t.Fatal("expected an error for an invalid gzip payload")
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("got %q, want the input returned unchanged %q", got, plain)
	}
}
