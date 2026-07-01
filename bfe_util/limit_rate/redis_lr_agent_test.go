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

	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

func makeRedisClient() redis_client.Client {
	opts := &redis_client.Options{
		ServiceConf: "127.0.0.1:6379",
		MaxIdle:     10,
		MaxActive:   20,
		Wait:        true,

		ConnTimeoutMs:  100,
		ReadTimeoutMs:  100,
		WriteTimeoutMs: 100,

		Password: "",
	}
	client := redis_client.NewRedisClient(opts)
	return client
}

func TestRedisAgent(t *testing.T) {
	// //disable when git push
	//tmpRedisAgentImplTtl(t)
	//tmpRedisAgentImpl(t)
}

func tmpRedisAgentImplTtl(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key := "testcase_conn_ttl"
	conThreshold := int64(1)
	ttl := int64(3)
	currentTime, isAllowed, curCount, err := agent.ConAcquire(
		key,
		conThreshold,
		ttl,
	)
	if err != nil {
		t.Fatalf("ConAcquire returned error: %v", err)
	}
	if !isAllowed {
		t.Errorf("isAllowed should be allowed")
	}
	fmt.Printf("ConAcquire0 currentTime:%v, isAllowed:%v, curCount:%v, err:%v\n", currentTime, isAllowed, curCount, err)

	currentTime, isAllowed, curCount, err = agent.ConAcquire(
		key,
		conThreshold,
		ttl,
	)
	if err != nil {
		t.Fatalf("ConAcquire returned error: %v", err)
	}
	if isAllowed {
		t.Errorf("isAllowed should not be allowed")
	}
	fmt.Printf("ConAcquire1 currentTime:%v, isAllowed:%v, curCount:%v, err:%v\n", currentTime, isAllowed, curCount, err)

	time.Sleep(6 * time.Second)
	currentTime, isAllowed, curCount, err = agent.ConAcquire(
		key,
		conThreshold,
		ttl,
	)
	if err != nil {
		t.Fatalf("ConAcquire returned error: %v", err)
	}
	if !isAllowed {
		t.Errorf("isAllowed should be allowed")
	}
	fmt.Printf("ConAcquire2 currentTime:%v, isAllowed:%v, curCount:%v, err:%v\n", currentTime, isAllowed, curCount, err)
}

func tmpRedisAgentImpl(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key := "testcase_conn"
	conThreshold := int64(5)
	ttl := int64(60)

	for idx := 0; idx < 5; idx++ {
		currentTime, isAllowed, curCount, err := agent.ConAcquire(
			key,
			conThreshold,
			ttl,
		)
		if err != nil {
			t.Fatalf("ConAcquire returned error: %v, idx:%d", err, idx)
		}

		if currentTime <= 0 {
			t.Errorf("unexpected currentTime: %v, idx:%d", currentTime, idx)
		}
		if !isAllowed {
			t.Errorf("isAllowed should be allowed on %d consume", idx)
		}
		expected := int64(idx) + 1
		if curCount != expected {
			t.Errorf("unexpected curCount: got %v, want %v, idx:%d", curCount, expected, idx)
		}
	}

	for idx := 5; idx < 8; idx++ {
		currentTime, isAllowed, curCount, err := agent.ConAcquire(
			key,
			conThreshold,
			ttl,
		)
		if err != nil {
			t.Fatalf("ConAcquire returned error: %v, idx:%d", err, idx)
		}

		if currentTime <= 0 {
			t.Errorf("unexpected currentTime: %v, idx:%d", currentTime, idx)
		}
		if isAllowed {
			t.Errorf("isAllowed should not be allowed on %d consume", idx)
		}

		if curCount != conThreshold {
			t.Errorf("unexpected curCount: got %v, want %v, idx:%d", curCount, conThreshold, idx)
		}
	}

	for idx := 0; idx < 5; idx++ {
		curCount, err := agent.ConRelease(
			key,
			ttl,
		)
		if err != nil {
			t.Fatalf("ConRelease returned error: %v, idx:%d", err, idx)
		}

		expected := int64(conThreshold - int64(idx) - 1)
		if curCount != expected {
			t.Errorf("unexpected curCount: got %v, want %v, idx:%d", curCount, expected, idx)
		}
	}

	currentTime, isAllowed, curCount, err := agent.ConAcquire(
		key,
		conThreshold,
		ttl,
	)
	if !isAllowed {
		t.Errorf("isAllowed should be allowed")
	}
	fmt.Printf("ConAcquire currentTime:%v, isAllowed:%v, curCount:%v, err:%v\n", currentTime, isAllowed, curCount, err)
	curCount, err = agent.ConRelease(
		key,
		ttl,
	)
	if curCount != 0 {
		t.Errorf("unexpected curCount")
	}
	fmt.Printf("ConRelease curCount:%v, err:%v\n", curCount, err)
}
