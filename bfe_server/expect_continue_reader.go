// Copyright (c) 2019 Baidu, Inc.
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
	"sync"
)

import (
	"github.com/bfenetworks/bfe/bfe_http"
)

// wrapper around io.ReaderCloser which on first read, sends an
// HTTP/1.1 100 Continue header
type expectContinueReader struct {
	resp       *response
	readCloser io.ReadCloser
	closed     bool
	mu         sync.Mutex
}

func (ecr *expectContinueReader) tryWriteContinue() {
	if !ecr.resp.wroteContinue {
		ecr.resp.wroteContinue = true
		ecr.resp.conn.buf.WriteString("HTTP/1.1 100 Continue\r\n\r\n")
		ecr.resp.conn.buf.Flush()
	}
}

func (ecr *expectContinueReader) Read(p []byte) (n int, err error) {
	ecr.mu.Lock()
	defer ecr.mu.Unlock()

	if ecr.closed {
		return 0, bfe_http.ErrBodyReadAfterClose
	}

	ecr.tryWriteContinue()
	return ecr.readCloser.Read(p)
}

func (ecr *expectContinueReader) Close() error {
	ecr.mu.Lock()
	defer ecr.mu.Unlock()

	ecr.closed = true
	return ecr.readCloser.Close()
}

var ErrExpectContinueReaderPeek = errors.New("http: expect continue reader peek failed")

// add peek function which is used by access log module
func (ecr *expectContinueReader) Peek(n int) ([]byte, error) {
	ecr.mu.Lock()
	defer ecr.mu.Unlock()

	if ecr.closed {
		return nil, bfe_http.ErrBodyReadAfterClose
	}

	// Ensure that "100-continue" has been written before peeking
	ecr.tryWriteContinue()
	if p, ok := ecr.readCloser.(bfe_http.Peeker); ok {
		return p.Peek(n)
	}
	return nil, ErrExpectContinueReaderPeek
}

// check whether expectContinueReader has sent 100-Continue response
func (ecr *expectContinueReader) WroteContinue() bool {
	ecr.mu.Lock()
	wroteContinue := ecr.resp.wroteContinue
	ecr.mu.Unlock()

	return wroteContinue
}
