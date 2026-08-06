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

	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

//go:embed redis_concurrency_limit_acquire.lua
var luaScriptConAcq string

//go:embed redis_concurrency_limit_release.lua
var luaScriptConRel string

//go:embed redis_tpm_limit_check.lua
var luaScriptTpmCheck string

//go:embed redis_tpm_limit_update.lua
var luaScriptTpmUpdate string

//go:embed redis_qpm_limit_check.lua
var luaScriptQpmCheck string

type RedisLRAgent struct {
	redisClient  redis_client.Client
	scriptConAcq redis_client.RedisScript
	scriptConRel redis_client.RedisScript

	scriptTpmCheck  redis_client.RedisScript
	scriptTpmUpdate redis_client.RedisScript

	scriptQpmCheck redis_client.RedisScript
}

func NewRedisLRAgent(redisClient redis_client.Client) *RedisLRAgent {
	return &RedisLRAgent{
		redisClient: redisClient,

		scriptConAcq: redisClient.NewScript(luaScriptConAcq),
		scriptConRel: redisClient.NewScript(luaScriptConRel),

		scriptTpmCheck:  redisClient.NewScript(luaScriptTpmCheck),
		scriptTpmUpdate: redisClient.NewScript(luaScriptTpmUpdate),

		scriptQpmCheck: redisClient.NewScript(luaScriptQpmCheck),
	}
}

func (agent *RedisLRAgent) ConAcquire(
	key string,
	conThreshold int64,
	ttl int64,
) (currentTime int64, isAllowed bool, curCount int64, err error) {
	if agent.scriptConAcq == nil {
		err = fmt.Errorf("scriptConnAcq is not initialized")
		return
	}

	args := []interface{}{conThreshold, ttl}

	res, errtmp := agent.scriptConAcq.Run(key, args...)
	if errtmp != nil {
		err = errtmp
		return
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) != 3 {
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

	if v, ok := arr[2].(int64); ok {
		curCount = v
	} else if f, ok := arr[2].(float64); ok {
		curCount = int64(f)
	} else if b, ok := arr[2].([]byte); ok {
		curCount, err = strconv.ParseInt(string(b), 10, 64)
		if err != nil {
			err = fmt.Errorf("parse curCount bytes error: %w", err)
			return
		}
	} else {
		err = fmt.Errorf("invalid curCount type: %T", arr[2])
		return
	}

	return
}

func (agent *RedisLRAgent) ConRelease(
	key string,
	ttl int64,
) (curCount int64, err error) {
	if agent.scriptConRel == nil {
		return 0, fmt.Errorf("scriptConnRel is not initialized")
	}

	args := []interface{}{1, ttl}

	res, errtmp := agent.scriptConRel.Run(key, args...)
	if errtmp != nil {
		err = errtmp
		return
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) != 1 {
		err = fmt.Errorf("unexpected lua return type: %T, value: %v", res, res)
		return
	}

	if v, ok := arr[0].(int64); ok {
		curCount = v
	} else if f, ok := arr[0].(float64); ok {
		curCount = int64(f)
	} else if b, ok := arr[0].([]byte); ok {
		curCount, err = strconv.ParseInt(string(b), 10, 64)
		if err != nil {
			err = fmt.Errorf("parse curCount bytes error: %w", err)
			return
		}
	} else {
		err = fmt.Errorf("invalid curCount type: %T", arr[0])
		return
	}

	return
}
