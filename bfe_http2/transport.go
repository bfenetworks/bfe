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

package bfe_http2

import (
	"github.com/bfenetworks/bfe/bfe_http"
	"golang.org/x/net/http2"
	"net/http"
)

// Transport is a Wrapper class for http2.Transport
// Why this needs?
// BFE customizes http.Request && http.Response as bfe_http.Request && bfe_http.Response,
// cannot use http2.Transport.RoundTrip directly
type Transport struct {
	T *http2.Transport
}

// RoundTrip is a wrapper function for http2.Transport.RoundTrip
func (t *Transport) RoundTrip(r *bfe_http.Request) (*bfe_http.Response, error) {
	req := http.Request{
		Method:           r.Method,
		URL:              r.URL,
		Proto:            r.Proto,
		ProtoMajor:       r.ProtoMajor,
		ProtoMinor:       r.ProtoMinor,
		Header:           http.Header{},
		Body:             r.Body,
		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,
		Close:            r.Close,
		Host:             r.Host,
		Form:             r.Form,
		PostForm:         r.PostForm,
		MultipartForm:    r.MultipartForm,
		Trailer:          http.Header{},
		RemoteAddr:       r.RemoteAddr,
		RequestURI:       r.RequestURI,
	}
	for k, v := range r.Header {
		req.Header[k] = v
	}
	for k, v := range r.Trailer {
		req.Trailer[k] = v
	}
	res, err := t.T.RoundTrip(&req)
	if err != nil {
		return nil, err
	}
	resp := bfe_http.Response{
		Status:           res.Status,
		StatusCode:       res.StatusCode,
		Proto:            res.Proto,
		ProtoMajor:       res.ProtoMajor,
		ProtoMinor:       res.ProtoMinor,
		Header:           bfe_http.Header{},
		Body:             res.Body,
		ContentLength:    res.ContentLength,
		TransferEncoding: res.TransferEncoding,
		Close:            res.Close,
		Request:          r,
		// trailer header is set after body closed, use pointer to acquire trailer header outside
		H2Trailer: &res.Trailer,
	}
	for k, v := range res.Header {
		resp.Header[k] = v
	}
	return &resp, nil
}
