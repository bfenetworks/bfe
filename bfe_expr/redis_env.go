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

/* redis_env.go - redis access for expressions
/*
DESCRIPTION
	表达式内访问 redis：GetRedisClient(addr) 按地址返回进程级单例 client，
	GetRedisClientWithOpts 支持自定义密码与超时；同一 地址+参数 组合只创建
	一次连接池，之后（任意表达式、任意请求）复用。
	client 句柄支持链式调用，如 GetRedisClient("10.0.0.1:6379").Get("key")。
	连接失败/key 不存在等错误均返回零值，不中断规则求值。
	本期不新增数据源配置项：地址/密码/超时直接写在表达式内。
*/

package bfe_expr

import (
	"strconv"
	"strings"
	"sync"

	"github.com/bfenetworks/bfe/bfe_util/redis_client"
	"github.com/bfenetworks/go-lib/log"
)

// 默认连接参数（GetRedisClient 简化入口使用）
const (
	defaultRedisConnTimeoutMs  = 100
	defaultRedisReadTimeoutMs  = 300
	defaultRedisWriteTimeoutMs = 300
	defaultRedisMaxIdle        = 10
	defaultRedisMaxActive      = 100
)

// RedisEnv 提供表达式内的 redis 访问函数
type RedisEnv struct{}

// client 单例表：key 为 地址+参数 组合，进程级共享
var (
	redisClientsMu sync.RWMutex
	redisClients   = make(map[string]*RedisClient)
)

// GetRedisClient 按地址返回单例 redis client，使用默认超时/连接池参数。
// addr 支持 bns 名或 ip:port（格式见 redis_client.Options.ServiceConf）。
func (e *RedisEnv) GetRedisClient(addr string) *RedisClient {
	return e.GetRedisClientWithOpts(addr, "",
		defaultRedisConnTimeoutMs, defaultRedisReadTimeoutMs, defaultRedisWriteTimeoutMs)
}

// GetRedisClientWithOpts 按地址与参数返回单例 redis client。
// 同一 地址+参数 组合只创建一次连接池，重复调用（含不同表达式/请求）返回同一实例；
// 相同 key 再次传入不同参数时，以首次创建的实例为准。
func (e *RedisEnv) GetRedisClientWithOpts(addr, password string,
	connTimeoutMs, readTimeoutMs, writeTimeoutMs int) *RedisClient {

	key := strings.Join([]string{addr, password,
		strconv.Itoa(connTimeoutMs), strconv.Itoa(readTimeoutMs), strconv.Itoa(writeTimeoutMs)}, "\x00")

	redisClientsMu.RLock()
	c, ok := redisClients[key]
	redisClientsMu.RUnlock()
	if ok {
		return c
	}

	redisClientsMu.Lock()
	defer redisClientsMu.Unlock()
	if c, ok = redisClients[key]; ok {
		return c
	}

	c = newRedisClient(addr, password, connTimeoutMs, readTimeoutMs, writeTimeoutMs)
	redisClients[key] = c
	return c
}

// newRedisClient 按参数创建底层 redis client；配置非法时返回不可用句柄（调用返回零值）
func newRedisClient(addr, password string,
	connTimeoutMs, readTimeoutMs, writeTimeoutMs int) *RedisClient {

	options := &redis_client.Options{
		ServiceConf:    addr,
		MaxIdle:        defaultRedisMaxIdle,
		MaxActive:      defaultRedisMaxActive,
		Wait:           true,
		ConnTimeoutMs:  connTimeoutMs,
		ReadTimeoutMs:  readTimeoutMs,
		WriteTimeoutMs: writeTimeoutMs,
		Password:       password,
	}

	c := &RedisClient{}
	if rc := redis_client.NewRedisBnsClient(options); rc != nil {
		c.client = rc
	} else {
		log.Logger.Warn("bfe_expr: new redis client(%s) failed, calls will return zero value", addr)
	}
	return c
}
