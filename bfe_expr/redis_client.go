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

/* redis_client.go - redis client handle used in expressions
/*
DESCRIPTION
	RedisClient 是表达式中持有的 redis client 句柄，由 GetRedisClient 返回（单例）。
	本期只提供读操作；所有方法 nil 安全，底层 client 不可用或调用出错（连接失败/
	key 不存在/类型不符）时返回零值（"" / 0 / 空 map），不中断规则求值。
*/

package bfe_expr

import (
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

// RedisClient redis client 表达式句柄（单例，进程级共享，勿按请求缓存）
type RedisClient struct {
	client redis_client.Client
}

// Get 读取 string 值；miss/错误/类型不符返回 ""
func (c *RedisClient) Get(key string) string {
	if c == nil || c.client == nil {
		return ""
	}
	v, err := c.client.Get(key)
	if err != nil || v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return ""
	}
}

// GetInt 读取整数值（string 存储解析）；miss/错误/解析失败返回 0
func (c *RedisClient) GetInt(key string) int {
	if c == nil || c.client == nil {
		return 0
	}
	n, err := c.client.GetInt64(key)
	if err != nil {
		return 0
	}
	return int(n)
}

// HGetAll 读取 Hash；miss/错误返回空 map
func (c *RedisClient) HGetAll(key string) map[string]string {
	if c == nil || c.client == nil {
		return nil
	}
	m, err := c.client.HGetAll(key)
	if err != nil {
		return nil
	}
	return m
}
