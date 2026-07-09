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
	"fmt"
	"strconv"
)

func (agent *RedisLRAgent) QpmCheck(
	key string,
	burst int64,
	period int64,
	limit int64,
	expectUse int64,
) (currentTime int64, isAllowed bool, tat float64, err error) {
	if period <= 0 {
		err = fmt.Errorf("period <= 0")
		return
	}

	if agent.scriptQpmCheck == nil {
		err = fmt.Errorf("scriptQpmCheck is not initialized")
		return
	}

	// now := float64(time.Now().UnixNano()) / 1e9
	// args := []interface{}{burst, period, limit, expectUse, now}
	args := []interface{}{burst, period, limit, expectUse}

	res, errtmp := agent.scriptQpmCheck.Run(key, args...)
	if errtmp != nil {
		err = errtmp
		return
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) != 4 {
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
			err = fmt.Errorf("parse currentTime string error: %w", err)
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
		isAllowed = (v != 0)
	} else if f, ok := arr[1].(float64); ok {
		isAllowed = (int64(f) != 0)
	} else if b, ok := arr[1].([]byte); ok {
		v, parseErr := strconv.ParseInt(string(b), 10, 64)
		if parseErr != nil {
			err = fmt.Errorf("parse isAllowed bytes error: %w", parseErr)
			return
		}
		isAllowed = (v != 0)
	} else {
		err = fmt.Errorf("invalid isAllowed type: %T", arr[1])
		return
	}

	if tatStr, ok := arr[2].(string); ok {
		tat, err = strconv.ParseFloat(tatStr, 64)
		if err != nil {
			err = fmt.Errorf("parse tat string error: %w", err)
		}
	} else if b, ok := arr[2].([]byte); ok {
		tat, err = strconv.ParseFloat(string(b), 64)
		if err != nil {
			err = fmt.Errorf("parse tat bytes error: %w", err)
		}
	} else if v, ok := arr[2].(float64); ok {
		tat = v
	} else if f, ok := arr[2].(int64); ok {
		tat = float64(f)
	} else {
		err = fmt.Errorf("invalid tat type: %T", arr[2])
		return
	}

	return
}
