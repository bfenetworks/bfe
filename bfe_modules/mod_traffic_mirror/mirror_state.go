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

package mod_traffic_mirror

import (
	"bytes"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// PrometheusStates exposes the mod_traffic_mirror metrics in prometheus text
// format through the module-private registry. Flat counters are re-set from
// the module counters on each scrape; labeled vectors and histograms are
// updated live by the sender.
type PrometheusStates struct {
	registry *prometheus.Registry

	// gauges re-set from module counters on scrape
	reqTotal      prometheus.Gauge
	submitDrop    prometheus.Gauge
	skipSample    prometheus.Gauge
	skipBodyLimit prometheus.Gauge
	skipRewrite   prometheus.Gauge
	sendFail      prometheus.Gauge

	// labeled vectors updated live
	reqTotalLabeled    *prometheus.CounterVec // product, cluster, model
	skipTotal          *prometheus.CounterVec // reason
	respStatusTotal    *prometheus.CounterVec // cluster, status
	errorTypeTotal     *prometheus.CounterVec // cluster, type, code
	finishReasonTotal  *prometheus.CounterVec // cluster, reason
	tokensTotal        *prometheus.CounterVec // cluster, kind
	failTotal          *prometheus.CounterVec // cluster, reason
	circuitOpenTotal   *prometheus.CounterVec // cluster
	respTruncatedTotal *prometheus.CounterVec // cluster
	inflight           *prometheus.GaugeVec   // cluster

	// histograms
	ttfbMs    *prometheus.HistogramVec // cluster
	latencyMs *prometheus.HistogramVec // cluster
}

func newPrometheusStates() *PrometheusStates {
	ret := &PrometheusStates{}
	ret.registry = prometheus.NewRegistry()

	newGauge := func(name, help string) prometheus.Gauge {
		g := prometheus.NewGauge(prometheus.GaugeOpts{Name: name, Help: help})
		ret.registry.MustRegister(g)
		return g
	}
	newCounterVec := func(name, help string, labels []string) *prometheus.CounterVec {
		c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
		ret.registry.MustRegister(c)
		return c
	}

	ret.reqTotal = newGauge("req_total", "requests submitted for mirroring")
	ret.submitDrop = newGauge("submit_drop_total", "mirror tasks dropped because queue is full")
	ret.skipSample = newGauge("skip_sample_total", "requests not selected by percentage sampling")
	ret.skipBodyLimit = newGauge("skip_body_limit_total", "requests skipped due to body size limit")
	ret.skipRewrite = newGauge("skip_rewrite_total", "body rewrites failed (mirrored with original body)")
	ret.sendFail = newGauge("send_fail_total", "mirror send/drain failures")

	ret.reqTotalLabeled = newCounterVec("req_labeled_total",
		"requests submitted for mirroring (labeled)", []string{"product", "cluster", "model"})
	ret.skipTotal = newCounterVec("skip_total",
		"requests skipped by mirror rule (labeled)", []string{"reason"})
	ret.respStatusTotal = newCounterVec("resp_status_total",
		"mirror response status code distribution", []string{"cluster", "status"})
	ret.errorTypeTotal = newCounterVec("error_type_total",
		"OpenAI error type/code parsed from mirror responses",
		[]string{"cluster", "type", "code"})
	ret.finishReasonTotal = newCounterVec("finish_reason_total",
		"finish_reason parsed from mirror responses", []string{"cluster", "reason"})
	ret.tokensTotal = newCounterVec("tokens_total",
		"mirror token consumption", []string{"cluster", "kind"})
	ret.failTotal = newCounterVec("fail_total",
		"mirror send/drain failures (labeled)", []string{"cluster", "reason"})
	ret.circuitOpenTotal = newCounterVec("circuit_open_total",
		"mirror tasks dropped because circuit breaker is open (labeled)", []string{"cluster"})
	ret.respTruncatedTotal = newCounterVec("resp_truncated_total",
		"mirror responses truncated by drain limits (labeled)", []string{"cluster"})
	ret.inflight = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "inflight", Help: "current mirror in-flight requests"}, []string{"cluster"})
	ret.registry.MustRegister(ret.inflight)

	buckets := prometheus.ExponentialBuckets(10, 2, 15) // 10ms .. ~164s
	ret.ttfbMs = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "ttfb_ms", Help: "mirror time to first byte (ms)",
		Buckets: buckets}, []string{"cluster"})
	ret.latencyMs = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "latency_ms", Help: "mirror request total latency (ms)",
		Buckets: buckets}, []string{"cluster"})
	ret.registry.MustRegister(ret.ttfbMs, ret.latencyMs)

	return ret
}

// reportRequest records a submitted mirror task
func (ps *PrometheusStates) reportRequest(product, cluster, model string) {
	ps.reqTotalLabeled.WithLabelValues(product, cluster, model).Inc()
}

// reportSkip records a skipped request with reason
func (ps *PrometheusStates) reportSkip(reason string) {
	ps.skipTotal.WithLabelValues(reason).Inc()
}

// reportResponse records a drained mirror response
func (ps *PrometheusStates) reportResponse(cluster string, status int, result *mirrorRespResult) {
	ps.respStatusTotal.WithLabelValues(cluster, strconv.Itoa(status)).Inc()
	if result == nil {
		return
	}
	if result.FinishReason != "" {
		ps.finishReasonTotal.WithLabelValues(cluster, result.FinishReason).Inc()
	}
	if result.ErrorType != "" || result.ErrorCode != "" {
		ps.errorTypeTotal.WithLabelValues(cluster, result.ErrorType, result.ErrorCode).Inc()
	}
	if result.PromptTokens > 0 {
		ps.tokensTotal.WithLabelValues(cluster, "prompt").Add(float64(result.PromptTokens))
	}
	if result.CompletionTokens > 0 {
		ps.tokensTotal.WithLabelValues(cluster, "completion").Add(float64(result.CompletionTokens))
	}
}

// reportFailure records a mirror send/drain failure
func (ps *PrometheusStates) reportFailure(cluster, reason string) {
	ps.failTotal.WithLabelValues(cluster, reason).Inc()
}

// reportCircuitOpen records a task dropped by the circuit breaker
func (ps *PrometheusStates) reportCircuitOpen(cluster string) {
	ps.circuitOpenTotal.WithLabelValues(cluster).Inc()
}

// reportTruncated records a response truncated by drain limits
func (ps *PrometheusStates) reportTruncated(cluster string) {
	ps.respTruncatedTotal.WithLabelValues(cluster).Inc()
}

// incInflight/decInflight track concurrent mirror requests
func (ps *PrometheusStates) incInflight(cluster string) {
	ps.inflight.WithLabelValues(cluster).Inc()
}

func (ps *PrometheusStates) decInflight(cluster string) {
	ps.inflight.WithLabelValues(cluster).Dec()
}

// reportLatency records TTFB and total latency in milliseconds
func (ps *PrometheusStates) reportLatency(cluster string, ttfb time.Duration, total time.Duration) {
	if ttfb > 0 {
		ps.ttfbMs.WithLabelValues(cluster).Observe(float64(ttfb.Milliseconds()))
	}
	if total > 0 {
		ps.latencyMs.WithLabelValues(cluster).Observe(float64(total.Milliseconds()))
	}
}

func (ps *PrometheusStates) toString() ([]byte, error) {
	metricFamilies, err := ps.registry.Gather()
	if err != nil {
		return []byte(""), err
	}

	var buf bytes.Buffer
	encoder := expfmt.NewEncoder(&buf, expfmt.FmtText)
	for _, mf := range metricFamilies {
		encoder.Encode(mf)
	}

	return buf.Bytes(), nil
}
