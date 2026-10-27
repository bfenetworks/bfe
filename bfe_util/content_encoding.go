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
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"strings"

	"github.com/bfenetworks/bfe/bfe_http"
)

// supportedContentEncodings is the set of Content-Encoding codings the gateway
// can decode, in preference order. It is deliberately small (issue #1468): the
// outbound Accept-Encoding of an AI request is narrowed to this set, so an
// upstream never answers with a coding that the streaming usage parser cannot
// read. Adding a coding here requires a decoder in newSingleContentDecoder.
var supportedContentEncodings = []string{"gzip", "deflate"}

// SupportedContentEncodings returns the codings accepted by NewContentDecoder
// and CanDecodeContentEncoding, in preference order.
func SupportedContentEncodings() []string {
	encodings := make([]string, len(supportedContentEncodings))
	copy(encodings, supportedContentEncodings)
	return encodings
}

// CanDecodeContentEncoding reports whether every coding of a Content-Encoding
// header value is supported by NewContentDecoder. An empty value and identity
// are always decodable.
func CanDecodeContentEncoding(encoding string) bool {
	for _, coding := range parseContentEncoding(encoding) {
		if !isSupportedContentEncoding(coding) {
			return false
		}
	}
	return true
}

// isSupportedContentEncoding reports whether a single coding can be decoded.
// It must stay in sync with newSingleContentDecoder.
func isSupportedContentEncoding(coding string) bool {
	switch coding {
	case "", "identity", "gzip", "x-gzip", "deflate":
		return true
	default:
		return false
	}
}

// NewContentDecoder returns a streaming decoder for the given Content-Encoding
// header value, reading the encoded bytes from r. A multi-value list is
// decoded in reverse application order.
//
// An unsupported coding returns an error so the caller can fall back to
// passing the encoded bytes through untouched.
func NewContentDecoder(encoding string, r io.Reader) (io.ReadCloser, error) {
	dec := io.NopCloser(r)

	encodings := parseContentEncoding(encoding)
	for i := len(encodings) - 1; i >= 0; i-- {
		next, err := newSingleContentDecoder(encodings[i], dec)
		if err != nil {
			dec.Close()
			return nil, err
		}
		dec = next
	}

	return dec, nil
}

// newSingleContentDecoder wraps src with the decoder of a single coding.
func newSingleContentDecoder(coding string, src io.Reader) (io.ReadCloser, error) {
	switch coding {
	case "", "identity":
		return io.NopCloser(src), nil
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(src)
		if err != nil {
			// gzip.NewReader reads the header eagerly; a truncated or
			// non-gzip body fails here. Fail open: let the caller keep the
			// encoded bytes untouched.
			return nil, err
		}
		return zr, nil
	case "deflate":
		return newDeflateDecoder(src)
	default:
		return nil, fmt.Errorf("unsupported content-encoding %q", coding)
	}
}

// newDeflateDecoder decodes an HTTP "deflate" body. RFC 9110 defines deflate
// as a zlib-wrapped stream, but some upstreams send raw DEFLATE, so the zlib
// header is sniffed before choosing the decoder.
func newDeflateDecoder(src io.Reader) (io.ReadCloser, error) {
	br := bufio.NewReader(src)
	if hdr, err := br.Peek(2); err == nil && isZlibHeader(hdr) {
		zr, zerr := zlib.NewReader(br)
		if zerr != nil {
			return nil, zerr
		}
		return zr, nil
	}
	return flate.NewReader(br), nil
}

// isZlibHeader reports whether b starts with a valid zlib header: CM=8
// (deflate), CINFO <= 7 and the two-byte value is a multiple of 31.
func isZlibHeader(b []byte) bool {
	return len(b) == 2 && b[0]&0x0f == 8 && b[0]>>4 <= 7 && (int(b[0])<<8|int(b[1]))%31 == 0
}

// DecodeContentEncoding decodes data according to the HTTP Content-Encoding
// header value, e.g. for a parse-only copy of a complete response body.
//
// The function fails open: an unknown encoding, an undecodable payload or one
// that decodes beyond the configured body buffer size returns the input bytes
// unchanged together with the error, so a caller that only wants the decoded
// form for its own parsing never breaks the request path.
func DecodeContentEncoding(encoding string, data []byte) ([]byte, error) {
	encodings := parseContentEncoding(encoding)
	if len(data) == 0 || len(encodings) == 0 {
		return data, nil
	}

	dec, err := NewContentDecoder(encoding, bytes.NewReader(data))
	if err != nil {
		return data, err
	}
	defer dec.Close()

	// Cap the decoded size to guard against a decompression bomb.
	maxSize := bfe_http.GetAccessibleBodySize()
	out, err := io.ReadAll(io.LimitReader(dec, maxSize+1))
	if err != nil {
		return data, err
	}
	if int64(len(out)) > maxSize {
		return data, fmt.Errorf("decoded content exceeds %d bytes", maxSize)
	}

	return out, nil
}

// parseContentEncoding splits an HTTP Content-Encoding header value into its
// individual encodings, lower-cased and stripped of empty items.
func parseContentEncoding(encoding string) []string {
	if encoding == "" {
		return nil
	}

	parts := strings.Split(encoding, ",")
	encodings := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			encodings = append(encodings, part)
		}
	}
	return encodings
}
