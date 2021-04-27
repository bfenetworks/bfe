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
	"bytes"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_bufio"
)

func TestPrintfLine(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(bfe_bufio.NewWriter(&buf))
	err := w.PrintfLine("foo %d", 123)
	if s := buf.String(); s != "foo 123\r\n" || err != nil {
		t.Fatalf("s=%q; err=%s", s, err)
	}
}

func TestDotWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(bfe_bufio.NewWriter(&buf))
	d := w.DotWriter()
	n, err := d.Write([]byte("abc\n.def\n..ghi\n.jkl\n."))
	if n != 21 || err != nil {
		t.Fatalf("Write: %d, %s", n, err)
	}
	d.Close()
	want := "abc\r\n..def\r\n...ghi\r\n..jkl\r\n..\r\n.\r\n"
	if s := buf.String(); s != want {
		t.Fatalf("wrote %q", s)
	}
}

func TestDotWriterCloseEmptyWrite(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(bfe_bufio.NewWriter(&buf))
	d := w.DotWriter()
	n, err := d.Write([]byte{})
	if n != 0 || err != nil {
		t.Fatalf("Write: %d, %s", n, err)
	}
	d.Close()
	want := "\r\n.\r\n"
	if s := buf.String(); s != want {
		t.Fatalf("wrote %q; want %q", s, want)
	}
}

func TestDotWriterCloseNoWrite(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(bfe_bufio.NewWriter(&buf))
	d := w.DotWriter()
	d.Close()
	want := "\r\n.\r\n"
	if s := buf.String(); s != want {
		t.Fatalf("wrote %q; want %q", s, want)
	}
}
