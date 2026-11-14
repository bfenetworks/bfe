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
	"strings"
	"sync"
	"time"

	"github.com/bfenetworks/go-lib/log"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
)

// priceMissWarnWindow bounds how often one missing-price shape is logged per
// (cluster, model, mode, type). A missing price is a configuration-state
// event, not a per-request one: the counter and the ai_price_miss /
// ai_price_warn access-log fields already carry it per request, so a warn per
// request would only track traffic volume (issue #1479).
const priceMissWarnWindow = 60 * time.Second

// priceMissWarnLimiter is the in-process rate limiter of PRICE_MISS warns.
type priceMissWarnLimiter struct {
	mu   sync.Mutex
	last map[string]time.Time
}

var priceMissWarns = &priceMissWarnLimiter{last: make(map[string]time.Time)}

// allow reports whether a warn for key may be emitted now, and records it.
func (l *priceMissWarnLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if t, ok := l.last[key]; ok && now.Sub(t) < priceMissWarnWindow {
		return false
	}
	l.last[key] = now
	return true
}

// logPriceMiss emits the structured missing-price line (issue #1479):
//
//	PRICE_MISS type=<shape> cluster=<c> model=<m> mode=<mode> action=<a> missing_keys=<k1,k2>
//
// The fixed "PRICE_MISS" tag and the stable key=value tokens make the line
// directly greppable and parseable; shape mirrors ai_price_warn so logs and
// access records share one vocabulary. Rate limited per shape key; the first
// occurrence of a shape is always printed immediately.
func (m *ModuleAITokenAuth) logPriceMiss(cluster, model, mode, shape, action, missingKeys string) {
	key := strings.Join([]string{cluster, model, mode, shape}, "|")
	if !priceMissWarns.allow(key, time.Now()) {
		return
	}

	if action == "" {
		action = "-"
	}
	if missingKeys == "" {
		missingKeys = "-"
	}
	log.Logger.Warn("PRICE_MISS type=%s cluster=%s model=%s mode=%s action=%s missing_keys=%s",
		shape, cluster, model, mode, action, missingKeys)
}

// missingSubitemKeys returns the sub-item price keys that this request
// actually used but the billed entry does not configure (issue #1479), as a
// comma separated list ("" when nothing is missing).
//
// Only dimensions with usage are checked — a missing key for an unused
// dimension is not a gap. Key presence, not value, is the discriminator
// (ModelPrice.HasPrice, the same convention issue #1420 uses): an explicit 0
// is a priced dimension and is not reported, an absent key is.
func missingSubitemKeys(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tier string) string {
	if entry == nil || usage == nil {
		return ""
	}

	var missing []string
	check := func(used int64, keys ...string) {
		if used <= 0 {
			return
		}
		for _, key := range keys {
			if !entry.HasPrice(tier, key) {
				missing = append(missing, key)
			}
		}
	}

	check(usage.CacheReadTokens, cluster_conf.PriceCacheReadInputTokenCost)
	check(usage.CacheWriteTokens, cluster_conf.PriceCacheCreationInputTokenCost)
	check(usage.CacheWriteTokens1h, cluster_conf.PriceCacheCreationInputTokenCost1h)

	return strings.Join(missing, ",")
}

// priceMissActionOf returns the effective PriceMissAction of a cluster,
// defaulting to fallback_chat for clusters that were never normalized
// (AIConfCheck normalizes on load; this keeps the billing path defensive).
func priceMissActionOf(aiConf *cluster_conf.AIConf) string {
	if aiConf == nil || aiConf.PriceMissAction == "" {
		return cluster_conf.PriceMissActionFallbackChat
	}
	return aiConf.PriceMissAction
}
