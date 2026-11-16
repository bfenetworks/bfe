# BFE Issue #1499 修复方案：mod_ai_cache JSON 监控 REDIS_ERR/LATENCY_MS 与 prometheus 计数分叉

- Issue: https://github.com/bfenetworks/bfe/issues/1499
- 缺陷：`mod_ai_cache` 在 Redis 依赖故障时业务 fail-open 正确（200、`state=miss`），但 JSON 监控 `/monitor/mod_ai_cache` 的 `REDIS_ERR`、`LATENCY_MS`、`EMBEDDING_ERR`、`EMBEDDING_LATENCY_MS`、`VECTOR_LATENCY_MS` 计数器**从不递增、恒定 0**；同一时刻 `/monitor/mod_ai_cache.prometheus` 的 `redis_err`、`latency_ms` 等正常增长（issue 现场：`redis_err=58` vs `REDIS_ERR=0`）。运维从模块状态页会误判"Redis 无错误"，缓存全量失效长期不可见。同一指标在两个出口自相矛盾，违反模块监控契约：`2026-09-24-ai-cache-exact-match/design-changes.md#§9.2`（"Redis 写失败仅记日志（redis_err 指标）"）、`sys_design/ai_cache.md#§10.2`、`docs/zh_cn/modules/mod_ai_cache/mod_ai_cache.md`（`REDIS_ERR | Counter | Redis 错误数`）。
- 代码库：`bfe/`（行号基于当前 HEAD `5ce3cedb`，逐条核实）
- 关联：#1468 同模式先例（"计费侧信号必须可观测，绝不静默"）；#1474/#1478/#1410 的监控契约惯例。**不需要产品裁定**（同一指标 JSON 与 prometheus 矛盾属实现缺陷）；唯一待裁定项是"启动期 BNS 解析失败是否应阻止启动/显式 degraded"，issue 已明确该条在裁定前不作为关闭判据，本方案将其列为独立后续项。

## 一、根因

### 1.1 模块存在两套并行计数器，本模块惯例是双写——但有 5 个键漏了 state 侧

`bfe_modules/mod_ai_cache` 对每个监控事件维护两份计数：

- **rule table 私有计数器**（`cache_rule_table.go:30-39` 的 `cacheCounters`，13 个键全齐，原子递增；`snapshotCounters()` `:174-191` 读取）——prometheus 出口的数据源（`mod_ai_cache.go:122-140` 的 `getPrometheus()` 用 `snap := m.ruleTable.snapshotCounters()`）；
- **模块 state 计数器**（`mod_ai_cache.go:59-70` `CounterKeys` 声明 13 个键，`m.state.CountersInit`）——JSON 出口的数据源（`getState()` `:113-115` 返回 `m.state.GetAll()`；`.diff` 出口 `:117-120` 基于同一 state）。

既有代码的惯例是**每个事件双写两处**（源码直读核实）：

| 事件 | rule table | module state | state 侧状态 |
| --- | --- | --- | --- |
| 请求计数 | `request.go:71` incReqTotal | `:72` Inc("REQ_TOTAL") | ✓ |
| cache skip | `:76` incCacheSkip | `:77` Inc("CACHE_SKIP") | ✓ |
| cache hit | `:107` incCacheHit | `:108` Inc("CACHE_HIT") | ✓ |
| cache miss | `:151` incCacheMiss | `:152` Inc("CACHE_MISS") | ✓ |
| semantic skipped | `:141` | `:142` | ✓ |
| value too large | `response.go:82` | `:83` | ✓ |
| vector err（响应写路径） | `response.go:102` | `:103` | ✓ |
| **redis err** | `redis.go:48` recordErr→incRedisErr | — | **✗ 全模块无 Inc("REDIS_ERR")** |
| **embedding err** | `semantic.go:60,65` | — | **✗** |
| **vector err（向量库路径）** | `semantic.go:74` | — | **✗（仅响应写路径有）** |
| **redis 延迟** | `redis.go:44` recordLatency→addLatencyMs | — | **✗ 无 Inc("LATENCY_MS")** |
| **embedding 延迟** | `semantic.go:58` | — | **✗** |
| **vector 延迟** | `semantic.go:72` | — | **✗** |

`:260-261` 式的早退不存在——问题不是逻辑走错分支，而是**这 5 个键的 state 侧递增调用从未被写上**。因此 JSON 恒定 0、prometheus 正常增长，两出口永久分叉。

### 1.2 触发路径与业务 fail-open 无关

`redisCache.Get/Setex`（`redis.go:48` 附近）在 Redis 操作失败时记 Warn + `recordErr()`，然后按 miss/false 返回——业务正确 fail-open。但每次故障只增长 rule table 计数，JSON 无任何反映。启动期 BNS 解析失败（`mod_ai_cache.go:221` 直接 `NewRedisClient` 不校验实例；`bfe_util/redis_client/redis_bns.go:283-286` 解析失败仅 Warn）使模块"启动即静默降级"，之后每次 Redis 操作都失败、缓存 100% miss，JSON 仍显示 `REDIS_ERR=0`——issue 现场 2026-10-20 即此形态。

## 二、修复设计

设计原则：**保持模块既有的"rule table + state 双写"惯例，把漏掉的 5 个键补齐**——不引入第三处计数源、不改 JSON/prometheus 两个出口的读取路径，从结构上保证两出口同源一致。

### 步骤 1（必改）：补齐双写

**① `redisCache`（`redis.go`）**：构造时注入模块 state（`newRedisCache` 由 `mod_ai_cache.go:221` 调用，改签名加一个 `state *module_state2.State` 参数），两个 record helper 双写：

```go
func (c *redisCache) recordLatency(start time.Time) {
	ms := time.Since(start).Milliseconds()
	c.table.addLatencyMs(ms)          // prometheus 源（既有）
	if ms > 0 {
		c.state.Inc("LATENCY_MS", ms) // JSON 源（补齐，守卫与 addLatencyMs 一致）
	}
}

func (c *redisCache) recordErr() {
	c.table.incRedisErr()
	c.state.Inc("REDIS_ERR", 1)
}
```

`recordErr`/`recordLatency` 是 `Get/Setex/Delete` 等所有 Redis 操作的统一出口，一处补齐、全部路径覆盖。

**② `semanticCache`（`semantic.go:58-74`）**：同法处理四个 helper（embedding/vector 的 err 与 latency），在其 table 递增旁补 state 递增（`EMBEDDING_ERR`/`EMBEDDING_LATENCY_MS`/`VECTOR_LATENCY_MS`，以及 `semantic.go:74` 向量库路径的 `VECTOR_ERR`——注意 `VECTOR_ERR` 的 state 递增已存在于 `response.go:103`，两处路径都要覆盖但**不得重复**：两条是不同代码路径上的各自一次递增）。

不改 `getState()`/`getStateDiff()`/`getPrometheus()` 的读取逻辑；不改 `CounterKeys`（13 键已齐）。

### 步骤 2（必改）：单元测试

文件：`bfe_modules/mod_ai_cache/` 现有测试旁新增：

1. **Redis 故障双写**：以失败 redis client（mock 或指向无实例的 BNS 配置）驱动 `redisCache.Get`，断言 `m.state` 的 `REDIS_ERR`/`LATENCY_MS` 递增，且与 `ruleTable.snapshotCounters()` 的 `redisErr`/`latencyMs` **精确相等**（机检一致性判据）。
2. **JSON=prometheus 全键一致性**：构造模块并模拟 hit/miss/err/latency 各事件后，断言 `getState()` 的 13 个键与 `snapshotCounters()` 的 13 个字段逐键相等（把 issue 验收判据 1 固化为单测）。
3. **语义缓存臂**：embedding/vector 的 err/latency 注入（复用 `semantic_test.go` 的 mock provider 模式），断言对应 state 键递增。
4. **不重复计数回归**：`REQ_TOTAL`/`CACHE_HIT`/`CACHE_MISS`/`CACHE_SKIP` 的 state 值在事件后恰为 1（不被本修复波及）；`VECTOR_ERR` 两条路径各自只计一次。

### 步骤 3（验收）：机检判据

- 单测（步骤 2）即"JSON 与 prometheus 逐键一致"的机检实现。
- 外部 harness：SC1401-TC017 的 `monitor-redis-err` 断言（`REDIS_ERR` 增量 ≥1）与 issue 复现步骤（key-scoped `redis/key_error` 故障注入）作为真跑验收；本仓用单元测试先行闭环。

### 步骤 4（独立后续项，需产品/运维 owner 裁定，不在本修复关闭范围）

启动期 BNS 解析失败的处理（issue 验收判据 4，裁定前不作为关闭判据）：

- **现状**：`redis_bns.go:283-286` 仅 Warn，模块照常 `Init()` 并对外服务，之后 100% Redis 失败但 JSON 监控恒 0（本修复后 REDIS_ERR 会在首次操作时开始增长，部分缓解"长期不可见"）。
- **建议（裁定参考）**：保持 fail-open 不阻止启动（与模块"缓存是可选加速"的定位一致），但启动期 BNS 解析失败时输出 ERROR 级日志 + 在 JSON 监控增加可机检的启动降级标记（如 `INIT_DEGRADED` 计数器或 `getState()` 附加字段），让"启动即降级"在状态页可见。该项单独立项，不在本卡实现。

## 三、影响面与风险

- 改动文件：`bfe_modules/mod_ai_cache/redis.go`（构造签名 + 2 个 helper）、`semantic.go`（4 个 helper）、`mod_ai_cache.go`（`newRedisCache`/`newSemanticCache` 调用点传 state）、新增测试。`cache_rule_table.go`、两个监控出口、请求/响应主链路**均不动**。
- 行为变化：仅 5 个计数器从"恒 0"变为真实计数；请求路径（fail-open、命中/未命中判定、缓存写入）零变化——issue 验收判据 2（200 fail-open）天然保持。
- 重复计数风险：只新增从未递增过的键的递增调用，既有键的递增点一个不改；`VECTOR_ERR` 两处新增路径与既有 `response.go:103` 是互斥代码路径，不是同一事件双计。
- `.diff` 出口：`stateDiff` 包装 `m.state`，补齐后 `mod_ai_cache.diff` 的 `REDIS_ERR` 等键同步恢复 diff 语义，无需额外改动。
- 验证：`go test ./bfe_modules/mod_ai_cache/...` + `go build ./...`；外部 harness SC1401-TC017 真跑复核。

## 四、验收判据映射（对应 issue）

1. 注入 Redis 故障后 JSON `REDIS_ERR` 增量 ≥1 且与 prometheus `redis_err` 增量一致；`LATENCY_MS`/`latency_ms` 同步——单测 1/2 + SC1401-TC017。
2. 故障下数据面仍 200 fail-open、不写 Redis key、请求到达上游——主链路零改动，现有 fail-open 测试（`request_test.go`/`response_test.go`）回归即证。
3. 既有计数器语义不变、无重复计数——单测 4。
4. 启动期可见性——独立后续项（步骤 4），裁定后单独立项实现。
