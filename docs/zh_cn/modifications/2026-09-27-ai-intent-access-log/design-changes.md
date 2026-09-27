# mod_ai_intent 访问日志字段落地（ai_intent_*）

> 日期：2026-09-27
> 上游设计：《意图访问日志设计.md》（v0.8/mod_ai_intent，字段语义与编号方案的
> 权威来源）、`../2026-09-26-mod-ai-intent/design-changes.md`（mod_ai_intent 模块本体）。
> 依赖：`bfe-access-pb` v0.3.9（已发布，proto 字段 803–809）。

---

## 1. 修改背景

`mod_ai_intent` 一期已上线（懒解析 + `req_ai_intent_in` 原语），但意图结果只进
了模块状态页计数器，未落访问日志。没有逐请求的 `(question, answer, confidence)`
记录，就无法做路由审计（"这个请求为什么路由到 model X"）和 `MinConfidence`
门限标定（当前 0.6 为占位值）。本次把意图结果接入既有 AI 访问日志链路：
模块写 `req` 上下文 → `mod_access_pb3` 统一回填 proto。

## 2. 修改内容

### 2.1 `go.mod`：升 bfe-access-pb v0.3.8 → v0.3.9

仅版本号变更，引入 proto 字段 803–809 的生成代码。

### 2.2 `bfe_basic/request_ai_intent.go`：记录路由实际消费的意图

`AiIntent` 新增两个字段，记录**首个被路由条件求值的意图问题**（命中与否都记录）：

```go
// in AiIntent:
ConsumedQuestion string   // first question evaluated by route conditions; "" if none
ConsumedUnknown  bool      // that answer was below threshold (derived at read time)
```

记录点在 `Match()`：`ConsumedQuestion` 为空时写入问题名（同一请求多次求值
只记首个，符合"路由实际消费"语义）。未知问题（未配置）不写——与 fail-safe
语义一致。

`AiIntent` 已有字段直接复用：`Source`（header/model/cache）、`LatencyMs`、
`QuestionsVersion`、`Answers[ConsumedQuestion].{Choice, AnswerConfidence}`。

### 2.3 `bfe_basic/request_ai_basic.go`：`AiBasicInfo` 不加字段

意图数据经 `req.GetAiIntent(req)` 按需读取（懒解析语义不变），不复制进
`AiBasicInfo`——避免双写漂移，`mod_access_pb3` 与路由条件读同一份数据。

### 2.4 `bfe_modules/mod_access_pb3/request_log.go`：回填 proto 字段

在现有 AI 字段回填段（`AiRouteRuleHits` 填充附近）新增：

| proto 字段（803–809） | 取值来源 | 说明 |
|---|---|---|
| `ai_intent_question` | `AiIntent.ConsumedQuestion` | 空 = 意图未启用/未求值，全组字段不写 |
| `ai_intent_answer` | 对应 `IntentAnswer.Choice`；`ConsumedUnknown` 时写 `"unknown"` | unknown 是常态结果，必须落日志（标定数据完整性） |
| `ai_intent_confidence` | `IntentAnswer.AnswerConfidence` | 门控后置信度 |
| `ai_intent_source` | `AiIntent.Source` 映射：`header→explicit_header`、`model→classifier`、`cache→cache` | 与 proto 文档枚举对齐 |
| `ai_intent_latency_us` | `AiIntent.LatencyMs × 1000` | cache/显式声明路径为 0 |
| `ai_intent_cache_hit` | `Source == "cache"` | |
| `ai_intent_questions_version` | `AiIntent.QuestionsVersion` | 配置回滚追溯 |

全部 optional，零值不写；`GetAiIntent` 返回 nil（未启用/未求值）时整组跳过。

### 2.5 测试

- `bfe_basic`：`request_ai_intent_test.go` 补 `Match()` 记录 Consumed 的用例
  （首个求值记录、重复求值不覆盖、未配置问题不写）；
- `mod_access_pb3`：`request_log_test.go` 补意图字段回填断言（含 unknown、
  cache 源、未求值三种形态）；
- 集成测试：新增场景（SC05 风格）起 bfe + 决策服务 shim，意图路由请求落日志
  后解析断言 `ai_intent_*`；未启用模块的请求断言字段为空。

## 3. 不改动的部分

- 路由行为零变化：本改动只读意图结果、写日志；
- 不记录全量概率分布（体积考虑，仅 debug 日志可见）；
- 不记录请求正文/state 原文；
- 一期按"路由实际消费的意图"记单条；未来如需全量问题答案，proto 升级为
  `repeated` 新消息并走新号段（803–809 已发布，不得复用）。

## 4. 兼容性

- proto 字段向前兼容：旧版 b2log 解析器跳过未知字段；
- 未启用 `mod_ai_intent` 的部署：日志中本组字段恒空，采集链路无感；
- 下游 Doris 表加列（ai-gateway-observability 仓），与 proto 字段名一致。
