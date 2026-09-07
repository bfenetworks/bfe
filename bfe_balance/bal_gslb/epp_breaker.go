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

// Per-cluster EPP circuit breaker (sliding window error rate), complementing
// address-level failover: failover switches address, breaker stops going to
// EPP at all and falls back to local balance.

package bal_gslb

import (
	"fmt"
	"sync"
	"time"

	"github.com/bfenetworks/go-lib/log"
)

// eppBreakerConf is configuration of eppBreaker.
type eppBreakerConf struct {
	disabled         bool
	windowSize       int           // ring buffer size, default 100
	minVolume        int           // min calls in window before evaluating, default 20
	errorRatePercent int           // error rate threshold, default 50
	openTimeout      time.Duration // OPEN duration before HALF-OPEN probing, default 30s
}

type eppBreakerState int

const (
	eppBreakerClosed eppBreakerState = iota
	eppBreakerOpen
	eppBreakerHalfOpen
)

// eppBreaker is a sliding-window circuit breaker for the EPP path of one
// cluster. Zero value is not usable; create with newEppBreaker.
type eppBreaker struct {
	name string
	mu   sync.Mutex
	conf eppBreakerConf

	state            eppBreakerState
	results          []bool // ring buffer of recent call results
	count            int    // valid entries in results
	idx              int    // next write position in ring
	openedAt         time.Time
	halfOpenInflight bool // a half-open probe request is in flight
}

func newEppBreaker(name string, conf eppBreakerConf) *eppBreaker {
	if conf.windowSize < 1 {
		conf.windowSize = 1
	}
	if conf.minVolume < 1 {
		conf.minVolume = 1
	}
	if conf.errorRatePercent < 1 || conf.errorRatePercent > 100 {
		conf.errorRatePercent = 50
	}
	if conf.openTimeout <= 0 {
		conf.openTimeout = 30 * time.Second
	}

	return &eppBreaker{
		name:    name,
		conf:    conf,
		results: make([]bool, conf.windowSize),
	}
}

// updateConf replaces configuration, keeping current state and window.
func (b *eppBreaker) updateConf(conf eppBreakerConf) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if conf.windowSize < 1 {
		conf.windowSize = 1
	}
	if conf.minVolume < 1 {
		conf.minVolume = 1
	}
	if conf.errorRatePercent < 1 || conf.errorRatePercent > 100 {
		conf.errorRatePercent = 50
	}
	if conf.openTimeout <= 0 {
		conf.openTimeout = 30 * time.Second
	}

	b.conf = conf
	b.results = make([]bool, conf.windowSize)
	b.count = 0
	b.idx = 0
}

// allow reports whether a request may go to EPP.
func (b *eppBreaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.conf.disabled {
		return true
	}

	switch b.state {
	case eppBreakerClosed:
		return true
	case eppBreakerOpen:
		if time.Since(b.openedAt) >= b.conf.openTimeout {
			b.setStateLocked(eppBreakerHalfOpen)
			b.halfOpenInflight = true
			return true
		}
		return false
	default: // half-open: allow a single probe at a time
		if b.halfOpenInflight {
			return false
		}
		b.halfOpenInflight = true
		return true
	}
}

// record feeds the result of an allowed request back into the breaker.
func (b *eppBreaker) record(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.conf.disabled {
		return
	}

	switch b.state {
	case eppBreakerOpen:
		// result of a late response to a pre-open request; ignore
		return
	case eppBreakerHalfOpen:
		b.halfOpenInflight = false
		if success {
			// probe succeeded: close and clear window
			b.setStateLocked(eppBreakerClosed)
		} else {
			// probe failed: re-open for another cooldown cycle
			b.setStateLocked(eppBreakerOpen)
		}
		return
	}

	// closed: append to sliding window
	b.results[b.idx] = success
	b.idx = (b.idx + 1) % len(b.results)
	if b.count < len(b.results) {
		b.count++
	}

	if b.count < b.conf.minVolume {
		return
	}
	errs := 0
	for i := 0; i < b.count; i++ {
		if !b.results[i] {
			errs++
		}
	}
	if errs*100 >= b.conf.errorRatePercent*b.count {
		b.setStateLocked(eppBreakerOpen)
	}
}

func (b *eppBreaker) setStateLocked(s eppBreakerState) {
	if b.state == s {
		return
	}
	b.state = s
	if s == eppBreakerOpen {
		b.openedAt = time.Now()
	}
	if s == eppBreakerClosed {
		b.count = 0
		b.idx = 0
	}

	log.Logger.Warn("eppBreaker[%s]: state -> %s", b.name, eppBreakerStateName(s))
	switch s {
	case eppBreakerOpen:
		state.ErrEppBreakerOpen.Inc(1)
	case eppBreakerHalfOpen:
		state.ErrEppBreakerHalfOpen.Inc(1)
	case eppBreakerClosed:
		state.ErrEppBreakerClosed.Inc(1)
	}
	eppProm.breakerTransitions.WithLabelValues(b.name, eppBreakerStateName(s)).Inc()
}

func (b *eppBreaker) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return fmt.Sprintf("eppBreaker[%s] state=%s window=%d/%d min=%d rate>=%d%% openTimeout=%v disabled=%v",
		b.name, eppBreakerStateName(b.state), b.count, len(b.results),
		b.conf.minVolume, b.conf.errorRatePercent, b.conf.openTimeout, b.conf.disabled)
}
