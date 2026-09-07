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

// EPP address runtime: ordered primary/backup consumption, health check
// and failover with hysteresis (cooldown + success threshold).

package bal_gslb

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_util/epp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
)

// eppRuntimeConf is runtime parameters of eppRuntime.
type eppRuntimeConf struct {
	tlsInsecure      bool
	tlsCAFile        string
	connectTimeout   time.Duration // for establishing data connection / probe connection
	callTimeout      time.Duration // for first message (RequestHeaders Send+Recv)
	checkDisabled    bool
	checkInterval    time.Duration
	failThreshold    int
	cooldown         time.Duration
	successThreshold int
}

// eppAddrHealth tracks failover state of one EPP address.
type eppAddrHealth struct {
	lastOK        bool      // result of most recent probe
	consecFail    int       // consecutive probe fails while active
	consecSuccess int       // consecutive probe successes after cooldown (failback)
	cooldownUntil time.Time // no failback to this addr before this time
}

// eppRuntime manages ordered EPP addresses of one cluster.
// Each address has one long-lived data connection (grpc multiplexes all
// streams of the address); streams are still created per request.
type eppRuntime struct {
	name    string
	addrs   []string // ordered, addrs[0] is primary
	conf    eppRuntimeConf
	tlsConf *tls.Config

	mu     sync.Mutex
	conns  []*grpc.ClientConn // lazy data connection per addr
	active int                // index of active addr
	health []eppAddrHealth

	stopCh   chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once
	closed   int32 // set when conns closed
	probing  int32 // round guard: skip tick if previous round still running
}

func newEPPRuntime(name string, addrs []string, conf eppRuntimeConf) (*eppRuntime, error) {
	tlsConf, err := epp.BuildTLSConfig(conf.tlsInsecure, conf.tlsCAFile)
	if err != nil {
		return nil, err
	}

	rt := &eppRuntime{
		name:    name,
		addrs:   append([]string(nil), addrs...),
		conf:    conf,
		tlsConf: tlsConf,
		conns:   make([]*grpc.ClientConn, len(addrs)),
		health:  make([]eppAddrHealth, len(addrs)),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}

	// create one long-lived data connection per address; the connection is
	// lazy (established on first stream) so an unreachable address does not
	// block runtime creation - the health checker tracks its recovery
	for i, addr := range addrs {
		conn, err := epp.NewGrpcConn(addr, conf.connectTimeout, conf.tlsInsecure, conf.tlsCAFile)
		if err != nil {
			log.Logger.Warn("eppRuntime[%s]: create conn for %s failed: %v", name, addr, err)
			continue
		}
		rt.conns[i] = conn
	}

	if !conf.checkDisabled {
		go rt.healthLoop()
	} else {
		close(rt.doneCh)
	}

	eppProm.activeAddr.WithLabelValues(name).Set(0)
	return rt, nil
}

func (rt *eppRuntime) sameAddrs(addrs []string) bool {
	if len(rt.addrs) != len(addrs) {
		return false
	}
	for i := range addrs {
		if rt.addrs[i] != addrs[i] {
			return false
		}
	}
	return true
}

// updateConf refreshes runtime parameters. They take effect on next probe
// round / new connection; existing data connections are kept.
func (rt *eppRuntime) updateConf(conf eppRuntimeConf) {
	rt.mu.Lock()
	rt.conf = conf
	rt.mu.Unlock()
}

func (rt *eppRuntime) callTimeout() time.Duration {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.conf.callTimeout
}

func (rt *eppRuntime) addrCount() int {
	return len(rt.addrs)
}

func (rt *eppRuntime) addrAt(idx int) string {
	if idx < 0 || idx >= len(rt.addrs) {
		return ""
	}
	return rt.addrs[idx]
}

func (rt *eppRuntime) activeIndex() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.active
}

func (rt *eppRuntime) connFor(idx int) *grpc.ClientConn {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if idx < 0 || idx >= len(rt.conns) {
		return nil
	}
	return rt.conns[idx]
}

// stopProbes stops the background health check goroutine.
func (rt *eppRuntime) stopProbes() {
	rt.stopOnce.Do(func() {
		close(rt.stopCh)
	})
	<-rt.doneCh
}

// closeConns closes all data connections.
func (rt *eppRuntime) closeConns() {
	atomic.StoreInt32(&rt.closed, 1)
	rt.mu.Lock()
	for i, c := range rt.conns {
		if c != nil {
			c.Close()
		}
		rt.conns[i] = nil
	}
	rt.mu.Unlock()
}

func (rt *eppRuntime) healthLoop() {
	defer close(rt.doneCh)

	for {
		rt.mu.Lock()
		interval := rt.conf.checkInterval
		rt.mu.Unlock()
		if interval <= 0 {
			interval = time.Second
		}

		timer := time.NewTimer(interval)
		select {
		case <-rt.stopCh:
			timer.Stop()
			return
		case <-timer.C:
			rt.checkRound()
		}
	}
}

// probe checks liveness of one EPP address with a short-lived connection,
// so probe traffic never interferes with data connections.
func (rt *eppRuntime) probe(idx int) error {
	addr := rt.addrs[idx]

	ctx, cancel := context.WithTimeout(context.Background(), rt.conf.connectTimeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(credentials.NewTLS(rt.tlsConf)),
		grpc.WithBlock(),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	cctx, ccancel := context.WithTimeout(context.Background(), rt.conf.connectTimeout)
	defer ccancel()
	resp, err := grpc_health_v1.NewHealthClient(conn).Check(cctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		return err
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return fmt.Errorf("health status is %v", resp.GetStatus())
	}
	return nil
}

// checkRound probes the active address (and failback candidates) once and
// applies failover/failback hysteresis. Never holds rt.mu across network IO.
func (rt *eppRuntime) checkRound() {
	if !atomic.CompareAndSwapInt32(&rt.probing, 0, 1) {
		return // previous round still running, skip this tick
	}
	defer atomic.StoreInt32(&rt.probing, 0)

	now := time.Now()

	rt.mu.Lock()
	active := rt.active
	n := len(rt.addrs)
	// probe all addrs every round: keeps lastOK fresh for failover target
	// selection (addr list is small, typically primary + backup)
	idxs := make([]int, 0, n)
	for i := 0; i < n; i++ {
		idxs = append(idxs, i)
	}
	rt.mu.Unlock()

	results := make(map[int]bool, len(idxs))
	for _, idx := range idxs {
		err := rt.probe(idx)
		results[idx] = err == nil
		if err != nil {
			log.Logger.Debug("eppRuntime[%s]: probe %s failed: %v", rt.name, rt.addrAt(idx), err)
		}
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	for idx, ok := range results {
		rt.health[idx].lastOK = ok
	}

	// update active address
	if results[active] {
		rt.health[active].consecFail = 0
	} else {
		h := &rt.health[active]
		h.consecFail++
		if h.consecFail >= rt.conf.failThreshold {
			h.consecFail = 0
			h.cooldownUntil = now.Add(rt.conf.cooldown)
			if next := rt.pickFailoverLocked(active, now); next >= 0 {
				log.Logger.Warn("eppRuntime[%s]: failover from %s to %s",
					rt.name, rt.addrAt(active), rt.addrAt(next))
				state.ErrEppFailover.Inc(1)
				eppProm.failoverTotal.WithLabelValues(rt.name).Inc()
				rt.setActiveLocked(next)
			}
		}
	}

	// failback to a recovered higher priority address
	for i := 0; i < rt.active; i++ {
		h := &rt.health[i]
		if now.Before(h.cooldownUntil) {
			continue
		}
		ok, probed := results[i]
		if !probed {
			continue
		}
		if !ok {
			h.consecSuccess = 0
			h.cooldownUntil = now.Add(rt.conf.cooldown)
			continue
		}
		h.consecSuccess++
		if h.consecSuccess >= rt.conf.successThreshold {
			log.Logger.Warn("eppRuntime[%s]: failback from %s to %s",
				rt.name, rt.addrAt(rt.active), rt.addrAt(i))
			state.ErrEppFailback.Inc(1)
			eppProm.failbackTotal.WithLabelValues(rt.name).Inc()
			rt.setActiveLocked(i)
			for j := range rt.health {
				rt.health[j].consecSuccess = 0
				rt.health[j].consecFail = 0
			}
			break
		}
	}
}

// setActiveLocked switches the active address and refreshes metrics.
// Caller must hold rt.mu.
func (rt *eppRuntime) setActiveLocked(idx int) {
	rt.active = idx
	eppProm.activeAddr.WithLabelValues(rt.name).Set(float64(idx))
}

// pickFailoverLocked returns the nearest addr after `from` that is not in
// cooldown and passed the latest probe; -1 if there is no such addr (stay
// on current active, per-request retry still tries other addrs).
func (rt *eppRuntime) pickFailoverLocked(from int, now time.Time) int {
	n := len(rt.addrs)
	for j := 1; j < n; j++ {
		idx := (from + j) % n
		if now.Before(rt.health[idx].cooldownUntil) {
			continue
		}
		if !rt.health[idx].lastOK {
			continue
		}
		return idx
	}
	return -1
}
