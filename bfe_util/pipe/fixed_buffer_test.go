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
	"reflect"
	"testing"
)

var bufferReadTests = []struct {
	buf      FixedBuffer
	read, wn int
	werr     error
	wp       []byte
	wbuf     FixedBuffer
}{
	{
		FixedBuffer{[]byte{'a', 0}, 0, 1},
		5, 1, nil, []byte{'a'},
		FixedBuffer{[]byte{'a', 0}, 0, 0},
	},
	{
		FixedBuffer{[]byte{0, 'a'}, 1, 2},
		5, 1, nil, []byte{'a'},
		FixedBuffer{[]byte{0, 'a'}, 0, 0},
	},
	{
		FixedBuffer{[]byte{'a', 'b'}, 0, 2},
		1, 1, nil, []byte{'a'},
		FixedBuffer{[]byte{'a', 'b'}, 1, 2},
	},
	{
		FixedBuffer{[]byte{}, 0, 0},
		5, 0, errReadEmpty, []byte{},
		FixedBuffer{[]byte{}, 0, 0},
	},
}

func TestBufferRead(t *testing.T) {
	for i, tt := range bufferReadTests {
		read := make([]byte, tt.read)
		n, err := tt.buf.Read(read)
		if n != tt.wn {
			t.Errorf("#%d: wn = %d want %d", i, n, tt.wn)
			continue
		}
		if err != tt.werr {
			t.Errorf("#%d: werr = %v want %v", i, err, tt.werr)
			continue
		}
		read = read[:n]
		if !reflect.DeepEqual(read, tt.wp) {
			t.Errorf("#%d: read = %+v want %+v", i, read, tt.wp)
		}
		if !reflect.DeepEqual(tt.buf, tt.wbuf) {
			t.Errorf("#%d: buf = %+v want %+v", i, tt.buf, tt.wbuf)
		}
	}
}

var bufferWriteTests = []struct {
	buf       FixedBuffer
	write, wn int
	werr      error
	wbuf      FixedBuffer
}{
	{
		buf: FixedBuffer{
			buf: []byte{},
		},
		wbuf: FixedBuffer{
			buf: []byte{},
		},
	},
	{
		buf: FixedBuffer{
			buf: []byte{1, 'a'},
		},
		write: 1,
		wn:    1,
		wbuf: FixedBuffer{
			buf: []byte{0, 'a'},
			w:   1,
		},
	},
	{
		buf: FixedBuffer{
			buf: []byte{'a', 1},
			r:   1,
			w:   1,
		},
		write: 2,
		wn:    2,
		wbuf: FixedBuffer{
			buf: []byte{0, 0},
			w:   2,
		},
	},
	{
		buf: FixedBuffer{
			buf: []byte{},
		},
		write: 5,
		werr:  errWriteFull,
		wbuf: FixedBuffer{
			buf: []byte{},
		},
	},
}

func TestBufferWrite(t *testing.T) {
	for i, tt := range bufferWriteTests {
		n, err := tt.buf.Write(make([]byte, tt.write))
		if n != tt.wn {
			t.Errorf("#%d: wrote %d bytes; want %d", i, n, tt.wn)
			continue
		}
		if err != tt.werr {
			t.Errorf("#%d: error = %v; want %v", i, err, tt.werr)
			continue
		}
		if !reflect.DeepEqual(tt.buf, tt.wbuf) {
			t.Errorf("#%d: buf = %+v; want %+v", i, tt.buf, tt.wbuf)
		}
	}
}
