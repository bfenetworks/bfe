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

package mod_ai_cache

import (
	"bytes"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// PrometheusStates exposes the mod_ai_cache counters in prometheus text
// format. Gauges are re-set from the module counters on each scrape.
type PrometheusStates struct {
	registry *prometheus.Registry

	reqTotal      prometheus.Gauge
	cacheHit      prometheus.Gauge
	cacheMiss     prometheus.Gauge
	cacheSkip     prometheus.Gauge
	redisErr      prometheus.Gauge
	latencyMs     prometheus.Gauge
	valueTooLarge prometheus.Gauge
}

func newPrometheusStates() *PrometheusStates {
	ret := &PrometheusStates{}
	ret.registry = prometheus.NewRegistry()

	newGauge := func(name, help string) prometheus.Gauge {
		g := prometheus.NewGauge(prometheus.GaugeOpts{Name: name, Help: help})
		ret.registry.MustRegister(g)
		return g
	}

	ret.reqTotal = newGauge("req_total", "mod_ai_cache requests entering the module")
	ret.cacheHit = newGauge("cache_hit", "mod_ai_cache cache hits")
	ret.cacheMiss = newGauge("cache_miss", "mod_ai_cache cache misses")
	ret.cacheSkip = newGauge("cache_skip", "mod_ai_cache requests skipped by header or strategy")
	ret.redisErr = newGauge("redis_err", "mod_ai_cache redis errors")
	ret.latencyMs = newGauge("latency_ms", "mod_ai_cache redis latency sum in milliseconds")
	ret.valueTooLarge = newGauge("value_too_large", "mod_ai_cache answers dropped by maxValueBytes")

	return ret
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
