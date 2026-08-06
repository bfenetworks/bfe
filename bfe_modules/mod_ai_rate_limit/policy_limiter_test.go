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

package mod_ai_rate_limit

import (
	"testing"

	"github.com/bfenetworks/bfe/bfe_util/limit_rate"
)

func TestBuildRedisKey(t *testing.T) {
	got := buildRedisKey("policy1", "suffix")
	want := "default_bfe_policy1_suffix"
	if got != want {
		t.Errorf("buildRedisKey: expected %s, got %s", want, got)
	}
}

func TestBuildTpmInstIdWithName(t *testing.T) {
	rule := &TPMRuleConf{Name: "abc", TimeWindow: 300, Threshold: 1000, BucketTimeWindow: 60, BucketThreshold: 1000}
	if got := buildTpmInstId(rule); got != "abc" {
		t.Errorf("expected abc, got %s", got)
	}
}

func TestBuildTpmInstIdWithoutName(t *testing.T) {
	rule := &TPMRuleConf{TimeWindow: 300, Threshold: 1000, BucketTimeWindow: 60, BucketThreshold: 1000}
	want := "tpm_300_1000_60_1000"
	if got := buildTpmInstId(rule); got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestBuildRpmInstIdWithName(t *testing.T) {
	rule := &RPMRuleConf{Name: "abc", TimeWindow: 120, MaxRequests: 100, Burst: 10}
	if got := buildRpmInstId(rule); got != "abc" {
		t.Errorf("expected abc, got %s", got)
	}
}

func TestBuildRpmInstIdWithoutName(t *testing.T) {
	rule := &RPMRuleConf{TimeWindow: 120, MaxRequests: 100, Burst: 10}
	want := "rpm_120_100_10"
	if got := buildRpmInstId(rule); got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestPredictTokenUsage(t *testing.T) {
	item := &tpmLimiterItem{ReservedX: 2.0, ReservedOff: 10.0}
	if got := item.predictTokenUsage(5); got != 20 {
		t.Errorf("expected 20, got %d", got)
	}
	item2 := &tpmLimiterItem{ReservedX: 0, ReservedOff: 0}
	if got := item2.predictTokenUsage(100); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestNewPolicyLimiterManager(t *testing.T) {
	m := newPolicyLimiterManager()
	if m == nil {
		t.Fatal("newPolicyLimiterManager should not return nil")
	}
	if m.limiters == nil {
		t.Fatal("limiters map should be initialized")
	}
}

func TestPolicyLimiterManagerGetLimiterPolicySetMissing(t *testing.T) {
	m := newPolicyLimiterManager()
	if got := m.getLimiterPolicySet("missing"); got != nil {
		t.Error("expected nil for missing policy id")
	}
}

func TestPolicyLimiterManagerUpdateLimitersAndGetStats(t *testing.T) {
	m := newPolicyLimiterManager()
	maxConn := int64(10)
	policies := map[string]*PolicyConf{
		"rlp-0001": {
			Name:    "ratelimitX",
			Enabled: true,
			Rules: &LimitRulesConf{
				TPM: []*TPMRuleConf{
					{Name: "tpm1", TimeWindow: 60, Threshold: 1000, BucketTimeWindow: 60, BucketThreshold: 1000},
				},
				RPM: []*RPMRuleConf{
					{Name: "rpm1", TimeWindow: 60, MaxRequests: 100, Burst: 1},
				},
				MaxConcurrency: &maxConn,
			},
		},
	}

	m.updateLimiters(policies)

	if got := m.getLimiterPolicySet("rlp-0001"); got == nil {
		t.Fatal("expected limiter set for rlp-0001")
	}
	if got := m.getLimiterPolicySet("missing"); got != nil {
		t.Error("expected nil for missing policy id after update")
	}

	tpmStats, rpmStats, conStats := m.getLimiterStats()
	if len(tpmStats) != 1 || tpmStats[0].PolicyId != "rlp-0001" || tpmStats[0].InstId != "tpm1" {
		t.Errorf("unexpected tpm stats: %v", tpmStats)
	}
	if len(rpmStats) != 1 || rpmStats[0].PolicyId != "rlp-0001" || rpmStats[0].InstId != "rpm1" {
		t.Errorf("unexpected rpm stats: %v", rpmStats)
	}
	if len(conStats) != 1 || conStats[0].PolicyId != "rlp-0001" || conStats[0].InstId != "concurrency" {
		t.Errorf("unexpected con stats: %v", conStats)
	}
}

func TestPolicyLimiterSetUpdateCount(t *testing.T) {
	old := &policyLimiterSet{
		policyId: "rlp-0001",
		tpmLimiters: []*tpmLimiterItem{
			{name: "tpm1", limiter: limit_rate.NewTPMLimiter("k1", 100, 60, 100, 60)},
		},
		rpmLimiters: []*rpmLimiterItem{
			{name: "rpm1", limiter: limit_rate.NewQPMLimiter("k2", 1, 60, 100)},
		},
		conLimiter: &conLimiterItem{conLimiter: limit_rate.NewConcurrencyLimiter("k3", 10, 300)},
	}
	old.tpmLimiters[0].matchCount.Add(5)
	old.tpmLimiters[0].hitCount.Add(3)
	old.tpmLimiters[0].tokenCount.Add(100)
	old.rpmLimiters[0].matchCount.Add(7)
	old.rpmLimiters[0].hitCount.Add(2)
	old.conLimiter.matchCount.Add(4)
	old.conLimiter.hitCount.Add(1)

	newSet := &policyLimiterSet{
		policyId: "rlp-0001",
		tpmLimiters: []*tpmLimiterItem{
			{name: "tpm1", limiter: limit_rate.NewTPMLimiter("k1", 100, 60, 100, 60)},
		},
		rpmLimiters: []*rpmLimiterItem{
			{name: "rpm1", limiter: limit_rate.NewQPMLimiter("k2", 1, 60, 100)},
		},
		conLimiter: &conLimiterItem{conLimiter: limit_rate.NewConcurrencyLimiter("k3", 10, 300)},
	}
	newSet.updateCount(old)

	if got := newSet.tpmLimiters[0].matchCount.Load(); got != 5 {
		t.Errorf("expected tpm match count 5, got %d", got)
	}
	if got := newSet.tpmLimiters[0].tokenCount.Load(); got != 100 {
		t.Errorf("expected tpm token count 100, got %d", got)
	}
	if got := newSet.rpmLimiters[0].hitCount.Load(); got != 2 {
		t.Errorf("expected rpm hit count 2, got %d", got)
	}
	if got := newSet.conLimiter.matchCount.Load(); got != 4 {
		t.Errorf("expected con match count 4, got %d", got)
	}
}

func TestNewPolicyLimiterSetEmptyRules(t *testing.T) {
	ps := newPolicyLimiterSet("rlp-empty", &PolicyConf{Name: "empty", Enabled: true})
	if ps == nil {
		t.Fatal("expected non-nil policyLimiterSet")
	}
	if len(ps.tpmLimiters) != 0 || len(ps.rpmLimiters) != 0 || ps.conLimiter != nil {
		t.Error("expected empty limiter set when Rules is nil")
	}
}

func TestPolicyLimiterSetCheckNoRules(t *testing.T) {
	req := newTestRequest("AI_product", "ak-123", "gpt-4")
	meta := req.GetAiBasicInfo()
	ctx := &PolicyLimiterContext{}
	ls := &policyLimiterSet{policyId: "rlp-0001"}

	if !ls.checkConcurrency(req, meta, nil, ctx, "gpt-4", false) {
		t.Error("checkConcurrency should return true when no concurrency limiter")
	}
	if !ls.checkRPM(req, meta, nil, ctx, "gpt-4", false) {
		t.Error("checkRPM should return true when no rpm limiters")
	}
	if !ls.checkTPM(req, meta, nil, ctx, "gpt-4", false) {
		t.Error("checkTPM should return true when no tpm limiters")
	}
}
