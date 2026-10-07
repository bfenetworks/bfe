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
	"strconv"
	"time"

	"github.com/gomodule/redigo/redis"

	"github.com/bfenetworks/go-lib/log"
	"github.com/bfenetworks/go-lib/quota"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

// This file implements the post-paid batch quota contract of mod_ai_batch:
// BatchPreCheck/BatchReserve at batch creation, and the settle/release
// execution at HandleRequestFinish driven by the AiBasicInfo contract
// (BatchSettle/BatchSettleId/BatchUsageByModel).
//
// Redis layout (same cluster as quota keys):
//
//	BATCH_RESERVE:<planRedisKey>   per-plan reserve mirror (INCRBY/DECRBY);
//	                               available = balance - mirror
//	BATCH_RESERVE_BATCH:<batchId>  HASH planRedisKey -> reserved units, so
//	                               release does not need the original amounts
//	BATCH_SETTLED:<settleKey>      SET NX EX idempotency (settle/release)
//
// All operations are best-effort fail-open with counters; the control-plane
// reconcile job (ai-gateway-api) compares mirrors against its task table.

const (
	batchReserveKeyPrefix      = "BATCH_RESERVE:"
	batchReserveBatchKeyPrefix = "BATCH_RESERVE_BATCH:"
	batchSettledKeyPrefix      = "BATCH_SETTLED:"
	batchReserveTTL            = 172800 // 48h, matches mod_ai_batch state TTL
)

var (
	luaReserveIncr = `
local n = redis.call('INCRBY', KEYS[1], tonumber(ARGV[1]))
redis.call('EXPIRE', KEYS[1], ARGV[2])
return n
`
	luaReserveDecr = `
local n = redis.call('GET', KEYS[1])
if n == false then return 0 end
local cur = tonumber(n)
local d = math.min(cur, tonumber(ARGV[1]))
if d > 0 then redis.call('DECRBY', KEYS[1], d) end
return d
`
	luaHashSetOneWithTtl = `
redis.call('HSET', KEYS[1], ARGV[1], ARGV[2])
redis.call('EXPIRE', KEYS[1], ARGV[3])
return 1
`
	luaHashGetAll = `
return redis.call('HGETALL', KEYS[1])
`
	luaSetNxWithTtl = `
return redis.call('SET', KEYS[1], '1', 'NX', 'EX', ARGV[1])
`
	luaHashDel = `
return redis.call('DEL', KEYS[1])
`
)

func batchReserveMirrorKey(planRedisKey string) string {
	return batchReserveKeyPrefix + planRedisKey
}

func batchReserveBatchKey(batchId string) string {
	return batchReserveBatchKeyPrefix + batchId
}

func batchSettledKey(settleKey string) string {
	return batchSettledKeyPrefix + settleKey
}

// reserveUnitsForPlan picks the reserve amount matching the plan's unit.
func reserveUnitsForPlan(plan *QuotaPlan, rmbUnits, tokenUnits int64) int64 {
	if quota.IsRMB(plan.Unit) {
		return rmbUnits
	}
	return tokenUnits
}

// BatchPreCheck reports whether every bound quota plan (except PassNoQuota
// and unlimited ones) can cover the reserve on top of already reserved
// amounts: available = balance - reserveMirror.
func BatchPreCheck(req *bfe_basic.Request, rmbUnits, tokenUnits int64) bool {
	m := defaultTokenAuthModule
	if m == nil || m.redisClient == nil {
		return true // fail-open: without redis there is no balance to check
	}
	ctx := GetTokenAuthContext(req)
	if ctx == nil || ctx.Token == nil {
		return true
	}
	for _, plan := range ctx.Token.QuotaPlans {
		if plan.Unlimited || plan.PassNoQuota {
			continue
		}
		units := reserveUnitsForPlan(plan, rmbUnits, tokenUnits)
		if units <= 0 {
			continue
		}
		ok, balance, err := plan.HasBalance(m.redisClient)
		if err != nil || !ok {
			if openDebug {
				log.Logger.Debug("mod_ai_token_auth: batch precheck no balance, plan=%s err=%v", plan.Id, err)
			}
			return false
		}
		reserved, err := m.redisClient.GetInt64(batchReserveMirrorKey(plan.RedisKey))
		if err != nil && !redis_client.IsKeyNotFound(err) {
			continue // fail-open on mirror read errors
		}
		if balance-reserved < units {
			return false
		}
	}
	return true
}

// BatchReserve writes the per-plan reserve mirrors and the per-batch reserve
// record after the upstream accepted the batch creation. Returns false when
// nothing could be reserved (recorded anyway by the caller; settle bills the
// actual usage).
func BatchReserve(req *bfe_basic.Request, batchId string, rmbUnits, tokenUnits int64) bool {
	m := defaultTokenAuthModule
	if m == nil || m.redisClient == nil || batchId == "" {
		return false
	}
	ctx := GetTokenAuthContext(req)
	if ctx == nil || ctx.Token == nil {
		return false
	}
	any := false
	for _, plan := range ctx.Token.QuotaPlans {
		if plan.Unlimited {
			continue
		}
		units := reserveUnitsForPlan(plan, rmbUnits, tokenUnits)
		if units <= 0 {
			continue
		}
		_, err := m.redisClient.NewScript(luaReserveIncr).
			Run(batchReserveMirrorKey(plan.RedisKey), units, batchReserveTTL)
		if err != nil {
			log.Logger.Warn("mod_ai_token_auth: batch reserve mirror err, plan=%s: %v", plan.Id, err)
			continue
		}
		_, err = m.redisClient.NewScript(luaHashSetOneWithTtl).
			Run(batchReserveBatchKey(batchId), plan.RedisKey, units, batchReserveTTL)
		if err != nil {
			log.Logger.Warn("mod_ai_token_auth: batch reserve record err, batch=%s: %v", batchId, err)
			continue
		}
		any = true
	}
	return any
}

// BatchRelease releases the reserve mirrors of a batch (terminal status or
// accepted cancel). Safe to call multiple times: the second call finds no
// reserve record. Reports whether a reserve record existed.
func BatchRelease(batchId string) bool {
	m := defaultTokenAuthModule
	if m == nil || m.redisClient == nil || batchId == "" {
		return false
	}
	key := batchReserveBatchKey(batchId)
	vals, err := m.redisClient.HGetAll(key)
	if err != nil {
		log.Logger.Warn("mod_ai_token_auth: batch release read err, batch=%s: %v", batchId, err)
		return false
	}
	if len(vals) == 0 {
		return false
	}
	for planKey, unitsStr := range vals {
		units, _ := strconv.ParseInt(unitsStr, 10, 64)
		if units <= 0 {
			continue
		}
		_, err := m.redisClient.NewScript(luaReserveDecr).
			Run(batchReserveMirrorKey(planKey), units)
		if err != nil {
			log.Logger.Warn("mod_ai_token_auth: batch release mirror err, plan=%s: %v", planKey, err)
		}
	}
	if _, err := m.redisClient.NewScript(luaHashDel).Run(key); err != nil {
		log.Logger.Warn("mod_ai_token_auth: batch release del err, batch=%s: %v", batchId, err)
	}
	// occupy the settlement slot: a released batch must never be settled
	// later (e.g. by the reconcile backstop)
	if _, err := m.redisClient.NewScript(luaSetNxWithTtl).
		Run(batchSettledKey(batchId), batchReserveTTL); err != nil {
		log.Logger.Warn("mod_ai_token_auth: batch release marker err, batch=%s: %v", batchId, err)
	}
	return true
}

// batchSettle executes the settlement at download: idempotency claim, per
// model group pricing at mode=batch rates, quota deduction and reserve
// release.
func (m *ModuleAITokenAuth) batchSettle(req *bfe_basic.Request, ctx *TokenAuthContext) {
	aiMeta := ctx.aiBasicInfo
	settleKey := aiMeta.BatchSettleId
	if settleKey == "" {
		return
	}

	// claim the settlement slot; a reconcile-job backstop or a concurrent
	// path may already own it
	reply, err := m.redisClient.NewScript(luaSetNxWithTtl).
		Run(batchSettledKey(settleKey), batchReserveTTL)
	if err != nil {
		log.Logger.Warn("mod_ai_token_auth: batch settle claim err, key=%s: %v", settleKey, err)
		return
	}
	s, _ := redis.String(reply, nil)
	if s != "OK" {
		if openDebug {
			log.Logger.Debug("mod_ai_token_auth: batch settle already claimed, key=%s", settleKey)
		}
		return
	}

	// price per model group at batch rates
	clusterName := req.Route.ClusterName
	var costUnits int64
	var totalTokens int64
	usageByModel := aiMeta.BatchUsageByModel
	if len(usageByModel) > 0 && ctx.serverConf != nil && clusterName != "" {
		cluster, lerr := ctx.serverConf.ClusterTableLookup(clusterName)
		if lerr == nil && cluster != nil && cluster.AIConf != nil && cluster.AIConf.ModelTable != nil {
			tierName := cluster.AIConf.ModelTable.ActiveTierName(time.Now())
			for model, usage := range usageByModel {
				u := usage
				entry := cluster_conf.LookupModelPrice(cluster.AIConf.ModelTable, model, bfe_basic.ModeBatch)
				if entry == nil {
					m.state.PriceLookupMiss.Inc(1)
					log.Logger.Warn("mod_ai_token_auth: batch price miss, cluster=%s model=%s mode=%s",
						clusterName, model, bfe_basic.ModeBatch)
					continue
				}
				costUnits += calcChatCost(entry, &u, tierName)
				totalTokens += u.UsedQuota
			}
		}
	}

	// deduct like a normal request; reserve mirrors are released regardless
	for _, plan := range ctx.Token.QuotaPlans {
		if plan.Unlimited {
			continue
		}
		if quota.IsRMB(plan.Unit) {
			if costUnits > 0 {
				if _, err := plan.Deduct(m.redisClient, costUnits); err != nil {
					log.Logger.Warn("mod_ai_token_auth: batch settle deduct rmb err: %v", err)
				}
			}
		} else if totalTokens > 0 {
			if _, err := plan.Deduct(m.redisClient, totalTokens); err != nil {
				log.Logger.Warn("mod_ai_token_auth: batch settle deduct token err: %v", err)
			}
		}
	}
	BatchRelease(settleKey)

	// surface the settled amounts in the access log fields
	aiMeta.BatchSettle = bfe_basic.BatchSettleSettle
	tu := aiMeta.GetTokenUsage()
	tu.UsedCost = costUnits
	tu.UsedQuota = totalTokens
}

// batchContractHandler executes the settle/release contract published by
// mod_ai_batch in AiBasicInfo; it runs at the top of the finish handler so
// that the batch path never falls into the standard usage pricing below.
func (m *ModuleAITokenAuth) batchContractHandler(req *bfe_basic.Request, ctx *TokenAuthContext) bool {
	settle := ctx.aiBasicInfo.BatchSettle
	if settle == "" || settle == bfe_basic.BatchSettleNone {
		return false
	}
	ctx.deducted = true
	switch settle {
	case bfe_basic.BatchSettleSettle:
		m.batchSettle(req, ctx)
	case bfe_basic.BatchSettleRelease:
		BatchRelease(ctx.aiBasicInfo.BatchSettleId)
	}
	return true
}
