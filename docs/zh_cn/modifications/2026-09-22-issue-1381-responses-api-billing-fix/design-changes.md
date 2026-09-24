# BFE Issue #1381 修复方案

- Issue: https://github.com/bfenetworks/bfe/issues/1381
- 缺陷：Codex 经 Responses API（`POST /compatible-mode/v1/responses`，SSE）调用时，流已完成且收到完整 `response.usage`，但整请求零扣费（Redis 只有密钥查询与 TTL 更新，无扣减命令）
- 代码库：`bfe/`（已对照当前代码逐条核实，行号基于 HEAD c8bbcbfe）

## 一、根因（三层叠加，缺一不可）

### 第 1 层（直接缺陷）：usage 解析不识别 `response.usage.*`

Responses API 的最终事件 `response.completed` 中，usage 嵌套在 `response` 对象下：

```json
{"type":"response.completed","response":{...,
  "usage":{"input_tokens":493323,"output_tokens":4245,"total_tokens":497568,
           "input_tokens_details":{"cached_tokens":N}}}}
```

而 `ParseOpenAIUsageFields`（`bfe_model_protocol/utils/usage_parse.go:57-95`）只读**顶层** `usage.*` 路径。其中第 89-91 行所谓的 "Responses API fallback" 读的仍是 `usage.input_token_details.cached_tokens`——顶层 `usage.` 前缀，够不到 `response.usage.*`。

跨协议兜底 `ParseUsageFieldsCrossProtocol`（`usage_parse.go:109-140`）也救不了：Anthropic 链只认 `usage.*` / `message.usage.*`（`:154-188`），Gemini 链只认 `usageMetadata.*`（`:204-216`），均不命中 `response.usage.*` → 解析结果全 0。

### 第 2 层：`response.completed` 不被识别为最终 usage / 流结束事件

- OpenAI 适配器 `IsStreamTerminal`（`bfe_model_protocol/openai/stream.go:29-31`）只认 `message_stop` / `[DONE]`；
- `IsFinalUsageEvent`（`openai/stream.go:41-43`）只认 `message_delta` / `message` / 无 type 的 usage chunk；
- `GetQuotaUsage`（`bfe_modules/mod_body_process/llm_util.go:156-163`）取事件 data 顶层 `type` 构造 `StreamEvent`，`response.completed` 两个判断都不命中 → `content_quota_usage.go:45-50` 永远不会调用 `MarkFinalUsageSeen()` / `MarkResponseCompleted()`。
- 与第 1 层双重叠加：usage 全 0 → `isguess=true`（`llm_util.go:141-145`），即便 `IsFinalUsageEvent` 认这个事件，也被 `:161` 的 `fields.CompletionTokens > 0` 门槛挡住。

### 第 3 层（表象）：#1352 客户端中止守卫跳过扣费

`tokenRequestFinishHandler`（`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:244-248`）：

```go
aborted := isClientAbortErr(req.ErrCode)
if aborted && !ctx.aiBasicInfo.IsFinalUsageSeen() {
    ctx.deducted = true
    return bfe_module.BfeHandlerGoOn // 跳过 plan.Deduct
}
```

Codex 在收到完整 `response.completed` 后正常关闭连接，BFE 记录 `CLIENT_CLOSE,EOF`（`ErrClientClose`）→ 守卫因 `IsFinalUsageSeen()==false`（第 2 层所致）误判为"客户端提前中止"→ 跳过扣费。与线上现象（4 个已完成请求合计 49 万+ token，Redis 无一条扣减）精确吻合。

**守卫本身无需修改**：其语义是"最终 usage 已识别时即使 CLIENT_CLOSE 也照常扣费，未识别时宁漏收不错收"（#1352），逻辑正确；病根在第 1、2 层导致标记永远打不上。

## 二、修复步骤

### 步骤 1（必改）：`ParseOpenAIUsageFields` 增加 `response.usage.*` 解析链

文件：`bfe_model_protocol/utils/usage_parse.go`，`:57-95`

把"按前缀取字段"的逻辑抽成 helper（现有 `parseCacheWriteTokens1h(data, prefix)` 已是同款模式），主链用 `usage` 前缀，主链 prompt/completion 全零时回落 `response.usage` 前缀：

| Responses API 字段 | 统一计费字段 |
| --- | --- |
| `response.usage.input_tokens` | `PromptTokens` |
| `response.usage.output_tokens` | `CompletionTokens` |
| `response.usage.total_tokens` | `UsedQuota` |
| `response.usage.input_tokens_details.cached_tokens` | `CacheReadTokens` |

两个必须注意的语义点：

1. **PromptTokens 归一化**：Responses API 的 `input_tokens` **不含** cached tokens（与 Anthropic `input_tokens` 同语义），而下游 `calcChatCost` 的成本拆分是 `normalInput = prompt − cacheRead − cacheWrite`（PromptTokens 必须是含缓存的总量）。因此命中 Responses 链后按 Anthropic 链同款归一化（`usage_parse.go:178`）：`PromptTokens = input_tokens + CacheReadTokens (+ CacheWriteTokens)`。

   > **更正（2026-09-24，issue #1389）**：上述前提有误，已被裁定作废。OpenAI Responses API 实为 **subset 语义**：`input_tokens` 已含 `cached_tokens`（`total_tokens = input_tokens + output_tokens`，与 Chat Completions、Gemini 一致；仅 Anthropic 为 additive）。该 additive 归一化导致 cached tokens 重复计费（未配 cache 价格时按 input 价多计一次），已在 issue #1389 修复中从 `usage_parse.go` 删除。Anthropic 链（`usage_parse.go:222` 附近）的归一化不受影响，保留。详见 `docs/zh_cn/modifications/2026-09-24-issue-1389-responses-api-cached-tokens-double-billing/design-changes.md`。
2. **回落而非并行**：沿用现有 "字段为 0 才回落" 的写法，顶层 `usage.*` 优先，避免与已有 Chat Completions / DeepSeek 链冲突。

**非流式形态修正**（实施时核实官方 API 形态后修正）：Responses API 非流式 create-response 对象的 usage 在**顶层** `usage` 下（不在 `response.usage` 下），字段名同样是 `input/output_tokens`。该形态以 Responses API 特有的 `input_token(s)_details` 字段为门控进入（Anthropic body 同样有 `usage.input_tokens`，但缓存字段是 `cache_read_input_tokens`，绝不能误判进来——否则跨协议兜底链被短路，cache 归一化丢失，正是 #1364 的漏收/错收形态）。此步骤自然惠及 OpenAI 适配器作为 registry 兜底的路径（`detect.go:31-37` Bearer 一律判 OpenAI，`openai/stream.go:26-28`）。

> 附带修复：主链历史 "Responses API fallback" 读的缓存字段是 `usage.input_token_details.cached_tokens`（单数 token），官方字段为 `input_tokens_details.cached_tokens`（复数）。实施改为复数优先、单数兼容（image_tokens 同理），两种拼写都接受。

### 步骤 2（必改）：`openai/stream.go` 识别 `response.completed`

文件：`bfe_model_protocol/openai/stream.go`，`:29-43`

- `IsStreamTerminal` 增加 `ev.Type == "response.completed"`；
- `IsFinalUsageEvent` 增加 `ev.Type == "response.completed"`。

必须**精确匹配**，不得放宽前缀/通配：`response.output_item.done`、`response.created`、`response.incomplete` 等事件不带最终 usage，误标会把未完成响应当作完成计费。

效果：`response.completed` 到达时（usage 已被步骤 1 解析出 `CompletionTokens>0`，通过 `llm_util.go:161` 门槛）→ `MarkFinalUsageSeen()` + `MarkResponseCompleted()` 置位。

### 步骤 3（无需改代码，语义确认）：#1352 / #1364 守卫保持原样

- `mod_ai_token_auth.go:244-248` 中止守卫：步骤 2 使 `IsFinalUsageSeen()==true`，CLIENT_CLOSE 自然放行扣费；真正中止的流仍被拦截，#1352 语义不变。
- `mod_ai_token_auth.go:260-272` 全字段清零守卫（#1364）：`response.completed` 事件 `IsFinalUsage==true` 不会触发清零，保持不动。

### 边界说明（本次不修，留作记录）

`response.incomplete`（如 max_output_tokens 截断）同样携带 `response.usage`，步骤 1 会把它解析进上下文，但因其不是 `response.completed`，`IsFinalUsageSeen()` 不置位 → 请求结束时被 #1364 守卫清零、不计费。这与 #1352"宁漏收、不错收"的语义一致；如需对 incomplete 按部分 usage 计费，属计费策略变更，另行设计。

## 三、回归测试

新增/修改用例（均放在被测代码旁的 `_test.go`，沿用 `testing` + `testify`）：

1. `utils/usage_parse_test.go`：`response.completed` 事件体（含 `input_tokens/output_tokens/total_tokens/input_tokens_details.cached_tokens`）断言四字段解析正确且 `PromptTokens` 含 cached 归一化；非流式顶层 usage + `input_tokens_details` 门控臂；无缓存 / 中间 delta / Chat Completions 优先级共 4 个新用例；既有 Chat Completions / DeepSeek / Anthropic / Gemini 用例全量回归（顶层 `usage.*` 优先不受影响）。
2. `openai/stream_test.go`：`response.completed` → `IsStreamTerminal==true` 且 `IsFinalUsageEvent==true`；`response.created` / `response.output_item.done` / `response.incomplete` / `response.output_text.delta` 均为 false；`[DONE]`、`message_stop`、无 type usage chunk 行为不变。
3. `mod_body_process` 单测：`TestQuotaUsageProcessorProcessResponsesAPICompleted`（SSE 序列注入 `response.completed` 带 usage，断言 `MarkFinalUsageSeen()`/`MarkResponseCompleted()` 置位且字段归一化正确）；`TestQuotaUsageProcessorProcessResponsesAPIIncomplete`（`response.incomplete` 不得置位两个标记，守住 #1352 语义）。
4. 集成测试（`bfe/tests/integration` SC03 RMB 计费）：新增 `TestTC18_RMBQuotaDeduction_ResponsesAPI_Streaming`（`response.created` → `response.output_text.delta` → `response.completed`，mode `responses`，扣减 900000 定点单位）与 `TestTC19_RMBQuotaDeduction_ResponsesAPI_NonStreaming`（非流式 create-response 对象臂），设计文档 `TC-18`/`TC-19` 及场景说明已同步；既有 TC-01~TC-17 全量回归。

验证命令：`cd bfe && make test`（go test -cover ./... + go vet ./...），集成测试跑 SC03 全场景。

## 四、修复后正确计费路径（目标行为）

Responses API（Codex）SSE 请求：

1. `GetQuotaUsage`（`llm_util.go:137`）→ `ParseUsageFieldsCrossProtocol` → `ParseOpenAIUsageFields` 经 `response.usage.*` 链解析出完整 usage，全部字段入 `TokenUsage`；
2. `response.completed` 命中 `IsFinalUsageEvent` / `IsStreamTerminal` → `MarkFinalUsageSeen()` + `MarkResponseCompleted()`；
3. 客户端正常关闭连接，`req.ErrCode=ErrClientClose`：`tokenRequestFinishHandler` 中止守卫不触发（final usage 已见）；
4. `calcChatCost`：`normalInput = prompt − cacheRead`（未命中输入按全价）、`cacheRead × 缓存价`、`normalOutput × 输出价` → `plan.Deduct` 产生 Redis 扣减记录。

## 五、验收标准对照（issue 原文）

| # | 验收标准 | 达成方式 |
| --- | --- | --- |
| 1 | `response.completed` 能正确解析 usage | 步骤 1 |
| 2 | input/output/total/cached 进入统一计费字段 | 步骤 1（含 PromptTokens 归一化） |
| 3 | 请求结束后正确产生 Redis 扣减记录 | 步骤 1+2（守卫自然放行） |
| 4 | `CLIENT_CLOSE,EOF` 不再阻止已完成请求扣费 | 步骤 2（`IsFinalUsageSeen` 置位） |
| 5 | Chat Completions、Anthropic 等现有计费不回归 | 精确匹配 + 顶层 `usage.*` 优先 + 第三节回归测试 |

## 六、实施记录（2026-09-22）

已按上述方案实施，变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_model_protocol/utils/usage_parse.go` | 抽出 `parseOpenAIUsageFieldsWithPrefix(data, prefix, leafIn, leafOut)`；`ParseOpenAIUsageFields` 在主链全零时回落 Responses 链：流式 `response.usage` 前缀、非流式顶层 `usage` 前缀（以 `input_token(s)_details` 存在为门控），命中后 `PromptTokens += CacheRead + CacheWrite` 归一化；缓存字段修正为 `input_tokens_details.cached_tokens` 优先、`input_token_details.cached_tokens` 兼容（image_tokens 同理） |
| `bfe_model_protocol/openai/stream.go` | `IsStreamTerminal`/`IsFinalUsageEvent` 精确增加 `response.completed`（issue #1381），其余事件行为不变 |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | 无改动（#1352/#1364 守卫按设计保持原样，`IsFinalUsageSeen` 置位后自然放行） |
| 单元测试 | `utils/usage_parse_test.go` 新增 4 例（流式/非流式/无缓存/Chat Completions 优先级 + delta 忽略）；`openai/stream_test.go` 两个表驱动用例增加 6 个事件臂；`content_quota_usage_test.go` 新增 `TestQuotaUsageProcessorProcessResponsesAPICompleted`、`TestQuotaUsageProcessorProcessResponsesAPIIncomplete` |
| 集成测试 | `tests/integration` SC03 新增 `TestTC18_RMBQuotaDeduction_ResponsesAPI_Streaming`、`TestTC19_RMBQuotaDeduction_ResponsesAPI_NonStreaming`（mode `responses`，input 2000 + cached 8000 + output 1500 → 扣减 900000 定点单位）；设计文档 `TC-18`/`TC-19` 及场景说明已同步 |

验证：`go test -cover`（bfe_model_protocol/...、bfe_modules/mod_body_process、mod_ai_token_auth、bfe_basic/...、bfe_server 等全部 ok）+ `go vet` 干净 + `gofmt` 干净；SC03 集成测试 TC-01~TC-19 全部通过。

> **后续更正（2026-09-24，issue #1389）**：本记录中"命中后 `PromptTokens += CacheRead + CacheWrite` 归一化"的实现已按 subset 语义删除（该归一化导致 cached_tokens 重复计费）；SC03 的 Responses fixture 与相关测试预期已同步更正，并新增 TC-21。详见 `2026-09-24-issue-1389-responses-api-cached-tokens-double-billing/design-changes.md`。

实施中对计划的偏离（均已在本文件步骤 1 标注）：

1. 计划假设非流式 usage 也在 `response.usage` 下；核对官方 API 形态后修正为非流式在顶层 `usage` 下、以 `input_token(s)_details` 门控进入，避免短路 Anthropic 跨协议兜底链。
2. leaf 名不同（`prompt/completion_tokens` vs `input/output_tokens`），前缀 helper 增加 leafIn/leafOut 参数而非纯前缀替换。
3. 发现并修正主链历史 `input_token_details`（单数）拼写，官方为 `input_tokens_details`（复数），两种拼写均兼容。
