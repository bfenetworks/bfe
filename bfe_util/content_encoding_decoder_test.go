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
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"testing"
)

func gzipCompress(t *testing.T, data []byte) []byte {
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

func zlibCompress(t *testing.T, data []byte) []byte {
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

func flateCompress(t *testing.T, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	fw, err := flate.NewWriter(&buf, flate.BestSpeed)
	if err != nil {
		t.Fatalf("flate writer: %s", err)
	}
	if _, werr := fw.Write(data); werr != nil {
		t.Fatalf("flate write: %s", werr)
	}
	if cerr := fw.Close(); cerr != nil {
		t.Fatalf("flate close: %s", cerr)
	}
	return buf.Bytes()
}

// TestNewContentDecoderRoundTrip covers every coding the gateway claims to
// decode, including both shapes of "deflate" (issue #1468).
func TestNewContentDecoderRoundTrip(t *testing.T) {
	plain := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")

	cases := []struct {
		name     string
		encoding string
		body     []byte
	}{
		{"gzip", "gzip", gzipCompress(t, plain)},
		{"x-gzip", "x-gzip", gzipCompress(t, plain)},
		{"deflate-zlib", "deflate", zlibCompress(t, plain)},
		{"deflate-raw", "deflate", flateCompress(t, plain)},
		{"identity", "identity", plain},
		{"empty", "", plain},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec, err := NewContentDecoder(tc.encoding, bytes.NewReader(tc.body))
			if err != nil {
				t.Fatalf("NewContentDecoder(%q): %s", tc.encoding, err)
			}
			defer dec.Close()

			got, err := io.ReadAll(dec)
			if err != nil {
				t.Fatalf("read: %s", err)
			}
			if !bytes.Equal(got, plain) {
				t.Errorf("decoded = %q, want %q", got, plain)
			}
		})
	}
}

func TestNewContentDecoderMultiValue(t *testing.T) {
	plain := []byte("payload")
	// "deflate, gzip" means the body was deflated and then gzipped, so the
	// decoder must undo gzip first and deflate second.
	nested := gzipCompress(t, zlibCompress(t, plain))

	dec, err := NewContentDecoder("deflate, gzip", bytes.NewReader(nested))
	if err != nil {
		t.Fatalf("NewContentDecoder: %s", err)
	}
	defer dec.Close()

	got, err := io.ReadAll(dec)
	if err != nil {
		t.Fatalf("read: %s", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("decoded = %q, want %q", got, plain)
	}
}

func TestNewContentDecoderUnsupported(t *testing.T) {
	for _, encoding := range []string{"br", "zstd", "compress"} {
		dec, err := NewContentDecoder(encoding, bytes.NewReader([]byte("payload")))
		if err == nil {
			dec.Close()
			t.Errorf("encoding %q: expected an error", encoding)
		}
	}
}

// TestCanDecodeContentEncoding pins the decodable set: it is what the outbound
// Accept-Encoding is narrowed to, so a coding must not be reported as decodable
// without a decoder behind it.
func TestCanDecodeContentEncoding(t *testing.T) {
	decodable := []string{"", "identity", "gzip", "x-gzip", "deflate", "GZIP", "gzip, deflate"}
	for _, encoding := range decodable {
		if !CanDecodeContentEncoding(encoding) {
			t.Errorf("CanDecodeContentEncoding(%q) = false, want true", encoding)
		}
	}

	undecodable := []string{"br", "zstd", "compress", "dcb", "gzip, zstd"}
	for _, encoding := range undecodable {
		if CanDecodeContentEncoding(encoding) {
			t.Errorf("CanDecodeContentEncoding(%q) = true, want false", encoding)
		}
	}
}

func TestSupportedContentEncodings(t *testing.T) {
	got := SupportedContentEncodings()
	if len(got) != 2 || got[0] != "gzip" || got[1] != "deflate" {
		t.Fatalf("SupportedContentEncodings() = %v, want [gzip deflate]", got)
	}

	// The returned slice must be a copy: a caller mutating it must not change
	// the negotiation set.
	got[0] = "zstd"
	if SupportedContentEncodings()[0] != "gzip" {
		t.Error("SupportedContentEncodings() must return a copy")
	}
}

// TestDecodeContentEncodingZstdFailsOpen pins that the deliberately unsupported
// codings stay fail-open on the non-streaming path (issue #1406 semantics).
func TestDecodeContentEncodingZstdFailsOpen(t *testing.T) {
	plain := []byte("payload")

	got, err := DecodeContentEncoding("zstd", plain)
	if err == nil {
		t.Fatal("expected an error for zstd (not supported)")
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("got %q, want the input returned unchanged %q", got, plain)
	}
}

func TestDecodeContentEncodingDeflate(t *testing.T) {
	plain := []byte(`{"usage":{"input_tokens":10,"output_tokens":2}}`)

	for name, body := range map[string][]byte{
		"zlib": zlibCompress(t, plain),
		"raw":  flateCompress(t, plain),
	} {
		got, err := DecodeContentEncoding("deflate", body)
		if err != nil {
			t.Fatalf("%s: decode deflate: %s", name, err)
		}
		if !bytes.Equal(got, plain) {
			t.Errorf("%s: decoded = %q, want %q", name, got, plain)
		}
	}
}
