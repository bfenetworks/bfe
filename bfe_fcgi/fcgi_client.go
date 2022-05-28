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

// Copyright 2012 Junqing Tan <ivan@mysqlab.net> and The Go Authors
// Use of client source code is governed by a BSD-style
// Part of source code is from Go fcgi package

package bfe_fcgi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

import (
	bufio "github.com/bfenetworks/bfe/bfe_bufio"
	http "github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_net/textproto"
)

// FCGIListenSockFileNo describes listen socket file number.
const FCGIListenSockFileNo uint8 = 0

// FCGIHeaderLen describes header length.
const FCGIHeaderLen uint8 = 8

// Version1 describes the version.
const Version1 uint8 = 1

// FCGINullRequestID describes the null request ID.
const FCGINullRequestID uint8 = 0

// FCGIKeepConn describes keep connection mode.
const FCGIKeepConn uint8 = 1

const doubleCRLF = "\r\n\r\n"

const (
	// FCGIBeginRequest is the begin request flag.
	FCGIBeginRequest uint8 = iota + 1

	// FCGIAbortRequest is the abort request flag.
	FCGIAbortRequest

	// FCGIEndRequest is the end request flag.
	FCGIEndRequest

	// FCGIParams is the parameters flag.
	FCGIParams

	// FCGIStdin is the standard input flag.
	FCGIStdin

	// FCGIStdout is the standard output flag.
	FCGIStdout

	// FCGIStderr is the standard error flag.
	FCGIStderr

	// FCGIData is the data flag.
	FCGIData

	// FCGIGetValues is the get values flag.
	FCGIGetValues

	// FCGIGetValuesResult is the get values result flag.
	FCGIGetValuesResult

	// FCGIUnknownType is the unknown type flag.
	FCGIUnknownType

	// FCGIMaxType is the maximum type flag.
	FCGIMaxType = FCGIUnknownType
)

const (
	// FCGIResponder is the responder flag.
	FCGIResponder uint8 = iota + 1

	// FCGIAuthorizer is the authorizer flag.
	FCGIAuthorizer

	// FCGIFilter is the filter flag.
	FCGIFilter
)

const (
	// FCGIRequestComplete is the completed request flag.
	FCGIRequestComplete uint8 = iota

	// FCGICantMpxConn is the multiplexed connections flag.
	FCGICantMpxConn

	// FCGIOverLoaded is the overloaded flag.
	FCGIOverLoaded

	// FCGIUnknownRole is the unknown role flag.
	FCGIUnknownRole
)

const (
	// FCGIMaxConns is the maximum connections flag.
	FCGIMaxConns string = "MAX_CONNS"

	// FCGIMaxReqs is the maximum requests flag.
	FCGIMaxReqs string = "MAX_REQS"

	// FCGIMpxsConns is the multiplex connections flag.
	FCGIMpxsConns string = "MPXS_CONNS"
)

const (
	maxWrite = 65500 // 65530 may work, but for compatibility
	maxPad   = 255
)

type header struct {
	Version       uint8
	Type          uint8
	Id            uint16
	ContentLength uint16
	PaddingLength uint8
	Reserved      uint8
}

// for padding so we don't have to allocate all the time
// not synchronized because we don't care what the contents are
var pad [maxPad]byte

func (h *header) init(recType uint8, reqId uint16, contentLength int) {
	h.Version = 1
	h.Type = recType
	h.Id = reqId
	h.ContentLength = uint16(contentLength)
	h.PaddingLength = uint8(-contentLength & 7)
}

type record struct {
	h    header
	rbuf []byte
}

func (rec *record) read(r io.Reader) (buf []byte, err error) {
	if err = binary.Read(r, binary.BigEndian, &rec.h); err != nil {
		return
	}
	if rec.h.Version != 1 {
		err = errors.New("fcgi: invalid header version")
		return
	}
	if rec.h.Type == FCGIEndRequest {
		err = io.EOF
		return
	}
	n := int(rec.h.ContentLength) + int(rec.h.PaddingLength)
	if len(rec.rbuf) < n {
		rec.rbuf = make([]byte, n)
	}
	if _, err = io.ReadFull(r, rec.rbuf[:n]); err != nil {
		return
	}
	buf = rec.rbuf[:int(rec.h.ContentLength)]

	return
}

// FCGIClient implements a FastCGI client, which is a standard for
// interfacing external applications with Web servers.
type FCGIClient struct {
	mutex     sync.Mutex
	rwc       io.ReadWriteCloser
	h         header
	buf       bytes.Buffer
	keepAlive bool
	reqId     uint16
}

// Dial connects to the fcgi responder at the specified network address.
// See func net.Dial for a description of the network and address parameters.
func Dial(network, address string) (fcgi *FCGIClient, err error) {
	var conn net.Conn

	conn, err = net.Dial(network, address)
	if err != nil {
		return
	}

	fcgi = &FCGIClient{
		rwc:       conn,
		keepAlive: false,
		reqId:     1,
	}

	return
}

// Close closes fcgi connection
func (client *FCGIClient) Close() {
	client.rwc.Close()
}

func (client *FCGIClient) writeRecord(recType uint8, content []byte) (err error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.buf.Reset()
	client.h.init(recType, client.reqId, len(content))
	if err := binary.Write(&client.buf, binary.BigEndian, client.h); err != nil {
		return err
	}
	if _, err := client.buf.Write(content); err != nil {
		return err
	}
	if _, err := client.buf.Write(pad[:client.h.PaddingLength]); err != nil {
		return err
	}
	_, err = client.rwc.Write(client.buf.Bytes())
	return err
}

func (client *FCGIClient) writeBeginRequest(role uint16, flags uint8) error {
	b := [8]byte{byte(role >> 8), byte(role), flags}
	return client.writeRecord(FCGIBeginRequest, b[:])
}

func (client *FCGIClient) writeEndRequest(appStatus int, protocolStatus uint8) error {
	b := make([]byte, 8)
	binary.BigEndian.PutUint32(b, uint32(appStatus))
	b[4] = protocolStatus
	return client.writeRecord(FCGIEndRequest, b)
}

func (client *FCGIClient) writePairs(recType uint8, pairs map[string]string) error {
	w := newWriter(client, recType)
	b := make([]byte, 8)
	nn := 0
	for k, v := range pairs {
		m := 8 + len(k) + len(v)
		if m > maxWrite {
			// param data size exceed 65535 bytes"
			vl := maxWrite - 8 - len(k)
			v = v[:vl]
		}
		n := encodeSize(b, uint32(len(k)))
		n += encodeSize(b[n:], uint32(len(v)))
		m = n + len(k) + len(v)
		if (nn + m) > maxWrite {
			w.Flush()
			nn = 0
		}
		nn += m
		if _, err := w.Write(b[:n]); err != nil {
			return err
		}
		if _, err := w.WriteString(k); err != nil {
			return err
		}
		if _, err := w.WriteString(v); err != nil {
			return err
		}
	}
	w.Close()
	return nil
}

func readSize(s []byte) (uint32, int) {
	if len(s) == 0 {
		return 0, 0
	}
	size, n := uint32(s[0]), 1
	if size&(1<<7) != 0 {
		if len(s) < 4 {
			return 0, 0
		}
		n = 4
		size = binary.BigEndian.Uint32(s)
		size &^= 1 << 31
	}
	return size, n
}

func readString(s []byte, size uint32) string {
	if size > uint32(len(s)) {
		return ""
	}
	return string(s[:size])
}

func encodeSize(b []byte, size uint32) int {
	if size > 127 {
		size |= 1 << 31
		binary.BigEndian.PutUint32(b, size)
		return 4
	}
	b[0] = byte(size)
	return 1
}

// bufWriter encapsulates bufio.Writer but also closes the underlying stream when
// Closed.
type bufWriter struct {
	closer io.Closer
	*bufio.Writer
}

func (w *bufWriter) Close() error {
	if err := w.Writer.Flush(); err != nil {
		w.closer.Close()
		return err
	}
	return w.closer.Close()
}

func newWriter(c *FCGIClient, recType uint8) *bufWriter {
	s := &streamWriter{c: c, recType: recType}
	w := bufio.NewWriterSize(s, maxWrite)
	return &bufWriter{s, w}
}

// streamWriter abstracts out the separation of a stream into discrete records.
// It only writes maxWrite bytes at a time.
type streamWriter struct {
	c       *FCGIClient
	recType uint8
}

func (w *streamWriter) Write(p []byte) (int, error) {
	nn := 0
	for len(p) > 0 {
		n := len(p)
		if n > maxWrite {
			n = maxWrite
		}
		if err := w.c.writeRecord(w.recType, p[:n]); err != nil {
			return nn, err
		}
		nn += n
		p = p[n:]
	}
	return nn, nil
}

func (w *streamWriter) Close() error {
	// send empty record to close the stream
	return w.c.writeRecord(w.recType, nil)
}

type streamReader struct {
	c   *FCGIClient
	buf []byte
}

func (w *streamReader) Read(p []byte) (n int, err error) {
	if len(p) > 0 {
		if len(w.buf) == 0 {
			rec := &record{}
			w.buf, err = rec.read(w.c.rwc)
			if err != nil {
				return
			}
		}

		n = len(p)
		if n > len(w.buf) {
			n = len(w.buf)
		}
		copy(p, w.buf[:n])
		w.buf = w.buf[n:]
	}

	return
}

// Do made the request and returns a io.Reader that translates the data read
// from fcgi responder out of fcgi packet before returning it.
func (client *FCGIClient) Do(p map[string]string, req io.Reader) (r io.Reader, err error) {
	err = client.writeBeginRequest(uint16(FCGIResponder), 0)
	if err != nil {
		return
	}

	err = client.writePairs(FCGIParams, p)
	if err != nil {
		return
	}

	body := newWriter(client, FCGIStdin)
	if req != nil {
		io.Copy(body, req)
	}
	body.Close()

	r = &streamReader{c: client}
	return
}

// Request returns a HTTP Response with Header and Body
// from fcgi responder
func (client *FCGIClient) Request(p map[string]string, req io.Reader) (resp *http.Response, err error) {
	r, err := client.Do(p, req)
	if err != nil {
		return
	}

	rb := bufio.NewReader(r)
	tp := textproto.NewReader(rb)
	resp = new(http.Response)

	// Parse the response headers.
	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil {
		return
	}
	resp.Header = http.Header(mimeHeader)

	// Parse the response status
	status := resp.Header.Get("Status")
	if status != "" {
		statusParts := strings.SplitN(status, " ", 2)
		resp.StatusCode, err = strconv.Atoi(statusParts[0])
		if err != nil {
			return
		}
		if len(statusParts) > 1 {
			resp.Status = statusParts[1]
		}
	} else {
		resp.StatusCode = http.StatusOK
	}

	// TODO: fixTransferEncoding ?
	resp.TransferEncoding = resp.Header["Transfer-Encoding"]
	resp.ContentLength, _ = strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)

	if chunked(resp.TransferEncoding) {
		resp.Body = ioutil.NopCloser(httputil.NewChunkedReader(rb))
	} else {
		resp.Body = ioutil.NopCloser(rb)
	}

	return
}

// Get issues a GET request to the fcgi responder.
func (client *FCGIClient) Get(p map[string]string) (resp *http.Response, err error) {
	p["REQUEST_METHOD"] = "GET"
	p["CONTENT_LENGTH"] = "0"

	return client.Request(p, nil)
}

// Head issues a HEAD request to the fcgi responder.
func (c *FCGIClient) Head(p map[string]string) (resp *http.Response, err error) {
	p["REQUEST_METHOD"] = "HEAD"
	p["CONTENT_LENGTH"] = "0"

	return c.Request(p, nil)
}

// Options issues an OPTIONS request to the fcgi responder.
func (c *FCGIClient) Options(p map[string]string) (resp *http.Response, err error) {
	p["REQUEST_METHOD"] = "OPTIONS"
	p["CONTENT_LENGTH"] = "0"

	return c.Request(p, nil)
}

// Post issues a Post request to the fcgi responder. with request body
// in the format that bodyType specified
func (client *FCGIClient) Post(p map[string]string, bodyType string, body io.Reader, l int) (resp *http.Response, err error) {
	if len(p["REQUEST_METHOD"]) == 0 || p["REQUEST_METHOD"] == "GET" {
		p["REQUEST_METHOD"] = "POST"
	}
	p["CONTENT_LENGTH"] = strconv.Itoa(l)
	if len(bodyType) > 0 {
		p["CONTENT_TYPE"] = bodyType
	} else {
		p["CONTENT_TYPE"] = "application/x-www-form-urlencoded"
	}

	return client.Request(p, body)
}

// PostForm issues a POST to the fcgi responder, with form
// as a string key to a list values (url.Values)
func (client *FCGIClient) PostForm(p map[string]string, data url.Values) (resp *http.Response, err error) {
	body := bytes.NewReader([]byte(data.Encode()))
	return client.Post(p, "application/x-www-form-urlencoded", body, body.Len())
}

// PostFile issues a POST to the fcgi responder in multipart(RFC 2046) standard,
// with form as a string key to a list values (url.Values),
// and/or with file as a string key to a list file path.
func (client *FCGIClient) PostFile(p map[string]string, data url.Values, file map[string]string) (resp *http.Response, err error) {
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	bodyType := writer.FormDataContentType()

	for key, val := range data {
		for _, v0 := range val {
			err = writer.WriteField(key, v0)
			if err != nil {
				return
			}
		}
	}

	for key, val := range file {
		fd, e := os.Open(val)
		if e != nil {
			return nil, e
		}
		defer fd.Close()

		part, e := writer.CreateFormFile(key, filepath.Base(val))
		if e != nil {
			return nil, e
		}
		_, err = io.Copy(part, fd)
	}

	err = writer.Close()
	if err != nil {
		return
	}

	return client.Post(p, bodyType, buf, buf.Len())
}

// SetReadTimeout sets the read timeout for future calls that read from the
// fcgi responder. A zero value for t means no timeout will be set.
func (c *FCGIClient) SetReadTimeout(t time.Duration) error {
	if conn, ok := c.rwc.(net.Conn); ok && t != 0 {
		return conn.SetReadDeadline(time.Now().Add(t))
	}
	return nil
}

// SetWriteTimeout sets the write timeout for future calls that send data to
// the fcgi responder. A zero value for t means no timeout will be set.
func (c *FCGIClient) SetWriteTimeout(t time.Duration) error {
	if conn, ok := c.rwc.(net.Conn); ok && t != 0 {
		return conn.SetWriteDeadline(time.Now().Add(t))
	}
	return nil
}

// Checks whether chunked is part of the encodings stack
func chunked(te []string) bool { return len(te) > 0 && te[0] == "chunked" }
