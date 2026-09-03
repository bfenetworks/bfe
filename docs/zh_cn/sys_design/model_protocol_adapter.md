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
└── anthropic/           # anthropic messages 协议
    ├── anthropic.go     # ProtocolAdapter 实现
    ├── auth.go          # x-api-key + anthropic-version
    └── usage.go         # Claude usage 提取（cache read/write 归一）
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

    // ErrorNormalizer 上游错误归一化；默认实现恒返回 nil（走现有状态码白名单）
    ErrorNormalizer() ErrorNormalizer
}
```

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
| 非流式 usage | `UpdateCtxByUsage` 一份全链 | 同上 |
| fallback 判定 | `shouldTriggerFallback` 纯状态码白名单 | 前置 `ErrorNormalizer` seam（默认返回 nil → 白名单，行为不变） |

`ModelProtocols` 未知协议名的加载校验挂在 `bfe_server/bfe_confdata_load.go` 的 `InitDataLoad`（启动失败）与 `serverDataConfReload`（热加载拒绝且不换配置）；`bfe_config` 不依赖 `bfe_model_protocol`。

**明确不在适配层的事**：模型名改写（prefix 剥离 / `ModelMapping`，协议无关，留在 `computeTargetModel`）；请求/响应体协议翻译（不做）；路由/限流/计费业务语义。

---

## 5. usage 归一设计

### 5.1 字段链按协议拆分

原实现是一份代码跑全部字段链：OpenAI 主链 → DeepSeek fallback（`prompt_cache_hit_tokens` / `prompt_tokens_details.cached_tokens`）→ Responses fallback（`input_token_details.cached_tokens`）→ Claude fallback（`input_tokens`/`output_tokens`/`cache_read_input_tokens`/`cache_creation_input_tokens`，且 `prompt += cacheRead + cacheWrite`）。

拆分后：

- **openai 适配器**：OpenAI 主链 + DeepSeek + Responses fallback（无 Claude 链）；
- **anthropic 适配器**：Claude 链（含 prompt 归一）。

等价性：各链互斥（OpenAI 系响应有 `prompt_tokens`；Claude 有 `input_tokens`；全零→estimate），按协议拆分后与全链结果逐字段等价。

### 5.2 累加语义留在调用方

- `UpdateCtxByUsage` 按 `AuthStyle` 取适配器，后续 `used>0 / else if prompt>0...` 分支逐行保留（写 `bfe_basic.TokenUsage`）；
- `SSEEvent/RawEvent.GetQuotaUsage` 签名无参数拿不到 `AuthStyle`，经共享 helper `extractUsageFields` 做两阶段组合（openai 链 → prompt/completion 全零时回落 anthropic 链，含 carry-in 合并），`isguess`/`EstimateContentToken` 逻辑原样；
- 扣费准确性依赖此约定：迁移时逐字段回归（流式累加、estimate 模式、Anthropic prompt 归一）。

---

## 6. 新增协议接入指南

接入一种新协议（以 gemini 为例）：

1. `utils/protocol.go` 增加 `ProtocolGemini` 常量；
2. 新建 `gemini/` 目录实现 `ProtocolAdapter`：`auth.go`（`x-goog-api-key`）、`usage.go`（`promptTokenCount` / `candidatesTokenCount` / `cachedContentTokenCount` 提取与归一）；
3. `detect.go` 增加识别规则（如 `x-goog-api-key` header）；
4. `registry.go` 的 `init()` 增加 `Register(gemini.New())`；
5. `shouldTriggerFallback` 的 seam 处已有 AuthStyle 时定制 `ErrorNormalizer`；
6. 控制面（ai-gateway-api）放开 `model_protocols` 枚举约束。

框架代码（reverseproxy / mod_body_process / mod_ai_token_auth）**零修改**。配置侧仅需 cluster 的 `AIConf.ModelProtocols` 包含新协议名。

---

## 7. 与相关文档的关系

| 文档 | 关系 |
|---|---|
| `claude_protocol_support.md` | anthropic 适配器的业务语义来源（识别规则、版本头、usage 归一）；实现已收敛到本层，行为不变 |
| `rmb_quota.md` | 计费依赖 usage 提取；字段链现由本层提供，累加/扣费语义不变 |
| `multi_api_key.md` | key 池选择与认证头注入；`SetApiKey` 现委托 `InjectAuth` |
| `provider_model_prefix_routing.md` | 模型名改写，协议无关，不受本层影响 |
| `modifications/2026-09-03-model-protocol-adapter/design-changes.md` | 本次收敛的修改方案与实施记录 |

---

## 8. 边界与风险

- **透传原则**：适配层只做边缘适配（认证/版本头/usage/错误），不做请求/响应体协议翻译；
- **兜底一致性**：`Get` 对未知协议回落 openai、`Supports` 空列表默认 openai，与历史行为一致；未知协议名在配置加载期报错（暴露既有配置错误）；
- **病态响应体**：同一 usage 同时混含 DeepSeek 与 Claude 字段且缺 `prompt_tokens` 时，事件侧两阶段合并与旧全链可能有差异——真实 provider 不会混合两套字段，未做特殊处理；
- **per-cluster 认证变体**（如 Azure OpenAI 的 `api-key` header）：当前 `InjectAuth` 未承载 per-cluster 参数，需要时另起评审扩展接口。
