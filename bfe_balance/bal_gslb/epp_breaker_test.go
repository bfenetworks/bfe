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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
)

func testBreakerConf(mutate func(*eppBreakerConf)) eppBreakerConf {
	conf := eppBreakerConf{
		windowSize:       4,
		minVolume:        2,
		errorRatePercent: 50,
		openTimeout:      100 * time.Millisecond,
	}
	if mutate != nil {
		mutate(&conf)
	}
	return conf
}

func TestEppBreakerOpensOnHighErrorRate(t *testing.T) {
	b := newEppBreaker("c", testBreakerConf(nil))

	assert.True(t, b.allow())
	b.record(false)
	assert.True(t, b.allow()) // minVolume (2) not reached yet
	b.record(false)

	// 2/2 failures, rate 100% >= 50% -> OPEN
	assert.False(t, b.allow())
	assert.False(t, b.allow())
}

func TestEppBreakerMinVolumeAndRate(t *testing.T) {
	// minVolume gates evaluation
	b := newEppBreaker("c", testBreakerConf(func(c *eppBreakerConf) {
		c.minVolume = 3
	}))
	b.record(false)
	b.record(false)
	assert.True(t, b.allow()) // 2 < 3, still closed

	// rate below threshold: 1/3 errors (33%) < 50%
	b2 := newEppBreaker("c", testBreakerConf(func(c *eppBreakerConf) {
		c.minVolume = 3
	}))
	b2.record(false)
	b2.record(true)
	b2.record(true)
	assert.True(t, b2.allow())

	// windowSize caps the window: old results slide out
	b3 := newEppBreaker("c", testBreakerConf(func(c *eppBreakerConf) {
		c.windowSize = 3
		c.minVolume = 3
		c.errorRatePercent = 60
	}))
	// 2/3 errors = 66% >= 60% -> open at window full
	b3.record(false)
	b3.record(false)
	b3.record(true)
	assert.False(t, b3.allow())
}

func TestEppBreakerHalfOpenRecovery(t *testing.T) {
	b := newEppBreaker("c", testBreakerConf(nil))
	b.record(false)
	b.record(false)
	require.False(t, b.allow()) // OPEN

	// within cooldown: still blocked
	time.Sleep(30 * time.Millisecond)
	assert.False(t, b.allow())

	// after cooldown: half-open probe allowed (single probe at a time)
	time.Sleep(80 * time.Millisecond)
	assert.True(t, b.allow())
	assert.False(t, b.allow()) // second concurrent probe blocked

	// probe succeeds -> CLOSED, window cleared
	b.record(true)
	assert.True(t, b.allow())
	assert.True(t, b.allow())
}

func TestEppBreakerHalfOpenFailureReopens(t *testing.T) {
	b := newEppBreaker("c", testBreakerConf(nil))
	b.record(false)
	b.record(false)
	require.False(t, b.allow())

	time.Sleep(120 * time.Millisecond)
	assert.True(t, b.allow()) // half-open probe
	b.record(false)           // probe fails -> OPEN again
	assert.False(t, b.allow())

	// and it can recover again in the next cycle
	time.Sleep(120 * time.Millisecond)
	assert.True(t, b.allow())
	b.record(true)
	assert.True(t, b.allow())
}

func TestEppBreakerDisabled(t *testing.T) {
	b := newEppBreaker("c", testBreakerConf(func(c *eppBreakerConf) {
		c.disabled = true
	}))
	for i := 0; i < 10; i++ {
		assert.True(t, b.allow())
		b.record(false)
	}
	assert.True(t, b.allow())
}

func TestEppBreakerUpdateConf(t *testing.T) {
	b := newEppBreaker("c", testBreakerConf(nil))
	b.record(false)
	b.record(false)
	require.False(t, b.allow())

	// disable via update: breaker stops blocking
	b.updateConf(testBreakerConf(func(c *eppBreakerConf) { c.disabled = true }))
	assert.True(t, b.allow())

	// re-enable: state and cooldown are kept (config reload does not reset
	// an open breaker), so it still blocks until the next half-open cycle
	b.updateConf(testBreakerConf(nil))
	assert.False(t, b.allow())
	time.Sleep(120 * time.Millisecond)
	assert.True(t, b.allow())
	b.record(true)
	assert.True(t, b.allow())
}

func TestEPPBreakerIntegration(t *testing.T) {
	server := startEPPTestServer(t, true)

	conf := makeEPPGslbBasicConf(t, []string{server.addr}, func(c *cluster_conf.GslbBasicConf) {
		c.EPPCheck = &cluster_conf.EPPCheckConf{Disabled: true}
		window, minV, rate := 4, 2, 50
		openT := "100ms"
		c.EPPBreaker = &cluster_conf.EPPBreakerConf{
			WindowSize:       &window,
			MinVolume:        &minV,
			ErrorRatePercent: &rate,
			OpenTimeout:      &openT,
		}
	})
	bal := makeTestBal(t, conf)

	// failing EPP calls: 2/2 >= minVolume with 100% error rate -> OPEN
	server.proc.setProcErr(status.Error(codes.Unavailable, "connection refused"))
	for i := 0; i < 2; i++ {
		_, err := bal.BalanceEpp(prepareEPPRequest())
		require.Error(t, err)
	}
	callsWhenOpen := server.proc.callCount()

	// OPEN: BalanceEpp short-circuits, no EPP call is made
	_, err := bal.BalanceEpp(prepareEPPRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "breaker")
	assert.Equal(t, callsWhenOpen, server.proc.callCount())

	// cooldown elapsed -> half-open probe allowed; success closes breaker
	time.Sleep(150 * time.Millisecond)
	server.proc.setProcErr(nil)
	bk, err := bal.BalanceEpp(prepareEPPRequest())
	require.NoError(t, err)
	require.NotNil(t, bk)

	// closed again: normal calls flow to EPP
	before := server.proc.callCount()
	_, err = bal.BalanceEpp(prepareEPPRequest())
	require.NoError(t, err)
	assert.Equal(t, before+1, server.proc.callCount())

	// failing again reopens the breaker
	server.proc.setProcErr(status.Error(codes.Internal, "unknown inference pool: cluster_demo"))
	for i := 0; i < 2; i++ {
		_, err = bal.BalanceEpp(prepareEPPRequest())
		require.Error(t, err)
	}
	callsWhenOpen = server.proc.callCount()
	_, err = bal.BalanceEpp(prepareEPPRequest())
	require.Error(t, err)
	assert.Equal(t, callsWhenOpen, server.proc.callCount())
}

func TestClassifyEPPError(t *testing.T) {
	assert.Equal(t, eppResultOK, classifyEPPError(nil))
	assert.Equal(t, eppResultNoPool, classifyEPPError(
		status.Error(codes.Internal, "missing inference-pool metadata")))
	assert.Equal(t, eppResultUnknownPool, classifyEPPError(
		status.Error(codes.Internal, "unknown inference pool: x")))
	assert.Equal(t, eppResultDraining, classifyEPPError(
		status.Error(codes.Unavailable, "cell is not serving")))
	assert.Equal(t, eppResultDraining, classifyEPPError(
		status.Error(codes.Unavailable, "cell draining")))
	assert.Equal(t, eppResultTransport, classifyEPPError(
		status.Error(codes.Unavailable, "connection refused")))
	assert.Equal(t, eppResultTransport, classifyEPPError(
		status.Error(codes.DeadlineExceeded, "deadline")))
	assert.Equal(t, eppResultTransport, classifyEPPError(assert.AnError))
}

func TestEppMetricsText(t *testing.T) {
	recordEPPCall("metrics-cluster", nil)
	recordEPPCall("metrics-cluster", status.Error(codes.Unavailable, "connection refused"))
	recordEPPFallbackLocal("metrics-cluster")

	text, err := EppMetricsText()
	require.NoError(t, err)
	output := string(text)
	assert.Contains(t, output, "epp_calls_total")
	assert.Contains(t, output, `cluster="metrics-cluster"`)
	assert.Contains(t, output, `result="ok"`)
	assert.Contains(t, output, `result="transport"`)
	assert.Contains(t, output, "epp_fallback_local_total")
	assert.Contains(t, output, "epp_active_addr_index")
}
