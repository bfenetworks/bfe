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

type QPMLimiter struct {
	redisKey string
	burst    atomic.Int64
	period   atomic.Int64 // fixed 60 for QPM
	limit    atomic.Int64
}

func NewQPMLimiter(redisKey string, burst int64, period int64, limit int64) *QPMLimiter {
	ret := &QPMLimiter{
		redisKey: redisKey,
	}
	ret.burst.Store(burst)
	ret.period.Store(period)
	ret.limit.Store(limit)
	return ret
}

func (l *QPMLimiter) Check(reqToConsume int64, agent *RedisLRAgent) (bool, int64, float64, error) {
	tat := float64(0)
	limit := l.limit.Load()
	burst := l.burst.Load()
	period := l.period.Load()

	if reqToConsume <= 0 {
		return true, time.Now().Unix(), tat, nil
	}

	//fast path
	if limit < 0 {
		return true, time.Now().Unix(), tat, nil
	}
	if limit == 0 {
		return false, time.Now().Unix(), tat, nil
	}

	//check remote
	currentTime, isAllowed, tat, err := agent.QpmCheck(l.redisKey, burst, period, limit, reqToConsume)
	if err != nil {
		return false, currentTime, tat, err
	}

	return isAllowed, currentTime, tat, nil
}

func (l *QPMLimiter) UpdateArgs(burst int64, period int64, limit int64) {
	l.burst.Store(burst)
	l.period.Store(period)
	l.limit.Store(limit)
}
