# BFE AI 网关错误码说明

## 1. 背景

BFE 作为 AI 网关时，会在请求处理的各个阶段（认证、路由、限流、转发等）对异常情况进行统一封装，并返回结构化的 JSON 错误响应。本文档给出 BFE 数据面当前实际返回的错误码、响应格式、触发条件及排查建议。

BFE 相关实现参考了 AI 网关错误返回码方案的整体思路，并依据实际模块划分进行了落地。错误码定义位于 `bfe_basic/request_ai_basic.go`，主要由以下模块产生：

- `mod_ai_token_auth`：API Key 认证与配额校验
- `mod_ai_rate_limit`：RPM / TPM / 并发限流
- `bfe_server/reverseproxy.go`：AI 请求转发与协议适配

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
| 目标集群无可用 Key（issue #1441） | 429 | `NO_AVAILABLE_KEY` | `rate_limit_error` | "no available upstream api key for this cluster" | 一次选 Key 均未成功：全部 Key 被惩罚（熔断，issue #1439）/ `Weight=0` / dead；响应携带 `Retry-After` 头与 `error.details.retry_after_seconds`（= 429 惩罚 TTL，60s） |
| 网关侧后端不可用（issue #1441） | 502 | `BACKEND_UNAVAILABLE` | `internal_error` | "upstream backend unavailable for this request" | 目标集群不存在 / 后端连接或传输失败（`ServeHTTPForAI` 的 `err != nil || res == nil` 出口）；客户端中断（`CLIENT_WRITE/CLOSE/RESET`）路径保持既有空体记录，不构造响应体 |

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
| `error.details.retry_after_seconds` | int | 否 | 建议等待秒数；已在 `NO_AVAILABLE_KEY`（429，issue #1441）路径使用，并同步输出 HTTP `Retry-After` 响应头（秒数形式） |

## 4. 预留错误码

以下错误码已在 `bfe_basic/request_ai_basic.go` 中定义，用于后续扩展场景，当前版本尚未主动返回：

| 错误码 | HTTP 状态码 | 错误类型 | 规划用途 |
|--------|-------------|----------|----------|
| `QUOTA_PACKAGE_DISABLED` | 429 | `quota_error` | Quota 套餐被禁用 |
| `QUOTA_MODEL_MISMATCH` | 429 | `quota_error` | 请求模型与配额套餐可用模型不匹配 |
| `QUOTA_PLAN_FAILED` | 429 | `quota_error` | 配额计划执行失败 |
| `CONTEXT_LENGTH_EXCEEDED` | 400 | `invalid_request_error` | 上下文长度超过模型限制 |
| `CONTENT_FILTERED` | 400 | `invalid_request_error` | 请求或响应内容被安全策略过滤 |
| `INVALID_REQUEST_BODY` | 400 | `invalid_request_error` | 请求体非法 |
| `MODEL_PARAM_MISSING` | 400 | `invalid_request_error` | 缺少必要的模型参数 |
| `MODEL_INTERNAL_ERROR` | 500 | `internal_error` | 模型服务返回内部错误 |
| `BACKEND_TIMEOUT` | 504 | `internal_error` | 后端模型服务响应超时 |
| `BACKEND_UNAVAILABLE` | 502 | `internal_error` | 后端模型服务不可用 |
| `CONFIG_LOAD_ERROR` | 500 | `internal_error` | 配置加载失败 |
| `USER_QUOTA_EXHAUSTED` | 429 | `quota_error` | 用户级别配额耗尽 |
| `SESSION_QUOTA_EXHAUSTED` | 429 | `quota_error` | 会话级别配额耗尽 |
| `FUNCTION_QUOTA_EXHAUSTED` | 429 | `quota_error` | 功能级别配额耗尽 |
| `COST_BUDGET_EXHAUSTED` | 429 | `quota_error` | 成本预算耗尽 |
| `GEO_RESTRICTED` | 403 | `authentication_error` | 地理区域限制 |
| `TIME_WINDOW_RESTRICTED` | 403 | `authentication_error` | 时间窗口限制 |

## 5. 错误码与访问日志字段的关联

BFE AI 访问日志通过 `mod_access_pb3` 输出以下与错误相关的字段，便于排查和计费对账：

| 日志字段 | 说明 |
|----------|------|
| `ai_auth_reject_reason` | 认证/配额拒绝时的错误码，对应本文档中的 `code` 字段 |

> 说明：转发层产生的网关侧错误码（如 `NO_AVAILABLE_KEY`、`BACKEND_UNAVAILABLE`）**不写入** `ai_auth_reject_reason`（该字段语义为认证/配额拒绝），而是由 `err_code` 承载（issue #1441 裁定）；转发失败场景下 `err_code` 的既有值（`BK_NO_CLUSTER`、`BK_CONNECT_BACKEND` 等）保持不变。
| `ai_auth_reject_quota_plans` | 因配额不足被拒绝时，余量不足的 Quota Plan ID 列表 |
| `ai_auth_hit_quota_plans` | 认证通过且余额充足的 Quota Plan ID 列表 |
| `ai_rate_limit_hits` | 触发的限流策略及规则名列表 |

更多访问日志字段说明请参考 [BFE AI 访问日志可观测字段设计](./ai_access_log_fields.md)。

## 6. 排查建议

| 状态码 | 常见原因 | 建议 |
|--------|----------|------|
| 400 | 请求参数或模型不合法、协议不匹配 | 检查请求体、模型名称、协议风格与目标集群配置 |
| 401 | 缺少 API Key 或 Key 不存在 | 检查 `Authorization` / `x-api-key` 头及 Key 配置 |
| 403 | Key 被禁用/过期、IP 不在白名单 | 检查 Token 状态、过期时间及子网配置 |
| 429 | 配额不足或触发限流 | 检查 Quota Plan 余额、RPM/TPM/并发策略阈值 |
| 500 | 内部错误或 Redis 访问失败 | 查看 BFE 错误日志及 Redis 连通性 |
| 502 | 后端不可用或目标集群不存在 | 检查后端模型服务健康状态、网络及 cluster/route 配置 |
| 504 | 后端响应超时 | 检查后端模型服务健康状态及网络 |

## 7. 参考

- `bfe_basic/request_ai_basic.go`：错误码与错误响应结构的 Go 语言定义
- `bfe_modules/mod_ai_token_auth/token_rule_table.go`：认证与配额错误产生逻辑
- `bfe_modules/mod_ai_rate_limit/mod_ai_rate_limit.go`：限流错误产生逻辑
- `bfe_server/reverseproxy.go`：协议适配错误产生逻辑
- [BFE AI 访问日志可观测字段设计](./ai_access_log_fields.md)
