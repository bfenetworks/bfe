# AI 限流 Redis 计数器 key 稳定性设计

## 1. 背景与问题

### 1.1 背景

`mod_ai_rate_limit` 负责 AI 请求的 TPM / RPM / 并发数限流。其中 RPM 计数器基于 Redis key 维护当前时间窗口内的请求次数。

### 1.2 问题

见 [ai-gateway-api/issues/84](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/84)。

当前 BFE 的 RPM 计数器 Redis key 由「策略 ID + 规则名」构成：

```go
// bfe/bfe_modules/mod_ai_rate_limit/policy_limiter.go

func buildRpmInstId(rule *RPMRuleConf) string {
    if rule.Name != "" {
        return rule.Name
    }
    return fmt.Sprintf("rpm_%d_%d_%d", rule.TimeWindow, rule.MaxRequests, rule.Burst)
}

redisKey := buildRedisKey(policyId, fmt.Sprintf("rpm_%s", rpmInstId))
// => default_bfe_<policyId>_rpm_<ruleName>
```

当管理面仅修改 RPM 规则名（其余字段不变）时，Redis key 发生变化，导致计数器从零开始。

### 1.3 为什么不能用 model 替代 name

有人可能提议将 key 中的 `rule.Name` 替换为 `rule.Model`。但 `model` 同样是用户可编辑字段，修改 model 仍会改变 Redis key；且 `model` 可能为通配符 `*`，多个规则也可能指向同一 model，存在 key 冲突和语义混乱。因此 **计数器 key 不能依赖任何用户可编辑的业务字段（name / model）**。

## 2. 参考：quota_plan 的 Redis key 生成方式

`ai-gateway-api` 在导出 quota_plan 时，由控制面生成稳定的 `RedisKey` 并下发给 BFE：

```go
// ai-gateway-api/model/imods/exporter.go

func convertQuotaPlanToExport(qp *quota.QuotaPlanParam, id string, redisKeyID string) *QuotaPlan {
    result := &QuotaPlan{
        Id:          id,
        RedisKey:    fmt.Sprintf("QUOTA_%s", redisKeyID),
        ...
    }
    ...
}
```

其中 `redisKeyID` 取的是配额归属方的稳定标识：
- API-Key 级配额：`redisKeyID = apiKey.Key`
- Entity 级配额：`redisKeyID = entity.EntityID`

BFE 侧只消费现成的 `QuotaPlan.RedisKey`，不再根据计划名称/ID 自己拼装 key。这保证了：修改 quota_plan 名称、修改 model 等都不会重置 Redis 计数器。

RPM/TPM 限流应参照同一原则：**由控制面生成稳定的 Redis key 并下发，BFE 直接使用，key 不依赖用户可编辑字段**。

## 3. 目标

1. 消除「改名/改 model 导致 RPM/TPM 计数器重置」的问题；
2. 保持限流策略可共享（多个 API-Key 可绑定同一策略）；
3. 向后兼容：未携带 `redis_key` 的旧配置仍能按原 `name` 逻辑工作；
4. 设计文档、配置文档、示例配置与代码实现保持一致。

## 4. 设计原则

- **稳定 key 由控制面生成**：`ai-gateway-api` 在导出限流策略时，基于不会随用户编辑而变的 `(policy_id, rule_index)` 为每条规则生成 `redis_key`；
- **BFE 直接使用下发 key**：BFE 读取规则中的 `redis_key` 字段构建 Redis key，不再根据 `name` 或 `model` 自己拼装；
- **兜底兼容**：旧配置或手工配置中若未指定 `redis_key`，BFE 保留原 `name` 逻辑，避免升级后所有计数器立即丢失；
- **规则结构性变更可接受**：删除/新增规则会改变后续规则下标，计数器会重置，这属于规则集合结构性变更，与单条规则改名不同。

## 5. 总体架构

```
┌─────────────────────────────────────────────┐
│ ai-gateway-api                              │
│ 导出 rate-limit-policy                      │
│ 为每条规则生成 redis_key = RL_{RPM|TPM}_rlp-<id>_<idx>
└──────────────────┬──────────────────────────┘
                   ▼
┌─────────────────────────────────────────────┐
│ conf-agent 下发 ai_rate_limit.data          │
│ 包含 RateLimitPolicies[].rules.{tpm,rpm}[].redis_key
└──────────────────┬──────────────────────────┘
                   ▼
┌─────────────────────────────────────────────┐
│ BFE mod_ai_rate_limit                       │
│ 加载规则，优先使用 redis_key 构建 Redis key  │
│ 未指定时按原 name 逻辑兜底                   │
└─────────────────────────────────────────────┘
```

## 6. 数据结构与字段变更

### 6.1 ai-gateway-api 导出结构

`ai-gateway-api/model/rate_limit_policy/rate_limit_policy.go`：

```go
type ExportRPMConfig struct {
    Name          string   `json:"name"`
    Models        []string `json:"models"`
    WindowMinutes int      `json:"window_minutes"`
    MaxRequests   int      `json:"max_requests"`
    Burst         int      `json:"burst"`
    RedisKey      string   `json:"redis_key"` // 新增
}

type ExportTPMConfig struct {
    Name          string   `json:"name"`
    Models        []string `json:"models"`
    WindowMinutes int      `json:"window_minutes"`
    MaxTokens     int      `json:"max_tokens"`
    StepMinutes   int      `json:"step_minutes"`
    RedisKey      string   `json:"redis_key"` // 新增
}
```

生成逻辑（`ai-gateway-api/model/rate_limit_policy/rate_limit_policy_manager.go`）：

```go
for idx, rpm := range policy.RpmConfigs {
    models := []string{"*"}
    if rpm.Model != "" && rpm.Model != "*" {
        models = []string{rpm.Model}
    }
    exportPolicy.Rules.RPM = append(exportPolicy.Rules.RPM, ExportRPMConfig{
        Name:          rpm.Name,
        Models:        models,
        WindowMinutes: rpm.WindowMinutes,
        MaxRequests:   rpm.MaxRequests,
        Burst:         1,
        RedisKey:      fmt.Sprintf("RL_RPM_rlp-%d_%d", policyID, idx),
    })
}
```

### 6.2 BFE 运行时结构

`bfe/bfe_modules/mod_ai_rate_limit/data_load.go`：

```go
type RPMRuleConfFile struct {
    Name          string   `json:"name"`
    WindowMinutes int      `json:"window_minutes"`
    MaxRequests   int64    `json:"max_requests"`
    Burst         int64    `json:"burst"`
    Models        []string `json:"models"`
    RedisKey      string   `json:"redis_key"` // 新增
}

type RPMRuleConf struct {
    Name        string
    TimeWindow  int
    MaxRequests int64
    Burst       int64
    Models      []string
    RedisKey    string // 新增
}
```

### 6.3 BFE Redis key 构建

`bfe/bfe_modules/mod_ai_rate_limit/policy_limiter.go`：

```go
func buildRpmRedisKey(policyId string, rule *RPMRuleConf) string {
    if rule.RedisKey != "" {
        return buildRedisKey(policyId, rule.RedisKey)
    }
    // 兼容旧配置
    return buildRedisKey(policyId, fmt.Sprintf("rpm_%s", buildRpmInstId(rule)))
}
```

### 6.4 缓存命中时不计量（issue #1433）

上述所有计数器的读写（rpm 的 `Check` 自增、tpm 的 `TryCheck` 预扣与结算、并发
`ConnAcquire/ConnRelease`）都只发生在 `HandleAfterAITargetModel`（`mod_ai_rate_limit.go:501`）
与 `HandleRequestFinish`（`:506`）——而 `mod_ai_cache` 命中会在更早的
`HandleFoundProduct` 直接 `BfeHandlerFinish`，这两个回调都不会触发。

因此：**缓存命中既不检查限流、也不消耗 rpm/tpm/concurrency 额度**，Redis 计数不变。
这与"命中不扣配额、不计费"（`mod_ai_token_auth.go:393-399`）是同一口径；条款与理由见
`docs/zh_cn/sys_design/ai_cache.md` §9.1 与
`docs/zh_cn/modifications/2026-10-20-issue-1433-cache-hit-policy-matrix/design-changes.md`。
副作用是"策略收紧为后生效"：新绑的限流策略不会让已有缓存条目失效，滞后上界为条目 TTL。

## 7. 配置示例

```json
{
    "Version": "1.0",
    "RateLimitPolicies": {
        "rlp-0001": {
            "name": "ratelimitX",
            "enabled": true,
            "rules": {
                "tpm": [
                    {"name":"tpm0", "window_minutes": 1, "max_tokens": 10000, "step_minutes": 1, "models": ["gpt-4"], "redis_key": "RL_TPM_rlp-0001_0"}
                ],
                "rpm": [
                    {"name":"rpm0", "window_minutes": 1, "max_requests": 100, "burst": 1, "models": ["gpt-4"], "redis_key": "RL_RPM_rlp-0001_0"}
                ],
                "max_concurrency": 50
            }
        }
    },
    "ApikeyRateLimitPolicyBindings": {
        "ak-2v8x9k3m7p": ["rlp-0001"]
    }
}
```

## 8. 兼容性

| 场景 | 行为 |
|------|------|
| 新配置携带 `redis_key` | BFE 直接使用该 key 作为计数器 key |
| 旧配置未携带 `redis_key` | BFE 按原 `name` 逻辑兜底，计数器行为不变 |
| 改名（其余字段不变） | `redis_key` 不变，计数器不重置 |
| 改 model（其余字段不变） | `redis_key` 不变，计数器不重置 |
| 删除/新增规则 | 后续规则下标变化，对应 `redis_key` 变化，计数器重置（结构性变更，可接受） |

## 9. 测试建议

1. **单元测试**
   - BFE `buildRpmRedisKey` / `buildTpmRedisKey`：覆盖 `redis_key` 存在与缺失两种场景；
   - BFE `Convert()`：验证 `redis_key` 从配置文件正确读取到运行时结构；
   - ai-gateway-api 导出：验证 `redis_key` 按 `(policy_id, rule_index)` 生成。

2. **集成测试**
   - 创建 API-Key 并绑定 RPM 策略，`max_requests=1`；
   - 同一窗口内连发 2 个请求，验证第 2 个返回 429；
   - PATCH 仅修改规则名，再发请求，验证仍返回 429；
   - PATCH 仅修改 model，再发请求，验证仍返回 429。

## 10. 相关文件

- `ai-gateway-api/model/rate_limit_policy/rate_limit_policy.go`
- `ai-gateway-api/model/rate_limit_policy/rate_limit_policy_manager.go`
- `ai-gateway-api/model/imods/exporter.go`（quota_plan RedisKey 生成参考）
- `bfe/bfe_modules/mod_ai_rate_limit/data_load.go`
- `bfe/bfe_modules/mod_ai_rate_limit/policy_limiter.go`
- `bfe/docs/zh_cn/configuration/mod_ai_rate_limit/ai_rate_limit.data.md`
- `bfe/conf/mod_ai_rate_limit/ai_rate_limit.data`
