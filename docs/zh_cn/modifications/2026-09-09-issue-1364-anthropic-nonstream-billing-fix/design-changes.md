# BFE Issue #1364 修复方案

- Issue: https://github.com/bfenetworks/bfe/issues/1364
- 缺陷：Anthropic 非流式（chunked）响应在 #1352 守卫后 RMB 计费只按 cache_read 入账，漏计未命中输入与输出（实测漏计 4.65x）
- 代码库：`bfe/`（已对照当前代码逐条核实，行号基于当前 HEAD）

## 一、根因（三层叠加，缺一不可）

### 第 1 层（直接缺陷）：#1352 守卫漏清 cache/audio/image 子字段

`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:243-247`：

```go
if !ctx.aiBasicInfo.IsFinalUsageSeen() && !estimateBillable {
    tokenUsage.PromptTokens = 0
    tokenUsage.CompletionTokens = 0
    tokenUsage.UsedQuota = 0
    // ← CacheReadTokens / CacheWriteTokens / Audio* / Image* / VideoCount / ImageCount 全部漏清
}
```

`TokenUsage` 共 12 个字段（`bfe_basic/request_ai_basic.go:59-71`），守卫只清了 3 个。
随后 `calcCostUnits` → `calcChatCost`（`mod_ai_token_auth.go:591-699`）拿到
`{Prompt:0, Completion:0, CacheRead:N}`：`normalInput = 0 - N → 截断 0`，`normalOutput = 0`，
只剩 `cacheRead × cache_read_price` 入账 —— 与线上"只收缓存命中"现象精确吻合。

### 第 2 层（触发条件）：Anthropic 非流式 JSON 永不标记 final-usage / response-completed

`MarkFinalUsageSeen()` 的两个来源对非流式 Anthropic JSON 均失效：

- `mod_body_process/content_quota_usage.go:44` 要求 `rquota.IsFinalUsage`；
  `isFinalUsage`（`mod_body_process/llm_util.go:179-185`）只认顶层
  `type=="message_delta" || ""`，Anthropic 非流式顶层是 `"type":"message"` → 永不标记。
- `MarkResponseCompleted()` 只认 `message_stop` / `[DONE]`（`llm_util.go:177`），
  非流式单 JSON 事件同样打不上 → `estimateBillable`（`mod_ai_token_auth.go:242`）也为 false。
- `tokenReadResponseHandler`（`mod_ai_token_auth.go:171`）要求 `res.ContentLength >= 0`；
  chunked 响应 `ContentLength == -1` → 整段跳过。

但组合解析链 `extractUsageFields`（`mod_body_process/llm_util.go:139-156`，
OpenAI 链 → Claude 链回退）对 Anthropic body 工作正常，已把全部 usage 字段（含 CacheRead）
写入上下文 —— "值填上了、标记没打上"，守卫一清就只剩 cache_read。

### 第 3 层（使能回归，commit 286b4e0a）：单适配器丢失跨协议兜底

`UpdateCtxByUsage`（`mod_ai_token_auth.go:116-163`）把 usage 解析从组合链改为
`modelprotocol.Get(ctx.aiBasicInfo.AuthStyle).ExtractUsageFields(data)` 单适配器（`:121`）。
OpenAI 链只认 `usage.prompt_tokens / total_tokens`（`bfe_model_protocol/utils/usage_parse.go:42-71`），
AuthStyle 与响应体格式错配（如 Bearer → ProtocolOpenAI + Anthropic body，DeepSeek 官方
key 为 `sk-` 前缀必然走这条路）时解析全零 → `UsedQuota=0` → 连
`tokenReadResponseHandler:178` 的打标也失效。旧组合链任何 body 都能解出 `UsedQuota>0`。

此前测试全过的原因：既有受控后端返回带 Content-Length 的小 JSON（走
tokenReadResponseHandler 正常打标）；SSE 靠 `message_delta` 打标；
"chunked + 非流式 Anthropic"从未被覆盖，真实 DeepSeek 流量恰好落入盲区。

## 二、修复步骤

### 步骤 1（必改，止血）：守卫清零全部计费字段

文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`，`:243-247`

将守卫改为清零 `TokenUsage` 的全部 token 字段（保留 `UsedCost`，其下有
`UsedCost <= 0` 判断，会随重新计算覆盖）：

```go
if !ctx.aiBasicInfo.IsFinalUsageSeen() && !estimateBillable {
    tokenUsage.PromptTokens = 0
    tokenUsage.CompletionTokens = 0
    tokenUsage.CacheReadTokens = 0
    tokenUsage.CacheWriteTokens = 0
    tokenUsage.AudioInputTokens = 0
    tokenUsage.AudioOutputTokens = 0
    tokenUsage.ImageInputTokens = 0
    tokenUsage.VideoCount = 0
    tokenUsage.ImageCount = 0
    tokenUsage.UsedQuota = 0
}
```

效果：未确认完成的响应不再按残存子字段收 RMB（宁漏收、不错收）。
注意：`ImageCount/VideoCount` 分支在 `content_quota_usage.go:44` 有独立打标路径
（`!IsGuess && (ImageCount>0 || VideoCount>0)`），清零它们不会改变"图片/视频
生成已成功即计费"的语义，因为那条路径会置 `IsFinalUsageSeen()`。

### 步骤 2（必改，治本）：`isFinalUsage` 认可非流式 Anthropic JSON

文件：`bfe_modules/mod_body_process/llm_util.go`，`:179-185`

```go
if evType == "message_delta" || evType == "message" || evType == "" {
    isFinalUsage = true
}
```

`gjson` 只读顶层 `type`，SSE 的 `message_start`（顶层 `event: message_start`，
JSON 里无顶层 `type` 字段或嵌套在 `message.` 下）不受影响；`message_stop`
分支本就不满足 `CompletionTokens > 0` 以外的条件组合，保持不变。

同时把 `isTermination`（`:177`）扩展为非流式完成信号，使
`MarkResponseCompleted()` 对非 SSE 单 JSON 生效：

```go
isTermination := evType == "message_stop" ||
    strings.TrimSpace(string(data)) == "[DONE]" ||
    (evType == "message" && !isStreaming) // 非流式 Anthropic 整体即一条完成事件
```

实现上更简单稳妥的做法：在 `QuotaUsageProcessor.Process` 中，当事件是"整个
响应体的单 JSON 且解析出非 guess usage"时同时调用 `MarkResponseCompleted()`，
而不是改动 `isTermination` 的纯数据语义（见步骤 4 的推荐实现）。

### 步骤 3（修回归）：恢复跨协议解析兜底

文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`，`:116-163`

`UpdateCtxByUsage` 保持单适配器优先（正常路径零开销），单适配器结果全零时
回退组合链，恢复 286b4e0a 之前的"响应格式无关"语义：

```go
adapter := modelprotocol.Get(ctx.aiBasicInfo.AuthStyle)
fields := adapter.ExtractUsageFields(data)
if fields.UsedQuota == 0 && fields.PromptTokens == 0 &&
    fields.CompletionTokens == 0 && fields.ImageCount == 0 && fields.VideoCount == 0 {
    // auth style 与响应体格式错配（如 Bearer→OpenAI 适配器 + Anthropic body）：
    // 回退到 OpenAI→Claude 组合链，恢复 286b4e0a 之前的跨协议解析语义。
    fields = extractUsageFieldsCrossProtocol(data) // 上提 mod_body_process 的 extractUsageFields
}
```

为避免 `bfe_modules` 与 `bfe_model_protocol` 之间出现新的依赖方向，建议把
组合链逻辑下沉到 `bfe_model_protocol/utils`（如新增
`ParseUsageFieldsWithFallback(data)`，内部 = OpenAI 链 + Claude 链回退），
`mod_body_process/llm_util.go:139-156` 与 `mod_ai_token_auth.go` 共用一份实现，
杜绝两处逻辑再次分叉。

### 步骤 4（加固）：非流式单 JSON 事件处理完后标记响应完成

文件：`bfe_modules/mod_body_process/content_quota_usage.go`，`Process`

对"整个响应体作为一个事件"的非 SSE 路径（`RawEvent` / 整条 body 一次性 decode），
在解析出非 guess usage 后追加 `MarkResponseCompleted()`，使
`estimateBillable`（`mod_ai_token_auth.go:242`）对正常完成的非流式响应生效，
`#1352` 守卫只拦截真正未完成/被截断的响应：

```go
if rquota.IsFinalUsage || (!rquota.IsGuess && (rquota.ImageCount > 0 || rquota.VideoCount > 0)) {
    caf.aiBasicInfo.MarkFinalUsageSeen()
    if !isStreamingResponse { // 非 SSE：单事件即完整响应
        caf.aiBasicInfo.MarkResponseCompleted()
    }
}
```

`isStreamingResponse` 可由 `Content-Type: text/event-stream` 或 decoder 类型判定；
若判定点不便获取，可退而求其次：事件数为 1 且 body 是合法完整 JSON 时标记完成。

## 三、回归测试

新增/修改用例（均放在被测代码旁的 `_test.go`，沿用 `testing` + `testify`）：

1. `mod_ai_token_auth` 单元测试：构造
   `{PromptTokens:0, CompletionTokens:0, CacheReadTokens:N, UsedQuota:0}` +
   `IsFinalUsageSeen()==false` + `IsResponseCompleted()==false` 的上下文，
   断言 `tokenRequestFinishHandler` 后 `UsedCost == 0`（止血守卫生效）。
2. `mod_body_process` 单元测试：SSE 事件序列注入顶层 `type:"message"` 的
   Anthropic 非流式 JSON（模拟整 body 单事件），断言 `MarkFinalUsageSeen()`
   与 `MarkResponseCompleted()` 均被调用。
3. `mod_ai_token_auth` 单元测试：`UpdateCtxByUsage` 传入 AuthStyle=OpenAI +
   Anthropic 格式 body，断言回退组合链后 `UsedQuota>0` 且各子字段正确。
4. E2E（`bfe/tests/integration` SC03）：新增
   `TestTC16_RMBQuotaDeduction_Anthropic_NonStream_Chunked`（非流式 + 无
   Content-Length(chunked) + Anthropic 格式 body）与
   `TestTC17_RMBQuotaDeduction_Anthropic_NonStream_ContentLength`（带 Content-Length
   臂，验证跨协议回退），断言 RMB 扣款 = input全价×未命中 + cache_read×缓存价
   + output×输出价；既有 SSE 臂与 Cache 计费臂防止误伤。

验证命令：`cd bfe && make test`（go test -cover ./... + go vet ./...）。

## 四、修复后正确计费路径（目标行为）

非流式 Anthropic（chunked）请求：

1. `extractUsageFields`（或单适配器+回退）解析出完整 usage → 全部字段入 `TokenUsage`；
2. `isFinalUsage` 认可 `type:"message"` → `MarkFinalUsageSeen()`；
   单事件非 SSE → `MarkResponseCompleted()`；
3. `tokenRequestFinishHandler`：守卫不触发（final usage 已见）；
4. `calcChatCost`：`normalInput = prompt − cacheRead − cacheWrite`（未命中部分按
   input 全价）、`cacheRead × cache_read价`、`normalOutput × output价` ——
   与 DeepSeek 后台账单对齐（如 issue 中 0.8612 + 0.3698 + 0.4920 = 1.7230 的分解）。

## 五、实施记录（2026-09-09）

已按上述方案实施，变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_model_protocol/utils/usage_parse.go` | 新增 `ParseUsageFieldsCrossProtocol`（OpenAI 链 → prompt/completion 全零时回落 Anthropic 链），组合链唯一实现 |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | ① 守卫清零全部 10 个计费字段（issue #1364）；② `UpdateCtxByUsage` 单适配器结果全零时回退 `ParseUsageFieldsCrossProtocol` |
| `bfe_modules/mod_body_process/llm_util.go` | 删除本地 `extractUsageFields` 改为调用 utils 共用实现；`isFinalUsage` 增加顶层 `type:"message"` |
| `bfe_modules/mod_body_process/body_process.go` | `RawEvent.GetQuotaUsage` 非 guess usage 置 `IsFinalUsage`/`IsTermination`（非 SSE 单 JSON 即完整响应） |
| 单元测试 | `utils/usage_parse_test.go`（新增）；`mod_ai_token_auth_test.go`：`TestUpdateCtxByUsage_CrossProtocolFallback`、`TestTokenRequestFinishHandler_GuardClearsSubTokenFields`、`TestTokenRequestFinishHandler_RMB_NonStreamingAnthropic`；`llm_util_test.go`：`TestRawEventGetQuotaUsage_*`、SSE CompletionFlags 增加 `message` 臂；`content_quota_usage_test.go`：`TestQuotaUsageProcessorProcessAnthropicNonStreamMarks` |
| 集成测试 | `bfe/tests/integration` SC03 新增 `TestTC16_RMBQuotaDeduction_Anthropic_NonStream_Chunked`（非流式+chunked+Anthropic body）、`TestTC17_RMBQuotaDeduction_Anthropic_NonStream_ContentLength`（带 Content-Length 臂，验证跨协议回退），扣减断言 900000 定点单位；`tests/integration/common/mock_backend.go` 新增 `NoContentLength` 开关（flush 响应头强制 chunked）；设计文档 `TC-16`/`TC-17` 及场景说明已同步 |

验证：`go test ./bfe_model_protocol/... ./bfe_modules/mod_body_process/... ./bfe_modules/mod_ai_token_auth/...` 全部通过；`bfe/tests/integration` SC03 集成测试 TC-01~TC-17 全部通过。
