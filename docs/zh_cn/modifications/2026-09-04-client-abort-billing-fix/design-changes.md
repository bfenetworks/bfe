# 修复客户端中断/响应写失败后的估算计费问题（issue #1352）

## 1. 背景

[issue #1352](https://github.com/bfenetworks/bfe/issues/1352)：EstimateToken = true 时，客户端在 SSE 响应刚收到 `message_start` 后发送 TCP RST，BFE 虽已设置 `ErrClientWrite`，但仍按**完整请求体**估算输入 token 并全量扣费（实测多扣 ¥0.17720100，可精确重构为 `117738 × 150 + 132 × 450` 计费单位）。

该 issue 与 #1343 相关但独立：#1343 修复的是 cache token 归一化问题（见 [2026-08-31-ai-token-auth-billing-fix](../2026-08-31-ai-token-auth-billing-fix/design-changes.md)），本方案解决的是**请求完成判定**与**估算计费**的边界问题。

---

## 2. 代码核实结果

issue 中引用的行号与逻辑均已在本仓库核实，全部属实（当前 HEAD）：

| issue 引用 | 实际位置 | 核实结论 |
|------------|----------|----------|
| `mod_ai_token_auth.go:285` 请求阶段初始化输入 token 估算 | `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:285-289`（`tokenFoundProductHandler`） | ✅ `EstimateToken=true` 时 `promptToken = int(GetPromptToken(req))`，经 `SetTokenAuthContext` 写入计费上下文 |
| `mod_ai_token_auth.go:193` 结束计费只判断 HTTP 200 | `mod_ai_token_auth.go:193`（`tokenRequestFinishHandler`），`:199` `if res == nil \|\| res.StatusCode != bfe_http.StatusOK` | ✅ 未判断 `req.ErrCode`，SSE 场景下响应头已以 200 刷出，状态码无区分度 |
| `content_quota_usage.go:83` 流式响应按内容估算输出 token | `bfe_modules/mod_body_process/content_quota_usage.go:83-90` | ✅ `tctx.UsedQuota <= 0 && IsAllowEstimateToken()` 时按事件内容长度累加估算 |
| `reverseproxy.go:1425` 写客户端失败设置 ErrClientWrite | `bfe_server/reverseproxy.go:1424-1435`，`ErrCode = bfe_basic.ErrClientWrite` 在 `:1431` | ✅ |
| `http_conn.go:594` 写失败后结束回调仍执行 | `bfe_server/http_conn.go:594-595`（`FinishReq`） | ✅ 回调无条件执行 |
| `bfe.conf.md:32` EstimateToken 配置说明 | `docs/zh_cn/configuration/bfe.conf.md:32`（`Server.EstimateToken`） | ✅ |

补充核实的相关事实：

- `GetPromptToken`（`mod_ai_token_auth.go:456-463`）：估算公式即 `ContentLength / 4`，与 issue 重构算式 `470952 / 4 = 117738` 一致。
- `bfe_basic/error_code.go:24`：`ErrClientWrite` 定义；同文件还有 `ErrClientClose`（:25）、`ErrClientReset`（:32）等客户端中断类错误码。
- `bfe_basic/request.go:87`：`Request.ErrCode` 为 `error` 类型，可在 `tokenRequestFinishHandler` 中直接与 `bfe_basic.ErrClientWrite` 比较。
- `SSEEvent.GetQuotaUsage`（`bfe_modules/mod_body_process/llm_util.go:153-179`）：逐事件提取 usage，**不区分事件类型**。Anthropic `message_start` 自带的 `usage.output_tokens = 0` 会被当作真实 usage（`IsGuess = false`），流在 `message_start` 后被截断时，该初始 usage 会被误当作最终 usage 参与计费。

---

## 3. 根因分析

实际链路：

```
客户端 RST
  → reverseproxy sendResponse 报错，设置 ErrClientWrite（reverseproxy.go:1431）
  → HTTP 状态码仍为 200（SSE 响应头早已刷出）
  → 未取得最终 usage（无 message_delta 最终 usage / message_stop）
  → FinishReq 回调照常执行（http_conn.go:595）
  → tokenRequestFinishHandler 仅看 StatusCode == 200（mod_ai_token_auth.go:199）
  → 请求阶段注入的 prompt 估算值（:285-289）+ 流式内容估算值
  → Redis DECRBY 按完整请求体全量扣费
```

设计缺陷：`EstimateToken` 的语义是"**响应 usage 不可用**时允许估算"，但当前实现未区分"usage 不可用"的原因——是正常完成缺 usage，还是客户端中断/写失败。两类场景被同等对待，均按全量估算扣费。

---

## 4. 修复方案

### 4.1 计费上下文增加请求完成状态

**改动位置**：`bfe_basic/request_ai_basic.go`（`AiBasicInfo`）

在 `AiBasicInfo` 中增加完成状态字段：

```go
type AiBasicInfo struct {
    // ... 原有字段 ...
    // responseCompleted marks whether the upstream response finished normally
    // (e.g. message_stop received for Anthropic SSE, [DONE] for OpenAI SSE,
    // or the full non-streaming body was read).
    responseCompleted bool
    // finalUsageSeen marks whether the final usage was parsed from the
    // response. An initial usage such as Anthropic message_start
    // (output_tokens = 0) does not count.
    finalUsageSeen bool
}
```

状态放在 `bfe_basic.AiBasicInfo` 而非 `TokenAuthContext`：`mod_body_process` 与 `mod_ai_token_auth` 都依赖该状态，而 `mod_body_process` 不 import `mod_ai_token_auth`（避免模块间循环依赖），只能通过共享的 `bfe_basic` 传递。

状态来源：

- `finalUsageSeen`：由 `mod_body_process` 在解析到**最终** usage 时置位。注意 Anthropic 流式协议中最终 usage 出现在 `message_delta`（含 `usage.output_tokens`）事件中，`message_start` 的 usage（`output_tokens = 0`）是初始值，不能视为最终 usage；图片/视频按次计费事件（非 guess 的 ImageCount/VideoCount）同样置位。
- `responseCompleted`：流式场景由终止事件置位（Anthropic `message_stop`、OpenAI `[DONE]`）；非流式场景由 `tokenReadResponseHandler` 在完整读取响应体后置位。

### 4.2 tokenRequestFinishHandler 增加异常请求保护

**改动位置**：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`（`tokenRequestFinishHandler`）

实际实现（`isClientAbortErr` 覆盖 `ErrClientWrite` / `ErrClientClose` / `ErrClientReset`）：

```go
aborted := isClientAbortErr(req.ErrCode)
if aborted && !ctx.aiBasicInfo.IsFinalUsageSeen() {
    ctx.deducted = true
    return bfe_module.BfeHandlerGoOn // 零扣费
}

tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
// EstimateToken 播种的值只有响应正常完成时才可计费；否则清零，
// 因为 calcCostUnits 直接按 token 字段计费，仅守卫 UsedQuota 不够。
estimateBillable := ctx.aiBasicInfo.IsAllowEstimateToken() && ctx.aiBasicInfo.IsResponseCompleted()
if !ctx.aiBasicInfo.IsFinalUsageSeen() && !estimateBillable {
    tokenUsage.PromptTokens = 0
    tokenUsage.CompletionTokens = 0
    tokenUsage.UsedQuota = 0
}
if tokenUsage.UsedQuota <= 0 && estimateBillable {
    tokenUsage.UsedQuota = CalcReqUsedQuota(req, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
}
```

**理由**：

- `res.StatusCode == 200` 在 SSE 场景下没有区分度（响应头先于流内容发送），不能作为计费判据。
- 实现过程中发现更深一层的问题：`calcCostUnits`（`calcChatCost`）直接按 `PromptTokens`/`CompletionTokens` 字段计费，与 `UsedQuota` 守卫无关。鉴权阶段播种的输入估算值（`mod_ai_token_auth.go` `tokenFoundProductHandler`）会因此流向最终扣费，所以估算不可计费时必须把字段清零。
- `EstimateToken` 只应作用于"正常完成但缺少 usage"的请求，不应覆盖客户端中断场景。
- `ErrClientClose` / `ErrClientReset` 与 `ErrClientWrite` 同属客户端中断类，需一并处理，否则客户端用 RST 之外的方式中断仍可绕过。

### 4.3 Anthropic 报文解析补齐与最终 usage 合并

**改动位置**：

- `bfe_model_protocol/utils/usage_parse.go`（`ParseAnthropicUsageFields`）
- `bfe_modules/mod_body_process/llm_util.go`（`SSEEvent.GetQuotaUsage`）
- `bfe_modules/mod_body_process/content_quota_usage.go`（`QuotaUsageProcessor.Process`）

三个配套修复：

1. **真实报文结构解析**：线上 Anthropic 流式的初始 usage 位于 `message_start.message.usage`（此前只解析顶层 `usage`），`ParseAnthropicUsageFields` 增加 `message.usage.*` 回退路径。
2. **初始/最终 usage 区分**：`QuotaUsage` 增加 `IsFinalUsage` / `IsTermination` 标志。`message_start` 的 usage 是初始值（`output_tokens = 0`，`IsFinalUsage = false`）；最终 usage 判定为：非 guess 且 `CompletionTokens > 0` 的 `message_delta`（Anthropic）或无 `type` 的最终 chunk（OpenAI，`stream_options.include_usage`）；终止事件为 `message_stop` / `[DONE]`。
3. **最终 usage 合并**：原实现在 `message_start` 设置 `UsedQuota` 后，后续 `message_delta` 的 completion tokens 被 `tctx.UsedQuota <= 0` 门槛忽略，导致输出 token 漏计。现改为 `tctx.UsedQuota <= 0 || rquota.IsFinalUsage` 都进入处理；`message_delta` 只带输出 token 时，从 `message_start` 已解析的上下文补齐 prompt 及子 token 字段后再落账。

### 4.4 产品计费规则（已按规则 2 实现）

异常请求（客户端中断/写失败）的计费策略：

1. **只对成功响应计费**：`ErrClient*` 且未拿到最终 usage 时零扣费。
2. **按已确认的实际 usage 计费**（**本次实现采用**）：`finalUsageSeen == true`（上游成本已确定）时按实际 usage 扣费，未拿到最终 usage 时不扣。

即：客户端中断于 `message_start` → 零扣费；中断于最终 usage 之后 → 按实际 usage 扣费。无论哪种情况，**禁止**按完整请求体估算扣费。

---

## 5. 代码变更汇总

| 改动点 | 文件 | 说明 |
|--------|------|------|
| 增加完成状态字段 | `bfe_basic/request_ai_basic.go` | `AiBasicInfo` 增加 `responseCompleted` / `finalUsageSeen` 及访问方法 |
| 异常请求计费保护 | `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | `tokenRequestFinishHandler`：`ErrClient*` 且未拿到最终 usage 时跳过扣费；估算不可计费时清零估算字段（`calcCostUnits` 直接按字段计费） |
| 非流式完成标记 | `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | `tokenReadResponseHandler`：完整读取响应体后置位 `responseCompleted` / `finalUsageSeen` |
| 客户端中断错误判定 | `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | 新增 `isClientAbortErr`（`ErrClientWrite`/`ErrClientClose`/`ErrClientReset`） |
| 终止事件识别 | `bfe_modules/mod_body_process/llm_util.go` | `QuotaUsage` 增加 `IsFinalUsage` / `IsTermination`；`SSEEvent.GetQuotaUsage` 识别 `message_stop` / `[DONE]` / 最终 usage chunk |
| 最终 usage 合并 | `bfe_modules/mod_body_process/content_quota_usage.go` | `Process` 置位完成状态；`message_delta` 最终 usage 与 `message_start` 输入 token 合并 |
| Anthropic 报文解析 | `bfe_model_protocol/utils/usage_parse.go` | `ParseAnthropicUsageFields` 增加 `message.usage.*` 回退路径 |

---

## 6. 测试

**单元测试**：

- `mod_ai_token_auth_test.go`：`TestTokenRequestFinishHandler_ClientAbortNoFinalUsage`（EstimateToken 开/关均不扣费）、`TestTokenRequestFinishHandler_ClientAbortWithFinalUsage`（按实际 usage 扣费）、`TestTokenRequestFinishHandler_EstimateRequiresCompletedResponse`（响应未完成时估算不生效）。
- `mod_body_process`（`body_process_test.go` / `llm_util_test.go`）：`TestQuotaUsageProcessorMarksResponseCompletion`、`TestSSEEventGetQuotaUsage_CompletionFlags`。
- `bfe_model_protocol/anthropic`：`TestExtractUsageFieldsClaudeStreamingMessageUsage`（`message.usage` 真实结构解析）。

**集成测试**：`tests/integration/implementation/scenario-SC12-client-abort-billing/`（SC12 客户端中断计费，3 个用例）：

1. TC-01：EstimateToken = true，客户端在 `message_start` 后 RST，后端追加 1MB 帧强制 `ErrClientWrite` → 零扣费。
2. TC-02：收到 `message_delta` 最终 usage 后 RST → 按实际 usage 扣费（320×452 + 150×2262 = 483940）。
3. TC-03：正常完整 SSE 流 → 按实际 usage 扣费（回归保护）。

设计文档：`tests/integration/测试设计文档/scenario-SC12-客户端中断计费/`。

---

## 7. 兼容性说明

- 修复后客户端中断请求从"按全量估算扣费"变为"零扣费或按实际 usage 扣费"，**收费金额会下降**，这是修正多收后的正确行为。
- 本次多扣的费用（如第 22 次请求的 ¥0.17720100）需在计费对账层单独处理。
- `EstimateToken` 配置语义收窄：仅对正常完成的请求生效。配置文档已同步更新：`docs/zh_cn/configuration/bfe.conf.md:32`、`docs/en_us/configuration/bfe.conf.md:32`。
