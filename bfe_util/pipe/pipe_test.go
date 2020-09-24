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
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"testing"
)

func TestPipeClose(t *testing.T) {
	var p Pipe
	p.b = new(bytes.Buffer)
	a := errors.New("a")
	b := errors.New("b")
	p.CloseWithError(a)
	p.CloseWithError(b)
	_, err := p.Read(make([]byte, 1))
	if err != a {
		t.Errorf("err = %v want %v", err, a)
	}
}

func TestPipeDoneChan(t *testing.T) {
	var p Pipe
	done := p.Done()
	select {
	case <-done:
		t.Fatal("done too soon")
	default:
	}
	p.CloseWithError(io.EOF)
	select {
	case <-done:
	default:
		t.Fatal("should be done")
	}
}

func TestPipeDoneChan_ErrFirst(t *testing.T) {
	var p Pipe
	p.CloseWithError(io.EOF)
	done := p.Done()
	select {
	case <-done:
	default:
		t.Fatal("should be done")
	}
}

func TestPipeDoneChan_Break(t *testing.T) {
	var p Pipe
	done := p.Done()
	select {
	case <-done:
		t.Fatal("done too soon")
	default:
	}
	p.BreakWithError(io.EOF)
	select {
	case <-done:
	default:
		t.Fatal("should be done")
	}
}

func TestPipeDoneChan_Break_ErrFirst(t *testing.T) {
	var p Pipe
	p.BreakWithError(io.EOF)
	done := p.Done()
	select {
	case <-done:
	default:
		t.Fatal("should be done")
	}
}

func TestPipeCloseWithError(t *testing.T) {
	p := &Pipe{b: new(bytes.Buffer)}
	const body = "foo"
	io.WriteString(p, body)
	a := errors.New("test error")
	p.CloseWithError(a)
	all, err := ioutil.ReadAll(p)
	if string(all) != body {
		t.Errorf("read bytes = %q; want %q", all, body)
	}
	if err != a {
		t.Logf("read error = %v, %v", err, a)
	}
}

func TestPipeBreakWithError(t *testing.T) {
	p := &Pipe{b: new(bytes.Buffer)}
	io.WriteString(p, "foo")
	a := errors.New("test err")
	p.BreakWithError(a)
	all, err := ioutil.ReadAll(p)
	if string(all) != "" {
		t.Errorf("read bytes = %q; want empty string", all)
	}
	if err != a {
		t.Logf("read error = %v, %v", err, a)
	}
}
