# BFE Issue #1498 修复方案：无 usage 的按次计费响应须按 data.# 回退计费

- Issue: https://github.com/bfenetworks/bfe/issues/1498
- 缺陷：`POST /v1/video/generations` 响应**完全省略 `usage` 对象**、请求 `n=3` 而响应 `data` 仅 2 项时，BFE 未按响应 `data.#=2` 回退计费。`EstimateToken=true` 时按鉴权阶段预置的请求 `n=3` 计费（`150000000` 定点单位，多扣 1 个视频单价）；`EstimateToken=false` 时整笔按 unrecognized 零扣。两者都不是条款规定的 `data.#=2 × 50000000 = 100000000`。确定性复现（2026-10-14 与 2026-11-09 多次 run 指纹一致）。
- 代码库：`bfe/`（行号基于当前 HEAD `a7048113`，issue #1497 提交之后，逐条核实）
- 关联：#1414（usage presence 判定）、#1364（跨协议解析回退链）、#1420（image generation 计费修复，已批准 `data.#` 回退条款的驱动 issue）、#1352（final usage 只来自终止事件）。**不需要产品裁定**：`#video-generations` 条款已明确规定 `usage.video_count` 缺失时按响应 `data.#` 回退，probe C 的固定 oracle 就是 `100000000`。
- 影响面：同一代码路径覆盖 image generation（`data.#` 回退同属 #1420 批准条款），本次一并修复。

## 一、根因

### 1.1 鉴权阶段用请求 `n` 预置 `VideoCount`（设计如此，本身不是缺陷）

`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:1026-1031`（`SetTokenAuthContext`）：

```go
if aiBasicInfo.Mode == bfe_basic.ModeImageGeneration {
    tusage.ImageCount = GetImageCountFromReq(req)
}
if aiBasicInfo.Mode == bfe_basic.ModeVideoGeneration {
    tusage.VideoCount = GetVideoCountFromReq(req)
}
```

`GetVideoCountFromReq`（`:1003-1015`）读请求体 `n`，缺省/非法返回 1。probe C 的 `n=3` 在此预置为 3——**目的**是"响应不带计数时也不漏计"（宁多收预备，响应观测到位后应被覆盖），与流式累加器的 keep-preset 语义（`UpdateCtxByUsage` `:270-275` 注释）一致。

### 1.2 解析层的 `data.#` 回退已实现，但同函数把 `UsagePresent` 置 false

`bfe_model_protocol/utils/usage_parse.go` 的 `parseOpenAIUsageFieldsWithPrefix`（chat completions 主链）：

```go
fields.UsagePresent = gjson.GetBytes(data, prefix).IsObject()   // :75 —— 无 usage 对象 ⇒ false
...
fields.VideoCount = gjson.GetBytes(data, prefix+".video_count").Int()
if fields.VideoCount == 0 {
    fields.VideoCount = gjson.GetBytes(data, "data.#").Int()    // :96-98 —— data.# 回退已实现
}
```

probe C 的解析结果：`VideoCount=2`（`data.#` 回退生效）、`UsagePresent=false`（没有 usage 对象）。**回退解析一直都在，死在下一步的采纳上。**

### 1.3 `UpdateCtxByUsage` 的 presence 早退丢弃了已解析的计数

`mod_ai_token_auth.go:250-261`：

```go
fields := modelprotocol.Get(ctx.aiBasicInfo.AuthStyle).ExtractUsageFields(data)
if !fields.UsagePresent || (fields.UsedQuota == 0 && ... 全零) {
    // 单适配器未找到 usage → 跨协议组合链再解析（issue #1364）
    fields = utils.ParseUsageFieldsCrossProtocol(data)
}
if !fields.UsagePresent {
    return false   // ← probe C 死在这里：VideoCount=2 未写回 tokenUsage
}
```

跨协议链对 probe C 重解析结果相同（`VideoCount=2`、`UsagePresent=false`），`:260-261` 直接 `return false`，已解析的 `VideoCount=2` 被丢弃，共享 `TokenUsage.VideoCount` 保持鉴权预置的 3。

### 1.4 计费检查点按 `EstimateToken` 分流成两种错误形态

调用方 `tokenReadResponseHandler`（`:364-376`）：`usagePresent=false` ⇒ 不置 `finalUsageSeen`；随后——

- **`EstimateToken=true`**：走估算臂（`:372-376`），只改写 `CompletionTokens`/`UsedQuota`，`VideoCount=3` 预置值幸存到计费副本；`calcCostUnits` 对 video mode 走 `calcVideoGenerationCost`（`:1213`）按 `VideoCount × output_cost_per_video` 计 ⇒ **3 × 50000000 = 150000000（多扣）**。
- **`EstimateToken=false`**：`estimateBillable=false` ⇒ `unrecognized=true`（`:502`），计费副本全字段清零（含 `VideoCount=0`）⇒ **零扣（漏计费）**。

两条路径都没有用到 `data.#=2`。**`EstimateToken` 不是根因，只是决定错成哪种形态**（issue 的对照 run 已证明：关闭开关把多扣变成零扣，仍未按 `data.#` 回退）。

## 二、修复设计

设计原则：**响应结构解析出的非零生成计数是"上游实际产出了什么"的真实观测，属于已批准条款的计费依据，不是估算**——必须采纳为计费基数并视同 recognized usage，使计费结果不再依赖 `EstimateToken` 开关。

### 步骤 1（必改）：`UpdateCtxByUsage` 在 presence 早退前采纳按次计数

文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`，`:260-261` 的早退改为：

```go
	// issue #1498: a per-count generation response (image / video generations)
	// may omit the usage object entirely. The generated count parsed from the
	// response structure (the approved data.# fallback, #1420 clause) is a
	// real observation of what the upstream produced and must override the
	// request-preset count: adopted as recognized usage it bills data.# under
	// BOTH EstimateToken positions (previously: on billed the request n,
	// off billed nothing). Restricted to per-count modes so a chat-shaped
	// body that merely carries a data array keeps the issue #1414
	// usage-object semantics.
	countObserved := !fields.UsagePresent &&
		((ctx.aiBasicInfo.Mode == bfe_basic.ModeImageGeneration && fields.ImageCount > 0) ||
			(ctx.aiBasicInfo.Mode == bfe_basic.ModeVideoGeneration && fields.VideoCount > 0))
	if !fields.UsagePresent && !countObserved {
		return false
	}
```

countObserved 命中后先做**兄弟计数清零**（实现时发现：解析层的 `data.#` 回退会同时填 `ImageCount` 与 `VideoCount` 两个字段——`parseOpenAIUsageFieldsWithPrefix` 的 image/video 回退各自独立触发；不清零会让视频响应带上一个假的 `ai_image_count`，且 `UsedQuota` 推导 switch 以 `ImageCount` 优先而取错配额）：

```go
	if countObserved {
		// The data.# fallback filled both generated-count fields; keep only
		// the one matching the request mode so the per-count billing and the
		// access log do not record a spurious sibling count.
		if ctx.aiBasicInfo.Mode == bfe_basic.ModeImageGeneration {
			fields.VideoCount = 0
		} else {
			fields.ImageCount = 0
		}
	}
```

早退放行后，函数体既有逻辑**原样复用、仅需一处 mode 感知修正**：

- `:270-275` keep-preset 守卫（"对象省略 image_count 时不让请求预置归零"）在 countObserved 形态下自然跳过（计数非零），语义不冲突；
- **UsedQuota 推导改为 mode 感知**（实现时由单测抓获的既有缺陷：video 响应带 `usage.video_count=3` 且 `data.#=2` 时，共享 `data.#` 回退把 `ImageCount` 填成 2，原 switch `ImageCount` 优先 ⇒ `UsedQuota=2` 与 `VideoCount=3` 撕裂；RMB 按次计费走 `calcVideoGenerationCost` 用 `VideoCount` 不受影响，但 token 计划扣减与日志 `ai_total_tokens` 会取错值）：

```go
	if fields.UsedQuota == 0 {
		switch {
		case ctx.aiBasicInfo.Mode == bfe_basic.ModeImageGeneration && fields.ImageCount > 0:
			fields.UsedQuota = fields.ImageCount
		case ctx.aiBasicInfo.Mode == bfe_basic.ModeVideoGeneration && fields.VideoCount > 0:
			fields.UsedQuota = fields.VideoCount
		case fields.ImageCount > 0: // 非按次模式保持原优先序
			fields.UsedQuota = fields.ImageCount
		case fields.VideoCount > 0:
			fields.UsedQuota = fields.VideoCount
		default:
			fields.UsedQuota = fields.PromptTokens + fields.CompletionTokens
		}
	}
```

- `:287-299` 写回共享 `TokenUsage`（`VideoCount=2`、`UsedQuota=2`）；
- `return true` ⇒ 调用方 `:365-368` 置 `MarkFinalUsageSeen()` ⇒ 计费走 parsed 臂，`ai_usage_source=parsed`，**两种 EstimateToken 形态都按 2 × 50000000 = 100000000 扣费**。

**mode 门控是本修复的关键自卫条件**：不按次计费的模式（chat/responses 等）遇到"无 usage 对象但有 `data` 数组"的 body 时维持 #1414 语义（estimate/unrecognized），行为逐字节不变。

### 步骤 2（必改）：单元测试锁定三种探针与门控

文件：`bfe_modules/mod_ai_token_auth/` 现有 `UpdateCtxByUsage` 用例旁新增：

| 用例 | fixture | 期望 |
| --- | --- | --- |
| probe C / video | mode=video_generation，预置 `VideoCount=3`（模拟 `SetTokenAuthContext`），body 无 usage、`data.#=2` | 返回 true；`VideoCount=2`、`UsedQuota=2`、`ImageCount=0` |
| probe C / image | 同上，mode=image_generation、`ImageCount` 预置 3、`data.#=2` | 返回 true；`ImageCount=2`、`UsedQuota=2` |
| probe B 回归 | body 带 `usage.video_count=3` | 返回 true；`VideoCount=3`（usage 字段优先于 `data.#`） |
| probe A 等价 | 无 usage、`data.#=1` | 返回 true；`VideoCount=1`（修复前后金额相同，行为对齐） |
| chat 门控 | mode=chat（或非按次模式），body 无 usage、`data.#=2` | 返回 false（#1414 语义不变） |
| EstimateToken=false 臂 | probe C fixture + `tokenRequestFinishHandler` 收尾 | `UsageSource=parsed`、按 `data.#` 计费、非 `unrecognized` |

### 步骤 3（必改）：集成测试

SC03（`tests/integration/implementation/scenario-SC03-rmb-quota/`，已有 video generations RMB 计费环境）新增 TC：受控后端返回无 usage、`data` 恰 2 项的响应，`EstimateToken=true` 形态断言余额差 `2 × 单价`；并验证带 `usage.video_count` 的响应仍按该字段计费（probe B 不回归）。

### 步骤 4（建议）：采纳形态的可观测信号

countObserved 命中时输出 Warn（`data.# fallback adopted for per-count billing: cluster=... mode=video_generation count=2`）并计 `mod_ai_token_auth` 新计数器（如 `PerCountDataFallback`）。`ai_usage_source` 维持 `parsed`（观测本身即批准计费依据），不新增取值；该信号用于上游异常形态（省略 usage 对象）的持续观测，类似 #1410/#1414 的"形状可分辨"惯例，不属于验收判据。

## 三、修复后行为推演

| 形态 | 修复前（EstimateToken=true） | 修复前（false） | 修复后（两种形态） |
| --- | --- | --- | --- |
| probe C：无 usage、`data.#=2`、请求 n=3 | 150000000（按预置 n，多扣） | 零扣（unrecognized） | **100000000（`data.#=2`，parsed）** |
| probe A：无 usage、`data.#=1`、n=1 | 50000000 | 零扣 | 50000000（金额不变，来源从预置改为观测） |
| probe B：`usage.video_count=3` | 150000000 | 150000000 | **不变**（usage 字段优先） |
| usage 对象在但缺 `video_count`，`data.#=2` | 2 × 单价（解析层回退已生效） | 同左 | **不变** |
| chat/responses 模式、无 usage、body 带 `data` 数组 | 按估算/零扣（#1414） | 同左 | **不变**（mode 门控） |
| 客户端中断/上游断流 | #1352/#1474/#1478 语义 | 同左 | **不变**（只动 `UpdateCtxByUsage` 完整 body 路径） |

## 四、验收判据（对应 issue）

1. 同一 fixture 在 `EstimateToken=true`/`false` 两种形态下，probe C 余额差都恰为 `100000000`；true 不再出现 `150000000`，false 不再零扣。probe A= `50000000`、probe B= `150000000` 不回归。——单测（步骤 2 末行）+ SC03 集成 TC + 外部 harness 真跑三重覆盖。
2. 计费观测中视频计数来源为 `data.#=2`（`ai_video_count=2`），不沿用请求 `n=3`；无 4xx/5xx、无漏扣/双扣。
3. 带 `usage.video_count` 的响应仍优先该字段；`n=1`、`/video/generations`（no-v1）、`/v10`、`/v1beta` 边界不回归（路由层 mode 判定在 `bfe_basic/openai_endpoint.go:41-46`，本修复不触碰）。
4. cleanup/residue 通过（外部 harness run 侧，本仓无涉）。

## 五、影响面与风险

- 改动文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`（早退放宽 + mode 门控 + 兄弟计数清零 + UsedQuota 推导 mode 感知，约 30 行）。解析层（`usage_parse.go`）、鉴权预置（`:1026-1031`）、计费检查点（`:498-505`）、`calcVideoGenerationCost` **均不动**。
- 计费口径变化仅限"按次计费模式 + 响应无 usage 对象 + `data.#` 与请求 `n` 不一致"的形态：金额从"按 n（EstimateToken=true）/ 零扣（false）"修正为"按 `data.#`"。`data.# < n` 时金额下降（多扣回补），`data.# > n` 时金额上升（少收补扣）——两方向都是向条款 oracle 收敛。
- 资损方向：修复前 true 形态多扣（可回补）、false 形态漏扣（持续少收）；修复后按真实观测计费。
- 验证：`go test ./bfe_modules/mod_ai_token_auth/ ./bfe_model_protocol/...` + SC03 集成场景全绿 + 外部 harness probe A/B/C 双形态真跑（run 环境 `qa-172-19-1-222`）。
