# BFE Issue #1420 修复方案：image_generation 按 token 计费规则不扣费

- Issue: https://github.com/bfenetworks/bfe/issues/1420
- 缺陷：控制面新增一条生图模型定价规则——模型 `gpt-image-2`，**模型模式 = image_generation，按 token 量计费**（默认价格仅含 token 单价键：`input_cost_per_token` ¥2.402e-5、`output_cost_per_token` ¥0.00014414、`cache_read_input_token_cost` ¥6.01e-6、`input_cost_per_image_token` ¥3.844e-5，**不含 `output_cost_per_image`**）。随后对 `/v1/images/generations` 发起请求，HTTP 200 且响应返回真实 token usage（`input_tokens=19`、`output_tokens=196`、`total_tokens=215`，`output_tokens_details.image_tokens=196`），但该 API Key（仅有 RMB 有限配额 ¥1000）余额不变，Redis `QUOTA_AI_product-96XY6kpPO6lGuvvlEoelyS4G` 维持 `100000000000` 未扣减
- 代码库：`bfe/`（已对照当前 HEAD 逐条核实，行号基于当前 HEAD）

## 一、根因

### 1.1 缺陷本体：计费公式由"请求 mode"硬编码选择，而非由"命中价格条目实际配置的价格键"选择

`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:992-1001` `calcCostUnits` 的派发：

```go
switch mode {
case bfe_basic.ModeImageGeneration:
    cost = calcImageGenerationCost(entry, usage, tierName)
case bfe_basic.ModeVideoGeneration:
    cost = calcVideoGenerationCost(entry, usage, tierName)
case bfe_basic.ModeResponses:
    cost = calcResponsesCost(entry, usage, tierName)
default:
    cost = calcChatCost(entry, usage, tierName)
}
```

`calcImageGenerationCost`（`:1028-1037`）**只**读取两个键：

```go
cost := quota.CalcCostUnits(imageCount, entry.GetPrice(tierName, cluster_conf.PriceOutputCostPerImage))
cost += quota.CalcCostUnits(usage.ImageInputTokens, entry.GetPrice(tierName, cluster_conf.PriceInputCostPerImageToken))
```

issue 场景代入：`output_cost_per_image` 未配置 → `GetPrice` 返回 0 → 第一项 0；响应 `input_tokens_details.image_tokens=0` → `ImageInputTokens=0` → 第二项 0 → **cost = 0**。条目上明明配置的 `input_cost_per_token` / `output_cost_per_token` / `cache_read_input_token_cost` 被完全忽略——**计费公式只看 mode，不看条目上配了什么价**。

`calcVideoGenerationCost`（`:1016-1026`）同构缺陷：只读 `output_cost_per_video`，token 计费的 video_generation 条目同样会算出 0（本次一并修复）。

### 1.2 全链路推演（issue 实证数字逐环）

1. **mode 识别**：`POST /v1/images/generations` → `DetectModeFromPath`（`bfe_basic/request_ai_basic.go:427-432`）命中 `openAIEndpointModes["/images/generations"] = ModeImageGeneration`（`bfe_basic/openai_endpoint.go:41`）→ `aiMeta.Mode = "image_generation"`。此环正确。
2. **auth 预置**：`SetTokenAuthContext`（`mod_ai_token_auth.go:836-838`）对 image_generation 预读请求体 `n`（未传 → 默认 1）→ `tusage.ImageCount = 1`。API Key 仅绑定 RMB 有限配额计划（¥1000，issue 截图实证）。
3. **usage 解析**：非流式 200 JSON → `tokenReadResponseHandler` → `UpdateCtxByUsage`（`:202-266`）。gpt-image 系 usage 使用 `input_tokens/output_tokens` 命名，主链（`prompt_tokens/completion_tokens`）读 0，由 Responses API 回退兜底（`bfe_model_protocol/utils/usage_parse.go:135-151`，门控 `usage.input_tokens_details` 存在）→ `PromptTokens=19`、`CompletionTokens=196`、`UsedQuota=215`、`CacheReadTokens=0`、`ImageInputTokens=0`；`ImageCount` 由响应 `data.#` 解析（≥1）。此环正确，`MarkFinalUsageSeen()=true`。
4. **计费**：finish handler（`:498-500`）调 `calcCostUnits` → `LookupModelPrice`（`cluster_conf_load.go:1649-1658`）按 `(gpt-image-2, image_generation)` **精确命中**新规则条目（`ModelTableCheck` `cluster_conf_load.go:1440-1554` 对价格键只校验非负、不要求任何键必须存在，token 计价条目合法加载）→ 派发进 `calcImageGenerationCost` → **cost = 0**（§1.1）。
5. **扣减**：RMB 分支以 `costUnits > 0` 为门槛（`:543-555`）→ 0 被跳过 → **Redis 余额不变**；该 Key 无 token 计划，其余配额键不存在。与 issue 两个截图（余额 ¥1000/已使用 ¥0、Redis `100000000000`）完全吻合。

### 1.3 为什么现有观测信号没有拦住

- 该请求并非"静默"绕过：`calcCostUnits` 在 cost==0 且存在可计费观测时置 `ZeroCostReasonPriceZero`（`:1002-1008`），billing checkpoint 计数 `BillingZeroCost` 并输出 warn `RMB plan bound but cost is 0 ... reason=price_zero`（`:521-541`），访问日志落 `ai_zero_cost_reason=price_zero`。
- 但 `price_zero` 语义是"条目在、每个计费维度单价为 0"，与真实意图（**该条目按 token 计费，token 单价 nonzero**）不符——信号把"公式选错"误报成"价格没配"。修复后该形态恢复计费，信号回归本义。

### 1.4 触发条件

控制面已支持给生图/生视频模型配"按 token 量计费"规则（价格键即证据），数据面对 `image_generation` / `video_generation` mode **只有按张/按次公式**，无 token 计费分支。凡 token 计费的生图/生视频规则上线即漏扣，与上游、客户端行为无关。

## 二、修复步骤

### 步骤 1（必做，治本）：计费方法改为"条目驱动"——image_generation / video_generation 无按次单价时回落 token 公式

文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

`calcImageGenerationCost`（`:1028-1037`）改造：

```go
// calcImageGenerationCost bills an image_generation entry. The billing
// method is entry-driven (issue #1420): an entry carrying a per-image
// price is billed per generated image; a token-priced entry (token unit
// prices but no output_cost_per_image) falls back to the token formula,
// so its RMB plan is actually charged. Mode alone must not pick the
// formula — the control plane already sells token-billed image rules.
func calcImageGenerationCost(entry *cluster_conf.ModelPrice, usage *bfe_basic.TokenUsage, tierName string) int64 {
	if !entry.HasPrice(tierName, cluster_conf.PriceOutputCostPerImage) {
		return calcChatCost(entry, usage, tierName)
	}

	imageCount := usage.ImageCount
	if imageCount < 0 {
		imageCount = 0
	}
	cost := quota.CalcCostUnits(imageCount, entry.GetPrice(tierName, cluster_conf.PriceOutputCostPerImage))
	cost += quota.CalcCostUnits(usage.ImageInputTokens, entry.GetPrice(tierName, cluster_conf.PriceInputCostPerImageToken))
	return cost
}
```

`calcVideoGenerationCost`（`:1016-1026`）对称改造（`PriceOutputCostPerVideo` 未配置 → `calcChatCost`）。

要点：

- **判别信号 = 价格键存在性**，不是 mode：token 计价条目的价格键里天然没有 `output_cost_per_image` / `output_cost_per_video`，无需控制面改配置格式。
- **`calcChatCost` 零改动复用**：token 公式已支持 input/output/cache（含 1h）/audio/image-input 分项、长度阶梯（length tier）与 peak 分时价（`GetPrice` 本就走 tier 优先回落），token 计价的生图规则自动全额受益；`input_cost_per_image_token` 在 token 公式内由既有的 image-aware 拆分（`calcChatCost:1109-1121`）继续生效。
- **显式配置 0 不切换公式**：`output_cost_per_image` 配成 0（键存在）仍走按张公式、计 ¥0——故意的免费配置不被改写；`HasPrice` 判键存在（见步骤 2），不判 >0。
- **混合条目（按张与 token 键同在）按张优先**：按次键是更显式的计费方法声明，保持与"按次计费"产品语义一致；该取舍写入文档与本条目注释。
- 流式生图（gpt-image-1 `partial_images` SSE / `response.completed` usage）与非流式共用 finish 时 `calcCostUnits`，本修复天然覆盖两条链路，无需改 `mod_body_process`。

### 步骤 2（配套）：`cluster_conf` 导出价格键存在性查询

文件：`bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`

`lookupPrice`（`:1594-1604`）已有 (值, 是否存在) 双返回值，仅缺导出口。新增：

```go
// HasPrice reports whether the given price key is configured for the
// tier (tier price map or default Prices), regardless of its value.
// It is the billing-method discriminator for per-unit vs token pricing
// (issue #1420): a per-image/per-video key present means unit billing.
func (p *ModelPrice) HasPrice(tier, key string) bool {
	_, ok := p.lookupPrice(tier, key)
	return ok
}
```

### 步骤 3（验证项）：观测信号回归核对

- `BillingZeroCost` / `ai_zero_cost_reason=price_zero` / warn `RMB plan bound but cost is 0`：修复后，token 计价生图条目带真实 usage 应恢复非零扣费；仍命中 `price_zero` 的应只剩"条目存在但所有维度单价未配"的真配置缺口。上线后核对该 counter 回落，并为其配告警（issue 暴露前该信号已存在但无人盯）。
- `PriceLookupMiss` / `PriceFallbackUsed`：本缺陷不经过这两条路径（条目精确命中），无需改动；回归确认计数无异常波动。

### 步骤 4（不改动项，明确边界）

- `UpdateCtxByUsage` / `QuotaUsageProcessor` 的 `UsedQuota` 语义：响应带 `total_tokens=215` 时 token 计划扣 215，对 token 计费是正确语义；按张计费条目扣 `image_count` 的现状不变（`mod_ai_token_auth.go:240-249`、`content_quota_usage.go:56-67`）。
- `ModelTableCheck` 不新增"image_generation 必须配 `output_cost_per_image`"之类约束——按 token 计费是合法产品形态，约束会反过来封死本修复支持的用法。
- 访问日志/上报字段无变化（`ai_mode`、`ai_image_count`、`ai_zero_cost_reason` 均已存在）。

## 三、回归测试

### 3.1 单元测试（`bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go`，沿用 `testing` + `testify`）

1. `TestCalcCostUnits_ImageGenerationTokenPriced`（新增，复刻 issue）：条目仅含 `input_cost_per_token=2.402e-5`、`output_cost_per_token=1.4414e-4`、`cache_read_input_token_cost=6.01e-6`、`input_cost_per_image_token=3.844e-5`（无 `output_cost_per_image`），usage = issue 实证值（prompt=19 / completion=196 / imageInput=0 / imageCount=1）→ 断言 `UsedCost = 19×2402 + 196×14414 = 2870782`（定点 1e-8 ¥，分项各自 `CalcCostUnits` 后求和），非 0。
2. `TestCalcCostUnits_ImageGenerationPerImageUnchanged`（回归）：条目含 `output_cost_per_image`，usage imageCount=2 → 仍按 `2 × output_cost_per_image + imageInput × input_cost_per_image_token` 计，与修复前一致；同条目再叠加 token 键时按张优先。
3. `TestCalcCostUnits_ImageGenerationEmptyEntry`：条目两个键都无、usage 有 token → cost=0 且 `ZeroCostReasonPriceZero` 被置（观测信号语义不变）。
4. `TestCalcCostUnits_VideoGenerationTokenPriced`：video 对称用例（无 `output_cost_per_video` → token 公式）。
5. `TestCalcCostUnits_ImageGenerationExplicitZeroPerImage`：`output_cost_per_image=0` 键存在 → 走按张公式计 0（不回落 token），锁定"存在性判别"语义。

### 3.2 配置层测试（`bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load_test.go`）

- `HasPrice`：default Prices 命中、tier 优先、键不存在为 false。
- `ModelTableCheck` 对"无 `output_cost_per_image` 的 image_generation 条目"继续放行（现状兼容回归）。

### 3.3 集成测试（`bfe/tests/integration`，扩展 SC03）

新增 TC（非流式 + 可选流式各一）：

- 后端 mock 返回 issue 实证响应体（`/v1/images/generations`，usage `input_tokens=19/output_tokens=196/total_tokens=215`，`data` 数组 1 张）；
- ModelTable 配 `(gpt-image-2, image_generation)` token 计价条目（价格键同 issue）；
- 断言：Redis RMB 配额从 `100000000000` 扣至 `99999712928`（`2870782` 定点）；访问日志 `ai_zero_cost_reason` 为空、`ai_usage_source=parsed`、`ai_cost_value=2870782`；
- 回归：SC03 既有 TC-12（按张计费）数值不变；TC-23（mode 无条目回落 chat 价，issue #1408 语义）不变——本修复与 #1408 互补：#1408 解决"条目缺失"，本修复解决"条目命中但公式选错"。

验证命令：`cd bfe && make test`。

## 四、修复后正确行为（目标链路，issue 数字代入）

1. `/v1/images/generations` + `gpt-image-2` → mode=image_generation，条目 `(gpt-image-2, image_generation)` 精确命中；
2. 条目无 `output_cost_per_image` → `calcImageGenerationCost` 回落 `calcChatCost`：
   - 输入：`19 × 2.402e-5 = ¥0.00045638`（cacheRead=0、imageInput=0，正常输入不拆分）；
   - 输出：`196 × 1.4414e-4 = ¥0.02825144`（`output_tokens_details.image_tokens=196` 按普通 output token 计——条目未配 output 侧 image 单价键，正是 `output_cost_per_token` 的语义）；
   - 合计 `UsedCost = 2,870,782` 定点单位（≈ ¥0.0287）；
3. finish 扣减：RMB 计划 `Deduct(2870782)` → Redis `QUOTA_AI_product-...` = `99999712928`，控制台余额/已使用随之更新；
4. 若 Key 另有 total_token 计划：`UsedQuota=215`（响应 `total_tokens`）照常扣减——token 计费下语义正确；
5. 按张计费的既有生图模型（配 `output_cost_per_image`）行为逐字节不变。

## 五、替代方案与取舍

| 方案 | 结论 |
| --- | --- |
| **A. 条目驱动（本方案）**：按 `output_cost_per_image`/`output_cost_per_video` 键存在性选择公式 | 采纳。纯数据面改动，价格键已携带计费方法信息，控制面/配置格式零改动 |
| B. 控制面显式 `billing_method` 字段下推 | 暂不采纳。语义最显式，但需 ai-gateway-api、配置 schema、BFE 三处联动；价格键天然互斥（按次键 vs token 键），A 已足够。可作为后续硬化：控制面校验"计费方法 × 价格键"一致性，并在定价详情页展示数据面实际生效的计费方法 |
| C. `calcImageGenerationCost` 内叠加 token 费（按张 + token 同时计） | 拒绝。混合条目产生不可预期的双计费，违背"一条规则一种计费方法"的产品语义 |

## 六、影响范围与兼容性

| 模块/文件 | 影响 |
| --- | --- |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | `calcImageGenerationCost` / `calcVideoGenerationCost` 增加"无按次单价 → `calcChatCost`"回落；仅新增分支，既有按张/按次路径原样 |
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | 新增 `ModelPrice.HasPrice` 导出方法（`lookupPrice` 的存在性包装） |
| 控制面（ai-gateway-api）/ 配置格式 / Redis key 结构 | 无改动、无迁移 |
| 既有按张计费生图、按次计费生视频 | 行为不变（按次键存在 → 原公式） |
| 既有 chat / responses / audio / cache / 阶梯计费 | 无影响（不经过改动路径） |

风险与缓解：

| 风险 | 缓解 |
| --- | --- |
| 存量 token 计价生图条目上线后补扣引发对账差异 | 该形态此前一律漏扣（收入损失方向），修复只会开始计费；上线前用 `BillingZeroCost`/`ai_zero_cost_reason=price_zero` 历史量评估影响面 |
| 部署混合构建导致修复不生效 | 参考 issue #1401 运营遗留：全量干净构建，勿选择性挑 hunk；发布后跑一条 gpt-image 请求核对 Redis 扣减 |

## 七、运营遗留事项（代码外）

- **告警落地**：为 `mod_ai_token_auth` 的 `BillingZeroCost`（及 `UsageUnrecognized`、`DeductFailed`）配监控告警——issue 场景中信号其实已产生，缺的是盯守。
- **漏扣窗口对账**：token 计价生图/生视频规则的漏扣窗口，可基于 pb3 访问日志（`ai_mode=image_generation` 且 `ai_cost_value=0` 且 `ai_usage_source=parsed`）回放评估是否回补。
- **控制面产品化**：定价规则已允许"image_generation + 按 token 计费"，建议定价详情页明示数据面计费方法解析结果（按张/按 token），避免运营误判。

## 八、实施记录（2026-10-13）

已按上述方案实施，变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | 新增 `ModelPrice.HasPrice`（issue #1420）：`lookupPrice` 的存在性包装，计费方法判别信号（键存在与否，与值大小无关） |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | `calcImageGenerationCost` / `calcVideoGenerationCost` 增加 entry-driven 回落（issue #1420）：无 `output_cost_per_image` / `output_cost_per_video` 键时回落 `calcChatCost` token 公式；按次键存在（含显式 0）维持原按张/按次公式 |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` | 新增 4 用例：token 计价生图条目按 token 公式计 `2870782`（issue 实证数字）、混合条目按张优先、显式 0 按张键不回落、video 对称回落 |
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load_test.go` | 新增 `TestHasPrice`：default/tier 命中、显式 0 价存在性、tier 回落 |
| `docs/zh_cn/sys_design/rmb_quota.md` | 图像/视频生成计费公式节改为 entry-driven 描述（含混合条目按张优先、显式 0 语义），测试覆盖描述同步 |
| `tests/integration/测试设计文档/scenario-SC03-RMB配额扣减/` | 新增 TC-27（issue 精确复刻）/ TC-28（判别语义三臂）设计文档，`场景说明.md` 背景与测试例列表同步 |
| `tests/integration/测试设计文档/测试场景总体说明.md` | SC03 清单（TC 数 26→28、依赖新增 issue #1420）与 TC 对应表同步 |
| `tests/integration/implementation/scenario-SC03-rmb-quota/sc03_rmb_quota_test.go` | 新增 `tokenPricedImageGenerationAIConf` / `billingMethodAIConf` 与 `TestTC27_*` / `TestTC28_*` |

验证结果（2026-10-13）：

- 单元测试：`go test $(go list ./... | grep -v tests/integration)` 全量通过（exit 0）；`go vet ./...` 全仓通过（exit 0）；
- 集成测试：SC03 全量（含新增 TC-27/TC-28）74s 通过、SC05 全量 60s 通过；TC-28 臂 2（显式 0 按张键）按预期产生 `RMB plan bound but cost is 0 ... reason=price_zero` 信号，观测语义不变。
