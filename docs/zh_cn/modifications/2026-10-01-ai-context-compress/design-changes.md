# `mod_ai_context` 上下文压缩与裁剪（一期：无损裁剪 + 规则改写）

## 1. 背景

本文档是上下文压缩与裁剪需求在 BFE 数据面的落地方案。总体设计（需求分析、竞品拆解、决策理由）见《上下文压缩与裁剪设计.md》，本文只描述 **bfe 数据面的新增与改造点**。

**动机**：bfe AI 网关无内置 token 压缩（竞品分析见 OmniRoute-vs-bfe 对比 §5.5）；长会话/Agent 场景 input token 占成本大头，且超模型上下文窗口直接报错。本功能在转发上游前对 OpenAI 协议 `messages` 做减量处理。

**一期目标**：

- 请求转发前按目标模型上下文窗口做 token 预算预检，超阈值主动触发，避免 `prompt too long` / `context_length_exceeded`；
- 两档能力三档模式：`conservative`（仅无损裁剪）、`balanced`（+ lite 规则改写）、`aggressive`（+ full 规则改写），按 product/路由规则配置；
- 透明改写：压缩后重序列化转发，客户端零改造，流式/非流式语义不变；
- 全程 fail-open：body 未缓冲完、解析失败、fidelity gate 不过、协议修复失败，一律放行原始请求；
- 压缩前后 token 数、档位、跳过原因全部可观测。

**明确不做（一期非目标）**：

| 不做的事 | 说明 |
|----------|------|
| LLM 会话摘要（管线 P3） | 二期扩展点（外部 OpenAI 兼容摘要服务），一期 `aggressive` = 无损裁剪 + full 强度改写 |
| token 级语义压缩（LLMLingua 类） | 需引入推理依赖，运维成本高；一期以规则式改写替代 |
| Anthropic / Gemini 协议改写 | 一期只处理 OpenAI chat completions；其他协议识别后跳过 |
| 多模态图像降采样/转文本 | 一期仅"旧图片裁剪"（保留最近 N 张） |
| 规则级 `override` 与 Defaults 内 `summary` 配置 | 二期与 P3 同期引入；一期加载器按未知字段忽略 + 计数告警，保证二期配置在旧数据面安全降级 |

**选型决策摘要**（详见总体设计文档第 4 节）：

- **插入点选 `HandleAfterAITargetModel`**（`bfe_server/reverseproxy.go:1582`，目标模型解析后、转发前）：此时 `TargetModel` 已知可做预算；缓存命中已在 `HandleAfterLocation` 短路返回，命中路径不经过压缩；改写对象是该点已有的 `OutRequest`，原始 `HttpRequest` 不动（访问日志、计费播种仍见原始请求）；
- **改写机制零新框架**：复用 `BodyAccessor.GetBytes → 处理 → SetBytes → ContentLength=-1` 既有链路（范本 `bfe_server/reverseproxy.go:1597` model 改写、`bfe_basic/condition/primitive.go:1222` `ReqBodyJsonSet`）；
- **无精确 tokenizer**：抽象 `TokenEstimator` 接口，默认启发式实现（`len(json)/4` 字节/token、内联图片固定估值 1200 token，系数 Defaults 可配热更），二期可换 tiktoken 类实现；
- **fallback 幂等守卫**：`HandleAfterAITargetModel` 每次 cluster attempt 触发，用 `AiBasicInfo` 状态位保证单次处理。

## 2. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 新模块 | `mod_ai_context`：Init、回调注册、状态记录 | `bfe/bfe_modules/mod_ai_context/mod_ai_context.go`（新建） |
| 模块配置 | INI 解析（[basic]/[log]） | `bfe/bfe_modules/mod_ai_context/conf_mod_ai_context.go`（新建） |
| 规则模型 | 规则表 Search/Match、rule.data 加载、Defaults 块、前向兼容未知字段告警 | `bfe/bfe_modules/mod_ai_context/context_rule_table.go`、`context_rule_load.go`（新建） |
| token 估算 | `TokenEstimator` 接口 + `HeuristicEstimator`（参数每次调用从规则参数构造，Init 不固化） | `bfe/bfe_modules/mod_ai_context/estimate.go`（新建） |
| 协议解析 | OpenAI messages 解析/重序列化（content 兼容 string/typed-parts）、协议合法性校验 | `bfe/bfe_modules/mod_ai_context/messages_parse.go`（新建，解析先例见 `mod_ai_intent/msg_extract.go:62`） |
| 裁剪层 | L1 工具结果截断 / L1.5 旧图片裁剪 / L2 thinking 删除 | `bfe/bfe_modules/mod_ai_context/trim.go`（新建） |
| 改写层 | tombstone 保护/还原、冗余改写规则包（含中文）、fidelity gate | `bfe/bfe_modules/mod_ai_context/rewrite.go`、`fidelity.go`（新建） |
| 结构保真 | tool_call/tool 配对修复、末尾孤儿清理、修复失败整体回滚 | `bfe/bfe_modules/mod_ai_context/repair.go`（新建） |
| 管线编排 | 降级管线 L1→L1.5→L2→P2，每步后重估，达标即出 | `bfe/bfe_modules/mod_ai_context/pipeline.go`（新建） |
| 请求阶段 | `contextCompressHandler`（HandleAfterAITargetModel）：规则匹配/协议判定/body 读取/估算/预算/管线/改写 | `bfe/bfe_modules/mod_ai_context/handler.go`（新建） |
| 响应标注 | `HandleReadResponse` 过滤器：压缩生效时加 `x-ai-context-compression` 响应头 | `bfe/bfe_modules/mod_ai_context/handler.go`（新建） |
| 基础信息 | `AiBasicInfo` 新增压缩状态/前后 token/档位字段（兼作 fallback 幂等守卫） | `bfe/bfe_basic/request_ai_basic.go`（修改） |
| 模块注册 | `moduleList` 新增 `mod_ai_context`（位置见 §3） | `bfe/bfe_modules/bfe_modules.go`（修改） |
| 监控 | 模块计数器与 web_monitor 端点 | `bfe/bfe_modules/mod_ai_context/context_state.go`（新建） |
| 访问日志 | protobuf 新增 4 字段（793–796，接入时与 bfe-access-pb 仓库对齐编号）；`mod_access_pb3` 取值 | `bfe-access-pb`（独立仓库）、`bfe/bfe_modules/mod_access_pb3/request_log.go`（修改） |
| 配置样例 | `mod_ai_context.conf` + `context_rule.data` 样例 | `bfe/conf/mod_ai_context/`（新建） |
| 文档 | 模块配置文档 | `bfe/docs/zh_cn/configuration/mod_ai_context/`（新建） |
| 测试 | 单元测试 + 集成测试场景 | `bfe/bfe_modules/mod_ai_context/*_test.go`（新建）；`bfe/tests/integration/implementation/scenario-SCxx-ai-context-compress`（新建） |

`bfe_module` 框架、`bfe_server/reverseproxy.go` 转发链路、`mod_ai_cache`/`mod_ai_token_auth`/`mod_ai_rate_limit`/`mod_body_process` 等既有模块**零改动**。

## 3. 模块注册位置

`bfe/bfe_modules/bfe_modules.go` 的 `moduleList` 中，注册于 `mod_ai_rate_limit` 之后、`mod_access_pb3` 之前：

```go
// mod_ai_context
// Requirement: after mod_ai_route (needs resolved TargetModel for token
// budget; callback is HandleAfterAITargetModel which fires after model
// resolution); after mod_ai_cache (a cache hit short-circuits at
// HandleAfterLocation, so compression never runs on the hit path and cache
// keys are unaffected); after mod_ai_rate_limit (rate limiting is decided
// on the uncompressed request context); before mod_access_pb3 (log fields
// must be set before access logging).
mod_ai_context.NewModuleAiContext(),
```

回调注册（模块 Init 内）：

```go
err = cbs.AddFilter(bfe_module.HandleAfterAITargetModel, m.contextCompressHandler) // 压缩主逻辑
err = cbs.AddFilter(bfe_module.HandleReadResponse, m.responseAnnotationHandler)    // 响应头标注
```

## 4. 配置模型

### 4.1 模块基础配置 `conf/mod_ai_context/mod_ai_context.conf`

INI 刻意只保留**引导类静态配置**（规则文件路径、日志开关），不放任何业务调参——`mode` 是规则语义（放规则条目且必填），调优参数与估算系数统一放 rule.data `Defaults` 块热更：

```ini
[basic]
ProductRulePath = ../conf/mod_ai_context/context_rule.data

[log]
OpenDebug = false
```

| 配置项 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
|--------|------|----------|------|----------|------------|
| Basic.ProductRulePath | String | 规则 data 文件路径 | Y | - | 非空字符串 |
| Log.OpenDebug | Boolean | 模块调试日志 | N | 默认 false | - |

### 4.2 规则文件 `context_rule.data`

顶层 `Defaults` 块承载全部运行调优参数（热更）；规则条目只写 `cond + mode + 可选预算覆盖`：

```json
{
  "Version": "1.0",
  "Defaults": {
    "triggerRatio": 0.7,
    "keepLatestImages": 2,
    "toolResultMaxChars": 2000,
    "thinkingPolicy": "trim-all-but-last",
    "charsPerToken": 4,
    "imageTokenEstimate": 1200,
    "rewrite": {
      "strength": "lite",
      "protectedSurvivalRate": 0.95
    }
  },
  "Config": {
    "default": [
      { "cond": "default_t()", "mode": "balanced" },
      { "cond": "req_ai_intent_in(\"long-doc-chat\")", "mode": "aggressive" }
    ]
  }
}
```

**规则级字段**：

| 字段 | 类型 | 参数含义 | 必填 | 合法性条件 |
|------|------|----------|------|------------|
| cond | String | bfe 条件表达式 | Y | 编译失败整文件拒绝加载（复用 `mod_ai_cache` 规则模型） |
| mode | String | `off` / `conservative` / `balanced` / `aggressive` | Y | 缺失或非法值拒绝加载 |
| maxContextTokens | Integer | 预算上限覆盖 | N | ≥0，缺省/0 = 用模型表窗口 |
| reserveTokens | Integer | 预留输出 token | N | ≥0，缺省/0 = 自动 `clamp(窗口×15%, 256, 16000)` |

**Defaults 块字段**：

| 字段 | 类型 | 参数含义 | 默认 | 合法性条件 |
|------|------|----------|------|------------|
| triggerRatio | Float | proactive 触发阈值（占预算比例） | 0.7 | (0, 1] |
| keepLatestImages | Integer | 保留最近 N 张内联图片 | 2 | ≥0，0=不裁图 |
| toolResultMaxChars | Integer | 单条 tool 结果最大字符数 | 2000 | ≥0，0=不截断 |
| thinkingPolicy | String | thinking 块策略 | trim-all-but-last | 枚举：`trim-all-but-last` / `keep` |
| charsPerToken | Integer | 文本估算系数（字节/token） | 4 | ≥1，中文密集场景可调 3 |
| imageTokenEstimate | Integer | 单张内联图片估值 token | 1200 | ≥0 |
| rewrite.strength | String | 改写强度 | lite | 枚举：`lite` / `full` |
| rewrite.protectedSurvivalRate | Float | fidelity gate 保护 token 存活率阈值 | 0.95 | (0, 1] |

### 4.3 校验与热加载

- 校验规则：mode 必填且枚举合法；数值字段范围校验；cond 编译失败整文件拒绝加载；
- **前向兼容（一期关键行为）**：对规则条目及 Defaults 中的**未知字段忽略并计数告警**（`CTX_CFG_UNKNOWN_FIELD`），不拒绝加载——二期引入的 `override`（规则级）与 `summary`（Defaults 块内）配置下发到旧数据面时安全降级；二期引入这两个字段后，未知键改为严格校验（拒绝加载）；
- 热加载：遵循 `mod_ai_cache` 先例——规则文件走 File/Mem 双类型 + web_monitor 端点 reload，变更只影响新请求。

## 5. 模块文件结构

```text
bfe/bfe_modules/mod_ai_context/
├── mod_ai_context.go          # 模块入口：Name/Init、回调注册
├── conf_mod_ai_context.go     # INI 配置解析（basic + log）
├── context_rule_table.go      # 规则表：Search(product) + Match(req)
├── context_rule_load.go       # rule.data 加载、Defaults 解析、校验、前向兼容告警
├── context_state.go           # 计数器 + web_monitor 端点
├── estimate.go                # TokenEstimator 接口 + HeuristicEstimator
├── messages_parse.go          # OpenAI messages 解析/重序列化/合法性校验
├── handler.go                 # contextCompressHandler + responseAnnotationHandler
├── pipeline.go                # 降级管线编排：L1→L1.5→L2→P2，每步后重估
├── trim.go                    # 裁剪层：工具结果截断、旧图片裁剪、thinking 删除
├── rewrite.go                 # 改写层：tombstone 保护/还原、冗余改写规则包
├── fidelity.go                # 质量门：保护 token 存活率校验
├── repair.go                  # 结构修复：tool_call/tool 配对、孤儿清理
└── *_test.go                  # 单元测试（testing + testify）
```

## 6. 关键接口设计

### 6.1 TokenEstimator（`estimate.go`）

```go
type EstimateParams struct {
    CharsPerToken     int   // 来自合并后的规则参数，每次调用传入，Init 不固化
    ImageTokenEstimate int
}

type TokenEstimator interface {
    // EstimateMessages 对整个 messages 数组估算 token（含图片占位估值）
    EstimateMessages(msgs []Message, p EstimateParams) int64
    EstimateText(s string, p EstimateParams) int64
}
```

默认实现 `HeuristicEstimator`（对齐 OmniRoute `estimateTokens` 与 bfe 现有惯例 `GetPromptToken` 的 len/4）：文本 `len(json.Marshal(msg))/CharsPerToken`；内联 base64 图片不按原文计，每张固定 `ImageTokenEstimate`。二期可实现 `TiktokenEstimator` 按规则配置切换，接口不变。

### 6.2 messages 模型（`messages_parse.go`）

对标 `mod_ai_intent/msg_extract.go:62` 的解析先例：content 兼容 string 与 typed-parts 数组；解析后产出 `[]Message`（role/content/tool_calls/tool_call_id 等 OpenAI 字段）。重序列化用 `json.Marshal` 后整体 `SetBytes`，不做 sjson 逐路径改写（压缩会改数组长度，sjson 路径方式不适用）。

## 7. 请求阶段设计（`handler.go`）

### 7.1 处理流程

```text
contextCompressHandler (HandleAfterAITargetModel)
    ① 幂等守卫：AiBasicInfo.ContextCompressStatus != "" → BfeHandlerGoOn
       （该回调每次 cluster attempt 触发，fallback 不得重复压缩）
    ② 规则匹配：无规则命中或 mode=off → 记 SKIP_NO_RULE
    ③ 协议判定：非 OpenAI chat completions → SKIP_PROTOCOL
    ④ body 读取：req.OutRequest.GetBodyAccessor()；
       GetBytes() 未读全（all=false，超出 AccessibleBodySize/全局上限）→ SKIP_BODY_INCOMPLETE
    ⑤ 解析 messages：失败 → SKIP_PARSE_ERR（以上均 fail-open 放行原始请求）
    ⑥ 预算：budget = min(modelWindow(TargetModel), maxContextTokens) − reserveTokens
       modelWindow 优先级：模型表 context_window → 模型名启发（claude 200k /
       gemini 1M / gpt·codex 400k）→ 默认 128k；窗口兜底记 WINDOW_DEFAULTED
    ⑦ 估算 ≤ budget × triggerRatio → SKIP_UNDER_THRESHOLD（BfeHandlerGoOn，零改写）
    ▼ 超阈值（保留原始 body 副本用于回滚）
    ⑧ 裁剪层 L1→L1.5→L2（conservative 止于此）：每步后重估，≤ budget 即出
    ⑨ 改写层 P2（mode ≥ balanced）：tombstone 保护 → 冗余改写 → 还原 →
       fidelity gate 校验 → 重估，≤ budget 即出
       fidelity gate 不过 → 回滚 P2，以裁剪层产物出（记 GATE_FALLBACK）
    ⑩ repair.go 结构修复 + 协议合法性校验：仍非法 → 整体回滚原始 body
       （记 REPAIR_ROLLBACK，fail-open）
    ⑪ 重序列化：bodyAccessor.SetBytes(newBody, false)
       req.OutRequest.ContentLength = -1；删除 Content-Length 头
       （缺一转发截断，范本 reverseproxy.go:1597-1610）
    ⑫ 写 AiBasicInfo：状态/前后 token 数/档位/耗时 → BfeHandlerGoOn
```

### 7.2 关键伪代码

```go
func (m *ModuleAiContext) contextCompressHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
    basic := req.GetContext(...).(*bfe_basic.AiBasicInfo) // __REQ_AI_BASIC_CONTEXT
    if basic.ContextCompressStatus != "" {
        return bfe_module.BfeHandlerGoOn // fallback 幂等守卫
    }
    rule, params, ok := m.ruleTable.Match(req)
    if !ok || rule.Mode == ModeOff {
        basic.ContextCompressStatus = StatusSkipNoRule
        return bfe_module.BfeHandlerGoOn
    }
    // 协议判定 → body 读取(all=false 即 skip) → 解析(失败即 skip) → 预算与估算
    if estimated <= budget*params.TriggerRatio {
        basic.ContextCompressStatus = StatusSkipUnderThreshold
        return bfe_module.BfeHandlerGoOn
    }
    original, _ := bodyAccessor.GetBytes()
    msgs := parseMessages(body) // string/typed-parts 兼容
    result := m.pipeline.Run(msgs, rule, params, budget) // L1→L1.5→L2→P2，逐步重估
    if !result.legalAfterRepair {                          // repair.go 修复后仍非法
        basic.ContextCompressStatus = StatusRepairRollback // fail-open：不改写
        return bfe_module.BfeHandlerGoOn                   // original 未动
    }
    newBody, err := json.Marshal(result.messages)
    if err != nil { /* 同 rollback 路径 */ }
    bodyAccessor.SetBytes(newBody, false)
    req.OutRequest.ContentLength = -1
    req.OutRequest.Header.Del("Content-Length")
    basic.ContextCompressStatus = result.stage
    basic.ContextTokensBefore, basic.ContextTokensAfter = estimated, result.estimated
    basic.ContextCompressMode = rule.Mode
    return bfe_module.BfeHandlerGoOn
}
```

### 7.3 结构保真（不可妥协项，`repair.go`）

- **最新消息不动**：最后一条 user（含多模态内容）永不压缩/裁剪——`mod_ai_cache` 取最后 user 问题做 key，保证缓存 key 稳定；
- **system prompt 冻结**：默认不裁不改，仅 P2 允许冗余措辞精简（可配关闭）；
- **tool_call/tool 成对**：裁剪可能拆散 `assistant.tool_calls` 与其后的 `tool` 消息——删除孤儿 tool 消息、剥离悬空 tool_calls（多趟清理，对齐 OmniRoute `fixToolPairs`/`fixToolAdjacency`）；修复后整体校验（messages 非空、role 序列合法），**仍非法则整体回滚原始 body**；
- **多模态结构合法**：图片裁剪后 content parts 非空、类型字段完整；
- **幂等防重压**：P2 产物打 `[COMPRESSED:...]` 标记，重试命中标记跳过。

### 7.4 降级管线参数（`pipeline.go`）

| 步骤 | 动作 | 类型 | 参数来源 |
|------|------|------|----------|
| L1 | tool/tool_result 结果截断到 `toolResultMaxChars` + `...[truncated]` | 无损 | Defaults |
| L1.5 | 内联图片只留最近 `keepLatestImages` 张，旧图替换为占位文本 | 无损 | Defaults |
| L2 | 删除除最后一条 assistant 外所有 thinking 块 | 无损 | `thinkingPolicy` |
| P2 | tombstone 保护（代码块/行内代码/URL/路径/版本号/JSON key/数字字面量）→ 规则改写 → 还原 → fidelity gate | 有损 | `rewrite.*` |

## 8. 响应阶段设计（`handler.go`）

压缩只发生在请求侧，流式/非流式响应透传语义不变。仅新增标注过滤器：

```go
func (m *ModuleAiContext) responseAnnotationHandler(req *bfe_basic.Request, res *bfe_http.Response) (int, *bfe_http.Response) {
    basic := ... // AiBasicInfo
    if isCompressDone(basic.ContextCompressStatus) && res != nil {
        res.Header.Set("x-ai-context-compression",
            fmt.Sprintf("tokens=%d->%d; mode=%s", basic.ContextTokensBefore,
                basic.ContextTokensAfter, basic.ContextCompressMode))
    }
    return bfe_module.BfeHandlerGoOn, nil
}
```

`HandleReadResponse` 在响应头回传前触发，加头对客户端可见，便于验证与排障；未压缩请求不加。

## 9. 配额/计费协同

- `mod_ai_token_auth` 配额预检在 `HandleFoundProduct` 读**原始** body 估算（`GetPromptToken`，`mod_ai_token_auth.go:535`），发生在压缩前：只可能高估、不会误拒，无需改动；
- 最终扣费以响应 `usage` 为准（`mod_body_process` 解析），压缩后 usage 变小自动少扣，无需改动；
- `AiBasicInfo` 新增 `ContextTokensAfter`（压缩后估算 prompt token）仅供审计与对账参考，不回写计费。

`AiBasicInfo` 新增字段（`bfe_basic/request_ai_basic.go`，新起一段注释）：

```go
// AI context compress (mod_ai_context) result
ContextCompressStatus string // "" = 未处理; skip_* / trim / rewrite / rollback（兼作 fallback 幂等守卫）
ContextTokensBefore   int64  // 压缩前估算 token（启发式估算）
ContextTokensAfter    int64  // 压缩后估算 token，未压缩为 0
ContextCompressMode   string // 生效档位：conservative / balanced / aggressive
```

## 10. 访问日志与监控

### 10.1 访问日志字段

`bfe-access-pb`（独立仓库）proto 新增 4 字段（793–796，已在 `bfe-access-pb` 落地并重新生成 pb.go）：

| 字段 | tag | 类型 | 含义 |
|------|-----|------|------|
| ai_context_compress_status | 793 | enum | 未触发/已裁剪/已改写/跳过（带原因码）/回滚 |
| ai_context_tokens_before | 794 | int64 | 压缩前估算 token |
| ai_context_tokens_after | 795 | int64 | 压缩后估算 token |
| ai_context_compress_mode | 796 | string | 生效档位 |

`bfe/bfe_modules/mod_access_pb3/request_log.go` 从 `AiBasicInfo` 取值（修改）。

### 10.2 模块监控指标（`context_state.go`）

| 计数器 | 含义 |
|--------|------|
| CTX_TOTAL | 进入回调的请求总数 |
| CTX_SKIP_NO_RULE / CTX_SKIP_PROTOCOL / CTX_SKIP_BODY_INCOMPLETE / CTX_SKIP_PARSE_ERR / CTX_SKIP_UNDER_THRESHOLD | 各跳过原因 |
| CTX_TRIGGERED | 实际触发管线的请求数（按 mode 分 label） |
| CTX_DONE_TRIM / CTX_DONE_REWRITE | 管线出口阶段（CTX_DONE_SUMMARY 为二期） |
| CTX_GATE_FALLBACK / CTX_REPAIR_ROLLBACK | 降级/回滚事件（CTX_SUMMARY_ERR 为二期） |
| CTX_WINDOW_DEFAULTED | 模型窗口未配置走默认的次数 |
| CTX_CFG_UNKNOWN_FIELD | 未知配置字段被忽略的次数（前向兼容告警） |
| CTX_LATENCY_MS | 管线耗时分布（裁剪层应 <1ms） |

## 11. 非功能性设计

- **fail-open 优先**：压缩是优化手段不是正确性手段——宁可不压不可压坏；所有跳过/回滚路径只记状态与计数器，不改写 body；
- **性能**：裁剪层纯字符串操作 <1ms；改写层规则匹配为线性扫描；估算为 json.Marshal + 长度计算，超大 body 已受 `AccessibleBodySize` 限制天然封顶；
- **内存**：管线内 messages 对象与 body 副本同量级，处理完即释放；不做跨请求缓存；
- **新源文件**加 Apache 2.0 license 头（`make license-fix` 可自动补）；
- **配置样例**落到 `conf/mod_ai_context/`，保证 `make package` 与 Docker 构建产出可用默认配置。

## 12. 开发任务拆分（WBS）

| # | 任务 | 产出 | 依赖 |
|---|------|------|------|
| 1 | 模块骨架：INI 解析、规则表、web_monitor 端点、moduleList 注册（含顺序注释） | 模块可加载、规则可热更 | - |
| 2 | 估算器 + messages 解析/序列化 + 预算计算 | 估算与预算单测通过 | 1 |
| 3 | 裁剪层 L1–L2 + repair + 单测 | conservative 档可用 | 2 |
| 4 | 改写层 P2 + tombstone + fidelity gate + 中文规则包 | balanced / aggressive 档可用 | 3 |
| 5 | 可观测：计数器、访问日志字段、`x-ai-context-compression` 响应头 | 指标上线 | 3 |
| 6 | 集成测试场景 `scenario-SCxx-ai-context-compress` | 场景测试绿 | 3 |
| 7 | ai-gateway-api 控制面（DDL/InnerAPI/OpenAPI）与模型表 `context_window` 字段 | 控制台可配（独立排期） | 1 |
| 8 | **二期** P3 会话摘要，同期引入 `summary`/`override` 配置与严格校验 | aggressive 档完整 | 4 |

## 13. 测试方案

### 13.1 单元测试重点

- 估算器：文本/图片/中文系数、`EstimateParams` 传入生效；
- messages 解析：string 与 typed-parts content、tool_calls、多模态 parts；
- 裁剪层：截断长度与标记、keepLatestImages 边界（0/1/N）、thinkingPolicy 两分支；
- 改写层：tombstone 保护块原样还原、fidelity gate 通过与回退；
- repair：孤儿 tool、悬空 tool_calls、多趟修复、修复失败回滚；
- 规则加载：mode 必填校验、未知字段告警计数（前向兼容）、cond 编译失败拒载；
- 预算：maxContextTokens/reserveTokens 覆盖、窗口兜底优先级。

### 13.2 集成测试（`bfe/tests/integration/implementation/scenario-SCxx-ai-context-compress`）

- mock 后端校验收到的 body：messages 已改写、`Content-Length` 正确（转发不截断）；
- 触发阈值上下边界：恰好低于/高于 triggerRatio；
- fallback 重试：同一请求两次 cluster attempt 只压缩一次（幂等守卫断言）；
- 与 `mod_ai_cache` 串联：缓存命中路径零压缩；压缩后请求仍可按原 key 命中缓存；
- fail-open：body 未读全/解析失败路径直接透传原始 body。

## 14. 风险与应对

| 风险 | 应对 |
|------|------|
| 启发式估算误差导致"该压没压/压过头" | 触发线留 30% 余量（triggerRatio 默认 0.7）；优先在模型表配置精确窗口；上线后按 `CTX_WINDOW_DEFAULTED` 补全 |
| 规则改写损伤语义（代码/JSON 被破坏） | tombstone 强制保护 + fidelity gate 存活率校验，不过即回退裁剪层无损产物 |
| fallback 重试重复压缩 | `AiBasicInfo.ContextCompressStatus` 幂等守卫 + `[COMPRESSED]` 产物标记双保险 |
| 压缩与缓存叠加导致命中率下降 | 理论上不影响（key 取自未触碰的最后 user 消息）；灰度期对比 `mod_ai_cache` 命中率指标验证 |

## 15. 参考文档

- 总体设计：《上下文压缩与裁剪设计.md》——需求、竞品拆解（OmniRoute `contextManager.ts`/`compression/`）、决策理由、灰度路径
- 需求分析：《上下文压缩与裁剪需求分析.md》
- 改造范本：`2026-09-24-ai-cache-exact-match`、`2026-09-30-ai-cache-semantic-cache`（模块结构/控制面/可观测格式）
- 回调点设计记录：`2026-09-23-issue-1387-sister-rate-limit-models-target-model`（`HandleAfterAITargetModel`）
- body 改写范本：`bfe_server/reverseproxy.go:1597`（model 改写 + Content-Length 修正）、`bfe_basic/condition/primitive.go:1222`（`ReqBodyJsonSet`）
