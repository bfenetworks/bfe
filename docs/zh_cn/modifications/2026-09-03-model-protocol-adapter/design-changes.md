# 按 model_protocol 维度收敛 AI 协议处理（协议适配层）

## 1. 背景

当前 BFE 的 AI 协议处理逻辑（认证头注入、版本头、usage 字段归一、协议识别）散落在 4 个包中：

| 协议关注点 | 当前落点 |
|---|---|
| 协议识别（AuthStyle） | `bfe/bfe_basic/request_ai_basic.go`（`GetApiKey` :129、`DetectAuthStyle` :155） |
| 协议兼容校验 | `bfe/bfe_server/reverseproxy.go:1505`、`:2152`（`clusterSupportsAuthStyle`） |
| 认证头注入 | `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:292`（`SetApiKey`），调用点 `reverseproxy.go:1572` |
| 版本头注入 | `bfe/bfe_server/reverseproxy.go:1577-1581`（Anthropic `anthropic-version`） |
| usage 归一（流式） | `bfe/bfe_modules/mod_body_process/llm_util.go:125`（`SSEEvent.GetQuotaUsage`）、`body_process.go`（`RawEvent.GetQuotaUsage`，第三份重复链） |
| usage 归一（非流式） | `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:115`（`UpdateCtxByUsage`） |
| 上游错误处理 | 无归一化，`reverseproxy.go:1761`（`shouldTriggerFallback`）按状态码白名单 |

由此带来的问题：

1. **协议知识无实体**：`AIConf.ModelProtocols` 只是字符串列表，"openai 协议怎么认证、usage 长什么样、anthropic 要补什么版本头"这些知识散在多处代码里，没有按协议聚合的实现实体。
2. **重复实现**：usage 字段归一有三份独立 fallback 链（流式两处 + 非流式一处），改动一个协议要同步改多处，且这是扣费链路的输入，容易不一致。
3. **新增协议侵入面大**：接入一种新协议（如 gemini）需要同时修改 `bfe_basic`、`reverseproxy`、`mod_ai_token_auth`、`mod_body_process` 共 5 处。

需要明确的概念边界：**适配维度是 model_protocol，不是 provider**。provider 在 BFE 中是用户自定义的 cluster 级实体（名称/key 池/价格表/地址已由 cluster + AIConf 承载，见 `docs/zh_cn/sys_design/claude_protocol_support.md` 中"不能靠 `AIConf.Provider` 名称判断协议"的设计决策），Groq、DeepSeek 等品牌均走 `openai` 协议，无需为品牌建适配器。参考 Bifrost（`bifrost/core/providers/<name>/`）时只借鉴其"按协议目录组织 + 编译期注册 + 共享 utils 下沉"的形态，不照搬其 30+ 方法的 Provider 接口和全量协议翻译。

总体方案设计见 `document-ai-gateway/迭代系统设计/v0.6/bfe-model-protocol-adapter/bfe-model-protocol-adapter架构设计.md`，本文档描述**阶段一（纯收敛、无行为变化）**的修改方案与实施结果。

---

## 2. 目标

1. 新增 `bfe/bfe_model_protocol/` 包，按协议（openai / anthropic）组织协议适配器，承载全部协议知识（认证、版本头、usage 提取、错误归一化）。
2. 将上述散落点的协议逻辑**原样搬入**适配器，调用方改为委托；本次修改**零行为变化**。
3. 合并流式/非流式三份 usage 归一字段链为一份，由协议适配器提供；累加语义留在原调用方。
4. `AIConf.ModelProtocols` 加载校验：值必须是注册表已知协议名（空仍默认 `["openai"]`）。
5. 为阶段二（gemini 等新协议接入）奠定框架：新增协议 = `detect.go` 一条识别规则 + 一个适配器目录 + registry 一行，不改框架代码。

非目标：不做请求/响应体协议翻译（OpenAI↔Anthropic 消息体转换）；不改动 `mod_ai_route` 路由、`mod_ai_token_auth` 计费、`mod_ai_rate_limit` 限流的业务语义；不改 provider/cluster 配置格式。

---

## 3. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 新增包 | 协议适配层：接口、注册表、ProtocolError、UsageFields、识别逻辑 | `bfe/bfe_model_protocol/`（新增） |
| 新增包 | openai 适配器（Bearer 认证/OpenAI+DeepSeek+Responses usage 链） | `bfe/bfe_model_protocol/openai/`（新增） |
| 新增包 | anthropic 适配器（x-api-key + anthropic-version / Claude usage 链） | `bfe/bfe_model_protocol/anthropic/`（新增） |
| 转发层 | `doSingleAIForward` 中认证头/版本头注入改为委托适配器 | `bfe/bfe_server/reverseproxy.go` |
| 转发层 | `shouldTriggerFallback` 前置协议错误归一化 seam（默认实现返回 nil，行为不变） | `bfe/bfe_server/reverseproxy.go` |
| 转发层 | `clusterSupportsAuthStyle` 改调 `modelprotocol.Supports` | `bfe/bfe_server/reverseproxy.go` |
| 配置加载 | `ModelProtocols` 已知协议名校验（启动失败/热加载拒绝） | `bfe/bfe_server/bfe_confdata_load.go` |
| 认证模块 | `SetApiKey` / `UpdateCtxByUsage` 内部委托适配器（签名不变） | `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` |
| 体处理模块 | `SSEEvent.GetQuotaUsage` / `RawEvent.GetQuotaUsage` 经共享提取 helper 委托适配器 | `bfe/bfe_modules/mod_body_process/llm_util.go`、`body_process.go` |
| 基础包 | `GetApiKey`/`DetectAuthStyle` 识别规则搬迁到 `detect.go`（函数保留，内部委托） | `bfe/bfe_basic/request_ai_basic.go`、`bfe/bfe_model_protocol/detect.go` |
| 测试 | 新增适配器/detect/registry 单元测试；既有测试补 AuthStyle 前置条件 | `bfe/bfe_model_protocol/**/*_test.go`、`bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` |

---

## 4. 详细设计（实施结果）

### 4.1 新包结构与接口

新增包 `bfe/bfe_model_protocol/`（与 `bfe_modules/` 平级），目录布局：

```
bfe/bfe_model_protocol/
├── protocol.go          # ProtocolAdapter 接口 + 协议常量
├── registry.go          # 编译期注册 + Get/Supports/ValidateProtocols
├── usage.go             # UsageFields 等类型的根包 alias（实体在 utils，见下）
├── errors.go            # ProtocolError/ErrorNormalizer 的根包 alias
├── detect.go            # 协议识别（收敛 request_ai_basic.go 的硬编码）
├── utils/               # 叶子包：仅依赖 gjson + 标准库，实体类型所在
│   ├── protocol.go      # 协议常量、UsageFields
│   ├── usage_parse.go   # ParseOpenAIUsageFields / ParseAnthropicUsageFields / EstimateContentToken
│   └── errors.go        # ProtocolError、ErrorNormalizer、DefaultErrorNormalizer
├── openai/
│   ├── openai.go        # openai 适配器实现
│   ├── auth.go          # Bearer 认证（导出供协议族复用）
│   └── usage.go         # OpenAI/DeepSeek/Responses 系 usage 提取
└── anthropic/
    ├── anthropic.go     # anthropic 适配器实现
    ├── auth.go          # x-api-key + anthropic-version
    └── usage.go         # Claude usage 提取（cache read/write 归一）
```

**类型放在 `utils` 叶子包、根包以 type alias 暴露**的原因：`registry.go`（根包）需 import openai/anthropic 子包做编译期注册，而子包需引用 `UsageFields`/`ProtocolError` 等类型——若实体定义在根包则形成 `根包→子包→根包` 的 import cycle。alias 保证调用方 API（`modelprotocol.UsageFields` 等）不变。

核心接口（`protocol.go`，已按实施落定）：

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

    // ErrorNormalizer 该协议的上游错误归一化，默认实现恒返回 nil（走现有白名单）
    ErrorNormalizer() ErrorNormalizer
}

type ErrorNormalizer interface {
    // Normalize 解析上游错误 body；未识别返回 nil（走默认状态码白名单）。
    // body 可能为 nil。
    Normalize(statusCode int, body []byte, header http.Header) *ProtocolError
}
```

usage 提取结果的中性结构（`utils/protocol.go`，根包 alias 为 `modelprotocol.UsageFields`）——**不复用/不替代** `bfe_basic.TokenUsage` 与 `mod_body_process.QuotaUsage`，避免 `bfe_model_protocol` 反向依赖这两个包：

```go
type UsageFields struct {
    UsedQuota         int64
    PromptTokens      int64
    CompletionTokens  int64
    CacheReadTokens   int64
    CacheWriteTokens  int64
    AudioInputTokens  int64
    AudioOutputTokens int64
    ImageInputTokens  int64
    ImageCount        int64
    VideoCount        int64
}
```

错误归一化类型（`utils/errors.go`）：

```go
type ProtocolError struct {
    Code       string   // auth_invalid | rate_limited | model_not_found | server_error | unknown
    StatusCode int      // 上游 HTTP 状态码
    IsUpstream bool     // true=上游错误(可换 key/fallback)，false=网关自身错误
    Retryable  bool     // 同 key 退避重试
    SwapKey    bool     // 429 轮换 key
    MarkDead   bool     // 401/402/403 标记 key 死亡
    Message    string
}
```

注册表（`registry.go`）：

```go
func init() {
    Register(openai.New())
    Register(anthropic.New())
}

// Get 按协议名取适配器；unknown/empty 回落 openai 适配器（保持现状兜底行为）
func Get(protocol string) ProtocolAdapter
func Supports(protocols []string, p string) bool      // 空列表默认仅 openai
func ValidateProtocols(protocols []string) error      // 空=合法；含未知协议名返回 error
```

协议选择是 **per-request** 的：cluster 可配 `["openai", "anthropic"]` 多协议（现有能力），按请求 `AuthStyle` 取适配器：`adapter := modelprotocol.Get(aiMeta.AuthStyle)`。

### 4.2 协议识别收敛（detect.go）

现状：`bfe_basic/request_ai_basic.go` 中 `GetApiKey`（:129）与 `DetectAuthStyle`（:155）是硬编码 if-else。

实施：识别规则逐行搬迁到 `bfe/bfe_model_protocol/detect.go`（签名用 `*bfe_http.Request`，避免依赖 bfe_basic）：

```go
func DetectProtocolAndKey(req *bfe_http.Request) (protocol, key string)
func DetectProtocol(req *bfe_http.Request) string
```

`bfe_basic.GetApiKey` / `DetectAuthStyle` 保留原签名，内部委托。识别优先级语义原样保留：

- `Authorization` 优先（剥 `Bearer ` 前缀再剥 `sk-` 前缀，置 `AuthStyle=openai`；即使值为 `"Bearer "` 剥空也置 openai）；
- fallback `x-api-key`（置 `AuthStyle=anthropic`）；
- `DetectAuthStyle` 规则：nil 请求→unknown；path 前缀 `/v1/messages`→anthropic；有 `x-api-key` 且无 `Authorization`→anthropic；默认 openai。

### 4.3 认证头与版本头注入收敛

现状两处：

```go
// reverseproxy.go doSingleAIForward (约 :1572-1581)
mod_ai_token_auth.SetApiKey(outreq, key, aiMeta.AuthStyle)   // Bearer / x-api-key
if aiMeta.AuthStyle == bfe_basic.AuthStyleAnthropic {
    outreq.Header.Set("anthropic-version", "2023-06-01")     // 硬编码版本头
}
```

实施后，`doSingleAIForward` 中收敛为：

```go
adapter := modelprotocol.Get(aiMeta.AuthStyle)
if err := adapter.InjectAuth(outreq, selectedKey.Key); err != nil {
    log.Logger.Warn(...)   // 该错误路径一期不可达（默认适配器对空 key no-op）
}
for k, v := range adapter.ExtraHeaders() {
    if outreq.Header.Get(k) == "" {   // 显式携带的 header 优先（保持 TC-07 语义）
        outreq.Header.Set(k, v)
    }
}
```

- `mod_ai_token_auth.SetApiKey`（:292）**保留函数签名**（存在其他调用点），函数体改为委托 `modelprotocol.Get(authStyle).InjectAuth`；`mod_ai_token_auth` 保留 quota/计费职责。`reverseproxy.go` 不再 import mod_ai_token_auth。
- `anthropic-version` 注入的"显式携带优先"语义保持不变（见集成测试 TC-07）；`ExtraHeaders` 循环不再以 `selectedKey.Key != ""` 为条件——默认适配器对空 key 为 no-op，与原行为一致。

### 4.4 usage 归一收敛（三份字段链合一）

现状三份实现（比方案调研时多发现一份）：

1. 流式：`mod_body_process/llm_util.go:125` `SSEEvent.GetQuotaUsage()`；
2. 流式：`mod_body_process/body_process.go` `RawEvent.GetQuotaUsage()`（第三份，此前未被发现）；
3. 非流式：`mod_ai_token_auth.go:115` `UpdateCtxByUsage()`。

三者的 gjson 字段链**完全相同**：OpenAI 主链 → DeepSeek fallback（`prompt_cache_hit_tokens` / `prompt_tokens_details.cached_tokens`）→ Responses fallback（`input_token_details.cached_tokens`）→ Claude fallback（`input_tokens`/`output_tokens`/`cache_read_input_tokens`/`cache_creation_input_tokens`，且 `prompt += cacheRead + cacheWrite`）。

实施：

- 字段链按协议拆分到适配器：**openai** = OpenAI 主链 + DeepSeek + Responses fallback（无 Claude 链）；**anthropic** = Claude 链（含 prompt 归一，注释一并搬迁）。各链互斥（OpenAI 系有 `prompt_tokens`，Claude 有 `input_tokens`，全零→estimate），按协议拆分后与全链结果逐字段等价。
- 累加语义留在原调用方，逐行保留：
  - `UpdateCtxByUsage` 按 `ctx.aiBasicInfo.AuthStyle` 取适配器调 `ExtractUsageFields`，后续 `used>0 / else if prompt>0...` 分支原样（写 `bfe_basic.TokenUsage`）。
  - `SSEEvent.GetQuotaUsage` / `RawEvent.GetQuotaUsage` 的签名（`GetQuotaUsage()` 无参数）拿不到 AuthStyle，经共享 helper `extractUsageFields` 做**两阶段组合**：openai 链 → prompt/completion 全零时回落 anthropic 链（含 cacheRead/cacheWrite/used 的 carry-in 合并），`isguess`/`curtoken` 逻辑原样。对真实响应体与现状逐字段一致。
- `EstimateContentToken` 搬迁到 `bfe_model_protocol/utils/usage_parse.go`，`llm_util.go` 保留委托包装（导出不破坏）。
- **迁移约束**：usage 是扣费链路（ServeHTTPForAI → BodyProcessor → HandleRequestFinish 扣费）的核心输入，逐字段保持现状：流式累加值、estimate 模式行为、Anthropic `prompt += cacheRead + cacheWrite` 归一。
- 既有单测 `TestUpdateCtxByUsage_AnthropicCache` 补设 `ai.AuthStyle = bfe_basic.AuthStyleAnthropic`——生产路径中调用 `UpdateCtxByUsage` 前 AuthStyle 必然已识别（`doSingleAIForward` 兜底），测试补设是向生产前置条件看齐。

### 4.5 错误归一化与 fallback

现状：`shouldTriggerFallback`（`reverseproxy.go:1761`）按状态码白名单 `aiFallbackStatusCodes`（`{400,401,402,403,422,429}` + ≥500）。

实施：函数体开头加 seam——该处无响应 body 且不一定拿得到 AuthStyle，故对默认实现（恒返回 nil）传 `""`（回落 openai 适配器）；nil 则走原白名单逻辑逐行保留，**一期行为零变化**：

```go
if perr := modelprotocol.Get("").ErrorNormalizer().Normalize(code, nil, nil); perr != nil {
    // 按 ProtocolError 字段判定（阶段二接入新协议时实现具体归一化后生效）
}
```

key 池罚分逻辑（`chooseAIKeyWithAffinity`，:2074）的 429 罚分 / 401/403 dead 判定保持现状；阶段二接入 gemini 等新协议时再在适配器中定制。

### 4.6 `clusterSupportsAuthStyle` 下沉

`reverseproxy.go` 的协议隶属判断改调 `modelprotocol.Supports`（空列表默认仅 openai，语义保持）；错误消息 `PROVIDER_PROTOCOL_MISMATCH`（"request protocol %s not supported by cluster provider (model_protocols=%v)"）格式不变。

### 4.7 `ModelProtocols` 加载校验

`bfe_model_protocol.ValidateProtocols` 挂在 `bfe/bfe_server/bfe_confdata_load.go` 两处：`InitDataLoad`（启动时未知协议名→启动失败）与 `serverDataConfReload`（热加载时未知协议名→拒绝加载且不换配置）。`bfe_config` 不依赖 `bfe_model_protocol`（保持配置层纯净）。空列表默认 `["openai"]` 的行为不变。

### 4.8 不动的部分

- `mod_ai_route` 三级路由表与 condition DSL 求值；
- `mod_ai_rate_limit` Redis 限流与并发释放；
- `mod_body_process` BodyProcessor 流水线框架（解码器选择、`fillBuffer`）；
- `mod_access_pb3` 日志填充（继续读 `AiBasicInfo`）；
- `computeTargetModel` 模型名改写（prefix 剥离 / ModelMapping，协议无关）；
- `bfe_modules/bfe_modules.go` 模块注册顺序；
- 访问日志 29 个 AI 字段体系。

---

## 5. 关键代码索引

| 文件 | 位置 | 现状 | 变更 |
|---|---|---|---|
| `bfe/bfe_server/reverseproxy.go` | `doSingleAIForward` | `SetApiKey`+硬编码 anthropic-version | 委托 `modelprotocol.Get(AuthStyle)` 的 `InjectAuth`+`ExtraHeaders` |
| `bfe/bfe_server/reverseproxy.go` | `shouldTriggerFallback` | 状态码白名单 | 前置 `ErrorNormalizer` seam（默认 nil→白名单） |
| `bfe/bfe_server/reverseproxy.go` | `clusterSupportsAuthStyle` | 布尔校验 | 改调 `modelprotocol.Supports` |
| `bfe/bfe_server/bfe_confdata_load.go` | `InitDataLoad`/`serverDataConfReload` | 无协议名校验 | 新增 `validateClusterModelProtocols` |
| `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | `SetApiKey` | 写死 Bearer/x-api-key | 内部委托 `InjectAuth`（签名不变） |
| `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | `UpdateCtxByUsage` | 非流式 usage fallback 链 | 委托 `ExtractUsageFields`，累加分支保留 |
| `bfe/bfe_modules/mod_body_process/llm_util.go` | `SSEEvent.GetQuotaUsage` | 流式 usage fallback 链 | 经 `extractUsageFields` 两阶段组合 |
| `bfe/bfe_modules/mod_body_process/body_process.go` | `RawEvent.GetQuotaUsage` | 第三份重复链 | 同上，删除 gjson import |
| `bfe/bfe_basic/request_ai_basic.go` | `GetApiKey`/`DetectAuthStyle` | 硬编码识别 | 内部委托 `detect.go`（签名不变） |

---

## 6. 测试计划与结果

### 6.1 单元测试（新增）

- `bfe_model_protocol/openai/usage_test.go`：chat usage / DeepSeek×3 / Responses cached_tokens / 全零 / 不解析 Claude 字段（7 例）；期望值按原实现链逐字段推导。
- `bfe_model_protocol/anthropic/usage_test.go`：Claude cache 归一 / 全 cache 命中 / `total_tokens` 保留 / 不解析 OpenAI 字段。
- `bfe_model_protocol/openai|anthropic/auth_test.go`：Bearer / x-api-key + anthropic-version 注入。
- `bfe_model_protocol/detect_test.go`：Authorization 优先、`sk-` 剥离、`x-api-key` fallback、`/v1/messages`、双头并存、nil 请求。
- `bfe_model_protocol/registry_test.go`：Get 未知协议回落 openai、Supports 空列表、ValidateProtocols、默认 normalizer 返回 nil。

结果：`go build ./...` ✅；`go test`（bfe_model_protocol / bfe_basic / bfe_server / mod_ai_token_auth / mod_body_process / bfe_config）全部 ✅；`go vet` 无输出；`gofmt -l bfe_model_protocol` 无输出（`reverseproxy.go` 在 HEAD 上即被新版 gofmt 列出注释缩进，非本次引入，未触碰）。

### 6.2 集成测试（SC06 回归基线）

`go test ./tests/integration/implementation/scenario-SC06-claude-protocol-support/`：**TC-01~TC-08 全部 PASS**（集成框架按源码 mtime 自动重建了 bfe 二进制，确认测的是新代码）。重点覆盖：协议不匹配拒绝（TC-03/04，错误消息格式不变）、多协议 cluster 双头并存 Authorization 优先（TC-05/06）、显式 `anthropic-version` 保留（TC-07）、Claude usage 解析（TC-08）。

### 6.3 扣费回归

usage 归一合并是扣费链路的变更点：`UpdateCtxByUsage` 累加分支逐行保留，`mod_ai_token_auth` 全部既有单测通过；SC05/SC03/SC09 中 5 个 Cache 计费用例的失败经 `git worktree` 在干净 HEAD 上复现（数值完全一致），确认为预存失败（测试期望未随 cacheWrite 扣除逻辑更新），与本次重构无关。

---

## 7. 风险与回滚

| 风险 | 缓解 |
|---|---|
| usage 归一合并影响扣费准确性 | 原样搬迁不改逻辑；SC06 + 既有单测回归；出错时可单点回滚 4.4（恢复旧链），其余收敛不受影响 |
| `SetApiKey`/`GetApiKey` 存在多处调用点 | 保留函数签名仅改内部实现，调用面无感 |
| AuthStyle 识别规则搬迁引入优先级变化 | 识别规则逐行搬迁；SC06 TC-06 双头优先级用例兜底 |
| cluster 配置含未知协议名导致启动/热加载失败 | 校验仅对非空 ModelProtocols 生效；未知值此前本就静默按 openai 处理，报错属于暴露既有配置错误 |
| 事件侧两阶段组合 vs 旧全链：理论上的混合字段病态响应体（同含 DeepSeek 与 Claude 字段且缺 `prompt_tokens`）可能有差异 | 真实 provider 响应不会混合两套字段（互斥论证），未做特殊处理；如未来出现可在 helper 内加断言 |

回滚策略：本次变更为纯重构（无配置格式、无接口契约变化），回滚 = revert 本次提交；新旧代码之间无数据面状态残留。

---

## 8. 与控制面的边界

- **零配置格式变更**：`AIConf.ModelProtocols` 语义与取值（`openai`/`anthropic`）不变，ai-gateway-api 的 provider 模型（`provider.model_protocols` → `AIConf.ModelProtocols` 的下发链路，见 `model/icluster_conf/cluster.go newAIConf`）不受影响。
- 访问日志 `ai_protocol` 字段继续由 `AuthStyle` 填充，不变。
- 错误码 `PROVIDER_PROTOCOL_MISMATCH`（400）的语义与消息格式不变。
- 阶段二（新增 gemini 等协议值）需要控制面放开 `model_protocols` 枚举约束时再另行评审。
