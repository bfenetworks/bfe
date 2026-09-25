# BFE AI 缓存设计（mod_ai_cache，简化版：仅精确匹配）

## 1. 背景与定位

### 1.1 背景

AI 网关场景中，终端用户重复提问（问法完全一致的请求）占比可观。对这类请求直接复用历史答案，可以：

- 跳过上游模型调用，显著降低 TTFT 与算力成本；
- 缓存命中时不产生 token 消耗，`mod_ai_token_auth` 跳过配额/费用扣减。

### 1.2 本期范围：仅精确匹配的简化版

本模块为 **第一阶段简化版**，只做 Redis 精确字符串匹配缓存：

- 请求体中用户问题与历史请求**完全一致**时才命中；
- 支持流式（SSE）与非流式响应的缓存与命中返回；
- 外部依赖仅 Redis。

**明确不做**（后续二期演进）：语义/向量缓存（embedding + vector）、相似问题检索、多缓存后端、缓存主动失效 API（本期仅 TTL 过期）。二期可复用本期模块骨架、规则表、SSE 解析与计费协同。

### 1.3 命中率预期

精确缓存只命中"问法完全一样"的请求，命中率低于语义缓存，但实现简单、结果 100% 准确、未命中仅增加一次毫秒级 Redis GET。适合作为低成本验证业务价值的第一阶段。

## 2. 目标

1. 对符合规则的 AI 请求（OpenAI 协议）提供 Redis 精确匹配缓存；
2. 支持流式与非流式响应的缓存与命中返回；
3. 缓存命中跳过配额/费用扣减，访问日志可观测（`ai_cache_status`）；
4. fail-open：Redis 故障只降级不阻断主请求；
5. 租户隔离：不同 API Key 的缓存键互不相通。

## 3. 设计原则

- **规则驱动**：沿用 AI 模块惯例，规则文件按 `map[product] → 规则列表` 组织，`Search(product)` + `Cond.Match(req)` 取第一条命中规则；AI 网关只有默认 product，规则间区分完全由 condition 承担（如 `req_body_json_in("model", ...)`）；
- **fail-open**：Redis 查询失败按未命中处理，回写失败仅记日志，任何情况下不影响主请求；
- **租户隔离**：缓存键强制带 API Key 的 `key_id` 前缀；
- **防缓存污染**：不完整/失败的响应、空答案、超限答案一律不写缓存；支持规则级 `disabled` 与请求级跳过头；
- **大小双限**：`maxBodyBytes`（请求体）与 `maxValueBytes`（缓存值）分别限制两个方向的内存开销。

## 4. 总体架构

```
                        ┌──────────────────────────────┐
                        │ ai-gateway-api 生成规则       │
                        │ conf-agent 下发               │
                        └──────────────┬───────────────┘
                                       ▼
客户端请求 ──► BFE 模块流水线
                 │
                 ▼ mod_ai_token_auth (HandleFoundProduct)  鉴权、配额计划绑定
                 ▼ mod_ai_route     (HandleAfterLocation)  路由解析
                 ▼ mod_ai_cache     (HandleAfterLocation)  查缓存
                 │     ├─ Redis GET
                 │     ├─ 命中：BfeHandlerFinish，模板构造响应
                 │     └─ 未命中：保存 ctx，放行
                 ▼ 上游模型服务
                 ▼ mod_ai_cache     (HandleReadResponse)   回写缓存
                 │     └─ 包装响应体，透传+累积，EOF 时提取答案 SETEX
                 ▼ mod_body_process (HandleReadResponse)   usage 解析
                 ▼ mod_ai_token_auth (HandleRequestFinish) 扣费（识别 AiCacheHit 跳过）
                 ▼ mod_access_pb3   访问日志（ai_cache_status=hit/miss/skip）
```

模块注册位置：`bfe/bfe_modules/bfe_modules.go` 中 `mod_ai_route` 之后、`mod_body_process` 之前：

- 放 `HandleAfterLocation`：此时 product 已解析，可按 product 定位规则；命中短路后不再进入上游；
- 在 `mod_body_process` 之前：命中短路时其响应解析无意义；
- 在 `mod_access_pb3` 之前：`AiCacheStatus` 需先于访问日志写入。

注册两个回调：

```go
cbs.AddFilter(bfe_module.HandleAfterLocation, m.cacheRequestHandler)  // 查缓存，命中短路
cbs.AddFilter(bfe_module.HandleReadResponse, m.cacheResponseHandler)  // 捕获响应，回写缓存
```

## 5. 配置模型

### 5.1 基础配置 `conf/mod_ai_cache/mod_ai_cache.conf`

Redis 连接参数与 `mod_ai_rate_limit.conf` 保持一致（BNS 代理 + 连接池 + 超时）：

```ini
[basic]
ProductRulePath = ../conf/mod_ai_cache/mod_ai_cache_rule.data
CacheKeyPrefix = ai_cache
DefaultCacheTTL = 3600

[redis]
bns = BLB.ALB-redis
connectTimeout = 20
readTimeout = 20
writeTimeout = 20
maxIdle = 20

[log]
OpenDebug = false
```

### 5.2 规则文件 `mod_ai_cache_rule.data`

```json
{
  "Version": "1.0",
  "Config": {
    "default": [
      {
        "cond": "req_path_in(\"/v1/chat/completions\", false) && req_body_json_in(\"model\", \"deepseek-chat\", false)",
        "cacheKeyStrategy": "lastQuestion",
        "cacheTTL": 3600,
        "maxBodyBytes": 1048576,
        "maxValueBytes": 1048576
      },
      {
        "cond": "default_t()",
        "cacheKeyStrategy": "disabled"
      }
    ]
  }
}
```

规则字段：`cacheKeyStrategy`（`lastQuestion` 默认 / `allQuestions` / `disabled`）、`cacheTTL`、`cacheKeyFrom` / `cacheValueFrom` / `cacheStreamValueFrom`（GJSON PATH 提取）、`responseTemplate` / `streamResponseTemplate`（`%s` 占位）、`maxBodyBytes` / `maxValueBytes`。product 查不到时直接放行（与其他 AI 模块一致，不回退 global product）。

## 6. 请求阶段：查缓存

```text
cacheRequestHandler (HandleAfterLocation)
  1. 无 AiBasicInfo / product 无规则 / condition 未命中 → 放行
  2. x-bfe-skip-ai-cache: on → 记 cache_status=skip，放行
  3. 非 application/json → 放行
  4. 读取请求体（> maxBodyBytes 或读不全 → 放行）
  5. 按 cacheKeyStrategy 提取问题，生成缓存键
  6. Redis GET：
     - 命中 → cache_status=hit，AiCacheHit=true，模板构造响应，BfeHandlerFinish
     - 未命中 → cache_status=miss，ctx{key, stream, rule} 存入 request context，放行
```

缓存键格式：

```
{CacheKeyPrefix}:{ClientKeyId}:{sha256(question)[:16]}
```

- `lastQuestion`：默认 PATH `messages.@reverse.0.content`（最后一个用户问题）；
- `allQuestions`：拼接所有 `role=user` 消息的 content；
- `ClientKeyId` 为 API Key 内部标识，未鉴权请求落入 `unknown` 键空间，实现租户隔离。

命中响应构造：`%s` 替换为 **JSON 转义后**的缓存内容（`strconv.Quote` 去引号），非流式返回 `application/json`，流式返回一段完整 SSE（含 `data:[DONE]`），响应头标记 `X-Bfe-Ai-Cache: hit`。

## 7. 响应阶段：回写缓存

```text
cacheResponseHandler (HandleReadResponse)
  1. 无缓存 ctx（未匹配/跳过/命中）或响应非 200 → 放行
  2. 包装 res.Body 为 cacheCaptureBody：透传客户端 + 累积（上限 maxValueBytes）
  3. 流读取完毕（EOF）标记 complete；读错误/客户端中断 → complete=false
  4. Close 时回调：
     - complete=false → 不写（防污染）
     - 非流式：按 cacheValueFrom（默认 choices.0.message.content）提取
     - 流式：解析 SSE 累积内容，按 cacheStreamValueFrom（默认 choices.0.delta.content）拼接
     - 空答案 / 超 maxValueBytes → 不写（计 value_too_large）
     - Redis SET key value EX cacheTTL
```

SSE 解析处理跨 chunk 的 partial message、`[DONE]` 与最后一段 content 同包、首包仅 role 无 content 等边界（对完整累积 buffer 重新分帧，天然免疫网络层拆包差异）。

与 `mod_body_process` / `mod_ai_token_auth` 的包装兼容：本模块先于二者包装 `res.Body`，后装模块的 `bytes_body` / `BodyProcessor` 均通过本包装器透传读取。

## 8. 配额/计费协同

`mod_ai_token_auth` 在 `HandleRequestFinish` 做最终扣费。缓存命中时上游未被调用、`usage` 不存在，因此在 `tokenRequestFinishHandler` 中识别 `AiCacheHit` 直接跳过扣减：

```go
// bfe/bfe_basic/request_ai_basic.go：AiBasicInfo 新增字段
AiCacheHit    bool   // true when the response was served from the AI cache
AiCacheStatus string // cache status: hit / miss / skip, empty if cache not enabled
AiCacheKey    string // cache key, only filled when mod_ai_cache debug is on

// bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go
if ctx.aiBasicInfo.AiCacheHit {
    ctx.deducted = true
    return bfe_module.BfeHandlerGoOn
}
```

## 9. 访问日志与监控

### 9.1 访问日志字段（bfe-access-pb）

| 字段 | 编号 | 说明 |
|---|---|---|
| `ai_cache_status` | 789 | `hit` / `miss` / `skip`，未启用缓存时为空 |
| `ai_cache_key` | 790 | 缓存键，仅 debug 开启时记录，避免日志膨胀 |

### 9.2 模块监控指标

```text
mod_ai_cache.req_total / cache_hit / cache_miss / cache_skip
mod_ai_cache.redis_err / latency_ms / value_too_large
```

## 10. 非功能性设计

| 维度 | 设计 |
|---|---|
| 性能 | 未命中仅增加一次 Redis GET（毫秒级）；连接池复用；超时按未命中处理；命中路径不读上游 |
| 安全-租户隔离 | 缓存键强制带 `key_id` 前缀 |
| 安全-敏感配置 | Redis 密码不进访问日志 |
| 安全-缓存污染 | `disabled` 策略 + 跳过头；错误/不完整/空/超限响应不写缓存 |
| 容灾 | fail-open；TTL + 连接池 + 超时防雪崩；热点 key 并发回源保护为后续增强 |

## 11. 验证

- 单元测试：配置解析、缓存键生成、SSE 解析、命中/未命中/跳过流程、包装器透传与 complete 标记（`bfe/bfe_modules/mod_ai_cache/*_test.go`）；
- 集成测试：SC20 AI 缓存精确匹配（`bfe/tests/integration/implementation/scenario-SC20-ai-cache-exact-match/`），11 个 TC 覆盖命中/未命中/跳过/流式/租户隔离/TTL/大小限制/失败不写缓存/计费协同/键策略。
