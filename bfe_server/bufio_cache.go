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
	"io"
	"sync"
)

import (
	bufio "github.com/bfenetworks/bfe/bfe_bufio"
)

var (
	bufioReaderPool    sync.Pool
	bufioWriter256Pool sync.Pool
	bufioWriter512Pool sync.Pool
	bufioWriter1kPool  sync.Pool
	bufioWriter2kPool  sync.Pool
	bufioWriter4kPool  sync.Pool
)

func bufioWriterPool(size int) *sync.Pool {
	switch size {
	case 1 << 8:
		return &bufioWriter256Pool
	case 1 << 9:
		return &bufioWriter512Pool
	case 1 << 10:
		return &bufioWriter1kPool
	case 2 << 10:
		return &bufioWriter2kPool
	case 4 << 10:
		return &bufioWriter4kPool
	}
	return nil
}

type BufioCache struct {
}

func NewBufioCache() *BufioCache {
	return new(BufioCache)
}

func (*BufioCache) newBufioReader(r io.Reader) *bufio.Reader {
	if v := bufioReaderPool.Get(); v != nil {
		br := v.(*bufio.Reader)
		br.Reset(r)
		return br
	}
	return bufio.NewReader(r)
}

func (*BufioCache) putBufioReader(br *bufio.Reader) {
	br.Reset(nil)
	bufioReaderPool.Put(br)
}

func (*BufioCache) newBufioWriterSize(w io.Writer, size int) *bufio.Writer {
	pool := bufioWriterPool(size)
	if pool != nil {
		if v := pool.Get(); v != nil {
			bw := v.(*bufio.Writer)
			bw.Reset(w)
			return bw
		}
	}
	return bufio.NewWriterSize(w, size)
}

func (*BufioCache) putBufioWriter(bw *bufio.Writer) {
	bw.Reset(nil)
	if pool := bufioWriterPool(bw.Available()); pool != nil {
		pool.Put(bw)
	}
}
