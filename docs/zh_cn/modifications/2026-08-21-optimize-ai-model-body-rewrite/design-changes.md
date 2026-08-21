# 优化 BFE AI 请求 model 字段多次重写

## 1. 背景

在 `bfe/bfe_server/reverseproxy.go` 的 `doSingleAIForward` 函数中，针对单个 AI 转发尝试，会按顺序对请求体 JSON 中的 `model` 字段进行最多三次改写：

1. **attempt.Model 覆盖**（line 1491-1502）：将请求体 `model` 设为路由目标/降级指定的模型。
2. **provider/model 前缀剥离**（line 1504-1507）：调用 `stripProviderPrefix`，按 `cluster.AIConf.MatchPrefix` 剥离前缀后再次改写 `model`。
3. **ModelMapping 映射**（line 1521-1540）：按集群配置的模型映射表再次改写 `model`。

每次改写都通过 `condition.ReqBodyJsonSet` 完成，其内部使用 `sjson.SetBytes` 对请求体做完整的 JSON 解析与序列化：

```go
// bfe/bfe_basic/condition/primitive.go
func ReqBodyJsonSet(req *bfe_basic.Request, path string, value string) error {
    ...
    body, _ := bodyAccessor.GetBytes()
    newBody, err = sjson.SetBytes(body, path, value)
    ...
    bodyAccessor.SetBytes(newBody, false)
    return nil
}
```

三次调用之间存在明显冗余：

- 同一条请求体被重复解析/序列化最多 3 次。
- 每次成功后都重复执行 `outreq.ContentLength` 重置逻辑。
- `stripProviderPrefix` 内部还会额外重置 `basicReq.HttpRequest.ContentLength`。

在 AI 网关场景下，请求体通常较大（尤其是携带长 prompt 或多轮对话时），多次 JSON 改写会带来不必要的 CPU 与内存开销。

---

## 2. 目标

1. 将 `doSingleAIForward` 中针对 `model` 字段的多次 `ReqBodyJsonSet` 调用合并为**最多一次**。
2. 将 `ContentLength` 重置逻辑也收敛到统一位置，避免重复代码。
3. 保持现有业务行为不变（模型覆盖 → 前缀剥离 → 模型映射的执行顺序与语义）。
4. 同步更新 `stripProviderPrefix` 单元测试，使其继续覆盖变换逻辑。

---

## 3. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 转发层 | 合并 `model` 字段改写逻辑，只调用一次 `ReqBodyJsonSet` | `bfe/bfe_server/reverseproxy.go` |
| 工具函数 | `stripProviderPrefix` 改为纯字符串计算函数（不再操作 body / ContentLength） | `bfe/bfe_server/reverseproxy.go` |
| 测试 | 更新 `TestStripProviderPrefix` 系列用例，验证新的纯计算函数 | `bfe/bfe_server/reverseproxy_ai_test.go` |

---

## 4. 详细设计

### 4.1 当前逻辑梳理

当前 `doSingleAIForward` 中的三段改写逻辑如下：

```go
// 1) attempt.Model 覆盖
if attempt.Model != "" && aiMeta != nil {
    if err := condition.ReqBodyJsonSet(basicReq, "model", attempt.Model); err != nil {
        log.Logger.Warn("Failed to set model in request body: %s", err)
    } else {
        if outreq.ContentLength >= 0 {
            outreq.ContentLength = -1
            outreq.Header.Del("Content-Length")
        }
        aiMeta.TargetModel = attempt.Model
    }
}

// 2) 前缀剥离
if cluster.AIConf != nil && aiMeta != nil && cluster.AIConf.StripPrefix && cluster.AIConf.MatchPrefix != "" {
    stripProviderPrefix(basicReq, outreq, aiMeta, cluster.AIConf.MatchPrefix)
}

// 3) ModelMapping 映射
if cluster.AIConf != nil && aiMeta != nil && cluster.AIConf.ModelMapping != nil {
    model := aiMeta.ClientModel
    if aiMeta.TargetModel != "" {
        model = aiMeta.TargetModel
    }
    if model != "" {
        if newModel, ok := (*cluster.AIConf.ModelMapping)[model]; ok {
            if err := condition.ReqBodyJsonSet(basicReq, "model", newModel); err != nil {
                log.Logger.Warn("Failed to set model in request body: %s", err)
            } else {
                if outreq.ContentLength >= 0 {
                    outreq.ContentLength = -1
                    outreq.Header.Del("Content-Length")
                }
                aiMeta.TargetModel = newModel
            }
        }
    }
}
```

三段逻辑存在先后顺序和依赖关系：

- `attempt.Model` 覆盖成功后，`aiMeta.TargetModel` 被更新。
- `stripProviderPrefix` 基于 **当前 `aiMeta.TargetModel`（若已设置）或 `aiMeta.ClientModel`** 决定剥离前缀。
- `ModelMapping` 同样基于 **当前 `aiMeta.TargetModel` 或 `aiMeta.ClientModel`** 查找映射。

### 4.2 优化后逻辑

在 `doSingleAIForward` 中按原顺序**计算最终 model 值**，然后统一写入请求体：

```go
// 计算最终需要写入请求体的 model 值
model := aiMeta.ClientModel
if aiMeta.TargetModel != "" {
    model = aiMeta.TargetModel
}

// 1) attempt.Model 覆盖
if attempt.Model != "" {
    model = attempt.Model
}

// 2) provider/model 前缀剥离
if cluster.AIConf != nil && cluster.AIConf.StripPrefix && cluster.AIConf.MatchPrefix != "" {
    if strings.HasPrefix(model, cluster.AIConf.MatchPrefix) {
        stripped := strings.TrimPrefix(model, cluster.AIConf.MatchPrefix)
        if stripped != "" {
            model = stripped
        } else {
            log.Logger.Warn("Model %s stripped by prefix %s results in empty model, skip stripping",
                model, cluster.AIConf.MatchPrefix)
        }
    }
}

// 3) ModelMapping 映射
if cluster.AIConf != nil && cluster.AIConf.ModelMapping != nil && model != "" {
    if newModel, ok := (*cluster.AIConf.ModelMapping)[model]; ok {
        model = newModel
    }
}

// 统一写入请求体，最多一次
if model != aiMeta.ClientModel {
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

设计说明：

- `model != aiMeta.ClientModel` 作为是否真正需要改写的判断条件。由于 `doSingleAIForward` 开始时 `outreq` 从原始请求复制，请求体中的 `model` 等于 `aiMeta.ClientModel`，因此该判断等价于"body 中的 model 是否需要变更"。
- 三个变换仍按原有顺序执行，语义与当前代码一致。
- `ReqBodyJsonSet` 失败时，不再更新 `aiMeta.TargetModel`，与当前行为一致（当前每段逻辑失败时都不更新 TargetModel）。
- `basicReq.HttpRequest.ContentLength` 的重置从 `stripProviderPrefix` 中上移到统一写入位置，所有需要改写 body 的场景都会触发。

### 4.3 `stripProviderPrefix` 重构

原函数同时承担"字符串变换"和"body 操作"两个职责。优化后将其拆分为纯字符串计算函数：

```go
// stripProviderPrefix returns the model string after stripping matchPrefix.
// If the prefix does not match or stripping results in an empty string, it
// returns the original model and false.
func stripProviderPrefix(model string, matchPrefix string) (string, bool) {
    if model == "" || !strings.HasPrefix(model, matchPrefix) {
        return model, false
    }

    stripped := strings.TrimPrefix(model, matchPrefix)
    if stripped == "" {
        log.Logger.Warn("Model %s stripped by prefix %s results in empty model, skip stripping",
            model, matchPrefix)
        return model, false
    }

    return stripped, true
}
```

在 `doSingleAIForward` 中调用：

```go
if cluster.AIConf != nil && cluster.AIConf.StripPrefix && cluster.AIConf.MatchPrefix != "" {
    if stripped, ok := stripProviderPrefix(model, cluster.AIConf.MatchPrefix); ok {
        model = stripped
    }
}
```

---

## 5. 关键代码变更示例

### 5.1 `bfe/bfe_server/reverseproxy.go`

#### 新增/改造 `stripProviderPrefix`

```go
func stripProviderPrefix(model string, matchPrefix string) (string, bool) {
    if model == "" || !strings.HasPrefix(model, matchPrefix) {
        return model, false
    }
    stripped := strings.TrimPrefix(model, matchPrefix)
    if stripped == "" {
        log.Logger.Warn("Model %s stripped by prefix %s results in empty model, skip stripping",
            model, matchPrefix)
        return model, false
    }
    return stripped, true
}
```

#### `doSingleAIForward` 中统一 model 改写

```go
// apply model override from ai route target/fallback, provider prefix stripping,
// and cluster model mapping in order; then write the final model to request body
// at most once.
model := aiMeta.ClientModel
if aiMeta.TargetModel != "" {
    model = aiMeta.TargetModel
}

if attempt.Model != "" {
    model = attempt.Model
}

if cluster.AIConf != nil && cluster.AIConf.StripPrefix && cluster.AIConf.MatchPrefix != "" {
    if stripped, ok := stripProviderPrefix(model, cluster.AIConf.MatchPrefix); ok {
        model = stripped
    }
}

if cluster.AIConf != nil && cluster.AIConf.ModelMapping != nil && model != "" {
    if newModel, ok := (*cluster.AIConf.ModelMapping)[model]; ok {
        model = newModel
    }
}

if model != aiMeta.ClientModel {
    if err := condition.ReqBodyJsonSet(basicReq, "model", model); err != nil {
        log.Logger.Warn("Failed to set model in request body: %s", err)
    } else {
        if outreq.ContentLength >= 0 {
            outreq.ContentLength = -1
            outreq.Header.Del("Content-Length")
        }
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

### 6.1 单元测试更新

`bfe/bfe_server/reverseproxy_ai_test.go` 中现有 `TestStripProviderPrefix`、`TestStripProviderPrefixNoMatch`、`TestStripProviderPrefixEmptyResult` 三个用例直接调用原 `stripProviderPrefix` 并验证 body 改写与 `ContentLength` 重置。

优化后需要：

1. 调整函数签名调用：由 `stripProviderPrefix(req, outreq, aiMeta, prefix)` 改为 `stripProviderPrefix(model, prefix)`。
2. 验证返回值与 `aiMeta.TargetModel` 的更新预期由调用方决定。
3. body 改写与 `ContentLength` 重置的验证可下沉到 `doSingleAIForward` 的集成/单元测试中，或新增针对统一改写逻辑的测试。

#### 更新后的 `TestStripProviderPrefix` 示例

```go
func TestStripProviderPrefix(t *testing.T) {
    model := "openrouter/anthropic/claude-sonnet-4.6"
    stripped, ok := stripProviderPrefix(model, "openrouter/")
    if !ok {
        t.Error("expected stripping to succeed")
    }
    if stripped != "anthropic/claude-sonnet-4.6" {
        t.Errorf("expected stripped model anthropic/claude-sonnet-4.6, got %s", stripped)
    }
}

func TestStripProviderPrefixNoMatch(t *testing.T) {
    model := "anthropic/claude-sonnet-4.6"
    stripped, ok := stripProviderPrefix(model, "openrouter/")
    if ok {
        t.Error("expected stripping to be skipped when prefix does not match")
    }
    if stripped != model {
        t.Errorf("expected model unchanged, got %s", stripped)
    }
}

func TestStripProviderPrefixEmptyResult(t *testing.T) {
    model := "openrouter/"
    stripped, ok := stripProviderPrefix(model, "openrouter/")
    if ok {
        t.Error("expected stripping to be skipped when result is empty")
    }
    if stripped != model {
        t.Errorf("expected model unchanged, got %s", stripped)
    }
}
```

### 6.2 集成测试

现有集成测试已覆盖：

- SC01：路由表查找、model 覆盖、前缀剥离、ModelMapping、fallback。
- SC04：provider/model 前缀剥离。

优化后这些场景的行为保持不变，因此全量运行即可：

```bash
cd bfe
go test ./tests/integration/... -v
```

### 6.3 回归测试

- `go test ./bfe_server/...` 通过。
- `go test ./bfe_modules/mod_ai_route/...` 通过。
- `go test ./tests/integration/...` 通过。

---

## 7. 文档更新

本次优化为纯代码实现优化，不引入新的用户配置或外部行为变更。但 `bfe/docs/zh_cn/sys_design` 中有若干文档包含 `doSingleAIForward()` 的旧实现伪代码，已同步更新：

1. **`bfe/docs/zh_cn/sys_design/multi_api_key.md`**
   - 更新 3.2 节 `doSingleAIForward()` 伪代码，体现 model override → prefix stripping → ModelMapping 的统一计算与单次 `ReqBodyJsonSet` 写入。

2. **`bfe/docs/zh_cn/sys_design/provider_model_prefix_routing.md`**
   - 更新 4.1 节裁剪位置伪代码，说明前缀裁剪现在是统一 model 计算流程中的一步，不再单独调用 `ReqBodyJsonSet`。

3. **`bfe/docs/zh_cn/sys_design/mod_ai_route_bfe_changes.md`**
   - 更新 7.2 节请求体处理，说明最终 model 计算顺序与单次写入机制。

4. **`bfe/docs/zh_cn/modifications/2026-08-21-optimize-ai-model-body-rewrite/design-changes.md`**
   - 本设计变更文档（即本文档）。

---

## 8. 与当前实现的对比

| 维度 | 当前实现 | 优化后 |
|---|---|---|
| `ReqBodyJsonSet` 调用次数 | 最多 3 次 | 最多 1 次 |
| JSON 解析/序列化次数 | 最多 3 次 | 最多 1 次 |
| `ContentLength` 重置代码 | 分散在 3 处 | 统一 1 处 |
| `basicReq.HttpRequest.ContentLength` 重置 | 仅在 `stripProviderPrefix` 中 | 所有 body 改写场景统一处理 |
| `stripProviderPrefix` 职责 | 字符串变换 + body 操作 + ContentLength 重置 | 仅字符串变换 |
| 业务语义 | model 覆盖 → 前缀剥离 → 模型映射 | 保持一致 |

---

## 9. 风险与回滚

| 风险 | 缓解措施 |
|---|---|
| 统一改写后 `aiMeta.TargetModel` 更新时机变化 | 保持"只有 `ReqBodyJsonSet` 成功才更新 TargetModel"的原则，与原逻辑一致 |
| `basicReq.HttpRequest.ContentLength` 重置范围扩大 | 属于正向修复：原本只有前缀剥离场景会重置，其他 body 改写场景也应重置，以保证 fallback/retry 时 body 长度一致 |
| `model != aiMeta.ClientModel` 判断导致某些边界场景跳过改写 | 由于 `doSingleAIForward` 开始时 `outreq` 从原始请求复制，body 中的 model 等于 `ClientModel`，判断等价；如不放心，可改为基于"是否任一变换被触发"判断 |
| 单元测试失效 | 同步更新 `reverseproxy_ai_test.go` |

**回滚**：若优化后发现问题，可回滚 `bfe/bfe_server/reverseproxy.go` 与 `bfe/bfe_server/reverseproxy_ai_test.go` 的修改。该优化不引入配置或协议变更，回滚影响面可控。

---

## 10. 关键代码索引

| 文件 | 行号范围 | 说明 |
|---|---|---|
| `bfe/bfe_server/reverseproxy.go` | 1429-1464 | 当前 `stripProviderPrefix` 实现 |
| `bfe/bfe_server/reverseproxy.go` | 1490-1541 | 当前 `doSingleAIForward` 中的三次 model 改写 |
| `bfe/bfe_basic/condition/primitive.go` | 1176-1200 | `ReqBodyJsonSet` 实现 |
| `bfe/bfe_server/reverseproxy_ai_test.go` | 252-342 | 当前 `stripProviderPrefix` 单元测试 |
