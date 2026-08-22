# BFE 支持音频 Token 计费能力扩展

## 1. 背景

AI 网关场景下，部分音频模型（如 `gpt-audio-1.5`）的 usage 中携带 `audio_input_tokens` 和 `audio_output_tokens`，需按音频规则分别计费：

- `audio_input_tokens` **已包含**在 `prompt_tokens` 中，计算普通 input 时需要剥离；
- `audio_output_tokens` **已包含**在 `completion_tokens` 中，计算普通 output 时需要剥离；
- 普通 input 计费基数为 `prompt_tokens - cache_read_tokens - audio_input_tokens`；
- 普通 output 计费基数为 `completion_tokens - audio_output_tokens`。

当前 BFE 已支持 cache 计费拆分，但尚未支持音频 token 从 input/output 双向剥离计费。本变更在不破坏现有 RMB / total_token / cache 计费逻辑的前提下，扩展 BFE 对音频 token 计费的端到端支持。

---

## 2. 需求示例

参考示例 4 的模型与 usage：

| 价格字段 | USD / token |
|---|---|
| `input_cost_per_token` | `1.7875e-06` |
| `output_cost_per_token` | `7.15e-06` |
| `input_cost_per_audio_token` | `2.288e-05` |
| `output_cost_per_audio_token` | `4.576e-05` |

| Usage 字段 | 值 |
|---|---|
| `prompt_tokens` | 4000 |
| `audio_input_tokens` | 1000 |
| `completion_tokens` | 500 |
| `audio_output_tokens` | 200 |

计费公式：

```
normal_input  = prompt_tokens - audio_input_tokens = 3000
normal_output = completion_tokens - audio_output_tokens = 300

cost = normal_input × input_cost_per_token
     + audio_input_tokens × input_cost_per_audio_token
     + normal_output × output_cost_per_token
     + audio_output_tokens × output_cost_per_audio_token
     = 0.0395395 USD
```

---

## 3. 当前现状

| 层级 | 当前能力 | 不足 |
|---|---|---|
| 数据结构 | `bfe_basic.TokenUsage` 已含 `PromptTokens`、`CompletionTokens`、`CacheReadTokens`、`CacheWriteTokens`、`UsedQuota`、`UsedCost` | 缺少音频字段 |
| 价格配置 | `ModelPrice.Prices` 已识别文本与 cache 单价 | 缺少音频单价 |
| Usage 解析 | 非流式 `UpdateCtxByUsage`、流式 `SSEEvent.GetQuotaUsage` / `QuotaUsageProcessor.Process` 已解析 cache 字段 | 不识别 `audio_input_tokens`、`audio_output_tokens` |
| 费用计算 | `calcCostUnits` 已支持 cache 拆分计费 | 无法拆分音频 input/output 计费 |
| 访问日志 | `RequestLog` 已有文本与 cache 明细 | 缺少音频明细 |

---

## 4. 变更目标

1. 在 `TokenUsage` 中增加 `AudioInputTokens`、`AudioOutputTokens`。
2. 在模型价格表中支持 `input_cost_per_audio_token`、`output_cost_per_audio_token`。
3. 改造 usage 解析、费用计算、访问日志三个环节，识别并正确拆分音频 token 计费。
4. 未配置音频单价时退化到原有逻辑，保持向后兼容。
5. 保持 `UsedQuota`（total_token 维度）为 `prompt_tokens + completion_tokens`，与现有配额机制对齐。
6. 补充单元测试与集成测试。

---

## 5. 变更总览

| 模块 | 主要改动 |
|---|---|
| `bfe_basic` | `TokenUsage` 增加 `AudioInputTokens`、`AudioOutputTokens` |
| `bfe_config/bfe_cluster_conf/cluster_conf` | 增加音频价格常量，`ModelTableCheck` 识别并转换音频单价为定点整数 |
| `mod_ai_token_auth` | `UpdateCtxByUsage` 解析音频 usage；`calcCostUnits` 按音频拆分公式计费 |
| `mod_body_process` | `SSEEvent.GetQuotaUsage`、`RawEvent.GetQuotaUsage`、`QuotaUsageProcessor.Process` 解析并累积音频 usage |
| `bfe-access-pb` / `mod_access_pb3` | 访问日志新增 `ai_audio_input_tokens`、`ai_audio_output_tokens`（可选但建议） |
| 测试 | 补充音频计费、流式/非流式、无音频单价退化、音频 token 超边界等测试 |

---

## 6. 详细设计

### 6.1 扩展 `TokenUsage`

**文件：** `bfe/bfe_basic/request_ai_basic.go`

```go
type TokenUsage struct {
    PromptTokens      int64 // 含 cache_read、audio_input
    CompletionTokens  int64 // 含 audio_output
    CacheReadTokens   int64 // usage.cache_read_tokens，已包含在 PromptTokens 中
    CacheWriteTokens  int64 // usage.cache_write_tokens，独立加项
    AudioInputTokens  int64 // usage.audio_input_tokens，已包含在 PromptTokens 中
    AudioOutputTokens int64 // usage.audio_output_tokens，已包含在 CompletionTokens 中
    UsedQuota         int64 // unit=total_token
    UsedCost          int64 // unit=RMB，1 unit = 1e-8 yuan
}
```

说明：
- `UsedQuota` 保持为 `prompt_tokens + completion_tokens`，用于 `total_token` 配额扣减。
- RMB 计费使用 `PromptTokens - CacheReadTokens - AudioInputTokens` 作为普通 input，`CompletionTokens - AudioOutputTokens` 作为普通 output，避免音频 token 被按文本价格重复计费。
- 与 cache 共存时，按 `prompt - cache_read - audio_input` 顺序剥离。若后端语义有重叠，需 backend 提供_disjoint_ 明细或后续调整策略。

### 6.2 扩展价格配置

**文件：** `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`

新增常量：

```go
const (
    PriceInputCostPerToken           = "input_cost_per_token"
    PriceOutputCostPerToken          = "output_cost_per_token"
    PriceCacheReadInputTokenCost     = "cache_read_input_token_cost"
    PriceCacheCreationInputTokenCost = "cache_creation_input_token_cost"
    PriceInputCostPerAudioToken      = "input_cost_per_audio_token"
    PriceOutputCostPerAudioToken     = "output_cost_per_audio_token"

    PriceInputCostPerTokenInt           = "input_cost_per_token_int"
    PriceOutputCostPerTokenInt          = "output_cost_per_token_int"
    PriceCacheReadInputTokenCostInt     = "cache_read_input_token_cost_int"
    PriceCacheCreationInputTokenCostInt = "cache_creation_input_token_cost_int"
    PriceInputCostPerAudioTokenInt      = "input_cost_per_audio_token_int"
    PriceOutputCostPerAudioTokenInt     = "output_cost_per_audio_token_int"
)
```

在 `ModelTableCheck` 中增加音频单价的读取与定点转换：

```go
input       := price.Prices[PriceInputCostPerToken]
output      := price.Prices[PriceOutputCostPerToken]
cacheRead   := price.Prices[PriceCacheReadInputTokenCost]
cacheWrite  := price.Prices[PriceCacheCreationInputTokenCost]
audioInput  := price.Prices[PriceInputCostPerAudioToken]
audioOutput := price.Prices[PriceOutputCostPerAudioToken]

if input < 0 || output < 0 || cacheRead < 0 || cacheWrite < 0 ||
    audioInput < 0 || audioOutput < 0 {
    return fmt.Errorf("negative price for model %s", price.Model)
}

price.Prices[PriceInputCostPerTokenInt]           = float64(quota.RmbToFixedPoint(input))
price.Prices[PriceOutputCostPerTokenInt]          = float64(quota.RmbToFixedPoint(output))
price.Prices[PriceCacheReadInputTokenCostInt]     = float64(quota.RmbToFixedPoint(cacheRead))
price.Prices[PriceCacheCreationInputTokenCostInt] = float64(quota.RmbToFixedPoint(cacheWrite))
price.Prices[PriceInputCostPerAudioTokenInt]      = float64(quota.RmbToFixedPoint(audioInput))
price.Prices[PriceOutputCostPerAudioTokenInt]     = float64(quota.RmbToFixedPoint(audioOutput))
```

音频单价为可选配置：
- 配置了对应方向音频单价 → 按音频拆分计费；
- 未配置 → 该方向音频 token 保留在普通 input/output 中按文本价格计费，避免漏计。

### 6.3 非流式 Usage 解析

**文件：** `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

函数 `UpdateCtxByUsage` 增加音频字段解析：

```go
audioInput  := gjson.GetBytes(data, "usage.audio_input_tokens").Int()
audioOutput := gjson.GetBytes(data, "usage.audio_output_tokens").Int()

tokenUsage.AudioInputTokens = audioInput
tokenUsage.AudioOutputTokens = audioOutput
```

### 6.4 流式 Usage 解析

**文件：** `bfe/bfe_modules/mod_body_process/llm_util.go`

函数 `SSEEvent.GetQuotaUsage` 返回的 `QuotaUsage` 增加音频字段：

```go
type QuotaUsage struct {
    PromptTokens      int64
    CompletionTokens  int64
    CacheReadTokens   int64
    CacheWriteTokens  int64
    AudioInputTokens  int64
    AudioOutputTokens int64
    UsedQuota         int64
    CurrentTokens     int64
    IsGuess           bool
}
```

函数内解析 `usage.audio_input_tokens`、`usage.audio_output_tokens`。

**文件：** `bfe/bfe_modules/mod_body_process/body_process.go`

非流式响应使用的 `RawEvent.GetQuotaUsage` 同样需要解析 `usage.audio_input_tokens`、`usage.audio_output_tokens`，以保证 `QuotaUsageProcessor` 在两种响应格式下都能正确收集音频用量。

**文件：** `bfe/bfe_modules/mod_body_process/content_quota_usage.go`

`QuotaUsageProcessor.Process` 在非 guess 事件覆盖 `tctx` 时同步写入 `AudioInputTokens`、`AudioOutputTokens`。

> 流式场景下音频 usage 通常只在最后一个 SSE 事件中出现，因此最后一个有效事件会覆盖之前的值。

### 6.5 RMB 费用计算改造

**文件：** `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

函数 `calcCostUnits` 使用 `usage *bfe_basic.TokenUsage`：

```go
func (m *ModuleAITokenAuth) calcCostUnits(
    req *bfe_basic.Request,
    serverConf bfe_basic.ServerDataConfInterface,
    usage *bfe_basic.TokenUsage,
) int64
```

计费逻辑（推荐精细分支版本）：

```go
promptTokens      := usage.PromptTokens
completionTokens  := usage.CompletionTokens
cacheReadTokens   := usage.CacheReadTokens
cacheWriteTokens  := usage.CacheWriteTokens
audioInputTokens  := usage.AudioInputTokens
audioOutputTokens := usage.AudioOutputTokens

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
if audioInputTokens < 0 {
    audioInputTokens = 0
}
if audioInputTokens > promptTokens-cacheReadTokens {
    audioInputTokens = promptTokens - cacheReadTokens
}
if audioOutputTokens < 0 {
    audioOutputTokens = 0
}
if audioOutputTokens > completionTokens {
    audioOutputTokens = completionTokens
}

cacheReadCost   := int64(entry.Prices[cluster_conf.PriceCacheReadInputTokenCostInt])
cacheWriteCost  := int64(entry.Prices[cluster_conf.PriceCacheCreationInputTokenCostInt])
audioInputCost  := int64(entry.Prices[cluster_conf.PriceInputCostPerAudioTokenInt])
audioOutputCost := int64(entry.Prices[cluster_conf.PriceOutputCostPerAudioTokenInt])

// 普通 input/output 初始值
normalInput  := promptTokens
normalOutput := completionTokens

// cache 拆分
if cacheReadCost > 0 || cacheWriteCost > 0 {
    normalInput = promptTokens - cacheReadTokens
    if normalInput < 0 {
        normalInput = 0
    }
}

// 音频 input 拆分：从 normalInput 中再剥离 audio_input
if audioInputCost > 0 {
    audioInputTokens = min(audioInputTokens, normalInput)
    normalInput = normalInput - audioInputTokens
    if normalInput < 0 {
        normalInput = 0
    }
} else {
    audioInputTokens = 0 // 不配置音频 input 单价时，按文本价格计费
}

// 音频 output 拆分
if audioOutputCost > 0 {
    audioOutputTokens = min(audioOutputTokens, completionTokens)
    normalOutput = completionTokens - audioOutputTokens
    if normalOutput < 0 {
        normalOutput = 0
    }
} else {
    audioOutputTokens = 0 // 不配置音频 output 单价时，按文本价格计费
}

var cost int64
if cacheReadCost > 0 || cacheWriteCost > 0 || audioInputCost > 0 || audioOutputCost > 0 {
    cost = normalInput*inputCost +
        cacheReadTokens*cacheReadCost +
        cacheWriteTokens*cacheWriteCost +
        audioInputTokens*audioInputCost +
        normalOutput*outputCost +
        audioOutputTokens*audioOutputCost
} else {
    cost = promptTokens*inputCost + completionTokens*outputCost
}

return cost
```

调用点无需改动：

```go
if tokenUsage.UsedCost <= 0 && hasRMBPlan(ctx.Token.QuotaPlans) {
    tokenUsage.UsedCost = m.calcCostUnits(req, ctx.serverConf, tokenUsage)
}
```

### 6.6 访问日志扩展（建议）

**文件：** `bfe-access-pb/bfe_access_pb/bfe_access.proto`

在 AI Observability 区域新增字段：

```protobuf
optional int64 ai_audio_input_tokens  = 783;
optional int64 ai_audio_output_tokens = 784;
```

**文件：** `bfe/bfe_modules/mod_access_pb3/request_log.go`

在 `reqAiInfoGen` 中补充：

```go
if usage.AudioInputTokens > 0 {
    reqLog.AiAudioInputTokens = proto.Int64(usage.AudioInputTokens)
}
if usage.AudioOutputTokens > 0 {
    reqLog.AiAudioOutputTokens = proto.Int64(usage.AudioOutputTokens)
}
```

修改 proto 后需执行 `bfe-access-pb/build.sh` 重新生成 Go 代码，并同步更新 `bfe-access-pb/docs/protobuf.md`。

---

## 7. 计费公式速查

### 7.1 含音频 Token 的模型

前提：`input_cost_per_audio_token` 或 `output_cost_per_audio_token` 已配置。

```
normal_input  = max(prompt_tokens - cache_read_tokens - audio_input_tokens, 0)
normal_output = max(completion_tokens - audio_output_tokens, 0)

cost = normal_input × input_cost_per_token
     + cache_read_tokens × cache_read_input_token_cost
     + cache_write_tokens × cache_creation_input_token_cost
     + audio_input_tokens × input_cost_per_audio_token
     + normal_output × output_cost_per_token
     + audio_output_tokens × output_cost_per_audio_token
```

### 7.2 不含音频 Token 的模型（向后兼容）

```
cost = prompt_tokens × input_cost_per_token
     + completion_tokens × output_cost_per_token
```

---

## 8. 边界情况与兼容性

| 场景 | 处理建议 |
|---|---|
| 后端未返回音频字段 | 字段值为 0，按不含音频计费 |
| `audio_input_tokens > prompt_tokens` | 截断为 `prompt_tokens - cache_read_tokens`，避免普通 input 为负 |
| `audio_output_tokens > completion_tokens` | 截断为 `completion_tokens`，避免普通 output 为负 |
| `audio_input_tokens` 或 `audio_output_tokens` 为负 | 按 0 处理 |
| 价格表只配置了部分音频单价 | 仅当对应方向音频单价 > 0 时才剥离并单独计价；否则该方向音频 token 保留在普通 input/output 中按文本价格计费 |
| 流式响应中音频 usage 出现在中间事件 | 非 guess 事件覆盖，最终事件为准 |
| `UsedQuota` 维度 | 继续为 `prompt_tokens + completion_tokens`，音频 token 不累加 |
| 与 cache 同时存在 | 按 `prompt - cache_read - audio_input` 计算普通 input；若后端语义有重叠，需 backend 提供_disjoint_ 明细或后续调整策略 |
| 非 chat 模式 | 当前 BFE 仅支持 chat；若后续扩展，需同步修改 `LookupModelPrice` 的 mode 参数 |

---

## 9. 测试计划

### 9.1 单元测试

在 `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` 中新增：

1. `TestCalcCostUnits_Audio`：验证音频 input/output 双向拆分计费公式。
2. `TestUpdateCtxByUsage_Audio`：验证音频字段解析。
3. `TestTokenRequestFinishHandler_RMB_Audio_NonStreaming`：非流式端到端扣费。
4. `TestTokenRequestFinishHandler_RMB_Audio_Streaming`：流式端到端扣费。
5. `TestCalcCostUnits_AudioFallback`：未配置音频单价时退化。
6. `TestCalcCostUnits_AudioInputExceedsPrompt`：边界处理。
7. `TestCalcCostUnits_AudioOutputExceedsCompletion`：边界处理。
8. `TestCalcCostUnits_CacheAndAudio`：cache 与音频同时存在时的计费正确性。

### 9.2 集成测试

在 `bfe/tests/integration/implementation/scenario-SC03-rmb-quota/sc03_rmb_quota_test.go` 中新增：

1. `TestTC10_RMBQuotaDeduction_Audio_NonStreaming`：
   - 后端返回含 `audio_input_tokens` / `audio_output_tokens` 的非流式响应；
   - `ModelTable` 配置音频单价；
   - 验证 Redis 扣减金额按音频拆分公式计算。
2. `TestTC11_RMBQuotaDeduction_Audio_Streaming`：
   - 后端返回 SSE 流，最终 chunk 含音频 usage；
   - 验证流式场景下 Redis 仍按音频拆分公式扣减。

在 `bfe/tests/integration/implementation/scenario-SC05-access-log-ai-fields/sc05_access_log_ai_fields_test.go` 中新增：

3. `TestTC09_AudioTokenFields`：
   - 后端返回含 `audio_input_tokens` / `audio_output_tokens` 的 usage；
   - `ModelTable` 配置音频单价；
   - 验证 `mod_access_pb3` 输出的 b2log 中 `ai_audio_input_tokens`、`ai_audio_output_tokens` 与 audio-aware `ai_cost_value` 正确。

### 9.3 配置加载测试

在 `bfe/bfe_config/bfe_cluster_conf/cluster_conf/` 中：

1. 验证 `ModelTableCheck` 正确转换音频单价为定点整数；
2. 验证未配置音频单价时不报错；
3. 验证音频单价为负数时报错。

---

## 10. 实施步骤建议

1. **数据结构与配置层**：扩展 `TokenUsage`；增加音频价格常量及转换逻辑。
2. **Usage 解析**：非流式与流式模块同时解析音频 usage。
3. **计费逻辑**：改造 `calcCostUnits`，实现音频 input/output 双向拆分计费及退化逻辑；同步调用点与单元测试。
4. **日志与可观测性**：扩展 `bfe_access.proto` 与 `request_log.go`。
5. **测试与回归**：补充单元测试、集成测试，验证现有 RMB / total_token / cache 计费场景不受影响。

---

## 11. 影响范围

| 模块/文件 | 影响 |
|---|---|
| `bfe/bfe_basic/request_ai_basic.go` | `TokenUsage` 新增音频字段 |
| `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | 新增音频价格常量与转换逻辑 |
| `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | Usage 解析与费用计算改造 |
| `bfe/bfe_modules/mod_body_process/llm_util.go` | `QuotaUsage` 与 `SSEEvent.GetQuotaUsage` 新增音频字段 |
| `bfe/bfe_modules/mod_body_process/body_process.go` | `RawEvent.GetQuotaUsage` 新增音频字段 |
| `bfe/bfe_modules/mod_body_process/content_quota_usage.go` | 流式/非流式音频 usage 累积 |
| `bfe-access-pb/bfe_access_pb/bfe_access.proto` | 访问日志新增音频字段 |
| `bfe/bfe_modules/mod_access_pb3/request_log.go` | 序列化音频字段 |
| `bfe/tests/integration/implementation/scenario-SC03-rmb-quota/sc03_rmb_quota_test.go` | 新增音频计费集成测试 |
| `bfe/tests/integration/implementation/scenario-SC05-access-log-ai-fields/sc05_access_log_ai_fields_test.go` | 新增音频访问日志字段集成测试 |
| `bfe/docs/zh_cn/modifications/2026-08-22-audio-token-billing-support/design-changes.md` | 本设计变更文档 |

---

## 12. 兼容性与风险

### 12.1 兼容性

- 未配置音频单价的模型行为完全不变。
- Token 配额（`total_token`）扣减逻辑不变。
- Cache 计费逻辑不变。
- Redis key 结构、配置格式均不发生改变。
- `UsedQuota` 继续保持 `prompt_tokens + completion_tokens`。

### 12.2 风险与缓解

| 风险 | 缓解措施 |
|---|---|
| 后端返回异常音频数据导致普通 input/output 为负 | `calcCostUnits` 中校验并截断 `audio_input_tokens` / `audio_output_tokens` |
| 流式音频 usage 事件提前或延后 | `QuotaUsageProcessor` 在非 guess 事件时覆盖，最终以可靠事件为准 |
| 与 cache 共存时重叠导致普通 input 为负 | 截断保护；若后端语义有重叠，需 backend 提供_disjoint_ 明细 |
| proto 字段变更需重新生成 | 修改后执行 `bfe-access-pb/build.sh` |

---

## 13. 参考资料

- `document-ai-gateway/迭代系统设计/v0.5/计费能力扩展/bfe-audio-token-billing-support-analysis.md`
- `document-ai-gateway/迭代系统设计/v0.5/计费能力扩展/bfe-cache-billing-support-analysis.md`
- `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`
- `bfe/bfe_modules/mod_body_process/content_quota_usage.go`
- `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`
- `bfe/bfe_basic/request_ai_basic.go`
