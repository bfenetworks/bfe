# BFE 支持 responses mode 及新价格键计费扩展

## 1. 背景

AI 网关的 model-list 已逐步引入以下三类模型/能力，但 BFE 当前计费逻辑尚未完全识别：

- `responses` mode（如 `o3-deep-research`）：OpenAI Responses API 入口 `/v1/responses`，usage 结构与 chat 类似，但 BFE 没有对应 mode 常量，路径识别默认回退为 `chat`，导致 `model-list` 中 `mode: responses` 的价格条目无法命中。
- `input_cost_per_image_token`（如 `gpt-image-*`、`qwen3-vl-embedding`）：图片输入 token 已包含在普通 prompt token 中，但当前没有独立价格键，导致图片输入被按普通 input token 计费。
- `output_cost_per_video`（如 `kling-*` 视频生成模型）：视频生成接口 `/v1/video/generations` 需要按生成视频数量计费，但 BFE 没有专用 mode 与价格键。

本变更在不破坏现有 `chat` / `image_generation` / `audio_*` 等计费逻辑的前提下，扩展 BFE 对以上三种场景的端到端支持，并同步扩展访问日志字段。

---

## 2. 需求示例

### 2.1 responses mode

| 价格字段 | USD / token |
|---|---|
| `input_cost_per_token` | `1.0e-05` |
| `output_cost_per_token` | `2.0e-05` |

| Usage 字段 | 值 |
|---|---|
| `prompt_tokens` | 100 |
| `completion_tokens` | 50 |

计费公式：

```
cost = 100 × 1.0e-05 + 50 × 2.0e-05 = 0.002 USD
```

### 2.2 input_cost_per_image_token

`gpt-image-*` 图像生成模型同时存在图像张数计费和图片输入 token 计费：

| 价格字段 | USD |
|---|---|
| `output_cost_per_image` | `0.04` / 张 |
| `input_cost_per_image_token` | `5.0e-07` / token |

| Usage 字段 | 值 |
|---|---|
| `image_count` | 2 |
| `input_token_details.image_tokens` | 200 |

计费公式：

```
cost = 2 × 0.04 + 200 × 5.0e-07 = 0.0801 USD
```

### 2.3 output_cost_per_video

`kling-*` 视频生成模型按生成视频数量计费：

| 价格字段 | USD |
|---|---|
| `output_cost_per_video` | `0.5` / 个 |

| Usage 字段 | 值 |
|---|---|
| `video_count` | 3 |

计费公式：

```
cost = 3 × 0.5 = 1.5 USD
```

---

## 3. 当前现状

| 层级 | 当前能力 | 不足 |
|---|---|---|
| mode 常量 | 已定义 `chat`/`image_generation`/`audio_*` 等 | 缺少 `responses`、`video_generation` 路径识别 |
| 价格配置 | 已支持 input/output/cache/audio/image 单价 | 缺少 `input_cost_per_image_token`、`output_cost_per_video` |
| Usage 解析 | 已解析 prompt/completion/cache/audio/image_count | 不识别 `image_input_tokens`、`video_count`、Responses API 的 `input_token_details.cached_tokens` |
| 费用计算 | `calcCostUnits` 按 mode 分支 | 缺少 `responses`、`video_generation` 分支；`image_generation` 未计入图片输入 token；`calcChatCost` 未拆分图片输入 token |
| 访问日志 | 已有 `ai_image_count` | 缺少 `ai_image_input_tokens`、`ai_video_count` |

---

## 4. 变更目标

1. 新增 `responses` mode 常量，并识别 `/v1/responses` 路径。
2. 新增 `video_generation` mode 路径识别（`/v1/video/generations`）。
3. 在 `TokenUsage` / `QuotaUsage` 中增加 `ImageInputTokens`、`VideoCount`。
4. 在模型价格表中支持 `input_cost_per_image_token`、`output_cost_per_video`。
5. 改造 usage 解析、费用计算、访问日志三个环节，支持新 mode 与新价格键。
6. 保持向后兼容：未配置新价格键或无法识别 mode 时，行为与现有逻辑一致。
7. 补充单元测试与集成测试。

---

## 5. 变更总览

| 模块 | 主要改动 |
|---|---|
| `bfe_basic` | 新增 `ModeResponses`；`DetectModeFromPath` 增加 `/v1/responses`、`/v1/video/generations`；`TokenUsage` 增加 `ImageInputTokens`、`VideoCount` |
| `bfe_config/bfe_cluster_conf/cluster_conf` | 增加 `input_cost_per_image_token`、`output_cost_per_video` 常量与定点转换；校验非负 |
| `mod_ai_token_auth` | 请求阶段预读 `n` 作为 `VideoCount`；响应阶段解析 `image_input_tokens`、`video_count`；`calcCostUnits` 增加 `responses`/`video_generation` 分支；`calcImageGenerationCost` 支持图片输入 token；`calcChatCost` 拆分图片输入 token |
| `mod_body_process` | `QuotaUsage` / `SSEEvent.GetQuotaUsage` / `RawEvent.GetQuotaUsage` / `QuotaUsageProcessor.Process` 解析并累积 `ImageInputTokens`、`VideoCount` |
| `bfe-access-pb` / `mod_access_pb3` | 访问日志新增 `ai_image_input_tokens`、`ai_video_count` |
| 文档 | 更新 `ai_access_log_fields.md`；新增本变更文档 |
| 测试 | 补充单元测试与集成测试 |

---

## 6. 详细设计

### 6.1 mode 常量与路径识别

**文件：** `bfe/bfe_basic/request_ai_basic.go`

新增 `ModeResponses`，并保留 `ModeVideoGeneration`（已存在）：

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
    ModeResponses          = "responses"   // 新增
)
```

`DetectModeFromPath` 增加两条路径匹配：

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
    case strings.HasPrefix(path, "/v1/video/generations"):
        return ModeVideoGeneration
    case strings.HasPrefix(path, "/v1/responses"):
        return ModeResponses
    default:
        return ModeChat
    }
}
```

说明：
- `/v1/responses` 是 OpenAI Responses API 入口，支持流式与非流式，路径不变。
- 默认 fallback 为 `chat`，保持向后兼容。

### 6.2 新增价格键常量

**文件：** `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`

追加常量与映射：

```go
const (
    PriceInputCostPerToken           = "input_cost_per_token"
    PriceOutputCostPerToken          = "output_cost_per_token"
    PriceCacheReadInputTokenCost     = "cache_read_input_token_cost"
    PriceCacheCreationInputTokenCost = "cache_creation_input_token_cost"
    PriceInputCostPerAudioToken      = "input_cost_per_audio_token"
    PriceOutputCostPerAudioToken     = "output_cost_per_audio_token"
    PriceOutputCostPerImage          = "output_cost_per_image"

    PriceInputCostPerImageToken      = "input_cost_per_image_token"  // 新增
    PriceOutputCostPerVideo          = "output_cost_per_video"       // 新增

    PriceInputCostPerTokenInt           = "input_cost_per_token_int"
    PriceOutputCostPerTokenInt          = "output_cost_per_token_int"
    PriceCacheReadInputTokenCostInt     = "cache_read_input_token_cost_int"
    PriceCacheCreationInputTokenCostInt = "cache_creation_input_token_cost_int"
    PriceInputCostPerAudioTokenInt      = "input_cost_per_audio_token_int"
    PriceOutputCostPerAudioTokenInt     = "output_cost_per_audio_token_int"
    PriceOutputCostPerImageInt          = "output_cost_per_image_int"

    PriceInputCostPerImageTokenInt      = "input_cost_per_image_token_int"  // 新增
    PriceOutputCostPerVideoInt          = "output_cost_per_video_int"       // 新增
)

var priceKeyToIntKey = map[string]string{
    PriceInputCostPerToken:           PriceInputCostPerTokenInt,
    PriceOutputCostPerToken:          PriceOutputCostPerTokenInt,
    PriceCacheReadInputTokenCost:     PriceCacheReadInputTokenCostInt,
    PriceCacheCreationInputTokenCost: PriceCacheCreationInputTokenCostInt,
    PriceInputCostPerAudioToken:      PriceInputCostPerAudioTokenInt,
    PriceOutputCostPerAudioToken:     PriceOutputCostPerAudioTokenInt,
    PriceOutputCostPerImage:          PriceOutputCostPerImageInt,
    PriceInputCostPerImageToken:      PriceInputCostPerImageTokenInt,
    PriceOutputCostPerVideo:          PriceOutputCostPerVideoInt,
}
```

在 `ModelTableCheck` 中增加读取、校验与定点转换：

```go
inputImageToken := price.Prices[PriceInputCostPerImageToken]
outputCostPerVideo := price.Prices[PriceOutputCostPerVideo]
if input < 0 || output < 0 || cacheRead < 0 || cacheWrite < 0 ||
    audioInput < 0 || audioOutput < 0 || outputCostPerImage < 0 ||
    inputImageToken < 0 || outputCostPerVideo < 0 {
    return fmt.Errorf("negative price for model %s", price.Model)
}

price.pricesInt[PriceInputCostPerImageTokenInt] = quota.RmbToFixedPoint(inputImageToken)
price.pricesInt[PriceOutputCostPerVideoInt]     = quota.RmbToFixedPoint(outputCostPerVideo)
```

说明：
- 新价格键为可选配置；未配置时按原有逻辑计费。
- 负价校验保持与现有价格键一致。

### 6.3 TokenUsage 与 usage 解析扩展

#### 6.3.1 `bfe_basic.TokenUsage`

**文件：** `bfe/bfe_basic/request_ai_basic.go`

```go
type TokenUsage struct {
    PromptTokens      int64
    CompletionTokens  int64
    CacheReadTokens   int64
    CacheWriteTokens  int64
    AudioInputTokens  int64
    AudioOutputTokens int64
    ImageInputTokens  int64  // 新增：图片输入 token（已包含在 PromptTokens 中）
    VideoCount        int64  // 新增：生成视频数量
    ImageCount        int64
    UsedQuota         int64
    UsedCost          int64
}
```

#### 6.3.2 `mod_body_process.QuotaUsage`

**文件：** `bfe/bfe_modules/mod_body_process/llm_util.go`

```go
type QuotaUsage struct {
    PromptTokens      int64
    CompletionTokens  int64
    CacheReadTokens   int64
    CacheWriteTokens  int64
    AudioInputTokens  int64
    AudioOutputTokens int64
    ImageInputTokens  int64  // 新增
    VideoCount        int64  // 新增
    ImageCount        int64
    UsedQuota         int64
    CurrentTokens     int64
    IsGuess           bool
}
```

#### 6.3.3 `llm_util.go` 解析逻辑

在 `GetQuotaUsage()` 中新增字段读取：

```go
imageInput := gjson.GetBytes(data, "usage.input_token_details.image_tokens").Int()
if imageInput == 0 {
    imageInput = gjson.GetBytes(data, "usage.image_input_tokens").Int()
}
videoCount := gjson.GetBytes(data, "usage.video_count").Int()
if videoCount == 0 {
    videoCount = gjson.GetBytes(data, "data.#").Int()
}

// Responses API 的 cache read 可能在 input_token_details.cached_tokens
if cacheRead == 0 {
    cacheRead = gjson.GetBytes(data, "usage.input_token_details.cached_tokens").Int()
}
```

返回时带上新增字段：

```go
return QuotaUsage{
    PromptTokens:      prompt,
    CompletionTokens:  completion,
    CacheReadTokens:   cacheRead,
    CacheWriteTokens:  cacheWrite,
    AudioInputTokens:  audioInput,
    AudioOutputTokens: audioOutput,
    ImageInputTokens:  imageInput,
    VideoCount:        videoCount,
    ImageCount:        imageCount,
    UsedQuota:         used,
    CurrentTokens:     curtoken,
    IsGuess:           isguess,
}
```

#### 6.3.4 `content_quota_usage.go` 透传

**文件：** `bfe/bfe_modules/mod_body_process/content_quota_usage.go`

在非 guess 事件覆盖 `tctx` 时同步写入新增字段；估算分支同样透传：

```go
} else if rquota.UsedQuota > 0 {
    tctx.CompletionTokens = rquota.CompletionTokens
    tctx.PromptTokens = rquota.PromptTokens
    tctx.CacheReadTokens = rquota.CacheReadTokens
    tctx.CacheWriteTokens = rquota.CacheWriteTokens
    tctx.AudioInputTokens = rquota.AudioInputTokens
    tctx.AudioOutputTokens = rquota.AudioOutputTokens
    tctx.ImageInputTokens = rquota.ImageInputTokens
    tctx.VideoCount = rquota.VideoCount
    tctx.UsedQuota = rquota.UsedQuota
} else if rquota.PromptTokens > 0 || rquota.CompletionTokens > 0 {
    ...
    tctx.ImageInputTokens = rquota.ImageInputTokens
    tctx.VideoCount = rquota.VideoCount
}
```

### 6.4 请求体预读（image / video count 兜底）

**文件：** `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

新增辅助函数读取请求体 `n` 字段：

```go
func GetVideoCountFromReq(req *bfe_basic.Request) int64 {
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
if aiBasicInfo.Mode == bfe_basic.ModeImageGeneration {
    tusage.ImageCount = GetImageCountFromReq(req)
}
if aiBasicInfo.Mode == bfe_basic.ModeVideoGeneration {
    tusage.VideoCount = GetVideoCountFromReq(req)
}
```

说明：
- `n` 是 OpenAI 风格图像/视频生成接口请求体中的生成数量参数。
- 响应返回后若 `usage.video_count` 或 `data.#` 更大则覆盖。
- 未传 `n` 时默认按 `1` 兜底。

### 6.5 计费逻辑扩展

**文件：** `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

在 `calcCostUnits` 的 `switch mode` 中新增分支：

```go
switch mode {
case bfe_basic.ModeImageGeneration:
    return calcImageGenerationCost(entry, usage, tierName)
case bfe_basic.ModeVideoGeneration:
    return calcVideoGenerationCost(entry, usage, tierName)
case bfe_basic.ModeResponses:
    return calcResponsesCost(entry, usage, tierName)
default:
    return calcChatCost(entry, usage, tierName)
}
```

#### 6.5.1 `calcResponsesCost`

Responses API 本质上属于按 token 计费，当前直接复用 `calcChatCost`：

```go
func calcResponsesCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
    return calcChatCost(entry, usage, tierName)
}
```

说明：
- 若后续 Responses API 需要单独处理 `input_token_details` 中的 `text/image/audio/reasoning` 拆分，可再独立实现。
- 流式 Responses API 的 usage 只在最后一个 `response.completed` 事件的 `data.response.usage` 里提供；BFE 在请求结束阶段统一结算，因此最终事件带有 usage 即可正常计费。

#### 6.5.2 `calcVideoGenerationCost`

```go
func calcVideoGenerationCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
    videoCount := usage.VideoCount
    if videoCount < 0 {
        videoCount = 0
    }

    costPerVideo := entry.GetPriceInt(tierName, cluster_conf.PriceOutputCostPerVideoInt)
    if costPerVideo < 0 {
        log.Logger.Warn("invalid model price for video generation model %s", entry.Model)
        return 0
    }

    return videoCount * costPerVideo
}
```

#### 6.5.3 `calcImageGenerationCost` 支持输入图片 token

扩展现有按张数计费逻辑：

```go
func calcImageGenerationCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
    imageCount := usage.ImageCount
    if imageCount < 0 {
        imageCount = 0
    }

    costPerImage := entry.GetPriceInt(tierName, cluster_conf.PriceOutputCostPerImageInt)
    inputImageTokenCost := entry.GetPriceInt(tierName, cluster_conf.PriceInputCostPerImageTokenInt)

    if costPerImage < 0 || inputImageTokenCost < 0 {
        log.Logger.Warn("invalid model price for image generation model %s", entry.Model)
        return 0
    }

    return imageCount*costPerImage + usage.ImageInputTokens*inputImageTokenCost
}
```

#### 6.5.4 `calcChatCost` 支持图片输入 token 拆分

参考 audio 拆分写法，在 `calcChatCost` 中加入：

```go
imageInputCost := entry.GetPriceInt(tierName, cluster_conf.PriceInputCostPerImageTokenInt)
imageInputTokens := usage.ImageInputTokens

// image-aware billing: split image input from normal input
if imageInputCost > 0 {
    if imageInputTokens > normalInput {
        imageInputTokens = normalInput
    }
    normalInput = normalInput - imageInputTokens
    if normalInput < 0 {
        normalInput = 0
    }
} else {
    imageInputTokens = 0
}
```

总成本公式更新为：

```go
if cacheReadCost > 0 || cacheWriteCost > 0 || audioInputCost > 0 || audioOutputCost > 0 || imageInputCost > 0 {
    cost = normalInput*inputCost +
        cacheReadTokens*cacheReadCost +
        cacheWriteTokens*cacheWriteCost +
        audioInputTokens*audioInputCost +
        imageInputTokens*imageInputCost +
        normalOutput*outputCost +
        audioOutputTokens*audioOutputCost
} else {
    cost = promptTokens*inputCost + completionTokens*outputCost
}
```

说明：
- 拆分顺序建议为 cache read → image input → audio input，与 OpenAI 等上游的账单明细顺序保持一致。
- 未配置 `input_cost_per_image_token` 时，`imageInputCost == 0`，走原有 fallback 公式，不影响现有模型。

### 6.6 访问日志字段扩展

**文件：** `bfe-access-pb/bfe_access_pb/bfe_access.proto`

在 781-790 子项区间新增：

```protobuf
// 781 - 790: cache, audio, image and video metering
...
optional int64 ai_image_count = 785;

// Number of image input tokens (included in ai_input_tokens)
optional int64 ai_image_input_tokens = 786;

// Number of generated videos (video_generation mode)
optional int64 ai_video_count = 787;
```

修改后执行 `bfe-access-pb/build.sh` 重新生成 `bfe_access.pb.go`。

**文件：** `bfe/bfe_modules/mod_access_pb3/request_log.go`

```go
if usage.ImageInputTokens > 0 {
    reqLog.AiImageInputTokens = proto.Int64(usage.ImageInputTokens)
}
if usage.VideoCount > 0 {
    reqLog.AiVideoCount = proto.Int64(usage.VideoCount)
}
```

**文件：** `bfe/docs/zh_cn/sys_design/ai_access_log_fields.md`

更新字段表，新增：

| 编号 | 字段名 | 类型 | 说明 |
|---|---|---|---|
| 786 | `ai_image_input_tokens` | `int64` | 图片输入 Token 数（已包含在 `ai_input_tokens` 中） |
| 787 | `ai_video_count` | `int64` | 生成视频数量（video_generation 模式） |

---

## 7. 计费公式速查

### 7.1 responses mode

前提：`mode = "responses"`，已配置 `input_cost_per_token` / `output_cost_per_token`。

```
cost = prompt_tokens × input_cost_per_token
     + completion_tokens × output_cost_per_token
```

### 7.2 含图片输入 token 的 chat/embedding 模型

前提：已配置 `input_cost_per_image_token`。

```
normal_input = max(prompt_tokens - cache_read_tokens - image_input_tokens, 0)

cost = normal_input × input_cost_per_token
     + cache_read_tokens × cache_read_input_token_cost
     + cache_write_tokens × cache_creation_input_token_cost
     + image_input_tokens × input_cost_per_image_token
     + completion_tokens × output_cost_per_token
```

### 7.3 图像生成模型（含图片输入 token）

前提：`mode = "image_generation"`，已配置 `output_cost_per_image` 与 `input_cost_per_image_token`。

```
image_count = usage.image_count ?? len(response.data) ?? request.n ?? 1
cost = image_count × output_cost_per_image
     + image_input_tokens × input_cost_per_image_token
```

### 7.4 视频生成模型

前提：`mode = "video_generation"`，已配置 `output_cost_per_video`。

```
video_count = usage.video_count ?? request.n ?? 1
cost = video_count × output_cost_per_video
```

---

## 8. 边界情况与兼容性

| 场景 | 处理建议 |
|---|---|
| 后端未返回 `image_input_tokens` | 字段值为 0，按不含图片输入 token 计费 |
| 后端未返回 `video_count` | fallback 到请求体 `n` 字段；未传时默认 `1` |
| `image_input_tokens > prompt_tokens` | 截断为 `prompt_tokens - cache_read_tokens`，避免普通 input 为负 |
| `video_count` 为负 | 按 `0` 处理 |
| 价格表未配置新价格键 | 走原有逻辑，`cost` 中对应子项为 0 |
| `/v1/responses` 被其它业务使用 | 仅影响 AI 网关 mode 识别与计费匹配，不影响转发 |
| 流式 Responses API 未在最终事件返回 usage | 回退到 `EstimateContentToken` 估算 |
| 与 cache/audio 同时存在 | 按 cache read → image input → audio input 顺序剥离 |

---

## 9. 测试计划

### 9.1 单元测试

在 `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` 中新增：

1. `TestDetectModeFromPath_Responses`：验证 `/v1/responses` 识别为 `responses`。
2. `TestDetectModeFromPath_VideoGeneration`：验证 `/v1/video/generations` 识别为 `video_generation`。
3. `TestCalcCostUnits_Responses`：验证 responses 模式按 chat 公式计费。
4. `TestCalcCostUnits_VideoGeneration`：验证 `video_count × output_cost_per_video` 公式。
5. `TestCalcCostUnits_ImageGenerationWithImageInputTokens`：验证图像生成同时按张数与图片输入 token 计费。
6. `TestCalcCostUnits_ChatWithImageInputTokens`：验证 chat 模式下图片输入 token 拆分。
7. `TestUpdateCtxByUsage_ImageInputTokens`：验证 `usage.input_token_details.image_tokens` 与 `usage.image_input_tokens` 解析。
8. `TestUpdateCtxByUsage_VideoCount`：验证 `usage.video_count` 与 `data.#` 兜底解析。

在 `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load_test.go` 中新增：

9. 验证 `input_cost_per_image_token`、`output_cost_per_video` 配置加载与定点转换。
10. 验证新价格键为负数时报错。

### 9.2 集成测试

扩展 `scenario-SC05-access-log-ai-fields`：

1. `TestTC11_ResponsesFields`：
   - `/v1/responses` 请求，返回 `{"usage":{"input_tokens":100,"output_tokens":50}}`；
   - 验证 `ai_mode=responses`、`ai_cost_value>0`。

2. `TestTC12_VideoGenerationFields`：
   - `/v1/video/generations` 请求，返回 `{"usage":{"video_count":2}}`；
   - 验证 `ai_mode=video_generation`、`ai_video_count=2`、`ai_cost_value=2×price`。

3. `TestTC13_ImageInputTokenFields`：
   - `/v1/images/generations` 请求，返回 `{"usage":{"image_count":1,"input_token_details":{"image_tokens":200}}}`；
   - 验证 `ai_image_input_tokens=200` 且成本包含两部分。

---

## 10. 实施步骤建议

1. **bfe-access-pb**：新增 proto 字段并重新生成 `bfe_access.pb.go`。
2. **bfe_basic**：新增 mode 常量、路径识别、`TokenUsage` 字段。
3. **bfe_config**：新增价格键常量、校验、索引。
4. **mod_body_process**：新增 usage 解析与透传。
5. **mod_ai_token_auth**：新增计费函数与请求体 count 读取。
6. **mod_access_pb3**：新增访问日志字段输出。
7. **文档**：更新 `ai_access_log_fields.md`，新增本变更文档。
8. **测试**：补充单元测试、集成测试，回归现有计费场景。

---

## 11. 影响范围

| 模块/文件 | 影响 |
|---|---|
| `bfe/bfe_basic/request_ai_basic.go` | 新增 `ModeResponses`；`DetectModeFromPath` 扩展；`TokenUsage` 新增字段 |
| `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | 新增价格键常量、校验、定点转换、映射 |
| `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | Usage 解析、计费函数、请求体预读扩展 |
| `bfe/bfe_modules/mod_body_process/llm_util.go` | `QuotaUsage` 与 usage 解析新增字段 |
| `bfe/bfe_modules/mod_body_process/content_quota_usage.go` | 流式/非流式新增字段透传 |
| `bfe-access-pb/bfe_access_pb/bfe_access.proto` | 访问日志新增字段 |
| `bfe-access-pb/bfe_access_pb/bfe_access.pb.go` | 重新生成 |
| `bfe/bfe_modules/mod_access_pb3/request_log.go` | 序列化新增字段 |
| `bfe/docs/zh_cn/sys_design/ai_access_log_fields.md` | 更新字段说明 |
| `bfe/docs/zh_cn/modifications/2026-08-31-bfe-mode-and-price-extension/design-changes.md` | 本设计变更文档 |

---

## 12. 兼容性与风险

### 12.1 兼容性

- 未配置 `input_cost_per_image_token` / `output_cost_per_video` 的模型行为完全不变。
- 无法识别 mode 的请求默认按 `chat` 处理。
- 现有 `chat` / `image_generation` / `audio_*` / cache 计费逻辑不变。
- Redis key 结构、配置格式均不发生改变。

### 12.2 风险与缓解

| 风险 | 缓解措施 |
|---|---|
| `calcChatCost` 增加 image input 拆分后，模型未配置新价格键 | `imageInputCost == 0` 时走原有 fallback 公式 |
| `DetectModeFromPath` 新增 `/v1/responses` 改变现有请求 mode | 仅影响 AI 网关计费匹配，不影响转发；默认 fallback 仍为 `chat` |
| 后端返回异常 `video_count` 导致费用为负 | `calcVideoGenerationCost` 中校验并截断为 `0` |
| proto 字段变更导致不兼容 | 重新生成 `bfe_access.pb.go`；紧急回滚时可只回滚生成代码 |

---

## 13. 参考资料

- `document-ai-gateway/迭代系统设计/v0.6/mode扩展/bfe-mode-and-price-extension-design.md`
- `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`
- `bfe/bfe_modules/mod_body_process/content_quota_usage.go`
- `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`
- `bfe/bfe_basic/request_ai_basic.go`
- `bfe/docs/zh_cn/sys_design/ai_access_log_fields.md`
