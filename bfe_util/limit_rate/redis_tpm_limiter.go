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
	"time"
)

// each Window can seperate into buckets
// bucketcount == windowSizeSec/bucketSizeSec
// require:  windowSizeSec % bucketSizeSec == 0
type TPMLimiter struct {
	key                string
	maxTokensPerWindow int64
	windowSizeSec      int64
	maxTokensPerBucket int64
	bucketSizeSec      int64
}

func NewTPMLimiter(
	redisKey string,
	maxTokensPerWindow int64,
	windowSizeSec int64,
	maxTokensPerBucket int64,
	bucketSizeSec int64,
) *TPMLimiter {
	return &TPMLimiter{
		key:                redisKey,
		maxTokensPerWindow: maxTokensPerWindow,
		windowSizeSec:      windowSizeSec,
		maxTokensPerBucket: maxTokensPerBucket,
		bucketSizeSec:      bucketSizeSec,
	}
}

func (l *TPMLimiter) TryCheck(tokensToConsume int64, agent *RedisLRAgent) (bool, int64, int64, error) {
	preconsumeToken := int64(0)
	//fast path
	if l.maxTokensPerWindow < 0 {
		return true, time.Now().Unix(), preconsumeToken, nil
	}
	if l.maxTokensPerWindow == 0 {
		return false, time.Now().Unix(), preconsumeToken, nil
	}
	if tokensToConsume >= l.maxTokensPerBucket {
		return false, time.Now().Unix(), preconsumeToken, nil
	}

	//check remote
	currentTime, _, _, _, _, isFinalAllowed, err := agent.CheckAndConsumeToken(l.key,
		l.maxTokensPerWindow, l.windowSizeSec, l.maxTokensPerBucket, l.bucketSizeSec, tokensToConsume)
	if err != nil {
		return false, currentTime, preconsumeToken, err
	}

	if isFinalAllowed {
		preconsumeToken = tokensToConsume
	}
	return isFinalAllowed, currentTime, preconsumeToken, nil
}

func (l *TPMLimiter) UpdateTokenUsage(bucketTime int64, tokensConsumeDelta int64, agent *RedisLRAgent) error {
	//tokensConsumeDelta = TotalConsumeToken - PresumeToken
	if tokensConsumeDelta == 0 {
		return nil
	}
	_, err := agent.UpdateConsumeToken(l.key, bucketTime, l.bucketSizeSec, tokensConsumeDelta)
	if err != nil {
		return err
	}
	return nil
}

func (l *TPMLimiter) UpdateArgs(maxTokensPerWindow int64,
	windowSizeSec int64,
	maxTokensPerBucket int64,
	bucketSizeSec int64) {

	l.maxTokensPerWindow = maxTokensPerWindow
	l.windowSizeSec = windowSizeSec
	l.maxTokensPerBucket = maxTokensPerBucket
	l.bucketSizeSec = bucketSizeSec
}
