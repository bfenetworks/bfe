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

package limit_rate

import (
	"sync/atomic"
	"time"
)

type ConcurrencyLimiter struct {
	redisKey      string
	connThreshold atomic.Int64
	ttl           int64 //ttl after last access
}

func NewConcurrencyLimiter(redisKey string, connThreshold int64, ttl int64) *ConcurrencyLimiter {
	ret := &ConcurrencyLimiter{
		redisKey: redisKey,
		ttl:      ttl,
	}
	ret.connThreshold.Store(connThreshold)
	return ret
}

func (l *ConcurrencyLimiter) ConnAcquire(agent *RedisLRAgent) (bool, int64, int64, error) {
	curCount := int64(0)
	connThreshold := l.connThreshold.Load()
	//fast path
	if connThreshold < 0 {
		return true, time.Now().Unix(), -1, nil
	}
	if connThreshold == 0 {
		return false, time.Now().Unix(), curCount, nil
	}

	requireCount := int64(1)
	if requireCount > connThreshold {
		return false, time.Now().Unix(), curCount, nil
	}

	//check remote
	currentTime, isAllowed, curCount, err := agent.ConAcquire(l.redisKey, connThreshold, l.ttl)
	if err != nil {
		return false, currentTime, curCount, err
	}

	return isAllowed, currentTime, curCount, nil
}

func (l *ConcurrencyLimiter) ConnRelease(agent *RedisLRAgent) (int64, error) {
	curCount, err := agent.ConRelease(l.redisKey, l.ttl)
	if err != nil {
		return curCount, err
	}

	return curCount, nil
}

func (l *ConcurrencyLimiter) UpdateArgs(connThreshold int64) {
	l.connThreshold.Store(connThreshold)
}
