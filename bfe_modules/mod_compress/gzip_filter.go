// Copyright (c) 2019 The BFE Authors.
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
	"io"
)

type GzipFilter struct {
	source    io.ReadCloser
	writer    *gzip.Writer
	buffer    bytes.Buffer
	flushSize int64
	closed    bool
}

func NewGzipFilter(source io.ReadCloser, level int, size int) (b *GzipFilter, err error) {
	b = new(GzipFilter)
	b.writer, err = gzip.NewWriterLevel(&b.buffer, level)
	if err != nil {
		return nil, err
	}
	b.source = source
	b.flushSize = int64(size)
	return b, nil
}

func (b *GzipFilter) Read(p []byte) (n int, err error) {
	c, err := io.CopyN(b.writer, b.source, b.flushSize)
	if err != nil && err != io.EOF {
		return 0, err
	}

	if c != 0 {
		if err := b.writer.Flush(); err != nil {
			return 0, err
		}
	} else if !b.closed {
		b.closed = true
		if err := b.writer.Close(); err != nil {
			return 0, err
		}
	}

	return b.buffer.Read(p)
}

func (b *GzipFilter) Close() error {
	if err := b.source.Close(); err != nil {
		return err
	}
	return nil
}
