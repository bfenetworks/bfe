# BFE AI 缓存设计（mod_ai_cache：精确匹配 + 语义缓存）

## 1. 背景与定位

### 1.1 背景

AI 网关场景中，终端用户重复提问占比可观，分两档：

- **问法完全一致**的重复请求：直接复用历史答案，跳过上游模型调用；
- **问法不同、意图相同**的相似请求（客服 FAQ、知识库问答、RAG 场景常见）：需要语义级匹配才能命中。

缓存命中时不产生上游 token 消耗，`mod_ai_token_auth` 跳过配额/费用扣减，同时显著降低 TTFT 与算力成本。

### 1.2 两阶段路线

本模块分两阶段演进：

| | 一期（已上线） | 二期（本文第 6-8 节，已实现） |
|---|---|---|
| 缓存形态 | Redis 精确字符串匹配 | 混合模式：精确优先，未命中走语义检索 |
| 命中条件 | 问题文本与历史请求完全一致 | 问题 embedding 与历史问题的向量相似度超过阈值 |
| 外部依赖 | Redis | Redis + embedding 服务 + Chroma 向量库 |
| 未命中额外延迟 | 一次 Redis GET（毫秒级） | + embedding（超时封顶 500ms）+ 向量查询（超时封顶 300ms） |

二期复用一期全部骨架：模块注册位置、规则表、SSE 解析、命中响应模板、计费协同、访问日志机制均不变，仅做插入式扩展。

### 1.3 命中率预期

精确缓存只命中"问法完全一样"的请求，命中率低但结果 100% 准确；语义缓存命中率取决于业务重复意图密度（客服/知识库/RAG 类场景公开数据约 30%–70%），需按路由灰度验证（见 11 节）。

## 2. 目标

1. 对符合规则的 AI 请求（OpenAI 协议）提供 Redis 精确匹配缓存；
2. 支持流式与非流式响应的缓存与命中返回；
3. 缓存命中跳过配额/费用扣减，访问日志可观测（`ai_cache_status`）；
4. fail-open：Redis 故障只降级不阻断主请求；
5. 租户隔离：不同 API Key 的缓存键、向量记录互不相通；
6. （二期）"问法不同、意图相同"的请求可通过语义检索命中历史答案；
7. （二期）响应阶段双写回：答案写 Redis（exact），`(question, embedding, answer)` 写向量库（semantic）；
8. （二期）语义链路全 fail-open：embedding/向量库故障只降级为纯精确缓存，**启动期无任何硬失败**；
9. （二期）语义调优参数（TopK/阈值）模块级全局配置、随规则文件热加载。

**明确不做**：多向量库 provider 矩阵（一期仅 Chroma）、多 embedding 厂商适配（仅 OpenAI 兼容协议）、缓存主动失效 API（仅 TTL）、跨轮对话上下文归一。

## 3. 设计原则

- **规则驱动**：沿用 AI 模块惯例，规则文件按 `map[product] → 规则列表` 组织，`Search(product)` + `Cond.Match(req)` 取第一条命中规则；AI 网关只有默认 product，规则间区分完全由 condition 承担（如 `req_body_json_in("model", ...)`）；
- **fail-open**：Redis/embedding/向量库任何一步失败按未命中处理，回写失败仅记日志；启动期配置缺失或 provider 初始化失败只降级不拒绝启动；
- **租户隔离**：缓存键强制带 API Key 的 `key_id` 前缀；向量记录强制带 `tenant_id` metadata 且查询强制按租户过滤；
- **防缓存污染**：不完整/失败的响应、空答案、超限答案一律不写缓存；支持规则级 `disabled` 与请求级跳过头（对两级缓存同时生效）；
- **大小双限**：`maxBodyBytes`（请求体）与 `maxValueBytes`（缓存值）分别限制两个方向的内存开销；
- **精确优先**：Redis 精确命中短路，绝不触发 embedding 调用（零额外延迟与成本）；
- **配置两层**：调优参数（`topK`/`threshold`/`thresholdRelation`）模块级全局一份，置于规则数据文件顶层随 reload 热加载；开关（`enableSemanticCache`）规则级，支撑按路由灰度。

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
                 │     ├─ ① Redis GET ── 命中(exact)：BfeHandlerFinish，模板构造响应
                 │     ├─ ② 规则开启语义？── 否 → 放行
                 │     ├─ ③ embedding.Embed(question)
                 │     ├─ ④ vector.Query（强制 tenant + TTL 过滤）
                 │     ├─ ⑤ 阈值判定 ── 命中(semantic)：BfeHandlerFinish
                 │     └─ 未命中：保存 ctx{key, question, embedding, stream, rule}，放行
                 ▼ 上游模型服务
                 ▼ mod_ai_cache     (HandleReadResponse)   回写缓存
                 │     ├─ 包装响应体，透传+累积，EOF 时提取答案
                 │     ├─ Redis SETEX（同步）
                 │     └─ vector.Upsert（异步 goroutine，值捕获）
                 ▼ mod_body_process (HandleReadResponse)   usage 解析
                 ▼ mod_ai_token_auth (HandleRequestFinish) 扣费（识别 AiCacheHit 跳过）
                 ▼ mod_access_pb3   访问日志（ai_cache_status=hit/hit_semantic/miss/skip）
```

模块注册位置：`bfe/bfe_modules/bfe_modules.go` 中 `mod_ai_route` 之后、`mod_body_process` 之前（两期相同）：

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

Redis 连接参数与 `mod_ai_rate_limit.conf` 保持一致（BNS 名字服务发现实例 + 直连 + 连接池 + 超时）。二期新增 `[embedding]`/`[vector]` 两段（连接信息，静态下发；未配置则语义能力全局关闭）：

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

[embedding]
serviceHost = 127.0.0.1        ; embedding 服务地址（无状态 HTTP 服务，多实例由部署侧 LB 承担）
servicePort = 11434            ; Ollama 默认 11434
useHttps    = false
apiKey      =                  ; Bearer 传递，不落日志
model       = nomic-embed-text
timeoutMs   = 500              ; 单次调用超时，超时按 miss 降级

[vector]
type         = chroma          ; 一期仅支持 chroma
serviceHost  = 127.0.0.1
servicePort  = 8000
apiKey       =
collection   = ai_cache_semantic
timeoutMs    = 300
maxQuestionBytes = 4096        ; 问题超长跳过语义检索（保护 embedding 成本）

[log]
OpenDebug = false
```

> 多实例取舍：`[embedding]`/`[vector]` 不照搬 Redis 的 BNS 多实例发现——embedding 无状态，多实例高可用交给部署侧负载均衡；Chroma 有状态，"多实例随机连"会造成写分散/读不一致，容量不足时优先换原生分布式的 Milvus（provider 接口已预留）。

### 5.2 规则文件 `mod_ai_cache_rule.data`

语义相关配置分两层：顶层 `Semantic` 块（模块级全局调优参数，随规则文件热加载）+ 规则级 `enableSemanticCache` 开关：

```json
{
  "Version": "1.0",
  "Semantic": {
    "topK": 1,
    "threshold": 0.15,
    "thresholdRelation": "lt"
  },
  "Config": {
    "default": [
      {
        "cond": "req_path_in(\"/v1/chat/completions\", false) && req_body_json_in(\"model\", \"deepseek-chat\", false)",
        "cacheKeyStrategy": "lastQuestion",
        "cacheTTL": 3600,
        "enableSemanticCache": true,
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

- 顶层 `Semantic`（可选）：`topK`（默认 1，范围 1-10）、`threshold`（默认 0.15，范围 0-2）、`thresholdRelation`（默认 `lt`，取值 `lt`/`lte`/`gt`/`gte`）；
- 规则级 `enableSemanticCache`（可选，默认 false）：`cacheKeyStrategy=disabled` 的规则上该字段被忽略（不报错）；
- **向后兼容**：旧版规则文件（无 `Semantic` 块、无 `enableSemanticCache`）可正常加载，语义能力关闭；
- 其余规则字段与一期一致：`cacheKeyStrategy`（`lastQuestion` 默认 / `allQuestions` / `disabled`）、`cacheTTL`、`cacheKeyFrom` / `cacheValueFrom` / `cacheStreamValueFrom`（GJSON PATH）、`responseTemplate` / `streamResponseTemplate`（`%s` 占位）、`maxBodyBytes` / `maxValueBytes`。product 查不到时直接放行（与其他 AI 模块一致，不回退 global product）。

## 6. 请求阶段：先精确后语义

### 6.1 精确匹配（一期）

```text
cacheRequestHandler (HandleAfterLocation)
  1. 无 AiBasicInfo / product 无规则 / condition 未命中 → 放行
  2. x-bfe-skip-ai-cache: on → 记 cache_status=skip，放行
  3. 非 application/json → 放行
  4. 读取请求体（> maxBodyBytes 或读不全 → 放行）
  5. 按 cacheKeyStrategy 提取问题，生成缓存键
  6. Redis GET：
     - 命中 → cache_status=hit，AiCacheHit=true，模板构造响应，BfeHandlerFinish
     - 未命中 → 进入 6.2 语义检索（若开启）
```

缓存键格式：

```
{CacheKeyPrefix}:{ClientKeyId}:{sha256(question)[:16]}
```

- `lastQuestion`：默认 PATH `messages.@reverse.0.content`（最后一个用户问题）；
- `allQuestions`：拼接所有 `role=user` 消息的 content；
- `ClientKeyId` 为 API Key 内部标识，未鉴权请求落入 `unknown` 键空间，实现租户隔离。

### 6.2 语义检索（二期）

```text
  6.2 语义分支（Redis 未命中后，规则 enableSemanticCache=true 且全局语义配置存在时）
    a. 问题超长（> maxQuestionBytes）→ 计 semantic_skipped，按 miss 放行
    b. embedding.Embed(question)
       - 成功：向量暂存 ctx（写回阶段复用，不重复计算）
       - 失败/超时 → 计 embedding_err，按 miss 放行（emb=nil）
    c. vector.Query(embedding, tenant, topK, cacheTTL)
       - 失败/超时 → 计 vector_err，按 miss 放行
    d. 阈值判定 compareScore(score, threshold, thresholdRelation)
       - 最优近邻未过阈值 / 空结果 / answer 为空 → miss
       - 过阈值 → cache_status=hit_semantic，AiCacheHit=true，AiCacheSemantic=true，
         AiCacheSimilarity=归一化相似度，模板构造响应（与精确命中同一 buildHitResponse），
         BfeHandlerFinish
    e. 未命中：cache_status=miss，ctx 携带 question/embedding 放行上游
```

要点：

- **精确命中优先**：Redis 命中时 embedding 调用次数为零（有单测断言）；
- 语义检索输入与精确键同源（同一 `extractQuestion`），保证"语义命中的问题必生成一致的精确键"；
- 问题超长保护：`maxQuestionBytes`（默认 4096）限制 embedding 成本，超长只跳过语义、不影响 exact 缓存；
- 阈值语义见 8.4 节，默认取值面向 Chroma cosine distance 语境，上线前须按实际 embedding 模型校准。

命中响应构造（两期共用）：`%s` 替换为 **JSON 转义后**的缓存内容（`strconv.Quote` 去引号），非流式返回 `application/json`，流式返回一段完整 SSE（含 `data:[DONE]`），响应头 `X-Bfe-Ai-Cache: hit` / `hit_semantic`。

## 7. 响应阶段：双写回写

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
     - Redis SETEX（同步，一期逻辑）
     - 二期：规则开启语义且 ctx.Embedding 非 nil → 异步上传向量索引
```

语义索引异步上传（二期）：

```go
if ctx.Rule.EnableSemanticCache && m.semantic != nil && ctx.Embedding != nil {
    question, tenant, emb, value := ctx.Question, tenantOf(req), ctx.Embedding, value
    go func() { // 值捕获，不引用 req；超时由 provider 内部承担（默认 300ms）
        _ = m.semantic.Upload(question, tenant, emb, value) // 失败仅计 vector_err
    }()
}
```

- 异步原因：`writeBack` 运行在 body `Close()`（响应已完整透传），同步上传会阻塞连接释放；
- 请求阶段 embedding 失败（nil）时写回阶段**不补算**——仅写 Redis，保持写回路径轻量；
- 进程退出丢失在途上传可接受（缓存语义，下次 miss 重新写入）。

SSE 解析处理跨 chunk 的 partial message、`[DONE]` 与最后一段 content 同包、首包仅 role 无 content 等边界（对完整累积 buffer 重新分帧，天然免疫网络层拆包差异）。

与 `mod_body_process` / `mod_ai_token_auth` 的包装兼容：本模块先于二者包装 `res.Body`，后装模块的 `bytes_body` / `BodyProcessor` 均通过本包装器透传读取。

## 8. 语义缓存设计要点

### 8.1 Provider 接口与存储解耦

```go
// provider/embedding：文本转向量（实现内部自带超时）
type EmbeddingProvider interface {
    Embed(text string) ([]float32, error)
}

// provider/vector：向量检索与写入
type VectorProvider interface {
    // 必须限定 tenant，且只返回 created_at 在 ttl 内的记录
    Query(embedding []float32, tenant string, topK int, ttl time.Duration) ([]Result, error)
    Upload(item Item) error // 确定性 id（sha256(tenant+question)），同 id 覆盖即 upsert
}
```

- 一期实现：`embedding/openai.go`（OpenAI 兼容 `POST /v1/embeddings`，覆盖 OpenAI/Ollama/vLLM/SiliconFlow 等）、`vector/chroma.go`（Chroma HTTP，`hnsw:space=cosine`）；
- 全局三参数与存储类型解耦：`topK` 是所有 ANN 引擎通用能力；`threshold`/`thresholdRelation` 作用在存储原生分数上，`thresholdRelation` 用于屏蔽不同库分数方向差异（distance 越小越相似 / similarity 越大越相似）。新增向量库 provider 只需实现接口并注明度量语义，全局参数零改动。

### 8.2 租户隔离

- 全模块共用一个 collection（默认 `ai_cache_semantic`），不按租户建 collection；
- 写入时 metadata 强制带 `tenant_id = ClientKeyId`；查询时 `where` 强制 `tenant_id == X` 与 TTL 条件合取——隔离在查询表达式层保证（单测关键断言）；
- 数据布局：`document` = 问题原文；metadata = `{tenant_id, answer, created_at(unix秒)}`；`id` = `sha256(tenant + "\n" + question)` hex（确定性 id，重复写入即幂等 upsert）。

### 8.3 TTL 策略

Chroma 无原生 per-item TTL：写入记录 `created_at`，查询时 `where` 追加 `created_at > now - cacheTTL`（`cacheTTL` 复用规则字段，与精确缓存语义一致）。过期记录物理清理（reaper/GC）为后续增强，一期靠查询期过滤 + collection 整体重置兜底。

### 8.4 阈值语义与校准

- Chroma cosine distance ∈ [0,2]，越小越相似；归一化相似度 `Similarity = 1 - distance` 供日志与调优；
- 默认 `threshold=0.15` + `lt`（distance 语境）为保守值；换向量库类型或 embedding 模型时必须重新校准（`thresholdRelation` 按度量语义重选，阈值数值重定）；
- 调优参数在规则数据文件顶层，改阈值无需动规则、reload 即生效；`ai_cache_similarity` 全量落日志供离线分析。

## 9. 配额/计费协同

`mod_ai_token_auth` 在 `HandleRequestFinish` 做最终扣费。缓存命中时上游未被调用、`usage` 不存在，因此在 `tokenRequestFinishHandler` 中识别 `AiCacheHit` 直接跳过扣减（一期已落地，二期零改动）：

```go
// bfe/bfe_basic/request_ai_basic.go：AiBasicInfo 字段
AiCacheHit        bool    // hit / hit_semantic 时置 true（计费跳过扣减的依据）
AiCacheStatus     string  // hit / hit_semantic / miss / skip，未启用缓存时为空
AiCacheKey        string  // 缓存键，仅 debug 开启时填充
AiCacheSemantic   bool    // 语义命中时置 true（→ 791 ai_cache_semantic）
AiCacheSimilarity float64 // 语义命中归一化相似度（→ 792 ai_cache_similarity）

// bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go
if ctx.aiBasicInfo.AiCacheHit {
    ctx.deducted = true
    return bfe_module.BfeHandlerGoOn
}
```

## 10. 访问日志与监控

### 10.1 访问日志字段（bfe-access-pb）

| 字段 | 编号 | 说明 |
|---|---|---|
| `ai_cache_status` | 789 | `hit` / `hit_semantic` / `miss` / `skip`，未启用缓存时为空 |
| `ai_cache_key` | 790 | 缓存键，仅 debug 开启时记录，避免日志膨胀 |
| `ai_cache_semantic` | 791 | 语义缓存命中标志：命中来自语义缓存时写 true |
| `ai_cache_similarity` | 792 | 语义命中归一化相似度 [0,1]，阈值调优依据；未做语义检索不写 |

### 10.2 模块监控指标

```text
mod_ai_cache.req_total / cache_hit / semantic_hit / cache_miss / cache_skip
mod_ai_cache.redis_err / embedding_err / vector_err
mod_ai_cache.latency_ms / embedding_latency_ms / vector_latency_ms
mod_ai_cache.value_too_large / semantic_skipped
```

`cache_hit`（精确）口径不变，语义命中独立计数，报表可按需拆分 two 口径。

## 11. 非功能性设计

| 维度 | 设计 |
|---|---|
| 性能 | 精确未命中仅一次 Redis GET；语义未命中新增 embedding（封顶 500ms）+ 向量查询（封顶 300ms）；命中路径（精确/语义）均不读上游；embedding 算一次复用两次（检索 + 写回） |
| 安全-租户隔离 | 缓存键强制 `key_id` 前缀；向量记录强制 `tenant_id` metadata + 查询强制过滤 |
| 安全-敏感配置 | Redis 密码、embedding/向量 apiKey 均不进访问日志 |
| 安全-缓存污染 | `disabled` 策略 + 跳过头；错误/不完整/空/超限响应不写缓存；语义命中但 answer 为空视为 miss |
| 容灾 | 全链路 fail-open：Redis/embedding/向量库故障只降级；**启动期**（连接未配置/provider 初始化失败）WARN + 语义全局关闭，BFE 照常启动；TTL + 连接池 + 超时防雪崩 |
| 成本控制 | 仅 miss 时调用 embedding；`maxQuestionBytes` 超长跳过；命中后写回复用向量不重复计算 |

## 12. 验证

- 单元测试（`bfe/bfe_modules/mod_ai_cache/`，含 `provider/embedding`、`provider/vector`）：配置解析、缓存键生成、SSE 解析、命中/未命中/跳过流程、包装器透传、阈值四关系、租户隔离 where 双条件断言、确定性 id 幂等、精确命中零 embedding 调用、embedding 复用、异步上传错误吞噬、启动降级计数；
- 集成测试：SC32 AI 缓存端到端（精确命中/跳过/流式/租户隔离/TTL/计费协同）；语义缓存场景见 `docs/zh_cn/modifications/2026-09-30-ai-cache-semantic-cache/design-changes.md` 第 13 节。
