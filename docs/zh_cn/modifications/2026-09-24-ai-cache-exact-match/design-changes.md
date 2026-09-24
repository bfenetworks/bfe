# 新增 `mod_ai_cache` 模块（简化版：仅 Redis 精确匹配缓存）

## 1. 背景

本文档是需求《ai-cache需求分析-简化版（仅精确匹配）》在 BFE 数据面的落地方案，与需求文档一一对应。

**第一阶段只做精确字符串缓存**：请求体中用户问题与历史请求完全一致时，直接返回缓存的答案，不再调用上游模型；语义/向量缓存（embedding + vector）整体砍掉，待精确缓存稳定运行并确认命中率达标后再作为二期演进。

**目标**：
- 对符合规则的 OpenAI 协议 AI 请求做 Redis 精确匹配缓存；
- 支持流式（SSE）和非流式响应的缓存与命中返回；
- 缓存命中时跳过配额/费用扣减；
- fail-open：Redis 故障只降级不阻断主请求。

**明确不做**（一期非目标）：语义/向量缓存、相似问题检索、多 provider 缓存后端、缓存主动失效 API（仅 TTL 过期）。

## 2. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 新模块 | 新建 `mod_ai_cache`：请求阶段查缓存（`HandleAfterLocation`），响应阶段回写缓存（`HandleReadResponse`） | `bfe/bfe_modules/mod_ai_cache/*.go`（新建） |
| 模块注册 | 在 `mod_ai_route` 之后、`mod_body_process` 之前插入 `mod_ai_cache` | `bfe/bfe_modules/bfe_modules.go` |
| 基础信息 | `AiBasicInfo` 新增 `AiCacheHit`、`AiCacheStatus` 字段 | `bfe/bfe_basic/request_ai_basic.go` |
| 计费协同 | `mod_ai_token_auth` 识别 `AiCacheHit == true` 时跳过 token/cost 扣减 | `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` |
| 访问日志 | protobuf 新增 `ai_cache_status`（789）、`ai_cache_key`（790，可选调试）字段 | `bfe-access-pb`（独立仓库）、`bfe/bfe_modules/mod_access_pb3/request_log.go` |
| 配置 | 新增模块配置与规则文件样例 | `bfe/conf/mod_ai_cache/mod_ai_cache.conf`、`mod_ai_cache_rule.data`（新建） |
| 文档 | 模块配置文档与部署样例 | `bfe/docs/zh_cn/configuration/mod_ai_cache/`（新建） |
| 测试 | 单元测试 + 集成测试 | `bfe/bfe_modules/mod_ai_cache/*_test.go`、`bfe/tests/integration/implementation/scenario-SCxx-ai-cache/` |

## 3. 模块注册位置

`bfe/bfe_modules/bfe_modules.go` 当前 AI 模块执行顺序：

```text
mod_ai_token_auth   // HandleFoundProduct：API Key 校验与配额计划绑定
mod_ai_route        // 需在 mod_ai_token_auth 之后（依赖 ClientApiKey）
mod_body_process    // HandleReadResponse：解析 SSE 流式 token usage
mod_ai_rate_limit   // 依赖 token 计算
mod_access_pb3
```

插入位置（after `mod_ai_route`）：

```go
// mod_ai_route
// Requirement: after mod_ai_token_auth (needs ClientApiKey)
mod_ai_route.NewModuleAiRoute(),

// mod_ai_cache
// Requirement: after mod_ai_route (only cache requests for a resolved
// product/route); before mod_body_process and mod_access_pb3 (reads the
// request body and must report the cache hit before access logging)
mod_ai_cache.NewModuleAiCache(),

// mod_body_process
mod_body_process.NewModuleBodyProcess(),
```

注册两个回调：

```go
cbs.AddFilter(bfe_module.HandleAfterLocation, m.cacheRequestHandler)  // 查缓存，命中短路
cbs.AddFilter(bfe_module.HandleReadResponse, m.cacheResponseHandler)  // 捕获响应，回写缓存
```

> 查缓存放 `HandleAfterLocation` 的原因：此时 host 路由已完成，`req.Route.Product` 已解析，可以按 product 定位规则表；命中后 `BfeHandlerFinish` 短路返回，不进入后端转发。
> 放 `mod_body_process` 之前的原因：命中短路时 `mod_body_process` 的响应解析对缓存命中无意义，且 `AiCacheStatus` 需要先于 `mod_access_pb3` 写入供访问日志记录。

## 4. 配置模型

配置只保留两类：**缓存服务（Redis）** 和 **规则（缓存策略 + 提取参数）**。完整版中的 `vector`、`embedding`、`thresholdRelation`、`enableSemanticCache` 等字段全部去掉。

### 4.1 模块基础配置 `conf/mod_ai_cache/mod_ai_cache.conf`

Redis 连接参数的命名与组织**与 `mod_ai_rate_limit.conf` 保持一致**（见 `bfe/docs/zh_cn/configuration/mod_ai_rate_limit/mod_ai_rate_limit.conf.md`），便于 `ai-gateway-api` / `conf-agent` 复用同一套生成逻辑。Redis 通过 BNS 代理地址访问，超时按未命中处理（fail-open），因此不提供"出错拒绝"开关。

| 配置项 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
|--------|------|----------|------|----------|------------|
| Basic.ProductRulePath | String | 缓存规则文件路径 | Y | - | 文件须存在且可读 |
| Basic.CacheKeyPrefix | String | 缓存键前缀 | N | 默认 `ai_cache` | 非空字符串 |
| Basic.DefaultCacheTTL | Integer | 默认缓存 TTL（秒） | N | 规则未配置 `cacheTTL` 时生效 | 非负整数 |
| Redis.Bns | String | Redis 代理 BNS 地址 | N | - | 非空字符串 |
| Redis.ConnectTimeout | Integer | 连接 Redis 超时时间 | N | 单位：毫秒 | 必须大于 0 |
| Redis.ReadTimeout | Integer | 读取 Redis 超时时间 | N | 单位：毫秒 | 必须大于 0 |
| Redis.WriteTimeout | Integer | 写入 Redis 超时时间 | N | 单位：毫秒 | 必须大于 0 |
| Redis.MaxIdle | Integer | Redis 连接池最大空闲连接数 | N | - | 非负整数 |
| Redis.MaxActive | Integer | Redis 连接池最大活跃连接数 | N | 0 表示无限制 | 非负整数 |
| Redis.Password | String | Redis 密码 | N | 未设置时忽略，不进访问日志 | - |
| Log.OpenDebug | Boolean | 是否开启 debug 日志 | N | - | - |

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

### 4.2 规则文件 `mod_ai_cache_rule.data`

**结构沿用 BFE AI 模块惯例**：`Config: map[product] → 规则列表`，与其他 AI 模块的 rule table 模式（`Search(product)` + `Cond.Match(req)`）保持一致，使 `ai-gateway-api` / `conf-agent` 可复用同一套生成与下发逻辑，并保留未来多 product 的扩展性。

AI 网关只有默认 product，节点上只部署默认 product 一个 key；**规则间的区分完全由 condition 承担**（如按 `req_body_json_in("model", ...)` 区分不同模型）。product 查不到时直接放行（与其他 AI 模块一致，不回退 global product），因此规则文件中默认 product 的名字必须与 `host_rule.data` 解析出的 product 名一致。

```json
{
  "Version": "1.0",
  "Config": {
    "default": [
      {
        "cond": "req_path_in(\"/v1/chat/completions\") && req_body_json_in(\"model\", \"deepseek-chat\", false)",
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

### 4.3 规则关键字段

| 配置块 | 关键字段 | 说明 |
|--------|----------|------|
| `cache` | `Basic.CacheKeyPrefix`、`Basic.DefaultCacheTTL` | 缓存键前缀与默认 TTL；Redis 连接由 `[redis]` 段统一配置（见 4.1） |
| 策略 | `cacheKeyStrategy` | 缓存键生成策略：`lastQuestion`（默认，取 `messages` 数组最后一个用户问题，GJSON PATH `messages.@reverse.0.content`）、`allQuestions`（拼接所有 `role=user` 的消息内容）、`disabled`（禁用缓存） |
| 提取 | `cacheKeyFrom`、`cacheValueFrom`、`cacheStreamValueFrom`、`cacheToolCallsFrom` | 基于 GJSON PATH 提取字段 |
| 模板 | `responseTemplate`、`streamResponseTemplate` | 命中缓存时返回的响应模板，`%s` 为缓存内容占位 |
| 限制 | `maxBodyBytes`、`maxValueBytes` | 请求体/缓存值大小上限，超限不缓存 |

### 4.4 热加载

与 `mod_ai_token_auth`、`mod_ai_rate_limit` 一致：通过 `web_monitor` 注册热加载（规则文件 + 基础配置），由 `ai-gateway-api` 生成规则、`conf-agent` 下发。模块加载在 `Init` 中先加载规则与 Redis 配置，`MonitorHandlers` 暴露 reload 接口。

## 5. 模块文件结构

```text
bfe/bfe_modules/mod_ai_cache/
├── mod_ai_cache.go              # 模块入口、回调注册、状态监控
├── conf_mod_ai_cache.go         # 模块基础配置解析
├── cache_rule_table.go          # 规则表：Search(product) + Cond.Match(req)
├── cache_rule_load.go           # 规则文件加载
├── cache_state.go               # Prometheus/模块状态
├── redis.go                     # Redis 缓存封装（Get / Setex）
├── request.go                   # 请求阶段：body 读取、键生成、缓存查询、命中响应构造
├── response.go                  # 响应阶段：Body 包装、内容提取、回写缓存
├── sse.go                       # SSE 解析工具
└── *_test.go                    # 单元测试（testing + testify）
```

相比完整版设计，**没有 `provider/` 目录**：embedding、vector 及 cache provider 接口抽象整体删除，Redis 访问直接封装在 `redis.go` 中（基于 `bfe_util/redis_client`，复用连接池，超时 fail-open）。

## 6. 请求阶段设计（`request.go`）

### 6.1 处理流程

```text
客户端请求
    ▼
cacheRequestHandler (HandleAfterLocation)
    - 无 AiBasicInfo → 放行
    - x-bfe-skip-ai-cache: on → 记 cache_status=skip，放行
    - 仅处理 application/json
    - 规则表 Search(req.Route.Product)，未命中规则 → 放行
    ▼
读取请求体（受 maxBodyBytes 限制，超限不缓存，放行）
    ▼
解析 JSON，按 cacheKeyStrategy 生成缓存键 key（租户前缀：key_id/entity + cacheKeyPrefix）
    ▼
Redis GET key
    - 命中：cache_status=hit，构造响应（非流式 responseTemplate / 流式 streamResponseTemplate），BfeHandlerFinish 短路
    - 未命中：cache_status=miss，保存上下文（key、stream 标志、规则）到 req context，恢复 body，放行上游
```

### 6.2 伪代码

```go
func (m *ModuleAiCache) cacheRequestHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
    aiInfo := req.GetAiBasicInfo()
    if aiInfo == nil {
        return bfe_module.BfeHandlerGoOn, nil
    }

    // 1. 匹配缓存规则：先按 product 定位规则列表，再逐条 condition 匹配
    rule := m.ruleTable.Search(req.Route.Product).Match(req)
    if rule == nil {
        return bfe_module.BfeHandlerGoOn, nil
    }

    // 2. 跳过缓存头
    if req.HttpRequest.Header.Get("x-bfe-skip-ai-cache") == "on" {
        m.setCacheStatus(req, "skip")
        return bfe_module.BfeHandlerGoOn, nil
    }

    // 3. 读取请求体（带大小限制）
    body, err := m.readRequestBody(req, rule.MaxBodyBytes)
    if err != nil {
        log.Logger.Warn("mod_ai_cache: read body failed: %v", err)
        return bfe_module.BfeHandlerGoOn, nil
    }

    // 4. 生成缓存键（强制带租户前缀，防止多租户串数据）
    key, stream, err := m.buildCacheKey(body, rule, req)
    if err != nil || key == "" {
        return bfe_module.BfeHandlerGoOn, nil
    }

    // 5. Redis 精确查询（超时/异常按未命中处理，fail-open）
    if value, hit := m.redisGet(rule, key); hit {
        m.setCacheStatus(req, "hit")
        m.state.CacheHitInc()
        return bfe_module.BfeHandlerFinish, m.buildHitResponse(rule, value, stream)
    }

    // 6. 未命中：保存上下文，恢复 body，继续上游
    m.setCacheStatus(req, "miss")
    req.SetContext(CtxAiCache, &aiCacheContext{Key: key, Stream: stream, Rule: rule})
    m.restoreRequestBody(req, body)
    return bfe_module.BfeHandlerGoOn, nil
}
```

### 6.3 缓存键生成

- key 组成：`{cacheKeyPrefix}:{tenant}:{strategy_hash}`，其中 tenant 取 `AiBasicInfo.ClientKeyId`（或 entity），strategy_hash 为按 `cacheKeyStrategy` 提取的问题内容的 hash（如 sha256，十六进制截断）。
- 提取用 GJSON PATH：
  - `lastQuestion`：`messages.@reverse.0.content`；
  - `allQuestions`：遍历拼接所有 `role=user` 的 `content`；
  - 可通过 `cacheKeyFrom` 覆盖默认 PATH。
- 提取值为空 → 不缓存（放行）；同时提取请求是否 `stream=true` 以决定命中时返回流式/非流式模板。

### 6.4 命中响应构造

- **非流式**：用 `responseTemplate`（`%s` 替换为缓存值），返回 `200 OK` + `application/json`。
- **流式**：用 `streamResponseTemplate` 返回一段完整 SSE：

```text
data:{"id":"from-cache","choices":[{"index":0,"delta":{"role":"assistant","content":"%s"}}]}

data:[DONE]

```

（`Content-Type: text/event-stream`，末尾必须有空行结束事件。）

## 7. 响应阶段设计（`response.go`）

### 7.1 处理流程

```text
cacheResponseHandler (HandleReadResponse)
    ▼
包装 res.Body 为 cacheCaptureReadCloser（边透传客户端边累积内容，带 maxValueBytes 上限）
    ▼
响应读取完毕时回调：
    - 从 req context 取 aiCacheContext；无上下文（未匹配规则/跳过/命中）→ 直接放行
    - 非流式：按 cacheValueFrom（默认 choices.0.message.content）提取答案
    - 流式：解析 SSE chunk，按 cacheStreamValueFrom（默认 choices.0.delta.content）累积
    - 提取值为空 → 不写缓存；超过 maxValueBytes → 不写缓存
    - Redis SET key value EX cacheTTL
    - 更新 AiBasicInfo 缓存状态
```

要点：
1. 包装器必须保证**对客户端的透传语义不变**——读多少写多少，只在内部累积；
2. 累积超过 `maxValueBytes` 后停止累积（继续透传），避免 OOM；
3. 响应未完成（客户端中断/上游截断）不写缓存；
4. Redis 写失败仅记日志（`redis_err` 指标），不影响响应。

### 7.2 SSE 边界 case（`sse.go`）

流式解析需处理（实现逻辑与完整版一致）：
- 跨 chunk 的 partial message（一个 SSE event 分多个网络 chunk 到达）；
- `[DONE]` 与最后一段 content 在同一个包内；
- 首包无 `delta.content`（仅 `role`）；
- 多 choices 场景的 index 处理。

实现参考 Higress `main_test.go` 的相关用例（role-only first chunk、content + [DONE] 同包等）。

## 8. 配额/计费协同

`mod_ai_token_auth` 在 `HandleRequestFinish` 做最终扣费，缓存命中时上游不会被调用、`usage` 不存在，因此：

1. `bfe/bfe_basic/request_ai_basic.go` 的 `AiBasicInfo` 新增字段：

```go
AiCacheHit    bool   // true when the response was served from mod_ai_cache
AiCacheStatus string // "", "hit", "miss", "skip"
```

2. `mod_ai_token_auth/mod_ai_token_auth.go` 扣费入口判断：

```go
if aiInfo.AiCacheHit {
    // cache hit: upstream was never called, skip token/cost deduction
    return bfe_module.BfeHandlerGoOn, nil
}
```

3. `mod_ai_cache` 在命中路径上设置 `AiCacheHit = true`、`AiCacheStatus = "hit"`，未命中/跳过分别记录 `"miss"` / `"skip"`，保证访问日志与计费口径一致。

## 9. 访问日志与监控

### 9.1 访问日志字段

| 字段 | 编号 | 说明 |
|---|---|---|
| `ai_cache_status` | 789 | 缓存状态：hit / miss / skip / 空（未启用） |
| `ai_cache_key` | 790 | 缓存键（可选调试，默认关闭，避免日志膨胀） |

改动：`bfe-access-pb`（独立仓库）的 protobuf 定义 + 生成代码；`bfe/bfe_modules/mod_access_pb3/request_log.go` 中的字段填充（从 `AiBasicInfo.AiCacheStatus` 读取）。

### 9.2 模块监控指标（`cache_state.go`）

```text
mod_ai_cache.req_total          // 进入模块的请求数
mod_ai_cache.cache_hit          // 缓存命中数
mod_ai_cache.cache_miss         // 缓存未命中数
mod_ai_cache.cache_skip         // 跳过缓存请求数（x-bfe-skip-ai-cache）
mod_ai_cache.redis_err          // Redis 错误数
mod_ai_cache.latency_ms         // Redis 查询耗时
mod_ai_cache.value_too_large    // 因超限未缓存次数
```

不含 embedding/vector 相关指标。

## 10. 非功能性设计

| 维度 | 设计 |
|---|---|
| 性能 | 未命中请求只增加一次 Redis GET（毫秒级），对 TTFT 影响可忽略；Redis 连接池复用连接；超时（默认 200ms）按未命中处理；命中路径不读上游、不做 body 处理 |
| 安全-租户隔离 | 缓存键强制带 `ClientKeyId`/`entity` 前缀，防止不同租户读到彼此缓存 |
| 安全-敏感配置 | Redis 密码脱敏，不进访问日志 |
| 安全-缓存污染 | 支持 `cacheKeyStrategy: disabled` 与 `x-bfe-skip-ai-cache` 跳过头；对工具调用多、随机性强的路由不开启缓存 |
| 容灾 | **fail-open**：Redis 故障只记日志并放行；缓存雪崩靠连接池 + 超时 + 合理 TTL 控制，热点 key 并发回源保护作为后续增强 |

## 11. 开发任务拆分（WBS）

| 编号 | 任务 | 主要改动文件 | 说明 |
|------|------|--------------|------|
| 1 | 模块骨架与注册 | `bfe/bfe_modules/mod_ai_cache/mod_ai_cache.go`、`bfe/bfe_modules/bfe_modules.go` | 新建模块，注册两个回调，确定执行顺序（第 3 节） |
| 2 | 配置解析与热加载 | `conf_mod_ai_cache.go`、`cache_rule_table.go`、`cache_rule_load.go`、`conf/mod_ai_cache/*` | 规则表按 map[product] 结构组织（仅默认 product 有数据），Redis 配置、模板、TTL、大小限制 |
| 3 | Redis 缓存封装 | `redis.go` | 基于 `bfe_util/redis_client` 封装 Get / Setex，连接池与超时 |
| 4 | 请求阶段处理 | `request.go` | body 读取、缓存键生成、Redis 查询、命中响应构造、body 恢复 |
| 5 | 响应阶段处理 | `response.go` | 包装响应体、累积内容、提取答案、回写 Redis |
| 6 | SSE 解析与流式支持 | `sse.go` | 解析/生成 SSE，处理 `[DONE]`、partial chunk 等边界 case |
| 7 | 配额/计费协同 | `bfe_basic/request_ai_basic.go`、`mod_ai_token_auth/mod_ai_token_auth.go` | 新增 `AiCacheHit` 标志，命中时跳过扣减 |
| 8 | 访问日志与监控 | `bfe-access-pb` protobuf、`mod_access_pb3/request_log.go`、`cache_state.go` | `ai_cache_status`/`ai_cache_key` 字段与模块指标 |
| 9 | 单元测试 | `*_test.go` | 配置解析、缓存键生成、SSE 解析、命中/未命中流程（testing + testify） |
| 10 | 集成测试 | `bfe/tests/integration/implementation/scenario-SCxx-ai-cache/` | 真实 BFE 进程 + Redis，验证命中、跳过、流式、TTL |
| 11 | 文档与配置样例 | `bfe/docs/zh_cn/configuration/mod_ai_cache/`、`conf/mod_ai_cache/` | 配置说明、部署样例 |

依赖关系：任务 1 → 2/3 → 4/5/6 → 7/8 → 9/10/11。任务 8 依赖 `bfe-access-pb` 仓库先行合入。

## 12. 风险与应对

| 风险 | 影响 | 应对 |
|------|------|------|
| 精确匹配命中率可能偏低 | 只能命中"问法完全一致"的请求 | 上线前按《价值与命中率分析》做重复意图统计；精确缓存作为低成本验证手段，命中率不理想再评估语义缓存（二期） |
| Redis 故障影响面 | 缓存功能失效 | fail-open 设计，Redis 异常只降级不阻断（任务 3 超时与错误处理） |
| 缓存与计费冲突 | 命中仍扣费 | 任务 7 修改 `mod_ai_token_auth` 识别 `AiCacheHit` |
| 多租户缓存串数据 | 安全事故 | 缓存键强制带 `key_id`/`entity` 前缀（任务 4 键生成约定） |
| 大响应体内存占用 | OOM 风险 | `maxBodyBytes` + `maxValueBytes` 双重限制，超限停止累积仅透传 |
| 流式解析边界 case | 缓存内容不完整 | 参考 Higress `main_test.go` 的 role-only first chunk、content + [DONE] 同包等用例（任务 6） |

## 13. 后续阶段展望

精确缓存上线后，按以下信号决定是否启动第二阶段（语义缓存，见完整版需求分析）：

1. 精确缓存命中率稳定但业务方反馈"问法不同导致 miss 过多"；
2. 重复意图统计分析显示相似问题占比可观；
3. 愿意接受 embedding + 向量检索带来的额外延迟与部署成本。

若启动二期，完整版中的 `provider/embedding`、`provider/vector` 设计与混合模式查询流程可直接复用；一期模块骨架、规则表、SSE、计费协同、日志监控均可平滑扩展。
