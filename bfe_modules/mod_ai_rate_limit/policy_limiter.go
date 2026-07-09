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
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_util/limit_rate"
)

const (
	concurrencyLimiterTTL = 300 // 5 minutes TTL for concurrency limiter keys
)

type tpmLimiterItem struct {
	limiter     *limit_rate.TPMLimiter
	name        string
	ReservedX   float64
	ReservedOff float64
	Models      []string
	matchCount  atomic.Uint64
	hitCount    atomic.Uint64
	tokenCount  atomic.Uint64
}

func (r *tpmLimiterItem) predictTokenUsage(promptToken int64) int64 {
	return int64(r.ReservedOff + r.ReservedX*float64(promptToken))
}

type rpmLimiterItem struct {
	limiter    *limit_rate.QPMLimiter
	name       string
	Models     []string
	matchCount atomic.Uint64
	hitCount   atomic.Uint64
}

type conLimiterItem struct {
	conLimiter *limit_rate.ConcurrencyLimiter
	matchCount atomic.Uint64
	hitCount   atomic.Uint64
}

type policyLimiterSet struct {
	policyId    string
	tpmLimiters []*tpmLimiterItem
	rpmLimiters []*rpmLimiterItem
	conLimiter  *conLimiterItem
}

type policyLimiterManager struct {
	lock     sync.Mutex
	limiters map[string]*policyLimiterSet // policyId => limiter set
}

func newPolicyLimiterManager() *policyLimiterManager {
	return &policyLimiterManager{
		limiters: make(map[string]*policyLimiterSet),
	}
}

func (m *policyLimiterManager) getLimiterPolicySet(policyId string) *policyLimiterSet {

	m.lock.Lock()
	defer m.lock.Unlock()

	if ls, ok := m.limiters[policyId]; ok {
		return ls
	}

	return nil
}

func buildRedisKey(policyId string, suffix string) string {
	return fmt.Sprintf("%s_%s_%s", "default_bfe", policyId, suffix)
}

func buildTpmInstId(rule *TPMRuleConf) string {
	if rule.Name != "" {
		return rule.Name
	}
	return fmt.Sprintf("tpm_%d_%d_%d_%d", rule.TimeWindow, rule.Threshold, rule.BucketTimeWindow, rule.BucketThreshold)
}

func buildRpmInstId(rule *RPMRuleConf) string {
	if rule.Name != "" {
		return rule.Name
	}
	return fmt.Sprintf("rpm_%d_%d_%d", rule.TimeWindow, rule.MaxRequests, rule.Burst)
}

func newTpmLimiterItem(limiter *limit_rate.TPMLimiter, tpmInstId string, rule *TPMRuleConf) *tpmLimiterItem {
	return &tpmLimiterItem{
		limiter:     limiter,
		name:        tpmInstId,
		ReservedX:   rule.ReservedX,
		ReservedOff: rule.ReservedOff,
		Models:      rule.Models,
	}
}

func newRpmLimiterItem(limiter *limit_rate.QPMLimiter, rpmInstId string, rule *RPMRuleConf) *rpmLimiterItem {
	return &rpmLimiterItem{
		limiter: limiter,
		name:    rpmInstId,
		Models:  rule.Models,
	}
}

func newPolicyLimiterSet(policyId string, policy *PolicyConf) *policyLimiterSet {
	ps := &policyLimiterSet{
		policyId: policyId,
	}

	if policy.Rules == nil {
		return ps
	}

	for _, rule := range policy.Rules.TPM {
		tpmInstId := buildTpmInstId(rule)
		redisKey := buildRedisKey(policyId, fmt.Sprintf("tpm_%s", tpmInstId))
		limiter := limit_rate.NewTPMLimiter(
			redisKey,
			rule.Threshold,
			rule.TimeWindow,
			rule.BucketThreshold,
			rule.BucketTimeWindow,
		)
		ps.tpmLimiters = append(ps.tpmLimiters, newTpmLimiterItem(limiter, tpmInstId, rule))
	}

	for _, rule := range policy.Rules.RPM {
		rpmInstId := buildRpmInstId(rule)
		redisKey := buildRedisKey(policyId, fmt.Sprintf("rpm_%s", rpmInstId))
		limiter := limit_rate.NewQPMLimiter(
			redisKey,
			rule.Burst,
			int64(rule.TimeWindow),
			rule.MaxRequests,
		)
		ps.rpmLimiters = append(ps.rpmLimiters, newRpmLimiterItem(limiter, rpmInstId, rule))
	}

	if policy.Rules.MaxConcurrency != nil {
		redisKey := buildRedisKey(policyId, "con")
		ps.conLimiter = &conLimiterItem{
			conLimiter: limit_rate.NewConcurrencyLimiter(
				redisKey,
				*policy.Rules.MaxConcurrency,
				concurrencyLimiterTTL,
			),
		}
	}

	return ps
}

func (ps *policyLimiterSet) updateCount(old *policyLimiterSet) {
	oldTpmByName := make(map[string]*tpmLimiterItem, len(old.tpmLimiters))
	for _, item := range old.tpmLimiters {
		oldTpmByName[item.name] = item
	}
	for _, item := range ps.tpmLimiters {
		if oldItem, ok := oldTpmByName[item.name]; ok {
			item.matchCount.Add(oldItem.matchCount.Load())
			item.hitCount.Add(oldItem.hitCount.Load())
			item.tokenCount.Add(oldItem.tokenCount.Load())
		}
	}

	oldRpmByName := make(map[string]*rpmLimiterItem, len(old.rpmLimiters))
	for _, item := range old.rpmLimiters {
		oldRpmByName[item.name] = item
	}

	for _, item := range ps.rpmLimiters {
		if oldItem, ok := oldRpmByName[item.name]; ok {
			item.matchCount.Add(oldItem.matchCount.Load())
			item.hitCount.Add(oldItem.hitCount.Load())
		}
	}

	if ps.conLimiter != nil && old.conLimiter != nil {
		ps.conLimiter.matchCount.Add(old.conLimiter.matchCount.Load())
		ps.conLimiter.hitCount.Add(old.conLimiter.hitCount.Load())
	}
}

func (m *policyLimiterManager) updateLimiters(ratePolicies map[string]*PolicyConf) {
	oldLimiters := m.limiters

	newLimiters := make(map[string]*policyLimiterSet, len(ratePolicies))
	for policyId, policy := range ratePolicies {
		new_item := newPolicyLimiterSet(policyId, policy)
		newLimiters[policyId] = new_item
	}

	m.lock.Lock()
	m.limiters = newLimiters

	for policyId, new_item := range newLimiters {
		if old_item, ok := oldLimiters[policyId]; ok {
			new_item.updateCount(old_item)
		}
	}
	m.lock.Unlock()
}

type TpmLimiterData struct {
	Limiter         *limit_rate.TPMLimiter
	PreConsumeToken int64
	BucketTimeSec   int64
	IsAllowed       bool
	Item            *tpmLimiterItem
}

type PolicyLimiterContext struct {
	TpmLimiterDataList []*TpmLimiterData
	ConLimiters        []*limit_rate.ConcurrencyLimiter
}

func (ls *policyLimiterSet) checkTPM(req *bfe_basic.Request, meta *bfe_basic.AiBasicInfo, agent *limit_rate.RedisLRAgent, ctx *PolicyLimiterContext, model string, isRejectOnRedisError bool) bool {
	tokenUsage := meta.GetTokenUsage()

	for i, item := range ls.tpmLimiters {
		if !matchModel(item.Models, model) {
			if openDebug {
				log.Logger.Debug("policyLimiterSet: checkTPM, limiter name[%s] models[%v] != req Model[%s], skip", item.name, item.Models, model)
			}
			continue
		}
		item.matchCount.Add(1)
		tokenConsume := item.predictTokenUsage(tokenUsage.PromptTokens)
		isAllowed, currentTime, preconsumeToken, err := item.limiter.TryCheck(tokenConsume, agent)

		tpmData := &TpmLimiterData{
			Limiter:         item.limiter,
			PreConsumeToken: preconsumeToken,
			BucketTimeSec:   currentTime,
			IsAllowed:       isAllowed,
			Item:            item,
		}
		ctx.TpmLimiterDataList = append(ctx.TpmLimiterDataList, tpmData)

		if err != nil {
			log.Logger.Warn("mod_ai_rate_limit: TPM redis error, policyId:%s, isRejectOnRedisError:%v, err:%v", ls.policyId, isRejectOnRedisError, err)
			if isRejectOnRedisError {
				hitInfo := req.GetAiRateLimitHitInfo()
				policyHitInfo := hitInfo.GetPolicyHitInfo(ls.policyId)
				policyHitInfo.IsRedisError = item.name
				return false
			} else {
				return true
			}
		}

		if !isAllowed {
			item.hitCount.Add(1)

			if openDebug {
				log.Logger.Debug("mod_ai_rate_limit: TPM denied, policyId:%s, tpmIndex:%d", ls.policyId, i)
			}
			hitInfo := req.GetAiRateLimitHitInfo()
			policyHitInfo := hitInfo.GetPolicyHitInfo(ls.policyId)
			policyHitInfo.TpmRules = append(policyHitInfo.TpmRules, item.name)

			return false
		}
	}

	return true
}

func (ls *policyLimiterSet) checkRPM(req *bfe_basic.Request, meta *bfe_basic.AiBasicInfo, agent *limit_rate.RedisLRAgent, ctx *PolicyLimiterContext, model string, isRejectOnRedisError bool) bool {
	for i, item := range ls.rpmLimiters {
		if !matchModel(item.Models, model) {
			if openDebug {
				log.Logger.Debug("policyLimiterSet: checkRPM, limiter name[%s] models[%v] != req Model[%s], skip", item.name, item.Models, model)
			}
			continue
		}

		item.matchCount.Add(1)
		isAllowed, _, _, err := item.limiter.Check(1, agent)

		if err != nil {
			log.Logger.Warn("mod_ai_rate_limit: RPM redis error, policyId:%s, isRejectOnRedisError:%v, err:%v", ls.policyId, isRejectOnRedisError, err)
			if isRejectOnRedisError {
				hitInfo := req.GetAiRateLimitHitInfo()
				policyHitInfo := hitInfo.GetPolicyHitInfo(ls.policyId)
				policyHitInfo.IsRedisError = item.name
				return false
			} else {
				return true
			}
		}

		if !isAllowed {
			item.hitCount.Add(1)
			if openDebug {
				log.Logger.Debug("mod_ai_rate_limit: RPM denied, policyId:%s, rpmIndex:%d", ls.policyId, i)
			}
			hitInfo := req.GetAiRateLimitHitInfo()
			policyHitInfo := hitInfo.GetPolicyHitInfo(ls.policyId)
			policyHitInfo.RpmRules = append(policyHitInfo.RpmRules, item.name)

			return false
		}
	}

	return true
}

func (ls *policyLimiterSet) checkConcurrency(req *bfe_basic.Request, meta *bfe_basic.AiBasicInfo, agent *limit_rate.RedisLRAgent, ctx *PolicyLimiterContext, model string, isRejectOnRedisError bool) bool {
	if ls.conLimiter == nil {
		return true
	}

	ls.conLimiter.matchCount.Add(1)
	isAllowed, _, _, err := ls.conLimiter.conLimiter.ConnAcquire(agent)

	if err != nil {
		log.Logger.Warn("mod_ai_rate_limit: Concurrency redis error, policyId:%s, isRejectOnRedisError:%v, err:%v", ls.policyId, isRejectOnRedisError, err)
		if isRejectOnRedisError {
			hitInfo := req.GetAiRateLimitHitInfo()
			policyHitInfo := hitInfo.GetPolicyHitInfo(ls.policyId)
			policyHitInfo.IsRedisError = "concurrency"
			return false
		} else {
			return true
		}
	}

	if !isAllowed {
		ls.conLimiter.hitCount.Add(1)
		if openDebug {
			log.Logger.Debug("mod_ai_rate_limit: Concurrency denied, policyId:%s", ls.policyId)
		}

		hitInfo := req.GetAiRateLimitHitInfo()
		policyHitInfo := hitInfo.GetPolicyHitInfo(ls.policyId)
		policyHitInfo.IsConcurrency = true

		return false
	}

	ctx.ConLimiters = append(ctx.ConLimiters, ls.conLimiter.conLimiter)
	return true
}

type LimiterStats struct {
	PolicyId   string
	InstId     string
	MatchCount uint64
	HitCount   uint64
	TokenCount uint64
}

func (m *policyLimiterManager) getLimiterStats() (tpmStats, rpmStats, conStats []LimiterStats) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for policyId, ls := range m.limiters {
		for _, item := range ls.tpmLimiters {
			tpmStats = append(tpmStats, LimiterStats{
				PolicyId:   policyId,
				InstId:     item.name,
				MatchCount: item.matchCount.Load(),
				HitCount:   item.hitCount.Load(),
				TokenCount: item.tokenCount.Load(),
			})
		}
		for _, item := range ls.rpmLimiters {
			rpmStats = append(rpmStats, LimiterStats{
				PolicyId:   policyId,
				InstId:     item.name,
				MatchCount: item.matchCount.Load(),
				HitCount:   item.hitCount.Load(),
			})
		}
		if ls.conLimiter != nil {
			conStats = append(conStats, LimiterStats{
				PolicyId:   policyId,
				InstId:     "concurrency",
				MatchCount: ls.conLimiter.matchCount.Load(),
				HitCount:   ls.conLimiter.hitCount.Load(),
			})
		}
	}

	return
}
