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
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/structpb"
)

// MockEPP is an in-process mock EPP (ext-proc) server. It implements the
// envoy ext_proc v3 ExternalProcessor service plus the gRPC health service
// (used by BFE's EPP health checker), speaks TLS with a self-signed
// certificate (BFE dials EPP with InsecureSkipVerify when EPPTLS is unset),
// and records the pool metadata, request headers/body and response
// headers/body of every processed stream for test assertions.
type MockEPP struct {
	t    *testing.T
	addr string
	srv  *grpc.Server
	ln   net.Listener

	// decisionEndpoint is returned as x-gateway-destination-endpoint.
	decisionEndpoint string
	// requestError, if non-nil, is returned as a gRPC status error on the
	// first message (RequestHeaders) of every stream.
	requestError error

	mu             sync.Mutex
	pools          []string
	reqHeaders     []http.Header
	reqBodies      [][]byte
	respHeaders    []http.Header
	respBodies     [][]byte // assembled body of each completed stream
	serving        bool
	health         *health.Server
	healthShutdown bool
	done           chan struct{}
}

// NewMockEPP starts a mock EPP server on a free loopback port.
func NewMockEPP(t *testing.T) *MockEPP {
	t.Helper()

	certPEM, keyPEM := generateSelfSignedCert(t)
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("mock epp: load key pair failed: %v", err)
	}

	port, err := FindFreePort()
	if err != nil {
		t.Fatalf("mock epp: find free port failed: %v", err)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("mock epp: listen failed: %v", err)
	}

	m := &MockEPP{
		t:       t,
		addr:    ln.Addr().String(),
		serving: true,
		done:    make(chan struct{}),
	}
	m.srv = grpc.NewServer(grpc.Creds(credentials.NewServerTLSFromCert(&cert)))
	extprocv3.RegisterExternalProcessorServer(m.srv, m)
	hs := health.NewServer()
	m.setHealthStatusLocked(hs)
	grpc_health_v1.RegisterHealthServer(m.srv, hs)
	m.health = hs

	go func() {
		if err := m.srv.Serve(ln); err != nil {
			select {
			case <-m.done:
			default:
				t.Logf("mock epp: serve error: %v", err)
			}
		}
	}()

	if err := WaitForTCP(m.addr, 10*time.Second); err != nil {
		m.srv.Stop()
		t.Fatalf("mock epp did not start: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

// generateSelfSignedCert creates a throwaway server certificate. BFE skips
// certificate verification when EPPTLS is unset (tlsInsecure), so the CA is
// irrelevant.
func generateSelfSignedCert(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("mock epp: generate key failed: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "mock-epp"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("mock epp: create certificate failed: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}

func (m *MockEPP) setHealthStatusLocked(hs *health.Server) {
	if m.serving {
		hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	} else {
		hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	}
}

// Addr returns the mock EPP listen address (host:port).
func (m *MockEPP) Addr() string { return m.addr }

// Close stops the mock EPP server.
func (m *MockEPP) Close() {
	select {
	case <-m.done:
		return
	default:
		close(m.done)
	}
	if m.srv != nil {
		m.srv.Stop()
	}
}

// SetDecisionEndpoint sets the endpoint address returned in dynamic metadata
// (envoy.lb -> x-gateway-destination-endpoint).
func (m *MockEPP) SetDecisionEndpoint(addr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decisionEndpoint = addr
}

// SetRequestError makes the server fail every stream on its first message
// with the given error (typically a gRPC status error created with
// status.Error). Pass nil to restore normal behavior.
func (m *MockEPP) SetRequestError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestError = err
}

// SetServing controls the gRPC health status reported to BFE's EPP health
// checker.
func (m *MockEPP) SetServing(serving bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serving = serving
	if m.health != nil && !m.healthShutdown {
		m.setHealthStatusLocked(m.health)
	}
}

// ShutdownHealth simulates an EPP process crash: the gRPC health service is
// deregistered entirely, so health checks fail with UNIMPLEMENTED.
func (m *MockEPP) ShutdownHealth() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthShutdown = true
	if m.health != nil {
		m.health.Shutdown()
	}
}

// Pools returns the inference-pool values observed in stream metadata.
func (m *MockEPP) Pools() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.pools...)
}

// RequestHeadersList returns the request headers of each processed stream.
func (m *MockEPP) RequestHeadersList() []http.Header {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]http.Header, len(m.reqHeaders))
	copy(out, m.reqHeaders)
	return out
}

// RequestBodies returns the request bodies of each processed stream.
func (m *MockEPP) RequestBodies() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]byte, len(m.reqBodies))
	copy(out, m.reqBodies)
	return out
}

// ResponseHeadersList returns the response headers of each processed stream.
func (m *MockEPP) ResponseHeadersList() []http.Header {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]http.Header, len(m.respHeaders))
	copy(out, m.respHeaders)
	return out
}

// ResponseBodies returns the assembled response bodies of each completed
// stream (what the mock received via ResponseBody messages, excluding the
// trailing EndOfStream message).
func (m *MockEPP) ResponseBodies() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]byte, len(m.respBodies))
	copy(out, m.respBodies)
	return out
}

// StreamCount returns the number of streams that sent RequestHeaders.
func (m *MockEPP) StreamCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pools)
}

// Process implements the ext-proc bidirectional stream. The conversation
// mirrors what BFE's bal_gslb.callEPP / EppClient produce:
//
//	request  dir: RequestHeaders [+ RequestBody]
//	response dir: decision (dynamic metadata) [+ empty body reply]
//	request  dir: ResponseHeaders, ResponseBody*
//	response dir: empty replies
func (m *MockEPP) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	// ---- request headers (+ optional body) ----
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	rh := req.GetRequestHeaders()
	if rh == nil {
		return fmt.Errorf("mock epp: first message is not RequestHeaders")
	}

	pool := extractPoolName(req.GetMetadataContext())
	m.mu.Lock()
	m.pools = append(m.pools, pool)
	m.reqHeaders = append(m.reqHeaders, headersToHTTP(rh.GetHeaders()))
	reqErr := m.requestError
	decision := m.decisionEndpoint
	m.mu.Unlock()

	if reqErr != nil {
		return reqErr
	}

	var reqBody []byte
	if !rh.EndOfStream {
		bodyMsg, err := stream.Recv()
		if err != nil {
			return err
		}
		rb := bodyMsg.GetRequestBody()
		if rb == nil {
			return fmt.Errorf("mock epp: expected RequestBody")
		}
		reqBody = rb.Body
	}

	m.mu.Lock()
	m.reqBodies = append(m.reqBodies, reqBody)
	m.mu.Unlock()

	// reply to RequestHeaders with the scheduling decision
	if err := stream.Send(decisionResponse(decision)); err != nil {
		return err
	}
	if !rh.EndOfStream {
		// reply to RequestBody
		if err := stream.Send(&extprocv3.ProcessingResponse{
			Response: &extprocv3.ProcessingResponse_RequestBody{
				RequestBody: &extprocv3.BodyResponse{},
			},
		}); err != nil {
			return err
		}
	}

	// ---- response headers ----
	msg, err := stream.Recv()
	if err != nil {
		// BFE may close the stream without a response (e.g. balancer gave up);
		// treat as a normal end.
		return nil
	}
	respH := msg.GetResponseHeaders()
	if respH == nil {
		return fmt.Errorf("mock epp: expected ResponseHeaders")
	}

	m.mu.Lock()
	m.respHeaders = append(m.respHeaders, headersToHTTP(respH.GetHeaders()))
	m.mu.Unlock()

	if err := stream.Send(&extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ResponseHeaders{
			ResponseHeaders: &extprocv3.HeadersResponse{},
		},
	}); err != nil {
		return err
	}

	// ---- response body until EndOfStream ----
	var respBody []byte
	for {
		msg, err := stream.Recv()
		if err != nil {
			return nil
		}
		rb := msg.GetResponseBody()
		if rb == nil {
			return fmt.Errorf("mock epp: expected ResponseBody")
		}
		if rb.EndOfStream {
			break
		}
		respBody = append(respBody, rb.Body...)
	}

	m.mu.Lock()
	m.respBodies = append(m.respBodies, respBody)
	m.mu.Unlock()

	return stream.Send(&extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ResponseBody{
			ResponseBody: &extprocv3.BodyResponse{},
		},
	})
}

// extractPoolName reads llm-d.ai/inference-pool from the stream metadata.
func extractPoolName(md *corev3.Metadata) string {
	if md == nil {
		return ""
	}
	s, ok := md.FilterMetadata["llm-d.ai"]
	if !ok || s == nil {
		return ""
	}
	v, ok := s.Fields["inference-pool"]
	if !ok {
		return ""
	}
	return v.GetStringValue()
}

// headersToHTTP converts an envoy header map to http.Header (last value wins
// for duplicate keys, which is fine for test assertions). BFE sends values
// in RawValue, so fall back from Value.
func headersToHTTP(hm *corev3.HeaderMap) http.Header {
	h := make(http.Header)
	if hm == nil {
		return h
	}
	for _, hv := range hm.GetHeaders() {
		v := hv.Value
		if v == "" && len(hv.RawValue) > 0 {
			v = string(hv.RawValue)
		}
		h.Set(hv.Key, v)
	}
	return h
}

// decisionResponse builds the ext-proc reply carrying the scheduling
// decision in dynamic metadata: envoy.lb -> x-gateway-destination-endpoint.
func decisionResponse(endpoint string) *extprocv3.ProcessingResponse {
	resp := &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocv3.HeadersResponse{},
		},
	}
	if endpoint == "" {
		return resp
	}
	lb, _ := structpb.NewStruct(map[string]interface{}{
		"x-gateway-destination-endpoint": endpoint,
	})
	resp.DynamicMetadata = &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"envoy.lb": structpb.NewStructValue(lb),
		},
	}
	return resp
}
