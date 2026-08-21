# BFE 适配 bfe-access-pb AI 可观测字段升级设计变更

## 1. 背景

`bfe-access-pb` 访问日志协议已完成 AI 可观测字段的扩展与重命名（详见 `bfe-access-pb/docs/protobuf.md`）。主要变化包括：

1. **字段重命名**（保持编号不变）：
   - `ai_apikey` → `ai_apikey_id`（记录 API Key 内部标识，不记录原始 key 值）
   - `ai_mapped_model` → `ai_target_model`
   - `ai_prompt_tokens` → `ai_input_tokens`
2. **新增字段**：
   - `ai_provider`：上游模型提供商标识
   - `ai_retry_count`：模型调用层重试次数
   - `ai_cost_value` / `ai_cost_currency`：RMB 成本计量
   - `ai_route_rule_hits`：命中的 AI 路由规则列表
   - `ai_cluster_key_names`：请求处理过程中尝试过的 (cluster, key) 列表
   - `ai_auth_hit_quota_plans`：正常请求时命中的 Quota Plan ID 列表
3. **新增辅助消息**：
   - `AIRouteRuleHit`：描述命中路由规则的 owner / owner_type / name
   - `ClusterKeyName`：描述尝试过的 cluster 与 key 名称组合

BFE 当前仍使用旧版协议（`github.com/bfenetworks/bfe-access-pb v0.1.0`），并且访问日志代码中引用的是旧字段名（`AiApikey`、`AiMappedModel`、`AiPromptTokens`）。为使 BFE 与新版协议对齐，需要在 BFE 侧进行配套改造。

---

## 2. 目标

1. 将 BFE 依赖的 `bfe-access-pb` 升级到包含新字段的版本（建议 `v0.2.0` 或本地 replace）。
2. 修正访问日志中对重名字段的引用。
3. 将 `ai_apikey_id` 的数据源从原始 API Key 改为 API Key 内部 ID（`Token.KeyId` / `AiBasicInfo.ClientKeyId`）。
4. 在 BFE 各 AI 模块中补充新增字段的采集逻辑。
5. 更新单元测试 `bfe_modules/mod_access_pb3/request_log_test.go`。
6. 保持非 AI 请求和未升级配置场景下的向后兼容。

---

## 3. 变更总览

| Proto 字段 | 编号 | 旧 BFE 字段/状态 | 新 BFE 数据源 | 需修改的文件 |
|------------|------|------------------|---------------|--------------|
| `ai_apikey_id` | 701 | `AiApikey`（记录 `ClientApiKey`） | `AiBasicInfo.ClientKeyId` | `request_log.go` |
| `ai_apikeytags` | 702 | 已支持 | `AiBasicInfo.ApikeyTags` | 不变 |
| `ai_requested_model` | 703 | 已支持 | `AiBasicInfo.ClientModel` | 不变 |
| `ai_target_model` | 704 | `AiMappedModel` | `AiBasicInfo.TargetModel` | `request_log.go` |
| `ai_stream` | 705 | 已支持 | `req.IsSse` | 不变 |
| `ai_input_tokens` | 706 | `AiPromptTokens` | `TokenUsage.PromptTokens` | `request_log.go` |
| `ai_output_tokens` | 707 | 已支持 | `TokenUsage.CompletionTokens` | 不变 |
| `ai_total_tokens` | 708 | 已支持 | `TokenUsage.UsedQuota` | 不变 |
| `ai_ttft_us` | 709 | 已支持 | `TokenTimeInfo.TTFT` | 不变 |
| `ai_tpot_us` | 710 | 已支持 | `TokenTimeInfo.TPOT` | 不变 |
| `ai_rate_limit_hits` | 711 | 已支持 | `AiRateLimitHitInfo` | 不变 |
| `ai_auth_reject_reason` | 712 | 已支持 | `AiAuthInfo.RejectReason` | 不变 |
| `ai_auth_reject_quota_plans` | 713 | 已支持 | `AiAuthInfo.RejectQuotaPlans` | 不变 |
| `ai_provider` | 714 | **缺失** | `cluster.AIConf.Provider` | `request_ai_basic.go`, `reverseproxy.go` |
| `ai_retry_count` | 715 | **缺失** | `AiBasicInfo.RetryCount` | `request_ai_basic.go`, `reverseproxy.go` |
| `ai_cost_value` | 761 | **缺失** | `TokenUsage.UsedCost` | `request_log.go` |
| `ai_cost_currency` | 762 | **缺失** | `cluster.AIConf.ModelTable.Currency` | `request_ai_basic.go`, `reverseproxy.go`, `request_log.go` |
| `ai_route_rule_hits` | 801 | **缺失** | `AiRouteResult` → `AIRouteRuleHit` | `request_ai_route.go`, `request_log.go` |
| `ai_cluster_key_names` | 802 | **缺失** | `AiBasicInfo.ClusterKeyNames` | `request_ai_basic.go`, `reverseproxy.go`, `request_log.go` |
| `ai_auth_hit_quota_plans` | 841 | **缺失** | `AiAuthInfo.HitQuotaPlans` | `request_ai_basic.go`, `token_rule_table.go`, `request_log.go` |

---

## 4. 详细设计

### 4.1 升级 bfe-access-pb 依赖

**文件：** `bfe/go.mod`

将依赖版本从 `v0.1.0` 升级到包含新字段的版本（例如 `v0.2.0`）：

```go
require (
    ...
    github.com/bfenetworks/bfe-access-pb v0.2.0
    ...
)
```

本地开发时可临时启用 replace：

```go
replace github.com/bfenetworks/bfe-access-pb => ../bfe-access-pb
```

升级后执行：

```bash
cd bfe
go mod tidy
go mod download
```

> 注意：`bfe-access-pb` 的 `.pb.go` 文件需要在 Linux 环境下执行 `build.sh` 重新生成并打 tag 后，BFE 才能引用到正确版本。

---

### 4.2 扩展 `AiBasicInfo` 结构

**文件：** `bfe/bfe_basic/request_ai_basic.go`

在 `AiBasicInfo` 中新增以下字段：

```go
type AiBasicInfo struct {
    ClientApiKey  string
    ClientKeyId   string
    ClientModel   string
    TargetModel   string
    Provider      string            // 新增：上游 provider，如 openai / deepseek
    RetryCount    uint32            // 新增：模型调用层重试次数
    CostCurrency  string            // 新增：成本币种，如 RMB / USD
    tokenUsage    TokenUsage
    ApikeyTags    []ApikeyTag
    TokenTimeInfo TokenTimeInfo
    AiAuthInfo    AiAuthInfo
    ClusterKeyNames []ClusterKeyName // 新增：尝试过的 (cluster, key) 列表

    allowEstimateToken bool
}

// ClusterKeyName 描述一次尝试的 cluster 与 key 名称组合
type ClusterKeyName struct {
    ClusterName string
    KeyName     string
}
```

新增辅助方法：

```go
func (aiinfo *AiBasicInfo) AppendClusterKeyName(clusterName, keyName string) {
    aiinfo.ClusterKeyNames = append(aiinfo.ClusterKeyNames, ClusterKeyName{
        ClusterName: clusterName,
        KeyName:     keyName,
    })
}

func (aiinfo *AiBasicInfo) IncrementRetryCount() {
    aiinfo.RetryCount++
}
```

---

### 4.3 扩展 `AiAuthInfo` 结构

**文件：** `bfe/bfe_basic/request_ai_basic.go`

在 `AiAuthInfo` 中新增 `HitQuotaPlans`，用于记录成功鉴权时参与余额检查并放行的 Quota Plan ID：

```go
type AiAuthInfo struct {
    RejectReason     string   // 拒绝原因
    RejectQuotaPlans []string // 拒绝时余额不足的 Quota Plan IDs
    HitQuotaPlans    []string // 新增：成功时命中的 Quota Plan IDs
}
```

---

### 4.4 模块数据填充修改

#### 4.4.1 `mod_ai_token_auth`：记录命中配额计划

**文件：** `bfe/bfe_modules/mod_ai_token_auth/token_rule_table.go`

在 `ValidateUserTokenByReq` 中，当 Quota Plan 通过余额检查（`hasBalance == true`）时，将其 ID 记录到 `AiAuthInfo.HitQuotaPlans`：

```go
// 在 for _, plan := range token.QuotaPlans 循环内
if !hasBalance {
    SetAiAuthInfo(req, bfe_basic.CodeQuotaExhausted, []string{plan.Id})
    ...
}
// 新增：记录成功命中的 quota plan
aiBasicInfo := req.GetAiBasicInfo()
if aiBasicInfo != nil {
    aiBasicInfo.AiAuthInfo.HitQuotaPlans = append(aiBasicInfo.AiAuthInfo.HitQuotaPlans, plan.Id)
}
```

> 说明：`Unlimited` 或 `PassNoQuota` 的 plan 不经过 `HasBalance` 检查，因此不会进入 `HitQuotaPlans`。这符合语义：`HitQuotaPlans` 仅记录实际参与余额校验并命中的计划。

#### 4.4.2 `bfe_server/reverseproxy.go`：记录 provider、currency、retry、cluster/key

**文件：** `bfe/bfe_server/reverseproxy.go`

**A. 在 `doSingleAIForward` 中记录 provider、currency 和 cluster/key：**

```go
func (p *ReverseProxy) doSingleAIForward(..., selectedKey cluster_conf.AIKey) (...) {
    ...
    if cluster.AIConf != nil && aiMeta != nil {
        if cluster.AIConf.Provider != "" {
            aiMeta.Provider = cluster.AIConf.Provider
        }
        if cluster.AIConf.ModelTable != nil && cluster.AIConf.ModelTable.Currency != "" {
            aiMeta.CostCurrency = cluster.AIConf.ModelTable.Currency
        }
        aiMeta.AppendClusterKeyName(cluster.Name, selectedKey.Name)
    }
    ...
}
```

> 注意：需要确认 `cluster` 对象是否有 `Name` 字段；如果没有，使用 `attempt.ClusterName`。

**B. 在 `aiClusterInvoke` 中统计重试次数：**

```go
for retry := 0; retry <= policy.MaxRetries; retry++ {
    if retry > 0 {
        if aiMeta != nil {
            aiMeta.IncrementRetryCount()
        }
        ...
    }
    ...
}
```

> 说明：`RetryCount` 只统计同一 cluster 内 key-level 的重试次数，与 HTTP 层 `basicReq.RetryTime` 解耦。fallback 到另一个 cluster 时，该计数器不累加（符合协议语义）。

---

### 4.5 访问日志赋值修改

**文件：** `bfe/bfe_modules/mod_access_pb3/request_log.go`

将 `reqAiInfoGen` 函数更新为使用新字段名并填充新增字段：

```go
func reqAiInfoGen(reqLog *bfe_access_pb3.RequestLog, req *bfe_basic.Request, res *bfe_http.Response) {
    aiInfo := req.GetAiBasicInfo()
    if aiInfo == nil {
        return
    }

    // API Key ID（不再记录原始 key）
    if aiInfo.ClientKeyId != "" {
        reqLog.AiApikeyId = proto.String(aiInfo.ClientKeyId)
    }

    // API Key Tags
    if len(aiInfo.ApikeyTags) > 0 {
        for _, tag := range aiInfo.ApikeyTags {
            reqLog.AiApikeytags = append(reqLog.AiApikeytags, &bfe_access_pb3.ApikeyTag{
                Tagname:  proto.String(tag.TagName),
                Tagvalue: proto.String(tag.TagValue),
            })
        }
    }

    // Model
    if aiInfo.ClientModel != "" {
        reqLog.AiRequestedModel = proto.String(aiInfo.ClientModel)
    }
    if aiInfo.TargetModel != "" {
        reqLog.AiTargetModel = proto.String(aiInfo.TargetModel)
    }

    // Provider
    if aiInfo.Provider != "" {
        reqLog.AiProvider = proto.String(aiInfo.Provider)
    }

    // Stream
    reqLog.AiStream = proto.Bool(isStreamResponse(req, res))

    // Token usage
    usage := aiInfo.GetTokenUsage()
    if usage != nil {
        reqLog.AiInputTokens = proto.Int64(usage.PromptTokens)
        reqLog.AiOutputTokens = proto.Int64(usage.CompletionTokens)
        reqLog.AiTotalTokens = proto.Int64(usage.UsedQuota)
        if usage.UsedCost > 0 {
            reqLog.AiCostValue = proto.Int64(usage.UsedCost)
        }
    }

    // Cost currency
    if aiInfo.CostCurrency != "" {
        reqLog.AiCostCurrency = proto.String(aiInfo.CostCurrency)
    }

    // Retry count
    if aiInfo.RetryCount > 0 {
        reqLog.AiRetryCount = proto.Uint32(aiInfo.RetryCount)
    }

    // TTFT / TPOT
    ti := aiInfo.TokenTimeInfo
    if ti.TTFT > 0 {
        reqLog.AiTtftUs = proto.Int64(ti.TTFT)
    }
    if ti.TPOT > 0 {
        reqLog.AiTpotUs = proto.Int64(ti.TPOT)
    }

    // Auth reject info
    if len(aiInfo.AiAuthInfo.RejectReason) > 0 {
        reqLog.AiAuthRejectReason = proto.String(aiInfo.AiAuthInfo.RejectReason)
    }
    for _, item := range aiInfo.AiAuthInfo.RejectQuotaPlans {
        reqLog.AiAuthRejectQuotaPlans = append(reqLog.AiAuthRejectQuotaPlans, item)
    }
    for _, item := range aiInfo.AiAuthInfo.HitQuotaPlans {
        reqLog.AiAuthHitQuotaPlans = append(reqLog.AiAuthHitQuotaPlans, item)
    }

    // Route rule hits
    if routeResult := req.GetAiRouteResult(); routeResult != nil {
        reqLog.AiRouteRuleHits = append(reqLog.AiRouteRuleHits, &bfe_access_pb3.AIRouteRuleHit{
            RuleOwner:     proto.String(routeResult.Owner),
            RuleOwnerType: proto.String(routeResult.RouteType),
            RuleName:      proto.String(routeResult.RuleName),
        })
    }

    // Cluster / key attempts
    for _, ckn := range aiInfo.ClusterKeyNames {
        reqLog.AiClusterKeyNames = append(reqLog.AiClusterKeyNames, &bfe_access_pb3.ClusterKeyName{
            ClusterName: proto.String(ckn.ClusterName),
            KeyName:     proto.String(ckn.KeyName),
        })
    }

    // Rate limit hit info（保持不变）
    hitInfo := req.GetAiRateLimitHitInfo()
    if hitInfo != nil && len(hitInfo.HitPolicyDict) > 0 {
        for policyId, info := range hitInfo.HitPolicyDict {
            ...
        }
    }
}
```

---

## 5. 涉及文件清单

| 文件 | 修改内容 |
|------|----------|
| `bfe/go.mod` | 升级 `bfe-access-pb` 到 `v0.2.0`（或启用 replace） |
| `bfe/go.sum` | 随 `go mod tidy` 自动更新 |
| `bfe/bfe_basic/request_ai_basic.go` | `AiBasicInfo` 新增 `Provider`、`RetryCount`、`CostCurrency`、`ClusterKeyNames`；新增 `ClusterKeyName` 结构及辅助方法；`AiAuthInfo` 新增 `HitQuotaPlans` |
| `bfe/bfe_basic/request_ai_route.go` | 可选：为 `AiRouteResult` 增加导出方法，便于 `request_log.go` 读取 |
| `bfe/bfe_modules/mod_ai_token_auth/token_rule_table.go` | `ValidateUserTokenByReq` 中记录成功命中的 `HitQuotaPlans` |
| `bfe/bfe_server/reverseproxy.go` | `doSingleAIForward` 记录 provider / currency / cluster-key；`aiClusterInvoke` 统计 retry count |
| `bfe/bfe_modules/mod_access_pb3/request_log.go` | `reqAiInfoGen` 使用新字段名并填充新增字段 |
| `bfe/bfe_modules/mod_access_pb3/request_log_test.go` | 更新测试断言，覆盖新字段 |

---

## 6. 测试计划

### 6.1 单元测试

**文件：** `bfe/bfe_modules/mod_access_pb3/request_log_test.go`

更新 `TestReqAiInfoGen`：

1. 将 `ClientApiKey` 替换为 `ClientKeyId`，并断言 `AiApikeyId`。
2. 将 `AiMappedModel` 断言改为 `AiTargetModel`。
3. 将 `AiPromptTokens` 断言改为 `AiInputTokens`。
4. 新增断言：
   - `AiProvider`
   - `AiRetryCount`
   - `AiCostValue` / `AiCostCurrency`
   - `AiRouteRuleHits`
   - `AiClusterKeyNames`
   - `AiAuthHitQuotaPlans`

示例补充：

```go
aiInfo := &bfe_basic.AiBasicInfo{
    ClientKeyId:   "key-id-123",
    ClientModel:   "model-a",
    TargetModel:   "model-b",
    Provider:      "deepseek",
    RetryCount:    1,
    CostCurrency:  "RMB",
    ClusterKeyNames: []bfe_basic.ClusterKeyName{
        {ClusterName: "cluster-a", KeyName: "key-001"},
    },
    ...
}
usage.UsedCost = 5000 // 1e-8 元
```

### 6.2 编译验证

```bash
cd bfe
go build ./...
go test ./bfe_modules/mod_access_pb3/...
```

### 6.3 集成验证

1. 启用 AI 网关，发起一次带 API Key 的模型请求。
2. 收集 `mod_access_pb3` 输出的 b2log，解码 `RequestLog`。
3. 校验字段：
   - `ai_apikey_id` 等于 Token 的 `key_id`，而不是原始 `key`。
   - `ai_target_model` 正确反映路由/映射后的模型。
   - `ai_provider`、`ai_retry_count`、`ai_cost_value`、`ai_cost_currency` 非空（RMB 配额场景）。
   - `ai_route_rule_hits`、`ai_cluster_key_names`、`ai_auth_hit_quota_plans` 与请求行为一致。

---

## 7. 兼容性说明

1. **字段重命名**：proto 字段编号不变（701、704、706），因此 protobuf 二进制层面完全兼容；变化只体现在生成代码的 Go 字段名上。
2. **语义变化**：`ai_apikey_id` 从记录原始 API Key 改为记录内部 `key_id`，避免在日志中泄露敏感信息。需要确认上游日志消费方不再依赖原始 key 值。
3. **新增字段**：均为 `optional`，对未升级的旧 BFE 版本无影响。
4. **版本依赖**：升级 `bfe-access-pb` 后，旧 BFE 代码无法直接编译，因此这是一个需要同步发布的破坏性变更（仅对 BFE 代码编译层面）。

---

## 8. 风险与回滚

### 8.1 主要风险

| 风险 | 说明 | 规避措施 |
|------|------|----------|
| 编译失败 | 新 proto 字段名与旧 BFE 代码不匹配 | 按本方案一次性更新所有引用 |
| 日志消费方依赖旧字段名 | 下游解析 `ai_apikey`、`ai_mapped_model`、`ai_prompt_tokens` 会失败 | 提前通知下游，按 proto 编号而非字段名解析；或在下游做映射 |
| `ai_apikey_id` 为空 | 如果 `Token.KeyId` 未配置，日志中将缺失 key 标识 | 确保 `ai-gateway-api` 导出的 Token 配置始终包含 `key_id` |
| 重试计数语义不清 | `RetryCount` 仅统计 key-level 重试，不统计 cluster fallback | 文档中明确语义；访问日志中已有 `backend_retry` 字段记录 HTTP 层重试 |

### 8.2 回滚方案

如需回滚到旧协议：

1. 将 `bfe/go.mod` 中的 `bfe-access-pb` 版本改回 `v0.1.0`。
2. 回滚 `request_log.go` 到旧字段名（`AiApikey`、`AiMappedModel`、`AiPromptTokens`）。
3. 移除 `request_ai_basic.go`、`reverseproxy.go`、`token_rule_table.go` 中新增字段的采集逻辑。
4. 重新编译部署。

> 注意：回滚后新字段（provider、retry、cost 等）将不再输出到日志。

---

## 9. 后续可选扩展

1. **`ai_route_rule_hits` 支持多条命中记录**：当前 `AiRouteResult` 只记录最终命中的规则。未来如果路由模块支持记录所有匹配规则，可将 `HitPolicyDict` 式的列表写入日志。
2. **`ai_cluster_key_names` 区分成功与失败尝试**：当前记录所有尝试；可扩展为标记最终成功的 key。
3. **`ai_auth_hit_quota_plans` 与 RMB 扣减计划对齐**：当前记录所有通过余额检查的 plan；可与实际扣减计划做交叉验证。

---

*文档生成日期：2026-08-19*
