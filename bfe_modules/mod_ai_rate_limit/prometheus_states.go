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

package mod_ai_rate_limit

import (
	"bytes"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

type PrometheusStates struct {
	registry *prometheus.Registry

	tpmMatchTotal prometheus.Gauge
	tpmMatchVec   *prometheus.CounterVec
	tpmHitTotal   prometheus.Gauge
	tpmHitVec     *prometheus.CounterVec

	tpmTokenTotal prometheus.Gauge
	tpmTokenVec   *prometheus.CounterVec

	rpmMatchTotal prometheus.Gauge
	rpmMatchVec   *prometheus.CounterVec
	rpmHitTotal   prometheus.Gauge
	rpmHitVec     *prometheus.CounterVec

	conMatchTotal prometheus.Gauge
	conMatchVec   *prometheus.CounterVec
	conHitTotal   prometheus.Gauge
	conHitVec     *prometheus.CounterVec
}

func newPrometheusState() *PrometheusStates {
	ret := &PrometheusStates{}
	ret.registry = prometheus.NewRegistry()

	ret.tpmMatchTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "tpm_match_total",
			Help: "ai rate limit tpm match total",
		})
	ret.registry.MustRegister(ret.tpmMatchTotal)

	ret.tpmMatchVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tpm_match",
			Help: "ai rate limit tpm match by policy and inst",
		},
		[]string{"policy_id", "inst_id"},
	)
	ret.registry.MustRegister(ret.tpmMatchVec)

	ret.tpmHitTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "tpm_hit_total",
			Help: "ai rate limit tpm hit total",
		})
	ret.registry.MustRegister(ret.tpmHitTotal)

	ret.tpmHitVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tpm_hit",
			Help: "ai rate limit tpm hit by policy and inst",
		},
		[]string{"policy_id", "inst_id"},
	)
	ret.registry.MustRegister(ret.tpmHitVec)

	ret.tpmTokenTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "tpm_token_total",
			Help: "ai rate limit tpm token total",
		})
	ret.registry.MustRegister(ret.tpmTokenTotal)

	ret.tpmTokenVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tpm_token",
			Help: "ai rate limit tpm token by policy and inst",
		},
		[]string{"policy_id", "inst_id"},
	)
	ret.registry.MustRegister(ret.tpmTokenVec)

	ret.rpmMatchTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "rpm_match_total",
			Help: "ai rate limit rpm match total",
		})
	ret.registry.MustRegister(ret.rpmMatchTotal)

	ret.rpmMatchVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpm_match",
			Help: "ai rate limit rpm match by policy and inst",
		},
		[]string{"policy_id", "inst_id"},
	)
	ret.registry.MustRegister(ret.rpmMatchVec)

	ret.rpmHitTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "rpm_hit_total",
			Help: "ai rate limit rpm hit total",
		})
	ret.registry.MustRegister(ret.rpmHitTotal)

	ret.rpmHitVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpm_hit",
			Help: "ai rate limit rpm hit by policy and inst",
		},
		[]string{"policy_id", "inst_id"},
	)
	ret.registry.MustRegister(ret.rpmHitVec)

	ret.conMatchTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "con_match_total",
			Help: "ai rate limit concurrency match total",
		})
	ret.registry.MustRegister(ret.conMatchTotal)

	ret.conMatchVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "con_match",
			Help: "ai rate limit concurrency match by policy and inst",
		},
		[]string{"policy_id", "inst_id"},
	)
	ret.registry.MustRegister(ret.conMatchVec)

	ret.conHitTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "con_hit_total",
			Help: "ai rate limit concurrency hit total",
		})
	ret.registry.MustRegister(ret.conHitTotal)

	ret.conHitVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "con_hit",
			Help: "ai rate limit concurrency hit by policy and inst",
		},
		[]string{"policy_id", "inst_id"},
	)
	ret.registry.MustRegister(ret.conHitVec)

	return ret
}

func (ps *PrometheusStates) resetVec() {
	ps.tpmMatchVec.Reset()
	ps.tpmHitVec.Reset()
	ps.tpmTokenVec.Reset()
	ps.rpmMatchVec.Reset()
	ps.rpmHitVec.Reset()
	ps.conMatchVec.Reset()
	ps.conHitVec.Reset()
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
