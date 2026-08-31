# 修复 AI Token 认证计费三处问题（issues #1343 #1344 #1345）

## 1. 背景

线上对账发现 `mod_ai_token_auth` 模块在 Anthropic / OpenAI 协议下存在三处计费异常，分别导致 **少收**、**多收** 和 **重复扣费**：

- [issue #1343](https://github.com/bfenetworks/bfe/issues/1343)：cache 读取 token 数被错误截断到 `promptTokens`，高 cache 命中场景下严重少收。
- [issue #1344](https://github.com/bfenetworks/bfe/issues/1344)：Anthropic `/count_tokens` 端点被当作普通请求扣费，产生不应有的多收。
- [issue #1345](https://github.com/bfenetworks/bfe/issues/1345)：`HandleRequestFinish` 回调被触发多次，同一笔费用重复扣款。

本方案对三处问题统一分析并给出最小改动修复。

---

## 2. 根因分析

### 2.1 Issue #1343：cache 读取 token 截断错误

**涉及文件**：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`  
**涉及函数**：`calcChatCost()`（第 531 行起）

当前代码在第 550-552 行对 `cacheReadTokens` 做截断：

```go
if cacheReadTokens > promptTokens {
    cacheReadTokens = promptTokens
}
```

对于 Anthropic 协议，`usage.PromptTokens` 来自 SSE 事件中的 `input_tokens`，该字段 **仅包含 cache miss 的 token 数**，而不是总 prompt token 数；`usage.CacheReadTokens` 来自 `cache_read_input_tokens`，是实际 cache 命中数。当 cache 命中率很高时，`cacheReadTokens >> promptTokens`，截断后大量 cache token 被丢弃，导致 cache 部分几乎不计费。

OpenAI / DeepSeek 协议虽然没有这个问题（`prompt_tokens` 已含 cache），但截断逻辑对它们同样不必要：后续 `normalInput = promptTokens - cacheReadTokens` 已经通过 `if normalInput < 0 { normalInput = 0 }` 做了非负保护。

### 2.2 Issue #1344：count_tokens 端点不应扣费

**涉及文件**：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`  
**涉及函数**：`tokenRequestFinishHandler()`（第 206 行起）

Anthropic 的 `POST /anthropic/v1/messages/count_tokens` 仅用于计算请求 token 数，不应产生任何费用。但 `tokenRequestFinishHandler` 对所有 200 OK 响应都会执行扣费逻辑，未按 endpoint 路径做豁免。

由于 `count_tokens` 是非流式请求（`ContentLength >= 0`），`tokenReadResponseHandler` 会调用 `UpdateCtxByUsage(body)`，但 `count_tokens` 的响应体格式不匹配任何解析模式，`tokenUsage` 未被正确更新。随后 `tokenRequestFinishHandler` 使用 `GetPromptToken` 估算的 prompt tokens 计算费用，造成按请求体字节数计费的多收。

### 2.3 Issue #1345：HandleRequestFinish 重复触发导致重复扣费

**涉及文件**：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`  
**涉及函数**：`tokenRequestFinishHandler()`（第 206 行起）

当前逻辑：

```go
if tokenUsage.UsedCost <= 0 && hasRMBPlan(ctx.Token.QuotaPlans) {
    tokenUsage.UsedCost = m.calcCostUnits(req, ctx.serverConf, tokenUsage)
}

costUnits := tokenUsage.UsedCost

// 扣费循环
for _, plan := range ctx.Token.QuotaPlans {
    ...
    plan.Deduct(m.redisClient, costUnits)
}
```

`UsedCost <= 0` 的守卫只能防止 **重复计算** `UsedCost`，但无法防止 **重复扣费**。当 `HandleRequestFinish` 被多次触发时：

1. 第一次调用设置 `UsedCost = X`，并执行扣费循环，Redis 余额减少。
2. 第二次调用发现 `UsedCost = X > 0`，跳过计算，但 `costUnits = X`，扣费循环再次执行。
3. 由于 Redis 余额已变化，`deductRMB` 的 Lua 脚本 `math.min(current, amount)` 会产生不同扣费额，形成随机的小额额外扣费。

---

## 3. 修复方案

### 3.1 修复 Issue #1343：移除不必要的 cache 截断

**改动位置**：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:550-552`

删除以下代码：

```go
if cacheReadTokens > promptTokens {
    cacheReadTokens = promptTokens
}
```

保留 `normalInput` 的非负保护（第 581-583 行）：

```go
normalInput = promptTokens - cacheReadTokens
if normalInput < 0 {
    normalInput = 0
}
```

**理由**：
- Anthropic 协议下 `promptTokens` 不是总 prompt，截断逻辑数学上错误。
- OpenAI / DeepSeek 协议下 `promptTokens` 已含 cache，截断会低估 cache 用量。
- 删除截断后，`normalInput` 的非负保护已足够防止负计费。

### 3.2 修复 Issue #1344：跳过 count_tokens 端点扣费

**改动位置**：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:206-210`

在 `tokenRequestFinishHandler` 开头增加 endpoint 白名单检查：

```go
func (m *ModuleAITokenAuth) tokenRequestFinishHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
    // 跳过非计费端点：Anthropic count_tokens 仅用于 token 计数，不应扣费
    if strings.Contains(req.HttpRequest.RequestURI, "/count_tokens") {
        return bfe_module.BfeHandlerGoOn
    }

    if res == nil || res.StatusCode != bfe_http.StatusOK {
        return bfe_module.BfeHandlerGoOn
    }
    // ... 原有逻辑
}
```

**依赖**：需要在文件头部引入 `strings` 包。

### 3.3 修复 Issue #1345：增加已扣费标记

**改动位置**：
- `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:388-394`（`TokenAuthContext` 结构体）
- `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:206-253`（`tokenRequestFinishHandler`）

在 `TokenAuthContext` 中增加 `deducted` 标记：

```go
type TokenAuthContext struct {
    Token       *Token
    aiBasicInfo *bfe_basic.AiBasicInfo
    serverConf  bfe_basic.ServerDataConfInterface
    deducted    bool // 标记本请求是否已执行过扣费
}
```

在 `tokenRequestFinishHandler` 开头和扣费循环后使用该标记：

```go
func (m *ModuleAITokenAuth) tokenRequestFinishHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
    // ... 跳过 count_tokens ...

    if res == nil || res.StatusCode != bfe_http.StatusOK {
        return bfe_module.BfeHandlerGoOn
    }

    ctx := GetTokenAuthContext(req)
    if ctx == nil {
        return bfe_module.BfeHandlerGoOn
    }

    // 已扣费则跳过，防止 HandleRequestFinish 多次触发导致重复扣费
    if ctx.deducted {
        return bfe_module.BfeHandlerGoOn
    }

    // ... 原有 UsedQuota / UsedCost 计算逻辑 ...

    if tokenUsage.UsedQuota > 0 || costUnits > 0 {
        for _, plan := range ctx.Token.QuotaPlans {
            // ... 原有扣费逻辑 ...
        }
    }

    ctx.deducted = true
    return bfe_module.BfeHandlerGoOn
}
```

**理由**：在请求上下文级别做幂等标记，实现简单，不引入分布式锁开销，且与现有 `UsedCost` 守卫互补。

---

## 4. 代码变更汇总

| 改动点 | 文件 | 行号 | 说明 |
|--------|------|------|------|
| 删除 cache 截断 | `mod_ai_token_auth.go` | 550-552 | 避免 Anthropic 高 cache 命中场景少收 |
| 增加 `strings` 导入 | `mod_ai_token_auth.go` | import 区 | 用于 `count_tokens` 路径判断 |
| 跳过 `count_tokens` | `mod_ai_token_auth.go` | 206-210 | 避免 Anthropic token 计数端点多收 |
| 增加 `deducted` 字段 | `mod_ai_token_auth.go` | 388-394 | 请求级扣费幂等标记 |
| 增加扣费守卫 | `mod_ai_token_auth.go` | 212-216 / 251 后 | 防止重复扣费 |

---

## 5. 测试建议

1. **单元测试**
   - 在 `mod_ai_token_auth_test.go` 中新增 `calcChatCost` 测试用例：
     - Anthropic 场景：`CacheReadTokens = 1000`，`PromptTokens = 50`，验证 cache 按 1000 计费、normalInput 为 0。
     - OpenAI 场景：`CacheReadTokens = 200`，`PromptTokens = 1000`，验证正常拆分计费。
   - 新增 `tokenRequestFinishHandler` 测试：
     - 模拟 `RequestURI` 包含 `/count_tokens`，验证不扣费。
     - 模拟 `HandleRequestFinish` 被调用两次，验证第二次不执行 `Deduct`。

2. **集成测试**
   - 使用 Anthropic 真实/模拟响应构造高 cache 命中请求，核对计费金额与上游账单一致。
   - 调用 `count_tokens` 端点，确认 Redis 余额不变。

3. **回归测试**
   - 运行 `make test`，确保现有 streaming / non-streaming 扣费逻辑不受影响。

---

## 6. 兼容性说明

- 删除 cache 截断后，Anthropic 高 cache 命中场景的计费会 **上升**，这是修正错误少收后的正确行为，需要在运营侧提前告知用户。
- `count_tokens` 端点此前多收的费用，需在计费对账层单独处理。
- `deducted` 标记仅影响同一请求生命周期内的重复扣费，不跨请求生效，不影响正常流量。
