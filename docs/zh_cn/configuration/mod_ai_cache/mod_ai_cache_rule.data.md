# mod_ai_cache 缓存规则配置

## 配置简介

`mod_ai_cache_rule.data` 是 `mod_ai_cache` 模块的缓存规则文件，按 product 组织规则列表。请求处理时先按 `req.Route.Product` 定位规则列表，再逐条匹配 condition，取第一条命中的规则；product 查不到时直接放行（与其他 AI 模块一致，不回退 global product）。

规则间的区分完全由 condition 承担（如用 `req_body_json_in("model", ...)` 区分不同模型）；`cacheKeyStrategy` 为 `disabled` 的规则表示匹配后不缓存。

文件顶层可选的 `Semantic` 块是**模块级全局语义配置**（topK / 阈值 / 阈值比较方向），随规则文件热加载；规则的 `enableSemanticCache` 是按规则粒度的语义缓存开关，用于按路由/模型灰度。

## 配置描述

| 配置项 | 类型 | 参数含义 | 必填 | 补充描述 |
| ------ | ---- | -------- | ---- | -------- |
| Version | String | 规则版本号 | Y | - |
| Semantic | Object | 全局语义配置块 | N | 见下表；缺省时语义能力关闭（向后兼容旧版规则文件） |
| Config | Map\<String, Array\> | product 到规则列表的映射 | Y | key 为 product 名，须与 `host_rule.data` 解析出的 product 一致 |
| cond | String | 规则匹配条件 | Y | 条件表达式语法见[条件原语](../condition/condition_grammar.md) |
| cacheKeyStrategy | String | 缓存键生成策略 | N | `lastQuestion`（默认，取最后一个用户问题）/ `allQuestions`（拼接所有用户问题）/ `disabled`（禁用缓存） |
| cacheTTL | Integer | 缓存 TTL（秒） | N | 默认取基础配置 `DefaultCacheTTL`（3600）；同时用于向量检索的 `created_at` 过期过滤 |
| enableSemanticCache | Boolean | 是否启用语义缓存 | N | 默认 false；仅在 `cacheKeyStrategy != disabled` 时生效，disabled 规则上被忽略（不报错）；模块未配置 `[embedding]`/`[vector]` 或全局 `Semantic` 块缺失时该开关无效 |
| cacheKeyFrom | String | 缓存键提取 GJSON PATH | N | 覆盖 `lastQuestion` 策略默认的 `messages.@reverse.0.content` |
| cacheValueFrom | String | 非流式答案提取 GJSON PATH | N | 默认 `choices.0.message.content` |
| cacheStreamValueFrom | String | 流式答案提取 GJSON PATH | N | 默认 `choices.0.delta.content` |
| cacheToolCallsFrom | String | 工具调用提取 GJSON PATH | N | 预留字段，简化版暂未启用 |
| responseTemplate | String | 命中缓存时返回的响应模板 | N | `%s` 为缓存内容占位，默认见配置示例 |
| streamResponseTemplate | String | 命中流式缓存时返回的 SSE 模板 | N | `%s` 为缓存内容占位，默认见配置示例 |
| maxBodyBytes | Integer | 请求体大小上限（字节） | N | 默认 1048576；超限不缓存 |
| maxValueBytes | Integer | 缓存值大小上限（字节） | N | 默认 1048576；超限不写缓存 |

顶层 `Semantic` 块字段（全局一份，语义上模块级、加载机制上随规则热更新）：

| 字段 | 类型 | 参数含义 | 默认值 | 合法性条件 |
| ---- | ---- | -------- | ------ | ---------- |
| topK | Integer | 向量检索近邻个数，仅取最优者判定 | 1 | 1–10 |
| threshold | Float | 相似度阈值（量纲由 thresholdRelation 决定） | 0.15 | 0–2（覆盖 distance/similarity 两种量纲） |
| thresholdRelation | String | 阈值比较方向：`lt`/`lte`（distance 语义，越小越相似，适配 Chroma cosine distance）或 `gt`/`gte`（similarity 语义，越大越相似） | lt | lt / lte / gt / gte |

## 配置示例

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

## 关键行为说明

- **查询顺序**：Redis 精确 GET 优先（命中即返回，绝不调用 embedding）；未命中且规则开启 `enableSemanticCache`、模块已配置 `[embedding]`/`[vector]` 且存在全局 `Semantic` 块时，依次执行 embedding → 向量 TopK 检索 → 阈值判定，通过且答案非空则语义命中。
- **写回**：响应完成后答案同步写 Redis（exact），同时异步 goroutine 把 `(question, embedding, answer)` 写入向量库；异步上传失败仅计数告警，不影响已完成响应。请求阶段已成功计算的 embedding 在写回阶段直接复用，不重算。
- **租户隔离**：缓存键强制带 API Key 的 `key_id` 前缀；向量记录强制带 `tenant_id` metadata，检索强制 `tenant_id == X` 与 `created_at > now - cacheTTL` 两个条件合取，不同租户不会读到彼此的缓存答案。
- **跳过头**：请求头 `x-bfe-skip-ai-cache: on` 时，当前请求既不读缓存也不写缓存（对两级缓存同时生效），访问日志记录 `ai_cache_status=skip`。
- **命中模板**：缓存命中时（精确与语义共用模板），`%s` 会被替换为 JSON 转义后的缓存内容；流式命中返回一段完整 SSE（含 `data:[DONE]`）。
- **计费协同**：命中缓存的请求在 `mod_ai_token_auth` 中跳过 token/费用扣减。精确命中访问日志记录 `ai_cache_status=hit`；语义命中记录 `ai_cache_status=hit_semantic`，并额外填充 `ai_cache_semantic=true` 与 `ai_cache_similarity`（归一化相似度 [0,1]，阈值调优依据）。
- **降级（fail-open）**：embedding 服务或向量库任何故障只降级为纯精确缓存——embedding 失败/超时按 miss 处理（记 `embedding_err`）、向量查询失败按 miss 处理（记 `vector_err`）、向量上传失败仅计数告警；Chroma 初始化失败时语义能力全局关闭并记 WARN，BFE 正常启动。
- **阈值调优**：`threshold`/`thresholdRelation` 作用在向量库原生分数上。Chroma 返回 cosine distance（[0,2]，越小越相似），故默认 `lt 0.15`；若换用返回 similarity 的向量库，应改用 `gt`/`gte` 并重校阈值。上线前建议用真实流量采样校准阈值（见模块设计文档第 15 节）。
- **监控指标**：`mod_ai_cache` 计数器含 `semantic_hit`（语义命中）、`embedding_err`、`vector_err`、`embedding_latency_ms`、`vector_latency_ms`、`semantic_skipped`（问题超长跳过语义）等。
