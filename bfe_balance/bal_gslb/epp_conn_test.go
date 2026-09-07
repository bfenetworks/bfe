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
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/stats"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
)

// connStatsHandler counts server-side connections and RPCs (streams).
type connStatsHandler struct {
	mu    sync.Mutex
	conns int
	rpcs  int
}

func (h *connStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	return ctx
}

func (h *connStatsHandler) HandleRPC(ctx context.Context, s stats.RPCStats) {
	if _, ok := s.(*stats.Begin); ok {
		h.mu.Lock()
		h.rpcs++
		h.mu.Unlock()
	}
}

func (h *connStatsHandler) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	return ctx
}

func (h *connStatsHandler) HandleConn(ctx context.Context, s stats.ConnStats) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch s.(type) {
	case *stats.ConnBegin:
		h.conns++
	case *stats.ConnEnd:
		h.conns--
	}
}

func (h *connStatsHandler) counts() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.conns, h.rpcs
}

type statsServer struct {
	addr  string
	proc  *fakeExtProc
	grpc  *grpc.Server
	stats *connStatsHandler
}

// startStatsServer starts a TLS EPP test server with connection/stream stats.
// Health check is not registered: tests using this server disable probes.
func startStatsServer(t *testing.T) (*statsServer, *connStatsHandler) {
	t.Helper()

	cert := testServerTLSCert(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	h := &connStatsHandler{}
	srv := grpc.NewServer(
		grpc.Creds(credentials.NewServerTLSFromCert(&cert)),
		grpc.StatsHandler(h),
	)
	proc := &fakeExtProc{md: decisionMD("127.0.0.1:9999")}
	extprocv3.RegisterExternalProcessorServer(srv, proc)

	ts := &statsServer{addr: lis.Addr().String(), proc: proc, grpc: srv, stats: h}
	go srv.Serve(lis)
	t.Cleanup(func() {
		srv.Stop()
		lis.Close()
	})
	return ts, h
}

func TestEPPConnReuse(t *testing.T) {
	server, h := startStatsServer(t)

	// health check disabled: only the data connection may exist
	conf := makeEPPGslbBasicConf(t, []string{server.addr}, func(c *cluster_conf.GslbBasicConf) {
		c.EPPCheck = &cluster_conf.EPPCheckConf{Disabled: true}
	})
	bal := makeTestBal(t, conf)

	// multiple BalanceEpp calls: 1 connection, N streams
	const calls = 5
	for i := 0; i < calls; i++ {
		bk, err := bal.BalanceEpp(prepareEPPRequest())
		require.NoError(t, err)
		require.NotNil(t, bk)
	}

	conns, rpcs := h.counts()
	assert.Equal(t, 1, conns, "all streams should share one connection")
	assert.Equal(t, calls, rpcs, "one stream per request")
}

func TestEPPConnCloseOnAddrSwitch(t *testing.T) {
	// shorten retire grace so the old connection closes quickly
	oldGrace := eppRetireGrace
	eppRetireGrace = 200 * time.Millisecond
	t.Cleanup(func() { eppRetireGrace = oldGrace })

	server1, h1 := startStatsServer(t)
	server2, h2 := startStatsServer(t)

	conf := makeEPPGslbBasicConf(t, []string{server1.addr}, func(c *cluster_conf.GslbBasicConf) {
		c.EPPCheck = &cluster_conf.EPPCheckConf{Disabled: true}
	})
	bal := makeTestBal(t, conf)

	_, err := bal.BalanceEpp(prepareEPPRequest())
	require.NoError(t, err)
	conns1, _ := h1.counts()
	require.Equal(t, 1, conns1)

	// switch to another address: new conn is used, old one lives during grace
	bal.SetGslbBasic(makeEPPGslbBasicConf(t, []string{server2.addr}, func(c *cluster_conf.GslbBasicConf) {
		c.EPPCheck = &cluster_conf.EPPCheckConf{Disabled: true}
	}))
	_, err = bal.BalanceEpp(prepareEPPRequest())
	require.NoError(t, err)

	conns2, _ := h2.counts()
	assert.Equal(t, 1, conns2)

	// after grace, old connection is closed
	waitForCond(t, 5*time.Second, "old connection closed", func() bool {
		c, _ := h1.counts()
		return c == 0
	})
}
