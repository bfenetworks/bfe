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
	"io"
	"testing"
	"time"

	http "github.com/bfenetworks/bfe/bfe_http"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/bfenetworks/go-lib/web-monitor/metrics"
)

func newTestEppClient(bufBudget int) *EppClient {
	c := &EppClient{
		bufBudget: bufBudget,
		datach:    make(chan []byte, 10),
		ctx:       context.Background(),
	}
	return c
}

func TestProcRespBodyBudget(t *testing.T) {
	eppState.RespBodyOverflow = new(metrics.Counter)
	c := newTestEppClient(100)

	// within budget: accepted, not aborted
	c.ProcRespBody(make([]byte, 60))
	c.ProcRespBody(make([]byte, 40))
	assert.False(t, c.RespBodyAborted())

	// overflow: aborted, counter increased only once
	before := eppState.RespBodyOverflow.Get()
	c.ProcRespBody(make([]byte, 1))
	assert.True(t, c.RespBodyAborted())
	c.ProcRespBody(make([]byte, 1))
	c.ProcRespBody(make([]byte, 1))
	assert.Equal(t, before+1, eppState.RespBodyOverflow.Get())
}

func TestProcRespBodyBudgetFreedOnConsume(t *testing.T) {
	c := newTestEppClient(100)

	c.ProcRespBody(make([]byte, 100))
	assert.False(t, c.RespBodyAborted())

	// simulate consumer draining buffer (as ProcRespHeader goroutine does)
	d := <-c.datach
	c.bufPending -= int64(len(d))

	// budget freed, next chunk accepted
	c.ProcRespBody(make([]byte, 100))
	assert.False(t, c.RespBodyAborted())
}

func TestProcRespBodyNeverBlocks(t *testing.T) {
	c := newTestEppClient(10)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			c.ProcRespBody(make([]byte, 8))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ProcRespBody blocked")
	}
	assert.True(t, c.RespBodyAborted())
}

// fakeProcessClient implements extprocv3.ExternalProcessor_ProcessClient.
type fakeProcessClient struct {
	grpc.ClientStream
	sent     []*extprocv3.ProcessingRequest
	recvResp *extprocv3.ProcessingResponse
	recvErr  error
}

func (f *fakeProcessClient) Header() (metadata.MD, error) { return nil, nil }
func (f *fakeProcessClient) Trailer() metadata.MD         { return nil }
func (f *fakeProcessClient) CloseSend() error             { return nil }
func (f *fakeProcessClient) Context() context.Context     { return context.Background() }
func (f *fakeProcessClient) SendMsg(m interface{}) error  { return nil }
func (f *fakeProcessClient) RecvMsg(m interface{}) error  { return nil }

func (f *fakeProcessClient) Send(req *extprocv3.ProcessingRequest) error {
	f.sent = append(f.sent, req)
	return nil
}

func (f *fakeProcessClient) Recv() (*extprocv3.ProcessingResponse, error) {
	return f.recvResp, f.recvErr
}

func newFakeStreamClient(t *testing.T, budget int) (*EppClient, *fakeProcessClient) {
	stream := &fakeProcessClient{
		recvResp: &extprocv3.ProcessingResponse{},
	}
	c := &EppClient{
		stream:    stream,
		bufBudget: budget,
		ctx:       context.Background(),
		cancel:    func() {},
	}
	return c, stream
}

func waitDone(t *testing.T, c *EppClient) {
	t.Helper()
	select {
	case <-c.donech:
	case <-time.After(2 * time.Second):
		t.Fatal("response forwarding goroutine did not finish")
	}
}

func lastSentRequest(t *testing.T, stream *fakeProcessClient) *extprocv3.ProcessingRequest {
	t.Helper()
	require.NotEmpty(t, stream.sent)
	return stream.sent[len(stream.sent)-1]
}

func TestProcRespHeaderEndOfStreamSent(t *testing.T) {
	c, stream := newFakeStreamClient(t, 100)

	c.ProcRespHeader(make(http.Header), false)
	c.ProcRespBody([]byte("chunk1"))
	c.ProcRespBody([]byte("chunk2"))
	c.Close()

	waitDone(t, c)

	// EndOfStream must be the last message sent
	last := lastSentRequest(t, stream)
	body := last.GetResponseBody()
	require.NotNil(t, body)
	assert.True(t, body.GetEndOfStream())
	assert.False(t, c.RespBodyAborted())
}

func TestProcRespHeaderEndOfStreamSentAfterOverflow(t *testing.T) {
	c, stream := newFakeStreamClient(t, 10)

	c.ProcRespHeader(make(http.Header), false)
	c.ProcRespBody([]byte("0123456789")) // fills budget
	c.ProcRespBody([]byte("overflow"))   // triggers abort
	c.ProcRespBody([]byte("skipped"))
	require.True(t, c.RespBodyAborted())
	c.Close()

	waitDone(t, c)

	// EOS must still be sent so the EPP stream terminates
	last := lastSentRequest(t, stream)
	body := last.GetResponseBody()
	require.NotNil(t, body)
	assert.True(t, body.GetEndOfStream())
	assert.Equal(t, 0, len(body.GetBody()))

	// the overflowed chunk must not be forwarded
	for _, req := range stream.sent {
		if b := req.GetResponseBody(); b != nil && !b.GetEndOfStream() {
			assert.NotEqual(t, "overflow", string(b.GetBody()))
			assert.NotEqual(t, "skipped", string(b.GetBody()))
		}
	}
}

func TestEppResponseBodyFilterOverflow(t *testing.T) {
	eppState.RespBodyOverflow = new(metrics.Counter)
	c, stream := newFakeStreamClient(t, 4)

	c.ProcRespHeader(make(http.Header), false)
	before := eppState.RespBodyOverflow.Get()

	f := NewEppResponseBodyFilter(io.NopCloser(&blockingReader{data: []byte("0123456789")}), c)
	buf := make([]byte, 16)
	n, err := f.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 10, n)

	assert.True(t, c.RespBodyAborted())
	assert.Equal(t, before+1, eppState.RespBodyOverflow.Get())

	require.NoError(t, f.Close())
	waitDone(t, c)

	// EOS still sent after overflow
	last := lastSentRequest(t, stream)
	assert.True(t, last.GetResponseBody().GetEndOfStream())
}

// blockingReader returns data once, then blocks until closed, simulating a
// slow backend that never drains.
type blockingReader struct {
	data   []byte
	closed bool
}

func (r *blockingReader) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	select {} // block forever
}

func (r *blockingReader) Close() error {
	r.closed = true
	return nil
}

func TestBuildTLSConfig(t *testing.T) {
	t.Run("insecure", func(t *testing.T) {
		conf, err := BuildTLSConfig(true, "")
		require.NoError(t, err)
		assert.True(t, conf.InsecureSkipVerify)
	})

	t.Run("missing CA file", func(t *testing.T) {
		_, err := BuildTLSConfig(false, "/nonexistent/ca.crt")
		assert.Error(t, err)
	})

	t.Run("invalid CA content", func(t *testing.T) {
		_, err := BuildTLSConfig(false, "epp_client.go")
		assert.Error(t, err)
	})
}

func TestNewGrpcConnLazyDial(t *testing.T) {
	// lazy dial: unreachable address returns a conn without blocking;
	// failure surfaces on first use (covered by bal_gslb tests)
	conn, err := NewGrpcConn("127.0.0.1:1", 100*time.Millisecond, true, "")
	require.NoError(t, err)
	require.NotNil(t, conn)
	conn.Close()
}
