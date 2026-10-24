# TC-07 语义 TTL 过期

## 用例编号与名称

TC-07 语义 TTL 过期

## 所属场景

SC23 AI 缓存语义缓存

## 版本声明

- `bfe`：当前源码版本（含 `mod_ai_cache` 语义缓存二期）

## 测试目的

验证语义缓存 TTL 治理：查询期按 `created_at` 过滤——`created_at` 超出规则 TTL（3600s）的向量记录被 where 条件过滤，相同问题不命中过期答案，重新回源取新答案。

## 运行模式

单组件模式：真实 `bfe` 进程 + 内存 Redis + 进程内 embedding mock / Chroma mock。

## 前置条件

1. 同 TC-01 前置条件 1-4。
2. bfe 启动前通过 `UpsertRecord` 向 Chroma mock 注入一条"过期记录"：同租户（`cache_key_id`）、同问题（`what is bfe?`）、同向量 [1,0,0]，但 `created_at` 为 2 小时前（超过 3600s TTL），答案为哨兵值 `STALE ANSWER MUST NOT BE SERVED`。

## 配置构造

- 同 TC-01（`Semantic` 块 `topK=1`/`threshold=0.15`/`thresholdRelation=lt`，规则 `enableSemanticCache=true`、`cacheTTL=3600`）。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body |
|------|------|------|---------------|------|
| 1 | `cache.example.org` | `/v1/chat/completions` | `Bearer ak_cache` | `{"model":"deepseek-chat","messages":[{"role":"user","content":"what is bfe?"}]}` |

## 预期结果

- 返回 200，响应体**不含**哨兵答案 `STALE ANSWER MUST NOT BE SERVED`（过期记录未被吐出）；
- 响应体含新鲜上游答案 `BFE is a layer-7 load balancer`；
- **无** `X-Bfe-Ai-Cache` 响应头（过期记录被过滤，视为 miss）；
- `backend=1`（重新回源）；`embedding=1`、`query=1`（语义检索执行，`created_at $gt` 过滤生效）。

## 清理

停止 `bfe` 进程、mock 后端、embedding mock、Chroma mock 与内存 Redis，删除临时目录。
