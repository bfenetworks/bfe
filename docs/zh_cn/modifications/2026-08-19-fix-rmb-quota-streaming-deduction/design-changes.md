# 修复 BFE RMB 配额在流式响应（SSE）下不扣费的问题

## 1. 背景

BFE 在 AI 网关场景下支持两种配额维度：

- **Token 配额**：按 `total_tokens` / `prompt_tokens` + `completion_tokens` 扣减。
- **RMB 配额**：按模型定价表计算的输入/输出成本扣减（单位 RMB，内部使用定点数）。

当前线上配置中，RMB 配额适用于部分产品和模型。当客户端以 `stream: true` 调用大模型接口时，上游返回 `text/event-stream`（SSE）流式响应。用户观察到：

- 上游 DeepSeek 已实际扣费；
- BFE 侧 Redis 中仅出现认证阶段的 `GET QUOTA_...` 余额检查；
- 请求结束后没有 `DECRBY` 扣减动作。

该问题已被记录为 GitHub issue：https://github.com/bfenetworks/bfe/issues/1316

---

## 2. 问题现象

### 2.1 用户配置

`token_rule.data`：

```json
{
    "Id": "AI_product-ZEAoKAKdGnPpck1uPoUsdNCb",
    "Unit": "RMB",
    "Quota": 500000000,
    "RedisKey": "QUOTA_AI_product-ZEAoKAKdGnPpck1uPoUsdNCb",
    ...
}
```

`cluster_conf.data`：

```json
"deepseek-backup": {
    "AIConf": {
        "Keys": [...],
        "ModelTable": {
            "Currency": "RMB",
            "Models": [
                {
                    "Model": "deepseek-v4-flash",
                    "BaseModel": "deepseek-v4-flash",
                    "Mode": "chat",
                    "Prices": {
                        "input_cost_per_token": 0.000003,
                        "output_cost_per_token": 0.000009,
                        ...
                    }
                }
            ]
        }
    }
}
```

### 2.2 观测到的 Redis 行为

```
1787130732.988709 [0 172.19.1.222:51706] "GET" "QUOTA_AI_product-ZEAoKAKdGnPpck1uPoUsdNCb"
```

只有一条 `GET`，没有 `DECRBY`。

---

## 3. 根因分析

### 3.1 RMB 成本计算被 `ContentLength >= 0` 条件卡住

`bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:139-151`：

```go
func (m *ModuleAITokenAuth) tokenReadResponseHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
    ctx := GetTokenAuthContext(req)
    if ctx == nil {
        return bfe_module.BfeHandlerGoOn
    }
    tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
    if res.StatusCode == bfe_http.StatusOK && res.ContentLength >= 0 {   // ← 问题在这里
        if bodyAccessor, err := res.GetBodyAccessor(); err == nil {
            body, _ := bodyAccessor.GetBytes()
            UpdateCtxByUsage(ctx, body)
        }
        if tokenUsage.UsedQuota <= 0 && ctx.aiBasicInfo.IsAllowEstimateToken() {
            tokenUsage.CompletionTokens = int64(res.ContentLength) / 4
            tokenUsage.UsedQuota = CalcReqUsedQuota(req, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
        }
        // calculate RMB cost while SvrDataConf is still available
        if hasRMBPlan(ctx.Token.QuotaPlans) {
            tokenUsage.UsedCost = m.calcCostUnits(req, ctx.serverConf, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
        }
    }

    return bfe_module.BfeHandlerGoOn
}
```

`RMB` 成本计算（`calcCostUnits`）和响应体 `usage` 解析（`UpdateCtxByUsage`）都被包在 `res.ContentLength >= 0` 条件内部。

### 3.2 流式响应的 `ContentLength` 恒为 `-1`

`bfe/bfe_modules/mod_body_process/body_process.go:343-345`：

```go
res.Body = bp
res.ContentLength = -1 // 设置为-1表示不确定长度
res.Header.Del("Content-Length")
```

只要请求经过 `mod_body_process`（生产环境通常启用），或者上游返回 chunked/SSE 响应，`ContentLength` 就是 `-1`。结果：

- `UpdateCtxByUsage` 不会执行；
- `calcCostUnits` 不会执行；
- `tokenUsage.UsedCost` 保持 `0`。

### 3.3 完成阶段只扣已计算的 `UsedCost`

`bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:165-208`：

```go
func (m *ModuleAITokenAuth) tokenRequestFinishHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
    ...
    tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
    if tokenUsage.UsedQuota <= 0 && ctx.aiBasicInfo.IsAllowEstimateToken() {
        tokenUsage.UsedQuota = CalcReqUsedQuota(req, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
    }

    // use RMB cost calculated at response-read stage (SvrDataConf may be nil here)
    costUnits := tokenUsage.UsedCost   // 流式下为 0

    if tokenUsage.UsedQuota > 0 || costUnits > 0 {
        for _, plan := range ctx.Token.QuotaPlans {
            if plan.Unlimited {
                continue
            }
            if quota.IsRMB(plan.Unit) {
                if costUnits > 0 {     // 永远不成立
                    _, err := plan.Deduct(m.redisClient, costUnits)
                    ...
                }
            } else {
                if tokenUsage.UsedQuota > 0 {
                    _, err := plan.Deduct(m.redisClient, tokenUsage.UsedQuota)
                    ...
                }
            }
        }
    }

    return bfe_module.BfeHandlerGoOn
}
```

完成阶段不再重新计算成本，而是直接使用 `tokenUsage.UsedCost`。对于流式响应，该值为 `0`，因此 `quota.IsRMB(plan.Unit)` 分支不会触发 `Deduct`。

### 3.4 `mod_body_process` 已经收集了流式 token 用量

`bfe/bfe_modules/mod_body_process/content_quota_usage.go:27-75`：

```go
type QuotaUsageProcessor struct {
    aiBasicInfo *bfe_basic.AiBasicInfo
}

func NewQuotaUsageProcessor(req *bfe_basic.Request, res *bfe_http.Response) *QuotaUsageProcessor {
    if res.StatusCode != bfe_http.StatusOK {
        return nil
    }
    aiBasicInfo := req.GetAiBasicInfo()
    return &QuotaUsageProcessor{aiBasicInfo: aiBasicInfo}
}

func (caf *QuotaUsageProcessor) Process(events []Event) ([]Event, error) {
    tctx := caf.aiBasicInfo.GetTokenUsage()
    for _, ev := range events {
        ...
        if !rquota.IsGuess {
            if rquota.UsedQuota > 0 {
                tctx.CompletionTokens = rquota.CompletionTokens
                tctx.PromptTokens = rquota.PromptTokens
                tctx.UsedQuota = rquota.UsedQuota
            } else if rquota.PromptTokens > 0 || rquota.CompletionTokens > 0 {
                tctx.UsedQuota = rquota.PromptTokens + rquota.CompletionTokens
                tctx.PromptTokens = rquota.PromptTokens
                tctx.CompletionTokens = rquota.CompletionTokens
            }
        }
        ...
    }
    return events, nil
}
```

也就是说，流式场景下 `PromptTokens` / `CompletionTokens` 在请求结束时已经可用，只是没有被用来计算 `UsedCost`。

---

## 4. 目标

1. 修复流式响应（SSE）下 RMB 配额不扣费的问题。
2. 保持非流式响应的 RMB 扣费行为不变。
3. 保持 Token 配额的扣费行为不变。
4. 降低对模块加载顺序和 `ContentLength` 的依赖。
5. 补充单元测试覆盖流式 + RMB 扣费路径。

---

## 5. 变更方案

### 5.1 核心改动

将 RMB 成本计算从 `tokenReadResponseHandler` 移到 `tokenRequestFinishHandler`，并在完成阶段基于此时已填充的 `PromptTokens` / `CompletionTokens` 进行计算。

**文件：** `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`

#### 5.1.1 `tokenReadResponseHandler`

保留响应体 `usage` 解析和 token 估算逻辑（供非流式/未启用 `mod_body_process` 场景使用），但**不再**在此处计算 `UsedCost`。

```go
func (m *ModuleAITokenAuth) tokenReadResponseHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
    ctx := GetTokenAuthContext(req)
    if ctx == nil {
        return bfe_module.BfeHandlerGoOn
    }
    tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
    if res.StatusCode == bfe_http.StatusOK && res.ContentLength >= 0 {
        if bodyAccessor, err := res.GetBodyAccessor(); err == nil {
            body, _ := bodyAccessor.GetBytes()
            UpdateCtxByUsage(ctx, body)
        }
        if tokenUsage.UsedQuota <= 0 && ctx.aiBasicInfo.IsAllowEstimateToken() {
            tokenUsage.CompletionTokens = int64(res.ContentLength) / 4
            tokenUsage.UsedQuota = CalcReqUsedQuota(req, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
        }
    }

    return bfe_module.BfeHandlerGoOn
}
```

#### 5.1.2 `tokenRequestFinishHandler`

在请求完成阶段统一计算 RMB 成本。

```go
func (m *ModuleAITokenAuth) tokenRequestFinishHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
    if res == nil || res.StatusCode != bfe_http.StatusOK {
        return bfe_module.BfeHandlerGoOn
    }

    ctx := GetTokenAuthContext(req)
    if ctx == nil {
        return bfe_module.BfeHandlerGoOn
    }

    tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
    if tokenUsage.UsedQuota <= 0 && ctx.aiBasicInfo.IsAllowEstimateToken() {
        tokenUsage.UsedQuota = CalcReqUsedQuota(req, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
    }

    // 统一在请求完成阶段计算 RMB 成本
    if tokenUsage.UsedCost <= 0 && hasRMBPlan(ctx.Token.QuotaPlans) {
        tokenUsage.UsedCost = m.calcCostUnits(req, ctx.serverConf, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
    }

    costUnits := tokenUsage.UsedCost

    if tokenUsage.UsedQuota > 0 || costUnits > 0 {
        for _, plan := range ctx.Token.QuotaPlans {
            if plan.Unlimited {
                continue
            }
            if quota.IsRMB(plan.Unit) {
                if costUnits > 0 {
                    _, err := plan.Deduct(m.redisClient, costUnits)
                    if err != nil {
                        log.Logger.Warn("deduct rmb quota failed: %v", err)
                    }
                }
            } else {
                if tokenUsage.UsedQuota > 0 {
                    _, err := plan.Deduct(m.redisClient, tokenUsage.UsedQuota)
                    if err != nil {
                        log.Logger.Warn("deduct token quota failed: %v", err)
                    }
                }
            }
        }
    }

    return bfe_module.BfeHandlerGoOn
}
```

### 5.2 为什么 `ctx.serverConf` 在 finish 阶段可用

`SetTokenAuthContext` 中已经缓存了 `req.SvrDataConf`：

```go
func SetTokenAuthContext(req *bfe_basic.Request, tok *Token, promptToken int64, tags []bfe_basic.ApikeyTag) {
    ...
    tokenCtx := &TokenAuthContext{
        Token:       tok,
        aiBasicInfo: aiBasicInfo,
        serverConf:  req.SvrDataConf,
    }
    req.SetContext(REQ_TOKEN_AUTH_CONTEXT, tokenCtx)
}
```

`TokenAuthContext.serverConf` 的注释也明确说明：

```go
// serverConf caches the SvrDataConf before it is cleared by the reverse proxy.
// It is used for RMB cost calculation at request finish time.
```

因此将 `calcCostUnits` 移到 finish 阶段在设计上完全可行。

### 5.3 边界场景

| 场景 | 处理结果 |
|------|---------|
| 非流式，有 `usage` | `tokenReadResponseHandler` 解析 usage；finish 阶段用已有 Prompt/Completion 计算成本并扣费。 |
| 非流式，无 `usage`，开启估算 | `tokenReadResponseHandler` 用 `ContentLength/4` 估算；finish 阶段计算成本。 |
| 流式，有最终 `usage` chunk | `mod_body_process` 解析并填充 Prompt/Completion；finish 阶段计算成本。 |
| 流式，无 `usage`，开启估算 | `mod_body_process` 累加估算 completion tokens；finish 阶段计算成本。 |
| 流式/非流式，无 RMB plan | `hasRMBPlan` 为 false，不调用 `calcCostUnits`，行为不变。 |
| 无 `serverConf` | `calcCostUnits` 内部返回 0，不会误扣费。 |

---

## 6. 附加脆弱点（建议同步关注，但不纳入本次最小修复）

### 6.1 `LookupModelPrice` 硬编码 `"chat"`

`mod_ai_token_auth.go:434`：

```go
entry := cluster_conf.LookupModelPrice(cluster.AIConf.ModelTable, targetModel, "chat")
```

如果模型定价表中的 `Mode` 不是 `chat`，或者请求实际不是 chat completion，价格会找不到，`UsedCost` 为 0，且无日志提示。建议后续根据实际请求类型或模型表中的 `Mode` 匹配。

### 6.2 依赖响应体 `usage` 字段

当 `EstimateToken = false` 且上游未返回 `usage` 时，Token 和 RMB 扣费都会是 0。这是当前设计行为，但在生产环境中容易静默漏扣。

### 6.3 模块加载顺序

BFE handler 链按 `AddFilter` 注册顺序执行。若某部署把 `mod_body_process` 放在 `mod_ai_token_auth` 之前，`tokenReadResponseHandler` 看到的 `ContentLength` 已经是 `-1`，连非流式 RMB 扣费也会失败。将 `calcCostUnits` 移到 finish 阶段后，可在一定程度上降低对模块顺序的依赖。

---

## 7. 测试计划

### 7.1 单元测试

新增/修改 `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go`：

1. **`TestTokenRequestFinishHandler_RMB_Streaming`**：
   - 构造 `TokenAuthContext`，`PromptTokens` 和 `CompletionTokens` 已填充；
   - 使用 mock Redis 客户端；
   - 验证 RMB plan 被正确扣减。

2. **`TestTokenRequestFinishHandler_RMB_NonStreaming`**：
   - 模拟非流式响应，`ContentLength >= 0`；
   - 验证 finish 阶段仍然正确扣减。

3. **`TestTokenReadResponseHandler_NoCostCalculation`**：
   - 验证 `tokenReadResponseHandler` 执行后 `UsedCost` 仍为 0（不再提前计算）。

### 7.2 集成测试

扩展现有 SC03 RMB 配额集成测试（`bfe/tests/integration/implementation/scenario-SC03-rmb-quota/`）：

1. 在 `testdata/bfe.conf` 中加载 `mod_body_process`，并新增 `testdata/mod_body_process/` 配置；
2. 扩展 `tests/integration/common/mock_backend.go`，支持通过 `ResponseHeaders` 设置响应头（如 `Content-Type: text/event-stream`）；
3. 新增 `TestTC07_RMBQuotaDeduction_Streaming`：
   - 模拟 `stream: true` 请求；
   - 后端返回 SSE 流，最后一个 chunk 包含 `usage`；
   - 验证请求成功后 Redis 中 RMB 配额被正确扣减（`100*input_cost + 50*output_cost`）。

### 7.3 回归测试

- 跑 `bfe/bfe_modules/mod_ai_token_auth/...` 单元测试；
- 跑 `bfe/bfe_modules/mod_body_process/...` 单元测试；
- 跑 `bfe/bfe_config/bfe_cluster_conf/cluster_conf/...` 相关测试。

---

## 8. 影响范围

| 模块/文件 | 影响 |
|-----------|------|
| `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | 核心修复 |
| `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` | 新增单元测试 |
| `bfe/tests/integration/common/mock_backend.go` | 新增 `ResponseHeaders`，支持 SSE 测试 |
| `bfe/tests/integration/implementation/scenario-SC03-rmb-quota/...` | 新增流式 RMB 扣费集成测试，启用 `mod_body_process` |
| `bfe/docs/zh_cn/modifications/...` | 新增本文档 |

---

## 9. 兼容性

- 非流式请求的 RMB 扣费行为保持一致，仅计算位置后移。
- Token 配额扣费逻辑不变。
- 不修改配置格式，不修改 Redis key 结构。
- 对未启用 RMB plan 的产品无影响。

---

## 10. 参考资料

- GitHub issue：https://github.com/bfenetworks/bfe/issues/1316
- `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`
- `bfe/bfe_modules/mod_body_process/body_process.go`
- `bfe/bfe_modules/mod_body_process/content_quota_usage.go`
- `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`
