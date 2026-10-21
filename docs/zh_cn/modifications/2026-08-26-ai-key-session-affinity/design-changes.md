# BFE 同 cluster 多 API-Key 会话级 Key 亲和性

## 1. 背景

BFE AI 网关已支持在一个 cluster 下配置多个 Provider API-Key，并在 `aiClusterInvoke()` 内按加权随机选择 Key，失败时在同请求内做 Key 级重试（详见 `bfe/docs/zh_cn/sys_design/multi_api_key.md`）。

当多个 API-Key 来自**不同 Provider 账户/组织**时，同一 agent 会话内的连续请求如果被轮流打到不同账户，每个账户只能看到本账户的 prompt cache，导致上游缓存命中率下降。因此需要在 BFE 侧实现**会话级 API-Key 亲和性**：让同一个客户会话的多个请求尽量落到同一个 Provider Key 上。

## 2. 需求澄清

1. 会话识别不依赖客户端额外传递 header/cookie：
   - 主流 LLM SDK（OpenAI、Anthropic、DeepSeek 等）默认不会发送 `X-Session-ID` 之类的 header；
   - HTTP keep-alive / TCP 连接在现代客户端下无法稳定对应一个逻辑会话；
   - 因此采用 BFE 内部已有的 `AiBasicInfo.ClientKeyId` 作为会话标识。
2. 亲和性只在 cluster 配置多个有效 Key 时生效；单 Key 场景直接短路，不增加 Redis 开销。
3. 保持现有高可用语义：亲和 Key 出现 429 / 401 / 403 / 5xx 时，当前请求仍可重试/切换到其他 Key。
4. 对未开启亲和性的集群完全向后兼容；Redis 故障时自动降级为加权随机。

## 3. 当前现状

| 层级 | 当前能力 | 不足 |
|---|---|---|
| Key 选择 | `aiClusterInvoke()` 调用 `chooseNextAIKey()` 做加权随机 | 同一客户多次请求可能命中不同 Key |
| 失败重试 | 同请求内 Key 级重试，标记 `used`/`dead` | 没有跨请求的状态绑定 |
| 会话标识 | `AiBasicInfo.ClientKeyId` 已存在且稳定 | 未用于 Key 选择 |
| Redis 连接 | `mod_ai_rate_limit` 已初始化 Redis 客户端 | 未在 Key 选择链路复用 |
| 配置 | `AIKeyPolicy` 只有 `Strategy`、`MaxRetries` 等 | 缺少亲和性相关开关 |

## 4. 变更目标

1. 在 `AIKeyPolicy` 中新增会话级 Key 亲和性配置项。
2. 在 `aiClusterInvoke()` 中引入 `chooseAIKeyWithAffinity()`，优先从 Redis 读取 `ClientKeyId` 的绑定 Key；未命中时按加权随机选择并写入 Redis。
3. 增加 Key 惩罚机制：对近期返回 429/401/403 的 Key 写短/长 TTL 惩罚键，选择时跳过。
4. 失败处理时支持重新绑定：当前请求切换 Key 成功后，更新 Redis 绑定。
5. 新增监控指标，用于观察亲和命中率、重新绑定次数、Redis 错误次数等。
6. 补充单元测试与集成测试。

## 5. 变更总览

| 模块 | 主要改动 |
|---|---|
| `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | `AIKeyPolicy` 新增 `SessionAffinity`、`SessionAffinityTTL`、`SessionAffinityRedisPrefix`、`SessionAffinityPenaltyEnable` 字段 |
| `bfe/bfe_server/reverseproxy.go` | 新增 `chooseAIKeyWithAffinity`、`clientKeySessionID`、Redis 绑定读写/删除、惩罚键读写/过滤等 helper；修改 `aiClusterInvoke()` 选择入口；在失败分支增加惩罚与重新绑定逻辑 |
| `bfe/bfe_server/reverseproxy.go` | 新增亲和性监控指标计数 |
| 控制面文档 | 更新 `ai-gateway-api/design-docs/api-define/OpenAPI接口定义/clusters.md`、`ai-gateway-api/design-docs/api-define/InnerAPI接口定义/server-data-conf.md` 中 `key_policy` 的字段说明 |
| 测试 | 新增 `bfe/bfe_server/reverseproxy_ai_affinity_test.go` 等单元测试；新增集成测试场景 |

## 6. 详细设计

### 6.1 配置扩展

**文件：** `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`

```go
type AIKeyPolicy struct {
    Strategy            string // "weighted_random" only in this version
    MaxRetries          int
    RetryBackoffInitial int
    RetryBackoffMax     int

    // 新增
    SessionAffinity              bool   // 默认 false
    SessionAffinityTTL           int    // 绑定空闲超时时间，单位秒，默认 600
    SessionAffinityRedisPrefix   string // 默认 "bfe:ai:key_affinity"
    SessionAffinityPenaltyEnable bool   // 默认 true
}
```

对应 OpenAPI / InnerAPI 的 `key_policy` 同步增加：

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `session_affinity` | bool | false | 是否开启基于 Redis + ClientKeyId 的会话级 Key 亲和 |
| `session_affinity_ttl` | int | 600 | 绑定空闲超时时间（秒）；命中绑定后会刷新 TTL，持续请求则绑定保持 |
| `session_affinity_redis_prefix` | string | "bfe:ai:key_affinity" | Redis key 前缀 |
| `session_affinity_penalty_enable` | bool | true | 是否跳过近期 429/401/403 的 Key |

Redis 连接优先复用 `mod_ai_rate_limit` 已配置的客户端；如需解耦，可在 `BfeServer` / `ReverseProxy` 中独立初始化一个 `redis_client.Client`。

### 6.2 Redis 数据结构

#### 6.2.1 客户绑定

```
key:   {prefix}:{cluster_name}:{client_key_id}
value: <key_name>
TTL:   session_affinity_ttl 秒（每次命中后刷新）
```

示例：

```
bfe:ai:key_affinity:deepseek-cluster:ckt-abc-123 -> "key-primary"  (TTL=600)
```

使用 `key_name` 而非数组下标，避免配置重载后下标变化导致绑定失效。

`session_affinity_ttl` 是**空闲超时时间**，不是固定生命周期。只要同一个 `client_key_id` 在超时时间内持续请求，BFE 就会在命中绑定后调用 `Expire` 刷新 TTL；只有会话空闲超过该时间，绑定才会自动释放。

#### 6.2.2 Key 惩罚（可选）

```
key:   {prefix}:penalty:{cluster_name}:{key_name}
value: <reason_code>
TTL:   429 为 60 秒；401/403 为 3600 秒
```

**过滤口径与熔断语义（2026-10-21 回补，issue #1439 裁定）**：

- `Weight=0` 的 Key 从不参与选 Key，惩罚过滤时**不计入候选**（也不计入 `ReqAiKeyAffinityPenaltySkip` 的 skipped 计数）；
- 被惩罚的 Key 在 TTL 窗口内**严格剔除**（熔断），**不做"回退到被惩罚 Key"的兜底**。过滤后没有候选 Key 时，判定本集群暂不可用：选 Key 失败，`aiClusterInvoke` 返回合成 429（issue #1440），由外层 cluster fallback 决策降级到备用集群；窗口内与上游零交互，TTL 到期后过滤器自然放行、流量自动恢复，新的 429 会刷新 TTL 并顺延熔断期；
- 早期实现曾带有"全部被惩罚则回退全量列表"的兜底分支，该分支与熔断语义冲突（会让被惩罚 Key 在窗口内继续被使用），已按本条款删除；
- 注意：物理上仅配置 1 个 Key 时，单 Key 短路（`len(keys)==1 && Weight>0` 直接返回）会绕过惩罚过滤——**单 Key 集群惩罚不生效**，请为此类集群关闭 `SessionAffinityPenaltyEnable`（配置建议见 issue #1442）。

### 6.3 Key 选择流程

**文件：** `bfe/bfe_server/reverseproxy.go`

在 `aiClusterInvoke()` 中把原来的：

```go
idx, key, ok = chooseNextAIKey(keys, state)
```

替换为：

```go
idx, key, ok = chooseAIKeyWithAffinity(basicReq, cluster.Name, keys, policy, state, redisClient)
```

新增 helper 核心逻辑：

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

    // 单 Key 短路
    if len(keys) == 1 && keys[0].Weight > 0 {
        return 0, keys[0], true
    }

    candidateKeys := keys
    if policy.SessionAffinityPenaltyEnable {
        candidateKeys = filterPenaltyKeys(clusterName, keys, redisClient)
    }

    boundName, err := redisGetBinding(clusterName, sessionID, policy, redisClient)
    if err != nil {
        log.Logger.Warn("aiKeyAffinity: redis get error[%v], fallback to random", err)
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
            log.Logger.Warn("aiKeyAffinity: redis set error[%v]", err)
            incAffinityRedisErr(clusterName)
        }
    }
    return idx, key, ok
}

func clientKeySessionID(basicReq *bfe_basic.Request) string {
    if ai := basicReq.GetAiBasicInfo(); ai != nil && ai.ClientKeyId != "" {
        return ai.ClientKeyId
    }
    return hashString(bfe_basic.GetApiKey(basicReq))
}
```

Redis helper 示意：

```go
func redisKey(clusterName, sessionID, prefix string) string {
    return fmt.Sprintf("%s:%s:%s", prefix, clusterName, sessionID)
}

func redisGetBinding(clusterName, sessionID string, policy cluster_conf.AIKeyPolicy,
    client redis_client.Client) (string, error) {
    key := redisKey(clusterName, sessionID, policy.SessionAffinityRedisPrefix)
    val, err := client.Get(key)
    if err != nil || val == nil {
        return "", err
    }
    if b, ok := val.([]byte); ok {
        return string(b), nil
    }
    return "", fmt.Errorf("unexpected redis value type %T", val)
}

func redisSetBinding(clusterName, sessionID, keyName string, policy cluster_conf.AIKeyPolicy,
    client redis_client.Client) error {
    key := redisKey(clusterName, sessionID, policy.SessionAffinityRedisPrefix)
    return client.Setex(key, []byte(keyName), policy.SessionAffinityTTL)
}
```

### 6.4 失败处理与重新绑定

在 `aiClusterInvoke()` 的失败处理分支中：

| 状态码 | 处理 |
|---|---|
| 429 | 标记 `used`；写短 TTL 惩罚键；删除本客户对该 Key 的绑定；继续选新 Key |
| 401 / 402 / 403 | 标记 `dead`；写长 TTL 惩罚键；删除绑定；继续选新 Key |
| 5xx / 连接错误 | 同 Key 退避重试；重试耗尽后换 Key |
| 其他 4xx | 直接返回 |

当最终成功且实际使用 Key 与绑定不一致时，更新 Redis 绑定：

```go
if success && usedKey.Name != boundName {
    redisSetBinding(clusterName, sessionID, usedKey.Name, policy, redisClient)
}
```

### 6.5 监控指标

新增 Counter 指标：

| 指标 | 含义 |
|---|---|
| `ReqAiKeyAffinityHit` | 命中 Redis 绑定次数 |
| `ReqAiKeyAffinityMiss` | 未命中 Redis 绑定次数 |
| `ReqAiKeyAffinityRebind` | 因失败重新绑定次数 |
| `ReqAiKeyAffinityPenaltySkip` | 因惩罚跳过 Key 次数 |
| `ReqAiKeyAffinityRedisErr` | Redis 操作失败次数 |

### 6.6 Redis 故障降级

| 场景 | 行为 |
|---|---|
| Redis Get 失败 | 记录错误指标，回退到 `chooseNextAIKey` 加权随机 |
| Redis Setex 失败 | 记录错误指标，仍按选择结果转发 |
| Redis 超时 | 设置较短读写超时，避免阻塞推理请求 |
| Redis 完全不可用 | 亲和能力退化为无状态加权随机，BFE 继续服务 |

## 7. 关键代码索引

| 文件 | 行号范围（预计） | 说明 |
|---|---|---|
| `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | `AIKeyPolicy` 结构体 | 新增亲和性配置字段 |
| `bfe/bfe_server/reverseproxy.go` | `aiClusterInvoke()` 附近 | Key 选择入口替换为亲和版本 |
| `bfe/bfe_server/reverseproxy.go` | 新增 helper | `chooseAIKeyWithAffinity`、`clientKeySessionID`、Redis 绑定读写、惩罚键处理 |
| `bfe/bfe_server/reverseproxy.go` | 失败处理分支 | 惩罚键写入、绑定删除、重新绑定 |
| `bfe/bfe_modules/mod_ai_rate_limit/mod_ai_rate_limit.go` | Redis 客户端初始化 | 复用或参考实现 |

## 8. 测试计划

### 8.1 单元测试

新增 `bfe/bfe_server/reverseproxy_ai_affinity_test.go`：

1. `TestChooseAIKeyWithAffinity_Hit`：Redis 命中绑定，返回对应 Key。
2. `TestChooseAIKeyWithAffinity_Miss`：Redis 未命中，按加权随机选择并写入绑定。
3. `TestChooseAIKeyWithAffinity_SingleKey`：单 Key 场景不访问 Redis。
4. `TestChooseAIKeyWithAffinity_PenaltySkip`：被惩罚 Key 被跳过。
5. `TestChooseAIKeyWithAffinity_RedisErrFallback`：Redis 故障时回退到加权随机。
6. `TestChooseAIKeyWithAffinity_RebindOnSuccess`：失败后切换 Key 成功，更新绑定。

使用 mock `redis_client.Client` 或内存 Redis 实现。

### 8.2 集成测试

新增集成测试场景 `bfe/tests/integration/implementation/scenario-SCxx-ai-key-session-affinity/`，覆盖：

1. 同一 `ClientKeyId` 的两次请求命中同一 Provider Key；
2. 亲和 Key 返回 429 后，后续请求切换到其他 Key；
3. Redis 不可用时请求仍能正常返回。

## 9. 影响范围

| 模块/文件 | 影响 |
|---|---|
| `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | `AIKeyPolicy` 新增字段，旧配置默认关闭亲和性 |
| `bfe/bfe_server/reverseproxy.go` | Key 选择、失败处理、指标计数逻辑改动 |
| `bfe/bfe_modules/mod_ai_rate_limit/` | 可能复用其 Redis 客户端 |
| 控制面接口文档 | `key_policy` 增加字段说明 |
| `bfe/docs/zh_cn/modifications/2026-08-26-ai-key-session-affinity/design-changes.md` | 本设计变更文档 |

## 10. 兼容性与风险

### 10.1 兼容性

- `session_affinity` 默认 `false`，未开启时行为与现有逻辑完全一致。
- 单 Key 场景直接短路，不引入 Redis 调用。
- Redis 故障时自动降级为加权随机，不影响请求成功率。

### 10.2 风险与缓解

| 风险 | 缓解措施 |
|---|---|
| 每次请求增加 Redis Get 延迟 | 设置读写超时（如 50ms）；后续可加进程内 LRU 缓存 |
| 长会话导致某些 Key 负载倾斜 | 通过 `ReqAiKeyAffinityHit` 和各 Key 用量监控观察；必要时调整权重 |
| `ClientKeyId` 不稳定导致绑定失效 | 使用 `ClientKeyId` 而非原始 key 值；确认控制面更新 key 值时保持 ID 不变 |
| Redis 单点故障 | 生产环境使用 Redis Sentinel / Cluster 或 BNS 多地址 |
| Token 列表变化后旧绑定指向被删 Key | `findKeyIndexByName` 找不到时自动重新选择 |

## 11. 参考

- `bfe/docs/zh_cn/sys_design/multi_api_key.md`
- `document-ai-gateway/迭代系统设计/v0.6/同cluster下多apikey轮转的会话保持/bfe多Key会话保持机制.md`
- `bfe/bfe_server/reverseproxy.go`
- `bfe/bfe_basic/request_ai_basic.go`
- `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`
- `bfe/bfe_util/redis_client/client.go`
- `bfe/bfe_modules/mod_ai_rate_limit/mod_ai_rate_limit.go`
