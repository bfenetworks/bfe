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

package bal_gslb

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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/gslb_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
)

// testServerTLSCert generates a self-signed certificate for 127.0.0.1/localhost.
func testServerTLSCert(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "bfe-epp-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	return cert
}

// fakeExtProc is a fake ExternalProcessor server capturing the first request.
type fakeExtProc struct {
	extprocv3.UnimplementedExternalProcessorServer
	onFirst func(req *extprocv3.ProcessingRequest) error
	md      *structpb.Struct

	mu      sync.Mutex
	first   *extprocv3.ProcessingRequest
	calls   int
	procErr error // if set, Process fails with this error
}

func (s *fakeExtProc) capturedFirst() *extprocv3.ProcessingRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.first
}

func (s *fakeExtProc) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *fakeExtProc) setProcErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.procErr = err
}

func (s *fakeExtProc) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.first = req
	s.calls++
	procErr := s.procErr
	onFirst := s.onFirst
	s.mu.Unlock()

	if procErr != nil {
		return procErr
	}
	if onFirst != nil {
		if err := onFirst(req); err != nil {
			return err
		}
	}

	resp := &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocv3.HeadersResponse{},
		},
		DynamicMetadata: s.md,
	}
	if err := stream.Send(resp); err != nil {
		return err
	}

	// if request has body, read it to the end and reply
	if !req.GetRequestHeaders().GetEndOfStream() {
		for {
			r, err := stream.Recv()
			if err != nil {
				return err
			}
			if b := r.GetRequestBody(); b != nil && b.GetEndOfStream() {
				break
			}
		}
		if err := stream.Send(&extprocv3.ProcessingResponse{
			Response: &extprocv3.ProcessingResponse_RequestBody{
				RequestBody: &extprocv3.BodyResponse{},
			},
		}); err != nil {
			return err
		}
	}

	// drain until client closes the stream
	for {
		if _, err := stream.Recv(); err != nil {
			return nil
		}
	}
}

func decisionMD(endpoint string) *structpb.Struct {
	md, _ := structpb.NewStruct(map[string]any{
		"envoy.lb": map[string]any{
			"x-gateway-destination-endpoint": endpoint,
		},
	})
	return md
}

type eppTestServer struct {
	addr   string
	health *health.Server
	proc   *fakeExtProc
	grpc   *grpc.Server
}

func (s *eppTestServer) setServing(serving bool) {
	status := grpc_health_v1.HealthCheckResponse_SERVING
	if !serving {
		status = grpc_health_v1.HealthCheckResponse_NOT_SERVING
	}
	s.health.SetServingStatus("", status)
}

func startEPPTestServer(t *testing.T, withProc bool) *eppTestServer {
	t.Helper()

	cert := testServerTLSCert(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer(grpc.Creds(credentials.NewServerTLSFromCert(&cert)))
	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(srv, hs)

	ts := &eppTestServer{
		addr:   lis.Addr().String(),
		health: hs,
		grpc:   srv,
	}
	if withProc {
		ts.proc = &fakeExtProc{md: decisionMD("127.0.0.1:9999")}
		extprocv3.RegisterExternalProcessorServer(srv, ts.proc)
	}

	go srv.Serve(lis)
	t.Cleanup(func() {
		srv.Stop()
		lis.Close()
	})
	return ts
}

// startPlaintextEPPTestServer starts a plaintext (no TLS) EPP test server.
func startPlaintextEPPTestServer(t *testing.T, withProc bool) *eppTestServer {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(srv, hs)

	ts := &eppTestServer{
		addr:   lis.Addr().String(),
		health: hs,
		grpc:   srv,
	}
	if withProc {
		ts.proc = &fakeExtProc{md: decisionMD("127.0.0.1:9999")}
		extprocv3.RegisterExternalProcessorServer(srv, ts.proc)
	}

	go srv.Serve(lis)
	t.Cleanup(func() {
		srv.Stop()
		lis.Close()
	})
	return ts
}

func testRuntimeConf(mutate func(*eppRuntimeConf)) eppRuntimeConf {
	conf := eppRuntimeConf{
		tlsInsecure:      true,
		connectTimeout:   300 * time.Millisecond,
		callTimeout:      2 * time.Second,
		checkInterval:    25 * time.Millisecond,
		failThreshold:    2,
		cooldown:         400 * time.Millisecond,
		successThreshold: 2,
	}
	if mutate != nil {
		mutate(&conf)
	}
	return conf
}

func waitForCond(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for: %s", msg)
}

func TestEPPRuntimeFailoverFailback(t *testing.T) {
	primary := startEPPTestServer(t, false)
	backup := startEPPTestServer(t, false)

	rt, err := newEPPRuntime("cluster-x", []string{primary.addr, backup.addr}, testRuntimeConf(nil))
	require.NoError(t, err)
	defer rt.closeConns()
	defer rt.stopProbes()

	assert.Equal(t, 0, rt.activeIndex())

	// primary goes down: failover to backup after failThreshold consecutive fails
	primary.setServing(false)
	waitForCond(t, 5*time.Second, "failover to backup", func() bool {
		return rt.activeIndex() == 1
	})

	// primary recovers: no failback within cooldown
	primary.setServing(true)
	time.Sleep(150 * time.Millisecond) // less than cooldown (400ms)
	assert.Equal(t, 1, rt.activeIndex())

	// after cooldown + successThreshold consecutive successes: failback
	waitForCond(t, 5*time.Second, "failback to primary", func() bool {
		return rt.activeIndex() == 0
	})
}

func TestEPPRuntimeAllDownStaysOnActive(t *testing.T) {
	primary := startEPPTestServer(t, false)
	backup := startEPPTestServer(t, false)

	rt, err := newEPPRuntime("cluster-x", []string{primary.addr, backup.addr}, testRuntimeConf(nil))
	require.NoError(t, err)
	defer rt.closeConns()
	defer rt.stopProbes()

	// both down: no failover target is healthy, active index stays put and
	// does not flap between unhealthy addrs
	primary.setServing(false)
	backup.setServing(false)

	time.Sleep(600 * time.Millisecond)
	assert.Equal(t, 0, rt.activeIndex())
}

func TestEPPRuntimeSingleAddrFailover(t *testing.T) {
	primary := startEPPTestServer(t, false)

	rt, err := newEPPRuntime("cluster-x", []string{primary.addr}, testRuntimeConf(nil))
	require.NoError(t, err)
	defer rt.closeConns()
	defer rt.stopProbes()

	// single address: no alternative, active index stays 0
	primary.setServing(false)
	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, 0, rt.activeIndex())
}

func TestEPPRuntimePlaintextProbe(t *testing.T) {
	server := startPlaintextEPPTestServer(t, false)

	rt, err := newEPPRuntime("cluster-x", []string{server.addr}, testRuntimeConf(func(c *eppRuntimeConf) {
		c.plaintext = true
	}))
	require.NoError(t, err)
	defer rt.closeConns()
	defer rt.stopProbes()

	// plaintext probe reaches the plaintext health service
	require.NoError(t, rt.probe(0))

	// NOT_SERVING surfaces as probe failure
	server.setServing(false)
	assert.Error(t, rt.probe(0))
}

func TestBalanceEppPlaintext(t *testing.T) {
	server := startPlaintextEPPTestServer(t, true)
	conf := makeEPPGslbBasicConf(t, []string{server.addr}, func(conf *cluster_conf.GslbBasicConf) {
		conf.EPPTLS = &cluster_conf.EPPTLSConf{Plaintext: true}
	})
	bal := makeTestBal(t, conf)

	bk, err := bal.BalanceEpp(prepareEPPRequest())
	require.NoError(t, err)
	require.NotNil(t, bk)
	assert.NotNil(t, server.proc.capturedFirst())
}

func makeEPPGslbBasicConf(t *testing.T, addrs []string, mutate func(*cluster_conf.GslbBasicConf)) cluster_conf.GslbBasicConf {
	t.Helper()

	crossRetry := 0
	retryMax := 2
	mode := cluster_conf.BalanceModeEPP
	conf := cluster_conf.GslbBasicConf{
		CrossRetry:  &crossRetry,
		RetryMax:    &retryMax,
		HashConf:    &cluster_conf.HashConf{},
		BalanceMode: &mode,
		EPPAddr:     &addrs,
	}
	if mutate != nil {
		mutate(&conf)
	}
	require.NoError(t, cluster_conf.GslbBasicConfCheck(&conf))
	return conf
}

func makeTestBal(t *testing.T, conf cluster_conf.GslbBasicConf) *BalanceGslb {
	t.Helper()

	var g gslb_conf.GslbClusterConf
	var c cluster_table_conf.ClusterBackend
	require.NoError(t, loadJson("testdata/g1", &g))
	require.NoError(t, loadJson("testdata/cluster1", &c))

	bal := NewBalanceGslb("cluster_demo")
	require.NoError(t, bal.Init(g))
	require.NoError(t, bal.BackendReload(c))
	bal.SetGslbBasic(conf)
	t.Cleanup(bal.Release)
	return bal
}

func prepareEPPRequest() *bfe_basic.Request {
	req := prepareRequest()
	req.Context = make(map[interface{}]interface{})
	req.OutRequest = &bfe_http.Request{
		Header: make(bfe_http.Header),
	}
	return req
}

func TestBalanceEppMetadataInjection(t *testing.T) {
	server := startEPPTestServer(t, true)
	conf := makeEPPGslbBasicConf(t, []string{server.addr}, nil)
	bal := makeTestBal(t, conf)

	bk, err := bal.BalanceEpp(prepareEPPRequest())
	require.NoError(t, err)
	require.NotNil(t, bk)

	first := server.proc.capturedFirst()
	require.NotNil(t, first)
	md := first.GetMetadataContext().GetFilterMetadata()["llm-d.ai"]
	require.NotNil(t, md)
	assert.Equal(t, "cluster_demo", md.Fields["inference-pool"].GetStringValue())
}

func TestBalanceEppAllDownFallbackError(t *testing.T) {
	// no server listening on this address
	conf := makeEPPGslbBasicConf(t, []string{"127.0.0.1:1"}, nil)
	bal := makeTestBal(t, conf)

	// data connection is unavailable: BalanceEpp returns error,
	// caller (reverseproxy) falls back to local balance
	bk, err := bal.BalanceEpp(prepareEPPRequest())
	require.Error(t, err)
	assert.Nil(t, bk)
}

func TestBalanceEppRetryOnNextAddr(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"unavailable cell draining", status.Error(codes.Unavailable, "cell is not serving")},
		{"internal unknown pool", status.Error(codes.Internal, "unknown inference pool: cluster_demo")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failing := startEPPTestServer(t, true)
			failing.proc.onFirst = func(req *extprocv3.ProcessingRequest) error {
				return tc.err
			}
			backup := startEPPTestServer(t, true)

			// health check disabled: retry is driven by request path only
			conf := makeEPPGslbBasicConf(t, []string{failing.addr, backup.addr}, func(conf *cluster_conf.GslbBasicConf) {
				conf.EPPCheck = &cluster_conf.EPPCheckConf{Disabled: true}
			})
			bal := makeTestBal(t, conf)

			bk, err := bal.BalanceEpp(prepareEPPRequest())
			require.NoError(t, err)
			require.NotNil(t, bk)

			// request reached backup and got its decision
			require.NotNil(t, backup.proc.capturedFirst())
			// active index unchanged: request-level retry does not affect state machine
			assert.Equal(t, 0, bal.getEPPRt().activeIndex())
		})
	}
}

func TestBalanceEppNoRetryOnMissingPool(t *testing.T) {
	// "missing inference-pool metadata" is a BFE defect: must not retry
	failing := startEPPTestServer(t, true)
	failing.proc.onFirst = func(req *extprocv3.ProcessingRequest) error {
		return status.Error(codes.Internal, "missing inference-pool metadata")
	}
	backup := startEPPTestServer(t, true)

	conf := makeEPPGslbBasicConf(t, []string{failing.addr, backup.addr}, func(conf *cluster_conf.GslbBasicConf) {
		conf.EPPCheck = &cluster_conf.EPPCheckConf{Disabled: true}
	})
	bal := makeTestBal(t, conf)

	_, err := bal.BalanceEpp(prepareEPPRequest())
	require.Error(t, err)
	assert.Nil(t, backup.proc.capturedFirst())
}

func TestSetGslbBasicEPPHotUpdate(t *testing.T) {
	server1 := startEPPTestServer(t, true)
	server2 := startEPPTestServer(t, true)

	bal := NewBalanceGslb("cluster_demo")
	t.Cleanup(bal.Release)

	bal.SetGslbBasic(makeEPPGslbBasicConf(t, []string{server1.addr}, nil))
	rt1 := bal.getEPPRt()
	require.NotNil(t, rt1)

	// same address table: runtime is kept, only conf updated
	bal.SetGslbBasic(makeEPPGslbBasicConf(t, []string{server1.addr}, func(conf *cluster_conf.GslbBasicConf) {
		call := "1s"
		conf.EPPTimeout = &cluster_conf.EPPTimeoutConf{Call: &call}
	}))
	assert.Same(t, rt1, bal.getEPPRt())

	// changed signature field (EPPCheck.Disabled) with same address table:
	// runtime is rebuilt, not updated in place
	bal.SetGslbBasic(makeEPPGslbBasicConf(t, []string{server1.addr}, func(conf *cluster_conf.GslbBasicConf) {
		conf.EPPCheck = &cluster_conf.EPPCheckConf{Disabled: true}
	}))
	rtDisabled := bal.getEPPRt()
	require.NotNil(t, rtDisabled)
	assert.NotSame(t, rt1, rtDisabled)

	// changed address table: new runtime, old one retired
	bal.SetGslbBasic(makeEPPGslbBasicConf(t, []string{server2.addr}, nil))
	rt2 := bal.getEPPRt()
	require.NotNil(t, rt2)
	assert.NotSame(t, rt1, rt2)
	bal.eppMu.Lock()
	assert.Len(t, bal.eppRetired, 2)
	bal.eppMu.Unlock()

	// new requests go to new address
	bk, err := bal.BalanceEpp(prepareEPPRequest())
	require.NoError(t, err)
	require.NotNil(t, bk)
	assert.NotNil(t, server2.proc.capturedFirst())
	assert.Nil(t, server1.proc.capturedFirst())

	// switch to non-EPP mode closes EPP runtime
	crossRetry := 0
	retryMax := 2
	mode := cluster_conf.BalanceModeWrr
	wrrConf := cluster_conf.GslbBasicConf{
		CrossRetry:  &crossRetry,
		RetryMax:    &retryMax,
		HashConf:    &cluster_conf.HashConf{},
		BalanceMode: &mode,
	}
	require.NoError(t, cluster_conf.GslbBasicConfCheck(&wrrConf))
	bal.SetGslbBasic(wrrConf)
	assert.Nil(t, bal.getEPPRt())
}

func TestIsEPPRetryable(t *testing.T) {
	assert.True(t, isEPPRetryable(status.Error(codes.Unavailable, "connection refused")))
	assert.True(t, isEPPRetryable(status.Error(codes.Internal, "unknown inference pool: x")))
	assert.True(t, isEPPRetryable(status.Error(codes.Internal, "cell is not serving")))
	assert.False(t, isEPPRetryable(status.Error(codes.Internal, "missing inference-pool metadata")))
	assert.False(t, isEPPRetryable(status.Error(codes.InvalidArgument, "bad request")))
	assert.False(t, isEPPRetryable(fmt.Errorf("not a grpc error")))
}

// fastCheckConf enables health check with short hysteresis for tests.
func fastCheckConf(conf *cluster_conf.GslbBasicConf) {
	interval := "25ms"
	cooldown := "100ms"
	failThreshold := 2
	successThreshold := 2
	conf.EPPCheck = &cluster_conf.EPPCheckConf{
		CheckInterval:    &interval,
		FailThreshold:    &failThreshold,
		Cooldown:         &cooldown,
		SuccessThreshold: &successThreshold,
	}
}

func TestEPPRuntimeHotUpdate_TLSToPlaintext(t *testing.T) {
	server := startPlaintextEPPTestServer(t, true)

	// initial: TLS (default Insecure=true) against a plaintext server fails
	bal := makeTestBal(t, makeEPPGslbBasicConf(t, []string{server.addr}, nil))
	rt1 := bal.getEPPRt()
	require.NotNil(t, rt1)
	_, err := bal.BalanceEpp(prepareEPPRequest())
	require.Error(t, err)

	// hot update: Plaintext=true with unchanged address table -> rebuild
	bal.SetGslbBasic(makeEPPGslbBasicConf(t, []string{server.addr}, func(conf *cluster_conf.GslbBasicConf) {
		conf.EPPTLS = &cluster_conf.EPPTLSConf{Plaintext: true}
	}))
	rt2 := bal.getEPPRt()
	require.NotNil(t, rt2)
	assert.NotSame(t, rt1, rt2)
	bal.eppMu.Lock()
	assert.Len(t, bal.eppRetired, 1)
	bal.eppMu.Unlock()

	// data connection now dials plaintext
	bk, err := bal.BalanceEpp(prepareEPPRequest())
	require.NoError(t, err)
	require.NotNil(t, bk)
	assert.NotNil(t, server.proc.capturedFirst())

	// health probe dials plaintext too: direct probe succeeds
	assert.NoError(t, rt2.probe(0))
}

func TestEPPRuntimeHotUpdate_PlaintextToTLS(t *testing.T) {
	server := startEPPTestServer(t, true)

	// initial: plaintext against a TLS server fails
	bal := makeTestBal(t, makeEPPGslbBasicConf(t, []string{server.addr}, func(conf *cluster_conf.GslbBasicConf) {
		conf.EPPTLS = &cluster_conf.EPPTLSConf{Plaintext: true}
	}))
	rt1 := bal.getEPPRt()
	require.NotNil(t, rt1)
	_, err := bal.BalanceEpp(prepareEPPRequest())
	require.Error(t, err)

	// hot update: drop Plaintext (defaults to Insecure=true) -> rebuild;
	// the new runtime must build tlsConf (D2)
	bal.SetGslbBasic(makeEPPGslbBasicConf(t, []string{server.addr}, nil))
	rt2 := bal.getEPPRt()
	require.NotNil(t, rt2)
	assert.NotSame(t, rt1, rt2)
	require.NotNil(t, rt2.tlsConf, "rebuilt runtime must build tlsConf (D2)")

	// data connection and probe both work over TLS (InsecureSkipVerify)
	bk, err := bal.BalanceEpp(prepareEPPRequest())
	require.NoError(t, err)
	require.NotNil(t, bk)
	assert.NoError(t, rt2.probe(0))
}

func TestEPPRuntimeHotUpdate_ConnectTimeout(t *testing.T) {
	server := startEPPTestServer(t, true)
	connect1, connect2 := "500ms", "50ms"

	bal := makeTestBal(t, makeEPPGslbBasicConf(t, []string{server.addr}, func(conf *cluster_conf.GslbBasicConf) {
		conf.EPPTimeout = &cluster_conf.EPPTimeoutConf{Connect: &connect1}
	}))
	rt1 := bal.getEPPRt()
	require.NotNil(t, rt1)

	// hot update: EPPTimeout.Connect change -> rebuild with new timeout (D4)
	bal.SetGslbBasic(makeEPPGslbBasicConf(t, []string{server.addr}, func(conf *cluster_conf.GslbBasicConf) {
		conf.EPPTimeout = &cluster_conf.EPPTimeoutConf{Connect: &connect2}
	}))
	rt2 := bal.getEPPRt()
	require.NotNil(t, rt2)
	assert.NotSame(t, rt1, rt2)
	assert.Equal(t, 50*time.Millisecond, rt2.conf.connectTimeout)
	bal.eppMu.Lock()
	assert.Len(t, bal.eppRetired, 1)
	bal.eppMu.Unlock()

	// new connection works with the new timeout
	bk, err := bal.BalanceEpp(prepareEPPRequest())
	require.NoError(t, err)
	require.NotNil(t, bk)
	assert.NoError(t, rt2.probe(0))
}

func TestEPPRuntimeHotUpdate_CheckDisabled(t *testing.T) {
	primary := startEPPTestServer(t, false)
	backup := startEPPTestServer(t, false)
	addrs := []string{primary.addr, backup.addr}

	// health check on: primary goes down -> failover to backup
	bal := makeTestBal(t, makeEPPGslbBasicConf(t, addrs, fastCheckConf))
	rt1 := bal.getEPPRt()
	require.NotNil(t, rt1)
	primary.setServing(false)
	waitForCond(t, 5*time.Second, "failover to backup", func() bool {
		return bal.getEPPRt().activeIndex() == 1
	})

	// hot update: disable health check, same address table -> rebuild,
	// failover state inherited
	bal.SetGslbBasic(makeEPPGslbBasicConf(t, addrs, func(conf *cluster_conf.GslbBasicConf) {
		fastCheckConf(conf)
		conf.EPPCheck.Disabled = true
	}))
	rt2 := bal.getEPPRt()
	require.NotNil(t, rt2)
	assert.NotSame(t, rt1, rt2)
	assert.Equal(t, 1, rt2.activeIndex(), "failover state inherited on rebuild")

	// health loop stopped: primary recovers (with short cooldown, failback
	// would take ~150ms if probes were still running) but active must stay
	primary.setServing(true)
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, 1, bal.getEPPRt().activeIndex(), "no failback while health check disabled")

	// reverse: enabling health check on a fresh balancer starts the loop
	primary2 := startEPPTestServer(t, false)
	backup2 := startEPPTestServer(t, false)
	addrs2 := []string{primary2.addr, backup2.addr}

	bal2 := makeTestBal(t, makeEPPGslbBasicConf(t, addrs2, func(conf *cluster_conf.GslbBasicConf) {
		conf.EPPCheck = &cluster_conf.EPPCheckConf{Disabled: true}
	}))
	rt3 := bal2.getEPPRt()
	require.NotNil(t, rt3)

	bal2.SetGslbBasic(makeEPPGslbBasicConf(t, addrs2, fastCheckConf))
	rt4 := bal2.getEPPRt()
	require.NotNil(t, rt4)
	assert.NotSame(t, rt3, rt4, "enabling health check rebuilds runtime")

	// health loop started: primary failure now triggers failover
	primary2.setServing(false)
	waitForCond(t, 5*time.Second, "failover after enabling health check", func() bool {
		return bal2.getEPPRt().activeIndex() == 1
	})
}

func TestEPPRuntimeHotUpdate_ReadTypeInPlace(t *testing.T) {
	server := startEPPTestServer(t, true)
	addrs := []string{server.addr}

	bal := makeTestBal(t, makeEPPGslbBasicConf(t, addrs, nil))
	rt1 := bal.getEPPRt()
	require.NotNil(t, rt1)

	// read-type parameter changes only: runtime must be reused in place
	call, interval := "1s", "50ms"
	failThreshold, successThreshold := 3, 2
	cooldown := "30s"
	bal.SetGslbBasic(makeEPPGslbBasicConf(t, addrs, func(conf *cluster_conf.GslbBasicConf) {
		conf.EPPTimeout = &cluster_conf.EPPTimeoutConf{Call: &call}
		conf.EPPCheck = &cluster_conf.EPPCheckConf{
			CheckInterval:    &interval,
			FailThreshold:    &failThreshold,
			Cooldown:         &cooldown,
			SuccessThreshold: &successThreshold,
		}
	}))
	assert.Same(t, rt1, bal.getEPPRt(), "read-type change must not rebuild runtime")

	rt1.mu.Lock()
	assert.Equal(t, time.Second, rt1.conf.callTimeout)
	assert.Equal(t, 50*time.Millisecond, rt1.conf.checkInterval)
	assert.Equal(t, 3, rt1.conf.failThreshold)
	assert.Equal(t, 30*time.Second, rt1.conf.cooldown)
	assert.Equal(t, 2, rt1.conf.successThreshold)
	rt1.mu.Unlock()

	// service still works with updated parameters
	bk, err := bal.BalanceEpp(prepareEPPRequest())
	require.NoError(t, err)
	require.NotNil(t, bk)
}

func TestEPPRuntimeHotUpdate_AddrChange(t *testing.T) {
	primary := startEPPTestServer(t, false)
	backup := startEPPTestServer(t, false)
	server2 := startEPPTestServer(t, true)

	oldGrace := eppRetireGrace
	eppRetireGrace = 100 * time.Millisecond
	t.Cleanup(func() { eppRetireGrace = oldGrace })

	addrs := []string{primary.addr, backup.addr}
	bal := makeTestBal(t, makeEPPGslbBasicConf(t, addrs, fastCheckConf))
	rt1 := bal.getEPPRt()
	require.NotNil(t, rt1)

	// establish failover state on the old runtime
	primary.setServing(false)
	waitForCond(t, 5*time.Second, "failover to backup", func() bool {
		return bal.getEPPRt().activeIndex() == 1
	})

	// address table change: rebuild, failover state NOT inherited
	bal.SetGslbBasic(makeEPPGslbBasicConf(t, []string{server2.addr}, nil))
	rt2 := bal.getEPPRt()
	require.NotNil(t, rt2)
	assert.NotSame(t, rt1, rt2)
	assert.Equal(t, 0, rt2.activeIndex(), "active resets on address table change")

	bal.eppMu.Lock()
	require.Len(t, bal.eppRetired, 1)
	retired := bal.eppRetired[0]
	bal.eppMu.Unlock()

	// retired runtime's connections are closed after the shortened grace
	waitForCond(t, 5*time.Second, "retired runtime conns closed", func() bool {
		return atomic.LoadInt32(&retired.closed) == 1
	})

	// new requests go to the new address
	bk, err := bal.BalanceEpp(prepareEPPRequest())
	require.NoError(t, err)
	require.NotNil(t, bk)
	assert.NotNil(t, server2.proc.capturedFirst())
}
