# BFE RMB 配额支持

## 1. 背景与目标

### 1.1 背景

当前 BFE 的配额扣减流程只支持 **Token** 单位：

1. 认证阶段：`mod_ai_token_auth` 校验 API Key，并通过 `QuotaPlan.HasBalance()` 检查 Redis 余额是否大于 0。
2. 响应阶段：`mod_body_process` / `mod_ai_token_auth` 从响应中提取 `prompt_tokens` / `completion_tokens`；`mod_ai_token_auth` 在请求结束时通过 Lua 脚本从 Redis 整数扣减 Token 数。

引入 **RMB（人民币）** 配额后，需要在响应阶段：

- 根据实际命中的 `cluster` 和 `target_model` 查找定价表；
- 把 `prompt_tokens` / `completion_tokens` 换算成人民币成本；
- 对 `unit = "RMB"` 的配额计划扣减相应金额。

### 1.2 目标

1. 配置层沿用 `AIConf.ModelTable`，价格以 `Prices` map（元/Token）下发，BFE 加载时转换为 1e-8 元/Token 定点整数；
2. `bfe_basic.TokenUsage` 增加 `UsedCost`，用于记录本次请求的 RMB 成本；
3. `mod_ai_token_auth.QuotaPlan` 增加 `Unit`，`Deduct` / `HasBalance` 支持 RMB；
4. 新增共享库 `go-lib/quota`，提供 RMB 定点数转换，供 ai-gateway-api 与 BFE 共同引用；
5. Redis Lua 支持 RMB 扣减脚本，当前暂时使用单 Key 定点数方案。

## 2. 设计原则

- **向后兼容**：存量 Token 配额完全兼容，`Unit` 默认 `"total_token"`，走原有扣减逻辑；
- **定点整数**：所有金额在 BFE 内部和 Redis 中均以定点整数表示，避免浮点误差；
- **配置不下发整数**：conf-agent 只负责配置下发，价格仍按原始浮点数下发，转换在 BFE 内部完成；
- **共享转换逻辑**：ai-gateway-api 与 BFE 共用 `go-lib/quota`，保证管理面与数据面对 Redis 值的解释一致。

## 3. 总体架构

```
┌─────────────────────────────────────────┐
│  conf-agent 下发 cluster_conf.data      │
│  (AIConf.ModelTable 价格仍为浮点数)      │
└─────────────────┬───────────────────────┘
                  ▼
┌─────────────────────────────────────────┐
│  bfe_config/bfe_cluster_conf/...        │
│  加载 AIConf，校验并构建 priceIndex      │
│  通过 go-lib/quota 转换定点整数价格       │
└─────────────────┬───────────────────────┘
                  ▼
┌─────────────────────────────────────────┐
│  请求运行时                              │
│  - 认证阶段：HasBalance() 按单位检查余额 │
│  - 响应阶段：calcCostUnits() 计算 RMB 成本│
│  - 扣减阶段：Lua 脚本原子扣减定点整数     │
└─────────────────────────────────────────┘
```

## 4. 配置层设计

### 4.1 文件

`bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`

### 4.2 数据结构

BFE 侧 `AIConf` 已存在 `ModelTable`（v0.4 货币固定为 RMB），结构如下：

```go
type ModelPrice struct {
    Provider            string
    Model               string // 模型名，用于匹配请求中的 target_model
    BaseModel           string
    Mode                string // 请求模式，如 "chat"
    Capabilities        []string
    SupportedParameters []string
    Limits              map[string]interface{}
    Prices              map[string]float64 // 价格对象，如 input_cost_per_token / output_cost_per_token
    Metadata            map[string]interface{}
}

type ModelTable struct {
    Currency string       // v0.4 固定为 "RMB"
    Models   []ModelPrice

    // 运行时索引，配置加载阶段构建：model -> mode -> *ModelPrice
    priceIndex map[string]map[string]*ModelPrice
}

type AIConf struct {
    Type         int                // 保留字段，当前应为 0
    ModelMapping *map[string]string // 模型映射
    Provider     string             // provider 名
    Keys         []AIKey            // 多 Key 模式
    KeyPolicy    *AIKeyPolicy       // Key 选择策略
    ModelTable   *ModelTable        // 成本定价表
}
```

> 说明：`AIConf` 旧字段 `Key` 已移除，统一使用 `Keys`。

### 4.3 校验规则

1. `ModelTable.Currency` 当前仅允许 `"RMB"`。
2. `ModelPrice.Prices["input_cost_per_token"]`、`Prices["output_cost_per_token"]` 必须 `>= 0`。
3. `Model` 为具体模型名；`Mode` 如 `"chat"`。
4. 同一个 `Mode` 下，`Model` 不能重复。
5. 加载时构建二维索引 `priceIndex[model][mode]`，便于运行时 O(1) 查询。
6. 加载阶段通过 `go-lib/quota.RmbToFixedPoint` 将浮点价格转换为定点整数，BFE 内部和 Redis 中只使用整数。

### 4.4 配置加载阶段处理

conf-agent 只负责配置下发，不做任何数据转换。`cluster_conf.data` 中 `AIConf.ModelTable.Models[].Prices` 仍按原始浮点数（元/Token）下发。

当 `unit = "RMB"` 时，小数到整数的转换及索引构建必须在 BFE 内部完成，例如在 `ClusterConfCheck` 或 `AIConf` 专有校验阶段。转换逻辑统一放到共享库 `go-lib/quota`：

```go
import "github.com/bfenetworks/go-lib/quota"

func buildModelTableIndex(table *ModelTable) error {
    table.priceIndex = make(map[string]map[string]*ModelPrice)

    for i := range table.Models {
        price := &table.Models[i]

        // 1. 价格转换：浮点元/Token -> 1e-8 元/Token 定点整数
        input := price.Prices["input_cost_per_token"]
        output := price.Prices["output_cost_per_token"]
        if input < 0 || output < 0 {
            return fmt.Errorf("negative price for model %s", price.Model)
        }
        price.Prices["input_cost_per_token_int"] = float64(quota.RmbToFixedPoint(input))
        price.Prices["output_cost_per_token_int"] = float64(quota.RmbToFixedPoint(output))

        // 2. 构建 model -> mode 二维索引
        if table.priceIndex[price.Model] == nil {
            table.priceIndex[price.Model] = make(map[string]*ModelPrice)
        }
        table.priceIndex[price.Model][price.Mode] = price
    }
    return nil
}
```

> 说明：
> - 转换后 BFE 内部及 Redis Lua 中只使用整数，避免浮点误差。
> - conf-agent 不感知 `unit` 类型，也不修改价格格式。
> - `go-lib/quota` 同时被 ai-gateway-api 和 BFE 引用，保证管理面与数据面对 Redis 值的解释完全一致。

## 5. 共享库 `go-lib/quota`

为避免 ai-gateway-api 与 BFE 对 Redis 中 RMB 配额值的解释不一致，定点数转换逻辑统一抽取到 `go-lib/quota`：

```go
package quota

const (
    UnitTotalToken = "total_token"
    UnitRMB        = "RMB"
)

const RmbPrecision = 1e8

// RmbToFixedPoint converts yuan to a fixed-point integer (1e-8 yuan per unit).
func RmbToFixedPoint(yuan float64) int64

// FixedPointToRmb converts a fixed-point integer back to yuan.
func FixedPointToRmb(value int64) float64

// ToRedisValue converts a quota value to a Redis fixed-point integer.
func ToRedisValue(quota float64, unit string) int64

// FromRedisValue converts a Redis fixed-point integer back to a quota value.
func FromRedisValue(value int64, unit string) float64
```

职责边界：

- **`go-lib/quota`**：只负责 **单位与定点数之间的转换**，不依赖 Redis 客户端，不执行任何 Redis 命令。
- **ai-gateway-api**：引用 `go-lib/quota`，负责管理面配额的初始化、重置、同步（使用 `IncrBy` 等）。
- **BFE**：引用 `go-lib/quota`，负责数据面请求成本的计算与 Lua 原子扣减。

## 6. 基础数据结构改动

### 6.1 `TokenUsage`

`bfe/bfe_basic/request_ai_basic.go`

```go
type TokenUsage struct {
    PromptTokens     int64 // 请求侧 Token 数
    CompletionTokens int64 // 响应侧 Token 数
    UsedQuota        int64 // 已用 Token 配额（unit=total_token 时使用）
    UsedCost         int64 // 已用 RMB 成本，1 单位 = 1e-8 元（unit=RMB 时使用）
}
```

### 6.2 `QuotaPlan`

`bfe/bfe_modules/mod_ai_token_auth/token.go`

```go
type QuotaPlan struct {
    Id          string
    Unlimited   bool
    PassNoQuota bool
    RedisKey    string
    ExpiredTime int64
    Quota       int64  // 固定点整数：total_token 时为 Token 数；RMB 时为 1e-8 元
    Unit        string // "total_token" 或 "RMB"
}
```

> 说明：
> - `Quota` 保持 `int64` 不变，但语义由 `Unit` 字段解释。这样可完全避免 `float64` 在 Redis Lua 和大额余额中的精度问题。
> - `Unit` 本身已隐含货币类型（如 `"RMB"`），`QuotaPlan` 不需要额外的 `Currency` 字段。

### 6.3 配置校验

`bfe/bfe_modules/mod_ai_token_auth/token_rule_load.go`

`quotaPlanCheck` 需要调整：

- `Unit` 为空时默认 `"total_token"`，保持兼容。
- `Unit = "total_token"`：`Unlimited=false` 时 `Quota > 0`。
- `Unit = "RMB"`：`Unlimited=false` 时 `Quota >= 0`。

## 7. 请求运行时改动

### 7.1 认证阶段：`ValidateUserTokenByReq`

当前逻辑已经遍历 `token.QuotaPlans` 并调用 `plan.HasBalance()`。对 RMB 配额：

- **不做按请求成本的精确预检**（因为最终输出 Token 数未知）。
- 仍按余额是否大于 0 进行粗略预检；若余额为 0 则拒绝。

> 如果需要更严格（如按 `max_tokens` 估算最坏成本），可在后续迭代中补充。

### 7.2 在 `TokenAuthContext` 中缓存 `serverConf`

`bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

BFE 的 reverse proxy 在请求结束前会将 `req.SvrDataConf` 清空为 `nil`。为了在最后扣减阶段仍能访问 cluster 配置，`SetTokenAuthContext` 在认证阶段把 `req.SvrDataConf` 缓存到 `TokenAuthContext` 中：

```go
type TokenAuthContext struct {
    Token       *Token
    aiBasicInfo *bfe_basic.AiBasicInfo
    // serverConf caches the SvrDataConf before it is cleared by the reverse proxy.
    serverConf  bfe_basic.ServerDataConfInterface
}

func SetTokenAuthContext(req *bfe_basic.Request, tok *Token, promptToken int64, tags []bfe_basic.ApikeyTag) {
    aiBasicInfo := req.GetAiBasicInfo()
    if aiBasicInfo != nil {
        tusage := aiBasicInfo.GetTokenUsage()
        tusage.PromptTokens = promptToken
        tusage.CompletionTokens = bfe_basic.COMPLETION_TOKENS_UNKNOWN
        aiBasicInfo.ApikeyTags = tags
    }

    tokenCtx := &TokenAuthContext{
        Token:       tok,
        aiBasicInfo: aiBasicInfo,
        serverConf:  req.SvrDataConf,
    }
    req.SetContext(REQ_TOKEN_AUTH_CONTEXT, tokenCtx)
}
```

### 7.3 响应阶段：`tokenReadResponseHandler`

`bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

响应阶段负责从响应体中提取 `usage`，或在未返回 `usage` 时按响应体长度估算 Token 数。对于非流式响应，`ContentLength >= 0` 时可直接读取完整响应体：

```go
func (m *ModuleAITokenAuth) tokenReadResponseHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
    ctx := GetTokenAuthContext(req)
    if ctx == nil {
        return bfe_module.BfeHandlerGoOn
    }
    tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
    if res.StatusCode == bfe_http.StatusOK && res.ContentLength >= 0 {
        if bodyAccessor, err := res.GetBodyAccessor(); err == nil {
            body, _ := bodyAccessor.GetBytes()
            UpdateCtxByUsage(ctx, body)
        }
        if tokenUsage.UsedQuota <= 0 && ctx.aiBasicInfo.IsAllowEstimateToken() {
            tokenUsage.CompletionTokens = int64(res.ContentLength) / 4
            tokenUsage.UsedQuota = CalcReqUsedQuota(req, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
        }
    }

    return bfe_module.BfeHandlerGoOn
}
```

> 说明：旧实现中 RMB 成本在此阶段计算，导致流式响应（`ContentLength = -1`）无法计费。当前实现已将成本计算移到请求结束阶段，见 7.4。

#### 流式响应的 Token 用量收集

对于 `stream: true` 的 SSE 流式响应，`mod_body_process` 默认会注册 `QuotaUsageProcessor`：

- `mod_body_process.DoResponseProcess` 根据响应 `Content-Type` 选择 SSE 解码器；
- 每个 SSE 事件经过 `QuotaUsageProcessor.Process` 时，会从事件数据中提取 `usage.*_tokens`；
- 当遇到包含 `usage` 的最后一个事件时，将 `PromptTokens` / `CompletionTokens` / `UsedQuota` 写入 `AiBasicInfo.TokenUsage`。

因此，到请求结束阶段，`tokenUsage.PromptTokens` 和 `tokenUsage.CompletionTokens` 已经就绪，无论流式还是非流式都可以统一计算 RMB 成本。

### 7.4 请求结束阶段：`tokenRequestFinishHandler`

`bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

请求结束阶段统一计算 RMB 成本并扣减。`TokenAuthContext` 中已缓存 `serverConf`，因此即使 `req.SvrDataConf` 已被 reverse proxy 清空，仍然可以访问 cluster 定价表：

```go
func (m *ModuleAITokenAuth) tokenRequestFinishHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
    if res == nil || res.StatusCode != bfe_http.StatusOK {
        return bfe_module.BfeHandlerGoOn
    }

    ctx := GetTokenAuthContext(req)
    if ctx == nil {
        return bfe_module.BfeHandlerGoOn
    }

    tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
    if tokenUsage.UsedQuota <= 0 && ctx.aiBasicInfo.IsAllowEstimateToken() {
        tokenUsage.UsedQuota = CalcReqUsedQuota(req, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
    }

    // 统一在请求完成阶段计算 RMB 成本（流式由 mod_body_process 填充 token 用量）
    if tokenUsage.UsedCost <= 0 && hasRMBPlan(ctx.Token.QuotaPlans) {
        tokenUsage.UsedCost = m.calcCostUnits(req, ctx.serverConf, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
    }

    costUnits := tokenUsage.UsedCost

    if tokenUsage.UsedQuota > 0 || costUnits > 0 {
        for _, plan := range ctx.Token.QuotaPlans {
            if plan.Unlimited {
                continue
            }
            if plan.Unit == "RMB" {
                if costUnits > 0 {
                    _, err := plan.Deduct(m.redisClient, costUnits)
                    if err != nil {
                        log.Logger.Warn("deduct rmb quota failed: %v", err)
                    }
                }
            } else {
                if tokenUsage.UsedQuota > 0 {
                    _, err := plan.Deduct(m.redisClient, tokenUsage.UsedQuota)
                    if err != nil {
                        log.Logger.Warn("deduct token quota failed: %v", err)
                    }
                }
            }
        }
    }

    return bfe_module.BfeHandlerGoOn
}
```

### 7.5 成本计算辅助方法

新增方法（位于 `mod_ai_token_auth`）：

```go
func (m *ModuleAITokenAuth) calcCostUnits(req *bfe_basic.Request, serverConf bfe_basic.ServerDataConfInterface, promptTokens, completionTokens int64) int64 {
    aiMeta := req.GetAiBasicInfo()
    if aiMeta == nil {
        return 0
    }

    clusterName := req.Route.ClusterName
    targetModel := aiMeta.TargetModel
    if clusterName == "" || targetModel == "" {
        return 0
    }

    if serverConf == nil {
        return 0
    }
    cluster, err := serverConf.ClusterTableLookup(clusterName)
    if err != nil || cluster == nil || cluster.AIConf == nil || cluster.AIConf.ModelTable == nil {
        log.Logger.Warn("model table not found for cluster %s", clusterName)
        return 0
    }

    entry := cluster_conf.LookupModelPrice(cluster.AIConf.ModelTable, targetModel, "chat")
    if entry == nil {
        log.Logger.Warn("model price not found for cluster %s model %s", clusterName, targetModel)
        return 0
    }

    // 使用配置加载阶段已转换好的定点整数价格（1 单位 = 1e-8 元）
    // 转换由 go-lib/quota.RmbToFixedPoint 统一完成
    inputCost  := int64(entry.Prices["input_cost_per_token_int"])
    outputCost := int64(entry.Prices["output_cost_per_token_int"])
    if inputCost < 0 || outputCost < 0 {
        log.Logger.Warn("invalid model price for cluster %s model %s", clusterName, targetModel)
        return 0
    }

    return promptTokens*inputCost + completionTokens*outputCost
}
```

说明：

- `req.Route.ClusterName` 在 `reverseproxy.go` 的 `aiClusterInvoke()` 中已被设置为最终实际使用的 cluster（包括 fallback 场景）。
- `aiMeta.TargetModel` 在 `reverseproxy.go` 的 `doSingleAIForward()` 中已被设置为路由目标模型 + cluster `ModelMapping` 映射后的最终模型名。
- 因此这里拿到的 `clusterName` 和 `targetModel` 就是计费所需的实际值。
- 价格到定点整数的转换在配置加载阶段通过 `go-lib/quota` 完成，运行时 `calcCostUnits` 只处理整数，保证 Redis Lua 不接触浮点。

### 7.6 定价匹配逻辑

```go
func lookupModelPrice(table *cluster_conf.ModelTable, model, mode string) *cluster_conf.ModelPrice {
    if table == nil {
        return nil
    }
    idx, ok := table.priceIndex[model]
    if !ok {
        return nil
    }
    return idx[mode]
}
```

索引在配置加载阶段构建，运行时按 `(model, mode)` 精确查询，为 O(1)。未命中时返回 `nil`，由调用方决定是否按 `0` 成本处理。

## 8. Redis Lua 脚本改造

当前 Token 配额 Lua：

```lua
local current = tonumber(redis.call('GET', KEYS[1]) or ARGV[2])
local amount = tonumber(ARGV[1])
local deduct = math.min(current, amount)
if deduct > 0 then
    redis.call('DECRBY', KEYS[1], deduct)
end
return math.max(0, current - deduct)
```

RMB 配额有两种可选实现。

### 8.1 方案一：单 Key 定点数（余额上限 ≤ 9000 万元）

以 **1e-8 元** 为一个单位，`Quota` 和余额都存为整数。

```lua
local raw = redis.call('GET', KEYS[1])
local current
if raw == false then
    current = tonumber(ARGV[2])
    redis.call('SET', KEYS[1], current)
else
    current = tonumber(raw)
end
local amount = tonumber(ARGV[1])
local deduct = math.min(current, amount)
if deduct > 0 then
    redis.call('DECRBY', KEYS[1], deduct)
end
return math.max(0, current - deduct)
```

- `ARGV[1]`：本次扣减金额（固定点整数）。
- `ARGV[2]`：初始配额（固定点整数），仅在 Key 不存在时使用。

> ⚠️ Lua number 为 IEEE 754 double，整数精确表示上限约为 `2^53`（9e15）。按 1e-8 元换算，理论余额上限约为 9007.20 万元；业务上统一限定 RMB 配额余额上限为 **9000 万元（90,000,000.00 元）**。若业务需要更大余额，请使用方案二。

### 8.2 方案二：Hash 拆分整数部分 + 小数部分（支持约 100 亿元上限）

用 Redis Hash 存两个字段：`yuan`（整数元）和 `fraction`（0 ~ 99,999,999）。

```lua
local yuan = tonumber(redis.call('HGET', KEYS[1], 'yuan'))
local frac = tonumber(redis.call('HGET', KEYS[1], 'fraction'))
if yuan == nil then
    yuan = tonumber(ARGV[1])
    frac = tonumber(ARGV[2])
    redis.call('HMSET', KEYS[1], 'yuan', yuan, 'fraction', frac)
end

local cost_yuan = tonumber(ARGV[3])
local cost_frac = tonumber(ARGV[4])

if frac < cost_frac then
    yuan = yuan - 1
    frac = frac + 100000000
end
yuan = yuan - cost_yuan
frac = frac - cost_frac

if yuan < 0 then
    yuan = 0
    frac = 0
end

redis.call('HMSET', KEYS[1], 'yuan', yuan, 'fraction', frac)
return {yuan, frac}
```

- `ARGV[1]` / `ARGV[2]`：初始配额的 `yuan` / `fraction`。
- `ARGV[3]` / `ARGV[4]`：本次扣减成本的 `yuan` / `fraction`。
- 所有数字都在 64 位整数范围内，无精度损失。

对应的 `HasBalance` 改为读取 Hash 并计算余额是否大于 0。

### 8.3 当前选型

对比方案一和方案二后，当前版本 **暂时使用方案一（单 Key 定点数）**。原因如下：

- v0.4 阶段 RMB 配额余额上限在 9 千万元以内即可满足业务需求；
- 方案一实现简单，Lua 脚本与现有 Token 扣减逻辑更接近，测试和运维成本更低。

若后续业务需要支持更大余额上限，再评估迁移到方案二。

## 9. 配置文件示例

`cluster_conf.data` 中的 `AIConf` 示例：

```json
{
    "AIConf": {
        "Type": 0,
        "Provider": "deepseek",
        "Keys": [
            {
                "Name": "key-primary",
                "Key": "sk-xxxxxxxxxxxx",
                "Weight": 70
            }
        ],
        "KeyPolicy": {
            "Strategy": "weighted_random",
            "MaxRetries": 3,
            "RetryBackoffInitial": 500,
            "RetryBackoffMax": 5000
        },
        "ModelMapping": {
            "gpt-4": "deepseek-chat"
        },
        "ModelTable": {
            "Currency": "RMB",
            "Models": [
                {
                    "Provider": "deepseek",
                    "Model": "deepseek-chat",
                    "BaseModel": "deepseek-chat",
                    "Mode": "chat",
                    "Capabilities": ["chat"],
                    "SupportedParameters": ["temperature", "max_tokens"],
                    "Limits": {
                        "context_window": 128000
                    },
                    "Prices": {
                        "input_cost_per_token": 0.000001,
                        "output_cost_per_token": 0.000002
                    }
                }
            ]
        }
    }
}
```

> 上例中 `input_cost_per_token = 0.000001` 元/Token，换算为固定点整数即 `100`（= 0.000001 * 1e8），表示 **0.1 元 / 百万 Token**。

## 10. 测试建议

1. **单元测试**
   - `QuotaPlan.Deduct`：分别覆盖 `total_token` 和 `RMB` 两种单位，以及余额不足、Key 不存在等边界。
   - `lookupModelPrice`：精确匹配、未命中返回 nil。
   - `calcCostUnits`：正常计算、ModelTable 缺失、模型未命中、价格转换精度。

2. **Lua 脚本测试**
   - 单 Key 定点数方案：验证扣减、余额归零、负数不溢出。
   - Hash 拆分方案：验证借位、余额归零、大数（接近 100 亿元）正确性。

3. **集成测试**
   - 创建一个 `unit = "RMB"` 的 API Key，发一次 chat 请求，验证 Redis 余额按预期扣减。
   - 测试 `ModelMapping` 场景：请求模型是 `gpt-4`，实际后端模型是 `deepseek-chat`，验证按 `deepseek-chat` 的价格计费。
   - 测试 fallback 场景：请求最终 fallback 到另一个 cluster，验证按最终 cluster + target_model 计费。
   - 测试流式（SSE）场景：请求体带 `stream: true`，后端返回 SSE 并在最后一个 chunk 中携带 `usage`，验证 RMB 配额仍能正确扣减。

## 11. 兼容性与注意事项

1. **存量 Token 配额完全兼容**：`Unit` 默认 `"total_token"`，走原有 Lua 扣减逻辑。
2. **浮点禁止进入 Redis**：所有金额在 BFE 内部和 Redis 中均以固定点整数表示，避免浮点误差；价格浮点转换仅在 BFE 配置加载阶段完成，conf-agent 不做任何转换。
3. **无 ModelTable 时的兜底行为**：若 RMB 配额计划命中的 cluster 没有配置 `ModelTable`，或没有匹配到模型条目，当前建议：
   - 记录告警日志；
   - 本次请求不对该 RMB 配额进行扣减（相当于按 `0` 成本处理）；
   - 具体是否拒绝请求，需产品进一步确认。
4. **与多 Key 改造的关系**：`AIConf.Keys` 与 `ModelTable` 相互独立，可并行下发、独立解析。
5. **流式响应计费**：RMB 成本在请求结束阶段计算，依赖 `mod_body_process`（或其他响应处理模块）在流式传输过程中填充 `PromptTokens` / `CompletionTokens`。生产环境若启用流式计费，需确保 `mod_body_process` 已加载。
6. **模块顺序建议**：`mod_ai_token_auth` 的 `HandleReadResponse` 不再负责 RMB 成本计算，因此对模块加载顺序的敏感度降低；但仍建议保持 `mod_ai_token_auth` 在 `mod_body_process` 之前注册，以便非流式场景下 token 用量解析逻辑保持一致。
7. **旧字段清理**：`AIConf.Key` 已移除，统一使用 `AIConf.Keys`。
