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

    // Key 惩罚时长（秒），加载期 0 补默认、负数报错、超 86400 截断
    SessionAffinityPenaltyTTL429  int // 上游 429 时的惩罚时长，默认 60
    SessionAffinityPenaltyTTLAuth int // 上游 401/402/403 时的惩罚时长，默认 300
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

    // MaxConcurrencyPerKey limits the number of distinct client sessions
    // (ClientKeyId) that may hold a slot on each upstream Key of this
    // cluster, across all BFE instances. 0 (default) = unlimited; when > 0
    // it requires the [AIKeyAffinity] Redis and fails open on Redis errors.
    MaxConcurrencyPerKey int
}
```

> 说明：旧字段 `AIConf.Key` 不再保留，统一使用 `AIConf.Keys`。

### 2.2 配置来源

`AIConf` 由 ai-gateway-api 通过 InnerAPI `/configs/tls_conf/server_data_conf` 下发，对应 OpenAPI `/clusters` 中的 `llm_config` 字段。详细导出格式见 `ai-gateway-api/design-docs/api-define/InnerAPI接口定义/server-data-conf.md`。

> 说明：`AIConf.Keys[].Key` 支持 `enc$v1$` 前缀的字段级密文落盘（控制面导出时加密，BFE 加载时经 `bfe.conf` `[Security]` keyring 解密进内存），无此前缀按明文直通。解密后内存形态与明文配置完全一致，本文所述 Key 选择/轮换/亲和逻辑不变。详见 [下发配置文件敏感字段加密落盘设计](config_file_field_encryption.md)。

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
3. **版本头注入**：Anthropic 风格下自动补 `anthropic-version: 2023-06-01`；
4. **残留凭证头清理（互斥注入，issue #1447）**：网关接管上游凭证时（`selectedKey.Key != ""`），出站请求**只保留本次注入的那一个凭证头**，客户端带来的其它凭证头（`Authorization` / `x-api-key` / `x-goog-api-key`）一律删除；删除发生在 `Header.Clone()` 之后的副本上，**入站请求的 header 不被改写**（客户端凭证在鉴权、计费、日志脱敏、Key 亲和性 `clientKeySessionID` 上仍完整可用）。原因：上游对这三个头的优先级是 **provider 私有且方向相反**的（实测 kimi 认 `x-api-key`、bailian 认 `Authorization`），残留头会让上游用**客户端的**凭证鉴权 → 上游 401 → 被误判为该 Key 鉴权失败并写入惩罚键（1 个客户端即可打死整个集群）；**无 `Keys` 配置的集群（透传模式）不清理、不注入**，客户端凭证原样转发。

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
            // issue #1440: key exhaustion must yield a decidable result —
            // a synthetic 429 (unified AiError, no ErrCode), never (nil, nil)
            log.Logger.Warn("aiClusterInvoke: all ai keys exhausted for cluster[%s], return synthetic 429", attempt.ClusterName)
            res = newSynthetic429Resp(basicReq)
            return res, action, cluster, nil
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

亲和性使用的 Redis 连接由 `bfe.conf` 的 `[AIKeyAffinity]` 段配置并由 `bfe_server` 核心自持（不再借用 `mod_ai_rate_limit` 模块的 Redis）；该段未配置或 `Disabled=true` 时，即使开启 `SessionAffinity`，亲和也静默不生效（fail-open），请求按加权随机分发。

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
TTL:   429 → SessionAffinityPenaltyTTL429（默认 60 秒）
       401/402/403 → SessionAffinityPenaltyTTLAuth（默认 300 秒）
       404-batch → 固定 60 秒（非配置项）
```

惩罚时长是集群级配置（`KeyPolicy.SessionAffinityPenaltyTTL*`），加载期归一化：缺省 `0` 补默认值，负数报错，超过 `86400` 截断并打 Warn。只有新写入的惩罚键使用新值，Redis 不支持批量重设 TTL，已存在的长惩罚仍需等过期或人工 `DEL`。所有可用 Key 都被惩罚时合成的 429，其 `Retry-After` 取 `SessionAffinityPenaltyTTL429`，表示「按当前配置，被惩罚 Key 的最长剩余等待时间」，不是精确剩余值（精确值需 `PTTL`，见 §7 二期）。

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
| 429 | 标记 `used` | 写惩罚键（`SessionAffinityPenaltyTTL429`，默认 60s）；删除本客户绑定；切换成功后更新绑定 |
| 401 / 402 / 403 | 标记 `dead` | 写惩罚键（`SessionAffinityPenaltyTTLAuth`，默认 300s）；删除本客户绑定；切换成功后更新绑定 |
| 批量 404 | 标记 `used` | 写惩罚键（固定 60s，reason `404-batch`）；清理批量绑定 |
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

### 3.6 Key 并发会话数限制（`AIConf.MaxConcurrencyPerKey`）

cluster 级配置 `MaxConcurrencyPerKey`（对应控制面 OpenAPI `llm_config.max_concurrency_per_key`）对集群内每把 Key 统一生效：同一时刻最多允许 N 个不同会话（以 `AiBasicInfo.ClientKeyId` 标识）绑定到**一把** Key，同一会话的多个并发请求只计 1。`0`（默认）= 不限，链路零改动、零 Redis 调用。

#### 3.6.1 占用语义：与亲和绑定同生命周期

占用**不按"最后一个在飞请求结束"释放**——若如此，会话 A 请求结束放槽 → 会话 B 抢占 → 会话 A 下一请求被挤走重绑，多会话交替突发时 `ClientKeyId` 在多把 Key 间 ping-pong，prompt cache 命中率显著下降。占用生命周期与 §3.5 的亲和绑定完全一致：

- 会话绑定 Key 起占用一个会话槽；
- 每次请求命中刷新空闲时间（与绑定 TTL 刷新同节奏）；
- **绑定空闲超时（`SessionAffinityTTL`，默认 600s）或失败解绑时释放**；
- 已绑定会话命中自己 Key 时永远放行（ZSet 成员命中即刷新），**永不被自己 Key 的并发限制挤走**。

代价（明确接受）：会话空闲期间仍占槽至 TTL。缓解：调小 `SessionAffinityTTL`、增大 N、增加 Key 数。

#### 3.6.2 Redis 数据结构与 Lua 脚本

**第一期即要求多 BFE 实例精确上限**（不做每实例独立计数——那会使上限退化为 N × 实例数）。复用 `srv.AIKeyAffinityRedis` 句柄（`bfe.conf [AIKeyAffinity]` 段，与亲和性同一连接）：

```
key:    {prefix}:conc:{cluster_name}:{key_name}
type:   ZSET
member: sessionID（ClientKeyId）
score:  占用过期时间戳（毫秒，取自 Redis TIME，消除多实例时钟漂移）
整体:   PEXPIRE = ttl*2 兜底（防实例崩溃后 key 永久残留）
```

Lua 原子脚本（EVALSHA，经 `redis_client.Client.NewScript` 缓存）：

```lua
-- KEYS[1] = ZSet key; ARGV[1]=sessionID, ARGV[2]=limit, ARGV[3]=ttl_ms
local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2] / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now)        -- 清过期
local expireAt = now + tonumber(ARGV[3])
if redis.call('ZSCORE', KEYS[1], ARGV[1]) then               -- 已占用：刷新
    redis.call('ZADD', KEYS[1], expireAt, ARGV[1])
    redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[3]) * 2)
    return 1
end
if redis.call('ZCARD', KEYS[1]) < tonumber(ARGV[2]) then     -- 未满：入集
    redis.call('ZADD', KEYS[1], expireAt, ARGV[1])
    redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[3]) * 2)
    return 1
end
return 0                                                     -- 满
```

#### 3.6.3 接入点与行为

接入点在 `aiClusterInvoke()` 选 Key 成功之后、`doSingleAIForward` 之前：

1. `limit == 0` 或 Redis 客户端为 nil：完全跳过（零开销）；
2. `acquireKeySessionSlot` 返回拒绝 → 该 Key 进 `usedSet` 本轮跳过（等效 429 轮换），**不写惩罚键**；但绑定未命中路径已新写入亲和绑定，必须**同步回滚该新绑定**（`boundName==""` 时删绑定），否则下一轮绑定命中路径（不查 `usedSet`）再次选中这把满 Key、轮换空转；对**失败导致的解绑**（429/401/402/403/batch 404），在现有 `redisDeleteBinding` 位置成对 `releaseKeySessionSlot`（ZREM）；
3. Redis 调用失败 → **fail-open**（放行 + `ReqAiKeyConcurrencyRedisErr` 指标 + Warn 日志），与亲和性现有降级策略一致——fail-close 会把 Redis 故障放大为全集群 429 风暴；
4. 全部 Key 不可用（槽满/惩罚/weight=0/dead）→ 走 §4.2 合成 429 路径（`ErrCode = ErrAiNoAvailableKey`，**不是** `ErrAiRateLimit`），429 在 `shouldTriggerFallback` 白名单 → **正常触发 cluster fallback**；
5. 失败解绑路径（429/401/402/403/batch 404 删绑定处）成对 `releaseKeySessionSlot`（ZREM）让槽；**无逐请求释放**（不挂 FinishReq），keepKey 退避重试只走脚本刷新分支。

#### 3.6.4 边界

| 场景 | 行为 |
|------|------|
| 单把有效 Key | 并发限制依然生效（占用判定在选 Key 后进行，单 Key 只是免亲和读绑定）；槽满时新会话直接合成 429 |
| `ClientKeyId` 为空（异常链路） | 退化为请求级临时占用（请求唯一标识，随 score 过期释放） |
| 热加载变更 limit/ttl | 即时生效（脚本每次按当前值判满），键空间不变，无需迁移 |
| Redis 未配置/故障 | fail-open，限制不生效；恢复后自动恢复限制 |
| 本地限流 `ErrAiRateLimit` 拒绝 | 已占槽的请求随 score TTL 自然释放（至多一个 TTL 周期的占用；业务敏感可在 early-return 分支补 ZREM） |

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

**Key 耗尽契约（issue #1440）**：当一次选 Key 都未成功（全部 Key 被惩罚 / Weight=0 / dead）时，`aiClusterInvoke()` **必须返回可判定的最终结果**——构造一个合成 429 响应（统一 AiError 目录的 `UPSTREAM_RATE_LIMITED`/`rate_limit_error` 形态，带 `X-Bfe-Gw-Error` 网关标记，**不设 `ErrCode`**，尤其不得设 `ErrAiRateLimit`，以免触发 local-RL 的 fallback 禁止短路），不得返回 `(res=nil, err=nil)`。这样外层 `shouldTriggerFallback()` 能按白名单触发 cluster fallback；未配置 fallback 时客户端拿到可重试的 429，而不是网关合成的空 body 500。网关标记同时保证 `normalizeUpstreamError`（`AIConf.NormalizeUpstreamError`）跳过该响应，不做二次改写。

**nil/nil 不变量（issue #1440 加固）**：`shouldTriggerFallback()` 将 `(res=nil, err=nil)` 一律视为可降级（返回 true），作为防御不变量——任何调用路径漏构造返回值时兜底生效，而不是静默吞掉降级机会。该判定位于 `ErrAiRateLimit` 短路之后，不改变本地限流拒绝的语义；no route / no cluster 等更早的失败路径在进入 fallback 循环前即退出，不受影响。

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
| `ReqAiKeyConcurrencyLimited` | Counter | Key 并发会话槽满被跳过的次数 |
| `ReqAiKeyConcurrencyRedisErr` | Counter | 并发会话占用/释放 Redis 失败次数（fail-open 触发次数） |

> 说明：`ReqAiKeyAffinityMiss` 统计未命中 Redis 绑定的次数，包含会话首触与绑定失效后重新选择；未命中是正常控制流，不计入 `ReqAiKeyAffinityRedisErr`（issue #1472）。

关键日志：

```
aiClusterInvoke: select ai key [name=%s weight=%d] for cluster[%s]
aiClusterInvoke: ai key [name=%s] failed with status[%d], rotate/dead/retry
aiClusterInvoke: all ai keys exhausted for cluster[%s]
aiClusterInvoke: all ai keys exhausted for cluster[%s], return synthetic 429  # issue #1440：耗尽时返回合成 429
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
        "MaxConcurrencyPerKey": 0,
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
5. **会话级亲和性依赖 Redis**：`SessionAffinity` 默认关闭；开启后需在 `bfe.conf` 配置 `[AIKeyAffinity]` 段（bfe_server 自持 Redis 连接，不复用 `mod_ai_rate_limit` 的 Redis），未配置时亲和静默不生效（fail-open）；Redis 故障时自动降级为加权随机；
6. **单 Key 短路**：当 `AIConf.Keys` 中只有一个有效 Key 时，无论是否开启亲和性，选择阶段都不读取惩罚状态、也不写入绑定键；但失败时写侧仍会写入惩罚键（没有读侧消费），因此惩罚时长配置在该形态下无实际效果；
7. **ClientKeyId 稳定性**：会话绑定基于 `AiBasicInfo.ClientKeyId`，控制面更新 API-Key 值时应保持该 ID 不变，否则原有绑定会失效；
8. **并发会话限制依赖 [AIKeyAffinity] Redis**：`MaxConcurrencyPerKey > 0` 时必须配置 `bfe.conf [AIKeyAffinity]` 段且保障其可用性；Redis 未配置/故障时限制 fail-open（不生效 + `ReqAiKeyConcurrencyRedisErr` 指标告警），恢复后自动恢复；
9. **并发会话限制的释放只有两条路径**：ZSet 成员 score 过期（空闲超时，同 `SessionAffinityTTL`）或失败解绑 ZREM；所有删亲和绑定的路径（429/401/402/403/batch 404）必须成对 ZREM，漏一处即槽泄漏（score 过期可自愈，泄漏期内 Key 假满）。
