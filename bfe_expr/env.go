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

/* env.go - expression evaluation env based on expr-lang/expr
/*
DESCRIPTION
	BFE 条件表达式（新风格）的求值上下文。
	函数实现为各子 env 上的导出方法，表达式中以方法名调用（PascalCase）。
	同一请求生命周期内通过 GetEnv 共享同一个 Env 实例。
*/

package bfe_expr

import (
	"math/rand/v2"
	"time"

	"github.com/bfenetworks/bfe/bfe_basic"
)

// HashBucketSize 哈希桶空间，与旧条件原语 *_hash_in 保持一致
const HashBucketSize = 10000

// Env 是表达式求值上下文，按域拆分为多个子 env（匿名嵌入，方法提升）
type Env struct {
	BaseEnv   // ReqXxx / ResXxx / 路由集群 / SesXxx / Env / LogXxx
	TimeEnv   // Now / InTimeRange / InPeriodicRange
	HashEnv   // Hash / IpIn / IpRange / Rand / RandInt / Srand
	VarEnv    // ReqContext / UsrVarXxx / UsrVarSetXxx（请求级）
	RedisEnv  // GetRedisClient* / RedisClient.Get|GetInt|HGetAll，见 redis_env.go
	HttpEnv   // http 访问（本期 stub），见 http_env.go
	GlbEnv    // 进程级变量（本期 stub），见 glb_env.go
	ConfEnv   // 配置表访问（本期 stub），见 conf_env.go
	InnerEnv  // 旧 %var 内置变量函数（BfeClientIp 等），见 inner_env.go
	LegacyEnv // 旧原语方法（ReqMethodIn 等），配合 legacyPatcher 兼容旧条件串
}

// envCtxKey 把 env 挂到请求 Context 的 key（私有类型，避免与业务 key 冲突）
type envCtxKey struct{}

// NewEnv 创建一个绑定到指定请求的 Env（每请求一次）
func NewEnv(req *bfe_basic.Request) *Env {
	seed := uint64(time.Now().UnixNano())
	e := &Env{
		BaseEnv: BaseEnv{req: req},
		TimeEnv: TimeEnv{req: req},
		HashEnv: HashEnv{
			req: req,
			rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
		},
		VarEnv:   VarEnv{req: req},
		RedisEnv: RedisEnv{},
		InnerEnv: InnerEnv{req: req},
	}
	e.LegacyEnv = LegacyEnv{env: e}
	return e
}

// GetEnv 返回请求级 Env（懒创建并按请求缓存，同一请求的所有表达式求值共享）。
// 注意：目标仓库 Request.Context 为无锁 map，与既有 SetContext/GetContext 用法
// 保持一致；同一请求的处理若为并发场景，调用方需自行保证时序安全。
func GetEnv(req *bfe_basic.Request) *Env {
	if req == nil {
		return nil
	}
	if req.Context != nil {
		if v := req.GetContext(envCtxKey{}); v != nil {
			if e, ok := v.(*Env); ok {
				return e
			}
		}
	}
	e := NewEnv(req)
	if req.Context != nil {
		req.SetContext(envCtxKey{}, e)
	}
	return e
}
