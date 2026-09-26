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
	"sync"
	"time"
)

// circuitState tracks the failure state of one mirror target cluster.
type circuitState struct {
	consecutiveFails int       // consecutive failures since last success
	open             bool      // whether the circuit is open (dropping)
	openedAt         time.Time // when the circuit was opened
}

// mirrorCircuitBreaker is a simple per-cluster circuit breaker: after
// threshold consecutive failures the cluster is dropped for a cooldown
// window; after the window a probe request is allowed through.
type mirrorCircuitBreaker struct {
	threshold int
	cooldown  time.Duration

	lock   sync.Mutex
	states map[string]*circuitState
}

func newMirrorCircuitBreaker(threshold int, cooldownSec int) *mirrorCircuitBreaker {
	return &mirrorCircuitBreaker{
		threshold: threshold,
		cooldown:  time.Duration(cooldownSec) * time.Second,
		states:    make(map[string]*circuitState),
	}
}

// Open returns true when the circuit for the cluster is open and the mirror
// request should be dropped. After the cooldown window the cluster is
// treated as half-open and a probe request is allowed through.
func (cb *mirrorCircuitBreaker) Open(cluster string) bool {
	cb.lock.Lock()
	defer cb.lock.Unlock()

	st, ok := cb.states[cluster]
	if !ok || !st.open {
		return false
	}

	if time.Since(st.openedAt) >= cb.cooldown {
		// half-open: allow a probe, keep the circuit open until the
		// probe result arrives (OnSuccess/OnFail)
		return false
	}
	return true
}

// OnFail records a failure for the cluster; when the consecutive failure
// count reaches the threshold the circuit opens.
func (cb *mirrorCircuitBreaker) OnFail(cluster string) {
	cb.lock.Lock()
	defer cb.lock.Unlock()

	st, ok := cb.states[cluster]
	if !ok {
		st = &circuitState{}
		cb.states[cluster] = st
	}

	st.consecutiveFails++
	if st.consecutiveFails >= cb.threshold {
		st.open = true
		st.openedAt = time.Now()
	}
}

// OnSuccess records a success for the cluster and closes the circuit.
func (cb *mirrorCircuitBreaker) OnSuccess(cluster string) {
	cb.lock.Lock()
	defer cb.lock.Unlock()

	st, ok := cb.states[cluster]
	if !ok {
		st = &circuitState{}
		cb.states[cluster] = st
	}

	st.consecutiveFails = 0
	st.open = false
	st.openedAt = time.Time{}
}
