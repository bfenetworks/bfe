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

package mod_compress

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/ioutil"
	"testing"
)

func TestGzipFilterCompressAndDecompress(t *testing.T) {
	data := prepareTestData(32 * 1024)

	source := prepareSource(data)
	f, err := NewGzipFilter(source, gzip.BestCompression, 512)
	if err != nil {
		t.Fatalf("NewGzipFilter() error: %v", err)
	}

	compressed, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	f.Close()

	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip.NewReader() error: %v", err)
	}
	defer reader.Close()

	got, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatalf("decompress error: %v", err)
	}

	if !bytes.Equal(got, data) {
		t.Fatalf("decompressed data does not match original")
	}
}

func TestGzipFilterInvalidLevel(t *testing.T) {
	source := prepareSource([]byte("hello"))
	_, err := NewGzipFilter(source, 100, 512)
	if err == nil {
		t.Fatalf("expected error for invalid gzip level")
	}
}

func TestGzipFilterReadError(t *testing.T) {
	wantErr := errors.New("source read error")
	source := &errReadCloser{err: wantErr}
	f, err := NewGzipFilter(source, gzip.BestCompression, 512)
	if err != nil {
		t.Fatalf("NewGzipFilter() error: %v", err)
	}

	_, err = f.Read(make([]byte, 1024))
	if err != wantErr {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}

func TestGzipFilterCloseError(t *testing.T) {
	wantErr := errors.New("source close error")
	source := &closeErrReadCloser{err: wantErr}
	f, err := NewGzipFilter(source, gzip.BestCompression, 512)
	if err != nil {
		t.Fatalf("NewGzipFilter() error: %v", err)
	}

	// Consume the filter so the writer is closed.
	io.Copy(ioutil.Discard, f)

	err = f.Close()
	if err != wantErr {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}
