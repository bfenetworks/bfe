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
	"strconv"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_http"
)

// eppImmediateResponse builds an Admission-refusal ImmediateResponse.
func eppImmediateResponse(code int, headers map[string]string, body string, details string) *extprocv3.ImmediateResponse {
	ir := &extprocv3.ImmediateResponse{
		Status:  &typev3.HttpStatus{Code: typev3.StatusCode(code)},
		Body:    []byte(body),
		Details: details,
	}
	if len(headers) > 0 {
		mutation := &extprocv3.HeaderMutation{}
		for k, v := range headers {
			mutation.SetHeaders = append(mutation.SetHeaders, &corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{Key: k, RawValue: []byte(v)},
			})
		}
		ir.Headers = mutation
	}
	return ir
}

// TestNewEppRejected locks the parsing contract of issue #1482: only 429/503
// are admission decisions, header mutation carries Retry-After / x-error-code,
// and the body is truncated for logs.
func TestNewEppRejected(t *testing.T) {
	// nil response: no decision.
	assert.Nil(t, newEppRejected(nil))

	// 429 with hints.
	rej := newEppRejected(eppImmediateResponse(bfe_http.StatusTooManyRequests,
		map[string]string{"Retry-After": "3", "x-error-code": "QUEUE_TTL_EXPIRED"},
		"pool saturated", ""))
	require.NotNil(t, rej)
	assert.Equal(t, bfe_http.StatusTooManyRequests, rej.StatusCode)
	assert.Equal(t, "3", rej.RetryAfter)
	assert.Equal(t, "QUEUE_TTL_EXPIRED", rej.Code)
	assert.Equal(t, "pool saturated", rej.Body)
	assert.True(t, IsEPPRejected(rej))
	assert.False(t, isEPPRetryable(rej))

	// 503 is the "no capacity" decision.
	rej = newEppRejected(eppImmediateResponse(bfe_http.StatusServiceUnavailable, nil, "", "no endpoint"))
	require.NotNil(t, rej)
	assert.Equal(t, bfe_http.StatusServiceUnavailable, rej.StatusCode)
	assert.Equal(t, "no endpoint", rej.Code, "details is used as the code when no header carries one")

	// Any other status stays on the transport path.
	assert.Nil(t, newEppRejected(eppImmediateResponse(bfe_http.StatusInternalServerError, nil, "boom", "")))
	assert.Nil(t, newEppRejected(eppImmediateResponse(bfe_http.StatusForbidden, nil, "nope", "")))

	// Missing status defaults to 429.
	rej = newEppRejected(&extprocv3.ImmediateResponse{Body: []byte("x")})
	require.NotNil(t, rej)
	assert.Equal(t, bfe_http.StatusTooManyRequests, rej.StatusCode)

	// Body is truncated; long bodies must not flood the log.
	long := string(make([]byte, eppRejectMaxBodyLen+10))
	rej = newEppRejected(eppImmediateResponse(bfe_http.StatusTooManyRequests, nil, long, ""))
	require.NotNil(t, rej)
	assert.LessOrEqual(t, len(rej.Body), eppRejectMaxBodyLen+3)
}

// TestBalanceEppRejectedNoRetryNoFallback verifies the core decision of issue
// #1482: a rejection is not retried on the next EPP address, does not fall back
// to local balance, does not count as a failure, and is recorded on the request
// for the failure response.
func TestBalanceEppRejectedNoRetryNoFallback(t *testing.T) {
	fallbackBefore := eppMetricValue(t, `epp_fallback_local_total{cluster="cluster_demo"}`)
	rejectedBefore := eppMetricValue(t, `epp_rejected_total{cluster="cluster_demo",status="429"}`)

	primary := startEPPTestServer(t, true)
	primary.proc.setImmediateResponse(eppImmediateResponse(bfe_http.StatusTooManyRequests,
		map[string]string{"Retry-After": "2"}, "too many", ""), false)
	backup := startEPPTestServer(t, true)

	conf := makeEPPGslbBasicConf(t, []string{primary.addr, backup.addr}, func(conf *cluster_conf.GslbBasicConf) {
		conf.EPPCheck = &cluster_conf.EPPCheckConf{Disabled: true}
	})
	bal := makeTestBal(t, conf)

	req := prepareEPPRequest()
	bk, err := bal.BalanceEpp(req)
	require.Error(t, err)
	assert.Nil(t, bk)
	assert.True(t, IsEPPRejected(err), "the error must keep the decision type")
	assert.Equal(t, bfe_basic.ErrEppRejected, req.ErrCode)

	detail := EPPRejectedDetail(req)
	require.NotNil(t, detail)
	assert.Equal(t, bfe_http.StatusTooManyRequests, detail.StatusCode)
	assert.Equal(t, "2", detail.RetryAfter)

	// No retry on the backup address: a decision would just repeat.
	assert.NotNil(t, primary.proc.capturedFirst())
	assert.Nil(t, backup.proc.capturedFirst())

	// Not a fallback: the labeled registry keeps the decision separate from
	// EPP failures (BalErrState counters are only wired by the server status
	// page, so the assertion goes through the Prometheus text). Values are
	// compared as deltas because the registry is package-global.
	assert.InDelta(t, fallbackBefore, eppMetricValue(t, `epp_fallback_local_total{cluster="cluster_demo"}`), 0.0001,
		"a rejection must not be recorded as a local fallback")
	assert.InDelta(t, rejectedBefore+1, eppMetricValue(t, `epp_rejected_total{cluster="cluster_demo",status="429"}`), 0.0001)
	metricsText, metricsErr := EppMetricsText()
	require.NoError(t, metricsErr)
	assert.Contains(t, string(metricsText), `epp_calls_total{cluster="cluster_demo",result="rejected"}`)
}

// eppMetricValue returns the value of one labeled series in the EPP metrics
// text, 0 when the series is absent.
func eppMetricValue(t *testing.T, series string) float64 {
	t.Helper()

	text, err := EppMetricsText()
	require.NoError(t, err)
	for _, line := range strings.Split(string(text), "\n") {
		if !strings.HasPrefix(line, series+" ") {
			continue
		}
		value, convErr := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, series)), 64)
		require.NoError(t, convErr)
		return value
	}
	return 0
}

// TestBalanceEppRejectedOnBodyRound verifies a rejection that arrives after the
// EPP has read the request body (the second round trip) is honoured too.
func TestBalanceEppRejectedOnBodyRound(t *testing.T) {
	server := startEPPTestServer(t, true)
	server.proc.setImmediateResponse(eppImmediateResponse(bfe_http.StatusServiceUnavailable,
		nil, "no capacity", ""), true)

	conf := makeEPPGslbBasicConf(t, []string{server.addr}, func(conf *cluster_conf.GslbBasicConf) {
		conf.EPPCheck = &cluster_conf.EPPCheckConf{Disabled: true}
	})
	bal := makeTestBal(t, conf)

	req := prepareEPPRequest()
	req.OutRequest.ContentLength = 8
	req.HttpRequest = &bfe_http.Request{ContentLength: 8}

	_, err := bal.BalanceEpp(req)
	require.Error(t, err)
	assert.True(t, IsEPPRejected(err))
	detail := EPPRejectedDetail(req)
	require.NotNil(t, detail)
	assert.Equal(t, bfe_http.StatusServiceUnavailable, detail.StatusCode)
}

// TestBalanceEppRejectedKeepsBreakerClosed verifies rejections never push the
// EPP circuit breaker open: doing so would degrade the pool to local balancing,
// which is exactly the loophole issue #1482 closes.
func TestBalanceEppRejectedKeepsBreakerClosed(t *testing.T) {
	server := startEPPTestServer(t, true)
	server.proc.setImmediateResponse(eppImmediateResponse(bfe_http.StatusTooManyRequests, nil, "saturated", ""), false)

	conf := makeEPPGslbBasicConf(t, []string{server.addr}, func(conf *cluster_conf.GslbBasicConf) {
		conf.EPPCheck = &cluster_conf.EPPCheckConf{Disabled: true}
		windowSize, minVolume, errorRate := 10, 3, 50
		openTimeout := "1s"
		conf.EPPBreaker = &cluster_conf.EPPBreakerConf{
			WindowSize:       &windowSize,
			MinVolume:        &minVolume,
			ErrorRatePercent: &errorRate,
			OpenTimeout:      &openTimeout,
		}
	})
	bal := makeTestBal(t, conf)

	for i := 0; i < 6; i++ {
		_, err := bal.BalanceEpp(prepareEPPRequest())
		require.Error(t, err)
		require.True(t, IsEPPRejected(err))
	}

	bal.eppMu.Lock()
	breaker := bal.eppBreaker
	bal.eppMu.Unlock()
	require.NotNil(t, breaker)
	assert.True(t, breaker.allow(), "rejections must not open the EPP breaker")
	assert.Equal(t, 6, server.proc.callCount(), "every request must reach the EPP")
}
