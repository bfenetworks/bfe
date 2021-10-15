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

package bfe_http

import (
	"bytes"
	"io/ioutil"
	"testing"
)

type respWriteTest struct {
	Resp Response
	Raw  string
}

func TestResponseWrite(t *testing.T) {
	respWriteTests := []respWriteTest{
		// HTTP/1.0, identity coding; no trailer
		{
			Response{
				StatusCode:    503,
				ProtoMajor:    1,
				ProtoMinor:    0,
				Request:       dummyReq(MethodGet),
				Header:        Header{},
				Body:          ioutil.NopCloser(bytes.NewBufferString("abcdef")),
				ContentLength: 6,
			},

			"HTTP/1.0 503 Service Unavailable\r\n" +
				"Content-Length: 6\r\n\r\n" +
				"abcdef",
		},
		// Unchunked response without Content-Length.
		{
			Response{
				StatusCode:    200,
				ProtoMajor:    1,
				ProtoMinor:    0,
				Request:       dummyReq(MethodGet),
				Header:        Header{},
				Body:          ioutil.NopCloser(bytes.NewBufferString("abcdef")),
				ContentLength: -1,
			},
			"HTTP/1.0 200 OK\r\n" +
				"\r\n" +
				"abcdef",
		},
		// HTTP/1.1, chunked coding; empty trailer; close
		{
			Response{
				StatusCode:       200,
				ProtoMajor:       1,
				ProtoMinor:       1,
				Request:          dummyReq(MethodGet),
				Header:           Header{},
				Body:             ioutil.NopCloser(bytes.NewBufferString("abcdef")),
				ContentLength:    6,
				TransferEncoding: []string{"chunked"},
				Close:            true,
			},

			"HTTP/1.1 200 OK\r\n" +
				"Connection: close\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"6\r\nabcdef\r\n0\r\n\r\n",
		},

		// Header value with a newline character (Issue 914).
		// Also tests removal of leading and trailing whitespace.
		{
			Response{
				StatusCode: 204,
				ProtoMajor: 1,
				ProtoMinor: 1,
				Request:    dummyReq(MethodGet),
				Header: Header{
					"Foo": []string{" Bar\nBaz "},
				},
				Body:             nil,
				ContentLength:    0,
				TransferEncoding: []string{"chunked"},
				Close:            true,
			},

			"HTTP/1.1 204 No Content\r\n" +
				"Connection: close\r\n" +
				"Foo: Bar Baz\r\n" +
				"\r\n",
		},
	}

	for i := range respWriteTests {
		tt := &respWriteTests[i]
		var braw bytes.Buffer
		err := tt.Resp.Write(&braw)
		if err != nil {
			t.Errorf("error writing #%d: %s", i, err)
			continue
		}
		sraw := braw.String()
		if sraw != tt.Raw {
			t.Errorf("Test %d, expecting:\n%q\nGot:\n%q\n", i, tt.Raw, sraw)
			continue
		}
	}
}
