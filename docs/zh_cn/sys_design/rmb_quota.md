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

v0.5 进一步引入 **cache** 与 **音频 token** 子项计费：后端返回的 `usage` 中可能包含 `cache_read_tokens`、`cache_write_tokens`、`audio_input_tokens`、`audio_output_tokens`，BFE 需要把这些子项从 `prompt_tokens` / `completion_tokens` 中剥离，并按各自价格分别计费。针对 DeepSeek 等返回 `usage.prompt_cache_hit_tokens` 或 `usage.prompt_tokens_details.cached_tokens` 的模型，BFE 也将其识别为 cache read token。

v0.6 引入 **图像生成按次计费**、**图片输入 token 计费**、**视频生成按次计费** 以及 **Responses API 支持**：

- 图像生成模型（如 `flux-2-pro`）除可按实际生成的图像张数计费外，还可按 `input_cost_per_image_token` 对图片输入 token 单独计价；请求路径 `/v1/images/generations` 被识别为 `image_generation` 模式。
- 视频生成模型（如 `kling-*`）按实际生成的视频数量计费；`AIConf.ModelTable` 中新增 `output_cost_per_video` 价格字段，响应 `usage` 中新增 `video_count` 字段，请求路径 `/v1/video/generations` 被识别为 `video_generation` 模式。
- OpenAI Responses API 入口 `/v1/responses` 被识别为 `responses` 模式，本质上按 token 计费，复用 `chat` 计费逻辑。
- `TokenUsage` 新增 `ImageInputTokens`、`VideoCount` 字段，分别记录图片输入 token 数与生成视频数量。

### 1.2 目标

1. 配置层沿用 `AIConf.ModelTable`，价格以 `Prices` map（元/Token）下发，BFE 加载时转换为 1e-8 元/Token 定点整数；
2. `bfe_basic.TokenUsage` 增加 `UsedCost`，用于记录本次请求的 RMB 成本；
3. `mod_ai_token_auth.QuotaPlan` 增加 `Unit`，`Deduct` / `HasBalance` 支持 RMB；
4. 新增共享库 `go-lib/quota`，提供 RMB 定点数转换，供 ai-gateway-api 与 BFE 共同引用；
5. Redis Lua 支持 RMB 扣减脚本，当前暂时使用单 Key 定点数方案；
6. 支持按 `cache_read_tokens` / `cache_write_tokens` / `audio_input_tokens` / `audio_output_tokens` / `image_input_tokens` 子项拆分计费，并与普通 input/output 价格共存；
7. 支持图像生成模型按 `image_count` 与 `output_cost_per_image` 计费，支持图片输入 token 按 `input_cost_per_image_token` 计费，并按请求路径识别 `mode`；
8. 支持视频生成模型按 `video_count` 与 `output_cost_per_video` 计费，并按请求路径识别 `mode`；
9. 支持 Responses API 按 `/v1/responses` 路径识别为 `responses` 模式并按 token 计费；
10. v0.5 引入 **分时段/分工作日计费**：`ModelTable` 支持 `TimeZone` 与 `Tiers` 定义，`ModelPrice` 支持 `TierPrices`，请求发生时按当前时刻匹配 tier，命中则取 tier 价格，未命中 fallback 到默认 `Prices`；**初期 tier name 只支持 `peak`**。

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
    Prices              map[string]float64 // 价格对象，支持 input/output、cache_read/cache_creation、audio_input/audio_output、output_cost_per_image、input_cost_per_image_token、output_cost_per_video 等
    Metadata            map[string]interface{}
}

type ModelTable struct {
    Currency string       // v0.4 固定为 "RMB"
    Models   []ModelPrice

    // 运行时索引，配置加载阶段构建：model -> mode -> *ModelPrice
    priceIndex map[string]map[string]*ModelPrice
}

type AIConf struct {
    Type           int                // 保留字段，当前应为 0
    ModelMapping   *map[string]string // 模型映射
    Provider       string             // provider 名
    Keys           []AIKey            // 多 Key 模式
    KeyPolicy      *AIKeyPolicy       // Key 选择策略
    ModelTable     *ModelTable        // 成本定价表
    ModelProtocols []string           // provider 支持的协议：openai / anthropic；为空默认仅 openai
}
```

> 说明：`AIConf` 旧字段 `Key` 已移除，统一使用 `Keys`。

### 4.3 校验规则

1. `ModelTable.Currency` 当前仅允许 `"RMB"`。
2. `ModelPrice.Prices` 中 `input_cost_per_token`、`output_cost_per_token`、`cache_read_input_token_cost`、`cache_creation_input_token_cost`、`input_cost_per_audio_token`、`output_cost_per_audio_token`、`output_cost_per_image`、`input_cost_per_image_token`、`output_cost_per_video` 均必须 `>= 0`（未配置时按 `0` 处理）。
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
        cacheRead := price.Prices["cache_read_input_token_cost"]
        cacheWrite := price.Prices["cache_creation_input_token_cost"]
        audioInput := price.Prices["input_cost_per_audio_token"]
        audioOutput := price.Prices["output_cost_per_audio_token"]
        outputCostPerImage := price.Prices["output_cost_per_image"]
        inputCostPerImageToken := price.Prices["input_cost_per_image_token"]
        outputCostPerVideo := price.Prices["output_cost_per_video"]
        if input < 0 || output < 0 || cacheRead < 0 || cacheWrite < 0 ||
            audioInput < 0 || audioOutput < 0 || outputCostPerImage < 0 ||
            inputCostPerImageToken < 0 || outputCostPerVideo < 0 {
            return fmt.Errorf("negative price for model %s", price.Model)
        }
        price.Prices["input_cost_per_token_int"] = float64(quota.RmbToFixedPoint(input))
        price.Prices["output_cost_per_token_int"] = float64(quota.RmbToFixedPoint(output))
        price.Prices["cache_read_input_token_cost_int"] = float64(quota.RmbToFixedPoint(cacheRead))
        price.Prices["cache_creation_input_token_cost_int"] = float64(quota.RmbToFixedPoint(cacheWrite))
        price.Prices["input_cost_per_audio_token_int"] = float64(quota.RmbToFixedPoint(audioInput))
        price.Prices["output_cost_per_audio_token_int"] = float64(quota.RmbToFixedPoint(audioOutput))
        price.Prices["output_cost_per_image_int"] = float64(quota.RmbToFixedPoint(outputCostPerImage))
        price.Prices["input_cost_per_image_token_int"] = float64(quota.RmbToFixedPoint(inputCostPerImageToken))
        price.Prices["output_cost_per_video_int"] = float64(quota.RmbToFixedPoint(outputCostPerVideo))

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

### 4.5 分时段/分工作日计费配置

v0.5 在 `ModelTable` 与 `ModelPrice` 中扩展分时段计费字段：

```go
type TimeRange struct {
    Weekdays []int  // 0=周日, 1=周一 ... 6=周六；为空表示每天
    Start    string // "HH:MM"
    End      string // "HH:MM"，必须 > Start；跨午夜请拆成两段
}

type PriceTier struct {
    Name       string      // 初期只支持 "peak"
    TimeRanges []TimeRange // 命中任意一个即属于该 Tier
}

type ModelPrice struct {
    Provider            string
    Model               string
    BaseModel           string
    Mode                string
    Capabilities        []string
    SupportedParameters []string
    Limits              map[string]interface{}
    Prices              map[string]float64            // 默认价格
    TierPrices          map[string]map[string]float64 // tier name -> 价格表
    Metadata            map[string]interface{}

    // 运行时字段：配置加载阶段预计算定点整数
    pricesInt     map[string]int64
    tierPricesInt map[string]map[string]int64
}

type ModelTable struct {
    Currency string       // 仍是 "RMB"
    TimeZone string       // 默认 "Asia/Shanghai"
    Tiers    []PriceTier  // 时段定义
    Models   []ModelPrice

    priceIndex map[string]map[string]*ModelPrice
    tierIndex  map[string]*PriceTier
    tz         *time.Location
}
```

配置归属：provider/cluster 分离后，时段模板（`time_zone`、`tiers`）由 `/providers` 维护，通过独立接口 `PUT /providers/{provider_name}/pricing-tiers` 设置；模型级分时段价格（`tier_prices`）保留在 `/model-prices`。`ai-gateway-api` 导出 BFE 配置时，把同一 provider 的时段模板与 model-prices 的价格数据拼接成 `AIConf.ModelTable`。

校验规则：

1. `ModelTable.TimeZone` 为空时默认 `"Asia/Shanghai"`，须为合法 IANA 时区名。
2. `Tiers` 中每个 tier 必须包含非空 `Name` 和至少一个 `TimeRange`。
3. **初期 `Tiers` 中 tier 的 `Name` 只支持 `"peak"`**。
4. `TimeRange.Weekdays` 元素必须在 `0-6` 之间；为空表示每天。
5. `TimeRange.Start` / `End` 格式为 `"HH:MM"`，且 `End` > `Start`；跨午夜需拆成两段。
6. 同一 tier 内部 `TimeRanges` 不得重叠；不同 tier 之间允许重叠，按列表顺序匹配第一个。
7. `TierPrices` 中 tier name **初期只支持 `"peak"`**；内部价格键名须为 `Prices` 的合法枚举键；`TierPrices` 与 `Tiers` 不做强制引用校验，未在 `Tiers` 中定义的 tier name 运行时自然无法命中。

加载阶段处理：

- 解析 `TimeZone` 并缓存 `*time.Location`。
- 构建 `tierIndex[name] -> *PriceTier`，便于运行时 O(1) 查询。
- 将 `TierPrices` 中每个 tier 的价格表同样通过 `go-lib/quota.RmbToFixedPoint` 转换为定点整数，存入 `tierPricesInt`。

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
    PromptTokens      int64 // 请求侧 Token 数（包含 cache_read_tokens、audio_input_tokens、image_input_tokens）
    CompletionTokens  int64 // 响应侧 Token 数（包含 audio_output_tokens）
    CacheReadTokens   int64 // 从 cache 读取的 Token 数，已包含在 PromptTokens 中
    CacheWriteTokens  int64 // 写入 cache 的 Token 数，独立附加项
    AudioInputTokens  int64 // 音频输入 Token 数，已包含在 PromptTokens 中
    AudioOutputTokens int64 // 音频输出 Token 数，已包含在 CompletionTokens 中
    ImageInputTokens  int64 // 图片输入 Token 数，已包含在 PromptTokens 中
    VideoCount        int64 // 生成的视频数量（video_generation 模式使用）
    ImageCount        int64 // 生成的图像张数（image_generation 模式使用）
    UsedQuota         int64 // 已用 Token 配额（unit=total_token 时使用；image_generation 模式下为 image_count，video_generation 模式下为 video_count）
    UsedCost          int64 // 已用 RMB 成本，1 单位 = 1e-8 元（unit=RMB 时使用）
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
    // deducted marks whether this request has already been billed, to avoid
    // duplicate deduction when HandleRequestFinish is triggered more than once.
    deducted    bool
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

### 7.3 请求 mode 识别

`bfe_server/http_conn.go` 在初始化 `AiBasicInfo` 后，根据请求路径推断请求 mode：

```go
aiMeta.Mode = bfe_basic.DetectModeFromPath(request.HttpRequest.URL.Path)
```

`bfe_basic.DetectModeFromPath` 支持常见 OpenAI 风格路径：

| 路径前缀 | mode |
|----------|------|
| `/v1/images/generations` | `image_generation` |
| `/v1/images/edits` | `image_edit` |
| `/v1/chat/completions` | `chat` |
| `/v1/completions` | `completion` |
| `/v1/embeddings` | `embedding` |
| `/v1/audio/speech` | `audio_speech` |
| `/v1/audio/transcriptions` | `audio_transcription` |
| `/v1/rerank` | `rerank` |
| `/v1/video/generations` | `video_generation` |
| `/v1/responses` | `responses` |
| 其他 | 默认 `chat` |

mode 用于后续定价匹配（`(model, mode)` 二维索引）和访问日志输出。

### 7.4 响应阶段：`tokenReadResponseHandler`

`bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

响应阶段负责从响应体中提取 `usage`，或在未返回 `usage` 时按响应体长度估算 Token 数。对于图像生成响应，优先读取 `usage.image_count`，未返回时统计响应 `data` 数组长度，仍无则兜底请求体 `n` 字段（默认 1）。对于视频生成响应，优先读取 `usage.video_count`，未返回时兜底请求体 `n` 字段（默认 1）。对于非流式响应，`ContentLength >= 0` 时可直接读取完整响应体：

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

> 说明：旧实现中 RMB 成本在此阶段计算，导致流式响应（`ContentLength = -1`）无法计费。当前实现已将成本计算移到请求结束阶段，见 7.5。

#### 流式响应的 Token 用量收集

对于 `stream: true` 的 SSE 流式响应，`mod_body_process` 默认会注册 `QuotaUsageProcessor`：

- `mod_body_process.DoResponseProcess` 根据响应 `Content-Type` 选择 SSE 解码器；
- 每个 SSE 事件经过 `QuotaUsageProcessor.Process` 时，会从事件数据中提取 `usage.*_tokens`；
- 当遇到包含 `usage` 的最后一个事件时，将 `PromptTokens` / `CompletionTokens` / `UsedQuota` 写入 `AiBasicInfo.TokenUsage`。

因此，到请求结束阶段，`tokenUsage.PromptTokens` 和 `tokenUsage.CompletionTokens` 已经就绪，无论流式还是非流式都可以统一计算 RMB 成本。

#### DeepSeek / Responses API cache 字段兜底

DeepSeek 与 OpenAI Responses API 在 usage 中使用与常规 OpenAI/Claude 不同的字段名表示缓存命中：

- `usage.prompt_cache_hit_tokens`（DeepSeek）
- `usage.prompt_tokens_details.cached_tokens`（DeepSeek / OpenAI）
- `usage.input_token_details.cached_tokens`（Responses API）

BFE 在以下三处解析中，当 `cache_read_tokens` / `cache_read_input_tokens` 为 0 时，会依次 fallback 到上述字段：

- `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`：`UpdateCtxByUsage`（非流式）
- `bfe_modules/mod_body_process/llm_util.go`：`SSEEvent.GetQuotaUsage`（SSE 流式）
- `bfe_modules/mod_body_process/body_process.go`：`RawEvent.GetQuotaUsage`（RawEvent 非流式）

这样无论后端返回哪种字段名，`TokenUsage.CacheReadTokens` 都能被正确填充，后续 `calcChatCost` 按统一逻辑拆分计费。

### 7.5 请求结束阶段：`tokenRequestFinishHandler`

`bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

请求结束阶段统一计算 RMB 成本并扣减。`TokenAuthContext` 中已缓存 `serverConf`，因此即使 `req.SvrDataConf` 已被 reverse proxy 清空，仍然可以访问 cluster 定价表：

```go
func (m *ModuleAITokenAuth) tokenRequestFinishHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
    // 跳过非计费端点：Anthropic count_tokens 仅用于 token 计数，不应扣费
    if strings.Contains(req.HttpRequest.RequestURI, "/count_tokens") {
        return bfe_module.BfeHandlerGoOn
    }

    if res == nil || res.StatusCode != bfe_http.StatusOK {
        return bfe_module.BfeHandlerGoOn
    }

    ctx := GetTokenAuthContext(req)
    if ctx == nil {
        return bfe_module.BfeHandlerGoOn
    }

    // 已扣费则跳过，防止 HandleRequestFinish 多次触发导致重复扣费
    if ctx.deducted {
        return bfe_module.BfeHandlerGoOn
    }

    tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
    if tokenUsage.UsedQuota <= 0 && ctx.aiBasicInfo.IsAllowEstimateToken() {
        tokenUsage.UsedQuota = CalcReqUsedQuota(req, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
    }

    // 统一在请求完成阶段计算 RMB 成本（流式由 mod_body_process 填充 token 用量）
    if tokenUsage.UsedCost <= 0 && hasRMBPlan(ctx.Token.QuotaPlans) {
        tokenUsage.UsedCost = m.calcCostUnits(req, ctx.serverConf, tokenUsage)
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

    ctx.deducted = true
    return bfe_module.BfeHandlerGoOn
}
```

### 7.6 成本计算辅助方法

新增方法（位于 `mod_ai_token_auth`）。成本计算按 `AiBasicInfo.Mode` 分支：

```go
func (m *ModuleAITokenAuth) calcCostUnits(req *bfe_basic.Request, serverConf bfe_basic.ServerDataConfInterface, usage *bfe_basic.TokenUsage) int64 {
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

    if serverConf == nil {
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

    tierName := ""
    if cluster.AIConf.ModelTable != nil {
        tierName = cluster.AIConf.ModelTable.ActiveTierName(time.Now())
    }

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
}

func calcResponsesCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
    // Responses API 本质上按 token 计费，当前复用 chat 计费逻辑。
    return calcChatCost(entry, usage, tierName)
}

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

func calcChatCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
    inputCost := entry.GetPriceInt(tierName, cluster_conf.PriceInputCostPerTokenInt)
    outputCost := entry.GetPriceInt(tierName, cluster_conf.PriceOutputCostPerTokenInt)
    if inputCost < 0 || outputCost < 0 {
        log.Logger.Warn("invalid model price for model %s", entry.Model)
        return 0
    }

    promptTokens := usage.PromptTokens
    completionTokens := usage.CompletionTokens
    cacheReadTokens := usage.CacheReadTokens
    cacheWriteTokens := usage.CacheWriteTokens
    audioInputTokens := usage.AudioInputTokens
    audioOutputTokens := usage.AudioOutputTokens

    // sanitize sub-token usage to avoid negative normal input/output or negative charges
    if cacheReadTokens < 0 {
        cacheReadTokens = 0
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

    cacheReadCost := entry.GetPriceInt(tierName, cluster_conf.PriceCacheReadInputTokenCostInt)
    cacheWriteCost := entry.GetPriceInt(tierName, cluster_conf.PriceCacheCreationInputTokenCostInt)
    audioInputCost := entry.GetPriceInt(tierName, cluster_conf.PriceInputCostPerAudioTokenInt)
    audioOutputCost := entry.GetPriceInt(tierName, cluster_conf.PriceOutputCostPerAudioTokenInt)
    imageInputCost := entry.GetPriceInt(tierName, cluster_conf.PriceInputCostPerImageTokenInt)

    // normal input/output start as the full totals
    normalInput := promptTokens
    normalOutput := completionTokens

    // cache-aware billing: split cache read from prompt
    if cacheReadCost > 0 || cacheWriteCost > 0 {
        normalInput = promptTokens - cacheReadTokens
        if normalInput < 0 {
            normalInput = 0
        }
    }

    // image-aware billing: split image input from normal input
    imageInputTokens := usage.ImageInputTokens
    if imageInputCost > 0 {
        if imageInputTokens > normalInput {
            imageInputTokens = normalInput
        }
        normalInput = normalInput - imageInputTokens
        if normalInput < 0 {
            normalInput = 0
        }
    } else {
        // no image input price configured: bill image input as normal input
        imageInputTokens = 0
    }

    // audio-aware billing: split audio input from normal input
    if audioInputCost > 0 {
        if audioInputTokens > normalInput {
            audioInputTokens = normalInput
        }
        normalInput = normalInput - audioInputTokens
        if normalInput < 0 {
            normalInput = 0
        }
    } else {
        // no audio input price configured: bill audio input as normal input
        audioInputTokens = 0
    }

    // audio-aware billing: split audio output from completion
    if audioOutputCost > 0 {
        if audioOutputTokens > completionTokens {
            audioOutputTokens = completionTokens
        }
        normalOutput = completionTokens - audioOutputTokens
        if normalOutput < 0 {
            normalOutput = 0
        }
    } else {
        // no audio output price configured: bill audio output as normal output
        audioOutputTokens = 0
    }

    var cost int64
    if cacheReadCost > 0 || cacheWriteCost > 0 || audioInputCost > 0 || audioOutputCost > 0 || imageInputCost > 0 {
        cost = normalInput*inputCost +
            cacheReadTokens*cacheReadCost +
            cacheWriteTokens*cacheWriteCost +
            audioInputTokens*audioInputCost +
            imageInputTokens*imageInputCost +
            normalOutput*outputCost +
            audioOutputTokens*audioOutputCost
    } else {
        // fallback to legacy billing when no cache/audio/image price is configured
        cost = promptTokens*inputCost + completionTokens*outputCost
    }

    return cost
}
```

#### 图像生成计费公式

`image_generation` 模式下成本由 **图像张数** 与 **图片输入 token** 两部分组成：

```
image_count = usage.image_count
            ?? len(response.data)
            ?? request.n
            ?? 1

image_input_tokens = usage.input_token_details.image_tokens
                   ?? usage.image_input_tokens
                   ?? 0

cost = image_count * output_cost_per_image
     + image_input_tokens * input_cost_per_image_token
```

- 优先读取响应 `usage.image_count`；
- 未返回时统计响应 `data` 数组长度（OpenAI 风格图像生成响应）；
- 仍无则兜底读取请求体 `n` 字段，未传时默认 `1`；
- 图片输入 token 优先读取 `usage.input_token_details.image_tokens`，fallback 到 `usage.image_input_tokens`；
- 未配置 `input_cost_per_image_token` 时，图片输入 token 按普通 input token 计费。

#### chat 模式 cache/image/audio 计费拆分公式

在 `calcChatCost` 中，RMB 成本按如下优先级拆分：

1. **Cache read 从 prompt 中剥离**：若配置了 `cache_read_input_token_cost`，则 `normal_input = prompt_tokens - cache_read_tokens`。`cache_read_tokens` 除直接读取 `usage.cache_read_tokens` 外，也支持 DeepSeek 的 `usage.prompt_cache_hit_tokens` / `usage.prompt_tokens_details.cached_tokens` 字段，以及 Responses API 的 `usage.input_token_details.cached_tokens` 字段。
2. **Image input 从剩余 normal input 中剥离**：若配置了 `input_cost_per_image_token`，则 `image_input_tokens` 按图片 input 价格计费，其余仍按普通 input 价格计费。
3. **Audio input 从剩余 normal input 中剥离**：若配置了 `input_cost_per_audio_token`，则 `audio_input_tokens` 按 audio 价格计费，其余仍按普通 input 价格计费。
4. **Audio output 从 completion 中剥离**：若配置了 `output_cost_per_audio_token`，则 `audio_output_tokens` 按 audio 价格计费，其余仍按普通 output 价格计费。
5. **Cache write 独立计费**：`cache_write_tokens` 不参与 prompt/completion 总量拆分，单独按 `cache_creation_input_token_cost` 计费。
6. **未配置子项价格时回退**：若某类子项价格未配置（`<= 0`），对应子项仍按普通 input/output 价格计费，保证向后兼容。

> **计费修复说明**： Anthropic 协议下 `prompt_tokens` 仅包含 cache miss 部分，`cache_read_tokens` 可能远大于 `prompt_tokens`。旧逻辑曾对 `cache_read_tokens` 做 `min(prompt_tokens)` 截断，导致高 cache 命中场景少收；修复后已移除该截断，仅保留 `normal_input = max(prompt_tokens - cache_read_tokens, 0)` 的非负保护。

#### 视频生成计费公式

`video_generation` 模式下成本仅与生成视频数量相关：

```
video_count = usage.video_count
            ?? request.n
            ?? 1

cost = video_count * output_cost_per_video
```

- 优先读取响应 `usage.video_count`；
- 未返回时兜底读取请求体 `n` 字段，未传时默认 `1`；
- 未配置 `output_cost_per_video` 时按 `0` 成本处理。

#### responses 计费公式

`responses` 模式本质上按 token 计费，当前复用 `chat` 计费逻辑：

```
cost = prompt_tokens * input_cost_per_token
     + completion_tokens * output_cost_per_token
```

- 若后端返回 `usage.input_token_details.cached_tokens`，同样会按 `cache_read_input_token_cost` 拆分计费；
- 流式 Responses API 的 usage 通常在最后一个 `response.completed` 事件中提供，BFE 在请求结束阶段统一结算。

说明：

- `req.Route.ClusterName` 在 `reverseproxy.go` 的 `aiClusterInvoke()` 中已被设置为最终实际使用的 cluster（包括 fallback 场景）。
- `aiMeta.TargetModel` 在 `reverseproxy.go` 的 `doSingleAIForward()` 中已被设置为路由目标模型 + cluster `ModelMapping` 映射后的最终模型名。
- 因此这里拿到的 `clusterName` 和 `targetModel` 就是计费所需的实际值。
- 价格到定点整数的转换在配置加载阶段通过 `go-lib/quota` 完成，运行时 `calcCostUnits` 只处理整数，保证 Redis Lua 不接触浮点。

### 7.7 定价匹配逻辑

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

索引在配置加载阶段构建，运行时按 `(model, mode)` 精确查询，为 O(1)。`mode` 由请求路径推断（如 `/v1/images/generations` → `image_generation`，`/v1/chat/completions` → `chat`），未识别时默认 `chat`。未命中时返回 `nil`，由调用方决定是否按 `0` 成本处理。

### 7.8 分时段计费运行时匹配

`ModelTable` 在运行时根据请求发生时刻（取 BFE 本地时间，按 `TimeZone` 转换）匹配活跃 tier：

```go
func (table *ModelTable) ActiveTierName(now time.Time) string {
    if table == nil || len(table.Tiers) == 0 {
        return ""
    }
    t := now.In(table.tz)
    wd := int(t.Weekday())
    hour, min := t.Hour(), t.Minute()
    cur := hour*60 + min

    for i := range table.Tiers {
        tier := &table.Tiers[i]
        for _, tr := range tier.TimeRanges {
            if len(tr.Weekdays) > 0 && !containsInt(tr.Weekdays, wd) {
                continue
            }
            start := parseHHMM(tr.Start)
            end := parseHHMM(tr.End)
            if start <= cur && cur < end {
                return tier.Name
            }
        }
    }
    return ""
}

func (p *ModelPrice) GetPriceInt(tier, key string) int64 {
    if tier != "" && p.tierPricesInt != nil {
        if tierMap, ok := p.tierPricesInt[tier]; ok {
            if v, ok := tierMap[key]; ok {
                return v
            }
        }
    }
    if p.pricesInt != nil {
        return p.pricesInt[key]
    }
    return 0
}
```

`calcCostUnits` 在计算成本前先调用 `ActiveTierName`，再按 tier 取价。以 `chat` 模式为例，`calcChatCost` 在读取 `inputCost`、`outputCost`、`cacheReadCost` 等定点整数价格时，均通过 `GetPriceInt(tierName, key)` 获取：命中 `peak` tier 且 `TierPrices.peak` 中配置了该键，则使用 tier 价格；否则 fallback 到默认 `Prices`。

```go
func (m *ModuleAITokenAuth) calcCostUnits(req *bfe_basic.Request, serverConf bfe_basic.ServerDataConfInterface, usage *bfe_basic.TokenUsage) int64 {
    // ... 查找 entry ...

    tierName := cluster.AIConf.ModelTable.ActiveTierName(time.Now())

    inputCost := entry.GetPriceInt(tierName, cluster_conf.PriceInputCostPerTokenInt)
    outputCost := entry.GetPriceInt(tierName, cluster_conf.PriceOutputCostPerTokenInt)
    cacheReadCost := entry.GetPriceInt(tierName, cluster_conf.PriceCacheReadInputTokenCostInt)
    // ... 其他子项价格同样按 tierName 获取 ...

    // 后续 cache/audio 拆分与 7.6 节一致
}
```

计费规则：

- 命中 `peak` tier：所有价格优先取 `TierPrices.peak`；若某键未配置，fallback 到默认 `Prices`。
- 未命中任何 tier：全部使用默认 `Prices`，行为与 v0.4 完全一致。
- `Tiers` 为空或 `TierPrices` 为空：直接退化到固定价格逻辑。
- 区间采用左闭右开 `[Start, End)`，例如 `18:00` 不命中 `09:00-18:00`。

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
                        "output_cost_per_token": 0.000002,
                        "cache_read_input_token_cost": 0.0000005,
                        "cache_creation_input_token_cost": 0.0000015,
                        "input_cost_per_audio_token": 0.00002,
                        "output_cost_per_audio_token": 0.00004
                    }
                },
                {
                    "Provider": "mock-provider",
                    "Model": "flux-2-pro",
                    "BaseModel": "flux-2-pro",
                    "Mode": "image_generation",
                    "Capabilities": ["image_generation"],
                    "SupportedParameters": ["prompt", "n", "size"],
                    "Limits": {
                        "context_window": 128000
                    },
                    "Prices": {
                        "output_cost_per_image": 0.03,
                        "input_cost_per_image_token": 0.0000005
                    }
                },
                {
                    "Provider": "mock-provider",
                    "Model": "kling-video-pro",
                    "BaseModel": "kling-video-pro",
                    "Mode": "video_generation",
                    "Capabilities": ["video_generation"],
                    "SupportedParameters": ["prompt", "n"],
                    "Limits": {
                        "context_window": 128000
                    },
                    "Prices": {
                        "output_cost_per_video": 0.5
                    }
                },
                {
                    "Provider": "openai",
                    "Model": "o3-deep-research",
                    "BaseModel": "o3-deep-research",
                    "Mode": "responses",
                    "Capabilities": ["chat", "reasoning"],
                    "SupportedParameters": ["temperature", "max_tokens"],
                    "Limits": {
                        "context_window": 200000
                    },
                    "Prices": {
                        "input_cost_per_token": 0.00001,
                        "output_cost_per_token": 0.00002
                    }
                }
            ]
        }
    }
}
```

> 上例中 `input_cost_per_token = 0.000001` 元/Token，换算为固定点整数即 `100`（= 0.000001 * 1e8），表示 **0.1 元 / 百万 Token**；`cache_read_input_token_cost` 等子项价格同样通过 `quota.RmbToFixedPoint` 转换为定点整数；`output_cost_per_image = 0.03` 元/张，换算为定点整数 `3000000`；`input_cost_per_image_token = 0.0000005` 元/Token 换算为定点整数 `50`；`output_cost_per_video = 0.5` 元/个换算为定点整数 `50000000`。

### 9.2 分时段计费配置示例（DeepSeek）

以 DeepSeek-V4-Pro 为例，北京时间周一至周五 `09:00-12:00`、`14:00-18:00` 为高峰时段，其余时间（含工作日非高峰及周末）为空闲时段；空闲价格为高峰价格的一半：

```json
{
    "AIConf": {
        "Type": 0,
        "Provider": "deepseek",
        "ModelTable": {
            "Currency": "RMB",
            "TimeZone": "Asia/Shanghai",
            "Tiers": [
                {
                    "Name": "peak",
                    "TimeRanges": [
                        { "Weekdays": [1, 2, 3, 4, 5], "Start": "09:00", "End": "12:00" },
                        { "Weekdays": [1, 2, 3, 4, 5], "Start": "14:00", "End": "18:00" }
                    ]
                }
            ],
            "Models": [
                {
                    "Provider": "deepseek",
                    "Model": "deepseek-v4-pro",
                    "BaseModel": "deepseek-v4-pro",
                    "Mode": "chat",
                    "Capabilities": ["chat", "reasoning", "tools", "prompt_caching"],
                    "SupportedParameters": ["temperature", "max_tokens"],
                    "Limits": {
                        "context_window": 128000,
                        "max_input_tokens": 128000,
                        "max_output_tokens": 8192
                    },
                    "Prices": {
                        "input_cost_per_token": 4.5e-06,
                        "output_cost_per_token": 1.35e-05,
                        "cache_read_input_token_cost": 1.5e-07
                    },
                    "TierPrices": {
                        "peak": {
                            "input_cost_per_token": 9.0e-06,
                            "output_cost_per_token": 2.7e-05,
                            "cache_read_input_token_cost": 3.0e-07
                        }
                    }
                }
            ]
        }
    }
}
```

> 说明：
> - 本示例只定义 `peak` tier；未命中高峰时段时，`calcChatCost` 自动使用默认 `Prices`（即空闲价格）。
> - `TierPrices.peak` 中未配置的键将 fallback 到 `Prices` 中的对应键。
> - 若后续需要把周末等时段单独定价，可新增 tier 并通过 `Weekdays` 指定，无需修改计费逻辑。

## 10. 测试建议

1. **单元测试**
   - `QuotaPlan.Deduct`：分别覆盖 `total_token` 和 `RMB` 两种单位，以及余额不足、Key 不存在等边界。
   - `lookupModelPrice`：精确匹配、未命中返回 nil。
   - `calcCostUnits`：正常计算、ModelTable 缺失、模型未命中、价格转换精度；
     - 覆盖 cache read/write 与 audio input/output 子项拆分；
     - 覆盖子项用量大于总量时的 clamp 行为；
     - 覆盖未配置子项价格时回退到普通 input/output 价格的行为；
     - 覆盖 `image_generation` 模式按 `image_count × output_cost_per_image + image_input_tokens × input_cost_per_image_token` 计费，以及未配置 `output_cost_per_image` 时按 0 成本处理；
     - 覆盖 `video_generation` 模式按 `video_count × output_cost_per_video` 计费；
     - 覆盖 `responses` 模式按 chat 公式计费；
     - 覆盖 chat 模式下 `image_input_tokens` 从 normal input 中拆分计费；
     - 覆盖 Anthropic 高 cache 命中场景（`cache_read_tokens > prompt_tokens`）不再被截断，cache 按实际值计费；
     - 覆盖 `tokenRequestFinishHandler` 对 `/count_tokens` 端点跳过扣费；
     - 覆盖 `HandleRequestFinish` 多次触发时仅扣费一次（`deducted` 幂等标记）。
   - `ActiveTierName`：验证北京时区周一 10:00 命中 `peak`、周一 13:00 未命中、周六 10:00 未命中、周一 18:00 不命中（左闭右开）。
   - `GetPriceInt`：验证命中 `peak` 时取 `TierPrices.peak`、未命中时 fallback 到 `Prices`、tier 中未配置某键时 fallback 到默认价格。

2. **Lua 脚本测试**
   - 单 Key 定点数方案：验证扣减、余额归零、负数不溢出。
   - Hash 拆分方案：验证借位、余额归零、大数（接近 100 亿元）正确性。

3. **集成测试**
   - 创建一个 `unit = "RMB"` 的 API Key，发一次 chat 请求，验证 Redis 余额按预期扣减。
   - 测试 `ModelMapping` 场景：请求模型是 `gpt-4`，实际后端模型是 `deepseek-chat`，验证按 `deepseek-chat` 的价格计费。
   - 测试 fallback 场景：请求最终 fallback 到另一个 cluster，验证按最终 cluster + target_model 计费。
   - 测试流式（SSE）场景：请求体带 `stream: true`，后端返回 SSE 并在最后一个 chunk 中携带 `usage`，验证 RMB 配额仍能正确扣减。
   - 测试 cache/audio 子项计费场景：后端返回 `usage.cache_read_tokens`、`usage.cache_write_tokens`、`usage.audio_input_tokens`、`usage.audio_output_tokens`，验证成本按各子项价格拆分计算；
   - 测试 DeepSeek cache 字段识别场景：后端返回 `usage.prompt_cache_hit_tokens` 或 `usage.prompt_tokens_details.cached_tokens`，验证 `CacheReadTokens` 被正确填充并按 `cache_read_input_token_cost` 拆分计费；
   - 测试图像生成按次计费场景：请求 `/v1/images/generations`，`ModelTable` 配置 `output_cost_per_image` 与 `input_cost_per_image_token`，后端返回 `usage.image_count` 与 `usage.input_token_details.image_tokens`，验证 RMB 配额按 `image_count × output_cost_per_image + image_input_tokens × input_cost_per_image_token` 扣减，且 `total_token` 配额按 `image_count` 扣减；
   - 测试视频生成按次计费场景：请求 `/v1/video/generations`，`ModelTable` 配置 `output_cost_per_video`，后端返回 `usage.video_count`，验证 RMB 配额按 `video_count × output_cost_per_video` 扣减，且 `total_token` 配额按 `video_count` 扣减；
   - 测试 Responses API 计费场景：请求 `/v1/responses`，`ModelTable` 配置 `input_cost_per_token` / `output_cost_per_token`，后端返回 `usage.input_tokens` / `output_tokens`，验证按 chat 公式扣减；
   - 测试 Anthropic `/count_tokens` 端点不计费：请求 `/anthropic/v1/messages/count_tokens`，验证 RMB 与 token 配额均不被扣减；
   - 测试计费幂等场景：模拟 `HandleRequestFinish` 被触发两次，验证 Redis 余额只扣减一次；
   - 测试分时段计费场景：`ModelTable` 配置 `Tiers` 与 `TierPrices`；分别在北京时间高峰时段（如周一 10:00）与非高峰时段（如周一 13:00 或周六 10:00）发起请求，验证 Redis 扣减金额分别按 `TierPrices.peak` 与默认 `Prices` 计算。
   - 测试分时段 + cache 命中组合场景：高峰时段且后端返回 `usage.cache_read_tokens`（或 DeepSeek 的 `usage.prompt_cache_hit_tokens` / `usage.prompt_tokens_details.cached_tokens`），验证缓存命中部分按 `TierPrices.peak.cache_read_input_token_cost` 计费，未命中部分按 `TierPrices.peak.input_cost_per_token` 计费。

## 11. 兼容性与注意事项

1. **存量 Token 配额完全兼容**：`Unit` 默认 `"total_token"`，走原有 Lua 扣减逻辑。
2. **浮点禁止进入 Redis**：所有金额在 BFE 内部和 Redis 中均以固定点整数表示，避免浮点误差；价格浮点转换仅在 BFE 配置加载阶段完成，conf-agent 不做任何转换。
3. **无 ModelTable 时的兜底行为**：若 RMB 配额计划命中的 cluster 没有配置 `ModelTable`，或没有匹配到模型条目，当前建议：
   - 记录告警日志；
   - 本次请求不对该 RMB 配额进行扣减（相当于按 `0` 成本处理）；
   - 具体是否拒绝请求，需产品进一步确认。
4. **与多 Key 改造的关系**：`AIConf.Keys` 与 `ModelTable` 相互独立，可并行下发、独立解析。
5. **流式响应计费**：RMB 成本在请求结束阶段计算，依赖 `mod_body_process`（或其他响应处理模块）在流式传输过程中填充 `PromptTokens` / `CompletionTokens` / `CacheReadTokens` / `CacheWriteTokens` / `AudioInputTokens` / `AudioOutputTokens` / `ImageInputTokens` / `ImageCount` / `VideoCount`。生产环境若启用流式计费，需确保 `mod_body_process` 已加载。
6. **模块顺序建议**：`mod_ai_token_auth` 的 `HandleReadResponse` 不再负责 RMB 成本计算，因此对模块加载顺序的敏感度降低；但仍建议保持 `mod_ai_token_auth` 在 `mod_body_process` 之前注册，以便非流式场景下 token 用量解析逻辑保持一致。
7. **旧字段清理**：`AIConf.Key` 已移除，统一使用 `AIConf.Keys`。
8. **分时段计费向后兼容**：`ModelTable.Tiers` / `ModelPrice.TierPrices` 均为可选字段；未配置时行为与固定价格完全一致。命中 tier 但该 tier 未配置某个价格键时，自动 fallback 到默认 `Prices`。
9. **计费修复影响**：
   - 移除 `cache_read_tokens > prompt_tokens` 截断后，Anthropic 高 cache 命中场景的计费会 **上升**，这是修正错误少收后的正确行为；
   - `count_tokens` 端点此前被误扣费，修复后该端点不再扣费，运营侧需在计费对账层单独处理历史差异；
   - `deducted` 幂等标记仅防止同一请求生命周期内的重复扣费，不跨请求生效，不影响正常流量。
