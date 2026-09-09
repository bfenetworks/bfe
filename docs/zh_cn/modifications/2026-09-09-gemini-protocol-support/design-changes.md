# Gemini 协议支持（model_protocol 新增 gemini）

## 1. 背景

AI 网关已支持 `openai`、`anthropic` 两种 model-protocol，协议知识已收敛在协议适配层 `bfe/bfe_model_protocol/`（见 `2026-09-03-model-protocol-adapter/design-changes.md`）。接入指南（`docs/zh_cn/sys_design/model_protocol_adapter.md` 第 6 节）以 gemini 为例预留了扩展路径，本期落地。

本文档描述 **BFE 数据面侧**的修改方案；控制面（ai-gateway-api）侧的枚举放开、模型发现不在本文档范围（见 §8 边界说明）。

gemini 与现有协议的差异（决定本期改造点）：

| 维度 | openai / anthropic 现状 | gemini |
|---|---|---|
| 认证头 | Bearer / `x-api-key` + `anthropic-version` | `x-goog-api-key`，无版本头 |
| 请求路径 | `/v1/...` 前缀风格 | `/v1beta/models/{model}:generateContent`、`:streamGenerateContent` |
| 非流式 usage | snake_case（`prompt_tokens` / `input_tokens`） | camelCase：`usageMetadata.promptTokenCount` / `candidatesTokenCount` / `totalTokenCount` / `cachedContentTokenCount` |
| 流式终止 | SSE `data: [DONE]` / `message_stop` 事件 | **无终止事件，HTTP 流 EOF 即终止** |
| 流式 usage | 最后一个 chunk / `message_delta` 事件携带 | **每个 chunk 均带累积 `usageMetadata`** |

---

## 2. 目标

1. 新增 gemini 协议适配器：认证注入（`x-goog-api-key`）、usage 字段提取与归一；
2. 协议识别支持 gemini（`x-goog-api-key` header + `:generateContent` 路径形态）；
3. gemini 流式/非流式 usage 正确进入计费链路（取最后一个含 `usageMetadata` 的 chunk，避免累积值少计）；
4. 修复流式终止/最终 usage 判定硬编码问题（现有 `message_stop`/`[DONE]` 写死判定无法承载无终止事件的协议），openai/anthropic 行为零变化；
5. 框架代码（reverseproxy 转发主路径 / mod_body_process / mod_ai_token_auth）零修改，保持适配层"新增协议 = 常量 + 适配器目录 + 识别规则 + 一行注册"的扩展模式。

非目标：

- **不做请求/响应体协议翻译**（透传原则，gemini 客户端仅可路由到 gemini 协议上游）；不做 gemini ↔ openai/anthropic 互转；
- 不改网关自身错误格式（网关产生的错误统一为 OpenAI 风格 `AiErrorBody`，与 anthropic 客户端现状一致，gemini 客户端收到的错误体非 gemini 原生格式，记为已知 gap）；
- 不改 `mod_ai_route` 路由、`mod_ai_token_auth` 计费、`mod_ai_rate_limit` 限流的业务语义；
- 不改配置格式：`AIConf.ModelProtocols` 字段语义不变，仅取值域新增 `gemini`。

---

## 3. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 新增包 | gemini 适配器（`x-goog-api-key` 认证 + usageMetadata 提取） | `bfe/bfe_model_protocol/gemini/`（新增） |
| 适配层 | 协议常量 `ProtocolGemini` | `bfe/bfe_model_protocol/utils/protocol.go`、`protocol.go` |
| 适配层 | 编译期注册 | `bfe/bfe_model_protocol/registry.go`（`init()` 加一行） |
| 适配层 | 识别规则：`x-goog-api-key` 探测 + gemini 路径形态 | `bfe/bfe_model_protocol/detect.go` |
| 适配层 | usage 组合链新增 gemini 链 | `bfe/bfe_model_protocol/utils/usage_parse.go` |
| 基础包 | `AuthStyleGemini` 常量 | `bfe/bfe_basic/request_ai_basic.go` |
| 基础包 | `DetectModeFromPath` 对 gemini 路径的 Mode 推断检查 | `bfe/bfe_basic/request_ai_basic.go` |
| 体处理模块 | **流式终止/最终 usage 判定下沉为适配器职责**（核心改造） | `bfe/bfe_modules/mod_body_process/llm_util.go` |
| 转发层 | `shouldTriggerFallback` 的 `ErrorNormalizer` seam 从占位改为按 AuthStyle 取值 | `bfe/bfe_server/reverseproxy.go` |
| 配置加载 | 无改动（`ValidateProtocols` 自动覆盖注册表新协议） | — |
| 测试 | gemini 适配器/识别/usage/流式判定单元测试；SC06 既有用例回归 | `bfe/bfe_model_protocol/**/*_test.go` 等 |

---

## 4. 详细设计

### 4.1 协议常量与注册

`bfe_model_protocol/utils/protocol.go`：

```go
const (
    ProtocolOpenAI    = "openai"
    ProtocolAnthropic = "anthropic"
    ProtocolGemini    = "gemini"   // 新增
    ProtocolUnknown   = "unknown"
)
```

`bfe_model_protocol/protocol.go` re-export `ProtocolGemini`；`registry.go` 的 `init()` 增加 `Register(gemini.New())`。

`bfe_basic/request_ai_basic.go:53-57` 增加 `AuthStyleGemini` 常量（注意此处存在第二份 AuthStyle 常量，需与适配层常量同步，勿漏）。

### 4.2 gemini 适配器（新增 `bfe/bfe_model_protocol/gemini/`）

照 `openai/`、`anthropic/` 目录形态三个文件：

**`gemini.go`** — 实现 `ProtocolAdapter`（接口签名以 `protocol.go` 现状为准）：

```go
type adapter struct{}

func New() modelprotocol.ProtocolAdapter { return &adapter{} }

func (a *adapter) Key() string { return modelprotocol.ProtocolGemini }

func (a *adapter) InjectAuth(outreq *bfe_http.Request, key string) error {
    // outreq.Header.Set("x-goog-api-key", key)
    return nil
}

func (a *adapter) ExtraHeaders() map[string]string { return nil } // gemini 无版本头

func (a *adapter) ExtractUsageFields(data []byte) utils.UsageFields {
    return ParseGeminiUsageFields(data)
}

func (a *adapter) ErrorNormalizer() utils.ErrorNormalizer {
    return utils.DefaultErrorNormalizer // 同 openai/anthropic，占位
}
```

**`auth.go`** — `InjectGeminiAuth`，注入 `x-goog-api-key`。

**`usage.go`** — `ParseGeminiUsageFields(data []byte) utils.UsageFields`：gjson 从 `usageMetadata` 提取：

| gemini 字段 | 中性 UsageFields | 说明 |
|---|---|---|
| `promptTokenCount` | `PromptTokens` | |
| `candidatesTokenCount` | `CompletionTokens` | |
| `cachedContentTokenCount` | `CachedTokens` | |
| `totalTokenCount` | `TotalTokens` | 独立字段直接取；**缺失时回退 `prompt + candidates` 求和** |

配套单测照 `utils/usage_parse_test.go` 现有用例（全字段 / 缺 `totalTokenCount` 回退 / 全零 / 不解析 openai、anthropic 字段）。

### 4.3 协议识别（`detect.go`）

1. **`DetectProtocolAndKey()`**：现有探测链 `Authorization: Bearer` 优先 → `x-api-key` fallback，gemini 的 `x-goog-api-key` 追加到链尾，保留现有优先级（避免误判既有请求）；
2. **`DetectProtocol()`**：新增规则——
   - 存在 `x-goog-api-key` 且无 `Authorization` → gemini；
   - 路径含 `:generateContent` / `:streamGenerateContent`，或以 `/v1beta/models/` 为前缀 → gemini（判定放在 openai 默认分支之前）。

单测补：`x-goog-api-key` 单独存在 / 与 Authorization 并存（Authorization 优先）/ gemini 路径 / 既有 openai、anthropic 识别不受影响。

**`DetectModeFromPath`**（`bfe/bfe_basic/request_ai_basic.go:181-206`）：实施时按实际调用链检查 gemini 路径形态下 Mode 推断是否需要新规则（如按 `:streamGenerateContent` 判流式），需要则补，不需要则在实施记录中注明原因。

### 4.4 流式终止/最终 usage 判定下沉适配器（核心改造）

**现状问题**：`bfe/bfe_modules/mod_body_process/llm_util.go` 的 `SSEEvent.GetQuotaUsage()` 中，终止事件判定写死 `type=="message_stop" || data=="[DONE]"`（约 :153），最终 usage 判定写死 `message_delta` / `message` / 空 type（约 :154-164）。该判定不走适配器，gemini 流式（`streamGenerateContent`）**没有 SSE 终止事件**——直接套用会导致终止判定永不触发，`MarkFinalUsageSeen` / `MarkResponseCompleted`（SC12 客户端中断计费依赖）不生效。

**方案**：将"终止事件判定 / 最终 usage 事件判定"下沉为 `ProtocolAdapter` 职责：

```go
// ProtocolAdapter 新增（命名以实施为准）：
// IsStreamTerminal 判定该 SSE 事件是否为流终止事件（gemini 恒 false，由流 EOF 兜底）
// IsFinalUsageEvent 判定该事件是否携带最终 usage
IsStreamTerminal(ev *SSEEvent) bool
IsFinalUsageEvent(ev *SSEEvent) bool
```

- openai 适配器承载现有 `type=="message_stop" || data=="[DONE]"` 逻辑；anthropic 适配器承载 `message_delta` / `message` 逻辑——**行为逐行搬迁，零变化**；
- gemini 适配器：`IsStreamTerminal` 恒 false（HTTP 流结束即终止，由现有 EOF 路径兜底）；`IsFinalUsageEvent` 按事件 data 中是否含 `usageMetadata` 判定；
- 调用点（`GetQuotaUsage` 及其在 body_process 中的事件侧调用）改为经 `modelprotocol.Get(authStyle)` 取判定。

**gemini 流式 usage 语义**：gemini 每个 chunk 均携带**累积** `usageMetadata`。计费对最终 usage 做一次性取值，必须确保取**最后一个**含 `usageMetadata` 的 chunk（适配器判定保证 `GetQuotaUsage` 只在最终事件上返回非零值），取中间 chunk 会导致少计——该语义必须有用例固化。

**usage 组合链**：`utils/usage_parse.go` 的 `ParseUsageFieldsCrossProtocol` 现为 openai 链 → 全零回落 anthropic 链两阶段。gemini 字段为 camelCase，与 openai snake_case 天然不冲突，但作为第三阶段显式加入（openai → anthropic → gemini），保持跨协议错配（issue #1364 场景）的兜底能力。

### 4.5 `shouldTriggerFallback` 的 `ErrorNormalizer` seam

现状 `reverseproxy.go:1781-1800` 用 `modelprotocol.Get("")` 占位取 `ErrorNormalizer`（默认恒返回 nil，走状态码白名单）。本期一并修正为按已识别 `AuthStyle` 取对应适配器的 normalizer；gemini 适配器暂返回 `DefaultErrorNormalizer`（行为不变，上游错误归一启用另起评审）。

### 4.6 不动的部分

- **转发主路径**（`reverseproxy.go doSingleAIForward`）：per-request `modelprotocol.Get(AuthStyle)` 选适配器、`clusterSupportsAuthStyle` 协议匹配（不支持的协议返回 `PROVIDER_PROTOCOL_MISMATCH`，行为不变）、key 池重试——均零修改；
- **计费累加语义**：`UpdateCtxByUsage`（`mod_ai_token_auth.go`）的累加/扣费分支不动，仅消费适配器提取结果；
- **配置加载**：`bfe_confdata_load.go` 的 `validateClusterModelProtocols` → `modelprotocol.ValidateProtocols` 自动覆盖注册表新协议，gemini 配置加载/热加载无需改动；
- **conf/ 示例配置**：无协议相关配置，不改动；
- **错误格式**：网关自身错误恒为 OpenAI 风格 `AiErrorBody`；
- 透传原则：请求/响应体不做协议翻译，`model` 字段改写（`computeTargetModel`）与协议无关。

---

## 5. 关键代码索引

| 文件 | 位置 | 现状 | 变更 |
|---|---|---|---|
| `bfe/bfe_model_protocol/utils/protocol.go` | 协议常量 | openai / anthropic / unknown | 加 `ProtocolGemini = "gemini"` |
| `bfe/bfe_model_protocol/protocol.go` | 常量 re-export | 同上 | 同上 |
| `bfe/bfe_model_protocol/registry.go` | `init()` | 注册 openai / anthropic | 加 `Register(gemini.New())` |
| `bfe/bfe_model_protocol/gemini/` | 新增 | — | `gemini.go` / `auth.go` / `usage.go` |
| `bfe/bfe_model_protocol/detect.go` | `DetectProtocolAndKey` | Bearer 优先 → `x-api-key` fallback | 链尾加 `x-goog-api-key` |
| `bfe/bfe_model_protocol/detect.go` | `DetectProtocol` | `/v1/messages` → anthropic，默认 openai | 加 `x-goog-api-key` 与 `:generateContent` / `/v1beta/models/` 规则 |
| `bfe/bfe_model_protocol/utils/usage_parse.go` | `ParseUsageFieldsCrossProtocol` | openai → anthropic 两阶段 | 加 gemini 第三阶段 |
| `bfe/bfe_basic/request_ai_basic.go` | AuthStyle 常量（:53-57） | OpenAI / Anthropic / Unknown | 加 `AuthStyleGemini` |
| `bfe/bfe_basic/request_ai_basic.go` | `DetectModeFromPath`（:181-206） | openai / anthropic 路径规则 | 检查并按需补 gemini 规则 |
| `bfe/bfe_modules/mod_body_process/llm_util.go` | `SSEEvent.GetQuotaUsage` 终止/最终 usage 判定（:134-183） | 硬编码 `message_stop`/`[DONE]`/`message_delta` | 判定下沉适配器；openai/anthropic 逐行搬迁，gemini 新实现 |
| `bfe/bfe_server/reverseproxy.go` | `shouldTriggerFallback`（:1781-1800） | `modelprotocol.Get("")` 占位 | 按 AuthStyle 取 normalizer（gemini 返回默认） |

---

## 6. 测试计划

### 6.1 单元测试（新增）

- `gemini/auth_test.go`：`x-goog-api-key` 注入；
- `gemini/usage_test.go`：`usageMetadata` 全字段 / 缺 `totalTokenCount` 回退求和 / 全零 / 不解析 openai、anthropic 字段；
- `detect_test.go` 补：`x-goog-api-key` 单独与并存场景、gemini 路径识别、既有识别优先级不变；
- 流式判定：gemini 无终止事件（恒 false）、含 `usageMetadata` 事件判定、EOF 兜底路径；
- `registry_test.go` 补：`ValidateProtocols` 接受 `gemini`。

### 6.2 回归

- **SC06（claude-protocol-support）TC-01~TC-08 全量回归**：4.4 判定下沉是行为敏感点，openai/anthropic 流式终止与最终 usage 语义必须逐用例通过；
- **扣费回归**：`mod_ai_token_auth` / `mod_body_process` 既有单测全量通过；gemini 流式"取最后一个含 usage 的 chunk"用例固化；
- `go build ./...`、`go test`（bfe_model_protocol / bfe_basic / bfe_server / mod_ai_token_auth / mod_body_process / bfe_config）、`go vet`、`gofmt -l` 全部通过。

### 6.3 端到端联调（与 ai-gateway-api 联调）

gemini 客户端 → 网关 → gemini 协议上游：非流式 generateContent、流式 streamGenerateContent 计费对账、协议不匹配拒绝（`PROVIDER_PROTOCOL_MISMATCH`）、gemini 请求命中 openai-only 集群拒绝。

---

## 7. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 4.4 判定下沉触及计费链路（`GetQuotaUsage` / `MarkFinalUsageSeen` / SC12 客户端中断计费） | openai/anthropic 逻辑逐行搬迁零变化；SC06 全量回归 + 扣费用例固化；出错时可先回滚 4.4（恢复硬编码判定），gemini 其余部分独立回滚 |
| gemini 流式累积 `usageMetadata` 若误取中间 chunk 导致少计 | 适配器判定保证仅最终事件返回 usage；专项用例固化 |
| 识别规则新增分支影响既有请求判定 | `x-goog-api-key` 放探测链尾、路径规则放在 openai 默认之前；SC06 双头优先级用例兜底 |
| cluster 配置含 `gemini` 但 BFE 未升级 | 控制面先放开枚举而 BFE 未升级时，`ValidateProtocols` 会拒绝加载（启动失败/热加载拒绝）——**发布顺序约束：BFE 先于 ai-gateway-api 发布**（见 §8） |
| gemini 客户端收到 OpenAI 风格网关错误 | 已知 gap，与 anthropic 客户端现状一致；如需按协议适配错误响应另起评审 |

回滚策略：适配器/常量/注册/识别为纯新增代码，回滚 = revert 本次提交；4.4 判定下沉与既有行为耦合最深，回滚时需整体回滚该文件；无配置格式与数据面状态残留。

---

## 8. 与控制面的边界

- **发布顺序（硬约束）**：BFE 必须先于 ai-gateway-api 发布。BFE 加载期 `ValidateProtocols` 校验协议名：若控制面先放开 `model_protocols` 枚举（接受 `gemini`）而 BFE 未注册 gemini 适配器，导出的含 gemini 配置会被 BFE 拒绝加载（启动失败/热加载拒绝）。
- **配置格式零变更**：`AIConf.ModelProtocols` 语义不变，仅取值域新增 `gemini`；conf-agent 不感知协议语义，无改动。
- **控制面配套改动**（不属于本文档范围）：ai-gateway-api 放开 `ValidModelProtocols` 枚举、`BuildAuthHeader` 加 gemini case（`x-goog-api-key`）、discover-models 按协议默认 URI（gemini 为 `GET /v1beta/models`，解析 `models[].name` 并剥离 `models/` 前缀）。cluster 级 `model_protocols` 声明与协议一致性校验为已否决方案，控制面不引入。
- 访问日志 `ai_protocol` 字段继续由 AuthStyle 填充（gemini 请求记为 `gemini`）；错误码 `PROVIDER_PROTOCOL_MISMATCH` 语义与消息格式不变。
