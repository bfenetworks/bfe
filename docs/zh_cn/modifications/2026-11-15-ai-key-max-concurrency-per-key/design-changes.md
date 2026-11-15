# BFE 变更说明：API-Key 并发会话数控制（cluster 级 `MaxConcurrencyPerKey`）

- 一句话变更：新增 cluster 级配置 `AIConf.MaxConcurrencyPerKey`（对应控制面 OpenAPI `llm_config.max_concurrency_per_key`）：集群内每把上游 Key 的最大并发会话数（以 `ClientKeyId` 为会话标识，`0`=不限，默认）；占用为 **Redis 全局计数**（复用 `[AIKeyAffinity]` 句柄，ZSet + Lua 原子脚本），**第一期即支持多 BFE 实例**；占用生命周期与 Key 会话亲和性绑定对齐（绑定即占用、空闲 TTL 超时或解绑释放）；槽满时新会话轮换其它 Key，全部不可用走既有合成 429 路径并正常触发 cluster fallback。
- 代码库：`bfe/`，分支 **v1.10.0-dev**。§二 行号为设计时工作区 `c8924529`（2026-11-15）逐条读码核实。
- 关联：`docs/zh_cn/modifications/2026-10-21-issue-1440-key-exhausted-no-fallback/`（耗尽降级）、`2026-10-21-issue-1441-key-exhaustion-error-contract/`（合成 429 错误契约）、`2026-11-04-issue-1448-configurable-key-penalty-ttl/`（KeyPolicy 配置范式）、`docs/zh_cn/sys_design/multi_api_key.md`（多 Key 选择/亲和设计）。
- 状态：**已实现（2026-11-15）**，SC48 集成测试全过（见 §五 验证计划勾选）。实现期间一处设计修正：槽满拒绝需**回滚选择阶段新写入的亲和绑定**（否则下一轮绑定命中路径不查 `usedSet`，反复选中同一把满 Key，轮换空转后 `(nil,nil)` 落出），见 §3.5 行为约定第 2 条。

## 一、背景

多 API-Key 集群（`AIConf.Keys` 加权随机 + 会话亲和，见 `docs/zh_cn/sys_design/multi_api_key.md`）目前对上游 Key 没有任何并发维度保护：任意数量会话可同时压在单把 Key 上。部分上游账号按并发会话授权/计费（如并发 1 的测试账号），需要"一把 Key 同时只服务有限个会话"的硬隔离能力。

本变更在 `llm_config` 增加 cluster 级参数（InnerAPI 导出 `AIConf.MaxConcurrencyPerKey`），语义要点：

| 决策点 | 约定 |
|--------|------|
| 配置位置 | cluster 级单值，对集群内每把 Key 统一生效（不逐 Key 配置） |
| 计数维度 | 会话（`ClientKeyId`），同一会话多个并发请求只计 1 |
| 占用区间 | 绑定即占用：与 Key 亲和绑定同生命周期（每次请求命中刷新空闲 TTL，默认 600s）；超时或解绑释放。**刻意不按"最后一个在飞请求结束"释放**——否则会话会在多把 Key 间 ping-pong，抵消亲和性的 prompt cache 收益 |
| 超限行为 | 非阻塞：新会话选 Key 时槽满 → 跳过轮换；全部 Key 不可用 → 合成 429（既有路径） |
| 一致性 | Redis 全局（多 BFE 实例第一期必做）：上限 = N 精确全局值 |
| Redis 降级 | 未配置/故障时 fail-open（限制不生效 + 指标告警），与亲和性现有降级策略一致 |

## 二、现状核对

### 2.1 接入点（`bfe_server/reverseproxy.go`，行号基于 `c8924529`）

| 事实 | 位置 | 形态 |
|------|------|------|
| cluster 无 Key 短路（透传，不注入不选择） | `:2199-2202` | `len(cluster.AIConf.Keys)==0` 直接 `doSingleAIForward` |
| Key 级重试循环 | `:2263-2407`（`aiClusterInvoke` 主体 `:2163`） | 每轮 `chooseAIKeyWithAffinity` → `doSingleAIForward` |
| 选 Key（亲和/惩罚/加权随机） | `:2279` 调用；实现 `:2990` | 返回 `(idx, key, boundName, ok)` |
| Key 耗尽合成 429（复用路径） | `:2297-2310` | `ErrCode = ErrAiNoAvailableKey`（`:2308`，**刻意非 `ErrAiRateLimit`**） |
| 成功重绑 | `:2328-2336` | `redisSetBinding`（`:3032`） |
| 本地限流短路（不占上游 Key 语义） | `:2342-2344` | `ErrAiRateLimit` 直接返回，不轮换不惩罚 |
| 失败分类 | 429 轮换 `:2364-2374`；401/402/403 dead `:2375-2385`；5xx `keepKey` 同 Key 退避 `:2386-2391`；batch 404 `:2398-2406` | 删绑定点：`:2370`、`:2381`、`redisDeleteBatchBindings :2400`（`:2973`） |
| 会话标识 | `clientKeySessionID` `:2915` | 即 `AiBasicInfo.ClientKeyId`，与亲和性同源 |
| Redis 句柄 | `srv.AIKeyAffinityRedis`（`bfe_server.go:62`） | `initAIKeyAffinityRedis()` `bfe_server.go:247`，`bfe.conf [AIKeyAffinity]` 段，核心自持 |
| fallback 判定 | 循环 `:1617-1678`；短路仅 `ErrAiRateLimit :1648`、`ErrEppRejected :1654`；`shouldTriggerFallback :2623-2653`（429 在白名单） | 合成 429 → **必然触发 cluster fallback**（batch/不可回放截断例外 `:1600-1607`）；链全败返回 primary 错误 `:1686-1695` |
| 本地限流回调（出网前） | `doSingleAIForward :2049-2062` | `HandleAfterAITargetModel` 提前返回分支 |
| Redis 辅助 | `redisGetBinding :3020`、`redisDeleteBinding :3037`、`setKeyPenalty :3051`、`filterPenaltyKeys :3072` | 均为 per-key 独立命令，本变更 Lua 脚本照此范式新增 |

### 2.2 配置与基础设施

| 事实 | 位置 |
|------|------|
| `AIKey{Name, Key, Weight}` | `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:134-139` |
| `AIConf`（`Keys`/`KeyPolicy` 等） | 同文件 `:230-256` |
| `AIConfCheck` / `AIKeyPolicyCheck` | `:1351` / `:1475`；`enc$v1$` 解密先于校验 `:1898` |
| Redis 客户端接口 | `bfe_util/redis_client/client.go:37-51`：`Client` 含 `NewScript(src) RedisScript`（`:50`），`RedisScript.Run(key, args...)`（`:24-29`）——**Lua 能力现成，接口零改动** |
| 指标注册范式 | `bfe_server/proxy_state.go:129-133`（`ReqAiKeyAffinity*`） |

## 三、变更设计

### 3.1 配置结构体

`cluster_conf_load.go`：

```go
// AIConf 新增字段（:230-256 结构体内）
// MaxConcurrencyPerKey limits the number of distinct client sessions
// (ClientKeyId) that may hold a slot on each upstream Key of this cluster,
// across all BFE instances. 0 (default) = unlimited. When > 0 it requires
// the [AIKeyAffinity] Redis (same handle as session affinity); on Redis
// failure the limit fails open (request allowed, counter incremented).
MaxConcurrencyPerKey int
```

`AIConfCheck`（`:1351`）追加：`MaxConcurrencyPerKey < 0` 返回 error（对齐控制面 `ValidateLLMConfig`，防配置穿透）。

### 3.2 Redis 数据结构

```
key:    {SessionAffinityRedisPrefix}:conc:{cluster_name}:{key_name}
type:   ZSET
member: sessionID（ClientKeyId）
score:  占用过期时间戳（毫秒，取自 Redis TIME，避免多实例时钟漂移）
整体:   PEXPIRE = ttl*2 兜底（正常由 member score 控制过期；防实例崩溃后 key 永久残留）
```

### 3.3 Lua 脚本（原子：清过期 + 判满 + 入集/刷新）

```lua
-- KEYS[1] = ZSet key
-- ARGV[1] = sessionID
-- ARGV[2] = limit
-- ARGV[3] = ttl_ms
local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2] / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now)       -- 清理已过期会话
local expireAt = now + tonumber(ARGV[3])
if redis.call('ZSCORE', KEYS[1], ARGV[1]) then              -- 本会话已占用：刷新
    redis.call('ZADD', KEYS[1], expireAt, ARGV[1])
    redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[3]) * 2)
    return 1
end
if redis.call('ZCARD', KEYS[1]) < tonumber(ARGV[2]) then    -- 未满：新会话入集
    redis.call('ZADD', KEYS[1], expireAt, ARGV[1])
    redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[3]) * 2)
    return 1
end
return 0                                                    -- 满
```

脚本经 `client.NewScript(src)` 缓存（`redis_client.Client` 接口已具备），运行时 `Run(key, args...)` 走 EVALSHA。

### 3.4 新组件 `bfe_server/ai_key_concurrency.go`

```go
// acquireKeySessionSlot 非阻塞。
// 返回 (allowed, err)：err != nil 时调用方 fail-open（放行 + 指标 + Warn）。
func acquireKeySessionSlot(client redis_client.Client, cluster, keyName, sessionID string,
    limit int, ttl time.Duration) (bool, error)

// releaseKeySessionSlot 解绑释放（ZREM），与 redisDeleteBinding 同位置成对调用。
func releaseKeySessionSlot(client redis_client.Client, cluster, keyName, sessionID string) error
```

`sessionID` 为空（异常链路无 `ClientKeyId`）时退化为请求级临时占用（用请求唯一标识，随 score 过期释放）。

### 3.5 `aiClusterInvoke` 接入

`reverseproxy.go:2279` 选 Key 成功之后、`:2318` `doSingleAIForward` 之前：

```go
// 伪代码
limit := cluster.AIConf.MaxConcurrencyPerKey
if limit > 0 && redisClient != nil {
    sessionID := clientKeySessionID(basicReq)
    allowed, rerr := acquireKeySessionSlot(redisClient, cluster.Name, key.Name,
        sessionID, limit, time.Duration(policy.SessionAffinityTTL)*time.Second)
    if rerr != nil {
        p.proxyState.ReqAiKeyConcurrencyRedisErr.Inc(1)   // fail-open
        log.Logger.Warn("aiClusterInvoke: acquire session slot failed, fail-open: %v", rerr)
    } else if !allowed {
        state.usedSet[idx] = struct{}{}                   // 本轮跳过，等效 429 轮换
        log.Logger.Info("aiClusterInvoke: ai key [name=%s] session slots full (%d), rotate",
            key.Name, limit)
        keepKey = false
        continue
    }
}
```

行为约定：

1. **已绑定会话永不自挤**：脚本 ZSCORE 命中分支保证亲和绑定会话永远放行（cache 亲和不受并发限制影响）；
2. 槽满只进 `usedSet`，**不写惩罚键**；但绑定未命中路径已新写入亲和绑定，必须**同步回滚该新绑定**（`boundName==""` 时删绑定）——否则下一轮绑定命中路径（不查 `usedSet`）再次选中这把满 Key、轮换空转后以 `(nil,nil)` 落出（客户端拿到 502 而非 429）；失败解绑路径（429/401/402/403/batch 404 三处 `redisDeleteBinding`）成对 `releaseKeySessionSlot`（ZREM）；
3. 全部 Key 不可用 → `:2297-2310` 合成 429 → `ErrAiNoAvailableKey` 不在 `:1648/:1654` 短路名单 → `shouldTriggerFallback` 白名单含 429 → **正常 cluster fallback**；链全败返回 primary 错误（`:1686-1695`）；
4. **无逐请求释放**（不挂 `FinishReq`）：占用随绑定生命周期；keepKey 退避重试只走脚本刷新分支；
5. 本地限流拒绝路径（`:2342-2344`）已占槽的请求随 score TTL 自然释放；业务敏感可在此补 `releaseKeySessionSlot`。

### 3.6 指标（`proxy_state.go` 仿 `:129-133` 追加）

| 指标 | 类型 | 含义 |
|------|------|------|
| `ReqAiKeyConcurrencyLimited` | Counter | 会话槽满跳过 Key 次数 |
| `ReqAiKeyConcurrencyRedisErr` | Counter | 占用/释放 Redis 失败次数（fail-open 次数） |

### 3.7 调用开销

`limit == 0` 的 cluster **零开销、零 Redis 依赖**（不调用）；`>0` 时每请求 1 次 EVALSHA（绑定命中同样要走刷新分支），与现有亲和性 per-key Redis 调用同量级。

## 四、边界与风险

1. **空闲占用**（固有代价）：`limit=1` 的 Key 被空闲会话占用至 TTL（默认 600s）。缓解：调小 `SessionAffinityTTL`、增大 N、增加 Key 数。
2. **Redis 硬依赖**：`>0` 时 Redis 不可用 = 限制静默失效（fail-open），必须配 `ReqAiKeyConcurrencyRedisErr` 告警；生产部署须配置 `[AIKeyAffinity]`。
3. **时钟**：score 必须取 Redis `TIME`（脚本内），多实例时钟漂移会导致误判/上限漂移。
4. **ZREM 覆盖完整性**：`:2370`/`:2381`/`:2400` 三处删绑定必须全部成对 ZREM，漏一处槽泄漏（score 过期可自愈，泄漏期内 Key 假满）。
5. **热加载**：limit/ttl 变更即时生效（脚本按当前值判满），键空间不变，无需迁移。

## 五、验证计划

- 单元测试（mock redis，仿 `bfe_server_init_test.go` 范式）：脚本三分支（刷新/入集/拒满）、fail-open、空 sessionID 退化、热加载变更。
- 集成测试（`tests/integration/`，新增 SC 场景）：**双 BFE 实例**跨实例同 Key 抢槽、第三会话合成 429 且 fallback 到 fallback 集群、绑定会话持续占用与 score 过期释放、失败解绑即时让槽、keepKey 刷新不重复占槽、Redis 故障 fail-open。
- `make test` + 覆盖新增 SC 回归。
