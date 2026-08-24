# 修正 AI 网关 model 字段在 fallback 与重试时的计算逻辑

## 1. 背景

`bfe/bfe_server/reverseproxy.go` 中的 `ServeHTTPForAI()` 负责 AI 网关请求的转发，并在 `doSingleAIForward()` 中根据路由目标、cluster 配置改写请求体里的 `model` 字段。此前实现存在以下问题：

1. **fallback 时未从 `ClientModel` 重新计算**。`doSingleAIForward()` 先从 `aiMeta.TargetModel` 读取当前 model，导致 fallback 到新 cluster 时继承了上一次改写后的 model。
2. **body 改写判断基准错误**。用 `model != aiMeta.ClientModel` 判断是否写 body，但 fallback 后 body 里可能已经是上一个 cluster 的 target model，此时若新 cluster 算出的目标恰好等于 `ClientModel`，会错误地跳过改写。
3. **`aiMeta.TargetModel` 更新不完整**。只在 body 被改写且成功时才更新 `TargetModel`，当目标 model 与 `ClientModel` 相同时会保留旧值。
4. **`HttpReqBodyJsonGet` 调用次数偏多**。每次 `doSingleAIForward()` 都会读取 body 中的 model，同 cluster 的 key 级重试也会重复读取。

## 2. 需求澄清

1. 客户端请求中的 model 字段称为 `ClientModel`，缓存在 `aiMeta.ClientModel`。
2. 最终转发给后端服务的 model 称为 `TargetModel`，缓存在 `aiMeta.TargetModel`。
3. `TargetModel` 的修改步骤：
   - 初始值 = `ClientModel`；
   - 若匹配的 `AiRouteTarget` 或 `AiRouteFallback` 存在 `Model`，则 `TargetModel = attempt.Model`；
   - 若 cluster 配置要求 `StripPrefix`，则从 `TargetModel` 去掉 prefix；
   - 若 cluster 配置了 `ModelMapping`，则 `TargetModel` 按映射表转成最终值；
   - 根据最终 `TargetModel` 修改 body 中的 `model` 字段。
4. 需要重试时，对新的 cluster 要重复上述步骤。注意 body 中可能已经是上一个路由目标或 cluster 修改后的 model，不要误传。
5. 尽量减少 `ReqBodyJsonSet` 调用：用变量缓存 body 中的实际 model，和 `TargetModel` 不同才修改。

## 3. 变更目标

1. 每个 cluster 尝试都从 `ClientModel` 开始计算 `TargetModel`。
2. 每次 `doSingleAIForward()` 都无条件更新 `aiMeta.TargetModel`。
3. 用变量缓存 body 中的实际 model，仅在真正变化时调用 `ReqBodyJsonSet`。
4. 保持 model 覆盖 → prefix strip → model mapping 的原有执行顺序。
5. 同步更新相关设计文档，确保伪代码与实际代码一致。

## 4. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 转发层 | `ServeHTTPForAI()` 增加 `bodyModel` 缓存并透传；`aiClusterInvoke()` / `doSingleAIForward()` 增加 `bodyModel` 入参与返回值 | `bfe/bfe_server/reverseproxy.go` |
| 文档 | 更新 `provider_model_prefix_routing.md`、`mod_ai_route_bfe_changes.md` 中的 model 计算伪代码与重试分析 | `docs/zh_cn/sys_design/...` |

## 5. 详细设计

### 5.1 `bodyModel` 缓存与透传

`ServeHTTPForAI()` 在 cluster 级 fallback 循环开始前从 `aiMeta.ClientModel` 初始化 `bodyModel`。`aiMeta.ClientModel` 在进入反向代理前已从请求体中解析出来，因此这里不需要再解析 body。后续每次 cluster/key 级尝试都复用 `bodyModel` 变量，并随 `aiClusterInvoke()` → `doSingleAIForward()` 调用链透传：

```go
// bodyModel tracks the model currently in the request body.
// aiMeta.ClientModel was already extracted from the request body before
// entering the reverse proxy, so we can use it as the initial value without
// parsing the body again. Only this loop modifies the body model, so we
// keep the value in a variable instead of parsing the body on every attempt.
bodyModel = aiMeta.ClientModel

for i, attempt := range attempts {
    ...
    res, action, lastCluster, invokeErr, bodyModel =
        p.aiClusterInvoke(srv, serverConf, basicReq, rw, attempt, aiMeta, bodyModel)
    ...
}
```

`aiClusterInvoke()` 与 `doSingleAIForward()` 的签名同步扩展：

```go
func (p *ReverseProxy) aiClusterInvoke(..., bodyModel string) (
    res *bfe_http.Response, action int, cluster *bfe_cluster.BfeCluster, err error, newBodyModel string)

func (p *ReverseProxy) doSingleAIForward(..., bodyModel string) (
    res *bfe_http.Response, action int, err error, newBodyModel string)
```

### 5.2 `TargetModel` 计算与 body 改写

`doSingleAIForward()` 中：

1. 每个 cluster 尝试都从 `aiMeta.ClientModel` 开始计算，不再读取 `aiMeta.TargetModel`。
2. 按 `attempt.Model` → `StripPrefix` → `ModelMapping` 顺序得到 `targetModel`。
3. 无条件更新 `aiMeta.TargetModel = targetModel`。
4. 比较 `targetModel` 与缓存的 `bodyModel`，只有不一致时才调用 `ReqBodyJsonSet`；改写成功后更新 `newBodyModel = targetModel`。

```go
// Calculate the final model in order: route target/fallback override ->
// provider/model prefix stripping -> cluster model mapping. Then write it
// to the request body only when it differs from the current body value.
// Always start from ClientModel for every cluster attempt, so fallbacks
// recompute the target model from the original client value.
targetModel := aiMeta.ClientModel

// apply model override from ai route target/fallback
if attempt.Model != "" {
    targetModel = attempt.Model
}

// strip provider/model prefix according to cluster AIConf
if cluster.AIConf != nil && cluster.AIConf.StripPrefix && cluster.AIConf.MatchPrefix != "" {
    if stripped, ok := stripProviderPrefix(targetModel, cluster.AIConf.MatchPrefix); ok {
        targetModel = stripped
    }
}

// apply cluster model mapping
if cluster.AIConf != nil && cluster.AIConf.ModelMapping != nil && targetModel != "" {
    if mapped, ok := (*cluster.AIConf.ModelMapping)[targetModel]; ok {
        targetModel = mapped
    }
}

// record the final target model for this cluster attempt
aiMeta.TargetModel = targetModel

// bodyModel is cached by the caller and updated here only when we actually
// change the body, so we avoid parsing the request body on every attempt.
newBodyModel = bodyModel
if targetModel != bodyModel {
    if err := condition.ReqBodyJsonSet(basicReq, "model", targetModel); err != nil {
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
        newBodyModel = targetModel
    }
}
```

### 5.3 fallback 与重试正确性

- **cluster 级 fallback**：新 cluster 从 `ClientModel` 重新计算，配合 `bodyModel` 缓存，能正确识别 body 中是否仍是上一个 cluster 的 target model，并在需要时改回或改成新的 target model。
- **key 级重试**：同一个 cluster 配置不变，`targetModel` 不变；第一次重试改写 body 后 `newBodyModel = targetModel`，后续重试传入的 `bodyModel` 已等于 targetModel，不会重复调用 `ReqBodyJsonSet`。

## 6. 关键代码索引

| 文件 | 行号范围 | 说明 |
|---|---|---|
| `bfe/bfe_server/reverseproxy.go` | 1280-1297 | `ServeHTTPForAI()` 中 `bodyModel` 缓存与透传 |
| `bfe/bfe_server/reverseproxy.go` | 1454-1546 | `doSingleAIForward()` 中 `TargetModel` 计算与 body 改写 |
| `bfe/bfe_server/reverseproxy.go` | 1548-1670 | `aiClusterInvoke()` 透传 `bodyModel` |

## 7. 测试验证

- `go test ./bfe_server/...` 通过。
- `make test` 全量通过，包括：
  - `scenario-SC04-provider-model-prefix-strip`
  - `scenario-SC05-access-log-ai-fields`

## 8. 文档更新

- 更新 `docs/zh_cn/sys_design/provider_model_prefix_routing.md` 第 4、5 节，同步 model 计算伪代码与重试分析。
- 更新 `docs/zh_cn/sys_design/mod_ai_route_bfe_changes.md` 第 4.2、4.3.2、7 节，同步 `bodyModel` 透传与模型覆盖逻辑。
