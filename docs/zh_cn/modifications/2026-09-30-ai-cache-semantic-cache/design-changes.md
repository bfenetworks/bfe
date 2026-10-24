# `mod_ai_cache` 语义缓存扩展（二期：embedding + 向量相似度缓存）

## 1. 背景

本文档是语义缓存需求（总体设计见《ai-cache语义缓存实现方案》）在 BFE 数据面的落地方案。

一期已完成 Redis 精确匹配缓存（见 `2026-09-24-ai-cache-exact-match`）：请求体中用户问题与历史请求**完全一致**才能命中。二期在既有模块上**增量扩展语义缓存能力**（Higress `ai-cache` 混合模式）：

**目标**：

- 查询顺序：Redis 精确 GET 优先，未命中时走语义检索（embedding → 向量 TopK → 阈值判定 → 返回答案）；
- 响应阶段双写回：答案继续写 Redis（exact），同时把 `(question, embedding, answer)` 写入向量库（semantic）；
- 支持流式/非流式（复用一期 SSE 能力与命中模板）；
- 语义命中同样跳过配额/费用扣减（复用一期 `AiCacheHit` 机制，`mod_ai_token_auth` 零改动）；
- 全链路 fail-open：embedding 服务或向量库任何故障只降级为纯精确缓存，不影响主请求。

**明确不做**（二期非目标）：

| 不做的事 | 说明 |
|----------|------|
| 多向量库 provider 矩阵 | 一期只做 Chroma 一种实现，接口预留扩展（Milvus/Qdrant/ES 后续按需加） |
| 多 embedding 厂商适配 | 一期只做 OpenAI 兼容 `POST /v1/embeddings`（覆盖 OpenAI / Ollama / vLLM / SiliconFlow / DashScope 兼容模式等） |
| 缓存主动失效 API | 仍只支持 TTL；向量侧靠 `created_at` 查询期过滤 |
| 跨轮对话上下文归一 | 语义检索输入仍是一期 `cacheKeyStrategy` 提取的问题文本 |
| 语义命中率报表 | 报表侧按需后续迭代，本期保证日志字段产出 |

**选型决策摘要**（详见方案文档第 4 节）：

- **向量库选 Chroma**：`bfe_util/redis_client` 基于 BNS 名字服务发现实例后直连后端，但 `Client` 接口仅暴露 12 个命令、无裸命令通道，且按 key 分片路由与索引级 `FT.*` 命令模型不兼容，Redis Stack 向量检索不可行；Chroma 已在团队环境就绪（`environment/chroma-data`），HTTP API 无额外客户端依赖，支持 `where` 元数据过滤（租户隔离与 TTL 过滤的基础）；
- **embedding 走 OpenAI 兼容接口**：一个实现覆盖最多来源；工程写法参照 `mod_ai_intent/decision_client.go`（同步 HTTP + 超时 + fail-open）；
- **答案内联存向量库**（Higress 同款）：语义命中一次向量查询即取回答案，TTL 用查询期过滤统一治理；
- **租户隔离**：单集合 + metadata 强制过滤（`tenant_id` + `created_at`），不按租户建 collection。

## 2. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 配置解析 | 模块配置新增 `[embedding]`、`[vector]` 段及校验 | `bfe/bfe_modules/mod_ai_cache/conf_mod_ai_cache.go`（修改） |
| 规则模型 | 规则新增 `enableSemanticCache`；`topK`/`threshold`/`thresholdRelation` 作为模块级全局配置置于规则数据文件顶层 `Semantic` 块（热加载，向后兼容） | `bfe/bfe_modules/mod_ai_cache/cache_rule_load.go`（修改） |
| 模块装配 | 初始化 embedding/vector provider、Init 降级处理 | `bfe/bfe_modules/mod_ai_cache/mod_ai_cache.go`（修改） |
| 新 provider | embedding 接口 + OpenAI 兼容实现 | `bfe/bfe_modules/mod_ai_cache/provider/embedding/*.go`（新建） |
| 新 provider | vector 接口 + Chroma 实现 | `bfe/bfe_modules/mod_ai_cache/provider/vector/*.go`（新建） |
| 语义封装 | Lookup / Upload / 阈值比较 / 长度保护 / 计时 | `bfe/bfe_modules/mod_ai_cache/semantic.go`（新建） |
| 请求阶段 | 语义检索分支、`extractQuestion` 拆分、ctx 扩展、命中状态 | `bfe/bfe_modules/mod_ai_cache/request.go`（修改） |
| 响应阶段 | 双写回：Redis（同步）+ 向量库（异步 goroutine） | `bfe/bfe_modules/mod_ai_cache/response.go`（修改） |
| 监控 | 新增 SEMANTIC_HIT / EMBEDDING_ERR / VECTOR_ERR 等计数器 | `bfe/bfe_modules/mod_ai_cache/cache_state.go`、`mod_ai_cache.go`（修改） |
| 访问日志 | protobuf 新增 `ai_cache_semantic`（791）、`ai_cache_similarity`（792） | `bfe-access-pb`（独立仓库）、`bfe/bfe_modules/mod_access_pb3/request_log.go`（修改） |
| 基础信息 | `AiBasicInfo` 新增 `AiCacheSemantic`、`AiCacheSimilarity` 字段 | `bfe/bfe_basic/request_ai_basic.go`（修改） |
| 配置样例 | `mod_ai_cache.conf` 样例新增两个配置段 | `bfe/conf/mod_ai_cache/mod_ai_cache.conf`（修改） |
| 文档 | 模块配置文档增补语义缓存章节 | `bfe/docs/zh_cn/configuration/mod_ai_cache/`（修改） |
| 测试 | 单元测试 + 集成测试 | `bfe/bfe_modules/mod_ai_cache/*_test.go`（新增/修改）；集成测试 `bfe/tests/integration/implementation/scenario-SC23-ai-cache-semantic`（新建，mock embedding + mock Chroma） |

模块注册位置、回调注册、执行顺序**均不变**（语义逻辑内聚在 `mod_ai_cache` 既有回调中）。

## 3. 总体查询架构

```text
客户端请求
    ▼
cacheRequestHandler (HandleAfterLocation)        【一期已有】
    - 规则匹配 / skip 头 / content-type / body 读取 / 提取 question
    ▼
① Redis GET {prefix}:{tenant}:{sha256(question)} 【一期已有】
    - 命中 → exact hit（cache_status=hit），模板响应，BfeHandlerFinish
    ▼ 未命中
② rule.EnableSemanticCache？
    - 否 → miss，保存 ctx，放行上游
    ▼ 是
③ embedding.Embed(question)                      【二期新增】
    - 成功：embedding 存入 ctx（响应阶段复用，不重复计算）
    - 失败/超时 → EMBEDDING_ERR，降级 miss（embedding=nil）
    ▼
④ vector.Query(embedding, tenant, topK, ttl)     【二期新增】
    - 失败/超时 → VECTOR_ERR，降级 miss
    ▼
⑤ 阈值判定 compareScore(score, threshold, relation)
    - 未过阈值 → miss
    - 过阈值且 answer 非空 → semantic hit（cache_status=hit_semantic），
      模板响应（复用 buildHitResponse），BfeHandlerFinish
    ▼ 未命中
⑥ 保存 ctx（key/question/embedding/rule），放行上游
    ▼
cacheResponseHandler (HandleReadResponse)          【一期已有】
    ▼
writeBack (body Close 回调)
    - 答案提取（非流式 GJSON / 流式 SSE 累积）       【一期已有】
    - Redis SETEX key answer TTL                   【一期已有】
    - 若 EnableSemanticCache 且 embedding 可用：    【二期新增】
      vector.Upload(question, embedding, answer)   （异步 goroutine，见 7.3）
```

要点：

- 语义缓存是一期流程的**插入式扩展**，不动已验证的精确命中/回写路径；
- 语义检索的问题文本与精确键同源（`extractQuestion(body, rule)`），保证同一条规则下语义命中的问题必然生成一致的精确键；
- 精确命中优先：Redis 命中时**绝不调用 embedding**（测试用 mock 断言零调用）。

## 4. 配置模型

连接信息与一期 Redis 同款处理：放模块静态 conf（conf-agent `CopyFiles` 下发），规则热更新不动它。两个新配置段**整体可选**——不配则语义能力全局关闭；规则里写了 `enableSemanticCache=true` 而未配置连接时，仅记 WARN 日志并忽略该开关，精确缓存不受影响（fail-open）。

### 4.1 模块基础配置 `conf/mod_ai_cache/mod_ai_cache.conf` 新增

`[basic]`、`[redis]`、`[log]` 三段不变，新增：

```ini
[embedding]
serviceHost = 127.0.0.1        ; embedding 服务地址
servicePort = 11434            ; Ollama 默认 11434；OpenAI 为 443（配合 useHttps）
useHttps    = false            ; true 时拼 https://host/v1/embeddings
apiKey      =                  ; 无鉴权服务留空；任何日志不输出明文
model       = nomic-embed-text ; embedding 模型名
timeoutMs   = 500              ; 单次 embedding 调用超时（含排队）

[vector]
type         = chroma          ; 一期仅支持 chroma
serviceHost  = 127.0.0.1
servicePort  = 8000            ; Chroma 默认 8000
apiKey       =                 ; Chroma 默认无鉴权
collection   = ai_cache_semantic ; 集合名
timeoutMs    = 300             ; 单次向量操作超时
maxQuestionBytes = 4096        ; 问题文本超过则跳过语义检索（保护 embedding 成本）
```

**`[embedding]` 配置项**：

| 配置项 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
|--------|------|----------|------|----------|------------|
| Embedding.ServiceHost | String | embedding 服务地址 | 语义启用时 Y | - | 非空字符串 |
| Embedding.ServicePort | Integer | embedding 服务端口 | 语义启用时 Y | - | 1–65535 |
| Embedding.UseHttps | Boolean | 是否使用 HTTPS | N | 默认 false | - |
| Embedding.ApiKey | String | 访问凭证 | N | Bearer 方式传递，日志脱敏 | - |
| Embedding.Model | String | embedding 模型名 | 语义启用时 Y | 请求体 `model` 字段 | 非空字符串 |
| Embedding.TimeoutMs | Integer | 调用超时 | N | 默认 500，超时按 miss 降级 | 1–5000 |

**`[vector]` 配置项**：

| 配置项 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
|--------|------|----------|------|----------|------------|
| Vector.Type | String | 向量库类型 | 语义启用时 Y | 一期仅支持 `chroma` | 枚举：chroma |
| Vector.ServiceHost | String | 向量库地址 | 语义启用时 Y | - | 非空字符串 |
| Vector.ServicePort | Integer | 向量库端口 | 语义启用时 Y | - | 1–65535 |
| Vector.ApiKey | String | 访问凭证 | N | Chroma 默认无鉴权 | - |
| Vector.Collection | String | 集合名 | N | 默认 `ai_cache_semantic` | 非空字符串 |
| Vector.TimeoutMs | Integer | 调用超时 | N | 默认 300 | 1–5000 |
| Vector.MaxQuestionBytes | Integer | 问题文本长度上限 | N | 默认 4096，超限跳过语义检索（exact 缓存不受影响） | 正整数 |

> 多实例扩展（一期不做，配置保持单地址）：embedding 是无状态 HTTP 服务，多实例高可用/负载分散交给部署侧负载均衡（域名/VIP/K8s Service），BFE 配置单入口即可；Chroma 有状态，"多实例随机连"会导致写分散/读不一致，二期容量不足时优先换原生分布式的 Milvus 而非给 Chroma 做客户端分片。未来若需 BFE 侧多地址，`serviceHost` 可平滑扩展为地址列表 + provider 内随机/failover，provider 接口不变。

### 4.2 规则文件 `mod_ai_cache_rule.data` 扩展

语义相关配置分两层：**调优参数为模块级全局配置**，放在文件顶层的 `Semantic` 块（随 `/reload/mod_ai_cache` 热加载）；**开关为规则级字段** `enableSemanticCache`（保留按路由/模型灰度的能力）。`Config: map[product] → 规则列表` 结构不变，**旧版规则文件（无新字段、无 Semantic 块）仍可加载**——加载后语义能力关闭，向后兼容：

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

顶层 `Semantic` 块（全局，模块一份）：

| 字段 | 类型 | 说明 | 默认值 |
|------|------|------|--------|
| `topK` | Integer | 向量检索近邻个数，仅取最优者判定 | 1 |
| `threshold` | Float | 相似度阈值（量纲由 `thresholdRelation` 决定） | 0.15 |
| `thresholdRelation` | String | 阈值比较：`lt`/`lte`（distance 语义，默认，适配 Chroma cosine distance）或 `gt`/`gte`（similarity 语义） | lt |

规则新增字段：

| 字段 | 类型 | 说明 | 默认值 |
|------|------|------|--------|
| `enableSemanticCache` | Bool | 是否启用语义缓存；`cacheKeyStrategy=disabled` 时该字段无效（被忽略，不报错） | false |

> 设计取舍：`topK`/`threshold`/`thresholdRelation` 不做规则级重复配置——它们描述向量检索与判定的全局特性，部署内一份即可；但放 `mod_ai_cache.conf` 静态配置又失去热加载能力（调阈值是高频运维操作），故置于规则数据文件顶层：**语义上模块级，加载机制上热更新**。
>
> `cacheTTL` 复用现有字段：精确缓存走 Redis TTL，语义缓存走查询期 `created_at` 过滤，两者语义一致。
>
> 与 Higress 的对应关系：Higress `ai-cache` 中 `topK`/`threshold`/`thresholdRelation` 位于 `vector` 配置块（插件级全局一份，见 higress `vector/provider.go` 的 `ProviderConfig`）。本设计同样全局一份（仅 `enableSemanticCache` 保留规则级开关用于按路由灰度），存放载体为规则数据文件顶层以便热加载。Higress 默认 `threshold=1000` 面向 Euclidean 语境，本设计默认 `0.15` 面向 Chroma cosine distance，均须按实际 embedding 模型校准（见第 15 节）。

### 4.3 校验规则

规则字段语义（不新增校验项）：

- `enableSemanticCache` 仅在 `cacheKeyStrategy != disabled` 时生效；`disabled` 规则上该字段**被忽略**（不报错，`disabled` 优先级最高），避免组合校验增大规则维护成本。

顶层 `Semantic` 块 Check（存在时）：

- `topK ∈ [1,10]`；`threshold ∈ [0,2]`（覆盖 distance/similarity 两种量纲）；`thresholdRelation ∈ {gt,gte,lt,lte}`。

模块 Init 行为（fail-open，语义配置问题不阻断启动）：

- `[embedding]`/`[vector]` 未配置（或 provider 初始化失败）→ WARN 告警 + 语义能力全局关闭：忽略所有规则的 `enableSemanticCache`，精确缓存正常服务，BFE 照常启动。

### 4.4 热加载

规则文件 reload：`Semantic` 全局块与规则开关随规则表原子切换——语义开关按规则粒度热开/热关（秒级灰度/回退），阈值/TopK 全局热调（无需动规则）。`mod_ai_cache.conf` 变更走进程级 reload 约定，语义连接信息不要求热更新。

## 5. 模块文件结构

```text
bfe/bfe_modules/mod_ai_cache/
├── mod_ai_cache.go              # 修改：装配 embedding/vector provider、语义开关、Init 校验
├── conf_mod_ai_cache.go         # 修改：[embedding]/[vector] 配置解析与校验
├── cache_rule_table.go          # 修改：新增计数器
├── cache_rule_load.go           # 修改：规则开关字段 + 顶层 Semantic 全局块 + Check + setDefaults
├── redis.go                     # 不动
├── request.go                   # 修改：extractQuestion 拆分、语义检索分支、ctx 扩展
├── response.go                  # 修改：writeBack 双写、异步 upload
├── semantic.go                  # 新增：语义缓存封装（Lookup/Upload/阈值比较/长度保护/计时）
├── sse.go                       # 不动
├── provider/
│   ├── embedding/
│   │   ├── provider.go          # 新增：EmbeddingProvider 接口
│   │   └── openai.go            # 新增：OpenAI 兼容 /v1/embeddings 实现
│   └── vector/
│       ├── provider.go          # 新增：VectorProvider 接口 + Item/Result 类型
│       └── chroma.go            # 新增：Chroma HTTP 实现
└── *_test.go                    # 新增/修改：provider、语义分支、隔离、降级用例
```

## 6. Provider 接口设计

### 6.1 EmbeddingProvider（`provider/embedding/provider.go`）

```go
// EmbeddingProvider turns a question text into a dense vector.
// Implementations must enforce the configured timeout internally.
type EmbeddingProvider interface {
    Embed(text string) ([]float32, error)
}
```

OpenAI 兼容实现（`openai.go`）要点：

- `POST {scheme}://{host}:{port}/v1/embeddings`，body `{"model":..., "input": text}`，从 `data[0].embedding` 取 `[]float32`；
- `http.Client` 直连 + `context.WithTimeout`（参照 `mod_ai_intent/decision_client.go`）；响应体限长（如 4MB）防爆破；
- apiKey 经 `Authorization: Bearer` 传递，任何日志不输出明文；
- 维度不做客户端校验：Chroma 集合维度不匹配会报错，按 fail-open 处理。

### 6.2 VectorProvider（`provider/vector/provider.go`）

```go
// Item 是写入向量库的一条缓存记录。
type Item struct {
    ID        string    // sha256(tenant + "\n" + question)，确定性 id → upsert 幂等
    Tenant    string    // = ClientKeyId
    Question  string    // 问题原文
    Answer    string    // 最终答案（内联存储，语义命中直接返回）
    Embedding []float32
    CreatedAt time.Time
}

// Result 是向量检索的一条近邻结果。
type Result struct {
    ID         string
    Question   string
    Answer     string
    Score      float64 // 存储原生分数（distance 或 similarity）
    Similarity float64 // 归一化到 [0,1]，越大越相似，仅供日志
}

type VectorProvider interface {
    // Query 返回与 embedding 最近邻的至多 topK 条记录；
    // 必须限定 tenant，且只返回 created_at 在 ttl 内的记录（ttl<=0 表示不过期）。
    Query(embedding []float32, tenant string, topK int, ttl time.Duration) ([]Result, error)
    // Upload 插入或更新一条记录（同 id 覆盖）。
    Upload(item Item) error
}
```

> 三个全局参数（`topK`/`threshold`/`thresholdRelation`）与向量库类型解耦，对任何 provider 适用（Higress 同样定义在共用 `ProviderConfig` 中，7 种向量库实现共用）：`topK` 是所有 ANN 引擎通用的近邻个数；`threshold`/`thresholdRelation` 作用在 `Result.Score`（存储原生分数）上，`thresholdRelation` 为屏蔽不同库分数方向差异（distance 越小越相似 / similarity 越大越相似）而设。新增向量库 provider 只需实现 `Query`/`Upload` 并返回原生分数、注明度量语义，全局参数无需改动；切换向量库类型时按新库度量语义选择 `thresholdRelation` 并重校 `threshold` 数值。

### 6.3 Chroma 实现要点（`chroma.go`）

| 操作 | 实现 |
|------|------|
| Init | `GET /api/v1/heartbeat` 探测 → `get_or_create_collection(name, metadata={"hnsw:space":"cosine"})`；失败向上返回，由模块 Init 决定降级 |
| Query | `POST /api/v1/collections/{id}/query`，`query_embeddings=[embedding]`、`n_results=topK`、`where={"$and":[{"tenant_id":{"$eq":tenant}},{"created_at":{"$gt":now-ttl}}]}`、`include=["metadatas","documents","distances"]` |
| 距离语义 | Chroma cosine distance ∈ [0,2]，越小越相似；`Similarity = 1 - distance` |
| Upload | `POST /api/v1/collections/{id}/upsert`（单元素数组；同 id 覆盖） |
| 数据布局 | `document`=question 原文；metadata：`tenant_id`（强制过滤）、`answer`（命中直接返回）、`created_at`（unix 秒，TTL 过滤）；`id`=sha256(tenant+"\n"+question) hex |

> 若实测 Chroma 后端对 metadata 值大小敏感（答案最大 1MB），备选 `document`/`metadata.answer` 互换布局，或回退"向量库存问题、Redis 存答案"的间接模式（见方案文档 4.3 节对比）。

### 6.4 语义缓存封装（`semantic.go`）

```go
type semanticCache struct {
    embedding        embedding.EmbeddingProvider
    vector           vector.VectorProvider
    maxQuestionBytes int64
}

// Lookup：embedding + 向量查询 + 阈值判定。
// 任何失败返回 ok=false（调用方降级 miss）；成功返回答案、归一化相似度、算出的向量。
func (s *semanticCache) Lookup(question, tenant string, cfg *SemanticConf) (answer string, similarity float64, emb []float32, ok bool)

// Upload：写向量库；仅记录错误（fail-open）。可安全在 goroutine 中调用。
func (s *semanticCache) Upload(question, tenant string, emb []float32, answer string) error
```

## 7. 请求阶段设计（`request.go` 改造）

### 7.1 重构点

**重构 1**：`buildCacheKey` 当前内部提取问题并立即哈希，语义路径也需要问题原文。拆出：

```go
// extractQuestion 按 cacheKeyStrategy 提取问题原文（一期逻辑原样抽出）
func extractQuestion(body []byte, rule *ProductRuleConf) (string, error)
// buildCacheKey 改为基于 extractQuestion 的结果生成哈希键
```

**重构 2**：`aiCacheContext` 扩展，供响应阶段复用：

```go
type aiCacheContext struct {
    Key       string
    Question  string    // 语义写回用（新增）
    Embedding []float32 // 请求阶段已算好的向量，响应阶段免重算（新增，可能为 nil）
    Stream    bool
    Rule      *ProductRuleConf
}
```

### 7.2 语义分支伪代码

插在现有一期"Redis GET 未命中"之后、"保存 ctx 放行"之前：

```go
// 6. exact lookup in redis（一期现有代码）
if value, hit := m.redisCache.Get(key); hit {
    ... // exact hit：CACHE_HIT、cache_status=hit，buildHitResponse，BfeHandlerFinish
}

// 6.5 语义检索（二期新增）
var emb []float32
semCfg := m.ruleTable.getSemantic() // 全局语义配置（Semantic 块，可能为 nil）
if rule.EnableSemanticCache && m.semantic != nil && semCfg != nil && question != "" {
    if len(question) <= m.semanticMaxQuestionBytes {
        answer, similarity, emb0, ok := m.semantic.Lookup(question, tenantOf(req), semCfg)
        emb = emb0 // 无论是否语义命中，成功算出的向量留给写回复用
        if ok {
            m.ruleTable.incSemanticHit()
            m.state.Inc("SEMANTIC_HIT", 1)
            m.setCacheStatus(req, CacheStatusHitSemantic) // AiCacheHit=true 一并设置
            m.setCacheSimilarity(req, similarity)
            return bfe_module.BfeHandlerFinish, m.buildHitResponse(req, rule, answer, stream)
        }
    }
}

// 7. miss：ctx 带 question/embedding，放行（一期现有代码扩展）
setAiCacheContext(req, &aiCacheContext{Key: key, Question: question,
    Embedding: emb, Stream: stream, Rule: rule})
```

### 7.3 边界处理

| 场景 | 行为 |
|------|------|
| embedding 失败/超时 | 记 `EMBEDDING_ERR`，`emb=nil`，miss；写回阶段仅写 Redis |
| 向量查询失败/超时 | 记 `VECTOR_ERR`，miss |
| 空结果 / 未过阈值 | 普通 miss（DEBUG 日志） |
| 命中但 `answer` 为空 | 视为 miss（防御脏数据） |
| question 超长 | 跳过语义（exact 不受影响），DEBUG 日志 |
| 算过 embedding 但最终 miss | 向量留给 ctx，写回阶段直接复用，**不重算** |

### 7.4 命中响应

语义命中与精确命中**完全复用** `buildHitResponse`（模板、`jsonQuote`、SSE 构造一期已就绪），区别仅是答案来源与状态标记。流式客户端同样收到 `streamResponseTemplate` 拼出的整段 SSE。

## 8. 响应阶段设计（`response.go` 改造）

`writeBack` 在一期 Redis 写回之后追加向量写回：

```go
func (m *ModuleAiCache) writeBack(req *bfe_basic.Request, ctx *aiCacheContext, body []byte, complete bool) {
    // ... 一期现有逻辑：complete 校验、答案提取、空值/大小限制 ...

    m.redisCache.Setex(ctx.Key, []byte(value), ctx.Rule.CacheTTL)   // 一期现有

    // 二期：语义索引双写（异步）
    if ctx.Rule.EnableSemanticCache && m.semantic != nil && ctx.Embedding != nil {
        question, tenant := ctx.Question, tenantOf(req)
        go func() { // 值捕获；带超时上下文；结果只记日志/指标
            if err := m.semantic.Upload(question, tenant, ctx.Embedding, value); err != nil {
                m.ruleTable.incVectorErr()
            }
        }()
    }
}
```

设计说明：

1. **异步上传**：`writeBack` 运行在 body `Close()`（响应已完整透传给客户端），同步上传会阻塞连接释放；改为 goroutine + 独立超时上下文（如 1s），只捕获值、不引用 req，规则 reload/请求生命周期结束后依然安全；
2. **最佳 Effort**：上传失败只计数告警，不影响已完成的响应；进程退出丢失在途上传可接受（缓存语义）；
3. **embedding 复用**：`ctx.Embedding` 非 nil 时不重算；请求阶段 embedding 失败（nil）时一期行为是仅写 Redis，写回阶段是否补算 embedding 以补全语义索引列入待决策（建议一期不补算，观察 `EMBEDDING_ERR` 再议）。

## 9. 配额/计费协同

机制不变：`AiBasicInfo.AiCacheHit == true` 时 `mod_ai_token_auth` 跳过扣减（一期已落地，零改动）。二期新增状态值：

| 状态（`ai_cache_status`） | 触发 | `AiCacheHit` | 计费 |
|---|---|---|---|
| `hit` | Redis 精确命中（一期） | true | 跳过扣减 |
| `hit_semantic` | 语义检索命中（二期新增） | true | 跳过扣减（现有逻辑直接生效） |
| `miss` | 两级均未命中 | false | 正常扣减 |
| `skip` | 跳过头 | false | 正常扣减 |

`setCacheStatus` 扩展：设置 `hit_semantic` 时同步置 `AiCacheHit=true`、`AiCacheSemantic=true`，并填充 `AiCacheSimilarity`。

## 10. 访问日志与监控

### 10.1 访问日志字段

现有 789 `ai_cache_status`、790 `ai_cache_key`（一期已占用）。新增：

| 字段 | 编号 | 类型 | 说明 |
|---|---|---|---|
| `ai_cache_semantic` | 791 | optional bool | 1 = 本次命中来自语义缓存（exact 命中与 miss 不填） |
| `ai_cache_similarity` | 792 | optional double | 语义命中归一化相似度 [0,1]，阈值调优依据；未做语义检索不填 |

改动：`bfe-access-pb`（独立仓库）`bfe_access.proto` 追加字段并重新生成；`bfe/bfe_modules/mod_access_pb3/request_log.go` 从 `AiBasicInfo.AiCacheSemantic` / `AiCacheSimilarity` 读取填充。`AiBasicInfo` 新增两个字段（`bfe_basic/request_ai_basic.go`）。

### 10.2 模块监控指标（`cache_state.go` 扩展）

```text
mod_ai_cache.req_total             // 进入模块的请求数（一期已有）
mod_ai_cache.cache_hit             // 精确命中数（一期已有）
mod_ai_cache.semantic_hit          // 语义命中数（新增）
mod_ai_cache.cache_miss            // 未命中数（一期已有）
mod_ai_cache.cache_skip            // 跳过数（一期已有）
mod_ai_cache.redis_err             // Redis 错误数（一期已有）
mod_ai_cache.embedding_err         // embedding 调用失败/超时（新增）
mod_ai_cache.vector_err            // 向量查询/上传失败（新增）
mod_ai_cache.latency_ms            // Redis 查询耗时（一期已有）
mod_ai_cache.embedding_latency_ms  // embedding 调用耗时累计（新增）
mod_ai_cache.vector_latency_ms     // 向量调用耗时累计（新增）
mod_ai_cache.value_too_large       // 因超限未缓存次数（一期已有）
mod_ai_cache.semantic_skipped      // 因问题超长跳过语义的次数（新增，可选）
```

`CACHE_HIT`（精确）口径不变，命中率报表区分 exact/semantic 两个口径。

## 11. 非功能性设计

| 维度 | 设计 |
|---|---|
| 性能 | 未命中请求新增 embedding（典型 50–300ms）+ 向量查询（10–50ms），均严格超时封顶（默认 500ms/300ms）；命中路径不变（一次向量查询即返回）；embedding 成功即缓存于 ctx，写回阶段零重复计算 |
| 安全-租户隔离 | 向量记录强制带 `tenant_id` metadata；查询强制 `where tenant_id == X` 与 TTL 条件合取，隔离在查询表达式层保证；不按租户建 collection |
| 安全-敏感配置 | embedding apiKey、向量库 apiKey 日志脱敏；Redis 密码处理不变 |
| 安全-缓存污染 | 语义开关按规则粒度，工具调用多/随机性强/时效性路由不开启；`x-bfe-skip-ai-cache` 跳过头对两级缓存同时生效 |
| 容灾 | **fail-open**：embedding/向量库故障只降级为纯精确缓存；Chroma 初始化失败告警 + 语义关闭（BFE 正常启动）；上传失败不影响已完成响应 |
| 成本控制 | 仅 miss 时调用 embedding；`maxQuestionBytes` 限制超长问题；监控 `EMBEDDING_ERR` 与调用量 |

## 12. 开发任务拆分（WBS）

| 编号 | 任务 | 主要改动文件 | 说明 |
|------|------|--------------|------|
| S1 | embedding provider | `provider/embedding/provider.go`、`openai.go` | 接口 + OpenAI 兼容实现；httptest mock 覆盖成功/超时/非 200/畸形响应 |
| S2 | vector provider（Chroma） | `provider/vector/provider.go`、`chroma.go` | 接口 + collection 初始化 + Query（where 过滤）+ upsert |
| S3 | 模块配置扩展 | `conf_mod_ai_cache.go` | `[embedding]`/`[vector]` 段解析、默认值、校验 |
| S4 | 规则与全局配置模型扩展 | `cache_rule_load.go` | 规则新增 `enableSemanticCache`；顶层 `Semantic` 全局块（File/Mem 类型、Check、setDefaults、向后兼容） |
| S5 | 语义封装 | `semantic.go` | Lookup/Upload/阈值比较/长度保护/计时 |
| S6 | 请求阶段改造 | `request.go` | extractQuestion 拆分、语义分支、ctx 扩展、hit_semantic 状态 |
| S7 | 响应阶段改造 | `response.go` | 异步双写、upload 超时上下文 |
| S8 | 监控与日志 | `cache_state.go`、`mod_ai_cache.go`、`bfe-access-pb`、`mod_access_pb3/request_log.go`、`bfe_basic/request_ai_basic.go` | 新计数器 + proto 791/792 字段 + 映射 |
| S9 | 模块装配 | `mod_ai_cache.go` | provider 初始化、Init 降级处理 |
| S10 | 计费验证 | — | 验证 `hit_semantic` 跳过扣减（预期零改动，仅测试） |
| S11 | 单元测试 | `*_test.go` | 阈值四关系、租户隔离、embedding 失败降级、异步上传、规则校验 |
| S12 | 集成测试 | `bfe/tests/integration/implementation/scenario-SC23-ai-cache-semantic` | 真实 BFE + miniredis + mock embedding + mock Chroma（httptest 进程内） |
| S13 | 文档与样例 | `bfe/docs/zh_cn/configuration/mod_ai_cache/`、`conf/mod_ai_cache/` | 配置说明、部署样例、阈值调优手册 |

依赖关系：S3/S4 → S1/S2 → S5 → S9 → S6/S7 → S8 → S10；S11 随各任务并行；S12 依赖 S6–S9 完成；S13 收尾。S8 依赖 `bfe-access-pb` 仓库先行合入。

预估 S1–S13 共 **10–12 个开发日**（控制面 ai-gateway-api 侧另有约 3.5 天，见方案文档第 9 节，不在本仓库）。

## 13. 测试方案

### 13.1 单元测试重点

- `compareScore`：四种 thresholdRelation × 命中/不命中；
- `extractQuestion` 与一期键生成的一致性（同一 body 提取结果不变）；
- Chroma provider：mock HTTP 验证 where 子句**必含 tenant + TTL 两个条件**（防隔离回归的关键断言）、upsert 幂等 id；
- embedding provider：超时、非 200、缺字段的 fail-open；
- 请求流程：exact 命中优先于语义（Redis 命中时 embedding 调用次数为零）；
- 响应流程：`ctx.Embedding` 复用（不重复 Embed）、异步 upload 的错误吞噬。

### 13.2 集成测试（`bfe/tests/integration/implementation/scenario-SC23-ai-cache-semantic`）

参照 `mock_decision_service` 的 mock 外部服务模式（均已实现于 `bfe/tests/integration/common/`）：

| 组件 | 方案 |
|------|------|
| BFE / Redis | 真实进程 + miniredis（复用 SC20 基建） |
| embedding 服务 | `mock_embedding_service.go`：OpenAI 兼容 `/v1/embeddings`，关键字→向量脚本，可注入故障 |
| Chroma | `mock_chroma_service.go`：Chroma API 子集，**真实 cosine distance 排序 + where 表达式求值**（租户/TTL 断言的基础） |

用例：① exact 命中不触发 embedding；② 语义命中返回缓存答案且 `ai_cache_status=hit_semantic`；③ 阈值外不命中；④ 租户 A 写入、租户 B 同问题不命中；⑤ embedding 故障降级为纯精确流程且主请求成功；⑥ 流式语义命中 SSE 完整；⑦ TTL 过期后语义不命中；⑧ 命中请求不计费。

## 14. 风险与应对

| 风险 | 影响 | 应对 |
|------|------|------|
| embedding 同步调用增加 miss 路径 TTFT | 未命中请求延迟上升 | 超时封顶（默认 500ms）；只对高重复意图路由开语义；监控 `EMBEDDING_LATENCY_MS`；`x-bfe-skip-ai-cache` 秒级旁路 |
| 误命中（形似神不同的问题返回旧答案） | 答案质量事故 | 保守默认阈值（0.15）；`ai_cache_similarity` 全量落日志供复盘；按路由灰度；敏感路由不开语义 |
| Chroma 成为新故障点 | 语义能力失效 | fail-open 降级纯精确缓存；生产部署考虑 Chroma 副本或后续切 Milvus |
| 向量库无限增长 | 存储膨胀 | TTL 查询期过滤 + 后续 reaper/GC；`maxQuestionBytes` 限制条目规模 |
| embedding 服务成本 | 调用费/资源消耗 | 仅 miss 时调用 + 写回复用 + 超长问题跳过；监控调用量 |
| 跨租户串答案 | 安全事故 | metadata 强制 where 过滤 + 专项单测/集成用例（13.1/13.2） |
| 异步上传丢失 | 个别答案未入语义索引 | 缓存语义允许丢失，下次 miss 时重新写入 |
| 默认阈值不适配所选 embedding 模型 | 命中过高（误命中）或过低（无收益） | 上线前用真实流量采样校准；阈值是全局配置（规则数据文件顶层 `Semantic` 块），热加载可调 |

## 15. 上线路径与灰度建议

1. **依赖就绪**：Chroma + embedding 服务（开发用 Ollama）可用，`mod_ai_cache.conf` 新增段下发；
2. **阈值校准**：抽取近 7 天目标路由真实问题，离线计算两两相似度分布，把 `threshold` 定在"重复意图 P90 相似度"之下（预期 0.1–0.2 区间）；
3. **小流量灰度**：仅对 1–2 条低敏路由开 `enableSemanticCache=true`，观察 3–5 天：
   - `SEMANTIC_HIT / (REQ_TOTAL - CACHE_HIT)`（语义命中率）；
   - `ai_cache_similarity` 分布与人工抽检误命中率；
   - `EMBEDDING_ERR`、`EMBEDDING_LATENCY_MS`、embedding 调用量成本；
4. **扩面或回退**：达标则逐路由放开；异常则规则热关语义开关（秒级），精确缓存不受影响；
5. **报表迭代**：稳定运行后评估在数据报表中拆分 exact/semantic 命中口径（本期不阻塞）。

## 16. 参考文档

- 总体方案：《ai-cache语义缓存实现方案》（需求侧总体设计文档）
- 一期改造：`bfe/docs/zh_cn/modifications/2026-09-24-ai-cache-exact-match/design-changes.md`
- 需求蓝本：《ai-cache需求分析.md》（Higress `ai-cache` 拆解）
- Higress 源码参照：`higress/plugins/wasm-go/extensions/ai-cache/`（`main.go`、`core.go`、`embedding/`、`vector/`）
- 工程先例：`bfe/bfe_modules/mod_ai_intent/decision_client.go`（外部推理服务调用）
