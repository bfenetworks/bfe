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
	"testing"
	"time"
)

func TestCircuitBreakerOpensAfterThreshold(t *testing.T) {
	cb := newMirrorCircuitBreaker(3, 60)

	cb.OnFail("c1")
	cb.OnFail("c1")
	if cb.Open("c1") {
		t.Error("circuit should stay closed below threshold")
	}

	cb.OnFail("c1")
	if !cb.Open("c1") {
		t.Error("circuit should open at threshold")
	}

	// another cluster is unaffected
	if cb.Open("c2") {
		t.Error("other cluster should not be open")
	}
}

func TestCircuitBreakerSuccessCloses(t *testing.T) {
	cb := newMirrorCircuitBreaker(2, 60)

	cb.OnFail("c1")
	cb.OnFail("c1")
	if !cb.Open("c1") {
		t.Fatal("circuit should be open")
	}

	cb.OnSuccess("c1")
	if cb.Open("c1") {
		t.Error("circuit should close after success")
	}

	// consecutive fail count resets: a single fail stays below threshold
	cb.OnFail("c1")
	if cb.Open("c1") {
		t.Error("circuit should tolerate threshold-1 fails after reset")
	}
}

func TestCircuitBreakerCooldownHalfOpen(t *testing.T) {
	cb := newMirrorCircuitBreaker(1, 60)
	// shorten the cooldown window for the test
	cb.cooldown = 50 * time.Millisecond

	cb.OnFail("c1")
	if !cb.Open("c1") {
		t.Fatal("circuit should be open")
	}

	time.Sleep(80 * time.Millisecond)
	if cb.Open("c1") {
		t.Error("circuit should allow a probe after cooldown")
	}
}
