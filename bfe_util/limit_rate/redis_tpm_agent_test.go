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
	"strconv"
	"strings"
	"testing"
	"time"
)

// tpmParams 汇总 TPM 用例的常用参数，减少重复。
type tpmParams struct {
	tpm        int64
	window     int64
	bucketPeak int64
	bucketSize int64
}

func bucketField(d time.Duration) string {
	return strconv.FormatInt(testBaseTime.Add(d).Unix(), 10)
}

// 双阈值都满足：放行、记账，并返回两条余量。
func TestTpmWithinBothThresholds(t *testing.T) {
	resetRedis(t)
	const key = "tpm_within"
	p := tpmParams{tpm: 100, window: 60, bucketPeak: 50, bucketSize: 10}

	tick(0)
	currentTime, bucketOK, wholeOK, bucketLeft, totalLeft, final, err :=
		testAgent.CheckAndConsumeToken(key, p.tpm, p.window, p.bucketPeak, p.bucketSize, 10)
	if err != nil {
		t.Fatalf("CheckAndConsumeToken failed: %v", err)
	}
	if !bucketOK || !wholeOK || !final {
		t.Fatalf("bucket=%v whole=%v final=%v, want all true", bucketOK, wholeOK, final)
	}
	if bucketLeft != 40 {
		t.Errorf("bucketRemaining = %d, want 40", bucketLeft)
	}
	if totalLeft != 90 {
		t.Errorf("totalRemaining = %d, want 90", totalLeft)
	}
	if currentTime != testBaseTime.Unix() {
		t.Errorf("currentTime = %d, want %d", currentTime, testBaseTime.Unix())
	}

	if got := testRedis.HGet(key, bucketField(0)); got != "10" {
		t.Errorf("bucket counter = %q, want 10", got)
	}
}

// 单桶峰值超限：桶判失败、整窗仍可，最终拒绝且不记账。
func TestTpmBucketPeakExceeded(t *testing.T) {
	resetRedis(t)
	const key = "tpm_bucket_peak"
	p := tpmParams{tpm: 100, window: 60, bucketPeak: 50, bucketSize: 10}

	tick(0)
	if _, _, _, _, _, final, err := testAgent.CheckAndConsumeToken(key, p.tpm, p.window, p.bucketPeak, p.bucketSize, 45); err != nil || !final {
		t.Fatalf("first consume (45) should be allowed, err=%v", err)
	}

	_, bucketOK, wholeOK, _, _, final, err := checkTpm(t, key, p, 10)
	if err != nil {
		t.Fatalf("CheckAndConsumeToken failed: %v", err)
	}
	if bucketOK {
		t.Error("bucket should be rejected: 45+10 > 50")
	}
	if !wholeOK {
		t.Error("whole window should still be allowed: 45+10 <= 100")
	}
	if final {
		t.Error("final should be rejected when either threshold fails")
	}

	// 被拒的请求不得记账
	if got := testRedis.HGet(key, bucketField(0)); got != "45" {
		t.Errorf("bucket counter = %q, want 45 (rejected consume must not be recorded)", got)
	}
}

// 整窗超限：桶可、整窗不可，最终拒绝。
func TestTpmWholeWindowExceeded(t *testing.T) {
	resetRedis(t)
	const key = "tpm_whole"
	p := tpmParams{tpm: 50, window: 60, bucketPeak: 100, bucketSize: 10}

	tick(0)
	if _, _, _, _, _, final, err := testAgent.CheckAndConsumeToken(key, p.tpm, p.window, p.bucketPeak, p.bucketSize, 30); err != nil || !final {
		t.Fatalf("first consume (30) should be allowed, err=%v", err)
	}

	tick(10 * time.Second)
	_, bucketOK, wholeOK, _, _, final, err := checkTpm(t, key, p, 30)
	if err != nil {
		t.Fatalf("CheckAndConsumeToken failed: %v", err)
	}
	if !bucketOK {
		t.Error("bucket should be allowed: current bucket is empty")
	}
	if wholeOK {
		t.Error("whole window should be rejected: 30+30 > 50")
	}
	if final {
		t.Error("final should be rejected")
	}

	if got := testRedis.HGet(key, bucketField(10*time.Second)); got != "" {
		t.Error("rejected consume must not create a bucket field")
	}
}

// 跨桶累计：两个桶的消耗都计入整窗余量。
func TestTpmCrossBucketAccumulate(t *testing.T) {
	resetRedis(t)
	const key = "tpm_cross_bucket"
	p := tpmParams{tpm: 100, window: 60, bucketPeak: 50, bucketSize: 10}

	tick(0)
	if _, _, _, _, _, final, err := testAgent.CheckAndConsumeToken(key, p.tpm, p.window, p.bucketPeak, p.bucketSize, 30); err != nil || !final {
		t.Fatalf("bucket 0 consume should be allowed, err=%v", err)
	}

	tick(10 * time.Second)
	_, _, _, bucketLeft, totalLeft, final, err := checkTpm(t, key, p, 20)
	if err != nil {
		t.Fatalf("CheckAndConsumeToken failed: %v", err)
	}
	if !final {
		t.Fatal("second bucket consume should be allowed")
	}
	if bucketLeft != 30 {
		t.Errorf("bucketRemaining = %d, want 30 (counted only in current bucket)", bucketLeft)
	}
	if totalLeft != 50 {
		t.Errorf("totalRemaining = %d, want 50 (30+20 consumed across buckets)", totalLeft)
	}

	fields, err := testRedis.HKeys(key)
	if err != nil {
		t.Fatalf("HKeys failed: %v", err)
	}
	if len(fields) != 2 {
		t.Errorf("bucket fields = %v, want 2 distinct buckets", fields)
	}
}

// 整窗滑出：超过 window 的桶被清理，余量恢复。
func TestTpmWindowCleanup(t *testing.T) {
	resetRedis(t)
	const key = "tpm_cleanup"
	p := tpmParams{tpm: 100, window: 30, bucketPeak: 100, bucketSize: 10}

	tick(0)
	if _, _, _, _, _, final, err := testAgent.CheckAndConsumeToken(key, p.tpm, p.window, p.bucketPeak, p.bucketSize, 40); err != nil || !final {
		t.Fatalf("first consume should be allowed, err=%v", err)
	}

	tick(40 * time.Second)
	_, _, _, _, totalLeft, final, err := checkTpm(t, key, p, 0)
	if err != nil {
		t.Fatalf("CheckAndConsumeToken failed: %v", err)
	}
	if !final {
		t.Fatal("consume with 0 tokens should be allowed")
	}
	if totalLeft != 100 {
		t.Errorf("totalRemaining = %d, want 100: expired bucket must be removed", totalLeft)
	}
	if got := testRedis.HGet(key, bucketField(0)); got != "" {
		t.Error("expired bucket field should be deleted")
	}
}

// 事后记账：UpdateConsumeToken 把真实用量累加到当前桶。
func TestTpmUpdateConsumeToken(t *testing.T) {
	resetRedis(t)
	const key = "tpm_update"
	p := tpmParams{tpm: 1000, window: 60, bucketPeak: 1000, bucketSize: 10}

	tick(0)
	currentTime, _, _, _, _, final, err := testAgent.CheckAndConsumeToken(key, p.tpm, p.window, p.bucketPeak, p.bucketSize, 10)
	if err != nil || !final {
		t.Fatalf("预扣 should be allowed, err=%v", err)
	}

	newValue, err := testAgent.UpdateConsumeToken(key, currentTime, p.bucketSize, 100)
	if err != nil {
		t.Fatalf("UpdateConsumeToken failed: %v", err)
	}
	if newValue != 110 {
		t.Errorf("newBucketValue = %d, want 110 (10 reserved + 100 actual)", newValue)
	}

	_, _, _, _, totalLeft, final, err := checkTpm(t, key, p, 0)
	if err != nil || !final {
		t.Fatalf("check after update should be allowed, err=%v", err)
	}
	if totalLeft != 890 {
		t.Errorf("totalRemaining = %d, want 890", totalLeft)
	}
}

// 非法桶大小直接报错。
func TestTpmInvalidBucketSize(t *testing.T) {
	resetRedis(t)
	for _, size := range []int64{0, -1} {
		if _, _, _, _, _, _, err := testAgent.CheckAndConsumeToken("tpm_bad", 100, 60, 50, size, 1); err == nil {
			t.Fatalf("bucketSize=%d should fail", size)
		} else if !strings.Contains(err.Error(), "bucketSize") {
			t.Errorf("unexpected error: %v", err)
		}

		if _, err := testAgent.UpdateConsumeToken("tpm_bad", testBaseTime.Unix(), size, 1); err == nil {
			t.Fatalf("UpdateConsumeToken with bucketSize=%d should fail", size)
		}
	}
}

// bucketTime <= 0 时更新直接返回 0，不做任何写操作。
func TestTpmUpdateWithNonPositiveBucketTime(t *testing.T) {
	resetRedis(t)
	newValue, err := testAgent.UpdateConsumeToken("tpm_no_time", 0, 10, 100)
	if err != nil {
		t.Fatalf("UpdateConsumeToken failed: %v", err)
	}
	if newValue != 0 {
		t.Errorf("newBucketValue = %d, want 0", newValue)
	}
	if testRedis.Exists("tpm_no_time") {
		t.Error("no key should be written when bucketTime <= 0")
	}
}

// 脚本未初始化时报错。
func TestTpmScriptNotInitialized(t *testing.T) {
	resetRedis(t)
	agent := &RedisLRAgent{}
	if _, _, _, _, _, _, err := agent.CheckAndConsumeToken("tpm_no_script", 100, 60, 50, 10, 1); err == nil {
		t.Fatal("expected error when check script is nil")
	}
	if _, err := agent.UpdateConsumeToken("tpm_no_script", testBaseTime.Unix(), 10, 1); err == nil {
		t.Fatal("expected error when update script is nil")
	}
}

// Redis 不可用时错误向上传播。
func TestTpmRedisError(t *testing.T) {
	resetRedis(t)
	if _, _, _, _, _, _, err := deadAgent.CheckAndConsumeToken("tpm_dead", 100, 60, 50, 10, 1); err == nil {
		t.Fatal("expected redis error")
	}
	if _, err := deadAgent.UpdateConsumeToken("tpm_dead", testBaseTime.Unix(), 10, 1); err == nil {
		t.Fatal("expected redis error")
	}
}

// checkTpm 是 CheckAndConsumeToken 的简化包装，只保留断言关心的返回值。
func checkTpm(t *testing.T, key string, p tpmParams, tokens int64) (
	currentTime int64, bucketOK, wholeOK bool, bucketLeft, totalLeft int64, final bool, err error) {
	t.Helper()
	return testAgent.CheckAndConsumeToken(key, p.tpm, p.window, p.bucketPeak, p.bucketSize, tokens)
}
