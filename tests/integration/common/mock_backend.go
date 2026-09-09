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
	// SSEHold, if non-nil, blocks the handler after SSEEvents have been
	// flushed and before SSETrailing is written. Closing it releases the stream.
	SSEHold <-chan struct{}
	// SSETrailing, if non-nil, is written (and flushed) after SSEHold is
	// released, simulating backend data arriving after a client abort.
	SSETrailing        []string
	hits               int
	mu                 sync.Mutex
	models             []string
	bodies             [][]byte
	authHeaders        []string
	xApiKeyHeaders     []string
	xGoogApiKeyHeaders []string
	anthropicVersions  []string
}

// NewMockBackend starts a local HTTP server that returns the given status code.
func NewMockBackend(clusterName string, response int, body string) *MockBackend {
	b := &MockBackend{
		ClusterName: clusterName,
		Response:    response,
		Body:        body,
	}
	b.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.hits++
		count := b.hits
		if b.ReadNotify != nil {
			close(b.ReadNotify)
			b.ReadNotify = nil
		}
		b.mu.Unlock()

		if b.HoldBeforeRead != nil {
			<-b.HoldBeforeRead
		}

		if b.ReadBeforeClose > 0 {
			_, _ = io.CopyN(io.Discard, r.Body, int64(b.ReadBeforeClose))
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

		if b.SSEEvents != nil {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(status)
			flusher, _ := w.(http.Flusher)
			writeSSE := func(events []string) {
				for _, event := range events {
					fmt.Fprintf(w, "data: %s\n\n", event)
					if flusher != nil {
						flusher.Flush()
					}
				}
			}
			writeSSE(b.SSEEvents)
			if b.SSEHold != nil {
				<-b.SSEHold
			}
			writeSSE(b.SSETrailing)
			return
		}

		for k, v := range b.ResponseHeaders {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		if b.NoContentLength {
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if body != "" {
			w.Write([]byte(body))
		}
	}))
	return b
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
