# BFE Claude 协议支持

## 1. 背景与目标

### 1.1 背景

BFE AI 网关早期默认按 **OpenAI 兼容协议** 处理请求：

- 客户端 API Key 只从 `Authorization: Bearer <key>` 提取；
- 转发给上游时统一注入 `Authorization: Bearer <cluster-key>`；
- Usage 只解析 `prompt_tokens` / `completion_tokens` / `total_tokens`；
- 没有"协议/认证风格"概念。

Anthropic Claude Messages API 与 OpenAI 在网关转发视角存在关键差异：

| 项目 | OpenAI / 兼容协议 | Claude Messages API |
|------|-------------------|---------------------|
| 对话端点 | `POST /v1/chat/completions` | `POST /v1/messages` |
| 客户端认证头 | `Authorization: Bearer <key>` | `x-api-key: <key>` |
| 版本头 | 无 | 必须 `anthropic-version: 2023-06-01` |
| 响应 usage | `usage.prompt_tokens` / `completion_tokens` / `total_tokens` | `usage.input_tokens` / `output_tokens`（无 total） |

BFE 对 Claude 协议**不做协议翻译**（`content[]` 与 `choices[]` 的结构差异对网关透明），但必须解决三个核心差异：

1. 认证头差异；
2. `anthropic-version` 版本头要求；
3. Usage 字段差异。

### 1.2 目标

1. 让 BFE 在只做转发的前提下，支持把客户端 Claude Messages API 请求原样转发到 Anthropic 后端；
2. 自动识别 Claude 协议风格（路径 + 请求头），并写入访问日志 `ai_protocol`；
3. 按协议风格向上游注入正确的 API Key（`x-api-key`）和 `anthropic-version`；
4. 扩展 usage 解析，支持 `input_tokens` / `output_tokens`，保证配额扣减、成本计算与访问日志正确；
5. 校验请求协议与集群 provider 支持的协议是否匹配，不匹配直接拒绝；
6. 保持 OpenAI / DeepSeek 等现有集群的向后兼容。

---

## 2. 协议风格定义

```go
const (
    AuthStyleOpenAI    = "openai"
    AuthStyleAnthropic = "anthropic"
    AuthStyleUnknown   = "unknown"
)
```

识别规则（由 BFE 底层代码完成，不依赖用户路由表条件）：

1. 请求路径：`strings.HasPrefix(req.HttpRequest.URL.Path, "/v1/messages")` → `anthropic`；
2. 请求头：存在 `x-api-key` 且不存在 `Authorization` → `anthropic`；
3. 否则默认 `openai`。

识别结果写入 `AiBasicInfo.AuthStyle`。

> **命名约定**：`anthropic` 是协议/风格标识符（与 `model_protocols` 枚举一致），`Claude` 是 Anthropic 的模型/助手品牌。BFE 代码中统一用 `anthropic` 表示该协议风格。
>
> 注意：不能靠 `model` 名字判断，也不能靠 `AIConf.Provider` 名称判断（provider 名称是用户自定义的，如 `my-anthropic`）。协议匹配校验使用 `AIConf.ModelProtocols`。

---

## 3. 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                        客户端请求                            │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  bfe_server/http_conn.go                                    │
│  - 初始化 AiBasicInfo                                       │
│  - 从 Authorization 或 x-api-key 提取 ClientApiKey           │
│  - 识别并写入 AuthStyle（openai / anthropic）                │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  HandleFoundProduct 回调链                                   │
│  ├─ mod_ai_token_auth: Token 鉴权、Quota 校验               │
│  ├─ mod_ai_route: AI 路由选择                               │
│  └─ mod_ai_rate_limit: 限流判断                             │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  bfe_server/reverseproxy.go                                  │
│  doSingleAIForward():                                       │
│  - 兜底 DetectAuthStyle()                                   │
│  - 校验 cluster.AIConf.ModelProtocols                       │
│  - 按 AuthStyle 注入 Authorization: Bearer 或 x-api-key     │
│  - Anthropic 风格注入 anthropic-version: 2023-06-01         │
│  - 模型映射 / 前缀裁剪后转发至后端                          │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  响应阶段                                                    │
│  ├─ mod_body_process: 流式 usage 解析（SSE）                │
│  └─ mod_ai_token_auth: 非流式 usage 解析、配额扣减          │
│     支持 OpenAI + Claude usage 字段                         │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  mod_access_pb3                                              │
│  - 输出 ai_protocol 等访问日志字段                          │
└─────────────────────────────────────────────────────────────┘
```

---

## 4. 核心数据结构

### 4.1 `bfe_basic.AiBasicInfo`

`bfe/bfe_basic/request_ai_basic.go`

```go
type AiBasicInfo struct {
    ClientApiKey    string
    ClientKeyId     string
    ClientModel     string
    TargetModel     string
    Mode            string // request mode, e.g. chat, image_generation
    Provider        string // upstream model provider, e.g. openai, deepseek
    AuthStyle       string // request protocol/auth style: openai / anthropic / unknown
    RetryCount      uint32
    CostCurrency    string
    tokenUsage      TokenUsage
    ApikeyTags      []ApikeyTag
    TokenTimeInfo   TokenTimeInfo
    AiAuthInfo      AiAuthInfo
    ClusterKeyNames []ClusterKeyName

    allowEstimateToken bool
}
```

### 4.2 `cluster_conf.AIConf`

`bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`

```go
type AIConf struct {
    Type           int
    ModelMapping   *map[string]string
    Provider       string
    Keys           []AIKey
    KeyPolicy      *AIKeyPolicy
    ModelTable     *ModelTable
    MatchPrefix    string
    StripPrefix    bool

    // 新增：provider 支持的模型访问协议
    ModelProtocols []string
}
```

`ModelProtocols` 来源：ai-gateway-api 的 `provider.model_protocols`，如 `["openai"]`、`["anthropic"]`、`["openai", "anthropic"]`。用于 `doSingleAIForward` 中判断请求协议风格是否被当前集群支持。

> 为什么不能通过 `AIConf.Provider` 名称推断协议？cluster/provider 分离后，`provider` 名称是用户自定义的（如 `my-anthropic`），不再限定为系统内置枚举，只有显式的 `ModelProtocols` 才能准确表达该 provider 支持哪些协议。

---

## 5. 模块职责

### 5.1 `bfe_server/http_conn.go`

- 初始化 `AiBasicInfo`；
- 调用 `bfe_basic.GetApiKey()` 提取 API Key：
  - 优先 `Authorization: Bearer <key>`，识别为 `AuthStyleOpenAI`；
  - fallback `x-api-key: <key>`，识别为 `AuthStyleAnthropic`；
- 提取 `model` 字段，设置 `ClientModel` / `TargetModel`；
- 根据路径推断 `Mode`。

### 5.2 `bfe_basic.DetectAuthStyle`

在 `bfe_basic/request_ai_basic.go` 中实现，作为兜底识别：

```go
func DetectAuthStyle(req *Request) string {
    if strings.HasPrefix(req.HttpRequest.URL.Path, "/v1/messages") {
        return AuthStyleAnthropic
    }
    if req.HttpRequest.Header.Get("x-api-key") != "" &&
       req.HttpRequest.Header.Get("Authorization") == "" {
        return AuthStyleAnthropic
    }
    return AuthStyleOpenAI
}
```

### 5.3 `bfe_server/reverseproxy.go`

在 `doSingleAIForward()` 中：

1. 若 `aiMeta.AuthStyle` 为空或 `unknown`，调用 `DetectAuthStyle()` 兜底；
2. 调用 `clusterSupportsAuthStyle(cluster.AIConf.ModelProtocols, aiMeta.AuthStyle)` 校验：
   - `ModelProtocols` 为空时默认只支持 `openai`；
   - 不匹配时返回 400 `PROVIDER_PROTOCOL_MISMATCH`；
3. 按 `AuthStyle` 调用 `mod_ai_token_auth.SetApiKey()`：
   - `openai` → `Authorization: Bearer <key>`；
   - `anthropic` → `x-api-key: <key>`；
4. Anthropic 风格下，若请求未带 `anthropic-version`，自动注入 `anthropic-version: 2023-06-01`；
5. 继续执行模型映射、前缀裁剪、cluster 转发。

### 5.4 `mod_ai_token_auth`

- `SetApiKey(req, apiKey, authStyle)` 按协议注入对应认证头；
- `UpdateCtxByUsage()` 在 OpenAI 字段后增加 Claude fallback：
  - `usage.input_tokens` → `PromptTokens`（**注意**：Anthropic 的 `input_tokens` 仅含 cache miss 的 fresh token，不含 cache 读写；解析时归一化为总输入 `input_tokens + cache_read_input_tokens + cache_creation_input_tokens`，与 OpenAI `prompt_tokens` 语义一致）；
  - `usage.output_tokens` → `CompletionTokens`；
  - `usage.cache_read_input_tokens` → `CacheReadTokens`；
  - `usage.cache_creation_input_tokens` → `CacheWriteTokens`；
  - `UsedQuota` 由归一化后的 `input + output` 推导。

### 5.5 `mod_body_process`

- `SSEEvent.GetQuotaUsage()` 与 `RawEvent.GetQuotaUsage()` 同样支持 Claude usage 字段 fallback；
- 继续负责流式场景的 `TTFT` / `TPOT` 计算。

### 5.6 `mod_access_pb3`

- `reqAiInfoGen()` 将 `AiBasicInfo.AuthStyle` 写入 `RequestLog.ai_protocol`（字段 717）。

---

## 6. 配置说明

### 6.1 `AIConf.ModelProtocols`

在 `cluster_conf.data` 的 `AIConf` 中配置：

```json
{
    "AIConf": {
        "Type": 0,
        "Provider": "my-anthropic",
        "ModelProtocols": ["anthropic"],
        "Keys": [
            {
                "Name": "key-primary",
                "Key": "sk-ant-api03-example",
                "Weight": 100
            }
        ],
        "ModelTable": {
            "Currency": "RMB",
            "Models": [
                {
                    "Provider": "my-anthropic",
                    "Model": "claude-3-5-sonnet",
                    "BaseModel": "claude-3-5-sonnet",
                    "Mode": "chat",
                    "Prices": {
                        "input_cost_per_token": 0.000003,
                        "output_cost_per_token": 0.000015,
                        "cache_read_input_token_cost": 0.000000375,
                        "cache_creation_input_token_cost": 0.00000375
                    }
                }
            ]
        }
    }
}
```

### 6.2 多协议集群

若同一个 provider 同时支持 OpenAI 与 Claude 协议（如聚合平台），可配置：

```json
"ModelProtocols": ["openai", "anthropic"]
```

BFE 会根据每个请求的 `AuthStyle` 自动选择认证头注入方式，不需要为不同协议创建不同集群。

---

## 7. 失败处理

| 场景 | 行为 |
|------|------|
| Anthropic 风格请求命中只支持 OpenAI 的集群 | 返回 400 `PROVIDER_PROTOCOL_MISMATCH` |
| OpenAI 风格请求命中只支持 Anthropic 的集群 | 返回 400 `PROVIDER_PROTOCOL_MISMATCH` |
| 同时存在 `Authorization` 和 `x-api-key` | 优先按 OpenAI 风格处理，保证现有客户端行为不变 |
| Anthropic 响应无 `total_tokens` | `UsedQuota` 由 `input_tokens + output_tokens` 推导 |

---

## 8. 访问日志

新增字段：

| 编号 | 字段名 | 类型 | 说明 | 采集模块 |
|------|--------|------|------|----------|
| 717 | `ai_protocol` | `string` | AI 协议/认证风格，如 `openai`、`anthropic` | `bfe_basic.GetApiKey` / `bfe_server/reverseproxy.go` |

> `ai_provider` 保存用户自定义的 provider 名称，无法准确反映协议类型；新增 `ai_protocol` 后，一个标识"哪个 provider"，一个标识"什么协议"。

---

## 9. 测试要点

### 9.1 单元测试

| 被测函数 | 场景 |
|---|---|
| `bfe_basic.GetApiKey` | `Authorization: Bearer xxx`；`x-api-key: xxx`；两者同时存在时的优先级；无 key |
| `bfe_basic.DetectAuthStyle` | `/v1/messages`、`x-api-key`、OpenAI 默认路径识别 |
| `mod_ai_token_auth.SetApiKey` | `openai` 风格生成 `Authorization: Bearer`；`anthropic` 风格生成 `x-api-key` |
| `mod_ai_token_auth.UpdateCtxByUsage` | OpenAI usage JSON；Claude usage JSON；混合字段时的优先级 |
| `mod_body_process.SSEEvent.GetQuotaUsage` | Claude 流式 usage 字段解析 |
| `mod_body_process.RawEvent.GetQuotaUsage` | Claude 非流式 usage 字段解析 |
| `bfe_server`（协议匹配） | Anthropic 风格请求命中 `model_protocols=["openai"]` 的集群返回 400；OpenAI 风格请求命中 `model_protocols=["anthropic"]` 的集群返回 400 |
| `mod_access_pb3` | `ai_protocol` 字段被正确填充为 `openai` 或 `anthropic` |

### 9.2 集成测试

| 用例编号 | 场景 | 预期 |
|---|---|---|
| SC02-TC-XX | 客户端 `POST /v1/messages`，带 `x-api-key` | upstream 收到 `x-api-key` 为选中的 cluster key，`anthropic-version` 被自动注入 |
| SC02-TC-XX | Claude 非流式响应返回 `input_tokens`/`output_tokens` | 访问日志 `ai_input_tokens`/`ai_output_tokens`/`ai_total_tokens` 正确；访问日志 `ai_protocol` 为 `anthropic` |
| SC02-TC-XX | Claude 流式响应返回 usage | 配额扣减与访问日志正确；访问日志 `ai_protocol` 为 `anthropic` |
| SC02-TC-XX | OpenAI 风格请求 | 访问日志 `ai_protocol` 为 `openai` |
| SC02-TC-XX | Anthropic 风格请求命中只支持 OpenAI 的集群 | BFE 返回 400 `PROVIDER_PROTOCOL_MISMATCH`，upstream 不被调用 |
| SC02-TC-XX | OpenAI 风格请求命中只支持 Claude 的集群 | BFE 返回 400 `PROVIDER_PROTOCOL_MISMATCH`，upstream 不被调用 |
| SC02-TC-XX | 同集群同时命中 OpenAI 与 Claude 路径 | 不同认证头不互相污染 |

---

## 10. 风险与回滚

| 风险 | 缓解措施 |
|---|---|
| 认证头冲突 | 明确优先级：`Authorization` > `x-api-key`，兼容现有 OpenAI 客户端 |
| `anthropic-version` 默认值过期 | 选择长期支持版本 `2023-06-01`；阶段一不引入额外配置字段 |
| 聚合平台模型名含 `anthropic/` 或 `claude` 但走 OpenAI 路径 | 不依赖 model 名判断，依赖路径/头/集群配置 |
| Claude 无 `total_tokens` | `UsedQuota` 由 `input + output` 推导 |
| 影响现有 OpenAI/DeepSeek 集群 | 默认 `AuthStyle == openai`，仅在明确识别为 Claude 时切换；`AIConf.ModelProtocols` 为空时默认只支持 OpenAI |
| 用 provider 名称硬编码协议风格 | provider 名称由用户自定义，不能再用 `"anthropic"` 判断；必须通过 `AIConf.ModelProtocols` 校验 |

**回滚**：该设计修改的是 BFE 二进制行为。若线上需要恢复旧行为，需回滚代码并重新发版；影响面仅涉及 AI 网关路径。

---

## 11. 参考文档

- `bfe/docs/zh_cn/modifications/2026-08-20-claude-protocol-support/design-changes.md`
- `bfe/docs/zh_cn/sys_design/ai_access_log_fields.md`
- `bfe/docs/zh_cn/sys_design/multi_api_key.md`
- `bfe/docs/zh_cn/sys_design/provider_model_prefix_routing.md`
- `bfe/docs/zh_cn/sys_design/rmb_quota.md`
- `bfe-access-pb/CHANGELOG.md`
- `bfe-access-pb/docs/protobuf.md`
