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
)

func TestTpmRedisAgent(t *testing.T) {
	// //disable when git push
	// tmpTpmRedisAgentImpl(t)
}

func tmpTpmRedisAgentImpl(t *testing.T) {
	client := makeRedisClient()
	agent := NewRedisLRAgent(client)

	key := "testcase_tpm"
	tpmThreshold := int64(100)
	windowSizeSec := int64(60)
	bucketPeakLimit := int64(20)
	bucketSizeSec := int64(10)
	tokensToConsume := int64(5)

	// 4. Call CheckAndConsumeToken
	currentTime, isBucket, isWhole, bucketRem, totalRem, isFinal, err := agent.CheckAndConsumeToken(
		key,
		tpmThreshold,
		windowSizeSec,
		bucketPeakLimit,
		bucketSizeSec,
		tokensToConsume,
	)
	if err != nil {
		t.Fatalf("CheckAndConsumeToken returned error: %v", err)
	}

	if currentTime <= 0 {
		t.Errorf("unexpected currentTime: %v", currentTime)
	}
	if !isBucket {
		t.Errorf("bucket should be allowed on first consume")
	}
	if !isWhole {
		t.Errorf("whole window should be allowed on first consume")
	}
	if !isFinal {
		t.Errorf("isFinal should be allowed on first consume")
	}

	if bucketRem != (bucketPeakLimit - tokensToConsume) {
		t.Errorf("unexpected bucketRem: got %v, want %v", bucketRem, bucketPeakLimit-tokensToConsume)
	}
	if totalRem != (tpmThreshold - tokensToConsume) {
		t.Errorf("unexpected totalRem: got %v, want %v", totalRem, tpmThreshold-tokensToConsume)
	}
	if totalRem != (tpmThreshold - tokensToConsume) {
		t.Errorf("unexpected totalRem: got %v, want %v", totalRem, tpmThreshold-tokensToConsume)
	}

	currentTime2, isBucket2, isWhole2, bucketRem2, totalRem2, isFinal2, err := agent.CheckAndConsumeToken(
		key,
		tpmThreshold,
		windowSizeSec,
		bucketPeakLimit,
		bucketSizeSec,
		tokensToConsume,
	)
	if err != nil {
		t.Fatalf("second call returned error: %v", err)
	}

	if !isBucket2 {
		t.Errorf("bucket should still be allowed if under peak limit")
	}
	if !isWhole2 {
		t.Errorf("whole window should still be allowed if under threshold")
	}
	if !isFinal2 {
		t.Errorf("isFinal should still be allowed if under peak limit")
	}

	if bucketRem2 != (bucketPeakLimit - tokensToConsume*2) {
		t.Errorf("unexpected bucketRem2: got %v, want %v", bucketRem2, bucketPeakLimit-tokensToConsume*2)
	}
	if totalRem2 != (tpmThreshold - tokensToConsume*2) {
		t.Errorf("unexpected totalRem2: got %v, want %v", totalRem2, tpmThreshold-tokensToConsume*2)
	}

	fmt.Printf("first call: currentTime=%v, bucketRem=%v, totalRem=%v\n", currentTime, bucketRem, totalRem)
	fmt.Printf("second call: currentTime=%v, bucketRem=%v, totalRem=%v\n", currentTime2, bucketRem2, totalRem2)

	tokensToConsume = 12
	newV, err := agent.UpdateConsumeToken(key, currentTime2, bucketSizeSec, tokensToConsume)
	if err != nil {
		t.Fatalf("UpdateConsumeToken returned error: %v", err)
	}
	fmt.Printf("UpdateConsumeToken currentTime2=%v, newV=%v\n", currentTime2, newV)

	currentTime3, isBucket3, isWhole3, bucketRem3, totalRem3, isFinal3, err := agent.CheckAndConsumeToken(
		key,
		tpmThreshold,
		windowSizeSec,
		bucketPeakLimit,
		bucketSizeSec,
		tokensToConsume,
	)
	if err != nil {
		t.Fatalf("second call returned error: %v", err)
	}
	if isFinal3 {
		t.Errorf("isFinal should still not be allowed")
	}

	fmt.Printf("third call: currentTime3=%v, bucketRem=%v, totalRem=%v\n", currentTime3, bucketRem3, totalRem3)
	fmt.Printf("third call: currentTime3=%v, isBucket3=%v, isWhole3=%v\n", currentTime3, isBucket3, isWhole3)
}
