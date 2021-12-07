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

package bfe_fcgi

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/url"
	"reflect"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_http"
)

type MockClientHandler func(*testing.T, *FCGIClient)

type MockServerHandler func(*testing.T, http.ResponseWriter, *http.Request)

type FastCGIServer struct {
	t *testing.T
	h MockServerHandler
}

func (s FastCGIServer) ServeHTTP(rsp http.ResponseWriter, req *http.Request) {
	if s.h != nil {
		s.h(s.t, rsp, req)
	}
}

func testFastCGIServer(t *testing.T, clientHandler MockClientHandler,
	serverHandler MockServerHandler) {

	// create and start fastcgi server
	server := new(FastCGIServer)
	server.t = t
	server.h = serverHandler

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("create ln error: %s", err)
	}
	defer ln.Close()

	go func() {
		fcgi.Serve(ln, server)
	}()

	// create fastcgi client
	client, err := Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("connect fastcgi server error: %s", err)
	}
	defer client.Close()

	// perform test
	clientHandler(t, client)
}

func prepareParams(method string) map[string]string {
	fcgiParams := make(map[string]string)
	fcgiParams["REQUEST_METHOD"] = method
	fcgiParams["SERVER_PROTOCOL"] = "HTTP/1.1"
	fcgiParams["SCRIPT_FILENAME"] = "fcgic_test.php"
	return fcgiParams
}

func testFastCGISimpleMethod(t *testing.T, method string) {
	testFastCGIServer(t, func(t *testing.T, c *FCGIClient) {
		var rsp *bfe_http.Response
		var err error
		params := prepareParams(method)
		switch method {
		case "GET":
			rsp, err = c.Get(params)
		case "HEAD":
			rsp, err = c.Head(params)
		case "OPTIONS":
			rsp, err = c.Options(params)
		}
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		if rsp.StatusCode != 200 {
			t.Fatalf("Unexpected status: %d", rsp.StatusCode)
		}
	}, func(t *testing.T, rw http.ResponseWriter, req *http.Request) {
		if req.Method != method {
			t.Errorf("Unexpected method, want %s, got %s", method, req.Method)
		}
		if req.ContentLength != 0 {
			t.Errorf("Unexpected length %d", req.ContentLength)
		}
	})
}

func generateRandomData(n int) []byte {
	buf := make([]byte, n)
	rand.Read(buf)
	return buf
}

func TestFastCGIGet(t *testing.T) {
	testFastCGISimpleMethod(t, "GET")
}

func TestFastCGIHead(t *testing.T) {
	testFastCGISimpleMethod(t, "HEAD")
}

func TestFastCGIOptions(t *testing.T) {
	testFastCGISimpleMethod(t, "OPTIONS")
}

func TestFastCGIPost(t *testing.T) {
	dlen := 100 * 1024
	data := generateRandomData(dlen)
	testFastCGIServer(t, func(t *testing.T, c *FCGIClient) {
		params := prepareParams("POST")
		rd := bytes.NewReader(data)
		_, err := c.Post(params, "", rd, rd.Len())
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
	}, func(t *testing.T, rw http.ResponseWriter, req *http.Request) {
		buf := make([]byte, dlen)
		io.ReadFull(req.Body, buf)
		if !reflect.DeepEqual(data, buf) {
			t.Fatalf("Unexpected request data: %v:%v", data, buf)
		}
	})
}

func TestFastCGIPostForm(t *testing.T) {
	testFastCGIServer(t, func(t *testing.T, c *FCGIClient) {
		values := url.Values{}
		values.Set("X-Protocol", "https")
		params := prepareParams("POST")
		_, err := c.PostForm(params, values)
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
	}, func(t *testing.T, rw http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		value := req.FormValue("X-Protocol")
		if value != "https" {
			t.Fatalf("Unexpected form value: %s", value)
		}
	})
}

func TestFastCGIRoundTrip(t *testing.T) {
	// create and start fastcgi server
	server := new(FastCGIServer)
	server.t = t
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("create ln error: %s", err)
	}
	defer ln.Close()

	go func() {
		fcgi.Serve(ln, server)
	}()

	// prepare request
	req := new(bfe_http.Request)
	req.URL, _ = url.Parse(fmt.Sprintf("http://%s/test", ln.Addr().String()))

	// RoundTrip call
	var trans Transport
	rsp, err := trans.RoundTrip(req)
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if rsp.StatusCode != 200 {
		t.Fatalf("Unexpected status: %d", rsp.StatusCode)
	}
}
