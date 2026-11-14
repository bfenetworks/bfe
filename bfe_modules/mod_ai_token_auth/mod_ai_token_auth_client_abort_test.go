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
	"github.com/bfenetworks/bfe/bfe_route/bfe_cluster"
)

// newClientAbortRequest builds a request holding an RMB plan with the given
// prompt/completion estimates, ready to be finished with a client-abort
// ErrCode.
func newClientAbortRequest(t *testing.T, redisKey string, promptTokens, completionTokens int64) (*ModuleAITokenAuth, *bfe_basic.Request, *QuotaPlan) {
	t.Helper()
	m := NewModuleAITokenAuth()
	m.redisClient = newMockRedisClient()

	req := newTestRequestWithCluster("ak-123", "AI_product", "deepseek-backup", "deepseek-v4-flash")
	// Price table so the estimate is priced (otherwise cost stays 0 and the
	// zero-cost checkpoint would attribute a config gap instead).
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{
		"deepseek-backup": buildTestClusterConf("deepseek-v4-flash", 0.000003, 0.000009),
	}}
	rmbPlan := &QuotaPlan{
		Id:       "rmb-plan",
		RedisKey: redisKey,
		Unit:     "RMB",
		Quota:    100000000,
	}
	SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id",
		QuotaPlans: []*QuotaPlan{rmbPlan}}, promptTokens, nil)

	usage := req.GetAiBasicInfo().GetTokenUsage()
	usage.PromptTokens = promptTokens
	usage.CompletionTokens = completionTokens
	return m, req, rmbPlan
}

// Issue #1478: a client abort billed by the forwarded-content estimate is
// mirrored into the shared usage (issue #1398 semantics), so the access log
// reports ai_total_tokens = in + out instead of the previous total=0 breakage.
func TestTokenRequestFinishHandler_ClientAbortBilledMirrorsEstimate(t *testing.T) {
	m, req, _ := newClientAbortRequest(t, "QUOTA_AI_product-AbortMirror", 100, 20)

	ai := req.GetAiBasicInfo()
	ai.SetAllowEstimateToken(true)
	req.ErrCode = bfe_basic.ErrClientWrite

	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK, ContentLength: -1}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	want := CalcReqUsedQuota(req, 100, 20)
	if got := ai.GetTokenUsage().UsedQuota; got != want {
		t.Errorf("mirrored UsedQuota = %d, want %d (forwarded-content estimate)", got, want)
	}
	if got := ai.UsageSource(); got != bfe_basic.UsageSourceAborted {
		t.Errorf("UsageSource = %q, want %q", got, bfe_basic.UsageSourceAborted)
	}
	if got := ai.ZeroCostReason(); got != "" {
		t.Errorf("ZeroCostReason = %q, want empty for a billed abort", got)
	}
	if got := m.state.AbortBillingEstimated.Get(); got != 1 {
		t.Errorf("AbortBillingEstimated = %d, want 1", got)
	}
	if got := m.state.AbortBillingSkipped.Get(); got != 0 {
		t.Errorf("AbortBillingSkipped = %d, want 0", got)
	}
}

// Issue #1474 + #1478: when both the upstream-drop and the client-abort
// markers are present (a synthetic worst case; the send path's attribution
// gives client-closed precedence so only one is marked in practice), the
// billing decision is taken once and exactly one billed counter advances.
func TestTokenRequestFinishHandler_ClientAbortWithDropFlagsCountsOnce(t *testing.T) {
	m, req, _ := newClientAbortRequest(t, "QUOTA_AI_product-AbortAndDrop", 100, 20)

	ai := req.GetAiBasicInfo()
	ai.SetAllowEstimateToken(true)
	ai.MarkUpstreamDropped()
	req.ErrCode = bfe_basic.ErrClientClose

	res := &bfe_http.Response{StatusCode: bfe_http.StatusOK, ContentLength: -1}
	if ret := m.tokenRequestFinishHandler(req, res); ret != bfe_module.BfeHandlerGoOn {
		t.Fatalf("expected goon, got %d", ret)
	}

	billed := m.state.AbortBillingEstimated.Get() + m.state.UpstreamDropBilled.Get()
	if billed != 1 {
		t.Errorf("billed-drop counters total = %d, want exactly 1, %v, %v",
			billed, m.state.AbortBillingEstimated.Get(), m.state.UpstreamDropBilled.Get())
	}
	if got := m.state.UsageUnrecognized.Get(); got != 0 {
		t.Errorf("UsageUnrecognized = %d, want 0", got)
	}
	if got := ai.GetTokenUsage().UsedQuota; got != CalcReqUsedQuota(req, 100, 20) {
		t.Errorf("UsedQuota = %d, want the forwarded-content estimate", got)
	}
}
