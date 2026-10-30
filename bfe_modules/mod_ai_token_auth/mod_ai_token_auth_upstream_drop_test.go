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

package mod_ai_token_auth

import (
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

// Issue #1474: an upstream that drops the connection mid-response is billed
// by the forwarded-content estimate (the auth-time prompt seeding plus the
// per-chunk completion accumulation). Unlike a client abort, the content
// already delivered is real consumption.
func TestTokenRequestFinishHandler_UpstreamDropBilledByEstimate(t *testing.T) {
	m := NewModuleAITokenAuth()
	m.redisClient = newMockRedisClient()

	req := newTestRequestWithCluster("ak-123", "AI_product", "deepseek-backup", "deepseek-v4-flash")
	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-UpstreamDrop",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id",
		QuotaPlans: []*QuotaPlan{rmbPlan}}, 100, nil)

	ai := req.GetAiBasicInfo()
	ai.SetAllowEstimateToken(true)
	ai.MarkUpstreamDropped()
	usage := ai.GetTokenUsage()
	usage.PromptTokens = 100
	usage.CompletionTokens = 20
	usage.UsedQuota = 0

	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	if got := ai.UsageSource(); got != bfe_basic.UsageSourceUpstreamDropped {
		t.Errorf("UsageSource = %q, want %q", got, bfe_basic.UsageSourceUpstreamDropped)
	}
	if got := m.state.UpstreamDropBilled.Get(); got != 1 {
		t.Errorf("UpstreamDropBilled = %d, want 1", got)
	}
	if got := m.state.AbortBillingSkipped.Get(); got != 0 {
		t.Errorf("AbortBillingSkipped = %d, want 0: an upstream drop is not a client abort", got)
	}
	if got := m.state.UsageUnrecognized.Get(); got != 0 {
		t.Errorf("UsageUnrecognized = %d, want 0 for an estimate-billed drop", got)
	}
	want := CalcReqUsedQuota(req, 100, 20)
	if u := ai.GetTokenUsage(); u.UsedQuota != want {
		t.Errorf("mirrored UsedQuota = %d, want %d (forwarded-content estimate)", u.UsedQuota, want)
	}
}

// Issue #1474: when EstimateToken is off, an upstream drop cannot be billed,
// but it must not be silent: the unrecognized signal keeps firing and the
// zero-cost reason distinguishes the drop from a generic unrecognized shape
// and from a client abort.
func TestTokenRequestFinishHandler_UpstreamDropEstimateDisabled(t *testing.T) {
	m := NewModuleAITokenAuth()
	m.redisClient = newMockRedisClient()

	req := newTestRequestWithCluster("ak-123", "AI_product", "deepseek-backup", "deepseek-v4-flash")
	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: "QUOTA_AI_product-UpstreamDropNoEst",
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id",
		QuotaPlans: []*QuotaPlan{rmbPlan}}, 100, nil)

	ai := req.GetAiBasicInfo()
	ai.SetAllowEstimateToken(false)
	ai.MarkUpstreamDropped()
	usage := ai.GetTokenUsage()
	usage.PromptTokens = 100
	usage.CompletionTokens = 20

	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	if got := ai.UsageSource(); got != bfe_basic.UsageSourceUnrecognized {
		t.Errorf("UsageSource = %q, want %q", got, bfe_basic.UsageSourceUnrecognized)
	}
	if got := ai.ZeroCostReason(); got != bfe_basic.ZeroCostReasonUpstreamDropped {
		t.Errorf("ZeroCostReason = %q, want %q", got, bfe_basic.ZeroCostReasonUpstreamDropped)
	}
	if got := m.state.UsageUnrecognized.Get(); got != 1 {
		t.Errorf("UsageUnrecognized = %d, want 1", got)
	}
	if got := m.state.BillingZeroCost.Get(); got != 1 {
		t.Errorf("BillingZeroCost = %d, want 1", got)
	}
	if got := m.state.UpstreamDropBilled.Get(); got != 0 {
		t.Errorf("UpstreamDropBilled = %d, want 0", got)
	}
	if got := m.state.AbortBillingSkipped.Get(); got != 0 {
		t.Errorf("AbortBillingSkipped = %d, want 0", got)
	}
	if u := ai.GetTokenUsage(); u.UsedQuota != 0 {
		t.Errorf("UsedQuota = %d, want 0 with estimation disabled", u.UsedQuota)
	}
}
