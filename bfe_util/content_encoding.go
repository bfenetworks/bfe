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
	"fmt"
	"io"
	"strings"

	"github.com/bfenetworks/bfe/bfe_http"
)

// DecodeContentEncoding decodes data according to the HTTP Content-Encoding
// header value. Only gzip (and its x-gzip alias) is supported; a multi-value
// list is decoded in reverse application order.
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

	decoded := data
	for i := len(encodings) - 1; i >= 0; i-- {
		switch encodings[i] {
		case "gzip", "x-gzip":
			out, err := decodeGzip(decoded)
			if err != nil {
				return data, err
			}
			decoded = out
		case "identity":
			// nothing to do
		default:
			return data, fmt.Errorf("unsupported content-encoding %q", encodings[i])
		}
	}

	return decoded, nil
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

// decodeGzip gunzips data, capping the decoded size at the configured body
// buffer size to guard against a decompression bomb.
func decodeGzip(data []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	maxSize := bfe_http.GetAccessibleBodySize()
	out, err := io.ReadAll(io.LimitReader(zr, maxSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(out)) > maxSize {
		return nil, fmt.Errorf("decoded content exceeds %d bytes", maxSize)
	}
	return out, nil
}
