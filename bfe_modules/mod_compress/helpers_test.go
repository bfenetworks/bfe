// Copyright (c) 2026 The BFE Authors.
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

package mod_compress

import (
	"io"
)

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

// errReadCloser returns data first and then returns the configured error.
type errReadCloser struct {
	data []byte
	err  error
}

func (e *errReadCloser) Read(p []byte) (int, error) {
	if len(e.data) > 0 {
		n := copy(p, e.data)
		e.data = e.data[n:]
		return n, nil
	}
	return 0, e.err
}

func (e *errReadCloser) Close() error {
	return nil
}

// closeErrReadCloser returns EOF on Read and the configured error on Close.
type closeErrReadCloser struct {
	err error
}

func (c *closeErrReadCloser) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func (c *closeErrReadCloser) Close() error {
	return c.err
}
