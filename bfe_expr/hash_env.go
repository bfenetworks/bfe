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

/* hash_env.go - hash / ip / random functions
/*
DESCRIPTION
	Hash 是旧 *_hash_in 原语的哈希部分（murmur3 小写归一 % HashBucketSize），
	灰度判断交给比较运算符；空输入返回本请求内稳定的随机桶。
	IpIn/IpRange 对齐旧 req_vip_in / *_range 原语语义。
	Rand/RandInt/Srand 基于每请求独立的随机源（math/rand/v2 风格）。
*/

package bfe_expr

import (
	"bytes"
	"math/rand/v2"
	"net"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_util/murmur3"
	"github.com/bfenetworks/bfe/bfe_util/net_util"
)

type HashEnv struct {
	req *bfe_basic.Request
	rng *rand.Rand

	// emptyHashBucket 缓存空输入的随机桶，保证本请求内多次 Hash("") 结果一致
	emptyHashBucket *int
}

// Hash 返回 value 的哈希桶号 [0, HashBucketSize)：
// murmur3(小写归一(value)) % HashBucketSize；value 为 "" 时返回本请求内稳定的随机桶。
func (e *HashEnv) Hash(value string) int {
	if value == "" {
		if e.emptyHashBucket != nil {
			return *e.emptyHashBucket
		}
		bucket := e.rng.IntN(HashBucketSize)
		e.emptyHashBucket = &bucket
		return bucket
	}
	return int(murmur3.Sum32([]byte(strings.ToLower(value))) % HashBucketSize)
}

// IpIn 判断 ip 是否命中 cidrs 列表，元素支持单 IP 和 CIDR（如 ["10.0.0.0/8"]）。
// ip 或列表元素解析失败按未命中处理。
func (e *HashEnv) IpIn(ip string, cidrs []string) bool {
	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		return false
	}
	for _, c := range cidrs {
		if strings.Contains(c, "/") {
			_, ipNet, err := net.ParseCIDR(c)
			if err != nil {
				continue
			}
			if ipNet.Contains(ipAddr) {
				return true
			}
			continue
		}
		pattern := net.ParseIP(c)
		if pattern != nil && pattern.Equal(ipAddr) {
			return true
		}
	}
	return false
}

// IpRange 判断 ip 是否在 [start, end] 区间内（v4/v6 不混用；解析失败返回 false）。
func (e *HashEnv) IpRange(ip, start, end string) bool {
	if net_util.IsIPv4Address(start) != net_util.IsIPv4Address(end) {
		return false
	}
	startIP := net.ParseIP(start)
	endIP := net.ParseIP(end)
	ipAddr := net.ParseIP(ip)
	if startIP == nil || endIP == nil || ipAddr == nil {
		return false
	}
	ipAddr = ipAddr.To16()
	return bytes.Compare(ipAddr, startIP.To16()) >= 0 &&
		bytes.Compare(ipAddr, endIP.To16()) <= 0
}

// Rand 返回 [0, 1) 的随机数，每次调用产生新值；
// 需要跨规则复用时先存入用户变量（UsrVarSetXxx）。
func (e *HashEnv) Rand() float64 {
	return e.rng.Float64()
}

// RandInt 返回 [0, max) 的随机整数，每次调用产生新值。
func (e *HashEnv) RandInt(max int) int {
	if max <= 0 {
		return 0
	}
	return e.rng.IntN(max)
}

// Srand 用 seed 重播种本请求的随机源，不影响其他请求；典型用途是配合调试头复现灰度结果。
// 返回 seed 本身（expr 不支持无返回值函数，返回值无语义，可忽略）。
func (e *HashEnv) Srand(seed int) int {
	e.rng = rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x9e3779b97f4a7c15))
	e.emptyHashBucket = nil
	return seed
}
