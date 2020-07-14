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

// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipe

import (
	"errors"
)

// FixedBuffer is an io.ReadWriter backed by a fixed size buffer.
// It never allocates, but moves old data as new data is written.
type FixedBuffer struct {
	buf  []byte
	r, w int
}

var (
	errReadEmpty = errors.New("read from empty FixedBuffer")
	errWriteFull = errors.New("write on full FixedBuffer")
)

// Read copies bytes from the buffer into p.
// It is an error to read when no data is available.
func (b *FixedBuffer) Read(p []byte) (n int, err error) {
	if b.r == b.w {
		return 0, errReadEmpty
	}
	n = copy(p, b.buf[b.r:b.w])
	b.r += n
	if b.r == b.w {
		b.r = 0
		b.w = 0
	}
	return n, nil
}

// Len returns the number of bytes of the unread portion of the buffer.
func (b *FixedBuffer) Len() int {
	return b.w - b.r
}

// Write copies bytes from p into the buffer.
// It is an error to write more data than the buffer can hold.
func (b *FixedBuffer) Write(p []byte) (n int, err error) {
	// Slide existing data to beginning.
	if b.r > 0 && len(p) > len(b.buf)-b.w {
		copy(b.buf, b.buf[b.r:b.w])
		b.w -= b.r
		b.r = 0
	}

	// Write new data.
	n = copy(b.buf[b.w:], p)
	b.w += n
	if n < len(p) {
		err = errWriteFull
	}
	return n, err
}

func NewFixedBuffer(buf []byte) *FixedBuffer {
	b := new(FixedBuffer)
	b.buf = buf
	return b
}

// Reset clear state of FixedBuffer.
func (b *FixedBuffer) Reset() {
	b.w = 0
	b.r = 0
}
