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

func TestQpmRedisAgent(t *testing.T) {
	// //disable when git push
	// tmpQpmRedisAgentImpl(t)
	// tmpQpmRedisAgentBurstImpl(t)
	// tmpQpmRedisAgentBurstImpl2(t)
}

func tmpQpmRedisAgentImpl(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key := "testcase_qpm"
	burst := int64(1)
	period := int64(60)
	limit := int64(60) //1 request per second
	expectUse := int64(1)

	// first request should be allowed
	currentTime, isAllowed, tat, err := agent.QpmCheck(
		key,
		burst,
		period,
		limit,
		expectUse,
	)
	if err != nil {
		t.Fatalf("QpmCheck returned error: %v", err)
	}
	if currentTime <= 0 {
		t.Errorf("unexpected currentTime: %v", currentTime)
	}
	if !isAllowed {
		t.Errorf("isAllowed should be allowed on first request")
	}
	fmt.Printf("QpmCheck1 currentTime:%v, isAllowed:%v, tat:%f, err:%v\n", currentTime, isAllowed, tat, err)

	// second request immediately should be denied (increment=1s, burst=1, only 1 buffered)
	currentTime, isAllowed, tat, err = agent.QpmCheck(
		key,
		burst,
		period,
		limit,
		expectUse,
	)
	if err != nil {
		t.Fatalf("QpmCheck returned error: %v", err)
	}
	if isAllowed {
		t.Errorf("isAllowed should not be allowed on immediate second request")
	}
	fmt.Printf("QpmCheck2 currentTime:%v, isAllowed:%v, tat:%f, err:%v\n", currentTime, isAllowed, tat, err)

	// wait for increment (1s) + a small buffer, then request should be allowed again
	time.Sleep(1500 * time.Millisecond)
	currentTime, isAllowed, tat, err = agent.QpmCheck(
		key,
		burst,
		period,
		limit,
		expectUse,
	)
	if err != nil {
		t.Fatalf("QpmCheck returned error: %v", err)
	}
	if !isAllowed {
		t.Errorf("isAllowed should be allowed after waiting 1.5s")
	}
	fmt.Printf("QpmCheck3 currentTime:%v, isAllowed:%v, tat:%v, err:%v\n", currentTime, isAllowed, tat, err)
}

func tmpQpmRedisAgentBurstImpl(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key := "testcase_qpm_burst"
	burst := int64(5)
	period := int64(60)
	limit := int64(6000)
	expectUse := int64(1)

	// with burst=5, increment=0.01s, first 6 requests should be allowed
	for idx := 0; idx < 6; idx++ {
		currentTime, isAllowed, tat, err := agent.QpmCheck(
			key,
			burst,
			period,
			limit,
			expectUse,
		)
		fmt.Printf("QpmCheckBurst idx:%d, currentTime:%v, isAllowed:%v, tat:%f\n", idx, currentTime, isAllowed, tat)
		if err != nil {
			t.Fatalf("QpmCheck returned error: %v, idx:%d", err, idx)
		}
		if currentTime <= 0 {
			t.Errorf("unexpected currentTime: %v, idx:%d, %f", currentTime, idx, tat)
		}
		if !isAllowed {
			t.Errorf("isAllowed should be allowed on request %d", idx)
		}
		time.Sleep(2 * time.Millisecond)
	}

	// 7th request should be denied
	currentTime, isAllowed, tat, err := agent.QpmCheck(
		key,
		burst,
		period,
		limit,
		expectUse,
	)
	if err != nil {
		t.Fatalf("QpmCheck returned error: %v", err)
	}
	if isAllowed {
		t.Errorf("isAllowed should not be allowed on 7th request (burst exhausted)")
	}
	fmt.Printf("QpmCheckBurst idx:6, currentTime:%v, isAllowed:%v, tat:%f\n", currentTime, isAllowed, tat)
}

func tmpQpmRedisAgentBurstImpl2(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key := "testcase_qpm_burst1"
	burst := int64(1)
	period := int64(60)
	limit := int64(6000) //0.01s each request
	expectUse := int64(1)

	// with burst=5, increment=0.01s, first 6 requests should be allowed
	for idx := 0; idx < 6; idx++ {
		currentTime, isAllowed, tat, err := agent.QpmCheck(
			key,
			burst,
			period,
			limit,
			expectUse,
		)
		fmt.Printf("QpmCheckBurst idx:%d, currentTime:%v, isAllowed:%v, tat:%f\n", idx, currentTime, isAllowed, tat)
		if err != nil {
			t.Fatalf("QpmCheck returned error: %v, idx:%d", err, idx)
		}
		if currentTime <= 0 {
			t.Errorf("unexpected currentTime: %v, idx:%d, %f", currentTime, idx, tat)
		}
		if !isAllowed {
			t.Errorf("isAllowed should be allowed on request %d", idx)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 7th request should be allowed
	currentTime, isAllowed, tat, err := agent.QpmCheck(
		key,
		burst,
		period,
		limit,
		expectUse,
	)
	if err != nil {
		t.Fatalf("QpmCheck returned error: %v", err)
	}
	if !isAllowed {
		t.Errorf("isAllowed should not be allowed on 7th request (burst exhausted)")
	}
	fmt.Printf("QpmCheckBurst idx:6, currentTime:%v, isAllowed:%v, tat:%f\n", currentTime, isAllowed, tat)
}
