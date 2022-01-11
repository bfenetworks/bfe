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

package bfe_util

import (
	"errors"
	"io"
	"sync"
)

import (
	"github.com/bfenetworks/bfe/bfe_http"
)

var copyPool sync.Pool

// errInvalidWrite means that a writer returned an impossible count.
var errInvalidWrite = errors.New("invalid write result")

func newByteBuf() []byte {
	if v := copyPool.Get(); v != nil {
		return v.([]byte)
	}
	return make([]byte, 32*1024)
}

func putByteBuf(buf []byte) {
	copyPool.Put(buf)
}

// CopyWithoutBuffer mimic the behavior of io.Copy.
func CopyWithoutBuffer(wf bfe_http.WriteFlusher, src io.Reader) (written int64, err error) {
	buf := newByteBuf()
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := wf.Write(buf[0:nr])
			// flush immediately regardless of write result.
			ef := wf.Flush()
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errInvalidWrite
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if ef != nil {
				err = ef
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	putByteBuf(buf)
	return written, err
}
