# Provider/Model 前缀路由裁剪

## 1. 背景与目标

### 1.1 背景

在 AI 网关实际使用中，部分模型聚合平台（如 OpenRouter）要求客户端在请求 Body 的 `model` 字段中携带 `provider_name/model_name` 格式，例如：

```json
{
  "model": "openrouter/anthropic/claude-sonnet-4.6",
  "messages": [...]
}
```

当请求命中对应的 cluster 后，BFE 转发给下游前需要将该前缀裁剪掉，使下游收到的是平台内部认可的模型名：

```json
{
  "model": "anthropic/claude-sonnet-4.6",
  "messages": [...]
}
```

本方案在 cluster 级别增加 `match_prefix` / `strip_prefix` 开关，由 ai-gateway-api 负责配置下发，BFE 负责实际裁剪逻辑。

### 1.2 目标

1. 支持 cluster 配置 `MatchPrefix` 和 `StripPrefix`；
2. BFE 在 `doSingleAIForward()` 中按配置完成前缀裁剪；
3. 保持 `ClientModel` 不变，更新 `TargetModel` 以反映下游实际模型名；
4. 前缀裁剪不影响 key-level retry 和 route-level fallback 的正确性；
5. 明确当前方案对 Token 鉴权、限流模块的局限性。

## 2. 设计原则

- **配置驱动**：是否裁剪、裁剪什么前缀完全由 cluster 配置决定，不做硬编码；
- **最小侵入**：只在 `doSingleAIForward()` 中插入裁剪逻辑，不修改已有路由、鉴权、限流流程；
- **状态清晰**：`ClientModel` 保持客户端原始值，`TargetModel` 表示当前应向下游发送的模型名；
- **向后兼容**：未配置 `MatchPrefix` / `StripPrefix` 时保持现有行为不变。

## 3. 配置扩展

### 3.1 `AIConf` 结构体扩展

**文件：** `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`

```go
type AIConf struct {
    Type         int                // type of LLM service, reserved for future use. should be 0 now.
    ModelMapping *map[string]string // model mapping, key is model name in req, value is model name in backend
    Provider     string             // provider name in model_prices
    Keys         []AIKey            // multiple API keys; empty means no key injection
    KeyPolicy    *AIKeyPolicy       // key selection & retry policy
    ModelTable   *ModelTable        // pricing table, auto-filled by InnerAPI

    // 新增
    MatchPrefix string `json:"MatchPrefix,omitempty"` // 例如 "openrouter/"
    StripPrefix bool   `json:"StripPrefix"`           // 是否裁剪该前缀
}
```

说明：

- `MatchPrefix`：定义该 cluster 负责匹配的前缀，必须以 `/` 结尾。
- `StripPrefix`：匹配成功后，转发给下游前是否裁剪该前缀。

### 3.2 配置加载时校验

在 `AIConfCheck()` 中增加校验逻辑：

```go
func AIConfCheck(conf *AIConf) error {
    if conf.ModelTable != nil {
        if err := ModelTableCheck(conf.ModelTable); err != nil {
            return fmt.Errorf("ModelTable:%s", err.Error())
        }
    }

    // 新增校验
    if conf.StripPrefix {
        if conf.MatchPrefix == "" {
            return fmt.Errorf("MatchPrefix is required when StripPrefix is true")
        }
        if !strings.HasSuffix(conf.MatchPrefix, "/") {
            return fmt.Errorf("MatchPrefix must end with '/'")
        }
    }

    return nil
}
```

说明：

- `StripPrefix=true` 时，`MatchPrefix` 必须非空；
- `MatchPrefix` 必须以 `/` 结尾，避免前缀匹配到模型名本身。

## 4. BFE 转发裁剪逻辑

### 4.1 裁剪位置

**文件：** `bfe/bfe_server/reverseproxy.go`

在 `doSingleAIForward()` 函数中，在 **route target model override** 之后、**cluster `ModelMapping`** 之前，插入前缀裁剪逻辑：

```go
// apply model override from ai route target/fallback
if attempt.Model != "" && aiMeta != nil {
    // ... 现有逻辑
}

// 新增：按 cluster AIConf 裁剪 provider 前缀
if cluster.AIConf != nil && aiMeta != nil && cluster.AIConf.StripPrefix && cluster.AIConf.MatchPrefix != "" {
    model := aiMeta.ClientModel
    if aiMeta.TargetModel != "" {
        model = aiMeta.TargetModel
    }
    if model != "" && strings.HasPrefix(model, cluster.AIConf.MatchPrefix) {
        stripped := strings.TrimPrefix(model, cluster.AIConf.MatchPrefix)
        if stripped != "" {
            if err := condition.ReqBodyJsonSet(basicReq, "model", stripped); err != nil {
                log.Logger.Warn("Failed to strip provider prefix in request body: %s", err)
            } else {
                // outreq body already changed, need reset Content-Length
                if outreq.ContentLength >= 0 {
                    outreq.ContentLength = -1
                    outreq.Header.Del("Content-Length")
                }
                aiMeta.TargetModel = stripped
            }
        }
    }
}

// apply cluster.AIConf (api key, model mapping)
if cluster.AIConf != nil && aiMeta != nil {
    // ... 现有逻辑
}
```

关键细节：

- 只裁剪 **第一段** provider 前缀。例如 `openrouter/anthropic/claude-xxx` → `anthropic/claude-xxx`。
- 裁剪后再执行 `ModelMapping`，因此 `ModelMapping` 的 key 应使用裁剪后的模型名。
- 如果 `attempt.Model` 已覆盖目标模型，则基于覆盖后的模型进行前缀裁剪。
- 裁剪后内容为空时（如 `openrouter/`），跳过裁剪并记录 warn，避免下发空 model。

### 4.2 `ClientModel` 与 `TargetModel` 的区分

**文件：** `bfe/bfe_basic/request_ai_basic.go`

`AiBasicInfo` 结构体已区分 `ClientModel`（客户端原始模型名）和 `TargetModel`（转发给下游的模型名）。前缀裁剪后应更新 `TargetModel`，保持语义一致。

当前 `http_conn.go` 初始化时：

```go
model, err := condition.ReqBodyJsonFetch(request, "model", nil)
if err == nil || len(model) > 0 {
    aiMeta.ClientModel = model
    aiMeta.TargetModel = model
}
```

`ClientModel` 保持原始值不变，`TargetModel` 在 `doSingleAIForward()` 中随裁剪/映射更新。

## 5. 重试安全性分析

修改 `aiMeta.TargetModel` 不会影响下一次重试的正确性，原因如下：

1. **请求体不会被回滚为原始请求体**：BFE 的 `bytes_body.Rewind()` 只是将当前 buffer 重新设为读取起点，不会恢复到客户端原始 body。因此已裁剪的请求内容在重试时会被保留。
2. **`HasPrefix` 保护避免重复裁剪**：第一次裁剪后 `TargetModel` 已不含该前缀，下次进入 `doSingleAIForward()` 时不会再命中 `MatchPrefix`，不会二次裁剪。
3. **`ClientModel` 始终保持原始值**：日志、计费等地方仍能看到客户端原始模型名。

下文给出关键结论。

## 6. 对 Token 鉴权和限流的影响

### 6.1 Token 鉴权的模型允许列表

**文件：** `bfe/bfe_modules/mod_ai_token_auth/token_rule_table.go`

`ValidateUserTokenByReq()` 从请求 Body 读取 `model` 并与 `token.Models` / `token.BlockModels` 做精确匹配。

**本次 v0.4 暂时采用方案 A**：保持现有 token 鉴权逻辑不变，客户在 API Key 的 `models` / `block_models` 中配置带前缀的完整模型名。

> ⚠️ **局限性**：方案 A 仅适用于请求模型名与产品语义上的真实模型名一致或一一对应的场景。对于 OpenRouter 等聚合中转站，请求模型名、裁剪后模型名、`ModelMapping` 目标模型名、ai-gateway-api 中的 `BaseModel` 四者可能各不相同。若 Token 允许列表仅按原始请求模型名配置，既无法表达“允许使用某个 BaseModel”的真实意图，也无法覆盖同一模型通过不同前缀请求的情况。后续需要引入基于 `BaseModel` 的鉴权机制。

### 6.2 限流的模型匹配

**文件：** `bfe/bfe_modules/mod_ai_rate_limit/mod_ai_rate_limit.go`

`matchModel()` 支持精确匹配和 `*` 通配符。限流策略中的 `model` 配置与 token 鉴权类似：

- 若配置带前缀的模型名（如 `openrouter/anthropic/claude-xxx`），当前逻辑可直接工作；
- 若希望按裁剪后模型名限流，需要前置裁剪逻辑。

**本次 v0.4 保持现状**，由用户在限流策略中配置与请求一致的模型名格式。

> ⚠️ **局限性**：当前限流按 `meta.ClientModel`（即原始请求模型名）做匹配和计数 key。在 OpenRouter 等聚合中转场景下，同一真实模型可能以多种请求形态出现，限流会被拆分为多个独立的 key，导致限流失准。更深层的问题在于：限流真正需要按“归一化模型名（`BaseModel`）”聚合，而不是按“裁剪后模型名”或“原始请求模型名”。具体原因在下文展开。

## 7. 配置示例

ai-gateway-api 下发到 BFE 的 cluster 配置示例：

```json
{
  "ClusterBasic": {...},
  "BackendConf": {...},
  "AIConf": {
    "Type": 0,
    "Provider": "openrouter",
    "MatchPrefix": "openrouter/",
    "StripPrefix": true,
    "Keys": [
      {
        "Name": "default",
        "Key": "sk-xxx",
        "Weight": 100
      }
    ],
    "KeyPolicy": {
      "Strategy": "weighted_random",
      "MaxRetries": 0,
      "RetryBackoffInitial": 500,
      "RetryBackoffMax": 5000
    }
  }
}
```

效果：

- 客户端请求 `model: "openrouter/anthropic/claude-sonnet-4.6"`
- 路由到该 cluster
- BFE 转发给 OpenRouter 时，`model` 变为 `"anthropic/claude-sonnet-4.6"`

## 8. 总结

BFE 侧改动范围：

1. `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`：
   - `AIConf` 增加 `MatchPrefix` / `StripPrefix`；
   - `AIConfCheck()` 增加校验逻辑。
2. `bfe_server/reverseproxy.go`：
   - `doSingleAIForward()` 中插入前缀裁剪逻辑。

实现原则：**最小化改动，仅在 cluster 配置层增加开关，不侵入现有路由、鉴权、限流逻辑。** 对于 OpenRouter 等聚合 provider 场景下 Token 鉴权和限流的问题，本期仅做标注，后续需引入基于 `BaseModel` 的鉴权/限流机制。
