# BFE AI 路由 fallback 状态码触发策略扩展设计变更

## 1. 背景

当前 BFE AI 网关的 cluster 级 fallback 逻辑固定为：

- 连接/传输错误触发 fallback；
- 后端返回 `5xx` 触发 fallback；
- 后端返回 `4xx`（包括 401/402/403/422/429 等）**不触发** fallback，直接把最后一个 4xx 响应返回给客户端。

相关代码位于 `bfe/bfe_server/reverseproxy.go`：

```go
// line 1289
if invokeErr == nil && res != nil && res.StatusCode < 500 {
    // success or 4xx (client error, do not fallback)
    break
}

// line 1664
func shouldTriggerFallback(res *bfe_http.Response, err error) bool {
    if err != nil {
        return true
    }
    if res != nil && res.StatusCode >= 500 {
        return true
    }
    return false
}
```

这与 DeepSeek 等上游的实际情况不符：某些 4xx（如 401 key 失效、429 限流、402 欠费、422 请求参数被上游拒绝、400 上游内部错误包装）本质上是**上游不可用或配额类错误**，应当允许降级到 fallback cluster。GitHub issue #1317 要求 DeepSeek 的 401/400/402/422/429/500/503 都能触发 fallback。

参考 Bifrost 的做法，它并不依赖用户逐条路由配置状态码，而是在代码中维护一个统一的状态码分类集合：

- `transientServerStatusCodes`：500/502/503/504 等 transient 服务端错误；
- `perKeyFailureStatusCodes`：401/402/403/429 等单 key 失败；
- 其余情况下只要没有显式禁用 fallback，就会继续尝试 fallback。

本设计借鉴该思路，**在 BFE 代码中定义默认 fallback 触发状态码集合**，避免增加用户配置成本。新行为默认生效，无需修改 `ai_route.data`。

---

## 2. 目标

1. 在 BFE 代码中定义统一的 AI fallback 状态码分类集合。
2. 外层 `ServeHTTPForAI` 不再对 `res.StatusCode < 500` 一刀切：只有 `< 400` 才视为成功；`>= 400` 按状态码分类决定是否 fallback。
3. key 级重试逻辑（`aiClusterInvoke`）保持现状：401/402/403 标记 key 死亡、429 轮换 key、5xx/错误重试；key 耗尽后把最终响应交给外层决策。
4. 默认覆盖 issue #1317 要求的状态码：400/401/402/422/429 与全部 5xx（含 500/503）。
5. 更新单元测试、集成测试与系统设计文档，使其与新行为一致。

---

## 3. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 常量定义 | 新增 `aiFallbackStatusCodes` 常量集合 | `bfe/bfe_server/reverseproxy.go` |
| 转发层 | `ServeHTTPForAI` 成功判定从 `< 500` 改为 `< 400` | `bfe/bfe_server/reverseproxy.go` |
| 转发层 | `shouldTriggerFallback` 按状态码集合判定 | `bfe/bfe_server/reverseproxy.go` |
| 测试 | 更新现有单元测试、集成测试用例 | `bfe/bfe_server/reverseproxy_ai_test.go`、`bfe/tests/integration/...` |
| 文档 | 更新 `multi_api_key.md`、`mod_ai_route_bfe_changes.md` 与集成测试说明 | `bfe/docs/zh_cn/sys_design/...` |

---

## 4. 详细设计

### 4.1 状态码分类集合

在 `bfe/bfe_server/reverseproxy.go` 中新增常量：

```go
// aiFallbackStatusCodes 定义默认会触发 cluster 级 fallback 的 HTTP 状态码。
// 包含：
//   - 全部 5xx（服务端错误，如 500/502/503/504）
//   - 401/402/403（鉴权/授权/欠费失败）
//   - 429（限流）
//   - 400/422（上游对请求体/参数的拒绝，换 provider 可能成功）
//
// 不在集合中的 4xx（如 404/405/413 等）视为客户端错误，不触发 fallback。
var aiFallbackStatusCodes = map[int]struct{}{
    400: {},
    401: {},
    402: {},
    403: {},
    422: {},
    429: {},
    // 5xx 由 >= 500 统一处理，不需要逐个列出
}
```

设计说明：

- 使用 `map[int]struct{}` 便于 O(1) 查找。
- 5xx 不全部列出，由 `code >= 500` 统一覆盖，避免遗漏 501/502/504 等。
- 该集合是**代码常量**，不需要在 `ai_route.data` 中配置，降低用户成本。
- 如果未来需要允许自定义，可以在此基础上扩展为全局配置或环境变量，但本次设计保持零配置。

### 4.2 改造外层 fallback 决策

#### 4.2.1 成功判定条件

当前 `ServeHTTPForAI` 在 `aiClusterInvoke` 返回后立即判定：

```go
if invokeErr == nil && res != nil && res.StatusCode < 500 {
    break
}
```

这会把 4xx 全部当作成功，必须改为只有 `< 400` 才视为成功：

```go
if invokeErr == nil && res != nil && res.StatusCode < 400 {
    break
}
```

#### 4.2.2 `shouldTriggerFallback` 改造

```go
func shouldTriggerFallback(res *bfe_http.Response, err error) bool {
    if err != nil {
        return true
    }
    code := getResponseStatus(res)

    // 5xx 统一触发 fallback
    if code >= 500 {
        return true
    }

    // 特定 4xx 触发 fallback
    if _, ok := aiFallbackStatusCodes[code]; ok {
        return true
    }

    return false
}
```

#### 4.2.3 外层循环伪代码

```go
for i, attempt := range attempts {
    if i > 0 {
        if !p.resetRequestForRetry(basicReq) {
            log.Logger.Warn("ServeHTTPForAI: fallback aborted, request body cannot be rewound")
            break
        }
    }

    res, action, lastCluster, invokeErr = p.aiClusterInvoke(srv, serverConf, basicReq, rw, attempt, aiMeta)

    // 2xx/3xx 直接返回，不再 fallback
    if invokeErr == nil && res != nil && res.StatusCode < 400 {
        break
    }

    if i == len(attempts)-1 {
        break
    }

    if !shouldTriggerFallback(res, invokeErr) {
        break
    }

    log.Logger.Info("ServeHTTPForAI: fallback triggered, cluster[%s] err[%v] status[%d]",
        attempt.ClusterName, invokeErr, getResponseStatus(res))

    if res != nil {
        res.Body.Close()
    }
}
```

### 4.3 key 级重试逻辑保持现状

`aiClusterInvoke` 内部逻辑不需要修改：

- 401/402/403 → 标记 key 死亡，换 key；
- 429 → 标记 key 已用，换 key；
- 5xx/错误 → 同 key 退避重试；
- 400/422/404 等 → 直接返回当前响应，不再浪费 key 预算。

当 key 耗尽或不再重试时，`aiClusterInvoke` 把最终响应（可能为 4xx）返回给外层。外层 `shouldTriggerFallback` 再决定是否继续 cluster 级 fallback。

### 4.4 不触发 fallback 的 4xx

以下状态码默认**不触发** fallback，直接返回最后一个响应：

- 404 Not Found
- 405 Method Not Allowed
- 406 Not Acceptable
- 407 Proxy Authentication Required
- 408 Request Timeout（可讨论，但当前不纳入）
- 409 Conflict
- 410 Gone
- 411~417 等请求级错误
- 413 Payload Too Large（BFE 层应已拦截）

这些错误通常不会通过换 provider 解决，避免无效降级。

### 4.5 请求体重置与模型覆盖

每次触发 fallback 前会调用 `resetRequestForRetry`：

- 减少后端连接计数；
- 重置 `OutRequest`；
- rewind 请求体；
- 重置 `Content-Length`；
- 清除 `ErrCode` / `ErrMsg`。

由于 `doSingleAIForward` 会对请求体做 `model` 覆盖与 `ModelMapping` 改写，`resetRequestForRetry` 的 rewind 保证下一次 attempt 从原始请求体重新开始。该机制已在 5xx fallback 中验证，新增 4xx fallback 路径复用同一逻辑。

### 4.6 响应体关闭

触发 fallback 时，必须关闭上一个 4xx 响应体，避免连接泄漏：

```go
if res != nil {
    res.Body.Close()
}
```

该逻辑已存在，本次无需改动。

---

## 5. 关键代码变更示例

### 5.1 `bfe/bfe_server/reverseproxy.go`

#### 新增常量

```go
// aiFallbackStatusCodes 定义默认触发 cluster 级 fallback 的 4xx 状态码集合。
var aiFallbackStatusCodes = map[int]struct{}{
    400: {},
    401: {},
    402: {},
    403: {},
    422: {},
    429: {},
}
```

#### 外层循环

```go
res, action, lastCluster, invokeErr = p.aiClusterInvoke(srv, serverConf, basicReq, rw, attempt, aiMeta)
if invokeErr == nil && res != nil && res.StatusCode < 400 {
    break
}

if i == len(attempts)-1 {
    break
}
if !shouldTriggerFallback(res, invokeErr) {
    break
}

log.Logger.Info("ServeHTTPForAI: fallback triggered, cluster[%s] err[%v] status[%d]",
    attempt.ClusterName, invokeErr, getResponseStatus(res))

if res != nil {
    res.Body.Close()
}
```

#### `shouldTriggerFallback`

```go
func shouldTriggerFallback(res *bfe_http.Response, err error) bool {
    if err != nil {
        return true
    }
    code := getResponseStatus(res)
    if code >= 500 {
        return true
    }
    if _, ok := aiFallbackStatusCodes[code]; ok {
        return true
    }
    return false
}
```

---

## 6. 测试计划

### 6.1 单元测试

更新 `bfe/bfe_server/reverseproxy_ai_test.go` 中的 `TestShouldTriggerFallback`：

| 输入状态码 | 预期 |
|---|---|
| 200 | false |
| 404 | false |
| 400 | true |
| 401 | true |
| 402 | true |
| 403 | true |
| 422 | true |
| 429 | true |
| 500 | true |
| 503 | true |
| ConnectError | true |

新增一个辅助函数 `TestAiFallbackStatusCodesCoverage`，确保集合中包含/排除预期状态码。

### 6.2 集成测试更新

#### TC-05 调整：Key 耗尽后触发 fallback

原 TC-05 期望 401/403/429 key 耗尽后**不触发** cluster fallback。新行为下这些状态码会触发 fallback，因此需要调整预期：

- 响应状态码：200（来自 fallback cluster）。
- `cluster_multi_key` 后端收到 3~4 次请求（尝试所有 key）。
- `cluster_fallback_ok` 后端被命中 1 次。
- BFE 日志中出现 `fallback triggered` 相关记录。

如需保留“某些 4xx 不触发 fallback”的用例，可新增 TC-05-B：使用 404 作为后端响应，验证 `cluster_fallback_ok` 未被命中。

#### TC-06 保持不变

5xx key 耗尽触发 cluster fallback，行为与现有逻辑一致。

#### 新增集成测试用例

| 用例编号 | 场景 | 预期 |
|---|---|---|
| SC02-TC-15 | primary cluster 单 key 返回 400 | key 不重试，直接触发 cluster fallback |
| SC02-TC-16 | primary cluster 单 key 返回 422 | key 不重试，直接触发 cluster fallback |
| SC02-TC-17 | primary cluster 返回 404 | 404 不在 fallback 集合，不触发 fallback，返回 404 |
| SC01-TC-12 | primary cluster 返回 429 | 触发 cluster fallback |

### 6.3 回归测试

- `go test ./bfe_server/...` 通过；
- `go test ./bfe_modules/mod_ai_route/...` 通过；
- 集成测试 SC01/SC02 全量通过。

---

## 7. 文档更新

需要同步更新以下文档，避免与代码行为不一致：

1. `bfe/docs/zh_cn/sys_design/mod_ai_route_bfe_changes.md`
   - 修改“fallback 只针对后端不可用场景，4xx/限流/鉴权失败不触发”。
   - 改为说明：默认情况下 5xx/网络错误以及 400/401/402/403/422/429 都会触发 fallback；404 等请求级 4xx 不触发。

2. `bfe/docs/zh_cn/sys_design/multi_api_key.md`
   - 修改 4.2 节“与 cluster 级 fallback 的边界”：
     - 原：401/403/429 不触发 cluster fallback；
     - 新：401/402/403/429 在 key 级重试耗尽后会触发 cluster fallback；400/422 也会触发；404 等不会。
   - 更新 `shouldTriggerFallback` 伪代码。

3. `bfe/tests/integration/测试设计文档/scenario-SC02-多API-Key轮换与重试/TC-05-Key耗尽后返回最后响应.md`
   - 按新行为更新预期，或拆分为 TC-05（401/403/429 触发 fallback）和 TC-05-B（404 不触发 fallback）。

4. `document-ai-gateway/迭代系统设计/v0.4/fallback支持/bifrost-fallback-analysis.md`
   - 可补充一条跟踪记录：BFE 已确定采用代码级状态码分类方案，无需 `ai_route.data` 配置。

---

## 8. 与 Bifrost 的对比

| 维度 | Bifrost | 本方案（BFE） |
|---|---|---|
| 状态码配置 | 无用户配置，代码内建集合 | 无用户配置，代码内建集合 |
| 5xx 处理 | transientServerStatusCodes：500/502/503/504 | 全部 5xx 触发 fallback |
| key 失败 | perKeyFailureStatusCodes：401/402/403/429 | 401/402/403/429 触发 fallback |
| 400/422 | 默认继续 fallback（未显式禁用时） | 显式加入集合，触发 fallback |
| 禁用 fallback | 通过 context flag 显式禁用 | 当前无显式禁用，未来可扩展 |
| 配置成本 | 低 | 低 |

本方案与 Bifrost 思路一致：通过代码中的状态码分类集合决定 fallback，避免用户在每条路由上维护状态码列表。

---

## 9. 风险与回滚

| 风险 | 缓解措施 |
|---|---|
| 默认行为改变，已有 4xx 用例返回不同 | 同步更新单元测试、集成测试与文档；404 等仍不触发 fallback，降低误触发 |
| 400/422 可能是真正的客户端错误 | 在 AI 网关场景下，不同 provider 对同一请求可能返回不同结果，fallback 有收益；若确认是恶意/错误请求，后续可扩展显式禁用 fallback 机制 |
| 4xx 响应体未关闭 | 沿用 `res.Body.Close()`，新增路径保证执行 |
| fallback 时请求体未正确 rewind | 复用现有 `resetRequestForRetry` 逻辑 |
| 无限循环 | fallback 有 attempts 数量上限，且不会把 2xx/3xx 纳入集合 |

**回滚**：该设计修改的是 BFE 二进制行为。若线上需要恢复旧行为，必须回滚代码并重新发版；但影响面可控（仅涉及 AI 路由且配置了 fallback cluster 的场景）。

---

## 10. 关键代码索引

| 文件 | 行号范围 | 说明 |
|---|---|---|
| `bfe/bfe_server/reverseproxy.go` | 1099-1103 | `aiForwardAttempt` 结构 |
| `bfe/bfe_server/reverseproxy.go` | 1279-1310 | 外层 fallback 循环 |
| `bfe/bfe_server/reverseproxy.go` | 1540-1661 | `aiClusterInvoke` key 级重试 |
| `bfe/bfe_server/reverseproxy.go` | 1664-1672 | `shouldTriggerFallback` |
| `bfe/bfe_server/reverseproxy_ai_test.go` | 73-92 | 现有 `TestShouldTriggerFallback` |
