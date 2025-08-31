// Copyright (c) 2025 The BFE Authors.
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

package redis_client

import (
	"fmt"
)

import (
)

// Client: redis client interface
type Client interface {
	Setex(key string, value []byte, expire int) error
	Get(key string) (interface{}, error)
	Expire(key string, expire int) error
	Incr(key string) (int64, error)
	IncrAndExpire(key string, expire int) (int64, error)
	Decr(key string) (int64, error)
	PIncr([]string) ([]int64, error)
	GetInt64(key string) (int64, error)
	IncrBy(key string, delta int64) (int64, error)
}

// counters for module state 2
var (
	RedisConn       = "REDIS_CONN"
	RedisConnFail   = "REDIS_CONN_FAIL"
	RedisAuthFail   = "REDIS_AUTH_FAIL"
	RedisExpire     = "REDIS_EXPIRE"
	RedisExpireFail = "REDIS_EXPIRE_FAIL"
	RedisSetex      = "REDIS_SETEX"
	RedisSetexFail  = "REDIS_SETEX_FAIL"
	RedisGet        = "REDIS_GET"
	RedisGetFail    = "REDIS_GET_FAIL"
	RedisGetMiss    = "REDIS_GET_MISS"
	RedisGetHit     = "REDIS_GET_HIT"
	RedisIncr       = "REDIS_INCR"
	RedisIncrFail   = "REDIS_INCR_FAIL"
	RedisDecr       = "REDIS_DECR"
	RedisDecrFail   = "REDIS_DECR_FAIL"
	RedisSendFail   = "REDIS_SEND_FAIL"
	RedisFlushFail  = "REDIS_FLUSH_FAIL"
)

type Options struct {
	// ServiceConf: string, bns name or a batch of bns name with weight of redis server
	ServiceConf string
	clusterList []RedisClusterConf
	// MaxIdle: int, max idle connections in connection pool
	MaxIdle int
	// MaxActive: int, max active connections in connection pool
	MaxActive int
	// wait: bool, if wait is true and pool at the maxActive limit,
	// command waits for a connection return to the pool
	Wait bool
	// ConnTimeoutMs: int, connect redis server timeout, in ms
	ConnTimeoutMs int
	// ReadTimeoutMs: int, read redis server timeout, in ms
	ReadTimeoutMs int
	// writeTimeoutMs: int, write redis server timeout, in ms
	WriteTimeoutMs int
	Password       string
}

func NewRedisClient(options *Options) Client {
	return NewRedisBnsClient(options)
}

func CheckRedisConf(redisServersStr string) error{
	_, err := ParseRedisBnsConf(redisServersStr)
	if err != nil {
		return fmt.Errorf("proxy mode, Redis.Bns check err: %s", err.Error())
	}

	return nil
}
