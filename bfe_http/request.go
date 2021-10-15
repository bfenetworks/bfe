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

// HTTP Request reading and parsing.

package bfe_http

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_bufio"
	"github.com/bfenetworks/bfe/bfe_net/textproto"
	"github.com/bfenetworks/bfe/bfe_tls"
)

const (
	maxValueLength   = 4096
	maxHeaderLines   = 1024
	chunkSize        = 4 << 10  // 4 KB chunks
	defaultMaxMemory = 32 << 20 // 32 MB
	MaxUriSize       = 1024 * 64
)

// ErrMissingFile is returned by FormFile when the provided file field name
// is either not present in the request or not a file field.
var ErrMissingFile = errors.New("http: no such file")

// HTTP request parsing errors.
type ProtocolError struct {
	ErrorString string
}

func (err *ProtocolError) Error() string { return err.ErrorString }

var (
	ErrHeaderTooLong        = &ProtocolError{"header too long"}
	ErrShortBody            = &ProtocolError{"entity body too short"}
	ErrNotSupported         = &ProtocolError{"feature not supported"}
	ErrUnexpectedTrailer    = &ProtocolError{"trailer header without chunked transfer encoding"}
	ErrMissingContentLength = &ProtocolError{"missing ContentLength in HEAD response"}
	ErrNotMultipart         = &ProtocolError{"request Content-Type isn't multipart/form-data"}
	ErrMissingBoundary      = &ProtocolError{"no multipart boundary param in Content-Type"}
)

type badStringError struct {
	what string
	str  string
}

func (e *badStringError) Error() string { return fmt.Sprintf("%s %q", e.what, e.str) }

// Headers that Request.Write handles itself and should be skipped.
var reqWriteExcludeHeader = map[string]bool{
	"Host":              true, // not in Header map anyway
	"Content-Length":    true,
	"Transfer-Encoding": true,
	"Trailer":           true,
}

// A Request represents an HTTP request received by a server
// or to be sent by a client.
type Request struct {
	Method string // GET, POST, PUT, etc.

	// URL is created from the URI supplied on the Request-Line
	// as stored in RequestURI.
	//
	// For most requests, fields other than Path and RawQuery
	// will be empty. (See RFC 2616, Section 5.1.2)
	URL *url.URL

	// The protocol version for incoming requests.
	// Outgoing requests always use HTTP/1.1.
	Proto      string // "HTTP/1.0"
	ProtoMajor int    // 1
	ProtoMinor int    // 0

	// A header maps request lines to their values.
	// If the header says
	//
	//	accept-encoding: gzip, deflate
	//	Accept-Language: en-us
	//	Connection: keep-alive
	//
	// then
	//
	//	Header = map[string][]string{
	//		"Accept-Encoding": {"gzip, deflate"},
	//		"Accept-Language": {"en-us"},
	//		"Connection": {"keep-alive"},
	//	}
	//
	// HTTP defines that header names are case-insensitive.
	// The request parser implements this by canonicalizing the
	// name, making the first character and any characters
	// following a hyphen uppercase and the rest lowercase.
	Header Header

	// a headerKeys represents keys of header in original order
	HeaderKeys textproto.MIMEKeys

	// Body is the request's body.
	//
	// For client requests, a nil body means the request has no
	// body, such as a GET request. The HTTP Client's Transport
	// is responsible for calling the Close method.
	//
	// For server requests, the Request Body is always non-nil
	// but will return EOF immediately when no body is present.
	// The Server will close the request body. The ServeHTTP
	// Handler does not need to.
	Body io.ReadCloser

	// ContentLength records the length of the associated content.
	// The value -1 indicates that the length is unknown.
	// Values >= 0 indicate that the given number of bytes may
	// be read from Body.
	// For outgoing requests, a value of 0 means unknown if Body is not nil.
	ContentLength int64

	// TransferEncoding lists the transfer encodings from outermost to
	// innermost. An empty list denotes the "identity" encoding.
	// TransferEncoding can usually be ignored; chunked encoding is
	// automatically added and removed as necessary when sending and
	// receiving requests.
	TransferEncoding []string

	// Close indicates whether to close the connection after
	// replying to this request.
	Close bool

	// The host on which the URL is sought.
	// Per RFC 2616, this is either the value of the Host: header
	// or the host name given in the URL itself.
	// It may be of the form "host:port".
	Host string

	// Form contains the parsed form data, including both the URL
	// field's query parameters and the POST or PUT form data.
	// This field is only available after ParseForm is called.
	// The HTTP client ignores Form and uses Body instead.
	Form url.Values

	// PostForm contains the parsed form data from POST or PUT
	// body parameters.
	// This field is only available after ParseForm is called.
	// The HTTP client ignores PostForm and uses Body instead.
	PostForm url.Values

	// MultipartForm is the parsed multipart form, including file uploads.
	// This field is only available after ParseMultipartForm is called.
	// The HTTP client ignores MultipartForm and uses Body instead.
	MultipartForm *multipart.Form

	// Trailer maps trailer keys to values.  Like for Header, if the
	// response has multiple trailer lines with the same key, they will be
	// concatenated, delimited by commas.
	// For server requests, Trailer is only populated after Body has been
	// closed or fully consumed.
	// Trailer support is only partially complete.
	Trailer Header

	// RemoteAddr allows HTTP servers and other software to record
	// the network address that sent the request, usually for
	// logging. This field is not filled in by ReadRequest and
	// has no defined format. The HTTP server in this package
	// sets RemoteAddr to an "IP:port" address before invoking a
	// handler.
	// This field is ignored by the HTTP client.
	RemoteAddr string

	// RequestURI is the unmodified Request-URI of the
	// Request-Line (RFC 2616, Section 5.1) as sent by the client
	// to a server. Usually the URL field should be used instead.
	// It is an error to set this field in an HTTP client request.
	RequestURI string

	// TLS allows HTTP servers and other software to record
	// information about the TLS connection on which the request
	// was received. This field is not filled in by ReadRequest.
	// The HTTP server in this package sets the field for
	// TLS-enabled connections before invoking a handler;
	// otherwise it leaves the field nil.
	// This field is ignored by the HTTP client.
	TLS *bfe_tls.ConnectionState

	// State allows HTTP server and other software to record
	// information about the request. This filed may be not filled.
	State *RequestState
}

type RequestState struct {
	// SerialNumber identify Nth request on one connection
	// this field of first request is 1, second is 2, and so on
	SerialNumber uint32

	// Conn is the connection from which request arrived
	Conn net.Conn

	// StartTime is the time when the first byte of request header was
	// received. StartTime may be approximate.
	StartTime time.Time

	// ConnectBackendStart is the time when start get a connection
	ConnectBackendStart time.Time

	// ConnectBackendEnd is the time when got a connection(may got from connection pool)
	ConnectBackendEnd time.Time

	// HeaderSize is the size of the request header.
	HeaderSize uint32

	// BodySize is the size of request body.
	BodySize uint32
}

// ProtoAtLeast reports whether the HTTP protocol used
// in the request is at least major.minor.
func (r *Request) ProtoAtLeast(major, minor int) bool {
	return r.ProtoMajor > major ||
		r.ProtoMajor == major && r.ProtoMinor >= minor
}

// UserAgent returns the client's User-Agent, if sent in the request.
func (r *Request) UserAgent() string {
	return r.Header.Get("User-Agent")
}

// Cookies parses and returns the HTTP cookies sent with the request.
func (r *Request) Cookies() []*Cookie {
	return readCookies(r.Header, "")
}

var ErrNoCookie = errors.New("http: named cookie not present")

// Cookie returns the named cookie provided in the request or
// ErrNoCookie if not found.
func (r *Request) Cookie(name string) (*Cookie, error) {
	for _, c := range readCookies(r.Header, name) {
		return c, nil
	}
	return nil, ErrNoCookie
}

// AddCookie adds a cookie to the request.  Per RFC 6265 section 5.4,
// AddCookie does not attach more than one Cookie header field.  That
// means all cookies, if any, are written into the same line,
// separated by semicolon.
func (r *Request) AddCookie(c *Cookie) {
	s := fmt.Sprintf("%s=%s", sanitizeCookieName(c.Name), sanitizeCookieValue(c.Value))
	if c := r.Header.Get("Cookie"); c != "" {
		r.Header.Set("Cookie", c+"; "+s)
	} else {
		r.Header.Set("Cookie", s)
	}
}

// Referer returns the referring URL, if sent in the request.
//
// Referer is misspelled as in the request itself, a mistake from the
// earliest days of HTTP.  This value can also be fetched from the
// Header map as Header["Referer"]; the benefit of making it available
// as a method is that the compiler can diagnose programs that use the
// alternate (correct English) spelling req.Referrer() but cannot
// diagnose programs that use Header["Referrer"].
func (r *Request) Referer() string {
	return r.Header.Get("Referer")
}

// multipartByReader is a sentinel value.
// Its presence in Request.MultipartForm indicates that parsing of the request
// body has been handed off to a MultipartReader instead of ParseMultipartFrom.
var multipartByReader = &multipart.Form{
	Value: make(map[string][]string),
	File:  make(map[string][]*multipart.FileHeader),
}

// MultipartReader returns a MIME multipart reader if this is a
// multipart/form-data POST request, else returns nil and an error.
// Use this function instead of ParseMultipartForm to
// process the request body as a stream.
func (r *Request) MultipartReader() (*multipart.Reader, error) {
	if r.MultipartForm == multipartByReader {
		return nil, errors.New("http: MultipartReader called twice")
	}
	if r.MultipartForm != nil {
		return nil, errors.New("http: multipart handled by ParseMultipartForm")
	}
	r.MultipartForm = multipartByReader
	return r.multipartReader()
}

func (r *Request) multipartReader() (*multipart.Reader, error) {
	v := r.Header.Get("Content-Type")
	if v == "" {
		return nil, ErrNotMultipart
	}
	d, params, err := mime.ParseMediaType(v)
	if err != nil || d != "multipart/form-data" {
		return nil, ErrNotMultipart
	}
	boundary, ok := params["boundary"]
	if !ok {
		return nil, ErrMissingBoundary
	}
	return multipart.NewReader(r.Body, boundary), nil
}

// Return value if nonempty, def otherwise.
func valueOrDefault(value, def string) string {
	if value != "" {
		return value
	}
	return def
}

// Write writes an HTTP/1.1 request -- header and body -- in wire format.
// This method consults the following fields of the request:
//	Host
//	URL
//	Method (defaults to "GET")
//	Header
//	ContentLength
//	TransferEncoding
//	Body
//
// If Body is present, Content-Length is <= 0 and TransferEncoding
// hasn't been set to "identity", Write adds "Transfer-Encoding:
// chunked" to the header. Body is closed after it is sent.
func (r *Request) Write(w io.Writer) error {
	return r.write(w, false, nil)
}

// WriteProxy is like Write but writes the request in the form
// expected by an HTTP proxy.  In particular, WriteProxy writes the
// initial Request-URI line of the request with an absolute URI, per
// section 5.1.2 of RFC 2616, including the scheme and host.
// In either case, WriteProxy also writes a Host header, using
// either r.Host or r.URL.Host.
func (r *Request) WriteProxy(w io.Writer) error {
	return r.write(w, true, nil)
}

// extraHeaders may be nil
func (req *Request) write(w io.Writer, usingProxy bool, extraHeaders Header) error {
	host := req.Host
	if host == "" {
		if req.URL == nil {
			return errors.New("http: Request.Write on Request with no Host or URL set")
		}
		host = req.URL.Host
	}

	ruri := req.URL.RequestURI()
	if usingProxy && req.URL.Scheme != "" && req.URL.Opaque == "" {
		ruri = req.URL.Scheme + "://" + host + ruri
	} else if req.Method == MethodConnect && req.URL.Path == "" {
		// CONNECT requests normally give just the host and port, not a full URL.
		ruri = host
	} else {
		// use req.RequestUri instead of req.URL.RequestURI() (decoded/encoded)
		// to be compatible with non-standard web server ONLY WHEN URL not changed since
		// ReadRequest()
		rawurl, err := url.ParseRequestURI(req.RequestURI)
		if err == nil && rawurl.RequestURI() == ruri {
			if rawurl.Scheme == "" && rawurl.Host == "" && rawurl.Opaque == "" {
				// if RequestUri contains Scheme Host Opaque, no replace
				ruri = req.RequestURI
			}
		}
	}
	// TODO(bradfitz): escape at least newlines in ruri?

	// Wrap the writer in a bufio Writer if it's not already buffered.
	// Don't always call NewWriter, as that forces a bytes.Buffer
	// and other small bufio Writers to have a minimum 4k buffer
	// size.
	var bw *bfe_bufio.Writer

	switch w.(type) {
	case io.ByteWriter:
		break
	case *MaxLatencyWriter:
		break
	default:
		bw = bfe_bufio.NewWriter(w)
		w = bw
	}

	fmt.Fprintf(w, "%s %s HTTP/1.1\r\n", valueOrDefault(req.Method, "GET"), ruri)

	// Header lines
	fmt.Fprintf(w, "Host: %s\r\n", host)

	// Process Body,ContentLength,Close,Trailer
	tw, err := newTransferWriter(req)
	if err != nil {
		return err
	}
	err = tw.WriteHeader(w)
	if err != nil {
		return err
	}

	// TODO: split long values?  (If so, should share code with Conn.Write)
	err = req.Header.WriteSubset(w, reqWriteExcludeHeader)
	if err != nil {
		return err
	}

	if extraHeaders != nil {
		err = extraHeaders.Write(w)
		if err != nil {
			return err
		}
	}

	io.WriteString(w, "\r\n")

	// flush req header immediately if w if a buffered writer.
	// in case that the backend read timeout setting is too short,
	// and body receiving from client is too slow, so until backend timeout the buffer is not fulfilled
	// and the backend may close the connection
	if rbw, ok := w.(Flusher); ok {
		err = rbw.Flush()
		if err != nil {
			return err
		}
	}

	// Write body and trailer
	n, err := tw.WriteBody(w)
	if err != nil {
		return err
	}
	req.State.BodySize = uint32(n)

	if bw != nil {
		return bw.Flush()
	}
	return nil
}

// ParseHTTPVersion parses a HTTP version string.
// "HTTP/1.0" returns (1, 0, true).
func ParseHTTPVersion(vers string) (major, minor int, ok bool) {
	const Big = 1000000 // arbitrary upper bound
	switch vers {
	case "HTTP/1.1":
		return 1, 1, true
	case "HTTP/1.0":
		return 1, 0, true
	}
	if !strings.HasPrefix(vers, "HTTP/") {
		return 0, 0, false
	}
	dot := strings.Index(vers, ".")
	if dot < 0 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(vers[5:dot])
	if err != nil || major < 0 || major > Big {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(vers[dot+1:])
	if err != nil || minor < 0 || minor > Big {
		return 0, 0, false
	}
	return major, minor, true
}

// NewRequest returns a new Request given a method, URL, and optional body.
//
// If the provided body is also an io.Closer, the returned
// Request.Body is set to body and will be closed by the Client
// methods Do, Post, and PostForm, and Transport.RoundTrip.
func NewRequest(method, urlStr string, body io.Reader) (*Request, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	rc, ok := body.(io.ReadCloser)
	if !ok && body != nil {
		rc = ioutil.NopCloser(body)
	}
	req := &Request{
		Method:     method,
		URL:        u,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(Header),
		Body:       rc,
		Host:       u.Host,
	}
	if body != nil {
		switch v := body.(type) {
		case *bytes.Buffer:
			req.ContentLength = int64(v.Len())
		case *bytes.Reader:
			req.ContentLength = int64(v.Len())
		case *strings.Reader:
			req.ContentLength = int64(v.Len())
		}
	}

	return req, nil
}

// BasicAuth returns the username and password provided in the request's
// Authorization header, if the request uses HTTP Basic Authentication.
// See RFC 2617, Section 2.
func (r *Request) BasicAuth() (username, password string, ok bool) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return
	}
	return parseBasicAuth(auth)
}

// parseBasicAuth parses an HTTP Basic Authentication string.
// "Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ==" returns ("Aladdin", "open sesame", true).
func parseBasicAuth(auth string) (username, password string, ok bool) {
	const prefix = "Basic "
	// Case insensitive prefix match. See Issue 22736.
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return
	}
	c, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return
	}
	cs := string(c)
	s := strings.IndexByte(cs, ':')
	if s < 0 {
		return
	}
	return cs[:s], cs[s+1:], true
}

// SetBasicAuth sets the request's Authorization header to use HTTP
// Basic Authentication with the provided username and password.
//
// With HTTP Basic Authentication the provided username and password
// are not encrypted.
func (r *Request) SetBasicAuth(username, password string) {
	r.Header.Set("Authorization", "Basic "+basicAuth(username, password))
}

// parseRequestLine parses "GET /foo HTTP/1.1" into its three parts.
func parseRequestLine(line string) (method, requestURI, proto string, ok bool) {
	s1 := strings.Index(line, " ")
	s2 := strings.Index(line[s1+1:], " ")
	if s1 < 0 || s2 < 0 {
		return
	}
	s2 += s1 + 1
	return line[:s1], line[s1+1 : s2], line[s2+1:], true
}

var textprotoReaderCache sync.Pool

func newTextprotoReader(br *bfe_bufio.Reader) *textproto.Reader {
	r := textprotoReaderCache.Get()
	if r != nil {
		tr := r.(*textproto.Reader)
		tr.R = br
		return tr
	}

	return textproto.NewReader(br)
}

func putTextprotoReader(r *textproto.Reader) {
	r.R = nil
	textprotoReaderCache.Put(r)
}

// ReadRequest reads and parses a request from b.
func ReadRequest(b *bfe_bufio.Reader, maxUriBytes int) (req *Request, err error) {
	totalRead := b.TotalRead

	tp := newTextprotoReader(b)
	req = new(Request)
	req.State = new(RequestState)

	// First line: GET /index.html HTTP/1.0
	var s string
	if s, err = tp.ReadLine(); err != nil {
		return nil, err
	}

	// mark start as time of reading out first line
	req.State.StartTime = time.Now()

	defer func() {
		putTextprotoReader(tp)
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
	}()

	var ok bool
	req.Method, req.RequestURI, req.Proto, ok = parseRequestLine(s)
	if !ok {
		return nil, &badStringError{"malformed HTTP request", s}
	}
	rawurl := req.RequestURI

	if len(rawurl) > maxUriBytes {
		return nil, fmt.Errorf("exceed maxUriBytes:%d", len(rawurl))
	}

	if req.ProtoMajor, req.ProtoMinor, ok = ParseHTTPVersion(req.Proto); !ok {
		return nil, &badStringError{"malformed HTTP version", req.Proto}
	}

	// CONNECT requests are used two different ways, and neither uses a full URL:
	// The standard use is to tunnel HTTPS through an HTTP proxy.
	// It looks like "CONNECT www.google.com:443 HTTP/1.1", and the parameter is
	// just the authority section of a URL. This information should go in req.URL.Host.
	//
	// The net/rpc package also uses CONNECT, but there the parameter is a path
	// that starts with a slash. It can be parsed with the regular URL parser,
	// and the path will end up in req.URL.Path, where it needs to be in order for
	// RPC to work.
	justAuthority := req.Method == MethodConnect && !strings.HasPrefix(rawurl, "/")
	if justAuthority {
		rawurl = "http://" + rawurl
	}

	if req.URL, err = url.ParseRequestURI(rawurl); err != nil {
		return nil, err
	}

	if justAuthority {
		// Strip the bogus "http://" back off.
		req.URL.Scheme = ""
	}

	// Subsequent lines: Key: value.
	mimeHeader, headerKeys, err := tp.ReadMIMEHeaderAndKeys()
	if err != nil {
		return nil, err
	}
	req.Header = Header(mimeHeader)
	req.HeaderKeys = headerKeys

	// RFC2616: Must treat
	//	GET /index.html HTTP/1.1
	//	Host: www.google.com
	// and
	//	GET http://www.google.com/index.html HTTP/1.1
	//	Host: doesntmatter
	// the same.  In the second case, any Host line is ignored.
	req.Host = req.URL.Host
	if req.Host == "" {
		req.Host = req.Header.GetDirect("Host")
	}
	delete(req.Header, "Host")

	fixPragmaCacheControl(req.Header)

	// TODO: Parse specific header values:
	//	Accept
	//	Accept-Encoding
	//	Accept-Language
	//	Authorization
	//	Cache-Control
	//	Connection
	//	Date
	//	Expect
	//	From
	//	If-Match
	//	If-Modified-Since
	//	If-None-Match
	//	If-Range
	//	If-Unmodified-Since
	//	Max-Forwards
	//	Proxy-Authorization
	//	Referer [sic]
	//	TE (transfer-codings)
	//	Trailer
	//	Transfer-Encoding
	//	Upgrade
	//	User-Agent
	//	Via
	//	Warning

	req.State.HeaderSize = uint32(b.TotalRead - totalRead)

	err = readTransfer(req, b)
	if err != nil {
		return nil, err
	}

	return req, nil
}

// MaxBytesReader is similar to io.LimitReader but is intended for
// limiting the size of incoming request bodies. In contrast to
// io.LimitReader, MaxBytesReader's result is a ReadCloser, returns a
// non-EOF error for a Read beyond the limit, and Closes the
// underlying reader when its Close method is called.
//
// MaxBytesReader prevents clients from accidentally or maliciously
// sending a large request and wasting server resources.
func MaxBytesReader(w ResponseWriter, r io.ReadCloser, n int64) io.ReadCloser {
	return &maxBytesReader{w: w, r: r, n: n}
}

type maxBytesReader struct {
	w       ResponseWriter
	r       io.ReadCloser // underlying reader
	n       int64         // max bytes remaining
	stopped bool
}

func (l *maxBytesReader) Read(p []byte) (n int, err error) {
	if l.n <= 0 {
		if !l.stopped {
			l.stopped = true
			//			if res, ok := l.w.(*response); ok {
			//				res.requestTooLarge()
			//			}
		}
		return 0, errors.New("http: request body too large")
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err = l.r.Read(p)
	l.n -= int64(n)
	return
}

func (l *maxBytesReader) Close() error {
	return l.r.Close()
}

func copyValues(dst, src url.Values) {
	for k, vs := range src {
		for _, value := range vs {
			dst.Add(k, value)
		}
	}
}

func parsePostForm(r *Request) (vs url.Values, err error) {
	if r.Body == nil {
		err = errors.New("missing form body")
		return
	}
	ct := r.Header.Get("Content-Type")
	ct, _, err = mime.ParseMediaType(ct)
	switch {
	case ct == "application/x-www-form-urlencoded":
		var reader io.Reader = r.Body
		maxFormSize := int64(1<<63 - 1)
		if _, ok := r.Body.(*maxBytesReader); !ok {
			maxFormSize = int64(10 << 20) // 10 MB is a lot of text.
			reader = io.LimitReader(r.Body, maxFormSize+1)
		}
		b, e := ioutil.ReadAll(reader)
		if e != nil {
			if err == nil {
				err = e
			}
			break
		}
		if int64(len(b)) > maxFormSize {
			err = errors.New("http: POST too large")
			return
		}
		vs, e = url.ParseQuery(string(b))
		if err == nil {
			err = e
		}
	case ct == "multipart/form-data":
		// handled by ParseMultipartForm (which is calling us, or should be)
		// TODO(bradfitz): there are too many possible
		// orders to call too many functions here.
		// Clean this up and write more tests.
		// request_test.go contains the start of this,
		// in TestRequestMultipartCallOrder.
	}
	return
}

// ParseForm parses the raw query from the URL and updates r.Form.
//
// For POST or PUT requests, it also parses the request body as a form and
// put the results into both r.PostForm and r.Form.
// POST and PUT body parameters take precedence over URL query string values
// in r.Form.
//
// If the request Body's size has not already been limited by MaxBytesReader,
// the size is capped at 10MB.
//
// ParseMultipartForm calls ParseForm automatically.
// It is idempotent.
func (r *Request) ParseForm() error {
	var err error
	if r.PostForm == nil {
		if r.Method == MethodPost || r.Method == MethodPut {
			r.PostForm, err = parsePostForm(r)
		}
		if r.PostForm == nil {
			r.PostForm = make(url.Values)
		}
	}
	if r.Form == nil {
		if len(r.PostForm) > 0 {
			r.Form = make(url.Values)
			copyValues(r.Form, r.PostForm)
		}
		var newValues url.Values
		if r.URL != nil {
			var e error
			newValues, e = url.ParseQuery(r.URL.RawQuery)
			if err == nil {
				err = e
			}
		}
		if newValues == nil {
			newValues = make(url.Values)
		}
		if r.Form == nil {
			r.Form = newValues
		} else {
			copyValues(r.Form, newValues)
		}
	}
	return err
}

// ParseMultipartForm parses a request body as multipart/form-data.
// The whole request body is parsed and up to a total of maxMemory bytes of
// its file parts are stored in memory, with the remainder stored on
// disk in temporary files.
// ParseMultipartForm calls ParseForm if necessary.
// After one call to ParseMultipartForm, subsequent calls have no effect.
func (r *Request) ParseMultipartForm(maxMemory int64) error {
	if r.MultipartForm == multipartByReader {
		return errors.New("http: multipart handled by MultipartReader")
	}
	if r.Form == nil {
		err := r.ParseForm()
		if err != nil {
			return err
		}
	}
	if r.MultipartForm != nil {
		return nil
	}

	mr, err := r.multipartReader()
	if err == ErrNotMultipart {
		return nil
	} else if err != nil {
		return err
	}

	f, err := mr.ReadForm(maxMemory)
	if err != nil {
		return err
	}
	for k, v := range f.Value {
		r.Form[k] = append(r.Form[k], v...)
	}
	r.MultipartForm = f

	return nil
}

// FormValue returns the first value for the named component of the query.
// POST and PUT body parameters take precedence over URL query string values.
// FormValue calls ParseMultipartForm and ParseForm if necessary.
// To access multiple values of the same key use ParseForm.
func (r *Request) FormValue(key string) string {
	if r.Form == nil {
		r.ParseMultipartForm(defaultMaxMemory)
	}
	if vs := r.Form[key]; len(vs) > 0 {
		return vs[0]
	}
	return ""
}

// PostFormValue returns the first value for the named component of the POST
// or PUT request body. URL query parameters are ignored.
// PostFormValue calls ParseMultipartForm and ParseForm if necessary.
func (r *Request) PostFormValue(key string) string {
	if r.PostForm == nil {
		r.ParseMultipartForm(defaultMaxMemory)
	}
	if vs := r.PostForm[key]; len(vs) > 0 {
		return vs[0]
	}
	return ""
}

// FormFile returns the first file for the provided form key.
// FormFile calls ParseMultipartForm and ParseForm if necessary.
func (r *Request) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	if r.MultipartForm == multipartByReader {
		return nil, nil, errors.New("http: multipart handled by MultipartReader")
	}
	if r.MultipartForm == nil {
		err := r.ParseMultipartForm(defaultMaxMemory)
		if err != nil {
			return nil, nil, err
		}
	}
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		if fhs := r.MultipartForm.File[key]; len(fhs) > 0 {
			f, err := fhs[0].Open()
			return f, fhs[0], err
		}
	}
	return nil, nil, ErrMissingFile
}

func (r *Request) ExpectsContinue() bool {
	return HasToken(r.Header.GetDirect("Expect"), "100-continue")
}

func (r *Request) WantsHttp10KeepAlive() bool {
	if r.ProtoMajor != 1 || r.ProtoMinor != 0 {
		return false
	}

	return HasToken(r.Header.GetDirect("Connection"), "keep-alive")
}

func (r *Request) WantsClose() bool {
	return HasToken(r.Header.GetDirect("Connection"), "close")
}

func (r *Request) closeBody() {
	if r.Body != nil {
		r.Body.Close()
	}
}
