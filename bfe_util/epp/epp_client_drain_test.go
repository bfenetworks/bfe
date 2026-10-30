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
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	http "github.com/bfenetworks/bfe/bfe_http"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/bfenetworks/go-lib/web-monitor/metrics"
)

// fakeExtProcServer implements extprocv3.ExternalProcessorServer with
// llm-d-like behavior knobs for the issue #1475 tests.
type fakeExtProcServer struct {
	extprocv3.UnimplementedExternalProcessorServer
	// respondPerChunk answers every message with a ProcessingResponse,
	// replicating the llm-d StreamingServer contract for streaming bodies.
	respondPerChunk bool
	// hangAfterHeaders stops reading and responding after the first
	// message, simulating an EPP peer that never consumes.
	hangAfterHeaders bool
	// hangAfterEndOfStream answers every message but never closes the
	// stream after the body's EndOfStream, simulating a stuck peer.
	hangAfterEndOfStream bool

	bytesReceived  int64
	sawEndOfStream int32
}

func (s *fakeExtProcServer) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	if body := first.GetResponseBody(); body != nil {
		atomic.AddInt64(&s.bytesReceived, int64(len(body.GetBody())))
	}
	if s.hangAfterHeaders {
		// Block until the stream/connection is torn down.
		<-stream.Context().Done()
		return stream.Context().Err()
	}
	if s.respondPerChunk {
		if err := stream.Send(&extprocv3.ProcessingResponse{}); err != nil {
			return err
		}
	}
	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}
		body := req.GetResponseBody()
		if body != nil {
			atomic.AddInt64(&s.bytesReceived, int64(len(body.GetBody())))
		}
		if s.respondPerChunk {
			if err := stream.Send(&extprocv3.ProcessingResponse{}); err != nil {
				return err
			}
		}
		if body != nil && body.GetEndOfStream() {
			atomic.StoreInt32(&s.sawEndOfStream, 1)
			if s.hangAfterEndOfStream {
				<-stream.Context().Done()
				return stream.Context().Err()
			}
			return nil // closing the stream lets the client drainer see EOF
		}
	}
}

func startFakeExtProc(t *testing.T, srv extprocv3.ExternalProcessorServer) *grpc.ClientConn {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	gs := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(gs, srv)
	go gs.Serve(lis)
	t.Cleanup(func() { gs.Stop(); lis.Close() })

	conn, err := NewGrpcConn(lis.Addr().String(), 3*time.Second, false, "", true)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn
}

// pacingReader yields body bytes with a small per-read delay so the
// producer (like a real upstream) never outruns the EPP forwarding
// goroutine; the byte-budget overflow protection stays untriggered and the
// test isolates the draining behavior.
type pacingReader struct {
	data []byte
	pace time.Duration
}

func (r *pacingReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	time.Sleep(r.pace)
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

// TestProcRespHeaderDrainsPerChunkResponses verifies the issue #1475
// fundamental fix: a peer that answers every response-body chunk (llm-d
// StreamingServer behavior) no longer stalls the exchange. A multi-MB body
// flows without triggering the buffer overflow, and Close settles quickly
// because the drainer consumes the per-chunk responses.
func TestProcRespHeaderDrainsPerChunkResponses(t *testing.T) {
	eppState.RespBodyOverflow = new(metrics.Counter)
	srv := &fakeExtProcServer{respondPerChunk: true}
	conn := startFakeExtProc(t, srv)

	c, err := NewEppClient(conn, 5*time.Second, WithCloseTimeout(5*time.Second))
	require.NoError(t, err)

	c.ProcRespHeader(make(http.Header), false)
	body := make([]byte, 5<<20)
	f := NewEppResponseBodyFilter(io.NopCloser(&pacingReader{data: body, pace: 100 * time.Microsecond}), c)
	n, err := io.Copy(io.Discard, f)
	require.NoError(t, err)
	assert.Equal(t, int64(len(body)), n)

	start := time.Now()
	require.NoError(t, f.Close())
	assert.Less(t, time.Since(start), 4*time.Second, "Close must settle quickly once drained")

	assert.False(t, c.RespBodyAborted(), "per-chunk responses must keep the exchange drained (no overflow)")
	assert.Equal(t, int64(0), eppState.RespBodyOverflow.Get())
	assert.Equal(t, int64(len(body)), atomic.LoadInt64(&srv.bytesReceived))
	assert.Equal(t, int32(1), atomic.LoadInt32(&srv.sawEndOfStream))
}

// TestCloseTimeoutWhenEppNeverResponds verifies the issue #1475 fallback: a
// peer that consumes nothing and never responds leaves the forwarding
// goroutine blocked in the header Recv; Close must return via the deadline
// (so the request finish handler still runs) and count EppCloseTimeout.
func TestCloseTimeoutWhenEppNeverResponds(t *testing.T) {
	eppState.EppCloseTimeout = new(metrics.Counter)
	before := eppState.EppCloseTimeout.Get()
	conn := startFakeExtProc(t, &fakeExtProcServer{hangAfterHeaders: true})

	c, err := NewEppClient(conn, 5*time.Second, WithCloseTimeout(300*time.Millisecond))
	require.NoError(t, err)

	c.ProcRespHeader(make(http.Header), false)
	time.Sleep(100 * time.Millisecond) // let the goroutine block in the header Recv

	start := time.Now()
	c.Close()
	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 300*time.Millisecond, "Close must wait out the deadline")
	assert.Less(t, elapsed, 5*time.Second, "Close must return via the deadline, not hang")
	assert.Equal(t, before+1, eppState.EppCloseTimeout.Get())
}

// TestCloseTimeoutWhenEppHangsAfterEndOfStream verifies that EndOfStream is
// still delivered (the EPP stream terminates from our side) and Close still
// returns via the deadline when the peer answers chunks but hangs after the
// body's EndOfStream instead of closing the stream.
func TestCloseTimeoutWhenEppHangsAfterEndOfStream(t *testing.T) {
	eppState.EppCloseTimeout = new(metrics.Counter)
	before := eppState.EppCloseTimeout.Get()
	srv := &fakeExtProcServer{respondPerChunk: true, hangAfterEndOfStream: true}
	conn := startFakeExtProc(t, srv)

	c, err := NewEppClient(conn, 5*time.Second, WithCloseTimeout(300*time.Millisecond))
	require.NoError(t, err)

	c.ProcRespHeader(make(http.Header), false)
	c.ProcRespBody([]byte("chunk"))

	start := time.Now()
	c.Close()
	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 300*time.Millisecond)
	assert.Less(t, elapsed, 5*time.Second)
	assert.Equal(t, before+1, eppState.EppCloseTimeout.Get())
	assert.Equal(t, int32(1), atomic.LoadInt32(&srv.sawEndOfStream),
		"EndOfStream must reach the peer even when it hangs afterwards")
}

// TestCloseSchedulingOnlyClient verifies Close on a client whose
// ProcRespHeader never ran (scheduling-only usage): no channels to wait on,
// cancel only.
func TestCloseSchedulingOnlyClient(t *testing.T) {
	c := &EppClient{cancel: func() {}}
	c.Close() // must not panic or block
}
