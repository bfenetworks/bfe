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

// Labeled EPP metrics (cluster dimension), exposed via the monitor endpoint
// "/monitor/epp_metrics" in prometheus text format. Flat counters for the
// status page live in BalErrState (bal_gslb.go) and EppState (epp package).

package bal_gslb

import (
	"bytes"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EPP call result categories (label value of epp_calls result).
const (
	eppResultOK          = "ok"
	eppResultNoPool      = "no_pool"      // missing inference-pool metadata (BFE defect)
	eppResultUnknownPool = "unknown_pool" // EPP has no cell of this cluster
	eppResultDraining    = "draining"     // cell not serving (draining)
	eppResultTransport   = "transport"    // connection / timeout / other errors
)

// eppPromState holds labeled EPP metrics.
type eppPromState struct {
	registry *prometheus.Registry

	callsTotal         *prometheus.CounterVec // {cluster, result}
	failoverTotal      *prometheus.CounterVec // {cluster}
	failbackTotal      *prometheus.CounterVec // {cluster}
	fallbackLocalTotal *prometheus.CounterVec // {cluster}
	breakerTransitions *prometheus.CounterVec // {cluster, state}: open/half_open/closed
	activeAddr         *prometheus.GaugeVec   // {cluster}
}

func newEppPromState() *eppPromState {
	s := &eppPromState{registry: prometheus.NewRegistry()}

	s.callsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "epp_calls_total",
		Help: "EPP calls by cluster and result category",
	}, []string{"cluster", "result"})
	s.failoverTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "epp_failover_total",
		Help: "EPP failover count by cluster",
	}, []string{"cluster"})
	s.failbackTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "epp_failback_total",
		Help: "EPP failback count by cluster",
	}, []string{"cluster"})
	s.fallbackLocalTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "epp_fallback_local_total",
		Help: "fallback to local balance after EPP failure, by cluster",
	}, []string{"cluster"})
	s.breakerTransitions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "epp_breaker_transitions_total",
		Help: "EPP breaker state transitions by cluster and target state",
	}, []string{"cluster", "state"})
	s.activeAddr = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "epp_active_addr_index",
		Help: "current active EPP address index by cluster",
	}, []string{"cluster"})

	s.registry.MustRegister(s.callsTotal, s.failoverTotal, s.failbackTotal,
		s.fallbackLocalTotal, s.breakerTransitions, s.activeAddr)
	return s
}

var eppProm = newEppPromState()

// EppMetricsText dumps all labeled EPP metrics in prometheus text format.
func EppMetricsText() ([]byte, error) {
	families, err := eppProm.registry.Gather()
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	encoder := expfmt.NewEncoder(&buf, expfmt.FmtText)
	for _, mf := range families {
		if err := encoder.Encode(mf); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// classifyEPPError maps an EPP call error to a result category for metrics.
// nil error maps to eppResultOK.
func classifyEPPError(err error) string {
	if err == nil {
		return eppResultOK
	}

	st, ok := status.FromError(err)
	if !ok {
		return eppResultTransport
	}

	switch st.Code() {
	case codes.Internal:
		msg := st.Message()
		switch {
		case strings.Contains(msg, "missing inference-pool"):
			return eppResultNoPool
		case strings.Contains(msg, "unknown inference pool"):
			return eppResultUnknownPool
		default:
			return eppResultTransport
		}
	case codes.Unavailable:
		msg := st.Message()
		if strings.Contains(msg, "not serving") || strings.Contains(msg, "draining") {
			return eppResultDraining
		}
		return eppResultTransport
	default:
		return eppResultTransport
	}
}

// recordEPPCall records the final outcome of one EPP-decided request.
func recordEPPCall(cluster string, err error) {
	eppProm.callsTotal.WithLabelValues(cluster, classifyEPPError(err)).Inc()
}

// recordEPPFallbackLocal records one fallback to local balance.
func recordEPPFallbackLocal(cluster string) {
	eppProm.fallbackLocalTotal.WithLabelValues(cluster).Inc()
}

func eppBreakerStateName(s eppBreakerState) string {
	switch s {
	case eppBreakerOpen:
		return "open"
	case eppBreakerHalfOpen:
		return "half_open"
	default:
		return "closed"
	}
}
