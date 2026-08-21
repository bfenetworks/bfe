# BFE 多 API-Key 支持

## 1. 背景与目标

### 1.1 背景

AI 网关场景下，一个后端集群（cluster）通常需要配置多个大模型服务 API-Key：

- 实现 API-Key 级别的负载分担与故障隔离；
- 当某个 API-Key 因限流（429）或鉴权失败（401/403）失效时，自动切换到其他 Key；
- 对后端 5xx 或连接错误，支持同 Key 退避重试。

BFE 已具备 AI 网关独立转发路径 `ServeHTTPForAI()` 与 cluster 级 fallback 机制（见 [mod_ai_route 对应 BFE 主程序修改方案](./mod_ai_route_bfe_changes.md)）。多 API-Key 支持在此基础上，将 Key 级选择/重试内聚到 `aiClusterInvoke()` 中，与外层 cluster 级 fallback 解耦。

### 1.2 目标

1. `cluster.AIConf` 支持 `Keys` 数组与 `KeyPolicy` 策略；
2. `aiClusterInvoke()` 内按权重选择 API-Key，失败时自动轮换或退避重试；
3. Key 级重试耗尽后，将结果返回给 `ServeHTTPForAI()` 外层，由 cluster 级 fallback 决定是否继续尝试下一个集群；
4. 与 ai-gateway-api 导出的 `server_data_conf` 格式对齐。

---

## 2. 数据结构

### 2.1 BFE 侧 `AIConf` 扩展

```go
// AIKey represents a single API key for AI service
type AIKey struct {
    Name   string // identifier
    Key    string // API key value
    Weight int    // weight for weighted random selection, [0,100]
}

// AIKeyPolicy represents routing/retry policy for AI keys
type AIKeyPolicy struct {
    Strategy            string // "weighted_random" only in this version
    MaxRetries          int    // total retry budget within one aiClusterInvoke call
    RetryBackoffInitial int    // ms
    RetryBackoffMax     int    // ms
}

// ModelPrice represents a single model pricing entry
type ModelPrice struct {
    Provider            string
    Model               string
    BaseModel           string
    Mode                string
    Capabilities        []string
    SupportedParameters []string
    Limits              map[string]int
    Prices              map[string]float64
}

// ModelTable represents the cost/pricing table for a cluster
type ModelTable struct {
    Currency string       // fixed "RMB" in v0.4
    Models   []ModelPrice
}

// AIConf is the AI service configuration for a cluster
type AIConf struct {
    Type               int
    ModelMapping       *map[string]string
    Provider           string       // provider name in model_prices
    Keys               []AIKey      // multiple API keys; empty means no key injection
    KeyPolicy          *AIKeyPolicy // key selection & retry policy
    ModelTable         *ModelTable  // pricing table, auto-filled by InnerAPI
}
```

> 说明：旧字段 `AIConf.Key` 不再保留，统一使用 `AIConf.Keys`。

### 2.2 配置来源

`AIConf` 由 ai-gateway-api 通过 InnerAPI `/configs/tls_conf/server_data_conf` 下发，对应 OpenAPI `/clusters` 中的 `llm_config` 字段。详细导出格式见 `ai-gateway-api/design-docs/api-define/InnerAPI接口定义/server-data-conf.md`。

---

## 3. 转发层设计

### 3.1 与 `ServeHTTPForAI()` 的关系

```
ServeHTTPForAI()
    │
    ├── 选择 target
    ├── 构建 attempts [selected target + fallbacks]
    ├── 准备可回退请求体
    │
    └── 对每个 attempt 循环（cluster 级 fallback）
            │
            ▼
        aiClusterInvoke()
            │
            ├── 选择 API-Key
            ├── 构造 OutRequest
            ├── 模型覆盖 / API-Key 注入
            ├── clusterInvoke()
            │
            └── 失败？→ Key 轮换 / 同 Key 退避重试
```

- **cluster 级 fallback**：由 `ServeHTTPForAI()` 控制，在 target 失败或后端 5xx 时切换到下一个 fallback cluster；
- **Key 级重试**：由 `aiClusterInvoke()` 控制，在同一 cluster 内多个 API-Key 之间选择/重试。

### 3.2 `aiClusterInvoke()` 改造

`aiClusterInvoke()` 新增 Key 级重试循环。为支持重试，将单次转发逻辑抽取为 `doSingleAIForward()`：

```go
func (p *ReverseProxy) doSingleAIForward(srv *BfeServer, cluster *bfe_cluster.BfeCluster,
    basicReq *bfe_basic.Request, rw bfe_http.ResponseWriter,
    attempt aiForwardAttempt, aiMeta *bfe_basic.AiBasicInfo,
    selectedKey cluster_conf.AIKey) (
    res *bfe_http.Response, action int, err error) {

    req := basicReq.HttpRequest

    // prepare out request
    outreq := new(bfe_http.Request)
    *outreq = *req
    basicReq.OutRequest = outreq

    httpProtoSet(outreq)
    hopByHopHeaderRemove(outreq, req)

    if cluster.DisableHostHeader {
        outreq.Host = ""
    }

    // Calculate the final model in order: route target/fallback override ->
    // provider/model prefix stripping -> cluster model mapping. Then write it
    // to the request body at most once to avoid repeated JSON parsing/serialization.
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

    // apply cluster.AIConf (api key)
    if cluster.AIConf != nil && selectedKey.Key != "" {
        mod_ai_token_auth.SetApiKey(outreq, selectedKey.Key)
    }

    // invoke cluster
    return p.clusterInvoke(srv, cluster, basicReq, rw)
}
```

`aiClusterInvoke()` 内部逻辑：

```go
func (p *ReverseProxy) aiClusterInvoke(srv *BfeServer, serverConf *bfe_route.ServerDataConf,
    basicReq *bfe_basic.Request, rw bfe_http.ResponseWriter,
    attempt aiForwardAttempt, aiMeta *bfe_basic.AiBasicInfo) (
    res *bfe_http.Response, action int, cluster *bfe_cluster.BfeCluster, err error) {

    // ... look up cluster ...

    // no keys configured
    if cluster.AIConf == nil || len(cluster.AIConf.Keys) == 0 {
        res, action, err = p.doSingleAIForward(..., cluster_conf.AIKey{})
        return res, action, cluster, err
    }

    policy := defaultAIKeyPolicy()
    if cluster.AIConf.KeyPolicy != nil {
        policy = *cluster.AIConf.KeyPolicy
    }

    keys := cluster.AIConf.Keys

    // ensure request body is rewindable for key-level retry
    if policy.MaxRetries > 0 && !prepareRequestBodyForRetry(basicReq.HttpRequest) {
        log.Logger.Warn("aiClusterInvoke: request body not rewindable, disable key-level retry")
        policy.MaxRetries = 0
    }

    state := &aiKeyAttemptState{
        usedSet: make(map[int]struct{}),
        deadSet: make(map[int]struct{}),
    }

    var lastErr error
    for retry := 0; retry <= policy.MaxRetries; retry++ {
        if retry > 0 {
            if !rewindRequestBody(basicReq.HttpRequest) {
                break
            }
            time.Sleep(calcBackoff(policy.RetryBackoffInitial, policy.RetryBackoffMax, retry))
        }

        idx, key, ok := chooseNextAIKey(keys, state)
        if !ok {
            log.Logger.Warn("aiClusterInvoke: all ai keys exhausted for cluster[%s]", attempt.ClusterName)
            break
        }

        res, action, err = p.doSingleAIForward(..., key)

        lastErr = err
        statusCode := 0
        if res != nil {
            statusCode = res.StatusCode
        }

        // success or 4xx client error
        if err == nil && statusCode < 500 {
            return res, action, cluster, nil
        }

        // classify failure
        switch {
        case statusCode == 429:
            state.usedSet[idx] = struct{}{} // rotate key
        case statusCode == 401 || statusCode == 402 || statusCode == 403:
            state.deadSet[idx] = struct{}{} // dead key
        case statusCode >= 500 || err != nil:
            // transient failure, retry same key with backoff
        }
    }

    return res, action, cluster, lastErr
}
```

### 3.3 Key 选择辅助函数

```go
// aiKeyAttemptState tracks key usage within one aiClusterInvoke call
type aiKeyAttemptState struct {
    usedSet map[int]struct{} // keys used for 429
    deadSet map[int]struct{} // keys dead for 401/402/403
}

var aiKeyRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// selectAIKey selects one key by weighted random.
func selectAIKey(keys []cluster_conf.AIKey) (cluster_conf.AIKey, int) {
    if len(keys) == 1 {
        return keys[0], 0
    }

    total := 0
    for _, k := range keys {
        total += k.Weight
    }
    if total <= 0 {
        return cluster_conf.AIKey{}, -1
    }

    r := aiKeyRand.Intn(total)
    sum := 0
    for i, k := range keys {
        sum += k.Weight
        if r < sum {
            return k, i
        }
    }
    return keys[len(keys)-1], len(keys) - 1
}

// chooseNextAIKey returns next eligible key and its index.
func chooseNextAIKey(keys []cluster_conf.AIKey, state *aiKeyAttemptState) (int, cluster_conf.AIKey, bool) {
    var eligible []cluster_conf.AIKey
    var indices []int

    for i, k := range keys {
        if k.Weight == 0 {
            continue
        }
        if _, dead := state.deadSet[i]; dead {
            continue
        }
        eligible = append(eligible, k)
        indices = append(indices, i)
    }

    if len(eligible) == 0 {
        return -1, cluster_conf.AIKey{}, false
    }

    var filteredKeys []cluster_conf.AIKey
    var filteredIdx []int
    for j, k := range eligible {
        idx := indices[j]
        if _, used := state.usedSet[idx]; used {
            continue
        }
        filteredKeys = append(filteredKeys, k)
        filteredIdx = append(filteredIdx, idx)
    }

    if len(filteredKeys) == 0 {
        // all alive keys used (429 only), reset used_set and retry
        state.usedSet = make(map[int]struct{})
        filteredKeys = eligible
        filteredIdx = indices
    }

    _, selectedIdx := selectAIKey(filteredKeys)
    if selectedIdx < 0 {
        return -1, cluster_conf.AIKey{}, false
    }
    return filteredIdx[selectedIdx], filteredKeys[selectedIdx], true
}

// calcBackoff calculates exponential backoff with jitter.
func calcBackoff(initial, max, attempt int) time.Duration {
    backoff := initial
    for i := 1; i < attempt; i++ {
        backoff *= 2
        if backoff > max {
            backoff = max
            break
        }
    }
    jitter := backoff / 5
    if jitter > 0 {
        backoff = backoff - jitter/2 + aiKeyRand.Intn(jitter)
    }
    return time.Duration(backoff) * time.Millisecond
}
```

---

## 4. 失败分类与边界

### 4.1 Key 级失败处理

| 错误类型 | 处理方式 |
| - | - |
| 429 Too Many Requests | 标记当前 Key 为 `used`，轮换到其他 Key |
| 401 / 402 / 403 | 标记当前 Key 为 `dead`，不再使用 |
| 5xx / 连接错误 / 超时 | 视为瞬态失败，同 Key 退避重试 |
| 成功或 4xx（除上述外） | 立即返回，停止 Key 级重试 |

### 4.2 与 cluster 级 fallback 的边界

`aiClusterInvoke()` 将最终结果返回给 `ServeHTTPForAI()` 外层：

- 若 Key 级重试最终得到 2xx/3xx，直接返回给客户端；
- 若 Key 级重试最终得到 5xx、连接错误或特定 4xx（400/401/402/403/422/429），`shouldTriggerFallback()` 返回 true，触发 cluster fallback；
- 若得到其他 4xx（如 404/405 等请求级错误），不触发 cluster fallback，直接返回。

```go
var aiFallbackStatusCodes = map[int]struct{}{
    400: {},
    401: {},
    402: {},
    403: {},
    422: {},
    429: {},
}

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

## 5. 监控与日志

建议增加的监控指标：

| 指标 | 类型 | 含义 |
| - | - | - |
| `ReqAiKeyRotation` | Counter | Key 轮换次数（按 429/401/403 分类） |
| `ReqAiKeyRetry` | Counter | Key 级重试次数 |
| `ReqAiKeyExhausted` | Counter | Key 全部耗尽次数 |

关键日志：

```
aiClusterInvoke: select ai key [name=%s weight=%d] for cluster[%s]
aiClusterInvoke: ai key [name=%s] failed with status[%d], rotate/dead/retry
aiClusterInvoke: all ai keys exhausted for cluster[%s]
```

---

## 6. 配置示例

```json
{
    "AIConf": {
        "Type": 0,
        "Provider": "deepseek",
        "Keys": [
            {
                "Name": "key-primary",
                "Key": "sk-aaaaaaaaaaaa",
                "Weight": 70
            },
            {
                "Name": "key-secondary",
                "Key": "sk-bbbbbbbbbbbb",
                "Weight": 30
            }
        ],
        "KeyPolicy": {
            "Strategy": "weighted_random",
            "MaxRetries": 3,
            "RetryBackoffInitial": 500,
            "RetryBackoffMax": 5000
        },
        "ModelMapping": {
            "gpt-4": "deepseek-v3"
        },
        "ModelTable": {
            "Currency": "RMB",
            "Models": [
                {
                    "Provider": "deepseek",
                    "Model": "deepseek-v3",
                    "BaseModel": "deepseek-v3",
                    "Mode": "chat",
                    "Capabilities": ["chat", "reasoning", "tools"],
                    "SupportedParameters": ["temperature", "max_tokens"],
                    "Limits": {
                        "context_window": 128000,
                        "max_input_tokens": 128000,
                        "max_output_tokens": 8192
                    },
                    "Prices": {
                        "input_cost_per_token": 0.000002,
                        "output_cost_per_token": 0.000008
                    }
                }
            ]
        }
    }
}
```

---

## 7. 注意事项

1. **请求体可回退性**：Key 级重试依赖 `basicReq.HttpRequest.Body` 可重复读取。`aiClusterInvoke()` 会在启用 Key 级重试前调用 `prepareRequestBodyForRetry()`；
2. **SSE 流式响应**：所有 Key 尝试完成后才返回响应，不会出现已开始发送后切换 Key 的情况；
3. **与 `ServeHTTP()` 隔离**：多 API-Key 逻辑仅作用于 `ServeHTTPForAI()` 路径；
4. **旧字段清理**：`AIConf.Key` 不再保留，统一使用 `AIConf.Keys`。
