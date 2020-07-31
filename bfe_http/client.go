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

// HTTP client. See RFC 2616.
//
// This is the high-level Client interface.
// The low-level implementation is in transport.go.

package bfe_http

import (
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

import (
	"github.com/baidu/go-lib/log"
)

// RoundTripper is an interface representing the ability to execute a
// single HTTP transaction, obtaining the Response for a given Request.
//
// A RoundTripper must be safe for concurrent use by multiple
// goroutines.
type RoundTripper interface {
	// RoundTrip executes a single HTTP transaction, returning
	// the Response for the request req.  RoundTrip should not
	// attempt to interpret the response.  In particular,
	// RoundTrip must return err == nil if it obtained a response,
	// regardless of the response's HTTP status code.  A non-nil
	// err should be reserved for failure to obtain a response.
	// Similarly, RoundTrip should not attempt to handle
	// higher-level protocol details such as redirects,
	// authentication, or cookies.
	//
	// RoundTrip should not modify the request, except for
	// consuming and closing the Body. The request's URL and
	// Header fields are guaranteed to be initialized.
	RoundTrip(*Request) (*Response, error)
}

// Given a string of the form "host", "host:port", or "[ipv6::address]:port",
// return true if the string includes a port.
func hasPort(s string) bool { return strings.LastIndex(s, ":") > strings.LastIndex(s, "]") }

// Used in Send to implement io.ReadCloser by bundling together the
// bufio.Reader through which we read the response, and the underlying
// network connection.
type readClose struct {
	io.Reader
	io.Closer
}

// send issues an HTTP request.
// Caller should close resp.Body when done reading from it.
func send(req *Request, t RoundTripper) (resp *Response, err error) {
	if t == nil {
		t = DefaultTransport
		if t == nil {
			err = errors.New("http: no Client.Transport or DefaultTransport")
			return
		}
	}

	if req.URL == nil {
		return nil, errors.New("http: nil Request.URL")
	}

	if req.RequestURI != "" {
		return nil, errors.New("http: Request.RequestURI can't be set in client requests.")
	}

	// Most the callers of send (Get, Post, et al) don't need
	// Headers, leaving it uninitialized.  We guarantee to the
	// Transport that this has been initialized, though.
	if req.Header == nil {
		req.Header = make(Header)
	}

	if u := req.URL.User; u != nil {
		username := u.Username()
		password, _ := u.Password()
		req.Header.Set("Authorization", "Basic "+basicAuth(username, password))
	}
	resp, err = t.RoundTrip(req)
	if err != nil {
		if resp != nil {
			log.Logger.Warn("RoundTripper returned a response & error; ignoring response")
		}
		return nil, err
	}
	return resp, nil
}

// See 2 (end of page 4) http://www.ietf.org/rfc/rfc2617.txt
// "To receive authorization, the client sends the userid and password,
// separated by a single colon (":") character, within a base64
// encoded string in the credentials."
// It is not meant to be urlencoded.
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

// True if the specified HTTP status code is one for which the Get utility should
// automatically redirect.
func shouldRedirectGet(statusCode int) bool {
	switch statusCode {
	case StatusMovedPermanently, StatusFound, StatusSeeOther, StatusTemporaryRedirect:
		return true
	}
	return false
}

// True if the specified HTTP status code is one for which the Post utility should
// automatically redirect.
func shouldRedirectPost(statusCode int) bool {
	switch statusCode {
	case StatusFound, StatusSeeOther:
		return true
	}
	return false
}

func defaultCheckRedirect(req *Request, via []*Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	return nil
}
