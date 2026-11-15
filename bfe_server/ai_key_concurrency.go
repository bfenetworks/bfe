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

package bfe_server

import (
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
	"github.com/bfenetworks/go-lib/log"
)

// Per-Key concurrent session limit (AIConf.MaxConcurrencyPerKey).
//
// The limit is enforced as a Redis-global slot set shared by all BFE
// instances: each upstream Key owns one ZSet whose members are session ids
// (AiBasicInfo.ClientKeyId). A session occupies one slot from binding until
// the binding idle timeout (SessionAffinityTTL, refreshed on every hit) or
// until an unbind on failure (ZREM) — mirroring the session-affinity
// binding lifecycle on purpose: releasing on "last in-flight request
// finished" would let sessions ping-pong between Keys and defeat the
// prompt-cache affinity. The acquire script is atomic (TIME-based expiry,
// cap check, insert/refresh) so the cap is exact across instances.
//
// All Redis failures fail open (request allowed, ReqAiKeyConcurrencyRedisErr
// incremented): a Redis outage must not turn into a business outage.

// aiKeyConcurrencyAcquireScript: KEYS[1] = slot ZSet;
// ARGV[1] = sessionID, ARGV[2] = limit, ARGV[3] = ttl_ms.
// Returns 1 when the session holds a slot (existing member refreshed, or
// new member inserted under the cap), 0 when the set is full.
var aiKeyConcurrencyAcquireScript = `
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now)
local ttl = tonumber(ARGV[3])
local expireAt = now + ttl
if redis.call('ZSCORE', KEYS[1], ARGV[1]) then
    redis.call('ZADD', KEYS[1], expireAt, ARGV[1])
    redis.call('PEXPIRE', KEYS[1], ttl * 2)
    return 1
end
if redis.call('ZCARD', KEYS[1]) < tonumber(ARGV[2]) then
    redis.call('ZADD', KEYS[1], expireAt, ARGV[1])
    redis.call('PEXPIRE', KEYS[1], ttl * 2)
    return 1
end
return 0
`

// aiKeyConcurrencyReleaseScript removes one session member (ZREM) and keeps
// the key TTL alive; the whole-key PEXPIRE is only a crash-residue guard.
// KEYS[1] = slot ZSet; ARGV[1] = sessionID, ARGV[2] = ttl_ms.
var aiKeyConcurrencyReleaseScript = `
if redis.call('ZREM', KEYS[1], ARGV[1]) > 0 then
    redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[2]) * 2)
end
return 0
`

// aiKeyConcAnonSeq generates identities for the defensive anonymous path
// (empty ClientKeyId): each such request occupies a request-level slot that
// expires with the score TTL.
var aiKeyConcAnonSeq uint64

// aiKeyConcurrencyRedisKey returns the Redis key of the per-Key slot ZSet.
func aiKeyConcurrencyRedisKey(clusterName, keyName, prefix string) string {
	return fmt.Sprintf("%s:conc:%s:%s", prefix, clusterName, keyName)
}

// acquireAIKeySessionSlot is non-blocking. It returns false only when the
// Key's session slots are all occupied; any Redis failure fails open (true)
// and is counted on ReqAiKeyConcurrencyRedisErr.
func acquireAIKeySessionSlot(client redis_client.Client, clusterName, keyName, sessionID, prefix string,
	limit, ttlSeconds int, proxyState *ProxyState) bool {
	if limit <= 0 || client == nil {
		return true
	}
	if sessionID == "" {
		sessionID = fmt.Sprintf("anon-%d", atomic.AddUint64(&aiKeyConcAnonSeq, 1))
	}
	if ttlSeconds <= 0 {
		// defensive: AIKeyPolicyCheck fills the default (600)
		ttlSeconds = 600
	}
	key := aiKeyConcurrencyRedisKey(clusterName, keyName, prefix)
	res, err := client.NewScript(aiKeyConcurrencyAcquireScript).
		Run(key, sessionID, limit, ttlSeconds*1000)
	if err != nil {
		if proxyState != nil {
			proxyState.ReqAiKeyConcurrencyRedisErr.Inc(1)
		}
		log.Logger.Warn("aiKeyConcurrency: acquire slot key[%s] error[%v], fail-open", keyName, err)
		return true
	}
	allowed, ok := res.(int64)
	if !ok {
		if proxyState != nil {
			proxyState.ReqAiKeyConcurrencyRedisErr.Inc(1)
		}
		log.Logger.Warn("aiKeyConcurrency: unexpected acquire reply %v (%T), fail-open", res, res)
		return true
	}
	return allowed == 1
}

// releaseAIKeySessionSlot frees the session slot on failure-driven unbind
// (rotate on 429, dead on 401/402/403, batch 404). Errors are counted but
// never propagated: the score TTL is the recovery net for a lost ZREM.
func releaseAIKeySessionSlot(client redis_client.Client, clusterName, keyName, sessionID, prefix string,
	ttlSeconds int, proxyState *ProxyState) {
	if client == nil || sessionID == "" {
		return
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 600
	}
	key := aiKeyConcurrencyRedisKey(clusterName, keyName, prefix)
	if _, err := client.NewScript(aiKeyConcurrencyReleaseScript).Run(key, sessionID, ttlSeconds*1000); err != nil {
		if proxyState != nil {
			proxyState.ReqAiKeyConcurrencyRedisErr.Inc(1)
		}
		log.Logger.Warn("aiKeyConcurrency: release slot key[%s] error[%v]", keyName, err)
	}
}

// allEligibleKeysSlotFull reports whether every selectable Key (weight > 0,
// not dead) has had its session slot refused for this request. It backs the
// synthetic-429 exhaustion contract (issue #1440): a full cap must yield a
// decidable result, not fall out of the retry loop with (nil, nil).
func allEligibleKeysSlotFull(keys []cluster_conf.AIKey, state *aiKeyAttemptState, slotFull map[int]struct{}) bool {
	for i, k := range keys {
		if k.Weight == 0 {
			continue
		}
		if _, dead := state.deadSet[i]; dead {
			continue
		}
		if _, full := slotFull[i]; !full {
			return false
		}
	}
	return true
}

// newNoAvailableKey429Resp builds the synthetic 429 (NO_AVAILABLE_KEY)
// shared by key exhaustion and the per-Key session-slot cap. Contract
// (issues #1440/#1441): ErrCode must be ErrAiNoAvailableKey (NOT
// ErrAiRateLimit, which would suppress cluster fallback); the unified
// AiError body marks the response as gateway-generated (X-Bfe-Gw-Error), so
// normalizeUpstreamError leaves it alone. retryAfter is a worst-case wait
// hint (penalty TTL for exhausted Keys, binding idle timeout for a full
// cap), not an exact remaining time.
func newNoAvailableKey429Resp(basicReq *bfe_basic.Request, model string, retryAfter int, msg string) *bfe_http.Response {
	if retryAfter <= 0 {
		retryAfter = 60
	}
	res := bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeNoAvailableKey, bfe_basic.TypeRateLimitError,
		msg,
		&bfe_basic.AiErrorDetail{Model: model, RetryAfterSeconds: retryAfter},
	).CreateErrorResponse(basicReq)
	res.Header.Set("Retry-After", strconv.Itoa(retryAfter))
	return res
}
