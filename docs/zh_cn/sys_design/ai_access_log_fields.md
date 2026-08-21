# BFE AI 访问日志可观测字段设计

## 1. 背景与目标

### 1.1 背景

BFE 作为 AI 网关，需要把请求在认证、路由、转发、计费等各阶段的关键信息输出到访问日志，供下游可观测平台进行分析、计费对账、故障排查和安全审计。

`bfe-access-pb` 访问日志协议为 AI 网关场景预留了编号 701-900 的字段区间。随着 BFE AI 网关能力从基础转发扩展到 RMB 配额、多 API-Key 重试、模型映射、路由规则命中、限流等场景，访问日志需要同步记录更多可观测信息。

### 1.2 目标

1. 统一记录 AI 请求全生命周期的可观测字段，覆盖认证、路由、转发、计费、限流、流式响应等环节；
2. 访问日志中不再记录原始 API Key 值，改为记录 API Key 内部标识（`key_id`），避免敏感信息泄露；
3. 字段命名与 `bfe-access-pb` 协议对齐，字段编号保持 701-900 区间规划；
4. 字段采集逻辑与业务模块解耦：各模块负责把运行时信息写入 `bfe_basic.Request` 的 AI 上下文，最终由 `mod_access_pb3` 统一组装输出。

---

## 2. 字段总览

AI 可观测字段统一占用 `bfe-access-pb` 的 701-900 编号区间，当前已定义 20 个字段：

| 编号 | 字段名 | 类型 | 说明 | 采集模块 |
|------|--------|------|------|----------|
| 701 | `ai_apikey_id` | `string` | API Key 内部标识（`key_id`），非原始 key 值 | `mod_ai_token_auth` |
| 702 | `ai_apikeytags` | `repeated ApikeyTag` | API Key 关联的 Entity 层级标签 | `mod_ai_token_auth` |
| 703 | `ai_requested_model` | `string` | 客户端请求原始模型名 | `bfe_server/http_conn.go` |
| 704 | `ai_target_model` | `string` | 网关实际路由/映射后的目标模型名 | `bfe_server/reverseproxy.go` |
| 705 | `ai_stream` | `bool` | 是否为流式响应 | `bfe_basic.Request.IsSse` |
| 706 | `ai_input_tokens` | `int64` | 输入 Token 数 | `mod_ai_token_auth` / `mod_body_process` |
| 707 | `ai_output_tokens` | `int64` | 输出 Token 数 | `mod_ai_token_auth` / `mod_body_process` |
| 708 | `ai_total_tokens` | `int64` | 总 Token 消耗 | `mod_ai_token_auth` |
| 709 | `ai_ttft_us` | `int64` | 首 Token 延迟（微秒），仅流式 | `mod_body_process` |
| 710 | `ai_tpot_us` | `int64` | 平均输出 Token 延迟（微秒），仅流式 | `mod_body_process` |
| 711 | `ai_rate_limit_hits` | `repeated RateLimitHit` | 触发的限流策略列表 | `mod_ai_rate_limit` |
| 712 | `ai_auth_reject_reason` | `string` | 鉴权拒绝原因 | `mod_ai_token_auth` |
| 713 | `ai_auth_reject_quota_plans` | `repeated string` | 拒绝时余额不足的 Quota Plan ID 列表 | `mod_ai_token_auth` |
| 714 | `ai_provider` | `string` | 上游模型提供商标识 | `bfe_server/reverseproxy.go` |
| 715 | `ai_retry_count` | `uint32` | 模型调用层 key-level 重试次数 | `bfe_server/reverseproxy.go` |
| 761 | `ai_cost_value` | `int64` | 估算成本（定点整数，精度由币种决定） | `mod_ai_token_auth` |
| 762 | `ai_cost_currency` | `string` | 成本币种，如 `RMB` / `USD` | `bfe_server/reverseproxy.go` |
| 801 | `ai_route_rule_hits` | `repeated AIRouteRuleHit` | 命中的 AI 路由规则列表 | `mod_ai_route` |
| 802 | `ai_cluster_key_names` | `repeated ClusterKeyName` | 请求处理过程中尝试过的 (cluster, key) 列表 | `bfe_server/reverseproxy.go` |
| 841 | `ai_auth_hit_quota_plans` | `repeated string` | 正常请求时命中的 Quota Plan ID 列表 | `mod_ai_token_auth` |

### 2.1 编号区间规划

| 编号区间 | 用途 |
|----------|------|
| 701 - 713 | 已投入使用字段，保持现状，不再调整 |
| 714 - 760 | 模型与请求基础信息（model、provider、stream、retry、cache 等） |
| 761 - 800 | Token 与成本计量 |
| 801 - 840 | 路由、转换与插件 |
| 841 - 880 | 安全、合规与隐私 |
| 881 - 900 | 厂商扩展与预留 |

---

## 3. 总体架构

```
┌─────────────────────────────────────────────────────────────┐
│                        客户端请求                            │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  bfe_server/http_conn.go                                    │
│  - 初始化 AiBasicInfo                                       │
│  - 提取 ai_apikey_id（原始 key，后续会被 key_id 覆盖）       │
│  - 提取 ai_requested_model                                   │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  HandleFoundProduct 回调链                                   │
│  ├─ mod_ai_token_auth: 校验 Token，设置 key_id、tags、       │
│  │   ai_auth_hit_quota_plans / ai_auth_reject_*              │
│  ├─ mod_ai_route: 设置 ai_route_rule_hits                    │
│  └─ mod_ai_rate_limit: 设置 ai_rate_limit_hits               │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  bfe_server/reverseproxy.go                                  │
│  - ServeHTTPForAI / aiClusterInvoke / doSingleAIForward      │
│  - 设置 ai_provider、ai_retry_count、                        │
│  │   ai_cluster_key_names、ai_target_model                   │
│  - 触发 cluster-level fallback                               │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  响应阶段                                                    │
│  ├─ mod_body_process: 计算 ai_ttft_us、ai_tpot_us            │
│  └─ mod_ai_token_auth: 解析 usage，计算 ai_input_tokens、    │
│     ai_output_tokens、ai_total_tokens、ai_cost_value         │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  mod_access_pb3                                              │
│  - 从 Request 上下文读取所有 AI 字段                         │
│  - 组装 RequestLog 并输出 b2log                              │
└─────────────────────────────────────────────────────────────┘
```

---

## 4. 核心数据结构

### 4.1 `bfe_basic.AiBasicInfo`

`bfe_basic/request_ai_basic.go`

```go
type AiBasicInfo struct {
    ClientApiKey    string
    ClientKeyId     string            // 701 ai_apikey_id
    ClientModel     string            // 703 ai_requested_model
    TargetModel     string            // 704 ai_target_model
    Provider        string            // 714 ai_provider
    RetryCount      uint32            // 715 ai_retry_count
    CostCurrency    string            // 762 ai_cost_currency
    tokenUsage      TokenUsage
    ApikeyTags      []ApikeyTag       // 702 ai_apikeytags
    TokenTimeInfo   TokenTimeInfo     // 709/710 ai_ttft_us / ai_tpot_us
    AiAuthInfo      AiAuthInfo        // 712/713/841
    ClusterKeyNames []ClusterKeyName  // 802 ai_cluster_key_names

    allowEstimateToken bool
}

type TokenUsage struct {
    PromptTokens     int64 // 706 ai_input_tokens
    CompletionTokens int64 // 707 ai_output_tokens
    UsedQuota        int64 // 708 ai_total_tokens
    UsedCost         int64 // 761 ai_cost_value
}

type AiAuthInfo struct {
    RejectReason     string   // 712 ai_auth_reject_reason
    RejectQuotaPlans []string // 713 ai_auth_reject_quota_plans
    HitQuotaPlans    []string // 841 ai_auth_hit_quota_plans
}

type ClusterKeyName struct {
    ClusterName string
    KeyName     string
}
```

### 4.2 `bfe_basic.AiRouteResult`

`bfe_basic/request_ai_route.go`

```go
type AiRouteResult struct {
    RouteType string // apikey / entity / global
    Owner     string // 路由表 owner
    RuleName  string // 命中规则名
    Targets   []AiRouteTarget
    Fallbacks []AiRouteFallback
}
```

`mod_access_pb3` 将其转换为 `AIRouteRuleHit`：

```protobuf
message AIRouteRuleHit {
    optional string rule_owner      = 1;
    optional string rule_owner_type = 2;
    optional string rule_name       = 3;
}
```

---

## 5. 模块职责

### 5.1 `bfe_server/http_conn.go`

在 `EnableAiGateway` 开启时初始化 `AiBasicInfo`：

- 从 `Authorization` 头提取原始 API Key，写入 `ClientApiKey`（后续会被 `ClientKeyId` 覆盖）；
- 从请求 JSON body 提取 `model` 字段，写入 `ClientModel` 和 `TargetModel`。

### 5.2 `mod_ai_token_auth`

- 在 `ValidateUserTokenByReq()` 中找到 Token 后，立即把 `Token.KeyId` 写入 `AiBasicInfo.ClientKeyId`，确保即使后续拒绝也能在日志中识别 key；
- 通过 `SetTokenAuthContext()` 写入 `ApikeyTags` 和初始 `PromptTokens`；
- 在响应阶段解析 `usage` 并估算 token，填充 `TokenUsage`；
- 对 RMB 配额调用 `calcCostUnits()` 计算 `UsedCost`；
- 在认证成功时记录 `HitQuotaPlans`，拒绝时记录 `RejectReason` 和 `RejectQuotaPlans`。

### 5.3 `mod_ai_route`

- `routeFoundProductHandler()` 调用 `routeTable.Search()` 得到 `AiRouteResult`；
- 通过 `req.SetAiRouteResult(result)` 把结果写入请求上下文。

### 5.4 `mod_ai_rate_limit`

- `limitFoundProductHandler()` 初始化 `AiRateLimitHitInfo`；
- 在 RPM/TPM/并发限流触发时，向 `HitPolicyDict[policyId]` 追加规则名；
- `mod_access_pb3` 读取后转换为 `RateLimitHit` 列表。

### 5.5 `bfe_server/reverseproxy.go`

在 `doSingleAIForward()` 中：

- 从 `cluster.AIConf.Provider` 写入 `AiBasicInfo.Provider`；
- 从 `cluster.AIConf.ModelTable.Currency` 写入 `AiBasicInfo.CostCurrency`；
- 调用 `AiBasicInfo.AppendClusterKeyName(cluster.Name, selectedKey.Name)` 记录尝试；
- 在 `ModelMapping`、路由 target/fallback model override、provider prefix strip 后更新 `TargetModel`。

在 `aiClusterInvoke()` 的 key-level retry 循环中：

- 当 `retry > 0` 时调用 `AiBasicInfo.IncrementRetryCount()`。

### 5.6 `mod_body_process`

- 在读取响应首包时记录 `TFirstToken`；
- 请求结束时记录 `TLastToken`；
- 调用 `calcTokenTime()` 计算 `TTFT` 和 `TPOT`。

### 5.7 `mod_access_pb3`

`reqAiInfoGen()` 负责把上述所有字段从 `AiBasicInfo`、`AiRateLimitHitInfo`、`AiRouteResult` 映射到 `RequestLog`：

- 字段重命名：`AiApikey`→`AiApikeyId`、`AiMappedModel`→`AiTargetModel`、`AiPromptTokens`→`AiInputTokens`；
- 新增字段：`AiProvider`、`AiRetryCount`、`AiCostValue`、`AiCostCurrency`、`AiRouteRuleHits`、`AiClusterKeyNames`、`AiAuthHitQuotaPlans`。

---

## 6. 安全与合规

1. **API Key 不落地**：`ai_apikey_id` 只记录内部 `key_id`，不记录原始 key 值。`ClientApiKey` 仍保留在内存中用于上游转发，但不会写入访问日志。
2. **成本精度**：`ai_cost_value` 使用定点整数（RMB 为 1e-8 元），避免浮点误差。
3. **字段可选**：所有 AI 字段均为 `optional`，未启用 AI 网关或非 AI 请求不会输出这些字段。

---

## 7. 测试与验证

1. **单元测试**：`bfe_modules/mod_access_pb3/request_log_test.go` 覆盖所有字段的赋值逻辑；
2. **集成测试**：`tests/integration/implementation/scenario-SC05-access-log-ai-fields/` 启动真实 BFE 进程，发送 AI 请求后解码 b2log，校验全部 20 个字段。

---

## 8. 参考文档

- `bfe-access-pb/docs/protobuf.md`
- `bfe/docs/zh_cn/modifications/2026-08-19-update-ai-access-log-fields/design-changes.md`
- `bfe/tests/integration/测试设计文档/scenario-SC05-AI访问日志字段校验/场景说明.md`
