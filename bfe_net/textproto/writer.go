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

// Copyright 2010 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package textproto

import (
	"fmt"
	"io"
)

import (
	"github.com/bfenetworks/bfe/bfe_bufio"
)

// A Writer implements convenience methods for writing
// requests or responses to a text protocol network connection.
type Writer struct {
	W   *bfe_bufio.Writer
	dot *dotWriter
}

// NewWriter returns a new Writer writing to w.
func NewWriter(w *bfe_bufio.Writer) *Writer {
	return &Writer{W: w}
}

var crnl = []byte{'\r', '\n'}
var dotcrnl = []byte{'.', '\r', '\n'}

// PrintfLine writes the formatted output followed by \r\n.
func (w *Writer) PrintfLine(format string, args ...interface{}) error {
	w.closeDot()
	fmt.Fprintf(w.W, format, args...)
	w.W.Write(crnl)
	return w.W.Flush()
}

// DotWriter returns a writer that can be used to write a dot-encoding to w.
// It takes care of inserting leading dots when necessary,
// translating line-ending \n into \r\n, and adding the final .\r\n line
// when the DotWriter is closed.  The caller should close the
// DotWriter before the next call to a method on w.
//
// See the documentation for Reader's DotReader method for details about dot-encoding.
func (w *Writer) DotWriter() io.WriteCloser {
	w.closeDot()
	w.dot = &dotWriter{w: w}
	return w.dot
}

func (w *Writer) closeDot() {
	if w.dot != nil {
		w.dot.Close() // sets w.dot = nil
	}
}

type dotWriter struct {
	w     *Writer
	state int
}

const (
	wstateBegin     = iota // initial state; must be zero
	wstateBeginLine        // beginning of line
	wstateCR               // wrote \r (possibly at end of line)
	wstateData             // writing data in middle of line
)

func (d *dotWriter) Write(b []byte) (n int, err error) {
	bw := d.w.W
	for n < len(b) {
		c := b[n]
		switch d.state {
		case wstateBegin, wstateBeginLine:
			d.state = wstateData
			if c == '.' {
				// escape leading dot
				bw.WriteByte('.')
			}
			fallthrough

		case wstateData:
			if c == '\r' {
				d.state = wstateCR
			}
			if c == '\n' {
				bw.WriteByte('\r')
				d.state = wstateBeginLine
			}

		case wstateCR:
			d.state = wstateData
			if c == '\n' {
				d.state = wstateBeginLine
			}
		}
		if err = bw.WriteByte(c); err != nil {
			break
		}
		n++
	}
	return
}

func (d *dotWriter) Close() error {
	if d.w.dot == d {
		d.w.dot = nil
	}
	bw := d.w.W
	switch d.state {
	default:
		bw.WriteByte('\r')
		fallthrough
	case wstateCR:
		bw.WriteByte('\n')
		fallthrough
	case wstateBeginLine:
		bw.Write(dotcrnl)
	}
	return bw.Flush()
}
