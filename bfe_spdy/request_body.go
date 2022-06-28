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

package bfe_spdy

import (
	"io"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/pipe"
)

type RequestBody struct {
	stream        *stream
	conn          *serverConn
	closed        bool
	pipe          *pipe.Pipe // non-nil if we have a HTTP entity message body
	needsContinue bool       // need to send a 100-continue
}

func (b *RequestBody) Read(p []byte) (n int, err error) {
	if b.needsContinue {
		b.needsContinue = false
		b.conn.write100ContinueHeaders(b.stream)
	}
	if b.pipe == nil {
		return 0, io.EOF
	}
	n, err = b.pipe.Read(p)
	if n > 0 {
		b.conn.noteBodyReadFromHandler(b.stream, n)
	}
	return
}

func (b *RequestBody) Close() error {
	if b.pipe != nil {
		b.pipe.CloseWithError(errClosedBody)
	}
	b.closed = true
	return nil
}

// Eof check whether without entity body
func (b *RequestBody) Eof() bool {
	return b.pipe == nil
}
