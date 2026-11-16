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

package sc03

import (
	"net/http"
	"testing"
	"time"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/tests/integration/common"
)

// issue #1476: a request whose cost exceeds the remaining quota must deduct
// the full amount, driving the balance negative, so the overrun stays
// recorded (used = quota - remaining) instead of being silently dropped.
// PassNoQuota must only bypass the auth-time balance check; the finish-time
// deduction still books the real consumption.

// overageRMBQuotaPlan is rmbQuotaPlan with PassNoQuota configurable.
func overageRMBQuotaPlan(quota int64, passNoQuota bool) common.QuotaPlan {
	p := rmbQuotaPlan(quota)
	p.PassNoQuota = passNoQuota
	return p
}

// waitRemaining polls the redis quota key until the async finish-time
// deduction lands (the SC03 tests use a fixed 500ms sleep; a poll is more
// robust for multi-request cases where deductions accumulate).
func (e *testEnv) waitRemaining(key string, timeout time.Duration, cond func(int64) bool) int64 {
	e.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if v := e.redis.GetQuota(key); cond(v) {
			return v
		}
		time.Sleep(50 * time.Millisecond)
	}
	v := e.redis.GetQuota(key)
	e.t.Fatalf("quota %s = %d, condition not met within %v", key, v, timeout)
	return v
}

// TestTC32 verifies that an over-cost request deducts the full amount: the
// balance goes negative (staying in redis), the overrun is not truncated,
// and the next request is rejected.
func TestTC32_RMBQuotaOverageFullyRecorded(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{overageRMBQuotaPlan(10000, false)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000)

	resp, body, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	// cost = 100*100 + 50*200 = 20000 fixed point units, double the balance:
	// the full amount must be deducted and the balance must go negative.
	remaining := e.waitRemaining(redisKeyRMB, 3*time.Second, func(v int64) bool { return v == -10000 })
	if remaining != -10000 {
		t.Fatalf("remaining quota = %d, want -10000", remaining)
	}

	// the overage is visible in the module state
	if !e.waitForTokenAuthMetric("QuotaOverage", 3*time.Second, func(v float64) bool { return v >= 1 }) {
		t.Fatalf("QuotaOverage counter did not reach 1")
	}

	// a negative balance means "no balance": the next request is rejected
	resp, body, err = e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		e.logBFEException()
		t.Fatalf("expected status 429, got %d, body: %s", resp.StatusCode, body)
	}
}

// TestTC33 verifies that PassNoQuota ("额度不足时放行") only bypasses the
// auth-time balance check: requests keep succeeding, and every request's
// real cost is fully deducted, so the balance keeps going more negative and
// the cumulative usage stays visible (used = quota - remaining).
func TestTC33_PassNoQuotaKeepsAccruingOverage(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{overageRMBQuotaPlan(10000, true)})
	defer e.Close()

	e.redis.SetQuota(redisKeyRMB, 10000)

	// 3 requests x 20000 fixed point units = 60000 total cost against a
	// 10000 balance. Before the fix the balance stayed pinned at 0 after
	// the first request and the remaining 40000 of usage was lost.
	for i := 0; i < 3; i++ {
		resp, body, err := e.sendRequest(apiHost, defaultBody)
		if err != nil {
			t.Fatalf("send request %d failed: %v", i+1, err)
		}
		if resp.StatusCode != http.StatusOK {
			e.logBFEException()
			t.Fatalf("request %d: expected status 200, got %d, body: %s", i+1, resp.StatusCode, body)
		}
	}

	remaining := e.waitRemaining(redisKeyRMB, 3*time.Second, func(v int64) bool { return v == -50000 })
	if remaining != -50000 {
		t.Fatalf("remaining quota = %d, want -50000", remaining)
	}

	if !e.waitForTokenAuthMetric("QuotaOverage", 3*time.Second, func(v float64) bool { return v >= 3 }) {
		t.Fatalf("QuotaOverage counter did not reach 3")
	}
}

// TestTC34 verifies the same full-deduction semantics for a total_token
// plan: usage of 150 tokens against a 100-token balance leaves -50.
func TestTC34_TokenQuotaOverageFullyRecorded(t *testing.T) {
	aiConfs := map[string]*cluster_conf.AIConf{
		clusterRMB: defaultRMBAIConf(),
	}
	e := newTestEnv(t, aiConfs, []common.QuotaPlan{tokenQuotaPlan(100)})
	defer e.Close()

	e.redis.SetQuota(redisKeyToken, 100)

	resp, body, err := e.sendRequest(apiHost, defaultBody)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		e.logBFEException()
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, body)
	}

	remaining := e.waitRemaining(redisKeyToken, 3*time.Second, func(v int64) bool { return v == -50 })
	if remaining != -50 {
		t.Fatalf("remaining token quota = %d, want -50", remaining)
	}
}
