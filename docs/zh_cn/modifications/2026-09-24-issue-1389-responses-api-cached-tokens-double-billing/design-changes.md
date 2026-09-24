# BFE Issue #1389 修复方案

- Issue: https://github.com/bfenetworks/bfe/issues/1389（源自 ai-gateway-api#206 的受控复现）
- 缺陷：BFE 对 OpenAI Responses API（`POST /compatible-mode/v1/responses`）的 `input_tokens_details.cached_tokens` 重复计费
- 代码库：`bfe/`（已对照当前代码逐条核实，行号基于 HEAD b1e98bbf）

## 一、缺陷描述与根因

### 现象

受控后端返回 Responses 最终 usage：`input_tokens=100, output_tokens=50, total_tokens=150, input_tokens_details.cached_tokens=20`。其中 `total_tokens=150 = 100 + 50`，自证 `input_tokens=100` 已含 cached 20（OpenAI subset 语义）。未配置 cache 价格时，实扣 `120×1000 + 50×2000 = 220000` 定点单位（1e-8 元），应扣 `100×1000 + 50×2000 = 200000`，多扣 `cached_tokens × input 价`。

### 根因：设计 #1381 前提错误，代码忠实实现

设计文档 `docs/zh_cn/modifications/2026-09-22-issue-1381-responses-api-billing-fix/design-changes.md` 步骤 1 语义点 1 明文规定：

> Responses API 的 `input_tokens` **不含** cached tokens（与 Anthropic `input_tokens` 同语义）……命中 Responses 链后按 Anthropic 链同款归一化：`PromptTokens = input_tokens + CacheReadTokens (+ CacheWriteTokens)`。

`ParseOpenAIUsageFields`（`bfe_model_protocol/utils/usage_parse.go:128-135`）忠实执行了该条款：

```go
// input_tokens excludes the cached tokens (Anthropic
// semantics): normalize PromptTokens to the total input count
// for the downstream cost splitting
// (prompt - cacheRead - cacheWrite).
resp.PromptTokens += resp.CacheReadTokens + resp.CacheWriteTokens
```

但该前提与 OpenAI 规范相悖。**OpenAI Responses API 是 subset 语义**：`input_tokens` 已含 `cached_tokens`，`total_tokens = input_tokens + output_tokens`。这与 Chat Completions（`prompt_tokens` 含 `prompt_tokens_details.cached_tokens`）、Gemini（`promptTokenCount` 含 `cachedContentTokenCount`）一致；只有 Anthropic 的 `input_tokens` 才是不含 cache 的 additive 语义。

### 为什么两种 cache 价格配置下都多扣

下游 `calcChatCost`（`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:634`）的契约是：`PromptTokens` 为输入总量，`CacheReadTokens`/`CacheWriteTokens` 为其子集（`bfe_basic/request_ai_basic.go:63` 注释："already included in PromptTokens"）。additive 归一化把 PromptTokens 虚增后：

| cache 价格配置 | 修复前行为 | 多扣金额 |
| --- | --- | --- |
| 未配置 | 不触发 cache 拆分（`mod_ai_token_auth.go:696`），虚增后的 120 全部按 input 价计费 | `cached × input 价`（issue 场景：20×1000=20000） |
| 已配置 | `normalInput = 120 - 20 = 100`，cached 20 既计入 normalInput 按 input 全价、又按 cache 价计一次 | 同样为 `cached × input 价` |

## 二、设计条款更正（语义裁定）

本方案正式裁定：**BFE 中 OpenAI Responses API 取 subset 语义**，更正 #1381 步骤 1 语义点 1 的前提。

| 协议链 | `input` 与 cached 关系 | 归一化处理 |
| --- | --- | --- |
| OpenAI Chat Completions | subset（`prompt_tokens` 含 cached） | 无归一化（现状正确） |
| **OpenAI Responses API** | **subset（`input_tokens` 含 cached）** | **删除 additive 归一化（本方案）** |
| Gemini | subset（`promptTokenCount` 含 cached） | 无归一化（现状正确） |
| Anthropic | additive（`input_tokens` 不含 cache read/write） | 保留 `:222` 归一化（现状正确） |

## 三、修复步骤

### 步骤 1（必改）：删除 Responses 链的 additive 归一化

文件：`bfe_model_protocol/utils/usage_parse.go`，`:128-135`

删除 `resp.PromptTokens += resp.CacheReadTokens + resp.CacheWriteTokens`，命中 Responses 链后直接 `fields = resp`，并把注释更正为 subset 语义（`input_tokens` 已含 cached，与主链 Chat Completions 一致，无需归一化）。

**注意**：同文件 `:222` 的 Anthropic 链归一化**保留不动**——Anthropic 的 `input_tokens` 确实不含 cache read/write，该归一化是正确且必要的。

### 步骤 2（必改）：更正单元测试预期

| 文件 / 用例 | 现状（错误预期） | 更正后 |
| --- | --- | --- |
| `bfe_model_protocol/utils/usage_parse_test.go` `TestParseOpenAIUsageFields_ResponsesAPICompleted` | `PromptTokens=140 (100+40)` | `PromptTokens=100`（即 `input_tokens`，已含 cached 40），同步更新注释 |
| 同文件 `TestParseOpenAIUsageFields_ResponsesAPINonStream` | `PromptTokens=90 (80+10)` | `PromptTokens=80`，同步更新注释 |
| `bfe_modules/mod_body_process/content_quota_usage_test.go` `TestQuotaUsageProcessorProcessResponsesAPICompleted` | `usage.PromptTokens=140 (100+40)` | `usage.PromptTokens=100` |

`ResponsesAPINoCache` / `ResponsesDeltaIgnored` / `ChatCompletionsUnchanged` 用例不受影响，保持原样（兼作无回归证明）。

### 步骤 3（必改）：集成测试 fixture 修正（TC-18 / TC-19 / TC-20）

文件：`bfe/tests/integration/implementation/scenario-SC03-rmb-quota/sc03_rmb_quota_test.go`

现有 fixture（`:1012-1018`）`input_tokens=2000, cached_tokens=8000` 在 subset 语义下**不合法**（cached 不可能大于 input），是当时按 additive 假设构造的。修正为合法 subset 形态：

```json
{"input_tokens": 10000, "output_tokens": 1500, "total_tokens": 11500,
 "input_tokens_details": {"cached_tokens": 8000}}
```

三个用例的期望扣减重算（价格 1e-6 input / 2e-6 output / 5e-7 cache_read，即 100 / 200 / 50 定点单位每 token）：

```
normalInput = 10000 - 8000 = 2000
cost = 2000×100 + 8000×50 + 1500×200 = 900000  （定点单位，与现状期望值一致）
```

TC-18/19/20 的 `want` 断言值不变，但注释中"input_tokens=2000 with cached_tokens=8000 normalizes PromptTokens to 10000"的错误说明必须改写为 subset 语义。

### 步骤 4（新增）：无 cache 价格回归用例 TC-21（issue 场景）

TC-18~20 均配置了 cache_read 价格，未覆盖 issue 的实际场景（未配置 cache 价格）。新增 `TestTC21_RMBQuotaDeduction_ResponsesAPI_NoCachePrice`：

- 配置：mode `responses`，仅 `input_cost_per_token=0.00001`、`output_cost_per_token=0.00002`，**不配置** cache 价格；
- fixture：`input_tokens=100, output_tokens=50, total_tokens=150, cached_tokens=20`（即 issue 受控 fixture）；
- 期望扣减：`100×1000 + 50×2000 = 200000` 定点单位——cached 20 已含在 input 100 内按 input 价计一次，不再叠加。

### 步骤 5（必改）：#1381 设计文档更正标注

在 `2026-09-22-issue-1381-responses-api-billing-fix/design-changes.md` 步骤 1 语义点 1 处追加更正说明：该前提已被本方案（issue #1389）裁定并更正为 subset 语义，避免后续维护者再按 additive 语义回改。#1381 的其余内容（解析链门控、`response.completed` 事件识别、#1352/#1364 守卫）均不受影响。

## 四、边界与影响面

- **修复面集中**：非流式（顶层 `usage` 门控臂）与流式（`response.usage` 臂）两条路径共用同一归一化点，步骤 1 一处改动全覆盖。
- **其他协议链不回归**：Chat Completions / DeepSeek / Gemini / Anthropic 解析链均不经过该归一化语句；Anthropic 归一化保留。
- **非标 relay**：若某 relay 对 Responses 形态返回 additive 语义的 usage（`input_tokens` 不含 cached），修复后将少计 cache 部分。BFE 以 OpenAI 官方规范语义为准（与既有 Chat Completions 链口径一致），此类非标形态不在支持范围。
- **CacheWrite**：OpenAI Responses API 无 cache-write 字段，删除归一化不影响 cache write 计费。
- **观测字段**：access log 中的 prompt tokens 恢复为上游真实 `input_tokens` 值（不再虚增），与上游口径一致。

## 五、验收标准对照（issue 原文）

| # | 验收标准 | 达成方式 |
| --- | --- | --- |
| 1 | `input=100/output=50/cached=20` 未配 cache 价格时扣减 200000 定点单位 | 步骤 1 + 步骤 4（TC-21） |
| 2 | subset 语义：`PromptTokens = input_tokens`（100），`CacheReadTokens = 20` | 步骤 1 + 步骤 2 |
| 3 | 已配 cache 价格时 `normalInput = input - cached`，不再对 cached 二次按 input 价计费 | 步骤 1 + 步骤 3（TC-18~20 重算） |
| 4 | Anthropic additive 语义不回归 | 步骤 1 保留 `:222` 归一化 + 既有用例全量回归 |

验证命令：`cd bfe && make test`（go test -cover ./... + go vet ./...），集成测试跑 SC03 全场景（TC-01~TC-21）。

## 六、实施记录（2026-09-24）

已按上述方案实施，变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_model_protocol/utils/usage_parse.go` | 删除 Responses 链 `PromptTokens += CacheRead + CacheWrite` additive 归一化（原 `:133`），注释更正为 subset 语义（issue #1389）；Anthropic 链归一化（`:222`）保留不动 |
| `bfe_model_protocol/utils/usage_parse_test.go` | `TestParseOpenAIUsageFields_ResponsesAPICompleted` 预期 140→100、`TestParseOpenAIUsageFields_ResponsesAPINonStream` 预期 90→80，注释同步更正 |
| `bfe_modules/mod_body_process/content_quota_usage_test.go` | `TestQuotaUsageProcessorProcessResponsesAPICompleted` 的 `usage.PromptTokens` 预期 140→100 |
| `tests/integration/implementation/scenario-SC03-rmb-quota/sc03_rmb_quota_test.go` | fixture `responsesStreamUsageResponse` / `responsesUsageResponse` 改为合法 subset 形态（`input_tokens=10000` 含 `cached_tokens=8000`，`total_tokens=11500`），`responsesAIConf` 注释按 subset 语义重算（扣减仍为 900000）；新增 `responsesNoCacheAIConf`、`responsesNoCacheUsageResponse` 与 `TestTC21_RMBQuotaDeduction_ResponsesAPI_NoCachePrice`（issue 受控 fixture 100/50/20，期望扣减 200000） |
| `tests/integration/测试设计文档/scenario-SC03-RMB配额扣减/` | TC-18/19/20 文档的 fixture 与"预期结果"计费说明更正为 subset 语义；新增 `TC-21-ResponsesAPI无缓存价格计费.md`；`场景说明.md` 补充 TC-21 要点 bullet 并补齐用例清单表 TC-18~TC-21 行 |
| `2026-09-22-issue-1381-responses-api-billing-fix/design-changes.md` | 步骤 1 语义点 1 追加更正标注（additive 前提裁定作废），第六节实施记录追加后续更正说明 |

验证：`gofmt` 干净、`go vet` 干净；`go test`（bfe_model_protocol/...、bfe_modules/mod_body_process、bfe_modules/mod_ai_token_auth）全部 ok；SC03 集成测试全场景 TC-01~TC-21 通过（54.6s，真实 bfe 进程 + 嵌入式 Redis，TC-21 实扣 200000 定点单位）。
