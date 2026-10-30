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

package epp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	http "github.com/bfenetworks/bfe/bfe_http"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	pb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/bfenetworks/go-lib/log"
	"github.com/bfenetworks/go-lib/web-monitor/metrics"
)

// DefaultRespBodyBufBudget is default byte budget for buffering response body
// chunks not yet consumed by EPP.
const DefaultRespBodyBufBudget = 1 << 20 // 1MB

// eppCloseTimeout bounds how long Close waits for the response forwarding
// goroutine and the response drainer to finish. Teardown normally completes
// in milliseconds; the deadline only covers a stuck EPP peer and guarantees
// the request finish handler (billing point) always runs (issue #1475).
const eppCloseTimeout = 30 * time.Second

// byteQueue is the FIFO of response body chunks pending for EPP (issue
// #1475). It is bounded by the byte budget in ProcRespBody alone: entries
// capacity is not an overflow criterion, so tiny frames cannot trigger a
// false overflow while the forwarding goroutine is still starting up or
// briefly lags (the old fixed-capacity channel overflowed after cap entries
// regardless of their size).
type byteQueue struct {
	mu     sync.Mutex
	chunks [][]byte
	wake   chan struct{} // capacity 1; closed by close
	closed bool
}

func newByteQueue() *byteQueue {
	return &byteQueue{wake: make(chan struct{}, 1)}
}

// push appends a chunk after Close; it never blocks. Chunks pushed after
// close are dropped (the stream is being torn down anyway).
func (q *byteQueue) push(d []byte) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.chunks = append(q.chunks, d)
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// pop blocks until a chunk is available or the queue is closed.
func (q *byteQueue) pop() ([]byte, bool) {
	for {
		q.mu.Lock()
		if len(q.chunks) > 0 {
			d := q.chunks[0]
			q.chunks[0] = nil
			q.chunks = q.chunks[1:]
			q.mu.Unlock()
			return d, true
		}
		if q.closed {
			q.mu.Unlock()
			return nil, false
		}
		q.mu.Unlock()
		<-q.wake
	}
}

func (q *byteQueue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		close(q.wake)
	}
}

// EppState is monitor state of EPP client.
type EppState struct {
	// RespBodyOverflow counts streams whose response body overflowed the
	// bounded buffer and was skipped (see ProcRespBody).
	RespBodyOverflow *metrics.Counter
	// EppCloseTimeout counts Close calls that hit eppCloseTimeout waiting
	// for the forwarding goroutine / response drainer and force-tore down
	// the stream (issue #1475). A non-zero value means the EPP peer is
	// stuck; the request still finishes and is billed.
	EppCloseTimeout *metrics.Counter
}

var eppState = new(EppState)

func GetEppState() *EppState {
	return eppState
}

// BuildTLSConfig builds client TLS config for EPP connections.
func BuildTLSConfig(insecureSkip bool, caFile string) (*tls.Config, error) {
	if insecureSkip {
		return &tls.Config{InsecureSkipVerify: true}, nil
	}

	pool := x509.NewCertPool()
	pemData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("epp: read CA file %q failed: %v", caFile, err)
	}
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("epp: parse CA file %q failed", caFile)
	}
	return &tls.Config{RootCAs: pool}, nil
}

// NewGrpcConn dials a long-lived EPP data connection. The connection is lazy:
// it is established on first use and multiplexes all streams of this address.
// connectTimeout bounds the connect handshake (grpc.WithConnectParams).
// plaintext=true dials without TLS (EPP serves plaintext gRPC), in which case
// insecureSkip/caFile are ignored. Caller owns the returned conn and must
// Close it when done.
func NewGrpcConn(addr string, connectTimeout time.Duration, insecureSkip bool, caFile string, plaintext bool) (*grpc.ClientConn, error) {
	var creds credentials.TransportCredentials
	if plaintext {
		creds = insecure.NewCredentials()
	} else {
		tlsConf, err := BuildTLSConfig(insecureSkip, caFile)
		if err != nil {
			return nil, err
		}
		creds = credentials.NewTLS(tlsConf)
	}

	params := grpc.ConnectParams{
		MinConnectTimeout: connectTimeout,
	}
	if params.MinConnectTimeout <= 0 {
		params.MinConnectTimeout = 3 * time.Second
	}

	return grpc.DialContext(context.Background(), addr,
		grpc.WithTransportCredentials(creds),
		grpc.WithConnectParams(params),
	)
}

type EppClient struct {
	client extprocv3.ExternalProcessorClient
	conn   *grpc.ClientConn
	ctx    context.Context
	cancel context.CancelFunc
	stream extprocv3.ExternalProcessor_ProcessClient
	datach *byteQueue
	donech chan struct{}
	// respDone is closed when the response drainer exits, i.e. every
	// response the EPP peer will send has been consumed and the stream is
	// settled from its side (issue #1475).
	respDone chan struct{}
	respOnce sync.Once

	closeOnce sync.Once // Close/CloseRespBody may be called twice (e.g. body Close in copyResponse plus deferred Close)

	callTimeout  time.Duration
	closeTimeout time.Duration // bound for Close's teardown wait (default eppCloseTimeout)
	bufBudget    int           // byte budget for chunks buffered in datach
	bufPending   int64         // bytes currently buffered in datach (atomic)
	aborted      int32         // response body skipped due to buffer overflow (atomic)
}

// EppClientOption customizes EppClient.
type EppClientOption func(*EppClient)

// WithRespBodyBufBudget sets byte budget of response body buffer.
func WithRespBodyBufBudget(budget int) EppClientOption {
	return func(c *EppClient) {
		if budget > 0 {
			c.bufBudget = budget
		}
	}
}

// WithCloseTimeout bounds how long Close waits for the forwarding goroutine
// and the response drainer before force-tearing down the stream (see
// eppCloseTimeout). Values <= 0 keep the default. Intended for tests.
func WithCloseTimeout(d time.Duration) EppClientOption {
	return func(c *EppClient) {
		if d > 0 {
			c.closeTimeout = d
		}
	}
}

// NewEppClient creates an EPP stream client on given connection.
// callTimeout limits the first message (RequestHeaders Send+Recv) round trip.
// The conn is NOT owned by EppClient: Close only tears down this stream and
// its goroutines, so a conn shared by many streams (one per request) is never
// closed by an individual stream teardown.
func NewEppClient(conn *grpc.ClientConn, callTimeout time.Duration, opts ...EppClientOption) (*EppClient, error) {
	eppClient := &EppClient{
		callTimeout:  callTimeout,
		closeTimeout: eppCloseTimeout,
		bufBudget:    DefaultRespBodyBufBudget,
	}
	for _, opt := range opts {
		opt(eppClient)
	}

	eppClient.client = extprocv3.NewExternalProcessorClient(conn)
	eppClient.conn = conn
	eppClient.ctx, eppClient.cancel = context.WithCancel(context.Background())

	stream, err := eppClient.client.Process(eppClient.ctx)
	if err != nil {
		return nil, err
	}
	eppClient.stream = stream

	return eppClient, nil
}

// Close tears down this stream (response forwarding goroutines included) and
// releases stream resources. It never closes the shared *grpc.ClientConn.
//
// Close waits for both the forwarding goroutine (donech) and the response
// drainer (respDone), bounded by eppCloseTimeout: the request finish handler
// (billing point) only runs after Close returns, so the wait must never be
// unbounded even when the EPP peer is stuck (issue #1475).
func (c *EppClient) Close() {
	c.CloseRespBody()

	deadline := time.NewTimer(c.closeTimeout)
	defer deadline.Stop()
	for _, ch := range []chan struct{}{c.donech, c.respDone} {
		if ch == nil {
			continue // scheduling-only client (ProcRespHeader never called)
		}
		select {
		case <-ch:
		case <-deadline.C:
			eppState.EppCloseTimeout.Inc(1)
			log.Logger.Warn("EppClient close timeout after %v, force teardown stream", c.closeTimeout)
			c.cancel()
			return
		}
	}
	c.cancel()
}

// markRespDone closes respDone exactly once. The drainer closes it when the
// stream settles; the forwarding goroutine closes it on early failures that
// happen before the drainer started.
func (c *EppClient) markRespDone() {
	c.respOnce.Do(func() {
		if c.respDone != nil {
			close(c.respDone)
		}
	})
}

// drainResponses consumes every ProcessingResponse the EPP peer sends and
// discards it (bfe does not apply response mutations). This is required by
// the envoy ext-proc FULL_DUPLEX_STREAMING contract: the peer (llm-d
// StreamingServer) answers every response-body chunk with a response, and
// its send blocks once the HTTP/2 windows fill if the client stops reading.
// An undrained stream therefore deadlocked both sides on long streams: the
// forwarding goroutine blocked in Send, donech never closed, Close blocked
// forever and the request was never billed (issue #1475). Draining keeps the
// peer's send path open so the whole exchange completes.
func (c *EppClient) drainResponses() {
	defer c.markRespDone()
	for {
		if _, err := c.stream.Recv(); err != nil {
			// io.EOF (peer finished), stream error, or ctx cancel: settled.
			return
		}
	}
}

func (c *EppClient) Send(req *extprocv3.ProcessingRequest) error {
	return c.stream.Send(req)
}

func (c *EppClient) Recv() (*extprocv3.ProcessingResponse, error) {
	return c.stream.Recv()
}

// RecvTimeout receives next message with timeout. On timeout the stream is
// left in unknown state; caller must Close the client.
func (c *EppClient) RecvTimeout(timeout time.Duration) (*extprocv3.ProcessingResponse, error) {
	if timeout <= 0 {
		return c.stream.Recv()
	}

	type recvResult struct {
		resp *extprocv3.ProcessingResponse
		err  error
	}
	ch := make(chan recvResult, 1)
	go func() {
		resp, err := c.stream.Recv()
		ch <- recvResult{resp: resp, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-ch:
		return r.resp, r.err
	case <-timer.C:
		return nil, fmt.Errorf("epp: recv timeout after %v", timeout)
	}
}

func (c *EppClient) ProcRespHeader(header http.Header, endofstream bool) {
	req := &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_ResponseHeaders{
			ResponseHeaders: BuildEnvoyGRPCHeaders(header, false, endofstream),
		},
	}
	c.datach = newByteQueue()
	c.donech = make(chan struct{})
	c.respDone = make(chan struct{})

	go func() {
		// defer c.Close()
		defer close(c.donech)

		// send request
		if err := c.Send(req); err != nil {
			log.Logger.Warn("EppClient ProcRespHeader send error: %v", err)
			c.markRespDone()
			return
		}
		// receive response
		_, err := c.Recv()
		if err != nil {
			log.Logger.Warn("EppClient ProcRespHeader recv error: %v", err)
			c.markRespDone()
			return
		}

		// From here on, stream.Recv is owned by the drainer: the peer sends
		// a response per body chunk (ext-proc FULL_DUPLEX_STREAMING) and
		// must never be left undrained (issue #1475).
		go c.drainResponses()

		for {
			d, ok := c.datach.pop()
			if !ok {
				break
			}
			atomic.AddInt64(&c.bufPending, -int64(len(d)))
			// send data to EPP server
			req := &extprocv3.ProcessingRequest{
				Request: &extprocv3.ProcessingRequest_ResponseBody{
					ResponseBody: &extprocv3.HttpBody{
						Body:        d,
						EndOfStream: false,
					},
				},
			}
			err := c.Send(req)
			if err != nil {
				// log error and return
				log.Logger.Warn("EppResponseBodyFilter send body chunk error: %v", err)
				return
			}
		}
		// send end of stream. This must always be sent (even if the body was
		// aborted on overflow), otherwise the EPP stream never terminates.
		req := &extprocv3.ProcessingRequest{
			Request: &extprocv3.ProcessingRequest_ResponseBody{
				ResponseBody: &extprocv3.HttpBody{
					Body:        []byte(""),
					EndOfStream: true,
				},
			},
		}
		err = c.Send(req)
		if err != nil {
			// log error
			log.Logger.Warn("EppResponseBodyFilter send end of stream error: %v", err)
			return
		}
		// No final Recv here: the drainer owns the stream's Recv side and
		// consumes the peer's trailing response (issue #1475).
	}()
}

// ProcRespBody buffers a response body chunk for EPP. Buffering is bounded by
// a byte budget: on overflow the whole remaining body of this stream is
// skipped (with counter and one log line), instead of silently dropping
// individual chunks. Never blocks the response forwarding path; the byte
// budget is the only overflow gate (see byteQueue).
func (c *EppClient) ProcRespBody(d []byte) {
	if atomic.LoadInt32(&c.aborted) != 0 {
		return
	}

	n := int64(len(d))
	for {
		pending := atomic.LoadInt64(&c.bufPending)
		if pending+n > int64(c.bufBudget) {
			c.abortRespBody()
			return
		}
		if atomic.CompareAndSwapInt64(&c.bufPending, pending, pending+n) {
			break
		}
	}

	c.datach.push(d)
}

func (c *EppClient) abortRespBody() {
	if atomic.CompareAndSwapInt32(&c.aborted, 0, 1) {
		eppState.RespBodyOverflow.Inc(1)
		log.Logger.Warn("EppClient response body buffer overflow (budget %d bytes), skip remaining body of this stream", c.bufBudget)
	}
}

// RespBodyAborted reports whether response body was skipped due to overflow.
func (c *EppClient) RespBodyAborted() bool {
	return atomic.LoadInt32(&c.aborted) != 0
}

func (c *EppClient) CloseRespBody() {
	c.closeOnce.Do(func() {
		if c.datach != nil {
			c.datach.close()
		}
	})
}

func BuildEnvoyGRPCHeaders(header http.Header, rawValue bool, endofstream bool) *pb.HttpHeaders {
	headerValues := make([]*corev3.HeaderValue, 0)
	for key, value := range header {
		header := &corev3.HeaderValue{Key: key}
		if rawValue {
			header.RawValue = []byte(value[0])
		} else {
			header.Value = value[0]
		}
		headerValues = append(headerValues, header)
	}
	return &pb.HttpHeaders{
		Headers: &corev3.HeaderMap{
			Headers: headerValues,
		},
		EndOfStream: endofstream,
	}
}

type EppResponseBodyFilter struct {
	source io.ReadCloser
	c      *EppClient
}

func NewEppResponseBodyFilter(source io.ReadCloser, c *EppClient) *EppResponseBodyFilter {
	return &EppResponseBodyFilter{
		source: source,
		c:      c,
	}
}

func (f *EppResponseBodyFilter) Read(p []byte) (n int, err error) {
	n, err = f.source.Read(p)
	if n > 0 {
		// Send a copy of the data to the channel to avoid data race
		dataCopy := make([]byte, n)
		copy(dataCopy, p[:n])
		f.c.ProcRespBody(dataCopy)
	}
	return n, err
}

func (f *EppResponseBodyFilter) Close() error {
	err := f.source.Close()
	f.c.Close()
	return err
}
