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

package mod_ai_cache

import (
	"time"

	"github.com/bfenetworks/go-lib/log"
	"github.com/bfenetworks/go-lib/web-monitor/module_state2"

	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

// redisCache is a thin wrapper over redis_client.Client for the exact-match
// AI cache. All failures are fail-open: the caller treats any error as a
// cache miss (or a skipped write-back) so that Redis problems never block
// the main request flow.
type redisCache struct {
	name   string
	client redis_client.Client
	table  *cacheRuleTable
	state  *module_state2.State // module state (JSON monitor source)
}

func newRedisCache(name string, client redis_client.Client, table *cacheRuleTable,
	state *module_state2.State) *redisCache {
	return &redisCache{
		name:   name,
		client: client,
		table:  table,
		state:  state,
	}
}

func (c *redisCache) recordLatency(start time.Time) {
	ms := time.Since(start).Milliseconds()
	c.table.addLatencyMs(ms)
	// issue #1499: the module state (JSON monitor source) must mirror the
	// rule table counters (prometheus source), so both endpoints agree.
	if ms > 0 {
		c.state.Inc("LATENCY_MS", int(ms))
	}
}

func (c *redisCache) recordErr() {
	c.table.incRedisErr()
	c.state.Inc("REDIS_ERR", 1)
}

// Get returns (value, true) on cache hit, ("", false) on miss or any error.
func (c *redisCache) Get(key string) (string, bool) {
	start := time.Now()
	defer c.recordLatency(start)

	value, err := c.client.Get(key)
	if err != nil {
		if !redis_client.IsKeyNotFound(err) {
			log.Logger.Warn("%s: redis get failed, key[%s], err[%v]", c.name, key, err)
			c.recordErr()
		}
		return "", false
	}

	if value == nil {
		return "", false
	}

	str, ok := value.(string)
	if !ok {
		if bytes, isBytes := value.([]byte); isBytes {
			return string(bytes), true
		}
		log.Logger.Warn("%s: redis get unexpected value type, key[%s]", c.name, key)
		return "", false
	}

	return str, true
}

// Setex stores the answer with a TTL. Any error is logged and swallowed.
func (c *redisCache) Setex(key string, value []byte, ttl int) {
	start := time.Now()
	defer c.recordLatency(start)

	if err := c.client.Setex(key, value, ttl); err != nil {
		log.Logger.Warn("%s: redis setex failed, key[%s], err[%v]", c.name, key, err)
		c.recordErr()
	}
}
