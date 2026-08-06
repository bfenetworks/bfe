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

package mod_static

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/bfenetworks/bfe/bfe_http"
)

func prepareStaticRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("./testdata", "mod_static_file_")
	if err != nil {
		t.Fatalf("MkdirTemp() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	if err := os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("plain text"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	var gzBuf bytes.Buffer
	gzWriter := gzip.NewWriter(&gzBuf)
	if _, err := gzWriter.Write([]byte("plain text")); err != nil {
		t.Fatalf("gzip Write() error: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("gzip Close() error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain.txt.gz"), gzBuf.Bytes(), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "plain.txt.br"), []byte("brotli bytes"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "empty"), []byte{}, 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return filepath.ToSlash(dir)
}

func TestConvertEncodeToExt(t *testing.T) {
	tests := []struct {
		encoding string
		want     string
	}{
		{EncodeGzip, FileExtensionGzip},
		{EncodeBrotil, FileExtensionBrotil},
		{"deflate", "deflate"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.encoding, func(t *testing.T) {
			got := ConvertEncodeToExt(tt.encoding)
			if got != tt.want {
				t.Errorf("ConvertEncodeToExt(%q) = %q, want %q", tt.encoding, got, tt.want)
			}
		})
	}
}

func TestCheckAcceptEncoding(t *testing.T) {
	buildReq := func(value string) *bfe_http.Request {
		req, _ := bfe_http.NewRequest("GET", "http://www.example.org/", nil)
		if value != "" {
			req.Header.Set("Accept-Encoding", value)
		}
		return req
	}

	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{"empty", "", []string{}},
		{"gzip", "gzip", []string{EncodeGzip}},
		{"br", "br", []string{EncodeBrotil}},
		{"gzip and br", "gzip, br", []string{EncodeGzip, EncodeBrotil}},
		{"br and gzip", "br, gzip", []string{EncodeGzip, EncodeBrotil}},
		{"with deflate", "deflate, gzip", []string{EncodeGzip}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckAcceptEncoding(buildReq(tt.value))
			if len(got) != len(tt.want) {
				t.Errorf("CheckAcceptEncoding(%q) = %v, want %v", tt.value, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("CheckAcceptEncoding(%q) = %v, want %v", tt.value, got, tt.want)
					return
				}
			}
		})
	}
}

func TestNewStaticFilePlain(t *testing.T) {
	root := prepareStaticRoot(t)
	m := NewModuleStatic()

	file, err := newStaticFile(root, "/plain.txt", nil, m)
	if err != nil {
		t.Fatalf("newStaticFile() error: %v", err)
	}
	defer file.Close()

	if file.extension != ".txt" {
		t.Errorf("extension should be .txt, got %s", file.extension)
	}
	if file.encoding != "" {
		t.Errorf("encoding should be empty, got %s", file.encoding)
	}
	if file.Size() != 10 {
		t.Errorf("size should be 10, got %d", file.Size())
	}

	body, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	if string(body) != "plain text" {
		t.Errorf("body should be \"plain text\", got %s", string(body))
	}
}

func TestNewStaticFileGzip(t *testing.T) {
	root := prepareStaticRoot(t)
	m := NewModuleStatic()

	file, err := newStaticFile(root, "/plain.txt", []string{EncodeGzip}, m)
	if err != nil {
		t.Fatalf("newStaticFile() error: %v", err)
	}
	defer file.Close()

	if file.encoding != EncodeGzip {
		t.Errorf("encoding should be gzip, got %s", file.encoding)
	}
	if file.extension != ".txt" {
		t.Errorf("extension should be .txt, got %s", file.extension)
	}
}

func TestNewStaticFileBrotli(t *testing.T) {
	root := prepareStaticRoot(t)
	m := NewModuleStatic()

	file, err := newStaticFile(root, "/plain.txt", []string{EncodeBrotil}, m)
	if err != nil {
		t.Fatalf("newStaticFile() error: %v", err)
	}
	defer file.Close()

	if file.encoding != EncodeBrotil {
		t.Errorf("encoding should be br, got %s", file.encoding)
	}
}

func TestNewStaticFilePreferGzipWhenBoth(t *testing.T) {
	root := prepareStaticRoot(t)
	m := NewModuleStatic()

	file, err := newStaticFile(root, "/plain.txt", []string{EncodeGzip, EncodeBrotil}, m)
	if err != nil {
		t.Fatalf("newStaticFile() error: %v", err)
	}
	defer file.Close()

	if file.encoding != EncodeGzip {
		t.Errorf("encoding should be gzip when both present, got %s", file.encoding)
	}
}

func TestNewStaticFileNotExist(t *testing.T) {
	root := prepareStaticRoot(t)
	m := NewModuleStatic()

	_, err := newStaticFile(root, "/not_exist.txt", nil, m)
	if err == nil {
		t.Errorf("newStaticFile() should return error for nonexistent file")
	}
}

func TestNewStaticFileUnexpectedDir(t *testing.T) {
	root := prepareStaticRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "subdir"), 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	m := NewModuleStatic()

	_, err := newStaticFile(root, "/subdir", nil, m)
	if err != errUnexpectedDir {
		t.Errorf("newStaticFile() should return errUnexpectedDir, got %v", err)
	}
}

func TestStaticFileClose(t *testing.T) {
	root := prepareStaticRoot(t)
	m := NewModuleStatic()

	file, err := newStaticFile(root, "/plain.txt", nil, m)
	if err != nil {
		t.Fatalf("newStaticFile() error: %v", err)
	}

	if m.state.FileCurrentOpened.Get() != 1 {
		t.Errorf("FileCurrentOpened should be 1, got %d", m.state.FileCurrentOpened.Get())
	}

	if err := file.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}

	if m.state.FileCurrentOpened.Get() != 0 {
		t.Errorf("FileCurrentOpened should be 0 after close, got %d", m.state.FileCurrentOpened.Get())
	}
}

func TestErrorStatusCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"not exist", os.ErrNotExist, bfe_http.StatusNotFound},
		{"permission", os.ErrPermission, bfe_http.StatusForbidden},
		{"other", errors.New("some error"), bfe_http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errorStatusCode(tt.err)
			if got != tt.want {
				t.Errorf("errorStatusCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
