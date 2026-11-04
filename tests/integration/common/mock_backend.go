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

package common

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"
)

// MockBackend wraps an httptest.Server and records request metadata.
type MockBackend struct {
	server      *httptest.Server
	ClusterName string
	Response    int
	Body        string
	// ReadBeforeClose, if greater than 0, causes the handler to read this
	// many bytes from the request body and then close the connection without
	// sending an HTTP response. This is useful for simulating a backend that
	// fails mid-stream.
	ReadBeforeClose int
	// DelayResponse sleeps for the given duration after reading the body and
	// before writing the response. It can be used to keep the request (and
	// any allocated body buffer) alive for a period of time.
	DelayResponse time.Duration
	// ReadNotify, if non-nil, is closed the first time the handler starts
	// reading the request body. This can be used to synchronize with the
	// allocation of body buffers inside BFE.
	ReadNotify chan struct{}
	// HoldResponse, if non-nil, blocks the handler after the request body has
	// been fully read and before the response is written. The response is only
	// sent after the channel is closed. This is useful for keeping a request
	// alive while another request is being processed.
	HoldResponse <-chan struct{}
	// HoldBeforeRead, if non-nil, blocks the handler after the request headers
	// have been received and before the body is read. This can be used to keep
	// BFE from closing the request body while another request is processed.
	HoldBeforeRead <-chan struct{}
	// ResponseFunc, if non-nil, overrides Response/Body and is called for each
	// request to determine the response status and body.
	ResponseFunc func(r *http.Request, count int) (int, string)
	// ResponseHeaders, if non-nil, is written to the response before the status code.
	ResponseHeaders map[string]string
	// NoContentLength, if true, flushes the response headers before writing
	// the body so the response uses chunked transfer encoding without a
	// Content-Length header. The default (false) lets net/http infer a
	// Content-Length for short bodies.
	NoContentLength bool
	// SSEEvents, if non-nil, switches the handler to server-sent event mode:
	// each entry is written as one "data: <event>\n\n" frame followed by a
	// flush. ResponseFunc/Response/Body are ignored in this mode.
	SSEEvents []string
	// SSERaw, if non-nil, switches the handler to raw server-sent event
	// mode: each entry is written verbatim (e.g.
	// "event: error\ndata: {\"type\":\"error\"}\n\n") followed by a flush.
	// It takes precedence over SSEEvents and exists for protocols whose
	// frames carry non-data lines (e.g. Anthropic "event: error").
	SSERaw []string
	// SSEHold, if non-nil, blocks the handler after SSEEvents have been
	// flushed and before SSETrailing is written. Closing it releases the stream.
	SSEHold <-chan struct{}
	// SSETrailing, if non-nil, is written (and flushed) after SSEHold is
	// released, simulating backend data arriving after a client abort.
	SSETrailing []string
	// SSEContentType overrides the Content-Type used in SSE/SSERaw modes
	// (default "text/event-stream"). It exists to exercise the response-side
	// SSE detection for vendor media types.
	SSEContentType string
	// SSECompress, when "gzip", compresses the SSE body: every frame is written
	// through one streaming gzip writer and flushed, the Content-Encoding: gzip
	// header is set and the response is chunked. It exists for issue #1468: an
	// upstream that compresses a server-sent event stream must still yield a
	// parseable usage, which only a per-frame flush can exercise (a single
	// compressed block does not cross frames).
	SSECompress string
	// SSEAbortAfter, if greater than 0, aborts the connection abruptly (no
	// terminating chunk, no gzip trailer) right after this many SSE frames
	// have been written and flushed, simulating an upstream process that dies
	// mid-stream (issue #1474, the kill -9 shape).
	SSEAbortAfter int
	// SNIRecorder, if non-nil, is called with the TLS ClientHello server name
	// on every TLS handshake (TLS mock backend only).
	SNIRecorder func(serverName string)
	// HoldAfterHeader, if non-nil, blocks the handler after the response
	// status/headers have been flushed and before the body is written. It
	// keeps BFE blocked on the response body while the headers are already on
	// the wire, so a client-side write timeout can be observed.
	HoldAfterHeader    <-chan struct{}
	hits               int
	mu                 sync.Mutex
	models             []string
	bodies             [][]byte
	authHeaders        []string
	xApiKeyHeaders     []string
	xGoogApiKeyHeaders []string
	anthropicVersions  []string
	urlPaths           []string
	rawQueries         []string
	hosts              []string
	contentLengths     []int64
	transferEncodings  [][]string
	headerCopies       []http.Header
}

// sseContentType returns the configured SSE media type.
func (b *MockBackend) sseContentType() string {
	if b.SSEContentType != "" {
		return b.SSEContentType
	}
	return "text/event-stream"
}

// SetReadBeforeClose sets ReadBeforeClose. It is called from tests while the
// server may still be handling a previous request, so the write must be
// synchronized with the handler reads.
func (b *MockBackend) SetReadBeforeClose(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ReadBeforeClose = n
}

// NewMockBackend starts a local HTTP server that returns the given status code.
func NewMockBackend(clusterName string, response int, body string) *MockBackend {
	b := &MockBackend{
		ClusterName: clusterName,
		Response:    response,
		Body:        body,
	}
	b.server = httptest.NewServer(b.handler())
	return b
}

// handler returns the recording mock handler shared by the plain and TLS
// mock backends.
func (b *MockBackend) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.hits++
		count := b.hits
		b.urlPaths = append(b.urlPaths, r.URL.Path)
		b.rawQueries = append(b.rawQueries, r.URL.RawQuery)
		b.hosts = append(b.hosts, r.Host)
		b.contentLengths = append(b.contentLengths, r.ContentLength)
		b.transferEncodings = append(b.transferEncodings, append([]string(nil), r.TransferEncoding...))
		b.headerCopies = append(b.headerCopies, r.Header.Clone())
		if b.ReadNotify != nil {
			close(b.ReadNotify)
			b.ReadNotify = nil
		}
		b.mu.Unlock()

		if b.HoldBeforeRead != nil {
			<-b.HoldBeforeRead
		}

		// snapshot under lock: tests may toggle ReadBeforeClose (via
		// SetReadBeforeClose) while a previous request is still being handled
		b.mu.Lock()
		readBeforeClose := b.ReadBeforeClose
		b.mu.Unlock()

		if readBeforeClose > 0 {
			_, _ = io.CopyN(io.Discard, r.Body, int64(readBeforeClose))
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, _ := hj.Hijack()
				if conn != nil {
					_ = conn.Close()
				}
			}
			return
		}

		if r.Body != nil {
			bodyBytes, _ := io.ReadAll(r.Body)
			b.mu.Lock()
			b.bodies = append(b.bodies, append([]byte(nil), bodyBytes...))
			b.authHeaders = append(b.authHeaders, r.Header.Get("Authorization"))
			b.xApiKeyHeaders = append(b.xApiKeyHeaders, r.Header.Get("x-api-key"))
			b.xGoogApiKeyHeaders = append(b.xGoogApiKeyHeaders, r.Header.Get("x-goog-api-key"))
			b.anthropicVersions = append(b.anthropicVersions, r.Header.Get("anthropic-version"))
			var reqBody map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &reqBody); err == nil {
				if model, ok := reqBody["model"].(string); ok {
					b.models = append(b.models, model)
				}
			}
			b.mu.Unlock()
		}
		if b.HoldResponse != nil {
			<-b.HoldResponse
		}
		if b.DelayResponse > 0 {
			time.Sleep(b.DelayResponse)
		}

		status, body := b.Response, b.Body
		if b.ResponseFunc != nil {
			status, body = b.ResponseFunc(r, count)
		}

		if b.SSERaw != nil || b.SSEEvents != nil {
			w.Header().Set("Content-Type", b.sseContentType())
			if b.SSECompress != "" {
				if b.SSECompress != "gzip" {
					http.Error(w, "unsupported SSECompress "+b.SSECompress, http.StatusInternalServerError)
					return
				}
				// Declared before WriteHeader so the header reaches the client;
				// BFE forwards it verbatim (issue #1468).
				w.Header().Set("Content-Encoding", "gzip")
			}
			w.WriteHeader(status)

			flusher, _ := w.(http.Flusher)
			var gzw *gzip.Writer
			if b.SSECompress == "gzip" {
				gzw = gzip.NewWriter(w)
			}
			// writeFrame emits one SSE frame, flushing the compressor per frame
			// so the response exercises incremental decompression on the way
			// back (a single compressed block would not cross frames).
			written := 0
			writeFrame := func(chunk string) {
				written++
				if gzw != nil {
					_, _ = io.WriteString(gzw, chunk)
					_ = gzw.Flush()
				} else {
					_, _ = io.WriteString(w, chunk)
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			// abortIfDue kills the connection without a terminating chunk once
			// SSEAbortAfter frames went out (issue #1474): net/http recovers
			// ErrAbortHandler by closing the TCP connection, exactly like an
			// upstream process killed mid-stream.
			abortIfDue := func() {
				if b.SSEAbortAfter > 0 && written >= b.SSEAbortAfter {
					panic(http.ErrAbortHandler)
				}
			}

			if b.SSERaw != nil {
				for _, chunk := range b.SSERaw {
					writeFrame(chunk)
					abortIfDue()
				}
			} else {
				for _, event := range b.SSEEvents {
					writeFrame("data: " + event + "\n\n")
					abortIfDue()
				}
				if b.SSEHold != nil {
					<-b.SSEHold
				}
				for _, event := range b.SSETrailing {
					writeFrame("data: " + event + "\n\n")
					abortIfDue()
				}
			}
			if gzw != nil {
				// Close writes the gzip trailer; the client (and BFE's tap) needs
				// it to see a clean end of stream.
				_ = gzw.Close()
			}
			return
		}

		for k, v := range b.ResponseHeaders {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		if b.HoldAfterHeader != nil {
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-b.HoldAfterHeader
		}
		if b.NoContentLength {
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if body != "" {
			w.Write([]byte(body))
		}
	}
}

// Hits returns the number of requests received.
func (b *MockBackend) Hits() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hits
}

// Models returns the list of model values observed in request bodies.
func (b *MockBackend) Models() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.models...)
}

// RequestBodies returns a deep copy of all observed request bodies.
func (b *MockBackend) RequestBodies() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([][]byte, len(b.bodies))
	for i, body := range b.bodies {
		result[i] = append([]byte(nil), body...)
	}
	return result
}

// AuthHeaders returns a deep copy of all observed Authorization headers.
func (b *MockBackend) AuthHeaders() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.authHeaders...)
}

// XApiKeyHeaders returns a deep copy of all observed x-api-key headers.
func (b *MockBackend) XApiKeyHeaders() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.xApiKeyHeaders...)
}

// XGoogApiKeyHeaders returns a deep copy of all observed x-goog-api-key headers.
func (b *MockBackend) XGoogApiKeyHeaders() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.xGoogApiKeyHeaders...)
}

// AnthropicVersions returns a deep copy of all observed anthropic-version headers.
func (b *MockBackend) AnthropicVersions() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.anthropicVersions...)
}

// URLPaths returns a deep copy of all observed request URL paths.
func (b *MockBackend) URLPaths() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.urlPaths...)
}

// RawQueries returns a deep copy of all observed request URL raw queries.
func (b *MockBackend) RawQueries() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.rawQueries...)
}

// Hosts returns the Host header values observed on the backend requests.
func (b *MockBackend) Hosts() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.hosts...)
}

// ContentLengths returns the ContentLength value of each backend request
// (-1 for chunked requests, 0 for requests without a body).
func (b *MockBackend) ContentLengths() []int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]int64(nil), b.contentLengths...)
}

// TransferEncodings returns the TransferEncoding values of each backend
// request (e.g. ["chunked"]).
func (b *MockBackend) TransferEncodings() [][]string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([][]string, len(b.transferEncodings))
	for i, enc := range b.transferEncodings {
		out[i] = append([]string(nil), enc...)
	}
	return out
}

// HeaderCopies returns deep copies of all observed request headers.
func (b *MockBackend) HeaderCopies() []http.Header {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]http.Header, len(b.headerCopies))
	for i, h := range b.headerCopies {
		result[i] = h.Clone()
	}
	return result
}

// Close shuts down the mock backend.
func (b *MockBackend) Close() {
	b.server.Close()
}

// Addr returns the host:port of the mock backend.
func (b *MockBackend) Addr() string {
	u, _ := url.Parse(b.server.URL)
	return u.Host
}

// HostPort returns the host and port of the mock backend.
func (b *MockBackend) HostPort() (string, int) {
	u, _ := url.Parse(b.server.URL)
	host := u.Hostname()
	port := 80
	if u.Port() != "" {
		fmt.Sscanf(u.Port(), "%d", &port)
	}
	return host, port
}
