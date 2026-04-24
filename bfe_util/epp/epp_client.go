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
	"io"
	"time"

	http "github.com/bfenetworks/bfe/bfe_http"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	pb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/baidu/go-lib/log"
)

type EppGrpcClient interface {
    Conn() *grpc.ClientConn
    Close()
}

type SimpleGrpcClient struct {
    conn *grpc.ClientConn
}

func NewSimpleGrpcClient(addr string, timeout time.Duration) (EppGrpcClient, error) {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    conn, err := grpc.DialContext(ctx, addr, 
        grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
	    	InsecureSkipVerify: true, // skip cert verification for testing
	    })), 
        grpc.WithBlock(),
    )
    if err != nil {
        return nil, err
    }

    return &SimpleGrpcClient{conn: conn}, nil
}

func (c *SimpleGrpcClient) Conn() *grpc.ClientConn {
    if c == nil {
        return nil
    }
    return c.conn
}

func (c *SimpleGrpcClient) Close() {
    if c == nil || c.conn == nil {
        return
    }
    c.conn.Close()
    c.conn = nil
}

type EppClient struct {
    client extprocv3.ExternalProcessorClient
    conn   *grpc.ClientConn
    ctx    context.Context
    cancel context.CancelFunc
    stream extprocv3.ExternalProcessor_ProcessClient
    datach chan []byte
    donech chan struct{}
}

func NewEppClient(conn *grpc.ClientConn) (*EppClient, error) {
    eppClient := &EppClient{}
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
        // send end of stream
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

func (c *EppClient) ProcRespBody(d []byte) {
    // Non-blocking send - if channel is full, skip sending
    select {
    case c.datach <- d:
    default:
        // Channel is full, skip this data block
    }
}

func (c *EppClient) CloseRespBody() {
    if c.datach == nil {
        return
    }
    close(c.datach)
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
    c *EppClient
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
