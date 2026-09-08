# BFE RMB 计费价格精度提升（浮点价格 + 结果取整）

## 1. 背景

控制面 `ai-gateway-api` 导入的模型目录（`model-list.yaml`，450 个模型）中，大量模型价格是 `unit_price_usd_per_1M × group_ratio / 1e6 × 6.8` 的计算结果，需要 **10~12 位小数** 才能无损表示，例如：

| 模型 | 价格字段 | 值 |
|------|---------|-----|
| `qwen2.5-omni-7b` | `output_cost_per_token` | `7.6234102728e-08`（12 位小数） |
| `Qwen/Qwen2.5-72B-Instruct` | `input_cost_per_token` | `4.141631732e-06`（11 位小数） |
| `glm-4.6` | `output_cost_per_token` | `1.4049457764e-05`（11 位小数） |
| `mimo-v2-flash` | `cache_read_input_token_cost` | `7.0197148e-08`（11 位小数） |

当前 BFE 的做法是：配置加载阶段通过 `go-lib/quota.RmbToFixedPoint` 把浮点价格转换为 **1e-8 元定点整数**（`cluster_conf_load.go:1412-1449`），运行时 `calcChatCost` 等只做整数运算。对超精度价格这会引入系统性截断误差，以上述 `7.6234102728e-08` 为例：

- 真实值 × 1e8 = `7.6234102728`（定点单位），截断为 int64 后为 `7`；
- 相对误差约 **8.1%**，100 万 token 输出少收约 0.0062 元。

同时 `ai-gateway-api` 的接口定义（`model-prices.md`）此前强制价格使用十进制表示法、禁止科学计数法，已不满足目录数据的表示需求。

总体方案：

1. 表示层放开科学计数法（ai-gateway-api 侧，另文描述）；
2. **BFE 价格不再预转定点整数**：加载后保持 `float64`（元/token），运行时先乘 1e8 做浮点计算，每项计费结果四舍五入为定点整数后累加，用于 Redis 扣减。

`go-lib` 侧已完成配套修改（commit `bfc855f`）：新增 `quota.CalcCostUnits(usage int64, priceYuan float64) int64`，`RmbToFixedPoint` 由截断改为四舍五入。本文描述 BFE 侧的修改。

---

## 2. 需求示例

`cluster_conf.data` 中将出现科学计数法表示的价格（JSON 语法原生支持，`encoding/json` 可直接解析为 float64）：

```json
{
    "AIConf": {
        "Provider": "example-provider",
        "ModelTable": {
            "Currency": "RMB",
            "Models": [
                {
                    "Provider": "example-provider",
                    "Model": "qwen2.5-omni-7b",
                    "BaseModel": "qwen2.5-omni-7b",
                    "Mode": "chat",
                    "Prices": {
                        "input_cost_per_token": 6.0168984e-09,
                        "output_cost_per_token": 7.6234102728e-08
                    }
                }
            ]
        }
    }
}
```

输出 100 万 token 的期望成本：

```
cost = round(1000000 × (7.6234102728e-08 × 1e8))
     = round(7623410.2728)
     = 7623410        （定点单位，即 0.07623410 元）
```

改造前同一请求只扣减 `7 × 1000000 = 7000000`（0.07 元），少收 8.1%。

---

## 3. 当前现状

| 层级 | 当前实现 | 不足 |
|------|---------|------|
| 配置加载 | `cluster_conf_load.go:1412-1449`：对 9 个价格键逐个 `quota.RmbToFixedPoint` 预转定点整数，存入 `pricesInt` / `tierPricesInt`；`TierPrices` 同样转换（`:1423-1440`） | 超 8 位小数的价格在加载期即被截断，精度不可逆地丢失 |
| 价格常量 | `cluster_conf_load.go:313-321` 定义 `Price*Int = "*_int"` 常量；`:325-336` `priceKeyToIntKey` 映射表 | 仅为定点转换服务 |
| 价格取值 | `GetPriceInt(tier, key) int64`（`:1482-1494`）：tier 价优先、fallback 默认价 | 只能返回定点整数 |
| 成本计算 | `mod_ai_token_auth.go`：`calcVideoGenerationCost`（`:574`）、`calcImageGenerationCost`（`:589-590`）、`calcChatCost`（`:600-641`）全部通过 `GetPriceInt` 取定点整数后做 int64 乘法 | 继承加载期的截断误差 |
| Redis 扣减 | 单 Key 定点数 Lua（rmb_quota.md 8.1），`UsedCost` 为 1e-8 元 int64 | 无需改动 |

---

## 4. 变更目标

1. `ModelTable` / `ModelPrice` 加载后价格保持 `float64`，删除 `pricesInt` / `tierPricesInt` 预转换及配套常量、映射表；
2. `GetPriceInt(tier, key) int64` 改为 `GetPrice(tier, key) float64`，tier 命中 / fallback 语义不变；
3. `calcChatCost` / `calcImageGenerationCost` / `calcVideoGenerationCost` / `calcResponsesCost` 改为**逐项** `quota.CalcCostUnits(用量, 浮点价格)` 取整后整数累加；
4. `TokenUsage.UsedCost`、`QuotaPlan`、Redis Lua 扣减、tier 时段匹配、cache/audio/image 子项拆分与 clamp 逻辑**全部不变**；
5. ≤8 位小数的存量价格，扣减金额与改造前**逐分一致**（由 go-lib 一致性测试保证）。

---

## 5. 变更总览

```
cluster_conf.data (浮点价格, 科学计数法或十进制)
        │
        ▼
cluster_conf_load.go          改造前: RmbToFixedPoint 预转 int → pricesInt
  ModelTableCheck /              改造后: 仅校验非负, 价格保持 float64
  buildModelTableIndex
        │
        ▼
mod_ai_token_auth.calcChatCost  改造前: int64 价格 × int64 token 数
  / calcImageGenerationCost      改造后: 逐项 quota.CalcCostUnits(token数, 价格)
  / calcVideoGenerationCost              = round(token数 × (价格 × 1e8)), int64 累加
        │
        ▼
TokenUsage.UsedCost (int64, 1e-8 元) ──► Redis Lua DECRBY   （不变）
```

---

## 6. 详细设计

### 6.1 `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`

**删除**：

- `ModelPrice` 的 `pricesInt map[string]int64` / `tierPricesInt map[string]map[string]int64` 字段（`:261-263`）；
- `PriceInputCostPerTokenInt` 等 9 个 `*_int` 常量（`:313-321`）；
- `priceKeyToIntKey` 映射表（`:325-336`）。

**加载逻辑**（`:1387-1449` 循环内）：

- 非负校验保留，对象从"定点转换结果"改为原始浮点值（现有 `input < 0 || ...` 检查逻辑不变）；
- 删除 `pricesInt[...] = quota.RmbToFixedPoint(...)` 与 `tierPricesInt` 转换块；`TierPrices` 的 `peak` tier name 校验与 tier 价格非负校验保留；
- `priceIndex` 构建不变。

**取值方法**：

```go
// GetPrice returns the float64 price (yuan per unit) for the given tier and key.
// If tier is empty or the tier/key is not configured, it falls back to default Prices.
// Prices are kept as float64; conversion to fixed-point integers happens per
// billing item at request time via quota.CalcCostUnits.
func (p *ModelPrice) GetPrice(tier, key string) float64 {
    if tier != "" && p.TierPrices != nil {
        if tierMap, ok := p.TierPrices[tier]; ok {
            if v, ok := tierMap[key]; ok {
                return v
            }
        }
    }
    return p.Prices[key]
}
```

说明：

- tier 命中优先、`TierPrices[tier]` 未配置该键时 fallback 默认 `Prices` 的语义与 `GetPriceInt` 完全一致；
- 未配置的键返回 0（map 零值），该计费项按 0 成本处理，与现状一致；
- 加载期已拒绝负价格，运行时无需再做 `< 0` 守卫。

### 6.2 `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

**`calcChatCost`**：定点整数价格变量改为浮点价格，逐项换算后整数累加：

```go
func calcChatCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
    // ... 原有 cache/audio/image 子项拆分与 clamp 逻辑完全不变,
    // 得到 normalInput / cacheReadTokens / cacheWriteTokens /
    //     audioInputTokens / imageInputTokens /
    //     normalOutput / audioOutputTokens ...

    // 逐项: round(用量 × (价格 × 1e8)), 整数累加避免浮点累加误差
    cost := quota.CalcCostUnits(normalInput, entry.GetPrice(tierName, cluster_conf.PriceInputCostPerToken))
    cost += quota.CalcCostUnits(cacheReadTokens, entry.GetPrice(tierName, cluster_conf.PriceCacheReadInputTokenCost))
    cost += quota.CalcCostUnits(cacheWriteTokens, entry.GetPrice(tierName, cluster_conf.PriceCacheCreationInputTokenCost))
    cost += quota.CalcCostUnits(audioInputTokens, entry.GetPrice(tierName, cluster_conf.PriceInputCostPerAudioToken))
    cost += quota.CalcCostUnits(imageInputTokens, entry.GetPrice(tierName, cluster_conf.PriceInputCostPerImageToken))
    cost += quota.CalcCostUnits(normalOutput, entry.GetPrice(tierName, cluster_conf.PriceOutputCostPerToken))
    cost += quota.CalcCostUnits(audioOutputTokens, entry.GetPrice(tierName, cluster_conf.PriceOutputCostPerAudioToken))
    return cost
}
```

要点：

- 原实现中"`cacheReadCost > 0 || ...` 才拆分、否则 fallback 全量按普通价计费"的分支判断**不再需要**——未配置子项价格时 `GetPrice` 返回 0，该项自然贡献 0 成本，等价于现状的回退语义（原有 `if` 分支可删除，行为不变）；
- `calcResponsesCost` 复用 `calcChatCost`，无独立改动。

**`calcImageGenerationCost` / `calcVideoGenerationCost`**：

```go
func calcImageGenerationCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
    cost := quota.CalcCostUnits(usage.ImageCount, entry.GetPrice(tierName, cluster_conf.PriceOutputCostPerImage))
    cost += quota.CalcCostUnits(usage.ImageInputTokens, entry.GetPrice(tierName, cluster_conf.PriceInputCostPerImageToken))
    return cost
}

func calcVideoGenerationCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
    return quota.CalcCostUnits(usage.VideoCount, entry.GetPrice(tierName, cluster_conf.PriceOutputCostPerVideo))
}
```

`ImageCount` / `VideoCount` 的 `< 0` clamp 保留。

### 6.3 依赖变更：`go-lib/quota`

BFE `go.mod` 当前依赖 `github.com/bfenetworks/go-lib v0.0.2`（无 replace）。本次需要 `CalcCostUnits`，go-lib 已发布 `v0.0.4`（包含 `bfc855f`："quota: add CalcCostUnits and round RmbToFixedPoint"）：

1. BFE `go.mod` 升级 `github.com/bfenetworks/go-lib v0.0.2 → v0.0.4`，`go mod tidy`。

`RmbToFixedPoint` 改舍入对 BFE 的影响：仅作用于配额初始化/重置（`ToRedisValue` 路径），配额额度为元级数值，实际无行为差异。

### 6.4 不变的部分

| 项 | 说明 |
|----|------|
| `TokenUsage.UsedCost` / `UsedQuota` | 仍为 int64；`UsedCost` 单位 1e-8 元 |
| `QuotaPlan` / `HasBalance` / `Deduct` | 语义不变 |
| Redis Lua 扣减脚本 | 单 Key 定点数方案不变 |
| `ActiveTierName` / `LookupModelPrice` | 时段匹配与 (model, mode) 索引不变 |
| usage 解析（流式/非流式、DeepSeek/Responses 字段兜底） | 不变 |
| 计费判定规则（issue #1352 系列） | 不变 |
| conf-agent | 零改动，继续透传浮点价格 |

---

## 7. 计费公式速查

chat 模式拆分公式（业务语义）与现状完全相同，仅"价格 → 定点整数"的换算时点从加载期推迟到每次计费项计算时：

```
cost = Σ round(分项用量_i × (分项价格_i × 1e8))
```

| 场景 | 改造前（加载期截断） | 改造后（运行时取整） |
|------|--------------------|--------------------|
| `output_cost_per_token = 7.6234102728e-08`，输出 100 万 token | 0.07 元（7,000,000 定点单位，-8.1%） | 0.076234 元（7,623,410 定点单位，误差 < 1e-8 元） |
| `input_cost_per_token = 0.000002`，输入 1000 token | 0.0002 元（200,000 定点单位） | 完全一致 |
| tier `peak` 命中且某键未配置 | fallback 默认价（定点整数） | fallback 默认价（浮点），逐项取整 |

---

## 8. 边界情况与兼容性

1. **≤8 位小数价格逐分一致**：价格 ×1e8 恰为整数，`CalcCostUnits(usage, price) == usage × RmbToFixedPoint(price)`（go-lib `TestCalcCostUnitsConsistentWithFixedPoint` 保证）；存量 `cluster_conf.data` 无需迁移。
2. **9 位小数价格略有差异**：模型目录中存在 `0.000155584` 这类 9 位小数价格（×1e8 = 15558.4 非整数），新方案按 15558400 定点单位/千 token 扣减，旧方案按 15558000，新方案更接近真实成本（差约 2.6e-6 元/千 token）。
3. **float64 精度上限**：有效数字约 15~16 位，模型目录中的 12 位有效数字价格可无损表示；单次请求成本上界（0.01 元/token × 100 万 token ≈ 1e12 定点单位）远小于 2^53 ≈ 9e15，无溢出风险。
4. **未配置价格键**：`GetPrice` 返回 0，该分项按 0 成本处理（未配置 `output_cost_per_video` 等时行为与现状一致）。
5. **负价格**：仍在配置加载阶段拒绝（`ModelTableCheck` 报错），运行时不做守卫。
6. **Redis 余额上限**：`MaxRMBQuota`（9000 万元）与 Lua 精度约束不受影响。

---

## 9. 测试计划

### 9.1 单元测试

- `cluster_conf_load_test.go`：
  - `GetPriceInt` 相关用例（`:147-213`、`:574-610`）改为 `GetPrice`，断言值由定点整数改为浮点原值（如 `100` → `0.000001`）；
  - 新增：科学计数法价格（`7.6234102728e-08`）加载后 `GetPrice` 返回原值；`TierPrices.peak` 命中 / fallback 语义回归。
- `mod_ai_token_auth_test.go`：
  - 现有 `TestCalcCostUnits_*` / `TestCalcChatCost_*` 系列**期望值无需修改**——它们经 `ModelTableCheck` 构建 entry 且价格均为 ≤8 位小数，一致性保证使期望值天然成立；
  - 新增：12 位小数价格的 chat / image / video 计费用例，断言与手算 `round(用量 × 价格 × 1e8)` 一致（误差 0）；
  - 新增：tier 价格超精度场景（`TierPrices.peak` 配 10 位小数）。
- go-lib 侧测试已随 `bfc855f` 提交（`TestCalcCostUnits` / `TestCalcCostUnitsConsistentWithFixedPoint` / `TestRmbToFixedPointRounding`）。

### 9.2 集成测试

- 用 `model-list.yaml` 全量导入控制面 → InnerAPI 导出 → BFE 加载 → 发起 chat / image_generation 请求，核对 Redis 扣减金额与手算成本一致；
- 流式（SSE）、cache 子项、视频按次、分时段（peak 命中/未命中）场景回归；
- 存量 v0.4/v0.5 十进制 `cluster_conf.data` 回归：扣减金额与旧版本逐分一致。

### 9.3 配置加载测试

- `bfe -t test_conf` 校验含科学计数法价格的 `cluster_conf.data` 可正常加载（`encoding/json` 原生支持，无需改动解析代码）。

---

## 10. 实施步骤建议

1. BFE `go.mod` 升级 go-lib 依赖至 `v0.0.4`（已发布，含 `bfc855f`），`go mod tidy`；
2. 修改 `cluster_conf_load.go`（6.1），同步更新 `cluster_conf_load_test.go`；
3. 修改 `mod_ai_token_auth.go`（6.2），新增超精度计费用例；
4. `make test` 全量回归；
5. 更新 `docs/zh_cn/sys_design/rmb_quota.md`：4.4 节（去掉定点预转换描述）、5 节（`go-lib/quota` 新增 `CalcCostUnits`）、7.6 节（`calcChatCost` 等改浮点）、11 节兼容性说明。

---

## 11. 影响范围

| 文件 | 变更类型 |
|------|---------|
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | 删除定点预转换与 `*_int` 常量；`GetPriceInt` → `GetPrice` |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | `calcChatCost` / `calcImageGenerationCost` / `calcVideoGenerationCost` 改逐项 `quota.CalcCostUnits` |
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load_test.go` | `GetPriceInt` 用例改造 |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` | 新增超精度用例（现有期望值不变） |
| `go.mod` / `go.sum` | go-lib `v0.0.2 → v0.0.4` |
| `docs/zh_cn/sys_design/rmb_quota.md` | 文档同步 |

无新增配置项；`cluster_conf.data` 格式不变（科学计数法本就是合法 JSON 数字字面量）。

---

## 12. 兼容性与风险

### 12.1 兼容性

- 存量 ≤8 位小数十进制配置：扣减金额逐分一致，无需迁移；
- Redis 存量余额、Lua 脚本、`QuotaPlan` 语义：完全不变；
- conf-agent、ai-gateway-api 导出链路：零改动（本变更仅消费方 BFE 调整）；
- 9 位小数价格扣减略有上升（更准），幅度约 2.6e-6 元/千 token，可忽略。

### 12.2 风险与缓解

| 风险 | 缓解 |
|------|------|
| 浮点累加误差 | 逐项 `CalcCostUnits` 先取整为 int64 再累加，浮点只参与单項乘法（量级 < 2^53，精确） |
| 未配置子项价格回退语义变化 | 删除的 `if cacheReadCost > 0 ...` 分支与"价格为 0 贡献 0 成本"数学等价，由现有 `TestCalcCostUnits_CacheFallback` / `AudioFallback` 用例回归保证 |
| `GetPriceInt` 被外部引用 | 全库检索确认仅 `mod_ai_token_auth` 与两处 `_test.go` 使用，无跨模块影响 |
| go-lib 版本漂移 | 使用已发布的正式版本 `v0.0.4` 并升级 `go.mod`，不使用 replace 本地路径 |

---

## 13. 参考资料

- `docs/zh_cn/sys_design/rmb_quota.md`（RMB 配额设计，本次需同步更新）
- go-lib commit `bfc855f`：`quota: add CalcCostUnits and round RmbToFixedPoint`
- `ai-gateway-api/design-docs/api-define/OpenAPI接口定义/model-prices.md`（控制面接口定义，配套放开科学计数法）
