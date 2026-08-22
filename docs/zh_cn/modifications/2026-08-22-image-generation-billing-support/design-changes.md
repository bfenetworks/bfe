# BFE 支持图像生成按次计费能力扩展

## 1. 背景

AI 网关场景下，图像生成模型（如 `flux-2-pro`）不按 Token 计费，而是按实际生成的图像张数计费。`model-list.yaml` 中对应的价格字段为 `output_cost_per_image`，响应 usage 中对应字段为 `image_count`。

当前 BFE 的 RMB 计费逻辑仅支持 `mode = "chat"` 的 Token 计费，`calcCostUnits` 中硬编码 `"chat"`，且未识别 `output_cost_per_image` 与 `image_count`。本变更在不破坏现有 RMB / total_token / cache / audio 计费逻辑的前提下，扩展 BFE 对 `image_generation` 模式按次计费的端到端支持，并补充请求 mode 识别与访问日志字段。

---

## 2. 需求示例

参考 `model-list对账示例.md` 示例 3 的模型与 usage：

| 价格字段 | USD / 张 |
|---|---|
| `output_cost_per_image` | `0.03` |

| Usage 字段 | 值 |
|---|---|
| `image_count` | 2 |

计费公式：

```
cost = image_count × output_cost_per_image
     = 2 × 0.03
     = 0.06 USD
```

---

## 3. 当前现状

| 层级 | 当前能力 | 不足 |
|---|---|---|
| 数据结构 | `bfe_basic.AiBasicInfo` 无 `Mode`；`TokenUsage` 仅含文本/cache/audio 字段 | 缺少 `Mode`、`ImageCount` |
| 价格配置 | `ModelPrice.Prices` 已识别文本、cache、音频单价 | 缺少 `output_cost_per_image` |
| Usage 解析 | 非流式 `UpdateCtxByUsage`、流式 `SSEEvent.GetQuotaUsage` / `QuotaUsageProcessor.Process` 只解析 Token 相关字段 | 不识别 `image_count`，也不识别图像生成响应 `data` 数组 |
| 费用计算 | `calcCostUnits` 硬编码 `mode = "chat"`，仅按 Token 计费 | 无法按 `image_generation` 模式匹配定价条目，无法按次计费 |
| 访问日志 | `RequestLog` 已有文本/cache/audio 明细 | 缺少 `ai_mode`、`ai_image_count` |
| 请求转发 | 转发链路路径无关，已能透传 `/v1/images/generations` | 转发正常，但计费侧无法识别 |

---

## 4. 变更目标

1. 在 `AiBasicInfo` 中增加 `Mode`，在 `TokenUsage` / `QuotaUsage` 中增加 `ImageCount`。
2. 在模型价格表中支持 `output_cost_per_image`。
3. 根据请求路径推断请求 mode（如 `/v1/images/generations` → `image_generation`）。
4. 改造 usage 解析、费用计算、访问日志三个环节，识别并正确按图像张数计费。
5. 未配置 `output_cost_per_image` 时按 0 成本 fail-open 处理，保持向后兼容。
6. `image_generation` 模式下 `UsedQuota` 使用 `image_count`，使 `total_token` 配额也能按次扣减。
7. 补充单元测试与集成测试。

---

## 5. 变更总览

| 模块 | 主要改动 |
|---|---|
| `bfe_server` | `http_conn.go` 中根据请求路径设置 `AiBasicInfo.Mode` |
| `bfe_basic` | `AiBasicInfo` 增加 `Mode`；`TokenUsage` 增加 `ImageCount`；新增 `DetectModeFromPath` |
| `bfe_config/bfe_cluster_conf/cluster_conf` | 增加 `output_cost_per_image` 常量，`ModelTableCheck` 识别并转换为定点整数 |
| `mod_ai_token_auth` | `UpdateCtxByUsage` 解析 `image_count` / `data.#`；`calcCostUnits` 按 mode 分支计费 |
| `mod_body_process` | `QuotaUsage` / `SSEEvent.GetQuotaUsage` / `RawEvent.GetQuotaUsage` / `QuotaUsageProcessor.Process` 解析并累积 `image_count` |
| `bfe-access-pb` / `mod_access_pb3` | 访问日志新增 `ai_mode`、`ai_image_count`（可选但建议） |
| 测试 | 补充图像生成计费、非流式、流式、无 usage 时按 data 数组/request.n fallback 等测试 |

---

## 6. 详细设计

### 6.1 扩展 `AiBasicInfo` 与 `TokenUsage`

**文件：** `bfe/bfe_basic/request_ai_basic.go`

```go
const (
    ModeChat               = "chat"
    ModeCompletion         = "completion"
    ModeImageGeneration    = "image_generation"
    ModeImageEdit          = "image_edit"
    ModeEmbedding          = "embedding"
    ModeAudioSpeech        = "audio_speech"
    ModeAudioTranscription = "audio_transcription"
    ModeRerank             = "rerank"
    ModeVideoGeneration    = "video_generation"
    ModeOcr                = "ocr"
    ModeSearch             = "search"
    ModeRealtime           = "realtime"
)

type AiBasicInfo struct {
    ClientApiKey    string
    ClientKeyId     string
    ClientModel     string
    TargetModel     string
    Mode            string          // 新增：请求模式
    Provider        string
    RetryCount      uint32
    CostCurrency    string
    tokenUsage      TokenUsage
    // ...
}

type TokenUsage struct {
    PromptTokens     int64
    CompletionTokens int64
    CacheReadTokens  int64
    CacheWriteTokens int64
    ImageCount       int64 // 新增：本次请求生成的图像张数
    UsedQuota        int64 // unit=total_token
    UsedCost         int64 // unit=RMB，1 unit = 1e-8 yuan
}
```

说明：
- `Mode` 在请求进入 BFE 时根据 `HttpRequest.URL.Path` 设置。
- `ImageCount` 用于 `image_generation` 模式，`total_token` 配额按 `ImageCount` 扣减。

### 6.2 请求 mode 识别

**文件：** `bfe/bfe_server/http_conn.go`

在 `serveRequest()` 初始化 `AiBasicInfo` 后：

```go
if c.server.Config.Server.EnableAiGateway {
    aiMeta := request.InitAiBasicInfo()
    aiMeta.SetAllowEstimateToken(c.server.Config.Server.EstimateToken)
    apikey := bfe_basic.GetApiKey(request)
    if len(apikey) > 0 {
        aiMeta.ClientApiKey = apikey
    }

    model, err := condition.ReqBodyJsonFetch(request, "model", nil)
    if err == nil || len(model) > 0 {
        aiMeta.ClientModel = model
        aiMeta.TargetModel = model
    }

    aiMeta.Mode = bfe_basic.DetectModeFromPath(request.HttpRequest.URL.Path)
}
```

**文件：** `bfe/bfe_basic/request_ai_basic.go`

```go
func DetectModeFromPath(path string) string {
    switch {
    case strings.HasPrefix(path, "/v1/images/generations"):
        return ModeImageGeneration
    case strings.HasPrefix(path, "/v1/images/edits"):
        return ModeImageEdit
    case strings.HasPrefix(path, "/v1/chat/completions"):
        return ModeChat
    case strings.HasPrefix(path, "/v1/completions"):
        return ModeCompletion
    case strings.HasPrefix(path, "/v1/embeddings"):
        return ModeEmbedding
    case strings.HasPrefix(path, "/v1/audio/speech"):
        return ModeAudioSpeech
    case strings.HasPrefix(path, "/v1/audio/transcriptions"):
        return ModeAudioTranscription
    case strings.HasPrefix(path, "/v1/rerank"):
        return ModeRerank
    default:
        return ModeChat
    }
}
```

说明：
- 默认 fallback 为 `chat`，保持向后兼容。
- 后续可扩展为从 cluster / provider 配置中读取 endpoint → mode 映射。

### 6.3 扩展价格配置

**文件：** `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`

新增常量：

```go
const (
    PriceOutputCostPerImage    = "output_cost_per_image"
    PriceOutputCostPerImageInt = "output_cost_per_image_int"
)
```

在 `ModelTableCheck` 中增加图像单价的读取与定点转换：

```go
outputCostPerImage := price.Prices[PriceOutputCostPerImage]
if outputCostPerImage < 0 {
    return fmt.Errorf("negative price for model %s", price.Model)
}
price.Prices[PriceOutputCostPerImageInt] = float64(quota.RmbToFixedPoint(outputCostPerImage))
```

说明：
- `output_cost_per_image` 为可选配置。
- 未配置时按 fail-open 处理，`cost = 0`。

### 6.4 非流式 Usage 解析

**文件：** `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

函数 `UpdateCtxByUsage` 增加 `image_count` 解析与兜底：

```go
imageCount := gjson.GetBytes(data, "usage.image_count").Int()
if imageCount == 0 {
    imageCount = gjson.GetBytes(data, "data.#").Int()
}

tokenUsage.ImageCount = imageCount

if used > 0 {
    tokenUsage.UsedQuota = used
} else if prompt > 0 || completion > 0 || imageCount > 0 {
    if imageCount > 0 {
        tokenUsage.UsedQuota = imageCount
    } else {
        tokenUsage.UsedQuota = prompt + completion
    }
}
```

说明：
- 优先读取 `usage.image_count`。
- 未返回时统计响应 `data` 数组长度（OpenAI 风格图像生成响应）。
- 若仍未获得，可在请求阶段预读请求体 `n` 字段写入 `ImageCount`。

### 6.5 请求阶段预读 `n` 字段

`n` 是 OpenAI 风格图像生成接口请求体中的参数，表示**请求生成多少张图片**。示例：

```json
{
  "model": "dall-e-3",
  "prompt": "a white cat",
  "n": 2,
  "size": "1024x1024"
}
```

- `n = 2` 表示希望生成 2 张图。
- 若未传 `n`，OpenAI 默认值为 `1`。

部分后端（包括 OpenAI 原生图像生成接口）的响应不返回 `usage.image_count`，只返回 `data` 数组。BFE 计费需要知道实际生成了多少张图，因此按以下优先级获取 `image_count`：

1. 响应 `usage.image_count`；
2. 响应 `data` 数组长度；
3. 请求体 `n` 字段；
4. 仍未获得则默认 `1`。

预读 `n` 是为了在响应缺少用量信息时，仍能用请求参数兜底计费，避免漏计。

**文件：** `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

新增辅助函数：

```go
func GetImageCountFromReq(req *bfe_basic.Request) int64 {
    bodyAccessor, _ := req.HttpRequest.GetBodyAccessor()
    if bodyAccessor == nil {
        return 1
    }
    body, _ := bodyAccessor.GetBytes()
    n := gjson.GetBytes(body, "n").Int()
    if n <= 0 {
        return 1
    }
    return n
}
```

在 `SetTokenAuthContext` 中：

```go
if aiBasicInfo != nil && aiBasicInfo.Mode == bfe_basic.ModeImageGeneration {
    tusage.ImageCount = GetImageCountFromReq(req)
}
```

说明：
- 请求阶段按 `n` 预读，响应返回后若 `usage.image_count` 或 `data.#` 更大则覆盖。
- OpenAI 默认 `n=1`。

### 6.6 流式 Usage 解析

**文件：** `bfe/bfe_modules/mod_body_process/llm_util.go`

函数 `SSEEvent.GetQuotaUsage` 返回的 `QuotaUsage` 增加 `ImageCount`：

```go
type QuotaUsage struct {
    PromptTokens     int64
    CompletionTokens int64
    CacheReadTokens  int64
    CacheWriteTokens int64
    ImageCount       int64 // 新增
    UsedQuota        int64
    CurrentTokens    int64
    IsGuess          bool
}
```

函数内解析 `usage.image_count`，未返回时解析 `data.#`。

**文件：** `bfe/bfe_modules/mod_body_process/body_process.go`

非流式响应使用的 `RawEvent.GetQuotaUsage` 同样需要解析 `usage.image_count` / `data.#`。

**文件：** `bfe/bfe_modules/mod_body_process/content_quota_usage.go`

`QuotaUsageProcessor.Process` 在非 guess 事件覆盖 `tctx` 时同步写入 `ImageCount`，并优先用 `ImageCount` 作为 `UsedQuota`。

### 6.7 RMB 费用计算改造

**文件：** `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

函数 `calcCostUnits` 按 mode 分支：

```go
func (m *ModuleAITokenAuth) calcCostUnits(
    req *bfe_basic.Request,
    serverConf bfe_basic.ServerDataConfInterface,
    usage *bfe_basic.TokenUsage,
) int64 {
    aiMeta := req.GetAiBasicInfo()
    if aiMeta == nil || usage == nil {
        return 0
    }

    clusterName := req.Route.ClusterName
    targetModel := aiMeta.TargetModel
    mode := aiMeta.Mode
    if mode == "" {
        mode = bfe_basic.ModeChat
    }
    if clusterName == "" || targetModel == "" {
        return 0
    }

    cluster, err := serverConf.ClusterTableLookup(clusterName)
    if err != nil || cluster == nil || cluster.AIConf == nil || cluster.AIConf.ModelTable == nil {
        log.Logger.Warn("model table not found for cluster %s", clusterName)
        return 0
    }

    entry := cluster_conf.LookupModelPrice(cluster.AIConf.ModelTable, targetModel, mode)
    if entry == nil {
        log.Logger.Warn("model price not found for cluster %s model %s mode %s", clusterName, targetModel, mode)
        return 0
    }

    switch mode {
    case bfe_basic.ModeImageGeneration:
        return calcImageGenerationCost(entry, usage)
    default:
        return calcChatCost(entry, usage)
    }
}

func calcImageGenerationCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage) int64 {
    imageCount := usage.ImageCount
    if imageCount < 0 {
        imageCount = 0
    }

    costPerImage := int64(entry.Prices[cluster_conf.PriceOutputCostPerImageInt])
    if costPerImage < 0 {
        log.Logger.Warn("invalid model price for image generation model %s", entry.Model)
        return 0
    }

    return imageCount * costPerImage
}
```

调用点无需改动：

```go
if tokenUsage.UsedCost <= 0 && hasRMBPlan(ctx.Token.QuotaPlans) {
    tokenUsage.UsedCost = m.calcCostUnits(req, ctx.serverConf, tokenUsage)
}
```

### 6.8 访问日志扩展（建议）

**文件：** `bfe-access-pb/bfe_access_pb/bfe_access.proto`

在 AI Observability 区域新增字段：

```protobuf
// 716 - 720: AI request mode
optional string ai_mode = 716;

// 785 - 790: image metering
optional int64 ai_image_count = 785;
```

**文件：** `bfe/bfe_modules/mod_access_pb3/request_log.go`

在 `reqAiInfoGen` 中补充：

```go
if aiInfo.Mode != "" {
    reqLog.AiMode = proto.String(aiInfo.Mode)
}
if usage.ImageCount > 0 {
    reqLog.AiImageCount = proto.Int64(usage.ImageCount)
}
```

修改 proto 后需执行 `bfe-access-pb/build.sh` 重新生成 Go 代码，并同步更新 `bfe-access-pb/docs/protobuf.md`。

---

## 7. 计费公式速查

### 7.1 图像生成模型

前提：`mode = "image_generation"` 且 `output_cost_per_image` 已配置。

```
image_count = usage.image_count
            ?? len(response.data)
            ?? request.n
            ?? 1

cost = image_count × output_cost_per_image
```

### 7.2 与示例 3 的对齐验证

```
image_count = 2
cost = 2 × 0.03 = 0.06 USD
```

---

## 8. 边界情况与兼容性

| 场景 | 处理建议 |
|---|---|
| 后端未返回 `usage.image_count` | fallback 到响应 `data` 数组长度 |
| 响应也没有 `data` 数组 | fallback 到请求体 `n` 字段；未传时默认 `1` |
| `image_count` 为负 | 按 `0` 处理 |
| 价格表未配置 `output_cost_per_image` | fail-open：记录 Warn 日志，`cost = 0`，不扣减 RMB 配额 |
| 请求路径无法识别 mode | 默认 `chat`，保持现有文本模型计费行为 |
| `total_token` 配额计划 | `image_generation` 模式下 `UsedQuota = image_count` |
| RMB 配额计划 | `image_generation` 模式下按 `image_count × output_cost_per_image` 扣减 |
| 模型映射后 target_model 变化 | 按映射后的 `target_model` + `mode` 匹配定价条目 |
| fallback 到另一个 cluster | 按最终命中的 `cluster` + `target_model` + `mode` 计费 |
| 图像生成请求转发 | BFE 已可正常透传 `/v1/images/generations`，无需改造转发链路 |

---

## 9. 测试计划

### 9.1 单元测试

在 `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` 中新增：

1. `TestDetectModeFromPath`：验证各路径对应的 mode 推断正确。
2. `TestCalcCostUnits_ImageGeneration`：验证 `image_count × output_cost_per_image` 公式正确。
3. `TestUpdateCtxByUsage_ImageGeneration`：验证 `usage.image_count` 与 `data.#` 解析正确。
4. `TestTokenRequestFinishHandler_RMB_ImageGeneration`：非流式图像生成请求端到端扣费。
5. `TestCalcCostUnits_ImageGenerationMissingPrice`：未配置 `output_cost_per_image` 时按 0 处理。
6. `TestCalcCostUnits_ImageGenerationFallbackN`：响应无 usage 时按请求 `n` 字段计费。

### 9.2 集成测试

在 `bfe/tests/integration/implementation/scenario-SC03-rmb-quota/sc03_rmb_quota_test.go` 中新增：

1. `TestTC12_RMBQuotaDeduction_ImageGeneration`：
   - 后端返回含 `usage.image_count = 2` 的图像生成响应；
   - `ModelTable` 配置 `output_cost_per_image = 0.03`；
   - 验证 Redis 扣减金额与示例 3 期望一致（0.06）。

在 `bfe/tests/integration/implementation/scenario-SC05-access-log-ai-fields/sc05_access_log_ai_fields_test.go` 中新增：

2. `TestTC10_ImageGenerationFields`：
   - 后端返回图像生成响应；
   - `ModelTable` 配置 `output_cost_per_image`；
   - 验证 `mod_access_pb3` 输出的 b2log 中 `ai_mode`、`ai_image_count`、`ai_cost_value` 正确。

### 9.3 配置加载测试

在 `bfe/bfe_config/bfe_cluster_conf/cluster_conf/` 中：

1. 验证 `ModelTableCheck` 正确转换 `output_cost_per_image` 为定点整数；
2. 验证未配置 `output_cost_per_image` 时不报错；
3. 验证 `output_cost_per_image` 为负数时报错；
4. 验证 `LookupModelPrice` 按 `(model, mode)` 精确命中 `image_generation` 条目。

---

## 10. 实施步骤建议

1. **mode 识别与数据结构**
   - `bfe_basic.AiBasicInfo` 增加 `Mode`。
   - 新增 `DetectModeFromPath` 并在 `bfe_server/http_conn.go` 中调用。
   - `bfe_basic.TokenUsage` 增加 `ImageCount`。

2. **配置层**
   - `cluster_conf_load.go` 增加 `output_cost_per_image` 常量及转换逻辑。

3. **Usage 解析**
   - `UpdateCtxByUsage` 解析非流式 `image_count` / `data.#`。
   - `SSEEvent.GetQuotaUsage` / `RawEvent.GetQuotaUsage` / `QuotaUsageProcessor.Process` 解析流式 `image_count`。
   - `SetTokenAuthContext` 中预读请求 `n` 字段作为兜底。

4. **计费逻辑**
   - 改造 `calcCostUnits`，按 `mode` 分支计费。
   - 同步修改调用点与单元测试。

5. **日志与可观测性（可选但建议）**
   - 扩展 `bfe_access.proto`（新增 `ai_mode`、`ai_image_count`）。
   - 修改 `reqAiInfoGen` 记录 `ai_mode`、`ai_image_count`。
   - 更新 `bfe-access-pb/docs/protobuf.md`。

6. **测试与回归**
   - 补充单元测试、集成测试。
   - 回归验证现有 RMB / total_token / cache / audio 计费场景不受影响。

---

## 11. 影响范围

| 模块/文件 | 影响 |
|---|---|
| `bfe/bfe_server/http_conn.go` | 设置 `AiBasicInfo.Mode` |
| `bfe/bfe_basic/request_ai_basic.go` | `AiBasicInfo` 新增 `Mode`；`TokenUsage` 新增 `ImageCount`；新增 `DetectModeFromPath` |
| `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | 新增 `output_cost_per_image` 价格常量与转换逻辑 |
| `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | Usage 解析与费用计算按 mode 分支改造 |
| `bfe/bfe_modules/mod_body_process/llm_util.go` | `QuotaUsage` 与 `SSEEvent.GetQuotaUsage` 新增 `ImageCount` |
| `bfe/bfe_modules/mod_body_process/body_process.go` | `RawEvent.GetQuotaUsage` 新增 `ImageCount` |
| `bfe/bfe_modules/mod_body_process/content_quota_usage.go` | 流式/非流式 `image_count` 累积 |
| `bfe-access-pb/bfe_access_pb/bfe_access.proto` | 访问日志新增 `ai_mode`、`ai_image_count` |
| `bfe/bfe_modules/mod_access_pb3/request_log.go` | 序列化 `ai_mode`、`ai_image_count` |
| `bfe/tests/integration/implementation/scenario-SC03-rmb-quota/sc03_rmb_quota_test.go` | 新增图像生成计费集成测试 |
| `bfe/tests/integration/implementation/scenario-SC05-access-log-ai-fields/sc05_access_log_ai_fields_test.go` | 新增图像生成访问日志字段集成测试 |
| `bfe/docs/zh_cn/modifications/2026-08-22-image-generation-billing-support/design-changes.md` | 本设计变更文档 |

---

## 12. 兼容性与风险

### 12.1 兼容性

- 未配置 `output_cost_per_image` 的模型行为完全不变。
- 无法识别 mode 的请求默认按 `chat` 处理，现有计费逻辑不变。
- Token 配额（`total_token`）在 chat 模式下扣减逻辑不变。
- Cache / audio 计费逻辑不变。
- Redis key 结构、配置格式均不发生改变。

### 12.2 风险与缓解

| 风险 | 缓解措施 |
|---|---|
| 后端返回异常 `image_count` 导致费用为负 | `calcImageGenerationCost` 中校验并截断为 `0` |
| 响应无 usage 且请求未传 `n` | 默认按 `1` 张计费，避免漏计 |
| 路径推断 mode 不准确 | 默认 fallback 为 `chat`；后续可扩展 cluster/provider 显式配置 |
| 图像生成耗时超过默认超时 | 为对应 cluster 配置更大的超时参数 |
| proto 字段变更需重新生成 | 修改后执行 `bfe-access-pb/build.sh` |

---

## 13. 参考资料

- `document-ai-gateway/迭代系统设计/v0.5/计费能力扩展/bfe-image-generation-billing-support-analysis.md`
- `document-ai-gateway/迭代系统设计/v0.4/quota-rmb-support/bfe-changes-for-rmb-quota.md`
- `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`
- `bfe/bfe_modules/mod_body_process/content_quota_usage.go`
- `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`
- `bfe/bfe_basic/request_ai_basic.go`
- `bfe/bfe_server/http_conn.go`
