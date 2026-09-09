# BFE 协议适配层（model_protocol adapter）

## 1. 背景与目标

### 1.1 背景

BFE AI 网关按 **model_protocol**（模型访问协议）适配上游差异。在协议适配层（`bfe/bfe_model_protocol/`）引入之前，协议知识散落在 4 个包中：

- 协议识别：`bfe_basic/request_ai_basic.go`（`GetApiKey` / `DetectAuthStyle` 硬编码 if-else）；
- 协议校验：`bfe_server/reverseproxy.go`（`clusterSupportsAuthStyle`）；
- 认证头/版本头注入：`mod_ai_token_auth`（`SetApiKey`）+ `reverseproxy.go`（硬编码 `anthropic-version`）；
- usage 字段归一：`mod_ai_token_auth`（`UpdateCtxByUsage`）与 `mod_body_process`（`SSEEvent.GetQuotaUsage` / `RawEvent.GetQuotaUsage`，三份重复链）。

这导致：协议知识无实体、usage 归一多份重复（扣费链路易不一致）、新增协议需侵入 5 个包。

### 1.2 目标

1. 按协议聚合全部协议知识到 `bfe/bfe_model_protocol/<protocol>/` 适配器目录；
2. 收敛重复的 usage 字段链为一份，累加语义留在原调用方；
3. 新增一种协议 = `detect.go` 一条识别规则 + 一个适配器目录 + registry 注册一行，不改框架代码；
4. 新增一个 OpenAI 兼容 provider（Groq 等）零代码，仅配置 `model_protocols: ["openai"]`。

修改方案与实施记录见 `bfe/docs/zh_cn/modifications/2026-09-03-model-protocol-adapter/design-changes.md`。

---

## 2. 概念模型：model_protocol ≠ provider

**适配轴心是 model_protocol，不是 provider**：

- provider 是**用户自定义的 cluster 级上游实体**（名称/key 池/价格表/地址），由 cluster + `AIConf` 承载，控制面（ai-gateway-api）管理；
- model_protocol 是**协议知识维度**：认证头怎么注入、补什么版本头、usage 字段长什么样、错误什么格式。OpenAI 兼容生态中 Groq、DeepSeek、OpenRouter 等品牌均走 `openai` 协议，无需为品牌建适配器。

这一边界继承自 Claude 协议支持的设计决策（见 `claude_protocol_support.md`）：provider 名称是用户自定义的（如 `my-anthropic`），不能用于推断协议；协议能力只能由显式的 `AIConf.ModelProtocols` 表达。

参考 Bifrost（`bifrost/core/providers/<name>/`）时只借鉴其形态（按协议目录组织、编译期注册、共享 utils 下沉），**不照搬**其 30+ 方法的 Provider 接口（BFE 是透传式网关，不需要每操作一个方法）与全量协议翻译（BFE 对请求/响应体不做协议转换）。

---

## 3. 核心设计

### 3.1 包结构

`bfe/bfe_model_protocol/`（与 `bfe_modules/` 平级）：

```
bfe/bfe_model_protocol/
├── protocol.go          # ProtocolAdapter 接口 + 协议常量
├── registry.go          # 编译期注册 + Get / Supports / ValidateProtocols
├── usage.go             # UsageFields 等类型的根包 alias（实体在 utils）
├── errors.go            # ProtocolError / ErrorNormalizer 的根包 alias
├── detect.go            # 协议识别（DetectProtocol / DetectProtocolAndKey）
├── utils/               # 叶子包：仅依赖 gjson + 标准库，实体类型所在
│   ├── protocol.go      # 协议常量、UsageFields
│   ├── usage_parse.go   # ParseOpenAIUsageFields / ParseAnthropicUsageFields / EstimateContentToken
│   └── errors.go        # ProtocolError、ErrorNormalizer、DefaultErrorNormalizer
├── openai/              # openai 兼容协议族
│   ├── openai.go        # ProtocolAdapter 实现
│   ├── auth.go          # Bearer 认证（导出供协议族复用）
│   └── usage.go         # OpenAI + DeepSeek + Responses 系 usage 提取
├── anthropic/           # anthropic messages 协议
│   ├── anthropic.go     # ProtocolAdapter 实现
│   ├── auth.go          # x-api-key + anthropic-version
│   └── usage.go         # Claude usage 提取（cache read/write 归一）
└── gemini/              # gemini generateContent 协议（2026-09-09 接入）
    ├── gemini.go        # ProtocolAdapter 实现
    ├── auth.go          # x-goog-api-key
    └── usage.go         # usageMetadata（camelCase）提取与归一
```

**类型放在 `utils` 叶子包、根包以 type alias 暴露**：`registry.go`（根包）需 import 子包做编译期注册，而子包需引用 `UsageFields`/`ProtocolError`——若实体在根包则形成 import cycle；alias 保证调用方 API 不变。`utils` 叶子包不依赖任何 BFE 业务包。

**依赖方向约束**：`bfe_model_protocol` 只允许 import `bfe/bfe_http` 与 gjson；不许 import `bfe_basic` / `bfe_config` / `bfe_modules`（否则出现环：bfe_basic 的识别函数委托 detect.go）。

### 3.2 接口

```go
const (
    ProtocolOpenAI    = "openai"
    ProtocolAnthropic = "anthropic"
    ProtocolUnknown   = "unknown"
)

// ProtocolAdapter 承载一种 model_protocol 的全部协议知识。
// 适配器为无状态单例，由编译期注册表持有；per-cluster 差异（用哪把 key）
// 通过方法参数传入，不在适配器内保存。
type ProtocolAdapter interface {
    Key() string

    // InjectAuth 向出站请求注入上游凭证（key 为 AIKey.Key 字符串）
    InjectAuth(outreq *bfe_http.Request, key string) error

    // ExtraHeaders 协议级补充 header（如 anthropic-version），无则返回 nil
    ExtraHeaders() map[string]string

    // ExtractUsageFields 从响应体/SSE 事件数据中提取 usage 字段（协议专属链）
    ExtractUsageFields(data []byte) UsageFields

    // IsStreamTerminal 判定一个流式事件是否为终止事件
    //（openai: data=="[DONE"]（兼作未知协议 fallback，承载原组合判定 message_stop || [DONE]）；
    //  anthropic: type=="message_stop"（保留 [DONE] 以维持既有组合语义）；
    //  gemini: 无终止事件，恒 false，由流 EOF 兜底）
    IsStreamTerminal(ev StreamEvent) bool

    // IsFinalUsageEvent 判定一个流式事件是否携带最终 usage
    //（openai: type=="message" 或空 type（协议无关最终 usage chunk）；
    //  anthropic: message_delta / message / 空 type（message_start 初始 usage 不计）；
    //  gemini 每个 chunk 均带累积 usageMetadata，仅最后一个含 usage 的 chunk 计为最终，
    //  防止计费取中间累积值导致少计）
    IsFinalUsageEvent(ev StreamEvent) bool

    // ErrorNormalizer 上游错误归一化；默认实现恒返回 nil（走现有状态码白名单）
    ErrorNormalizer() ErrorNormalizer
}
```

> `IsStreamTerminal` / `IsFinalUsageEvent` 随 2026-09-09 gemini 接入从 `mod_body_process` 的硬编码判定（原 `type=="message_stop" || data=="[DONE]"` 及 `message_delta`/`message` 判定）下沉而来，openai/anthropic 行为逐行搬迁、零变化。参数为事件级抽象（`StreamEvent`，承载 type/data），**不能是 `mod_body_process.SSEEvent`**——适配层只允许 import `bfe_http` 与 gjson（见 3.1 依赖约束），由调用方做事件字段映射。

usage 提取结果用中性结构 `UsageFields`（不替代 `bfe_basic.TokenUsage` / `mod_body_process.QuotaUsage`），由调用方做字段映射——这是避免依赖反转的关键：

```go
type UsageFields struct {
    UsedQuota, PromptTokens, CompletionTokens int64
    CacheReadTokens, CacheWriteTokens         int64
    AudioInputTokens, AudioOutputTokens       int64
    ImageInputTokens, ImageCount, VideoCount  int64
}
```

错误归一化类型（为阶段二新协议定制 fallback 语义预留）：

```go
type ProtocolError struct {
    Code       string   // auth_invalid | rate_limited | model_not_found | server_error | unknown
    StatusCode int
    IsUpstream bool     // true=上游错误(可换 key/fallback)
    Retryable  bool     // 同 key 退避重试
    SwapKey    bool     // 429 轮换 key
    MarkDead   bool     // 401/402/403 标记 key 死亡
    Message    string
}
```

### 3.3 注册表

编译期注册（init 内 `Register`），无动态加载：

```go
// Get 按协议名取适配器；unknown/empty 回落 openai（保持现状兜底行为）
func Get(protocol string) ProtocolAdapter
// Supports：空 protocols 默认仅 openai
func Supports(protocols []string, p string) bool
// ValidateProtocols：空=合法；含未知协议名返回 error
func ValidateProtocols(protocols []string) error
```

协议选择是 **per-request** 的：cluster 可配 `["openai", "anthropic"]` 多协议，按请求 `AiBasicInfo.AuthStyle` 取适配器：`modelprotocol.Get(aiMeta.AuthStyle)`。

---

## 4. 请求生命周期中的适配点

```
http_conn.serveRequest()
  → GetApiKey / DetectAuthStyle（bfe_basic，内部委托 detect.go）     ← 协议识别
  → reverseproxy.doSingleAIForward
      → Supports(ModelProtocols, AuthStyle)                          ← 协议校验（PROVIDER_PROTOCOL_MISMATCH）
      → adapter.InjectAuth + ExtraHeaders                            ← 认证头/版本头注入
  → mod_body_process QuotaUsageProcessor
      → SSEEvent/RawEvent.GetQuotaUsage（经 extractUsageFields）      ← 流式 usage
      → SSEEvent 终止/最终 usage 判定（适配器 IsStreamTerminal / IsFinalUsageEvent） ← 流式终止判定（2026-09-09 下沉）
  → mod_ai_token_auth
      → UpdateCtxByUsage（按 AuthStyle 取适配器 ExtractUsageFields）  ← 非流式 usage
      → tokenRequestFinishHandler 成本计算与扣费（不变）
```

| 适配点 | 原实现 | 现实现 |
|---|---|---|
| 协议识别 | `bfe_basic` 硬编码 | `detect.go`，`bfe_basic` 保留签名委托 |
| 认证头注入 | `mod_ai_token_auth.SetApiKey` 写死两种 | 适配器 `InjectAuth`；`SetApiKey` 保留签名委托 |
| 版本头注入 | `reverseproxy.go` 硬编码 `anthropic-version` | 适配器 `ExtraHeaders`，显式携带优先（TC-07） |
| 协议校验 | `clusterSupportsAuthStyle` | `modelprotocol.Supports`（语义不变） |
| 流式 usage | `SSEEvent/RawEvent.GetQuotaUsage` 各一份全链 | 适配器字段链 + 调用方累加语义保留 |
| 流式终止/最终 usage 判定 | `llm_util.go` 硬编码 `message_stop` / `[DONE]` / `message_delta` | 适配器 `IsStreamTerminal` / `IsFinalUsageEvent`；openai/anthropic 逐行搬迁，gemini 无终止事件由流 EOF 兜底（2026-09-09 随 gemini 接入下沉） |
| 非流式 usage | `UpdateCtxByUsage` 一份全链 | 按 `AuthStyle` 取单适配器；结果全零时回退两阶段组合（issue #1364） |
| fallback 判定 | `shouldTriggerFallback` 纯状态码白名单 | 前置 `ErrorNormalizer` seam（默认返回 nil → 白名单，行为不变） |

`ModelProtocols` 未知协议名的加载校验挂在 `bfe_server/bfe_confdata_load.go` 的 `InitDataLoad`（启动失败）与 `serverDataConfReload`（热加载拒绝且不换配置）；`bfe_config` 不依赖 `bfe_model_protocol`。

**明确不在适配层的事**：模型名改写（prefix 剥离 / `ModelMapping`，协议无关，留在 `computeTargetModel`）；请求/响应体协议翻译（不做）；路由/限流/计费业务语义。

---

## 5. usage 归一设计

### 5.1 字段链按协议拆分

原实现是一份代码跑全部字段链：OpenAI 主链 → DeepSeek fallback（`prompt_cache_hit_tokens` / `prompt_tokens_details.cached_tokens`）→ Responses fallback（`input_token_details.cached_tokens`）→ Claude fallback（`input_tokens`/`output_tokens`/`cache_read_input_tokens`/`cache_creation_input_tokens`，且 `prompt += cacheRead + cacheWrite`）。

拆分后：

- **openai 适配器**：OpenAI 主链 + DeepSeek + Responses fallback（无 Claude 链）；
- **anthropic 适配器**：Claude 链（含 prompt 归一）。2026-09-04 起增加真实流式报文结构解析（issue #1352）：Anthropic 流式 `message_start` 的初始 usage 位于 `message.usage.*`（此前只解析顶层 `usage.*`），`message_delta` 的最终 usage 位于顶层 `usage.*`，两条路径均支持。
- **gemini 适配器**：`usageMetadata` 链（camelCase：`promptTokenCount` / `candidatesTokenCount` / `cachedContentTokenCount` → 中性字段；`totalTokenCount` 独立字段直接取，缺失时回退 `prompt + candidates` 求和）。gemini 流式每个 chunk 均带**累积** `usageMetadata`，与 openai/anthropic"最终事件携带"的语义不同，由 `IsFinalUsageEvent` 保证仅最后一个含 usage 的 chunk 计入最终 usage（取中间 chunk 会少计）。

等价性：各链互斥（OpenAI 系响应有 `prompt_tokens`；Claude 有 `input_tokens`；全零→estimate），按协议拆分后与全链结果逐字段等价。**例外**（issue #1364）：当鉴权识别出的协议与响应体实际格式错配（如 Bearer→openai 适配器 + Anthropic body）时，单适配器解析全零、不等价于旧全链；该场景由 `UpdateCtxByUsage` 的全零回退兜底（见 5.2）。

### 5.2 累加语义留在调用方

- `UpdateCtxByUsage` 按 `AuthStyle` 取适配器，后续 `used>0 / else if prompt>0...` 分支逐行保留（写 `bfe_basic.TokenUsage`）；**跨协议兜底**（issue #1364）：单适配器结果全零（`UsedQuota`/`PromptTokens`/`CompletionTokens`/`ImageCount`/`VideoCount` 均为 0，典型场景：Bearer 鉴权被识别为 OpenAI 协议、后端返回 Anthropic 格式 body）时，回退到与 `mod_body_process` 相同的三阶段组合链（openai 链 → anthropic 链 → gemini 链，逐链全零则继续）再解析一次，恢复收敛前"响应格式无关"语义，避免 `UsedQuota = 0` 导致最终 usage 标记失效、计费漏项；
- `SSEEvent/RawEvent.GetQuotaUsage(authStyle string)` 由调用方（`QuotaUsageProcessor.Process`，经 `aiBasicInfo.AuthStyle`）传入已识别的 AuthStyle，内部经 `modelprotocol.Get(authStyle)` 取适配器做终止/最终 usage 判定（未知/空 AuthStyle 回落 openai 适配器，承载原组合判定，语义与硬编码时代一致）；usage 字段解析经 `ParseUsageFieldsCrossProtocol` 做三阶段组合（openai 链 → anthropic 链 → gemini 链，逐链全零则继续，含 carry-in 合并），`isguess`/`EstimateContentToken` 逻辑原样；
- 扣费准确性依赖此约定：迁移时逐字段回归（流式累加、estimate 模式、Anthropic prompt 归一）。

---

## 6. 新增协议接入指南

接入一种新协议（步骤已按 2026-09-09 gemini 实际落地修订）：

1. `utils/protocol.go` 增加协议常量（如 `ProtocolGemini`），根包 `protocol.go` re-export；
2. 新建 `<protocol>/` 适配器目录实现 `ProtocolAdapter`：认证注入（如 `x-goog-api-key`）、usage 提取与归一（如 `usageMetadata` 的 camelCase 字段）；**若该协议流式终止语义与 openai/anthropic 不同**（如 gemini 无 SSE 终止事件、每 chunk 带累积 usage），实现 `IsStreamTerminal` / `IsFinalUsageEvent`；
3. `detect.go` 增加识别规则（header / 路径），新规则不得改变既有协议的识别优先级（gemini 的 `x-goog-api-key` 放在探测链尾、路径规则放在 openai 默认分支之前）；
4. `registry.go` 的 `init()` 增加 `Register(<protocol>.New())`；
5. `bfe_basic/request_ai_basic.go` 同步 AuthStyle 常量——该处存在**第二份** AuthStyle 常量（:53-57），勿漏；
6. `shouldTriggerFallback` 的 seam 处按已识别 AuthStyle 取 `ErrorNormalizer`；
7. 控制面（ai-gateway-api）放开 `model_protocols` 枚举约束；**BFE 必须先于控制面发布**——加载期 `ValidateProtocols` 会拒绝未知协议名，控制面先放开会导致 BFE 拒载配置。

框架代码（reverseproxy / mod_body_process / mod_ai_token_auth）**零修改**。唯一例外：新协议流式终止语义不同时，需把终止/最终 usage 判定从 `mod_body_process` 硬编码下沉为适配器方法（gemini 即如此，见 §3.2 / §4），下沉对既有协议为逐行搬迁、零行为变化。配置侧仅需 cluster 的 `AIConf.ModelProtocols` 包含新协议名。

> gemini 是首个按本指南落地的新协议，实施记录见 `modifications/2026-09-09-gemini-protocol-support/design-changes.md`。

---

## 7. 与相关文档的关系

| 文档 | 关系 |
|---|---|
| `claude_protocol_support.md` | anthropic 适配器的业务语义来源（识别规则、版本头、usage 归一）；实现已收敛到本层，行为不变 |
| `rmb_quota.md` | 计费依赖 usage 提取；字段链现由本层提供，累加/扣费语义不变 |
| `multi_api_key.md` | key 池选择与认证头注入；`SetApiKey` 现委托 `InjectAuth` |
| `provider_model_prefix_routing.md` | 模型名改写，协议无关，不受本层影响 |
| `modifications/2026-09-03-model-protocol-adapter/design-changes.md` | 本次收敛的修改方案与实施记录 |
| `modifications/2026-09-09-gemini-protocol-support/design-changes.md` | gemini 接入的修改方案（首个按 §6 指南落地的新协议；流式终止/最终 usage 判定随本次下沉适配器） |

---

## 8. 边界与风险

- **透传原则**：适配层只做边缘适配（认证/版本头/usage/错误），不做请求/响应体协议翻译；
- **兜底一致性**：`Get` 对未知协议回落 openai、`Supports` 空列表默认 openai，与历史行为一致；未知协议名在配置加载期报错（暴露既有配置错误）；
- **病态响应体**：同一 usage 同时混含 DeepSeek 与 Claude 字段且缺 `prompt_tokens` 时，事件侧两阶段合并与旧全链可能有差异——真实 provider 不会混合两套字段，未做特殊处理；
- **AuthStyle 与响应体格式错配**（issue #1364）：鉴权协议由请求头/路径识别，响应体格式由后端实际返回决定，两者可能不一致（如 DeepSeek 官方 Key 走 Anthropic 兼容端点但识别为 openai）。此时单适配器解析全零，依赖 `UpdateCtxByUsage` 的全零回退组合链兜底；`mod_body_process` 事件侧本就走组合链，不受影响；
- **per-cluster 认证变体**（如 Azure OpenAI 的 `api-key` header）：当前 `InjectAuth` 未承载 per-cluster 参数，需要时另起评审扩展接口；
- **无终止事件协议的流式语义**（gemini）：gemini 流式无 SSE 终止事件，`IsStreamTerminal` 恒 false，流结束依赖 HTTP EOF 兜底路径，`MarkFinalUsageSeen` / `MarkResponseCompleted`（SC12 客户端中断计费依赖）的正确性以"最后一个含 `usageMetadata` 的 chunk"为前提——若上游异常截断流，usage 语义与 openai/anthropic 截断场景保持一致（按已收到部分结算）；
- **错误格式**（gemini）：网关自身产生的错误恒为 OpenAI 风格 `AiErrorBody`，gemini 客户端收到的错误体非 gemini 原生 `{error: {code, status}}` 格式——与 anthropic 客户端现状一致的已知 gap，如需按 AuthStyle 适配错误响应另起评审；
- **识别规则演进**：gemini 接入在 `DetectProtocolAndKey` 探测链尾与 `DetectProtocol` 默认分支前插入了新规则，既有 openai/anthropic 请求的识别优先级不变（SC06 双头并存用例兜底）。
