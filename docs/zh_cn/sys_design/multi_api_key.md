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

    // 会话级 Key 亲和性
    SessionAffinity              bool   // 默认 false
    SessionAffinityTTL           int    // 绑定空闲超时时间，单位秒，默认 600
    SessionAffinityRedisPrefix   string // Redis key 前缀，默认 "bfe:ai:key_affinity"
    SessionAffinityPenaltyEnable bool   // 是否开启 Key 惩罚，默认 true
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
    ModelProtocols     []string     // supported protocols: openai / anthropic; empty defaults to ["openai"]
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

### 3.2 协议风格识别与匹配

BFE 在 `doSingleAIForward()` 中根据请求特征识别协议/认证风格（`AuthStyle`）：

- 请求路径以 `/v1/messages` 开头 → `anthropic`；
- 请求头存在 `x-api-key` 且不存在 `Authorization` → `anthropic`；
- 否则默认 `openai`。

识别结果写入 `AiBasicInfo.AuthStyle`，并用于：

1. **协议匹配校验**：比较 `AuthStyle` 与 `cluster.AIConf.ModelProtocols`，不匹配时直接返回 400 `PROVIDER_PROTOCOL_MISMATCH`；
2. **认证头注入**：`mod_ai_token_auth.SetApiKey()` 按 `AuthStyle` 注入 `Authorization: Bearer`（OpenAI）或 `x-api-key`（Anthropic）（2026-09 起内部委托协议适配层 `bfe/bfe_model_protocol/` 的适配器 `InjectAuth`，见 `sys_design/model_protocol_adapter.md`）；
3. **版本头注入**：Anthropic 风格下自动补 `anthropic-version: 2023-06-01`。

`AIConf.ModelProtocols` 为空时默认仅支持 `openai`，保证旧配置向后兼容。

### 3.3 `aiClusterInvoke()` 改造

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
    model := aiMeta.ClientModel
    if aiMeta.TargetModel != "" {
        model = aiMeta.TargetModel
    }

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

    // apply cluster.AIConf (api key, provider, protocol)
    if cluster.AIConf != nil {
        if cluster.AIConf.Provider != "" {
            aiMeta.Provider = cluster.AIConf.Provider
        }
        if selectedKey.Key != "" {
            mod_ai_token_auth.SetApiKey(outreq, selectedKey.Key, aiMeta.AuthStyle)
        }
    }

    // Inject anthropic-version for Anthropic style requests if not present.
    if aiMeta.AuthStyle == "anthropic" {
        if outreq.Header.Get("anthropic-version") == "" {
            outreq.Header.Set("anthropic-version", "2023-06-01")
        }
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

### 3.4 Key 选择辅助函数

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

### 3.5 会话级 Key 亲和性（可选）

当 `AIKeyPolicy.SessionAffinity = true` 时，BFE 会基于 `ClientKeyId` 在 Redis 中维护会话到 API-Key 的绑定，使得同一客户的多次请求尽量命中同一个 Provider Key，从而提升跨账户多 Key 场景下的 prompt cache 命中率。

#### 3.5.1 会话标识

使用 BFE 内部 `AiBasicInfo.ClientKeyId` 作为 session id：

- 无需客户端传递额外 header/cookie；
- 比 TCP 连接更稳定（现代客户端普遍存在连接池 / HTTP/2 多路复用）；
- 客户更新 API-Key 值时，只要 `ClientKeyId` 不变，绑定仍然有效。

#### 3.5.2 Redis 数据结构

**客户绑定：**

```
key:   {prefix}:{cluster_name}:{client_key_id}
value: <key_name>
TTL:   session_affinity_ttl 秒（每次命中后刷新）
```

示例：

```
bfe:ai:key_affinity:deepseek-cluster:ckt-abc-123 -> "key-primary"  (TTL=600)
```

`session_affinity_ttl` 是**空闲超时时间**，不是固定生命周期。只要同一个 `client_key_id` 在超时时间内持续请求，BFE 就会在命中绑定后调用 `Expire` 刷新 TTL；只有当会话空闲超过该时间，绑定才会自动释放。

**Key 惩罚（可选）：**

```
key:   {prefix}:penalty:{cluster_name}:{key_name}
value: <reason_code>
TTL:   429 为 60 秒；401/403 为 3600 秒
```

#### 3.5.3 选择流程

在 `aiClusterInvoke()` 中，将原来的 `chooseNextAIKey(keys, state)` 替换为 `chooseAIKeyWithAffinity(...)`：

1. 若未开启亲和性、无 Redis 客户端或 `ClientKeyId` 为空，回退到加权随机；
2. 若 cluster 只有一个有效 Key，直接短路返回，不访问 Redis；
3. 若开启惩罚，先过滤被惩罚的 Key；
4. 从 Redis 读取 `ClientKeyId -> key_name` 绑定：
   - 命中且 Key 仍有效：返回该 Key；
   - 未命中：按候选列表加权随机选择，并写入新绑定。

```go
func chooseAIKeyWithAffinity(
    basicReq *bfe_basic.Request,
    clusterName string,
    keys []cluster_conf.AIKey,
    policy cluster_conf.AIKeyPolicy,
    state *aiKeyAttemptState,
    redisClient redis_client.Client,
) (int, cluster_conf.AIKey, bool) {

    if !policy.SessionAffinity || redisClient == nil {
        return chooseNextAIKey(keys, state)
    }

    sessionID := clientKeySessionID(basicReq)
    if sessionID == "" {
        return chooseNextAIKey(keys, state)
    }

    if len(keys) == 1 && keys[0].Weight > 0 {
        return 0, keys[0], true
    }

    candidateKeys := keys
    if policy.SessionAffinityPenaltyEnable {
        candidateKeys = filterPenaltyKeys(clusterName, keys, redisClient)
    }

    boundName, err := redisGetBinding(clusterName, sessionID, policy, redisClient)
    if err != nil {
        incAffinityRedisErr(clusterName)
        return chooseNextAIKey(keys, state)
    }

    if boundName != "" {
        idx := findKeyIndexByName(boundName, keys)
        if idx >= 0 && isKeyAlive(idx, keys, state) &&
           !isKeyPenalized(clusterName, boundName, redisClient) {
            incAffinityHit(clusterName)
            // 刷新 TTL：将该绑定视为仍在活跃会话中使用
            if err := redisClient.Expire(redisKey(clusterName, sessionID, policy.SessionAffinityRedisPrefix), policy.SessionAffinityTTL); err != nil {
                log.Logger.Warn("aiKeyAffinity: refresh ttl error[%v]", err)
                incAffinityRedisErr(clusterName)
            }
            return idx, keys[idx], true
        }
    }

    idx, key, ok := chooseNextAIKey(candidateKeys, state)
    if ok {
        if err := redisSetBinding(clusterName, sessionID, key.Name, policy, redisClient); err != nil {
            incAffinityRedisErr(clusterName)
        }
    }
    return idx, key, ok
}
```

#### 3.5.4 失败处理与重新绑定

在 Key 级失败处理分支中增加：

| 错误类型 | 原有处理 | 亲和性增强 |
|---|---|---|
| 429 | 标记 `used` | 写短 TTL 惩罚键；删除本客户绑定；切换成功后更新绑定 |
| 401 / 402 / 403 | 标记 `dead` | 写长 TTL 惩罚键；删除本客户绑定；切换成功后更新绑定 |
| 5xx / 连接错误 | 同 Key 退避重试 | 重试耗尽后换 Key，成功则更新绑定 |

当最终成功且实际使用 Key 与绑定不一致时：

```go
if success && usedKey.Name != boundName {
    redisSetBinding(clusterName, sessionID, usedKey.Name, policy, redisClient)
}
```

#### 3.5.5 Redis 故障降级

| 场景 | 行为 |
|---|---|
| Redis Get 失败 | 记录错误指标，回退到加权随机 |
| Redis Setex 失败 | 记录错误指标，仍按选择结果转发 |
| Redis 超时 | 设置较短读写超时，避免阻塞推理请求 |
| Redis 完全不可用 | 亲和能力退化为无状态加权随机 |

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
| `ReqAiKeyAffinityHit` | Counter | 命中 Redis 绑定次数 |
| `ReqAiKeyAffinityMiss` | Counter | 未命中 Redis 绑定次数 |
| `ReqAiKeyAffinityRebind` | Counter | 因失败重新绑定次数 |
| `ReqAiKeyAffinityPenaltySkip` | Counter | 因惩罚跳过 Key 次数 |
| `ReqAiKeyAffinityRedisErr` | Counter | Redis 操作失败次数 |

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
        "ModelProtocols": ["openai"],
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
            "RetryBackoffMax": 5000,
            "SessionAffinity": true,
            "SessionAffinityTTL": 600,
            "SessionAffinityRedisPrefix": "bfe:ai:key_affinity",
            "SessionAffinityPenaltyEnable": true
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
4. **旧字段清理**：`AIConf.Key` 不再保留，统一使用 `AIConf.Keys`；
5. **会话级亲和性依赖 Redis**：`SessionAffinity` 默认关闭，开启后需确保 `mod_ai_rate_limit` 的 Redis 配置可用；Redis 故障时自动降级为加权随机；
6. **单 Key 短路**：当 `AIConf.Keys` 中只有一个有效 Key 时，无论是否开启亲和性，都不会访问 Redis；
7. **ClientKeyId 稳定性**：会话绑定基于 `AiBasicInfo.ClientKeyId`，控制面更新 API-Key 值时应保持该 ID 不变，否则原有绑定会失效。
