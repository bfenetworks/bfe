# BFE AI 网关错误码说明

## 1. 背景

BFE 作为 AI 网关时，会在请求处理的各个阶段（认证、路由、限流、转发等）对异常情况进行统一封装，并返回结构化的 JSON 错误响应。本文档给出 BFE 数据面当前实际返回的错误码、响应格式、触发条件及排查建议。

BFE 相关实现参考了 AI 网关错误返回码方案的整体思路，并依据实际模块划分进行了落地。错误码定义位于 `bfe_basic/request_ai_basic.go`，主要由以下模块产生：

- `mod_ai_token_auth`：API Key 认证与配额校验
- `mod_ai_rate_limit`：RPM / TPM / 并发限流
- `bfe_server/reverseproxy.go`：AI 请求转发与协议适配
- `bfe_server/reverseproxy.go` 上游错误体归一（`AIConf.NormalizeUpstreamError`，见 §2.5；实现方案见 [修改说明](../modifications/2026-10-06-upstream-error-normalization/design-changes.md)）

## 2. HTTP 状态码与错误码映射表

### 2.1 认证与准入层

由 `mod_ai_token_auth` 在 `HandleFoundProduct` 阶段校验 API Key 时返回。

| 场景 | HTTP 状态码 | `error.code` | `error.type` | `error.message` 示例 | 触发条件 |
|------|-------------|--------------|--------------|---------------------|----------|
| 产品线不存在 | 400 | `INVALID_REQUEST` | `invalid_request_error` | "product not found." | 请求未匹配到 Product |
| 无 API Key | 401 | `NO_API_KEY` | `authentication_error` | "no api key in request." | 请求未携带 `Authorization: Bearer <key>` 或 `x-api-key` 头 |
| API Key 不存在 | 401 | `INVALID_API_KEY` | `authentication_error` | "Invalid API key: ak-xxxxx. Key not found in system." | 请求携带的 API Key 在系统中不存在 |
| API Key 已禁用 | 403 | `KEY_DISABLED` | `authentication_error` | "Invalid API key: ak-xxxxx. disabled." | API Key 的 `Enabled` 字段为 false |
| API Key 已过期 | 403 | `KEY_EXPIRED` | `authentication_error` | "Invalid API key: ak-xxxxx. expired." | API Key 的 `ExpiredTime` 已到期且非 -1 |
| 客户端 IP 不在白名单 | 403 | `SUBNET_NOT_ALLOWED` | `authentication_error` | "Client IP not in subnet of key ak-xxxxx." | 客户端 IP 不在 API Key 允许的子网范围内 |
| 请求模型不在白名单 | 400 | `MODEL_NOT_ALLOWED` | `invalid_request_error` | "Model gpt-5 not allowed by key ak-xxxxx." | 请求的模型不在 API Key 的允许列表中，或被加入黑名单 |

### 2.2 限流检查层

由 `mod_ai_rate_limit` 在 `HandleFoundProduct` 阶段根据 Redis 计数器或本地并发限制返回。

| 场景 | HTTP 状态码 | `error.code` | `error.type` | `error.message` 示例 | 触发条件 |
|------|-------------|--------------|--------------|---------------------|----------|
| RPM 窗口请求数超限 | 429 | `RPM_LIMIT_EXCEEDED` | `rate_limit_error` | "Rate limit exceeded for policy rlp-0001." | 固定窗口计数器：`used_requests + 1 > max_requests` |
| TPM 窗口 Token 数超限 | 429 | `TPM_LIMIT_EXCEEDED` | `rate_limit_error` | "Rate limit exceeded for policy rlp-0001." | 滑动窗口：`used_tokens + request_tokens > max_tokens` |
| 并发请求数超限 | 429 | `CONCURRENCY_LIMIT_EXCEEDED` | `rate_limit_error` | "Rate limit exceeded for policy rlp-0001." | 当前并发请求数达到或超过 `max_concurrency` |
| Redis 限流访问失败 | 500 | `RATE_LIMIT_REDIS_ERROR` | `rate_limit_error` | "Rate limit exceeded for policy rlp-0001." | 访问 Redis 计数器失败，且配置 `IsRejectOnRedisError` 为 true |

### 2.3 配额扣减层

由 `mod_ai_token_auth` 在请求进入时进行预扣配额校验返回。

| 场景 | HTTP 状态码 | `error.code` | `error.type` | `error.message` 示例 | 触发条件 |
|------|-------------|--------------|--------------|---------------------|----------|
| 配额包余额不足 | 429 | `QUOTA_EXHAUSTED` | `quota_error` | "Quota plan qplan-0001 exhausted." | Quota Plan 余额已扣减到 0 |
| 配额包已过期 | 429 | `QUOTA_EXPIRED` | `quota_error` | "Quota plan qplan-0001 expired." | `plan.ExpiredTime` 已到期且非 -1 |
| 配额 Redis 查询异常 | 500 | `INTERNAL_QUOTA_ERROR` | `internal_error` | "Internal error during quota deduction for plan qplan-0001: <error_msg>." | 查询 Redis 配额余额时发生内部错误 |

### 2.4 转发与协议适配层

由 `bfe_server/reverseproxy.go` 在 AI 请求转发阶段返回。

| 场景 | HTTP 状态码 | `error.code` | `error.type` | `error.message` 示例 | 触发条件 |
|------|-------------|--------------|--------------|---------------------|----------|
| 提供商协议不匹配 | 400 | `PROVIDER_PROTOCOL_MISMATCH` | `invalid_request_error` | "request protocol anthropic not supported by cluster provider (model_protocols=[openai])." | 请求的 `AuthStyle` 不在目标集群 `AIConf.ModelProtocols` 支持范围内 |

### 2.5 上游归一错误层（上游错误体归一）

由上游错误体归一产生（配置项 `AIConf.NormalizeUpstreamError`，详见 [cluster_conf.data 配置](../configuration/server_data_conf/cluster_conf.data.md)）：集群开启 `Enabled`（非流式）后，上游 4xx/5xx 错误被重写为统一错误体并按下表重映射状态码；上游原始状态码与原始错误码记录于 `error.details` 与访问日志字段。网关自生成错误（§2.1-§2.4）不参与归一。流式（SSE）错误不改写状态码（恒 200），仅错误事件载荷归一（`StreamEnabled`），另见 §5 截断标记。

| 场景 | HTTP 状态码 | `error.code` | `error.type` | `error.message` 示例 | 触发条件（上游信号） |
|------|-------------|--------------|--------------|---------------------|----------|
| 上游请求类错误 | 400 | `UPSTREAM_INVALID_REQUEST` | `invalid_request_error` | "upstream request invalid: <upstream_msg>." | 上游 400/422 `invalid_request_error` 等 |
| 上游上下文超限 | 400 | `CONTEXT_LENGTH_EXCEEDED` | `invalid_request_error` | "upstream context length exceeded." | 上游 `context_length_exceeded` / `request_too_large` |
| 上游内容审核拦截 | 400 | `CONTENT_FILTERED` | `invalid_request_error` | "upstream content filtered." | 上游 `content_policy_violation` / `content_filter` 等 |
| 上游模型不存在 | 404 | `UPSTREAM_MODEL_NOT_FOUND` | `invalid_request_error` | "upstream model not found: <model>." | 上游 `model_not_found` / `not_found_error` |
| 上游限流 | 429 | `UPSTREAM_RATE_LIMITED` | `rate_limit_error` | "upstream rate limited." | 上游 429 `rate_limit_exceeded` / `rate_limit_error` / `RESOURCE_EXHAUSTED`（限流语义） |
| 上游配额耗尽 | 429 | `UPSTREAM_QUOTA_EXHAUSTED` | `quota_error` | "upstream quota exhausted." | 上游 `insufficient_quota` / `RESOURCE_EXHAUSTED`（配额语义） |
| 上游凭证失效 | 502 | `UPSTREAM_AUTH_ERROR` | `internal_error` | "upstream auth failed: credential rejected by provider." | 上游 401/402/403（cluster key 被上游拒绝；与客户端自身凭证错误的 401 区分） |
| 上游过载 | 503 | `UPSTREAM_OVERLOADED` | `internal_error` | "upstream overloaded." | Anthropic `overloaded_error` / 529 |
| 上游内部错误 | 500 | `MODEL_INTERNAL_ERROR` | `internal_error` | "upstream model internal error." | 上游其余 5xx |
| 上游超时 | 504 | `BACKEND_TIMEOUT` | `internal_error` | "upstream backend timeout." | 上游 408 / 504 |
| 未识别错误强制归一 | 502 | `UPSTREAM_UNKNOWN` | `internal_error` | "upstream error unrecognized." | 识别为错误但无映射（仅 `UnrecognizedAction=rewrite_generic` 时产生；默认 `passthrough` 时原样透传，不产生本错误码） |

说明：

- 归一仅在转发重试（fallback）结束、最终结果确定后生效，不影响重试与 Key 罚分决策；
- 上游错误消息嵌入 `error.message` 前会经凭证脱敏（`RedactSecrets`，默认开启），集群 API-Key 的各编码形态替换为掩码；
- 配置未开启（或未配置该字段）时，上游错误（状态码与响应体）原样透传，不产生本节任何错误码。

## 3. 响应体结构规范

### 3.1 标准错误响应体（OpenAI 兼容格式）

所有数据面错误响应采用统一顶层结构：

```json
{
  "error": {
    "code": "QUOTA_EXHAUSTED",
    "type": "quota_error",
    "message": "Quota plan qplan-0001 exhausted.",
    "param": null,
    "details": {
      "api_key": "ak-2v8x9k3m7p",
      "key_id": "key-001",
      "quota_plan_id": "qplan-0001",
      "limit_type": "api_key_quota",
      "model": "gpt-4",
      "retry_after_seconds": 0
    }
  }
}
```

### 3.2 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `error` | object | 是 | 错误信息根对象 |
| `error.code` | string | 是 | 机器可读的错误子类型，采用大写下划线命名法 |
| `error.type` | string | 是 | 错误大类：`authentication_error`、`invalid_request_error`、`rate_limit_error`、`quota_error`、`internal_error` |
| `error.message` | string | 是 | 人类可读的错误描述，使用英文 |
| `error.param` | string / null | 否 | 出错的参数名，与 OpenAI 标准兼容，无具体参数时返回 null |
| `error.details` | object | 否 | 结构化详情，供客户端和自动化工具做精确决策 |
| `error.details.api_key` | string | 否 | API Key 标识（原始 key 值） |
| `error.details.key_id` | string | 否 | API Key 内部标识 |
| `error.details.quota_plan_id` | string | 否 | 配额计划 ID（配额类错误） |
| `error.details.limit_type` | string | 否 | 配额/限流维度：`api_key_quota`、`rpm`、`tpm`、`concurrency` |
| `error.details.model` | string | 否 | 请求中的模型标识 |
| `error.details.retry_after_seconds` | int | 否 | 建议等待秒数（限流类错误预留） |
| `error.details.upstream_status` | int | 否 | 上游原始 HTTP 状态码（§2.5 上游归一错误时存在） |
| `error.details.upstream_code` | string | 否 | 上游原始错误码（OpenAI `error.code` / Anthropic `error.type` / Gemini `error.status`），便于对照厂商文档 |

## 4. 预留错误码

以下错误码已在 `bfe_basic/request_ai_basic.go` 中定义，当前版本尚未主动返回：

| 错误码 | HTTP 状态码 | 错误类型 | 规划用途 |
|--------|-------------|----------|----------|
| `QUOTA_PACKAGE_DISABLED` | 429 | `quota_error` | Quota 套餐被禁用 |
| `QUOTA_MODEL_MISMATCH` | 429 | `quota_error` | 请求模型与配额套餐可用模型不匹配 |
| `QUOTA_PLAN_FAILED` | 429 | `quota_error` | 配额计划执行失败 |
| `INVALID_REQUEST_BODY` | 400 | `invalid_request_error` | 请求体非法 |
| `MODEL_PARAM_MISSING` | 400 | `invalid_request_error` | 缺少必要的模型参数 |
| `CONFIG_LOAD_ERROR` | 500 | `internal_error` | 配置加载失败 |
| `BACKEND_UNAVAILABLE` | 502 | `internal_error` | 后端模型服务不可用 |
| `USER_QUOTA_EXHAUSTED` | 429 | `quota_error` | 用户级别配额耗尽 |
| `SESSION_QUOTA_EXHAUSTED` | 429 | `quota_error` | 会话级别配额耗尽 |
| `FUNCTION_QUOTA_EXHAUSTED` | 429 | `quota_error` | 功能级别配额耗尽 |
| `COST_BUDGET_EXHAUSTED` | 429 | `quota_error` | 成本预算耗尽 |
| `GEO_RESTRICTED` | 403 | `authentication_error` | 地理区域限制 |
| `TIME_WINDOW_RESTRICTED` | 403 | `authentication_error` | 时间窗口限制 |

> `CONTEXT_LENGTH_EXCEEDED`、`CONTENT_FILTERED`、`MODEL_INTERNAL_ERROR`、`BACKEND_TIMEOUT` 四个错误码已由上游错误体归一激活（见 §2.5），不再属于预留。

## 5. 错误码与访问日志字段的关联

BFE AI 访问日志通过 `mod_access_pb3` 输出以下与错误相关的字段，便于排查和计费对账：

| 日志字段 | 说明 |
|----------|------|
| `ai_auth_reject_reason` | 认证/配额拒绝时的错误码，对应本文档中的 `code` 字段 |
| `ai_auth_reject_quota_plans` | 因配额不足被拒绝时，余量不足的 Quota Plan ID 列表 |
| `ai_auth_hit_quota_plans` | 认证通过且余额充足的 Quota Plan ID 列表 |
| `ai_rate_limit_hits` | 触发的限流策略及规则名列表 |
| `ai_upstream_status` | 上游原始 HTTP 状态码（上游归一生效时存在） |
| `ai_upstream_err_code` | 上游原始错误码（脱敏后内容） |
| `ai_err_normalized` | 上游归一是否生效：`1` 重写 / `0` 透传（含未识别、开关关闭） |
| `ai_err_normalize_miss` | 归一未识别标记（parser 返回 nil 的样本，用于补映射表） |
| `ai_stream_error_rewritten` | 流内错误事件改写标记（`StreamEnabled` 时：`1` = 本流至少一个错误事件被归一） |
| `ai_stream_truncated` | 流截断标记（`StreamEnabled` 时：`1` = EOF 时缺失协议终止事件） |

更多访问日志字段说明请参考 [BFE AI 访问日志可观测字段设计](./ai_access_log_fields.md)。

## 6. 排查建议

| 状态码 | 常见原因 | 建议 |
|--------|----------|------|
| 400 | 请求参数或模型不合法、协议不匹配、上游请求类错误 | 检查请求体、模型名称、协议风格与目标集群配置；`UPSTREAM_*` 归一错误对照 `details.upstream_code` 查厂商文档 |
| 401 | 缺少 API Key 或 Key 不存在 | 检查 `Authorization` / `x-api-key` 头及 Key 配置 |
| 403 | Key 被禁用/过期、IP 不在白名单 | 检查 Token 状态、过期时间及子网配置 |
| 404 | 上游模型不存在（`UPSTREAM_MODEL_NOT_FOUND`） | 核对 `ModelMapping` 与上游可用模型名 |
| 429 | 配额不足、触发限流或上游限流 | 网关限流/配额检查 Quota Plan 余额、RPM/TPM/并发策略阈值；`UPSTREAM_RATE_LIMITED`/`UPSTREAM_QUOTA_EXHAUSTED` 为上游侧限流，需稍后重试或扩容上游配额 |
| 500 | 内部错误、Redis 访问失败、上游内部错误 | 查看 BFE 错误日志及 Redis 连通性；`MODEL_INTERNAL_ERROR` 需联系上游 |
| 502 | 后端不可用，或上游凭证失效（`UPSTREAM_AUTH_ERROR`） | 检查后端模型服务健康状态及网络；`UPSTREAM_AUTH_ERROR` 需检查集群 API-Key 是否被上游禁用/删除 |
| 503 | 上游过载（`UPSTREAM_OVERLOADED`） | 稍后重试；评估上游容量与熔断策略 |
| 504 | 后端或上游超时（`BACKEND_TIMEOUT`） | 检查后端模型服务响应时间及超时配置 |

## 7. 参考

- `bfe_basic/request_ai_basic.go`：错误码与错误响应结构的 Go 语言定义
- `bfe_modules/mod_ai_token_auth/token_rule_table.go`：认证与配额错误产生逻辑
- `bfe_modules/mod_ai_rate_limit/mod_ai_rate_limit.go`：限流错误产生逻辑
- `bfe_server/reverseproxy.go`：协议适配错误产生逻辑；上游错误体归一拦截点（`normalizeUpstreamError` 与 SSE 事件过滤器）
- `bfe_model_protocol/utils/errors.go`：`ProtocolError`、`ErrorParser`/`StreamErrorParser` 解析接口定义
- [BFE 修改说明：上游错误体归一](../modifications/2026-10-06-upstream-error-normalization/design-changes.md)
- [cluster_conf.data 配置：AIConf.NormalizeUpstreamError](../configuration/server_data_conf/cluster_conf.data.md)
- [BFE AI 访问日志可观测字段设计](./ai_access_log_fields.md)
