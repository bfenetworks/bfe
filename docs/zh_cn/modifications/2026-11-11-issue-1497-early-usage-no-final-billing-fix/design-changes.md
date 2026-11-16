# BFE Issue #1497 修复方案：early usage 不得成为无 final usage 流的计费基数

- Issue: https://github.com/bfenetworks/bfe/issues/1497
- 缺陷：调用方以 `stream=true` 请求 `POST /compatible-mode/v1/responses`，上游在**非终止 delta**（`response.output_text.delta`，顶层 `type` 不在 final 白名单）中提前给出 usage（input=100/output=50），随后发送**不带 usage 的** `response.completed` 终止事件。按设计 early usage 不得成为 final usage，本笔应零扣费或按已转发内容估算计费；实际 early usage 精确进入计费基数，余额被扣 `200000` 个 1e-8 RMB 单位（0.002 元）。回归引入点：2026-10-13 `dc86cce1`（#1415 clean-EOF 完成标记）——终止事件路径开始置 `responseCompleted=true` 后，`:500` 的 `estimateBillable` 恒真，`unrecognized` 清零守卫被绕过，early usage 直接从共享 `TokenUsage` 进入计费副本。
- 代码库：`bfe/`（行号基于当前 HEAD `0bdaba44`，2026-11-11 upstream/v1.9.0-dev，逐条核实）
- 关联：#1352（final usage 只来自终止事件）、#1398（共享 TokenUsage 只写回计费结果）、#1414（usage presence 判定 / all-zero final 按 0 计费）、#1415（clean-EOF 无终止事件按估算计费）、#1474（上游中断估算计费）、#1478（客户端中断估算计费）、#1473（终止后客户端关闭不改变 final 判定）。
- 产品裁定：**已裁定（2026-11-11，采纳 §五 建议）——按 #1415 估算正扣费**，early usage 只贡献 prompt 侧协议上下文，其数值不得进入计费基数；EstimateToken=false 时本形态自然落入既有 `unrecognized` 零扣费路径。两种已发布条款的交叉处由此收敛为单一开关（`Server.EstimateToken`）决定，无需新配置。

## 一、根因

### 1.1 early usage 被判为非 final，但写账分支不要求 final

事件级 usage 判定（SSE 路径 `bfe_modules/mod_body_process/llm_util.go:160-178`，非流式 RawEvent 路径 `body_process.go:524-542`，两处逻辑同构）：

```go
isguess := !fields.UsagePresent
...
isFinalUsage := !isguess && adapter.IsFinalUsageEvent(streamEv)
```

early usage 所在事件的顶层 `type`（`response.output_text.delta` 等）不在适配器 final 白名单（`message_delta` / `message` / `response.completed` / `""`，`llm_util.go:169-178` 注释）⇒ `IsFinalUsage=false`；随后不带 usage 的 `response.completed` 因 `UsagePresent=false` 成为 guess 事件，同样不是 final。故 `finalUsageSeen` 始终为 false。

但共享 `TokenUsage` 的写账分支（`bfe_modules/mod_body_process/content_quota_usage.go:56`）的进入条件是：

```go
if tctx.UsedQuota <= 0 || rquota.IsFinalUsage {
```

**不要求**本事件是 final usage。early usage 到达时 `UsedQuota<=0` 成立 ⇒ 走 `:67-95` 直通分支，把 `UsedQuota=200000`、CompletionTokens=50、PromptTokens=100 全部写入共享 `TokenUsage`（协议上下文与计费基数共用一个对象，这是根因的结构基础）。

### 1.2 终止事件置完成标记 ⇒ 计费侧估算守卫被绕过

不带 usage 的 `response.completed` 是终止事件（`adapter.IsStreamTerminal` 为真）⇒ `content_quota_usage.go:48-50` 调 `MarkResponseCompleted()`；客户端读到终止帧后正常 EOF，clean-EOF 路径（`bfe_server/reverseproxy.go:1701-1706`，经 `shouldMarkAIStreamCompleted` `:1722-1731`）再次幂等置位。`dc86cce1`（#1415）之前全仓没有后者这个调用点，本形态 `responseCompleted=false` ⇒ `estimateBillable=false` ⇒ 落入 `unrecognized` 清零——这正是 TC 在 2026-09-24 真实通过的原因。

收尾计费（`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:492-502`）：

```go
billingUsage := *tokenUsage
estimateBillable := ctx.aiBasicInfo.IsAllowEstimateToken() &&
    (ctx.aiBasicInfo.IsResponseCompleted() || ctx.aiBasicInfo.IsUpstreamDropped() || abortedNoFinalUsage)
unrecognized := !ctx.aiBasicInfo.IsFinalUsageSeen() && !estimateBillable
```

本形态 `estimateBillable=true` ⇒ `unrecognized=false` ⇒ 计费副本保留 early usage 的 200000；`:566` 的估算兜底 `CalcReqUsedQuota` gated 在 `billingUsage.UsedQuota <= 0` 上，不再触发；`calcCostUnits` 按 token 字段直接计费 ⇒ 精确扣 200000，`ai_usage_source=estimated`（`:560` default 分支）——报表侧无法从 source 看出计费基数其实来自提前出现的 usage。

### 1.3 估算累加同时被掐断

转发内容估算依赖逐事件累加（`content_quota_usage.go:115-122`）：

```go
if !caf.aiBasicInfo.IsFinalUsageSeen() && tctx.UsedQuota <= 0 && caf.aiBasicInfo.IsAllowEstimateToken() {
    tctx.CompletionTokens += curCompletionToken
}
```

early usage 置 `UsedQuota>0` 后该门控恒假 ⇒ 后续 guess 事件的内容 token 不再累加。即使只在计费侧兜底，也无法重建"已转发内容估算"——**修复必须在写账侧（mod_body_process）动刀**，计费侧只需补可观测信号。

## 二、修复设计

设计原则（与 issue 修复方向一致）：**协议上下文与计费基数分离**。非 final 事件的 usage 可以继续写入协议上下文字段（Anthropic `message_start` 的 prompt/子 token 是后续 `message_delta` final usage 合并的必要输入，#1497 验收第 5 条），但不得写入计费基数（`UsedQuota`、`CompletionTokens`）；`finalUsageSeen=false` 时计费基数只能来自 #1415 的已转发内容估算。

### 步骤 1（必改）：写账分支按"final / 非 final"分臂

文件：`bfe_modules/mod_body_process/content_quota_usage.go`，`QuotaUsageProcessor.Process`（`:52-109`）。

把现有单一写账块拆为两臂：

- **计费臂**（`rquota.IsFinalUsage`，以及既有 image/video 计数语义 `:45-46`、`:61-66` 对应的非 guess 计数事件）：保留现有 `:56-108` 全部直通逻辑不变，含 #1414 的 all-zero 直通、`:72-85` 的 Anthropic prompt 合并（从 tctx 读 prompt 侧上下文字段）。
- **上下文臂**（`!rquota.IsGuess && !rquota.IsFinalUsage`，即 early usage 类事件）：只写 prompt 侧协议上下文字段——`PromptTokens`、`CacheReadTokens`、`CacheWriteTokens`、`CacheWriteTokens1h`、`AudioInputTokens`、`AudioOutputTokens`、`ImageInputTokens`、`VideoCount`（与 `:72-85` 合并分支读取的字段清单对齐）；**不写 `UsedQuota`、`CompletionTokens`**。`:96-107` 的兜底分支（`PromptTokens>0 || CompletionTokens>0` 时 `UsedQuota=prompt+completion`）同属计费基数，非 final 事件一律不得进入。

效果：early usage 之后 `tctx.UsedQuota` 保持 0 ⇒ `:115` 的估算累加门控自然恢复工作，guess 事件的内容 token 继续累加进 `CompletionTokens`；计费侧既有 `:566` 估算兜底、`unrecognized` 清零守卫、#1414 all-zero 语义**一行不用改**即自动接管本形态。

非流式 RawEvent 路径（`body_process.go` 的 `GetQuotaUsage` + 同一个 `Process`）自动覆盖：非流式整体 body 的 usage 事件要么 `type` 为空串、要么 `type=message`/`response.completed`（均在 final 白名单，`llm_util.go` 注释列出 `""`），`IsFinalUsage=true` 走计费臂，行为不变；仅"type 非白名单且带 usage"的非流式 body 会从"按直通值计费"变为"按估算计费"，该形态与流式 early usage 同构，正是本卡要修的形状，属预期收敛而非回归。

### 步骤 2（必改）：新增可观测标记，区分"early usage 被压制后的估算"

计费路径现在的 default 分支会让本形态落 `ai_usage_source=estimated`（`mod_ai_token_auth.go:560`），与 clean-EOF 无 usage 的估算（#1415 批准形态）在报表上不可区分——这是 issue 指出的放大因素。

- `bfe_basic/request_ai_basic.go:416-448` 新增常量，如 `UsageSourceEarlySuppressed = "early_suppressed"`（命名以实现时仓库惯例为准）。
- `content_quota_usage.go` 上下文臂命中时置一个标记（如 `aiBasicInfo.MarkEarlyUsageSeen()`，新增 bool 字段 + 访问器，与 `finalUsageSeen` 同风格）。
- `mod_ai_token_auth.go:541-561` 的 source 开关在 `default` 之前加一支：`finalUsageSeen=false && EarlyUsageSeen && estimateBillable` ⇒ 新 source 值，并配套计数器（仿 `UsageUnrecognized` / `AbortBillingEstimated` 的 `monitorHandlers` 惯例）+ warn 日志（仿 `:515-520`，带 `response_completed` / `allow_estimate` 等子条件）。

### 步骤 3（测试）：用例与 mock

- 单元测试：`bfe_modules/mod_body_process` 现有 `llm_util_test.go` / content quota 用例补充"非 final usage 只写上下文、不写 UsedQuota/CompletionTokens"与"early usage 不掐断后续估算累加"两例；`mod_ai_token_auth` 计费决策用例补 `early_suppressed` source 分支（估算开启/关闭两臂）。
- 集成（本仓 `bfe/tests`）：SC29 新增 TC-10 `TestTC10_EarlyUsageNoFinalBillsForwardedContentEstimate`（`tests/integration/implementation/scenario-SC29-usage-snapshot-billing/`）——真实 BFE 进程 + mock 上游复现本形态：content chunk + 非终止事件携带 early usage + 无 usage 的 `[DONE]`，断言余额按已转发内容估算扣减（精确等于 `prompt 上下文 × input 价 + guess 帧估算 × output 价`，**排除修复前按 early usage 聚合 21000 单位计费的缺陷签名**）、访问日志 `ai_usage_source=early_suppressed`。
- 集成（外部 harness）：issue 最小复现已给出受控后端变体 `early-usage-no-final`（mock-billing-responses-usage）与失败指纹 `1593b66ced14813f4ecce8e13e851273cfb77ea9c1b42eef9cd29cf4cadef94b`，SC1401-TC006 期望值按已裁定口径更新为估算值（见 §四验收 1）。

## 三、修复后各形态推演

| 形态 | finalUsageSeen | responseCompleted | 修复前 | 修复后 |
| --- | --- | --- | --- | --- |
| 正常：`response.completed` 带 usage | true | true | 直通计费 200000（对） | **不变** |
| all-zero final usage（#1414） | true | true | 按 0 计费（对） | **不变**（presence 驱动，计费臂直通） |
| clean-EOF 完全无 usage（SC3201-TC027 / SC29 TC-04/05/06） | false | true | 估算正扣费（对） | **不变**（早无 UsedQuota 写入，估算路径原样） |
| **early usage + 无 usage 终止事件（本 bug）** | false | true | **按 early usage 扣 200000（错）** | prompt 侧取已观测上下文，output 侧取已转发内容累加，`CalcReqUsedQuota` 估算正扣费，`ai_usage_source=early_suppressed` |
| early usage + 客户端流中中断（#1478 家族） | false | false（aborted） | 按 early usage 计费（同 bug 变种） | 已转发内容估算计费，`ai_usage_source=aborted`——顺带修正 |
| 上游流中故障（#1474） | false | false（dropped） | 按转发内容估算（对） | **不变** |
| Anthropic `message_start`(usage) + `message_delta`(final usage) | true | true | 合并 prompt 后按 final 计费（对） | **不变**（`message_start` 走上下文臂写 prompt 侧字段，`:72-85` 合并读取的字段清单与上下文臂写入清单一致） |
| 非流式 untyped/typed JSON body 带 usage | true | true（`tokenReadResponseHandler` 置位） | 直通计费（对） | **不变**（type 空串或 `message` 在 final 白名单） |

关键不变量：**只有 `IsFinalUsage`（或 image/video 计数 final-seen 语义）可以写 `UsedQuota` 与 `CompletionTokens`**；`finalUsageSeen` 置位前后的写账语义与 #1398/#1414/#1415 完全一致。

## 四、验收判据（对应 issue）

1. `early-usage-no-final` 变体：`remaining_quota` 差值**不等于 200000**；按已裁定口径（#1415）等于 `CalcReqUsedQuota(prompt 上下文, 已转发内容累加)` 的估算值，`ai_usage_source=early_suppressed`。产品裁定已闭环（§五），实现不得再引入"出现过 usage 对象即零扣费"的分支。
2. `response.completed` 带 usage 的正常 SSE：差值精确 200000；duplicate-terminal 只扣一次（计费臂重复直通同值，收尾单次结算，天然保证）；malformed 为 0。均不变。
3. OpenAI `stream_options.include_usage` final chunk 与 Anthropic `message_delta` 计费语义不变（计费臂原样）。
4. SC3201-TC027 不回归：clean-EOF 且完全无 usage 的流仍按已转发内容估算正扣费、`ai_usage_source=estimated`，不回落到 0。
5. Anthropic prompt 合并不回归：`message_start` 初始 usage 仍作为 `message_delta` final usage 的 prompt/子 token 上下文（上下文臂保留这些字段的写入）。

## 五、产品裁定：估算正扣费（#1415 口径）（2026-11-11 已裁定）

issue 指出 `2026-10-13-issue-1415` 与 `2026-09-04-client-abort-billing-fix#4.4` 在本形态上给出不同期望。裁定结论：**采用 #1415 口径，按已转发内容估算正扣费**；`#1352 §4.4` 字面的零扣费不适用本形态。裁定依据：

1. **形态同构**：本形态与 TC-027 批准的 clean-EOF 无 usage 形态的唯一差别是"中途出现过 usage 对象"。终止事件已送达、内容已完整转发、上游是干净收尾——传输层事实与 TC-027 完全相同；按"未拿到最终 usage 时不扣"字面执行零扣费，等于让上游一个非终止帧里的 usage 字段决定**消费者免费拿到一次真实推理消耗**，且该字段本来就是非法提前量（early usage 按设计不得采信其数值）。免费口子比 0.002 元量级的估算偏差更不可接受。
2. **条款层级**：#1415 是更新、更专门的条款（针对"无最终 usage"的 clean-EOF 流给出估算计费，且明确"不得把中途出现的 usage 当作最终 usage 的金额"——禁的是数值，不是计费本身）。#1352 §4.4 的"不扣"语境是**客户端中断**（内容未完整交付），与本形态（完整交付）不属同一类。
3. **实现自洽**：采估算口径时，EstimateToken 开关统一裁决"无 final usage 但已完整交付"的全部形态（有/无中途 usage 对象），`#1352 §4.4` 的零扣费仍由 EstimateToken=false 承载，两条已发布条款各归其位。已否决的零扣费口径要求引入"出现过 usage 对象即压制估算"的新规则，直接违背 #1415"已完整交付 + EstimateToken=true ⇒ 估算"的批准逻辑，并把 TC-027 语义劈成两半——此为裁定否决该口径的技术理由。

## 六、影响面与风险

- 改动文件：`bfe_modules/mod_body_process/content_quota_usage.go`（写账分臂 + EarlyUsageSeen 标记）、`bfe_basic/request_ai_basic.go`（新 source 常量与标记）、`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`（source 分支 + 计数器 + warn）。`bfe_server/reverseproxy.go`（#1415 完成标记）**不动**。
- 计费口径变化仅限"finalUsageSeen=false 且出现过非 final usage 对象"的形态：修复前按 early usage 直通值计费（本 bug），修复后按已转发内容估算计费。资金影响方向为**少扣**（200000 定点单位/笔，随 usage 线性），与 issue 影响评估一致。
- `tctx.UsedQuota` 在流中途不再反映 early usage ⇒ 访问日志 `AiTotalTokens`（`mod_access_pb3` 同读共享对象）在"early usage + 无 final"形态下记录估算镜像值而非 early usage 值——这正是 #1398 确立的"镜像估算、可观测、不静默置 0"语义，方向一致。
- 验证：`make test`（`bfe_modules/mod_body_process`、`bfe_modules/mod_ai_token_auth` 包级用例必过）+ SC1401-TC006 真实重跑（失败指纹 `1593b66c…` 应翻转）。
