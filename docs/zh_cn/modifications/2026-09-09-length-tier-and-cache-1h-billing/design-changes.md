# BFE 长度分档计费与 1h 缓存写价支持

## 1. 背景

控制面 `ai-gateway-api` 导入的模型目录（`model-list.yaml`，450 条记录）中出现了两类当前 BFE 不认识、且 api 侧也仅部分支持的价格键：

1. **长度分档价**：`input/output_cost_per_token_above_{200k,256k,272k,512k}_tokens`，共 19 个模型（gpt-5.4/5.5/5.6 系列、qwen3.6-flash、qwen3.7-plus、gemini-3.1-pro-preview、MiniMax-M3）。上下文长度超过阈值后单价跳档，当前 BFE 只按基础价计费，超长请求**少计费**。
2. **1h TTL 缓存写价**：`cache_creation_input_token_cost_1h`，共 6 个 claude 系模型（opus-4-7/4-8/4-8-thinking/5、fable-5、sonnet-5）。1h TTL 缓存写价格约为 5m 的 1.6 倍，当前 BFE 统一按 5m 价计，**少计费**。

总体方案（控制面与数据面协同修改的完整说明另见于控制面仓库设计文档）：

- **两端硬编码**长度分档四档（200k/256k/272k/512k）与 `_1h` 缓存写价，不做动态键泛化；
- api 侧补齐 `input_cost_per_image_token`、`input/output_cost_per_audio_token` 三个 BFE 已支持的对齐键，导出链路（`model/icluster_conf/cluster.go` map 透传）零改动。

本文描述 BFE 侧的修改；ai-gateway-api 侧修改（`model/imodel_price/validate.go` 枚举扩充等）在其仓库文档中另行说明。

前置依赖：`bfe-access-pb` 已发布 `v0.3.6`，新增 `ai_cache_write_1h_tokens`（field 788），BFE `go.mod` 需从 `v0.3.5` 升级。

## 2. 需求示例

`cluster_conf.data` 中将出现新价格键：

```json
{
    "Provider": "example-provider",
    "ModelTable": {
        "Currency": "RMB",
        "Models": [
            {
                "Provider": "example-provider",
                "Model": "gpt-5.5",
                "BaseModel": "gpt-5.5",
                "Mode": "chat",
                "Prices": {
                    "input_cost_per_token": 2.431e-05,
                    "output_cost_per_token": 0.00014586,
                    "cache_read_input_token_cost": 2.431e-06,
                    "input_cost_per_token_above_272k_tokens": 4.862e-05,
                    "output_cost_per_token_above_272k_tokens": 0.00021879
                }
            },
            {
                "Provider": "example-provider",
                "Model": "claude-opus-4-8",
                "BaseModel": "claude-opus-4-8",
                "Mode": "chat",
                "Prices": {
                    "input_cost_per_token": 3.077e-05,
                    "output_cost_per_token": 0.00015385,
                    "cache_creation_input_token_cost": 3.84625e-05,
                    "cache_creation_input_token_cost_1h": 6.154e-05
                }
            }
        ]
    }
}
```

期望计费行为：

- `gpt-5.5` 输入 30 万 token（≥272k 档）：输入按 `4.862e-05`/token、输出按 `0.00021879`/token 计；
- `claude-opus-4-8` 写 1h 缓存 1 万 token、5m 缓存 5 千 token：`10000×6.154e-05 + 5000×3.84625e-05`。

## 3. 当前现状

| 层级 | 当前实现 | 不足 |
|------|---------|------|
| 价格常量 | `cluster_conf_load.go:227-237` 定义 9 个价格键常量 | 无长度分档键、无 `_1h` 键 |
| 配置校验 | `ModelTableCheck`（`:1288-1311`）对 9 个已知键做负价校验；未知键原样保留在 `Prices` 中 | 新键不参与校验，也不参与计费 |
| 计费选价 | `calcChatCost`（`mod_ai_token_auth.go:591` 起）只取基础 input/output/cache 价 | 超长上下文与 1h 缓存写均按基础价计，少计费 |
| usage 解析 | `llm_util.go` 解析 cache write 总量，不区分 TTL 档位 | 无法拆分 5m/1h |
| 访问日志 | proto 781-787 区间已有 cache/audio/image/video 计量字段 | 无 1h 缓存写字段 |

## 4. 变更目标

1. `ModelPrice` 加载时从硬编码的 8 个长度分档键解析出有序档位表，提供按上下文长度选价的取值方法；
2. `cache_creation_input_token_cost_1h` 参与负价校验与计费，usage 链路新增 1h 缓存写 token 的解析、透传与访问日志输出；
3. 未配置新键的模型走原有代码路径，计费结果与现状**逐分一致**；
4. `calcResponsesCost` 复用 `calcChatCost`，自动继承两项能力。

## 5. 变更总览

```
cluster_conf.data (含 above_*_tokens / *_1h 键)
        │
        ▼
cluster_conf_load.go            新增: 8 个档位键 + _1h 键常量、负价校验、
  ModelTableCheck                    ModelPrice.lengthTiers 解析、GetLengthTierPrice
        │
        ▼
mod_body_process.llm_util.go    新增: usage.cache_creation.ephemeral_1h_input_tokens
  QuotaUsage / bfe_basic.TokenUsage  解析 → CacheWriteTokens1h，逐层透传
        │
        ▼
mod_ai_token_auth.calcChatCost  新增: 按 promptTokens 选长度档位价；
                                 cache write 拆分为 5m/1h 两段分别计价
        │
        ▼
mod_access_pb3.request_log.go   新增: ai_cache_write_1h_tokens (788) 输出
```

## 6. 详细设计

### 6.1 `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`

**新增常量**（`:227-237` 常量区，硬编码）：

```go
PriceCacheCreationInputTokenCost1h = "cache_creation_input_token_cost_1h"

PriceInputCostPerTokenAbove200kTokens  = "input_cost_per_token_above_200k_tokens"
PriceOutputCostPerTokenAbove200kTokens = "output_cost_per_token_above_200k_tokens"
PriceInputCostPerTokenAbove256kTokens  = "input_cost_per_token_above_256k_tokens"
PriceOutputCostPerTokenAbove256kTokens = "output_cost_per_token_above_256k_tokens"
PriceInputCostPerTokenAbove272kTokens  = "input_cost_per_token_above_272k_tokens"
PriceOutputCostPerTokenAbove272kTokens = "output_cost_per_token_above_272k_tokens"
PriceInputCostPerTokenAbove512kTokens  = "input_cost_per_token_above_512k_tokens"
PriceOutputCostPerTokenAbove512kTokens = "output_cost_per_token_above_512k_tokens"
```

**`ModelPrice` 新增解析结果字段**：

```go
// lengthTiers 在 ModelTableCheck 时从 Prices 中上述 8 个硬编码键解析：
// 有序档位表（threshold tokens 升序）。
lengthTiers []lengthTier

type lengthTier struct {
    threshold   int64   // 200000 / 256000 / 272000 / 512000
    inputPrice  float64 // 未配置该档 input 键时为 -1（沿用基础价）
    outputPrice float64
}
```

**`ModelTableCheck`**（`:1288-1311`）负价校验扩展：

```go
cacheWrite1h := price.Prices[PriceCacheCreationInputTokenCost1h]
if cacheWrite1h < 0 {
    return fmt.Errorf("negative price for model %s", price.Model)
}
// 8 个档位键逐个负价校验；非负者填入 price.lengthTiers 并按 threshold 升序排序。
```

**新增取值方法**（与 `GetPrice` 同级，tier 优先 / fallback 默认价语义与 `GetPrice` 完全一致，`TierPrices["peak"]` 中配置的档位键同样参与选档）：

```go
// GetLengthTierPrice 返回超过 threshold 档位后的 input/output 单价；
// 未配置任何档位键时返回 ok=false，调用方走原有基础价逻辑。
func (p *ModelPrice) GetLengthTierPrice(tierName string, promptTokens int64) (input, output float64, ok bool)
```

### 6.2 usage 解析（1h 缓存写拆分）

`bfe_modules/mod_body_process/llm_util.go` 的 `GetQuotaUsage()` 新增解析（Anthropic 系上游对 1h TTL 缓存写单独报告，部分中转用兜底字段）：

```go
cacheWrite1h := gjson.GetBytes(data, "usage.cache_creation.ephemeral_1h_input_tokens").Int()
if cacheWrite1h == 0 {
    cacheWrite1h = gjson.GetBytes(data, "usage.cache_creation_input_tokens_1h").Int()
}
```

`QuotaUsage`（`llm_util.go`）与 `bfe_basic.TokenUsage`（`bfe_basic/request_ai_basic.go`）各新增字段：

```go
CacheWriteTokens1h int64 // 1h TTL 缓存写入 token（已包含在 CacheWriteTokens 中）
```

`content_quota_usage.go` 两处透传分支（`UsedQuota > 0` 与估算分支）同步赋值。

### 6.3 计费逻辑 `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

**`calcChatCost` 长度选档**（拿到 `promptTokens` 后先选价）：

```go
inputPrice := entry.GetPrice(tierName, cluster_conf.PriceInputCostPerToken)
outputPrice := entry.GetPrice(tierName, cluster_conf.PriceOutputCostPerToken)
if tierIn, tierOut, ok := entry.GetLengthTierPrice(tierName, promptTokens); ok {
    if tierIn >= 0  { inputPrice = tierIn }
    if tierOut >= 0 { outputPrice = tierOut }
}
```

选档口径：**按输入 token 数（`promptTokens`，含 cache read/write）与阈值比较**，超过某档即整单（输入+输出）按该档价计，与 GPT 系上游"按上下文长度定价"口径一致；若后续上游改为按输出分档，只需改这一处比较量。

**`calcChatCost` 1h 缓存写拆分**（仿现有 audio 拆分的 clamp 写法）：

```go
cacheWrite1h := usage.CacheWriteTokens1h
cacheWritePrice1h := entry.GetPrice(tierName, cluster_conf.PriceCacheCreationInputTokenCost1h)
if cacheWritePrice1h > 0 {
    if cacheWrite1h > cacheWriteTokens {
        cacheWrite1h = cacheWriteTokens
    }
    cacheWriteTokens -= cacheWrite1h // 剩余按 5m（基础 cache_creation 价）计
} else {
    cacheWrite1h = 0 // 未配置 1h 价：全部按基础价计，行为与现状一致
}
```

总价公式新增 `quota.CalcCostUnits(cacheWrite1h, cacheWritePrice1h)` 一项。

**估算兜底兼容**：`EstimateContentToken` 估算路径不区分档位与 TTL，按基础价计——接受（估算本身已保守），`docs/zh_cn/sys_design/rmb_quota.md` 注明。

### 6.4 访问日志

依赖 `bfe-access-pb v0.3.6`（`ai_cache_write_1h_tokens`，field 788；Go 侧字段名为 `AiCacheWrite_1HTokens`，protoc-gen-go 对 `1h` 的驼峰化结果）。`bfe/go.mod` 升级：

```
github.com/bfenetworks/bfe-access-pb v0.3.5 → v0.3.6
```

`bfe_modules/mod_access_pb3/request_log.go` 中新增：

```go
if usage.CacheWriteTokens1h > 0 {
    reqLog.AiCacheWrite_1HTokens = proto.Int64(usage.CacheWriteTokens1h)
}
```

`docs/zh_cn/sys_design/ai_access_log_fields.md` 字段表同步新增 788 行。

> 长度分档不产生新 usage 字段（选档依据已有的 `ai_input_tokens`），无需 proto 变更。

## 7. 测试计划

### 7.1 单元测试

- `cluster_conf_load_test.go`：`_1h` 键与 8 个档位键的加载、负价拒绝、`lengthTiers` 排序（256k<272k<512k 乱序输入）、`GetLengthTierPrice` 的 tier 优先 / fallback / 未配置返回 ok=false。
- `mod_ai_token_auth_test.go`：
  - 输入 30 万 token 的 gpt-5.5 按 272k 档价计（断言值与 `quota.CalcCostUnits` 逐项一致）；
  - 只配置 input 档键的模型，output 沿用基础价；
  - `CacheWriteTokens1h` 拆分：配 1h 价时两段分别计价；未配时全部按 5m、结果与改造前一致；
  - peak tier 下档位价正确覆盖。
- `llm_util` 测试：`usage.cache_creation.ephemeral_1h_input_tokens` 与 `cache_creation_input_tokens_1h` 兜底解析。
- `mod_access_pb3` 测试：`CacheWriteTokens1h > 0` 时 `reqLog.AiCacheWrite_1HTokens` 赋值。

### 7.2 集成测试

- `integration-test` SC05：Anthropic 风格响应带 `cache_creation.ephemeral_1h_input_tokens`，断言访问日志 `ai_cache_write_1h_tokens` 与 `ai_cost_value` 含 1h 段计价。
- SC19/SC25 回归：存量价格模型扣减金额与改造前逐分一致。

## 8. 兼容性与回滚

- **向后兼容**：未配置新键的模型，`GetLengthTierPrice` 返回 ok=false、1h 价为 0 时拆分不生效，计费结果与现状逐分一致；proto 字段为 optional，旧日志消费方无感。
- **回滚**：计费改动（6.3）可独立回滚，配置层新键被忽略即退回基础价计费；`bfe-access-pb` 依赖升级可单独回退。
- **升级路径**：后续新增档位（如 128k/1M）在两端枚举各加一对常量；档位超过 2-3 个时再考虑泛化为动态模式。`time_factors` 高峰倍率复用既有 `TierPrices["peak"]` 机制，数据面无需再改。

## 9. 参考文档

- `bfe-access-pb` CHANGELOG v0.3.6（`ai_cache_write_1h_tokens` field 788）
- `docs/zh_cn/sys_design/rmb_quota.md`（RMB 配额与计费口径）
