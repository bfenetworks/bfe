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
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_route/bfe_cluster"
)

// Issue #1479: PriceMissAction accepts the three documented values, rejects
// anything else, and normalizes the empty value to the fallback default.
func TestAIConfCheck_PriceMissAction(t *testing.T) {
	for _, ok := range []string{"", cluster_conf.PriceMissActionFallbackChat,
		cluster_conf.PriceMissActionBillZero, cluster_conf.PriceMissActionReject} {
		conf := &cluster_conf.AIConf{PriceMissAction: ok}
		if err := cluster_conf.AIConfCheck(conf); err != nil {
			t.Errorf("PriceMissAction %q: unexpected error %v", ok, err)
		}
		want := ok
		if want == "" {
			want = cluster_conf.PriceMissActionFallbackChat
		}
		if conf.PriceMissAction != want {
			t.Errorf("PriceMissAction %q normalized to %q, want %q", ok, conf.PriceMissAction, want)
		}
	}

	bad := &cluster_conf.AIConf{PriceMissAction: "reject_all"}
	if err := cluster_conf.AIConfCheck(bad); err == nil {
		t.Error("expected an error for an unknown PriceMissAction value")
	}
}

// Issue #1479: the bill_zero action bills the missing (model, mode) entry at
// 0, marks the shape as no_entry and counts PriceMissZeroBilled.
func TestCalcCostUnits_PriceMissActionBillZero(t *testing.T) {
	m := NewModuleAITokenAuth()
	clusterName := "responses-backup"
	model := "gpt-5-codex"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	cluster := buildTestClusterConf(model, 0.000003, 0.000009)
	cluster.AIConf.PriceMissAction = cluster_conf.PriceMissActionBillZero
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}
	ai := req.GetAiBasicInfo()
	ai.Mode = bfe_basic.ModeResponses

	usage := &bfe_basic.TokenUsage{PromptTokens: 100, CompletionTokens: 200}
	if got := m.calcCostUnits(req, req.SvrDataConf, usage); got != 0 {
		t.Errorf("bill_zero action: cost = %d, want 0", got)
	}
	if v := m.state.PriceMissZeroBilled.Get(); v != 1 {
		t.Errorf("PriceMissZeroBilled = %d, want 1", v)
	}
	if v := m.state.PriceFallbackUsed.Get(); v != 0 {
		t.Errorf("PriceFallbackUsed = %d, want 0 under bill_zero", v)
	}
	if !ai.IsPriceMiss() {
		t.Error("expected ai_price_miss to be marked")
	}
	if got := ai.PriceWarn(); got != bfe_basic.PriceWarnNoEntry {
		t.Errorf("PriceWarn = %q, want %q", got, bfe_basic.PriceWarnNoEntry)
	}
	if got := ai.ZeroCostReason(); got != bfe_basic.ZeroCostReasonPriceMiss {
		t.Errorf("ZeroCostReason = %q, want %q", got, bfe_basic.ZeroCostReasonPriceMiss)
	}
}

// Issue #1479: reaching calcCostUnits with the reject action (an in-flight
// request; normally rejected pre-forward) stays fail-safe: bill 0 and keep the
// miss visible.
func TestCalcCostUnits_PriceMissActionRejectInFlight(t *testing.T) {
	m := NewModuleAITokenAuth()
	clusterName := "responses-backup"
	model := "gpt-5-codex"
	req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)

	cluster := buildTestClusterConf(model, 0.000003, 0.000009)
	cluster.AIConf.PriceMissAction = cluster_conf.PriceMissActionReject
	req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}
	ai := req.GetAiBasicInfo()
	ai.Mode = bfe_basic.ModeResponses

	usage := &bfe_basic.TokenUsage{PromptTokens: 100, CompletionTokens: 200}
	if got := m.calcCostUnits(req, req.SvrDataConf, usage); got != 0 {
		t.Errorf("reject action in flight: cost = %d, want 0", got)
	}
	if v := m.state.PriceMissZeroBilled.Get(); v != 1 {
		t.Errorf("PriceMissZeroBilled = %d, want 1", v)
	}
	if got := ai.PriceWarn(); got != bfe_basic.PriceWarnNoEntry {
		t.Errorf("PriceWarn = %q, want %q", got, bfe_basic.PriceWarnNoEntry)
	}
}

// Issue #1479: a fallback-billed request whose chat entry lacks cache sub-item
// prices that the request actually used is counted by PriceFallbackSubItemMiss
// and its shape overrides the plain fallback mark; configured cache prices or
// unused dimensions never count.
func TestCalcCostUnits_FallbackSubitemMissing(t *testing.T) {
	clusterName := "responses-backup"
	model := "gpt-5-codex"
	usage := &bfe_basic.TokenUsage{PromptTokens: 100, CompletionTokens: 200,
		CacheReadTokens: 40, CacheWriteTokens: 10}

	// The helpers always write cache price keys (with cost 0 when unset), so
	// the "absent key" shape is built by removing the keys from the entry.
	stripCacheKeys := func(cluster *bfe_cluster.BfeCluster) {
		for i := range cluster.AIConf.ModelTable.Models {
			delete(cluster.AIConf.ModelTable.Models[i].Prices, cluster_conf.PriceCacheReadInputTokenCost)
			delete(cluster.AIConf.ModelTable.Models[i].Prices, cluster_conf.PriceCacheCreationInputTokenCost)
			delete(cluster.AIConf.ModelTable.Models[i].Prices, cluster_conf.PriceCacheCreationInputTokenCost1h)
		}
	}
	newReq := func(cluster *bfe_cluster.BfeCluster) *bfe_basic.Request {
		req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
		req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}
		req.GetAiBasicInfo().Mode = bfe_basic.ModeResponses
		return req
	}

	t.Run("keys absent and dimensions used", func(t *testing.T) {
		m := NewModuleAITokenAuth()
		cluster := buildTestClusterConf(model, 0.000003, 0.000009)
		stripCacheKeys(cluster)
		req := newReq(cluster)
		if got := m.calcCostUnits(req, req.SvrDataConf, usage); got == 0 {
			t.Fatal("expected the fallback price to bill a non-zero cost")
		}
		if v := m.state.PriceFallbackSubItemMiss.Get(); v != 1 {
			t.Errorf("PriceFallbackSubItemMiss = %d, want 1", v)
		}
		if got := req.GetAiBasicInfo().PriceWarn(); got != bfe_basic.PriceWarnSubitemMissing {
			t.Errorf("PriceWarn = %q, want %q (most specific shape wins)", got, bfe_basic.PriceWarnSubitemMissing)
		}
	})

	t.Run("cache prices configured", func(t *testing.T) {
		m := NewModuleAITokenAuth()
		req := newReq(buildTestClusterConfWithCache(model, 0.000003, 0.000009, 0.0000005, 0.000001))
		m.calcCostUnits(req, req.SvrDataConf, usage)
		if v := m.state.PriceFallbackSubItemMiss.Get(); v != 0 {
			t.Errorf("PriceFallbackSubItemMiss = %d, want 0 when cache prices are configured", v)
		}
		if got := req.GetAiBasicInfo().PriceWarn(); got != bfe_basic.PriceWarnFallback {
			t.Errorf("PriceWarn = %q, want %q", got, bfe_basic.PriceWarnFallback)
		}
	})
}

// Issue #1479: an explicit 0 sub-item price is a priced dimension (HasPrice
// semantics, same convention as issue #1420) and must not be reported as
// missing; unused dimensions are never checked.
func TestMissingSubitemKeys(t *testing.T) {
	entry := &cluster_conf.ModelPrice{
		Prices: cluster_conf.PriceMap{
			"cache_read_input_token_cost": 0, // explicitly priced at 0
		},
	}

	if got := missingSubitemKeys(entry, &bfe_basic.TokenUsage{CacheReadTokens: 10}, ""); got != "" {
		t.Errorf("explicit 0 cache read price reported as missing: %q", got)
	}
	if got := missingSubitemKeys(entry, &bfe_basic.TokenUsage{CacheWriteTokens: 10}, ""); got != cluster_conf.PriceCacheCreationInputTokenCost {
		t.Errorf("missing cache write key = %q, want %q", got, cluster_conf.PriceCacheCreationInputTokenCost)
	}
	if got := missingSubitemKeys(entry, &bfe_basic.TokenUsage{PromptTokens: 10}, ""); got != "" {
		t.Errorf("unused dimensions must not be checked, got %q", got)
	}
	if got := missingSubitemKeys(nil, &bfe_basic.TokenUsage{CacheReadTokens: 10}, ""); got != "" {
		t.Errorf("nil entry must return empty, got %q", got)
	}
}

// Issue #1479: the "reject" action rejects a request whose (model, mode) has
// no price entry before forwarding, counts PriceRejected and marks the shape;
// configured prices, other actions, exclusions and a broken cluster lookup all
// pass through.
func TestRejectIfPriceMissing(t *testing.T) {
	clusterName := "responses-backup"
	model := "gpt-5-codex"

	setup := func(action string) (*ModuleAITokenAuth, *bfe_basic.Request, *bfe_cluster.BfeCluster) {
		req := newTestRequestWithCluster("ak-123", "AI_product", clusterName, model)
		cluster := buildTestClusterConf(model, 0.000003, 0.000009)
		cluster.AIConf.PriceMissAction = action
		req.SvrDataConf = &mockServerDataConf{clusters: map[string]*bfe_cluster.BfeCluster{clusterName: cluster}}
		req.GetAiBasicInfo().Mode = bfe_basic.ModeResponses
		SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id"}, 0, nil)
		m := NewModuleAITokenAuth()
		return m, req, cluster
	}

	t.Run("reject on missing mode price", func(t *testing.T) {
		m, req, _ := setup(cluster_conf.PriceMissActionReject)
		rejected, resp := m.rejectIfPriceMissing(req)
		if !rejected || resp == nil {
			t.Fatalf("expected rejection, got rejected=%v resp=%v", rejected, resp)
		}
		if resp.StatusCode != 503 {
			t.Errorf("status = %d, want 503", resp.StatusCode)
		}
		if v := m.state.PriceRejected.Get(); v != 1 {
			t.Errorf("PriceRejected = %d, want 1", v)
		}
		if got := req.GetAiBasicInfo().PriceWarn(); got != bfe_basic.PriceWarnReject {
			t.Errorf("PriceWarn = %q, want %q", got, bfe_basic.PriceWarnReject)
		}
		if !req.GetAiBasicInfo().IsPriceMiss() {
			t.Error("expected ai_price_miss to be marked on rejection")
		}
	})

	t.Run("model table missing also rejects", func(t *testing.T) {
		m, req, cluster := setup(cluster_conf.PriceMissActionReject)
		cluster.AIConf.ModelTable = nil
		if rejected, _ := m.rejectIfPriceMissing(req); !rejected {
			t.Error("expected rejection when the model table is missing")
		}
	})

	t.Run("configured price passes", func(t *testing.T) {
		m, req, _ := setup(cluster_conf.PriceMissActionReject)
		req.GetAiBasicInfo().Mode = bfe_basic.ModeChat // chat entry exists
		if rejected, _ := m.rejectIfPriceMissing(req); rejected {
			t.Error("configured price must not be rejected")
		}
	})

	t.Run("other actions pass", func(t *testing.T) {
		for _, action := range []string{cluster_conf.PriceMissActionFallbackChat, cluster_conf.PriceMissActionBillZero, ""} {
			m, req, _ := setup(action)
			if rejected, _ := m.rejectIfPriceMissing(req); rejected {
				t.Errorf("action %q must not reject", action)
			}
		}
	})

	t.Run("count_tokens endpoint passes", func(t *testing.T) {
		m, req, _ := setup(cluster_conf.PriceMissActionReject)
		req.HttpRequest.RequestURI = "/v1/responses/count_tokens"
		if rejected, _ := m.rejectIfPriceMissing(req); rejected {
			t.Error("count_tokens endpoints must not be rejected")
		}
	})

	t.Run("cluster lookup failure fails open", func(t *testing.T) {
		m, req, _ := setup(cluster_conf.PriceMissActionReject)
		req.SvrDataConf = &mockServerDataConf{} // cluster not found -> no AIConf
		// Re-capture the context so the auth context sees the new conf.
		SetTokenAuthContext(req, &Token{Key: "ak-123", KeyId: "ak-123-id"}, 0, nil)
		if rejected, _ := m.rejectIfPriceMissing(req); rejected {
			t.Error("a broken cluster lookup must fail open, not reject")
		}
	})
}

// Issue #1479: PRICE_MISS warns are rate limited per (cluster, model, mode,
// shape) window while the first occurrence is always emitted.
func TestPriceMissWarnLimiter(t *testing.T) {
	now := time.Now()
	l := &priceMissWarnLimiter{last: make(map[string]time.Time)}

	if !l.allow("c|m|responses|fallback", now) {
		t.Fatal("first occurrence must be allowed")
	}
	if l.allow("c|m|responses|fallback", now.Add(30*time.Second)) {
		t.Error("same key inside the window must be suppressed")
	}
	if !l.allow("c|m|responses|fallback", now.Add(priceMissWarnWindow+time.Second)) {
		t.Error("same key after the window must be allowed again")
	}
	if !l.allow("c|m|responses|subitem_missing", now.Add(31*time.Second)) {
		t.Error("a different shape key must be allowed")
	}
}

// Issue #1479: MarkPriceWarn keeps the most specific shape.
func TestMarkPriceWarnPrecedence(t *testing.T) {
	ai := &bfe_basic.AiBasicInfo{}
	ai.MarkPriceWarn(bfe_basic.PriceWarnFallback)
	ai.MarkPriceWarn(bfe_basic.PriceWarnNoEntry) // less specific: ignored
	if got := ai.PriceWarn(); got != bfe_basic.PriceWarnFallback {
		t.Errorf("PriceWarn = %q, want %q", got, bfe_basic.PriceWarnFallback)
	}
	ai.MarkPriceWarn(bfe_basic.PriceWarnSubitemMissing)
	if got := ai.PriceWarn(); got != bfe_basic.PriceWarnSubitemMissing {
		t.Errorf("PriceWarn = %q, want %q", got, bfe_basic.PriceWarnSubitemMissing)
	}
}
