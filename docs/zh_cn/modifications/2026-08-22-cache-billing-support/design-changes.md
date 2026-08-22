# BFE 支持 Cache 计费能力扩展

## 1. 背景

AI 网关场景下，部分模型（如 `claude-opus-4-6`）的 usage 中携带 `cache_read_tokens` 和 `cache_write_tokens`，需按 cache 规则分别计费：

- `prompt_tokens` **已包含** `cache_read_tokens`；
- `cache_write_tokens` 为**独立加项**，不体现在 `prompt_tokens` / `completion_tokens` / `total_tokens` 中；
- 普通 input 计费基数为 `prompt_tokens - cache_read_tokens`。

当前 BFE 仅支持 `input_cost_per_token` / `output_cost_per_token` 两个价格字段，无法识别 cache 用量，导致 cache 场景计费金额不准确。本变更在不破坏现有 RMB / total_token 配额逻辑的前提下，扩展 BFE 对 cache 计费的端到端支持。

---

## 2. 需求示例

参考示例 2 的模型与 usage：

| 价格字段 | USD / token |
|---|---|
| `input_cost_per_token` | `4.525e-06` |
| `output_cost_per_token` | `2.2625e-05` |
| `cache_read_input_token_cost` | `4.525e-07` |
| `cache_creation_input_token_cost` | `5.65625e-06` |

| Usage 字段 | 值 |
|---|---|
| `prompt_tokens` | 8000 |
| `cache_read_tokens` | 5000 |
| `cache_write_tokens` | 1000 |
| `completion_tokens` | 1500 |

计费公式：

```
normal_input = prompt_tokens - cache_read_tokens = 3000

cost = normal_input × input_cost_per_token
     + cache_read_tokens × cache_read_input_token_cost
     + cache_write_tokens × cache_creation_input_token_cost
     + completion_tokens × output_cost_per_token
     = 0.05543125 USD
```

---

## 3. 当前现状

| 层级 | 当前能力 | 不足 |
|---|---|---|
| 数据结构 | `bfe_basic.TokenUsage` 仅含 `PromptTokens`、`CompletionTokens`、`UsedQuota`、`UsedCost` | 缺少 cache 字段 |
| 价格配置 | `ModelPrice.Prices` 仅识别 `input_cost_per_token`、`output_cost_per_token` | 缺少 cache 单价 |
| Usage 解析 | 非流式 `UpdateCtxByUsage`、流式 `SSEEvent.GetQuotaUsage` / `QuotaUsageProcessor.Process` 只解析 `total_tokens`、`prompt_tokens`、`completion_tokens` | 不识别 `cache_read_tokens`、`cache_write_tokens` |
| 费用计算 | `calcCostUnits` 仅按 `promptTokens*inputCost + completionTokens*outputCost` 计算 | 无法拆分 cache 计费 |
| 访问日志 | `RequestLog` 只有 `ai_input_tokens`、`ai_output_tokens` 等 | 缺少 cache 明细 |

---

## 4. 变更目标

1. 在 `TokenUsage` 中增加 `CacheReadTokens`、`CacheWriteTokens`。
2. 在模型价格表中支持 `cache_read_input_token_cost`、`cache_creation_input_token_cost`。
3. 改造 usage 解析、费用计算、访问日志三个环节，识别并正确拆分 cache 计费。
4. 未配置 cache 单价时退化到原有逻辑，保持向后兼容。
5. 保持 `UsedQuota`（total_token 维度）为 `prompt_tokens + completion_tokens`，与现有配额机制对齐。
6. 补充单元测试与集成测试。

---

## 5. 变更总览

| 模块 | 主要改动 |
|---|---|
| `bfe_basic` | `TokenUsage` 增加 `CacheReadTokens`、`CacheWriteTokens` |
| `bfe_config/bfe_cluster_conf/cluster_conf` | 增加 cache 价格常量，`ModelTableCheck` 识别并转换 cache 单价为定点整数 |
| `mod_ai_token_auth` | `UpdateCtxByUsage` 解析 cache usage；`calcCostUnits` 按 cache 拆分公式计费 |
| `mod_body_process` | `SSEEvent.GetQuotaUsage`、`QuotaUsageProcessor.Process` 解析并累积 cache usage |
| `bfe-access-pb` / `mod_access_pb3` | 访问日志新增 `ai_cache_read_tokens`、`ai_cache_write_tokens`（可选但建议） |
| 测试 | 补充 cache 计费、流式/非流式、无 cache 单价退化等测试 |

---

## 6. 详细设计

### 6.1 扩展 `TokenUsage`

**文件：** `bfe/bfe_basic/request_ai_basic.go`

```go
type TokenUsage struct {
    PromptTokens     int64 // 含 cache_read
    CompletionTokens int64
    CacheReadTokens  int64 // usage.cache_read_tokens，已包含在 PromptTokens 中
    CacheWriteTokens int64 // usage.cache_write_tokens，独立加项
    UsedQuota        int64 // unit=total_token
    UsedCost         int64 // unit=RMB，1 unit = 1e-8 yuan
}
```

说明：
- `UsedQuota` 保持为 `prompt_tokens + completion_tokens`，用于 `total_token` 配额扣减。
- RMB 计费使用 `PromptTokens - CacheReadTokens` 作为普通 input，避免 cache read 被重复计费。

### 6.2 扩展价格配置

**文件：** `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`

新增常量：

```go
const (
    PriceInputCostPerToken            = "input_cost_per_token"
    PriceOutputCostPerToken           = "output_cost_per_token"
    PriceCacheReadInputTokenCost      = "cache_read_input_token_cost"
    PriceCacheCreationInputTokenCost  = "cache_creation_input_token_cost"

    PriceInputCostPerTokenInt            = "input_cost_per_token_int"
    PriceOutputCostPerTokenInt           = "output_cost_per_token_int"
    PriceCacheReadInputTokenCostInt      = "cache_read_input_token_cost_int"
    PriceCacheCreationInputTokenCostInt  = "cache_creation_input_token_cost_int"
)
```

在 `ModelTableCheck` 中增加 cache 单价的读取与定点转换：

```go
input       := price.Prices[PriceInputCostPerToken]
output      := price.Prices[PriceOutputCostPerToken]
cacheRead   := price.Prices[PriceCacheReadInputTokenCost]
cacheWrite  := price.Prices[PriceCacheCreationInputTokenCost]

if input < 0 || output < 0 || cacheRead < 0 || cacheWrite < 0 {
    return fmt.Errorf("negative price for model %s", price.Model)
}

price.Prices[PriceInputCostPerTokenInt]           = float64(quota.RmbToFixedPoint(input))
price.Prices[PriceOutputCostPerTokenInt]          = float64(quota.RmbToFixedPoint(output))
price.Prices[PriceCacheReadInputTokenCostInt]     = float64(quota.RmbToFixedPoint(cacheRead))
price.Prices[PriceCacheCreationInputTokenCostInt] = float64(quota.RmbToFixedPoint(cacheWrite))
```

cache 单价为可选配置：
- 配置了 cache 单价 → 按 cache 拆分计费；
- 未配置 → 退化到 `promptTokens*inputCost + completionTokens*outputCost`。

### 6.3 非流式 Usage 解析

**文件：** `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

函数 `UpdateCtxByUsage` 增加 cache 字段解析：

```go
cacheRead  := gjson.GetBytes(data, "usage.cache_read_tokens").Int()
cacheWrite := gjson.GetBytes(data, "usage.cache_write_tokens").Int()

tokenUsage.CacheReadTokens = cacheRead
tokenUsage.CacheWriteTokens = cacheWrite
```

### 6.4 流式 Usage 解析

**文件：** `bfe/bfe_modules/mod_body_process/llm_util.go`

函数 `SSEEvent.GetQuotaUsage` 返回的 `QuotaUsage` 增加 cache 字段：

```go
type QuotaUsage struct {
    PromptTokens     int64
    CompletionTokens int64
    CacheReadTokens  int64
    CacheWriteTokens int64
    UsedQuota        int64
    CurrentTokens    int64
    IsGuess          bool
}
```

函数内解析 `usage.cache_read_tokens`、`usage.cache_write_tokens`。

**文件：** `bfe/bfe_modules/mod_body_process/body_process.go`

非流式响应使用的 `RawEvent.GetQuotaUsage` 同样需要解析 `usage.cache_read_tokens`、`usage.cache_write_tokens`，以保证 `QuotaUsageProcessor` 在两种响应格式下都能正确收集 cache 用量。

**文件：** `bfe/bfe_modules/mod_body_process/content_quota_usage.go`

`QuotaUsageProcessor.Process` 在非 guess 事件覆盖 `tctx` 时同步写入 `CacheReadTokens`、`CacheWriteTokens`。

> 流式场景下 cache usage 通常只在最后一个 SSE 事件中出现，因此最后一个有效事件会覆盖之前的值。

### 6.5 RMB 费用计算改造

**文件：** `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

函数 `calcCostUnits` 签名增加 `usage *bfe_basic.TokenUsage`：

```go
func (m *ModuleAITokenAuth) calcCostUnits(
    req *bfe_basic.Request,
    serverConf bfe_basic.ServerDataConfInterface,
    usage *bfe_basic.TokenUsage,
) int64
```

计费逻辑：

```go
promptTokens     := usage.PromptTokens
completionTokens := usage.CompletionTokens
cacheReadTokens  := usage.CacheReadTokens
cacheWriteTokens := usage.CacheWriteTokens

// 边界保护
if cacheReadTokens < 0 {
    cacheReadTokens = 0
}
if cacheReadTokens > promptTokens {
    cacheReadTokens = promptTokens
}
if cacheWriteTokens < 0 {
    cacheWriteTokens = 0
}

cacheReadCost  := int64(entry.Prices[cluster_conf.PriceCacheReadInputTokenCostInt])
cacheWriteCost := int64(entry.Prices[cluster_conf.PriceCacheCreationInputTokenCostInt])

var cost int64
if cacheReadCost > 0 || cacheWriteCost > 0 {
    normalInput := promptTokens - cacheReadTokens
    if normalInput < 0 {
        normalInput = 0
    }
    cost = normalInput*inputCost +
        cacheReadTokens*cacheReadCost +
        cacheWriteTokens*cacheWriteCost +
        completionTokens*outputCost
} else {
    cost = promptTokens*inputCost + completionTokens*outputCost
}

return cost
```

调用点同步修改：

```go
if tokenUsage.UsedCost <= 0 && hasRMBPlan(ctx.Token.QuotaPlans) {
    tokenUsage.UsedCost = m.calcCostUnits(req, ctx.serverConf, tokenUsage)
}
```

### 6.6 访问日志扩展（建议）

**文件：** `bfe-access-pb/bfe_access_pb/bfe_access.proto`

在 AI Observability 区域新增字段：

```protobuf
optional int64 ai_cache_read_tokens  = 781;
optional int64 ai_cache_write_tokens = 782;
```

**文件：** `bfe/bfe_modules/mod_access_pb3/request_log.go`

在 `reqAiInfoGen` 中补充：

```go
if usage.CacheReadTokens > 0 {
    reqLog.AiCacheReadTokens = proto.Int64(usage.CacheReadTokens)
}
if usage.CacheWriteTokens > 0 {
    reqLog.AiCacheWriteTokens = proto.Int64(usage.CacheWriteTokens)
}
```

修改 proto 后需执行 `bfe-access-pb/build.sh` 重新生成 Go 代码。

---

## 7. 计费公式速查

### 7.1 含 Cache 的模型

前提：`cache_read_input_token_cost` 或 `cache_creation_input_token_cost` 已配置。

```
normal_input = max(prompt_tokens - cache_read_tokens, 0)

cost = normal_input × input_cost_per_token
     + cache_read_tokens × cache_read_input_token_cost
     + cache_write_tokens × cache_creation_input_token_cost
     + completion_tokens × output_cost_per_token
```

### 7.2 不含 Cache 的模型（向后兼容）

```
cost = prompt_tokens × input_cost_per_token
     + completion_tokens × output_cost_per_token
```

---

## 8. 边界情况与兼容性

| 场景 | 处理建议 |
|---|---|
| 后端未返回 cache 字段 | 字段值为 0，按不含 cache 计费 |
| `cache_read_tokens > prompt_tokens` | 截断为 `prompt_tokens`，避免普通 input 为负 |
| `cache_read_tokens` 或 `cache_write_tokens` 为负 | 按 0 处理 |
| 价格表只配置了部分 cache 单价 | 只要任一 cache 单价 > 0，启用 cache 拆分；否则退化 |
| 流式响应中 cache usage 出现在中间事件 | 非 guess 事件覆盖，最终事件为准 |
| `UsedQuota` 维度 | 继续为 `prompt_tokens + completion_tokens`，cache_write 不累加 |
| 非 chat 模式 | 当前 BFE 仅支持 chat；若后续扩展，需同步修改 `LookupModelPrice` 的 mode 参数 |

---

## 9. 测试计划

### 9.1 单元测试

在 `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` 中新增：

1. `TestCalcCostUnits_Cache`：验证 cache 拆分计费公式。
2. `TestUpdateCtxByUsage_Cache`：验证 cache 字段解析。
3. `TestTokenRequestFinishHandler_RMB_Cache_NonStreaming`：非流式端到端扣费。
4. `TestTokenRequestFinishHandler_RMB_Cache_Streaming`：流式端到端扣费。
5. `TestCalcCostUnits_CacheFallback`：未配置 cache 单价时退化。
6. `TestCalcCostUnits_CacheReadExceedsPrompt`：边界处理。

### 9.2 集成测试

在 `bfe/tests/integration/implementation/scenario-SC03-rmb-quota/sc03_rmb_quota_test.go` 中新增：

1. `TestTC08_RMBQuotaDeduction_Cache_NonStreaming`：
   - 后端返回含 `cache_read_tokens` / `cache_write_tokens` 的非流式响应；
   - `ModelTable` 配置 cache 单价；
   - 验证 Redis 扣减金额按 cache 拆分公式计算。
2. `TestTC09_RMBQuotaDeduction_Cache_Streaming`：
   - 后端返回 SSE 流，最终 chunk 含 cache usage；
   - 验证流式场景下 Redis 仍按 cache 拆分公式扣减。

在 `bfe/tests/integration/implementation/scenario-SC05-access-log-ai-fields/sc05_access_log_ai_fields_test.go` 中新增：

3. `TestTC08_CacheTokenFields`：
   - 后端返回含 `cache_read_tokens` / `cache_write_tokens` 的 usage；
   - `ModelTable` 配置 cache 单价；
   - 验证 `mod_access_pb3` 输出的 b2log 中 `ai_cache_read_tokens`、`ai_cache_write_tokens` 与 cache-aware `ai_cost_value` 正确。

### 9.3 配置加载测试

在 `bfe/bfe_config/bfe_cluster_conf/cluster_conf/` 中：

1. 验证 `ModelTableCheck` 正确转换 cache 单价为定点整数；
2. 验证未配置 cache 单价时不报错。

---

## 10. 实施步骤建议

1. **数据结构与配置层**：扩展 `TokenUsage`；增加 cache 价格常量及转换逻辑。
2. **Usage 解析**：非流式与流式模块同时解析 cache usage。
3. **计费逻辑**：改造 `calcCostUnits`，实现 cache 拆分计费及退化逻辑；同步调用点与单元测试。
4. **日志与可观测性**：扩展 `bfe_access.proto` 与 `request_log.go`。
5. **测试与回归**：补充单元测试、集成测试，验证现有 RMB / total_token 配额场景不受影响。

---

## 11. 影响范围

| 模块/文件 | 影响 |
|---|---|
| `bfe/bfe_basic/request_ai_basic.go` | `TokenUsage` 新增 cache 字段 |
| `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | 新增 cache 价格常量与转换逻辑 |
| `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | Usage 解析与费用计算改造 |
| `bfe/bfe_modules/mod_body_process/llm_util.go` | `QuotaUsage` 与 `SSEEvent.GetQuotaUsage` 新增 cache 字段 |
| `bfe/bfe_modules/mod_body_process/body_process.go` | `RawEvent.GetQuotaUsage` 新增 cache 字段 |
| `bfe/bfe_modules/mod_body_process/content_quota_usage.go` | 流式/非流式 cache usage 累积 |
| `bfe-access-pb/bfe_access_pb/bfe_access.proto` | 访问日志新增 cache 字段 |
| `bfe/bfe_modules/mod_access_pb3/request_log.go` | 序列化 cache 字段 |
| `bfe/tests/integration/implementation/scenario-SC03-rmb-quota/sc03_rmb_quota_test.go` | 新增 cache 计费集成测试 |
| `bfe/tests/integration/implementation/scenario-SC05-access-log-ai-fields/sc05_access_log_ai_fields_test.go` | 新增 cache 访问日志字段集成测试 |
| `bfe/docs/zh_cn/modifications/2026-08-22-cache-billing-support/design-changes.md` | 本设计变更文档 |

---

## 12. 兼容性与风险

### 12.1 兼容性

- 未配置 cache 单价的模型行为完全不变。
- Token 配额（`total_token`）扣减逻辑不变。
- Redis key 结构、配置格式均不发生改变。
- `UsedQuota` 继续保持 `prompt_tokens + completion_tokens`。

### 12.2 风险与缓解

| 风险 | 缓解措施 |
|---|---|
| 后端返回异常 cache 数据导致普通 input 为负 | `calcCostUnits` 中校验并截断 `cache_read_tokens` |
| 流式 cache usage 事件提前或延后 | `QuotaUsageProcessor` 在非 guess 事件时覆盖，最终以可靠事件为准 |
| proto 字段变更需重新生成 | 修改后执行 `bfe-access-pb/build.sh` |

---

## 13. 参考资料

- `document-ai-gateway/迭代系统设计/v0.5/计费能力扩展/bfe-cache-billing-support-analysis.md`
- `document-ai-gateway/迭代系统设计/v0.4/quota-rmb-support/bfe-changes-for-rmb-quota.md`
- `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`
- `bfe/bfe_modules/mod_body_process/content_quota_usage.go`
- `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`
- `bfe/bfe_basic/request_ai_basic.go`
