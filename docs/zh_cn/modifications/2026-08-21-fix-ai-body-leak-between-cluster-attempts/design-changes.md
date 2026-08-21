# 修复 AI 请求体在 cluster 级 fallback 之间泄漏的问题

## 1. 背景

在 `bfe/bfe_server/reverseproxy.go` 的 `doSingleAIForward()` 中，会按 cluster 配置改写请求体 JSON 中的 `model` 字段：

1. `attempt.Model` 覆盖；
2. `cluster.AIConf.MatchPrefix` provider/model 前缀剥离；
3. `cluster.AIConf.ModelMapping` 模型映射。

这些改写通过 `condition.ReqBodyJsonSet()` 完成，其内部调用 `bfe_http.BodyAccessor.SetBytes()` 直接修改底层 `bytes_body.buf`。

`doSingleAIForward()` 首先执行：

```go
outreq := new(bfe_http.Request)
*outreq = *req // includes shallow copies of maps, but okay
basicReq.OutRequest = outreq
```

这里 `outreq.Body` 与 `req.Body`（即 `basicReq.HttpRequest.Body`）指向**同一个** `bytes_body` 对象。因此 `ReqBodyJsonSet` 修改 `outreq.Body` 时，实际上也修改了 `basicReq.HttpRequest.Body`。

当 `ServeHTTPForAI()` 触发 cluster 级 fallback 时，会调用 `resetRequestForRetry()`：

```go
func (p *ReverseProxy) resetRequestForRetry(basicReq *bfe_basic.Request) bool {
    ...
    if !rewindRequestBody(basicReq.HttpRequest) {
        return false
    }
    ...
}
```

而 `rewindRequestBody()` 只是将读取位置重置到 buffer 起点：

```go
func rewindRequestBody(req *bfe_http.Request) bool {
    ...
    rewindable, ok := req.Body.(bfe_http.Rewindable)
    ...
    return rewindable.Rewind()
}
```

`bytes_body.Rewind()` 实现为：

```go
func (b *bytes_body) Rewind() bool {
    ...
    b.SetBytes(b.buf, b.all)
    return true
}
```

它使用的是**已经被修改过的** `b.buf`，并不会恢复到客户端原始请求体。

### 1.1 漏洞场景

假设配置了两个 fallback cluster：

- **cluster A**：配置了 `ModelMapping`，将 `gpt-4` 映射为 `deepseek-chat`。
- **cluster B**：没有 `ModelMapping`，也不剥离前缀，期望直接透传客户端原始模型名 `gpt-4`。

请求流程：

1. 第一次尝试 cluster A，`ReqBodyJsonSet` 把 body 中的 `model` 改为 `deepseek-chat`。
2. cluster A 返回 503，触发 fallback。
3. `resetRequestForRetry` 调用 `Rewind()`，但 body 内容仍是 `deepseek-chat`。
4. 第二次尝试 cluster B 时，`doSingleAIForward` 从 `basicReq.HttpRequest` 复制 `outreq`，得到的是已被改为 `deepseek-chat` 的 body。
5. cluster B 实际收到 `model=deepseek-chat`，而非客户端原始的 `gpt-4`。

这是一个跨 cluster 的**请求体状态泄漏**漏洞。

### 1.2 另一个相关状态泄漏：`aiMeta.TargetModel`

除请求体外，`doSingleAIForward()` 原实现中计算 model 时还会读取 `aiMeta.TargetModel`：

```go
model := aiMeta.ClientModel
if aiMeta.TargetModel != "" {
    model = aiMeta.TargetModel
}
```

当 cluster A 改写 model 成功后，`aiMeta.TargetModel` 被更新。若 cluster B 没有自己的 `attempt.Model` / `ModelMapping` / `StripPrefix`，第二次进入 `doSingleAIForward()` 时会把 `aiMeta.TargetModel` 当作本次计算的起点，导致 cluster B 仍使用 cluster A 改写后的模型名，即使请求体本身已被隔离。

因此修复需要同时做到：

1. 隔离请求体；
2. 每次 cluster 尝试都从 `aiMeta.ClientModel` 重新计算 model，不再继承上一次的 `aiMeta.TargetModel`。

---

## 2. 目标

1. 保证每次 cluster 级转发尝试都基于**客户端原始请求体**开始计算和改写。
2. 单个 cluster 内的 model 改写只影响本次转发，不会泄漏到下一次 fallback/retry。
3. 尽量减小对现有代码结构的侵入，保持 key 级重试逻辑不变。
4. 同步更新相关单元测试和集成测试，确保行为正确。

---

## 3. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 转发层 | `doSingleAIForward()` 创建 `outreq` 后，复制独立 body | `bfe/bfe_server/reverseproxy.go` |
| 测试 | 新增/更新测试用例，验证跨 cluster body 隔离 | `bfe/bfe_server/reverseproxy_ai_test.go`、`bfe/tests/integration/...` |
| 文档 | 更新设计文档与修改方案文档 | `bfe/docs/zh_cn/sys_design/...`、`bfe/docs/zh_cn/modifications/...` |

---

## 4. 详细设计

### 4.1 根因

`bytes_body.Rewind()` 只重置读取位置，不恢复原始内容。这是该类型的预期行为（它设计用于多次读取同一份 body，而不是恢复到历史版本）。

因此，一旦 `ReqBodyJsonSet()` 修改了 `basicReq.HttpRequest.Body` 的内容，该修改会持续到请求结束，影响后续所有 cluster 尝试。

### 4.2 解决思路

#### 请求体隔离

只有在真正需要改写请求体时才复制 `outreq.Body`。如果当前 cluster 没有 model 改写规则，不复制 body，避免对大 body 场景造成不必要的开销，也避免引入 `Content-Length` 与 body 长度不一致的问题。

当需要改写时，让 `outreq.Body` 指向一个**独立的 body 副本**，而不是与 `basicReq.HttpRequest.Body` 共享同一个对象：

- `ReqBodyJsonSet()` 修改的是 `outreq.Body` 的副本；
- `basicReq.HttpRequest.Body` 保持原始内容不变；
- 下一次 cluster 尝试从 `basicReq.HttpRequest` 复制 `outreq` 时，得到的是原始 body；
- key 级重试（同一 cluster 内换 key）仍从 `basicReq.HttpRequest` 复制，也基于原始 body，符合预期——因为同一 cluster 的 model 改写规则相同，基于原始 body 重新计算即可。

如果 body 未能被完整缓冲（`all == false`，通常因为超过 `AccessibleBodySize`），则跳过复制并记录 warn。此时 fallback 本来就会被 BFE 禁用，因此不会触发跨 cluster 泄漏。

#### `aiMeta.TargetModel` 隔离

每次进入 `doSingleAIForward()` 时，model 计算都从 `aiMeta.ClientModel` 开始，不再读取 `aiMeta.TargetModel`。这样即使上一次尝试把 `TargetModel` 更新为改写后的值，也不会影响本次尝试。

请求成功时，`aiMeta.TargetModel` 会被更新为本次成功 cluster 实际发送的 model，供 access log、计费等后续逻辑使用。

### 4.3 代码实现

在 `doSingleAIForward()` 中，先计算最终 model，再决定是否需要复制并改写 body：

```go
// Calculate the final model in order: route target/fallback override ->
// provider/model prefix stripping -> cluster model mapping.
// Always start from ClientModel so that each cluster attempt is independent.
model := aiMeta.ClientModel

// apply model override from ai route target/fallback
if attempt.Model != "" {
    model = attempt.Model
}

// strip provider/model prefix according to cluster AIConf
if cluster.AIConf != nil && cluster.AIConf.StripPrefix && cluster.AIConf.MatchPrefix != "" {
    if stripped, ok := stripProviderPrefix(model, cluster.AIConf.MatchPrefix); ok {
        model = stripped
    }
}

// apply cluster model mapping
if cluster.AIConf != nil && cluster.AIConf.ModelMapping != nil && model != "" {
    if newModel, ok := (*cluster.AIConf.ModelMapping)[model]; ok {
        model = newModel
    }
}

if model != aiMeta.ClientModel {
    // Need to rewrite the body. Isolate outreq.Body from req.Body so that
    // the rewrite does not leak into the next fallback/retry attempt.
    if req.Body != nil {
        if bodyAccessor, err := req.GetBodyAccessor(); err != nil {
            log.Logger.Warn("doSingleAIForward: failed to get body accessor: %s", err)
        } else if bodyAccessor != nil {
            bodyBytes, all := bodyAccessor.GetBytes()
            if !all {
                log.Logger.Warn("doSingleAIForward: request body not fully buffered, model rewrite may leak between attempts")
            } else {
                newBody, err := bfe_http.NewBytesBody(io.NopCloser(bytes.NewReader(bodyBytes)), int64(len(bodyBytes)))
                if err != nil {
                    log.Logger.Warn("doSingleAIForward: failed to copy request body: %s", err)
                } else {
                    outreq.Body = newBody
                }
            }
        }
    }

    if err := condition.ReqBodyJsonSet(basicReq, "model", model); err != nil {
        log.Logger.Warn("Failed to set model in request body: %s", err)
    } else {
        // reset Content-Length for both outreq and original request
        ...
        aiMeta.TargetModel = model
    }
}
```

关键点：

- model 计算始终从 `aiMeta.ClientModel` 开始，避免 `aiMeta.TargetModel` 跨 cluster 泄漏。
- 只有确认需要改写 model 时，才复制独立 body 副本。
- 复制前检查 `bodyAccessor.GetBytes()` 返回的 `all` 标志；未完整缓冲时不复制，记录 warn。
- 复制失败时记录 warn 并继续，退化为修改原始 body 的行为（此时通常不会有下一次 cluster 尝试）。

### 4.4 与原优化方案的关系

本次修复与之前的“合并多次 `ReqBodyJsonSet` 调用”优化是正交的：

- 之前优化减少了单次 cluster 尝试内的 JSON 改写次数；
- 本次修复保证单次尝试内的改写不会泄漏到其他 cluster 尝试，并且每次尝试都从原始 model 重新计算。

两者结合后，每次 cluster 尝试都会：

1. 从 `ClientModel` 计算最终 model；
2. 如果需要改写，从原始 body 复制出独立的 `outreq.Body`；
3. 最多调用一次 `ReqBodyJsonSet` 修改副本；
4. 无论本次成功或失败，`basicReq.HttpRequest.Body` 都保持原始内容。

### 4.5 对 key 级重试的影响

`aiClusterInvoke()` 在同一 cluster 内多个 key 之间重试时，每次都会调用 `doSingleAIForward()`。由于每次都会从 `aiMeta.ClientModel` 重新计算 model，并从 `basicReq.HttpRequest.Body` 复制独立副本，因此同一 cluster 的多次 key 尝试也都基于原始 body 重新计算 model。

这与修复前行为一致（同一 cluster 的规则相同，重新计算结果相同），且避免了 key 级重试时 body 或 `TargetModel` 被意外保留上一次改写结果的问题。

---

## 5. 关键代码变更示例

### 5.1 `bfe/bfe_server/reverseproxy.go`

#### import 增加

```go
import (
    "bytes"
    "crypto/tls"
    "errors"
    "io"
    ...
)
```

#### `doSingleAIForward()` 中 model 计算与 body 隔离

```go
// Calculate the final model in order: route target/fallback override ->
// provider/model prefix stripping -> cluster model mapping.
// Always start from ClientModel so that each cluster attempt is independent.
model := aiMeta.ClientModel

// apply model override from ai route target/fallback
if attempt.Model != "" {
    model = attempt.Model
}

// strip provider/model prefix according to cluster AIConf
if cluster.AIConf != nil && cluster.AIConf.StripPrefix && cluster.AIConf.MatchPrefix != "" {
    if stripped, ok := stripProviderPrefix(model, cluster.AIConf.MatchPrefix); ok {
        model = stripped
    }
}

// apply cluster model mapping
if cluster.AIConf != nil && cluster.AIConf.ModelMapping != nil && model != "" {
    if newModel, ok := (*cluster.AIConf.ModelMapping)[model]; ok {
        model = newModel
    }
}

if model != aiMeta.ClientModel {
    // Need to rewrite the body. Isolate outreq.Body from req.Body so that
    // the rewrite does not leak into the next fallback/retry attempt.
    if req.Body != nil {
        if bodyAccessor, err := req.GetBodyAccessor(); err != nil {
            log.Logger.Warn("doSingleAIForward: failed to get body accessor: %s", err)
        } else if bodyAccessor != nil {
            bodyBytes, all := bodyAccessor.GetBytes()
            if !all {
                log.Logger.Warn("doSingleAIForward: request body not fully buffered, model rewrite may leak between attempts")
            } else {
                newBody, err := bfe_http.NewBytesBody(io.NopCloser(bytes.NewReader(bodyBytes)), int64(len(bodyBytes)))
                if err != nil {
                    log.Logger.Warn("doSingleAIForward: failed to copy request body: %s", err)
                } else {
                    outreq.Body = newBody
                }
            }
        }
    }

    if err := condition.ReqBodyJsonSet(basicReq, "model", model); err != nil {
        log.Logger.Warn("Failed to set model in request body: %s", err)
    } else {
        // outreq body already changed, need reset Content-Length
        if outreq.ContentLength >= 0 {
            outreq.ContentLength = -1
            outreq.Header.Del("Content-Length")
        }
        // Also reset the original request's Content-Length so that fallback/retry
        // creates a new outreq with consistent body length.
        if basicReq.HttpRequest != nil && basicReq.HttpRequest.ContentLength >= 0 {
            basicReq.HttpRequest.ContentLength = -1
            basicReq.HttpRequest.Header.Del("Content-Length")
        }
        aiMeta.TargetModel = model
    }
}
```

---

## 6. 测试计划

### 6.1 单元测试

`doSingleAIForward()` 涉及 `ReverseProxy`、`BfeServer`、`BfeCluster` 等对象的完整转发链路，单元测试构造成本较高。核心隔离逻辑已在代码中直接体现，建议在集成测试层面覆盖跨 cluster body 隔离场景。

### 6.2 集成测试

已在 `bfe/tests/integration/implementation/scenario-SC04-provider-model-prefix-strip/sc04_provider_model_prefix_strip_test.go` 中新增 `TestTC07_FallbackNoPrefixStripGetsOriginalModel`：

- 构造请求 `{"model":"openrouter/anthropic/claude-sonnet-4.6", ...}`；
- `cluster_openrouter` 启用 `StripPrefix=true`，并返回 500；
- `cluster_fallback` 不配置 `AIConf`（即不剥离前缀），返回 200；
- 验证 `cluster_fallback` 收到原始 model `openrouter/anthropic/claude-sonnet-4.6`，而非剥离后的 `anthropic/claude-sonnet-4.6`。

该用例直接覆盖“前一个 cluster 改写了 body/model，fallback 到下一个无改写规则的 cluster 时，下一个 cluster 仍应收到客户端原始内容”的漏洞。

### 6.3 回归测试

- `go test ./bfe_server/...` 通过；
- `go test ./bfe_modules/mod_ai_route/...` 通过；
- `go test ./tests/integration/...` 通过。

---

## 7. 文档更新

已同步更新以下文档：

1. **`bfe/docs/zh_cn/modifications/2026-08-21-fix-ai-body-leak-between-cluster-attempts/design-changes.md`**
   - 本设计变更文档。

2. **`bfe/docs/zh_cn/sys_design/multi_api_key.md`**
   - 在 `doSingleAIForward()` 伪代码中增加 body 复制隔离逻辑说明。

3. **`bfe/docs/zh_cn/sys_design/mod_ai_route_bfe_changes.md`**
   - 在 7.2 节请求体处理中补充说明：每次 cluster 尝试前复制独立 body，避免改写泄漏。

4. **`bfe/docs/zh_cn/sys_design/provider_model_prefix_routing.md`**
   - 在 4.1 节中补充 body 隔离说明。

---

## 8. 与现有机制的对比

| 维度 | 修复前 | 修复后 |
|---|---|---|
| `outreq.Body` 与 `req.Body` 关系 | 共享同一个 `bytes_body` | 独立的 `bytes_body` 副本 |
| `ReqBodyJsonSet` 影响范围 | 影响当前及后续所有 cluster/key 尝试 | 仅影响当前 cluster 尝试 |
| fallback 后 body 内容 | 保留上一次改写结果 | 恢复为客户端原始内容 |
| 内存开销 | 无额外复制 | 每个 cluster 尝试复制一次 body |
| 正确性 | 跨 cluster 可能发送错误 model | 每个 cluster 都基于原始 model 重新计算 |

---

## 9. 风险与回滚

| 风险 | 缓解措施 |
|---|---|
| 复制大 body 增加内存占用 | 这是为保证正确性必须付出的代价；`bytes_body` 已有全局 buffer 上限控制 |
| 复制失败导致请求继续但行为退化 | 记录 warn 日志，且仅退化为旧行为，不会直接失败 |
| 集成测试需要新增用例 | 补充 SC01/SC04 跨 cluster body 隔离用例 |
| 影响非 AI 请求路径 | 修改仅在 `doSingleAIForward()` 中，不影响普通 `ServeHTTP()` |

**回滚**：若出现问题，可回滚 `bfe/bfe_server/reverseproxy.go` 的修改。该修复仅影响 AI 路由且配置了 cluster 级 fallback 的场景。

---

## 10. 关键代码索引

| 文件 | 行号范围 | 说明 |
|---|---|---|
| `bfe/bfe_server/reverseproxy.go` | 1452-1467 | `doSingleAIForward()` 创建 `outreq` |
| `bfe/bfe_server/reverseproxy.go` | 1694-1722 | `resetRequestForRetry()` |
| `bfe/bfe_http/transfer.go` | 819-927 | `bytes_body` 定义、`SetBytes()`、`Rewind()` |
| `bfe/bfe_basic/condition/primitive.go` | 1176-1200 | `ReqBodyJsonSet()` 实现 |
