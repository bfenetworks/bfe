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
	_ "embed"
	"fmt"
	"strconv"
)

func (agent *RedisLRAgent) CheckAndConsumeToken(
	key string,
	tpmThreshold int64,
	windowSizeSec int64,
	bucketPeakLimit int64,
	bucketSizeSec int64,
	tokensToConsume int64,
) (currentTime int64, isBucketAllowed bool, isWholeAllowed bool,
	currentBucketRemaining int64, totalRemaining int64, isFinalAllowed bool, err error) {
	if bucketSizeSec <= 0 {
		err = fmt.Errorf("sbucketSizeSec <= 0")
		return
	}

	if agent.scriptTpmCheck == nil {
		err = fmt.Errorf("scriptCheck is not initialized")
		return
	}

	args := []interface{}{tpmThreshold, windowSizeSec, bucketPeakLimit, bucketSizeSec, tokensToConsume}

	res, err := agent.scriptTpmCheck.Run(key, args...)
	if err != nil {
		return
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) != 5 {
		err = fmt.Errorf("unexpected lua return type: %T, value: %v", res, res)
		return
	}

	if v, ok := arr[0].(int64); ok {
		currentTime = v
	} else if f, ok := arr[0].(float64); ok {
		currentTime = int64(f)
	} else if currentTimeStr, ok := arr[0].(string); ok {
		currentTime, err = strconv.ParseInt(currentTimeStr, 10, 64)
		if err != nil {
			err = fmt.Errorf("parse currentTime strin error: %w", err)
			return
		}
	} else if b, ok := arr[0].([]byte); ok {
		currentTime, err = strconv.ParseInt(string(b), 10, 64)
		if err != nil {
			err = fmt.Errorf("parse currentTime bytes error: %w", err)
			return
		}
	} else {
		err = fmt.Errorf("invalid currentTime type: %T", arr[0])
		return
	}

	if v, ok := arr[1].(int64); ok {
		isBucketAllowed = (v != 0)
	} else if f, ok := arr[1].(float64); ok {
		isBucketAllowed = (int64(f) != 0)
	} else if b, ok := arr[1].([]byte); ok {
		v, parseErr := strconv.ParseInt(string(b), 10, 64)
		if parseErr != nil {
			err = fmt.Errorf("parse isBucketAllowed bytes error: %w", parseErr)
			return
		}
		isBucketAllowed = (v != 0)
	} else {
		err = fmt.Errorf("invalid isBucketAllowed type: %T", arr[1])
		return
	}

	if v, ok := arr[2].(int64); ok {
		isWholeAllowed = (v != 0)
	} else if f, ok := arr[2].(float64); ok {
		isWholeAllowed = (int64(f) != 0)
	} else if b, ok := arr[2].([]byte); ok {
		v, parseErr := strconv.ParseInt(string(b), 10, 64)
		if parseErr != nil {
			err = fmt.Errorf("parse isWholeAllowed bytes error: %w", parseErr)
			return
		}
		isWholeAllowed = (v != 0)
	} else {
		err = fmt.Errorf("invalid isWholeAllowed type: %T", arr[2])
		return
	}

	if v, ok := arr[3].(int64); ok {
		currentBucketRemaining = v
	} else if f, ok := arr[3].(float64); ok {
		currentBucketRemaining = int64(f)
	} else if b, ok := arr[3].([]byte); ok {
		currentBucketRemaining, err = strconv.ParseInt(string(b), 10, 64)
		if err != nil {
			err = fmt.Errorf("parse currentBucketRemaining bytes error: %w", err)
			return
		}
	} else {
		err = fmt.Errorf("invalid currentBucketRemaining type: %T", arr[3])
		return
	}

	if v, ok := arr[4].(int64); ok {
		totalRemaining = v
	} else if f, ok := arr[4].(float64); ok {
		totalRemaining = int64(f)
	} else if b, ok := arr[4].([]byte); ok {
		totalRemaining, err = strconv.ParseInt(string(b), 10, 64)
		if err != nil {
			err = fmt.Errorf("parse totalRemaining bytes error: %w", err)
			return
		}
	} else {
		err = fmt.Errorf("invalid totalRemaining type: %T", arr[4])
		return
	}

	if isBucketAllowed && isWholeAllowed {
		isFinalAllowed = true
	}

	return
}

func (agent *RedisLRAgent) UpdateConsumeToken(
	key string,
	bucketTime int64,
	bucketSizeSec int64,
	tokensToConsume int64,
) (newBucketValue int64, err error) {
	if bucketSizeSec <= 0 {
		err = fmt.Errorf("sbucketSizeSec <= 0")
		return
	}
	if agent.scriptTpmUpdate == nil {
		return 0, fmt.Errorf("scriptUpdate is not initialized")
	}

	args := []interface{}{bucketTime, bucketSizeSec, tokensToConsume}

	res, err := agent.scriptTpmUpdate.Run(key, args...)
	if err != nil {
		return
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) != 1 {
		err = fmt.Errorf("unexpected lua return type: %T, value: %v", res, res)
		return
	}

	if v, ok := arr[0].(int64); ok {
		newBucketValue = v
	} else if f, ok := arr[0].(float64); ok {
		newBucketValue = int64(f)
	} else if b, ok := arr[0].([]byte); ok {
		newBucketValue, err = strconv.ParseInt(string(b), 10, 64)
		if err != nil {
			err = fmt.Errorf("parse newBucketValue bytes error: %w", err)
			return
		}
	} else {
		err = fmt.Errorf("invalid newBucketValue type: %T", arr[0])
		return
	}

	return
}
