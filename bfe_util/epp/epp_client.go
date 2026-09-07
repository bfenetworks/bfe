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

	"github.com/bfenetworks/go-lib/log"
	"github.com/bfenetworks/go-lib/web-monitor/metrics"
)

// DefaultRespBodyBufBudget is default byte budget for buffering response body
// chunks not yet consumed by EPP.
const DefaultRespBodyBufBudget = 1 << 20 // 1MB

// EppState is monitor state of EPP client.
type EppState struct {
	// RespBodyOverflow counts streams whose response body overflowed the
	// bounded buffer and was skipped (see ProcRespBody).
	RespBodyOverflow *metrics.Counter
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
// Caller owns the returned conn and must Close it when done.
func NewGrpcConn(addr string, connectTimeout time.Duration, insecureSkip bool, caFile string) (*grpc.ClientConn, error) {
	tlsConf, err := BuildTLSConfig(insecureSkip, caFile)
	if err != nil {
		return nil, err
	}

	params := grpc.ConnectParams{
		MinConnectTimeout: connectTimeout,
	}
	if params.MinConnectTimeout <= 0 {
		params.MinConnectTimeout = 3 * time.Second
	}

	return grpc.DialContext(context.Background(), addr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)),
		grpc.WithConnectParams(params),
	)
}

type EppClient struct {
	client extprocv3.ExternalProcessorClient
	conn   *grpc.ClientConn
	ctx    context.Context
	cancel context.CancelFunc
	stream extprocv3.ExternalProcessor_ProcessClient
	datach chan []byte
	donech chan struct{}

	closeOnce sync.Once // Close/CloseRespBody may be called twice (e.g. body Close in copyResponse plus deferred Close)

	callTimeout time.Duration
	bufBudget   int   // byte budget for chunks buffered in datach
	bufPending  int64 // bytes currently buffered in datach (atomic)
	aborted     int32 // response body skipped due to buffer overflow (atomic)
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

// NewEppClient creates an EPP stream client on given connection.
// callTimeout limits the first message (RequestHeaders Send+Recv) round trip.
// The conn is NOT owned by EppClient: Close only tears down this stream and
// its goroutines, so a conn shared by many streams (one per request) is never
// closed by an individual stream teardown.
func NewEppClient(conn *grpc.ClientConn, callTimeout time.Duration, opts ...EppClientOption) (*EppClient, error) {
	eppClient := &EppClient{callTimeout: callTimeout, bufBudget: DefaultRespBodyBufBudget}
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
func (c *EppClient) Close() {
	c.CloseRespBody()

	if c.donech != nil {
		<-c.donech
	}
	c.cancel()
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
	c.datach = make(chan []byte, 10)
	c.donech = make(chan struct{})

	go func() {
		// defer c.Close()
		defer close(c.donech)

		// send request
		if err := c.Send(req); err != nil {
			log.Logger.Warn("EppClient ProcRespHeader send error: %v", err)
			return
		}
		// receive response
		_, err := c.Recv()
		if err != nil {
			log.Logger.Warn("EppClient ProcRespHeader recv error: %v", err)
			return
		}

		for d := range c.datach {
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
		// receive response from EPP server
		_, err = c.Recv()
		if err != nil {
			// log error
			log.Logger.Warn("EppResponseBodyFilter recv end of stream response error: %v", err)
			return
		}
	}()
}

// ProcRespBody buffers a response body chunk for EPP. Buffering is bounded by
// a byte budget: on overflow the whole remaining body of this stream is
// skipped (with counter and one log line), instead of silently dropping
// individual chunks. Never blocks the response forwarding path.
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

	select {
	case c.datach <- d:
	default:
		// should not happen if budget accounting is correct; treat as overflow
		atomic.AddInt64(&c.bufPending, -n)
		c.abortRespBody()
	}
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
			close(c.datach)
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
