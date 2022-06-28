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

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// HTTP server.  See RFC 2616.

package bfe_server

import (
	"errors"
	"io"
)

import (
	"github.com/bfenetworks/bfe/bfe_http"
)

// wrapper around io.ReaderCloser which on first read, sends an
// HTTP/1.1 100 Continue header
type expectContinueReader struct {
	resp       *response
	readCloser io.ReadCloser
	closed     atomicBool
	sawEOF     atomicBool
}

func (ecr *expectContinueReader) tryWriteContinue() {
	w := ecr.resp

	if !w.wroteContinue && w.canWriteContinue.isSet() {
		w.wroteContinue = true
		w.writeContinueMu.Lock()
		if w.canWriteContinue.isSet() {
			ecr.resp.conn.buf.WriteString("HTTP/1.1 100 Continue\r\n\r\n")
			ecr.resp.conn.buf.Flush()
			w.canWriteContinue.setFalse()
		}
		w.writeContinueMu.Unlock()
	}
}

func (ecr *expectContinueReader) Read(p []byte) (n int, err error) {
	if ecr.closed.isSet() {
		return 0, bfe_http.ErrBodyReadAfterClose
	}

	ecr.tryWriteContinue()
	n, err = ecr.readCloser.Read(p)
	if err == io.EOF {
		ecr.sawEOF.setTrue()
	}
	return
}

func (ecr *expectContinueReader) Close() error {
	ecr.closed.setTrue()
	return ecr.readCloser.Close()
}

var ErrExpectContinueReaderPeek = errors.New("http: expect continue reader peek failed")

// Peek add peek function which is used by access log module
func (ecr *expectContinueReader) Peek(n int) ([]byte, error) {
	if ecr.closed.isSet() {
		return nil, bfe_http.ErrBodyReadAfterClose
	}

	// Ensure that "100-continue" has been written before peeking
	ecr.tryWriteContinue()
	if p, ok := ecr.readCloser.(bfe_http.Peeker); ok {
		n, err := p.Peek(n)
		if err == io.EOF {
			ecr.sawEOF.setTrue()
		}
		return n, err
	}
	return nil, ErrExpectContinueReaderPeek
}

// WroteContinue check whether expectContinueReader has sent 100-Continue response
func (ecr *expectContinueReader) WroteContinue() bool {
	return ecr.resp.wroteContinue
}
