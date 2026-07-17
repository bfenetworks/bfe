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

package limit_rate

import (
	"fmt"
	"testing"
	"time"
)

func TestQpmSlidingWindow(t *testing.T) {
	// Integration tests requiring Redis at 127.0.0.1:6379
	// t.Run("WithinLimit", testQpmWithinLimit)
	// t.Run("ExceedLimit", testQpmExceedLimit)
	// t.Run("WindowExpire", testQpmWindowExpire)
	// t.Run("SlidingBehavior", testQpmSlidingBehavior)
	// t.Run("CountCorrectness", testQpmCountCorrectness)
	// t.Run("KeyIsolation", testQpmKeyIsolation)
	// t.Run("LimitOne", testQpmLimitOne)
	// t.Run("EmissionCost", testQpmEmissionCost)
}

func testQpmWithinLimit(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key := "testcase_qpm_within"
	period := int64(60)
	limit := int64(10)

	for i := int64(0); i < limit; i++ {
		_, isAllowed, _, err := agent.QpmCheck(key, period, limit, 1)
		if err != nil {
			t.Fatalf("QpmCheck error at idx %d: %v", i, err)
		}
		if !isAllowed {
			t.Errorf("request %d should be allowed", i)
		}
	}
	fmt.Println("WithinLimit: all 10 requests allowed")
}

func testQpmExceedLimit(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key := "testcase_qpm_exceed"
	period := int64(60)
	limit := int64(10)

	for i := int64(0); i < limit; i++ {
		_, isAllowed, _, err := agent.QpmCheck(key, period, limit, 1)
		if err != nil {
			t.Fatalf("QpmCheck error at idx %d: %v", i, err)
		}
		if !isAllowed {
			t.Fatalf("request %d should be allowed, but was rejected", i)
		}
	}

	_, isAllowed, count, err := agent.QpmCheck(key, period, limit, 1)
	if err != nil {
		t.Fatalf("QpmCheck error: %v", err)
	}
	if isAllowed {
		t.Errorf("request %d should be rejected, count=%f", limit+1, count)
	}
	if int64(count) != limit {
		t.Errorf("count should be %d, got %f", limit, count)
	}
	fmt.Printf("ExceedLimit: 11th request rejected, count=%f\n", count)
}

func testQpmWindowExpire(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key := "testcase_qpm_expire"
	period := int64(2)
	limit := int64(5)

	for i := int64(0); i < limit; i++ {
		_, isAllowed, _, err := agent.QpmCheck(key, period, limit, 1)
		if err != nil {
			t.Fatalf("QpmCheck error at idx %d: %v", i, err)
		}
		if !isAllowed {
			t.Fatalf("request %d should be allowed", i)
		}
	}

	_, isAllowed, _, err := agent.QpmCheck(key, period, limit, 1)
	if err != nil {
		t.Fatalf("QpmCheck error: %v", err)
	}
	if isAllowed {
		t.Errorf("request should be rejected when window full")
	}
	fmt.Println("WindowExpire: window full, request rejected")

	time.Sleep(3 * time.Second)

	_, isAllowed, _, err = agent.QpmCheck(key, period, limit, 1)
	if err != nil {
		t.Fatalf("QpmCheck error after window expire: %v", err)
	}
	if !isAllowed {
		t.Errorf("request should be allowed after window expired")
	}
	fmt.Println("WindowExpire: after 3s, request allowed")
}

func testQpmCountCorrectness(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key := "testcase_qpm_count"
	period := int64(60)
	limit := int64(10)

	for i := int64(1); i <= 5; i++ {
		_, _, count, err := agent.QpmCheck(key, period, limit, 1)
		if err != nil {
			t.Fatalf("QpmCheck error: %v", err)
		}
		if int64(count) != i {
			t.Errorf("count should be %d, got %f", i, count)
		}
	}
	fmt.Println("CountCorrectness: count increments correctly from 1 to 5")
}

func testQpmKeyIsolation(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key1 := "testcase_qpm_iso_1"
	key2 := "testcase_qpm_iso_2"
	period := int64(60)
	limit := int64(5)

	for i := int64(0); i < limit; i++ {
		_, isAllowed, _, err := agent.QpmCheck(key1, period, limit, 1)
		if err != nil {
			t.Fatalf("key1 QpmCheck error at idx %d: %v", i, err)
		}
		if !isAllowed {
			t.Fatalf("key1 request %d should be allowed", i)
		}
	}

	_, isAllowed, _, err := agent.QpmCheck(key1, period, limit, 1)
	if err != nil {
		t.Fatalf("key1 QpmCheck error: %v", err)
	}
	if isAllowed {
		t.Errorf("key1 should be rejected")
	}

	_, isAllowed, _, err = agent.QpmCheck(key2, period, limit, 1)
	if err != nil {
		t.Fatalf("key2 QpmCheck error: %v", err)
	}
	if !isAllowed {
		t.Errorf("key2 should be allowed (isolated from key1)")
	}
	fmt.Println("KeyIsolation: key2 not affected by key1 exhaustion")
}

func testQpmLimitOne(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key := "testcase_qpm_limit1"
	period := int64(60)
	limit := int64(1)

	_, isAllowed, _, err := agent.QpmCheck(key, period, limit, 1)
	if err != nil {
		t.Fatalf("QpmCheck error: %v", err)
	}
	if !isAllowed {
		t.Errorf("first request should be allowed")
	}

	_, isAllowed, _, err = agent.QpmCheck(key, period, limit, 1)
	if err != nil {
		t.Fatalf("QpmCheck error: %v", err)
	}
	if isAllowed {
		t.Errorf("second request should be rejected (limit=1)")
	}
	fmt.Println("LimitOne: 1st allowed, 2nd rejected")
}

func testQpmEmissionCost(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key := "testcase_qpm_cost"
	period := int64(60)
	limit := int64(10)

	_, isAllowed, count, err := agent.QpmCheck(key, period, limit, 3)
	if err != nil {
		t.Fatalf("QpmCheck error: %v", err)
	}
	if !isAllowed {
		t.Errorf("request with emission_cost=3 should be allowed")
	}
	if int64(count) != 3 {
		t.Errorf("count should be 3, got %f", count)
	}

	_, isAllowed, count, err = agent.QpmCheck(key, period, limit, 1)
	if err != nil {
		t.Fatalf("QpmCheck error: %v", err)
	}
	if !isAllowed {
		t.Errorf("request should be allowed")
	}
	if int64(count) != 2 {
		t.Errorf("count should be 2, got %f", count)
	}
	fmt.Println("EmissionCost: count reflects emission_cost correctly")
}

func testQpmSlidingBehavior(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key := "testcase_qpm_sliding"
	period := int64(10)
	limit := int64(5)

	// t≈1s: send 3 requests, all allowed (count 0→3)
	time.Sleep(1 * time.Second)
	for i := 0; i < 3; i++ {
		_, isAllowed, _, err := agent.QpmCheck(key, period, limit, 1)
		if err != nil {
			t.Fatalf("t=1s idx %d: %v", i, err)
		}
		if !isAllowed {
			t.Errorf("t=1s idx %d should be allowed", i)
		}
	}
	fmt.Println("t≈1s: 3 requests allowed")

	// t≈2s: send 3 requests, first 2 allowed, 3rd rejected (count 3→5)
	time.Sleep(1 * time.Second)
	for i := 0; i < 2; i++ {
		_, isAllowed, _, err := agent.QpmCheck(key, period, limit, 1)
		if err != nil {
			t.Fatalf("t=2s idx %d: %v", i, err)
		}
		if !isAllowed {
			t.Errorf("t=2s idx %d should be allowed", i)
		}
	}
	_, isAllowed, _, err := agent.QpmCheck(key, period, limit, 1)
	if err != nil {
		t.Fatalf("t=2s idx 3: %v", err)
	}
	if isAllowed {
		t.Errorf("t=2s idx 3 should be rejected (window full)")
	}
	fmt.Println("t≈2s: 2 allowed, 1 rejected (window full at 5)")

	// t≈6s: send 4 requests, all rejected (window still full)
	time.Sleep(4 * time.Second)
	for i := 0; i < 4; i++ {
		_, isAllowed, _, err := agent.QpmCheck(key, period, limit, 1)
		if err != nil {
			t.Fatalf("t=6s idx %d: %v", i, err)
		}
		if isAllowed {
			t.Errorf("t=6s idx %d should be rejected", i)
		}
	}
	fmt.Println("t≈6s: all 4 rejected")

	// t≈7s: send 1 request, rejected
	time.Sleep(1 * time.Second)
	_, isAllowed, _, err = agent.QpmCheck(key, period, limit, 1)
	if err != nil {
		t.Fatalf("t=7s: %v", err)
	}
	if isAllowed {
		t.Errorf("t=7s should be rejected")
	}
	fmt.Println("t≈7s: rejected")

	// t≈12s: send 3 requests, all allowed (t≈1s entries expired, window slid)
	time.Sleep(5 * time.Second)
	for i := 0; i < 3; i++ {
		_, isAllowed, _, err := agent.QpmCheck(key, period, limit, 1)
		if err != nil {
			t.Fatalf("t=12s idx %d: %v", i, err)
		}
		if !isAllowed {
			t.Errorf("t=12s idx %d should be allowed", i)
		}
	}
	fmt.Println("t≈12s: 3 requests allowed (window slid)")
}
