# mod_ai_cache

## 模块简介

mod_ai_cache 对 AI 请求（OpenAI 协议）提供两级缓存：

- **精确匹配缓存**：请求体中用户问题与历史请求完全一致时，直接返回缓存答案（Redis）；
- **语义缓存**：问题的 embedding 与历史问题的向量相似度超过阈值时命中（embedding 服务 + Chroma 向量库），支持"问法不同、意图相同"的相似问题复用答案。

查询顺序为**精确优先、语义兜底**；两级均支持流式（SSE）与非流式响应的缓存与命中返回。缓存命中时上游模型不被调用，`mod_ai_token_auth` 跳过配额/费用扣减。全链路 fail-open：任何缓存子系统故障只降级为低级缓存或直接放行，不影响主请求。

外部依赖：Redis（精确缓存，必选）；embedding 服务（OpenAI 兼容 `POST /v1/embeddings`）与 Chroma 向量库（语义缓存，可选——未配置或不可用时语义能力自动关闭，精确缓存不受影响）。

## 基础配置

模块基础配置文件说明详见 [mod_ai_cache.conf](../../configuration/mod_ai_cache/mod_ai_cache.conf.md)。

## 规则配置

规则配置文件说明详见 [mod_ai_cache_rule.data](../../configuration/mod_ai_cache/mod_ai_cache_rule.data.md)。

## 工作原理

### 缓存查询流程（HandleAfterLocation）

1. **规则匹配**：按 product 定位规则列表，逐条 condition 匹配取第一条；请求头 `x-bfe-skip-ai-cache: on` 时整个请求不读不写缓存；
2. **提取问题**：按 `cacheKeyStrategy`（`lastQuestion` / `allQuestions`）提取问题文本，生成带租户前缀（`key_id`）的缓存键，实现多租户隔离；
3. **精确查询**：Redis GET，命中即返回模板构造的响应（非流式 JSON / 流式 SSE），不再触发 embedding；
4. **语义检索**（规则开启 `enableSemanticCache` 且全局 `Semantic` 配置存在）：embedding → 向量 TopK 检索（强制 `tenant_id` + TTL 过滤）→ 按 `threshold`/`thresholdRelation` 判定，命中返回缓存答案（状态 `hit_semantic`）；
5. **未命中**：放行上游，将键/问题/向量/规则存入 request context 供回写阶段使用。

### 缓存回写流程（HandleReadResponse）

包装响应体边透传边累积；响应完整读取后：提取答案（非流式按 GJSON PATH / 流式解析 SSE 累积）→ Redis SETEX **同步**写回；语义开启且请求阶段已成功计算向量时，**异步** goroutine 将 `(question, embedding, answer)` 写入向量库（失败仅计数，不影响已完成响应）。空答案、不完整响应、超限答案一律不写。

### 监控指标

| 指标名称 | 类型 | 描述 |
| -------- | ---- | ---- |
| REQ_TOTAL | Counter | 进入模块的请求数 |
| CACHE_HIT | Counter | 精确缓存命中数 |
| SEMANTIC_HIT | Counter | 语义缓存命中数 |
| CACHE_MISS | Counter | 未命中数 |
| CACHE_SKIP | Counter | 跳过缓存请求数 |
| REDIS_ERR | Counter | Redis 错误数 |
| EMBEDDING_ERR | Counter | embedding 调用失败/超时数 |
| VECTOR_ERR | Counter | 向量查询/上传失败数 |
| LATENCY_MS | Counter | Redis 查询耗时累计（毫秒） |
| EMBEDDING_LATENCY_MS | Counter | embedding 调用耗时累计（毫秒） |
| VECTOR_LATENCY_MS | Counter | 向量库操作耗时累计（毫秒） |
| VALUE_TOO_LARGE | Counter | 因超限未缓存次数 |
| SEMANTIC_SKIPPED | Counter | 问题超长跳过语义检索次数 |
