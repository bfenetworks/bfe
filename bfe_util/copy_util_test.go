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
	"bytes"
	"fmt"
	"io"
	"testing"
	"testing/iotest"
)

import (
	"github.com/bfenetworks/bfe/bfe_bufio"
	"github.com/bfenetworks/bfe/bfe_http"
)

type writeFlusher struct {
	sink *bytes.Buffer
	buf  *bfe_bufio.Writer
}

func newWriteFlusher() *writeFlusher {
	wf := &writeFlusher{}
	wf.sink = &bytes.Buffer{}
	wf.buf = bfe_bufio.NewWriter(wf.sink)
	return wf
}

func (wf *writeFlusher) Write(p []byte) (n int, err error) {
	return wf.buf.Write(p)
}

func (wf *writeFlusher) Flush() error {
	return wf.buf.Flush()
}

func TestCopyWithoutBuffer(t *testing.T) {
	data1 := bytes.Repeat([]byte("1234567"), 1)
	data2 := bytes.Repeat([]byte("12"), 32*1024+1)
	cases := []struct {
		data    []byte
		wf      bfe_http.WriteFlusher
		r       io.Reader
		written int64
		err     error
	}{
		{
			data:    data1,
			r:       bytes.NewBuffer(data1),
			written: int64(len(data1)),
			err:     nil,
		},
		{
			data:    data1,
			r:       iotest.OneByteReader(bytes.NewBuffer(data1)),
			written: int64(len(data1)),
			err:     nil,
		},
		{
			data:    data1,
			r:       iotest.HalfReader(bytes.NewBuffer(data1)),
			written: int64(len(data1)),
			err:     nil,
		},
		{
			data:    data1,
			r:       iotest.TimeoutReader(bytes.NewBuffer(data1)),
			written: int64(len(data1)),
			err:     iotest.ErrTimeout,
		},
		{
			data:    data1,
			r:       iotest.DataErrReader(iotest.TimeoutReader(bytes.NewBuffer(data1))),
			written: int64(len(data1)),
			err:     iotest.ErrTimeout,
		},
		{
			data:    data2,
			r:       bytes.NewBuffer(data2),
			written: int64(len(data2)),
			err:     nil,
		},
		{
			data:    data2,
			r:       iotest.TimeoutReader(bytes.NewBuffer(data2)),
			written: 32 * 1024,
			err:     iotest.ErrTimeout,
		},
	}

	for i := range cases {
		wf := newWriteFlusher()
		written, err := CopyWithoutBuffer(wf, cases[i].r)
		expectWritten := cases[i].written
		if written != expectWritten {
			t.Fatalf("Case %d: wrong written, got:%d, expect:%d.\n", i, written, expectWritten)
		}
		expectErr := cases[i].err
		if err != nil || expectErr != nil {
			if (err != nil && expectErr == nil) || (err == nil && expectErr != nil) || (err.Error() != expectErr.Error()) {
				t.Fatalf("Case %d: wrong err, got:%v, expect:%v.\n", i, err, expectErr)
			}
		}
		expectBytes := cases[i].data[:written]
		if !bytes.Equal(wf.sink.Bytes(), expectBytes) {
			t.Fatalf("Case %d: written bytes not match.\n", i)
		}
		fmt.Printf("Case %d: written=%d, err=%v, expectWritten=%d, expectErr=%v.\n", i, written, err, expectWritten, expectErr)
	}
}
